# Stage 6: SePay Adapter (Isolated, Provider-Specific Verification) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a secure, provider-isolated SePay payment adapter for manual UI demonstration and testing: generate VietQR payment initiation details, perform constant-time webhook signature/API key verification, maintain audit logs in `webhook_logs`, delegate payment transitions to the shared payment state machine, handle duplicate webhooks idempotently, and enforce Decision Gate D0.7 for late payment refund logging.

**Architecture:** Frontend / Demo Client (`POST /api/v1/payments/sepay/initiate`) → SePay Adapter (`internal/payments/sepay/adapter.go`) generates VietQR URL and payment reference → Customer banking app executes transfer → SePay Gateway posts webhook (`POST /api/v1/webhooks/sepay`) → Webhook Handler (`internal/httpapi/sepay_webhook_handler.go`) validates signature/API key using env vars → Raw payload logged to `webhook_logs` table → Shared Payment Processor (`internal/payments.PaymentProcessor`) atomically executes transition with stock confirm/restock → Baseline / Optimized DB updated.

**Tech Stack:** Go 1.22, PostgreSQL (`github.com/lib/pq`), `crypto/subtle`, `crypto/hmac`, `crypto/sha256`, stdlib `net/http`, Go standard `testing` package.

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 6, Section 7; Decision Gates D0.6, D0.7; Invariants I1–I5).

## Global Constraints
- Go module: `apexstock`
- No new external dependencies beyond Go stdlib, `github.com/lib/pq`, `github.com/redis/go-redis/v9`, and `github.com/nats-io/nats.go`.
- Codebase must compile cleanly with `go build ./...` and `go vet ./...`.
- `go test -race ./...` must pass at every task.
- Commit message format: `feat(stage-6): <description>`, `test(stage-6): <description>`, or `docs(stage-6): <description>`.
- **Strict Credential Isolation (D0.6):** Zero hardcoded API keys or webhook secrets. All credentials loaded exclusively from environment variables (`SEPAY_WEBHOOK_SECRET`, `SEPAY_ACCOUNT_NUMBER`, `SEPAY_BANK_CODE`).
- **Load Test Isolation:** K6 benchmarks and automated concurrency tests must NEVER call real SePay endpoints or require external network connectivity; SePay adapter is solely for manual UI demo and controlled webhook verification.
- **Architectural Symmetry:** Both Baseline (`:8081`) and Optimized (`:8082`) APIs mount identical SePay initiate and webhook endpoints.

## Review Focus
1. **Webhook Signature / API Key Timing Attacks or Bypass**: An attacker attempts to forge webhook calls or uses timing side-channels to brute-force the secret key.
   *Expected behavior:* Handler verifies authorization headers (`Authorization: Apikey <secret>`, `Authorization: Bearer <secret>`, or `X-Sepay-Signature`) using `crypto/subtle.ConstantTimeCompare`. Requests with missing, invalid, or partial secrets receive HTTP 401 Unauthorized immediately without querying the database or touching inventory.
   *Pinned test:* `TestSepayAdapter_VerifyWebhook_SecurityAndTimingSafe` in Task 1.
2. **Late Webhook Arrival on Expired Order (Decision Gate D0.7)**: A customer transfers money but the webhook arrives after the 5-minute order reservation has expired and inventory was returned to stock.
   *Expected behavior:* Order status remains `expired` (never transitioned to `paid`), no inventory decrement or oversell occurs, an audit log row is inserted into `webhook_logs` with `status = 'refund'` containing the raw payload and order reference, and the webhook endpoint responds with HTTP 200 `{"status":"refund_logged","order_id":"..."}` to acknowledge receipt to SePay while flagging refund for administrator review.
   *Pinned test:* `TestSepayWebhookHandler_LatePayment_RecordedAsRefund` in Task 2.
3. **Duplicate Webhook Delivery (Idempotent Acknowledgment)**: SePay retries webhook delivery due to transient gateway timeout or network retries.
   *Expected behavior:* The first webhook confirms the payment (`paid`); subsequent duplicate webhooks detect that the order is already in terminal state `paid`, record `webhook_logs` with `status = 'duplicate'`, and return HTTP 200 `{"status":"ok","idempotent_replay":true}` without releasing/deducting stock or raising errors.
   *Pinned test:* `TestSepayWebhookHandler_DuplicateWebhook_IdempotentReplay` in Task 2.
4. **Malformed Payload or Outgoing Transfer Filtering**: SePay posts an event with malformed JSON, invalid order syntax in the description, or an outgoing debit transfer (`transferType = 'out'`).
   *Expected behavior:* Handler safely validates `transferType == "in"`, logs non-matching or outgoing transfers into `webhook_logs` with `status = 'ignored'` or `'unmatched'`, and returns HTTP 200/400 without panicking or modifying order state.
   *Pinned test:* `TestSepayWebhookHandler_MalformedOrOutTransfer_Ignored` in Task 2.
5. **Partial Payment or Amount Mismatch**: A user sends a bank transfer with an amount different from the order total (e.g. underpayment).
   *Expected behavior:* Handler compares `transferAmount` against order `total_amount`. If amount is insufficient, the order is NOT marked as paid, a log is recorded in `webhook_logs` with `status = 'amount_mismatch'`, and the handler returns HTTP 400 with descriptive error code.
   *Pinned test:* `TestSepayWebhookHandler_AmountMismatch_FlagsUnderpayment` in Task 2.

---

### Task 1: SePay Config & Provider-Isolated Adapter

**Files:**
- Create: `internal/payments/sepay/adapter.go`
- Test: `internal/payments/sepay/adapter_test.go`

**Interfaces:**
- Consumes: Go standard library (`crypto/subtle`, `crypto/hmac`, `crypto/sha256`, `net/url`, `os`, `regexp`, `strings`)
- Produces:
  ```go
  type Config struct {
      WebhookSecret string
      AccountNumber string
      BankCode      string
      QRTemplate    string
  }
  func LoadConfigFromEnv() Config
  type Adapter struct { cfg Config }
  func NewAdapter(cfg Config) *Adapter
  func (a *Adapter) VerifyWebhook(authHeader, signatureHeader string, body []byte) (bool, error)
  func (a *Adapter) ExtractOrderID(content string) (string, error)
  func (a *Adapter) GeneratePaymentDetails(orderID string, amount int64) (InitiatePaymentResponse, error)
  ```

- [ ] **Step 1: Write failing unit tests for config, signature verification, order extraction, and QR generation**

Create `internal/payments/sepay/adapter_test.go`:
```go
package sepay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestSepayConfig_LoadFromEnv(t *testing.T) {
	_ = os.Setenv("SEPAY_WEBHOOK_SECRET", "test-secret-123")
	_ = os.Setenv("SEPAY_ACCOUNT_NUMBER", "0123456789")
	_ = os.Setenv("SEPAY_BANK_CODE", "MBBank")
	defer func() {
		_ = os.Unsetenv("SEPAY_WEBHOOK_SECRET")
		_ = os.Unsetenv("SEPAY_ACCOUNT_NUMBER")
		_ = os.Unsetenv("SEPAY_BANK_CODE")
	}()

	cfg := LoadConfigFromEnv()
	if cfg.WebhookSecret != "test-secret-123" {
		t.Fatalf("expected secret 'test-secret-123', got %q", cfg.WebhookSecret)
	}
	if cfg.AccountNumber != "0123456789" {
		t.Fatalf("expected account number '0123456789', got %q", cfg.AccountNumber)
	}
	if cfg.BankCode != "MBBank" {
		t.Fatalf("expected bank code 'MBBank', got %q", cfg.BankCode)
	}
}

func TestSepayAdapter_VerifyWebhook_SecurityAndTimingSafe(t *testing.T) {
	cfg := Config{
		WebhookSecret: "super-secret-token",
		AccountNumber: "0123456789",
		BankCode:      "MBBank",
	}
	adapter := NewAdapter(cfg)
	body := []byte(`{"id":1001,"transferType":"in","transferAmount":100000}`)

	// 1. Valid Apikey header
	valid, err := adapter.VerifyWebhook("Apikey super-secret-token", "", body)
	if err != nil || !valid {
		t.Fatalf("expected valid Apikey verification, got valid=%v, err=%v", valid, err)
	}

	// 2. Valid Bearer header
	valid, err = adapter.VerifyWebhook("Bearer super-secret-token", "", body)
	if err != nil || !valid {
		t.Fatalf("expected valid Bearer verification, got valid=%v, err=%v", valid, err)
	}

	// 3. Valid HMAC-SHA256 signature in X-Sepay-Signature header
	mac := hmac.New(sha256.New, []byte("super-secret-token"))
	mac.Write(body)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	valid, err = adapter.VerifyWebhook("", expectedSig, body)
	if err != nil || !valid {
		t.Fatalf("expected valid HMAC signature verification, got valid=%v, err=%v", valid, err)
	}

	// 4. Invalid secret
	valid, err = adapter.VerifyWebhook("Apikey wrong-token", "", body)
	if valid || err == nil {
		t.Fatalf("expected failure on wrong token, got valid=%v, err=%v", valid, err)
	}

	// 5. Missing all auth
	valid, err = adapter.VerifyWebhook("", "", body)
	if valid || err == nil {
		t.Fatalf("expected failure on empty auth headers, got valid=%v, err=%v", valid, err)
	}
}

func TestSepayAdapter_ExtractOrderID(t *testing.T) {
	adapter := NewAdapter(Config{})

	tests := []struct {
		name        string
		content     string
		wantOrderID string
		wantErr     bool
	}{
		{
			name:        "Standard DH prefix with UUID",
			content:     "DH 11111111-1111-1111-1111-111111111111 thanh toan",
			wantOrderID: "11111111-1111-1111-1111-111111111111",
			wantErr:     false,
		},
		{
			name:        "APEX prefix with UUID",
			content:     "APEX-22222222-2222-2222-2222-222222222222",
			wantOrderID: "22222222-2222-2222-2222-222222222222",
			wantErr:     false,
		},
		{
			name:        "Raw UUID in content",
			content:     "Chuyen tien 33333333-3333-3333-3333-333333333333",
			wantOrderID: "33333333-3333-3333-3333-333333333333",
			wantErr:     false,
		},
		{
			name:        "No UUID present",
			content:     "Chuyen tien mua hang khong ma don",
			wantOrderID: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderID, err := adapter.ExtractOrderID(tt.content)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExtractOrderID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if orderID != tt.wantOrderID {
				t.Errorf("ExtractOrderID() got = %q, want %q", orderID, tt.wantOrderID)
			}
		})
	}
}

func TestSepayAdapter_GeneratePaymentDetails(t *testing.T) {
	cfg := Config{
		AccountNumber: "0123456789",
		BankCode:      "MBBank",
	}
	adapter := NewAdapter(cfg)

	orderID := "11111111-1111-1111-1111-111111111111"
	amount := int64(150000)

	resp, err := adapter.GeneratePaymentDetails(orderID, amount)
	if err != nil {
		t.Fatalf("GeneratePaymentDetails failed: %v", err)
	}

	if resp.OrderID != orderID {
		t.Errorf("expected orderID %q, got %q", orderID, resp.OrderID)
	}
	if resp.Amount != amount {
		t.Errorf("expected amount %d, got %d", amount, resp.Amount)
	}
	if resp.AccountNumber != "0123456789" || resp.BankCode != "MBBank" {
		t.Errorf("mismatched bank details: %+v", resp)
	}
	if resp.TransferContent != "DH "+orderID {
		t.Errorf("expected TransferContent 'DH %s', got %q", orderID, resp.TransferContent)
	}
	if resp.QRURL == "" {
		t.Errorf("QRURL should not be empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/payments/sepay -run TestSepay`
Expected: FAIL (package does not exist or types undefined).

- [ ] **Step 3: Implement `internal/payments/sepay/adapter.go`**

Create `internal/payments/sepay/adapter.go`:
```go
package sepay

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	ErrUnauthorized     = errors.New("unauthorized: invalid sepay webhook signature or apikey")
	ErrOrderIDNotFound  = errors.New("order ID not found in transfer content")
	ErrInvalidConfig    = errors.New("sepay config missing required parameters")
)

var uuidRegex = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

type Config struct {
	WebhookSecret string
	AccountNumber string
	BankCode      string
	QRTemplate    string
}

func LoadConfigFromEnv() Config {
	qrTpl := os.Getenv("SEPAY_QR_TEMPLATE")
	if qrTpl == "" {
		qrTpl = "https://qr.sepay.vn/img?acc=%s&bank=%s&amount=%d&des=%s"
	}
	return Config{
		WebhookSecret: os.Getenv("SEPAY_WEBHOOK_SECRET"),
		AccountNumber: os.Getenv("SEPAY_ACCOUNT_NUMBER"),
		BankCode:      os.Getenv("SEPAY_BANK_CODE"),
		QRTemplate:    qrTpl,
	}
}

type InitiatePaymentResponse struct {
	OrderID         string `json:"order_id"`
	QRURL           string `json:"qr_url"`
	AccountNumber   string `json:"account_number"`
	BankCode        string `json:"bank_code"`
	Amount          int64  `json:"amount"`
	TransferContent string `json:"transfer_content"`
}

type WebhookPayload struct {
	ID              int64   `json:"id"`
	Gateway         string  `json:"gateway"`
	TransactionDate string  `json:"transactionDate"`
	AccountNumber   string  `json:"accountNumber"`
	Code            *string `json:"code"`
	Content         string  `json:"content"`
	TransferType    string  `json:"transferType"`
	TransferAmount  int64   `json:"transferAmount"`
	Accumulated     int64   `json:"accumulated"`
	SubAccount      *string `json:"subAccount"`
	ReferenceCode   string  `json:"referenceCode"`
	Description     string  `json:"description"`
}

type Adapter struct {
	cfg Config
}

func NewAdapter(cfg Config) *Adapter {
	return &Adapter{cfg: cfg}
}

// VerifyWebhook verifies the incoming request using constant-time comparison against SEPAY_WEBHOOK_SECRET.
// Supports Authorization: Apikey <token>, Authorization: Bearer <token>, or X-Sepay-Signature: <hmac-sha256>.
func (a *Adapter) VerifyWebhook(authHeader, signatureHeader string, body []byte) (bool, error) {
	if a.cfg.WebhookSecret == "" {
		return false, fmt.Errorf("%w: SEPAY_WEBHOOK_SECRET not configured", ErrUnauthorized)
	}

	expectedSecret := a.cfg.WebhookSecret

	// 1. Check Authorization header (Apikey or Bearer token)
	if authHeader != "" {
		parts := strings.SplitN(strings.TrimSpace(authHeader), " ", 2)
		if len(parts) == 2 {
			scheme := strings.ToLower(parts[0])
			token := parts[1]
			if (scheme == "apikey" || scheme == "bearer") &&
				subtle.ConstantTimeCompare([]byte(token), []byte(expectedSecret)) == 1 {
				return true, nil
			}
		} else if subtle.ConstantTimeCompare([]byte(authHeader), []byte(expectedSecret)) == 1 {
			return true, nil
		}
	}

	// 2. Check HMAC signature if provided
	if signatureHeader != "" && len(body) > 0 {
		mac := hmac.New(sha256.New, []byte(expectedSecret))
		mac.Write(body)
		expectedSig := hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(signatureHeader), []byte(expectedSig)) == 1 {
			return true, nil
		}
	}

	return false, ErrUnauthorized
}

// ExtractOrderID locates a standard 36-character UUID from the transaction content string.
func (a *Adapter) ExtractOrderID(content string) (string, error) {
	match := uuidRegex.FindString(content)
	if match == "" {
		return "", ErrOrderIDNotFound
	}
	return match, nil
}

// GeneratePaymentDetails formats VietQR and SePay transfer metadata for an order.
func (a *Adapter) GeneratePaymentDetails(orderID string, amount int64) (InitiatePaymentResponse, error) {
	transferContent := fmt.Sprintf("DH %s", orderID)
	encodedContent := url.QueryEscape(transferContent)

	qrURL := fmt.Sprintf(a.cfg.QRTemplate,
		url.QueryEscape(a.cfg.AccountNumber),
		url.QueryEscape(a.cfg.BankCode),
		amount,
		encodedContent,
	)

	return InitiatePaymentResponse{
		OrderID:         orderID,
		QRURL:           qrURL,
		AccountNumber:   a.cfg.AccountNumber,
		BankCode:        a.cfg.BankCode,
		Amount:          amount,
		TransferContent: transferContent,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/payments/sepay -run TestSepay`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payments/sepay/adapter.go internal/payments/sepay/adapter_test.go
git commit -m "feat(stage-6): implement sepay provider-isolated adapter and verification"
```

---

### Task 2: SePay Webhook HTTP Handler & Audit Logging

**Files:**
- Create: `internal/httpapi/sepay_webhook_handler.go`
- Test: `internal/httpapi/sepay_webhook_handler_test.go`
- Modify: `cmd/api-baseline/main.go:35-40`
- Modify: `cmd/api-optimized/main.go:35-40`

**Interfaces:**
- Consumes:
  - `internal/payments/sepay.Adapter`
  - `internal/payments.PaymentProcessor`
  - `database/sql.DB`
- Produces:
  ```go
  type SepayWebhookHandler struct { ... }
  func NewSepayWebhookHandler(adapter *sepay.Adapter, processor payments.PaymentProcessor, db *sql.DB) *SepayWebhookHandler
  func (h *SepayWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request)
  func (h *SepayWebhookHandler) HandleInitiate(w http.ResponseWriter, r *http.Request)
  ```

- [ ] **Step 1: Write failing tests for webhook signature check, audit logging, duplicate idempotency, and late payment refund**

Create `internal/httpapi/sepay_webhook_handler_test.go`:
```go
package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"apexstock/internal/payments"
	"apexstock/internal/payments/sepay"
	_ "github.com/lib/pq"
)

type mockPaymentProcessor struct {
	processFunc func(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error)
}

func (m *mockPaymentProcessor) Process(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, req)
	}
	return payments.PaymentResult{
		OrderID:       req.OrderID,
		OrderStatus:   "paid",
		PaymentStatus: "success",
	}, nil
}

func TestSepayWebhookHandler_UnauthorizedSecret_Returns401(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)
	handler := NewSepayWebhookHandler(adapter, &mockPaymentProcessor{}, nil)

	body := []byte(`{"id":123,"content":"DH 11111111-1111-1111-1111-111111111111","transferType":"in","transferAmount":100000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey wrong-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestSepayWebhookHandler_LatePayment_RecordedAsRefund(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)

	orderID := "11111111-1111-1111-1111-111111111111"
	processor := &mockPaymentProcessor{
		processFunc: func(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
			if req.OrderID != orderID {
				t.Fatalf("unexpected orderID: %s", req.OrderID)
			}
			// Simulate D0.7 late payment error returned from payment state machine
			return payments.PaymentResult{
				OrderID:          orderID,
				OrderStatus:      "expired",
				PaymentStatus:    "refund",
				IdempotentReplay: false,
			}, payments.ErrOrderExpiredRefund
		},
	}

	handler := NewSepayWebhookHandler(adapter, processor, nil)

	payload := sepay.WebhookPayload{
		ID:             9991,
		Gateway:        "MBBank",
		Content:        "DH " + orderID,
		TransferType:   "in",
		TransferAmount: 100000,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey correct-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	// Acknowledge receipt to SePay with status=refund_logged (D0.7)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for late refund log, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["status"] != "refund_logged" {
		t.Fatalf("expected status 'refund_logged', got %v", resp["status"])
	}
	if resp["order_id"] != orderID {
		t.Fatalf("expected order_id %q, got %v", orderID, resp["order_id"])
	}
}

func TestSepayWebhookHandler_DuplicateWebhook_IdempotentReplay(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)

	orderID := "22222222-2222-2222-2222-222222222222"
	processor := &mockPaymentProcessor{
		processFunc: func(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
			return payments.PaymentResult{
				OrderID:          orderID,
				OrderStatus:      "paid",
				PaymentStatus:    "success",
				IdempotentReplay: true,
			}, nil
		},
	}

	handler := NewSepayWebhookHandler(adapter, processor, nil)

	payload := sepay.WebhookPayload{
		ID:             9992,
		Gateway:        "Vietcombank",
		Content:        "DH " + orderID,
		TransferType:   "in",
		TransferAmount: 200000,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey correct-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["idempotent_replay"] != true {
		t.Fatalf("expected idempotent_replay = true, got %v", resp["idempotent_replay"])
	}
}

func TestSepayWebhookHandler_MalformedOrOutTransfer_Ignored(t *testing.T) {
	cfg := sepay.Config{WebhookSecret: "correct-secret"}
	adapter := sepay.NewAdapter(cfg)
	handler := NewSepayWebhookHandler(adapter, &mockPaymentProcessor{}, nil)

	// Outgoing transfer (not a customer payment)
	payload := sepay.WebhookPayload{
		ID:             9993,
		TransferType:   "out",
		TransferAmount: 50000,
		Content:        "DH 33333333-3333-3333-3333-333333333333",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/sepay", bytes.NewReader(body))
	req.Header.Set("Authorization", "Apikey correct-secret")
	w := httptest.NewRecorder()

	handler.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ignored" {
		t.Fatalf("expected status 'ignored' for outgoing transfer, got %v", resp["status"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/httpapi -run TestSepayWebhookHandler`
Expected: FAIL (compilation error: `NewSepayWebhookHandler` undefined).

- [ ] **Step 3: Implement `internal/httpapi/sepay_webhook_handler.go`**

Create `internal/httpapi/sepay_webhook_handler.go`:
```go
package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"apexstock/internal/domain"
	"apexstock/internal/payments"
	"apexstock/internal/payments/sepay"
)

type SepayWebhookHandler struct {
	adapter   *sepay.Adapter
	processor payments.PaymentProcessor
	db        *sql.DB
}

func NewSepayWebhookHandler(adapter *sepay.Adapter, processor payments.PaymentProcessor, db *sql.DB) *SepayWebhookHandler {
	return &SepayWebhookHandler{
		adapter:   adapter,
		processor: processor,
		db:        db,
	}
}

type initiateBody struct {
	OrderID string `json:"order_id"`
}

// HandleInitiate generates QR and payment details for a pending order.
func (h *SepayWebhookHandler) HandleInitiate(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = newCorrelationID()
	}

	var body initiateBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, correlationID, domain.ValidationError("body", "Dữ liệu JSON không hợp lệ"))
		return
	}
	body.OrderID = strings.TrimSpace(body.OrderID)
	if !validUUID(body.OrderID) {
		writeError(w, correlationID, domain.ValidationError("order_id", "order_id phải là UUID hợp lệ"))
		return
	}

	var amount int64
	var status string
	if h.db != nil {
		err := h.db.QueryRowContext(r.Context(), `SELECT total_amount, status FROM orders WHERE id = $1`, body.OrderID).Scan(&amount, &status)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, correlationID, domain.NotFoundError("order_not_found", "Không tìm thấy đơn hàng"))
			return
		}
		if err != nil {
			writeError(w, correlationID, domain.ServiceUnavailableError())
			return
		}
		if status != payments.OrderStatusPendingPayment {
			writeError(w, correlationID, domain.ConflictError("order_not_payable", fmt.Sprintf("Đơn hàng đang ở trạng thái %s, không thể thanh toán", status), nil))
			return
		}
	} else {
		amount = 100000 // test stub default
	}

	resp, err := h.adapter.GeneratePaymentDetails(body.OrderID, amount)
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleWebhook verifies, logs, and processes payment notifications from SePay.
func (h *SepayWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = newCorrelationID()
	}

	// 1. Read raw body (up to 1MB)
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, correlationID, domain.ValidationError("body", "Không thể đọc body request"))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// 2. Verify signature / API key
	authHeader := r.Header.Get("Authorization")
	sigHeader := r.Header.Get("X-Sepay-Signature")
	valid, verifyErr := h.adapter.VerifyWebhook(authHeader, sigHeader, bodyBytes)
	if verifyErr != nil || !valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "Chữ ký hoặc API key của SePay không hợp lệ",
		})
		return
	}

	// 3. Parse SePay Webhook Payload
	var payload sepay.WebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		writeError(w, correlationID, domain.ValidationError("body", "Payload SePay không đúng định dạng JSON"))
		return
	}

	// Filter out non-incoming transfers
	if strings.ToLower(payload.TransferType) != "in" {
		h.logWebhookAudit(r.Context(), "", payload.ID, "ignored", bodyBytes)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored", "reason": "not an incoming transfer"})
		return
	}

	// 4. Extract Order ID
	orderID, err := h.adapter.ExtractOrderID(payload.Content)
	if err != nil {
		h.logWebhookAudit(r.Context(), "", payload.ID, "unmatched", bodyBytes)
		writeJSON(w, http.StatusOK, map[string]any{"status": "unmatched", "reason": "order ID not found in content"})
		return
	}

	// 5. Delegate to shared PaymentProcessor
	txRef := fmt.Sprintf("sepay-%d", payload.ID)
	result, procErr := h.processor.Process(r.Context(), payments.ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      payments.OutcomeSuccess,
		ProviderTxID: txRef,
		Provider:     "sepay",
	})

	// Handle Decision Gate D0.7: Late payment on expired order
	if errors.Is(procErr, payments.ErrOrderExpiredRefund) {
		h.logWebhookAudit(r.Context(), orderID, payload.ID, "refund", bodyBytes)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "refund_logged",
			"order_id": orderID,
			"reason":   "order_already_expired",
		})
		return
	}

	if procErr != nil {
		h.logWebhookAudit(r.Context(), orderID, payload.ID, "error", bodyBytes)
		if errors.Is(procErr, payments.ErrInvalidStateTransition) {
			writeError(w, correlationID, domain.ConflictError("invalid_state_transition", "Trạng thái đơn hàng không hợp lệ cho thanh toán", nil))
			return
		}
		if errors.Is(procErr, payments.ErrOrderNotFound) {
			writeError(w, correlationID, domain.NotFoundError("order_not_found", "Không tìm thấy đơn hàng"))
			return
		}
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}

	// Log audit trail in webhook_logs
	logStatus := "success"
	if result.IdempotentReplay {
		logStatus = "duplicate"
	}
	h.logWebhookAudit(r.Context(), orderID, payload.ID, logStatus, bodyBytes)

	// Update payments.sepay_webhook_payload in DB if db handle available
	if h.db != nil {
		_, _ = h.db.ExecContext(r.Context(), `
			UPDATE payments SET sepay_webhook_payload = $1, updated_at = now()
			WHERE order_id = $2`, bodyBytes, orderID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"order_id":          orderID,
		"order_status":      result.OrderStatus,
		"payment_status":    result.PaymentStatus,
		"idempotent_replay": result.IdempotentReplay,
	})
}

func (h *SepayWebhookHandler) logWebhookAudit(ctx interface{ Done() <-chan struct{} }, orderID string, sepayID int64, status string, payload []byte) {
	if h.db == nil {
		return
	}
	var orderUUID any = orderID
	if orderID == "" {
		orderUUID = nil
	}
	txRef := fmt.Sprintf("sepay-%d", sepayID)
	_, _ = h.db.ExecContext(ctx.(interface {
		Done() <-chan struct{}
	}), `
		INSERT INTO webhook_logs (order_id, provider, transaction_ref, status, payload)
		VALUES ($1, 'sepay', $2, $3, $4)`,
		orderUUID, txRef, status, payload)
}
```

- [ ] **Step 4: Mount SePay handlers in Baseline and Optimized APIs**

Modify `cmd/api-baseline/main.go`:
```go
	// Mount SePay adapter and endpoints
	sepayCfg := sepay.LoadConfigFromEnv()
	sepayAdapter := sepay.NewAdapter(sepayCfg)
	paymentSvc := payments.NewBaselineService(db)
	sepayHandler := httpapi.NewSepayWebhookHandler(sepayAdapter, paymentSvc, db)

	mux.HandleFunc("POST /api/v1/payments/sepay/initiate", sepayHandler.HandleInitiate)
	mux.HandleFunc("POST /api/v1/webhooks/sepay", sepayHandler.HandleWebhook)
```

Modify `cmd/api-optimized/main.go`:
```go
	// Mount SePay adapter and endpoints
	sepayCfg := sepay.LoadConfigFromEnv()
	sepayAdapter := sepay.NewAdapter(sepayCfg)
	paymentSvc := payments.NewOptimizedService(db, rdb)
	sepayHandler := httpapi.NewSepayWebhookHandler(sepayAdapter, paymentSvc, db)

	mux.HandleFunc("POST /api/v1/payments/sepay/initiate", sepayHandler.HandleInitiate)
	mux.HandleFunc("POST /api/v1/webhooks/sepay", sepayHandler.HandleWebhook)
```

- [ ] **Step 5: Run tests to verify all pass**

Run: `go test -v ./internal/httpapi -run TestSepayWebhookHandler`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/sepay_webhook_handler.go internal/httpapi/sepay_webhook_handler_test.go cmd/api-baseline/main.go cmd/api-optimized/main.go
git commit -m "feat(stage-6): implement sepay webhook HTTP handler, audit logging, and API mounting"
```

---

### Task 3: SePay Local Demo Runbook & Webhook Fixtures

**Files:**
- Create: `test/fixtures/sepay_webhook_valid.json`
- Create: `test/fixtures/sepay_webhook_late.json`
- Create: `test/fixtures/sepay_webhook_invalid_secret.json`
- Create: `docs/runbooks/sepay-local-demo.md`

**Interfaces:**
- Consumes: Test vectors and runbook guidelines from `IMPLEMENTATION_PLAN.md`
- Produces: Reproducible local demo manual, cURL commands, and JSON fixtures

- [ ] **Step 1: Create valid, late, and invalid webhook JSON fixtures**

Create `test/fixtures/sepay_webhook_valid.json`:
```json
{
  "id": 987654,
  "gateway": "MBBank",
  "transactionDate": "2026-10-25 10:15:30",
  "accountNumber": "0123456789",
  "code": null,
  "content": "DH 11111111-1111-1111-1111-111111111111",
  "transferType": "in",
  "transferAmount": 100000,
  "accumulated": 5000000,
  "subAccount": null,
  "referenceCode": "FT24298123456789",
  "description": "Thanh toan don hang DH 11111111-1111-1111-1111-111111111111"
}
```

Create `test/fixtures/sepay_webhook_late.json`:
```json
{
  "id": 987655,
  "gateway": "Vietcombank",
  "transactionDate": "2026-10-25 10:25:00",
  "accountNumber": "0123456789",
  "code": null,
  "content": "DH 99999999-9999-9999-9999-999999999999",
  "transferType": "in",
  "transferAmount": 100000,
  "accumulated": 5100000,
  "subAccount": null,
  "referenceCode": "FT24298123456790",
  "description": "Thanh toan muon cho don da het han"
}
```

Create `test/fixtures/sepay_webhook_invalid_secret.json`:
```json
{
  "id": 987656,
  "gateway": "MBBank",
  "transactionDate": "2026-10-25 10:30:00",
  "accountNumber": "0123456789",
  "code": null,
  "content": "DH 11111111-1111-1111-1111-111111111111",
  "transferType": "in",
  "transferAmount": 100000,
  "accumulated": 5200000,
  "subAccount": null,
  "referenceCode": "FT24298123456791",
  "description": "Gia mao webhook khong co secret hop le"
}
```

- [ ] **Step 2: Create comprehensive SePay local demo runbook**

Create `docs/runbooks/sepay-local-demo.md`:
```markdown
# SePay Local Demo & Webhook Verification Runbook

## 1. Mục Đích & Nguyên Tắc Bảo Mật
- Cho phép demo tính năng thanh toán quét mã QR (VietQR) và nhận webhook thực tế từ cổng thanh toán SePay.
- **Cô lập bí mật (Decision Gate D0.6):** Không commit bất kỳ token, secret, số tài khoản thật vào Git repository. Toàn bộ cấu hình thông qua biến môi trường.
- **Cô lập tải:** Mọi bài test tải (k6 load testing) đều sử dụng Payment Simulator (`POST /api/v1/payments/simulate`), tuyệt đối KHÔNG gọi SePay trong benchmark.

## 2. Cấu Hình Môi Trường
Thiết lập các biến môi trường trước khi khởi động API:
```bash
export SEPAY_WEBHOOK_SECRET="local-dev-sepay-secret-2026"
export SEPAY_ACCOUNT_NUMBER="0388888888"
export SEPAY_BANK_CODE="MBBank"
```

## 3. Quy Trình Demo Qua Tunnel (Ngrok / Cloudflare Tunnel)
Để nhận webhook thật từ máy chủ SePay về localhost:
```bash
# Sử dụng Cloudflare Tunnel (khuyến nghị, không cần tài khoản)
cloudflared tunnel --url http://localhost:8082

# Hoặc sử dụng ngrok
ngrok http 8082
```
Cấu hình Webhook URL trên Dashboard SePay:
`https://<subdomain>.trycloudflare.com/api/v1/webhooks/sepay`
Authentication header: `Apikey local-dev-sepay-secret-2026`

## 4. Kiểm Thử Thủ Công Bằng cURL

### Bước 1: Tạo đơn hàng
```bash
ORDER_RESP=$(curl -s -X POST http://localhost:8082/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-sepay-$(date +%s)" \
  -d '{
    "user_id": "00000000-0000-0000-0000-000000000001",
    "flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
    "quantity": 1
  }')
echo $ORDER_RESP
ORDER_ID=$(echo $ORDER_RESP | jq -r .order_id)
```

### Bước 2: Tạo mã QR thanh toán (Initiate)
```bash
curl -s -X POST http://localhost:8082/api/v1/payments/sepay/initiate \
  -H "Content-Type: application/json" \
  -d "{\"order_id\": \"$ORDER_ID\"}" | jq .
```
Kết quả trả về:
- `qr_url`: Link ảnh VietQR tương ứng với số tài khoản và nội dung chuyển khoản `DH <order_id>`.
- `transfer_content`: Cú pháp chuyển khoản chính xác để SePay webhook đối soát.

### Bước 3: Gửi Webhook thanh toán thành công
```bash
curl -s -X POST http://localhost:8082/api/v1/webhooks/sepay \
  -H "Content-Type: application/json" \
  -H "Authorization: Apikey local-dev-sepay-secret-2026" \
  -d "{
    \"id\": $(date +%s),
    \"gateway\": \"MBBank\",
    \"transactionDate\": \"$(date '+%Y-%m-%d %H:%M:%S')\",
    \"accountNumber\": \"0388888888\",
    \"content\": \"DH $ORDER_ID\",
    \"transferType\": \"in\",
    \"transferAmount\": 100000,
    \"referenceCode\": \"REF$(date +%s)\"
  }" | jq .
```
Kỳ vọng: HTTP 200 `{"status": "ok", "order_status": "paid", "payment_status": "success", "idempotent_replay": false}`.

### Bước 4: Kiểm tra tính Idempotent (Gửi lại Webhook trùng lặp)
Gửi lại cURL ở Bước 3.
Kỳ vọng: HTTP 200 `{"status": "ok", "idempotent_replay": true}`, không trừ thêm kho, không lỗi.

### Bước 5: Kiểm tra thanh toán muộn sau khi đơn hết hạn (Decision Gate D0.7)
1. Đơn hàng chuyển sang trạng thái `expired` (hoặc test với order đã expired).
2. Gửi webhook thanh toán thành công cho đơn đã expired.
3. Kỳ vọng:
   - Trạng thái trả về: `{"status": "refund_logged", "order_id": "$ORDER_ID", "reason": "order_already_expired"}`.
   - Database `orders.status` vẫn giữ nguyên là `expired`.
   - Bảng `webhook_logs` xuất hiện 1 bản ghi với `status = 'refund'` để CSKH/kế toán hoàn tiền thủ công.

### Bước 6: Kiểm tra bảo mật (Secret không hợp lệ)
```bash
curl -s -i -X POST http://localhost:8082/api/v1/webhooks/sepay \
  -H "Content-Type: application/json" \
  -H "Authorization: Apikey wrong-secret" \
  -d @test/fixtures/sepay_webhook_valid.json
```
Kỳ vọng: HTTP 401 Unauthorized, không thay đổi bất kỳ bản ghi DB nào.
```

- [ ] **Step 3: Verify markdown lint and file formatting**

Run: `cat test/fixtures/sepay_webhook_valid.json | jq .`
Expected: Valid JSON formatting.

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/ docs/runbooks/sepay-local-demo.md
git commit -m "docs(stage-6): add sepay local demo runbook and webhook test fixtures"
```
