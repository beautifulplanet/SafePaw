// =============================================================
// NOPEnclaw Router — Redis Streams Consumer
// =============================================================
// Reads messages from nopenclaw_inbound using XREADGROUP.
//
// Why XREADGROUP instead of XREAD:
// 1. Consumer groups: multiple Router instances share the load.
//    Each message goes to exactly ONE consumer (not all of them).
// 2. Acknowledgment: XACK confirms processing. If the Router
//    crashes mid-process, unacked messages can be reclaimed.
// 3. Pending list: Redis tracks which messages are in-flight.
//    We can detect stuck messages and retry them.
//
// This is the difference between "demo that breaks at scale"
// and "system that handles failures gracefully."
// =============================================================

package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"nopenclaw/router/internal/config"
)

// Message represents a message read from the inbound stream.
// Includes the stream ID needed for acknowledgment.
type Message struct {
	StreamID       string            `json:"-"`
	MessageID      string            `json:"message_id"`
	SessionID      string            `json:"session_id"`
	Channel        string            `json:"channel"`
	SenderID       string            `json:"sender_id"`
	SenderPlatform string            `json:"sender_platform"`
	ContentType    string            `json:"content_type"`
	Content        string            `json:"content"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Timestamp      int64             `json:"timestamp"`
	// Retry tracking
	DeliveryCount int64 `json:"-"`
}

// Handler is a function that processes a single message.
// Returns an error if processing fails (message will be retried).
type Handler func(ctx context.Context, msg *Message) error

// Consumer reads from a Redis Stream using consumer groups.
type Consumer struct {
	client *goredis.Client
	cfg    *config.Config
	wg     sync.WaitGroup

	// work is a buffered channel that distributes messages to workers.
	// This decouples the read loop from processing, preventing a slow
	// message from blocking reads of new messages.
	work chan *Message
}

// NewConsumer creates a Consumer and ensures the consumer group exists.
// If the group already exists, this is a no-op (idempotent).
func NewConsumer(cfg *config.Config) (*Consumer, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     cfg.WorkerCount + 5, // Workers + reader + health + headroom
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  time.Duration(cfg.BlockTime.Milliseconds()+2000) * time.Millisecond, // Block time + buffer
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	// Create consumer group if it doesn't exist.
	// "0" means: start reading from the beginning of the stream.
	// MKSTREAM creates the stream if it doesn't exist yet.
	err := client.XGroupCreateMkStream(ctx, cfg.InboundStream, cfg.ConsumerGroup, "0").Err()
	if err != nil {
		// "BUSYGROUP" means group already exists — that's fine
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			client.Close()
			return nil, fmt.Errorf("failed to create consumer group %q: %w", cfg.ConsumerGroup, err)
		}
	}

	log.Printf("[CONSUMER] Connected to %s, group=%s, consumer=%s, workers=%d",
		cfg.RedisAddr, cfg.ConsumerGroup, cfg.ConsumerName, cfg.WorkerCount)

	return &Consumer{
		client: client,
		cfg:    cfg,
		work:   make(chan *Message, cfg.WorkerCount*2), // 2× workers for buffering
	}, nil
}

// Run starts the consumer loop and worker pool. Blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, handler Handler) {
	// Start worker pool
	for i := 0; i < c.cfg.WorkerCount; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i, handler)
	}

	// Start pending message reclaimer (recovers crashed messages)
	c.wg.Add(1)
	go c.pendingReclaimer(ctx)

	// Main read loop — reads NEW messages from the stream
	c.readLoop(ctx)

	// Shutdown: close work channel, wait for workers to finish
	close(c.work)
	c.wg.Wait()
}

// readLoop continuously reads new messages from the stream.
func (c *Consumer) readLoop(ctx context.Context) {
	log.Printf("[CONSUMER] Read loop started (batch=%d, block=%v)",
		c.cfg.BatchSize, c.cfg.BlockTime)

	for {
		select {
		case <-ctx.Done():
			log.Println("[CONSUMER] Read loop stopping (context cancelled)")
			return
		default:
		}

		// XREADGROUP GROUP <group> <consumer> COUNT <n> BLOCK <ms> STREAMS <stream> >
		// The ">" means: only give me messages that haven't been delivered to any consumer yet
		results, err := c.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    c.cfg.ConsumerGroup,
			Consumer: c.cfg.ConsumerName,
			Streams:  []string{c.cfg.InboundStream, ">"},
			Count:    c.cfg.BatchSize,
			Block:    c.cfg.BlockTime,
		}).Result()

		if err != nil {
			if ctx.Err() != nil {
				return // Shutting down
			}
			if err == goredis.Nil {
				continue // Timeout, no messages — normal
			}
			log.Printf("[CONSUMER] XREADGROUP error: %v (retrying in 1s)", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range results {
			for _, xmsg := range stream.Messages {
				msg, err := parseMessage(xmsg)
				if err != nil {
					log.Printf("[CONSUMER] Malformed message %s: %v (acking to skip)", xmsg.ID, err)
					// ACK malformed messages so they don't clog the pending list
					c.ack(ctx, xmsg.ID)
					continue
				}

				// Send to worker pool. If workers are all busy and buffer is full,
				// this blocks — which naturally applies backpressure to Redis reads.
				select {
				case c.work <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// worker processes messages from the work channel.
func (c *Consumer) worker(ctx context.Context, id int, handler Handler) {
	defer c.wg.Done()
	log.Printf("[WORKER-%d] Started", id)

	for msg := range c.work {
		if ctx.Err() != nil {
			return
		}

		// Process with timeout
		processCtx, cancel := context.WithTimeout(ctx, c.cfg.AckTimeout)
		err := handler(processCtx, msg)
		cancel()

		if err != nil {
			log.Printf("[WORKER-%d] Processing failed for msg=%s: %v (delivery #%d)",
				id, msg.MessageID, err, msg.DeliveryCount)
			// Don't ACK — message stays in pending list for retry by pendingReclaimer
			continue
		}

		// Success — acknowledge the message
		c.ack(ctx, msg.StreamID)
	}

	log.Printf("[WORKER-%d] Stopped", id)
}

// pendingReclaimer periodically checks for stuck messages (delivered but not acked)
// and re-queues them for processing.
//
// This handles the case where:
// - A worker crashes mid-processing
// - A message takes too long and the worker moves on
// - A previous Router instance died with unacked messages
func (c *Consumer) pendingReclaimer(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.AckTimeout) // Check at the same interval as the timeout
	defer ticker.Stop()

	log.Printf("[RECLAIMER] Started (check interval=%v, max retries=%d)",
		c.cfg.AckTimeout, c.cfg.MaxRetries)

	for {
		select {
		case <-ctx.Done():
			log.Println("[RECLAIMER] Stopped")
			return
		case <-ticker.C:
			c.reclaimPending(ctx)
		}
	}
}

// reclaimPending checks the pending list for stuck messages.
func (c *Consumer) reclaimPending(ctx context.Context) {
	// XPENDING <stream> <group> - + <count>
	// Returns messages that were delivered but not yet acknowledged
	pending, err := c.client.XPendingExt(ctx, &goredis.XPendingExtArgs{
		Stream: c.cfg.InboundStream,
		Group:  c.cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  c.cfg.BatchSize,
		Idle:   c.cfg.AckTimeout, // Only messages idle longer than the timeout
	}).Result()

	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[RECLAIMER] XPENDING error: %v", err)
		return
	}

	if len(pending) == 0 {
		return
	}

	log.Printf("[RECLAIMER] Found %d stuck messages", len(pending))

	for _, p := range pending {
		if ctx.Err() != nil {
			return
		}

		// Check retry count — if exceeded, dead-letter the message
		if p.RetryCount >= int64(c.cfg.MaxRetries) {
			log.Printf("[RECLAIMER] Message %s exceeded max retries (%d/%d) — dead-lettering",
				p.ID, p.RetryCount, c.cfg.MaxRetries)
			// ACK to remove from pending, log for manual investigation
			// In production, this would go to a dead-letter stream
			c.ack(ctx, p.ID)
			continue
		}

		// XCLAIM: take ownership of the stuck message and re-deliver it
		claimed, err := c.client.XClaim(ctx, &goredis.XClaimArgs{
			Stream:   c.cfg.InboundStream,
			Group:    c.cfg.ConsumerGroup,
			Consumer: c.cfg.ConsumerName,
			MinIdle:  c.cfg.AckTimeout,
			Messages: []string{p.ID},
		}).Result()

		if err != nil {
			log.Printf("[RECLAIMER] XCLAIM failed for %s: %v", p.ID, err)
			continue
		}

		for _, xmsg := range claimed {
			msg, err := parseMessage(xmsg)
			if err != nil {
				log.Printf("[RECLAIMER] Malformed reclaimed message %s: %v (acking)", xmsg.ID, err)
				c.ack(ctx, xmsg.ID)
				continue
			}
			msg.DeliveryCount = p.RetryCount + 1

			select {
			case c.work <- msg:
				log.Printf("[RECLAIMER] Re-queued message %s (attempt #%d)", msg.MessageID, msg.DeliveryCount)
			case <-ctx.Done():
				return
			}
		}
	}
}

// ack acknowledges a message, removing it from the pending list.
func (c *Consumer) ack(ctx context.Context, streamID string) {
	err := c.client.XAck(ctx, c.cfg.InboundStream, c.cfg.ConsumerGroup, streamID).Err()
	if err != nil {
		log.Printf("[CONSUMER] XACK failed for %s: %v", streamID, err)
	}
}

// HealthCheck verifies Redis is reachable.
func (c *Consumer) HealthCheck(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close shuts down the Redis connection.
func (c *Consumer) Close() error {
	return c.client.Close()
}

// parseMessage converts a Redis stream message into our Message type.
func parseMessage(xmsg goredis.XMessage) (*Message, error) {
	dataStr, ok := xmsg.Values["data"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'data' field")
	}

	var msg Message
	if err := json.Unmarshal([]byte(dataStr), &msg); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w", err)
	}

	msg.StreamID = xmsg.ID

	// Validate required fields
	if msg.MessageID == "" {
		return nil, fmt.Errorf("missing message_id")
	}
	if msg.SessionID == "" {
		return nil, fmt.Errorf("missing session_id")
	}

	return &msg, nil
}
