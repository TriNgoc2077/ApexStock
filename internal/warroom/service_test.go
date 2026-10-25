package warroom

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetricsAggregator_ParsesPrometheusMetrics(t *testing.T) {
	sampleMetrics := `# HELP http_requests_total Total
# TYPE http_requests_total counter
http_requests_total{arch="optimized",method="POST",route="/orders",status="201"} 150
http_requests_total{arch="optimized",method="POST",route="/orders",status="409"} 10
http_requests_total{arch="optimized",method="POST",route="/orders",status="500"} 2
# HELP http_request_duration_seconds Latency
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.01"} 100
http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.05"} 140
http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="+Inf"} 162
http_request_duration_seconds_sum{arch="optimized",route="/orders"} 1.62
http_request_duration_seconds_count{arch="optimized",route="/orders"} 162
# HELP stock_remaining Remaining
# TYPE stock_remaining gauge
stock_remaining{arch="optimized",item_id="item-1"} 850
# HELP oversell_total Oversell
# TYPE oversell_total counter
oversell_total{item_id="item-1"} 0
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/metrics" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleMetrics))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	svc := NewWarRoomService(ts.URL, ts.URL)
	snap := svc.pollInstance(context.Background(), "optimized", ts.URL)

	if !snap.Healthy {
		t.Fatalf("expected healthy = true")
	}
	if snap.Status2xx != 150 {
		t.Errorf("expected Status2xx = 150, got %d", snap.Status2xx)
	}
	if snap.Status409 != 10 {
		t.Errorf("expected Status409 = 10, got %d", snap.Status409)
	}
	if snap.Status5xx != 2 {
		t.Errorf("expected Status5xx = 2, got %d", snap.Status5xx)
	}
	if snap.RemainingStock != 850 {
		t.Errorf("expected RemainingStock = 850, got %d", snap.RemainingStock)
	}
	if snap.OversellCount != 0 {
		t.Errorf("expected OversellCount = 0, got %d", snap.OversellCount)
	}
}

func TestHealthWatcher_MeasuresRecoveryTime(t *testing.T) {
	var isDown atomic.Bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	svc := NewWarRoomService(ts.URL, ts.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initial healthy poll
	svc.tick(ctx)
	if svc.GetSnapshot().Optimized.Status != "healthy" {
		t.Fatalf("expected healthy initial status")
	}

	// Simulate chaos: container killed
	isDown.Store(true)
	svc.tick(ctx)
	if svc.GetSnapshot().Optimized.Status != "down" {
		t.Fatalf("expected down status after failure")
	}

	time.Sleep(50 * time.Millisecond)

	// Simulate recovery: container restarted
	isDown.Store(false)
	svc.tick(ctx)
	if svc.GetSnapshot().Optimized.Status != "healthy" {
		t.Fatalf("expected healthy status after recovery")
	}

	logs := svc.GetSnapshot().ActivityLogs
	foundRecovery := false
	for _, l := range logs {
		if l.Type == "recovered" && l.RecoveryTimeMs != nil && *l.RecoveryTimeMs > 0 {
			foundRecovery = true
			break
		}
	}
	if !foundRecovery {
		t.Fatalf("expected activity log with recovery time measurement, logs: %+v", logs)
	}
}

func TestMetricsAggregator_BoundedMemory_SlidingWindow(t *testing.T) {
	svc := NewWarRoomService("http://localhost:8081", "http://localhost:8082")

	// Append 200 activity logs
	for i := 0; i < 200; i++ {
		svc.appendActivityLog("info", fmt.Sprintf("Event %d", i), nil)
	}

	snap := svc.GetSnapshot()
	if len(snap.ActivityLogs) > 100 {
		t.Fatalf("activity logs exceeded max capacity (got %d, max 100)", len(snap.ActivityLogs))
	}
}
