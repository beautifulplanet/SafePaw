// =============================================================
// SafePaw Gateway - Audit Emitter Middleware
// =============================================================
// Wraps the outermost layer of the middleware chain. When the
// request completes, it emits a single structured JSON audit
// line to stdout containing every security decision that was
// recorded in the SecurityContext.
//
// Works in both text and JSON LOG_FORMAT modes — the audit line
// is always JSON for machine consumption.
// =============================================================

package middleware

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// auditRecord is the final shape emitted as one JSON line.
type auditRecord struct {
	Timestamp  string              `json:"ts"`
	Type       string              `json:"type"`
	RequestID  string              `json:"request_id"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	RemoteIP   string              `json:"remote_ip"`
	StatusCode int                 `json:"status_code"`
	DurationMs int64               `json:"duration_ms"`
	Auth       *AuthDecision       `json:"auth,omitempty"`
	InputScan  *ScanDecision       `json:"input_scan,omitempty"`
	OutputScan *ScanDecision       `json:"output_scan,omitempty"`
	RateLimit  *RateLimitDecision  `json:"rate_limit,omitempty"`
	BruteForce *BruteForceDecision `json:"brute_force,omitempty"`
	Proxy      *ProxyResult        `json:"proxy,omitempty"`
}

// statusCapture wraps ResponseWriter to capture the status code.
type statusCapture struct {
	http.ResponseWriter
	code    int
	written bool
}

func (sc *statusCapture) WriteHeader(code int) {
	if !sc.written {
		sc.code = code
		sc.written = true
	}
	sc.ResponseWriter.WriteHeader(code)
}

func (sc *statusCapture) Write(b []byte) (int, error) {
	if !sc.written {
		sc.code = http.StatusOK
		sc.written = true
	}
	return sc.ResponseWriter.Write(b)
}

// Hijack delegates to the underlying ResponseWriter so WebSocket
// upgrades can obtain the raw connection through the audit wrapper.
func (sc *statusCapture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sc.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// AuditEmitter is middleware that creates a SecurityContext, attaches
// it to the request, and emits the audit record when the request completes.
// It should be placed just inside RequestID (needs X-Request-ID) and
// outside all security middleware (captures all decisions).
func AuditEmitter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip audit for health/metrics (high frequency, low value)
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		sc := NewSecurityContext(r)
		r = WithSecurityContext(r, sc)

		capture := &statusCapture{ResponseWriter: w, code: http.StatusOK}

		next.ServeHTTP(capture, r)

		rec := auditRecord{
			Timestamp:  sc.StartTime.UTC().Format(time.RFC3339Nano),
			Type:       "gateway_audit",
			RequestID:  sc.RequestID,
			Method:     sc.Method,
			Path:       sc.Path,
			RemoteIP:   sc.RemoteIP,
			StatusCode: capture.code,
			DurationMs: time.Since(sc.StartTime).Milliseconds(),
			Auth:       sc.Auth,
			InputScan:  sc.InputScan,
			OutputScan: sc.OutputScan,
			RateLimit:  sc.RateLimit,
			BruteForce: sc.BruteForce,
			Proxy:      sc.Proxy,
		}

		line, err := json.Marshal(rec)
		if err != nil {
			log.Printf("[AUDIT] marshal error: %v", err)
			return
		}
		log.Printf("[AUDIT] %s", line)
	})
}
