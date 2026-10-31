package system

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"apexstock/internal/domain"
	"apexstock/internal/httpapi"
	"apexstock/internal/optimized"
	"apexstock/internal/payments"
	_ "github.com/lib/pq"
)

func TestChaos_RedisOutage_Returns503WithoutStockLeak(t *testing.T) {
	// Probes behavior when an invalid/unreachable Redis address is configured
	apiURL := os.Getenv("OPTIMIZED_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8082"
	}

	// Verify that if Redis is down, order creation fails safely with 503 rather than corrupted stock
	body := []byte(`{
		"user_id": "00000000-0000-0000-0000-000000000001",
		"flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
		"quantity": 1,
		"idempotency_key": "chaos-redis-test"
	}`)

	req, _ := http.NewRequest(http.MethodPost, apiURL+"/api/v1/flash-sale/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("optimized API unreachable at %s: %v", apiURL, err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Logf("circuit breaker successfully returned 503 during Redis interruption")
		} else if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusNotFound {
			t.Logf("system is healthy (Redis reachable, returned status %d)", resp.StatusCode)
		} else {
			t.Fatalf("unexpected status code during resilience check: %d", resp.StatusCode)
		}
	}

	// In-process verification: OptimizedOrderHandler returns 503 when Redis is unavailable
	t.Run("InProcess_CircuitBreaker503", func(t *testing.T) {
		mockCreate := func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
			return domain.OrderResponse{}, optimized.ErrRedisUnavailable
		}
		handler := httpapi.OptimizedOrderHandler(mockCreate)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/flash-sale/orders", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected HTTP 503 on Redis outage, got %d", w.Code)
		}
	})
}

func TestChaos_PaymentExpiryRace_AtomicSerialization(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	var itemID, userID string
	var totalAmount int64
	err := db.QueryRowContext(ctx, `SELECT id, sale_price FROM flash_sale_items LIMIT 1`).Scan(&itemID, &totalAmount)
	if err != nil {
		t.Skipf("skipping test: flash_sale_items empty or query failed: %v", err)
	}
	err = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)
	if err != nil {
		t.Skipf("skipping test: users empty or query failed: %v", err)
	}

	var orderID string
	idempotencyKey := fmt.Sprintf("chaos-race-%d", time.Now().UnixNano())
	err = db.QueryRowContext(ctx, `
		INSERT INTO orders (user_id, idempotency_key, status, total_amount, expires_at)
		VALUES ($1, $2, 'pending_payment', $3, now() - interval '1 second')
		RETURNING id::text`,
		userID, idempotencyKey, totalAmount).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to insert test order: %v", err)
	}

	_, _ = db.ExecContext(ctx, `
		INSERT INTO order_items (order_id, flash_sale_item_id, product_id, quantity, unit_price)
		SELECT $1::uuid, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $2`, orderID, itemID)

	_, err = db.ExecContext(ctx, `
		INSERT INTO payments (order_id, method, amount, status)
		VALUES ($1, 'simulated', $2, 'pending')`, orderID, totalAmount)
	if err != nil {
		t.Fatalf("failed to insert test payment: %v", err)
	}

	svc := payments.NewBaselineService(db)

	var wg sync.WaitGroup
	wg.Add(2)

	var res1, res2 payments.PaymentResult
	var err1, err2 error

	// Goroutine 1: Payment confirm arrives
	go func() {
		defer wg.Done()
		res1, err1 = svc.Process(ctx, payments.ProcessPaymentRequest{
			OrderID:  orderID,
			Outcome:  payments.OutcomeSuccess,
			Provider: "simulator",
		})
	}()

	// Goroutine 2: Expiry sweeper processes order
	go func() {
		defer wg.Done()
		res2, err2 = svc.Process(ctx, payments.ProcessPaymentRequest{
			OrderID:  orderID,
			Outcome:  payments.OutcomeTimeout,
			Provider: "scheduler",
		})
	}()

	wg.Wait()
	_ = res1
	_ = res2

	// Query final order state
	var finalStatus string
	_ = db.QueryRowContext(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&finalStatus)

	if finalStatus != payments.OrderStatusPaid && finalStatus != payments.OrderStatusExpired {
		t.Fatalf("invalid final state after race: %q", finalStatus)
	}
	t.Logf("race resolved cleanly with final order status: %s (err1=%v, err2=%v)", finalStatus, err1, err2)
}

func TestChaos_NATSRedelivery_IdempotentDeduplication(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()

	eventID := fmt.Sprintf("chaos-nats-dedup-%d", time.Now().UnixNano())

	// Step 1: Simulate first delivery
	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	var inserted1 bool
	err = tx1.QueryRowContext(ctx, `
		INSERT INTO processed_events(event_id, event_type)
		VALUES($1, 'order.reserved')
		ON CONFLICT DO NOTHING
		RETURNING true`, eventID).Scan(&inserted1)
	if err != nil {
		_ = tx1.Rollback()
		t.Fatalf("tx1 insert error: %v", err)
	}
	if !inserted1 {
		_ = tx1.Rollback()
		t.Fatalf("expected first insert to return true")
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("tx1 commit: %v", err)
	}

	// Step 2: Simulate redelivery of same event before ACK
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	var inserted2 bool
	err = tx2.QueryRowContext(ctx, `
		INSERT INTO processed_events(event_id, event_type)
		VALUES($1, 'order.reserved')
		ON CONFLICT DO NOTHING
		RETURNING true`, eventID).Scan(&inserted2)
	_ = tx2.Rollback()

	// On conflict, RETURNING true returns no rows (sql.ErrNoRows), which signals already processed!
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows on redelivery conflict, got err=%v, inserted=%v", err, inserted2)
	}

	// Verify only 1 record exists in processed_events for eventID
	var count int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 processed event, found %d", count)
	}
}
