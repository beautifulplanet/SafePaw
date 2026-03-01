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
)

func TestGetConfig(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "ANTHROPIC_API_KEY=sk-secret-12345\nTLS_ENABLED=false\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Port:       3000,
		EnvFilePath: envPath,
	}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/config: status = %d, want 200", rec.Code)
	}
	var resp struct {
		Config map[string]string `json:"config"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Config["ANTHROPIC_API_KEY"] != "***2345" {
		t.Errorf("ANTHROPIC_API_KEY should be masked, got %q", resp.Config["ANTHROPIC_API_KEY"])
	}
	if resp.Config["TLS_ENABLED"] != "false" {
		t.Errorf("TLS_ENABLED = %q, want false", resp.Config["TLS_ENABLED"])
	}
}

func TestPutConfigAllowedKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "TLS_ENABLED=false\nANTHROPIC_API_KEY=old\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Port: 3000, EnvFilePath: envPath}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()

	body := `{"TLS_ENABLED":"true","ANTHROPIC_API_KEY":"new-key"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/config: status = %d, want 200", rec.Code)
	}
	// Verify file was updated
	env, err := readEnvFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if env["TLS_ENABLED"] != "true" {
		t.Errorf("TLS_ENABLED = %q, want true", env["TLS_ENABLED"])
	}
	if env["ANTHROPIC_API_KEY"] != "new-key" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want new-key", env["ANTHROPIC_API_KEY"])
	}
}

func TestPutConfigRejectsDisallowedKey(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("POSTGRES_PASSWORD=secret\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Port: 3000, EnvFilePath: envPath}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()

	body := `{"POSTGRES_PASSWORD":"hacked"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT with disallowed key: status = %d (rejected keys are skipped, still 200)", rec.Code)
	}
	env, _ := readEnvFile(envPath)
	if env["POSTGRES_PASSWORD"] != "secret" {
		t.Errorf("POSTGRES_PASSWORD should be unchanged, got %q", env["POSTGRES_PASSWORD"])
	}
}
