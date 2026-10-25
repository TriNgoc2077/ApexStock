package domain

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeTimeout = "timeout"
	OutcomeCancel  = "cancel"
)

// SimulatePaymentRequest represents the body of POST /api/v1/payments/simulate.
type SimulatePaymentRequest struct {
	OrderID      string `json:"order_id"`
	Outcome      string `json:"outcome,omitempty"`
	ProviderTxID string `json:"provider_tx_id,omitempty"`
}

// PaymentResultResponse represents the contract defined in openapi.yaml.
type PaymentResultResponse struct {
	OrderID          string `json:"order_id"`
	OrderStatus      string `json:"order_status"`
	PaymentStatus    string `json:"payment_status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}
