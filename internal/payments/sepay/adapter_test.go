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
