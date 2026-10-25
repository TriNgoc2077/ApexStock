package payments

import (
	"context"
	"math"
	"testing"

	"apexstock/internal/domain"
)

type mockProcessor struct {
	lastReq ProcessPaymentRequest
}

func (m *mockProcessor) Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error) {
	m.lastReq = req
	return PaymentResult{
		OrderID:       req.OrderID,
		OrderStatus:   req.Outcome,
		PaymentStatus: req.Outcome,
	}, nil
}

func TestSimulator_DeterministicOverride(t *testing.T) {
	mock := &mockProcessor{}
	sim := NewSimulator(mock, DefaultSimulatorConfig())

	req := domain.SimulatePaymentRequest{
		OrderID:      "test-order-1",
		Outcome:      OutcomeCancel,
		ProviderTxID: "manual-tx-99",
	}

	res, err := sim.Simulate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastReq.Outcome != OutcomeCancel {
		t.Errorf("expected outcome %q, got %q", OutcomeCancel, mock.lastReq.Outcome)
	}
	if mock.lastReq.ProviderTxID != "manual-tx-99" {
		t.Errorf("expected tx id 'manual-tx-99', got %q", mock.lastReq.ProviderTxID)
	}
	if res.OrderID != "test-order-1" {
		t.Errorf("expected orderID test-order-1, got %q", res.OrderID)
	}
}

func TestSimulator_RatioDistribution(t *testing.T) {
	mock := &mockProcessor{}
	cfg := SimulatorConfig{
		SuccessRatio: 0.70,
		FailureRatio: 0.20,
		TimeoutRatio: 0.10,
		CancelRatio:  0.00,
	}
	sim := NewSimulator(mock, cfg)

	counts := make(map[string]int)
	const total = 5000

	for i := 0; i < total; i++ {
		_, _ = sim.Simulate(context.Background(), domain.SimulatePaymentRequest{
			OrderID: "order-multi",
		})
		counts[mock.lastReq.Outcome]++
	}

	successRate := float64(counts[OutcomeSuccess]) / float64(total)
	failureRate := float64(counts[OutcomeFailure]) / float64(total)
	timeoutRate := float64(counts[OutcomeTimeout]) / float64(total)

	if math.Abs(successRate-0.70) > 0.04 {
		t.Errorf("success rate deviated too much: got %.4f, want ~0.70", successRate)
	}
	if math.Abs(failureRate-0.20) > 0.04 {
		t.Errorf("failure rate deviated too much: got %.4f, want ~0.20", failureRate)
	}
	if math.Abs(timeoutRate-0.10) > 0.04 {
		t.Errorf("timeout rate deviated too much: got %.4f, want ~0.10", timeoutRate)
	}
}
