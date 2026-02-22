// =============================================================
// NOPEnclaw Agent — Outbound Publisher
// =============================================================
// Publishes processed messages to the outbound Redis stream.
// The Gateway reads from this stream and delivers to WebSocket
// clients.
//
// Message format MUST match what the Gateway expects:
//   { message_id, session_id, content, timestamp }
//
// Uses XADD with MAXLEN ~10000 — same as Router's publisher.
// =============================================================

import Redis from "ioredis";
import { Config } from "./config";

/**
 * OutboundMessage — what the Gateway expects on the outbound stream.
 * Must match the Go struct in router/internal/publisher/publisher.go:
 *
 *   type OutboundMessage struct {
 *       MessageID string `json:"message_id"`
 *       SessionID string `json:"session_id"`
 *       Content   string `json:"content"`
 *       Timestamp int64  `json:"timestamp"`
 *   }
 */
export interface OutboundMessage {
  message_id: string;
  session_id: string;
  content: string;
  timestamp: number;
}

/**
 * Publisher writes messages to the outbound Redis stream.
 * Uses its own Redis connection (separate from the consumer).
 */
export class Publisher {
  private client: Redis;
  private outboundStream: string;
  private maxLen: number;
  private maxOutboundSize: number;

  constructor(cfg: Config) {
    const [host, portStr] = cfg.redisAddr.split(":");
    const port = parseInt(portStr || "6379", 10);

    this.outboundStream = cfg.outboundStream;
    this.maxLen = 10000;
    this.maxOutboundSize = cfg.maxOutboundSize;

    this.client = new Redis({
      host,
      port,
      password: cfg.redisPassword,
      db: cfg.redisDB,
      maxRetriesPerRequest: 3,
      lazyConnect: true,
    });
  }

  /**
   * Connect to Redis and verify connectivity.
   */
  async connect(): Promise<void> {
    await this.client.connect();
    console.log(
      `[PUBLISHER] Connected, stream=${this.outboundStream}`
    );
  }

  /**
   * Publish writes a message to the outbound stream.
   * Returns the Redis stream ID assigned to the entry.
   *
   * Format: XADD <stream> MAXLEN ~ 10000 * data <json>
   * The Gateway reads entries and expects {"data": "<json>"}.
   */
  async publish(msg: OutboundMessage): Promise<string> {
    const data = JSON.stringify(msg);

    // Reject oversized messages before writing to Redis.
    // Without this, a runaway response could fill the stream with
    // multi-MB entries that the Gateway tries to send over WebSocket.
    if (this.maxOutboundSize > 0 && data.length > this.maxOutboundSize) {
      throw new Error(
        `Outbound message too large: ${data.length} bytes (max ${this.maxOutboundSize})`
      );
    }

    const streamId = await this.client.xadd(
      this.outboundStream,
      "MAXLEN",
      "~",
      String(this.maxLen),
      "*",
      "data",
      data
    );

    if (!streamId) {
      throw new Error(`XADD to ${this.outboundStream} returned null`);
    }

    return streamId;
  }

  /**
   * Health check — verifies Redis connectivity.
   */
  async healthCheck(): Promise<void> {
    const result = await this.client.ping();
    if (result !== "PONG") {
      throw new Error(`Redis health check failed: ${result}`);
    }
  }

  /**
   * Close the Redis connection.
   */
  async close(): Promise<void> {
    await this.client.quit();
    console.log("[PUBLISHER] Connection closed");
  }
}
