package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testSecret = "this-is-a-test-secret-that-is-at-least-32-bytes-long!"

func newTestAuth(t *testing.T) *Authenticator {
	t.Helper()
	auth, err := NewAuthenticator([]byte(testSecret), 24*time.Hour, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestNewAuthenticator_ShortSecret(t *testing.T) {
	_, err := NewAuthenticator([]byte("short"), time.Hour, time.Hour)
	if err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestCreateAndValidateToken(t *testing.T) {
	auth := newTestAuth(t)
	token, err := auth.CreateToken("user1", "proxy", nil)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.Sub != "user1" {
		t.Errorf("sub = %q, want user1", claims.Sub)
	}
	if claims.Scope != "proxy" {
		t.Errorf("scope = %q, want proxy", claims.Scope)
	}
}

func TestValidateToken_Expired(t *testing.T) {
	auth := newTestAuth(t)
	auth.clockSkew = 0
	token, _ := auth.CreateTokenWithTTL("user1", "proxy", nil, 1*time.Second)
	time.Sleep(2100 * time.Millisecond)
	_, err := auth.ValidateToken(token)
	if err == nil {
		t.Fatal("expired token should be rejected")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	auth1 := newTestAuth(t)
	auth2, _ := NewAuthenticator([]byte("different-secret-that-is-also-32-bytes!!"), time.Hour, time.Hour)
	token, _ := auth1.CreateToken("user1", "proxy", nil)
	_, err := auth2.ValidateToken(token)
	if err == nil {
		t.Fatal("token signed with different secret should be rejected")
	}
}

func TestValidateToken_InvalidFormat(t *testing.T) {
	auth := newTestAuth(t)
	invalids := []string{"", "notsplit", "too.many.dots", ".empty", "empty."}
	for _, tok := range invalids {
		_, err := auth.ValidateToken(tok)
		if err == nil {
			t.Errorf("ValidateToken(%q) should fail", tok)
		}
	}
}

func TestValidateToken_EmptySubject(t *testing.T) {
	auth := newTestAuth(t)
	_, err := auth.CreateToken("", "proxy", nil)
	if err == nil {
		t.Fatal("empty subject should be rejected at creation")
	}
}

func TestValidateToken_DefaultScope(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("user1", "", nil)
	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Scope != "ws" {
		t.Errorf("default scope = %q, want ws", claims.Scope)
	}
}

func TestCreateToken_TTLExceedsMax(t *testing.T) {
	auth := newTestAuth(t)
	_, err := auth.CreateTokenWithTTL("user1", "proxy", nil, 365*24*time.Hour)
	if err == nil {
		t.Fatal("TTL exceeding max should be rejected")
	}
}

func TestTokenClaims_IsExpired(t *testing.T) {
	past := &TokenClaims{Exp: time.Now().Unix() - 100}
	if !past.IsExpired() {
		t.Error("past token should be expired")
	}
	future := &TokenClaims{Exp: time.Now().Unix() + 3600}
	if future.IsExpired() {
		t.Error("future token should not be expired")
	}
}

func TestTokenClaims_RemainingTTL(t *testing.T) {
	c := &TokenClaims{Exp: time.Now().Unix() + 60}
	ttl := c.RemainingTTL()
	if ttl < 55*time.Second || ttl > 65*time.Second {
		t.Errorf("remaining TTL = %v, expected ~60s", ttl)
	}
	expired := &TokenClaims{Exp: time.Now().Unix() - 10}
	if expired.RemainingTTL() != 0 {
		t.Error("expired token TTL should be 0")
	}
}

// --- HTTP middleware tests ---

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

func TestAuthRequired_NoToken(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthRequired_ValidToken_Bearer(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("user1", "proxy", nil)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthRequired_ValidToken_QueryParam(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("user1", "proxy", nil)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	// ?token= is only accepted for WebSocket upgrade requests
	req := httptest.NewRequest("GET", "/test?token="+token, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthRequired_QueryParam_RejectedForHTTP(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("user1", "proxy", nil)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	// Plain HTTP request with ?token= should be rejected (use Bearer header instead)
	req := httptest.NewRequest("GET", "/test?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (?token= should not work for plain HTTP)", rec.Code)
	}
}

func TestAuthRequired_InvalidToken(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer garbage.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthRequired_WrongScope(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("user1", "ws", nil)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for wrong scope", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "insufficient_scope" {
		t.Errorf("error = %q, want insufficient_scope", body["error"])
	}
}

func TestAuthRequired_AdminScopeBypassesCheck(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("admin1", "admin", nil)
	handler := AuthRequired(auth, "proxy", nil, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("admin scope should bypass proxy check, got %d", rec.Code)
	}
}

func TestAuthOptional_NoToken(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthOptional(auth, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("optional auth with no token should pass, got %d", rec.Code)
	}
}

func TestAuthOptional_InvalidToken(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthOptional(auth, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("optional auth with invalid token should still pass, got %d", rec.Code)
	}
}

func TestAuthOptional_ValidToken_SetsHeaders(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("viewer1", "proxy", nil)

	var gotSubject, gotScope string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotSubject = r.Header.Get("X-Auth-Subject")
		gotScope = r.Header.Get("X-Auth-Scope")
	})

	handler := AuthOptional(auth, inner)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if gotSubject != "viewer1" {
		t.Errorf("X-Auth-Subject = %q, want %q", gotSubject, "viewer1")
	}
	if gotScope != "proxy" {
		t.Errorf("X-Auth-Scope = %q, want %q", gotScope, "proxy")
	}
}

// --- Revocation tests ---

func TestRevocationList_RevokeAndCheck(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	pastIat := time.Now().Unix() - 60
	revoked, _ := rl.IsRevoked("user1", pastIat)
	if revoked {
		t.Error("should not be revoked before Revoke() is called")
	}

	rl.Revoke("user1", "compromised")

	revoked, reason := rl.IsRevoked("user1", pastIat)
	if !revoked {
		t.Error("token with iat before revocation should be revoked")
	}
	if reason != "compromised" {
		t.Errorf("reason = %q, want compromised", reason)
	}

	futureIat := time.Now().Unix() + 10
	revoked, _ = rl.IsRevoked("user1", futureIat)
	if revoked {
		t.Error("token issued after revocation should NOT be revoked")
	}
}

func TestRevocationList_DifferentSubjects(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	rl.Revoke("user1", "leaked")
	revoked, _ := rl.IsRevoked("user2", time.Now().Unix()-60)
	if revoked {
		t.Error("user2 should not be affected by user1 revocation")
	}
}

func TestAuthRequired_RevokedToken(t *testing.T) {
	auth := newTestAuth(t)
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	token, _ := auth.CreateToken("user1", "proxy", nil)

	time.Sleep(10 * time.Millisecond)
	rl.Revoke("user1", "test-revoke")

	handler := AuthRequired(auth, "proxy", rl, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked token should get 401, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "token_revoked" {
		t.Errorf("error = %q, want token_revoked", body["error"])
	}
}

func TestAuthRequired_TokenAfterRevocation_Passes(t *testing.T) {
	auth := newTestAuth(t)
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	rl.Revoke("user1", "old-revoke")
	time.Sleep(1100 * time.Millisecond) // Ensure next second for iat comparison

	token, _ := auth.CreateToken("user1", "proxy", nil)
	handler := AuthRequired(auth, "proxy", rl, okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("token issued after revocation should pass, got %d", rec.Code)
	}
}

func TestRevocationList_Count(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()
	if rl.Count() != 0 {
		t.Error("empty list should have count 0")
	}
	rl.Revoke("a", "test")
	rl.Revoke("b", "test")
	if rl.Count() != 2 {
		t.Errorf("count = %d, want 2", rl.Count())
	}
}

func TestAuthRequiredWithGuard_HealthExempt(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthRequiredWithGuard(auth, "proxy", nil, nil, okHandler())
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/health should be exempt from auth, got %d", rec.Code)
	}
}

func TestAuthRequiredWithGuard_MetricsExempt(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthRequiredWithGuard(auth, "proxy", nil, nil, okHandler())
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/metrics should be exempt from auth, got %d", rec.Code)
	}
}

func TestAuthRequiredWithGuard_OtherPathsRequireAuth(t *testing.T) {
	auth := newTestAuth(t)
	handler := AuthRequiredWithGuard(auth, "proxy", nil, nil, okHandler())
	req := httptest.NewRequest("GET", "/api/chat", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("/api/chat without token should be 401, got %d", rec.Code)
	}
}
