package payments

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func getBaselineTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping baseline test: cannot open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping baseline test: DB not reachable: %v", err)
	}
	return db
}

func setupTestBaselineOrder(t *testing.T, db *sql.DB, status string) (orderID, itemID string, totalAmount int64) {
	ctx := context.Background()
	// Fetch an active flash sale item
	err := db.QueryRowContext(ctx, `SELECT id, sale_price FROM flash_sale_items LIMIT 1`).Scan(&itemID, &totalAmount)
	if err != nil {
		t.Fatalf("failed to query flash_sale_item: %v", err)
	}

	var userID string
	err = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to query user: %v", err)
	}

	idempotencyKey := "test-idemp-" + time.Now().Format("20060102150405.000000000")
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if status == OrderStatusExpired {
		expiresAt = time.Now().UTC().Add(-5 * time.Minute)
	}

	err = db.QueryRowContext(ctx, `
		INSERT INTO orders (user_id, idempotency_key, status, total_amount, expires_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, idempotencyKey, status, totalAmount, expiresAt).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to insert test order: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO order_items (order_id, flash_sale_item_id, product_id, quantity, unit_price)
		SELECT $1, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $2`, orderID, itemID)
	if err != nil {
		t.Fatalf("failed to insert order_item: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO payments (order_id, method, amount, status)
		VALUES ($1, 'simulated', $2, 'pending')`, orderID, totalAmount)
	if err != nil {
		t.Fatalf("failed to insert payment: %v", err)
	}

	// In baseline, creating an order increments sold_quantity
	_, err = db.ExecContext(ctx, `UPDATE flash_sale_items SET sold_quantity = sold_quantity + 1 WHERE id = $1`, itemID)
	if err != nil {
		t.Fatalf("failed to increment sold_quantity: %v", err)
	}

	return orderID, itemID, totalAmount
}

func TestBaseline_ConfirmPayment_MarksPaidAndKeepsStock(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, itemID, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	var initialSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialSold)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-success-1",
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process confirm failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaid || res.PaymentStatus != PaymentStatusSuccess {
		t.Fatalf("unexpected result: %+v", res)
	}

	var afterSold int32
	var orderStatus string
	var paymentStatus string
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&afterSold)
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
	_ = db.QueryRow(`SELECT status FROM payments WHERE order_id = $1`, orderID).Scan(&paymentStatus)

	if orderStatus != OrderStatusPaid {
		t.Errorf("order status in DB = %q, want paid", orderStatus)
	}
	if paymentStatus != PaymentStatusSuccess {
		t.Errorf("payment status in DB = %q, want success", paymentStatus)
	}
	if afterSold != initialSold {
		t.Errorf("baseline sold_quantity changed on paid: got %d, want %d", afterSold, initialSold)
	}
}

func TestBaseline_FailPayment_RestocksSoldQuantity(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, itemID, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	var initialSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialSold)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeFailure,
		ProviderTxID: "tx-fail-1",
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process fail failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaymentFailed || !res.Restocked {
		t.Fatalf("unexpected result: %+v", res)
	}

	var afterSold int32
	var orderStatus string
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&afterSold)
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)

	if orderStatus != OrderStatusPaymentFailed {
		t.Errorf("order status = %q, want payment_failed", orderStatus)
	}
	if afterSold != initialSold-1 {
		t.Errorf("baseline stock not decremented: got %d, want %d", afterSold, initialSold-1)
	}
}

func TestBaseline_LatePayment_RecordsWebhookRefundLog(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusExpired)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-late-1",
		Provider:     "simulator",
	})
	if !errors.Is(err, ErrOrderExpiredRefund) {
		t.Fatalf("expected ErrOrderExpiredRefund, got %v", err)
	}
	if res.OrderStatus != OrderStatusExpired || res.PaymentStatus != "refund" {
		t.Fatalf("expected status expired / refund, got %+v", res)
	}

	var webhookLogCount int
	var loggedStatus string
	err = db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(status), '') FROM webhook_logs WHERE order_id = $1`, orderID).Scan(&webhookLogCount, &loggedStatus)
	if err != nil {
		t.Fatalf("query webhook_logs failed: %v", err)
	}
	if webhookLogCount != 1 || loggedStatus != "refund" {
		t.Fatalf("webhook_logs expected 1 row with status 'refund', got %d rows with status %q", webhookLogCount, loggedStatus)
	}
}

func TestBaseline_PaymentStatusEnumCompliance(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	// Verify that failing payment sets payments.status to 'failed' (valid PostgreSQL ENUM value)
	_, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:  orderID,
		Outcome:  OutcomeFailure,
		Provider: "simulator",
	})
	if err != nil {
		t.Fatalf("failed to process failure outcome: %v", err)
	}

	var paymentStatus string
	err = db.QueryRow(`SELECT status::text FROM payments WHERE order_id = $1`, orderID).Scan(&paymentStatus)
	if err != nil {
		t.Fatalf("query payments failed: %v", err)
	}
	if paymentStatus != "failed" {
		t.Errorf("payment_status in DB = %q, want failed", paymentStatus)
	}
}
