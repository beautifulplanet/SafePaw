// =============================================================
// SafePaw Gateway — Proxy Signer (PL2)
// =============================================================
// HMAC-SHA256 signing for gateway→OpenClaw requests.
//
// WHY: The X-SafePaw-User header identifies requests from the
// gateway, but any container on the Docker network can spoof it.
// A shared secret (GATEWAY_PROXY_SECRET) lets OpenClaw verify
// requests actually came from the gateway.
//
// FORMAT: X-SafePaw-Signature: t=<unix>,sig=<base64url_hmac>
// The HMAC covers "t:header_value" so replay requires both the
// secret AND a valid timestamp.
//
// Clock window: 5 minutes (generous for container clock drift).
// =============================================================

package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// ProxySignatureHeader is the header carrying the HMAC signature.
	ProxySignatureHeader = "X-SafePaw-Signature"

	// ProxyUserHeader is the header identifying the gateway.
	ProxyUserHeader = "X-SafePaw-User"

	// proxyClockWindow is the maximum age of a signature before rejection.
	proxyClockWindow = 5 * time.Minute
)

// ProxySigner signs outbound requests from the gateway to OpenClaw.
type ProxySigner struct {
	secret []byte
}

// NewProxySigner creates a signer with the given shared secret.
// Returns nil if secret is empty (signing disabled — dev mode).
func NewProxySigner(secret []byte) *ProxySigner {
	if len(secret) == 0 {
		return nil
	}
	return &ProxySigner{secret: secret}
}

// Sign produces a signature header value for the given identity.
// Format: t=<unix_timestamp>,sig=<base64url_hmac>
func (ps *ProxySigner) Sign(identity string) string {
	return ps.signAt(identity, time.Now())
}

// signAt is the testable core — accepts explicit timestamp.
func (ps *ProxySigner) signAt(identity string, now time.Time) string {
	ts := now.Unix()
	message := fmt.Sprintf("%d:%s", ts, identity)

	mac := hmac.New(sha256.New, ps.secret)
	mac.Write([]byte(message))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("t=%d,sig=%s", ts, sig)
}

// Verify checks that a signature is valid and within the clock window.
// Returns true if valid, false otherwise.
func (ps *ProxySigner) Verify(identity, signatureHeader string) bool {
	return ps.verifyAt(identity, signatureHeader, time.Now())
}

// verifyAt is the testable core — accepts explicit "now".
func (ps *ProxySigner) verifyAt(identity, signatureHeader string, now time.Time) bool {
	ts, sig, ok := parseSignatureHeader(signatureHeader)
	if !ok {
		return false
	}

	// Check clock window
	sigTime := time.Unix(ts, 0)
	drift := now.Sub(sigTime)
	if drift < 0 {
		drift = -drift
	}
	if drift > proxyClockWindow {
		return false
	}

	// Recompute HMAC
	message := fmt.Sprintf("%d:%s", ts, identity)
	mac := hmac.New(sha256.New, ps.secret)
	mac.Write([]byte(message))
	expected := mac.Sum(nil)

	// Decode provided signature
	provided, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}

	// Constant-time comparison
	return hmac.Equal(provided, expected)
}

// parseSignatureHeader extracts timestamp and signature from the header.
// Expected format: t=<unix>,sig=<base64url>
func parseSignatureHeader(header string) (int64, string, bool) {
	var tsStr, sig string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return 0, "", false
		}
		switch strings.TrimSpace(kv[0]) {
		case "t":
			tsStr = strings.TrimSpace(kv[1])
		case "sig":
			sig = strings.TrimSpace(kv[1])
		}
	}
	if tsStr == "" || sig == "" {
		return 0, "", false
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return ts, sig, true
}
