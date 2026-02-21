// =============================================================
// NOPEnclaw Router — Routing Logic
// =============================================================
// The Router's job: look at an inbound message, decide where
// it should go, and produce the outbound response.
//
// Current implementation: echo mode.
// The message is returned to the sender's session with the
// content prefixed by "[echo] ". This proves the full pipeline
// works end-to-end without needing the Agent service.
//
// Future implementation: channel-based routing.
// The Router will inspect msg.Channel ("discord", "telegram")
// and XADD to the appropriate Agent's inbox stream.
// =============================================================

package router

import (
	"context"
	"fmt"
	"log"
	"time"

	"nopenclaw/router/internal/consumer"
	"nopenclaw/router/internal/publisher"
)

// Router processes inbound messages and produces outbound responses.
type Router struct {
	pub *publisher.Publisher
}

// New creates a Router with the given publisher for outbound messages.
func New(pub *publisher.Publisher) *Router {
	return &Router{pub: pub}
}

// Handle processes a single inbound message.
// This is the function passed to consumer.Run as the Handler.
//
// Current behavior (echo mode):
//   - Takes the inbound message
//   - Produces an outbound message with "[echo] " prefix
//   - Publishes to the outbound stream for Gateway delivery
//
// This function must be safe for concurrent use — multiple workers
// call it simultaneously. It holds no mutable state.
func (r *Router) Handle(ctx context.Context, msg *consumer.Message) error {
	start := time.Now()

	// Route based on channel. For now, everything echos.
	// This switch is where future channel-specific routing goes:
	//   case "discord": publish to nopenclaw_agent_discord
	//   case "telegram": publish to nopenclaw_agent_telegram
	//   default: echo
	var responseContent string

	switch msg.Channel {
	default:
		responseContent = fmt.Sprintf("[echo] %s", msg.Content)
	}

	// Build outbound message
	out := &publisher.OutboundMessage{
		MessageID: msg.MessageID,
		SessionID: msg.SessionID,
		Content:   responseContent,
		Timestamp: time.Now().Unix(),
	}

	// Publish to outbound stream for Gateway delivery
	streamID, err := r.pub.Publish(ctx, out)
	if err != nil {
		return fmt.Errorf("failed to publish outbound message: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("[ROUTER] Processed msg=%s session=%s channel=%s → stream=%s (%v)",
		msg.MessageID, msg.SessionID, msg.Channel, streamID, elapsed.Round(time.Microsecond))

	return nil
}
