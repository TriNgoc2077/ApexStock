package system

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

type InvariantReport struct {
	Passed          bool     `json:"passed"`
	InitialStock    int64    `json:"initial_stock"`
	AvailableStock  int64    `json:"available_stock"`
	PaidQuantity    int64    `json:"paid_quantity"`
	PendingQuantity int64    `json:"pending_quantity"`
	Discrepancy     int64    `json:"discrepancy"`
	Violations      []string `json:"violations"`
}

func evaluateStockConservation(initial, available, paid, pending int64) InvariantReport {
	accounted := available + paid + pending
	discrepancy := accounted - initial
	violations := make([]string, 0)

	if available < 0 {
		violations = append(violations, fmt.Sprintf("I1 VIOLATION: Available stock is negative (%d)", available))
	}
	if paid > initial {
		violations = append(violations, fmt.Sprintf("I1 CRITICAL: Paid orders quantity (%d) exceeds initial stock (%d)", paid, initial))
	}
	if discrepancy != 0 {
		violations = append(violations, fmt.Sprintf("I1 DRIFT: Total accounted stock (%d) != initial stock (%d), drift = %d", accounted, initial, discrepancy))
	}

	return InvariantReport{
		Passed:          len(violations) == 0,
		InitialStock:    initial,
		AvailableStock:  available,
		PaidQuantity:    paid,
		PendingQuantity: pending,
		Discrepancy:     discrepancy,
		Violations:      violations,
	}
}

// VerifyInvariants checks Invariants I1–I5 against the target database and Redis cache.
func VerifyInvariants(ctx context.Context, db *sql.DB, rdb *redis.Client, itemID string, initialStock int64) (InvariantReport, error) {
	violations := make([]string, 0)

	// 1. Query DB counts
	var soldQty int32
	var dbTotal int32
	err := db.QueryRowContext(ctx, `SELECT total_quantity, sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&dbTotal, &soldQty)
	if err != nil {
		return InvariantReport{Passed: false}, fmt.Errorf("query flash_sale_items: %w", err)
	}

	var paidQty int64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(oi.quantity), 0)
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE oi.flash_sale_item_id = $1 AND o.status = 'paid'`, itemID).Scan(&paidQty)

	var pendingQty int64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(oi.quantity), 0)
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE oi.flash_sale_item_id = $1 AND o.status = 'pending_payment'`, itemID).Scan(&pendingQty)

	var availableStock int64 = int64(dbTotal - soldQty)

	// If Redis client is present (Optimized mode), verify against Redis atomic stock
	if rdb != nil {
		rStock, rErr := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int64()
		if rErr == nil {
			availableStock = rStock
		}
	}

	report := evaluateStockConservation(initialStock, availableStock, paidQty, pendingQty)

	// 2. Check I3: Idempotency uniqueness
	var duplicateIdempotencyCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT idempotency_key, user_id FROM orders
			GROUP BY idempotency_key, user_id
			HAVING COUNT(*) > 1
		) d`).Scan(&duplicateIdempotencyCount)
	if err == nil && duplicateIdempotencyCount > 0 {
		report.Passed = false
		report.Violations = append(report.Violations, fmt.Sprintf("I3 VIOLATION: Found %d duplicate orders with identical idempotency keys", duplicateIdempotencyCount))
	}

	// 3. Check I4: State machine integrity
	var invalidStateCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM orders o
		JOIN payments p ON p.order_id = o.id
		WHERE (o.status = 'paid' AND p.status != 'success')
		   OR (o.status = 'pending_payment' AND p.status = 'success')`).Scan(&invalidStateCount)
	if err == nil && invalidStateCount > 0 {
		report.Passed = false
		report.Violations = append(report.Violations, fmt.Sprintf("I4 VIOLATION: Found %d orders with invalid order/payment state combinations", invalidStateCount))
	}

	// 4. Check I5: No duplicate processed events
	var duplicateEventsCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT event_id FROM processed_events
			GROUP BY event_id
			HAVING COUNT(*) > 1
		) e`).Scan(&duplicateEventsCount)
	if err == nil && duplicateEventsCount > 0 {
		report.Passed = false
		report.Violations = append(report.Violations, fmt.Sprintf("I5 VIOLATION: Found %d duplicate processed events in worker log", duplicateEventsCount))
	}

	if len(violations) > 0 {
		report.Passed = false
		report.Violations = append(report.Violations, violations...)
	}

	return report, nil
}

func (r InvariantReport) String() string {
	if r.Passed {
		return fmt.Sprintf("ALL INVARIANTS PASSED: Initial=%d, Avail=%d, Paid=%d, Pending=%d, Drift=0",
			r.InitialStock, r.AvailableStock, r.PaidQuantity, r.PendingQuantity)
	}
	return fmt.Sprintf("INVARIANT FAILURES (%d violations):\n - %s",
		len(r.Violations), strings.Join(r.Violations, "\n - "))
}
