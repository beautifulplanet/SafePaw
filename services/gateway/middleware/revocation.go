// =============================================================
// SafePaw Gateway - Token Revocation (Phase 2 → Phase 3)
// =============================================================
// Revocation list for invalidating leaked or compromised tokens
// before their natural expiry.
//
// Phase 2: In-memory only (original).
// Phase 3: Redis-backed persistence (current). Falls back to
//          in-memory if Redis is unavailable.
//
// HOW IT WORKS:
//   1. Admin calls POST /admin/revoke with a subject (user ID)
//   2. Gateway records: "reject all tokens for subject X issued
//      before timestamp T"
//   3. Entry is stored in-memory AND persisted to Redis (if available)
//   4. On every auth check, after signature+expiry validation,
//      the gateway checks the revocation list
//   5. If the token's iat <= revocation timestamp → rejected
//
// REDIS PERSISTENCE:
//   - Key format: safepaw:revoke:<subject>
//   - Value format: <unix_timestamp>|<reason>
//   - TTL: matches maxTTL (entries auto-expire in Redis too)
//   - On startup, the in-memory list starts empty. Redis is checked
//     on cache miss, so revocations survive gateway restarts.
//
// WHY subject-level (not per-token)?
//   Stateless HMAC tokens don't have unique IDs (by design).
//   Revoking by subject is simpler and more useful: if a user's
//   credentials are compromised, ALL their tokens should be invalid.
// =============================================================

package middleware

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RevocationEntry records when a subject's tokens were revoked.
type RevocationEntry struct {
	RevokedAt time.Time
	ExpiresAt time.Time
	Reason    string
}

// RevocationList is a thread-safe revocation list with optional
// Redis persistence. In-memory is the primary store; Redis is
// the durable backing store for crash recovery.
type RevocationList struct {
	mu       sync.RWMutex
	entries  map[string]*RevocationEntry
	cleanupT *time.Ticker
	redis    *RedisClient
	maxTTL   time.Duration
}

const redisRevocationPrefix = "safepaw:revoke:"

// NewRevocationList creates an in-memory-only revocation list.
func NewRevocationList(maxTTL time.Duration) *RevocationList {
	return NewRevocationListWithRedis(maxTTL, nil)
}

// NewRevocationListWithRedis creates a revocation list with Redis persistence.
// If redis is nil, operates in-memory only (Phase 2 behavior).
func NewRevocationListWithRedis(maxTTL time.Duration, redis *RedisClient) *RevocationList {
	rl := &RevocationList{
		entries:  make(map[string]*RevocationEntry),
		cleanupT: time.NewTicker(maxTTL / 2),
		redis:    redis,
		maxTTL:   maxTTL,
	}
	go func() {
		for range rl.cleanupT.C {
			rl.cleanup()
		}
	}()

	if redis != nil {
		log.Printf("[REVOKE] Redis-backed revocation enabled (persistence across restarts)")
	} else {
		log.Printf("[REVOKE] In-memory revocation only (lost on restart)")
	}

	return rl
}

// Revoke marks all tokens for a subject issued before now as invalid.
func (rl *RevocationList) Revoke(subject, reason string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	existing, exists := rl.entries[subject]
	if exists && existing.RevokedAt.After(now) {
		return
	}

	entry := &RevocationEntry{
		RevokedAt: now,
		ExpiresAt: now.Add(rl.maxTTL),
		Reason:    reason,
	}
	rl.entries[subject] = entry
	log.Printf("[REVOKE] Subject=%q revoked (reason=%s)", subject, reason)

	if rl.redis != nil {
		value := fmt.Sprintf("%d|%s", now.Unix(), reason)
		if err := rl.redis.Set(redisRevocationPrefix+subject, value, rl.maxTTL); err != nil {
			log.Printf("[REVOKE] Redis persist failed for %q: %v (in-memory still active)", subject, err)
		}
	}
}

// IsRevoked checks if a token's subject+issuedAt falls under a revocation.
// Checks in-memory first, then falls back to Redis on cache miss.
func (rl *RevocationList) IsRevoked(subject string, issuedAt int64) (bool, string) {
	rl.mu.RLock()
	entry, exists := rl.entries[subject]
	rl.mu.RUnlock()

	if exists {
		if issuedAt <= entry.RevokedAt.Unix() {
			return true, entry.Reason
		}
		return false, ""
	}

	if rl.redis != nil {
		if revoked, reason := rl.checkRedis(subject, issuedAt); revoked {
			return true, reason
		}
	}

	return false, ""
}

func (rl *RevocationList) checkRedis(subject string, issuedAt int64) (bool, string) {
	val, err := rl.redis.Get(redisRevocationPrefix + subject)
	if err != nil || val == "" {
		return false, ""
	}

	parts := strings.SplitN(val, "|", 2)
	if len(parts) != 2 {
		return false, ""
	}

	revokedAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false, ""
	}

	reason := parts[1]

	// Cache in memory for future checks
	rl.mu.Lock()
	rl.entries[subject] = &RevocationEntry{
		RevokedAt: time.Unix(revokedAt, 0),
		ExpiresAt: time.Now().Add(rl.maxTTL),
		Reason:    reason,
	}
	rl.mu.Unlock()

	if issuedAt <= revokedAt {
		return true, reason
	}
	return false, ""
}

// Count returns the number of active in-memory revocation entries.
func (rl *RevocationList) Count() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.entries)
}

func (rl *RevocationList) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for sub, entry := range rl.entries {
		if now.After(entry.ExpiresAt) {
			delete(rl.entries, sub)
			log.Printf("[REVOKE] Expired revocation entry for subject=%q", sub)
		}
	}
}

// Stop stops the background cleanup goroutine.
func (rl *RevocationList) Stop() {
	rl.cleanupT.Stop()
	if rl.redis != nil {
		rl.redis.Close()
	}
}
