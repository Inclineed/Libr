package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ledger/internal/ledger"
)

// envDuration reads a duration from an environment variable, returning
// defaultVal if the variable is absent or cannot be parsed.
func envDuration(key string, defaultVal time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("warning: invalid %s=%q, using default %s", key, s, defaultVal)
		return defaultVal
	}
	return d
}

// envInt reads an integer from an environment variable, returning defaultVal
// if the variable is absent or cannot be parsed.
func envInt(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("warning: invalid %s=%q, using default %d", key, s, defaultVal)
		return defaultVal
	}
	return n
}

func main() {
	port := os.Getenv("LEDGER_PORT")
	if port == "" {
		port = "9000"
	}

	// --- Timing / TTL config -------------------------------------------
	// LEDGER_ENTRY_TTL: window used to filter live mods and nodes in GET
	//   responses (e.g. "3m"). Zero / unset returns all entries.
	// LEDGER_RELAY_TTL: duration after which a relay is evicted (in-memory
	//   only). Zero / unset disables relay eviction.
	// LEDGER_CLEANUP_INTERVAL: how often the background cleaner runs.
	entryTTL := envDuration("LEDGER_ENTRY_TTL", 3*time.Minute)
	relayTTL := envDuration("LEDGER_RELAY_TTL", 0)
	cleanupInterval := envDuration("LEDGER_CLEANUP_INTERVAL", 90*time.Second)

	// Signal context — used for clean shutdown and stopping background jobs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Storage backend -----------------------------------------------
	// MONGO_URI: if set, use MongoDB (database "Addrs") for persistence.
	//   Mirrors the schema of the legacy JS relay server.
	// If unset, fall back to in-memory storage (data lost on restart).
	var stores ledger.LedgerStores
	var mongoStore *ledger.MongoStore
	var inMemStore *ledger.InMemoryStore

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI != "" {
		ms, err := ledger.NewMongoStore(ctx, mongoURI)
		if err != nil {
			log.Fatalf("failed to connect to MongoDB: %v", err)
		}
		mongoStore = ms

		// Create indexes; use entryTTL as the server-side TTL for nodes/mods.
		if err := mongoStore.EnsureIndexes(ctx, entryTTL, entryTTL); err != nil {
			log.Printf("warning: failed to ensure MongoDB indexes: %v", err)
		}

		// Application-level expiry sweep (belt-and-suspenders alongside TTL indexes).
		mongoStore.StartCleanup(ctx, cleanupInterval, entryTTL)

		stores = ledger.LedgerStores{
			Relays: mongoStore,
			Nodes:  mongoStore,
			Mods:   mongoStore,
		}
		log.Printf("storage: MongoDB uri=%s", mongoURI)
	} else {
		if relayTTL > 0 {
			inMemStore = ledger.NewInMemoryStoreWithTTL(relayTTL)
			log.Printf("storage: in-memory with TTL ttl=%s cleanup_interval=%s", relayTTL, cleanupInterval)
		} else {
			inMemStore = ledger.NewInMemoryStore()
			log.Printf("storage: in-memory (no TTL eviction)")
		}
		inMemStore.StartCleanup(ctx, cleanupInterval)
		stores = ledger.LedgerStores{
			Relays: inMemStore,
			Nodes:  inMemStore,
			Mods:   inMemStore,
		}
	}

	// --- Rate limiting --------------------------------------------------
	// LEDGER_RATE_LIMIT: max requests per IP per window (default: 60).
	// LEDGER_RATE_WINDOW: window duration (default: 1m).
	rateLimit := envInt("LEDGER_RATE_LIMIT", 60)
	rateWindow := envDuration("LEDGER_RATE_WINDOW", time.Minute)

	var rl *ledger.RateLimiter
	if rateLimit > 0 {
		rl = ledger.NewRateLimiter(rateLimit, rateWindow)
		log.Printf("rate limiting enabled: limit=%d window=%s", rateLimit, rateWindow)
	}

	// --- Signature validation -------------------------------------------
	// LEDGER_VALIDATE_SIGNATURES: set to "true" to require Ed25519
	// signatures on PUT /relays.
	validateSigs := os.Getenv("LEDGER_VALIDATE_SIGNATURES") == "true"
	if validateSigs {
		log.Printf("relay signature validation enabled")
	}

	cfg := ledger.ServerConfig{
		RateLimiter:        rl,
		ValidateSignatures: validateSigs,
		EntryTTL:           entryTTL,
	}

	handler := ledger.NewServerWithConfig(stores, cfg)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("ledger server starting on :%s", port)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
			os.Exit(1)
		}

		// Disconnect MongoDB cleanly if in use.
		if mongoStore != nil {
			if err := mongoStore.Close(shutdownCtx); err != nil {
				log.Printf("failed to close MongoDB connection: %v", err)
			}
		}
	case err := <-errCh:
		if err != nil {
			log.Printf("server error: %v", err)
			os.Exit(1)
		}
	}
}
