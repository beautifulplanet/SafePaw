// =============================================================
// NOPEnclaw Router — Outbound Publisher
// =============================================================
// Publishes processed messages to the outbound Redis stream.
// The Gateway reads from this stream and delivers to WebSocket clients.
//
// Uses the same XADD pattern as the Gateway's inbound publisher,
// with MAXLEN ~10000 to prevent unbounded stream growth.
// =============================================================

package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// OutboundMessage is the structure published to the outbound stream.
// Must match what the Gateway's outboundReader expects.
type OutboundMessage struct {
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Publisher writes messages to the outbound Redis stream.
type Publisher struct {
	client         *goredis.Client
	outboundStream string
	maxLen         int64
	maxOutbound    int // Max serialized message size in bytes (0 = no limit)
}

// NewPublisher creates a publisher that shares the given Redis client.
func NewPublisher(client *goredis.Client, outboundStream string) *Publisher {
	return &Publisher{
		client:         client,
		outboundStream: outboundStream,
		maxLen:         10000,
	}
}

// NewPublisherWithConnection creates a Publisher with its own Redis connection.
// Use this when the publisher needs an independent connection pool.
func NewPublisherWithConnection(addr, password string, db int, outboundStream string, maxOutbound int) (*Publisher, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("publisher redis connection failed: %w", err)
	}

	log.Printf("[PUBLISHER] Connected to %s, stream=%s", addr, outboundStream)

	return &Publisher{
		client:         client,
		outboundStream: outboundStream,
		maxLen:         10000,
		maxOutbound:    maxOutbound,
	}, nil
}

// Publish writes a message to the outbound stream.
// Returns the Redis stream ID assigned to the message.
func (p *Publisher) Publish(ctx context.Context, msg *OutboundMessage) (string, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal outbound message: %w", err)
	}

	// Reject oversized messages before writing to Redis.
	// Without this, a runaway agent response could write 50MB to the stream
	// and the Gateway would try to send it over a WebSocket.
	if p.maxOutbound > 0 && len(data) > p.maxOutbound {
		return "", fmt.Errorf("outbound message too large: %d bytes (max %d)", len(data), p.maxOutbound)
	}

	result, err := p.client.XAdd(ctx, &goredis.XAddArgs{
		Stream: p.outboundStream,
		MaxLen: p.maxLen,
		Approx: true,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()

	if err != nil {
		return "", fmt.Errorf("XADD to %s failed: %w", p.outboundStream, err)
	}

	return result, nil
}

// Close shuts down the publisher's Redis connection.
func (p *Publisher) Close() error {
	return p.client.Close()
}
