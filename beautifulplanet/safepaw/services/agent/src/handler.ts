// =============================================================
// NOPEnclaw Agent — Message Handler
// =============================================================
// The handler is the brain of the Agent. It receives an inbound
// message and produces an outbound response.
//
// Current implementation: echo mode with thought-process saving.
// Mirrors the Router's echo behavior but from the Agent side.
// This proves the full 4-hop pipeline works:
//   Gateway → Router → Agent → Gateway
//
// Thought-Process Saver (OPTIONAL):
//   When THOUGHT_PROCESS_ENABLED=true, the handler writes a
//   reasoning trace to a separate Redis stream for every message.
//   This captures the Agent's internal "thought process" —
//   what it received, what it decided, and why.
//
//   Use cases:
//   - Audit trail for every AI decision
//   - Debugging why a response was generated
//   - Training data collection for fine-tuning
//   - Compliance logging ("why did the bot say that?")
//
// Future implementation: LLM integration.
// The handler will call an AI model, manage conversation context,
// and produce intelligent responses. The plumbing (consumer,
// publisher, health, shutdown) stays the same.
// =============================================================

import Redis from "ioredis";
import { InboundMessage } from "./consumer";
import { OutboundMessage, Publisher } from "./publisher";
import { Config } from "./config";

/**
 * ThoughtTrace represents the Agent's internal reasoning for one message.
 * Saved to a Redis stream for audit/debug/analysis.
 */
interface ThoughtTrace {
  /** The message this thought belongs to */
  message_id: string;
  /** Who sent the original message */
  session_id: string;
  /** The channel the message came from */
  channel: string;
  /** What the Agent received */
  input_content: string;
  /** What the Agent decided to respond with */
  output_content: string;
  /** The Agent's reasoning steps (human-readable) */
  reasoning: string;
  /** Processing time in milliseconds */
  elapsed_ms: number;
  /** When this thought was recorded */
  timestamp: number;
  /** Handler mode ("echo", "llm", etc.) */
  mode: string;
}

/**
 * EchoHandler wraps the publisher and produces echo responses.
 *
 * This is deliberately a class (not a bare function) to support
 * future state: conversation memory, model client, rate limiter, etc.
 *
 * When thought-process saving is enabled, every message processing
 * event is recorded to a separate Redis stream for auditing.
 */
export class EchoHandler {
  private publisher: Publisher;
  private thoughtClient: Redis | null = null;
  private thoughtStream: string;
  private thoughtEnabled: boolean;

  constructor(publisher: Publisher, cfg: Config) {
    this.publisher = publisher;
    this.thoughtEnabled = cfg.thoughtProcessEnabled;
    this.thoughtStream = cfg.thoughtProcessStream;

    if (this.thoughtEnabled) {
      // Create a dedicated Redis connection for thought traces.
      // Separate from consumer/publisher to avoid contention.
      const [host, portStr] = cfg.redisAddr.split(":");
      const port = parseInt(portStr || "6379", 10);

      this.thoughtClient = new Redis({
        host,
        port,
        password: cfg.redisPassword,
        db: cfg.redisDB,
        maxRetriesPerRequest: 3,
        lazyConnect: true,
      });

      console.log(
        `[HANDLER] Thought-process saving ENABLED → stream=${this.thoughtStream}`
      );
    } else {
      console.log(
        "[HANDLER] Thought-process saving DISABLED (set THOUGHT_PROCESS_ENABLED=true to enable)"
      );
    }
  }

  /**
   * Connect the thought-process Redis client (if enabled).
   */
  async connect(): Promise<void> {
    if (this.thoughtClient) {
      await this.thoughtClient.connect();
      console.log(
        `[HANDLER] Thought-process Redis connected, stream=${this.thoughtStream}`
      );
    }
  }

  /**
   * Handle processes a single inbound message.
   *
   * Current behavior (echo mode):
   * 1. Takes the inbound message
   * 2. Logs the full input context
   * 3. Builds an outbound message with "[agent-echo] " prefix
   * 4. Publishes to the outbound stream for Gateway delivery
   * 5. Optionally saves a thought-process trace
   *
   * The prefix "[agent-echo]" distinguishes Agent echoes from
   * Router echoes ("[echo]") — helpful for debugging which
   * service produced the response.
   */
  async handle(msg: InboundMessage): Promise<void> {
    const start = Date.now();

    // ---- Log everything about the inbound message ----
    console.log(
      `[HANDLER] ─── Processing message ───\n` +
        `  msg_id     = ${msg.messageId}\n` +
        `  session_id = ${msg.sessionId}\n` +
        `  channel    = ${msg.channel}\n` +
        `  sender_id  = ${msg.senderId}\n` +
        `  platform   = ${msg.senderPlatform}\n` +
        `  type       = ${msg.contentType}\n` +
        `  content    = ${msg.content.substring(0, 200)}${msg.content.length > 200 ? "..." : ""}\n` +
        `  metadata   = ${JSON.stringify(msg.metadata)}\n` +
        `  timestamp  = ${new Date(msg.timestamp * 1000).toISOString()}`
    );

    // ---- Build the response ----
    const reasoning =
      `[echo-mode] Received message from session=${msg.sessionId} ` +
      `on channel=${msg.channel}. Content length=${msg.content.length} bytes. ` +
      `Echoing back with [agent-echo] prefix. ` +
      `No LLM invoked — echo mode active.`;

    const responseContent = `[agent-echo] ${msg.content}`;

    console.log(
      `[HANDLER] Reasoning: ${reasoning}`
    );

    const out: OutboundMessage = {
      message_id: msg.messageId,
      session_id: msg.sessionId,
      content: responseContent,
      timestamp: Math.floor(Date.now() / 1000),
    };

    // ---- Publish response ----
    const streamId = await this.publisher.publish(out);
    const elapsed = Date.now() - start;

    console.log(
      `[HANDLER] ✔ Published response:\n` +
        `  msg_id   = ${msg.messageId}\n` +
        `  session  = ${msg.sessionId}\n` +
        `  channel  = ${msg.channel}\n` +
        `  stream   = ${streamId}\n` +
        `  elapsed  = ${elapsed}ms\n` +
        `  response = ${responseContent.substring(0, 200)}${responseContent.length > 200 ? "..." : ""}`
    );

    // ---- Save thought-process trace (if enabled) ----
    if (this.thoughtEnabled && this.thoughtClient) {
      const trace: ThoughtTrace = {
        message_id: msg.messageId,
        session_id: msg.sessionId,
        channel: msg.channel,
        input_content: msg.content,
        output_content: responseContent,
        reasoning,
        elapsed_ms: elapsed,
        timestamp: Math.floor(Date.now() / 1000),
        mode: "echo",
      };

      try {
        const traceId = await this.thoughtClient.xadd(
          this.thoughtStream,
          "MAXLEN",
          "~",
          "50000", // Keep recent 50k traces
          "*",
          "data",
          JSON.stringify(trace)
        );
        console.log(
          `[THOUGHT] Saved trace for msg=${msg.messageId} → ${traceId}`
        );
      } catch (err) {
        // Thought-process saving is best-effort — never fail the message
        console.error(
          `[THOUGHT] Failed to save trace for msg=${msg.messageId}: ${err}`
        );
      }
    }
  }

  /**
   * Close the thought-process Redis connection.
   */
  async close(): Promise<void> {
    if (this.thoughtClient) {
      await this.thoughtClient.quit();
      console.log("[HANDLER] Thought-process Redis connection closed");
    }
  }
}
