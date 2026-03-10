package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that might interfere
	for _, key := range []string{"PROXY_TARGET", "GATEWAY_PORT", "AUTH_ENABLED", "TLS_ENABLED", "AUTH_SECRET"} {
		os.Unsetenv(key)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with defaults failed: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.ProxyTarget.String() != "http://openclaw:18789" {
		t.Errorf("ProxyTarget = %q, want http://openclaw:18789", cfg.ProxyTarget.String())
	}
	if cfg.AuthEnabled {
		t.Error("AuthEnabled should be false by default")
	}
	if cfg.TLSEnabled {
		t.Error("TLSEnabled should be false by default")
	}
	if cfg.RateLimit != 60 {
		t.Errorf("RateLimit = %d, want 60", cfg.RateLimit)
	}
	if cfg.MaxBodySize != 1048576 {
		t.Errorf("MaxBodySize = %d, want 1048576", cfg.MaxBodySize)
	}
}

func TestLoad_CustomPort(t *testing.T) {
	os.Setenv("GATEWAY_PORT", "9090")
	defer os.Unsetenv("GATEWAY_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
}

func TestLoad_InvalidProxyTarget(t *testing.T) {
	os.Setenv("PROXY_TARGET", "not-a-url")
	defer os.Unsetenv("PROXY_TARGET")

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid PROXY_TARGET")
	}
}

func TestLoad_AuthEnabledWithoutSecret(t *testing.T) {
	os.Setenv("AUTH_ENABLED", "true")
	os.Unsetenv("AUTH_SECRET")
	defer os.Unsetenv("AUTH_ENABLED")

	_, err := Load()
	if err == nil {
		t.Error("expected error when AUTH_ENABLED=true but no AUTH_SECRET")
	}
}

func TestLoad_AuthEnabledWithSecret(t *testing.T) {
	os.Setenv("AUTH_ENABLED", "true")
	os.Setenv("AUTH_SECRET", "this-is-a-test-secret-that-is-at-least-32-bytes-long!")
	defer func() {
		os.Unsetenv("AUTH_ENABLED")
		os.Unsetenv("AUTH_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if !cfg.AuthEnabled {
		t.Error("AuthEnabled should be true")
	}
	if string(cfg.AuthSecret) != "this-is-a-test-secret-that-is-at-least-32-bytes-long!" {
		t.Error("AuthSecret not set correctly")
	}
}

func TestLoad_TLSEnabled(t *testing.T) {
	os.Setenv("TLS_ENABLED", "true")
	os.Setenv("TLS_CERT_FILE", "/certs/tls.crt")
	os.Setenv("TLS_KEY_FILE", "/certs/tls.key")
	defer func() {
		os.Unsetenv("TLS_ENABLED")
		os.Unsetenv("TLS_CERT_FILE")
		os.Unsetenv("TLS_KEY_FILE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TLSEnabled {
		t.Error("TLSEnabled should be true")
	}
}

func TestLoad_AllowedOrigins(t *testing.T) {
	os.Setenv("ALLOWED_ORIGINS", "https://app.example.com, https://staging.example.com")
	defer os.Unsetenv("ALLOWED_ORIGINS")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins len = %d, want 2", len(cfg.AllowedOrigins))
	}
	if cfg.AllowedOrigins[0] != "https://app.example.com" {
		t.Errorf("AllowedOrigins[0] = %q", cfg.AllowedOrigins[0])
	}
	if cfg.AllowedOrigins[1] != "https://staging.example.com" {
		t.Errorf("AllowedOrigins[1] = %q", cfg.AllowedOrigins[1])
	}
}

func TestLoad_CustomRateLimit(t *testing.T) {
	os.Setenv("RATE_LIMIT", "100")
	os.Setenv("RATE_LIMIT_WINDOW_SEC", "30")
	defer func() {
		os.Unsetenv("RATE_LIMIT")
		os.Unsetenv("RATE_LIMIT_WINDOW_SEC")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateLimit != 100 {
		t.Errorf("RateLimit = %d, want 100", cfg.RateLimit)
	}
}

func TestHelpers_EnvStr(t *testing.T) {
	os.Setenv("TEST_KEY_STR", "hello")
	defer os.Unsetenv("TEST_KEY_STR")

	if envStr("TEST_KEY_STR", "default") != "hello" {
		t.Error("should return env value")
	}
	if envStr("NONEXISTENT_KEY", "fallback") != "fallback" {
		t.Error("should return fallback")
	}
}

func TestHelpers_EnvInt(t *testing.T) {
	os.Setenv("TEST_KEY_INT", "42")
	defer os.Unsetenv("TEST_KEY_INT")

	if envInt("TEST_KEY_INT", 0) != 42 {
		t.Error("should parse int from env")
	}
	if envInt("NONEXISTENT_KEY", 99) != 99 {
		t.Error("should return fallback")
	}
}

func TestHelpers_SplitAndTrim(t *testing.T) {
	result := splitAndTrim("  a , b , c  ", ",")
	if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("splitAndTrim result = %v", result)
	}

	empty := splitAndTrim("", ",")
	if len(empty) != 0 {
		t.Errorf("empty string should return empty slice, got %v", empty)
	}
}

func TestHelpers_EnvFloat(t *testing.T) {
	os.Setenv("TEST_KEY_FLOAT", "3.14")
	defer os.Unsetenv("TEST_KEY_FLOAT")

	if got := envFloat("TEST_KEY_FLOAT", 0); got != 3.14 {
		t.Errorf("envFloat = %f, want 3.14", got)
	}
	if got := envFloat("NONEXISTENT_FLOAT", 9.9); got != 9.9 {
		t.Errorf("expected fallback 9.9, got %f", got)
	}

	// Invalid float falls back
	os.Setenv("TEST_KEY_FLOAT", "not-a-float")
	if got := envFloat("TEST_KEY_FLOAT", 1.5); got != 1.5 {
		t.Errorf("expected fallback 1.5 for invalid float, got %f", got)
	}
}

func TestLoad_CostAlertThresholds(t *testing.T) {
	os.Setenv("COST_ALERT_DAILY_WARN", "5.0")
	os.Setenv("COST_ALERT_DAILY_CRIT", "50.0")
	defer func() {
		os.Unsetenv("COST_ALERT_DAILY_WARN")
		os.Unsetenv("COST_ALERT_DAILY_CRIT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CostAlertDailyWarn != 5.0 {
		t.Errorf("CostAlertDailyWarn = %f, want 5.0", cfg.CostAlertDailyWarn)
	}
	if cfg.CostAlertDailyCrit != 50.0 {
		t.Errorf("CostAlertDailyCrit = %f, want 50.0", cfg.CostAlertDailyCrit)
	}
}

func TestLoad_ProxyTargetNoScheme(t *testing.T) {
	t.Setenv("PROXY_TARGET", "just-a-host")
	_, err := Load()
	if err == nil {
		t.Error("expected error for PROXY_TARGET without scheme")
	}
}

// ── Per-Endpoint Rate Limits (CO-001) ─────────────────

func TestParseEndpointLimits(t *testing.T) {
	limits := parseEndpointLimits("/api/v1/chat=10;/api/v1/models=30;/admin/=5")
	if len(limits) != 3 {
		t.Fatalf("expected 3 limits, got %d", len(limits))
	}
	// Should be sorted longest prefix first
	if limits[0].Prefix != "/api/v1/models" {
		t.Errorf("expected longest prefix first, got %q", limits[0].Prefix)
	}
	if limits[0].Limit != 30 {
		t.Errorf("expected limit=30 for /api/v1/models, got %d", limits[0].Limit)
	}
	if limits[1].Prefix != "/api/v1/chat" {
		t.Errorf("expected /api/v1/chat second, got %q", limits[1].Prefix)
	}
}

func TestParseEndpointLimits_Empty(t *testing.T) {
	limits := parseEndpointLimits("")
	if limits != nil {
		t.Error("expected nil for empty string")
	}
}

func TestParseEndpointLimits_Malformed(t *testing.T) {
	limits := parseEndpointLimits("noequals;=5;/ok=10;/bad=NaN;=;")
	if len(limits) != 1 {
		t.Fatalf("expected 1 valid limit, got %d", len(limits))
	}
	if limits[0].Prefix != "/ok" || limits[0].Limit != 10 {
		t.Errorf("expected /ok=10, got %s=%d", limits[0].Prefix, limits[0].Limit)
	}
}

func TestLoad_EndpointRateLimits(t *testing.T) {
	t.Setenv("ENDPOINT_RATE_LIMITS", "/api/chat=10;/admin/=5")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.EndpointRateLimits) != 2 {
		t.Fatalf("expected 2 endpoint limits, got %d", len(cfg.EndpointRateLimits))
	}
}
