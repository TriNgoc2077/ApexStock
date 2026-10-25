package messaging

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPaymentProcessedEvent_Validation(t *testing.T) {
	evt := PaymentProcessedEvent{
		EventID:       "evt-123",
		OrderID:       "11111111-1111-1111-1111-111111111111",
		OrderStatus:   "paid",
		PaymentStatus: "success",
		ProcessedAt:   time.Now().UTC(),
	}

	if err := evt.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed PaymentProcessedEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.OrderID != evt.OrderID || parsed.OrderStatus != evt.OrderStatus {
		t.Fatalf("mismatch: got %+v, want %+v", parsed, evt)
	}
}
