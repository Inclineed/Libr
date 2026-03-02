package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ledger/internal/ledger"
)

func main() {
	port := os.Getenv("LEDGER_PORT")
	if port == "" {
		port = "9000"
	}

	store := ledger.NewInMemoryStore()
	handler := ledger.NewServer(store)
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

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-signalCtx.Done():
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
