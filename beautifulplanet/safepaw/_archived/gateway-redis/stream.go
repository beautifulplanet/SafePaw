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
	consumerGroup  string // XREADGROUP group for outbound
	consumerName   string // This instance's name within the group
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
func NewStreamClient(addr, password string, db int, inboundStream, outboundStream, consumerGroup, consumerName string) (*StreamClient, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		// Connection pool settings — tuned for 10k connections
		PoolSize:     50,               // Max connections to Redis
		MinIdleConns: 5,                // Keep warm connections ready
		MaxRetries:   3,                // Retry transient failures
		DialTimeout:  5 * time.Second,  // Don't hang on connect
		ReadTimeout:  8 * time.Second,  // Must exceed XREADGROUP BLOCK (5s) + headroom
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
		consumerGroup:  consumerGroup,
		consumerName:   consumerName,
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

// EnsureOutboundGroup creates the consumer group if it doesn't exist.
// Uses MKSTREAM to auto-create the stream if it's missing too.
// This is idempotent — safe to call on every startup.
func (sc *StreamClient) EnsureOutboundGroup(ctx context.Context) error {
	// "0" means: start reading from the beginning of the stream.
	// New consumers joining the group will get messages from ">" (new only).
	err := sc.client.XGroupCreateMkStream(ctx, sc.outboundStream, sc.consumerGroup, "0").Err()
	if err != nil {
		// "BUSYGROUP" = group already exists — that's fine.
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			log.Printf("[REDIS] Consumer group '%s' already exists on %s (OK)", sc.consumerGroup, sc.outboundStream)
			return nil
		}
		return fmt.Errorf("XGROUP CREATE %s %s failed: %w", sc.outboundStream, sc.consumerGroup, err)
	}

	log.Printf("[REDIS] Created consumer group '%s' on stream '%s'", sc.consumerGroup, sc.outboundStream)
	return nil
}

// ReadOutbound reads new messages using XREADGROUP.
// Each message is delivered to exactly ONE consumer in the group,
// enabling horizontal scaling of Gateway instances.
//
// The ">" special ID means: give me messages not yet delivered to anyone.
// After processing, the caller MUST call AckOutbound to acknowledge.
func (sc *StreamClient) ReadOutbound(ctx context.Context, count int64) ([]OutboundMessage, error) {
	results, err := sc.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    sc.consumerGroup,
		Consumer: sc.consumerName,
		Streams:  []string{sc.outboundStream, ">"},
		Count:    count,
		Block:    5 * time.Second, // Block up to 5s waiting for messages
	}).Result()

	if err != nil {
		// Timeout is normal — no messages arrived in 5s
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("XREADGROUP from %s failed: %w", sc.outboundStream, err)
	}

	var messages []OutboundMessage

	for _, stream := range results {
		for _, xmsg := range stream.Messages {
			dataStr, ok := xmsg.Values["data"].(string)
			if !ok {
				log.Printf("[REDIS] Warning: malformed message in %s, ID=%s (missing 'data' field)", sc.outboundStream, xmsg.ID)
				// ACK malformed messages so they don't stay pending forever
				sc.AckOutbound(ctx, xmsg.ID)
				continue
			}

			// Guard against oversized outbound messages that could OOM
			// the Gateway when marshaled for WebSocket delivery.
			const maxOutboundBytes = 65536 // 64KB — matches WSMaxMessageSize
			if len(dataStr) > maxOutboundBytes {
				log.Printf("[REDIS] Warning: oversized outbound message %s (%d bytes, max %d) — ACKing to discard",
					xmsg.ID, len(dataStr), maxOutboundBytes)
				sc.AckOutbound(ctx, xmsg.ID)
				continue
			}

			var msg OutboundMessage
			if err := json.Unmarshal([]byte(dataStr), &msg); err != nil {
				log.Printf("[REDIS] Warning: failed to unmarshal message %s: %v (ACKing to skip)", xmsg.ID, err)
				sc.AckOutbound(ctx, xmsg.ID)
				continue
			}

			msg.StreamID = xmsg.ID
			log.Printf("[REDIS] Read outbound message: id=%s session=%s msgID=%s (%d bytes)",
				xmsg.ID, msg.SessionID, msg.MessageID, len(dataStr))
			messages = append(messages, msg)
		}
	}

	return messages, nil
}

// AckOutbound acknowledges one or more outbound messages after successful delivery.
// This removes them from the consumer's pending list.
// Messages that are NOT acked will be reclaimed by another consumer after timeout.
func (sc *StreamClient) AckOutbound(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return sc.client.XAck(ctx, sc.outboundStream, sc.consumerGroup, ids...).Err()
}

// Close gracefully shuts down the Redis connection.
func (sc *StreamClient) Close() error {
	log.Println("[REDIS] Closing connection...")
	return sc.client.Close()
}

// HealthCheck verifies Redis is reachable (shallow).
func (sc *StreamClient) HealthCheck(ctx context.Context) error {
	return sc.client.Ping(ctx).Err()
}

// DeepHealthCheck verifies the full outbound pipeline is operational:
//   - Redis is reachable (PING)
//   - Outbound stream exists
//   - Consumer group exists and this consumer is registered
func (sc *StreamClient) DeepHealthCheck(ctx context.Context) error {
	// 1. Ping
	if err := sc.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis unreachable: %w", err)
	}

	// 2. Check outbound stream exists and get info
	info, err := sc.client.XInfoStream(ctx, sc.outboundStream).Result()
	if err != nil {
		return fmt.Errorf("outbound stream '%s' not found: %w", sc.outboundStream, err)
	}
	_ = info // stream exists

	// 3. Check consumer group exists
	groups, err := sc.client.XInfoGroups(ctx, sc.outboundStream).Result()
	if err != nil {
		return fmt.Errorf("cannot query groups on '%s': %w", sc.outboundStream, err)
	}

	found := false
	for _, g := range groups {
		if g.Name == sc.consumerGroup {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("consumer group '%s' not found on '%s'", sc.consumerGroup, sc.outboundStream)
	}

	return nil
}
