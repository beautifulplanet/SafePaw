// =============================================================
// SafePaw Setup Wizard - Security Middleware
// =============================================================
// Defense-in-depth for the wizard UI:
//   - CSP headers prevent XSS
//   - CORS locked to localhost
//   - Admin auth via Bearer token or cookie
//   - Rate limiting prevents brute force
// =============================================================

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SecurityHeaders adds defense-in-depth HTTP headers.
// These protect against XSS, clickjacking, and MIME sniffing.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0") // Modern browsers: CSP > this header
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' https://fonts.googleapis.com; "+
				"img-src 'self' data:; "+
				"connect-src 'self' ws://localhost:3000 wss://localhost:3000; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// CORS handles Cross-Origin Resource Sharing.
// Locked to localhost origins only — wizard should never be accessed remotely.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.ToLower(o)] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originSet[strings.ToLower(origin)] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SessionValidator is a function that validates a session token and returns the role and validity.
// Used so the handler can supply password and session generation (invalidated on credential change).
type SessionValidator func(token string) (role string, ok bool)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const roleContextKey contextKey = iota

// GetRole extracts the authenticated role from the request context.
// Returns empty string if not authenticated.
func GetRole(r *http.Request) string {
	if v, ok := r.Context().Value(roleContextKey).(string); ok {
		return v
	}
	return ""
}

// SetRole returns a new request with the given role injected into its context.
// Intended for use by handlers and tests that need to set the role directly.
func SetRole(r *http.Request, role string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), roleContextKey, role))
}

// AdminAuth protects API endpoints with signed session tokens.
// Accepts Bearer token in Authorization header or "session" cookie.
// validate is typically handler.SessionValidator() so password and session generation stay in sync.
// On successful auth, the role is stored in the request context (accessible via GetRole).
// Static assets (UI files) are served without auth so the login page loads.
func AdminAuth(validate SessionValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization: Bearer <token>
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if role, ok := validate(token); ok {
				ctx := context.WithValue(r.Context(), roleContextKey, role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Check cookie fallback (for browser-based access)
		if cookie, err := r.Cookie("session"); err == nil {
			if role, ok := validate(cookie.Value); ok {
				ctx := context.WithValue(r.Context(), roleContextKey, role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
}

// RequireRole returns middleware that restricts access to the given roles.
// Must be used after AdminAuth (expects role in context).
func RequireRole(roles []string, next http.HandlerFunc) http.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(w http.ResponseWriter, r *http.Request) {
		role := GetRole(r)
		if !allowed[role] {
			http.Error(w, `{"error":"forbidden","required_role":"`+roles[0]+`"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// isPublicPath returns true for paths that don't require auth.
func isPublicPath(path string) bool {
	// API paths that are public
	if path == "/api/v1/auth/login" || path == "/api/v1/health" {
		return true
	}
	// All non-API paths are static UI files
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	return false
}

// ─── CSRF Protection (Double-Submit Cookie) ──────────────────

// CSRFProtect validates CSRF tokens on state-mutating requests.
// On login, the handler sets a "csrf" cookie (readable by JS).
// For POST/PUT/DELETE requests to API endpoints, the client must
// send the cookie value back as the X-CSRF-Token header.
func CSRFProtect(secureCookies bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF for safe methods, public paths, and non-API paths
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if isPublicPath(r.URL.Path) || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// If the request uses Bearer token auth (not cookies), CSRF is not applicable
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		// Cookie-based auth: require X-CSRF-Token header matching csrf cookie
		cookie, err := r.Cookie("csrf")
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"csrf_token_missing"}`, http.StatusForbidden)
			return
		}
		headerToken := r.Header.Get("X-CSRF-Token")
		if headerToken == "" || headerToken != cookie.Value {
			http.Error(w, `{"error":"csrf_token_invalid"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateCSRFToken creates a random CSRF token string.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ─── Rate Limiter ────────────────────────────────────────────

// rateLimiter tracks request counts per IP.
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string]*ipRecord
	limit    int
	window   time.Duration
	stopCh   chan struct{}
}

type ipRecord struct {
	count    int
	windowAt time.Time
}

// RateLimit returns middleware that limits requests per IP.
// This prevents brute-force attacks on the admin password.
func RateLimit(limit int, window time.Duration, next http.Handler) http.Handler {
	rl := &rateLimiter{
		requests: make(map[string]*ipRecord),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}

	// Cleanup goroutine
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-rl.stopCh:
				return
			}
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rec, ok := rl.requests[ip]
	if !ok || now.Sub(rec.windowAt) > rl.window {
		rl.requests[ip] = &ipRecord{count: 1, windowAt: now}
		return true
	}

	rec.count++
	return rec.count <= rl.limit
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for ip, rec := range rl.requests {
		if rec.windowAt.Before(cutoff) {
			delete(rl.requests, ip)
		}
	}
}

func extractIP(r *http.Request) string {
	// Trust X-Forwarded-For only behind a trusted proxy
	// For localhost wizard, use RemoteAddr directly
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ─── Login Rate Limiter / Account Lockout ────────────────────

// LoginGuard provides per-IP rate limiting and lockout specifically for the login endpoint.
// After maxAttempts failures within window, the IP is locked out for lockoutDuration.
type LoginGuard struct {
	mu              sync.Mutex
	attempts        map[string]*loginRecord
	maxAttempts     int
	window          time.Duration
	lockoutDuration time.Duration
	stopCh          chan struct{}
}

type loginRecord struct {
	failures   int
	firstFail  time.Time
	lockedAt   time.Time
	lockoutDur time.Duration
}

// NewLoginGuard creates a login guard.
func NewLoginGuard(maxAttempts int, window, lockoutDuration time.Duration) *LoginGuard {
	lg := &LoginGuard{
		attempts:        make(map[string]*loginRecord),
		maxAttempts:     maxAttempts,
		window:          window,
		lockoutDuration: lockoutDuration,
		stopCh:          make(chan struct{}),
	}
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				lg.cleanup()
			case <-lg.stopCh:
				return
			}
		}
	}()
	return lg
}

// Stop terminates the background cleanup goroutine.
func (lg *LoginGuard) Stop() {
	close(lg.stopCh)
}

// RecordFailure records a failed login attempt. Returns true if the IP is now locked out.
func (lg *LoginGuard) RecordFailure(ip string) bool {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	now := time.Now()
	rec, ok := lg.attempts[ip]
	if !ok || now.Sub(rec.firstFail) > lg.window {
		rec = &loginRecord{firstFail: now}
		lg.attempts[ip] = rec
	}
	rec.failures++
	if rec.failures >= lg.maxAttempts {
		rec.lockedAt = now
		rec.lockoutDur = lg.lockoutDuration
		return true
	}
	return false
}

// LockoutDuration returns the configured lockout duration.
func (lg *LoginGuard) LockoutDuration() time.Duration {
	return lg.lockoutDuration
}

// IsLockedOut checks if an IP is currently locked out.
func (lg *LoginGuard) IsLockedOut(ip string) (bool, time.Duration) {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	rec, ok := lg.attempts[ip]
	if !ok || rec.lockedAt.IsZero() {
		return false, 0
	}
	remaining := rec.lockoutDur - time.Since(rec.lockedAt)
	if remaining <= 0 {
		delete(lg.attempts, ip)
		return false, 0
	}
	return true, remaining
}

// ResetIP clears failure count on successful login.
func (lg *LoginGuard) ResetIP(ip string) {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	delete(lg.attempts, ip)
}

func (lg *LoginGuard) cleanup() {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	now := time.Now()
	for ip, rec := range lg.attempts {
		if !rec.lockedAt.IsZero() {
			if now.Sub(rec.lockedAt) > rec.lockoutDur {
				delete(lg.attempts, ip)
			}
		} else if now.Sub(rec.firstFail) > lg.window {
			delete(lg.attempts, ip)
		}
	}
}
