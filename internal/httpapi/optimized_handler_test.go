package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apexstock/internal/domain"
	"apexstock/internal/optimized"
)

func TestOptimizedOrderHandler_RedisUnavailable_Returns503(t *testing.T) {
	mockCreate := func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
		return domain.OrderResponse{}, optimized.ErrRedisUnavailable
	}

	handler := OptimizedOrderHandler(mockCreate)
	body := `{"user_id":"a0000000-0000-4000-8000-000000000001","flash_sale_item_id":"b0000000-0000-4000-8000-000000000001","quantity":1,"idempotency_key":"test-key-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flash-sale/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "service_unavailable" {
		t.Fatalf("expected error 'service_unavailable', got %v", resp["error"])
	}
}
