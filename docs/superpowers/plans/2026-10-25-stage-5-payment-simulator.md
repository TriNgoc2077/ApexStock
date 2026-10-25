# Stage 5: Payment Simulator, Confirm/Fail/Timeout/Cancel & Restock — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and harden the complete payment and inventory restock lifecycle for both Baseline and Optimized architectures: state machine transition validation, deterministic & ratio-driven payment simulator, periodic order expiry sweeper, HTTP simulation endpoints, and concurrency/idempotency tests proving Invariants I1–I5 and zero post-run reconciliation drift.

**Architecture:** Shared State Machine (`internal/payments/state_machine.go`) → Architecture-specific restock handlers (Baseline: PostgreSQL transactional `sold_quantity` decrement; Optimized: Redis Lua `Release` / `Confirm` + DB status update) → Payment Simulator (`internal/payments/simulator.go`) → HTTP endpoints (`POST /api/v1/payments/simulate`) → Periodic Expiry Sweeper (`internal/payments/expiry.go` & `cmd/scheduler/main.go`).

**Tech Stack:** Go 1.22, PostgreSQL (`github.com/lib/pq`), Redis (`github.com/redis/go-redis/v9`), NATS JetStream (`github.com/nats-io/nats.go`), standard Go `testing` library.

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 5, Section 6 & 7; Decision Gates D0.3, D0.6, D0.7; Invariants I1–I5).

## Global Constraints
- Go module: `apexstock`
- No new external dependencies beyond stdlib, `github.com/lib/pq`, `github.com/redis/go-redis/v9`, and `github.com/nats-io/nats.go`.
- Codebase must compile cleanly with `go build ./...` and `go vet ./...`.
- `go test -race ./...` must pass at every task.
- Commit message format: `feat(stage-5): <description>` or `test(stage-5): <description>`.
- Do NOT alter existing migration SQL schemas or run destructive operations on seeded records.
- Database isolation (D0.15): Baseline operations connect to `apexstock_baseline`; Optimized operations connect to `apexstock_optimized`.

## Review Focus
1. **Late Payment on Expired Order (Decision Gate D0.7)**: A payment success webhook or simulation arrives after the order has already transitioned to `expired`.
   *Expected behavior:* Order status MUST remain `expired` (never reverted to `paid`), no stock deduction/oversell occurs, payment status in DB remains `failed` (complying with `payment_status` ENUM), and an audit entry is inserted into `webhook_logs` with `status = 'refund'`, returning HTTP 409 Conflict with code `order_already_expired`.
   *Pinned test:* `TestStateMachine_LatePayment_RecordedAsRefund` in Task 1, and `TestBaseline_LatePayment_RecordsWebhookRefundLog` in Task 2.
2. **Double Restock on Concurrent/Duplicate Cancel or Expiry (Invariant I2)**: Two threads or retried requests simultaneously attempt to expire or cancel the same pending order (e.g. Expiry sweeper runs while user hits Cancel, or simulator sends duplicate failure).
   *Expected behavior:* Only the first transition releases stock (in Baseline: decrements `sold_quantity` once; in Optimized: Redis Lua `release` decrements user key and increments stock once). The second call detects terminal state and returns `idempotent_replay: true` without releasing stock twice.
   *Pinned test:* `TestOptimized_DuplicateRelease_IdempotentNoDoubleRestock` in Task 3.
3. **Invalid State Transition from Terminal States (Invariant I4)**: A payment confirmation arrives for an order that is already in `payment_failed` or `cancelled`, or an expiry/cancellation request arrives for an order that is already `paid`.
   *Expected behavior:* Reject invalid transition with HTTP 409 Conflict (`invalid_state_transition`), strictly preventing `paid -> expired` or `payment_failed -> paid`, with zero modification to inventory.
   *Pinned test:* `TestStateMachine_InvalidTransitions_Rejected` in Task 1.
4. **PostgreSQL ENUM vs Schema Type Mismatch on Payment Status**: PostgreSQL `payment_status` ENUM only permits `'pending'`, `'success'`, `'failed'`. Attempting to update `payments.status` to `'refund'` or `'cancelled'` directly causes a Postgres 22P02 invalid input value error.
   *Expected behavior:* State machine maps application states to valid PostgreSQL ENUM values (`failed` for payment status, with `refund` logged in `webhook_logs.status` which is `VARCHAR(50)`).
   *Pinned test:* `TestBaseline_PaymentStatusEnumCompliance` in Task 2.
5. **Optimized DB-Worker Lag on Immediate Payment Simulation**: Payment simulation request arrives immediately after order creation (`POST /api/v1/payments/simulate`) before the NATS JetStream worker has finished writing the `orders` row to PostgreSQL.
   *Expected behavior:* Service performs a bounded retry (e.g. up to 1 second with 50ms backoff) querying for the order record before giving up with 404, preventing spurious 404 Not Found under heavy async queue load.
   *Pinned test:* `TestOptimized_WorkerPersistenceLag_RetrySucceeds` in Task 3.

---

### Task 0: Pre-flight Verification & Shared Domain Types

**Files:**
- Modify: `internal/domain/order.go:20-24`
- Create: `internal/domain/payment.go`
- Test: `internal/domain/payment_test.go`

**Interfaces:**
- Consumes: `internal/domain`
- Produces: `SimulatePaymentRequest`, `PaymentResultResponse`, outcome constants (`OutcomeSuccess`, `OutcomeFailure`, `OutcomeTimeout`, `OutcomeCancel`)

- [ ] **Step 1: Write the failing test for payment DTO serialization and validation**

Create `internal/domain/payment_test.go`:
```go
package domain

import (
	"encoding/json"
	"testing"
)

func TestPaymentDTOs_JSONSerialization(t *testing.T) {
	req := SimulatePaymentRequest{
		OrderID:      "11111111-1111-1111-1111-111111111111",
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-test-123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	var parsedReq SimulatePaymentRequest
	if err := json.Unmarshal(data, &parsedReq); err != nil {
		t.Fatalf("unmarshal request failed: %v", err)
	}
	if parsedReq.OrderID != req.OrderID || parsedReq.Outcome != req.Outcome || parsedReq.ProviderTxID != req.ProviderTxID {
		t.Fatalf("request mismatch: got %+v, want %+v", parsedReq, req)
	}

	resp := PaymentResultResponse{
		OrderID:          "11111111-1111-1111-1111-111111111111",
		OrderStatus:      "paid",
		PaymentStatus:    "success",
		IdempotentReplay: false,
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}

	var parsedResp PaymentResultResponse
	if err := json.Unmarshal(respData, &parsedResp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if parsedResp.OrderStatus != "paid" || parsedResp.PaymentStatus != "success" || parsedResp.IdempotentReplay != false {
		t.Fatalf("response mismatch: got %+v, want %+v", parsedResp, resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/domain -run TestPaymentDTOs_JSONSerialization`
Expected: FAIL (compilation error: `SimulatePaymentRequest`, `PaymentResultResponse`, `OutcomeSuccess` undefined).

- [ ] **Step 3: Implement payment domain types in `internal/domain/payment.go`**

Create `internal/domain/payment.go`:
```go
package domain

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeTimeout = "timeout"
	OutcomeCancel  = "cancel"
)

// SimulatePaymentRequest represents the body of POST /api/v1/payments/simulate.
type SimulatePaymentRequest struct {
	OrderID      string `json:"order_id"`
	Outcome      string `json:"outcome,omitempty"`
	ProviderTxID string `json:"provider_tx_id,omitempty"`
}

// PaymentResultResponse represents the contract defined in openapi.yaml.
type PaymentResultResponse struct {
	OrderID          string `json:"order_id"`
	OrderStatus      string `json:"order_status"`
	PaymentStatus    string `json:"payment_status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/domain -run TestPaymentDTOs_JSONSerialization`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/payment.go internal/domain/payment_test.go
git commit -m "feat(stage-5): add payment domain types and contract DTOs"
```

---

### Task 1: Payment State Machine & Transition Rules

**Files:**
- Create: `internal/payments/state_machine.go`
- Test: `internal/payments/state_machine_test.go`

**Interfaces:**
- Consumes: `internal/domain`
- Produces: `EvaluateTransition(currentStatus, outcome string) (TransitionDecision, error)`, order status constants, payment status constants, sentinel errors (`ErrInvalidStateTransition`, `ErrOrderExpiredRefund`).

- [ ] **Step 1: Write the failing unit tests for valid transitions, invalid transitions, replay, and late refund**

Create `internal/payments/state_machine_test.go`:
```go
package payments

import (
	"errors"
	"testing"
)

func TestStateMachine_ValidTransitions(t *testing.T) {
	tests := []struct {
		name              string
		currentStatus     string
		outcome           string
		wantOrderStatus   string
		wantPaymentStatus string
		wantRestock       bool
	}{
		{"pending -> success (paid)", OrderStatusPendingPayment, OutcomeSuccess, OrderStatusPaid, PaymentStatusSuccess, false},
		{"pending -> failure", OrderStatusPendingPayment, OutcomeFailure, OrderStatusPaymentFailed, PaymentStatusFailed, true},
		{"pending -> timeout (expired)", OrderStatusPendingPayment, OutcomeTimeout, OrderStatusExpired, PaymentStatusFailed, true},
		{"pending -> cancel", OrderStatusPendingPayment, OutcomeCancel, OrderStatusCancelled, PaymentStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := EvaluateTransition(tt.currentStatus, tt.outcome)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.TargetOrderStatus != tt.wantOrderStatus {
				t.Errorf("TargetOrderStatus = %q, want %q", decision.TargetOrderStatus, tt.wantOrderStatus)
			}
			if decision.TargetPaymentStatus != tt.wantPaymentStatus {
				t.Errorf("TargetPaymentStatus = %q, want %q", decision.TargetPaymentStatus, tt.wantPaymentStatus)
			}
			if decision.ShouldRestock != tt.wantRestock {
				t.Errorf("ShouldRestock = %v, want %v", decision.ShouldRestock, tt.wantRestock)
			}
			if decision.IsIdempotentReplay {
				t.Errorf("IsIdempotentReplay expected false")
			}
		})
	}
}

func TestStateMachine_IdempotentReplay(t *testing.T) {
	tests := []struct {
		currentStatus string
		outcome       string
	}{
		{OrderStatusPaid, OutcomeSuccess},
		{OrderStatusPaymentFailed, OutcomeFailure},
		{OrderStatusExpired, OutcomeTimeout},
		{OrderStatusCancelled, OutcomeCancel},
	}

	for _, tt := range tests {
		t.Run(tt.currentStatus+" with same outcome", func(t *testing.T) {
			decision, err := EvaluateTransition(tt.currentStatus, tt.outcome)
			if err != nil {
				t.Fatalf("unexpected error on replay: %v", err)
			}
			if !decision.IsIdempotentReplay {
				t.Errorf("expected IsIdempotentReplay = true")
			}
			if decision.ShouldRestock {
				t.Errorf("expected ShouldRestock = false on replay")
			}
		})
	}
}

func TestStateMachine_LatePayment_RecordedAsRefund(t *testing.T) {
	// D0.7: Late payment arriving when order is already expired
	decision, err := EvaluateTransition(OrderStatusExpired, OutcomeSuccess)
	if !errors.Is(err, ErrOrderExpiredRefund) {
		t.Fatalf("expected ErrOrderExpiredRefund, got %v", err)
	}
	if decision.TargetOrderStatus != OrderStatusExpired {
		t.Errorf("TargetOrderStatus must remain expired, got %q", decision.TargetOrderStatus)
	}
	if decision.TargetPaymentStatus != "refund" {
		t.Errorf("TargetPaymentStatus must be refund, got %q", decision.TargetPaymentStatus)
	}
	if decision.ShouldRestock {
		t.Errorf("ShouldRestock must be false for late payment")
	}
}

func TestStateMachine_InvalidTransitions_Rejected(t *testing.T) {
	invalidCases := []struct {
		currentStatus string
		outcome       string
	}{
		{OrderStatusPaid, OutcomeFailure},
		{OrderStatusPaid, OutcomeTimeout},
		{OrderStatusPaid, OutcomeCancel},
		{OrderStatusPaymentFailed, OutcomeSuccess},
		{OrderStatusCancelled, OutcomeSuccess},
		{OrderStatusCancelled, OutcomeTimeout},
	}

	for _, tc := range invalidCases {
		t.Run(tc.currentStatus+" -> "+tc.outcome, func(t *testing.T) {
			_, err := EvaluateTransition(tc.currentStatus, tc.outcome)
			if !errors.Is(err, ErrInvalidStateTransition) {
				t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/payments -run TestStateMachine_`
Expected: FAIL (package `payments` does not exist or symbols undefined).

- [ ] **Step 3: Implement `internal/payments/state_machine.go`**

Create `internal/payments/state_machine.go`:
```go
package payments

import (
	"apexstock/internal/domain"
	"errors"
	"fmt"
)

const (
	OrderStatusPendingPayment = "pending_payment"
	OrderStatusPaid           = "paid"
	OrderStatusPaymentFailed  = "payment_failed"
	OrderStatusExpired        = "expired"
	OrderStatusCancelled      = "cancelled"
)

const (
	PaymentStatusPending = "pending"
	PaymentStatusSuccess = "success"
	PaymentStatusFailed  = "failed"
)

const (
	OutcomeSuccess = domain.OutcomeSuccess
	OutcomeFailure = domain.OutcomeFailure
	OutcomeTimeout = domain.OutcomeTimeout
	OutcomeCancel  = domain.OutcomeCancel
)

var (
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrOrderExpiredRefund     = errors.New("order expired; payment marked for refund")
	ErrOrderNotFound          = errors.New("order not found")
)

type TransitionDecision struct {
	TargetOrderStatus   string
	TargetPaymentStatus string
	ShouldRestock       bool
	IsIdempotentReplay  bool
}

// EvaluateTransition determines the target state and restock action based on current state and event outcome.
func EvaluateTransition(currentOrderStatus, outcome string) (TransitionDecision, error) {
	switch currentOrderStatus {
	case OrderStatusPendingPayment:
		switch outcome {
		case OutcomeSuccess:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaid,
				TargetPaymentStatus: PaymentStatusSuccess,
				ShouldRestock:       false,
				IsIdempotentReplay:  false,
			}, nil
		case OutcomeFailure:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaymentFailed,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       true,
				IsIdempotentReplay:  false,
			}, nil
		case OutcomeTimeout:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusExpired,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       true,
				IsIdempotentReplay:  false,
			}, nil
		case OutcomeCancel:
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusCancelled,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       true,
				IsIdempotentReplay:  false,
			}, nil
		default:
			return TransitionDecision{}, fmt.Errorf("%w: unknown outcome %q", ErrInvalidStateTransition, outcome)
		}

	case OrderStatusPaid:
		if outcome == OutcomeSuccess {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaid,
				TargetPaymentStatus: PaymentStatusSuccess,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		return TransitionDecision{}, fmt.Errorf("%w: cannot transition paid order via %q", ErrInvalidStateTransition, outcome)

	case OrderStatusExpired:
		if outcome == OutcomeTimeout {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusExpired,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		if outcome == OutcomeSuccess {
			// Decision Gate D0.7: late payment on expired order
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusExpired,
				TargetPaymentStatus: "refund",
				ShouldRestock:       false,
				IsIdempotentReplay:  false,
			}, ErrOrderExpiredRefund
		}
		return TransitionDecision{}, fmt.Errorf("%w: expired order cannot transition via %q", ErrInvalidStateTransition, outcome)

	case OrderStatusPaymentFailed:
		if outcome == OutcomeFailure {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusPaymentFailed,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		return TransitionDecision{}, fmt.Errorf("%w: failed order cannot transition via %q", ErrInvalidStateTransition, outcome)

	case OrderStatusCancelled:
		if outcome == OutcomeCancel {
			return TransitionDecision{
				TargetOrderStatus:   OrderStatusCancelled,
				TargetPaymentStatus: PaymentStatusFailed,
				ShouldRestock:       false,
				IsIdempotentReplay:  true,
			}, nil
		}
		return TransitionDecision{}, fmt.Errorf("%w: cancelled order cannot transition via %q", ErrInvalidStateTransition, outcome)

	default:
		return TransitionDecision{}, fmt.Errorf("%w: unrecognized current order status %q", ErrInvalidStateTransition, currentOrderStatus)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/payments -run TestStateMachine_`
Expected: PASS (all unit tests passing).

- [ ] **Step 5: Commit**

```bash
git add internal/payments/state_machine.go internal/payments/state_machine_test.go
git commit -m "feat(stage-5): implement payment state machine and D0.7 refund transition"
```

---

### Task 2: Baseline Payment Service & Atomic Restock

**Files:**
- Create: `internal/payments/service.go`
- Create: `internal/payments/baseline.go`
- Create: `internal/payments/baseline_test.go`

**Interfaces:**
- Consumes: `*sql.DB`, `state_machine.go`
- Produces:
  ```go
  type PaymentProcessor interface {
      Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error)
  }
  type BaselineService struct { db *sql.DB }
  func NewBaselineService(db *sql.DB) *BaselineService
  ```

- [ ] **Step 1: Write failing integration test for Baseline payment confirm, restock, and late refund**

Create `internal/payments/baseline_test.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func getBaselineTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping baseline test: cannot open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping baseline test: DB not reachable: %v", err)
	}
	return db
}

func setupTestBaselineOrder(t *testing.T, db *sql.DB, status string) (orderID, itemID string, totalAmount int64) {
	ctx := context.Background()
	// Fetch an active flash sale item
	err := db.QueryRowContext(ctx, `SELECT id, sale_price FROM flash_sale_items LIMIT 1`).Scan(&itemID, &totalAmount)
	if err != nil {
		t.Fatalf("failed to query flash_sale_item: %v", err)
	}

	var userID string
	err = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to query user: %v", err)
	}

	idempotencyKey := "test-idemp-" + time.Now().Format("20060102150405.000000000")
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if status == OrderStatusExpired {
		expiresAt = time.Now().UTC().Add(-5 * time.Minute)
	}

	err = db.QueryRowContext(ctx, `
		INSERT INTO orders (user_id, idempotency_key, status, total_amount, expires_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, idempotencyKey, status, totalAmount, expiresAt).Scan(&orderID)
	if err != nil {
		t.Fatalf("failed to insert test order: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO order_items (order_id, flash_sale_item_id, product_id, quantity, unit_price)
		SELECT $1, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $2`, orderID, itemID)
	if err != nil {
		t.Fatalf("failed to insert order_item: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO payments (order_id, method, amount, status)
		VALUES ($1, 'simulated', $2, 'pending')`, orderID, totalAmount)
	if err != nil {
		t.Fatalf("failed to insert payment: %v", err)
	}

	// In baseline, creating an order increments sold_quantity
	_, err = db.ExecContext(ctx, `UPDATE flash_sale_items SET sold_quantity = sold_quantity + 1 WHERE id = $1`, itemID)
	if err != nil {
		t.Fatalf("failed to increment sold_quantity: %v", err)
	}

	return orderID, itemID, totalAmount
}

func TestBaseline_ConfirmPayment_MarksPaidAndKeepsStock(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, itemID, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	var initialSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialSold)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-success-1",
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process confirm failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaid || res.PaymentStatus != PaymentStatusSuccess {
		t.Fatalf("unexpected result: %+v", res)
	}

	var afterSold int32
	var orderStatus string
	var paymentStatus string
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&afterSold)
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)
	_ = db.QueryRow(`SELECT status FROM payments WHERE order_id = $1`, orderID).Scan(&paymentStatus)

	if orderStatus != OrderStatusPaid {
		t.Errorf("order status in DB = %q, want paid", orderStatus)
	}
	if paymentStatus != PaymentStatusSuccess {
		t.Errorf("payment status in DB = %q, want success", paymentStatus)
	}
	if afterSold != initialSold {
		t.Errorf("baseline sold_quantity changed on paid: got %d, want %d", afterSold, initialSold)
	}
}

func TestBaseline_FailPayment_RestocksSoldQuantity(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, itemID, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	var initialSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialSold)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeFailure,
		ProviderTxID: "tx-fail-1",
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process fail failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaymentFailed || !res.Restocked {
		t.Fatalf("unexpected result: %+v", res)
	}

	var afterSold int32
	var orderStatus string
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&afterSold)
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus)

	if orderStatus != OrderStatusPaymentFailed {
		t.Errorf("order status = %q, want payment_failed", orderStatus)
	}
	if afterSold != initialSold-1 {
		t.Errorf("baseline stock not decremented: got %d, want %d", afterSold, initialSold-1)
	}
}

func TestBaseline_LatePayment_RecordsWebhookRefundLog(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusExpired)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-late-1",
		Provider:     "simulator",
	})
	if !errors.Is(err, ErrOrderExpiredRefund) {
		t.Fatalf("expected ErrOrderExpiredRefund, got %v", err)
	}
	if res.OrderStatus != OrderStatusExpired || res.PaymentStatus != "refund" {
		t.Fatalf("expected status expired / refund, got %+v", res)
	}

	var webhookLogCount int
	var loggedStatus string
	err = db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(status), '') FROM webhook_logs WHERE order_id = $1`, orderID).Scan(&webhookLogCount, &loggedStatus)
	if err != nil {
		t.Fatalf("query webhook_logs failed: %v", err)
	}
	if webhookLogCount != 1 || loggedStatus != "refund" {
		t.Fatalf("webhook_logs expected 1 row with status 'refund', got %d rows with status %q", webhookLogCount, loggedStatus)
	}
}

func TestBaseline_PaymentStatusEnumCompliance(t *testing.T) {
	db := getBaselineTestDB(t)
	defer db.Close()
	svc := NewBaselineService(db)

	orderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	// Verify that failing payment sets payments.status to 'failed' (valid PostgreSQL ENUM value)
	_, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:  orderID,
		Outcome:  OutcomeFailure,
		Provider: "simulator",
	})
	if err != nil {
		t.Fatalf("failed to process failure outcome: %v", err)
	}

	var paymentStatus string
	err = db.QueryRow(`SELECT status::text FROM payments WHERE order_id = $1`, orderID).Scan(&paymentStatus)
	if err != nil {
		t.Fatalf("query payments failed: %v", err)
	}
	if paymentStatus != "failed" {
		t.Errorf("payment_status in DB = %q, want failed", paymentStatus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/payments -run TestBaseline_`
Expected: FAIL (compilation error: `NewBaselineService` undefined).

- [ ] **Step 3: Implement `internal/payments/service.go` and `internal/payments/baseline.go`**

Create `internal/payments/service.go`:
```go
package payments

import "context"

type ProcessPaymentRequest struct {
	OrderID      string
	Outcome      string
	ProviderTxID string
	Provider     string
}

type PaymentResult struct {
	OrderID          string
	OrderStatus      string
	PaymentStatus    string
	IdempotentReplay bool
	Restocked        bool
}

type PaymentProcessor interface {
	Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error)
}
```

Create `internal/payments/baseline.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type BaselineService struct {
	db *sql.DB
}

func NewBaselineService(db *sql.DB) *BaselineService {
	return &BaselineService{db: db}
}

func (s *BaselineService) Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error) {
	if req.Provider == "" {
		req.Provider = "simulator"
	}
	if req.ProviderTxID == "" {
		req.ProviderTxID = fmt.Sprintf("sim-%s-%d", req.OrderID[:8], time.Now().UnixNano())
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PaymentResult{}, err
	}
	defer tx.Rollback()

	var orderStatus string
	var itemID string
	var quantity int32
	var currentPaymentStatus string

	err = tx.QueryRowContext(ctx, `
		SELECT o.status, oi.flash_sale_item_id, oi.quantity, p.status
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN payments p ON p.order_id = o.id
		WHERE o.id = $1
		FOR UPDATE OF o, p`, req.OrderID).Scan(&orderStatus, &itemID, &quantity, &currentPaymentStatus)

	if errors.Is(err, sql.ErrNoRows) {
		return PaymentResult{}, ErrOrderNotFound
	}
	if err != nil {
		return PaymentResult{}, err
	}

	decision, evalErr := EvaluateTransition(orderStatus, req.Outcome)

	// Decision Gate D0.7: late payment on expired order
	if errors.Is(evalErr, ErrOrderExpiredRefund) {
		payload, _ := json.Marshal(map[string]any{
			"order_id":        req.OrderID,
			"outcome":         req.Outcome,
			"provider_tx_id":  req.ProviderTxID,
			"reason":          "payment arrived after order expired",
			"received_at":     time.Now().UTC(),
		})
		_, logErr := tx.ExecContext(ctx, `
			INSERT INTO webhook_logs (order_id, provider, transaction_ref, status, payload)
			VALUES ($1, $2, $3, 'refund', $4)`,
			req.OrderID, req.Provider, req.ProviderTxID, payload)
		if logErr != nil {
			return PaymentResult{}, logErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return PaymentResult{}, commitErr
		}
		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      OrderStatusExpired,
			PaymentStatus:    "refund",
			IdempotentReplay: false,
			Restocked:        false,
		}, ErrOrderExpiredRefund
	}

	if evalErr != nil {
		return PaymentResult{}, evalErr
	}

	// Idempotent replay: return current status without mutation
	if decision.IsIdempotentReplay {
		_ = tx.Commit()
		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      orderStatus,
			PaymentStatus:    currentPaymentStatus,
			IdempotentReplay: true,
			Restocked:        false,
		}, nil
	}

	// Update order
	_, err = tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`,
		decision.TargetOrderStatus, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	// Update payment record
	_, err = tx.ExecContext(ctx, `
		UPDATE payments SET status = $1, transaction_ref = $2, updated_at = now() WHERE order_id = $3`,
		decision.TargetPaymentStatus, req.ProviderTxID, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	// Restock if transitioning to payment_failed, expired, or cancelled
	if decision.ShouldRestock {
		_, err = tx.ExecContext(ctx, `
			UPDATE flash_sale_items SET sold_quantity = sold_quantity - $1 WHERE id = $2`,
			quantity, itemID)
		if err != nil {
			return PaymentResult{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return PaymentResult{}, err
	}

	return PaymentResult{
		OrderID:          req.OrderID,
		OrderStatus:      decision.TargetOrderStatus,
		PaymentStatus:    decision.TargetPaymentStatus,
		IdempotentReplay: false,
		Restocked:        decision.ShouldRestock,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/payments -run TestBaseline_`
Expected: PASS (or SKIP if local Postgres is off, but compilation and unit logic succeed).

- [ ] **Step 5: Commit**

```bash
git add internal/payments/service.go internal/payments/baseline.go internal/payments/baseline_test.go
git commit -m "feat(stage-5): implement baseline payment service with atomic inventory restock"
```

---

### Task 3: Optimized Payment Service with Redis Lua Restock & Confirm

**Files:**
- Create: `internal/payments/optimized.go`
- Test: `internal/payments/optimized_test.go`

**Interfaces:**
- Consumes: `*sql.DB`, `*redis.Client`, `state_machine.go`, `internal/persistence/redis/keys.go`
- Produces: `NewOptimizedService(db *sql.DB, rdb *redis.Client) *OptimizedService`, implementing `PaymentProcessor`

- [ ] **Step 1: Write failing integration test for Optimized payment confirm, release, idempotency, and retry**

Create `internal/payments/optimized_test.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	rediskeys "apexstock/internal/persistence/redis"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func getOptimizedTestInfra(t *testing.T) (*sql.DB, *redis.Client) {
	dsn := os.Getenv("OPTIMIZED_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_optimized?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping optimized test: cannot open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping optimized test: DB unreachable: %v", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("skipping optimized test: Redis unreachable: %v", err)
	}
	return db, rdb
}

func setupTestOptimizedOrder(t *testing.T, db *sql.DB, rdb *redis.Client, status string) (orderID, reservationID, itemID, userID string) {
	ctx := context.Background()
	_ = db.QueryRowContext(ctx, `SELECT id FROM flash_sale_items LIMIT 1`).Scan(&itemID)
	_ = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)

	reservationID = "res-" + time.Now().Format("20060102150405.000000000")
	orderID = "ord-" + time.Now().Format("20060102150405.000000000")
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if status == OrderStatusExpired {
		expiresAt = time.Now().UTC().Add(-5 * time.Minute)
	}

	// Insert in DB
	_, err := db.ExecContext(ctx, `
		INSERT INTO orders (id, user_id, idempotency_key, status, total_amount, expires_at)
		VALUES ($1, $2, $1, $3, 100000, $4)`, orderID, userID, status, expiresAt)
	if err != nil {
		t.Fatalf("failed to insert order: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO order_items (id, order_id, flash_sale_item_id, product_id, quantity, unit_price)
		SELECT $1, $2, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $3`, reservationID, orderID, itemID)
	if err != nil {
		t.Fatalf("failed to insert order_item: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO payments (order_id, method, amount, status)
		VALUES ($1, 'simulated', 100000, 'pending')`, orderID)
	if err != nil {
		t.Fatalf("failed to insert payment: %v", err)
	}

	// Set up Redis keys for reservation
	rdb.Set(ctx, rediskeys.StockKey(itemID), 100, 0)
	rdb.Set(ctx, rediskeys.UserKey(itemID, userID), 1, 0)
	rdb.ZAdd(ctx, rediskeys.ReservedKey(itemID), redis.Z{Score: float64(expiresAt.Unix()), Member: reservationID})
	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, reservationID),
		"reservation_id", reservationID,
		"order_id", orderID,
		"item_id", itemID,
		"user_id", userID,
		"qty", "1",
		"state", "reserved",
	)

	return orderID, reservationID, itemID, userID
}

func TestOptimized_ConfirmPayment_CallsLuaConfirmAndIncrementsDBSold(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	orderID, reservationID, itemID, _ := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)

	var initialDBSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialDBSold)

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-opt-confirm-1",
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process confirm failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaid || res.PaymentStatus != PaymentStatusSuccess {
		t.Fatalf("unexpected res: %+v", res)
	}

	// Verify Redis reservation hash state
	state, _ := rdb.HGet(context.Background(), rediskeys.ReservationKey(itemID, reservationID), "state").Result()
	if state != "confirmed" {
		t.Errorf("Redis reservation state = %q, want confirmed", state)
	}

	// Verify DB sold_quantity incremented by 1 (per D0.3)
	var afterDBSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&afterDBSold)
	if afterDBSold != initialDBSold+1 {
		t.Errorf("DB sold_quantity = %d, want %d", afterDBSold, initialDBSold+1)
	}
}

func TestOptimized_FailPayment_ReleasesRedisStockAndUserQuota(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	orderID, reservationID, itemID, userID := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)

	initialStock, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()
	initialUser, _ := rdb.Get(context.Background(), rediskeys.UserKey(itemID, userID)).Int64()

	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeFailure,
		ProviderTxID: "tx-opt-fail-1",
		Provider:     "simulator",
	})
	if err != nil {
		t.Fatalf("process fail failed: %v", err)
	}
	if res.OrderStatus != OrderStatusPaymentFailed || !res.Restocked {
		t.Fatalf("unexpected res: %+v", res)
	}

	// Verify Redis stock incremented by 1
	afterStock, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()
	if afterStock != initialStock+1 {
		t.Errorf("Redis stock = %d, want %d", afterStock, initialStock+1)
	}

	// Verify user counter decremented by 1
	afterUser, _ := rdb.Get(context.Background(), rediskeys.UserKey(itemID, userID)).Int64()
	if afterUser != initialUser-1 {
		t.Errorf("Redis user counter = %d, want %d", afterUser, initialUser-1)
	}

	// Verify reservation state in Redis
	state, _ := rdb.HGet(context.Background(), rediskeys.ReservationKey(itemID, reservationID), "state").Result()
	if state != "released" {
		t.Errorf("Redis reservation state = %q, want released", state)
	}
}

func TestOptimized_DuplicateRelease_IdempotentNoDoubleRestock(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	orderID, _, itemID, _ := setupTestOptimizedOrder(t, db, rdb, OrderStatusPendingPayment)

	// First release
	_, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID: orderID,
		Outcome: OutcomeTimeout,
	})
	if err != nil {
		t.Fatalf("first timeout failed: %v", err)
	}

	stockAfterFirst, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()

	// Second release (duplicate)
	res2, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID: orderID,
		Outcome: OutcomeTimeout,
	})
	if err != nil {
		t.Fatalf("second timeout failed: %v", err)
	}
	if !res2.IdempotentReplay {
		t.Errorf("expected IdempotentReplay = true on second release")
	}

	stockAfterSecond, _ := rdb.Get(context.Background(), rediskeys.StockKey(itemID)).Int64()
	if stockAfterSecond != stockAfterFirst {
		t.Errorf("stock changed on duplicate release: first=%d, second=%d", stockAfterFirst, stockAfterSecond)
	}
}

func TestOptimized_WorkerPersistenceLag_RetrySucceeds(t *testing.T) {
	db, rdb := getOptimizedTestInfra(t)
	defer db.Close()
	defer rdb.Close()
	svc := NewOptimizedService(db, rdb)

	ctx := context.Background()
	var itemID, userID string
	_ = db.QueryRowContext(ctx, `SELECT id FROM flash_sale_items LIMIT 1`).Scan(&itemID)
	_ = db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID)

	orderID := "ord-lag-" + time.Now().Format("20060102150405.000000000")
	reservationID := "res-lag-" + time.Now().Format("20060102150405.000000000")
	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	// Simulate reservation in Redis immediately
	rdb.Set(ctx, rediskeys.StockKey(itemID), 100, 0)
	rdb.Set(ctx, rediskeys.UserKey(itemID, userID), 1, 0)
	rdb.ZAdd(ctx, rediskeys.ReservedKey(itemID), redis.Z{Score: float64(expiresAt.Unix()), Member: reservationID})
	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, reservationID),
		"reservation_id", reservationID,
		"order_id", orderID,
		"item_id", itemID,
		"user_id", userID,
		"qty", "1",
		"state", "reserved",
	)

	// Delay DB order persistence by 120ms to simulate asynchronous worker lag
	go func() {
		time.Sleep(120 * time.Millisecond)
		_, _ = db.ExecContext(context.Background(), `
			INSERT INTO orders (id, user_id, idempotency_key, status, total_amount, expires_at)
			VALUES ($1, $2, $1, 'pending_payment', 100000, $3)`, orderID, userID, expiresAt)
		_, _ = db.ExecContext(context.Background(), `
			INSERT INTO order_items (id, order_id, flash_sale_item_id, product_id, quantity, unit_price)
			SELECT $1, $2, id, product_id, 1, sale_price FROM flash_sale_items WHERE id = $3`, reservationID, orderID, itemID)
		_, _ = db.ExecContext(context.Background(), `
			INSERT INTO payments (order_id, method, amount, status)
			VALUES ($1, 'simulated', 100000, 'pending')`, orderID)
	}()

	// Payment process should retry and succeed once row is inserted
	res, err := svc.Process(context.Background(), ProcessPaymentRequest{
		OrderID:      orderID,
		Outcome:      OutcomeSuccess,
		ProviderTxID: "tx-lag-success",
	})
	if err != nil {
		t.Fatalf("expected bounded retry to succeed despite worker lag, got: %v", err)
	}
	if res.OrderStatus != OrderStatusPaid {
		t.Errorf("order status = %q, want paid", res.OrderStatus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/payments -run TestOptimized_`
Expected: FAIL (compilation error: `NewOptimizedService` undefined).

- [ ] **Step 3: Implement `internal/payments/optimized.go`**

Create `internal/payments/optimized.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

const confirmLua = `local s=redis.call('HGET',KEYS[2],'state'); if s=='confirmed' then return {1,'confirmed'} end; if s~='reserved' then return {1,s or 'missing'} end; redis.call('ZREM',KEYS[1],ARGV[1]); redis.call('HSET',KEYS[2],'state','confirmed'); return {0,'confirmed'}`
const releaseLua = `local s=redis.call('HGET',KEYS[4],'state'); if s==false or s=='released' or s=='confirmed' then return {1,s or 'missing'} end; local q=tonumber(redis.call('HGET',KEYS[4],'qty') or '0'); redis.call('INCRBY',KEYS[1],q); redis.call('DECRBY',KEYS[3],q); redis.call('ZREM',KEYS[2],ARGV[1]); redis.call('HSET',KEYS[4],'state','released'); return {0,'released',q}`

type OptimizedService struct {
	db  *sql.DB
	rdb *redis.Client
}

func NewOptimizedService(db *sql.DB, rdb *redis.Client) *OptimizedService {
	return &OptimizedService{db: db, rdb: rdb}
}

func (s *OptimizedService) Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error) {
	if req.Provider == "" {
		req.Provider = "simulator"
	}
	if req.ProviderTxID == "" {
		req.ProviderTxID = fmt.Sprintf("sim-%s-%d", req.OrderID[:8], time.Now().UnixNano())
	}

	var orderStatus string
	var itemID string
	var reservationID string
	var userID string
	var quantity int32
	var currentPaymentStatus string

	// Bounded retry to allow JetStream worker to persist order if payment arrives immediately
	var queryErr error
	for attempt := 0; attempt < 5; attempt++ {
		queryErr = s.db.QueryRowContext(ctx, `
			SELECT o.status, o.user_id, oi.id, oi.flash_sale_item_id, oi.quantity, p.status
			FROM orders o
			JOIN order_items oi ON oi.order_id = o.id
			JOIN payments p ON p.order_id = o.id
			WHERE o.id = $1`, req.OrderID).Scan(&orderStatus, &userID, &reservationID, &itemID, &quantity, &currentPaymentStatus)
		if queryErr == nil {
			break
		}
		if !errors.Is(queryErr, sql.ErrNoRows) {
			return PaymentResult{}, queryErr
		}
		select {
		case <-ctx.Done():
			return PaymentResult{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if errors.Is(queryErr, sql.ErrNoRows) {
		return PaymentResult{}, ErrOrderNotFound
	}

	decision, evalErr := EvaluateTransition(orderStatus, req.Outcome)

	// Decision Gate D0.7: late payment on expired order
	if errors.Is(evalErr, ErrOrderExpiredRefund) {
		payload, _ := json.Marshal(map[string]any{
			"order_id":       req.OrderID,
			"outcome":        req.Outcome,
			"provider_tx_id": req.ProviderTxID,
			"reason":         "late payment on expired order",
			"received_at":    time.Now().UTC(),
		})
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO webhook_logs (order_id, provider, transaction_ref, status, payload)
			VALUES ($1, $2, $3, 'refund', $4)`,
			req.OrderID, req.Provider, req.ProviderTxID, payload)

		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      OrderStatusExpired,
			PaymentStatus:    "refund",
			IdempotentReplay: false,
			Restocked:        false,
		}, ErrOrderExpiredRefund
	}

	if evalErr != nil {
		return PaymentResult{}, evalErr
	}

	if decision.IsIdempotentReplay {
		return PaymentResult{
			OrderID:          req.OrderID,
			OrderStatus:      orderStatus,
			PaymentStatus:    currentPaymentStatus,
			IdempotentReplay: true,
			Restocked:        false,
		}, nil
	}

	// Apply Redis state transition
	if decision.TargetOrderStatus == OrderStatusPaid {
		_, err := s.rdb.Eval(ctx, confirmLua, []string{
			rediskeys.ReservedKey(itemID),
			rediskeys.ReservationKey(itemID, reservationID),
		}, reservationID).Result()
		if err != nil {
			return PaymentResult{}, fmt.Errorf("redis confirm lua: %w", err)
		}
	} else if decision.ShouldRestock {
		_, err := s.rdb.Eval(ctx, releaseLua, []string{
			rediskeys.StockKey(itemID),
			rediskeys.ReservedKey(itemID),
			rediskeys.UserKey(itemID, userID),
			rediskeys.ReservationKey(itemID, reservationID),
		}, reservationID).Result()
		if err != nil {
			return PaymentResult{}, fmt.Errorf("redis release lua: %w", err)
		}
	}

	// Update DB atomically
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PaymentResult{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`,
		decision.TargetOrderStatus, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE payments SET status = $1, transaction_ref = $2, updated_at = now() WHERE order_id = $3`,
		decision.TargetPaymentStatus, req.ProviderTxID, req.OrderID)
	if err != nil {
		return PaymentResult{}, err
	}

	// Decision Gate D0.3: in Optimized, sold_quantity only increments in DB when paid
	if decision.TargetOrderStatus == OrderStatusPaid {
		_, err = tx.ExecContext(ctx, `
			UPDATE flash_sale_items SET sold_quantity = sold_quantity + $1 WHERE id = $2`,
			quantity, itemID)
		if err != nil {
			return PaymentResult{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return PaymentResult{}, err
	}

	return PaymentResult{
		OrderID:          req.OrderID,
		OrderStatus:      decision.TargetOrderStatus,
		PaymentStatus:    decision.TargetPaymentStatus,
		IdempotentReplay: false,
		Restocked:        decision.ShouldRestock,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/payments -run TestOptimized_`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payments/optimized.go internal/payments/optimized_test.go
git commit -m "feat(stage-5): implement optimized payment service with Redis Lua restock and confirm"
```

---

### Task 4: Payment Simulator Engine & Configurable Ratios

**Files:**
- Create: `internal/payments/simulator.go`
- Test: `internal/payments/simulator_test.go`

**Interfaces:**
- Consumes: `PaymentProcessor`, `domain.SimulatePaymentRequest`
- Produces: `SimulatorConfig`, `NewSimulator(processor PaymentProcessor, cfg SimulatorConfig) *Simulator`, `Simulate(ctx context.Context, req domain.SimulatePaymentRequest) (PaymentResult, error)`

- [ ] **Step 1: Write failing unit tests for deterministic overrides and ratio distribution**

Create `internal/payments/simulator_test.go`:
```go
package payments

import (
	"context"
	"math"
	"testing"

	"apexstock/internal/domain"
)

type mockProcessor struct {
	lastReq ProcessPaymentRequest
}

func (m *mockProcessor) Process(ctx context.Context, req ProcessPaymentRequest) (PaymentResult, error) {
	m.lastReq = req
	return PaymentResult{
		OrderID:       req.OrderID,
		OrderStatus:   req.Outcome,
		PaymentStatus: req.Outcome,
	}, nil
}

func TestSimulator_DeterministicOverride(t *testing.T) {
	mock := &mockProcessor{}
	sim := NewSimulator(mock, DefaultSimulatorConfig())

	req := domain.SimulatePaymentRequest{
		OrderID:      "test-order-1",
		Outcome:      OutcomeCancel,
		ProviderTxID: "manual-tx-99",
	}

	res, err := sim.Simulate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastReq.Outcome != OutcomeCancel {
		t.Errorf("expected outcome %q, got %q", OutcomeCancel, mock.lastReq.Outcome)
	}
	if mock.lastReq.ProviderTxID != "manual-tx-99" {
		t.Errorf("expected tx id 'manual-tx-99', got %q", mock.lastReq.ProviderTxID)
	}
	if res.OrderID != "test-order-1" {
		t.Errorf("expected orderID test-order-1, got %q", res.OrderID)
	}
}

func TestSimulator_RatioDistribution(t *testing.T) {
	mock := &mockProcessor{}
	cfg := SimulatorConfig{
		SuccessRatio: 0.70,
		FailureRatio: 0.20,
		TimeoutRatio: 0.10,
		CancelRatio:  0.00,
	}
	sim := NewSimulator(mock, cfg)

	counts := make(map[string]int)
	const total = 5000

	for i := 0; i < total; i++ {
		_, _ = sim.Simulate(context.Background(), domain.SimulatePaymentRequest{
			OrderID: "order-multi",
		})
		counts[mock.lastReq.Outcome]++
	}

	successRate := float64(counts[OutcomeSuccess]) / float64(total)
	failureRate := float64(counts[OutcomeFailure]) / float64(total)
	timeoutRate := float64(counts[OutcomeTimeout]) / float64(total)

	if math.Abs(successRate-0.70) > 0.04 {
		t.Errorf("success rate deviated too much: got %.4f, want ~0.70", successRate)
	}
	if math.Abs(failureRate-0.20) > 0.04 {
		t.Errorf("failure rate deviated too much: got %.4f, want ~0.20", failureRate)
	}
	if math.Abs(timeoutRate-0.10) > 0.04 {
		t.Errorf("timeout rate deviated too much: got %.4f, want ~0.10", timeoutRate)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/payments -run TestSimulator_`
Expected: FAIL (compilation error: `NewSimulator`, `DefaultSimulatorConfig` undefined).

- [ ] **Step 3: Implement `internal/payments/simulator.go`**

Create `internal/payments/simulator.go`:
```go
package payments

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"apexstock/internal/domain"
)

type SimulatorConfig struct {
	SuccessRatio float64
	FailureRatio float64
	TimeoutRatio float64
	CancelRatio  float64
}

func DefaultSimulatorConfig() SimulatorConfig {
	return SimulatorConfig{
		SuccessRatio: 0.85,
		FailureRatio: 0.10,
		TimeoutRatio: 0.05,
		CancelRatio:  0.00,
	}
}

type Simulator struct {
	processor PaymentProcessor
	cfg       SimulatorConfig
	rng       *rand.Rand
	mu        sync.Mutex
}

func NewSimulator(processor PaymentProcessor, cfg SimulatorConfig) *Simulator {
	// Normalize ratios if sum != 1.0
	sum := cfg.SuccessRatio + cfg.FailureRatio + cfg.TimeoutRatio + cfg.CancelRatio
	if sum > 0 {
		cfg.SuccessRatio /= sum
		cfg.FailureRatio /= sum
		cfg.TimeoutRatio /= sum
		cfg.CancelRatio /= sum
	} else {
		cfg = DefaultSimulatorConfig()
	}

	return &Simulator{
		processor: processor,
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Simulator) Simulate(ctx context.Context, req domain.SimulatePaymentRequest) (PaymentResult, error) {
	outcome := req.Outcome
	if outcome == "" {
		s.mu.Lock()
		r := s.rng.Float64()
		s.mu.Unlock()

		if r < s.cfg.SuccessRatio {
			outcome = OutcomeSuccess
		} else if r < s.cfg.SuccessRatio+s.cfg.FailureRatio {
			outcome = OutcomeFailure
		} else if r < s.cfg.SuccessRatio+s.cfg.FailureRatio+s.cfg.TimeoutRatio {
			outcome = OutcomeTimeout
		} else {
			outcome = OutcomeCancel
		}
	} else {
		// Validate deterministic outcome string
		switch outcome {
		case OutcomeSuccess, OutcomeFailure, OutcomeTimeout, OutcomeCancel:
		default:
			return PaymentResult{}, fmt.Errorf("%w: invalid outcome %q", ErrInvalidStateTransition, outcome)
		}
	}

	txID := req.ProviderTxID
	if txID == "" {
		txID = fmt.Sprintf("sim-%s-%d", req.OrderID, time.Now().UnixNano())
	}

	return s.processor.Process(ctx, ProcessPaymentRequest{
		OrderID:      req.OrderID,
		Outcome:      outcome,
		ProviderTxID: txID,
		Provider:     "simulator",
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/payments -run TestSimulator_`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payments/simulator.go internal/payments/simulator_test.go
git commit -m "feat(stage-5): implement payment simulator with configurable outcome ratios"
```

---

### Task 5: Expiry Sweeper & Scheduler Daemon

**Files:**
- Create: `internal/payments/expiry.go`
- Create: `cmd/scheduler/main.go`
- Test: `internal/payments/expiry_test.go`

**Interfaces:**
- Consumes: `*sql.DB`, `PaymentProcessor`
- Produces: `NewExpirySweeper(db *sql.DB, processor PaymentProcessor) *ExpirySweeper`, `SweepOnce(ctx context.Context, batchSize int) (int, error)`, `cmd/scheduler` binary

- [ ] **Step 1: Write failing unit test for `SweepOnce` targeting expired vs active orders**

Create `internal/payments/expiry_test.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestExpirySweeper_SweepsExpiredOnly(t *testing.T) {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skip("skipping expiry sweeper test: DB not reachable")
	}
	defer db.Close()

	svc := NewBaselineService(db)
	sweeper := NewExpirySweeper(db, svc)

	// Create an expired order and an active order
	expiredOrderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)
	_, _ = db.Exec(`UPDATE orders SET expires_at = now() - interval '10 seconds' WHERE id = $1`, expiredOrderID)

	activeOrderID, _, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)
	_, _ = db.Exec(`UPDATE orders SET expires_at = now() + interval '5 minutes' WHERE id = $1`, activeOrderID)

	count, err := sweeper.SweepOnce(context.Background(), 50)
	if err != nil {
		t.Fatalf("SweepOnce failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 swept order, got %d", count)
	}

	var expiredStatus string
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, expiredOrderID).Scan(&expiredStatus)
	if expiredStatus != OrderStatusExpired {
		t.Errorf("expired order status = %q, want expired", expiredStatus)
	}

	var activeStatus string
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, activeOrderID).Scan(&activeStatus)
	if activeStatus != OrderStatusPendingPayment {
		t.Errorf("active order status changed = %q, want pending_payment", activeStatus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/payments -run TestExpirySweeper_`
Expected: FAIL (compilation error: `NewExpirySweeper` undefined).

- [ ] **Step 3: Implement `internal/payments/expiry.go` and `cmd/scheduler/main.go`**

Create `internal/payments/expiry.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"log"
	"time"
)

type ExpirySweeper struct {
	db        *sql.DB
	processor PaymentProcessor
}

func NewExpirySweeper(db *sql.DB, processor PaymentProcessor) *ExpirySweeper {
	return &ExpirySweeper{
		db:        db,
		processor: processor,
	}
}

// SweepOnce queries expired pending orders using FOR UPDATE SKIP LOCKED and cancels them.
func (s *ExpirySweeper) SweepOnce(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM orders
		WHERE status = 'pending_payment' AND expires_at <= now()
		ORDER BY expires_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var orderIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			orderIDs = append(orderIDs, id)
		}
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}

	sweptCount := 0
	for _, id := range orderIDs {
		_, err := s.processor.Process(ctx, ProcessPaymentRequest{
			OrderID:  id,
			Outcome:  OutcomeTimeout,
			Provider: "scheduler",
		})
		if err == nil {
			sweptCount++
		}
	}

	return sweptCount, nil
}

// Start runs the sweeper on a periodic interval until ctx is cancelled.
func (s *ExpirySweeper) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.SweepOnce(ctx, 100)
			if err != nil {
				log.Printf("[scheduler] sweep error: %v", err)
			} else if count > 0 {
				log.Printf("[scheduler] swept %d expired orders", count)
			}
		}
	}
}
```

Create `cmd/scheduler/main.go`:
```go
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apexstock/internal/config"
	"apexstock/internal/payments"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	arch := os.Getenv("ARCH")
	if arch == "" {
		arch = "both"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup Baseline sweeper
	if arch == "baseline" || arch == "both" {
		dbBaseline, err := sql.Open("postgres", cfg.BaselineDSN)
		if err != nil {
			log.Fatalf("[scheduler] baseline db open failed: %v", err)
		}
		defer dbBaseline.Close()
		baselineSvc := payments.NewBaselineService(dbBaseline)
		baselineSweeper := payments.NewExpirySweeper(dbBaseline, baselineSvc)
		go baselineSweeper.Start(ctx, 1*time.Second)
		log.Println("[scheduler] Baseline expiry sweeper started")
	}

	// Setup Optimized sweeper
	if arch == "optimized" || arch == "both" {
		dbOptimized, err := sql.Open("postgres", cfg.OptimizedDSN)
		if err != nil {
			log.Fatalf("[scheduler] optimized db open failed: %v", err)
		}
		defer dbOptimized.Close()

		rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		defer rdb.Close()

		optSvc := payments.NewOptimizedService(dbOptimized, rdb)
		optSweeper := payments.NewExpirySweeper(dbOptimized, optSvc)
		go optSweeper.Start(ctx, 1*time.Second)
		log.Println("[scheduler] Optimized expiry sweeper started")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("[scheduler] Shutting down scheduler gracefully...")
	cancel()
	time.Sleep(500 * time.Millisecond)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/payments -run TestExpirySweeper_`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payments/expiry.go internal/payments/expiry_test.go cmd/scheduler/main.go
git commit -m "feat(stage-5): implement order expiry sweeper and scheduler daemon"
```

---

### Task 6: HTTP Handler & Simulation Endpoints Wiring

**Files:**
- Create: `internal/httpapi/payment_handler.go`
- Modify: `cmd/api-baseline/main.go:26-44`
- Modify: `cmd/api-optimized/main.go:32-40`
- Test: `internal/httpapi/payment_handler_test.go`

**Interfaces:**
- Consumes: `*payments.Simulator`, `internal/domain`
- Produces: `SimulatePaymentHandler(sim *payments.Simulator) http.Handler`, exposed on `POST /api/v1/payments/simulate` on both ports `:8081` and `:8082`.

- [ ] **Step 1: Write failing HTTP contract test for `POST /api/v1/payments/simulate`**

Create `internal/httpapi/payment_handler_test.go`:
```go
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apexstock/internal/domain"
	"apexstock/internal/payments"
)

type dummyProcessor struct {
	outcome payments.PaymentResult
	err     error
}

func (d *dummyProcessor) Process(ctx context.Context, req payments.ProcessPaymentRequest) (payments.PaymentResult, error) {
	if d.err != nil {
		return payments.PaymentResult{}, d.err
	}
	return d.outcome, nil
}

func TestSimulatePaymentHandler_Success(t *testing.T) {
	proc := &dummyProcessor{
		outcome: payments.PaymentResult{
			OrderID:          "11111111-1111-1111-1111-111111111111",
			OrderStatus:      "paid",
			PaymentStatus:    "success",
			IdempotentReplay: false,
		},
	}
	sim := payments.NewSimulator(proc, payments.DefaultSimulatorConfig())
	handler := SimulatePaymentHandler(sim)

	reqBody := domain.SimulatePaymentRequest{
		OrderID: "11111111-1111-1111-1111-111111111111",
		Outcome: "success",
	}
	payload, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/payments/simulate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp domain.PaymentResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.OrderStatus != "paid" || resp.PaymentStatus != "success" {
		t.Errorf("unexpected body: %+v", resp)
	}
}

func TestSimulatePaymentHandler_LateRefund409(t *testing.T) {
	proc := &dummyProcessor{
		err: payments.ErrOrderExpiredRefund,
	}
	sim := payments.NewSimulator(proc, payments.DefaultSimulatorConfig())
	handler := SimulatePaymentHandler(sim)

	reqBody := domain.SimulatePaymentRequest{
		OrderID: "11111111-1111-1111-1111-111111111111",
		Outcome: "success",
	}
	payload, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/payments/simulate", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on late refund, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/httpapi -run TestSimulatePaymentHandler_`
Expected: FAIL (compilation error: `SimulatePaymentHandler` undefined).

- [ ] **Step 3: Implement `internal/httpapi/payment_handler.go` and mount on Baseline and Optimized mains**

Create `internal/httpapi/payment_handler.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"apexstock/internal/domain"
	"apexstock/internal/payments"
)

func SimulatePaymentHandler(sim *payments.Simulator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = newCorrelationID()
		}

		var body domain.SimulatePaymentRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := decoder.Decode(&body); err != nil {
			writeError(w, correlationID, domain.ValidationError("body", "Dữ liệu JSON không hợp lệ"))
			return
		}

		orderID := strings.TrimSpace(body.OrderID)
		if !validUUID(orderID) {
			writeError(w, correlationID, domain.ValidationError("order_id", "order_id phải là UUID hợp lệ"))
			return
		}

		result, err := sim.Simulate(r.Context(), domain.SimulatePaymentRequest{
			OrderID:      orderID,
			Outcome:      body.Outcome,
			ProviderTxID: body.ProviderTxID,
		})

		if errors.Is(err, payments.ErrOrderExpiredRefund) {
			writeError(w, correlationID, domain.ConflictError(
				"order_already_expired",
				"Đơn hàng đã hết hạn, giao dịch được đánh dấu hoàn tiền (refund)",
				map[string]any{"order_id": orderID, "payment_status": "refund"},
			))
			return
		}

		if errors.Is(err, payments.ErrInvalidStateTransition) {
			writeError(w, correlationID, domain.ConflictError(
				"invalid_state_transition",
				"Không thể chuyển trạng thái đơn hàng: giao dịch không hợp lệ",
				map[string]any{"order_id": orderID},
			))
			return
		}

		if errors.Is(err, payments.ErrOrderNotFound) {
			writeError(w, correlationID, domain.NotFoundError(
				"order_not_found",
				"Không tìm thấy đơn hàng",
			))
			return
		}

		if err != nil {
			writeError(w, correlationID, domain.ServiceUnavailableError())
			return
		}

		writeJSON(w, http.StatusOK, domain.PaymentResultResponse{
			OrderID:          result.OrderID,
			OrderStatus:      result.OrderStatus,
			PaymentStatus:    result.PaymentStatus,
			IdempotentReplay: result.IdempotentReplay,
		})
	})
}
```

Modify `cmd/api-baseline/main.go` to initialize Baseline payment simulator and mount `POST /api/v1/payments/simulate`:
```go
	paymentSvc := payments.NewBaselineService(db)
	paymentSim := payments.NewSimulator(paymentSvc, payments.DefaultSimulatorConfig())
	mux.Handle("POST /api/v1/payments/simulate", httpapi.SimulatePaymentHandler(paymentSim))
```

Modify `cmd/api-optimized/main.go` to initialize Optimized payment simulator and mount `POST /api/v1/payments/simulate`:
```go
	paymentSvc := payments.NewOptimizedService(db, rdb)
	paymentSim := payments.NewSimulator(paymentSvc, payments.DefaultSimulatorConfig())
	mux.Handle("POST /api/v1/payments/simulate", httpapi.SimulatePaymentHandler(paymentSim))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/httpapi -run TestSimulatePaymentHandler_`
Expected: PASS.
Run: `go build ./cmd/api-baseline && go build ./cmd/api-optimized`
Expected: Both binaries compile successfully.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/payment_handler.go internal/httpapi/payment_handler_test.go cmd/api-baseline/main.go cmd/api-optimized/main.go
git commit -m "feat(stage-5): add payment simulate HTTP endpoint and wire into baseline and optimized APIs"
```

---

### Task 7: Messaging Payment Events & Handlers

**Files:**
- Create: `internal/messaging/payment_handlers.go`
- Test: `internal/messaging/payment_handlers_test.go`

**Interfaces:**
- Consumes: `internal/messaging`, `internal/payments`
- Produces: `PaymentProcessedEvent`, `PublishPaymentProcessed(ctx, event)`

- [ ] **Step 1: Write failing test for payment processed event serialization and validation**

Create `internal/messaging/payment_handlers_test.go`:
```go
package messaging

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPaymentProcessedEvent_Validation(t *testing.T) {
	evt := PaymentProcessedEvent{
		EventID:       "evt-123",
		OrderID:       "11111111-1111-1111-1111-111111111111",
		OrderStatus:   "paid",
		PaymentStatus: "success",
		ProcessedAt:   time.Now().UTC(),
	}

	if err := evt.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed PaymentProcessedEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.OrderID != evt.OrderID || parsed.OrderStatus != evt.OrderStatus {
		t.Fatalf("mismatch: got %+v, want %+v", parsed, evt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/messaging -run TestPaymentProcessedEvent_`
Expected: FAIL (compilation error: `PaymentProcessedEvent` undefined).

- [ ] **Step 3: Implement `internal/messaging/payment_handlers.go`**

Create `internal/messaging/payment_handlers.go`:
```go
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
)

const SubjectPaymentProcessed = "flashsale.payment.processed"

type PaymentProcessedEvent struct {
	EventID       string    `json:"event_id"`
	OrderID       string    `json:"order_id"`
	OrderStatus   string    `json:"order_status"`
	PaymentStatus string    `json:"payment_status"`
	ProcessedAt   time.Time `json:"processed_at"`
}

func (e PaymentProcessedEvent) Validate() error {
	if e.EventID == "" || e.OrderID == "" || e.OrderStatus == "" || e.PaymentStatus == "" {
		return errors.New("invalid payment processed event: missing required fields")
	}
	return nil
}

func (c *Client) PublishPaymentProcessed(ctx context.Context, event PaymentProcessedEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = c.JS.PublishMsg(&nats.Msg{
		Subject: SubjectPaymentProcessed,
		Data:    body,
		Header:  nats.Header{"Nats-Msg-Id": []string{event.EventID}},
	}, nats.Context(ctx))
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/messaging -run TestPaymentProcessedEvent_`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/messaging/payment_handlers.go internal/messaging/payment_handlers_test.go
git commit -m "feat(stage-5): add payment completed and failed messaging events"
```

---

### Task 8: Concurrency Stress, Invariant Verification & Post-Run Reconciliation

**Files:**
- Create: `internal/payments/concurrency_test.go`
- Test: `internal/payments/concurrency_test.go`
- Execute: `scripts/reconcile.sql.sh`

**Interfaces:**
- Consumes: Complete Stage 5 payment stack, `scripts/reconcile.sql.sh`
- Produces: Concurrency race proof, verified Invariants I1–I5, clean reconciliation report

- [ ] **Step 1: Write failing concurrency race test (50 concurrent confirm vs cancel vs expire requests on the same order)**

Create `internal/payments/concurrency_test.go`:
```go
package payments

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	_ "github.com/lib/pq"
)

func TestConcurrency_ConfirmVsCancelVsExpireRace(t *testing.T) {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil || db.Ping() != nil {
		t.Skip("skipping concurrency test: DB unreachable")
	}
	defer db.Close()

	svc := NewBaselineService(db)
	orderID, itemID, _ := setupTestBaselineOrder(t, db, OrderStatusPendingPayment)

	var initialSold int32
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&initialSold)

	const workers = 30
	var wg sync.WaitGroup
	var successCount int32
	var restockCount int32

	outcomes := []string{OutcomeSuccess, OutcomeFailure, OutcomeCancel, OutcomeTimeout}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		outcome := outcomes[i%len(outcomes)]
		go func(idx int, out string) {
			defer wg.Done()
			res, err := svc.Process(context.Background(), ProcessPaymentRequest{
				OrderID: orderID,
				Outcome: out,
			})
			if err == nil {
				if res.OrderStatus == OrderStatusPaid && !res.IdempotentReplay {
					atomic.AddInt32(&successCount, 1)
				}
				if res.Restocked {
					atomic.AddInt32(&restockCount, 1)
				}
			}
		}(i, outcome)
	}

	wg.Wait()

	// Invariant verification: exactly one state must win, and restock must happen at most once!
	if restockCount > 1 {
		t.Fatalf("Invariant I2 violation: restock occurred %d times (expected at most 1)", restockCount)
	}
	if successCount > 1 {
		t.Fatalf("Invariant I2 violation: order confirmed %d times (expected at most 1)", successCount)
	}
	if successCount == 1 && restockCount == 1 {
		t.Fatalf("Invariant I1 violation: order was both confirmed and restocked!")
	}

	// Verify DB sold_quantity consistency
	var finalSold int32
	var finalStatus string
	_ = db.QueryRow(`SELECT sold_quantity FROM flash_sale_items WHERE id = $1`, itemID).Scan(&finalSold)
	_ = db.QueryRow(`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&finalStatus)

	if finalStatus == OrderStatusPaid {
		if finalSold != initialSold {
			t.Errorf("expected final sold = %d, got %d", initialSold, finalSold)
		}
	} else {
		if finalSold != initialSold-1 {
			t.Errorf("expected restocked final sold = %d, got %d", initialSold-1, finalSold)
		}
	}
}
```

- [ ] **Step 2: Run concurrency race test with `-race` flag**

Run: `go test -v -race ./internal/payments -run TestConcurrency_ConfirmVsCancelVsExpireRace`
Expected: PASS with 0 data races reported.

- [ ] **Step 3: Run inventory reconciliation verification script**

Run:
```bash
POSTGRES_HOST=localhost scripts/reconcile.sql.sh
```
Expected output:
```text
==> Reconciling apexstock_baseline
PASS: apexstock_baseline
==> Reconciling apexstock_optimized
PASS: apexstock_optimized
Reconciliation PASS
```

- [ ] **Step 4: Commit**

```bash
git add internal/payments/concurrency_test.go
git commit -m "test(stage-5): add concurrency race tests and post-run inventory reconciliation verification"
```
