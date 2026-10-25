package payments

import (
	"apexstock/internal/domain"
	"errors"
	"fmt"
)

const (
	OrderStatusPendingPayment = "pending_payment"
	OrderStatusPaid           = "paid"
	OrderStatusPaymentFailed  = "payment_failed"
	OrderStatusExpired        = "expired"
	OrderStatusCancelled      = "cancelled"
)

const (
	PaymentStatusPending = "pending"
	PaymentStatusSuccess = "success"
	PaymentStatusFailed  = "failed"
)

const (
	OutcomeSuccess = domain.OutcomeSuccess
	OutcomeFailure = domain.OutcomeFailure
	OutcomeTimeout = domain.OutcomeTimeout
	OutcomeCancel  = domain.OutcomeCancel
)

var (
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrOrderExpiredRefund     = errors.New("order expired; payment marked for refund")
	ErrOrderNotFound          = errors.New("order not found")
)

type TransitionDecision struct {
	TargetOrderStatus   string
	TargetPaymentStatus string
	ShouldRestock       bool
	IsIdempotentReplay  bool
}

// EvaluateTransition determines the target state and restock action based on current state and event outcome.
func EvaluateTransition(currentOrderStatus, outcome string) (TransitionDecision, error) {
	switch currentOrderStatus {
	case OrderStatusPendingPayment:
		switch outcome {
		case OutcomeSuccess:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaid,
				TargetPaymentStatus: PaymentStatusSuccess,
				ShouldRestock:       false,
				IsIdempotentReplay:  false,
			}, nil
		case OutcomeFailure:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaymentFailed,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       true,
				IsIdempotentReplay:  false,
			}, nil
		case OutcomeTimeout:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusExpired,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       true,
				IsIdempotentReplay:  false,
			}, nil
		case OutcomeCancel:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusCancelled,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       true,
				IsIdempotentReplay:  false,
			}, nil
		default:
			return TransitionDecision{}, fmt.Errorf("%w: unknown outcome %q", ErrInvalidStateTransition, outcome)
		}

	case OrderStatusPaid:
		if outcome == OutcomeSuccess {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaid,
				TargetPaymentStatus: PaymentStatusSuccess,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		return TransitionDecision{}, fmt.Errorf("%w: cannot transition paid order via %q", ErrInvalidStateTransition, outcome)

	case OrderStatusExpired:
		if outcome == OutcomeTimeout {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusExpired,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		if outcome == OutcomeSuccess {
			// Decision Gate D0.7: late payment on expired order
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusExpired,
				TargetPaymentStatus: "refund",
				ShouldRestock:       false,
				IsIdempotentReplay:  false,
			}, ErrOrderExpiredRefund
		}
		return TransitionDecision{}, fmt.Errorf("%w: expired order cannot transition via %q", ErrInvalidStateTransition, outcome)

	case OrderStatusPaymentFailed:
		if outcome == OutcomeFailure {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaymentFailed,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		return TransitionDecision{}, fmt.Errorf("%w: failed order cannot transition via %q", ErrInvalidStateTransition, outcome)

	case OrderStatusCancelled:
		if outcome == OutcomeCancel {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusCancelled,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		return TransitionDecision{}, fmt.Errorf("%w: cancelled order cannot transition via %q", ErrInvalidStateTransition, outcome)

	default:
		return TransitionDecision{}, fmt.Errorf("%w: unrecognized current order status %q", ErrInvalidStateTransition, currentOrderStatus)
	}
}
