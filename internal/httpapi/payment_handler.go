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
