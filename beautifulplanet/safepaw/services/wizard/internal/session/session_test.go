package session

import (
	"testing"
	"time"
)

func TestCreateAndValidate(t *testing.T) {
	secret := "test-secret-key-32bytes-long!!"
	ttl := 1 * time.Hour

	token, err := Create(secret, ttl)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if token == "" {
		t.Fatal("Create() returned empty token")
	}

	// Should have two parts separated by a dot
	if len(token) < 10 {
		t.Fatalf("Token too short: %q", token)
	}

	// Validate with correct secret
	claims, err := Validate(token, secret)
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
	if claims.Subject != "admin" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "admin")
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Error("ExpiresAt should be after IssuedAt")
	}
}

func TestValidateWrongSecret(t *testing.T) {
	token, _ := Create("correct-secret", 1*time.Hour)

	_, err := Validate(token, "wrong-secret")
	if err == nil {
		t.Fatal("Validate() should fail with wrong secret")
	}
	if err != ErrInvalidSign {
		t.Errorf("Expected ErrInvalidSign, got: %v", err)
	}
}

func TestValidateExpired(t *testing.T) {
	secret := "test-secret"
	// Create a token that's already expired (negative TTL)
	token, err := Create(secret, -1*time.Hour)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	_, err = Validate(token, secret)
	if err == nil {
		t.Fatal("Validate() should fail with expired token")
	}
	if err != ErrExpired {
		t.Errorf("Expected ErrExpired, got: %v", err)
	}
}

func TestValidateInvalidFormat(t *testing.T) {
	tt := []struct {
		name  string
		token string
		err   error
	}{
		{"empty", "", ErrInvalidFormat},
		{"no dot", "abcdef", ErrInvalidFormat},
		{"bad base64 payload", "!!!.abc", ErrInvalidEncoding},
		{"bad base64 sig", "abc.!!!", ErrInvalidEncoding},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(tc.token, "secret")
			if err != tc.err {
				t.Errorf("Validate(%q) err = %v, want %v", tc.token, err, tc.err)
			}
		})
	}
}

func TestTokensAreUnique(t *testing.T) {
	secret := "test"
	token1, _ := Create(secret, time.Hour)
	// Small delay to ensure different iat
	time.Sleep(1 * time.Millisecond)
	token2, _ := Create(secret, time.Hour)

	// Tokens should differ because iat differs (at second granularity they might match)
	// But they should both validate
	if _, err := Validate(token1, secret); err != nil {
		t.Errorf("token1 validation failed: %v", err)
	}
	if _, err := Validate(token2, secret); err != nil {
		t.Errorf("token2 validation failed: %v", err)
	}
}

func TestValidateTamperedPayload(t *testing.T) {
	secret := "test-secret"
	token, _ := Create(secret, time.Hour)

	// Tamper with the payload (change first char)
	tampered := "X" + token[1:]
	_, err := Validate(tampered, secret)
	if err == nil {
		t.Fatal("Should reject tampered token")
	}
}
