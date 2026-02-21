// =============================================================
// NOPEnclaw Gateway — Configuration
// =============================================================
// All config comes from environment variables.
// NOTHING is hardcoded. This is OPSEC rule #1.
//
// Think of this like the control panel — you set the dials
// with .env, and this file reads them.
// =============================================================

package config

import (
	"fmt"
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

	// WebSocket
	WSReadBufferSize  int
	WSWriteBufferSize int
	WSMaxMessageSize  int64
	WSPongWait        time.Duration
	WSPingInterval    time.Duration
	WSWriteWait       time.Duration
	WSMaxConnections  int

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Redis Streams
	InboundStream  string // Gateway writes here (user → system)
	OutboundStream string // Gateway reads here (system → user)

	// Security
	AllowedOrigins []string

	// Authentication
	AuthSecret     []byte        // HMAC-SHA256 signing secret (min 32 bytes)
	AuthEnabled    bool          // If false, all connections are anonymous (dev only)
	AuthDefaultTTL time.Duration // Default token lifetime
	AuthMaxTTL     time.Duration // Maximum token lifetime

	// TLS
	TLSEnabled  bool   // If true, gateway serves HTTPS/WSS
	TLSCertFile string // Path to TLS certificate file (PEM)
	TLSKeyFile  string // Path to TLS private key file (PEM)
	TLSPort     int    // HTTPS port (default: 8443)
}

// Load reads config from environment variables with safe defaults.
// Returns an error if any REQUIRED variable is missing (passwords, etc.)
func Load() (*Config, error) {
	redisPassword := os.Getenv("REDIS_PASSWORD")
	if redisPassword == "" {
		return nil, fmt.Errorf("REDIS_PASSWORD environment variable is required (never hardcode it)")
	}

	cfg := &Config{
		// Server — conservative defaults for 16GB dev machine
		Port:         envInt("GATEWAY_PORT", 8080),
		ReadTimeout:  time.Duration(envInt("GATEWAY_READ_TIMEOUT_SEC", 10)) * time.Second,
		WriteTimeout: time.Duration(envInt("GATEWAY_WRITE_TIMEOUT_SEC", 10)) * time.Second,
		IdleTimeout:  time.Duration(envInt("GATEWAY_IDLE_TIMEOUT_SEC", 120)) * time.Second,

		// WebSocket — tuned for security + performance balance
		WSReadBufferSize:  envInt("WS_READ_BUFFER_SIZE", 1024),
		WSWriteBufferSize: envInt("WS_WRITE_BUFFER_SIZE", 1024),
		WSMaxMessageSize:  int64(envInt("WS_MAX_MESSAGE_SIZE", 65536)), // 64KB max per message
		WSPongWait:        time.Duration(envInt("WS_PONG_WAIT_SEC", 60)) * time.Second,
		WSPingInterval:    time.Duration(envInt("WS_PING_INTERVAL_SEC", 54)) * time.Second, // Must be < PongWait
		WSWriteWait:       time.Duration(envInt("WS_WRITE_WAIT_SEC", 10)) * time.Second,
		WSMaxConnections:  envInt("WS_MAX_CONNECTIONS", 10000),

		// Redis — connect via Docker network name (not localhost)
		RedisAddr:     envStr("REDIS_ADDR", "redis:6379"),
		RedisPassword: redisPassword,
		RedisDB:       envInt("REDIS_DB", 0),

		// Streams — nopenclaw namespace
		InboundStream:  envStr("REDIS_INBOUND_STREAM", "nopenclaw_inbound"),
		OutboundStream: envStr("REDIS_OUTBOUND_STREAM", "nopenclaw_outbound"),

		// Security — empty = reject all in production (dev allows localhost)
		AllowedOrigins: []string{}, // Set via ALLOWED_ORIGINS env var

		// Auth — env-driven, disabled by default for safe local dev
		AuthEnabled:    envStr("AUTH_ENABLED", "false") == "true",
		AuthDefaultTTL: time.Duration(envInt("AUTH_DEFAULT_TTL_HOURS", 24)) * time.Hour,
		AuthMaxTTL:     time.Duration(envInt("AUTH_MAX_TTL_HOURS", 168)) * time.Hour, // 7 days

		// TLS — disabled by default (dev uses plain HTTP)
		TLSEnabled:  envStr("TLS_ENABLED", "false") == "true",
		TLSCertFile: envStr("TLS_CERT_FILE", "/certs/tls.crt"),
		TLSKeyFile:  envStr("TLS_KEY_FILE", "/certs/tls.key"),
		TLSPort:     envInt("TLS_PORT", 8443),
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
		// Simple comma-separated parsing
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
