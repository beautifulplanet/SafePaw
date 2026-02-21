// =============================================================
// NOPEnclaw Gateway — Redis Streams Bridge
// =============================================================
// This connects the Gateway to Redis Streams (our message queue).
//
// When a user sends a WebSocket message:
//   User → WebSocket → Gateway → XADD nopenclaw_inbound → Router
//
// When the Router/Agent sends a reply:
//   Agent → Router → XADD nopenclaw_outbound → Gateway → WebSocket → User
//
// Redis Streams is like a conveyor belt at a factory:
//   - XADD puts a new package on the belt
//   - XREAD takes a package off the belt
//   - Each package has an ID so nothing gets lost
// =============================================================

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// StreamClient wraps the Redis connection for stream operations.
type StreamClient struct {
	client         *goredis.Client
	inboundStream  string
	outboundStream string
}

// InboundMessage is what the Gateway publishes TO the Router.
// This is the JSON structure written to nopenclaw_inbound.
type InboundMessage struct {
	MessageID      string            `json:"message_id"`
	SessionID      string            `json:"session_id"`
	Channel        string            `json:"channel"`
	SenderID       string            `json:"sender_id"`
	SenderPlatform string            `json:"sender_platform"`
	ContentType    string            `json:"content_type"`
	Content        string            `json:"content"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Timestamp      int64             `json:"timestamp"`
}

// OutboundMessage is what the Gateway reads FROM the Router.
// This is the JSON structure read from nopenclaw_outbound.
type OutboundMessage struct {
	StreamID  string `json:"-"`              // Redis stream ID (set by reader)
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// NewStreamClient creates a new Redis Streams client.
// It validates the connection immediately — fail fast, fail loud.
func NewStreamClient(addr, password string, db int, inboundStream, outboundStream string) (*StreamClient, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		// Connection pool settings — tuned for 10k connections
		PoolSize:     50,               // Max connections to Redis
		MinIdleConns: 5,                // Keep warm connections ready
		MaxRetries:   3,                // Retry transient failures
		DialTimeout:  5 * time.Second,  // Don't hang on connect
		ReadTimeout:  3 * time.Second,  // Don't hang on read
		WriteTimeout: 3 * time.Second,  // Don't hang on write
		PoolTimeout:  4 * time.Second,  // Don't hang waiting for pool slot

		// TLS would go here in production:
		// TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	})

	// Validate connection NOW — don't discover issues later
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis connection failed: %w (check REDIS_PASSWORD and REDIS_ADDR)", err)
	}

	log.Printf("[REDIS] Connected to %s (pool=%d, streams: in=%s, out=%s)",
		addr, 50, inboundStream, outboundStream)

	return &StreamClient{
		client:         client,
		inboundStream:  inboundStream,
		outboundStream: outboundStream,
	}, nil
}

// PublishInbound writes a message to the inbound stream (user → router).
// Uses XADD with MAXLEN ~10000 to prevent unbounded stream growth.
//
// The ~ (tilde) before MAXLEN means "approximately" — Redis will trim
// efficiently without blocking on every single write.
func (sc *StreamClient) PublishInbound(ctx context.Context, msg *InboundMessage) (string, error) {
	// Serialize the message to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inbound message: %w", err)
	}

	// XADD nopenclaw_inbound MAXLEN ~10000 * data <json_payload>
	result, err := sc.client.XAdd(ctx, &goredis.XAddArgs{
		Stream: sc.inboundStream,
		MaxLen: 10000, // Cap stream at ~10k entries
		Approx: true,  // Use ~ for performance
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()

	if err != nil {
		return "", fmt.Errorf("XADD to %s failed: %w", sc.inboundStream, err)
	}

	return result, nil
}

// ReadOutbound blocks and reads new messages from the outbound stream.
// Uses XREAD with BLOCK to efficiently wait for new messages.
//
// lastID should be "$" on first call (only new messages), or a specific
// stream ID to resume from where we left off.
func (sc *StreamClient) ReadOutbound(ctx context.Context, lastID string, count int64) ([]OutboundMessage, string, error) {
	if lastID == "" {
		lastID = "$"
	}

	results, err := sc.client.XRead(ctx, &goredis.XReadArgs{
		Streams: []string{sc.outboundStream, lastID},
		Count:   count,
		Block:   5 * time.Second, // Block up to 5s waiting for messages
	}).Result()

	if err != nil {
		// Timeout is normal — no messages arrived in 5s
		if err == goredis.Nil {
			return nil, lastID, nil
		}
		return nil, lastID, fmt.Errorf("XREAD from %s failed: %w", sc.outboundStream, err)
	}

	var messages []OutboundMessage
	newLastID := lastID

	for _, stream := range results {
		for _, xmsg := range stream.Messages {
			dataStr, ok := xmsg.Values["data"].(string)
			if !ok {
				log.Printf("[REDIS] Warning: malformed message in %s, ID=%s", sc.outboundStream, xmsg.ID)
				continue
			}

			var msg OutboundMessage
			if err := json.Unmarshal([]byte(dataStr), &msg); err != nil {
				log.Printf("[REDIS] Warning: failed to unmarshal message %s: %v", xmsg.ID, err)
				continue
			}

			msg.StreamID = xmsg.ID
			messages = append(messages, msg)
			newLastID = xmsg.ID
		}
	}

	return messages, newLastID, nil
}

// Close gracefully shuts down the Redis connection.
func (sc *StreamClient) Close() error {
	log.Println("[REDIS] Closing connection...")
	return sc.client.Close()
}

// HealthCheck verifies Redis is reachable.
func (sc *StreamClient) HealthCheck(ctx context.Context) error {
	return sc.client.Ping(ctx).Err()
}
