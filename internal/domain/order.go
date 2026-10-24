package domain

import "time"

type CreateOrderRequest struct {
	UserID          string
	FlashSaleItemID string
	Quantity        int32
	IdempotencyKey  string
}

type OrderResponse struct {
	OrderID         string    `json:"order_id"`
	ReservationID   string    `json:"reservation_id"`
	UserID          string    `json:"user_id"`
	FlashSaleItemID string    `json:"flash_sale_item_id"`
	Quantity        int32     `json:"quantity"`
	TotalAmount     int64     `json:"total_amount"`
	Status          string    `json:"status"`
	PaymentStatus   string    `json:"payment_status"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
	Idempotent      bool      `json:"idempotent"`
}
