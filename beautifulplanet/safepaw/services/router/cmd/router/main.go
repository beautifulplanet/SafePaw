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
	log.Printf("[CONFIG] Inbound=%s Outbound=%s AgentInbox=%s",
		cfg.InboundStream, cfg.OutboundStream, cfg.AgentInboxStream)
	log.Printf("[CONFIG] BlockTime=%v AckTimeout=%v MaxRetries=%d MaxMessageSize=%d MaxOutboundSize=%d",
		cfg.BlockTime, cfg.AckTimeout, cfg.MaxRetries, cfg.MaxMessageSize, cfg.MaxOutboundSize)
	log.Printf("[CONFIG] Health=:%d", cfg.HealthPort)

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
	// Step 3b: Create agent inbox publisher (if configured)
	// --------------------------------------------------------
	// When AGENT_INBOX_STREAM is set, the Router forwards messages
	// to the Agent instead of echoing them directly. This enables
	// the full pipeline: Gateway → Router → Agent → Gateway
	var agentPub *publisher.Publisher
	if cfg.AgentInboxStream != "" {
		agentPub, err = publisher.NewPublisherWithConnection(
			cfg.RedisAddr,
			cfg.RedisPassword,
			cfg.RedisDB,
			cfg.AgentInboxStream,
			cfg.MaxOutboundSize,
		)
		if err != nil {
			log.Fatalf("[FATAL] Agent inbox publisher creation failed: %v", err)
		}
		defer agentPub.Close()
		log.Printf("[CONFIG] Agent mode: forwarding to %s", cfg.AgentInboxStream)
	} else {
		log.Println("[CONFIG] Echo mode: no AGENT_INBOX_STREAM configured")
	}

	// --------------------------------------------------------
	// Step 4: Create router (message processing logic)
	// --------------------------------------------------------
	var rtr *router.Router
	if agentPub != nil {
		rtr = router.NewWithAgent(pub, agentPub)
	} else {
		rtr = router.New(pub)
	}

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
	shutdownStart := time.Now()

	// Phase 1: Cancel consumer context — stops read loop and workers
	log.Println("[SHUTDOWN] Phase 1: Cancelling consumer context...")
	cancel()

	// Wait for consumer goroutine to fully stop before closing connections.
	log.Println("[SHUTDOWN] Phase 2: Waiting for consumer to drain...")
	<-consumerDone
	log.Printf("[SHUTDOWN] Phase 2 complete — consumer stopped (%v)",
		time.Since(shutdownStart).Round(time.Millisecond))

	// Shutdown health server
	log.Println("[SHUTDOWN] Phase 3: Closing health server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[SHUTDOWN] Health server shutdown error: %v", err)
	}
	log.Printf("[SHUTDOWN] Phase 3 complete (%v)",
		time.Since(shutdownStart).Round(time.Millisecond))

	log.Printf("=== NOPEnclaw Router stopped (total shutdown: %v) ===",
		time.Since(shutdownStart).Round(time.Millisecond))
}
