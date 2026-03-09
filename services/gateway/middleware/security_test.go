package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(okHandler())

	// Plain HTTP — HSTS must NOT be set
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	checks := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	}
	for header, want := range checks {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	// CSP should NOT be set by SecurityHeaders — the backend provides its own
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "" {
		t.Errorf("Content-Security-Policy should not be set by SecurityHeaders, got %q", csp)
	}
	if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Errorf("HSTS should not be set over plain HTTP, got %q", hsts)
	}

	// TLS — HSTS must be set
	reqTLS := httptest.NewRequest("GET", "/", nil)
	reqTLS.TLS = &tls.ConnectionState{}
	recTLS := httptest.NewRecorder()
	handler.ServeHTTP(recTLS, reqTLS)
	if got := recTLS.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS over TLS = %q, want %q", got, "max-age=31536000; includeSubDomains")
	}

	// X-Forwarded-Proto: https — HSTS must be set
	reqFwd := httptest.NewRequest("GET", "/", nil)
	reqFwd.Header.Set("X-Forwarded-Proto", "https")
	recFwd := httptest.NewRecorder()
	handler.ServeHTTP(recFwd, reqFwd)
	if got := recFwd.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS over X-Forwarded-Proto=https = %q, want %q", got, "max-age=31536000; includeSubDomains")
	}
}

func TestOriginCheck_NoOriginNoAllowlist(t *testing.T) {
	handler := OriginCheck(nil, okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("no origin + no allowlist = allow in dev, got %d", rec.Code)
	}
}

func TestOriginCheck_OriginNoAllowlist_Blocked(t *testing.T) {
	handler := OriginCheck(nil, okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("origin with no allowlist should block, got %d", rec.Code)
	}
}

func TestOriginCheck_AllowedOrigin(t *testing.T) {
	handler := OriginCheck([]string{"https://myapp.com"}, okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://myapp.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("allowed origin should pass, got %d", rec.Code)
	}
}

func TestOriginCheck_DisallowedOrigin(t *testing.T) {
	handler := OriginCheck([]string{"https://myapp.com"}, okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disallowed origin should be blocked, got %d", rec.Code)
	}
}

func TestOriginCheck_NoOriginWithAllowlist_Passes(t *testing.T) {
	handler := OriginCheck([]string{"https://myapp.com"}, okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("no origin header with allowlist should pass (same-origin), got %d", rec.Code)
	}
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	defer rl.Stop()
	for i := 0; i < 5; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	defer rl.Stop()
	for i := 0; i < 3; i++ {
		rl.Allow("10.0.0.1")
	}
	if rl.Allow("10.0.0.1") {
		t.Error("4th request should be denied")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	defer rl.Stop()
	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.1")
	if rl.Allow("10.0.0.1") {
		t.Error("IP 1 should be blocked")
	}
	if !rl.Allow("10.0.0.2") {
		t.Error("IP 2 should be allowed (different IP)")
	}
}

func TestRateLimitMiddleware_Returns429(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()
	handler := RateLimit(rl, okHandler())

	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "10.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:12346"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be 429, got %d", rec2.Code)
	}
}

func TestRequestID_Generates(t *testing.T) {
	handler := RequestID(okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	rid := rec.Header().Get("X-Request-ID")
	if rid == "" {
		t.Error("X-Request-ID should be generated")
	}
}

func TestRequestID_AlwaysGeneratesNew(t *testing.T) {
	handler := RequestID(okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "client-provided-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	rid := rec.Header().Get("X-Request-ID")
	if rid == "" {
		t.Fatal("X-Request-ID should be set")
	}
	if rid == "client-provided-id" {
		t.Error("server must ignore client X-Request-ID and generate its own (prevents log injection)")
	}
	// Should be a UUID (36 chars with hyphens)
	if len(rid) != 36 {
		t.Errorf("X-Request-ID should be UUID, got len=%d", len(rid))
	}
}

func TestStripAuthHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Subject") != "" {
			t.Error("X-Auth-Subject should be stripped")
		}
		if r.Header.Get("X-Auth-Scope") != "" {
			t.Error("X-Auth-Scope should be stripped")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := StripAuthHeaders(inner)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth-Subject", "spoofed")
	req.Header.Set("X-Auth-Scope", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestExtractIP_DirectIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	ip := extractIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("got %q, want 192.168.1.1", ip)
	}
}

func TestExtractIP_LoopbackTrustsXRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.50")
	ip := extractIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("got %q, want 203.0.113.50 (from X-Real-IP via loopback)", ip)
	}
}

func TestExtractIP_NonLoopbackIgnoresXRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("X-Real-IP", "spoofed")
	ip := extractIP(req)
	if ip != "10.0.0.5" {
		t.Errorf("got %q, want 10.0.0.5 (should ignore X-Real-IP from non-loopback)", ip)
	}
}

func TestSanitizeLogValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"clean", "clean"},
		{"line1\nline2", "line1line2"},
		{"has\r\ntabs\there", "hastabshere"},
		{"192.168.1.1", "192.168.1.1"},
		{"/path/to/resource", "/path/to/resource"},
		{"evil\x00null", "evilnull"},
	}
	for _, tt := range tests {
		got := SanitizeLogValue(tt.in)
		if got != tt.want {
			t.Errorf("SanitizeLogValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRateLimitWithGuard_StrikeOnDeny(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute) // 1 req/min
	defer rl.Stop()
	guard := NewBruteForceGuard(2, time.Minute)
	defer guard.Stop()

	handler := RateLimitWithGuard(rl, guard, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request passes
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("1st request: got %d, want 200", rr.Code)
	}

	// Second request exceeds rate limit → 429 + strike
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("2nd request: got %d, want 429", rr.Code)
	}
}

func TestRateLimitWithGuard_ExemptsHealth(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	defer rl.Stop()

	handler := RateLimitWithGuard(rl, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Health should always pass
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/health", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("health request %d: got %d, want 200", i, rr.Code)
		}
	}
}

func TestExtractIP_NoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1" // no port
	ip := extractIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("got %q, want 10.0.0.1", ip)
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.1", false},
		{"not-an-ip", false},
	}
	for _, tt := range tests {
		if got := isLoopback(tt.ip); got != tt.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(10, 50*time.Millisecond)
	defer rl.Stop()

	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.2")

	// Wait for window to expire
	time.Sleep(100 * time.Millisecond)

	// Directly call cleanup
	rl.cleanup()

	// After cleanup, entries should be gone — new window starts
	rl.Allow("10.0.0.1")
	// Should succeed (fresh window)
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)
	defer rl.Stop()

	if !rl.Allow("10.0.0.1") {
		t.Error("first request should be allowed")
	}
	if rl.Allow("10.0.0.1") {
		t.Error("second request should be denied (limit=1)")
	}

	// Wait for window to expire
	time.Sleep(100 * time.Millisecond)

	// Should be allowed again (window expired)
	if !rl.Allow("10.0.0.1") {
		t.Error("request should be allowed after window expiry")
	}
}
