package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =============================================================
// Integration Tests — Middleware Chain Combinations
// =============================================================
// These test realistic combinations of middleware working together,
// not individual units in isolation.
// =============================================================

func TestAuthFailure_FeedsBruteForceGuard(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	guard := NewBruteForceGuard(3, 1*time.Minute)
	defer guard.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthRequiredWithGuard(auth, "proxy", nil, guard, ok)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.99:1234"
		req.Header.Set("Authorization", "Bearer bad-token-attempt")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}

	banned, reason, _ := guard.IsBanned("10.0.0.99")
	if !banned {
		t.Fatal("expected IP to be banned after 3 auth failures")
	}
	if reason != "invalid_token" {
		t.Errorf("expected reason=invalid_token, got %q", reason)
	}
}

func TestAuthSuccess_ClearsStrikes(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	guard := NewBruteForceGuard(3, 1*time.Minute)
	defer guard.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthRequiredWithGuard(auth, "proxy", nil, guard, ok)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.50:1234"
		req.Header.Set("Authorization", "Bearer bad-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	token, _ := auth.CreateToken("user1", "proxy", nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.50:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 after valid token, got %d", rr.Code)
	}

	banned, _, _ := guard.IsBanned("10.0.0.50")
	if banned {
		t.Fatal("expected strikes cleared after successful auth")
	}
}

func TestRateLimitDenial_FeedsBruteForceGuard(t *testing.T) {
	rl := NewRateLimiter(2, 1*time.Minute)
	defer rl.Stop()
	guard := NewBruteForceGuard(3, 1*time.Minute)
	defer guard.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitWithGuard(rl, guard, ok)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.77:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	banned, reason, _ := guard.IsBanned("10.0.0.77")
	if !banned {
		t.Fatal("expected IP banned after repeated rate limit violations")
	}
	if reason != "rate_limit_exceeded" {
		t.Errorf("expected reason=rate_limit_exceeded, got %q", reason)
	}
}

func TestBruteForceMiddleware_ChainedWithAuth(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	guard := NewBruteForceGuard(2, 1*time.Minute)
	defer guard.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authHandler := AuthRequiredWithGuard(auth, "proxy", nil, guard, ok)
	handler := BruteForceMiddleware(guard, authHandler)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.88:1234"
		req.Header.Set("Authorization", "Bearer garbage")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.88:1234"
	req.Header.Set("Authorization", "Bearer another-attempt")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (banned), got %d", rr.Code)
	}
}

func TestRevocationList_WithNilRedis_WorksInMemory(t *testing.T) {
	rl := NewRevocationListWithRedis(1*time.Hour, nil)
	defer rl.Stop()

	rl.Revoke("alice", "compromised")

	revoked, reason := rl.IsRevoked("alice", time.Now().Unix())
	if !revoked {
		t.Fatal("expected revoked")
	}
	if reason != "compromised" {
		t.Errorf("expected reason=compromised, got %q", reason)
	}

	revoked, _ = rl.IsRevoked("bob", time.Now().Unix())
	if revoked {
		t.Fatal("expected bob not revoked")
	}
}

func TestMissingToken_FeedsBruteForceGuard(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	guard := NewBruteForceGuard(2, 1*time.Minute)
	defer guard.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthRequiredWithGuard(auth, "proxy", nil, guard, ok)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.33:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	banned, reason, _ := guard.IsBanned("10.0.0.33")
	if !banned {
		t.Fatal("expected IP banned after repeated missing-token requests")
	}
	if reason != "missing_token" {
		t.Errorf("expected reason=missing_token, got %q", reason)
	}
}

func TestRevokedToken_FeedsBruteForceGuard(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	guard := NewBruteForceGuard(2, 1*time.Minute)
	defer guard.Stop()
	rl := NewRevocationList(168 * time.Hour)
	defer rl.Stop()

	token, _ := auth.CreateToken("baduser", "proxy", nil)
	time.Sleep(1100 * time.Millisecond)
	rl.Revoke("baduser", "leaked")
	time.Sleep(100 * time.Millisecond)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthRequiredWithGuard(auth, "proxy", rl, guard, ok)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.44:1234"
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}

	banned, reason, _ := guard.IsBanned("10.0.0.44")
	if !banned {
		t.Fatal("expected IP banned after repeated revoked-token attempts")
	}
	if reason != "token_revoked" {
		t.Errorf("expected reason=token_revoked, got %q", reason)
	}
}

// =============================================================
// E2E Privilege Check Tests
// =============================================================

func TestScopeEnforcement_ProxyCannotAccessAdmin(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)

	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("admin-only"))
	})

	handler := AuthRequired(auth, "admin", nil, adminHandler)

	proxyToken, _ := auth.CreateToken("user1", "proxy", nil)
	req := httptest.NewRequest("POST", "/admin/revoke", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+proxyToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("proxy-scoped token should not access admin endpoint, got %d", rr.Code)
	}
}

func TestScopeEnforcement_AdminCanAccessProxy(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)

	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthRequired(auth, "proxy", nil, proxyHandler)

	adminToken, _ := auth.CreateToken("admin", "admin", nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("admin-scoped token should access proxy endpoints, got %d", rr.Code)
	}
}

func TestScopeEnforcement_UnknownScopeRejected(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)

	handler := AuthRequired(auth, "proxy", nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token, _ := auth.CreateToken("user1", "readonly", nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unknown scope should be rejected, got %d", rr.Code)
	}
}

func TestFullChain_RateLimitThenAuthThenBan(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	rl := NewRateLimiter(100, 1*time.Minute) // High limit so rate limit doesn't interfere
	defer rl.Stop()
	guard := NewBruteForceGuard(3, 1*time.Minute)
	defer guard.Stop()

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Build chain: BruteForce → RateLimit → Auth → backend
	authHandler := AuthRequiredWithGuard(auth, "proxy", nil, guard, backend)
	rateLimitHandler := RateLimitWithGuard(rl, guard, authHandler)
	handler := BruteForceMiddleware(guard, rateLimitHandler)

	// Send 3 bad auth requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.200:1234"
		req.Header.Set("Authorization", "Bearer garbage")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}

	// Now even a valid token should be blocked (IP is banned)
	validToken, _ := auth.CreateToken("user1", "proxy", nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.200:1234"
	req.Header.Set("Authorization", "Bearer "+validToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("banned IP should get 403 even with valid token, got %d", rr.Code)
	}
}

func TestFullChain_CleanIPPassesThrough(t *testing.T) {
	auth, _ := NewAuthenticator([]byte("test-secret-that-is-long-enough-for-hmac-sha256"), 24*time.Hour, 168*time.Hour)
	rl := NewRateLimiter(100, 1*time.Minute)
	defer rl.Stop()
	guard := NewBruteForceGuard(10, 1*time.Minute)
	defer guard.Stop()

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	authHandler := AuthRequiredWithGuard(auth, "proxy", nil, guard, backend)
	rateLimitHandler := RateLimitWithGuard(rl, guard, authHandler)
	handler := BruteForceMiddleware(guard, rateLimitHandler)

	token, _ := auth.CreateToken("gooduser", "proxy", nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.100:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("clean IP with valid token should get 200, got %d", rr.Code)
	}
}

func TestStripAuthHeaders_PreventsIdentitySpoofing(t *testing.T) {
	var capturedSubject string
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSubject = r.Header.Get("X-Auth-Subject")
		w.WriteHeader(http.StatusOK)
	})

	handler := StripAuthHeaders(backend)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth-Subject", "spoofed-admin")
	req.Header.Set("X-Auth-Scope", "admin")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedSubject != "" {
		t.Fatalf("StripAuthHeaders should remove X-Auth-Subject, but got %q", capturedSubject)
	}
}
