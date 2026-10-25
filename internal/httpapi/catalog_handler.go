package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"apexstock/internal/domain"
	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

type CatalogHandler struct {
	db  *sql.DB
	rdb *redis.Client
}

func NewCatalogHandler(db *sql.DB, rdb *redis.Client) *CatalogHandler {
	return &CatalogHandler{db: db, rdb: rdb}
}

type FlashSaleItemDTO struct {
	ID              string `json:"id"`
	EventID         string `json:"event_id"`
	ProductID       string `json:"product_id"`
	ProductName     string `json:"product_name"`
	ProductDesc     string `json:"product_description"`
	ProductImage    string `json:"product_image"`
	OriginalPrice   int64  `json:"original_price"`
	SalePrice       int64  `json:"sale_price"`
	InitialQuantity int32  `json:"initial_quantity"`
	AvailableStock  int32  `json:"available_stock"`
	MaxPerUser      int32  `json:"max_per_user"`
}

type FlashSaleEventDTO struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	StartTime time.Time          `json:"start_time"`
	EndTime   time.Time          `json:"end_time"`
	Status    string             `json:"status"`
	Items     []FlashSaleItemDTO `json:"items"`
}

type OrderDetailDTO struct {
	OrderID        string    `json:"order_id"`
	UserID         string    `json:"user_id"`
	Status         string    `json:"status"`
	TotalAmount    int64     `json:"total_amount"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	ItemID         string    `json:"item_id"`
	ProductName    string    `json:"product_name"`
	ProductImage   string    `json:"product_image"`
	Quantity       int32     `json:"quantity"`
	UnitPrice      int64     `json:"unit_price"`
	PaymentStatus  string    `json:"payment_status"`
	PaymentMethod  string    `json:"payment_method"`
	TransactionRef *string   `json:"transaction_ref"`
}

// ListActiveEvents returns active or scheduled flash sale events with catalog items and remaining stock.
func (h *CatalogHandler) ListActiveEvents(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	ctx := r.Context()

	rows, err := h.db.QueryContext(ctx, `
		SELECT e.id, e.name, e.start_time, e.end_time, e.status,
		       i.id, i.product_id, p.name, COALESCE(p.description, ''), COALESCE(p.image_url, ''),
		       p.original_price, i.sale_price, i.initial_quantity, i.sold_quantity, i.max_per_user
		FROM flash_sale_events e
		LEFT JOIN flash_sale_items i ON i.event_id = e.id
		LEFT JOIN products p ON p.id = i.product_id
		WHERE e.status IN ('scheduled', 'active')
		ORDER BY e.start_time ASC, i.id ASC`)
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}
	defer rows.Close()

	eventsMap := make(map[string]*FlashSaleEventDTO)
	var orderedEventIDs []string

	for rows.Next() {
		var eventID, eventName, eventStatus string
		var startTime, endTime time.Time
		var itemID, prodID, prodName, prodDesc, prodImg sql.NullString
		var origPrice, salePrice sql.NullInt64
		var initQty, soldQty, maxUser sql.NullInt32

		if err := rows.Scan(
			&eventID, &eventName, &startTime, &endTime, &eventStatus,
			&itemID, &prodID, &prodName, &prodDesc, &prodImg,
			&origPrice, &salePrice, &initQty, &soldQty, &maxUser,
		); err != nil {
			writeError(w, correlationID, domain.ServiceUnavailableError())
			return
		}

		event, exists := eventsMap[eventID]
		if !exists {
			event = &FlashSaleEventDTO{
				ID:        eventID,
				Name:      eventName,
				StartTime: startTime,
				EndTime:   endTime,
				Status:    eventStatus,
				Items:     []FlashSaleItemDTO{},
			}
			eventsMap[eventID] = event
			orderedEventIDs = append(orderedEventIDs, eventID)
		}

		if itemID.Valid {
			availStock := initQty.Int32 - soldQty.Int32
			// In Optimized mode with Redis, read real-time available stock
			if h.rdb != nil {
				if rStock, rErr := h.rdb.Get(ctx, rediskeys.StockKey(itemID.String)).Int64(); rErr == nil {
					availStock = int32(rStock)
				}
			}
			if availStock < 0 {
				availStock = 0
			}

			event.Items = append(event.Items, FlashSaleItemDTO{
				ID:              itemID.String,
				EventID:         eventID,
				ProductID:       prodID.String,
				ProductName:     prodName.String,
				ProductDesc:     prodDesc.String,
				ProductImage:    prodImg.String,
				OriginalPrice:   origPrice.Int64,
				SalePrice:       salePrice.Int64,
				InitialQuantity: initQty.Int32,
				AvailableStock:  availStock,
				MaxPerUser:      maxUser.Int32,
			})
		}
	}

	result := make([]FlashSaleEventDTO, 0, len(orderedEventIDs))
	for _, id := range orderedEventIDs {
		result = append(result, *eventsMap[id])
	}

	writeJSON(w, http.StatusOK, result)
}

// GetItemDetail returns details and real-time stock for a single flash sale item.
func (h *CatalogHandler) GetItemDetail(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	ctx := r.Context()

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, correlationID, domain.ValidationError("id", "ID sản phẩm không hợp lệ"))
		return
	}
	itemID := pathParts[len(pathParts)-1]

	var item FlashSaleItemDTO
	var soldQty int32

	err := h.db.QueryRowContext(ctx, `
		SELECT i.id, i.event_id, i.product_id, p.name, COALESCE(p.description, ''), COALESCE(p.image_url, ''),
		       p.original_price, i.sale_price, i.initial_quantity, i.sold_quantity, i.max_per_user
		FROM flash_sale_items i
		JOIN products p ON p.id = i.product_id
		WHERE i.id = $1`, itemID).Scan(
		&item.ID, &item.EventID, &item.ProductID, &item.ProductName, &item.ProductDesc, &item.ProductImage,
		&item.OriginalPrice, &item.SalePrice, &item.InitialQuantity, &soldQty, &item.MaxPerUser,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, correlationID, domain.NotFoundError("item_not_found", "Không tìm thấy sản phẩm"))
		return
	}
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}

	item.AvailableStock = item.InitialQuantity - soldQty
	if h.rdb != nil {
		if rStock, rErr := h.rdb.Get(ctx, rediskeys.StockKey(itemID)).Int64(); rErr == nil {
			item.AvailableStock = int32(rStock)
		}
	}
	if item.AvailableStock < 0 {
		item.AvailableStock = 0
	}

	writeJSON(w, http.StatusOK, item)
}

// GetOrderDetail returns order metadata and payment status for UI status polling.
func (h *CatalogHandler) GetOrderDetail(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	ctx := r.Context()

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, correlationID, domain.ValidationError("id", "ID đơn hàng không hợp lệ"))
		return
	}
	orderID := pathParts[len(pathParts)-1]

	var dto OrderDetailDTO
	err := h.db.QueryRowContext(ctx, `
		SELECT o.id, o.user_id, o.status, o.total_amount, o.expires_at, o.created_at,
		       oi.flash_sale_item_id, p.name, COALESCE(p.image_url, ''), oi.quantity, oi.unit_price,
		       COALESCE(pm.status::text, 'pending'), COALESCE(pm.method::text, 'bank_transfer'), pm.transaction_ref
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN products p ON p.id = oi.product_id
		LEFT JOIN payments pm ON pm.order_id = o.id
		WHERE o.id = $1`, orderID).Scan(
		&dto.OrderID, &dto.UserID, &dto.Status, &dto.TotalAmount, &dto.ExpiresAt, &dto.CreatedAt,
		&dto.ItemID, &dto.ProductName, &dto.ProductImage, &dto.Quantity, &dto.UnitPrice,
		&dto.PaymentStatus, &dto.PaymentMethod, &dto.TransactionRef,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, correlationID, domain.NotFoundError("order_not_found", "Không tìm thấy đơn hàng"))
		return
	}
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}

	writeJSON(w, http.StatusOK, dto)
}
