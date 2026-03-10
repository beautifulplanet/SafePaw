package middleware

import (
	"testing"
	"time"
)

// =============================================================
// PL2 — Proxy Signer Tests
// =============================================================

func TestProxySigner_SignAndVerify(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	identity := "safepaw-gateway"
	sig := signer.Sign(identity)

	if sig == "" {
		t.Fatal("Sign returned empty string")
	}
	if !signer.Verify(identity, sig) {
		t.Errorf("Verify failed for valid signature: %s", sig)
	}
}

func TestProxySigner_NilOnEmptySecret(t *testing.T) {
	signer := NewProxySigner(nil)
	if signer != nil {
		t.Error("expected nil signer for empty secret")
	}
	signer = NewProxySigner([]byte{})
	if signer != nil {
		t.Error("expected nil signer for zero-length secret")
	}
}

func TestProxySigner_RejectWrongIdentity(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	sig := signer.Sign("safepaw-gateway")

	if signer.Verify("evil-container", sig) {
		t.Error("expected verification to fail for different identity")
	}
}

func TestProxySigner_RejectWrongSecret(t *testing.T) {
	signer1 := NewProxySigner([]byte("secret-one-at-least-32-bytes-long!!!"))
	signer2 := NewProxySigner([]byte("secret-two-at-least-32-bytes-long!!!"))

	identity := "safepaw-gateway"
	sig := signer1.Sign(identity)

	if signer2.Verify(identity, sig) {
		t.Error("expected verification to fail with different secret")
	}
}

func TestProxySigner_RejectExpiredSignature(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	identity := "safepaw-gateway"
	// Sign with a timestamp 10 minutes ago
	oldTime := time.Now().Add(-10 * time.Minute)
	sig := signer.signAt(identity, oldTime)

	if signer.Verify(identity, sig) {
		t.Error("expected verification to fail for expired signature (10min old)")
	}
}

func TestProxySigner_AcceptWithinClockWindow(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	identity := "safepaw-gateway"
	// Sign with a timestamp 3 minutes ago (within 5min window)
	recentTime := time.Now().Add(-3 * time.Minute)
	sig := signer.signAt(identity, recentTime)

	if !signer.Verify(identity, sig) {
		t.Error("expected verification to pass for signature within clock window")
	}
}

func TestProxySigner_RejectFutureSignature(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	identity := "safepaw-gateway"
	// Sign with a timestamp 10 minutes in the future
	futureTime := time.Now().Add(10 * time.Minute)
	sig := signer.signAt(identity, futureTime)

	if signer.Verify(identity, sig) {
		t.Error("expected verification to fail for future signature (10min ahead)")
	}
}

func TestProxySigner_RejectMalformedHeaders(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	identity := "safepaw-gateway"
	malformed := []string{
		"",
		"garbage",
		"t=abc,sig=def",           // non-numeric timestamp
		"t=1234567890",            // missing sig
		"sig=abc123",              // missing t
		"t=1234567890,sig=",       // empty sig
		"t=,sig=abc123",           // empty t
		"x=1234567890,y=abc123",   // wrong keys
		"t=1234567890,sig=!!!!!!", // invalid base64
	}

	for _, header := range malformed {
		if signer.Verify(identity, header) {
			t.Errorf("expected verification to fail for malformed header: %q", header)
		}
	}
}

func TestProxySigner_RejectTamperedSignature(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	identity := "safepaw-gateway"
	sig := signer.Sign(identity)

	// Tamper with the signature by replacing last character
	tampered := sig[:len(sig)-1] + "X"
	if signer.Verify(identity, tampered) {
		t.Error("expected verification to fail for tampered signature")
	}
}

func TestProxySigner_SignatureFormat(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes-long!!")
	signer := NewProxySigner(secret)

	sig := signer.Sign("safepaw-gateway")

	// Should contain t= and sig=
	ts, sigVal, ok := parseSignatureHeader(sig)
	if !ok {
		t.Fatalf("parseSignatureHeader failed for: %s", sig)
	}
	if ts <= 0 {
		t.Errorf("expected positive timestamp, got %d", ts)
	}
	if sigVal == "" {
		t.Error("expected non-empty signature value")
	}
}

func TestParseSignatureHeader(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantTS  int64
		wantSig string
	}{
		{"valid", "t=1710000000,sig=abc123", true, 1710000000, "abc123"},
		{"with spaces", "t = 1710000000 , sig = abc123 ", true, 1710000000, "abc123"},
		{"empty", "", false, 0, ""},
		{"missing sig", "t=1710000000", false, 0, ""},
		{"missing t", "sig=abc123", false, 0, ""},
		{"bad timestamp", "t=notanumber,sig=abc123", false, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, sig, ok := parseSignatureHeader(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok {
				if ts != tt.wantTS {
					t.Errorf("ts = %d, want %d", ts, tt.wantTS)
				}
				if sig != tt.wantSig {
					t.Errorf("sig = %q, want %q", sig, tt.wantSig)
				}
			}
		})
	}
}
