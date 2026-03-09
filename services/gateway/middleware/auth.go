// =============================================================
// SafePaw Gateway - Authentication Middleware
// =============================================================
// Token-based authentication for WebSocket connections.
//
// HOW IT WORKS:
//   1. Client connects to /ws with a token (query param or header)
//   2. Gateway validates the token cryptographically (no DB hit)
//   3. If valid → connection proceeds with authenticated identity
//   4. If invalid/expired → connection rejected with 401
//
// TOKEN FORMAT:
//   <base64url_payload>.<base64url_signature>
//
//   Payload: {"sub":"user123","iat":1708000000,"exp":1708086400,"scope":"ws"}
//   Signature: HMAC-SHA256(payload_bytes, AUTH_SECRET)
//
// WHY HMAC-SHA256 (not JWT library)?
//   - Zero external dependencies (OPSEC: smaller attack surface)
//   - We control the format exactly (no "alg:none" attacks)
//   - Gateway stays stateless (no DB queries on every connect)
//   - Tokens are issued by a separate auth service with DB access
//
// OPSEC Lesson #11: "Stateless tokens are fast but unrevocable."
// If a token leaks, it's valid until expiry. Keep TTLs short (24h)
// and implement a revocation list for emergencies (TODO: Phase 2).
// =============================================================

package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ================================================================
// Token Types
// ================================================================

// TokenClaims represents the payload inside an auth token.
// This is the "identity card" that travels with every connection.
type TokenClaims struct {
	// Sub is the subject — WHO this token belongs to (user ID or service ID)
	Sub string `json:"sub"`

	// Iat is "issued at" — WHEN the token was created (Unix timestamp)
	Iat int64 `json:"iat"`

	// Exp is "expires at" — WHEN the token dies (Unix timestamp)
	Exp int64 `json:"exp"`

	// Scope defines WHAT this token can do
	// "ws" = WebSocket connections only
	// "admin" = admin operations (future)
	Scope string `json:"scope"`

	// Metadata for optional extra claims (channel restrictions, etc.)
	Meta map[string]string `json:"meta,omitempty"`
}

// IsExpired checks if the token has passed its expiration time.
func (tc *TokenClaims) IsExpired() bool {
	return time.Now().Unix() > tc.Exp
}

// RemainingTTL returns how much time is left before expiry.
func (tc *TokenClaims) RemainingTTL() time.Duration {
	remaining := tc.Exp - time.Now().Unix()
	if remaining < 0 {
		return 0
	}
	return time.Duration(remaining) * time.Second
}

// ================================================================
// Token Authenticator
// ================================================================

// Authenticator validates and creates HMAC-SHA256 tokens.
// It holds the secret key and configuration.
type Authenticator struct {
	secret     []byte        // HMAC secret key (from AUTH_SECRET env var)
	defaultTTL time.Duration // Default token lifetime
	maxTTL     time.Duration // Maximum allowed token lifetime
	clockSkew  time.Duration // Allowed clock drift between services
}

// NewAuthenticator creates a token authenticator.
// secret MUST be at least 32 bytes (256 bits) for HMAC-SHA256 security.
func NewAuthenticator(secret []byte, defaultTTL, maxTTL time.Duration) (*Authenticator, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("AUTH_SECRET must be at least 32 bytes (got %d) — use: openssl rand -base64 48", len(secret))
	}

	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour // 24 hours default
	}
	if maxTTL <= 0 {
		maxTTL = 7 * 24 * time.Hour // 7 days max
	}

	return &Authenticator{
		secret:     secret,
		defaultTTL: defaultTTL,
		maxTTL:     maxTTL,
		clockSkew:  30 * time.Second, // Allow 30s clock drift
	}, nil
}

// ================================================================
// Token Creation
// ================================================================

// CreateToken generates a signed token for a given subject and scope.
// Returns the token string in format: <payload_b64>.<signature_b64>
func (a *Authenticator) CreateToken(subject, scope string, meta map[string]string) (string, error) {
	return a.CreateTokenWithTTL(subject, scope, meta, a.defaultTTL)
}

// CreateTokenWithTTL generates a token with a specific lifetime.
func (a *Authenticator) CreateTokenWithTTL(subject, scope string, meta map[string]string, ttl time.Duration) (string, error) {
	if subject == "" {
		return "", fmt.Errorf("token subject (sub) cannot be empty")
	}
	if scope == "" {
		scope = "ws" // Default to WebSocket scope
	}
	if ttl > a.maxTTL {
		return "", fmt.Errorf("requested TTL %v exceeds maximum %v", ttl, a.maxTTL)
	}

	now := time.Now().Unix()
	claims := TokenClaims{
		Sub:   subject,
		Iat:   now,
		Exp:   now + int64(ttl.Seconds()),
		Scope: scope,
		Meta:  meta,
	}

	// Serialize payload to JSON
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token claims: %w", err)
	}

	// Base64url encode the payload
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Sign it: HMAC-SHA256(payload_bytes, secret)
	signature := a.sign(payloadBytes)
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	// Token format: payload.signature
	return payloadB64 + "." + sigB64, nil
}

// ================================================================
// Token Validation
// ================================================================

// ValidateToken parses and validates a token string.
// Returns the claims if valid, or an error explaining why it's invalid.
//
// Validation checks (in order):
// 1. Token format (must have exactly one ".")
// 2. Base64 decoding (payload and signature)
// 3. Signature verification (HMAC-SHA256)
// 4. JSON parsing (valid claims structure)
// 5. Expiration check (with clock skew tolerance)
// 6. Required fields (sub, scope must be non-empty)
func (a *Authenticator) ValidateToken(tokenStr string) (*TokenClaims, error) {
	// Step 1: Split token into payload and signature
	payloadB64, sigB64, ok := splitToken(tokenStr)
	if !ok {
		return nil, fmt.Errorf("invalid token format: expected <payload>.<signature>")
	}

	// Step 2: Decode base64
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid token: payload is not valid base64url")
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("invalid token: signature is not valid base64url")
	}

	// Step 3: Verify signature (CRITICAL — this is the crypto check)
	// Uses constant-time comparison to prevent timing attacks
	expectedSig := a.sign(payloadBytes)
	if !hmac.Equal(sigBytes, expectedSig) {
		return nil, fmt.Errorf("invalid token: signature verification failed")
	}

	// Step 4: Parse claims from validated payload
	var claims TokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid token: malformed claims JSON")
	}

	// Step 5: Check expiration (with clock skew tolerance)
	if time.Now().Unix()-int64(a.clockSkew.Seconds()) > claims.Exp {
		return nil, fmt.Errorf("token expired at %s (expired %v ago)",
			time.Unix(claims.Exp, 0).UTC().Format(time.RFC3339),
			time.Since(time.Unix(claims.Exp, 0)).Round(time.Second))
	}

	// Step 6: Validate required fields
	if claims.Sub == "" {
		return nil, fmt.Errorf("invalid token: missing subject (sub)")
	}
	if claims.Scope == "" {
		return nil, fmt.Errorf("invalid token: missing scope")
	}

	return &claims, nil
}

// ================================================================
// HMAC Signing
// ================================================================

// sign produces the HMAC-SHA256 signature for the given data.
func (a *Authenticator) sign(data []byte) []byte {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(data)
	return mac.Sum(nil)
}

// ================================================================
// HTTP Middleware
// ================================================================

// AuthRequired is HTTP middleware that validates auth tokens.
// It extracts the token from:
//  1. Query parameter: ?token=<token>   (for WebSocket upgrades)
//  2. Authorization header: Bearer <token> (for REST-style calls)
//
// On success, the validated claims are stored in the response header
// X-Auth-Subject so downstream handlers can read the authenticated identity.
//
// On failure, returns 401 Unauthorized with a JSON error body.
//
// revocations is optional (nil = no revocation checks).
// guard is optional (nil = no brute-force tracking).
func AuthRequired(auth *Authenticator, requiredScope string, revocations *RevocationList, next http.Handler) http.Handler {
	return AuthRequiredWithGuard(auth, requiredScope, revocations, nil, next)
}

// AuthRequiredWithGuard is AuthRequired + brute-force integration.
// On auth failure, the client IP gets a strike. After enough strikes
// the BruteForceGuard bans the IP entirely (handled upstream).
// On success, strikes are cleared (legitimate user shouldn't be penalized).
func AuthRequiredWithGuard(auth *Authenticator, requiredScope string, revocations *RevocationList, guard *BruteForceGuard, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt health and metrics endpoints from auth (Docker/k8s probes, monitoring)
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		ip := extractIP(r)
		reqID := r.Header.Get("X-Request-ID")

		token := extractToken(r)
		if token == "" {
			log.Printf("[AUTH] Rejected: no token provided (remote=%s path=%s request_id=%s)",
				ip, r.URL.Path, reqID)
			if sc := GetSecurityContext(r); sc != nil {
				sc.Auth = &AuthDecision{Outcome: "reject_missing"}
			}
			if guard != nil {
				guard.RecordFailure(ip, "missing_token")
			}
			writeAuthError(w, "missing_token", "Authentication required. Provide a valid Bearer token.")
			return
		}

		claims, err := auth.ValidateToken(token)
		if err != nil {
			log.Printf("[AUTH] Rejected: %v (remote=%s request_id=%s)", err, ip, reqID)
			if sc := GetSecurityContext(r); sc != nil {
				sc.Auth = &AuthDecision{Outcome: "reject_invalid", Reason: err.Error()}
			}
			if guard != nil {
				guard.RecordFailure(ip, "invalid_token")
			}
			writeAuthError(w, "invalid_token", "Token is expired or invalid")
			return
		}

		if revocations != nil {
			if revoked, reason := revocations.IsRevoked(claims.Sub, claims.Iat); revoked {
				log.Printf("[AUTH] Rejected: token revoked for sub=%s reason=%s (remote=%s request_id=%s)",
					claims.Sub, reason, ip, reqID)
				if sc := GetSecurityContext(r); sc != nil {
					sc.Auth = &AuthDecision{Outcome: "reject_revoked", Sub: claims.Sub, Reason: reason}
				}
				if guard != nil {
					guard.RecordFailure(ip, "token_revoked")
				}
				log.Printf("[AUTH] Revocation reason: %s", reason)
				writeAuthError(w, "token_revoked", "Token has been revoked. Please obtain a new token.")
				return
			}
		}

		if requiredScope != "" && claims.Scope != requiredScope && claims.Scope != "admin" {
			log.Printf("[AUTH] Rejected: scope=%q required=%q (sub=%s remote=%s request_id=%s)",
				claims.Scope, requiredScope, claims.Sub, ip, reqID)
			if sc := GetSecurityContext(r); sc != nil {
				sc.Auth = &AuthDecision{Outcome: "reject_scope", Sub: claims.Sub, Scope: claims.Scope, Reason: requiredScope}
			}
			if guard != nil {
				guard.RecordFailure(ip, "insufficient_scope")
			}
			writeAuthError(w, "insufficient_scope", "Insufficient permissions for this endpoint")
			return
		}

		// Auth succeeded — decrement strikes instead of full reset
		// to prevent an attacker with one valid token from clearing
		// all accumulated brute-force strikes between guesses.
		if guard != nil {
			guard.Decrement(ip)
		}

		r.Header.Set("X-Auth-Subject", claims.Sub)
		r.Header.Set("X-Auth-Scope", claims.Scope)

		if sc := GetSecurityContext(r); sc != nil {
			sc.Auth = &AuthDecision{Outcome: "allow", Sub: claims.Sub, Scope: claims.Scope}
		}

		log.Printf("[AUTH] Authenticated: sub=%s scope=%s ttl=%v (remote=%s request_id=%s)",
			claims.Sub, claims.Scope, claims.RemainingTTL().Round(time.Second), ip, reqID)

		next.ServeHTTP(w, r)
	})
}

// AuthOptional is middleware that validates a token IF present,
// but allows unauthenticated requests through.
// Useful for endpoints that have different behavior for authed vs anon users.
func AuthOptional(auth *Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip client-injected identity headers to prevent spoofing
		r.Header.Del("X-Auth-Subject")
		r.Header.Del("X-Auth-Scope")

		token := extractToken(r)
		if token != "" {
			claims, err := auth.ValidateToken(token)
			if err != nil {
				// Token present but invalid — log it but allow through as anon
				log.Printf("[AUTH] Invalid token ignored (optional auth): %v (remote=%s request_id=%s)",
					err, extractIP(r), r.Header.Get("X-Request-ID"))
			} else {
				// Valid token — set identity on request headers only (not response)
				r.Header.Set("X-Auth-Subject", claims.Sub)
				r.Header.Set("X-Auth-Scope", claims.Scope)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ================================================================
// Token Extraction Helpers
// ================================================================

// extractToken gets the auth token from the request.
// For WebSocket upgrades, query param ?token= is accepted (browsers cannot set
// custom headers during the upgrade handshake).
// For regular HTTP requests, only the Authorization: Bearer header is checked.
func extractToken(r *http.Request) string {
	// 1. Check query parameter (WebSocket upgrade only)
	if isUpgrade(r) {
		if token := r.URL.Query().Get("token"); token != "" {
			return token
		}
	}

	// 2. Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}

	return ""
}

// isUpgrade reports whether r looks like a WebSocket upgrade request.
func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// splitToken splits a token string on the single "." separator.
func splitToken(token string) (payload, signature string, ok bool) {
	dotIdx := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			if dotIdx >= 0 {
				// Multiple dots — invalid
				return "", "", false
			}
			dotIdx = i
		}
	}
	if dotIdx <= 0 || dotIdx >= len(token)-1 {
		return "", "", false
	}
	return token[:dotIdx], token[dotIdx+1:], true
}

// ================================================================
// Error Response
// ================================================================

// writeAuthError sends a structured 401 JSON response.
func writeAuthError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	resp := map[string]string{
		"error":   code,
		"message": message,
	}
	json.NewEncoder(w).Encode(resp) // #nosec G104 -- best-effort HTTP error response
}
