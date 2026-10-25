package warroom

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetricsAggregator_ParsesPrometheusMetrics(t *testing.T) {
	sampleMetricsFirst := `# HELP http_requests_total Total
# TYPE http_requests_total counter
http_requests_total{arch="optimized",method="POST",route="/orders",status="201"} 100
# HELP http_request_duration_seconds Latency
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.01"} 60
http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.05"} 90
http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="+Inf"} 100
# HELP stock_remaining Remaining
# TYPE stock_remaining gauge
stock_remaining{arch="optimized",item_id="item-1"} 900
`

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

	var pollCount atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/metrics" {
			w.WriteHeader(http.StatusOK)
			if pollCount.Add(1) == 1 {
				_, _ = w.Write([]byte(sampleMetricsFirst))
			} else {
				_, _ = w.Write([]byte(sampleMetrics))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	svc := NewWarRoomService("http://127.0.0.1:0", ts.URL)
	ctx := context.Background()

	// First tick initializes baseline request count and timestamp
	svc.tick(ctx)

	// Sleep briefly so elapsed time > 0
	time.Sleep(15 * time.Millisecond)

	// Second tick calculates non-zero RPS and parses updated metrics
	svc.tick(ctx)
	snap := svc.GetSnapshot().Optimized

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
	if snap.P50LatencyMs <= 0 {
		t.Errorf("expected P50LatencyMs > 0, got %f", snap.P50LatencyMs)
	}
	if snap.P95LatencyMs <= 0 {
		t.Errorf("expected P95LatencyMs > 0, got %f", snap.P95LatencyMs)
	}
	if snap.P99LatencyMs <= 0 {
		t.Errorf("expected P99LatencyMs > 0, got %f", snap.P99LatencyMs)
	}
	if snap.RPS <= 0 {
		t.Errorf("expected RPS > 0, got %f", snap.RPS)
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

	svc := NewWarRoomService("http://127.0.0.1:0", ts.URL)
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

func TestSnapshot_ActivityLogsClone_RaceFree(t *testing.T) {
	svc := NewWarRoomService("http://localhost:8081", "http://localhost:8082")

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Goroutine 1: Rapidly writes logs
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				i++
				svc.appendActivityLog("info", fmt.Sprintf("Log %d", i), nil)
			}
		}
	}()

	// Goroutine 2: Rapidly reads snapshots and inspects ActivityLogs
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				snap := svc.GetSnapshot()
				for _, l := range snap.ActivityLogs {
					_ = l.Message
				}
			}
		}
	}()

	wg.Wait()
}
