// =============================================================
// NOPEnclaw Agent — Message Handler
// =============================================================
// The handler is the brain of the Agent. It receives an inbound
// message and produces an outbound response.
//
// Current implementation: echo mode.
// Mirrors the Router's echo behavior but from the Agent side.
// This proves the full 4-hop pipeline works:
//   Gateway → Router → Agent → Gateway
//
// Future implementation: LLM integration.
// The handler will call an AI model, manage conversation context,
// and produce intelligent responses. The plumbing (consumer,
// publisher, health, shutdown) stays the same.
// =============================================================

import { InboundMessage } from "./consumer";
import { OutboundMessage, Publisher } from "./publisher";

/**
 * EchoHandler wraps the publisher and produces echo responses.
 *
 * This is deliberately a class (not a bare function) to support
 * future state: conversation memory, model client, rate limiter, etc.
 */
export class EchoHandler {
  private publisher: Publisher;

  constructor(publisher: Publisher) {
    this.publisher = publisher;
  }

  /**
   * Handle processes a single inbound message.
   *
   * Current behavior (echo mode):
   * 1. Takes the inbound message
   * 2. Builds an outbound message with "[agent-echo] " prefix
   * 3. Publishes to the outbound stream for Gateway delivery
   *
   * The prefix "[agent-echo]" distinguishes Agent echoes from
   * Router echoes ("[echo]") — helpful for debugging which
   * service produced the response.
   */
  async handle(msg: InboundMessage): Promise<void> {
    const start = Date.now();

    const out: OutboundMessage = {
      message_id: msg.messageId,
      session_id: msg.sessionId,
      content: `[agent-echo] ${msg.content}`,
      timestamp: Math.floor(Date.now() / 1000),
    };

    const streamId = await this.publisher.publish(out);
    const elapsed = Date.now() - start;

    console.log(
      `[HANDLER] Processed msg=${msg.messageId} session=${msg.sessionId} ` +
        `channel=${msg.channel} → stream=${streamId} (${elapsed}ms)`
    );
  }
}
