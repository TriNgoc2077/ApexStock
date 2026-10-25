package payments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	rediskeys "apexstock/internal/persistence/redis"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func newTestUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x-%04x-4000-8000-%012x", time.Now().UnixNano()>>32, time.Now().UnixNano()&65535, time.Now().UnixNano()&0xffffffffffff)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func getOptimizedTestInfra(t *testing.T) (*sql.DB, *redis.Client) {
	dsn := os.Getenv("OPTIMIZED_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_optimized?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping optimized test: cannot open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping optimized test: DB unreachable: %v", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("skipping optimized test: Redis unreachable: %v", err)
	}
	return db, rdb
}

func setupTestOptimizedOrder(t *testing.T, db *sql.DB, rdb *redis.Client, status string) (orderID, reservationID, itemID, userID string) {
	ctx := context.Background()
	_ = db.QueryRowContext(ctx, `SELECT id FROM flash_sale_items LIMIT 1`).Scan(&itemID)
	_ = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)

	reservationID = newTestUUID()
	orderID = newTestUUID()
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if status == OrderStatusExpired {
		expiresAt = time.Now().UTC().Add(-5 * time.Minute)
	}

	// Insert in DB
	_, err := db.ExecContext(ctx, `
		INSERT INTO orders (id, user_id, idempotency_key, status, total_amount, expires_at)
		VALUES ($1, $2, $3, $4, 100000, $5)`, orderID, userID, orderID, status, expiresAt)
	if err != nil {
		t.Fatalf("failed to insert order: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO order_items (id, order_id, flash_sale_item_id, product_id, quantity, unit_price)
		SELECT $1, $2, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $3`, reservationID, orderID, itemID)
	if err != nil {
		t.Fatalf("failed to insert order_item: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO payments (order_id, method, amount, status)
		VALUES ($1, 'simulated', 100000, 'pending')`, orderID)
	if err != nil {
		t.Fatalf("failed to insert payment: %v", err)
	}

	// Set up Redis keys for reservation
	rdb.Set(ctx, rediskeys.StockKey(itemID), 100, 0)
	rdb.Set(ctx, rediskeys.UserKey(itemID, userID), 1, 0)
	rdb.ZAdd(ctx, rediskeys.ReservedKey(itemID), redis.Z{Score: float64(expiresAt.Unix()), Member: reservationID})
	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, reservationID),
		"reservation_id", reservationID,
		"order_id", orderID,
		"item_id", itemID,
		"user_id", userID,
		"qty", "1",
		"state", "reserved",
	)

	return orderID, reservationID, itemID, userID
}

func TestOptimized_ConfirmPayment_CallsLuaConfirmAndIncrementsDBSold(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	orderID, reservationID, itemID, _ := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)

	var initialDBSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialDBSold)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: fmt.Sprintf("tx-opt-confirm-%d", time.Now().UnixNano()),
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process confirm failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaid || res.PaymentStatus != PaymentStatusSuccess {
		t.Fatalf("unexpected res: %+v", res)
	}

	// Verify Redis reservation hash state
	state, _ := rdb.HGet(context.Background(), rediskeys.ReservationKey(itemID, reservationID), "state").Result()
	if state != "confirmed" {
		t.Errorf("Redis reservation state = %q, want confirmed", state)
	}

	// Verify DB sold_quantity incremented by 1 (per D0.3)
	var afterDBSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&afterDBSold)
	if afterDBSold != initialDBSold+1 {
		t.Errorf("DB sold_quantity = %d, want %d", afterDBSold, initialDBSold+1)
	}
}

func TestOptimized_FailPayment_ReleasesRedisStockAndUserQuota(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	orderID, reservationID, itemID, userID := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)

	initialStock, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()
	initialUser, _ := rdb.Get(context.Background(), rediskeys.UserKey(itemID, userID)).Int64()

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeFailure,
		ProviderTxID: fmt.Sprintf("tx-opt-fail-%d", time.Now().UnixNano()),
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process fail failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaymentFailed || !res.Restocked {
		t.Fatalf("unexpected res: %+v", res)
	}

	// Verify Redis stock incremented by 1
	afterStock, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()
	if afterStock != initialStock+1 {
		t.Errorf("Redis stock = %d, want %d", afterStock, initialStock+1)
	}

	// Verify user counter decremented by 1
	afterUser, _ := rdb.Get(context.Background(), rediskeys.UserKey(itemID, userID)).Int64()
	if afterUser != initialUser-1 {
		t.Errorf("Redis user counter = %d, want %d", afterUser, initialUser-1)
	}

	// Verify reservation state in Redis
	state, _ := rdb.HGet(context.Background(), rediskeys.ReservationKey(itemID, reservationID), "state").Result()
	if state != "released" {
		t.Errorf("Redis reservation state = %q, want released", state)
	}
}

func TestOptimized_DuplicateRelease_IdempotentNoDoubleRestock(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	orderID, _, itemID, _ := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)

	// First release
	_, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID: orderID,
		Outcome: OutcomeTimeout,
	})
	if err != nil {
		t.Fatalf("first timeout failed: %v", err)
	}

	stockAfterFirst, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()

	// Second release (duplicate)
	res2, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID: orderID,
		Outcome: OutcomeTimeout,
	})
	if err != nil {
		t.Fatalf("second timeout failed: %v", err)
	}
	if !res2.IdempotentReplay {
		t.Errorf("expected IdempotentReplay = true on second release")
	}

	stockAfterSecond, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()
	if stockAfterSecond != stockAfterFirst {
		t.Errorf("stock changed on duplicate release: first=%d, second=%d", stockAfterFirst, stockAfterSecond)
	}
}

func TestOptimized_WorkerPersistenceLag_RetrySucceeds(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	ctx := context.Background()
	var itemID, userID string
	_ = db.QueryRowContext(ctx, `SELECT id FROM flash_sale_items LIMIT 1`).Scan(&itemID)
	_ = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)

	orderID := newTestUUID()
	reservationID := newTestUUID()
	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	// Simulate reservation in Redis immediately
	rdb.Set(ctx, rediskeys.StockKey(itemID), 100, 0)
	rdb.Set(ctx, rediskeys.UserKey(itemID, userID), 1, 0)
	rdb.ZAdd(ctx, rediskeys.ReservedKey(itemID), redis.Z{Score: float64(expiresAt.Unix()), Member: reservationID})
	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, reservationID),
		"reservation_id", reservationID,
		"order_id", orderID,
		"item_id", itemID,
		"user_id", userID,
		"qty", "1",
		"state", "reserved",
	)

	// Delay DB order persistence by 120ms to simulate asynchronous worker lag
	go func() {
		time.Sleep(120 * time.Millisecond)
		_, _ = db.ExecContext(context.Background(), `
			INSERT INTO orders (id, user_id, idempotency_key, status, total_amount, expires_at)
			VALUES ($1, $2, $3, 'pending_payment', 100000, $4)`, orderID, userID, orderID, expiresAt)
		_, _ = db.ExecContext(context.Background(), `
			INSERT INTO order_items (id, order_id, flash_sale_item_id, product_id, quantity, unit_price)
			SELECT $1, $2, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $3`, reservationID, orderID, itemID)
		_, _ = db.ExecContext(context.Background(), `
			INSERT INTO payments (order_id, method, amount, status)
			VALUES ($1, 'simulated', 100000, 'pending')`, orderID)
	}()

	// Payment process should retry and succeed once row is inserted
	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: fmt.Sprintf("tx-lag-success-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("expected bounded retry to succeed despite worker lag, got: %v", err)
	}
	if res.OrderStatus != OrderStatusPaid {
		t.Errorf("order status = %q, want paid", res.OrderStatus)
	}
}
