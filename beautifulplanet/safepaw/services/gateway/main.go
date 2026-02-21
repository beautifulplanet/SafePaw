// =============================================================
// NOPEnclaw Gateway — Main Entry Point
// =============================================================
// This is where the Gateway starts up. It:
//
// 1. Loads configuration from environment variables
// 2. Connects to Redis (validates immediately)
// 3. Sets up security middleware (headers, rate limit, origin)
// 4. Starts the WebSocket endpoint on /ws
// 5. Starts the outbound reader (delivers replies to clients)
// 6. Starts an HTTP health check on /health
// 7. Handles graceful shutdown (SIGINT/SIGTERM)
//
// The Gateway is the front door of NOPEnclaw.
// Every message in and out passes through here.
// =============================================================

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"nopenclaw/gateway/config"
	"nopenclaw/gateway/middleware"
	redisStream "nopenclaw/gateway/redis"
	"nopenclaw/gateway/ws"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("=== NOPEnclaw Gateway starting ===")

	// --------------------------------------------------------
	// Step 1: Load configuration
	// --------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Configuration error: %v", err)
	}
	log.Printf("[CONFIG] Port=%d MaxConns=%d Redis=%s Streams=[%s, %s] Workers=%d Group=%s Consumer=%s",
		cfg.Port, cfg.WSMaxConnections, cfg.RedisAddr, cfg.InboundStream, cfg.OutboundStream,
		cfg.DeliveryWorkers, cfg.OutboundGroup, cfg.OutboundConsumer)

	// --------------------------------------------------------
	// Step 2: Connect to Redis
	// --------------------------------------------------------
	stream, err := redisStream.NewStreamClient(
		cfg.RedisAddr,
		cfg.RedisPassword,
		cfg.RedisDB,
		cfg.InboundStream,
		cfg.OutboundStream,
		cfg.OutboundGroup,
		cfg.OutboundConsumer,
	)
	if err != nil {
		log.Fatalf("[FATAL] Redis connection failed: %v", err)
	}
	defer stream.Close()

	// --------------------------------------------------------
	// Step 2b: Ensure outbound consumer group exists
	// --------------------------------------------------------
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := stream.EnsureOutboundGroup(initCtx); err != nil {
		initCancel()
		log.Fatalf("[FATAL] Cannot create outbound consumer group: %v", err)
	}
	initCancel()

	// --------------------------------------------------------
	// Step 3: Set up connection hub + rate limiter
	// --------------------------------------------------------
	hub := ws.NewHub(cfg.WSMaxConnections)
	rateLimiter := middleware.NewRateLimiter(30, 1*time.Minute) // 30 new conns/min per IP
	defer rateLimiter.Stop()

	// --------------------------------------------------------
	// Step 4: Build HTTP routes with middleware stack
	// --------------------------------------------------------
	mux := http.NewServeMux()

	// Health check — no auth, no middleware (used by Docker/k8s)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := map[string]interface{}{
			"status":      "ok",
			"connections": hub.Count(),
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}

		// Deep health check: verifies stream + consumer group, not just PING
		if err := stream.DeepHealthCheck(ctx); err != nil {
			status["status"] = "degraded"
			status["redis"] = err.Error()
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			status["redis"] = "connected"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// WebSocket endpoint — full middleware stack
	wsHandler := ws.Handler(hub, stream, cfg)
	mux.Handle("/ws", wsHandler)

	// Apply middleware (outermost first):
	// Request → SecurityHeaders → RequestID → OriginCheck → RateLimit → [Auth] → Handler
	var handler http.Handler = mux

	// Auth middleware (only if enabled — disabled in dev by default)
	if cfg.AuthEnabled {
		auth, err := middleware.NewAuthenticator(cfg.AuthSecret, cfg.AuthDefaultTTL, cfg.AuthMaxTTL)
		if err != nil {
			log.Fatalf("[FATAL] Auth setup failed: %v", err)
		}
		log.Printf("[AUTH] Authentication ENABLED (default TTL=%v, max TTL=%v)",
			cfg.AuthDefaultTTL, cfg.AuthMaxTTL)
		handler = middleware.AuthRequired(auth, "ws", handler)
	} else {
		log.Println("[AUTH] Authentication DISABLED (set AUTH_ENABLED=true for production)")
		// Strip auth identity headers that clients could spoof when auth is off.
		// Without this, any client can send X-Auth-Subject: admin and ws/handler.go
		// will trust it as an authenticated identity.
		handler = middleware.StripAuthHeaders(handler)
	}

	handler = middleware.RateLimit(rateLimiter, handler)
	handler = middleware.OriginCheck(cfg.AllowedOrigins, handler)
	handler = middleware.RequestID(handler)
	handler = middleware.SecurityHeaders(handler)

	// --------------------------------------------------------
	// Step 5: Start outbound delivery workers (deliver replies to clients)
	// Each worker independently calls XREADGROUP — Redis distributes
	// messages across all workers in the consumer group, so each message
	// is delivered to exactly ONE worker.
	// --------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var workerWg sync.WaitGroup
	for i := 0; i < cfg.DeliveryWorkers; i++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()
			outboundWorker(ctx, workerID, hub, stream)
		}(i)
	}
	log.Printf("[OUTBOUND] Started %d delivery workers", cfg.DeliveryWorkers)

	// --------------------------------------------------------
	// Step 6: Create and start HTTP server
	// --------------------------------------------------------
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		// Limit header size to prevent resource exhaustion
		MaxHeaderBytes: 1 << 16, // 64KB
	}

	// Start server in background
	go func() {
		if cfg.TLSEnabled {
			// ---- TLS Mode ----
			// Configure TLS with modern, secure settings
			server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
				CurvePreferences: []tls.CurveID{
					tls.X25519, // Fastest, most secure
					tls.CurveP256,
				},
				CipherSuites: []uint16{
					// TLS 1.3 cipher suites are automatic (can't configure them)
					// These are TLS 1.2 fallbacks (strong only):
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				},
			}

			// Override port for TLS
			server.Addr = fmt.Sprintf(":%d", cfg.TLSPort)
			log.Printf("[SERVER] Listening on :%d (TLS ENABLED — cert=%s)",
				cfg.TLSPort, cfg.TLSCertFile)

			if err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("[FATAL] TLS server error: %v", err)
			}
		} else {
			// ---- Plain HTTP Mode (dev only) ----
			log.Printf("[SERVER] Listening on :%d (TLS disabled — dev mode)", cfg.Port)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("[FATAL] Server error: %v", err)
			}
		}
	}()

	// --------------------------------------------------------
	// Step 7: Graceful shutdown
	// --------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[SHUTDOWN] Received signal: %v", sig)

	// Phase 1: Stop accepting new HTTP connections
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[SHUTDOWN] Server shutdown error: %v", err)
	}

	// Phase 2: Send WebSocket close frames to all connected clients.
	// This tells clients "server is going away, reconnect to another instance."
	closed := hub.CloseAll()
	log.Printf("[SHUTDOWN] Sent CloseGoingAway to %d WebSocket connections", closed)

	// Phase 3: Stop outbound workers and wait for them to drain
	cancel()
	workerWg.Wait()
	log.Println("[SHUTDOWN] All delivery workers stopped")

	log.Println("=== NOPEnclaw Gateway stopped ===")
}

// outboundWorker is one of N goroutines that read from the outbound
// Redis stream using XREADGROUP. Redis distributes messages across
// all workers in the consumer group, so each message is processed
// by exactly one worker.
//
// After delivering each batch to WebSocket clients, the worker ACKs
// the messages so they're removed from the pending entries list.
func outboundWorker(ctx context.Context, workerID int, hub *ws.Hub, stream *redisStream.StreamClient) {
	log.Printf("[OUTBOUND] Worker %d started", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[OUTBOUND] Worker %d stopped", workerID)
			return
		default:
		}

		messages, err := stream.ReadOutbound(ctx, 100)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, shutting down
			}
			log.Printf("[OUTBOUND] Worker %d: read error: %v (retrying in 1s)", workerID, err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Collect stream IDs to ACK after delivery
		var acked []string

		for _, msg := range messages {
			conn, ok := hub.GetConnection(msg.SessionID)
			if !ok {
				log.Printf("[OUTBOUND] Worker %d: no connection for session=%s, message=%s dropped",
					workerID, msg.SessionID, msg.MessageID)
				// ACK even if no connection — the message was processed, just undeliverable.
				// It would be wrong to leave it pending forever.
				acked = append(acked, msg.StreamID)
				continue
			}

			data, err := json.Marshal(map[string]interface{}{
				"type":       "message",
				"message_id": msg.MessageID,
				"content":    msg.Content,
				"timestamp":  msg.Timestamp,
			})
			if err != nil {
				log.Printf("[OUTBOUND] Worker %d: marshal error: %v", workerID, err)
				acked = append(acked, msg.StreamID)
				continue
			}

			if !conn.Send(data) {
				log.Printf("[OUTBOUND] Worker %d: buffer full for session=%s, message=%s dropped",
					workerID, msg.SessionID, msg.MessageID)
			}

			acked = append(acked, msg.StreamID)
		}

		// Batch ACK all processed messages
		if len(acked) > 0 {
			if err := stream.AckOutbound(ctx, acked...); err != nil {
				log.Printf("[OUTBOUND] Worker %d: ACK error: %v (messages will be re-delivered)", workerID, err)
			}
		}
	}
}
