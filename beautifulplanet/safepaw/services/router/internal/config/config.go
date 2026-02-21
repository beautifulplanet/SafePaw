// =============================================================
// NOPEnclaw Router — Configuration
// =============================================================
// All config from environment variables. Nothing hardcoded.
// Mirrors the Gateway pattern for consistency across services.
// =============================================================

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all Router configuration. Loaded once at startup.
type Config struct {
	// Redis connection
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Redis Streams — names must match Gateway's config
	InboundStream  string // Router reads from here (user → system)
	OutboundStream string // Router writes here (system → user)

	// Consumer group settings
	ConsumerGroup string // Consumer group name for XREADGROUP
	ConsumerName  string // This instance's name within the group

	// Processing
	BatchSize       int64         // Max messages per XREADGROUP call
	BlockTime       time.Duration // How long to block waiting for messages
	AckTimeout      time.Duration // Max time to process before message is considered stuck
	MaxRetries      int           // Max times a message can be retried after failure
	WorkerCount     int           // Number of parallel message processing workers
	MaxMessageSize  int           // Max byte size of a single message payload (prevents OOM)
	MaxOutboundSize int           // Max byte size of outbound message (prevents oversized writes)

	// Health
	HealthPort int // HTTP port for health check endpoint
}

// Load reads config from environment variables with safe defaults.
func Load() (*Config, error) {
	redisPassword := os.Getenv("REDIS_PASSWORD")
	if redisPassword == "" {
		return nil, fmt.Errorf("REDIS_PASSWORD environment variable is required")
	}

	consumerName := os.Getenv("CONSUMER_NAME")
	if consumerName == "" {
		// Default to hostname — unique per container in Docker
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("CONSUMER_NAME not set and hostname unavailable: %w", err)
		}
		consumerName = hostname
	}

	cfg := &Config{
		// Redis — same defaults as Gateway for consistency
		RedisAddr:     envStr("REDIS_ADDR", "redis:6379"),
		RedisPassword: redisPassword,
		RedisDB:       envInt("REDIS_DB", 0),

		// Streams — must match Gateway's stream names exactly
		InboundStream:  envStr("REDIS_INBOUND_STREAM", "nopenclaw_inbound"),
		OutboundStream: envStr("REDIS_OUTBOUND_STREAM", "nopenclaw_outbound"),

		// Consumer group — enables horizontal scaling and exactly-once delivery
		ConsumerGroup: envStr("CONSUMER_GROUP", "nopenclaw_routers"),
		ConsumerName:  consumerName,

		// Processing — conservative defaults
		BatchSize:   int64(envInt("BATCH_SIZE", 10)),
		BlockTime:   time.Duration(envInt("BLOCK_TIME_MS", 5000)) * time.Millisecond,
		AckTimeout:  time.Duration(envInt("ACK_TIMEOUT_SEC", 30)) * time.Second,
		MaxRetries:  envInt("MAX_RETRIES", 3),
		WorkerCount:     envInt("WORKER_COUNT", 4),
		MaxMessageSize:  envInt("MAX_MESSAGE_SIZE", 65536),  // 64KB — matches Gateway's WSMaxMessageSize
		MaxOutboundSize: envInt("MAX_OUTBOUND_SIZE", 65536), // 64KB — prevents oversized Redis writes

		// Health
		HealthPort: envInt("HEALTH_PORT", 8081),
	}

	// Validate worker count bounds
	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 1
	}
	if cfg.WorkerCount > 64 {
		return nil, fmt.Errorf("WORKER_COUNT=%d exceeds maximum of 64", cfg.WorkerCount)
	}

	// Validate batch size bounds
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 1
	}
	if cfg.BatchSize > 1000 {
		return nil, fmt.Errorf("BATCH_SIZE=%d exceeds maximum of 1000", cfg.BatchSize)
	}

	return cfg, nil
}

// --- Helpers (same pattern as Gateway — no external deps) ---

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
