package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("[worker] Starting worker process...")
	log.Println("[worker] NATS consumer and payment confirmation logic pending Stage 4-5")

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("[worker] Shutting down gracefully...")
	time.Sleep(1 * time.Second)
}
