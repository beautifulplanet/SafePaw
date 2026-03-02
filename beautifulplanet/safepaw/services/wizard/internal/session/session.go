// =============================================================
// SafePaw Wizard - Session Token (HMAC-SHA256)
// =============================================================
// Signed session tokens for the wizard UI. Uses HMAC-SHA256
// so tokens can't be forged without the secret. The admin
// password itself is NEVER stored in cookies or returned to
// the client — only a signed, time-limited session token.
//
// Token format: base64url(payload).base64url(hmac_signature)
//   payload = JSON {"sub":"admin","iat":unix,"exp":unix,"jti":"nonce"}
//
// This is similar to JWT but stripped to essentials with zero
// external dependencies. The signing key is the admin password
// itself (unique per installation, auto-generated if not set).
//
// REPLAY PROTECTION:
//   Each token includes a cryptographic nonce (jti) generated from
//   crypto/rand. This ensures every token is unique even if created
//   in the same second for the same subject. Two calls to Create()
//   will always produce different tokens.
// =============================================================

package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Claims represents the token payload.
type Claims struct {
	Subject   string `json:"sub"`           // Always "admin" for wizard
	IssuedAt  int64  `json:"iat"`           // Unix timestamp
	ExpiresAt int64  `json:"exp"`           // Unix timestamp
	JTI       string `json:"jti,omitempty"` // Unique nonce (replay protection)
	Gen       uint64 `json:"gen,omitempty"` // Session generation; tokens with gen < current are invalid (e.g. after password/TOTP change)
}

var (
	ErrInvalidFormat   = errors.New("invalid token format")
	ErrInvalidEncoding = errors.New("invalid token encoding")
	ErrInvalidSign     = errors.New("invalid signature")
	ErrExpired         = errors.New("token expired")
)

// Create generates an HMAC-SHA256 signed session token.
// The secret should be the admin password (never leaves the server).
// Each token includes a unique cryptographic nonce (jti) for replay protection.
// gen is the current session generation; when password or TOTP is changed, bump gen so old tokens fail Validate.
func Create(secret string, ttl time.Duration, gen uint64) (string, error) {
	nonce, err := generateNonce()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := Claims{
		Subject:   "admin",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		JTI:       nonce,
		Gen:       gen,
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	sig := sign(payload, secret)

	// Token = base64url(payload) . base64url(signature)
	return encode(payload) + "." + encode(sig), nil
}

// ErrSessionInvalidated is returned when the token was issued before a credential rotation (password/TOTP change).
var ErrSessionInvalidated = errors.New("session invalidated by credential change")

// Validate verifies an HMAC-SHA256 signed token and returns claims.
// currentGen is the current session generation; tokens with claims.Gen < currentGen are rejected (password/TOTP was changed).
// Returns an error if the signature is invalid, the token is expired, or the session was invalidated.
func Validate(token, secret string, currentGen uint64) (*Claims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidFormat
	}

	payload, err := decode(parts[0])
	if err != nil {
		return nil, ErrInvalidEncoding
	}

	sig, err := decode(parts[1])
	if err != nil {
		return nil, ErrInvalidEncoding
	}

	// Verify HMAC (constant-time comparison via hmac.Equal)
	expected := sign(payload, secret)
	if !hmac.Equal(sig, expected) {
		return nil, ErrInvalidSign
	}

	// Parse claims
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidFormat
	}

	// Reject tokens issued before the latest credential rotation
	if currentGen > 0 && claims.Gen < currentGen {
		return nil, ErrSessionInvalidated
	}

	// Check expiry
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrExpired
	}

	return &claims, nil
}

func sign(payload []byte, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return mac.Sum(nil)
}

func encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// generateNonce produces a 16-byte cryptographic random hex string.
// Used as the jti (JWT ID) field for replay protection.
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
