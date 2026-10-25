package main

import (
	"apexstock/internal/config"
	"apexstock/internal/httpapi"
	"apexstock/internal/messaging"
	"apexstock/internal/optimized"
	"apexstock/internal/payments"
	"apexstock/internal/payments/sepay"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	cfg := config.Load()
	db, err := sql.Open("postgres", cfg.OptimizedDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()
	nc, err := messaging.Connect(cfg.NatsURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()
	svc := optimized.NewService(db, rdb, nc)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", httpapi.HealthzHandler)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) { httpapi.ReadyzHandler(w, r, cfg.OptimizedDSN) })
	mux.Handle("POST /api/v1/flash-sale/orders", httpapi.OptimizedOrderHandler(svc.Create))
	paymentSvc := payments.NewOptimizedService(db, rdb)
	paymentSim := payments.NewSimulator(paymentSvc, payments.DefaultSimulatorConfig())
	mux.Handle("POST /api/v1/payments/simulate", httpapi.SimulatePaymentHandler(paymentSim))

	// Mount SePay adapter and endpoints
	sepayCfg := sepay.LoadConfigFromEnv()
	sepayAdapter := sepay.NewAdapter(sepayCfg)
	sepayHandler := httpapi.NewSepayWebhookHandler(sepayAdapter, paymentSvc, db)

	mux.HandleFunc("POST /api/v1/payments/sepay/initiate", sepayHandler.HandleInitiate)
	mux.HandleFunc("POST /api/v1/webhooks/sepay", sepayHandler.HandleWebhook)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.OptimizedPort), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("[api-optimized] Starting on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	_ = os.Stdout
}
