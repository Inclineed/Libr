package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ledger/internal/ledger"
)

func main() {
	port := os.Getenv("LEDGER_PORT")
	if port == "" {
		port = "9000"
	}

	// --- relay TTL -----------------------------------------------------------
	relayTTL := ledger.DefaultRelayTTL
	if raw := strings.TrimSpace(os.Getenv("RELAY_TTL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			relayTTL = d
		} else {
			log.Printf("invalid RELAY_TTL %q, using default %s", raw, relayTTL)
		}
	}

	// --- rate limiting -------------------------------------------------------
	cfg := ledger.DefaultServerConfig()
	if raw := strings.TrimSpace(os.Getenv("RATE_LIMIT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			cfg.RateLimit = n
		} else {
			log.Printf("invalid RATE_LIMIT %q, using default %d", raw, cfg.RateLimit)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RATE_WINDOW")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			cfg.RateWindow = d
		} else {
			log.Printf("invalid RATE_WINDOW %q, using default %s", raw, cfg.RateWindow)
		}
	}

	// -------------------------------------------------------------------------
	// Build the signal context first so cleanup goroutines respect shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := ledger.NewInMemoryStoreWithTTL(relayTTL)
	store.StartCleanup(ctx)

	handler := ledger.NewServerWithConfig(store, cfg)
	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	errCh := make(chan error, 1)

	go func() {
		log.Printf("ledger server starting on :%s (relay_ttl=%s rate_limit=%d/%s)",
			port, relayTTL, cfg.RateLimit, cfg.RateWindow)
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
