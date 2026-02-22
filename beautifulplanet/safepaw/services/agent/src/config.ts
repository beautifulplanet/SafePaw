// =============================================================
// NOPEnclaw Agent — Configuration
// =============================================================
// All config from environment variables. Nothing hardcoded.
// Mirrors the Router/Gateway pattern for consistency.
//
// Required env vars:
//   REDIS_PASSWORD          — Auth for Redis
//
// Optional env vars (with defaults):
//   REDIS_ADDR              — Redis address         (redis:6379)
//   REDIS_DB                — Redis database index   (0)
//   AGENT_INBOX_STREAM      — Stream Agent reads     (nopenclaw_agent_inbox)
//   OUTBOUND_STREAM         — Stream Agent writes    (nopenclaw_outbound)
//   CONSUMER_GROUP          — Consumer group name    (nopenclaw_agents)
//   CONSUMER_NAME           — This instance's name   (hostname)
//   WORKER_COUNT            — Concurrent handlers    (4)
//   BATCH_SIZE              — Messages per XREADGROUP(10)
//   BLOCK_TIME_MS           — Block wait time (ms)   (5000)
//   MAX_RETRIES             — Max retry attempts      (3)
//   MAX_MESSAGE_SIZE        — Max inbound size bytes  (65536)
//   MAX_OUTBOUND_SIZE       — Max outbound size bytes (65536)
//   HEALTH_PORT             — Health endpoint port    (8082)
// =============================================================

import { hostname } from "os";

export interface Config {
  // Redis connection
  redisAddr: string;
  redisPassword: string;
  redisDB: number;

  // Redis Streams — names must match Router/Gateway config
  agentInboxStream: string; // Agent reads from here (Router → Agent)
  outboundStream: string; // Agent writes here (Agent → Gateway)

  // Consumer group settings
  consumerGroup: string; // Consumer group name for XREADGROUP
  consumerName: string; // This instance's name within the group

  // Processing
  workerCount: number; // Max concurrent handler calls per batch
  batchSize: number; // Max messages per XREADGROUP call
  blockTimeMs: number; // How long to block waiting for messages
  maxRetries: number; // Max retries before dead-lettering
  maxMessageSize: number; // Max inbound message size in bytes
  maxOutboundSize: number; // Max outbound message size in bytes

  // Thought Process
  thoughtProcessEnabled: boolean; // Save agentic reasoning traces
  thoughtProcessStream: string;   // Redis stream for thought traces

  // Health
  healthPort: number; // HTTP port for health check endpoint
}

// --- Helpers (same pattern as Router — no external deps) ---

function envStr(key: string, fallback: string): string {
  const val = process.env[key];
  return val !== undefined && val !== "" ? val : fallback;
}

function envInt(key: string, fallback: number): number {
  const val = process.env[key];
  if (val !== undefined && val !== "") {
    const n = parseInt(val, 10);
    if (!isNaN(n)) return n;
  }
  return fallback;
}

/**
 * Load reads config from environment variables with safe defaults.
 * Throws on missing required vars or invalid bounds.
 */
export function loadConfig(): Config {
  const redisPassword = process.env.REDIS_PASSWORD;
  if (!redisPassword) {
    throw new Error("REDIS_PASSWORD environment variable is required");
  }

  // Default consumer name to hostname — unique per container in Docker
  const consumerName = envStr("CONSUMER_NAME", hostname());

  const workerCount = envInt("WORKER_COUNT", 4);
  if (workerCount < 1) throw new Error("WORKER_COUNT must be >= 1");
  if (workerCount > 64)
    throw new Error(`WORKER_COUNT=${workerCount} exceeds maximum of 64`);

  const batchSize = envInt("BATCH_SIZE", 10);
  if (batchSize < 1) throw new Error("BATCH_SIZE must be >= 1");
  if (batchSize > 1000)
    throw new Error(`BATCH_SIZE=${batchSize} exceeds maximum of 1000`);

  return {
    // Redis — same defaults as Router/Gateway for consistency
    redisAddr: envStr("REDIS_ADDR", "redis:6379"),
    redisPassword,
    redisDB: envInt("REDIS_DB", 0),

    // Streams
    agentInboxStream: envStr("AGENT_INBOX_STREAM", "nopenclaw_agent_inbox"),
    outboundStream: envStr("OUTBOUND_STREAM", "nopenclaw_outbound"),

    // Consumer group — enables horizontal scaling
    consumerGroup: envStr("CONSUMER_GROUP", "nopenclaw_agents"),
    consumerName,

    // Processing — conservative defaults matching Router
    workerCount,
    batchSize,
    blockTimeMs: envInt("BLOCK_TIME_MS", 5000),
    maxRetries: envInt("MAX_RETRIES", 3),
    maxMessageSize: envInt("MAX_MESSAGE_SIZE", 65536), // 64KB
    maxOutboundSize: envInt("MAX_OUTBOUND_SIZE", 65536), // 64KB

    // Thought Process — optional agentic reasoning traces
    // When enabled, the handler saves its internal reasoning to a
    // separate Redis stream for later analysis / auditing.
    thoughtProcessEnabled:
      envStr("THOUGHT_PROCESS_ENABLED", "false") === "true",
    thoughtProcessStream: envStr(
      "THOUGHT_PROCESS_STREAM",
      "nopenclaw_thought_traces"
    ),

    // Health
    healthPort: envInt("HEALTH_PORT", 8082),
  };
}
