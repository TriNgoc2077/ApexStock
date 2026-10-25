package payments

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"apexstock/internal/domain"
)

type SimulatorConfig struct {
	SuccessRatio float64
	FailureRatio float64
	TimeoutRatio float64
	CancelRatio  float64
}

func DefaultSimulatorConfig() SimulatorConfig {
	return SimulatorConfig{
		SuccessRatio: 0.85,
		FailureRatio: 0.10,
		TimeoutRatio: 0.05,
		CancelRatio:  0.00,
	}
}

type Simulator struct {
	processor PaymentProcessor
	cfg       SimulatorConfig
	rng       *rand.Rand
	mu        sync.Mutex
}

func NewSimulator(processor PaymentProcessor, cfg SimulatorConfig) *Simulator {
	// Normalize ratios if sum != 1.0
	sum := cfg.SuccessRatio + cfg.FailureRatio + cfg.TimeoutRatio + cfg.CancelRatio
	if sum > 0 {
		cfg.SuccessRatio /= sum
		cfg.FailureRatio /= sum
		cfg.TimeoutRatio /= sum
		cfg.CancelRatio /= sum
	} else {
		cfg = DefaultSimulatorConfig()
	}

	return &Simulator{
		processor: processor,
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Simulator) Simulate(ctx context.Context, req domain.SimulatePaymentRequest) (PaymentResult, error) {
	outcome := req.Outcome
	if outcome == "" {
		s.mu.Lock()
		r := s.rng.Float64()
		s.mu.Unlock()

		if r < s.cfg.SuccessRatio {
			outcome = OutcomeSuccess
		} else if r < s.cfg.SuccessRatio+s.cfg.FailureRatio {
			outcome = OutcomeFailure
		} else if r < s.cfg.SuccessRatio+s.cfg.FailureRatio+s.cfg.TimeoutRatio {
			outcome = OutcomeTimeout
		} else {
			outcome = OutcomeCancel
		}
	} else {
		// Validate deterministic outcome string
		switch outcome {
		case OutcomeSuccess, OutcomeFailure, OutcomeTimeout, OutcomeCancel:
		default:
			return PaymentResult{}, fmt.Errorf("%w: invalid outcome %q", ErrInvalidStateTransition, outcome)
		}
	}

	txID := req.ProviderTxID
	if txID == "" {
		txID = fmt.Sprintf("sim-%s-%d", req.OrderID, time.Now().UnixNano())
	}

	return s.processor.Process(ctx, ProcessPaymentRequest{
		OrderID:      req.OrderID,
		Outcome:      outcome,
		ProviderTxID: txID,
		Provider:     "simulator",
	})
}
