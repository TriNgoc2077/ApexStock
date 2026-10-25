package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

const confirmLua = `local s=redis.call('HGET',KEYS[2],'state'); if s=='confirmed' then return {1,'confirmed'} end; if s~='reserved' then return {1,s or 'missing'} end; redis.call('ZREM',KEYS[1],ARGV[1]); redis.call('HSET',KEYS[2],'state','confirmed'); return {0,'confirmed'}`
const releaseLua = `local s=redis.call('HGET',KEYS[4],'state'); if s==false or s=='released' or s=='confirmed' then return {1,s or 'missing'} end; local q=tonumber(redis.call('HGET',KEYS[4],'qty') or '0'); redis.call('INCRBY',KEYS[1],q); redis.call('DECRBY',KEYS[3],q); redis.call('ZREM',KEYS[2],ARGV[1]); redis.call('HSET',KEYS[4],'state','released'); return {0,'released',q}`

type OptimizedService struct {
	db  *sql.DB
	rdb *redis.Client
}

func NewOptimizedService(db *sql.DB, rdb *redis.Client) *OptimizedService {
	return &OptimizedService{db: db, rdb: rdb}
}

func (s *OptimizedService) Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error) {
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

	var orderStatus string
	var itemID string
	var reservationID string
	var userID string
	var quantity int32
	var currentPaymentStatus string

	// Bounded retry to allow JetStream worker to persist order if payment arrives immediately
	var queryErr error
	for attempt := 0; attempt < 5; attempt++ {
		queryErr = s.db.QueryRowContext(ctx, `
			SELECT o.status, o.user_id, oi.id, oi.flash_sale_item_id, oi.quantity, p.status
			FROM orders o
			JOIN order_items oi ON oi.order_id = o.id
			JOIN payments p ON p.order_id = o.id
			WHERE o.id = $1`, req.OrderID).Scan(&orderStatus, &userID, &reservationID, &itemID, &quantity, &currentPaymentStatus)
		if queryErr == nil {
			break
		}
		if !errors.Is(queryErr, sql.ErrNoRows) {
			return PaymentResult{}, queryErr
		}
		select {
		case <-ctx.Done():
			return PaymentResult{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if errors.Is(queryErr, sql.ErrNoRows) {
		return PaymentResult{}, ErrOrderNotFound
	}

	decision, evalErr := EvaluateTransition(orderStatus, req.Outcome)

	// Decision Gate D0.7: late payment on expired order
	if errors.Is(evalErr, ErrOrderExpiredRefund) {
		payload, _ := json.Marshal(map[string]any{
			"order_id":       req.OrderID,
			"outcome":        req.Outcome,
			"provider_tx_id": req.ProviderTxID,
			"reason":         "late payment on expired order",
			"received_at":    time.Now().UTC(),
		})
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO webhook_logs (order_id, provider, transaction_ref, status, payload)
			VALUES ($1, $2, $3, 'refund', $4)`,
			req.OrderID, req.Provider, req.ProviderTxID, payload)

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

	if decision.IsIdempotentReplay {
		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      orderStatus,
			PaymentStatus:    currentPaymentStatus,
			IdempotentReplay: true,
			Restocked:        false,
		}, nil
	}

	// Apply Redis state transition
	if decision.TargetOrderStatus == OrderStatusPaid {
		_, err := s.rdb.Eval(ctx, confirmLua, []string{
			rediskeys.ReservedKey(itemID),
			rediskeys.ReservationKey(itemID, reservationID),
		}, reservationID).Result()
		if err != nil {
			return PaymentResult{}, fmt.Errorf("redis confirm lua: %w", err)
		}
	} else if decision.ShouldRestock {
		_, err := s.rdb.Eval(ctx, releaseLua, []string{
			rediskeys.StockKey(itemID),
			rediskeys.ReservedKey(itemID),
			rediskeys.UserKey(itemID, userID),
			rediskeys.ReservationKey(itemID, reservationID),
		}, reservationID).Result()
		if err != nil {
			return PaymentResult{}, fmt.Errorf("redis release lua: %w", err)
		}
	}

	// Update DB atomically
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PaymentResult{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`,
		decision.TargetOrderStatus, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE payments SET status = $1, transaction_ref = $2, updated_at = now() WHERE order_id = $3`,
		decision.TargetPaymentStatus, req.ProviderTxID, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	// Decision Gate D0.3: in Optimized, sold_quantity only increments in DB when paid
	if decision.TargetOrderStatus == OrderStatusPaid {
		_, err = tx.ExecContext(ctx, `
			UPDATE flash_sale_items SET sold_quantity = sold_quantity + $1 WHERE id = $2`,
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
