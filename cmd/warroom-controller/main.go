package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("[warroom-controller] Starting War Room controller...")
	log.Println("[warroom-controller] WebSocket and chaos control endpoints pending Stage 9")

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("[warroom-controller] Shutting down gracefully...")
	time.Sleep(1 * time.Second)
}
