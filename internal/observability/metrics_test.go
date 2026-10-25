package observability

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestMetricsRegistry_CounterAndGauge(t *testing.T) {
	reg := NewRegistry()
	counter := reg.NewCounter("test_counter_total", "Test counter help", "arch", "status")
	gauge := reg.NewGauge("test_gauge", "Test gauge help", "arch")

	counter.WithLabels("optimized", "success").Inc()
	counter.WithLabels("optimized", "success").Inc()
	counter.WithLabels("baseline", "conflict").Add(3)

	gauge.WithLabels("optimized").Set(42.5)

	out := reg.Gather()

	if !strings.Contains(out, `test_counter_total{arch="optimized",status="success"} 2`) {
		t.Fatalf("missing or wrong counter in output:\n%s", out)
	}
	if !strings.Contains(out, `test_counter_total{arch="baseline",status="conflict"} 3`) {
		t.Fatalf("missing or wrong baseline counter in output:\n%s", out)
	}
	if !strings.Contains(out, `test_gauge{arch="optimized"} 42.5`) {
		t.Fatalf("missing or wrong gauge in output:\n%s", out)
	}
}

func TestHistogram_PrometheusTextFormatCompliance(t *testing.T) {
	reg := NewRegistry()
	buckets := []float64{0.005, 0.01, 0.05, 0.1, 1.0}
	hist := reg.NewHistogram("http_request_duration_seconds", "HTTP duration", buckets, "arch", "route")

	hist.WithLabels("optimized", "/orders").Observe(0.004) // <= 0.005
	hist.WithLabels("optimized", "/orders").Observe(0.008) // <= 0.01
	hist.WithLabels("optimized", "/orders").Observe(0.06)  // <= 0.1

	out := reg.Gather()

	// Verify cumulative buckets
	if !strings.Contains(out, `http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.005"} 1`) {
		t.Errorf("bucket 0.005 missing or mismatch:\n%s", out)
	}
	if !strings.Contains(out, `http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.01"} 2`) {
		t.Errorf("bucket 0.01 missing or mismatch:\n%s", out)
	}
	if !strings.Contains(out, `http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="0.1"} 3`) {
		t.Errorf("bucket 0.1 missing or mismatch:\n%s", out)
	}
	if !strings.Contains(out, `http_request_duration_seconds_bucket{arch="optimized",route="/orders",le="+Inf"} 3`) {
		t.Errorf("bucket +Inf missing:\n%s", out)
	}
	if !strings.Contains(out, `http_request_duration_seconds_count{arch="optimized",route="/orders"} 3`) {
		t.Errorf("histogram _count missing:\n%s", out)
	}
}

func TestMetricsRegistry_ConcurrentUpdates_ThreadSafe(t *testing.T) {
	reg := NewRegistry()
	counter := reg.NewCounter("concurrent_requests_total", "help", "worker")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("w-%d", i%4)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				counter.WithLabels(id).Inc()
			}
		}(workerID)
	}
	wg.Wait()

	out := reg.Gather()
	for i := 0; i < 4; i++ {
		expected := fmt.Sprintf(`concurrent_requests_total{worker="w-%d"} 2500`, i)
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in gather output:\n%s", expected, out)
		}
	}
}

func TestMetricsRegistry_BoundedCardinality(t *testing.T) {
	reg := NewRegistry()
	counter := reg.NewCounter("orders_total", "Orders total", "arch", "status")

	// Verify that valid low-cardinality calls work
	counter.WithLabels("optimized", "201").Inc()
	counter.WithLabels("optimized", "409").Inc()
	counter.WithLabels("baseline", "201").Inc()

	out := reg.Gather()
	if !strings.Contains(out, `orders_total{arch="optimized",status="201"} 1`) {
		t.Fatalf("expected orders_total metric present")
	}
}

func TestMetricsHandler_HTTPResponse(t *testing.T) {
	DefaultRegistry = NewRegistry()
	IncHTTPRequests("baseline", "/healthz", "GET", "200")

	handler := MetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %q", contentType)
	}
	if !strings.Contains(w.Body.String(), `http_requests_total{arch="baseline",method="GET",route="/healthz",status="200"} 1`) {
		t.Fatalf("expected metric in handler response:\n%s", w.Body.String())
	}
}
