// =============================================================
// NOPEnclaw Gateway — Security Middleware
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
		// Prevent clickjacking — never allow this page in an iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing — browser must trust Content-Type
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Enable XSS filter in older browsers
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Strict Transport Security — force HTTPS for 1 year
		// (harmless in dev over HTTP, critical in production)
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Content Security Policy — only allow resources from same origin
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

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

		// If no origins configured, allow only in dev (no Origin header = same origin)
		if len(allowed) == 0 {
			if origin == "" {
				// Same-origin request or tool like curl — allow in dev
				next.ServeHTTP(w, r)
				return
			}
			// Has Origin but no allowlist configured — block
			log.Printf("[SECURITY] Blocked request with Origin=%q (no allowed origins configured)", origin)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Check against allowlist
		if origin != "" && !allowed[origin] {
			log.Printf("[SECURITY] Blocked request from unauthorized Origin=%q", origin)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
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
		return true
	}

	if rec.count >= rl.limit {
		return false
	}

	rec.count++
	rec.lastSeen = now
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !rl.Allow(ip) {
			log.Printf("[SECURITY] Rate limited IP=%s", ip)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Layer 4: Request ID
// ================================================================

// RequestID injects a unique UUID into every request for tracing.
// This lets us track a single user's message across gateway → router → agent.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", reqID)
		// Store in request context would be even better, but this is MVP
		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Helpers
// ================================================================

// extractIP gets the real client IP, handling proxies.
func extractIP(r *http.Request) string {
	// Check X-Real-IP first (set by nginx/reverse proxy)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// Fall back to RemoteAddr (strip port)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
