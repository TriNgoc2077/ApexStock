package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"apexstock/internal/domain"
	"apexstock/internal/optimized"
)

// OptimizedOrderHandler applies the common order contract to Redis/Lua and NATS.
func OptimizedOrderHandler(create func(context.Context, domain.CreateOrderRequest) (domain.OrderResponse, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = newCorrelationID()
		}
		var body orderBody
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := dec.Decode(&body); err != nil {
			writeError(w, correlationID, domain.ValidationError("body", "Dữ liệu JSON không hợp lệ"))
			return
		}
		userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
		if userID == "" {
			userID = strings.TrimSpace(body.UserID)
		}
		itemID := strings.TrimSpace(body.FlashSaleItemID)
		if itemID == "" {
			itemID = strings.TrimSpace(body.ItemID)
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			key = strings.TrimSpace(body.IdempotencyKey)
		} else if body.IdempotencyKey != "" && key != body.IdempotencyKey {
			writeError(w, correlationID, domain.ValidationError("idempotency_key", "Idempotency key trong header và body phải khớp"))
			return
		}
		if !validUUID(userID) {
			writeError(w, correlationID, domain.ValidationError("user_id", "user_id phải là UUID hợp lệ"))
			return
		}
		if !validUUID(itemID) {
			writeError(w, correlationID, domain.ValidationError("flash_sale_item_id", "flash_sale_item_id phải là UUID hợp lệ"))
			return
		}
		if body.Quantity != 1 {
			writeError(w, correlationID, domain.ValidationError("quantity", "Dữ liệu không hợp lệ: quantity phải bằng 1"))
			return
		}
		if len(key) < 1 || len(key) > 255 {
			writeError(w, correlationID, domain.ValidationError("idempotency_key", "idempotency_key phải dài từ 1 đến 255 ký tự"))
			return
		}
		got, err := create(r.Context(), domain.CreateOrderRequest{UserID: userID, FlashSaleItemID: itemID, Quantity: body.Quantity, IdempotencyKey: key})
		if err != nil {
			writeOptimizedError(w, correlationID, err)
			return
		}
		writeJSON(w, http.StatusCreated, got)
	})
}
func writeOptimizedError(w http.ResponseWriter, correlationID string, err error) {
	var d *domain.Error
	switch {
	case errors.Is(err, optimized.ErrItemNotFound):
		d = domain.NotFoundError("item_not_found", "Không tìm thấy sản phẩm flash sale")
	case errors.Is(err, optimized.ErrOutOfStock):
		d = domain.ConflictError("out_of_stock", "Sản phẩm đã hết hàng", nil)
	case errors.Is(err, optimized.ErrMaxPerUser):
		d = domain.ConflictError("max_per_user_reached", "Bạn đã đạt giới hạn mua cho sản phẩm này", nil)
	case errors.Is(err, optimized.ErrIdempotencyConflict):
		d = domain.ConflictError("idempotency_conflict", "Idempotency key đã được dùng với payload khác", nil)
	default:
		d = domain.ServiceUnavailableError()
	}
	writeError(w, correlationID, d)
}
