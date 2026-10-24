package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"apexstock/internal/domain"
	"github.com/lib/pq"
)

var ErrNotFound = errors.New("not found")
var ErrUserNotFound = errors.New("user not found")
var ErrIdempotencyConflict = errors.New("idempotency conflict")
var ErrOutOfStock = errors.New("out of stock")
var ErrMaxPerUser = errors.New("max per user")

// OrderRepository persists the baseline order transactionally.
type OrderRepository struct{ db *sql.DB }

func NewOrderRepository(db *sql.DB) *OrderRepository { return &OrderRepository{db: db} }

func (r *OrderRepository) Create(ctx context.Context, req domain.CreateOrderRequest, ttl time.Duration) (domain.OrderResponse, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.OrderResponse{}, err
	}
	defer tx.Rollback()

	// Serialize claims for the same user/key even before the idempotency row exists.
	// This closes the check-then-insert race while preserving replay semantics.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))`, req.UserID, req.IdempotencyKey); err != nil {
		return domain.OrderResponse{}, err
	}

	var existingBody []byte
	var existingStatus int
	err = tx.QueryRowContext(ctx, `SELECT response_status, response_body FROM idempotency_keys WHERE key=$1 AND user_id=$2 FOR UPDATE`, req.IdempotencyKey, req.UserID).Scan(&existingStatus, &existingBody)
	if err == nil {
		var got domain.OrderResponse
		if json.Unmarshal(existingBody, &got) != nil {
			return domain.OrderResponse{}, fmt.Errorf("decode idempotency response: %w", err)
		}
		if got.FlashSaleItemID != req.FlashSaleItemID || got.Quantity != req.Quantity {
			return domain.OrderResponse{}, ErrIdempotencyConflict
		}
		got.Idempotent = true
		return got, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.OrderResponse{}, err
	}

	var userExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, req.UserID).Scan(&userExists); err != nil {
		return domain.OrderResponse{}, err
	}
	if !userExists {
		return domain.OrderResponse{}, ErrUserNotFound
	}

	var item FlashSaleItem
	err = tx.QueryRowContext(ctx, `
		SELECT fsi.id, fsi.event_id, fsi.product_id, fsi.sale_price, fsi.total_quantity,
		       fsi.sold_quantity, fsi.max_per_user, fse.name, fse.start_time, fse.end_time, fse.status::text
		FROM flash_sale_items fsi JOIN flash_sale_events fse ON fse.id=fsi.event_id
		WHERE fsi.id=$1 FOR UPDATE`, req.FlashSaleItemID).Scan(
		&item.ID, &item.EventID, &item.ProductID, &item.SalePrice, &item.TotalQuantity, &item.SoldQuantity,
		&item.MaxPerUser, &item.EventName, &item.StartTime, &item.EndTime, &item.EventStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OrderResponse{}, ErrNotFound
	}
	if err != nil {
		return domain.OrderResponse{}, err
	}
	now := time.Now().UTC()
	if item.EventStatus != "active" || now.Before(item.StartTime) || !now.Before(item.EndTime) {
		return domain.OrderResponse{}, ErrNotFound
	}
	if item.TotalQuantity-item.SoldQuantity < req.Quantity {
		return domain.OrderResponse{}, ErrOutOfStock
	}

	var purchased int32
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(oi.quantity),0) FROM orders o JOIN order_items oi ON oi.order_id=o.id WHERE o.user_id=$1 AND oi.flash_sale_item_id=$2 AND o.status IN ('pending_payment','paid')`, req.UserID, req.FlashSaleItemID).Scan(&purchased); err != nil {
		return domain.OrderResponse{}, err
	}
	if purchased+req.Quantity > item.MaxPerUser {
		return domain.OrderResponse{}, ErrMaxPerUser
	}

	var orderID, paymentID string
	var createdAt, expiresAt time.Time
	expiresAt = now.Add(ttl)
	err = tx.QueryRowContext(ctx, `INSERT INTO orders (user_id,idempotency_key,status,total_amount,expires_at) VALUES ($1,$2,'pending_payment',$3,$4) RETURNING id,created_at,expires_at`, req.UserID, req.IdempotencyKey, item.SalePrice*int64(req.Quantity), expiresAt).Scan(&orderID, &createdAt, &expiresAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return domain.OrderResponse{}, ErrIdempotencyConflict
		}
		return domain.OrderResponse{}, err
	}
	var reservationID string
	if err = tx.QueryRowContext(ctx, `INSERT INTO order_items (order_id,flash_sale_item_id,product_id,quantity,unit_price) VALUES ($1,$2,$3,$4,$5) RETURNING id`, orderID, req.FlashSaleItemID, item.ProductID, req.Quantity, item.SalePrice).Scan(&reservationID); err != nil {
		return domain.OrderResponse{}, err
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO payments (order_id,method,amount,status) VALUES ($1,'simulated',$2,'pending') RETURNING id`, orderID, item.SalePrice*int64(req.Quantity)).Scan(&paymentID); err != nil {
		return domain.OrderResponse{}, err
	}
	_ = paymentID
	if _, err = tx.ExecContext(ctx, `UPDATE flash_sale_items SET sold_quantity=sold_quantity+$1 WHERE id=$2`, req.Quantity, req.FlashSaleItemID); err != nil {
		return domain.OrderResponse{}, err
	}
	response := domain.OrderResponse{OrderID: orderID, ReservationID: reservationID, UserID: req.UserID, FlashSaleItemID: req.FlashSaleItemID, Quantity: req.Quantity, TotalAmount: item.SalePrice * int64(req.Quantity), Status: "pending_payment", PaymentStatus: "pending", ExpiresAt: expiresAt, CreatedAt: createdAt}
	body, err := json.Marshal(response)
	if err != nil {
		return domain.OrderResponse{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency_keys (key,user_id,response_status,response_body,expires_at) VALUES ($1,$2,$3,$4,$5)`, req.IdempotencyKey, req.UserID, 201, body, expiresAt); err != nil {
		return domain.OrderResponse{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.OrderResponse{}, err
	}
	return response, nil
}
