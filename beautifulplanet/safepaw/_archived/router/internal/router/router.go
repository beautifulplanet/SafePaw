// =============================================================
// NOPEnclaw Router — Routing Logic
// =============================================================
// The Router's job: look at an inbound message, decide where
// it should go, and forward it accordingly.
//
// Two modes of operation:
//
// 1. Agent mode (AgentInboxStream configured):
//    Forward the full inbound message to nopenclaw_agent_inbox.
//    The Agent processes it and publishes the response to outbound.
//    Pipeline: Gateway → Router → Agent → Gateway
//
// 2. Echo mode (no AgentInboxStream — fallback):
//    Echo the message back directly via the outbound stream.
//    Pipeline: Gateway → Router → Gateway
//    Useful for testing without the Agent service running.
//
// Future: channel-based routing with multiple Agent inboxes.
// =============================================================

package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"nopenclaw/router/internal/consumer"
	"nopenclaw/router/internal/publisher"
)

// Router processes inbound messages and routes them appropriately.
type Router struct {
	outboundPub *publisher.Publisher // Direct outbound (echo mode fallback)
	agentPub    *publisher.Publisher // Agent inbox (nil = echo mode)
}

// New creates a Router with an outbound publisher (echo mode).
func New(outboundPub *publisher.Publisher) *Router {
	return &Router{outboundPub: outboundPub}
}

// NewWithAgent creates a Router that forwards messages to the Agent.
// The outbound publisher is kept for potential future direct-publish needs.
func NewWithAgent(outboundPub *publisher.Publisher, agentPub *publisher.Publisher) *Router {
	return &Router{outboundPub: outboundPub, agentPub: agentPub}
}

// Handle processes a single inbound message.
// This is the function passed to consumer.Run as the Handler.
//
// If agentPub is configured → forward to Agent inbox.
// Otherwise → echo mode (prefix content, publish to outbound).
//
// This function must be safe for concurrent use — multiple workers
// call it simultaneously. It holds no mutable state.
func (r *Router) Handle(ctx context.Context, msg *consumer.Message) error {
	start := time.Now()

	log.Printf("[ROUTER] ─── Processing message ───\n"+
		"  msg_id     = %s\n"+
		"  session_id = %s\n"+
		"  channel    = %s\n"+
		"  sender_id  = %s\n"+
		"  platform   = %s\n"+
		"  type       = %s\n"+
		"  content    = %.200s\n"+
		"  delivery   = #%d\n"+
		"  timestamp  = %s",
		msg.MessageID, msg.SessionID, msg.Channel,
		msg.SenderID, msg.SenderPlatform, msg.ContentType,
		msg.Content, msg.DeliveryCount,
		time.Unix(msg.Timestamp, 0).UTC().Format(time.RFC3339))

	// ---- Agent mode: forward full message to Agent inbox ----
	if r.agentPub != nil {
		return r.forwardToAgent(ctx, msg, start)
	}

	// ---- Echo mode: respond directly to outbound ----
	return r.echoToOutbound(ctx, msg, start)
}

// forwardToAgent marshals the full inbound message and publishes
// to the agent inbox stream. The Agent will process it and write
// the response to the outbound stream.
func (r *Router) forwardToAgent(ctx context.Context, msg *consumer.Message, start time.Time) error {
	// Marshal the full message. Fields tagged json:"-" (StreamID,
	// DeliveryCount) are automatically excluded.
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message for agent: %w", err)
	}

	streamID, err := r.agentPub.PublishRaw(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to forward message to agent inbox: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("[ROUTER] Forwarded msg=%s session=%s channel=%s → agent_inbox stream=%s (%v)",
		msg.MessageID, msg.SessionID, msg.Channel, streamID, elapsed.Round(time.Microsecond))

	return nil
}

// echoToOutbound is the fallback mode: prefix content and publish
// directly to the outbound stream for Gateway delivery.
func (r *Router) echoToOutbound(ctx context.Context, msg *consumer.Message, start time.Time) error {
	// Route based on channel. For now, everything echos.
	var responseContent string

	switch msg.Channel {
	default:
		responseContent = fmt.Sprintf("[echo] %s", msg.Content)
	}

	out := &publisher.OutboundMessage{
		MessageID: msg.MessageID,
		SessionID: msg.SessionID,
		Content:   responseContent,
		Timestamp: time.Now().Unix(),
	}

	streamID, err := r.outboundPub.Publish(ctx, out)
	if err != nil {
		return fmt.Errorf("failed to publish outbound message: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("[ROUTER] Processed msg=%s session=%s channel=%s → stream=%s (%v)",
		msg.MessageID, msg.SessionID, msg.Channel, streamID, elapsed.Round(time.Microsecond))

	return nil
}
