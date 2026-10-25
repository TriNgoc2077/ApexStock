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
	ErrUnauthorized    = errors.New("unauthorized: invalid sepay webhook signature or apikey")
	ErrOrderIDNotFound = errors.New("order ID not found in transfer content")
	ErrInvalidConfig   = errors.New("sepay config missing required parameters")
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
