package api

import (
	"os"
	"path/filepath"
	"testing"

	"safepaw/wizard/internal/config"
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
		TOTPSecret:   "totp1",
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
		TOTPSecret:   "totp1",
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
		TOTPSecret:   "t",
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
