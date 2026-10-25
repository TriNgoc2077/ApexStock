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
