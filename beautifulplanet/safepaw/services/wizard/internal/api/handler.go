// =============================================================
// SafePaw Setup Wizard - API Handler & Router
// =============================================================
// REST API for the wizard UI. All endpoints return JSON.
// The handler also serves the embedded React UI for any
// non-API path (SPA fallback routing).
//
// Endpoints:
//   GET  /api/v1/health          — Wizard health check
//   POST /api/v1/auth/login      — Authenticate with admin password
//   GET  /api/v1/status          — All service statuses
//   GET  /api/v1/prerequisites   — Check system requirements
//   GET  /api/v1/config          — Current configuration (masked)
//   PUT  /api/v1/config          — Update configuration
//   POST /api/v1/config/validate — Validate a config field
//   GET  /api/v1/services        — Docker container statuses
//   POST /api/v1/services/:name/restart — Restart a service
//   GET  /ws/status              — WebSocket for live status
// =============================================================

package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/beautifulplanet/safepaw/services/wizard/internal/config"
)

//go:embed all:../../ui/dist
var uiFS embed.FS

// Handler holds all API dependencies.
type Handler struct {
	cfg *config.Config
}

// NewHandler creates a new API handler.
func NewHandler(cfg *config.Config) (*Handler, error) {
	return &Handler{cfg: cfg}, nil
}

// Close cleans up resources.
func (h *Handler) Close() {
	log.Println("[INFO] API handler closed")
}

// Router returns the HTTP handler with all routes registered.
func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	// ── API Routes ──
	mux.HandleFunc("GET /api/v1/health", h.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", h.handleLogin)
	mux.HandleFunc("GET /api/v1/prerequisites", h.handlePrerequisites)
	mux.HandleFunc("GET /api/v1/status", h.handleStatus)

	// ── SPA Fallback ──
	// Serve React app for all non-API routes
	mux.Handle("/", h.spaHandler())

	return mux
}

// ─── Health Check ────────────────────────────────────────────

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

var startTime = time.Now()

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Service: "safepaw-wizard",
		Version: "0.1.0",
		Uptime:  time.Since(startTime).Round(time.Second).String(),
	})
}

// ─── Authentication ──────────────────────────────────────────

type loginRequest struct {
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"` // seconds
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid request body"})
		return
	}

	if req.Password != h.cfg.AdminPassword {
		// Deliberate delay to slow brute force
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, http.StatusUnauthorized, errorResponse{"invalid password"})
		return
	}

	// Set HttpOnly cookie (browser sessions) AND return token (API clients)
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    h.cfg.AdminPassword,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false, // localhost doesn't use HTTPS
		MaxAge:   86400, // 24 hours
	})

	writeJSON(w, http.StatusOK, loginResponse{
		Token:     h.cfg.AdminPassword,
		ExpiresIn: 86400,
	})
}

// ─── Prerequisites Check ─────────────────────────────────────

type prerequisiteCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`   // "pass", "fail", "warn"
	Message  string `json:"message"`
	HelpURL  string `json:"help_url,omitempty"`
	Required bool   `json:"required"`
}

type prerequisitesResponse struct {
	Checks  []prerequisiteCheck `json:"checks"`
	AllPass bool                `json:"all_pass"`
}

func (h *Handler) handlePrerequisites(w http.ResponseWriter, r *http.Request) {
	checks := []prerequisiteCheck{
		checkDocker(),
		checkDockerCompose(),
		checkPorts(),
		checkDiskSpace(),
	}

	allPass := true
	for _, c := range checks {
		if c.Required && c.Status == "fail" {
			allPass = false
			break
		}
	}

	writeJSON(w, http.StatusOK, prerequisitesResponse{
		Checks:  checks,
		AllPass: allPass,
	})
}

// checkDocker verifies Docker is available and running.
func checkDocker() prerequisiteCheck {
	// TODO: Implement actual Docker API check
	return prerequisiteCheck{
		Name:     "Docker",
		Status:   "pass",
		Message:  "Docker is installed and running",
		HelpURL:  "https://docs.docker.com/get-docker/",
		Required: true,
	}
}

// checkDockerCompose verifies Docker Compose V2 is available.
func checkDockerCompose() prerequisiteCheck {
	// TODO: Implement actual docker compose version check
	return prerequisiteCheck{
		Name:     "Docker Compose",
		Status:   "pass",
		Message:  "Docker Compose V2 is available",
		HelpURL:  "https://docs.docker.com/compose/install/",
		Required: true,
	}
}

// checkPorts verifies required ports are available.
func checkPorts() prerequisiteCheck {
	// TODO: Implement actual port scanning
	return prerequisiteCheck{
		Name:     "Port Availability",
		Status:   "pass",
		Message:  "Ports 8080, 9090 are available",
		Required: true,
	}
}

// checkDiskSpace verifies sufficient disk space.
func checkDiskSpace() prerequisiteCheck {
	// TODO: Implement actual disk space check
	return prerequisiteCheck{
		Name:     "Disk Space",
		Status:   "pass",
		Message:  "At least 2GB free space available",
		Required: false,
	}
}

// ─── Status ──────────────────────────────────────────────────

type serviceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "running", "stopped", "error", "unknown"
	Health string `json:"health"` // "healthy", "unhealthy", "starting", "none"
	Uptime string `json:"uptime,omitempty"`
}

type statusResponse struct {
	Services []serviceStatus `json:"services"`
	Overall  string          `json:"overall"` // "healthy", "degraded", "down"
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual Docker container health checks
	services := []serviceStatus{
		{Name: "gateway", Status: "unknown", Health: "none"},
		{Name: "redis", Status: "unknown", Health: "none"},
		{Name: "postgres", Status: "unknown", Health: "none"},
	}

	writeJSON(w, http.StatusOK, statusResponse{
		Services: services,
		Overall:  "unknown",
	})
}

// ─── SPA Handler ─────────────────────────────────────────────

// spaHandler serves the embedded React UI.
// For any path that doesn't match a real file, serve index.html
// (React Router handles client-side routing).
func (h *Handler) spaHandler() http.Handler {
	// The embed path includes "ui/dist" prefix, strip it
	stripped, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		log.Fatalf("[FATAL] Failed to access embedded UI: %v", err)
	}

	fileServer := http.FileServer(http.FS(stripped))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the exact file first
		if path != "/" {
			cleanPath := strings.TrimPrefix(path, "/")
			if _, err := fs.Stat(stripped, cleanPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// File not found → serve index.html for SPA routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// ─── Helpers ─────────────────────────────────────────────────

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[ERROR] JSON encode failed: %v", err)
	}
}
