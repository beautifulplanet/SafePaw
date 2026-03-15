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

func TestCredentialHash_deterministic(t *testing.T) {
	h1 := credentialHash("admin1", "totp1")
	h2 := credentialHash("admin1", "totp1")
	if h1 != h2 {
		t.Errorf("credentialHash should be deterministic, got %q vs %q", h1, h2)
	}
	if h1 == credentialHash("admin2", "totp1") {
		t.Error("different password should produce different hash")
	}
	if h1 == credentialHash("admin1", "totp2") {
		t.Error("different TOTP should produce different hash")
	}
}

func TestReadSessionGenFile_missingOrInvalid(t *testing.T) {
	gen, hash := readSessionGenFile("")
	if gen != 0 || hash != "" {
		t.Errorf("empty path: gen=%d hash=%q, want 0, \"\"", gen, hash)
	}
	gen, hash = readSessionGenFile(filepath.Join(t.TempDir(), "nonexistent"))
	if gen != 0 || hash != "" {
		t.Errorf("missing file: gen=%d hash=%q, want 0, \"\"", gen, hash)
	}
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad")
	if err := os.WriteFile(badPath, []byte("not-a-number\nx"), 0600); err != nil {
		t.Fatal(err)
	}
	gen, hash = readSessionGenFile(badPath)
	if gen != 0 || hash != "" {
		t.Errorf("invalid file: gen=%d hash=%q, want 0, \"\"", gen, hash)
	}
}

func TestWriteReadSessionGenFile_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".safepaw_session_gen")
	writeSessionGenFile(path, 3, "abc123")
	gen, hash := readSessionGenFile(path)
	if gen != 3 || hash != "abc123" {
		t.Errorf("roundTrip: gen=%d hash=%q, want 3, \"abc123\"", gen, hash)
	}
}

func TestInitSessionGenFromFile_emptyPath_noop(t *testing.T) {
	cfg := &config.Config{EnvFilePath: "", AdminPassword: "p", TOTPSecret: "t", SessionSecret: "01234567890123456789012345678901"}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.InitSessionGenFromFile()
	if h.sessionGen.Load() != 0 {
		t.Errorf("empty EnvFilePath should leave gen 0, got %d", h.sessionGen.Load())
	}
}

func TestInitSessionGenFromFile_bumpsGenWhenCredentialsChange(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("x=1"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		EnvFilePath:   envPath,
		AdminPassword: "pass1",
		TOTPSecret:    "totp1",
		SessionSecret: "01234567890123456789012345678901",
	}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.InitSessionGenFromFile()
	if g := h.sessionGen.Load(); g != 0 {
		t.Errorf("first run: gen=%d, want 0", g)
	}
	genPath := filepath.Join(dir, ".safepaw_session_gen")
	if _, err := os.Stat(genPath); err != nil {
		t.Errorf("session gen file should exist: %v", err)
	}

	// Simulate credential change: new handler with different password
	cfg2 := &config.Config{
		EnvFilePath:   envPath,
		AdminPassword: "pass2",
		TOTPSecret:    "totp1",
		SessionSecret: "01234567890123456789012345678901",
	}
	h2, err := NewHandler(cfg2, nil)
	if err != nil {
		t.Fatal(err)
	}
	h2.InitSessionGenFromFile()
	if g := h2.sessionGen.Load(); g != 1 {
		t.Errorf("after credential change: gen=%d, want 1", g)
	}
}

func TestBumpSessionGen_persistsToFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("x=1"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		EnvFilePath:   envPath,
		AdminPassword: "p",
		TOTPSecret:    "t",
		SessionSecret: "01234567890123456789012345678901",
	}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.InitSessionGenFromFile()
	h.BumpSessionGen()
	if g := h.sessionGen.Load(); g != 1 {
		t.Errorf("after BumpSessionGen: gen=%d, want 1", g)
	}
	gen, hash := readSessionGenFile(filepath.Join(dir, ".safepaw_session_gen"))
	if gen != 1 {
		t.Errorf("persisted gen=%d, want 1", gen)
	}
	if hash != credentialHash("p", "t") {
		t.Error("persisted hash should match current credentials")
	}
}

// TestSessionInvalidation_AfterPasswordChange verifies that after BumpSessionGen (e.g. password
// or TOTP change), a request with the old session cookie receives 401 Unauthorized.
// This locks in S4 behavior: credential rotation invalidates existing sessions.
func TestSessionInvalidation_AfterPasswordChange(t *testing.T) {
	cfg := &config.Config{
		EnvFilePath:   "", // no file so gen stays 0 until we bump
		AdminPassword: "test-admin-pass",
		SessionSecret: "01234567890123456789012345678901",
		DockerHost:    "unix:///var/run/docker.sock",
	}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	chain := middleware.AdminAuth(h.SessionValidator(), h.Router())

	// 1) Login to get a session cookie (gen 0)
	body, _ := json.Marshal(loginRequest{Password: "test-admin-pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not set session cookie")
	}

	// 2) Request with that cookie → 200
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req2.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()
	chain.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("auth/me with valid cookie: status = %d, want 200", rec2.Code)
	}

	// 3) Bump session gen (simulates password or TOTP change)
	h.BumpSessionGen()

	// 4) Same cookie again → 401 (session invalidated)
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req3.AddCookie(sessionCookie)
	rec3 := httptest.NewRecorder()
	chain.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Errorf("auth/me with old cookie after bump: status = %d, want 401", rec3.Code)
	}
}
