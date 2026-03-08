// =============================================================
// SafePaw Gateway - Auth Token CLI Tool
// =============================================================
// Generates signed auth tokens for testing and bootstrapping.
//
// Usage:
//   go run tools/tokengen/main.go -sub "user123" -scope "ws"
//   go run tools/tokengen/main.go -sub "admin" -scope "admin" -ttl 1h
//
// The generated token can be used to connect to the gateway:
//   wscat -c "ws://localhost:8080/ws?token=<generated_token>"
//
// OPSEC: This tool reads AUTH_SECRET from the environment.
// NEVER hardcode the secret or commit generated tokens.
// =============================================================

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"safepaw/gateway/middleware"
)

func main() {
	// Define CLI flags
	sub := flag.String("sub", "", "Subject (user ID or service ID) — REQUIRED")
	scope := flag.String("scope", "ws", "Token scope: ws, admin")
	ttlStr := flag.String("ttl", "24h", "Token lifetime: 1h, 24h, 7d, etc.")
	flag.Parse()

	if *sub == "" {
		fmt.Fprintln(os.Stderr, "Error: -sub is required")
		fmt.Fprintln(os.Stderr, "Usage: tokengen -sub <user_id> [-scope ws] [-ttl 24h]")
		os.Exit(1)
	}

	// Read secret from environment
	secret := os.Getenv("AUTH_SECRET")
	if secret == "" {
		fmt.Fprintln(os.Stderr, "Error: AUTH_SECRET environment variable is required")
		fmt.Fprintln(os.Stderr, "Generate one with: openssl rand -base64 48")
		os.Exit(1)
	}

	// Parse TTL
	ttl, err := time.ParseDuration(*ttlStr)
	if err != nil {
		// Try parsing "7d" style durations
		ttl, err = parseDayDuration(*ttlStr)
		if err != nil {
			log.Fatalf("Invalid TTL %q: %v", *ttlStr, err)
		}
	}

	// Create authenticator
	auth, err := middleware.NewAuthenticator(
		[]byte(secret),
		ttl,
		7*24*time.Hour, // Max 7 days
	)
	if err != nil {
		log.Fatalf("Failed to create authenticator: %v", err)
	}

	// Generate token
	token, err := auth.CreateTokenWithTTL(*sub, *scope, nil, ttl)
	if err != nil {
		log.Fatalf("Failed to create token: %v", err)
	}

	// Validate it immediately (sanity check)
	claims, err := auth.ValidateToken(token)
	if err != nil {
		log.Fatalf("BUG: Generated token failed validation: %v", err)
	}

	// Output
	fmt.Fprintf(os.Stderr, "╔══════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "║  SafePaw Auth Token Generated            ║\n")
	fmt.Fprintf(os.Stderr, "╠══════════════════════════════════════════╣\n")
	fmt.Fprintf(os.Stderr, "║  Subject: %-30s ║\n", claims.Sub)
	fmt.Fprintf(os.Stderr, "║  Scope:   %-30s ║\n", claims.Scope)
	fmt.Fprintf(os.Stderr, "║  TTL:     %-30s ║\n", claims.RemainingTTL().Round(time.Second))
	fmt.Fprintf(os.Stderr, "║  Expires: %-30s ║\n", time.Unix(claims.Exp, 0).UTC().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "╚══════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\nWebSocket test:\n")
	fmt.Fprintf(os.Stderr, "  wscat -c \"ws://localhost:8080/ws?token=%s\"\n\n", token)

	// Print ONLY the token to stdout (for piping)
	fmt.Println(token)
}

// parseDayDuration handles "7d" style durations that Go doesn't support.
func parseDayDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}
	if s[len(s)-1] == 'd' {
		days := 0
		for i := 0; i < len(s)-1; i++ {
			if s[i] < '0' || s[i] > '9' {
				return 0, fmt.Errorf("invalid duration: %s", s)
			}
			days = days*10 + int(s[i]-'0')
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid duration: %s (use Go durations like 24h or day format like 7d)", s)
}
