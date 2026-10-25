package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apexstock/internal/config"
	"apexstock/internal/messaging"
	"apexstock/internal/observability"
	"apexstock/internal/optimized"
	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	observability.InitLogger(cfg.Environment)
	log.Printf("[worker] Starting worker process (env=%s)...", cfg.Environment)

	// Start background HTTP server for Prometheus scrape and container health
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/metrics", observability.MetricsHandler())
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})

	metricsPort := os.Getenv("WORKER_METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "8080"
	}
	metricsServer := &http.Server{
		Addr:    ":" + metricsPort,
		Handler: metricsMux,
	}

	go func() {
		log.Printf("[worker] Prometheus metrics listening on :%s/metrics", metricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[worker] Metrics server error: %v", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()

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
