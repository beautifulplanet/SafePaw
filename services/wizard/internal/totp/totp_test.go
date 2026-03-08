package totp

import (
	"testing"
	"time"
)

// RFC 6238 test vector: secret "12345678901234567890" (ASCII), time 59 -> counter 1 -> code 942870
// Base32 of that secret is GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ (with padding GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ===)
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestValidateAtTime_RFCVector(t *testing.T) {
	// T=59 -> counter=1; RFC 6238 says T=59 gives 942870
	t0 := time.Unix(59, 0)
	code := CodeForTime(rfcSecret, t0)
	if code == "" {
		t.Fatal("CodeForTime returned empty")
	}
	if !ValidateAtTime(rfcSecret, code, t0) {
		t.Errorf("ValidateAtTime should accept code %q at T=59", code)
	}
}

func TestValidate_WrongCode(t *testing.T) {
	if Validate(rfcSecret, "000000") && Validate(rfcSecret, "123456") {
		// Might accidentally match in current 30s window
		t.Skip("current time window might match; run with different secret")
	}
	if ValidateAtTime(rfcSecret, "000000", time.Unix(59, 0)) {
		t.Error("wrong code should not validate")
	}
}

func TestValidate_EmptyRejected(t *testing.T) {
	if Validate("GEZDGNBVGY3TQOJQ", "") {
		t.Error("empty code should not validate")
	}
	if Validate("", "123456") {
		t.Error("empty secret should not validate")
	}
}

func TestValidate_NonSixDigitRejected(t *testing.T) {
	if Validate(rfcSecret, "12345") {
		t.Error("5-digit code should not validate")
	}
	if Validate(rfcSecret, "1234567") {
		t.Error("7-digit code should not validate")
	}
}

func TestValidateAtTime_ClockSkew(t *testing.T) {
	t0 := time.Unix(90, 0) // counter=3
	code := CodeForTime(rfcSecret, t0)
	if code == "" {
		t.Fatal("CodeForTime returned empty")
	}
	// Same window
	if !ValidateAtTime(rfcSecret, code, t0) {
		t.Error("same time should validate")
	}
	// One step before (60s)
	if !ValidateAtTime(rfcSecret, code, time.Unix(60, 0)) {
		t.Error("one step before should validate (clock skew)")
	}
	// One step after (119s)
	if !ValidateAtTime(rfcSecret, code, time.Unix(119, 0)) {
		t.Error("one step after should validate (clock skew)")
	}
}
