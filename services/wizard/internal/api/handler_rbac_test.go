package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"safepaw/wizard/internal/config"
	"safepaw/wizard/internal/middleware"
)

// newRBACHandler creates a handler with admin, operator, and viewer passwords configured.
func newRBACHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("AUTH_SECRET=this-is-a-very-long-test-secret-that-is-at-least-32-bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:             3000,
		AdminPassword:    "admin-pass-123",
		OperatorPassword: "operator-pass-456",
		ViewerPassword:   "viewer-pass-789",
		SessionSecret:    "test-session-secret-32bytes!!!",
		DockerHost:       "unix:///var/run/docker.sock",
		EnvFilePath:      envPath,
	}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h, envPath
}

// withRole injects the given role into the request context.
func withRole(r *http.Request, role string) *http.Request {
	return middleware.SetRole(r, role)
}

// ─── Login role assignment ───────────────────────────────────

func TestLogin_AdminRole(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "admin-pass-123"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	var resp loginResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Role != "admin" {
		t.Errorf("Role = %q, want admin", resp.Role)
	}
}

func TestLogin_OperatorRole(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "operator-pass-456"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	var resp loginResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Role != "operator" {
		t.Errorf("Role = %q, want operator", resp.Role)
	}
}

func TestLogin_ViewerRole(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body, _ := json.Marshal(loginRequest{Password: "viewer-pass-789"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	var resp loginResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Role != "viewer" {
		t.Errorf("Role = %q, want viewer", resp.Role)
	}
}

// ─── Viewer restrictions ─────────────────────────────────────

func TestViewer_CanReadConfig(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "viewer"))

	if rec.Code != http.StatusOK {
		t.Errorf("Viewer GET /config: status = %d, want 200", rec.Code)
	}
}

func TestViewer_CannotPutConfig(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body := `{"TLS_ENABLED":"true"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "viewer"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("Viewer PUT /config: status = %d, want 403", rec.Code)
	}
}

func TestViewer_CannotRestart(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/services/wizard/restart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "viewer"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("Viewer POST /services/restart: status = %d, want 403", rec.Code)
	}
}

func TestViewer_CannotCreateGatewayToken(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{Subject: "test"})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "viewer"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("Viewer POST /gateway/token: status = %d, want 403", rec.Code)
	}
}

func TestViewer_CanReadMetrics(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/gateway/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "viewer"))

	if rec.Code != http.StatusOK {
		t.Errorf("Viewer GET /gateway/metrics: status = %d, want 200", rec.Code)
	}
}

// ─── Operator restrictions ───────────────────────────────────

func TestOperator_CanReadConfig(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "operator"))

	if rec.Code != http.StatusOK {
		t.Errorf("Operator GET /config: status = %d, want 200", rec.Code)
	}
}

func TestOperator_CannotPutConfig(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body := `{"TLS_ENABLED":"true"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "operator"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("Operator PUT /config: status = %d, want 403", rec.Code)
	}
}

func TestOperator_CanRestart(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	// "wizard" is a known service; nil Docker → 503, but that means role check passed
	req := httptest.NewRequest("POST", "/api/v1/services/wizard/restart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "operator"))

	// 503 = Docker unavailable (expected); NOT 403
	if rec.Code == http.StatusForbidden {
		t.Error("Operator should be allowed to restart services")
	}
}

func TestOperator_CannotCreateGatewayToken(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{Subject: "test"})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "operator"))

	if rec.Code != http.StatusForbidden {
		t.Errorf("Operator POST /gateway/token: status = %d, want 403", rec.Code)
	}
}

// ─── Admin full access ───────────────────────────────────────

func TestAdmin_CanPutConfig(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body := `{"TLS_ENABLED":"true"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "admin"))

	if rec.Code == http.StatusForbidden {
		t.Error("Admin should have PUT /config access")
	}
}

func TestAdmin_CanCreateGatewayToken(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{Subject: "test", Scope: "proxy", TTLHrs: 1})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, withRole(req, "admin"))

	if rec.Code == http.StatusForbidden {
		t.Error("Admin should have POST /gateway/token access")
	}
}

// ─── No role → 403 ──────────────────────────────────────────

func TestNoRole_Forbidden(t *testing.T) {
	h, _ := newRBACHandler(t)
	router := h.Router()

	// Request without any role context → RequireRole blocks
	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("No-role GET /config: status = %d, want 403", rec.Code)
	}
}
