package payments

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	_ "github.com/lib/pq"
)

func TestConcurrency_ConfirmVsCancelVsExpireRace(t *testing.T) {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skip("skipping concurrency test: DB unreachable")
	}
	defer db.Close()

	svc := NewBaselineService(db)
	orderID, itemID, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	var initialSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialSold)

	const workers = 30
	var wg sync.WaitGroup
	var successCount int32
	var restockCount int32

	outcomes := []string{OutcomeSuccess, OutcomeFailure, OutcomeCancel, OutcomeTimeout}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		outcome := outcomes[i%len(outcomes)]
		go func(idx int, out string) {
			defer wg.Done()
			res, err := svc.Process(context.Background(), ProcessPaymentRequest{
				OrderID: orderID,
				Outcome: out,
			})
			if err == nil {
				if res.OrderStatus == OrderStatusPaid && !res.IdempotentReplay {
					atomic.AddInt32(&successCount, 1)
				}
				if res.Restocked {
					atomic.AddInt32(&restockCount, 1)
				}
			}
		}(i, outcome)
	}

	wg.Wait()

	// Invariant verification: exactly one state must win, and restock must happen at most once!
	if restockCount > 1 {
		t.Fatalf("Invariant I2 violation: restock occurred %d times (expected at most 1)", restockCount)
	}
	if successCount > 1 {
		t.Fatalf("Invariant I2 violation: order confirmed %d times (expected at most 1)", successCount)
	}
	if successCount == 1 && restockCount == 1 {
		t.Fatalf("Invariant I1 violation: order was both confirmed and restocked!")
	}

	// Verify DB sold_quantity consistency
	var finalSold int32
	var finalStatus string
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&finalSold)
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&finalStatus)

	if finalStatus == OrderStatusPaid {
		if finalSold != initialSold {
			t.Errorf("expected final sold = %d, got %d", initialSold, finalSold)
		}
	} else {
		if finalSold != initialSold-1 {
			t.Errorf("expected restocked final sold = %d, got %d", initialSold-1, finalSold)
		}
	}
}

func TestConcurrency_ConfirmVsCancelVsExpireRace_Optimized(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()

	svc := NewOptimizedService(db, rdb)
	orderID, reservationID, itemID, userID := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)
	_ = reservationID
	_ = userID

	var initialDBSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialDBSold)

	const workers = 30
	var wg sync.WaitGroup
	var successCount int32
	var restockCount int32

	outcomes := []string{OutcomeSuccess, OutcomeFailure, OutcomeCancel, OutcomeTimeout}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		outcome := outcomes[i%len(outcomes)]
		go func(idx int, out string) {
			defer wg.Done()
			res, err := svc.Process(context.Background(), ProcessPaymentRequest{
				OrderID: orderID,
				Outcome: out,
			})
			if err == nil {
				if res.OrderStatus == OrderStatusPaid && !res.IdempotentReplay {
					atomic.AddInt32(&successCount, 1)
				}
				if res.Restocked {
					atomic.AddInt32(&restockCount, 1)
				}
			}
		}(i, outcome)
	}

	wg.Wait()

	// Invariant verification: exactly one state must win, and restock must happen at most once!
	if restockCount > 1 {
		t.Fatalf("Invariant I2 violation: restock occurred %d times (expected at most 1)", restockCount)
	}
	if successCount > 1 {
		t.Fatalf("Invariant I2 violation: order confirmed %d times (expected at most 1)", successCount)
	}
	if successCount == 1 && restockCount == 1 {
		t.Fatalf("Invariant I1 violation: order was both confirmed and restocked!")
	}
}
