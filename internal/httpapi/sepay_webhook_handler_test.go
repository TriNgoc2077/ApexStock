package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apexstock/internal/payments"
	"apexstock/internal/payments/sepay"
	_ "github.com/lib/pq"
)

type mockPaymentProcessor struct {
	processFunc func(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error)
}

func (m *mockPaymentProcessor) Process(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, req)
	}
	return payments.PaymentResult{
		OrderID:       req.OrderID,
		OrderStatus:   "paid",
		PaymentStatus: "success",
	}, nil
}

func TestSepayWebhookHandler_UnauthorizedSecret_Returns401(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)
	handler := NewSepayWebhookHandler(adapter, &mockPaymentProcessor{}, nil)

	body := []byte(`{"id":123,"content":"DH 11111111-1111-1111-1111-111111111111","transferType":"in","transferAmount":100000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey wrong-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestSepayWebhookHandler_LatePayment_RecordedAsRefund(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)

	orderID := "11111111-1111-1111-1111-111111111111"
	processor := &mockPaymentProcessor{
		processFunc: func(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
			if req.OrderID != orderID {
				t.Fatalf("unexpected orderID: %s", req.OrderID)
			}
			// Simulate D0.7 late payment error returned from payment state machine
			return payments.PaymentResult{
				OrderID:          orderID,
				OrderStatus:      "expired",
				PaymentStatus:    "refund",
				IdempotentReplay: false,
			}, payments.ErrOrderExpiredRefund
		},
	}

	handler := NewSepayWebhookHandler(adapter, processor, nil)

	payload := sepay.WebhookPayload{
		ID:             9991,
		Gateway:        "MBBank",
		Content:        "DH " + orderID,
		TransferType:   "in",
		TransferAmount: 100000,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey correct-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	// Acknowledge receipt to SePay with status=refund_logged (D0.7)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for late refund log, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["status"] != "refund_logged" {
		t.Fatalf("expected status 'refund_logged', got %v", resp["status"])
	}
	if resp["order_id"] != orderID {
		t.Fatalf("expected order_id %q, got %v", orderID, resp["order_id"])
	}
}

func TestSepayWebhookHandler_DuplicateWebhook_IdempotentReplay(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)

	orderID := "22222222-2222-2222-2222-222222222222"
	processor := &mockPaymentProcessor{
		processFunc: func(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
			return payments.PaymentResult{
				OrderID:          orderID,
				OrderStatus:      "paid",
				PaymentStatus:    "success",
				IdempotentReplay: true,
			}, nil
		},
	}

	handler := NewSepayWebhookHandler(adapter, processor, nil)

	payload := sepay.WebhookPayload{
		ID:             9992,
		Gateway:        "Vietcombank",
		Content:        "DH " + orderID,
		TransferType:   "in",
		TransferAmount: 200000,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey correct-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["idempotent_replay"] != true {
		t.Fatalf("expected idempotent_replay = true, got %v", resp["idempotent_replay"])
	}
}

func TestSepayWebhookHandler_MalformedOrOutTransfer_Ignored(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)
	handler := NewSepayWebhookHandler(adapter, &mockPaymentProcessor{}, nil)

	// Outgoing transfer (not a customer payment)
	payload := sepay.WebhookPayload{
		ID:             9993,
		TransferType:   "out",
		TransferAmount: 50000,
		Content:        "DH 33333333-3333-3333-3333-333333333333",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey correct-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ignored" {
		t.Fatalf("expected status 'ignored' for outgoing transfer, got %v", resp["status"])
	}
}
