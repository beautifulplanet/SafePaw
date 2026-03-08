// =============================================================
// SafePaw Setup Wizard - API Handler & Router
// =============================================================
// REST API for the wizard UI. All endpoints return JSON.
// The handler also serves the embedded React UI for any
// non-API path (SPA fallback routing).
//
// Endpoints:
//   GET  /api/v1/health          — Wizard health check (includes needs_setup flag)
//   POST /api/v1/auth/login      — Authenticate with admin password
//   GET  /api/v1/status          — All service statuses (Docker container list + overall health)
//   GET  /api/v1/prerequisites   — Check system requirements
//   GET  /api/v1/config          — Current .env config (secrets masked)
//   PUT  /api/v1/config         — Update allowed keys in .env
//   POST /api/v1/services/{name}/restart — Restart a SafePaw service
//   POST /api/v1/gateway/token   — Generate a gateway auth token (reads AUTH_SECRET from .env)
//   GET  /api/v1/gateway/metrics — Proxy Prometheus metrics from gateway
//   GET  /api/v1/gateway/activity — Parsed gateway activity summary
//   GET  /api/v1/gateway/usage   — Proxy OpenClaw cost/usage data from gateway
// =============================================================

package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"safepaw/wizard/internal/audit"
	"safepaw/wizard/internal/config"
	"safepaw/wizard/internal/docker"
	"safepaw/wizard/internal/middleware"
	"safepaw/wizard/internal/session"
	"safepaw/wizard/internal/totp"
	"safepaw/wizard/ui"
)

// Handler holds all API dependencies.
// sessionGen is bumped when admin password or TOTP secret is changed via PUT /config so existing sessions are invalidated.
type Handler struct {
	cfg        *config.Config
	docker     *docker.Client
	audit      *audit.Logger
	sessionGen atomic.Uint64
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

// clientIP extracts the client IP from the request, preferring X-Forwarded-For
// when the wizard sits behind a reverse proxy. Only the first (leftmost) address
// is used because that is the original client.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if ip, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(ip)
		}
		return strings.TrimSpace(fwd)
	}
	return r.RemoteAddr
}

// SessionValidator returns a function that validates session tokens using the server session secret and session generation.
// Pass this to middleware.AdminAuth so that when password or TOTP is changed via PUT /config, existing tokens fail validation.
// Returns (role, true) on success, ("", false) on invalid token.
func (h *Handler) SessionValidator() middleware.SessionValidator {
	return func(token string) (string, bool) {
		claims, err := session.Validate(token, h.cfg.SessionSecret, int(h.sessionGen.Load()%uint64(^uint(0)>>1))) //nolint:gosec // #nosec G115 -- sessionGen is a small counter
		if err != nil {
			return "", false
		}
		return claims.EffectiveRole(), true
	}
}

// ReloadCredsFromEnv re-reads the .env file and updates AdminPassword, OperatorPassword, ViewerPassword, and TOTPSecret in memory.
// Call after PUT /config updates those keys so new logins use the new credentials.
func (h *Handler) ReloadCredsFromEnv() {
	env, err := readEnvFile(h.cfg.EnvFilePath)
	if err != nil {
		log.Printf("[WARN] ReloadCredsFromEnv: read failed: %v", err)
		return
	}
	if v, ok := env["WIZARD_ADMIN_PASSWORD"]; ok {
		h.cfg.AdminPassword = v
	}
	if v, ok := env["WIZARD_TOTP_SECRET"]; ok {
		h.cfg.TOTPSecret = v
	}
	if v, ok := env["WIZARD_OPERATOR_PASSWORD"]; ok {
		h.cfg.OperatorPassword = v
	}
	if v, ok := env["WIZARD_VIEWER_PASSWORD"]; ok {
		h.cfg.ViewerPassword = v
	}
}

// BumpSessionGen increments the session generation so all existing tokens fail validation (e.g. after password or TOTP change).
func (h *Handler) BumpSessionGen() {
	h.sessionGen.Add(1)
	log.Println("[INFO] Session generation bumped; existing sessions invalidated")
}

// Router returns the HTTP handler with all routes registered.
// Routes enforce RBAC:
//   - viewer:   read-only (GET status, config, metrics, activity, prerequisites)
//   - operator: viewer + restart services
//   - admin:    full access (config changes, token generation)
func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	adminOnly := []string{"admin"}
	operatorUp := []string{"admin", "operator"}
	anyRole := []string{"admin", "operator", "viewer"}

	// ── API Routes ──
	mux.HandleFunc("GET /api/v1/health", h.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", h.handleLogin)
	mux.HandleFunc("GET /api/v1/prerequisites", middleware.RequireRole(anyRole, h.handlePrerequisites))
	mux.HandleFunc("GET /api/v1/status", middleware.RequireRole(anyRole, h.handleStatus))
	mux.HandleFunc("GET /api/v1/config", middleware.RequireRole(anyRole, h.handleGetConfig))
	mux.HandleFunc("PUT /api/v1/config", middleware.RequireRole(adminOnly, h.handlePutConfig))
	mux.HandleFunc("POST /api/v1/services/{name}/restart", middleware.RequireRole(operatorUp, h.handleServiceRestart))
	mux.HandleFunc("POST /api/v1/gateway/token", middleware.RequireRole(adminOnly, h.handleGatewayToken))
	mux.HandleFunc("GET /api/v1/gateway/metrics", middleware.RequireRole(anyRole, h.handleGatewayMetrics))
	mux.HandleFunc("GET /api/v1/gateway/activity", middleware.RequireRole(anyRole, h.handleGatewayActivity))
	mux.HandleFunc("GET /api/v1/gateway/usage", middleware.RequireRole(anyRole, h.handleGatewayUsage))

	// ── SPA Fallback ──
	// Serve React app for all non-API routes
	mux.Handle("/", h.spaHandler())

	return mux
}

// ─── Health Check ────────────────────────────────────────────

type healthResponse struct {
	Status     string `json:"status"`
	Service    string `json:"service"`
	Version    string `json:"version"`
	Uptime     string `json:"uptime"`
	NeedsSetup bool   `json:"needs_setup"`
}

var startTime = time.Now()

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:     "ok",
		Service:    "safepaw-wizard",
		Version:    "0.1.0",
		Uptime:     time.Since(startTime).Round(time.Second).String(),
		NeedsSetup: h.needsSetup(),
	})
}

// needsSetup returns true when critical configuration is missing.
// Checks: at least one LLM API key must be configured.
func (h *Handler) needsSetup() bool {
	env, err := readEnvFile(h.cfg.EnvFilePath)
	if err != nil {
		// If we can't read the env file, setup is definitely needed
		return true
	}
	// At least one LLM API key must be present
	if env["ANTHROPIC_API_KEY"] != "" || env["OPENAI_API_KEY"] != "" {
		return false
	}
	return true
}

// ─── Authentication ──────────────────────────────────────────

type loginRequest struct {
	Password string `json:"password"`
	TOTP     string `json:"totp,omitempty"` // Required when MFA (WIZARD_TOTP_SECRET) is enabled
}

type loginResponse struct {
	ExpiresIn int    `json:"expires_in"` // seconds
	Role      string `json:"role"`       // "admin", "operator", or "viewer"
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1KB to prevent memory exhaustion attacks
	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid request body"})
		return
	}

	// Determine role by matching password (check all configured passwords)
	role := h.matchPasswordToRole(req.Password)
	if role == "" {
		ip := clientIP(r)
		log.Printf("[WARN] Failed login attempt from %s", sanitizeLog(ip))
		h.audit.LoginFailure(ip, "invalid_password")
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, http.StatusUnauthorized, errorResponse{"invalid password"})
		return
	}

	// When MFA is enabled, require and validate TOTP code (admin and operator only)
	if h.cfg.TOTPSecret != "" && role != "viewer" {
		if req.TOTP == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{"totp_required"})
			return
		}
		if !totp.Validate(h.cfg.TOTPSecret, req.TOTP) {
			ip := clientIP(r)
			log.Printf("[WARN] Failed TOTP verification from %s", sanitizeLog(ip))
			h.audit.LoginFailure(ip, "invalid_totp")
			time.Sleep(500 * time.Millisecond)
			writeJSON(w, http.StatusUnauthorized, errorResponse{"invalid totp code"})
			return
		}
	}

	// Generate signed session token (24h TTL); include current gen so credential rotation invalidates old tokens
	const ttl = 24 * time.Hour
	token, err := session.Create(h.cfg.SessionSecret, ttl, int(h.sessionGen.Load()%uint64(^uint(0)>>1)), role) //nolint:gosec // #nosec G115 -- sessionGen is a small counter
	if err != nil {
		log.Printf("[ERROR] Failed to create session token: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{"internal error"})
		return
	}

	ip := clientIP(r)
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

	// Set CSRF cookie (readable by JS, not HttpOnly) for double-submit protection
	csrfToken, err := middleware.GenerateCSRFToken()
	if err != nil {
		log.Printf("[ERROR] Failed to generate CSRF token: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{"internal error"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false, // Must be readable by JavaScript
		SameSite: http.SameSiteStrictMode,
		Secure:   h.cfg.SecureCookies,
		MaxAge:   int(ttl.Seconds()),
	})

	writeJSON(w, http.StatusOK, loginResponse{
		ExpiresIn: int(ttl.Seconds()),
		Role:      role,
	})
}

// matchPasswordToRole checks the given password against all configured passwords
// and returns the corresponding role. Returns "" if no match.
// Uses constant-time comparison for all checks to prevent timing attacks.
func (h *Handler) matchPasswordToRole(password string) string {
	adminMatch := subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.AdminPassword)) == 1

	operatorMatch := false
	if h.cfg.OperatorPassword != "" {
		operatorMatch = subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.OperatorPassword)) == 1
	}

	viewerMatch := false
	if h.cfg.ViewerPassword != "" {
		viewerMatch = subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.ViewerPassword)) == 1
	}

	// Priority: admin > operator > viewer (in case same password used for multiple roles)
	switch {
	case adminMatch:
		return "admin"
	case operatorMatch:
		return "operator"
	case viewerMatch:
		return "viewer"
	default:
		return ""
	}
}

// ─── Prerequisites Check ─────────────────────────────────────

type prerequisiteCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // "pass", "fail", "warn"
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
// inContainer reports whether the process is running inside a Docker container.
func inContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func checkPorts(ports ...int) prerequisiteCheck {
	// When running inside the compose stack the wizard and gateway already
	// own these ports — a bind-test would always fail for them.
	if inContainer() {
		return prerequisiteCheck{
			Name:     "Port Availability",
			Status:   "pass",
			Message:  fmt.Sprintf("Ports %s managed by Docker Compose", joinInts(ports)),
			Required: true,
		}
	}

	var busy []string
	for _, port := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			busy = append(busy, fmt.Sprintf("%d", port))
		} else {
			_ = ln.Close()
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
	// Use plain 'df' (busybox-compatible) and parse the Available column (KB).
	out, err := exec.Command("df", "/").Output()
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

	// Busybox df output: Filesystem 1K-blocks Used Available Use% Mounted on
	// Available is the 4th column in KB.
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return prerequisiteCheck{
			Name:     "Disk Space",
			Status:   "warn",
			Message:  "Unable to parse disk space output",
			Required: false,
		}
	}
	var kb int
	if _, err := fmt.Sscanf(fields[3], "%d", &kb); err == nil {
		gb := kb / (1024 * 1024)
		if gb >= 2 {
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
			Message:  fmt.Sprintf("Low disk space: %dGB (recommend 2GB+)", gb),
			Required: false,
		}
	}

	return prerequisiteCheck{
		Name:     "Disk Space",
		Status:   "warn",
		Message:  "Unable to parse disk space output",
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

	restartIP := clientIP(r)

	if err := h.docker.RestartContainer(ctx, containerName, 10); err != nil {
		log.Printf("[WARN] Restart %s failed: %v", containerName, err)
		h.audit.ServiceRestart(restartIP, name, "failure")
		writeJSON(w, http.StatusInternalServerError, errorResponse{"restart failed; check server logs for details"})
		return
	}

	h.audit.ServiceRestart(restartIP, name, "success")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": name})
}

// ─── Gateway Token ───────────────────────────────────────────

type gatewayTokenRequest struct {
	Subject string `json:"subject"` // e.g. "wizard-proxy"
	Scope   string `json:"scope"`   // e.g. "proxy", "admin"
	TTLHrs  int    `json:"ttl_hours,omitempty"`
}

type gatewayTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// handleGatewayToken generates a gateway-compatible HMAC-SHA256 token.
// Reads AUTH_SECRET from .env at call time so it always uses the current key.
func (h *Handler) handleGatewayToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var req gatewayTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid request body"})
		return
	}

	if req.Subject == "" {
		req.Subject = "wizard-proxy"
	}
	if req.Scope == "" {
		req.Scope = "proxy"
	}
	if req.TTLHrs <= 0 {
		req.TTLHrs = 24
	}
	if req.TTLHrs > 168 { // max 7 days
		writeJSON(w, http.StatusBadRequest, errorResponse{"ttl_hours must be <= 168 (7 days)"})
		return
	}

	// Read AUTH_SECRET from .env
	env, err := readEnvFile(h.cfg.EnvFilePath)
	if err != nil {
		log.Printf("[ERROR] handleGatewayToken: cannot read env: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{"cannot read configuration"})
		return
	}
	secret := env["AUTH_SECRET"]
	if secret == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{"AUTH_SECRET is not configured; set it in Settings first"})
		return
	}
	if len(secret) < 32 {
		writeJSON(w, http.StatusBadRequest, errorResponse{"AUTH_SECRET must be at least 32 characters"})
		return
	}

	// Create the token using the same format as the gateway
	now := time.Now().Unix()
	ttl := time.Duration(req.TTLHrs) * time.Hour
	exp := now + int64(ttl.Seconds())

	payload := map[string]interface{}{
		"sub":   req.Subject,
		"iat":   now,
		"exp":   exp,
		"scope": req.Scope,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"internal error"})
		return
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadBytes)
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	token := payloadB64 + "." + sigB64

	ip := clientIP(r)
	h.audit.Log(ip, "gateway_token_created", "gateway", "success", map[string]string{
		"subject": req.Subject, "scope": req.Scope, "ttl_hours": fmt.Sprintf("%d", req.TTLHrs),
	})

	writeJSON(w, http.StatusOK, gatewayTokenResponse{
		Token:     token,
		ExpiresAt: time.Unix(exp, 0).UTC().Format(time.RFC3339),
	})
}

// ─── Gateway Metrics ─────────────────────────────────────────

type gatewayMetricsSummary struct {
	TotalRequests    int64   `json:"total_requests"`
	AuthFailures     int64   `json:"auth_failures"`
	InjectionsFound  int64   `json:"injections_found"`
	RateLimited      int64   `json:"rate_limited"`
	ActiveConns      int64   `json:"active_connections"`
	AvgResponseMs    float64 `json:"avg_response_ms"`
	TokensRevoked    int64   `json:"tokens_revoked"`
	GatewayReachable bool    `json:"gateway_reachable"`
}

// handleGatewayMetrics fetches /metrics from the gateway and returns a parsed summary.
// The gateway /metrics endpoint is unauthenticated (exempted from auth middleware).
func (h *Handler) handleGatewayMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	gatewayURL := "http://safepaw-gateway:8080/metrics"
	// Allow override for development
	if gw := os.Getenv("GATEWAY_URL"); gw != "" {
		gatewayURL = gw + "/metrics"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", gatewayURL, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, gatewayMetricsSummary{GatewayReachable: false})
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, gatewayMetricsSummary{GatewayReachable: false})
		return
	}
	defer resp.Body.Close()

	// Read at most 1MB of metrics text
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusOK, gatewayMetricsSummary{GatewayReachable: false})
		return
	}

	summary := parsePrometheusMetrics(string(body))
	summary.GatewayReachable = true
	writeJSON(w, http.StatusOK, summary)
}

// parsePrometheusMetrics extracts key counters from Prometheus text format.
func parsePrometheusMetrics(text string) gatewayMetricsSummary {
	var s gatewayMetricsSummary

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse "metric_name{labels} value" or "metric_name value"
		name, value := parseMetricLine(line)
		switch name {
		case "safepaw_requests_total":
			s.TotalRequests += parseInt64(value)
		case "safepaw_auth_failures_total":
			s.AuthFailures += parseInt64(value)
		case "safepaw_injection_detected_total":
			s.InjectionsFound += parseInt64(value)
		case "safepaw_rate_limited_total":
			s.RateLimited += parseInt64(value)
		case "safepaw_active_connections":
			s.ActiveConns = parseInt64(value)
		case "safepaw_tokens_revoked_total":
			s.TokensRevoked += parseInt64(value)
		case "safepaw_request_duration_seconds_sum":
			s.AvgResponseMs = parseFloat64(value) * 1000
		}
	}

	// Convert sum to average (rough estimate if we have count)
	// We'll refine this once we know the exact metric names
	return s
}

// parseMetricLine splits "name{labels} value" into name and value.
func parseMetricLine(line string) (name, value string) {
	// Strip labels: "name{label=val} 123" → "name" and "123"
	idx := strings.IndexByte(line, '{')
	if idx >= 0 {
		name = line[:idx]
		// Find closing brace, value is after space
		braceEnd := strings.IndexByte(line[idx:], '}')
		if braceEnd >= 0 {
			rest := strings.TrimSpace(line[idx+braceEnd+1:])
			// Value may have timestamp after space
			if sp := strings.IndexByte(rest, ' '); sp >= 0 {
				value = rest[:sp]
			} else {
				value = rest
			}
		}
	} else {
		// Simple "name value" format
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name = parts[0]
			value = parts[1]
		}
	}
	return
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ─── Gateway Activity ────────────────────────────────────────

type gatewayActivity struct {
	Metrics   gatewayMetricsSummary `json:"metrics"`
	TopPaths  []pathCount           `json:"top_paths"`
	RecentIPs []string              `json:"recent_ips"`
}

type pathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// handleGatewayActivity returns an activity summary built from gateway metrics.
// This is a higher-level view intended for the Activity page.
func (h *Handler) handleGatewayActivity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	gatewayURL := "http://safepaw-gateway:8080/metrics"
	if gw := os.Getenv("GATEWAY_URL"); gw != "" {
		gatewayURL = gw + "/metrics"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", gatewayURL, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, gatewayActivity{})
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, gatewayActivity{})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusOK, gatewayActivity{})
		return
	}

	metricsText := string(body)
	summary := parsePrometheusMetrics(metricsText)
	summary.GatewayReachable = true

	topPaths := parseTopPaths(metricsText)

	writeJSON(w, http.StatusOK, gatewayActivity{
		Metrics:  summary,
		TopPaths: topPaths,
	})
}

// parseTopPaths extracts per-path request counts from safepaw_requests_total{path="..."} lines.
func parseTopPaths(text string) []pathCount {
	pathMap := make(map[string]int64)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "safepaw_requests_total{") {
			continue
		}
		// Extract path label
		pathLabel := extractLabel(line, "path")
		if pathLabel == "" {
			continue
		}
		_, value := parseMetricLine(line)
		pathMap[pathLabel] += parseInt64(value)
	}

	// Convert to sorted slice (top N)
	result := make([]pathCount, 0, len(pathMap))
	for p, c := range pathMap {
		result = append(result, pathCount{Path: p, Count: c})
	}
	// Simple insertion sort (small N)
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].Count > result[j-1].Count; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

// extractLabel finds label="value" in a Prometheus metric line.
func extractLabel(line, label string) string {
	key := label + `="`
	idx := strings.Index(line, key)
	if idx < 0 {
		return ""
	}
	start := idx + len(key)
	end := strings.IndexByte(line[start:], '"')
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}

// ─── Gateway Usage Proxy ─────────────────────────────────────

// handleGatewayUsage proxies the gateway's /admin/usage endpoint, which returns
// OpenClaw cost/token data. The gateway requires admin-scoped auth, so we mint
// a short-lived HMAC token using AUTH_SECRET from .env.
func (h *Handler) handleGatewayUsage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	gatewayBase := "http://safepaw-gateway:8080"
	if gw := os.Getenv("GATEWAY_URL"); gw != "" {
		gatewayBase = gw
	}

	// Read AUTH_SECRET to mint a gateway admin token
	env, err := readEnvFile(h.cfg.EnvFilePath)
	if err != nil {
		log.Printf("[WARN] handleGatewayUsage: cannot read env: %v", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}
	secret := env["AUTH_SECRET"]
	if secret == "" || len(secret) < 32 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}

	// Build a short-lived admin-scoped token (same format as handleGatewayToken)
	now := time.Now().Unix()
	payload, _ := json.Marshal(map[string]interface{}{
		"sub": "wizard-usage-proxy", "iat": now, "exp": now + 30, "scope": "admin",
	})
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	token := payloadB64 + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, "GET", gatewayBase+"/admin/usage", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unavailable"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
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

// sanitizeLog strips control characters from a string before logging
// to prevent log injection attacks (gosec G706).
func sanitizeLog(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
