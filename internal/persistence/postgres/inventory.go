package postgres

import (
	"context"
	"database/sql"
	"time"
)

// FlashSaleItem is the inventory projection used by both architectures.
type FlashSaleItem struct {
	ID            string
	EventID       string
	ProductID     string
	SalePrice     int64
	TotalQuantity int32
	SoldQuantity  int32
	MaxPerUser    int32
	EventName     string
	StartTime     time.Time
	EndTime       time.Time
	EventStatus   string
}

// PendingReservation describes an order reservation that still holds stock.
type PendingReservation struct {
	OrderID       string
	UserID        string
	ItemID        string
	Quantity      int32
	ExpiresAt     time.Time
	ReservationID string
}

// InventoryRepository provides read and transaction primitives for inventory.
type InventoryRepository struct{ db *sql.DB }

func NewInventoryRepository(db *sql.DB) *InventoryRepository { return &InventoryRepository{db: db} }

// GetActiveItems returns active flash-sale items and their event metadata.
func (r *InventoryRepository) GetActiveItems(ctx context.Context) ([]FlashSaleItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT fsi.id, fsi.event_id, fsi.product_id, fsi.sale_price,
		       fsi.total_quantity, fsi.sold_quantity, fsi.max_per_user,
		       fse.name, fse.start_time, fse.end_time, fse.status::text
		FROM flash_sale_items fsi
		JOIN flash_sale_events fse ON fse.id = fsi.event_id
		WHERE fse.status = 'active' ORDER BY fsi.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]FlashSaleItem, 0)
	for rows.Next() {
		var item FlashSaleItem
		if err := rows.Scan(&item.ID, &item.EventID, &item.ProductID, &item.SalePrice,
			&item.TotalQuantity, &item.SoldQuantity, &item.MaxPerUser,
			&item.EventName, &item.StartTime, &item.EndTime, &item.EventStatus); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetItemForUpdate locks one inventory row inside tx, preventing oversell.
func (r *InventoryRepository) GetItemForUpdate(ctx context.Context, tx *sql.Tx, itemID string) (FlashSaleItem, error) {
	var item FlashSaleItem
	err := tx.QueryRowContext(ctx, `
		SELECT fsi.id, fsi.event_id, fsi.product_id, fsi.sale_price,
		       fsi.total_quantity, fsi.sold_quantity, fsi.max_per_user,
		       fse.name, fse.start_time, fse.end_time, fse.status::text
		FROM flash_sale_items fsi
		JOIN flash_sale_events fse ON fse.id = fsi.event_id
		WHERE fsi.id = $1 FOR UPDATE`, itemID).Scan(
		&item.ID, &item.EventID, &item.ProductID, &item.SalePrice,
		&item.TotalQuantity, &item.SoldQuantity, &item.MaxPerUser,
		&item.EventName, &item.StartTime, &item.EndTime, &item.EventStatus)
	return item, err
}

// ListPendingReservations is used to warm/rebuild Redis after restart.
func (r *InventoryRepository) ListPendingReservations(ctx context.Context) ([]PendingReservation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT o.id, o.user_id, oi.flash_sale_item_id, oi.quantity, o.expires_at,
		       o.id::text || ':' || o.user_id::text || ':' || oi.quantity::text
		FROM orders o JOIN order_items oi ON oi.order_id = o.id
		WHERE o.status = 'pending_payment' AND o.expires_at > now() ORDER BY o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reservations := make([]PendingReservation, 0)
	for rows.Next() {
		var reservation PendingReservation
		if err := rows.Scan(&reservation.OrderID, &reservation.UserID, &reservation.ItemID,
			&reservation.Quantity, &reservation.ExpiresAt, &reservation.ReservationID); err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func (r *InventoryRepository) ResetItemInventory(ctx context.Context, itemID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE flash_sale_items SET sold_quantity = 0 WHERE id = $1`, itemID)
	return err
}

func (r *InventoryRepository) DB() *sql.DB { return r.db }
