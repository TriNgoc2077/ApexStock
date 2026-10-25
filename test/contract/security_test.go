package contract

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"apexstock/internal/domain"
	"apexstock/internal/httpapi"
	"apexstock/internal/warroom"
)

func TestSecurity_OversizedPayload_Rejected(t *testing.T) {
	// Attempt to send a 2MB payload (exceeding 1MB limit)
	oversizedBody := make([]byte, 2*1024*1024)
	for i := range oversizedBody {
		oversizedBody[i] = 'a'
	}

	for _, target := range getAPIBaseURLs() {
		t.Run(target.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, target.baseURL+"/api/v1/flash-sale/orders", bytes.NewReader(oversizedBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Skipf("skipping %s: server unreachable", target.name)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
				t.Fatalf("expected 400 or 413 for oversized body, got %d", resp.StatusCode)
			}
		})
	}

	// In-process verification against order handler
	t.Run("InProcess_BaselineHandler", func(t *testing.T) {
		handler := httpapi.BaselineOrderHandler(func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
			return domain.OrderResponse{}, nil
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		req, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(oversizedBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 400 or 413 for oversized body, got %d", resp.StatusCode)
		}
	})
}

func TestSecurity_SQLInjectionPayload_RejectedOrEscaped(t *testing.T) {
	sqlInjectionPayloads := []string{
		`' OR '1'='1`,
		`00000000-0000-0000-0000-000000000001'; DROP TABLE orders; --`,
		`" UNION SELECT NULL, NULL, NULL --`,
	}

	for _, target := range getAPIBaseURLs() {
		for _, sql := range sqlInjectionPayloads {
			t.Run(target.name+"_SQLInjection", func(t *testing.T) {
				body := []byte(`{
					"user_id": "` + sql + `",
					"flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
					"quantity": 1,
					"idempotency_key": "sqli-test"
				}`)
				req, _ := http.NewRequest(http.MethodPost, target.baseURL+"/api/v1/flash-sale/orders", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				if err != nil {
					t.Skipf("skipping %s: server unreachable", target.name)
				}
				defer resp.Body.Close()

				// Expect validation rejection (400) because user_id must be valid UUID
				if resp.StatusCode != http.StatusBadRequest {
					t.Fatalf("expected 400 Bad Request on SQL injection payload, got %d", resp.StatusCode)
				}
			})
		}
	}

	// In-process verification
	t.Run("InProcess_SQLInjectionRejection", func(t *testing.T) {
		handler := httpapi.BaselineOrderHandler(func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
			return domain.OrderResponse{}, nil
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		for _, sql := range sqlInjectionPayloads {
			body := []byte(`{
				"user_id": "` + sql + `",
				"flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
				"quantity": 1,
				"idempotency_key": "sqli-test"
			}`)
			resp, err := httpClient.Post(server.URL, "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request on SQL injection payload %q, got %d", sql, resp.StatusCode)
			}
		}
	})
}

func TestSecurity_NoContainerControlEndpointsExposed(t *testing.T) {
	// Decision Gate D0.12: Strict security check that no docker/container control endpoint exists
	warroomURL := os.Getenv("WARROOM_URL")
	if warroomURL == "" {
		warroomURL = "http://localhost:9000"
	}

	probedEndpoints := []string{
		"/api/v1/chaos/kill",
		"/api/v1/chaos/stop",
		"/api/v1/docker/kill",
		"/api/v1/containers/stop",
		"/api/v1/system/exec",
	}

	for _, endpoint := range probedEndpoints {
		req, _ := http.NewRequest(http.MethodPost, warroomURL+endpoint, nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			continue // Unreachable is safe
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("CRITICAL SECURITY VIOLATION: Endpoint %s responded with status %d (must be 404/405)", endpoint, resp.StatusCode)
		}
	}

	// In-process verification against WarRoom router
	t.Run("InProcess_WarRoomRouter", func(t *testing.T) {
		svc := warroom.NewWarRoomService("http://localhost:8081", "http://localhost:8082")
		handler := warroom.NewHandler(svc)
		router := handler.Routes()

		for _, endpoint := range probedEndpoints {
			req := httptest.NewRequest(http.MethodPost, endpoint, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("CRITICAL SECURITY VIOLATION: In-process route %s returned %d (must be 404/405)", endpoint, w.Code)
			}
		}
	})
}
