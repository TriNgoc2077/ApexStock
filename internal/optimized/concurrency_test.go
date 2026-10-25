package optimized

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"apexstock/internal/domain"
	rediskeys "apexstock/internal/persistence/redis"
)

func TestConcurrency_ReserveNoOversell(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping concurrency test; set INTEGRATION_TEST=1")
	}

	db, rdb, nc, itemID, _, _ := setupIntegration(t)
	defer db.Close()
	defer rdb.Close()
	defer nc.Close()

	ctx := context.Background()
	svc := NewService(db, rdb, nc)

	// Available stock = 10
	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)

	const totalRequests = 100
	var successCount int32
	var oosCount int32
	var otherErrors int32

	var wg sync.WaitGroup
	wg.Add(totalRequests)

	for i := 0; i < totalRequests; i++ {
		go func() {
			defer wg.Done()
			uniqueUser := newID()
			_, _ = db.Exec(`INSERT INTO users(id, email, password_hash, name) VALUES($1, $2, 'hash', 'User') ON CONFLICT DO NOTHING`, uniqueUser, "u-"+uniqueUser+"@test.com")

			req := domain.CreateOrderRequest{
				UserID:          uniqueUser,
				FlashSaleItemID: itemID,
				Quantity:        1,
				IdempotencyKey:  "key-" + newID(),
			}

			_, err := svc.Create(ctx, req)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if errors.Is(err, ErrOutOfStock) {
				atomic.AddInt32(&oosCount, 1)
			} else {
				atomic.AddInt32(&otherErrors, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != 10 {
		t.Fatalf("Invariant I1 violated: expected exactly 10 successes, got %d (other errors: %d)", successCount, otherErrors)
	}
	if oosCount != 90 {
		t.Fatalf("expected exactly 90 out of stock errors, got %d", oosCount)
	}

	finalStock, _ := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int()
	if finalStock != 0 {
		t.Fatalf("expected final Redis stock to be 0, got %d", finalStock)
	}
}

func TestConcurrency_SameIdempotencyKeySingleReservation(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping concurrency test; set INTEGRATION_TEST=1")
	}

	db, rdb, nc, itemID, userID, _ := setupIntegration(t)
	defer db.Close()
	defer rdb.Close()
	defer nc.Close()

	ctx := context.Background()
	svc := NewService(db, rdb, nc)

	// Stock = 10
	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)

	const concurrency = 50
	sameKey := "shared-idemp-" + newID()

	var wg sync.WaitGroup
	wg.Add(concurrency)

	var newReservations int32
	var idempotentReplays int32
	var orderIDs sync.Map

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			req := domain.CreateOrderRequest{
				UserID:          userID,
				FlashSaleItemID: itemID,
				Quantity:        1,
				IdempotencyKey:  sameKey,
			}

			resp, err := svc.Create(ctx, req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			orderIDs.Store(resp.OrderID, true)
			if resp.Idempotent {
				atomic.AddInt32(&idempotentReplays, 1)
			} else {
				atomic.AddInt32(&newReservations, 1)
			}
		}()
	}

	wg.Wait()

	if newReservations != 1 {
		t.Fatalf("Invariant I3 violated: expected exactly 1 new reservation, got %d", newReservations)
	}
	if idempotentReplays != concurrency-1 {
		t.Fatalf("expected %d idempotent replays, got %d", concurrency-1, idempotentReplays)
	}

	// Verify stock was only decremented by 1
	stock, _ := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int()
	if stock != 9 {
		t.Fatalf("stock drift: expected 9, got %d", stock)
	}
}
