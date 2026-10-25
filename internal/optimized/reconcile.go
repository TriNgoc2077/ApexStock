package optimized

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

type ReconciliationReport struct {
	ItemID          string    `json:"item_id"`
	TotalQuantity   int32     `json:"total_quantity"`
	SoldQuantity    int32     `json:"sold_quantity"`
	RedisStock      int32     `json:"redis_stock"`
	ActiveReserved  int32     `json:"active_reserved"`
	PaidQuantity    int32     `json:"paid_quantity"`
	CalculatedTotal int32     `json:"calculated_total"`
	IsConsistent    bool      `json:"is_consistent"`
	Discrepancy     int32     `json:"discrepancy"`
	ReconciledAt    time.Time `json:"reconciled_at"`
}

func ReconcileItem(ctx context.Context, db *sql.DB, rdb *redis.Client, itemID string) (ReconciliationReport, error) {
	var total, sold int32
	err := db.QueryRowContext(ctx, `SELECT total_quantity, sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&total, &sold)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("failed to query flash_sale_item: %w", err)
	}

	var reserved int32
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(oi.quantity), 0)
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE oi.flash_sale_item_id = $1 AND o.status = 'pending_payment' AND o.expires_at > now()
	`, itemID).Scan(&reserved)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("failed to query reserved quantity: %w", err)
	}

	var paid int32
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(oi.quantity), 0)
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE oi.flash_sale_item_id = $1 AND o.status = 'paid'
	`, itemID).Scan(&paid)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("failed to query paid quantity: %w", err)
	}

	var redisStock int32
	val, err := rdb.Get(ctx, rediskeys.StockKey(itemID)).Result()
	if err == nil {
		n, _ := strconv.Atoi(val)
		redisStock = int32(n)
	} else if err != redis.Nil {
		return ReconciliationReport{}, fmt.Errorf("failed to read stock from redis: %w", err)
	}

	calcTotal := redisStock + reserved + paid
	discrepancy := total - calcTotal
	consistent := (discrepancy == 0) && (redisStock >= 0) && (sold >= 0) && (sold <= total)

	return ReconciliationReport{
		ItemID:          itemID,
		TotalQuantity:   total,
		SoldQuantity:    sold,
		RedisStock:      redisStock,
		ActiveReserved:  reserved,
		PaidQuantity:    paid,
		CalculatedTotal: calcTotal,
		IsConsistent:    consistent,
		Discrepancy:     discrepancy,
		ReconciledAt:    time.Now().UTC(),
	}, nil
}

func ReconcileAll(ctx context.Context, db *sql.DB, rdb *redis.Client) ([]ReconciliationReport, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM flash_sale_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ReconciliationReport
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		rep, err := ReconcileItem(ctx, db, rdb, id)
		if err != nil {
			return nil, err
		}
		reports = append(reports, rep)
	}
	return reports, rows.Err()
}
