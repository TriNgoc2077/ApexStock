package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
)

const SubjectPaymentProcessed = "flashsale.payment.processed"

type PaymentProcessedEvent struct {
	EventID       string    `json:"event_id"`
	OrderID       string    `json:"order_id"`
	OrderStatus   string    `json:"order_status"`
	PaymentStatus string    `json:"payment_status"`
	ProcessedAt   time.Time `json:"processed_at"`
}

func (e PaymentProcessedEvent) Validate() error {
	if e.EventID == "" || e.OrderID == "" || e.OrderStatus == "" || e.PaymentStatus == "" {
		return errors.New("invalid payment processed event: missing required fields")
	}
	return nil
}

func (c *Client) PublishPaymentProcessed(ctx context.Context, event PaymentProcessedEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = c.JS.PublishMsg(&nats.Msg{
		Subject: SubjectPaymentProcessed,
		Data:    body,
		Header:  nats.Header{"Nats-Msg-Id": []string{event.EventID}},
	}, nats.Context(ctx))
	return err
}
