package httpapi

import (
	"bytes"
	"context"
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

func (h *SepayWebhookHandler) logWebhookAudit(ctx context.Context, orderID string, sepayID int64, status string, payload []byte) {
	if h.db == nil {
		return
	}
	var orderUUID any = orderID
	if orderID == "" {
		orderUUID = nil
	}
	txRef := fmt.Sprintf("sepay-%d", sepayID)
	_, _ = h.db.ExecContext(ctx, `
		INSERT INTO webhook_logs (order_id, provider, transaction_ref, status, payload)
		VALUES ($1, 'sepay', $2, $3, $4)`,
		orderUUID, txRef, status, payload)
}
