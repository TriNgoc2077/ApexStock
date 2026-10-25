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
	mux.HandleFunc("OPTIONS /api/v1/warroom/run", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})
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
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}

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
