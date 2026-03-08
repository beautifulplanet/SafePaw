package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestRevocationList_ReRevoke(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	rl.Revoke("user1", "first reason")
	rl.Revoke("user1", "second reason") // re-revoke same subject

	if rl.Count() != 1 {
		t.Errorf("count = %d, want 1 (same subject)", rl.Count())
	}
}

func TestRevocationList_IsRevoked_TokenAfterRevocation(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	rl.Revoke("user1", "compromised")

	// Token issued in the future should NOT be revoked
	futureIat := time.Now().Unix() + 3600
	revoked, _ := rl.IsRevoked("user1", futureIat)
	if revoked {
		t.Error("token issued after revocation should not be revoked")
	}

	// Token issued before revocation SHOULD be revoked
	pastIat := time.Now().Unix() - 3600
	revoked, reason := rl.IsRevoked("user1", pastIat)
	if !revoked {
		t.Error("token issued before revocation should be revoked")
	}
	if reason != "compromised" {
		t.Errorf("reason = %q, want 'compromised'", reason)
	}
}

func TestRevocationList_IsRevoked_UnknownSubject(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	revoked, _ := rl.IsRevoked("nobody", time.Now().Unix())
	if revoked {
		t.Error("unknown subject should not be revoked")
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

func TestValidateToken_BadBase64Signature(t *testing.T) {
	auth := newTestAuth(t)
	token, _ := auth.CreateToken("user1", "ws", nil)
	// Corrupt the signature part with invalid base64
	parts := splitTokenForTest(token)
	badToken := parts[0] + ".!!!invalid-base64!!!"
	_, err := auth.ValidateToken(badToken)
	if err == nil {
		t.Error("expected error for bad base64 signature")
	}
}

func TestValidateToken_BadBase64Payload(t *testing.T) {
	_, err := newTestAuth(t).ValidateToken("!!!invalid.validbase64")
	if err == nil {
		t.Error("expected error for bad base64 payload")
	}
}

func TestNewAuthenticator_DefaultTTLs(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}
	// Zero TTLs should use defaults
	auth, err := NewAuthenticator(secret, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if auth.defaultTTL != 24*time.Hour {
		t.Errorf("defaultTTL = %v, want 24h", auth.defaultTTL)
	}
	if auth.maxTTL != 7*24*time.Hour {
		t.Errorf("maxTTL = %v, want 168h", auth.maxTTL)
	}
}

func TestCreateToken_EmptySubject(t *testing.T) {
	auth := newTestAuth(t)
	_, err := auth.CreateToken("", "ws", nil)
	if err == nil {
		t.Error("expected error for empty subject")
	}
}

func TestAuthRequiredWithGuard_InvalidToken_Strike(t *testing.T) {
	auth := newTestAuth(t)
	guard := NewBruteForceGuard(5, time.Minute)
	defer guard.Stop()

	handler := AuthRequiredWithGuard(auth, "ws", nil, guard, okHandler())

	req := httptest.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func splitTokenForTest(token string) [2]string {
	for i, c := range token {
		if c == '.' {
			return [2]string{token[:i], token[i+1:]}
		}
	}
	return [2]string{token, ""}
}

// craftToken creates a validly-signed token with arbitrary claims for testing.
func craftToken(claims TokenClaims) string {
	payloadBytes, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(payloadBytes)
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", payloadB64, sigB64)
}

func TestValidateToken_EmptySub(t *testing.T) {
	auth := newTestAuth(t)
	token := craftToken(TokenClaims{
		Sub:   "",
		Scope: "ws",
		Exp:   time.Now().Add(time.Hour).Unix(),
		Iat:   time.Now().Unix(),
	})
	_, err := auth.ValidateToken(token)
	if err == nil || err.Error() != "invalid token: missing subject (sub)" {
		t.Errorf("expected missing subject error, got %v", err)
	}
}

func TestValidateToken_EmptyScope(t *testing.T) {
	auth := newTestAuth(t)
	token := craftToken(TokenClaims{
		Sub:   "user1",
		Scope: "",
		Exp:   time.Now().Add(time.Hour).Unix(),
		Iat:   time.Now().Unix(),
	})
	_, err := auth.ValidateToken(token)
	if err == nil || err.Error() != "invalid token: missing scope" {
		t.Errorf("expected missing scope error, got %v", err)
	}
}

func TestAuthRequiredWithGuard_WrongScope_Strike(t *testing.T) {
	auth := newTestAuth(t)
	guard := NewBruteForceGuard(5, time.Minute)
	defer guard.Stop()

	handler := AuthRequiredWithGuard(auth, "admin", nil, guard, okHandler())

	token, _ := auth.CreateTokenWithTTL("user1", "ws", nil, time.Hour)
	req := httptest.NewRequest("GET", "/admin", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthRequiredWithGuard_RevokedToken_Strike(t *testing.T) {
	auth := newTestAuth(t)
	guard := NewBruteForceGuard(5, time.Minute)
	defer guard.Stop()
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	token, _ := auth.CreateTokenWithTTL("user1", "ws", nil, time.Hour)
	rl.Revoke("user1", "test")

	handler := AuthRequiredWithGuard(auth, "ws", rl, guard, okHandler())

	req := httptest.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthRequiredWithGuard_Success_Decrement(t *testing.T) {
	auth := newTestAuth(t)
	guard := NewBruteForceGuard(5, time.Minute)
	defer guard.Stop()

	guard.RecordFailure("10.0.0.1", "test")
	guard.RecordFailure("10.0.0.1", "test")

	handler := AuthRequiredWithGuard(auth, "ws", nil, guard, okHandler())

	token, _ := auth.CreateTokenWithTTL("user1", "ws", nil, time.Hour)
	req := httptest.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthRequiredWithGuard_MissingToken_Strike(t *testing.T) {
	auth := newTestAuth(t)
	guard := NewBruteForceGuard(5, time.Minute)
	defer guard.Stop()

	handler := AuthRequiredWithGuard(auth, "ws", nil, guard, okHandler())

	req := httptest.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRevocationList_Cleanup(t *testing.T) {
	rl := NewRevocationList(50 * time.Millisecond)
	defer rl.Stop()

	rl.Revoke("user1", "test")
	rl.Revoke("user2", "test")

	if rl.Count() != 2 {
		t.Fatalf("expected 2 entries, got %d", rl.Count())
	}

	// Wait for entries to expire
	time.Sleep(100 * time.Millisecond)

	// Directly call cleanup
	rl.cleanup()

	if rl.Count() != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", rl.Count())
	}
}

func TestRevocationList_CleanupPartial(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	defer rl.Stop()

	// Manually insert an expired entry
	rl.mu.Lock()
	rl.entries["expired-user"] = &RevocationEntry{
		RevokedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
		Reason:    "old",
	}
	rl.entries["active-user"] = &RevocationEntry{
		RevokedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		Reason:    "fresh",
	}
	rl.mu.Unlock()

	rl.cleanup()

	if rl.Count() != 1 {
		t.Errorf("expected 1 entry after partial cleanup, got %d", rl.Count())
	}
}

func TestRevocationList_NewWithRedisNil(t *testing.T) {
	rl := NewRevocationListWithRedis(time.Hour, nil)
	defer rl.Stop()

	// Should still work, just without persistence
	rl.Revoke("user1", "test")
	revoked, reason := rl.IsRevoked("user1", time.Now().Add(-time.Minute).Unix())
	if !revoked {
		t.Error("expected revoked")
	}
	if reason != "test" {
		t.Errorf("expected reason 'test', got %q", reason)
	}
}

func TestRevocationList_StopWithoutRedis(t *testing.T) {
	rl := NewRevocationList(time.Hour)
	// Should not panic
	rl.Stop()
}

func TestValidateToken_MalformedJSON(t *testing.T) {
	auth := newTestAuth(t)
	// Create a validly-signed token whose payload is not valid JSON
	payloadBytes := []byte("this is not json")
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(payloadBytes)
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := payloadB64 + "." + sigB64

	_, err := auth.ValidateToken(token)
	if err == nil {
		t.Error("expected error for malformed JSON payload")
	}
}
