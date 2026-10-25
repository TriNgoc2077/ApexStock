package payments

import (
	"errors"
	"testing"
)

func TestStateMachine_ValidTransitions(t *testing.T) {
	tests := []struct {
		name              string
		currentStatus     string
		outcome           string
		wantOrderStatus   string
		wantPaymentStatus string
		wantRestock       bool
	}{
		{"pending -> success (paid)", OrderStatusPendingPayment, OutcomeSuccess, OrderStatusPaid, PaymentStatusSuccess, false},
		{"pending -> failure", OrderStatusPendingPayment, OutcomeFailure, OrderStatusPaymentFailed, PaymentStatusFailed, true},
		{"pending -> timeout (expired)", OrderStatusPendingPayment, OutcomeTimeout, OrderStatusExpired, PaymentStatusFailed, true},
		{"pending -> cancel", OrderStatusPendingPayment, OutcomeCancel, OrderStatusCancelled, PaymentStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := EvaluateTransition(tt.currentStatus, tt.outcome)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.TargetOrderStatus != tt.wantOrderStatus {
				t.Errorf("TargetOrderStatus = %q, want %q", decision.TargetOrderStatus, tt.wantOrderStatus)
			}
			if decision.TargetPaymentStatus != tt.wantPaymentStatus {
				t.Errorf("TargetPaymentStatus = %q, want %q", decision.TargetPaymentStatus, tt.wantPaymentStatus)
			}
			if decision.ShouldRestock != tt.wantRestock {
				t.Errorf("ShouldRestock = %v, want %v", decision.ShouldRestock, tt.wantRestock)
			}
			if decision.IsIdempotentReplay {
				t.Errorf("IsIdempotentReplay expected false")
			}
		})
	}
}

func TestStateMachine_IdempotentReplay(t *testing.T) {
	tests := []struct {
		currentStatus string
		outcome       string
	}{
		{OrderStatusPaid, OutcomeSuccess},
		{OrderStatusPaymentFailed, OutcomeFailure},
		{OrderStatusExpired, OutcomeTimeout},
		{OrderStatusCancelled, OutcomeCancel},
	}

	for _, tt := range tests {
		t.Run(tt.currentStatus+" with same outcome", func(t *testing.T) {
			decision, err := EvaluateTransition(tt.currentStatus, tt.outcome)
			if err != nil {
				t.Fatalf("unexpected error on replay: %v", err)
			}
			if !decision.IsIdempotentReplay {
				t.Errorf("expected IsIdempotentReplay = true")
			}
			if decision.ShouldRestock {
				t.Errorf("expected ShouldRestock = false on replay")
			}
		})
	}
}

func TestStateMachine_LatePayment_RecordedAsRefund(t *testing.T) {
	// D0.7: Late payment arriving when order is already expired
	decision, err := EvaluateTransition(OrderStatusExpired, OutcomeSuccess)
	if !errors.Is(err, ErrOrderExpiredRefund) {
		t.Fatalf("expected ErrOrderExpiredRefund, got %v", err)
	}
	if decision.TargetOrderStatus != OrderStatusExpired {
		t.Errorf("TargetOrderStatus must remain expired, got %q", decision.TargetOrderStatus)
	}
	if decision.TargetPaymentStatus != "refund" {
		t.Errorf("TargetPaymentStatus must be refund, got %q", decision.TargetPaymentStatus)
	}
	if decision.ShouldRestock {
		t.Errorf("ShouldRestock must be false for late payment")
	}
}

func TestStateMachine_InvalidTransitions_Rejected(t *testing.T) {
	invalidCases := []struct {
		currentStatus string
		outcome       string
	}{
		{OrderStatusPaid, OutcomeFailure},
		{OrderStatusPaid, OutcomeTimeout},
		{OrderStatusPaid, OutcomeCancel},
		{OrderStatusPaymentFailed, OutcomeSuccess},
		{OrderStatusCancelled, OutcomeSuccess},
		{OrderStatusCancelled, OutcomeTimeout},
	}

	for _, tc := range invalidCases {
		t.Run(tc.currentStatus+" -> "+tc.outcome, func(t *testing.T) {
			_, err := EvaluateTransition(tc.currentStatus, tc.outcome)
			if !errors.Is(err, ErrInvalidStateTransition) {
				t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
			}
		})
	}
}
