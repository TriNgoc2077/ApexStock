package warroom

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
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

	snapshot    WarRoomSnapshot
	subscribers map[chan WarRoomSnapshot]struct{}
	subMu       sync.Mutex

	baseState instanceState
	optState  instanceState
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

func computeRPS(telem *ArchitectureTelemetry, state *instanceState, now time.Time) {
	totalReqs := telem.Status2xx + telem.Status409 + telem.Status429 + telem.Status5xx
	if !state.prevReqTime.IsZero() && totalReqs >= state.prevReqs {
		elapsed := now.Sub(state.prevReqTime).Seconds()
		if elapsed > 0 {
			telem.RPS = float64(totalReqs-state.prevReqs) / elapsed
		}
	}
	state.prevReqs = totalReqs
	state.prevReqTime = now
}

func (s *WarRoomService) tick(ctx context.Context) {
	baseTelem := s.pollInstance(ctx, "baseline", s.baselineURL)
	optTelem := s.pollInstance(ctx, "optimized", s.optimizedURL)

	s.mu.Lock()
	now := time.Now().UTC()

	computeRPS(&baseTelem, &s.baseState, now)
	computeRPS(&optTelem, &s.optState, now)

	// Detect state transitions and calculate recovery times
	s.evaluateTransition("Baseline (:8081)", &s.baseState, baseTelem.Status, now)
	s.evaluateTransition("Optimized (:8082)", &s.optState, optTelem.Status, now)

	s.snapshot.Timestamp = now
	s.snapshot.Baseline = baseTelem
	s.snapshot.Optimized = optTelem
	currentSnap := s.cloneSnapshotLocked()
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
	bucketCounts := make(map[float64]uint64)
	var infCount uint64

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

		case strings.HasPrefix(metricWithLabels, "http_request_duration_seconds_bucket"):
			val, _ := strconv.ParseUint(valStr, 10, 64)
			leIdx := strings.Index(metricWithLabels, `le="`)
			if leIdx != -1 {
				start := leIdx + 4
				end := strings.Index(metricWithLabels[start:], `"`)
				if end != -1 {
					leVal := metricWithLabels[start : start+end]
					if leVal == "+Inf" {
						infCount += val
					} else if bound, err := strconv.ParseFloat(leVal, 64); err == nil {
						bucketCounts[bound] += val
					}
				}
			}

		case strings.HasPrefix(metricWithLabels, "stock_remaining"):
			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil {
				telem.RemainingStock = int64(val)
			}

		case strings.HasPrefix(metricWithLabels, "oversell_total"):
			val, _ := strconv.ParseUint(valStr, 10, 64)
			telem.OversellCount += val
		}
	}

	// Compute histogram quantiles in milliseconds (seconds * 1000)
	if len(bucketCounts) > 0 {
		bounds := make([]float64, 0, len(bucketCounts))
		for b := range bucketCounts {
			bounds = append(bounds, b)
		}
		sort.Float64s(bounds)

		totalCount := infCount
		if totalCount == 0 {
			totalCount = bucketCounts[bounds[len(bounds)-1]]
		}

		if totalCount > 0 {
			calcQuantile := func(q float64) float64 {
				rank := q * float64(totalCount)
				var prevBound float64
				var prevCount uint64
				for _, b := range bounds {
					c := bucketCounts[b]
					if float64(c) >= rank {
						bucketDelta := float64(c - prevCount)
						if bucketDelta > 0 {
							fraction := (rank - float64(prevCount)) / bucketDelta
							return (prevBound + (b-prevBound)*fraction) * 1000.0
						}
						return b * 1000.0
					}
					prevBound = b
					prevCount = c
				}
				return bounds[len(bounds)-1] * 1000.0
			}

			telem.P50LatencyMs = calcQuantile(0.50)
			telem.P95LatencyMs = calcQuantile(0.95)
			telem.P99LatencyMs = calcQuantile(0.99)
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

func (s *WarRoomService) cloneSnapshotLocked() WarRoomSnapshot {
	snap := s.snapshot
	if len(s.snapshot.ActivityLogs) > 0 {
		snap.ActivityLogs = append([]ActivityLogEntry(nil), s.snapshot.ActivityLogs...)
	}
	return snap
}

func (s *WarRoomService) GetSnapshot() WarRoomSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cloneSnapshotLocked()
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
