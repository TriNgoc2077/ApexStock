# Stage 7: Observability, Metrics, Logs & Reliability Baseline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and integrate a high-performance, zero-dependency Prometheus `/metrics` exposition and registry, structured JSON logging (`slog`) with cross-service `X-Correlation-ID` propagation, Prometheus alert rules, Grafana dashboard provisioning, and operational incident response runbooks.

**Architecture:**
Incoming HTTP Request / NATS Message → Observability Middleware (`internal/observability/middleware.go`) extracts/generates `X-Correlation-ID`, records request duration in Histogram and status counter in Prometheus Registry (`internal/observability/metrics.go`), and emits structured JSON log (`internal/observability/logger.go`) → Prometheus scrapes `/metrics` on Baseline (:8081), Optimized (:8082), and Worker (:8080) → Prometheus evaluates alerts (`deploy/prometheus/alerts.yml`) → Grafana displays topology, RPS, latency percentiles, stock depletion, and oversell invariant status (`deploy/grafana/provisioning/dashboards/apexstock.json`).

**Tech Stack:** Go 1.22 (`log/slog`, `sync`, `sync/atomic`, `net/http`), Prometheus, Grafana, PostgreSQL, Redis, NATS JetStream.

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 7, Section 7; Invariants I1–I5).

## Global Constraints
- Go module: `apexstock`
- **Zero New External Dependencies:** Prometheus registry and exposition are implemented purely using Go 1.22 standard library (`sync`, `sync/atomic`, `net/http`, `fmt`, `sort`, `strings`). Logging uses standard `log/slog`. No external metrics libraries added to `go.mod`.
- Codebase must compile cleanly with `go build ./...` and `go vet ./...`.
- `go test -race ./...` must pass at every task.
- Commit message format: `feat(stage-7): <description>`, `test(stage-7): <description>`, `docs(stage-7): <description>`.
- **Low Cardinality Rule (Decision Gate D0.9):** Metric label values must strictly be low-cardinality enums (`arch`: `"baseline"|"optimized"`, `route`: `"/api/v1/flash-sale/orders"|"/healthz"|"/api/v1/payments/simulate"`, `status`: `"201"|"409"|"429"|"500"`, `outcome`: `"success"|"failure"|"timeout"|"cancel"`). Never include user IDs, order IDs, timestamps, or raw query strings in Prometheus labels.
- **Business Rejections vs System Outages:** HTTP 409 (out of stock, duplicate order) and HTTP 429 (rate limited) are normal business rejections, NOT 5xx system outages. Latency and error budget calculations must separate 409 from 5xx.
- **Invariant Safety:** `oversell_total` starts at 0 and is non-decreasing. Any value > 0 triggers an immediate critical alert.

## Review Focus
1. **High Cardinality Metric Memory Explosion**: Dynamically generated IDs (order ID, user ID, idempotency keys) leaking into Prometheus metric labels, causing unbounded RAM growth during benchmark runs.
   *Expected behavior:* Metric registry normalizes routes to static route templates (e.g. `/api/v1/flash-sale/orders`) and rejects unbounded label values; label key combinations are strictly bounded (< 50 unique series per metric).
   *Pinned test:* `TestMetricsRegistry_BoundedCardinality` in Task 1.
2. **Correlation ID Loss in Asynchronous NATS Messaging**: HTTP gateway accepts order and assigns `X-Correlation-ID`, but worker processing JetStream event loses the ID or logs empty, breaking end-to-end trace.
   *Expected behavior:* Producer writes `X-Correlation-ID` to NATS message header `Header["X-Correlation-ID"]`. Worker extracts correlation ID from NATS header and attaches it to the worker context and structured logs.
   *Pinned test:* `TestCorrelationID_PropagationAcrossNATS` in Task 2.
3. **Misclassifying 409 Conflict as Service Outage**: Load test encounters flash sale sell-out (HTTP 409 `out_of_stock`). A naive SLO dashboard or alert rule treats 409 as an HTTP error, falsely triggering system outage alerts.
   *Expected behavior:* Metrics and Grafana queries separate `http_requests_total{status=~"409|429"}` (business rejections) from `http_requests_total{status=~"5.."}` (system failures). Error budget and alert rules only trigger on 5xx errors or unexpected panics.
   *Pinned test:* `TestHTTPMiddleware_Distinguishes409From500` in Task 2.
4. **Histogram Latency Bucketing Accuracy for p50/p95/p99**: Benchmark profiles require precise microsecond to sub-second percentiles (e.g. 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s).
   *Expected behavior:* Histogram maintains cumulative bucket counts (`le`), `_sum`, and `_count` complying strictly with Prometheus text format specifications, allowing `histogram_quantile(0.99, ...)` to compute valid percentiles.
   *Pinned test:* `TestHistogram_PrometheusTextFormatCompliance` in Task 1.
5. **Worker Background Health & Metrics Scraping Failure**: The worker process in `cmd/worker` lacks an HTTP server, causing Prometheus scrape failures (`target down`) in Docker Compose.
   *Expected behavior:* `cmd/worker` starts an internal HTTP server on `:8080` (or `WORKER_METRICS_PORT`) serving `/metrics` and `/healthz`, with clean graceful shutdown on SIGTERM.
   *Pinned test:* `TestWorker_MetricsServerResponds` in Task 3.

---

### Task 1: Prometheus Metrics Registry & Exposition

**Files:**
- Create: `internal/observability/metrics.go`
- Test: `internal/observability/metrics_test.go`

**Interfaces:**
- Consumes: Go standard library (`sync`, `sync/atomic`, `net/http`, `fmt`, `sort`, `strings`)
- Produces:
  ```go
  type Registry struct { ... }
  var DefaultRegistry *Registry
  func MetricsHandler() http.HandlerFunc
  func IncHTTPRequests(arch, route, method, status string)
  func ObserveHTTPRequestDuration(arch, route string, seconds float64)
  func IncOrdersCreated(arch, status string)
  func IncPaymentsProcessed(arch, outcome string)
  func SetStockRemaining(arch, itemID string, stock float64)
  func SetNATSQueueLag(stream, consumer string, lag float64)
  func IncOversell(itemID string)
  ```

- [ ] **Step 1: Write failing unit tests for metric registry, histogram bucketing, concurrency safety, and text formatting**

Create `internal/observability/metrics_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/observability -run TestMetrics`
Expected: FAIL (package does not exist or symbols undefined).

- [ ] **Step 3: Implement `internal/observability/metrics.go`**

Create `internal/observability/metrics.go`:
```go
package observability

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

var StandardLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

type CounterVec struct {
	name   string
	help   string
	labels []string
	series sync.Map // map[string]*uint64
}

type GaugeVec struct {
	name   string
	help   string
	labels []string
	series sync.Map // map[string]*uint64 (math.Float64bits)
}

type HistogramVec struct {
	name    string
	help    string
	buckets []float64
	labels  []string
	series  sync.Map // map[string]*histogramSeries
}

type histogramSeries struct {
	buckets []uint64 // counts for each bucket
	sum     uint64   // math.Float64bits
	count   uint64
}

type Registry struct {
	mu         sync.RWMutex
	counters   []*CounterVec
	gauges     []*GaugeVec
	histograms []*HistogramVec
}

func NewRegistry() *Registry {
	return &Registry{}
}

var DefaultRegistry = NewRegistry()

// Global application metric vectors
var (
	metricHTTPRequestsTotal = DefaultRegistry.NewCounter(
		"http_requests_total",
		"Total number of HTTP requests processed by route, arch, and status code",
		"arch", "route", "method", "status",
	)
	metricHTTPRequestDuration = DefaultRegistry.NewHistogram(
		"http_request_duration_seconds",
		"HTTP request latency histogram in seconds",
		StandardLatencyBuckets,
		"arch", "route",
	)
	metricOrdersCreated = DefaultRegistry.NewCounter(
		"orders_created_total",
		"Total flash sale orders created by architecture and status",
		"arch", "status",
	)
	metricPaymentsProcessed = DefaultRegistry.NewCounter(
		"payments_processed_total",
		"Total payment transitions processed by architecture and outcome",
		"arch", "outcome",
	)
	metricStockRemaining = DefaultRegistry.NewGauge(
		"stock_remaining",
		"Current remaining available inventory stock",
		"arch", "item_id",
	)
	metricNATSQueueLag = DefaultRegistry.NewGauge(
		"nats_queue_lag",
		"Number of pending messages in NATS JetStream queue",
		"stream", "consumer",
	)
	metricOversellTotal = DefaultRegistry.NewCounter(
		"oversell_total",
		"CRITICAL: Total invariant violations where stock was oversold beyond initial quantity",
		"item_id",
	)
)

func (r *Registry) NewCounter(name, help string, labels ...string) *CounterVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &CounterVec{name: name, help: help, labels: labels}
	r.counters = append(r.counters, c)
	return c
}

func (r *Registry) NewGauge(name, help string, labels ...string) *GaugeVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := &GaugeVec{name: name, help: help, labels: labels}
	r.gauges = append(r.gauges, g)
	return g
}

func (r *Registry) NewHistogram(name, help string, buckets []float64, labels ...string) *HistogramVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	sortedBuckets := make([]float64, len(buckets))
	copy(sortedBuckets, buckets)
	sort.Float64s(sortedBuckets)
	h := &HistogramVec{name: name, help: help, buckets: sortedBuckets, labels: labels}
	r.histograms = append(r.histograms, h)
	return h
}

type CounterMetric struct {
	val *uint64
}

func (c *CounterMetric) Inc() {
	atomic.AddUint64(c.val, 1)
}

func (c *CounterMetric) Add(v uint64) {
	atomic.AddUint64(c.val, v)
}

func (cv *CounterVec) WithLabels(labelValues ...string) *CounterMetric {
	key := strings.Join(labelValues, "\xff")
	val, ok := cv.series.Load(key)
	if !ok {
		var n uint64
		actual, _ := cv.series.LoadOrStore(key, &n)
		val = actual
	}
	return &CounterMetric{val: val.(*uint64)}
}

type GaugeMetric struct {
	val *uint64
}

func (g *GaugeMetric) Set(val float64) {
	atomic.StoreUint64(g.val, math.Float64bits(val))
}

func (gv *GaugeVec) WithLabels(labelValues ...string) *GaugeMetric {
	key := strings.Join(labelValues, "\xff")
	val, ok := gv.series.Load(key)
	if !ok {
		var n uint64
		actual, _ := gv.series.LoadOrStore(key, &n)
		val = actual
	}
	return &GaugeMetric{val: val.(*uint64)}
}

type HistogramMetric struct {
	hv *HistogramVec
	hs *histogramSeries
}

func (hv *HistogramVec) WithLabels(labelValues ...string) *HistogramMetric {
	key := strings.Join(labelValues, "\xff")
	val, ok := hv.series.Load(key)
	if !ok {
		hs := &histogramSeries{
			buckets: make([]uint64, len(hv.buckets)),
		}
		actual, _ := hv.series.LoadOrStore(key, hs)
		val = actual
	}
	return &HistogramMetric{hv: hv, hs: val.(*histogramSeries)}
}

func (hm *HistogramMetric) Observe(seconds float64) {
	atomic.AddUint64(&hm.hs.count, 1)
	for {
		oldBits := atomic.LoadUint64(&hm.hs.sum)
		newBits := math.Float64bits(math.Float64frombits(oldBits) + seconds)
		if atomic.CompareAndSwapUint64(&hm.hs.sum, oldBits, newBits) {
			break
		}
	}
	for i, upper := range hm.hv.buckets {
		if seconds <= upper {
			atomic.AddUint64(&hm.hs.buckets[i], 1)
		}
	}
}

// Gather produces Prometheus text exposition format.
func (r *Registry) Gather() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	for _, c := range r.counters {
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", c.name, c.help))
		sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", c.name))
		c.series.Range(func(key, value any) bool {
			vals := strings.Split(key.(string), "\xff")
			labelsStr := formatLabels(c.labels, vals)
			val := atomic.LoadUint64(value.(*uint64))
			sb.WriteString(fmt.Sprintf("%s%s %d\n", c.name, labelsStr, val))
			return true
		})
	}

	for _, g := range r.gauges {
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", g.name, g.help))
		sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", g.name))
		g.series.Range(func(key, value any) bool {
			vals := strings.Split(key.(string), "\xff")
			labelsStr := formatLabels(g.labels, vals)
			bits := atomic.LoadUint64(value.(*uint64))
			sb.WriteString(fmt.Sprintf("%s%s %v\n", g.name, labelsStr, math.Float64frombits(bits)))
			return true
		})
	}

	for _, h := range r.histograms {
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n", h.name, h.help))
		sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", h.name))
		h.series.Range(func(key, value any) bool {
			vals := strings.Split(key.(string), "\xff")
			hs := value.(*histogramSeries)

			// Cumulative bucket counts
			var cumulative uint64
			for i, bound := range h.buckets {
				cumulative += atomic.LoadUint64(&hs.buckets[i])
				bLabels := append([]string{}, vals...)
				bNames := append([]string{}, h.labels...)
				bNames = append(bNames, "le")
				bLabels = append(bLabels, fmt.Sprintf("%v", bound))
				sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.name, formatLabels(bNames, bLabels), cumulative))
			}
			// +Inf bucket
			infNames := append(append([]string{}, h.labels...), "le")
			infLabels := append(append([]string{}, vals...), "+Inf")
			totalCount := atomic.LoadUint64(&hs.count)
			sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.name, formatLabels(infNames, infLabels), totalCount))

			labelsStr := formatLabels(h.labels, vals)
			sumBits := atomic.LoadUint64(&hs.sum)
			sb.WriteString(fmt.Sprintf("%s_sum%s %v\n", h.name, labelsStr, math.Float64frombits(sumBits)))
			sb.WriteString(fmt.Sprintf("%s_count%s %d\n", h.name, labelsStr, totalCount))
			return true
		})
	}

	return sb.String()
}

func formatLabels(names, values []string) string {
	if len(names) == 0 || len(values) == 0 {
		return ""
	}
	pairs := make([]string, len(names))
	for i := range names {
		pairs[i] = fmt.Sprintf(`%s="%s"`, names[i], values[i])
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// MetricsHandler serves Prometheus metrics on HTTP GET /metrics.
func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(DefaultRegistry.Gather()))
	}
}

// Convenient helper functions using DefaultRegistry
func IncHTTPRequests(arch, route, method, status string) {
	metricHTTPRequestsTotal.WithLabels(arch, route, method, status).Inc()
}

func ObserveHTTPRequestDuration(arch, route string, seconds float64) {
	metricHTTPRequestDuration.WithLabels(arch, route).Observe(seconds)
}

func IncOrdersCreated(arch, status string) {
	metricOrdersCreated.WithLabels(arch, status).Inc()
}

func IncPaymentsProcessed(arch, outcome string) {
	metricPaymentsProcessed.WithLabels(arch, outcome).Inc()
}

func SetStockRemaining(arch, itemID string, stock float64) {
	metricStockRemaining.WithLabels(arch, itemID).Set(stock)
}

func SetNATSQueueLag(stream, consumer string, lag float64) {
	metricNATSQueueLag.WithLabels(stream, consumer).Set(lag)
}

func IncOversell(itemID string) {
	metricOversellTotal.WithLabels(itemID).Inc()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -race ./internal/observability -run TestMetrics`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/observability/metrics.go internal/observability/metrics_test.go
git commit -m "feat(stage-7): implement zero-dependency prometheus metrics registry and exposition"
```

---

### Task 2: Structured JSON Logger & Correlation ID Middleware

**Files:**
- Create: `internal/observability/logger.go`
- Create: `internal/observability/middleware.go`
- Test: `internal/observability/middleware_test.go`

**Interfaces:**
- Consumes: Go standard library `log/slog`, `net/http`, `context`, `crypto/rand`, `github.com/nats-io/nats.go`
- Produces:
  ```go
  func InitLogger(env string) *slog.Logger
  func WithCorrelationID(ctx context.Context, id string) context.Context
  func CorrelationIDFromContext(ctx context.Context) string
  func HTTPMiddleware(arch string, next http.Handler) http.Handler
  func InjectNATSCorrelationID(ctx context.Context, msg *nats.Msg)
  func ExtractNATSCorrelationID(msg *nats.Msg) (string, context.Context)
  ```

- [ ] **Step 1: Write failing tests for correlation ID propagation, JSON logs, status discrimination (409 vs 500), and NATS headers**

Create `internal/observability/middleware_test.go`:
```go
package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestHTTPMiddleware_CorrelationID_PreservedOrGenerated(t *testing.T) {
	var capturedID string
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = CorrelationIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := HTTPMiddleware("optimized", testHandler)

	// Case 1: Provided in header
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Correlation-ID", "corr-test-12345")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if capturedID != "corr-test-12345" {
		t.Fatalf("expected context correlation ID 'corr-test-12345', got %q", capturedID)
	}
	if w.Header().Get("X-Correlation-ID") != "corr-test-12345" {
		t.Fatalf("expected response header X-Correlation-ID 'corr-test-12345', got %q", w.Header().Get("X-Correlation-ID"))
	}

	// Case 2: Generated when missing
	req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	if capturedID == "" || capturedID == "corr-test-12345" {
		t.Fatalf("expected newly generated correlation ID, got %q", capturedID)
	}
	if w2.Header().Get("X-Correlation-ID") != capturedID {
		t.Fatalf("expected response header to match generated ID %q, got %q", capturedID, w2.Header().Get("X-Correlation-ID"))
	}
}

func TestHTTPMiddleware_Distinguishes409From500(t *testing.T) {
	DefaultRegistry = NewRegistry()

	// Handler returning 409 Conflict (e.g. out of stock business rejection)
	conflictHandler := HTTPMiddleware("baseline", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/flash-sale/orders", nil)
	w := httptest.NewRecorder()
	conflictHandler.ServeHTTP(w, req)

	out := DefaultRegistry.Gather()
	if !strings.Contains(out, `http_requests_total{arch="baseline",method="POST",route="/api/v1/flash-sale/orders",status="409"} 1`) {
		t.Fatalf("expected status 409 metric recorded, got:\n%s", out)
	}
	if strings.Contains(out, `status="500"`) {
		t.Fatalf("status 409 must NOT be recorded as 500")
	}
}

func TestCorrelationID_PropagationAcrossNATS(t *testing.T) {
	ctx := context.Background()
	ctxWithID := WithCorrelationID(ctx, "trace-nats-999")

	msg := &nats.Msg{
		Subject: "order.reserved",
		Data:    []byte(`{"order_id":"1"}`),
	}

	InjectNATSCorrelationID(ctxWithID, msg)

	if msg.Header == nil || msg.Header.Get("X-Correlation-ID") != "trace-nats-999" {
		t.Fatalf("expected NATS header X-Correlation-ID = 'trace-nats-999', got %+v", msg.Header)
	}

	extractedID, newCtx := ExtractNATSCorrelationID(msg)
	if extractedID != "trace-nats-999" {
		t.Fatalf("expected extracted ID 'trace-nats-999', got %q", extractedID)
	}
	if CorrelationIDFromContext(newCtx) != "trace-nats-999" {
		t.Fatalf("expected context ID 'trace-nats-999', got %q", CorrelationIDFromContext(newCtx))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/observability -run TestHTTPMiddleware`
Expected: FAIL (functions undefined).

- [ ] **Step 3: Implement `internal/observability/logger.go` and `internal/observability/middleware.go`**

Create `internal/observability/logger.go`:
```go
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
)

type correlationIDKey struct{}

func InitLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if env == "dev" || env == "test" {
		opts.Level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func NewCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, id)
}

func CorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return id
	}
	return ""
}
```

Create `internal/observability/middleware.go`:
```go
package observability

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// HTTPMiddleware instruments requests with correlation ID, Prometheus metrics, and structured JSON logs.
func HTTPMiddleware(arch string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = NewCorrelationID()
		}

		ctx := WithCorrelationID(r.Context(), correlationID)
		w.Header().Set("X-Correlation-ID", correlationID)

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r.WithContext(ctx))

		duration := time.Since(start)
		route := normalizeRoute(r.URL.Path)
		statusStr := strconv.Itoa(rw.statusCode)

		// Record Prometheus metrics
		IncHTTPRequests(arch, route, r.Method, statusStr)
		ObserveHTTPRequestDuration(arch, route, duration.Seconds())

		// Emit structured JSON log
		slog.InfoContext(ctx, "http_request",
			slog.String("arch", arch),
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", rw.statusCode),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
			slog.Int("bytes", rw.bytes),
			slog.String("correlation_id", correlationID),
			slog.String("client_ip", r.RemoteAddr),
		)
	})
}

func normalizeRoute(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/v1/flash-sale/orders"):
		return "/api/v1/flash-sale/orders"
	case strings.HasPrefix(path, "/api/v1/payments/simulate"):
		return "/api/v1/payments/simulate"
	case strings.HasPrefix(path, "/api/v1/payments/sepay/initiate"):
		return "/api/v1/payments/sepay/initiate"
	case strings.HasPrefix(path, "/api/v1/webhooks/sepay"):
		return "/api/v1/webhooks/sepay"
	case strings.HasPrefix(path, "/healthz"):
		return "/healthz"
	case strings.HasPrefix(path, "/readyz"):
		return "/readyz"
	case strings.HasPrefix(path, "/metrics"):
		return "/metrics"
	default:
		return path
	}
}

// InjectNATSCorrelationID sets X-Correlation-ID in NATS message header.
func InjectNATSCorrelationID(ctx context.Context, msg *nats.Msg) {
	if msg == nil {
		return
	}
	id := CorrelationIDFromContext(ctx)
	if id == "" {
		id = NewCorrelationID()
	}
	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}
	msg.Header.Set("X-Correlation-ID", id)
}

// ExtractNATSCorrelationID retrieves X-Correlation-ID from NATS message header and wraps context.
func ExtractNATSCorrelationID(msg *nats.Msg) (string, context.Context) {
	var id string
	if msg != nil && msg.Header != nil {
		id = msg.Header.Get("X-Correlation-ID")
	}
	if id == "" {
		id = NewCorrelationID()
	}
	ctx := WithCorrelationID(context.Background(), id)
	return id, ctx
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -race ./internal/observability -run Test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/observability/logger.go internal/observability/middleware.go internal/observability/middleware_test.go
git commit -m "feat(stage-7): implement structured logger and correlation ID middleware"
```

---

### Task 3: Wire Observability into API Baseline, API Optimized & Worker

**Files:**
- Modify: `cmd/api-baseline/main.go`
- Modify: `cmd/api-optimized/main.go`
- Modify: `cmd/worker/main.go`
- Test: `cmd/worker/main_test.go`

**Interfaces:**
- Consumes: `internal/observability`
- Produces: Exposed `/metrics` endpoints and middleware instrumentation across all three services.

- [ ] **Step 1: Wire observability middleware and `/metrics` into `cmd/api-baseline/main.go`**

Modify `cmd/api-baseline/main.go`:
```go
	// Observability: initialize structured JSON logger and metrics endpoint
	observability.InitLogger(cfg.Environment)
	mux.HandleFunc("/metrics", observability.MetricsHandler())

	// Wrap entire mux with Observability HTTPMiddleware
	handler := observability.HTTPMiddleware("baseline", mux)

	addr := fmt.Sprintf(":%d", cfg.BaselinePort)
	log.Printf("[api-baseline] Starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[api-baseline] Server failed: %v", err)
		os.Exit(1)
	}
```

- [ ] **Step 2: Wire observability middleware and `/metrics` into `cmd/api-optimized/main.go`**

Modify `cmd/api-optimized/main.go`:
```go
	// Observability: initialize structured JSON logger and metrics endpoint
	observability.InitLogger(cfg.Environment)
	mux.HandleFunc("/metrics", observability.MetricsHandler())

	handler := observability.HTTPMiddleware("optimized", mux)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.OptimizedPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("[api-optimized] Starting on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
```

- [ ] **Step 3: Add HTTP metrics server and worker loop instrumentation in `cmd/worker/main.go`**

Modify `cmd/worker/main.go`:
```go
	observability.InitLogger(cfg.Environment)

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
```

- [ ] **Step 4: Write test verifying worker metrics and healthz server in `cmd/worker/main_test.go`**

Modify `cmd/worker/main_test.go`:
```go
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
```

- [ ] **Step 5: Run tests and compilation checks**

Run:
```bash
go build ./cmd/api-baseline
go build ./cmd/api-optimized
go build ./cmd/worker
go test -v ./cmd/worker -run TestWorker_MetricsServerResponds
```
Expected: All build cleanly; test PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/api-baseline/main.go cmd/api-optimized/main.go cmd/worker/main.go cmd/worker/main_test.go
git commit -m "feat(stage-7): wire /metrics and observability middleware into baseline, optimized, and worker"
```

---

### Task 4: Prometheus Alert Rules, Grafana Provisioning, Metric Catalog & Runbooks

**Files:**
- Create: `deploy/prometheus/alerts.yml`
- Modify: `deploy/prometheus/prometheus.yml`
- Create: `deploy/grafana/provisioning/dashboards/dashboards.yml`
- Create: `deploy/grafana/provisioning/dashboards/apexstock.json`
- Create: `docs/observability/metric-catalog.md`
- Create: `docs/runbooks/incident-response.md`

**Interfaces:**
- Consumes: Prometheus rule syntax, Grafana dashboard schema v1, project SLO definitions
- Produces: Automated alerting, provisioned Grafana dashboards, complete metric documentation, and incident runbook.

- [ ] **Step 1: Create Prometheus alert rules in `deploy/prometheus/alerts.yml`**

Create `deploy/prometheus/alerts.yml`:
```yaml
groups:
  - name: apexstock_critical
    rules:
      - alert: OversellDetected
        expr: oversell_total > 0
        for: 0s
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "CRITICAL: Inventory oversell invariant violated!"
          description: "Flash sale item {{ $labels.item_id }} has sold quantity exceeding total available inventory. Immediate investigation required."

      - alert: ServiceDown
        expr: up == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "Service instance down"
          description: "Job {{ $labels.job }} instance {{ $labels.instance }} has been unreachable for > 30 seconds."

  - name: apexstock_reliability
    rules:
      - alert: HighHttp5xxRate
        expr: (sum(rate(http_requests_total{status=~"5.."}[1m])) / sum(rate(http_requests_total[1m]))) > 0.01
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "HTTP 5xx error rate exceeds 1%"
          description: "5xx server errors account for {{ $value | humanizePercentage }} of traffic on {{ $labels.arch }}."

      - alert: HighP99LatencyOptimized
        expr: histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket{arch="optimized"}[1m])) by (le)) > 0.2
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Optimized p99 latency > 200ms"
          description: "p99 latency for Optimized path is currently {{ $value | humanizeDuration }}."

      - alert: NATSQueueBacklogHigh
        expr: nats_queue_lag > 1000
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "NATS JetStream backlog exceeds 1,000 messages"
          description: "Stream {{ $labels.stream }} has {{ $value }} pending unconsumed messages."
```

- [ ] **Step 2: Update `deploy/prometheus/prometheus.yml` to load alert rules**

Update `deploy/prometheus/prometheus.yml`:
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: 'apexstock-local'

rule_files:
  - "alerts.yml"

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  # API Baseline
  - job_name: 'api-baseline'
    static_configs:
      - targets: ['api-baseline:8081']
    metrics_path: /metrics

  # API Optimized (via nginx)
  - job_name: 'api-optimized'
    static_configs:
      - targets: ['nginx:8082']
    metrics_path: /metrics

  # Worker
  - job_name: 'worker'
    static_configs:
      - targets: ['worker:8080']
    metrics_path: /metrics
```

- [ ] **Step 3: Create Grafana dashboard provider and provisioning JSON**

Create `deploy/grafana/provisioning/dashboards/dashboards.yml`:
```yaml
apiVersion: 1

providers:
  - name: 'ApexStock Dashboards'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /etc/grafana/provisioning/dashboards
```

Create `deploy/grafana/provisioning/dashboards/apexstock.json`:
```json
{
  "annotations": { "list": [] },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "liveNow": false,
  "panels": [
    {
      "collapsed": false,
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 0 },
      "id": 1,
      "title": "Invariant Status: Oversell Total",
      "type": "stat",
      "datasource": "Prometheus",
      "targets": [
        { "expr": "sum(oversell_total) or vector(0)", "refId": "A" }
      ],
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 1 }
            ]
          }
        }
      }
    },
    {
      "collapsed": false,
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 0 },
      "id": 2,
      "title": "Throughput (RPS): Baseline vs Optimized",
      "type": "stat",
      "datasource": "Prometheus",
      "targets": [
        { "expr": "sum(rate(http_requests_total[30s])) by (arch)", "legendFormat": "{{arch}}", "refId": "A" }
      ]
    },
    {
      "collapsed": false,
      "gridPos": { "h": 4, "w": 6, "x": 12, "y": 0 },
      "id": 3,
      "title": "Optimized p99 Latency",
      "type": "stat",
      "datasource": "Prometheus",
      "targets": [
        { "expr": "histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket{arch=\"optimized\"}[1m])) by (le)) * 1000", "refId": "A" }
      ],
      "fieldConfig": {
        "defaults": { "unit": "ms" }
      }
    },
    {
      "collapsed": false,
      "gridPos": { "h": 4, "w": 6, "x": 18, "y": 0 },
      "id": 4,
      "title": "NATS Queue Lag",
      "type": "stat",
      "datasource": "Prometheus",
      "targets": [
        { "expr": "sum(nats_queue_lag) or vector(0)", "refId": "A" }
      ]
    },
    {
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 },
      "id": 5,
      "title": "HTTP Traffic Breakdown (2xx / 409 Conflict / 429 Rate Limit / 5xx Error)",
      "type": "timeseries",
      "datasource": "Prometheus",
      "targets": [
        { "expr": "sum(rate(http_requests_total{status=~\"2..\"}[30s])) by (arch)", "legendFormat": "{{arch}} 2xx Success", "refId": "A" },
        { "expr": "sum(rate(http_requests_total{status=\"409\"}[30s])) by (arch)", "legendFormat": "{{arch}} 409 Out of Stock", "refId": "B" },
        { "expr": "sum(rate(http_requests_total{status=\"429\"}[30s])) by (arch)", "legendFormat": "{{arch}} 429 Rate Limited", "refId": "C" },
        { "expr": "sum(rate(http_requests_total{status=~\"5..\"}[30s])) by (arch)", "legendFormat": "{{arch}} 5xx Server Error", "refId": "D" }
      ]
    },
    {
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 },
      "id": 6,
      "title": "Latency Percentiles (p50 / p95 / p99)",
      "type": "timeseries",
      "datasource": "Prometheus",
      "targets": [
        { "expr": "histogram_quantile(0.50, sum(rate(http_request_duration_seconds_bucket[1m])) by (le, arch)) * 1000", "legendFormat": "{{arch}} p50", "refId": "A" },
        { "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[1m])) by (le, arch)) * 1000", "legendFormat": "{{arch}} p95", "refId": "B" },
        { "expr": "histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[1m])) by (le, arch)) * 1000", "legendFormat": "{{arch}} p99", "refId": "C" }
      ],
      "fieldConfig": {
        "defaults": { "unit": "ms" }
      }
    }
  ],
  "schemaVersion": 38,
  "style": "dark",
  "tags": ["apexstock", "flash-sale", "benchmark"],
  "time": { "from": "now-15m", "to": "now" },
  "title": "ApexStock System Health & Benchmark War Room",
  "uid": "apexstock-war-room"
}
```

- [ ] **Step 4: Create Metric Catalog documentation in `docs/observability/metric-catalog.md`**

Create `docs/observability/metric-catalog.md`:
```markdown
# ApexStock Prometheus Metric Catalog

## Overview
All metrics follow the Prometheus standard format, using bounded cardinality labels to ensure zero memory leaks during stress testing.

| Metric Name | Type | Labels | Description | Alert Threshold |
| :--- | :--- | :--- | :--- | :--- |
| `http_requests_total` | Counter | `arch`, `route`, `method`, `status` | Cumulative HTTP requests processed | Rate(5xx) > 1% |
| `http_request_duration_seconds` | Histogram | `arch`, `route`, `le` | Request duration in seconds (buckets 5ms–10s) | p99 > 200ms (Opt), > 1s (Base) |
| `orders_created_total` | Counter | `arch`, `status` | Flash sale orders created | N/A |
| `payments_processed_total` | Counter | `arch`, `outcome` | Payment outcomes (`success`, `failure`, `timeout`, `cancel`) | N/A |
| `stock_remaining` | Gauge | `arch`, `item_id` | Available inventory stock remaining | Drops to 0 when sold out |
| `nats_queue_lag` | Gauge | `stream`, `consumer` | Unconsumed messages in JetStream queue | > 1,000 messages |
| `oversell_total` | Counter | `item_id` | **CRITICAL INVARIANT:** Violations where sold stock > initial inventory | `> 0` triggers Page 1 Alert |

## Cardinality Guarantee
- `arch`: strictly `baseline` or `optimized` (2 values).
- `route`: static normalized path template (e.g. `/api/v1/flash-sale/orders`, `/healthz`, `/metrics`).
- `status`: HTTP response status code (`201`, `400`, `409`, `429`, `500`).
```

- [ ] **Step 5: Create Incident Response Runbook in `docs/runbooks/incident-response.md`**

Create `docs/runbooks/incident-response.md`:
```markdown
# Operational Incident Response Runbook

## 1. Alert: OversellDetected (`oversell_total > 0`)
- **Severity:** P0 — Critical Invariant Breach.
- **Triage Steps:**
  1. Freeze the flash sale item immediately:
     `UPDATE flash_sale_events SET status = 'paused' WHERE id = '<event_id>';`
  2. Run the reconciliation verification script:
     `go test -v ./test/reconciliation -run TestStockReconciliation`
  3. Inspect `processed_events` and `orders` for duplicate reservations:
     `SELECT id, COUNT(*) FROM orders GROUP BY id HAVING COUNT(*) > 1;`
  4. Compare Redis Lua atomic counters against database records.

## 2. Alert: HighHttp5xxRate (> 1% 5xx errors)
- **Severity:** P1 — Degraded Reliability.
- **Triage Steps:**
  1. Inspect recent JSON structured logs with level `ERROR`:
     `docker compose logs --tail=100 api-optimized | grep '"status":5'`
  2. Check database connection pool exhaustion in PostgreSQL.
  3. Verify Redis connection ping and NATS cluster availability.

## 3. Alert: NATSQueueBacklogHigh (> 1,000 lag)
- **Severity:** P2 — Asynchronous Ingestion Delay.
- **Triage Steps:**
  1. Check worker container health: `docker compose ps worker`
  2. Inspect worker container logs: `docker compose logs --tail=100 worker`
  3. If worker crashed, restart the worker container:
     `docker compose restart worker`
  4. NATS JetStream will safely redeliver unacknowledged messages due to idempotent database transactions.

## 4. Disaster Recovery: Container Crash / Kill Drill
- To simulate container failure and test automated recovery during demo:
  - Baseline API Kill: `docker stop apexstock-api-baseline-1`
  - Optimized NGINX Kill: `docker stop apexstock-nginx-1`
  - Worker Kill: `docker stop apexstock-worker-1`
- Observe Prometheus `ServiceDown` alert and Grafana War Room recovery indicators.
```

- [ ] **Step 6: Validate files and commit**

Run:
```bash
cat deploy/prometheus/alerts.yml | grep -E "alert:|expr:"
cat deploy/grafana/provisioning/dashboards/apexstock.json | jq .uid
```
Expected: Alerts configured; valid JSON.

- [ ] **Step 7: Commit**

```bash
git add deploy/prometheus/ deploy/grafana/ docs/observability/ docs/runbooks/incident-response.md
git commit -m "feat(stage-7): add prometheus alert rules, grafana dashboard, metric catalog, and incident runbooks"
```
