package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"apexstock/internal/baseline"
	"apexstock/internal/config"
	"apexstock/internal/domain"
	"apexstock/internal/httpapi"
	"apexstock/internal/payments"
	"apexstock/internal/payments/sepay"

	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("postgres", cfg.BaselineDSN)
	if err != nil {
		log.Fatalf("[api-baseline] database open failed: %v", err)
	}
	defer db.Close()
	service := baseline.NewOrderService(db)

	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/healthz", httpapi.HealthzHandler)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		httpapi.ReadyzHandler(w, r, cfg.BaselineDSN)
	})

	// Flash sale order endpoint
	mux.Handle("POST /api/v1/flash-sale/orders", httpapi.BaselineOrderHandler(func(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
		return service.Create(ctx, req)
	}))

	// Payment simulation endpoint
	paymentSvc := payments.NewBaselineService(db)
	paymentSim := payments.NewSimulator(paymentSvc, payments.DefaultSimulatorConfig())
	mux.Handle("POST /api/v1/payments/simulate", httpapi.SimulatePaymentHandler(paymentSim))

	// Mount SePay adapter and endpoints
	sepayCfg := sepay.LoadConfigFromEnv()
	sepayAdapter := sepay.NewAdapter(sepayCfg)
	sepayHandler := httpapi.NewSepayWebhookHandler(sepayAdapter, paymentSvc, db)

	mux.HandleFunc("POST /api/v1/payments/sepay/initiate", sepayHandler.HandleInitiate)
	mux.HandleFunc("POST /api/v1/webhooks/sepay", sepayHandler.HandleWebhook)

	addr := fmt.Sprintf(":%d", cfg.BaselinePort)
	log.Printf("[api-baseline] Starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[api-baseline] Server failed: %v", err)
		os.Exit(1)
	}
}
