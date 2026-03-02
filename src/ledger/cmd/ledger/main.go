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

	// --- Relay TTL -------------------------------------------------------
	// LEDGER_RELAY_TTL: duration after which a relay is considered stale.
	//   Example: "30m", "1h". Zero / unset disables TTL eviction.
	// LEDGER_CLEANUP_INTERVAL: how often the background cleaner runs.
	//   Default: 5 minutes.
	relayTTL := envDuration("LEDGER_RELAY_TTL", 0)
	cleanupInterval := envDuration("LEDGER_CLEANUP_INTERVAL", 5*time.Minute)

	var store *ledger.InMemoryStore
	if relayTTL > 0 {
		store = ledger.NewInMemoryStoreWithTTL(relayTTL)
		log.Printf("relay TTL enabled: ttl=%s cleanup_interval=%s", relayTTL, cleanupInterval)
	} else {
		store = ledger.NewInMemoryStore()
	}

	// --- Rate limiting ---------------------------------------------------
	// LEDGER_RATE_LIMIT: max requests per IP per window (default: 60).
	// LEDGER_RATE_WINDOW: window duration (default: 1m).
	rateLimit := envInt("LEDGER_RATE_LIMIT", 60)
	rateWindow := envDuration("LEDGER_RATE_WINDOW", time.Minute)

	var rl *ledger.RateLimiter
	if rateLimit > 0 {
		rl = ledger.NewRateLimiter(rateLimit, rateWindow)
		log.Printf("rate limiting enabled: limit=%d window=%s", rateLimit, rateWindow)
	}

	// --- Signature validation --------------------------------------------
	// LEDGER_VALIDATE_SIGNATURES: set to "true" to require Ed25519
	// signatures on PUT /relays. peer_id must be the base64-encoded
	// 32-byte Ed25519 public key; the request must include a "signature"
	// field (base64 Ed25519 sig over "<peer_id>:<ws_address>").
	validateSigs := os.Getenv("LEDGER_VALIDATE_SIGNATURES") == "true"
	if validateSigs {
		log.Printf("relay signature validation enabled")
	}

	cfg := ledger.ServerConfig{
		RateLimiter:        rl,
		ValidateSignatures: validateSigs,
	}

	handler := ledger.NewServerWithConfig(store, cfg)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// Start TTL cleanup goroutine (no-op when TTL is disabled).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store.StartCleanup(ctx, cleanupInterval)

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
	case err := <-errCh:
		if err != nil {
			log.Printf("server error: %v", err)
			os.Exit(1)
		}
	}
}
