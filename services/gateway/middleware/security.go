// =============================================================
// SafePaw Gateway - Security Middleware
// =============================================================
// Defense-in-depth HTTP middleware applied BEFORE any WebSocket
// upgrade happens. Multiple layers of protection:
//
// 1. Security headers (HSTS, CSP, X-Frame-Options, etc.)
// 2. Origin validation (prevents CSRF on WebSocket)
// 3. Rate limiting (per-IP connection throttle)
// 4. Request ID injection (for tracing/debugging)
//
// OPSEC Lesson #9: "Defense in depth" means never relying on
// a single security check. If one layer fails, the next catches it.
// Like a castle with a moat, wall, AND archers.
// =============================================================

package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ================================================================
// Layer 1: Security Headers
// ================================================================

// SecurityHeaders adds hardened HTTP headers to every response.
// These headers tell browsers to enforce strict security policies.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[SECURITY] SecurityHeaders applied for %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Prevent clickjacking — never allow this page in an iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing — browser must trust Content-Type
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Enable XSS filter in older browsers
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Strict Transport Security — only sent when the request arrived over TLS
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy — strict fallback for gateway-generated
		// responses (401, 403, 429, 502 error pages). When the reverse proxy
		// forwards a backend response, the backend's own CSP header is also
		// present; per the CSP spec, multiple CSP headers intersect (most
		// restrictive wins), so this fallback never weakens the backend's policy.
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; frame-ancestors 'none'")

		// Don't leak referrer info to other sites
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Disable browser features we don't need
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Remove Go's default Server header (don't reveal tech stack)
		w.Header().Del("Server")

		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Layer 2: Origin Validation
// ================================================================

// OriginCheck validates the Origin header on WebSocket upgrade
// requests. This prevents Cross-Site WebSocket Hijacking (CSWSH).
func OriginCheck(allowedOrigins []string, next http.Handler) http.Handler {
	// Build a map for O(1) lookup
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// If no origins configured, auto-allow localhost variants.
		// This prevents cross-port blocking when the Wizard (e.g. :3000)
		// connects to the Gateway (e.g. :8080) during local development.
		if len(allowed) == 0 {
			if origin == "" || isLocalhostOrigin(origin) || isCodespacesOrigin(origin) {
				next.ServeHTTP(w, r)
				return
			}
			// Has non-localhost Origin but no allowlist — block
			log.Printf("[SECURITY] Blocked request with Origin=%q (no allowed origins configured, set ALLOWED_ORIGINS) request_id=%s", origin, r.Header.Get("X-Request-ID"))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Check against allowlist
		if origin != "" && !allowed[origin] {
			log.Printf("[SECURITY] Blocked request from unauthorized Origin=%q request_id=%s", origin, r.Header.Get("X-Request-ID"))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isLocalhostOrigin returns true for localhost/127.0.0.1/[::1] origins
// on any port. Used to auto-allow local dev cross-port requests.
func isLocalhostOrigin(origin string) bool {
	origin = strings.ToLower(origin)
	// Strip scheme
	for _, prefix := range []string{"https://", "http://"} {
		origin = strings.TrimPrefix(origin, prefix)
	}
	// Strip port
	if idx := strings.LastIndex(origin, ":"); idx > 0 {
		origin = origin[:idx]
	}
	return origin == "localhost" || origin == "127.0.0.1" || origin == "[::1]" || origin == "::1"
}

// isCodespacesOrigin returns true for GitHub Codespaces port-forwarded origins
// (e.g. https://xxx-8080.app.github.dev). These are trusted because Codespaces
// port forwarding is authenticated by GitHub.
func isCodespacesOrigin(origin string) bool {
	origin = strings.ToLower(origin)
	for _, prefix := range []string{"https://", "http://"} {
		origin = strings.TrimPrefix(origin, prefix)
	}
	return strings.HasSuffix(origin, ".app.github.dev")
}

// ================================================================
// Layer 3: Per-IP Rate Limiter
// ================================================================

// ipRecord tracks connection attempts per IP address.
type ipRecord struct {
	count    int
	lastSeen time.Time
}

// RateLimiter limits how many connections a single IP can open
// in a given time window. Prevents resource exhaustion attacks.
type RateLimiter struct {
	mu       sync.Mutex
	records  map[string]*ipRecord
	limit    int           // Max connections per window
	window   time.Duration // Time window
	cleanupT *time.Ticker  // Background cleanup
}

// NewRateLimiter creates a rate limiter.
// limit=10, window=1m means: max 10 new connections per minute per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		records:  make(map[string]*ipRecord),
		limit:    limit,
		window:   window,
		cleanupT: time.NewTicker(window),
	}

	// Background goroutine cleans up expired entries to prevent memory leak
	go func() {
		for range rl.cleanupT.C {
			rl.cleanup()
		}
	}()

	return rl
}

// Allow checks if an IP is within its rate limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, exists := rl.records[ip]
	now := time.Now()

	if !exists || now.Sub(rec.lastSeen) > rl.window {
		// First request or window expired — reset
		rl.records[ip] = &ipRecord{count: 1, lastSeen: now}
		log.Printf("[RATELIMIT] New window for ip=%s (1/%d)", ip, rl.limit)
		return true
	}

	if rec.count >= rl.limit {
		log.Printf("[RATELIMIT] DENIED ip=%s (%d/%d reached)", ip, rec.count, rl.limit)
		return false
	}

	rec.count++
	rec.lastSeen = now
	log.Printf("[RATELIMIT] Allowed ip=%s (%d/%d)", SanitizeLogValue(ip), rec.count, rl.limit)
	return true
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, rec := range rl.records {
		if now.Sub(rec.lastSeen) > rl.window {
			delete(rl.records, ip)
		}
	}
}

// Stop stops the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	rl.cleanupT.Stop()
}

// RateLimit wraps a handler with per-IP rate limiting.
func RateLimit(rl *RateLimiter, next http.Handler) http.Handler {
	return RateLimitWithGuard(rl, nil, next)
}

// RateLimitWithGuard is RateLimit + brute-force integration.
// Every rate limit denial counts as a strike. Persistent abusers
// get escalating bans from the guard.
func RateLimitWithGuard(rl *RateLimiter, guard *BruteForceGuard, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt monitoring endpoints so internal probes never get rate-limited
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		// Exempt static assets — a single page load triggers many sub-requests
		// for JS bundles, CSS, images, and fonts that shouldn't count toward
		// the rate limit.
		if isStaticAsset(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ip := extractIP(r)
		if !rl.Allow(ip) {
			log.Printf("[SECURITY] Rate limited IP=%s request_id=%s", SanitizeLogValue(ip), SanitizeLogValue(r.Header.Get("X-Request-ID")))
			if sc := GetSecurityContext(r); sc != nil {
				sc.RateLimit = &RateLimitDecision{Allowed: false}
			}
			if guard != nil {
				guard.RecordFailure(ip, "rate_limit_exceeded")
			}
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		if sc := GetSecurityContext(r); sc != nil {
			sc.RateLimit = &RateLimitDecision{Allowed: true}
		}
		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Layer 4: Request ID
// ================================================================

// RequestID injects a unique server-generated UUID into every request for tracing.
// Client-provided X-Request-ID is ignored so that IDs are always unique and
// server-controlled (prevents log injection and correlation confusion).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := uuid.New().String()
		r.Header.Set("X-Request-ID", reqID) // Downstream middleware and handlers use this for logs
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Layer 5: Strip Auth Headers (when auth is disabled)
// ================================================================

// StripAuthHeaders removes X-Auth-Subject and X-Auth-Scope from incoming
// requests. Without this, when AUTH_ENABLED=false, any client can send
// these headers and ws/handler.go will treat them as authenticated identity.
func StripAuthHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Auth-Subject")
		r.Header.Del("X-Auth-Scope")
		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Helpers
// ================================================================

// extractIP gets the real client IP, handling proxies.
// Only trusts X-Real-IP from loopback addresses (reverse proxy on same host).
// Without this check, any client can spoof X-Real-IP to bypass rate limiting.
func extractIP(r *http.Request) string {
	// Only trust proxy headers from loopback (nginx/caddy on same host)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if isLoopback(host) {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return ip
		}
	}

	return host
}

// isLoopback checks if an IP is a loopback address (127.x.x.x or ::1).
func isLoopback(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

// SanitizeLogValue strips control characters (newlines, tabs, etc.) from
// a string before it is interpolated into a log message. This prevents
// log injection attacks where an attacker embeds \n or \r in request
// fields like URL path or RemoteAddr to forge log entries.
func SanitizeLogValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1 // drop control characters
		}
		return r
	}, s)
}
