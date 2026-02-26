package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"safepaw/wizard/internal/session"
)

// ok is a simple handler that returns 200 OK.
var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(ok)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"Referrer-Policy":       "strict-origin-when-cross-origin",
	}

	for header, want := range checks {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	// CSP should be present
	if csp := rec.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("Content-Security-Policy header is missing")
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"}, ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("ACAO = %q, want %q", got, "http://localhost:3000")
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"}, ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO should be empty for disallowed origin, got %q", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"}, ok)

	req := httptest.NewRequest("OPTIONS", "/api/v1/status", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestAdminAuth_PublicPaths(t *testing.T) {
	handler := AdminAuth("secret", ok)

	publicPaths := []string{
		"/api/v1/health",
		"/api/v1/auth/login",
		"/",
		"/index.html",
		"/assets/index.js",
	}

	for _, path := range publicPaths {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Path %s: status = %d, want 200", path, rec.Code)
		}
	}
}

func TestAdminAuth_ProtectedWithoutToken(t *testing.T) {
	handler := AdminAuth("secret", ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rec.Code)
	}
}

func TestAdminAuth_ValidBearerToken(t *testing.T) {
	secret := "test-admin-password"
	handler := AdminAuth(secret, ok)

	token, _ := session.Create(secret, time.Hour)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rec.Code)
	}
}

func TestAdminAuth_ValidCookie(t *testing.T) {
	secret := "test-admin-password"
	handler := AdminAuth(secret, ok)

	token, _ := session.Create(secret, time.Hour)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rec.Code)
	}
}

func TestAdminAuth_InvalidToken(t *testing.T) {
	handler := AdminAuth("real-secret", ok)

	token, _ := session.Create("wrong-secret", time.Hour)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rec.Code)
	}
}

func TestAdminAuth_ExpiredToken(t *testing.T) {
	secret := "test-admin-password"
	handler := AdminAuth(secret, ok)

	token, _ := session.Create(secret, -1*time.Hour) // Already expired

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401 for expired token", rec.Code)
	}
}

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	handler := RateLimit(5, time.Minute, ok)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: status = %d, want 200", i+1, rec.Code)
		}
	}
}

func TestRateLimit_BlocksOverLimit(t *testing.T) {
	handler := RateLimit(3, time.Minute, ok)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want 429", rec.Code)
	}
}

func TestRateLimit_DifferentIPs(t *testing.T) {
	handler := RateLimit(1, time.Minute, ok)

	// First IP
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.1.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Error("First IP first request should be 200")
	}

	// Second IP should have its own quota
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "2.2.2.2:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Error("Second IP first request should be 200")
	}
}

func TestIsPublicPath(t *testing.T) {
	tt := []struct {
		path   string
		public bool
	}{
		{"/api/v1/health", true},
		{"/api/v1/auth/login", true},
		{"/", true},
		{"/index.html", true},
		{"/assets/style.css", true},
		{"/api/v1/status", false},
		{"/api/v1/prerequisites", false},
		{"/api/v1/config", false},
	}

	for _, tc := range tt {
		got := isPublicPath(tc.path)
		if got != tc.public {
			t.Errorf("isPublicPath(%q) = %v, want %v", tc.path, got, tc.public)
		}
	}
}
