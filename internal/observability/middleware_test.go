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
