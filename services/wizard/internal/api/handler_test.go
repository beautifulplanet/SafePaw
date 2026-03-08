package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"safepaw/wizard/internal/config"
	"safepaw/wizard/internal/session"
	"safepaw/wizard/internal/totp"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	cfg := &config.Config{
		Port:          3000,
		AdminPassword: "test-password-123",
		SessionSecret: "test-session-secret-32bytes!!!",
		DockerHost:    "unix:///var/run/docker.sock",
	}
	h, err := NewHandler(cfg, nil) // nil docker client (no Docker in tests)
	if err != nil {
		t.Fatalf("NewHandler() failed: %v", err)
	}
	return h
}

func TestHealthEndpoint(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Status = %q, want %q", resp.Status, "ok")
	}
	if resp.Service != "safepaw-wizard" {
		t.Errorf("Service = %q, want %q", resp.Service, "safepaw-wizard")
	}
	if resp.Version == "" {
		t.Error("Version should not be empty")
	}
}

func TestLoginSuccess(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "test-password-123"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.ExpiresIn <= 0 {
		t.Errorf("ExpiresIn = %d, want positive value", resp.ExpiresIn)
	}

	// Should set a session cookie
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
		}
		if c.Name == "csrf" {
			csrfCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("Should set a 'session' cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Error("Session cookie should be HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Error("Session cookie should have SameSite=Strict")
	}

	// Cookie value should be a valid token signed with SessionSecret
	claims, err := session.Validate(sessionCookie.Value, "test-session-secret-32bytes!!!", 0)
	if err != nil {
		t.Fatalf("Cookie token is invalid: %v", err)
	}
	if claims.Subject != "admin" {
		t.Errorf("Token subject = %q, want %q", claims.Subject, "admin")
	}

	// Should set a CSRF cookie (readable by JS, NOT HttpOnly)
	if csrfCookie == nil {
		t.Fatal("Should set a 'csrf' cookie")
	}
	if csrfCookie.HttpOnly {
		t.Error("CSRF cookie should NOT be HttpOnly (must be readable by JS)")
	}
	if csrfCookie.Value == "" {
		t.Error("CSRF cookie should have a non-empty value")
	}
}

func TestLoginWithMFA_RequiresTOTP(t *testing.T) {
	cfg := &config.Config{
		Port:          3000,
		AdminPassword: "test-password-123",
		SessionSecret: "test-session-secret-32bytes!!!",
		TOTPSecret:    "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", // RFC test vector
		DockerHost:    "unix:///var/run/docker.sock",
	}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "test-password-123"}) // no TOTP
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Status = %d, want 401 when MFA enabled but no TOTP", rec.Code)
	}
	var errResp errorResponse
	_ = json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp.Error != "totp_required" {
		t.Errorf("Error = %q, want totp_required", errResp.Error)
	}
}

func TestLoginWithMFA_RejectsInvalidTOTP(t *testing.T) {
	cfg := &config.Config{
		Port:          3000,
		AdminPassword: "test-password-123",
		SessionSecret: "test-session-secret-32bytes!!!",
		TOTPSecret:    "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		DockerHost:    "unix:///var/run/docker.sock",
	}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "test-password-123", TOTP: "000000"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Status = %d, want 401 for wrong TOTP", rec.Code)
	}
}

func TestLoginWithMFA_AcceptsValidTOTP(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cfg := &config.Config{
		Port:          3000,
		AdminPassword: "test-password-123",
		SessionSecret: "test-session-secret-32bytes!!!",
		TOTPSecret:    secret,
		DockerHost:    "unix:///var/run/docker.sock",
	}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	code := totp.CodeForTime(secret, time.Now())
	if code == "" {
		t.Fatal("CodeForTime returned empty")
	}
	body, _ := json.Marshal(loginRequest{Password: "test-password-123", TOTP: code})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200 when password + valid TOTP. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "wrong-password"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", rec.Code)
	}
}

func TestLoginBadBody(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", rec.Code)
	}
}

func TestSPAFallback(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	// Non-API paths should serve the SPA (index.html)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	// Should return 200 (SPA index.html), not 404
	if rec.Code != http.StatusOK {
		t.Errorf("SPA fallback: status = %d, want 200", rec.Code)
	}
}

func TestServiceRestartUnknownService(t *testing.T) {
	h := newTestHandler(t)
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/services/unknownservice/restart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400 for unknown service", rec.Code)
	}
	var errResp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if errResp.Error == "" {
		t.Error("Expected error message for unknown service")
	}
}

func TestServiceRestartNoDocker(t *testing.T) {
	h := newTestHandler(t) // nil docker
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/services/wizard/restart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	// With nil docker we return 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want 503 when Docker client unavailable", rec.Code)
	}
}
