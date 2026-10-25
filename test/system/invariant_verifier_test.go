package system

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func getTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping test: cannot open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping test: DB unreachable: %v", err)
	}
	return db
}

func TestInvariantVerifier_StockEquation_Calculation(t *testing.T) {
	// Unit test calculation logic
	report := evaluateStockConservation(1000, 800, 150, 50)
	if !report.Passed {
		t.Fatalf("expected stock equation to pass when 800 + 150 + 50 == 1000")
	}
	if report.Discrepancy != 0 {
		t.Fatalf("expected discrepancy 0, got %d", report.Discrepancy)
	}

	// Discrepancy: 1000 initial, but 800 avail + 150 paid + 60 pending = 1010 (drift of 10)
	reportBroken := evaluateStockConservation(1000, 800, 150, 60)
	if reportBroken.Passed {
		t.Fatalf("expected stock equation to fail on discrepancy")
	}
	if reportBroken.Discrepancy != 10 {
		t.Fatalf("expected discrepancy 10, got %d", reportBroken.Discrepancy)
	}

	// Negative available stock
	reportNeg := evaluateStockConservation(100, -5, 100, 5)
	if reportNeg.Passed {
		t.Fatalf("expected failure on negative stock")
	}

	// Paid exceeds initial
	reportExcess := evaluateStockConservation(100, 0, 105, 0)
	if reportExcess.Passed {
		t.Fatalf("expected failure on paid > initial")
	}

	// String format test
	if str := report.String(); len(str) == 0 {
		t.Fatalf("expected non-empty report string for passed")
	}
	if str := reportBroken.String(); len(str) == 0 {
		t.Fatalf("expected non-empty report string for failed")
	}
}

func TestInvariantVerifier_NoDuplicateIdempotencyOrders(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()
	var duplicatesCount int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT idempotency_key, user_id FROM orders
			GROUP BY idempotency_key, user_id
			HAVING COUNT(*) > 1
		) d`).Scan(&duplicatesCount)
	if err != nil {
		t.Fatalf("failed to query idempotency duplicates: %v", err)
	}
	if duplicatesCount > 0 {
		t.Fatalf("CRITICAL: Found %d duplicate orders with same idempotency key!", duplicatesCount)
	}
}
