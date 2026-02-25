// =============================================================
// SafePaw Setup Wizard - Entry Point
// =============================================================
// Single binary that serves:
//   1. REST API for configuration and health checks
//   2. Embedded React UI (built at compile time via go:embed)
//   3. WebSocket endpoint for real-time service status
//
// Design decisions:
//   - go:embed bakes the UI into the binary → one container, no nginx
//   - Separate service from gateway → different failure domains
//   - Admin password generated on first launch → wizard is never open
//   - CSP/CORS locked to localhost → no XSS attack surface
// =============================================================

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"safepaw/wizard/internal/api"
	"safepaw/wizard/internal/config"
	"safepaw/wizard/internal/docker"
	"safepaw/wizard/internal/middleware"
)

func main() {
	// ── Step 1: Load configuration ──
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Configuration error: %v", err)
	}

	log.Printf("[INFO] SafePaw Setup Wizard starting on :%d", cfg.Port)

	// ── Step 2: Initialize Docker client ──
	dc := docker.New(cfg.DockerHost, "safepaw")

	// ── Step 3: Initialize API handler ──
	handler, err := api.NewHandler(cfg, dc)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize API: %v", err)
	}

	// ── Step 4: Build middleware chain ──
	// Order matters: outermost runs first
	//   SecurityHeaders → CORS → AdminAuth → RateLimit → Router
	chain := middleware.SecurityHeaders(
		middleware.CORS(cfg.AllowedOrigins,
			middleware.AdminAuth(cfg.AdminPassword,
				middleware.RateLimit(60, time.Minute,
					handler.Router(),
				),
			),
		),
	)

	// ── Step 5: Configure HTTP server ──
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      chain,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Step 6: Start server in goroutine ──
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	log.Printf("[INFO] Wizard UI available at http://localhost:%d", cfg.Port)
	if cfg.AdminPassword != "" {
		log.Printf("[INFO] Admin password: %s (save this — shown once)", cfg.AdminPassword)
	}

	// ── Step 7: Graceful shutdown ──
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("[INFO] Received %s, shutting down...", sig)
	case err := <-errCh:
		log.Printf("[ERROR] Server error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Shutdown error: %v", err)
	}

	handler.Close()
	log.Println("[INFO] Wizard stopped")
}
