package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBruteForceGuard_NotBannedInitially(t *testing.T) {
	g := NewBruteForceGuard(3, 1*time.Minute)
	defer g.Stop()

	banned, _, _ := g.IsBanned("10.0.0.1")
	if banned {
		t.Error("expected IP not to be banned initially")
	}
}

func TestBruteForceGuard_BansAfterThreshold(t *testing.T) {
	g := NewBruteForceGuard(3, 1*time.Minute)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "auth_failure")
	g.RecordFailure("10.0.0.1", "auth_failure")

	banned, _, _ := g.IsBanned("10.0.0.1")
	if banned {
		t.Error("expected not banned before threshold")
	}

	result := g.RecordFailure("10.0.0.1", "auth_failure")
	if !result {
		t.Error("expected RecordFailure to return true on ban")
	}

	banned, reason, remaining := g.IsBanned("10.0.0.1")
	if !banned {
		t.Error("expected IP to be banned after threshold")
	}
	if reason != "auth_failure" {
		t.Errorf("expected reason=auth_failure, got %q", reason)
	}
	if remaining <= 0 {
		t.Error("expected positive remaining duration")
	}
}

func TestBruteForceGuard_DifferentIPsIndependent(t *testing.T) {
	g := NewBruteForceGuard(2, 1*time.Minute)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "test")
	g.RecordFailure("10.0.0.1", "test")

	banned1, _, _ := g.IsBanned("10.0.0.1")
	banned2, _, _ := g.IsBanned("10.0.0.2")

	if !banned1 {
		t.Error("expected 10.0.0.1 to be banned")
	}
	if banned2 {
		t.Error("expected 10.0.0.2 to NOT be banned")
	}
}

func TestBruteForceGuard_Reset(t *testing.T) {
	g := NewBruteForceGuard(2, 1*time.Minute)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "test")
	g.RecordFailure("10.0.0.1", "test")

	banned, _, _ := g.IsBanned("10.0.0.1")
	if !banned {
		t.Error("expected banned before reset")
	}

	g.Reset("10.0.0.1")
	banned, _, _ = g.IsBanned("10.0.0.1")
	if banned {
		t.Error("expected not banned after reset")
	}
}

func TestBruteForceGuard_BannedIPs(t *testing.T) {
	g := NewBruteForceGuard(1, 1*time.Minute)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "test")
	g.RecordFailure("10.0.0.2", "test")

	if g.BannedIPs() != 2 {
		t.Errorf("expected 2 banned IPs, got %d", g.BannedIPs())
	}
}

func TestBruteForceGuard_EscalatedDuration(t *testing.T) {
	g := NewBruteForceGuard(2, 5*time.Minute)
	defer g.Stop()

	ip := "10.0.0.1"

	// 1st ban: 2 strikes → bans=1 → baseBan (5m)
	g.RecordFailure(ip, "test")
	g.RecordFailure(ip, "test")
	_, _, d1 := g.IsBanned(ip)
	if d1 > 5*time.Minute+time.Second || d1 < 4*time.Minute {
		t.Errorf("expected ~5min for 1st ban, got %v", d1)
	}

	// 2nd ban: 4 strikes → bans=2 → baseBan*3 (15m)
	g.RecordFailure(ip, "test")
	g.RecordFailure(ip, "test")
	_, _, d2 := g.IsBanned(ip)
	if d2 > 15*time.Minute+time.Second || d2 < 14*time.Minute {
		t.Errorf("expected ~15min for 2nd ban, got %v", d2)
	}

	// 3rd ban: 6 strikes → bans=3 → baseBan*12 (60m)
	g.RecordFailure(ip, "test")
	g.RecordFailure(ip, "test")
	_, _, d3 := g.IsBanned(ip)
	if d3 > 60*time.Minute+time.Second || d3 < 59*time.Minute {
		t.Errorf("expected ~60min for 3rd ban, got %v", d3)
	}

	// 4th ban: 8 strikes → bans=4 → baseBan*48 (240m)
	g.RecordFailure(ip, "test")
	g.RecordFailure(ip, "test")
	_, _, d4 := g.IsBanned(ip)
	if d4 > 240*time.Minute+time.Second || d4 < 239*time.Minute {
		t.Errorf("expected ~240min for 4th ban, got %v", d4)
	}
}

func TestBruteForceGuard_Decrement(t *testing.T) {
	g := NewBruteForceGuard(3, 5*time.Minute)
	defer g.Stop()

	ip := "10.0.0.1"
	g.RecordFailure(ip, "test")
	g.RecordFailure(ip, "test")

	// Decrement should reduce strikes
	g.Decrement(ip)
	banned, _, _ := g.IsBanned(ip)
	if banned {
		t.Error("should not be banned after decrement reduced strikes below threshold")
	}

	// Decrement to zero removes entry
	g.Decrement(ip)
	g.Decrement(ip) // no-op on non-existent

	// Decrement non-existent IP is safe
	g.Decrement("10.0.0.99")
}

func TestBruteForceMiddleware_AllowsCleanIP(t *testing.T) {
	g := NewBruteForceGuard(3, 1*time.Minute)
	defer g.Stop()

	handler := BruteForceMiddleware(g, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBruteForceMiddleware_BlocksBannedIP(t *testing.T) {
	g := NewBruteForceGuard(1, 1*time.Minute)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "test")

	handler := BruteForceMiddleware(g, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header")
	}
}

func TestBruteForceMiddleware_ExemptsHealth(t *testing.T) {
	g := NewBruteForceGuard(1, time.Minute)
	defer g.Stop()

	// Ban the IP
	g.RecordFailure("10.0.0.1", "test")

	handler := BruteForceMiddleware(g, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// /health should still be accessible even when banned
	req := httptest.NewRequest("GET", "/health", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/health should be exempt, got %d", rr.Code)
	}

	// /metrics should also be exempt
	req = httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/metrics should be exempt, got %d", rr.Code)
	}
}

func TestBruteForceGuard_Cleanup(t *testing.T) {
	g := NewBruteForceGuard(1, 50*time.Millisecond)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "test")
	banned, _, _ := g.IsBanned("10.0.0.1")
	if !banned {
		t.Error("should be banned")
	}

	// Wait for ban to expire
	time.Sleep(100 * time.Millisecond)

	// Directly call cleanup
	g.cleanup()

	// After cleanup, IP should be removed
	banned, _, _ = g.IsBanned("10.0.0.1")
	if banned {
		t.Error("should not be banned after cleanup")
	}
}

func TestBruteForceGuard_IsBanned_ExpiredBan(t *testing.T) {
	g := NewBruteForceGuard(1, 50*time.Millisecond)
	defer g.Stop()

	g.RecordFailure("10.0.0.1", "test")
	banned, _, _ := g.IsBanned("10.0.0.1")
	if !banned {
		t.Error("should be banned immediately")
	}

	// Wait for ban to expire
	time.Sleep(100 * time.Millisecond)

	// IsBanned should return false for expired ban
	banned, _, _ = g.IsBanned("10.0.0.1")
	if banned {
		t.Error("should not be banned after expiry")
	}
}

// ── Redis persistence tests ─────────────────────────────────────

func TestBruteForce_PersistBan_WithRedis(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	defer rc.Close()

	g := NewBruteForceGuardWithRedis(2, 5*time.Minute, rc)
	defer g.Stop()

	// Record failures until banned
	g.RecordFailure("192.168.1.1", "auth_failure")
	g.RecordFailure("192.168.1.1", "auth_failure")

	banned, reason, _ := g.IsBanned("192.168.1.1")
	if !banned {
		t.Fatal("expected IP to be banned after threshold")
	}
	if reason != "auth_failure" {
		t.Errorf("expected reason=auth_failure, got %q", reason)
	}

	// Verify it was persisted to Redis
	val, err := rc.Get("safepaw:ban:192.168.1.1")
	if err != nil {
		t.Fatalf("Redis GET failed: %v", err)
	}
	if val == "" {
		t.Error("expected ban to be persisted in Redis")
	}
}

func TestBruteForce_LoadBan_FromRedis(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	defer rc.Close()

	// Guard 1: ban an IP
	g1 := NewBruteForceGuardWithRedis(1, 10*time.Minute, rc)
	g1.RecordFailure("10.0.0.5", "scanner_hit")
	g1.Stop()

	// Guard 2: new guard (simulates restart), same Redis
	g2 := NewBruteForceGuardWithRedis(1, 10*time.Minute, rc)
	defer g2.Stop()

	// Should load ban from Redis on cache miss
	banned, reason, remaining := g2.IsBanned("10.0.0.5")
	if !banned {
		t.Fatal("expected ban to be loaded from Redis after restart")
	}
	if reason != "scanner_hit" {
		t.Errorf("expected reason=scanner_hit, got %q", reason)
	}
	if remaining <= 0 {
		t.Error("expected positive remaining duration")
	}
}

func TestBruteForce_DeleteBan_FromRedis(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	defer rc.Close()

	g := NewBruteForceGuardWithRedis(1, 5*time.Minute, rc)
	defer g.Stop()

	// Ban then reset
	g.RecordFailure("10.0.0.6", "test")
	g.Reset("10.0.0.6")

	// Verify deleted from Redis
	val, err := rc.Get("safepaw:ban:10.0.0.6")
	if err != nil {
		t.Fatalf("Redis GET failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected ban to be deleted from Redis, got %q", val)
	}
}

func TestBruteForce_PersistBan_NilRedis(t *testing.T) {
	// Should not panic with nil Redis
	g := NewBruteForceGuardWithRedis(1, 5*time.Minute, nil)
	defer g.Stop()

	g.RecordFailure("10.0.0.7", "test")
	banned, _, _ := g.IsBanned("10.0.0.7")
	if !banned {
		t.Error("expected ban even without Redis")
	}
}

func TestBruteForce_Decrement_WithRedis(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	defer rc.Close()

	g := NewBruteForceGuardWithRedis(3, 5*time.Minute, rc)
	defer g.Stop()

	// Record 2 failures (below threshold)
	g.RecordFailure("10.0.0.8", "test")
	g.RecordFailure("10.0.0.8", "test")

	// Decrement on successful auth
	g.Decrement("10.0.0.8")

	// One more failure should NOT trigger ban (2-1+1=2 < 3)
	banned := g.RecordFailure("10.0.0.8", "test")
	if banned {
		t.Error("expected not banned after decrement + one more failure")
	}
}

func TestBruteForce_EscalatedBan_WithRedis(t *testing.T) {
	srv := newMockRedisServer(t, "")
	defer srv.Close()

	rc := NewRedisClient(srv.Addr(), "")
	defer rc.Close()

	g := NewBruteForceGuardWithRedis(1, 5*time.Minute, rc)
	defer g.Stop()

	// First ban
	g.RecordFailure("10.0.0.9", "test")
	banned, _, dur1 := g.IsBanned("10.0.0.9")
	if !banned {
		t.Fatal("expected first ban")
	}

	// Second ban (should escalate)
	g.RecordFailure("10.0.0.9", "test")
	_, _, dur2 := g.IsBanned("10.0.0.9")
	if dur2 <= dur1 {
		t.Errorf("expected escalated duration: %v should be > %v", dur2, dur1)
	}
}
