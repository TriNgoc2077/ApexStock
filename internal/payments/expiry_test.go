package payments

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func TestExpirySweeper_SweepsExpiredOnly(t *testing.T) {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skip("skipping expiry sweeper test: DB not reachable")
	}
	defer db.Close()

	svc := NewBaselineService(db)
	sweeper := NewExpirySweeper(db, svc)

	// Create an expired order and an active order
	expiredOrderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)
	_, _ = db.Exec(`UPDATE orders SET expires_at = now() - interval '10 seconds' WHERE id = $1`, expiredOrderID)

	activeOrderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)
	_, _ = db.Exec(`UPDATE orders SET expires_at = now() + interval '5 minutes' WHERE id = $1`, activeOrderID)

	count, err := sweeper.SweepOnce(context.Background(), 50)
	if err != nil {
		t.Fatalf("SweepOnce failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 swept order, got %d", count)
	}

	var expiredStatus string
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, expiredOrderID).Scan(&expiredStatus)
	if expiredStatus != OrderStatusExpired {
		t.Errorf("expired order status = %q, want expired", expiredStatus)
	}

	var activeStatus string
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, activeOrderID).Scan(&activeStatus)
	if activeStatus != OrderStatusPendingPayment {
		t.Errorf("active order status changed = %q, want pending_payment", activeStatus)
	}
}
