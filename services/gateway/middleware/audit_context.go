// =============================================================
// SafePaw Gateway - Per-Request Security Audit Context
// =============================================================
// A SecurityContext rides on every request via context.Context.
// Each middleware writes its decision (1-2 lines) so the
// AuditEmitter can log a single enriched JSON record when the
// request completes.
//
// Existing log.Printf calls are untouched — this is additive.
// =============================================================

package middleware

import (
	"context"
	"net/http"
	"time"
)

type contextKey struct{}

// SecurityContext collects per-request audit data from each middleware.
type SecurityContext struct {
	// Request identity
	RequestID string    `json:"request_id"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	RemoteIP  string    `json:"remote_ip"`
	StartTime time.Time `json:"-"`

	// Auth decision
	Auth *AuthDecision `json:"auth,omitempty"`

	// Input scan decision
	InputScan *ScanDecision `json:"input_scan,omitempty"`

	// Output scan decision
	OutputScan *ScanDecision `json:"output_scan,omitempty"`

	// Rate limit decision
	RateLimit *RateLimitDecision `json:"rate_limit,omitempty"`

	// Brute force decision
	BruteForce *BruteForceDecision `json:"brute_force,omitempty"`

	// Proxy result
	Proxy *ProxyResult `json:"proxy,omitempty"`
}

// AuthDecision records the authentication outcome.
type AuthDecision struct {
	Outcome string `json:"outcome"`          // allow, reject_missing, reject_invalid, reject_revoked, reject_scope, skipped
	Sub     string `json:"sub,omitempty"`    // subject from token
	Scope   string `json:"scope,omitempty"`  // token scope
	Reason  string `json:"reason,omitempty"` // failure reason
}

// ScanDecision records an input or output scan result.
type ScanDecision struct {
	Risk      string   `json:"risk"`                // none, low, medium, high
	Triggers  []string `json:"triggers,omitempty"`  // pattern names that fired
	Sanitized bool     `json:"sanitized,omitempty"` // whether output was rewritten
}

// RateLimitDecision records whether the request was rate-limited.
type RateLimitDecision struct {
	Allowed bool `json:"allowed"`
}

// BruteForceDecision records whether the IP was banned.
type BruteForceDecision struct {
	Banned bool   `json:"banned"`
	Reason string `json:"reason,omitempty"`
}

// ProxyResult records backend proxy timing.
type ProxyResult struct {
	StatusCode int   `json:"status_code"`
	BackendMs  int64 `json:"backend_ms"`
	WebSocket  bool  `json:"websocket,omitempty"`
}

// NewSecurityContext creates a fresh audit context for a request.
func NewSecurityContext(r *http.Request) *SecurityContext {
	return &SecurityContext{
		RequestID: r.Header.Get("X-Request-ID"),
		Method:    r.Method,
		Path:      r.URL.Path,
		RemoteIP:  extractIP(r),
		StartTime: time.Now(),
	}
}

// WithSecurityContext attaches a SecurityContext to a request.
func WithSecurityContext(r *http.Request, sc *SecurityContext) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), contextKey{}, sc))
}

// GetSecurityContext retrieves the SecurityContext from a request, or nil.
func GetSecurityContext(r *http.Request) *SecurityContext {
	sc, _ := r.Context().Value(contextKey{}).(*SecurityContext)
	return sc
}
