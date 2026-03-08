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

	// 1st ban (2 strikes)
	g.RecordFailure("10.0.0.1", "test")
	g.RecordFailure("10.0.0.1", "test")
	_, _, d1 := g.IsBanned("10.0.0.1")

	if d1 > 5*time.Minute+time.Second || d1 < 4*time.Minute {
		t.Errorf("expected ~5min for 1st ban, got %v", d1)
	}
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
