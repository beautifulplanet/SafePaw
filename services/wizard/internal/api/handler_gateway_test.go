package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"safepaw/wizard/internal/config"
)

// newTestHandlerWithEnv creates a handler with a temporary .env file.
func newTestHandlerWithEnv(t *testing.T, envContent string) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := &config.Config{
		Port:          3000,
		AdminPassword: "test-password-123",
		DockerHost:    "unix:///var/run/docker.sock",
		EnvFilePath:   envPath,
	}
	h, err := NewHandler(cfg, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h, envPath
}

// ─── Health / NeedsSetup ─────────────────────────────────────

func TestHealthNeedsSetup_True(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "# no keys\n")
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !resp.NeedsSetup {
		t.Error("NeedsSetup should be true when no API keys configured")
	}
}

func TestHealthNeedsSetup_FalseWithAnthropicKey(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "ANTHROPIC_API_KEY=sk-ant-test-123\n")
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	var resp healthResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.NeedsSetup {
		t.Error("NeedsSetup should be false when ANTHROPIC_API_KEY is set")
	}
}

func TestHealthNeedsSetup_FalseWithOpenAIKey(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "OPENAI_API_KEY=sk-test-123\n")
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	var resp healthResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.NeedsSetup {
		t.Error("NeedsSetup should be false when OPENAI_API_KEY is set")
	}
}

func TestHealthNeedsSetup_TrueWhenEnvFileMissing(t *testing.T) {
	cfg := &config.Config{
		Port:          3000,
		AdminPassword: "test-password-123",
		DockerHost:    "unix:///var/run/docker.sock",
		EnvFilePath:   "/nonexistent/.env",
	}
	h, _ := NewHandler(cfg, nil)
	router := h.Router()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	var resp healthResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.NeedsSetup {
		t.Error("NeedsSetup should be true when .env file is unreadable")
	}
}

// ─── Gateway Token ───────────────────────────────────────────

func TestGatewayToken_Success(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	h, _ := newTestHandlerWithEnv(t, fmt.Sprintf("AUTH_SECRET=%s\n", secret))
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{Subject: "test-user", Scope: "proxy", TTLHrs: 1})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	var resp gatewayTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("Token should not be empty")
	}
	if resp.ExpiresAt == "" {
		t.Fatal("ExpiresAt should not be empty")
	}

	// Verify the token is valid HMAC — parse and check signature
	parts := splitTokenString(resp.Token)
	if len(parts) != 2 {
		t.Fatalf("Token should have 2 parts separated by '.', got %d", len(parts))
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("Decode payload: %v", err)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("Decode sig: %v", err)
	}

	// Verify HMAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadBytes)
	if !hmac.Equal(sigBytes, mac.Sum(nil)) {
		t.Error("Token signature is invalid")
	}

	// Verify payload
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		t.Fatalf("Unmarshal claims: %v", err)
	}
	if claims["sub"] != "test-user" {
		t.Errorf("sub = %v, want test-user", claims["sub"])
	}
	if claims["scope"] != "proxy" {
		t.Errorf("scope = %v, want proxy", claims["scope"])
	}
	exp := int64(claims["exp"].(float64))
	if exp <= time.Now().Unix() {
		t.Error("Token should not already be expired")
	}
}

func TestGatewayToken_NoAuthSecret(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "# no auth secret\n")
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want 400 when AUTH_SECRET missing", rec.Code)
	}
}

func TestGatewayToken_ShortSecret(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "AUTH_SECRET=short\n")
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want 400 when AUTH_SECRET too short", rec.Code)
	}
}

func TestGatewayToken_TTLTooLarge(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	h, _ := newTestHandlerWithEnv(t, fmt.Sprintf("AUTH_SECRET=%s\n", secret))
	router := h.Router()

	body, _ := json.Marshal(gatewayTokenRequest{TTLHrs: 999})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Status = %d, want 400 when TTL too large", rec.Code)
	}
}

func TestGatewayToken_DefaultValues(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	h, _ := newTestHandlerWithEnv(t, fmt.Sprintf("AUTH_SECRET=%s\n", secret))
	router := h.Router()

	// Empty request — should use defaults
	body, _ := json.Marshal(gatewayTokenRequest{})
	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}

	var resp gatewayTokenResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	// Parse the token payload to check defaults
	parts := splitTokenString(resp.Token)
	payloadBytes, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var claims map[string]interface{}
	_ = json.Unmarshal(payloadBytes, &claims)

	if claims["sub"] != "wizard-proxy" {
		t.Errorf("default sub = %v, want wizard-proxy", claims["sub"])
	}
	if claims["scope"] != "proxy" {
		t.Errorf("default scope = %v, want proxy", claims["scope"])
	}
}

func TestGatewayToken_BadBody(t *testing.T) {
	secret := "this-is-a-very-long-test-secret-that-is-at-least-32-bytes"
	h, _ := newTestHandlerWithEnv(t, fmt.Sprintf("AUTH_SECRET=%s\n", secret))
	router := h.Router()

	req := httptest.NewRequest("POST", "/api/v1/gateway/token", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400 for bad body", rec.Code)
	}
}

// splitTokenString splits on "." — test helper (we can't import the gateway's splitToken).
func splitTokenString(token string) []string {
	idx := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{token}
	}
	return []string{token[:idx], token[idx+1:]}
}

// ─── Gateway Metrics ─────────────────────────────────────────

func TestParsePrometheusMetrics(t *testing.T) {
	sample := `# HELP safepaw_requests_total Total requests
# TYPE safepaw_requests_total counter
safepaw_requests_total{method="GET",path="/health"} 42
safepaw_requests_total{method="POST",path="/ws"} 100
# HELP safepaw_auth_failures_total Total auth failures
safepaw_auth_failures_total 5
safepaw_injection_detected_total 2
safepaw_rate_limited_total 3
safepaw_active_connections 7
safepaw_tokens_revoked_total 1
safepaw_request_duration_seconds_sum 1.5
`
	s := parsePrometheusMetrics(sample)

	if s.TotalRequests != 142 {
		t.Errorf("TotalRequests = %d, want 142", s.TotalRequests)
	}
	if s.AuthFailures != 5 {
		t.Errorf("AuthFailures = %d, want 5", s.AuthFailures)
	}
	if s.InjectionsFound != 2 {
		t.Errorf("InjectionsFound = %d, want 2", s.InjectionsFound)
	}
	if s.RateLimited != 3 {
		t.Errorf("RateLimited = %d, want 3", s.RateLimited)
	}
	if s.ActiveConns != 7 {
		t.Errorf("ActiveConns = %d, want 7", s.ActiveConns)
	}
	if s.TokensRevoked != 1 {
		t.Errorf("TokensRevoked = %d, want 1", s.TokensRevoked)
	}
	if s.AvgResponseMs < 1400 || s.AvgResponseMs > 1600 {
		t.Errorf("AvgResponseMs = %f, want ~1500", s.AvgResponseMs)
	}
}

func TestParsePrometheusMetrics_Empty(t *testing.T) {
	s := parsePrometheusMetrics("")
	if s.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d, want 0 for empty input", s.TotalRequests)
	}
}

func TestParseTopPaths(t *testing.T) {
	sample := `safepaw_requests_total{method="GET",path="/health"} 42
safepaw_requests_total{method="POST",path="/ws"} 100
safepaw_requests_total{method="GET",path="/metrics"} 10
`
	paths := parseTopPaths(sample)

	if len(paths) != 3 {
		t.Fatalf("len(paths) = %d, want 3", len(paths))
	}
	// Should be sorted descending by count
	if paths[0].Path != "/ws" || paths[0].Count != 100 {
		t.Errorf("paths[0] = %v, want /ws:100", paths[0])
	}
	if paths[1].Path != "/health" || paths[1].Count != 42 {
		t.Errorf("paths[1] = %v, want /health:42", paths[1])
	}
}

func TestExtractLabel(t *testing.T) {
	line := `safepaw_requests_total{method="GET",path="/health"} 42`

	if got := extractLabel(line, "path"); got != "/health" {
		t.Errorf("path = %q, want /health", got)
	}
	if got := extractLabel(line, "method"); got != "GET" {
		t.Errorf("method = %q, want GET", got)
	}
	if got := extractLabel(line, "nonexistent"); got != "" {
		t.Errorf("nonexistent = %q, want empty", got)
	}
}

// ─── Gateway Metrics HTTP (unreachable gateway) ──────────────

func TestGatewayMetrics_UnreachableGateway(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "")
	router := h.Router()

	// Set GATEWAY_URL to something unreachable
	t.Setenv("GATEWAY_URL", "http://127.0.0.1:1") // port 1 = almost certainly refused

	req := httptest.NewRequest("GET", "/api/v1/gateway/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200 (graceful degradation)", rec.Code)
	}

	var resp gatewayMetricsSummary
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.GatewayReachable {
		t.Error("GatewayReachable should be false when gateway is unreachable")
	}
}

func TestGatewayMetrics_WithMockGateway(t *testing.T) {
	// Start a mock metrics server
	metricsText := `safepaw_requests_total{method="GET",path="/health"} 50
safepaw_auth_failures_total 3
safepaw_active_connections 2
`
	mockGW := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(metricsText))
			return
		}
		w.WriteHeader(404)
	}))
	defer mockGW.Close()

	h, _ := newTestHandlerWithEnv(t, "")
	router := h.Router()
	t.Setenv("GATEWAY_URL", mockGW.URL)

	req := httptest.NewRequest("GET", "/api/v1/gateway/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}

	var resp gatewayMetricsSummary
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if !resp.GatewayReachable {
		t.Error("GatewayReachable should be true")
	}
	if resp.TotalRequests != 50 {
		t.Errorf("TotalRequests = %d, want 50", resp.TotalRequests)
	}
	if resp.AuthFailures != 3 {
		t.Errorf("AuthFailures = %d, want 3", resp.AuthFailures)
	}
	if resp.ActiveConns != 2 {
		t.Errorf("ActiveConns = %d, want 2", resp.ActiveConns)
	}
}

// ─── Gateway Activity ────────────────────────────────────────

func TestGatewayActivity_WithMockGateway(t *testing.T) {
	metricsText := `safepaw_requests_total{method="GET",path="/health"} 10
safepaw_requests_total{method="POST",path="/ws"} 50
safepaw_auth_failures_total 1
`
	mockGW := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			_, _ = w.Write([]byte(metricsText))
			return
		}
		w.WriteHeader(404)
	}))
	defer mockGW.Close()

	h, _ := newTestHandlerWithEnv(t, "")
	router := h.Router()
	t.Setenv("GATEWAY_URL", mockGW.URL)

	req := httptest.NewRequest("GET", "/api/v1/gateway/activity", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200", rec.Code)
	}

	var resp gatewayActivity
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Metrics.TotalRequests != 60 {
		t.Errorf("TotalRequests = %d, want 60", resp.Metrics.TotalRequests)
	}
	if len(resp.TopPaths) != 2 {
		t.Fatalf("len(TopPaths) = %d, want 2", len(resp.TopPaths))
	}
	if resp.TopPaths[0].Path != "/ws" {
		t.Errorf("TopPaths[0].Path = %q, want /ws", resp.TopPaths[0].Path)
	}
}

func TestGatewayActivity_UnreachableGateway(t *testing.T) {
	h, _ := newTestHandlerWithEnv(t, "")
	router := h.Router()
	t.Setenv("GATEWAY_URL", "http://127.0.0.1:1")

	req := httptest.NewRequest("GET", "/api/v1/gateway/activity", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, asAdmin(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200 (graceful degradation)", rec.Code)
	}
}
