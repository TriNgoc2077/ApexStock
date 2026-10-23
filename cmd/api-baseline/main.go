package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"apexstock/internal/config"
	"apexstock/internal/httpapi"
)

func main() {
	cfg := config.Load()

	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/healthz", httpapi.HealthzHandler)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		httpapi.ReadyzHandler(w, r, cfg.BaselineDSN)
	})

	// Flash sale order endpoint (stub)
	mux.HandleFunc("POST /api/v1/flash-sale/orders", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintln(w, `{"error":"not_implemented","message":"Baseline reservation logic pending Stage 3"}`)
	})

	addr := fmt.Sprintf(":%d", cfg.BaselinePort)
	log.Printf("[api-baseline] Starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[api-baseline] Server failed: %v", err)
		os.Exit(1)
	}
}
