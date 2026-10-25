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

func (r *Registry) GetOrCreateCounter(name, help string, labels ...string) *CounterVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.counters {
		if c.name == name {
			return c
		}
	}
	c := &CounterVec{name: name, help: help, labels: labels}
	r.counters = append(r.counters, c)
	return c
}

func (r *Registry) GetOrCreateGauge(name, help string, labels ...string) *GaugeVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.gauges {
		if g.name == name {
			return g
		}
	}
	g := &GaugeVec{name: name, help: help, labels: labels}
	r.gauges = append(r.gauges, g)
	return g
}

func (r *Registry) GetOrCreateHistogram(name, help string, buckets []float64, labels ...string) *HistogramVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range r.histograms {
		if h.name == name {
			return h
		}
	}
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
			for i, bound := range h.buckets {
				val := atomic.LoadUint64(&hs.buckets[i])
				bLabels := append([]string{}, vals...)
				bNames := append([]string{}, h.labels...)
				bNames = append(bNames, "le")
				bLabels = append(bLabels, fmt.Sprintf("%v", bound))
				sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.name, formatLabels(bNames, bLabels), val))
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
	DefaultRegistry.GetOrCreateCounter(
		"http_requests_total",
		"Total number of HTTP requests processed by route, arch, and status code",
		"arch", "method", "route", "status",
	).WithLabels(arch, method, route, status).Inc()
}

func ObserveHTTPRequestDuration(arch, route string, seconds float64) {
	DefaultRegistry.GetOrCreateHistogram(
		"http_request_duration_seconds",
		"HTTP request latency histogram in seconds",
		StandardLatencyBuckets,
		"arch", "route",
	).WithLabels(arch, route).Observe(seconds)
}

func IncOrdersCreated(arch, status string) {
	DefaultRegistry.GetOrCreateCounter(
		"orders_created_total",
		"Total flash sale orders created by architecture and status",
		"arch", "status",
	).WithLabels(arch, status).Inc()
}

func IncPaymentsProcessed(arch, outcome string) {
	DefaultRegistry.GetOrCreateCounter(
		"payments_processed_total",
		"Total payment transitions processed by architecture and outcome",
		"arch", "outcome",
	).WithLabels(arch, outcome).Inc()
}

func SetStockRemaining(arch, itemID string, stock float64) {
	DefaultRegistry.GetOrCreateGauge(
		"stock_remaining",
		"Current remaining available inventory stock",
		"arch", "item_id",
	).WithLabels(arch, itemID).Set(stock)
}

func SetNATSQueueLag(stream, consumer string, lag float64) {
	DefaultRegistry.GetOrCreateGauge(
		"nats_queue_lag",
		"Number of pending messages in NATS JetStream queue",
		"stream", "consumer",
	).WithLabels(stream, consumer).Set(lag)
}

func IncOversell(itemID string) {
	DefaultRegistry.GetOrCreateCounter(
		"oversell_total",
		"CRITICAL: Total invariant violations where stock was oversold beyond initial quantity",
		"item_id",
	).WithLabels(itemID).Inc()
}
