package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apexstock/internal/warroom"
)

func main() {
	log.Println("[warroom-controller] Starting War Room controller...")

	baselineURL := os.Getenv("BASELINE_METRICS_URL")
	if baselineURL == "" {
		baselineURL = "http://api-baseline:8081"
	}

	optimizedURL := os.Getenv("OPTIMIZED_METRICS_URL")
	if optimizedURL == "" {
		optimizedURL = "http://api-optimized:8082"
	}

	port := os.Getenv("WARROOM_PORT")
	if port == "" {
		port = "9000"
	}

	svc := warroom.NewWarRoomService(baselineURL, optimizedURL)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go svc.Start(ctx)

	handler := warroom.NewHandler(svc)
	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler.Routes(),
	}

	go func() {
		log.Printf("[warroom-controller] Listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[warroom-controller] Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[warroom-controller] Shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	log.Println("[warroom-controller] Shutdown complete.")
}
