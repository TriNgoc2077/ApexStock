package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apexstock/internal/domain"
	"apexstock/internal/payments"
)

type dummyProcessor struct {
	outcome payments.PaymentResult
	err     error
}

func (d *dummyProcessor) Process(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
	if d.err != nil {
		return payments.PaymentResult{}, d.err
	}
	return d.outcome, nil
}

func TestSimulatePaymentHandler_Success(t *testing.T) {
	proc := &dummyProcessor{
		outcome: payments.PaymentResult{
			OrderID:          "11111111-1111-1111-1111-111111111111",
			OrderStatus:      "paid",
			PaymentStatus:    "success",
			IdempotentReplay: false,
		},
	}
	sim := payments.NewSimulator(proc, payments.DefaultSimulatorConfig())
	handler := SimulatePaymentHandler(sim)

	reqBody := domain.SimulatePaymentRequest{
		OrderID: "11111111-1111-1111-1111-111111111111",
		Outcome: "success",
	}
	payload, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/payments/simulate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp domain.PaymentResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.OrderStatus != "paid" || resp.PaymentStatus != "success" {
		t.Errorf("unexpected body: %+v", resp)
	}
}

func TestSimulatePaymentHandler_LateRefund409(t *testing.T) {
	proc := &dummyProcessor{
		err: payments.ErrOrderExpiredRefund,
	}
	sim := payments.NewSimulator(proc, payments.DefaultSimulatorConfig())
	handler := SimulatePaymentHandler(sim)

	reqBody := domain.SimulatePaymentRequest{
		OrderID: "11111111-1111-1111-1111-111111111111",
		Outcome: "success",
	}
	payload, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/payments/simulate", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on late refund, got %d: %s", rec.Code, rec.Body.String())
	}
}
