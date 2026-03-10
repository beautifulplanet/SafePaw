package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"safepaw/wizard/internal/config"
)

// ─── OWNER_NAME / USE_CASE Config Keys ───────────────────────

func TestPutConfig_OwnerNameAllowed(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("# empty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Port: 3000, EnvFilePath: envPath}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	body := `{"OWNER_NAME":"Alice"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	env, err := ReadEnvFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if env["OWNER_NAME"] != "Alice" {
		t.Errorf("OWNER_NAME = %q, want Alice", env["OWNER_NAME"])
	}
}

func TestPutConfig_UseCaseAllowed(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("# empty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Port: 3000, EnvFilePath: envPath}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	body := `{"USE_CASE":"team"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}
	env, _ := ReadEnvFile(envPath)
	if env["USE_CASE"] != "team" {
		t.Errorf("USE_CASE = %q, want team", env["USE_CASE"])
	}
}

func TestPutConfig_OwnerNamePreserved(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("OWNER_NAME=Bob\nANTHROPIC_API_KEY=sk-test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Port: 3000, EnvFilePath: envPath}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	// Update API key — OWNER_NAME should be preserved
	body := `{"ANTHROPIC_API_KEY":"sk-new"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}
	env, _ := ReadEnvFile(envPath)
	if env["OWNER_NAME"] != "Bob" {
		t.Errorf("OWNER_NAME = %q, want Bob (should be preserved)", env["OWNER_NAME"])
	}
	if env["ANTHROPIC_API_KEY"] != "sk-new" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want sk-new", env["ANTHROPIC_API_KEY"])
	}
}

// ─── Setup Verify Endpoint ───────────────────────────────────

func TestSetupVerify_AllPass(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	envContent := fmt.Sprintf("ANTHROPIC_API_KEY=sk-test\nAUTH_SECRET=%s\nAUTH_ENABLED=true\n", secret)

	// Mock gateway that responds to /health and / (backend probe)
	mockGW := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			// Backend probe — return 200 (means backend is alive)
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer mockGW.Close()

	h, _ := newTestHandlerWithEnv(t, envContent)
	router := h.Router()
	t.Setenv("GATEWAY_URL", mockGW.URL)

	req := httptest.NewRequest("POST", "/api/v1/setup/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	var resp verifyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if !resp.Overall {
		t.Error("Overall should be true when all checks pass")
		for _, c := range resp.Checks {
			t.Logf("  %s: pass=%v msg=%q", c.Name, c.Pass, c.Message)
		}
	}

	if len(resp.Checks) < 3 {
		t.Errorf("Expected at least 3 checks, got %d", len(resp.Checks))
	}

	for _, c := range resp.Checks {
		if !c.Pass {
			t.Errorf("Check %q should pass, got: %s", c.Name, c.Message)
		}
	}
}

func TestSetupVerify_NoApiKey(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "# no keys\n")
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/setup/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}

	var resp verifyResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	// API key check should fail
	if resp.Checks[0].Pass {
		t.Error("API Key check should fail when no key configured")
	}
	if resp.Overall {
		t.Error("Overall should be false when API key missing")
	}
}

func TestSetupVerify_GatewayUnreachable(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	envContent := fmt.Sprintf("ANTHROPIC_API_KEY=sk-test\nAUTH_SECRET=%s\n", secret)

	h, _ := newTestHandlerWithEnv(t, envContent)
	router := h.Router()
	t.Setenv("GATEWAY_URL", "http://127.0.0.1:1") // unreachable

	req := httptest.NewRequest("POST", "/api/v1/setup/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}

	var resp verifyResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Overall {
		t.Error("Overall should be false when gateway unreachable")
	}

	// API key should pass, gateway should fail
	foundAPIPass := false
	foundGWFail := false
	for _, c := range resp.Checks {
		if c.Name == "API Key" && c.Pass {
			foundAPIPass = true
		}
		if c.Name == "Gateway" && !c.Pass {
			foundGWFail = true
		}
	}
	if !foundAPIPass {
		t.Error("API Key check should pass")
	}
	if !foundGWFail {
		t.Error("Gateway check should fail")
	}
}

func TestSetupVerify_BackendDown(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	envContent := fmt.Sprintf("ANTHROPIC_API_KEY=sk-test\nAUTH_SECRET=%s\n", secret)

	// Mock gateway that responds to /health but returns 502 for backend probe
	mockGW := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusBadGateway) // backend down
		}
	}))
	defer mockGW.Close()

	h, _ := newTestHandlerWithEnv(t, envContent)
	router := h.Router()
	t.Setenv("GATEWAY_URL", mockGW.URL)

	req := httptest.NewRequest("POST", "/api/v1/setup/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	var resp verifyResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Overall {
		t.Error("Overall should be false when backend is down")
	}

	// Gateway should pass, backend should fail
	for _, c := range resp.Checks {
		if c.Name == "Gateway" && !c.Pass {
			t.Error("Gateway check should pass")
		}
		if c.Name == "Backend (AI Service)" && c.Pass {
			t.Error("Backend check should fail when returning 502")
		}
	}
}

func TestSetupVerify_RequiresAdmin(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "ANTHROPIC_API_KEY=sk-test\n")
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/setup/verify", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asViewer(req))

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want 403 for viewer role", rec.Code)
	}
}

// ─── NeedsSetup with OWNER_NAME (upgrade-safe) ──────────────

func TestNeedsSetup_TrueEvenWithOwnerName(t *testing.T) {
	// OWNER_NAME alone doesn't satisfy setup — API key is required
	h, _ := newTestHandlerWithEnv(t, "OWNER_NAME=Alice\n")
	if !h.needsSetup() {
		t.Error("needsSetup should be true with only OWNER_NAME (no API key)")
	}
}

func TestNeedsSetup_FalseWithKeyAndOwnerName(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "OWNER_NAME=Alice\nANTHROPIC_API_KEY=sk-test\n")
	if h.needsSetup() {
		t.Error("needsSetup should be false when API key and OWNER_NAME are both set")
	}
}

// asViewer already defined in handler_cost_test.go
