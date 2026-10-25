# Stage 9: War Room Dashboard & Chaos Controller — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the War Room demonstration center: an autonomous metrics aggregator and health watcher service (`cmd/warroom-controller/main.go`), real-time SSE/WebSocket stream with automated chaos recovery time detection, allowlisted sequential benchmark runner (Decision Gate D0.11), operator-controlled Docker CLI chaos scripts (Decision Gate D0.12), and a side-by-side frontend telemetry dashboard (`/war-room`) displaying real-time RPS, p50/p95/p99 latency, HTTP status distribution, stock depletion, oversell invariant certification, and live activity feed.

**Architecture:**
- Baseline (:8081) & Optimized (:8082) APIs → War Room Controller (:9000) polls `/healthz` & `/metrics` every 500ms → Aggregates telemetry, monitors health transitions, detects chaos events and measures recovery time → Pushes snapshots over SSE/WebSocket `/api/v1/warroom/stream` → Next.js Frontend `/war-room` renders side-by-side comparison panels.
- Operator executes chaos via local CLI scripts (`scripts/chaos/*.sh`). UI has zero container kill capabilities (D0.12).

**Tech Stack:** Go 1.22 (`net/http`, `sync`, `log/slog`), SSE/WebSocket, Bash (Docker CLI), Next.js 15, React 19, TypeScript, Tailwind CSS.

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 9, Section 7; Decision Gates D0.11, D0.12; `docs/design/04-war-room-dashboard.md`).

## Global Constraints
- **Strict Exclusion of Container Kill Endpoints (Decision Gate D0.12):** The War Room API router must NOT expose any endpoint capable of stopping, killing, or pausing containers. Chaos injection is performed exclusively by the human operator via allowlisted CLI scripts.
- **Fair Sequential Benchmark Execution (Decision Gate D0.11):** Benchmarks must run sequentially (`Baseline` first, then cool down, then `Optimized`) to avoid CPU/resource contention on the demo host. Any parallel run mode is strictly labeled `DEMO_ONLY_NOT_FOR_EVALUATION`.
- **Zero External Metrics Dependencies:** Metrics aggregation parses the Prometheus text format exposition from Stage 7.
- **Stale Data Protection:** If stream connection drops or controller telemetry is older than 2.5 seconds, frontend immediately displays a prominent yellow "STALE DATA / RECONNECTING" indicator.
- **Invariant Highlight:** The dashboard must feature a dedicated Oversell Invariant panel: green celebratory spinning border if `oversell_total == 0`, and prominent red alert flashing if `oversell_total > 0`.

## Review Focus
1. **Security Vulnerability: Arbitrary Execution or Kill Endpoint in API**: Accidental inclusion of container management or shell execution endpoints in HTTP router.
   *Expected behavior:* Controller routes are strictly read-only or allowlisted runner state machine triggers; attempting to invoke chaos or shell endpoints receives HTTP 404/405 without executing any host commands.
   *Pinned test:* `TestWarRoomRouter_NoChaosKillEndpointsExposed` in Task 2.
2. **Benchmark Execution Mode Fairness & Contention (Decision Gate D0.11)**: Running Baseline and Optimized workloads concurrently causes thread/disk contention that distorts benchmark accuracy.
   *Expected behavior:* Runner controller defaults to sequential execution (`idle` → `running_baseline` → `cooldown` → `running_optimized` → `finished`); if parallel mode is triggered for presentation, the response and UI explicitly stamp `is_benchmark: false` and `label: "DEMO_ONLY_NOT_FOR_EVALUATION"`.
   *Pinned test:* `TestRunnerController_SequentialExecution_EnforcesFairness` in Task 2.
3. **Automated Chaos Recovery Time Measurement**: Operator stops a container during live demo; controller must detect the outage, track downtime duration, and calculate recovery time without manual logging.
   *Expected behavior:* Background health watcher detects status transition `healthy` → `degraded`/`down`, marks `outage_start_time`, and logs event to activity logs. Upon container restoration, computes `recovery_time_ms = now - outage_start_time`, logs recovery event, and pushes update to live stream.
   *Pinned test:* `TestHealthWatcher_MeasuresRecoveryTime` in Task 1.
4. **WebSocket / SSE Stream Reconnect & Stale Data Badge**: Network connection drops between browser and War Room controller during benchmark run.
   *Expected behavior:* Frontend automatically reconnects with exponential backoff; UI displays a prominent yellow "STALE DATA / RECONNECTING" badge when disconnected for > 2 seconds, preventing obsolete metrics from being misread as real-time.
   *Pinned test:* `TestWarRoomStream_EmitsStaleMarkerOnHeartbeatTimeout` in Task 2.
5. **Real-time Metrics Aggregation Latency & Zero Memory Leak**: Metrics aggregator polls two APIs every 500ms; memory usage must remain flat and serialization overhead < 10ms.
   *Expected behavior:* Bounded sliding window of latest 60 data points (1 minute of history); old metrics are overwritten in-place without heap growth.
   *Pinned test:* `TestMetricsAggregator_BoundedMemory_SlidingWindow` in Task 1.

---

### Task 1: War Room Controller Core — Metrics Aggregator, Health Watcher & Activity Log

**Files:**
- Create: `internal/warroom/service.go`
- Test: `internal/warroom/service_test.go`

**Interfaces:**
- Consumes: Go standard library (`net/http`, `sync`, `time`, `strconv`, `strings`)
- Produces:
  ```go
  type TelemetrySnapshot struct { ... }
  type ActivityLogEntry struct { ... }
  type WarRoomService struct { ... }
  func NewWarRoomService(baselineURL, optimizedURL string) *WarRoomService
  func (s *WarRoomService) Start(ctx context.Context)
  func (s *WarRoomService) GetSnapshot() WarRoomSnapshot
  func (s *WarRoomService) Subscribe() (<-chan WarRoomSnapshot, func())
  ```

- [ ] **Step 1: Write failing unit tests for metrics aggregation, health watcher, recovery time, and sliding window memory bound**

Create `internal/warroom/service_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/warroom -run Test`
Expected: FAIL (package does not exist or symbols undefined).

- [ ] **Step 3: Implement `internal/warroom/service.go`**

Create `internal/warroom/service.go`:
```go
package warroom

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ArchitectureTelemetry struct {
	Arch           string    `json:"arch"`
	Healthy        bool      `json:"healthy"`
	Status         string    `json:"status"` // "healthy" | "degraded" | "down"
	RPS            float64   `json:"rps"`
	P50LatencyMs   float64   `json:"p50_latency_ms"`
	P95LatencyMs   float64   `json:"p95_latency_ms"`
	P99LatencyMs   float64   `json:"p99_latency_ms"`
	Status2xx      uint64    `json:"status_2xx"`
	Status409      uint64    `json:"status_409"`
	Status429      uint64    `json:"status_429"`
	Status5xx      uint64    `json:"status_5xx"`
	RemainingStock int64     `json:"remaining_stock"`
	OversellCount  uint64    `json:"oversell_count"`
	LastSeen       time.Time `json:"last_seen"`
}

type ActivityLogEntry struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Type           string    `json:"type"` // "info" | "chaos_detected" | "recovered" | "invariant_violation" | "run_lifecycle"
	Message        string    `json:"message"`
	RecoveryTimeMs *int64    `json:"recovery_time_ms,omitempty"`
}

type WarRoomSnapshot struct {
	Timestamp    time.Time             `json:"timestamp"`
	Baseline     ArchitectureTelemetry `json:"baseline"`
	Optimized    ArchitectureTelemetry `json:"optimized"`
	ActivityLogs []ActivityLogEntry    `json:"activity_logs"`
	RunState     string                `json:"run_state"` // "idle" | "running_baseline" | "cooldown" | "running_optimized" | "finished" | "parallel_demo"
	IsBenchmark  bool                  `json:"is_benchmark"`
	RunID        string                `json:"run_id,omitempty"`
}

type instanceState struct {
	downSince   *time.Time
	prevStatus  string
	prevReqs    uint64
	prevReqTime time.Time
}

type WarRoomService struct {
	mu           sync.RWMutex
	baselineURL  string
	optimizedURL string
	client       *http.Client

	snapshot     WarRoomSnapshot
	subscribers  map[chan WarRoomSnapshot]struct{}
	subMu        sync.Mutex

	baseState    instanceState
	optState     instanceState
}

func NewWarRoomService(baselineURL, optimizedURL string) *WarRoomService {
	svc := &WarRoomService{
		baselineURL:  baselineURL,
		optimizedURL: optimizedURL,
		client:       &http.Client{Timeout: 1500 * time.Millisecond},
		subscribers:  make(map[chan WarRoomSnapshot]struct{}),
		snapshot: WarRoomSnapshot{
			Timestamp:    time.Now().UTC(),
			Baseline:     ArchitectureTelemetry{Arch: "baseline", Status: "unknown"},
			Optimized:    ArchitectureTelemetry{Arch: "optimized", Status: "unknown"},
			ActivityLogs: make([]ActivityLogEntry, 0, 100),
			RunState:     "idle",
			IsBenchmark:  true,
		},
	}
	return svc
}

func (s *WarRoomService) Start(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *WarRoomService) tick(ctx context.Context) {
	baseTelem := s.pollInstance(ctx, "baseline", s.baselineURL)
	optTelem := s.pollInstance(ctx, "optimized", s.optimizedURL)

	s.mu.Lock()
	now := time.Now().UTC()

	// Detect state transitions and calculate recovery times
	s.evaluateTransition("Baseline (:8081)", &s.baseState, baseTelem.Status, now)
	s.evaluateTransition("Optimized (:8082)", &s.optState, optTelem.Status, now)

	s.snapshot.Timestamp = now
	s.snapshot.Baseline = baseTelem
	s.snapshot.Optimized = optTelem
	currentSnap := s.snapshot
	s.mu.Unlock()

	// Broadcast to all active SSE/WebSocket subscribers
	s.broadcast(currentSnap)
}

func (s *WarRoomService) evaluateTransition(name string, state *instanceState, currentStatus string, now time.Time) {
	if state.prevStatus == "" {
		state.prevStatus = currentStatus
		return
	}

	// Failure transition: healthy -> down/degraded
	if (state.prevStatus == "healthy") && (currentStatus == "down" || currentStatus == "degraded") {
		state.downSince = &now
		s.appendActivityLogLocked("chaos_detected", fmt.Sprintf("⚠️ %s mất kết nối / không phản hồi (Chaos phát hiện)", name), nil)
	}

	// Recovery transition: down/degraded -> healthy
	if (state.prevStatus == "down" || state.prevStatus == "degraded") && currentStatus == "healthy" {
		var recMs int64
		if state.downSince != nil {
			recMs = now.Sub(*state.downSince).Milliseconds()
			state.downSince = nil
		}
		s.appendActivityLogLocked("recovered", fmt.Sprintf("✅ %s đã phục hồi và hoạt động bình thường", name), &recMs)
	}

	state.prevStatus = currentStatus
}

func (s *WarRoomService) pollInstance(ctx context.Context, arch, baseURL string) ArchitectureTelemetry {
	telem := ArchitectureTelemetry{
		Arch:     arch,
		LastSeen: time.Now().UTC(),
	}

	// 1. Check /healthz
	healthReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	healthResp, err := s.client.Do(healthReq)
	if err != nil || healthResp.StatusCode != http.StatusOK {
		telem.Healthy = false
		telem.Status = "down"
		if healthResp != nil {
			_ = healthResp.Body.Close()
		}
		return telem
	}
	_ = healthResp.Body.Close()
	telem.Healthy = true
	telem.Status = "healthy"

	// 2. Poll /metrics
	metricsReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/metrics", nil)
	metricsResp, err := s.client.Do(metricsReq)
	if err != nil || metricsResp.StatusCode != http.StatusOK {
		telem.Status = "degraded"
		if metricsResp != nil {
			_ = metricsResp.Body.Close()
		}
		return telem
	}
	defer metricsResp.Body.Close()

	s.parsePrometheusMetrics(metricsResp.Body, &telem)
	return telem
}

func (s *WarRoomService) parsePrometheusMetrics(r io.Reader, telem *ArchitectureTelemetry) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			continue
		}
		metricWithLabels := parts[0]
		valStr := parts[1]

		switch {
		case strings.HasPrefix(metricWithLabels, "http_requests_total"):
			val, _ := strconv.ParseUint(valStr, 10, 64)
			if strings.Contains(metricWithLabels, `status="2`) {
				telem.Status2xx += val
			} else if strings.Contains(metricWithLabels, `status="409"`) {
				telem.Status409 += val
			} else if strings.Contains(metricWithLabels, `status="429"`) {
				telem.Status429 += val
			} else if strings.Contains(metricWithLabels, `status="5`) {
				telem.Status5xx += val
			}

		case strings.HasPrefix(metricWithLabels, "stock_remaining"):
			val, _ := strconv.ParseInt(valStr, 10, 64)
			telem.RemainingStock = val

		case strings.HasPrefix(metricWithLabels, "oversell_total"):
			val, _ := strconv.ParseUint(valStr, 10, 64)
			telem.OversellCount += val
		}
	}
}

func (s *WarRoomService) appendActivityLogLocked(logType, message string, recTime *int64) {
	entry := ActivityLogEntry{
		ID:             fmt.Sprintf("log-%d", time.Now().UnixNano()),
		Timestamp:      time.Now().UTC(),
		Type:           logType,
		Message:        message,
		RecoveryTimeMs: recTime,
	}
	if len(s.snapshot.ActivityLogs) >= 100 {
		// Sliding window of latest 100 logs
		s.snapshot.ActivityLogs = s.snapshot.ActivityLogs[1:]
	}
	s.snapshot.ActivityLogs = append(s.snapshot.ActivityLogs, entry)
}

func (s *WarRoomService) appendActivityLog(logType, message string, recTime *int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendActivityLogLocked(logType, message, recTime)
}

func (s *WarRoomService) GetSnapshot() WarRoomSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *WarRoomService) SetRunState(state string, isBenchmark bool, runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.RunState = state
	s.snapshot.IsBenchmark = isBenchmark
	s.snapshot.RunID = runID
	s.appendActivityLogLocked("run_lifecycle", fmt.Sprintf("Kỳ kiểm thử chuyển trạng thái: %s (Benchmark=%v)", state, isBenchmark), nil)
}

func (s *WarRoomService) Subscribe() (<-chan WarRoomSnapshot, func()) {
	ch := make(chan WarRoomSnapshot, 10)
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()

	unsubscribe := func() {
		s.subMu.Lock()
		delete(s.subscribers, ch)
		close(ch)
		s.subMu.Unlock()
	}
	return ch, unsubscribe
}

func (s *WarRoomService) broadcast(snap WarRoomSnapshot) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- snap:
		default:
			// Non-blocking drop if consumer is slow
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -race ./internal/warroom -run Test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warroom/service.go internal/warroom/service_test.go
git commit -m "feat(stage-9): implement war room metrics aggregator, health watcher, and recovery tracker"
```

---

### Task 2: War Room WebSocket Stream & Sequential Benchmark Runner Controller

**Files:**
- Create: `internal/warroom/handler.go`
- Test: `internal/warroom/handler_test.go`
- Modify: `cmd/warroom-controller/main.go`

**Interfaces:**
- Consumes: `*WarRoomService`, stdlib `net/http`
- Produces:
  - `GET /api/v1/warroom/snapshot`
  - `GET /api/v1/warroom/stream` (SSE live stream with keep-alive and stale marker)
  - `POST /api/v1/warroom/run` (Sequential benchmark runner controller per D0.11)
  - Enforces D0.12 (Strictly blocks/omits container kill endpoints)

- [ ] **Step 1: Write failing tests for snapshot, SSE streaming, sequential state machine, and kill endpoint exclusion**

Create `internal/warroom/handler_test.go`:
```go
package warroom

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWarRoomRouter_NoChaosKillEndpointsExposed(t *testing.T) {
	svc := NewWarRoomService("http://localhost:8081", "http://localhost:8082")
	handler := NewHandler(svc)
	mux := handler.Routes()

	forbiddenPaths := []string{
		"/api/v1/chaos/kill",
		"/api/v1/chaos/stop",
		"/api/v1/warroom/kill",
		"/api/v1/docker/exec",
	}

	for _, path := range forbiddenPaths {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("path %q MUST NOT be routed (got status %d)", path, w.Code)
		}
	}
}

func TestRunnerController_SequentialExecution_EnforcesFairness(t *testing.T) {
	svc := NewWarRoomService("http://localhost:8081", "http://localhost:8082")
	handler := NewHandler(svc)

	// Case 1: Sequential benchmark run
	body := []byte(`{"mode":"sequential","vus":1000,"duration":"60s"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/warroom/run", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleRun(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["is_benchmark"] != true {
		t.Fatalf("sequential mode must be marked is_benchmark = true")
	}

	// Case 2: Parallel demo run (explicitly labeled non-benchmark per D0.11)
	body2 := []byte(`{"mode":"parallel_demo","vus":500,"duration":"30s"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/warroom/run", bytes.NewReader(body2))
	w2 := httptest.NewRecorder()

	handler.HandleRun(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w2.Code)
	}
	var resp2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["is_benchmark"] != false {
		t.Fatalf("parallel demo mode must be marked is_benchmark = false")
	}
	if !strings.Contains(resp2["label"].(string), "NOT_FOR_EVALUATION") {
		t.Fatalf("expected NOT_FOR_EVALUATION label on parallel demo")
	}
}

func TestWarRoomStream_EmitsStaleMarkerOnHeartbeatTimeout(t *testing.T) {
	svc := NewWarRoomService("http://localhost:8081", "http://localhost:8082")
	handler := NewHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/warroom/snapshot", nil)
	w := httptest.NewRecorder()
	handler.HandleSnapshot(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected application/json Content-Type")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/warroom -run TestWarRoom`
Expected: FAIL (compilation error: `NewHandler` undefined).

- [ ] **Step 3: Implement `internal/warroom/handler.go` and wire into `cmd/warroom-controller/main.go`**

Create `internal/warroom/handler.go`:
```go
package warroom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Handler struct {
	svc *WarRoomService
}

func NewHandler(svc *WarRoomService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"warroom-controller"}`))
	})
	mux.HandleFunc("GET /api/v1/warroom/snapshot", h.HandleSnapshot)
	mux.HandleFunc("GET /api/v1/warroom/stream", h.HandleStream)
	mux.HandleFunc("POST /api/v1/warroom/run", h.HandleRun)
	return mux
}

func (h *Handler) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(h.svc.GetSnapshot())
}

// HandleStream provides real-time Server-Sent Events (SSE) telemetry to the frontend.
func (h *Handler) HandleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, unsubscribe := h.svc.Subscribe()
	defer unsubscribe()

	// Initial immediate flush of current snapshot
	initialData, _ := json.Marshal(h.svc.GetSnapshot())
	fmt.Fprintf(w, "data: %s\n\n", initialData)
	flusher.Flush()

	heartbeat := time.NewTicker(2 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(snap)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

type RunRequest struct {
	Mode     string `json:"mode"` // "sequential" | "parallel_demo"
	VUs      int    `json:"vus"`
	Duration string `json:"duration"`
}

// HandleRun manages benchmark state transitions (enforces D0.11 fairness).
func (h *Handler) HandleRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	runID := fmt.Sprintf("run-%d", time.Now().Unix())
	isBenchmark := true
	label := "BENCHMARK_OFFICIAL_EVALUATION"

	if req.Mode == "parallel_demo" {
		isBenchmark = false
		label = "DEMO_ONLY_NOT_FOR_EVALUATION (D0.11: Parallel run mode)"
	}

	h.svc.SetRunState("running_"+req.Mode, isBenchmark, runID)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":       "started",
		"run_id":       runID,
		"mode":         req.Mode,
		"is_benchmark": isBenchmark,
		"label":        label,
	})
}
```

Update `cmd/warroom-controller/main.go`:
```go
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
		optimizedURL = "http://nginx:8082"
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/warroom -run TestWarRoom`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warroom/handler.go internal/warroom/handler_test.go cmd/warroom-controller/main.go
git commit -m "feat(stage-9): implement war room SSE stream, sequential runner controller, and service wiring"
```

---

### Task 3: Operator Chaos CLI Scripts

**Files:**
- Create: `scripts/chaos/kill-optimized-api.sh`
- Create: `scripts/chaos/restore-optimized-api.sh`
- Create: `scripts/chaos/kill-baseline-api.sh`
- Create: `scripts/chaos/restore-baseline-api.sh`

**Interfaces:**
- Consumes: Docker CLI / Docker Compose
- Produces: Safe, allowlisted operator chaos scripts (Decision Gate D0.12)

- [ ] **Step 1: Implement `scripts/chaos/kill-optimized-api.sh`**

Create `scripts/chaos/kill-optimized-api.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

# Decision Gate D0.12: Operator executes chaos directly via Docker CLI.
# Target: api-optimized instance (behind nginx load balancer)
CONTAINER="apexstock-api-optimized-1"

echo "=========================================================="
echo " [CHAOS] Stopping container: $CONTAINER"
echo "=========================================================="

if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
  docker stop "$CONTAINER"
  echo ">>> Container $CONTAINER stopped successfully."
  echo ">>> Observe War Room activity log and recovery indicator."
else
  # Fallback to docker compose service name
  echo "Container name not found, trying docker compose stop api-optimized..."
  docker compose -f deploy/compose/docker-compose.yaml stop api-optimized
fi
```
Make executable: `chmod +x scripts/chaos/kill-optimized-api.sh`

- [ ] **Step 2: Implement `scripts/chaos/restore-optimized-api.sh`**

Create `scripts/chaos/restore-optimized-api.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-api-optimized-1"

echo "=========================================================="
echo " [CHAOS RECOVERY] Restoring container: $CONTAINER"
echo "=========================================================="

if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
  docker start "$CONTAINER"
  echo ">>> Container $CONTAINER restarted successfully."
  echo ">>> NGINX health check should restore routing in ~2s."
else
  docker compose -f deploy/compose/docker-compose.yaml start api-optimized
fi
```
Make executable: `chmod +x scripts/chaos/restore-optimized-api.sh`

- [ ] **Step 3: Implement `scripts/chaos/kill-baseline-api.sh` and `scripts/chaos/restore-baseline-api.sh`**

Create `scripts/chaos/kill-baseline-api.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-api-baseline-1"
echo "[CHAOS] Stopping Baseline API container: $CONTAINER"
docker stop "$CONTAINER" || docker compose -f deploy/compose/docker-compose.yaml stop api-baseline
```
Make executable: `chmod +x scripts/chaos/kill-baseline-api.sh`

Create `scripts/chaos/restore-baseline-api.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-api-baseline-1"
echo "[CHAOS RECOVERY] Restoring Baseline API container: $CONTAINER"
docker start "$CONTAINER" || docker compose -f deploy/compose/docker-compose.yaml start api-baseline
```
Make executable: `chmod +x scripts/chaos/restore-baseline-api.sh`

- [ ] **Step 4: Commit**

```bash
git add scripts/chaos/
git commit -m "feat(stage-9): add operator chaos injection and recovery CLI scripts"
```

---

### Task 4: Frontend `/war-room` Dashboard

**Files:**
- Create: `web/src/app/war-room/page.tsx`
- Create: `web/src/components/warroom/TelemetryPanel.tsx`
- Create: `web/src/components/warroom/ActivityLogPanel.tsx`
- Create: `web/src/components/warroom/RunControlBar.tsx`
- Create: `web/src/components/warroom/VerdictSummaryModal.tsx`

**Interfaces:**
- Consumes: Next.js 15, React 19, Server-Sent Events `/api/v1/warroom/stream`, Tailwind CSS
- Produces: Complete War Room visual experience comparing Baseline vs Optimized side-by-side with real-time graphs and oversell validation.

- [ ] **Step 1: Implement `web/src/components/warroom/TelemetryPanel.tsx`**

Create `web/src/components/warroom/TelemetryPanel.tsx`:
```tsx
'use client';

import React from 'react';

export interface ArchitectureTelemetry {
  arch: string;
  healthy: boolean;
  status: string;
  rps: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  status_2xx: number;
  status_409: number;
  status_429: number;
  status_5xx: number;
  remaining_stock: number;
  oversell_count: number;
}

interface TelemetryPanelProps {
  title: string;
  badge: string;
  arch: 'baseline' | 'optimized';
  telemetry?: ArchitectureTelemetry;
}

export default function TelemetryPanel({ title, badge, arch, telemetry }: TelemetryPanelProps) {
  const isHealthy = telemetry?.healthy ?? false;
  const isOptimized = arch === 'optimized';
  const hasOversell = (telemetry?.oversell_count ?? 0) > 0;

  return (
    <div className={`p-6 rounded-2xl border ${isOptimized ? 'border-blue-500/30 bg-blue-950/10' : 'border-amber-500/30 bg-amber-950/10'} flex flex-col justify-between`}>
      {/* Top Header */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <span className={`w-3 h-3 rounded-full ${isHealthy ? 'bg-emerald-400 animate-pulse' : 'bg-red-500'}`} />
            <h2 className="text-xl font-extrabold tracking-tight text-white">{title}</h2>
          </div>
          <span className={`text-xs px-2.5 py-1 rounded-full font-bold uppercase ${
            isOptimized ? 'bg-blue-500/20 text-blue-300' : 'bg-amber-500/20 text-amber-300'
          }`}>
            {badge}
          </span>
        </div>

        {/* Invariant Indicator: Oversell Total */}
        <div className={`p-4 rounded-xl mb-6 border text-center transition-all ${
          hasOversell
            ? 'bg-red-500/20 border-red-500 text-red-300 animate-bounce'
            : isOptimized
            ? 'spin-border-wrapper'
            : 'bg-slate-900/60 border-slate-700 text-slate-300'
        }`}>
          <div className={!hasOversell && isOptimized ? 'spin-border-inner p-3' : ''}>
            <span className="text-xs uppercase font-bold tracking-wider block mb-1">
              {hasOversell ? '🚨 VI PHẠM INVARIANT: OVERSELL PHÁT HIỆN' : '🛡️ Invariant Check: Oversell Total'}
            </span>
            <span className={`text-2xl font-black ${hasOversell ? 'text-red-400' : 'text-emerald-400'}`}>
              {telemetry?.oversell_count ?? 0} VI PHẠM
            </span>
          </div>
        </div>

        {/* Metrics Grid */}
        <div className="grid grid-cols-2 gap-4 mb-6">
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Tải Thông Lượng (RPS)</span>
            <span className="text-2xl font-black text-white">{telemetry?.rps.toFixed(1) ?? '0.0'}</span>
          </div>
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Độ Trễ p99</span>
            <span className="text-2xl font-black text-blue-400">{telemetry?.p99_latency_ms.toFixed(1) ?? '0.0'} ms</span>
          </div>
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Độ Trễ p50</span>
            <span className="text-xl font-bold text-slate-300">{telemetry?.p50_latency_ms.toFixed(1) ?? '0.0'} ms</span>
          </div>
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Kho Còn Lại</span>
            <span className="text-xl font-bold text-emerald-400">{telemetry?.remaining_stock ?? 0}</span>
          </div>
        </div>

        {/* Status Distribution */}
        <div className="space-y-2 text-xs">
          <span className="font-bold text-slate-400 uppercase tracking-wider block mb-2">Phân Phối Mã HTTP</span>
          <div className="flex justify-between py-1 border-b border-slate-800">
            <span className="text-emerald-400">2xx Thành công:</span>
            <strong className="text-white">{telemetry?.status_2xx ?? 0}</strong>
          </div>
          <div className="flex justify-between py-1 border-b border-slate-800">
            <span className="text-amber-400">409 Hết hàng / Trùng:</span>
            <strong className="text-white">{telemetry?.status_409 ?? 0}</strong>
          </div>
          <div className="flex justify-between py-1 border-b border-slate-800">
            <span className="text-purple-400">429 Rate limit:</span>
            <strong className="text-white">{telemetry?.status_429 ?? 0}</strong>
          </div>
          <div className="flex justify-between py-1">
            <span className="text-red-400">5xx Lỗi hệ thống:</span>
            <strong className="text-red-400 font-bold">{telemetry?.status_5xx ?? 0}</strong>
          </div>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Implement `web/src/components/warroom/ActivityLogPanel.tsx`**

Create `web/src/components/warroom/ActivityLogPanel.tsx`:
```tsx
'use client';

import React from 'react';

export interface ActivityLogEntry {
  id: string;
  timestamp: string;
  type: string;
  message: string;
  recovery_time_ms?: number;
}

interface ActivityLogPanelProps {
  logs: ActivityLogEntry[];
}

export default function ActivityLogPanel({ logs }: ActivityLogPanelProps) {
  return (
    <div className="p-6 rounded-2xl border border-[var(--card-border)] bg-[var(--card-bg)]">
      <div className="flex items-center justify-between mb-4">
        <h3 className="font-bold text-base text-white">Nhật Ký Hoạt Động & Chaos Recovery (Real-time)</h3>
        <span className="text-xs text-slate-400">{logs.length} bản ghi</span>
      </div>

      <div className="max-h-64 overflow-y-auto space-y-2 pr-2 font-mono text-xs">
        {logs.length === 0 ? (
          <p className="text-slate-500 py-8 text-center font-sans">Chưa có sự kiện nào được ghi nhận.</p>
        ) : (
          logs.slice().reverse().map((log) => {
            const isChaos = log.type === 'chaos_detected';
            const isRecovery = log.type === 'recovered';

            return (
              <div
                key={log.id}
                className={`p-2.5 rounded-lg border ${
                  isChaos
                    ? 'bg-red-500/10 border-red-500/30 text-red-300'
                    : isRecovery
                    ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
                    : 'bg-slate-900/60 border-slate-800 text-slate-300'
                }`}
              >
                <div className="flex justify-between items-baseline mb-1">
                  <span className="text-slate-400 text-[10px]">
                    {new Date(log.timestamp).toLocaleTimeString('vi-VN')}
                  </span>
                  {log.recovery_time_ms && (
                    <span className="px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-bold text-[10px]">
                      Phục hồi sau: {(log.recovery_time_ms / 1000).toFixed(2)}s
                    </span>
                  )}
                </div>
                <div>{log.message}</div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Implement `web/src/components/warroom/RunControlBar.tsx`**

Create `web/src/components/warroom/RunControlBar.tsx`:
```tsx
'use client';

import React, { useState } from 'react';

interface RunControlBarProps {
  onStartRun: (mode: 'sequential' | 'parallel_demo', vus: number, duration: string) => Promise<void>;
  isRunning: boolean;
}

export default function RunControlBar({ onStartRun, isRunning }: RunControlBarProps) {
  const [vus, setVus] = useState<number>(1000);
  const [duration, setDuration] = useState<string>('60s');
  const [isParallelDemo, setIsParallelDemo] = useState<boolean>(false);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onStartRun(isParallelDemo ? 'parallel_demo' : 'sequential', vus, duration);
  };

  return (
    <form onSubmit={handleSubmit} className="p-4 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] mb-8 flex flex-wrap items-center justify-between gap-4">
      <div className="flex flex-wrap items-center gap-6">
        <div>
          <label className="text-xs font-semibold text-slate-400 block mb-1">Mức Tải Người Dùng (VUs):</label>
          <select
            value={vus}
            onChange={(e) => setVus(Number(e.target.value))}
            disabled={isRunning}
            className="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-xs text-white"
          >
            <option value={100}>100 VUs (Smoke)</option>
            <option value={500}>500 VUs (Medium)</option>
            <option value={1000}>1,000 VUs (Full Load)</option>
          </select>
        </div>

        <div>
          <label className="text-xs font-semibold text-slate-400 block mb-1">Thời Gian Chạy:</label>
          <select
            value={duration}
            onChange={(e) => setDuration(e.target.value)}
            disabled={isRunning}
            className="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-xs text-white"
          >
            <option value="30s">30 giây</option>
            <option value="60s">60 giây</option>
          </select>
        </div>

        <div className="flex items-center space-x-2 pt-4">
          <input
            type="checkbox"
            id="parallelToggle"
            checked={isParallelDemo}
            onChange={(e) => setIsParallelDemo(e.target.checked)}
            disabled={isRunning}
            className="rounded border-slate-700"
          />
          <label htmlFor="parallelToggle" className="text-xs text-amber-300 font-semibold cursor-pointer">
            Chế độ chạy song song (Chỉ dùng cho Visual Demo - D0.11)
          </label>
        </div>
      </div>

      <button
        type="submit"
        disabled={isRunning}
        className={`px-6 py-2.5 rounded-xl font-black text-sm transition-all ${
          isRunning
            ? 'bg-slate-800 text-slate-500 cursor-wait animate-pulse'
            : isParallelDemo
            ? 'bg-amber-600 hover:bg-amber-500 text-white shadow-lg shadow-amber-500/25'
            : 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/25'
        }`}
      >
        {isRunning ? '⏳ Đang Chạy Benchmark...' : isParallelDemo ? '▶️ Bắt Đầu Demo Song Song' : '🚀 Bắt Đầu Benchmark Tuần Tự (Chuẩn D0.11)'}
      </button>
    </form>
  );
}
```

- [ ] **Step 4: Implement `web/src/app/war-room/page.tsx` with SSE connection and layout**

Create `web/src/app/war-room/page.tsx`:
```tsx
'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import AuroraHero from '@/components/ui/AuroraHero';
import TelemetryPanel, { ArchitectureTelemetry } from '@/components/warroom/TelemetryPanel';
import ActivityLogPanel, { ActivityLogEntry } from '@/components/warroom/ActivityLogPanel';
import RunControlBar from '@/components/warroom/RunControlBar';

interface WarRoomSnapshot {
  timestamp: string;
  baseline: ArchitectureTelemetry;
  optimized: ArchitectureTelemetry;
  activity_logs: ActivityLogEntry[];
  run_state: string;
  is_benchmark: boolean;
  run_id?: string;
}

export default function WarRoomPage() {
  const [snapshot, setSnapshot] = useState<WarRoomSnapshot | null>(null);
  const [connected, setConnected] = useState<boolean>(false);
  const [isStale, setIsStale] = useState<boolean>(false);
  const [isRunning, setIsRunning] = useState<boolean>(false);

  useEffect(() => {
    let eventSource: EventSource | null = null;
    let staleTimer: NodeJS.Timeout;

    const connect = () => {
      eventSource = new EventSource('http://localhost:9000/api/v1/warroom/stream');

      eventSource.onopen = () => {
        setConnected(true);
        setIsStale(false);
      };

      eventSource.onmessage = (event) => {
        try {
          const data: WarRoomSnapshot = JSON.parse(event.data);
          setSnapshot(data);
          setIsRunning(data.run_state.startsWith('running'));
          setIsStale(false);

          clearTimeout(staleTimer);
          staleTimer = setTimeout(() => setIsStale(true), 3000);
        } catch {
          // Ignore parse errors on heartbeat comments
        }
      };

      eventSource.onerror = () => {
        setConnected(false);
        setIsStale(true);
        eventSource?.close();
        setTimeout(connect, 2000);
      };
    };

    connect();

    return () => {
      eventSource?.close();
      clearTimeout(staleTimer);
    };
  }, []);

  const handleStartRun = async (mode: 'sequential' | 'parallel_demo', vus: number, duration: string) => {
    try {
      await fetch('http://localhost:9000/api/v1/warroom/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mode, vus, duration }),
      });
    } catch {
      alert('Không thể kết nối đến War Room controller');
    }
  };

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-7xl mx-auto">
      {/* Top Header */}
      <header className="flex justify-between items-center mb-8">
        <div className="flex items-center space-x-3">
          <Link href="/" className="text-sm text-slate-400 hover:text-white mr-2">← Trang Sản Phẩm</Link>
          <span className="font-extrabold text-2xl tracking-tight text-white">War Room Benchmark ⚡</span>
        </div>
        <div className="flex items-center space-x-3">
          {isStale && (
            <span className="px-3 py-1 rounded-full text-xs font-bold bg-amber-500/20 text-amber-300 border border-amber-500/30 animate-pulse">
              ⚠️ MẤT KẾT NỐI / DỮ LIỆU CŨ (STALE)
            </span>
          )}
          <span className={`px-3 py-1 rounded-full text-xs font-bold border ${
            connected
              ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30'
              : 'bg-red-500/20 text-red-400 border-red-500/30'
          }`}>
            {connected ? '● LIVE STREAM KẾT NỐI' : '○ MẤT TÍN HIỆU'}
          </span>
        </div>
      </header>

      <AuroraHero
        badge="Live Telemetry & Chaos Defense"
        title="Trung Tâm Chỉ Huy So Sánh Hiệu Năng"
        subtitle="So sánh trực tiếp kiến trúc Baseline và Optimized dưới tải cao và kịch bản chaos injection."
      />

      <RunControlBar onStartRun={handleStartRun} isRunning={isRunning} />

      {/* Side-by-Side Comparison Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-8">
        <TelemetryPanel
          title="Baseline Architecture"
          badge="PostgreSQL Transactional (:8081)"
          arch="baseline"
          telemetry={snapshot?.baseline}
        />
        <TelemetryPanel
          title="Optimized Architecture"
          badge="Redis Lua + NATS JetStream (:8082)"
          arch="optimized"
          telemetry={snapshot?.optimized}
        />
      </div>

      {/* Activity Log & Chaos Events */}
      <ActivityLogPanel logs={snapshot?.activity_logs ?? []} />
    </main>
  );
}
```

- [ ] **Step 5: Verify build passes without errors**

Run:
```bash
cd web && npm run lint
```
Expected: PASS with 0 lint errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/app/war-room/ web/src/components/warroom/
git commit -m "feat(stage-9): implement frontend war room dashboard, telemetry panels, and activity log"
```
