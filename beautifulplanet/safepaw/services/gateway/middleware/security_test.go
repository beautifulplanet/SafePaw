package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(okHandler())
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	checks := map[string]string{
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'",
		"Referrer-Policy":           "no-referrer",
	}
	for header, want := range checks {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
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
