// =============================================================
// SafePaw Wizard - Session Token (HMAC-SHA256)
// =============================================================
// Signed session tokens for the wizard UI. Uses HMAC-SHA256
// so tokens can't be forged without the secret. The admin
// password itself is NEVER stored in cookies or returned to
// the client — only a signed, time-limited session token.
//
// Token format: base64url(payload).base64url(hmac_signature)
//   payload = JSON {"sub":"admin","iat":unix,"exp":unix}
//
// This is similar to JWT but stripped to essentials with zero
// external dependencies. The signing key is the admin password
// itself (unique per installation, auto-generated if not set).
// =============================================================

package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Claims represents the token payload.
type Claims struct {
	Subject   string `json:"sub"` // Always "admin" for wizard
	IssuedAt  int64  `json:"iat"` // Unix timestamp
	ExpiresAt int64  `json:"exp"` // Unix timestamp
}

var (
	ErrInvalidFormat   = errors.New("invalid token format")
	ErrInvalidEncoding = errors.New("invalid token encoding")
	ErrInvalidSign     = errors.New("invalid signature")
	ErrExpired         = errors.New("token expired")
)

// Create generates an HMAC-SHA256 signed session token.
// The secret should be the admin password (never leaves the server).
func Create(secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		Subject:   "admin",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	sig := sign(payload, secret)

	// Token = base64url(payload) . base64url(signature)
	return encode(payload) + "." + encode(sig), nil
}

// Validate verifies an HMAC-SHA256 signed token and returns claims.
// Returns an error if the signature is invalid or the token is expired.
func Validate(token, secret string) (*Claims, error) {
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
