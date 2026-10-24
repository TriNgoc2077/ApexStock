package optimized

import (
	"apexstock/internal/messaging"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"time"
)

type Worker struct {
	db     *sql.DB
	client *messaging.Client
}

func NewWorker(db *sql.DB, client *messaging.Client) *Worker { return &Worker{db: db, client: client} }
func (w *Worker) Run(ctx context.Context) error {
	sub, err := w.client.JS.PullSubscribe(messaging.SubjectOrderReserved, "optimized-worker", nats.BindStream(messaging.StreamName), nats.ManualAck())
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		msgs, err := sub.Fetch(10, nats.MaxWait(time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			return err
		}
		for _, msg := range msgs {
			if err = w.process(ctx, msg); err != nil {
				_ = msg.Nak()
				continue
			}
			_ = msg.Ack()
		}
	}
}
func (w *Worker) process(ctx context.Context, msg *nats.Msg) error {
	var e messaging.OrderReserved
	if err := json.Unmarshal(msg.Data, &e); err != nil {
		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var inserted bool
	err = tx.QueryRowContext(ctx, `INSERT INTO processed_events(event_id,event_type) VALUES($1,$2) ON CONFLICT DO NOTHING RETURNING true`, e.EventID, "order.reserved").Scan(&inserted)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO orders(id,user_id,idempotency_key,status,total_amount,expires_at,created_at) VALUES($1,$2,$3,'pending_payment',$4,$5,$6) ON CONFLICT (id) DO NOTHING`, e.OrderID, e.UserID, e.IdempotencyKey, e.TotalAmount, e.ExpiresAt, e.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO order_items(id,order_id,flash_sale_item_id,product_id,quantity,unit_price) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, e.ReservationID, e.OrderID, e.ItemID, e.ProductID, e.Quantity, e.TotalAmount/int64(e.Quantity))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payments(order_id,method,amount,status) VALUES($1,'simulated',$2,'pending') ON CONFLICT (order_id) DO NOTHING`, e.OrderID, e.TotalAmount)
	if err != nil {
		return err
	}
	return tx.Commit()
}

var _ = fmt.Sprintf
