// =============================================================
// SafePaw Gateway — Configuration
// =============================================================
// Secure reverse proxy in front of OpenClaw.
// All config comes from environment variables.
// NOTHING is hardcoded. This is OPSEC rule #1.
// =============================================================

package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config holds all gateway configuration. Loaded once at startup.
type Config struct {
	// Server
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// Reverse proxy target (OpenClaw backend)
	ProxyTarget *url.URL

	// Security
	AllowedOrigins []string

	// Rate limiting
	RateLimit       int           // Max requests per window per IP
	RateLimitWindow time.Duration // Time window for rate limiting

	// Authentication
	AuthSecret     []byte        // HMAC-SHA256 signing secret (min 32 bytes)
	AuthEnabled    bool          // If false, all requests pass through (dev only)
	AuthDefaultTTL time.Duration // Default token lifetime
	AuthMaxTTL     time.Duration // Maximum token lifetime

	// TLS
	TLSEnabled  bool   // If true, gateway serves HTTPS
	TLSCertFile string // Path to TLS certificate file (PEM)
	TLSKeyFile  string // Path to TLS private key file (PEM)
	TLSPort     int    // HTTPS port (default: 8443)

	// Body scanning
	MaxBodySize int64 // Max request body size for scanning (default: 1MB)

	// Redis (for persistent revocation, optional)
	RedisAddr     string
	RedisPassword string
}

// Load reads config from environment variables with safe defaults.
func Load() (*Config, error) {
	// Parse proxy target (required)
	proxyTargetStr := envStr("PROXY_TARGET", "http://openclaw:18789")
	proxyTarget, err := url.Parse(proxyTargetStr)
	if err != nil {
		return nil, fmt.Errorf("invalid PROXY_TARGET %q: %w", proxyTargetStr, err)
	}
	if proxyTarget.Scheme == "" || proxyTarget.Host == "" {
		return nil, fmt.Errorf("PROXY_TARGET must include scheme and host (got %q)", proxyTargetStr)
	}

	cfg := &Config{
		// Server
		Port:         envInt("GATEWAY_PORT", 8080),
		ReadTimeout:  time.Duration(envInt("GATEWAY_READ_TIMEOUT_SEC", 30)) * time.Second,
		WriteTimeout: time.Duration(envInt("GATEWAY_WRITE_TIMEOUT_SEC", 30)) * time.Second,
		IdleTimeout:  time.Duration(envInt("GATEWAY_IDLE_TIMEOUT_SEC", 120)) * time.Second,

		// Reverse proxy target
		ProxyTarget: proxyTarget,

		// Rate limiting
		RateLimit:       envInt("RATE_LIMIT", 60),                                                            // 60 req/min per IP
		RateLimitWindow: time.Duration(envInt("RATE_LIMIT_WINDOW_SEC", 60)) * time.Second,

		// Security — empty = reject all in production (dev allows localhost)
		AllowedOrigins: []string{},

		// Auth — env-driven, disabled by default for safe local dev
		AuthEnabled:    envStr("AUTH_ENABLED", "false") == "true",
		AuthDefaultTTL: time.Duration(envInt("AUTH_DEFAULT_TTL_HOURS", 24)) * time.Hour,
		AuthMaxTTL:     time.Duration(envInt("AUTH_MAX_TTL_HOURS", 168)) * time.Hour, // 7 days

		// TLS — disabled by default (dev uses plain HTTP)
		TLSEnabled:  envStr("TLS_ENABLED", "false") == "true",
		TLSCertFile: envStr("TLS_CERT_FILE", "/certs/tls.crt"),
		TLSKeyFile:  envStr("TLS_KEY_FILE", "/certs/tls.key"),
		TLSPort:     envInt("TLS_PORT", 8443),

		// Body scanning
		MaxBodySize: int64(envInt("MAX_BODY_SIZE", 1048576)), // 1MB

		// Redis (optional — enables persistent revocation)
		RedisAddr:     envStr("REDIS_ADDR", ""),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
	}

	// Load auth secret (required if auth is enabled)
	authSecret := os.Getenv("AUTH_SECRET")
	if cfg.AuthEnabled && authSecret == "" {
		return nil, fmt.Errorf("AUTH_SECRET is required when AUTH_ENABLED=true (generate with: openssl rand -base64 48)")
	}
	if authSecret != "" {
		cfg.AuthSecret = []byte(authSecret)
	}

	// Validate TLS config if enabled
	if cfg.TLSEnabled {
		if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
			return nil, fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE are required when TLS_ENABLED=true")
		}
	}

	// Parse allowed origins if provided
	originsStr := os.Getenv("ALLOWED_ORIGINS")
	if originsStr != "" {
		origins := splitAndTrim(originsStr, ",")
		cfg.AllowedOrigins = origins
	}

	return cfg, nil
}

// --- Helper functions ---

func envStr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Avoid importing strings package for these two tiny helpers
func splitString(s, sep string) []string {
	var parts []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
	return parts
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
