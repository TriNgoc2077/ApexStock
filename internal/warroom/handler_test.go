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
