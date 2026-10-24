package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"time"
)

const SubjectOrderReserved = "flashsale.order.reserved"
const StreamName = "FLASHSALE_ORDERS"

type OrderReserved struct {
	EventID        string    `json:"event_id"`
	OrderID        string    `json:"order_id"`
	ReservationID  string    `json:"reservation_id"`
	UserID         string    `json:"user_id"`
	ItemID         string    `json:"flash_sale_item_id"`
	ProductID      string    `json:"product_id"`
	Quantity       int32     `json:"quantity"`
	TotalAmount    int64     `json:"total_amount"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	IdempotencyKey string    `json:"idempotency_key"`
}
type Client struct {
	NC *nats.Conn
	JS nats.JetStreamContext
}

func Connect(url string) (*Client, error) {
	nc, err := nats.Connect(url, nats.Name("apexstock"), nats.Timeout(5*time.Second))
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	if _, err = js.StreamInfo(StreamName); err != nil {
		_, err = js.AddStream(&nats.StreamConfig{Name: StreamName, Subjects: []string{SubjectOrderReserved}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, MaxAge: 7 * 24 * time.Hour})
		if err != nil {
			nc.Close()
			return nil, err
		}
	}
	return &Client{NC: nc, JS: js}, nil
}
func (c *Client) Publish(ctx context.Context, event OrderReserved) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = c.JS.PublishMsg(&nats.Msg{Subject: SubjectOrderReserved, Data: body, Header: nats.Header{"Nats-Msg-Id": []string{event.EventID}}}, nats.Context(ctx))
	return err
}
func (c *Client) Close() {
	if c != nil && c.NC != nil {
		c.NC.Drain()
	}
}
func (e OrderReserved) Validate() error {
	if e.OrderID == "" || e.ReservationID == "" || e.UserID == "" || e.ItemID == "" || e.Quantity != 1 {
		return fmt.Errorf("invalid order event")
	}
	return nil
}
