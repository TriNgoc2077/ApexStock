package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type BaselineService struct {
	db *sql.DB
}

func NewBaselineService(db *sql.DB) *BaselineService {
	return &BaselineService{db: db}
}

func (s *BaselineService) Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error) {
	if req.Provider == "" {
		req.Provider = "simulator"
	}
	if req.ProviderTxID == "" {
		prefix := req.OrderID
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		req.ProviderTxID = fmt.Sprintf("sim-%s-%d", prefix, time.Now().UnixNano())
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PaymentResult{}, err
	}
	defer tx.Rollback()

	var orderStatus string
	var itemID string
	var quantity int32
	var currentPaymentStatus string

	err = tx.QueryRowContext(ctx, `
		SELECT o.status, oi.flash_sale_item_id, oi.quantity, p.status
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN payments p ON p.order_id = o.id
		WHERE o.id = $1
		FOR UPDATE OF o, p`, req.OrderID).Scan(&orderStatus, &itemID, &quantity, &currentPaymentStatus)

	if errors.Is(err, sql.ErrNoRows) {
		return PaymentResult{}, ErrOrderNotFound
	}
	if err != nil {
		return PaymentResult{}, err
	}

	decision, evalErr := EvaluateTransition(orderStatus, req.Outcome)

	// Decision Gate D0.7: late payment on expired order
	if errors.Is(evalErr, ErrOrderExpiredRefund) {
		payload, _ := json.Marshal(map[string]any{
			"order_id":       req.OrderID,
			"outcome":        req.Outcome,
			"provider_tx_id": req.ProviderTxID,
			"reason":         "payment arrived after order expired",
			"received_at":    time.Now().UTC(),
		})
		_, logErr := tx.ExecContext(ctx, `
			INSERT INTO webhook_logs (order_id, provider, transaction_ref, status, payload)
			VALUES ($1, $2, $3, 'refund', $4)`,
			req.OrderID, req.Provider, req.ProviderTxID, payload)
		if logErr != nil {
			return PaymentResult{}, logErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return PaymentResult{}, commitErr
		}
		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      OrderStatusExpired,
			PaymentStatus:    "refund",
			IdempotentReplay: false,
			Restocked:        false,
		}, ErrOrderExpiredRefund
	}

	if evalErr != nil {
		return PaymentResult{}, evalErr
	}

	// Idempotent replay: return current status without mutation
	if decision.IsIdempotentReplay {
		_ = tx.Commit()
		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      orderStatus,
			PaymentStatus:    currentPaymentStatus,
			IdempotentReplay: true,
			Restocked:        false,
		}, nil
	}

	// Update order
	_, err = tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`,
		decision.TargetOrderStatus, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	// Update payment record
	_, err = tx.ExecContext(ctx, `
		UPDATE payments SET status = $1, transaction_ref = $2, updated_at = now() WHERE order_id = $3`,
		decision.TargetPaymentStatus, req.ProviderTxID, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	// Restock if transitioning to payment_failed, expired, or cancelled
	if decision.ShouldRestock {
		_, err = tx.ExecContext(ctx, `
			UPDATE flash_sale_items SET sold_quantity = sold_quantity - $1 WHERE id = $2`,
			quantity, itemID)
		if err != nil {
			return PaymentResult{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return PaymentResult{}, err
	}

	return PaymentResult{
		OrderID:          req.OrderID,
		OrderStatus:      decision.TargetOrderStatus,
		PaymentStatus:    decision.TargetPaymentStatus,
		IdempotentReplay: false,
		Restocked:        decision.ShouldRestock,
	}, nil
}
