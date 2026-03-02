// =============================================================
// TOTP (RFC 6238) — stdlib-only, no external deps
// =============================================================
// Time-based one-time passwords for wizard MFA.
// Validate checks a 6-digit code against a base32-encoded secret.
// =============================================================

package totp

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- RFC 6238 interoperable TOTP uses HMAC-SHA1 by default.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// Validate checks a 6-digit TOTP code against the base32-encoded secret.
// Allows one step before and after (30s window) for clock skew.
// Returns true if the code matches.
func Validate(secretBase32, code string) bool {
	return ValidateAtTime(secretBase32, code, time.Now())
}

// ValidateAtTime is like Validate but uses the given time (for tests).
func ValidateAtTime(secretBase32, code string, t time.Time) bool {
	if secretBase32 == "" || code == "" {
		return false
	}
	secretBase32 = strings.ToUpper(strings.TrimSpace(secretBase32))
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretBase32)
	if err != nil {
		return false
	}
	if len(secret) < 10 {
		return false
	}
	counter := t.Unix() / 30
	for _, offset := range []int64{-1, 0, 1} {
		if match(secret, counter+offset, code) {
			return true
		}
	}
	return false
}

func match(secret []byte, counter int64, code string) bool {
	if counter < 0 {
		return false
	}

	// HOTP: HMAC-SHA1(secret, counter), then dynamic truncation
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226)
	offset := int(sum[len(sum)-1] & 0x0f)
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	expected := int(truncated % 1000000)

	var actual int
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
		actual = actual*10 + int(c-'0')
	}
	return actual == expected
}

// CodeForTime returns the 6-digit TOTP code for the given secret and time (for tests).
func CodeForTime(secretBase32 string, t time.Time) string {
	secretBase32 = strings.ToUpper(strings.TrimSpace(secretBase32))
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretBase32)
	if err != nil || len(secret) < 10 {
		return ""
	}
	counter := t.Unix() / 30
	if counter < 0 {
		return ""
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := int(sum[len(sum)-1] & 0x0f)
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	code := int(truncated % 1000000)
	return fmt.Sprintf("%06d", code)
}
