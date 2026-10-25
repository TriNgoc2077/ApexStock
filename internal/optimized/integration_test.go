package optimized

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"apexstock/internal/domain"
	"apexstock/internal/messaging"
	rediskeys "apexstock/internal/persistence/redis"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

func setupIntegration(t *testing.T) (*sql.DB, *redis.Client, *messaging.Client, string, string, string) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test; set INTEGRATION_TEST=1")
	}

	pgDSN := os.Getenv("OPTIMIZED_DSN")
	if pgDSN == "" {
		pgDSN = "postgres://apexstock:apexstock@localhost:5432/apexstock_optimized?sslmode=disable"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	nc, err := messaging.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Seed flash sale event, product, and item
	ctx := context.Background()
	eventID := newID()
	productID := newID()
	itemID := newID()
	userID := newID()

	_, err = db.ExecContext(ctx, `INSERT INTO users(id, email, password_hash, name) VALUES($1, $2, 'hash', 'Test User') ON CONFLICT DO NOTHING`, userID, "user-"+userID+"@test.com")
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO products(id, name, slug, original_price) VALUES($1, 'Product 1', $2, 200000) ON CONFLICT DO NOTHING`, productID, "slug-"+productID)
	if err != nil {
		t.Fatalf("Failed to seed product: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO flash_sale_events(id, name, start_time, end_time, status) VALUES($1, 'Sale 1', now() - interval '1 hour', now() + interval '1 hour', 'active')`, eventID)
	if err != nil {
		t.Fatalf("Failed to seed event: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO flash_sale_items(id, event_id, product_id, sale_price, total_quantity, sold_quantity, max_per_user) VALUES($1, $2, $3, 100000, 10, 0, 2)`, itemID, eventID, productID)
	if err != nil {
		t.Fatalf("Failed to seed flash sale item: %v", err)
	}

	// Seed Redis stock
	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)

	return db, rdb, nc, itemID, userID, productID
}

func TestIntegration_EndToEnd_OrderCreationAndWorkerPersistence(t *testing.T) {
	db, rdb, nc, itemID, userID, _ := setupIntegration(t)
	defer db.Close()
	defer rdb.Close()
	defer nc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker in background
	worker := NewWorker(db, nc)
	go func() {
		_ = worker.Run(ctx)
	}()

	svc := NewService(db, rdb, nc)
	idempKey := "idemp-" + newID()
	req := domain.CreateOrderRequest{
		UserID:          userID,
		FlashSaleItemID: itemID,
		Quantity:        1,
		IdempotencyKey:  idempKey,
	}

	// 1. Create order
	resp, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("svc.Create failed: %v", err)
	}
	if resp.OrderID == "" || resp.ReservationID == "" {
		t.Fatalf("expected valid order and reservation IDs, got: %+v", resp)
	}
	if resp.Idempotent {
		t.Fatalf("first creation should not be idempotent")
	}

	// 2. Poll DB for worker persistence
	var orderFound bool
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		var orderStatus string
		err = db.QueryRowContext(ctx, `SELECT status FROM orders WHERE id = $1`, resp.OrderID).Scan(&orderStatus)
		if err == nil && orderStatus == "pending_payment" {
			orderFound = true
			break
		}
	}
	if !orderFound {
		t.Fatalf("Worker did not persist order %s to DB within timeout", resp.OrderID)
	}

	// 3. Verify order_items and payments
	var itemQty int
	err = db.QueryRowContext(ctx, `SELECT quantity FROM order_items WHERE order_id = $1`, resp.OrderID).Scan(&itemQty)
	if err != nil || itemQty != 1 {
		t.Fatalf("expected order_item with quantity 1, got %d, err: %v", itemQty, err)
	}

	var paymentStatus string
	err = db.QueryRowContext(ctx, `SELECT status FROM payments WHERE order_id = $1`, resp.OrderID).Scan(&paymentStatus)
	if err != nil || paymentStatus != "pending" {
		t.Fatalf("expected payment with status pending, got %s, err: %v", paymentStatus, err)
	}

	// 4. Verify processed_events entry
	var processedExists bool
	err = db.QueryRowContext(ctx, `SELECT true FROM processed_events WHERE event_id = $1`, resp.OrderID).Scan(&processedExists)
	if err != nil || !processedExists {
		t.Fatalf("expected processed_events entry for %s, err: %v", resp.OrderID, err)
	}
}

func TestWorker_TransactionalRollback_AllowsRedelivery(t *testing.T) {
	// Review Focus 2: Test that if DB transaction fails, processed_events is rolled back and message can be redelivered
	db, _, nc, _, _, _ := setupIntegration(t)
	defer db.Close()
	defer nc.Close()

	ctx := context.Background()
	worker := NewWorker(db, nc)

	// An event with non-existent user_id will fail foreign key constraint on orders table
	badEvent := messaging.OrderReserved{
		EventID:        newID(),
		OrderID:        newID(),
		ReservationID:  newID(),
		UserID:         newID(), // Non-existent user
		ItemID:         newID(),
		ProductID:      newID(),
		Quantity:       1,
		TotalAmount:    100000,
		ExpiresAt:      time.Now().Add(5 * time.Minute),
		CreatedAt:      time.Now(),
		IdempotencyKey: "bad-event-key",
	}

	// Process should return error
	data, _ := json.Marshal(badEvent)
	err := worker.process(ctx, &nats.Msg{Data: data})
	if err == nil {
		t.Fatalf("expected foreign key failure, got nil")
	}

	// Verify processed_events was rolled back (not marked processed)
	var exists bool
	err = db.QueryRowContext(ctx, `SELECT true FROM processed_events WHERE event_id = $1`, badEvent.EventID).Scan(&exists)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows for processed_events on rolled back transaction, got: %v", err)
	}
}

func TestWorker_MalformedMessage_HandledGracefully(t *testing.T) {
	// Review Focus 5: Test that malformed JSON or validation failure returns error without panicking
	w := NewWorker(nil, nil)
	err := w.process(context.Background(), &nats.Msg{Data: []byte(`{invalid-json`)})
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}

	err = w.process(context.Background(), &nats.Msg{Data: []byte(`{"quantity": 0}`)})
	if err == nil {
		t.Fatal("expected validation error on quantity 0, got nil")
	}
}
