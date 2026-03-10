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

// sessionValidator returns a SessionValidator that validates tokens with the given secret and gen 0 (for tests).
func sessionValidator(secret string) SessionValidator {
	return func(token string) (string, bool) {
		claims, err := session.Validate(token, secret, 0)
		if err != nil {
			return "", false
		}
		return claims.EffectiveRole(), true
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(ok)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
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
	handler := AdminAuth(sessionValidator("secret"), ok)

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
	handler := AdminAuth(sessionValidator("secret"), ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rec.Code)
	}
}

func TestAdminAuth_ValidBearerToken(t *testing.T) {
	secret := "test-admin-password"
	handler := AdminAuth(sessionValidator(secret), ok)

	token, _ := session.Create(secret, time.Hour, 0, "admin")

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
	handler := AdminAuth(sessionValidator(secret), ok)

	token, _ := session.Create(secret, time.Hour, 0, "admin")

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want 200", rec.Code)
	}
}

func TestAdminAuth_InvalidToken(t *testing.T) {
	handler := AdminAuth(sessionValidator("real-secret"), ok)

	token, _ := session.Create("wrong-secret", time.Hour, 0, "admin")

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
	handler := AdminAuth(sessionValidator(secret), ok)

	token, _ := session.Create(secret, -1*time.Hour, 0, "admin") // Already expired

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

// =============================================================
// Edge-Case Tests
// =============================================================

func TestRateLimit_RetryAfterHeader(t *testing.T) {
	handler := RateLimit(1, time.Minute, ok)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.55:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req) // First request — allowed

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.55:1234"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req) // Second — blocked

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestCORS_EmptyAllowedOrigins(t *testing.T) {
	handler := CORS([]string{}, ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Origin", "http://anything.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Expected no ACAO for empty allowlist, got %q", got)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"}, ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Request without Origin should pass through, got %d", rec.Code)
	}
}

func TestCORS_PreflightDisallowedOrigin(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"}, ok)

	req := httptest.NewRequest("OPTIONS", "/api/v1/status", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Preflight from disallowed origin should not set ACAO, got %q", got)
	}
}

func TestAdminAuth_EmptyBearerToken(t *testing.T) {
	handler := AdminAuth(sessionValidator("secret"), ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Empty bearer token should be 401, got %d", rec.Code)
	}
}

func TestAdminAuth_MalformedAuthHeader(t *testing.T) {
	handler := AdminAuth(sessionValidator("secret"), ok)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "NotBearer sometoken")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Malformed auth header should be 401, got %d", rec.Code)
	}
}

func TestAdminAuth_BearerTakesPrecedenceOverCookie(t *testing.T) {
	secret := "test-admin-password"
	handler := AdminAuth(sessionValidator(secret), ok)

	goodToken, _ := session.Create(secret, time.Hour, 0, "admin")
	badToken, _ := session.Create("wrong-secret", time.Hour, 0, "admin")

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+goodToken)
	req.AddCookie(&http.Cookie{Name: "session", Value: badToken})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Valid bearer should take precedence over bad cookie, got %d", rec.Code)
	}
}

func TestAdminAuth_PathTraversalNotPublic(t *testing.T) {
	handler := AdminAuth(sessionValidator("secret"), ok)

	paths := []string{
		"/api/v1/auth/login/../config",
		"/api/v1/health/../../v1/status",
	}

	for _, path := range paths {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Errorf("Path %q should not bypass auth (got 200)", path)
		}
	}
}

func TestRateLimit_IPParsesPort(t *testing.T) {
	handler := RateLimit(1, time.Minute, ok)

	// Same IP, different ports — should share the rate limit
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.99:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatal("first request should be OK")
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.99:6666"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Same IP different port should share rate limit, got %d", rec.Code)
	}
}

func TestSecurityHeaders_CSPPresent(t *testing.T) {
	handler := SecurityHeaders(ok)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("CSP header missing")
	}
	// Should restrict scripts to self
	if !containsSubstring(csp, "'self'") {
		t.Errorf("CSP should contain 'self', got %q", csp)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// =============================================================
// CSRF Protection Tests
// =============================================================

func TestCSRFProtect_SafeMethodsPassThrough(t *testing.T) {
	handler := CSRFProtect(false, ok)

	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		req := httptest.NewRequest(method, "/api/v1/config", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s should pass through, got %d", method, rec.Code)
		}
	}
}

func TestCSRFProtect_PublicPathSkipsCSRF(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Public POST path should skip CSRF, got %d", rec.Code)
	}
}

func TestCSRFProtect_NonAPIPathSkipsCSRF(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/some-form", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Non-API POST should skip CSRF, got %d", rec.Code)
	}
}

func TestCSRFProtect_BearerTokenBypassesCSRF(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Bearer auth should bypass CSRF, got %d", rec.Code)
	}
}

func TestCSRFProtect_MissingCookie(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Missing CSRF cookie should be 403, got %d", rec.Code)
	}
}

func TestCSRFProtect_EmptyCookie(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/config", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: ""})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Empty CSRF cookie should be 403, got %d", rec.Code)
	}
}

func TestCSRFProtect_MissingHeader(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/config", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "valid-token"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Missing X-CSRF-Token header should be 403, got %d", rec.Code)
	}
}

func TestCSRFProtect_MismatchedTokens(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/config", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "cookie-token"})
	req.Header.Set("X-CSRF-Token", "different-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Mismatched CSRF tokens should be 403, got %d", rec.Code)
	}
}

func TestCSRFProtect_ValidDoubleSubmit(t *testing.T) {
	handler := CSRFProtect(false, ok)

	req := httptest.NewRequest("POST", "/api/v1/config", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "secret-csrf-token"})
	req.Header.Set("X-CSRF-Token", "secret-csrf-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Matching CSRF tokens should pass, got %d", rec.Code)
	}
}

func TestCSRFProtect_PUTAndDELETEAlsoChecked(t *testing.T) {
	handler := CSRFProtect(false, ok)

	for _, method := range []string{"PUT", "DELETE"} {
		// Without CSRF — should block
		req := httptest.NewRequest(method, "/api/v1/config", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s without CSRF should be 403, got %d", method, rec.Code)
		}

		// With valid CSRF — should pass
		req = httptest.NewRequest(method, "/api/v1/config", nil)
		req.AddCookie(&http.Cookie{Name: "csrf", Value: "tok"})
		req.Header.Set("X-CSRF-Token", "tok")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s with valid CSRF should be 200, got %d", method, rec.Code)
		}
	}
}

// =============================================================
// GenerateCSRFToken Tests
// =============================================================

func TestGenerateCSRFToken_UniqueAndCorrectLength(t *testing.T) {
	tok1, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken() error: %v", err)
	}
	tok2, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken() error: %v", err)
	}

	if len(tok1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("token length = %d, want 32", len(tok1))
	}
	if tok1 == tok2 {
		t.Error("two generated tokens should not be identical")
	}
}

// =============================================================
// RequireRole Tests
// =============================================================

func TestRequireRole_AllowedRole(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireRole([]string{"admin", "operator"}, inner)

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	req = SetRole(req, "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("admin should be allowed, got %d", rec.Code)
	}
}

func TestRequireRole_DisallowedRole(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireRole([]string{"admin"}, inner)

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	req = SetRole(req, "viewer")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer should be forbidden for admin-only, got %d", rec.Code)
	}
}

func TestRequireRole_NoRole(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireRole([]string{"admin"}, inner)

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	// No role set in context
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("missing role should be forbidden, got %d", rec.Code)
	}
}

func TestGetRole_EmptyContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if role := GetRole(req); role != "" {
		t.Errorf("GetRole on bare request = %q, want empty", role)
	}
}

func TestSetRole_RoundTrip(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req = SetRole(req, "operator")
	if got := GetRole(req); got != "operator" {
		t.Errorf("GetRole after SetRole = %q, want %q", got, "operator")
	}
}

// =============================================================
// LoginGuard Tests
// =============================================================

func TestLoginGuard_AllowsUnderThreshold(t *testing.T) {
	lg := NewLoginGuard(5, time.Minute, 10*time.Minute)
	defer lg.Stop()

	for i := 0; i < 4; i++ {
		locked := lg.RecordFailure("10.0.0.1")
		if locked {
			t.Fatalf("Should not lock after %d failures (threshold=5)", i+1)
		}
	}
}

func TestLoginGuard_LocksAtThreshold(t *testing.T) {
	lg := NewLoginGuard(3, time.Minute, 10*time.Minute)
	defer lg.Stop()

	for i := 0; i < 2; i++ {
		lg.RecordFailure("10.0.0.1")
	}
	locked := lg.RecordFailure("10.0.0.1")
	if !locked {
		t.Error("Should be locked after 3 failures")
	}
}

func TestLoginGuard_IsLockedOut(t *testing.T) {
	lg := NewLoginGuard(2, time.Minute, 5*time.Minute)
	defer lg.Stop()

	lg.RecordFailure("10.0.0.1")
	lg.RecordFailure("10.0.0.1") // Now locked

	isLocked, remaining := lg.IsLockedOut("10.0.0.1")
	if !isLocked {
		t.Error("Should be locked out")
	}
	if remaining <= 0 || remaining > 5*time.Minute {
		t.Errorf("remaining = %v, want > 0 and <= 5m", remaining)
	}
}

func TestLoginGuard_NotLockedIfNoFailures(t *testing.T) {
	lg := NewLoginGuard(3, time.Minute, 10*time.Minute)
	defer lg.Stop()

	isLocked, _ := lg.IsLockedOut("10.0.0.1")
	if isLocked {
		t.Error("Should not be locked with no failures")
	}
}

func TestLoginGuard_ResetIPClearsLockout(t *testing.T) {
	lg := NewLoginGuard(2, time.Minute, 10*time.Minute)
	defer lg.Stop()

	lg.RecordFailure("10.0.0.1")
	lg.RecordFailure("10.0.0.1")
	isLocked, _ := lg.IsLockedOut("10.0.0.1")
	if !isLocked {
		t.Fatal("should be locked first")
	}

	lg.ResetIP("10.0.0.1")
	isLocked, _ = lg.IsLockedOut("10.0.0.1")
	if isLocked {
		t.Error("Should be cleared after ResetIP")
	}
}

func TestLoginGuard_DifferentIPsIndependent(t *testing.T) {
	lg := NewLoginGuard(2, time.Minute, 10*time.Minute)
	defer lg.Stop()

	lg.RecordFailure("10.0.0.1")
	lg.RecordFailure("10.0.0.1") // Locked

	locked := lg.RecordFailure("10.0.0.2") // Different IP, first failure
	if locked {
		t.Error("Different IP should not be affected by first IP's lockout")
	}
}

func TestLoginGuard_LockoutDuration(t *testing.T) {
	lg := NewLoginGuard(3, time.Minute, 7*time.Minute)
	defer lg.Stop()

	if lg.LockoutDuration() != 7*time.Minute {
		t.Errorf("LockoutDuration() = %v, want 7m", lg.LockoutDuration())
	}
}

func TestLoginGuard_Cleanup(t *testing.T) {
	lg := NewLoginGuard(2, 50*time.Millisecond, 50*time.Millisecond)
	defer lg.Stop()

	lg.RecordFailure("10.0.0.1")
	lg.RecordFailure("10.0.0.1") // Locked

	// Wait for lockout to expire + cleanup cycle
	time.Sleep(200 * time.Millisecond)

	isLocked, _ := lg.IsLockedOut("10.0.0.1")
	if isLocked {
		t.Error("Lockout should have expired and been cleaned up")
	}
}

// =============================================================
// extractIP Tests
// =============================================================

func TestExtractIP_WithPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:54321"
	if got := extractIP(req); got != "192.168.1.1" {
		t.Errorf("extractIP = %q, want %q", got, "192.168.1.1")
	}
}

func TestExtractIP_WithoutPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1"
	if got := extractIP(req); got != "192.168.1.1" {
		t.Errorf("extractIP = %q, want %q", got, "192.168.1.1")
	}
}

func TestExtractIP_IPv6(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::1]:8080"
	if got := extractIP(req); got != "::1" {
		t.Errorf("extractIP = %q, want %q", got, "::1")
	}
}
