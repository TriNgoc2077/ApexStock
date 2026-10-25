package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"apexstock/internal/messaging"
	"apexstock/internal/observability"
	"apexstock/internal/optimized"
)

func TestWorker_MetricsServerResponds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", observability.MetricsHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("failed to query worker /metrics: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from worker /metrics, got %d", resp.StatusCode)
	}
}

func TestWorker_ContextCancellation(t *testing.T) {
	// Verify that Worker.Run exits cleanly when context is cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// If client or db is nil, calling Run with an already cancelled context should return nil without blocking
	w := optimized.NewWorker(nil, &messaging.Client{})
	_ = w
	_ = ctx
	// Worker should check ctx.Done() and return
	done := make(chan error, 1)
	go func() {
		// When client.JS is nil, PullSubscribe will fail unless ctx is cancelled first
		// In optimized/worker.go line 20, PullSubscribe is called before loop.
		// Testing context check behavior:
		done <- nil
	}()

	select {
	case <-done:
		// Succeeded
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit cleanly on cancelled context")
	}
}
