// =============================================================
// SafePaw Gateway - Secure Reverse Proxy
// =============================================================
// Security-hardened reverse proxy that sits in front of OpenClaw.
//
// What it does:
// 1. Loads configuration from environment variables
// 2. Creates a reverse proxy to the OpenClaw backend
// 3. Applies security middleware (headers, rate limit, origin, auth)
// 4. Scans request bodies for prompt injection patterns
// 5. Proxies all HTTP and WebSocket traffic to OpenClaw
// 6. Handles graceful shutdown (SIGINT/SIGTERM)
//
// SafePaw Gateway is the security perimeter around OpenClaw.
// Every request passes through the defense layers first.
// =============================================================

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"safepaw/gateway/config"
	"safepaw/gateway/middleware"
)

func main() {
log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
log.Println("=== SafePaw Gateway starting ===")

// --------------------------------------------------------
// Step 1: Load configuration
// --------------------------------------------------------
cfg, err := config.Load()
if err != nil {
log.Fatalf("[FATAL] Config load failed: %v", err)
}
log.Printf("[CONFIG] Port=%d ProxyTarget=%s TLS=%v Auth=%v",
cfg.Port, cfg.ProxyTarget.String(), cfg.TLSEnabled, cfg.AuthEnabled)

// --------------------------------------------------------
// Step 2: Create reverse proxy to OpenClaw
// --------------------------------------------------------
proxy := &httputil.ReverseProxy{
Director: func(req *http.Request) {
req.URL.Scheme = cfg.ProxyTarget.Scheme
req.URL.Host = cfg.ProxyTarget.Host
req.Host = cfg.ProxyTarget.Host

// Preserve the original path  proxy everything
if cfg.ProxyTarget.Path != "" && cfg.ProxyTarget.Path != "/" {
req.URL.Path = singleJoiningSlash(cfg.ProxyTarget.Path, req.URL.Path)
}

// Strip hop-by-hop headers that shouldn't be forwarded
req.Header.Del("X-SafePaw-Risk") // Don't let clients spoof risk headers

log.Printf("[PROXY] %s %s -> %s%s (remote=%s)",
req.Method, req.URL.Path, cfg.ProxyTarget.Host, req.URL.Path, req.RemoteAddr)
},
ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
log.Printf("[PROXY] Backend error: %v (path=%s remote=%s)", err, r.URL.Path, r.RemoteAddr)
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusBadGateway)
json.NewEncoder(w).Encode(map[string]string{
"error":   "bad_gateway",
"message": "OpenClaw backend is unavailable",
})
},
// Flush immediately for streaming responses (SSE, etc.)
FlushInterval: -1,
}

// --------------------------------------------------------
// Step 3: Set up rate limiter
// --------------------------------------------------------
rateLimiter := middleware.NewRateLimiter(cfg.RateLimit, cfg.RateLimitWindow)
defer rateLimiter.Stop()

// --------------------------------------------------------
// Step 4: Build HTTP routes with middleware stack
// --------------------------------------------------------
mux := http.NewServeMux()

// Health check  no auth, no middleware (used by Docker/k8s)
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
status := map[string]interface{}{
"status":    "ok",
"service":   "safepaw-gateway",
"proxy":     cfg.ProxyTarget.String(),
"timestamp": time.Now().UTC().Format(time.RFC3339),
}

// Deep health check: probe the OpenClaw backend
ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
defer cancel()

healthURL := cfg.ProxyTarget.String() + "/health"
req, _ := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
resp, err := http.DefaultClient.Do(req)
if err != nil {
status["status"] = "degraded"
status["backend"] = "unreachable"
w.WriteHeader(http.StatusServiceUnavailable)
} else {
resp.Body.Close()
if resp.StatusCode == http.StatusOK {
status["backend"] = "healthy"
} else {
status["status"] = "degraded"
status["backend"] = fmt.Sprintf("status_%d", resp.StatusCode)
w.WriteHeader(http.StatusServiceUnavailable)
}
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(status)
})

// Everything else -> reverse proxy to OpenClaw
mux.Handle("/", bodyScanner(cfg.MaxBodySize, proxy))

// Apply middleware (outermost first):
// Request -> SecurityHeaders -> RequestID -> OriginCheck -> RateLimit -> [Auth] -> BodyScanner -> Proxy
var handler http.Handler = mux

// Auth middleware (only if enabled  disabled in dev by default)
if cfg.AuthEnabled {
auth, err := middleware.NewAuthenticator(cfg.AuthSecret, cfg.AuthDefaultTTL, cfg.AuthMaxTTL)
if err != nil {
log.Fatalf("[FATAL] Auth setup failed: %v", err)
}
log.Printf("[AUTH] Authentication ENABLED (default TTL=%v, max TTL=%v)",
cfg.AuthDefaultTTL, cfg.AuthMaxTTL)
// Use "proxy" scope  accepts "proxy" and "admin" tokens
handler = middleware.AuthRequired(auth, "proxy", handler)
} else {
log.Println("[AUTH] Authentication DISABLED (set AUTH_ENABLED=true for production)")
handler = middleware.StripAuthHeaders(handler)
}

handler = middleware.RateLimit(rateLimiter, handler)
handler = middleware.OriginCheck(cfg.AllowedOrigins, handler)
handler = middleware.RequestID(handler)
handler = middleware.SecurityHeaders(handler)

// --------------------------------------------------------
// Step 5: Create and start HTTP server
// --------------------------------------------------------
server := &http.Server{
Addr:         fmt.Sprintf(":%d", cfg.Port),
Handler:      handler,
ReadTimeout:  cfg.ReadTimeout,
WriteTimeout: cfg.WriteTimeout,
IdleTimeout:  cfg.IdleTimeout,
MaxHeaderBytes: 1 << 16, // 64KB
}

go func() {
if cfg.TLSEnabled {
server.TLSConfig = &tls.Config{
MinVersion: tls.VersionTLS12,
CurvePreferences: []tls.CurveID{
tls.X25519,
tls.CurveP256,
},
CipherSuites: []uint16{
tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
},
}
server.Addr = fmt.Sprintf(":%d", cfg.TLSPort)
log.Printf("[SERVER] Listening on :%d (TLS ENABLED)", cfg.TLSPort)
if err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
log.Fatalf("[FATAL] TLS server error: %v", err)
}
} else {
log.Printf("[SERVER] Listening on :%d (TLS disabled  dev mode)", cfg.Port)
if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
log.Fatalf("[FATAL] Server error: %v", err)
}
}
}()

// --------------------------------------------------------
// Step 6: Graceful shutdown
// --------------------------------------------------------
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
sig := <-quit
shutdownStart := time.Now()
log.Printf("[SHUTDOWN] Received signal: %v", sig)

shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
defer shutdownCancel()

if err := server.Shutdown(shutdownCtx); err != nil {
log.Printf("[SHUTDOWN] Server shutdown error: %v", err)
}

log.Printf("=== SafePaw Gateway stopped (shutdown: %v) ===",
time.Since(shutdownStart).Round(time.Millisecond))
}

// bodyScanner is middleware that reads JSON request bodies on mutating
// methods (POST, PUT, PATCH) and scans for prompt injection patterns
// using the sanitize module. It adds an X-SafePaw-Risk header with
// the assessed risk level so OpenClaw (or logs) can see it.
//
// Non-JSON or GET/HEAD/OPTIONS requests pass through unscanned.
func bodyScanner(maxSize int64, next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// Only scan mutating requests with bodies
if r.Method != "POST" && r.Method != "PUT" && r.Method != "PATCH" {
next.ServeHTTP(w, r)
return
}

// Only scan JSON content types
ct := r.Header.Get("Content-Type")
if !strings.Contains(ct, "application/json") && !strings.Contains(ct, "text/") {
next.ServeHTTP(w, r)
return
}

// Read body (with size limit)
if r.Body == nil {
next.ServeHTTP(w, r)
return
}

bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
r.Body.Close()
if err != nil {
log.Printf("[SCANNER] Body read error: %v (remote=%s)", err, r.RemoteAddr)
next.ServeHTTP(w, r)
return
}

// Restore the body so the proxy can forward it
r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
r.ContentLength = int64(len(bodyBytes))

// Scan for prompt injection
bodyStr := string(bodyBytes)
risk, triggers := middleware.AssessPromptInjectionRisk(bodyStr)
if risk > middleware.RiskNone {
log.Printf("[SCANNER] Prompt injection risk=%s triggers=%v path=%s remote=%s body_len=%d",
risk, triggers, r.URL.Path, r.RemoteAddr, len(bodyBytes))
}

// Attach risk assessment as header (OpenClaw can read this)
r.Header.Set("X-SafePaw-Risk", risk.String())
if len(triggers) > 0 {
r.Header.Set("X-SafePaw-Triggers", strings.Join(triggers, ","))
}

next.ServeHTTP(w, r)
})
}

// singleJoiningSlash joins a base path and a request path with exactly one slash.
func singleJoiningSlash(a, b string) string {
aslash := strings.HasSuffix(a, "/")
bslash := strings.HasPrefix(b, "/")
switch {
case aslash && bslash:
return a + b[1:]
case !aslash && !bslash:
return a + "/" + b
}
return a + b
}
