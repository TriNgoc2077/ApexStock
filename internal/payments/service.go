package payments

import "context"

type ProcessPaymentRequest struct {
	OrderID      string
	Outcome      string
	ProviderTxID string
	Provider     string
}

type PaymentResult struct {
	OrderID          string
	OrderStatus      string
	PaymentStatus    string
	IdempotentReplay bool
	Restocked        bool
}

type PaymentProcessor interface {
	Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error)
}
