// =============================================================
// SafePaw Gateway - Brute Force Protection
// =============================================================
// Automatic IP banning for repeated abuse. Works in tandem with
// the rate limiter to escalate responses:
//
//   1st offense:  Rate limiter returns 429 (temporary)
//   Nth offense:  BruteForceGuard bans the IP (sticky)
//
// Ban duration escalates exponentially:
//   - 1st ban:    5 minutes
//   - 2nd ban:   15 minutes
//   - 3rd ban:   60 minutes
//   - 4th+ ban: 240 minutes
//
// This prevents:
//   - Credential stuffing (repeated auth failures)
//   - Prompt injection fuzzing (repeated scanner hits)
//   - DoS via rate limit boundary riding
//
// Bans auto-expire. No persistence (restart = clean slate).
// For production: integrate with fail2ban or WAF for IP-level blocks.
// =============================================================

package middleware

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type banEntry struct {
	strikes  int
	bannedAt time.Time
	duration time.Duration
	reason   string
}

// BruteForceGuard tracks and bans abusive IPs.
type BruteForceGuard struct {
	mu        sync.Mutex
	strikes   map[string]*banEntry
	threshold int
	baseBan   time.Duration
	cleanupT  *time.Ticker
	stopCh    chan struct{}
}

// NewBruteForceGuard creates a guard that bans after threshold strikes.
func NewBruteForceGuard(threshold int, baseBan time.Duration) *BruteForceGuard {
	g := &BruteForceGuard{
		strikes:   make(map[string]*banEntry),
		threshold: threshold,
		baseBan:   baseBan,
		cleanupT:  time.NewTicker(5 * time.Minute),
		stopCh:    make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-g.cleanupT.C:
				g.cleanup()
			case <-g.stopCh:
				return
			}
		}
	}()

	return g
}

// RecordFailure increments the strike counter for an IP.
// Returns true if the IP is now banned.
func (g *BruteForceGuard) RecordFailure(ip, reason string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, exists := g.strikes[ip]
	if !exists {
		entry = &banEntry{}
		g.strikes[ip] = entry
	}

	entry.strikes++
	entry.reason = reason

	if entry.strikes >= g.threshold {
		entry.duration = g.escalatedDuration(entry.strikes)
		entry.bannedAt = time.Now()
		log.Printf("[SECURITY] IP BANNED ip=%s strikes=%d duration=%v reason=%s",
			ip, entry.strikes, entry.duration, reason)
		return true
	}

	return false
}

// IsBanned checks whether an IP is currently banned.
func (g *BruteForceGuard) IsBanned(ip string) (bool, string, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, exists := g.strikes[ip]
	if !exists {
		return false, "", 0
	}

	if entry.bannedAt.IsZero() {
		return false, "", 0
	}

	remaining := entry.duration - time.Since(entry.bannedAt)
	if remaining <= 0 {
		return false, "", 0
	}

	return true, entry.reason, remaining
}

// Reset clears all strikes for an IP (e.g., after successful auth).
func (g *BruteForceGuard) Reset(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.strikes, ip)
}

// Decrement reduces the strike counter for an IP by one on successful auth.
// Unlike Reset, this prevents an attacker with one valid token from clearing
// all accumulated strikes between brute-force guesses.
func (g *BruteForceGuard) Decrement(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, exists := g.strikes[ip]
	if !exists {
		return
	}
	entry.strikes--
	if entry.strikes <= 0 {
		delete(g.strikes, ip)
	}
}

func (g *BruteForceGuard) escalatedDuration(strikes int) time.Duration {
	bans := strikes / g.threshold
	switch {
	case bans <= 1:
		return g.baseBan
	case bans == 2:
		return g.baseBan * 3
	case bans == 3:
		return g.baseBan * 12
	default:
		return g.baseBan * 48
	}
}

func (g *BruteForceGuard) cleanup() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for ip, entry := range g.strikes {
		if !entry.bannedAt.IsZero() && now.Sub(entry.bannedAt) > entry.duration {
			delete(g.strikes, ip)
		}
	}
}

// Stop terminates background cleanup.
func (g *BruteForceGuard) Stop() {
	g.cleanupT.Stop()
	close(g.stopCh)
}

// BannedIPs returns the count of currently banned IPs.
func (g *BruteForceGuard) BannedIPs() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	count := 0
	now := time.Now()
	for _, entry := range g.strikes {
		if !entry.bannedAt.IsZero() {
			remaining := entry.duration - now.Sub(entry.bannedAt)
			if remaining > 0 {
				count++
			}
		}
	}
	return count
}

// BruteForceMiddleware rejects requests from banned IPs before
// any further processing. Health and metrics endpoints are exempt
// so internal monitors (wizard, Docker health checks) are never blocked.
func BruteForceMiddleware(guard *BruteForceGuard, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt monitoring endpoints from brute-force blocking
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		ip := extractIP(r)

		banned, reason, remaining := guard.IsBanned(ip)
		if banned {
			log.Printf("[SECURITY] BLOCKED banned ip=%s reason=%s remaining=%v request_id=%s",
				ip, reason, remaining.Round(time.Second), r.Header.Get("X-Request-ID"))
			if sc := GetSecurityContext(r); sc != nil {
				sc.BruteForce = &BruteForceDecision{Banned: true, Reason: reason}
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", remaining.Seconds()))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if sc := GetSecurityContext(r); sc != nil {
			sc.BruteForce = &BruteForceDecision{Banned: false}
		}

		next.ServeHTTP(w, r)
	})
}
