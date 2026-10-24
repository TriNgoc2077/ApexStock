package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"apexstock/internal/domain"
	"apexstock/internal/persistence/postgres"
)

type orderBody struct {
	UserID          string `json:"user_id"`
	FlashSaleItemID string `json:"flash_sale_item_id"`
	ItemID          string `json:"item_id"`
	Quantity        int32  `json:"quantity"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type OrderCreator interface {
	Create(rctx interface{ Done() <-chan struct{} }, req domain.CreateOrderRequest) (domain.OrderResponse, error)
}

// BaselineOrderHandler handles validation and maps service errors to the common envelope.
func BaselineOrderHandler(create func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = newCorrelationID()
		}
		var body orderBody
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := decoder.Decode(&body); err != nil {
			writeError(w, correlationID, domain.ValidationError("body", "Dữ liệu JSON không hợp lệ"))
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, correlationID, domain.ValidationError("body", "Request chỉ được chứa một JSON object"))
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
		result, err := create(r.Context(), domain.CreateOrderRequest{UserID: userID, FlashSaleItemID: itemID, Quantity: body.Quantity, IdempotencyKey: key})
		if err != nil {
			writeOrderError(w, correlationID, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	})
}

func writeOrderError(w http.ResponseWriter, correlationID string, err error) {
	var de *domain.Error
	switch {
	case errors.Is(err, postgres.ErrUserNotFound):
		de = domain.ValidationError("user_id", "Không tìm thấy người dùng")
	case errors.Is(err, postgres.ErrNotFound):
		de = domain.NotFoundError("item_not_found", "Không tìm thấy sản phẩm flash sale")
	case errors.Is(err, postgres.ErrOutOfStock):
		de = domain.ConflictError("out_of_stock", "Sản phẩm đã hết hàng", nil)
	case errors.Is(err, postgres.ErrMaxPerUser):
		de = domain.ConflictError("max_per_user_reached", "Bạn đã đạt giới hạn mua cho sản phẩm này", nil)
	case errors.Is(err, postgres.ErrIdempotencyConflict):
		de = domain.ConflictError("idempotency_conflict", "Idempotency key đã được dùng với payload khác", nil)
	default:
		de = domain.ServiceUnavailableError()
	}
	writeError(w, correlationID, de)
}

type errorEnvelope struct {
	Error         string         `json:"error"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
}

func writeError(w http.ResponseWriter, correlationID string, err *domain.Error) {
	writeJSON(w, err.Status, errorEnvelope{Error: err.Code, Message: err.Message, CorrelationID: correlationID, Details: err.Details})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func newCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
