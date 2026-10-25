package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apexstock/internal/config"
	"apexstock/internal/messaging"
	"apexstock/internal/optimized"
	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	log.Printf("[worker] Starting worker process (env=%s)...", cfg.Environment)

	db, err := sql.Open("postgres", cfg.OptimizedDSN)
	if err != nil {
		log.Fatalf("[worker] Failed to connect to PostgreSQL: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	nc, err := messaging.Connect(cfg.NatsURL)
	if err != nil {
		log.Fatalf("[worker] Failed to connect to NATS (%s): %v", cfg.NatsURL, err)
	}
	defer nc.Close()

	worker := optimized.NewWorker(db, nc)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Println("[worker] Listening for order.reserved events on NATS JetStream...")
		errCh <- worker.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		log.Println("[worker] Shutdown signal received, waiting for pending events to complete...")
	case err := <-errCh:
		if err != nil {
			log.Fatalf("[worker] Worker stopped with error: %v", err)
		}
	}

	log.Println("[worker] Shutdown complete.")
}
