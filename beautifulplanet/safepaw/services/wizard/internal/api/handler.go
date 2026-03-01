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
//   GET  /api/v1/status          — All service statuses (Docker container list + overall health)
//   GET  /api/v1/prerequisites   — Check system requirements
//   GET  /api/v1/config          — Current .env config (secrets masked)
//   PUT  /api/v1/config         — Update allowed keys in .env
//   POST /api/v1/services/{name}/restart — Restart a SafePaw service (wizard, gateway, openclaw, redis, postgres)
// =============================================================

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"safepaw/wizard/internal/audit"
	"safepaw/wizard/internal/config"
	"safepaw/wizard/internal/docker"
	"safepaw/wizard/internal/session"
	"safepaw/wizard/internal/totp"
	"safepaw/wizard/ui"
)

// Handler holds all API dependencies.
type Handler struct {
	cfg    *config.Config
	docker *docker.Client
	audit  *audit.Logger
}

// NewHandler creates a new API handler.
func NewHandler(cfg *config.Config, dc *docker.Client) (*Handler, error) {
	return &Handler{cfg: cfg, docker: dc, audit: audit.New()}, nil
}

// Close cleans up resources.
func (h *Handler) Close() {
	if h.docker != nil {
		h.docker.Close()
	}
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
	mux.HandleFunc("GET /api/v1/config", h.handleGetConfig)
	mux.HandleFunc("PUT /api/v1/config", h.handlePutConfig)
	mux.HandleFunc("POST /api/v1/services/{name}/restart", h.handleServiceRestart)

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
	TOTP     string `json:"totp,omitempty"` // Required when MFA (WIZARD_TOTP_SECRET) is enabled
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"` // seconds
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1KB to prevent memory exhaustion attacks
	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid request body"})
		return
	}

	if req.Password != h.cfg.AdminPassword {
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		log.Printf("[WARN] Failed login attempt from %s", ip)
		h.audit.LoginFailure(ip, "invalid_password")
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, http.StatusUnauthorized, errorResponse{"invalid password"})
		return
	}

	// When MFA is enabled, require and validate TOTP code
	if h.cfg.TOTPSecret != "" {
		if req.TOTP == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{"totp_required"})
			return
		}
		if !totp.Validate(h.cfg.TOTPSecret, req.TOTP) {
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = fwd
			}
			log.Printf("[WARN] Failed TOTP verification from %s", ip)
			h.audit.LoginFailure(ip, "invalid_totp")
			time.Sleep(500 * time.Millisecond)
			writeJSON(w, http.StatusUnauthorized, errorResponse{"invalid totp code"})
			return
		}
	}

	// Generate signed session token (24h TTL)
	const ttl = 24 * time.Hour
	token, err := session.Create(h.cfg.AdminPassword, ttl)
	if err != nil {
		log.Printf("[ERROR] Failed to create session token: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{"internal error"})
		return
	}

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = fwd
	}
	h.audit.LoginSuccess(ip)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.cfg.SecureCookies,
		MaxAge:   int(ttl.Seconds()),
	})

	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresIn: int(ttl.Seconds()),
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	checks := []prerequisiteCheck{
		h.checkDocker(ctx),
		h.checkDockerCompose(ctx),
		checkPorts(8080, 3000),
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

// checkDocker verifies Docker is available and running via the API.
func (h *Handler) checkDocker(ctx context.Context) prerequisiteCheck {
	if err := h.docker.Ping(ctx); err != nil {
		return prerequisiteCheck{
			Name:     "Docker",
			Status:   "fail",
			Message:  fmt.Sprintf("Docker daemon unreachable: %v", err),
			HelpURL:  "https://docs.docker.com/get-docker/",
			Required: true,
		}
	}
	return prerequisiteCheck{
		Name:     "Docker",
		Status:   "pass",
		Message:  "Docker daemon is running and accessible",
		HelpURL:  "https://docs.docker.com/get-docker/",
		Required: true,
	}
}

// checkDockerCompose verifies Docker Compose V2 is available.
func (h *Handler) checkDockerCompose(ctx context.Context) prerequisiteCheck {
	out, err := exec.CommandContext(ctx, "docker", "compose", "version", "--short").Output()
	if err != nil {
		return prerequisiteCheck{
			Name:     "Docker Compose",
			Status:   "fail",
			Message:  "Docker Compose V2 not found (need 'docker compose' CLI plugin)",
			HelpURL:  "https://docs.docker.com/compose/install/",
			Required: true,
		}
	}
	version := strings.TrimSpace(string(out))
	return prerequisiteCheck{
		Name:     "Docker Compose",
		Status:   "pass",
		Message:  fmt.Sprintf("Docker Compose %s", version),
		HelpURL:  "https://docs.docker.com/compose/install/",
		Required: true,
	}
}

// checkPorts probes whether the required ports are available.
func checkPorts(ports ...int) prerequisiteCheck {
	var busy []string
	for _, port := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			busy = append(busy, fmt.Sprintf("%d", port))
		} else {
			ln.Close()
		}
	}

	if len(busy) > 0 {
		return prerequisiteCheck{
			Name:     "Port Availability",
			Status:   "fail",
			Message:  fmt.Sprintf("Ports already in use: %s", strings.Join(busy, ", ")),
			Required: true,
		}
	}
	return prerequisiteCheck{
		Name:     "Port Availability",
		Status:   "pass",
		Message:  fmt.Sprintf("Ports %s are available", joinInts(ports)),
		Required: true,
	}
}

// checkDiskSpace checks for at least 2GB free on the working directory's volume.
func checkDiskSpace() prerequisiteCheck {
	// Note: precise disk space check requires platform-specific syscalls.
	// For the container environment (Linux), we use 'df'.
	out, err := exec.Command("df", "-BG", "--output=avail", "/").Output()
	if err != nil {
		return prerequisiteCheck{
			Name:     "Disk Space",
			Status:   "warn",
			Message:  "Unable to check disk space (non-critical)",
			Required: false,
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return prerequisiteCheck{
			Name:     "Disk Space",
			Status:   "warn",
			Message:  "Unable to parse disk space output",
			Required: false,
		}
	}

	// Parse "42G" → 42
	avail := strings.TrimSpace(lines[1])
	avail = strings.TrimSuffix(avail, "G")
	var gb int
	if _, err := fmt.Sscanf(avail, "%d", &gb); err == nil && gb >= 2 {
		return prerequisiteCheck{
			Name:     "Disk Space",
			Status:   "pass",
			Message:  fmt.Sprintf("%dGB free space available", gb),
			Required: false,
		}
	}

	return prerequisiteCheck{
		Name:     "Disk Space",
		Status:   "warn",
		Message:  fmt.Sprintf("Low disk space: %sGB (recommend 2GB+)", avail),
		Required: false,
	}
}

// joinInts formats a slice of ints as a comma-separated string.
func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ", ")
}

// ─── Status ──────────────────────────────────────────────────

type statusResponse struct {
	Services []docker.ServiceInfo `json:"services"`
	Overall  string               `json:"overall"` // "healthy", "degraded", "down"
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	services, err := h.docker.Services(ctx)
	if err != nil {
		log.Printf("[WARN] Failed to query Docker services: %v", err)
		writeJSON(w, http.StatusOK, statusResponse{
			Services: []docker.ServiceInfo{},
			Overall:  "unknown",
		})
		return
	}

	// Determine overall health
	overall := "healthy"
	running := 0
	for _, svc := range services {
		if svc.State == "running" {
			running++
			if svc.Health == "unhealthy" {
				overall = "degraded"
			}
		}
	}
	if running == 0 && len(services) > 0 {
		overall = "down"
	} else if running < len(services) {
		overall = "degraded"
	}

	writeJSON(w, http.StatusOK, statusResponse{
		Services: services,
		Overall:  overall,
	})
}

// Allowed service names for restart (maps to container name safepaw-{name}).
var allowedRestartServices = map[string]bool{
	"wizard": true, "gateway": true, "openclaw": true, "redis": true, "postgres": true,
}

func (h *Handler) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{"missing service name"})
		return
	}
	if !allowedRestartServices[name] {
		writeJSON(w, http.StatusBadRequest, errorResponse{"unknown service; allowed: wizard, gateway, openclaw, redis, postgres"})
		return
	}
	if h.docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{"Docker client not available"})
		return
	}
	containerName := "safepaw-" + name

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	restartIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		restartIP = fwd
	}

	if err := h.docker.RestartContainer(ctx, containerName, 10); err != nil {
		log.Printf("[WARN] Restart %s failed: %v", containerName, err)
		h.audit.ServiceRestart(restartIP, name, "failure")
		writeJSON(w, http.StatusInternalServerError, errorResponse{fmt.Sprintf("restart failed: %v", err)})
		return
	}

	h.audit.ServiceRestart(restartIP, name, "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": name})
}

// ─── SPA Handler ─────────────────────────────────────────────

// spaHandler serves the embedded React UI.
// For any path that doesn't match a real file, serve index.html
// (React Router handles client-side routing).
func (h *Handler) spaHandler() http.Handler {
	// The embed FS has "dist" prefix from the ui package, strip it
	stripped, err := fs.Sub(ui.DistFS, "dist")
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
