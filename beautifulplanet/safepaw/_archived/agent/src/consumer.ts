// =============================================================
// NOPEnclaw Agent — Redis Streams Consumer
// =============================================================
// Reads messages from nopenclaw_agent_inbox using XREADGROUP.
//
// Pattern mirrors the Router's Go consumer:
// 1. XREADGROUP with consumer groups for load distribution
// 2. XACK after successful processing
// 3. Batch processing with configurable concurrency
//
// If the Agent crashes, unacked messages stay in the pending
// list (PEL) and can be reclaimed by another instance.
// =============================================================

import Redis from "ioredis";
import { Config } from "./config";

/**
 * InboundMessage — what the Agent receives from the Router.
 * Matches the full inbound message format from the Gateway.
 * The Router forwards the complete message for maximum context.
 */
export interface InboundMessage {
  /** Redis stream entry ID — needed for XACK */
  streamId: string;
  /** Application-level message identifier */
  messageId: string;
  /** Session that sent the message — response routes back here */
  sessionId: string;
  /** Channel the message originated from (e.g., "discord", "telegram") */
  channel: string;
  /** Who sent the message */
  senderId: string;
  /** Platform of the sender */
  senderPlatform: string;
  /** MIME-like content type ("text/plain", "text/markdown", etc.) */
  contentType: string;
  /** The actual message content */
  content: string;
  /** Optional key-value metadata */
  metadata: Record<string, string>;
  /** Unix epoch seconds */
  timestamp: number;
}

/**
 * Handler processes a single inbound message.
 * Must throw on failure (message stays in PEL for retry).
 */
export type Handler = (msg: InboundMessage) => Promise<void>;

/**
 * Consumer reads from a Redis Stream using consumer groups.
 * Manages the read loop plus batch-level concurrency.
 */
export class Consumer {
  private client: Redis;
  private cfg: Config;
  private running = false;
  private reclaimerTimer: ReturnType<typeof setInterval> | null = null;

  constructor(cfg: Config) {
    this.cfg = cfg;

    // Parse host:port from addr string
    const [host, portStr] = cfg.redisAddr.split(":");
    const port = parseInt(portStr || "6379", 10);

    this.client = new Redis({
      host,
      port,
      password: cfg.redisPassword,
      db: cfg.redisDB,
      maxRetriesPerRequest: 3,
      lazyConnect: true,
      // Read timeout must exceed block time to prevent false disconnects.
      // XREADGROUP BLOCK blocks the connection for up to blockTimeMs.
      // If the socket timeout is shorter, ioredis kills the connection.
      commandTimeout: cfg.blockTimeMs + 5000,
    });
  }

  /**
   * Connect to Redis and ensure the consumer group exists.
   * Idempotent — safe to call even if the group already exists.
   */
  async connect(): Promise<void> {
    await this.client.connect();
    console.log(
      `[CONSUMER] Connected to ${this.cfg.redisAddr}, ` +
        `group=${this.cfg.consumerGroup}, consumer=${this.cfg.consumerName}`
    );

    // Create consumer group if it doesn't exist.
    // "0" means: start reading from the beginning of the stream.
    // MKSTREAM creates the stream if it doesn't exist yet.
    try {
      await this.client.xgroup(
        "CREATE",
        this.cfg.agentInboxStream,
        this.cfg.consumerGroup,
        "0",
        "MKSTREAM"
      );
      console.log(
        `[CONSUMER] Created group ${this.cfg.consumerGroup} on ${this.cfg.agentInboxStream}`
      );
    } catch (err: unknown) {
      // "BUSYGROUP" means group already exists — that's fine
      const message = err instanceof Error ? err.message : String(err);
      if (!message.includes("BUSYGROUP")) throw err;
      console.log(
        `[CONSUMER] Group ${this.cfg.consumerGroup} already exists (OK)`
      );
    }
  }

  /**
   * Run starts the read loop. Blocks until the AbortSignal fires.
   *
   * Processing model:
   * 1. XREADGROUP fetches up to batchSize messages
   * 2. All messages in the batch are processed concurrently
   * 3. Successful messages are XACKed individually
   * 4. Failed messages stay in the pending list for retry
   * 5. Loop repeats
   *
   * Concurrency is bounded by batchSize — at most batchSize
   * handler calls are in-flight at any time.
   */
  async run(handler: Handler, signal: AbortSignal): Promise<void> {
    this.running = true;
    console.log(
      `[CONSUMER] Read loop starting — batch=${this.cfg.batchSize}, ` +
        `block=${this.cfg.blockTimeMs}ms`
    );

    // Start pending message reclaimer (mirrors Router's pendingReclaimer).
    // Recovers messages stuck in PEL due to crashes or timeouts.
    this.startReclaimer(handler);

    while (this.running && !signal.aborted) {
      try {
        // XREADGROUP GROUP <group> <consumer> COUNT <batch> BLOCK <ms> STREAMS <stream> >
        const results = await this.client.xreadgroup(
          "GROUP",
          this.cfg.consumerGroup,
          this.cfg.consumerName,
          "COUNT",
          this.cfg.batchSize,
          "BLOCK",
          this.cfg.blockTimeMs,
          "STREAMS",
          this.cfg.agentInboxStream,
          ">"
        );

        // null means BLOCK timed out with no new messages
        if (!results) {
          console.log("[CONSUMER] XREADGROUP BLOCK timed out — no new messages (normal)");
          continue;
        }

        // ioredis xreadgroup returns: [[streamName, [[id, [field, value, ...]], ...]]]
        // TypeScript doesn't know the exact shape, so we cast it.
        const streams = results as [string, [string, string[]][]][];
        const batch: InboundMessage[] = [];

        for (const [, entries] of streams) {
          for (const [streamId, fields] of entries) {
            const msg = this.parseMessage(streamId, fields);
            if (!msg) {
              // ACK malformed messages so they don't clog the pending list forever.
              // Without this, unparseable entries stay in PEL permanently.
              await this.client.xack(
                this.cfg.agentInboxStream,
                this.cfg.consumerGroup,
                streamId
              );
              continue;
            }

            // Reject oversized messages at the consumer boundary
            if (msg.content.length > this.cfg.maxMessageSize) {
              console.error(
                `[CONSUMER] Message ${msg.messageId} too large ` +
                  `(${msg.content.length} bytes, max ${this.cfg.maxMessageSize}). ACKing to discard.`
              );
              await this.client.xack(
                this.cfg.agentInboxStream,
                this.cfg.consumerGroup,
                streamId
              );
              continue;
            }

            batch.push(msg);
          }
        }

        if (batch.length === 0) {
          console.log("[CONSUMER] Batch empty after filtering — skipping");
          continue;
        }

        console.log(`[CONSUMER] Processing batch of ${batch.length} messages`);

        // Process all messages in the batch concurrently.
        // Each handler call is independent — failure of one doesn't
        // affect others. Successful messages are XACKed individually.
        await Promise.allSettled(
          batch.map(async (msg) => {
            try {
              await handler(msg);
              // ACK on success — removes from pending list
              await this.client.xack(
                this.cfg.agentInboxStream,
                this.cfg.consumerGroup,
                msg.streamId
              );
              console.log(
                `[CONSUMER] ACKed msg=${msg.messageId} stream=${msg.streamId}`
              );
            } catch (err) {
              // Message stays in PEL — will be reclaimed on next
              // pending scan or by another consumer instance.
              console.error(
                `[CONSUMER] Handler failed for msg=${msg.messageId} ` +
                  `session=${msg.sessionId}: ${err}`
              );
            }
          })
        );
      } catch (err) {
        if (signal.aborted) break;
        console.error(`[CONSUMER] Read loop error: ${err}`);
        // Back off before retrying to avoid tight error loops
        await new Promise((r) => setTimeout(r, 1000));
      }
    }

    console.log("[CONSUMER] Read loop stopped");
  }

  /**
   * Parse a raw Redis stream entry into an InboundMessage.
   * The entry format is: ["data", "<json_string>"]
   * Returns null if the entry is malformed.
   */
  private parseMessage(
    streamId: string,
    fields: string[]
  ): InboundMessage | null {
    // Fields come as flat array: [key1, val1, key2, val2, ...]
    const dataIdx = fields.indexOf("data");
    if (dataIdx === -1 || dataIdx + 1 >= fields.length) {
      console.error(
        `[CONSUMER] Entry ${streamId} missing 'data' field — skipping`
      );
      return null;
    }

    try {
      const raw = JSON.parse(fields[dataIdx + 1]);
      return {
        streamId,
        messageId: raw.message_id || "",
        sessionId: raw.session_id || "",
        channel: raw.channel || "",
        senderId: raw.sender_id || "",
        senderPlatform: raw.sender_platform || "",
        contentType: raw.content_type || "",
        content: raw.content || "",
        metadata: raw.metadata || {},
        timestamp: raw.timestamp || 0,
      };
    } catch (err) {
      console.error(
        `[CONSUMER] Failed to parse entry ${streamId}: ${err}`
      );
      return null;
    }
  }

  // ================================================================
  // Pending Message Reclaimer — recovers stuck PEL messages
  // ================================================================
  // Mirrors the Router's Go pendingReclaimer. Without this, any
  // message that fails mid-processing (crash, timeout, OOM) stays
  // stuck in the pending entries list (PEL) forever.
  //
  // How it works:
  //   1. XPENDING scans for messages idle longer than ackTimeoutMs
  //   2. Messages under maxRetries → XCLAIM and re-process
  //   3. Messages over maxRetries → dead-letter to DLQ stream
  //   4. DLQ entries can be manually investigated / replayed
  // ================================================================

  /**
   * Start the periodic pending message reclaimer.
   * Runs on a setInterval — checks every ackTimeoutMs.
   */
  private startReclaimer(handler: Handler): void {
    const intervalMs = 30_000; // Check every 30s
    console.log(
      `[RECLAIMER] Started (interval=${intervalMs}ms, maxRetries=${this.cfg.maxRetries})`
    );

    this.reclaimerTimer = setInterval(async () => {
      if (!this.running) return;
      try {
        await this.reclaimPending(handler);
      } catch (err) {
        console.error(`[RECLAIMER] Error during reclaim cycle: ${err}`);
      }
    }, intervalMs);
  }

  /**
   * Stop the reclaimer timer.
   */
  private stopReclaimer(): void {
    if (this.reclaimerTimer) {
      clearInterval(this.reclaimerTimer);
      this.reclaimerTimer = null;
      console.log("[RECLAIMER] Stopped");
    }
  }

  /**
   * Scan the PEL for stuck messages and either re-process or dead-letter them.
   *
   * Uses XPENDING to find idle messages, then:
   * - If retryCount < maxRetries → XCLAIM + re-process
   * - If retryCount >= maxRetries → write to DLQ + XACK
   */
  private async reclaimPending(handler: Handler): Promise<void> {
    // XPENDING <stream> <group> IDLE <ms> - + <count>
    // Returns messages delivered but not yet acknowledged
    const idleMs = 30_000; // Messages idle for 30s+ are considered stuck

    // ioredis xpending with extended args
    const pending = await this.client.call(
      "XPENDING",
      this.cfg.agentInboxStream,
      this.cfg.consumerGroup,
      "IDLE",
      idleMs,
      "-",
      "+",
      this.cfg.batchSize
    ) as string[][];

    if (!pending || !Array.isArray(pending) || pending.length === 0) return;

    console.log(`[RECLAIMER] Found ${pending.length} stuck messages in PEL`);

    for (const entry of pending) {
      if (!this.running) return;

      // XPENDING extended returns: [messageId, consumer, idleTime, deliveryCount]
      const [msgId, , , deliveryCountStr] = entry;
      const deliveryCount = parseInt(String(deliveryCountStr), 10) || 0;

      // Dead-letter if retries exceeded
      if (deliveryCount >= this.cfg.maxRetries) {
        console.log(
          `[RECLAIMER] Message ${msgId} exceeded max retries ` +
            `(${deliveryCount}/${this.cfg.maxRetries}) — dead-lettering`
        );
        await this.deadLetter(msgId, deliveryCount);
        await this.client.xack(
          this.cfg.agentInboxStream,
          this.cfg.consumerGroup,
          msgId
        );
        continue;
      }

      // XCLAIM: take ownership and re-deliver
      try {
        const claimed = await this.client.xclaim(
          this.cfg.agentInboxStream,
          this.cfg.consumerGroup,
          this.cfg.consumerName,
          idleMs,
          msgId
        ) as unknown as [string, string[]][];

        if (!claimed || claimed.length === 0) continue;

        for (const [claimedId, fields] of claimed) {
          const msg = this.parseMessage(claimedId, fields);
          if (!msg) {
            console.log(
              `[RECLAIMER] Malformed reclaimed message ${claimedId} — ACKing to discard`
            );
            await this.client.xack(
              this.cfg.agentInboxStream,
              this.cfg.consumerGroup,
              claimedId
            );
            continue;
          }

          console.log(
            `[RECLAIMER] Re-processing msg=${msg.messageId} session=${msg.sessionId} ` +
              `(attempt #${deliveryCount + 1})`
          );

          try {
            await handler(msg);
            await this.client.xack(
              this.cfg.agentInboxStream,
              this.cfg.consumerGroup,
              msg.streamId
            );
            console.log(
              `[RECLAIMER] Successfully re-processed msg=${msg.messageId}`
            );
          } catch (err) {
            console.error(
              `[RECLAIMER] Re-processing failed for msg=${msg.messageId}: ${err}`
            );
            // Leave in PEL — will be picked up next cycle with incremented retry count
          }
        }
      } catch (err) {
        console.error(`[RECLAIMER] XCLAIM failed for ${msgId}: ${err}`);
      }
    }
  }

  /**
   * Write a failed message to the dead-letter queue (DLQ).
   * DLQ stream name = agent inbox stream + ":dlq" suffix.
   * Preserves original data for manual investigation / replay.
   */
  private async deadLetter(
    streamId: string,
    retryCount: number
  ): Promise<void> {
    const dlqStream = this.cfg.agentInboxStream + ":dlq";

    try {
      // Read original message data
      const msgs = await this.client.xrange(
        this.cfg.agentInboxStream,
        streamId,
        streamId
      );

      if (!msgs || msgs.length === 0) {
        console.error(
          `[DLQ] Could not read original message ${streamId} for dead-lettering`
        );
        return;
      }

      const [, fields] = msgs[0];
      const dataIdx = fields.indexOf("data");
      const originalData =
        dataIdx !== -1 && dataIdx + 1 < fields.length
          ? fields[dataIdx + 1]
          : "";

      // Write to DLQ with metadata
      await this.client.xadd(
        dlqStream,
        "MAXLEN",
        "~",
        "1000", // Cap DLQ — 1000 failed messages
        "*",
        "data",
        originalData,
        "original_id",
        streamId,
        "retry_count",
        String(retryCount),
        "consumer",
        this.cfg.consumerName,
        "dead_lettered_at",
        String(Math.floor(Date.now() / 1000))
      );

      console.log(
        `[DLQ] Message ${streamId} written to ${dlqStream} (retries=${retryCount})`
      );
    } catch (err) {
      console.error(
        `[DLQ] Failed to write message ${streamId} to DLQ: ${err}`
      );
    }
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
   * Gracefully stop the consumer.
   */
  stop(): void {
    console.log("[CONSUMER] Stop requested — finishing in-flight work");
    this.running = false;
    this.stopReclaimer();
  }

  /**
   * Close the Redis connection.
   */
  async close(): Promise<void> {
    this.running = false;
    this.stopReclaimer();
    await this.client.quit();
    console.log("[CONSUMER] Redis connection closed");
  }
}
