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
