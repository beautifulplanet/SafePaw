// =============================================================
// NOPEnclaw Router — Main Entry Point
// =============================================================
// The Router is the middle piece of the message pipeline:
//
//   Gateway → Redis (inbound) → [ROUTER] → Redis (outbound) → Gateway
//
// It:
// 1. Loads configuration from environment variables
// 2. Connects to Redis and creates a consumer group
// 3. Starts a worker pool for parallel message processing
// 4. Starts a pending message reclaimer for crash recovery
// 5. Starts a health check HTTP endpoint
// 6. Handles graceful shutdown (SIGINT/SIGTERM)
//
// The Router uses XREADGROUP (not XREAD) for:
// - Exactly-once delivery across multiple Router instances
// - Message acknowledgment after successful processing
// - Automatic retry of failed/stuck messages
// =============================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nopenclaw/router/internal/config"
	"nopenclaw/router/internal/consumer"
	"nopenclaw/router/internal/publisher"
	"nopenclaw/router/internal/router"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("=== NOPEnclaw Router starting ===")

	// --------------------------------------------------------
	// Step 1: Load configuration
	// --------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Configuration error: %v", err)
	}
	log.Printf("[CONFIG] Redis=%s Group=%s Consumer=%s Workers=%d Batch=%d",
		cfg.RedisAddr, cfg.ConsumerGroup, cfg.ConsumerName, cfg.WorkerCount, cfg.BatchSize)

	// --------------------------------------------------------
	// Step 2: Create consumer (connects to Redis, creates group)
	// --------------------------------------------------------
	cons, err := consumer.NewConsumer(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Consumer creation failed: %v", err)
	}
	defer cons.Close()

	// --------------------------------------------------------
	// Step 3: Create publisher (for outbound messages)
	// --------------------------------------------------------
	pub, err := publisher.NewPublisherWithConnection(
		cfg.RedisAddr,
		cfg.RedisPassword,
		cfg.RedisDB,
		cfg.OutboundStream,
		cfg.MaxOutboundSize,
	)
	if err != nil {
		log.Fatalf("[FATAL] Publisher creation failed: %v", err)
	}
	defer pub.Close()

	// --------------------------------------------------------
	// Step 4: Create router (message processing logic)
	// --------------------------------------------------------
	rtr := router.New(pub)

	// --------------------------------------------------------
	// Step 5: Start health check endpoint
	// --------------------------------------------------------
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := map[string]interface{}{
			"status":    "ok",
			"service":   "router",
			"consumer":  cfg.ConsumerName,
			"group":     cfg.ConsumerGroup,
			"workers":   cfg.WorkerCount,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}

		if err := cons.HealthCheck(ctx); err != nil {
			status["status"] = "degraded"
			status["redis"] = "unreachable"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			status["redis"] = "connected"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	healthServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("[HEALTH] Listening on :%d", cfg.HealthPort)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[HEALTH] Server error: %v", err)
		}
	}()

	// --------------------------------------------------------
	// Step 6: Start consumer (blocks until context cancelled)
	// --------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())

	// consumerDone signals when consumer.Run() has fully exited
	// (all workers drained, WaitGroup done). We must wait for this
	// before closing Redis connections, or workers may XACK on a
	// closed client.
	consumerDone := make(chan struct{})
	go func() {
		cons.Run(ctx, rtr.Handle)
		close(consumerDone)
	}()

	log.Println("[ROUTER] Running — waiting for messages")

	// --------------------------------------------------------
	// Step 7: Graceful shutdown
	// --------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[SHUTDOWN] Received signal: %v", sig)

	// Cancel consumer context — stops read loop and workers
	cancel()

	// Wait for consumer goroutine to fully stop before closing connections.
	// This ensures all in-flight XACKs complete before the Redis client closes.
	log.Println("[SHUTDOWN] Waiting for consumer to drain...")
	<-consumerDone
	log.Println("[SHUTDOWN] Consumer stopped")

	// Shutdown health server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[SHUTDOWN] Health server shutdown error: %v", err)
	}

	log.Println("=== NOPEnclaw Router stopped ===")
}
