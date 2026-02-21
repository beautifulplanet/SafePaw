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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	log.Printf("[CONFIG] Port=%d MaxConns=%d Redis=%s Streams=[%s, %s]",
		cfg.Port, cfg.WSMaxConnections, cfg.RedisAddr, cfg.InboundStream, cfg.OutboundStream)

	// --------------------------------------------------------
	// Step 2: Connect to Redis
	// --------------------------------------------------------
	stream, err := redisStream.NewStreamClient(
		cfg.RedisAddr,
		cfg.RedisPassword,
		cfg.RedisDB,
		cfg.InboundStream,
		cfg.OutboundStream,
	)
	if err != nil {
		log.Fatalf("[FATAL] Redis connection failed: %v", err)
	}
	defer stream.Close()

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

		if err := stream.HealthCheck(ctx); err != nil {
			status["status"] = "degraded"
			status["redis"] = "unreachable"
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
	// Request → SecurityHeaders → RequestID → OriginCheck → RateLimit → Handler
	var handler http.Handler = mux
	handler = middleware.RateLimit(rateLimiter, handler)
	handler = middleware.OriginCheck(cfg.AllowedOrigins, handler)
	handler = middleware.RequestID(handler)
	handler = middleware.SecurityHeaders(handler)

	// --------------------------------------------------------
	// Step 5: Start outbound reader (deliver replies to clients)
	// --------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go outboundReader(ctx, hub, stream)

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
		log.Printf("[SERVER] Listening on :%d", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server error: %v", err)
		}
	}()

	// --------------------------------------------------------
	// Step 7: Graceful shutdown
	// --------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[SHUTDOWN] Received signal: %v", sig)

	// Give active connections 10 seconds to finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[SHUTDOWN] Server shutdown error: %v", err)
	}

	cancel() // Stop outbound reader
	log.Println("=== NOPEnclaw Gateway stopped ===")
}

// outboundReader continuously reads from the outbound Redis stream
// and delivers messages to connected WebSocket clients.
func outboundReader(ctx context.Context, hub *ws.Hub, stream *redisStream.StreamClient) {
	lastID := "$" // Only read NEW messages (skip history)
	log.Println("[OUTBOUND] Reader started, waiting for messages...")

	for {
		select {
		case <-ctx.Done():
			log.Println("[OUTBOUND] Reader stopped")
			return
		default:
		}

		messages, newLastID, err := stream.ReadOutbound(ctx, lastID, 100)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, shutting down
			}
			log.Printf("[OUTBOUND] Read error: %v (retrying in 1s)", err)
			time.Sleep(1 * time.Second)
			continue
		}

		lastID = newLastID

		for _, msg := range messages {
			conn, ok := hub.GetConnection(msg.SessionID)
			if !ok {
				log.Printf("[OUTBOUND] No connection for session=%s, message=%s dropped",
					msg.SessionID, msg.MessageID)
				continue
			}

			data, err := json.Marshal(map[string]interface{}{
				"type":       "message",
				"message_id": msg.MessageID,
				"content":    msg.Content,
				"timestamp":  msg.Timestamp,
			})
			if err != nil {
				log.Printf("[OUTBOUND] Marshal error: %v", err)
				continue
			}

			if !conn.Send(data) {
				log.Printf("[OUTBOUND] Buffer full for session=%s, message=%s dropped",
					msg.SessionID, msg.MessageID)
			}
		}
	}
}
