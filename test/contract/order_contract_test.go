package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"apexstock/internal/domain"
	"apexstock/internal/httpapi"
	"apexstock/internal/persistence/postgres"
)

type errorEnvelope struct {
	Error         string         `json:"error"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id"`
	Details       map[string]any `json:"details,omitempty"`
}

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func getAPIBaseURLs() []struct {
	name    string
	baseURL string
} {
	baselineURL := os.Getenv("BASELINE_URL")
	if baselineURL == "" {
		baselineURL = "http://localhost:8081"
	}
	optimizedURL := os.Getenv("OPTIMIZED_URL")
	if optimizedURL == "" {
		optimizedURL = "http://localhost:8082"
	}
	return []struct {
		name    string
		baseURL string
	}{
		{"Baseline", baselineURL},
		{"Optimized", optimizedURL},
	}
}

func TestOrderContract_InvalidJSON_Returns400WithEnvelope(t *testing.T) {
	for _, target := range getAPIBaseURLs() {
		t.Run(target.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, target.baseURL+"/api/v1/flash-sale/orders", bytes.NewReader([]byte(`{invalid-json`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Correlation-ID", "corr-contract-400")

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Skipf("skipping %s: server unreachable: %v", target.name, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
			}

			var env errorEnvelope
			if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
				t.Fatalf("failed to decode ErrorEnvelope: %v", err)
			}
			if env.Error != "validation_error" {
				t.Errorf("expected error code 'validation_error', got %q", env.Error)
			}
			if env.CorrelationID != "corr-contract-400" {
				t.Errorf("expected correlation_id 'corr-contract-400', got %q", env.CorrelationID)
			}
		})
	}
}

func TestOrderContract_NonExistentItem_Returns404WithEnvelope(t *testing.T) {
	for _, target := range getAPIBaseURLs() {
		t.Run(target.name, func(t *testing.T) {
			body := []byte(`{
				"user_id": "00000000-0000-0000-0000-000000000001",
				"flash_sale_item_id": "ffffffff-ffff-ffff-ffff-ffffffffffff",
				"quantity": 1,
				"idempotency_key": "contract-404-test"
			}`)
			req, _ := http.NewRequest(http.MethodPost, target.baseURL+"/api/v1/flash-sale/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Skipf("skipping %s: server unreachable: %v", target.name, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404 Not Found, got %d", resp.StatusCode)
			}

			var env errorEnvelope
			_ = json.NewDecoder(resp.Body).Decode(&env)
			if env.Error != "item_not_found" {
				t.Errorf("expected error code 'item_not_found', got %q", env.Error)
			}
		})
	}
}

func TestOrderContract_QuantityNotOne_Returns400(t *testing.T) {
	// D0.4: Flash sale orders must strictly have quantity = 1
	for _, target := range getAPIBaseURLs() {
		t.Run(target.name, func(t *testing.T) {
			body := []byte(`{
				"user_id": "00000000-0000-0000-0000-000000000001",
				"flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
				"quantity": 2,
				"idempotency_key": "contract-qty-2"
			}`)
			req, _ := http.NewRequest(http.MethodPost, target.baseURL+"/api/v1/flash-sale/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Skipf("skipping %s: server unreachable", target.name)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request for quantity=2, got %d", resp.StatusCode)
			}
		})
	}
}

func TestOrderContract_InProcessHandlerVerification(t *testing.T) {
	handler := httpapi.BaselineOrderHandler(func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
		if req.FlashSaleItemID == "ffffffff-ffff-ffff-ffff-ffffffffffff" {
			return domain.OrderResponse{}, postgres.ErrNotFound
		}
		return domain.OrderResponse{
			OrderID: "11111111-1111-1111-1111-111111111111",
			Status:  "pending_payment",
		}, nil
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// 1. Invalid JSON
	reqInvalid, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader([]byte(`{not-json`)))
	reqInvalid.Header.Set("Content-Type", "application/json")
	reqInvalid.Header.Set("X-Correlation-ID", "corr-test-123")
	respInvalid, err := httpClient.Do(reqInvalid)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respInvalid.Body.Close()
	if respInvalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", respInvalid.StatusCode)
	}
	var envInvalid errorEnvelope
	if err := json.NewDecoder(respInvalid.Body).Decode(&envInvalid); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if envInvalid.Error != "validation_error" || envInvalid.CorrelationID != "corr-test-123" {
		t.Fatalf("unexpected error envelope: %+v", envInvalid)
	}

	// 2. Non-existent item (404)
	body404 := []byte(`{
		"user_id": "00000000-0000-0000-0000-000000000001",
		"flash_sale_item_id": "ffffffff-ffff-ffff-ffff-ffffffffffff",
		"quantity": 1,
		"idempotency_key": "contract-404-test"
	}`)
	resp404, err := httpClient.Post(server.URL, "application/json", bytes.NewReader(body404))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp404.Body.Close()
	if resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp404.StatusCode)
	}
	var env404 errorEnvelope
	_ = json.NewDecoder(resp404.Body).Decode(&env404)
	if env404.Error != "item_not_found" {
		t.Fatalf("expected item_not_found, got %s", env404.Error)
	}

	// 3. Quantity != 1 (400)
	bodyQty2 := []byte(`{
		"user_id": "00000000-0000-0000-0000-000000000001",
		"flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
		"quantity": 2,
		"idempotency_key": "contract-qty-2"
	}`)
	respQty2, err := httpClient.Post(server.URL, "application/json", bytes.NewReader(bodyQty2))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respQty2.Body.Close()
	if respQty2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", respQty2.StatusCode)
	}
}

