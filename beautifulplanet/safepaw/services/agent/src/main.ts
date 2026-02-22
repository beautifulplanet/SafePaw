// =============================================================
// NOPEnclaw Agent — Main Entry Point
// =============================================================
// The Agent is the intelligence layer of the message pipeline:
//
//   Gateway → Redis (inbound) → Router → Redis (agent_inbox)
//     → [AGENT] → Redis (outbound) → Gateway
//
// It:
// 1. Loads configuration from environment variables
// 2. Connects consumer and publisher to Redis
// 3. Ensures the consumer group exists on the agent inbox stream
// 4. Starts the read loop with batch processing
// 5. Starts a health check HTTP endpoint
// 6. Handles graceful shutdown (SIGINT/SIGTERM)
//
// The Agent uses XREADGROUP (not XREAD) for:
// - Exactly-once delivery across multiple Agent instances
// - Message acknowledgment after successful processing
// - Automatic retry of failed/stuck messages via PEL
// =============================================================

import * as http from "http";
import { loadConfig } from "./config";
import { Consumer } from "./consumer";
import { Publisher } from "./publisher";
import { EchoHandler } from "./handler";

async function main(): Promise<void> {
  console.log("=== NOPEnclaw Agent starting ===");

  // --------------------------------------------------------
  // Step 1: Load configuration
  // --------------------------------------------------------
  const cfg = loadConfig();
  console.log(
    `[CONFIG] Redis=${cfg.redisAddr} Group=${cfg.consumerGroup} ` +
      `Consumer=${cfg.consumerName} Workers=${cfg.workerCount} ` +
      `Batch=${cfg.batchSize}`
  );

  // --------------------------------------------------------
  // Step 2: Create and connect consumer (reads from agent inbox)
  // --------------------------------------------------------
  const consumer = new Consumer(cfg);
  await consumer.connect();

  // --------------------------------------------------------
  // Step 3: Create and connect publisher (writes to outbound)
  // --------------------------------------------------------
  const publisher = new Publisher(cfg);
  await publisher.connect();

  // --------------------------------------------------------
  // Step 4: Create handler (echo mode for now)
  // --------------------------------------------------------
  const handler = new EchoHandler(publisher);

  // --------------------------------------------------------
  // Step 5: Start health check endpoint
  // --------------------------------------------------------
  const healthServer = http.createServer(async (req, res) => {
    if (req.url !== "/health" || req.method !== "GET") {
      res.writeHead(404);
      res.end();
      return;
    }

    const status: Record<string, unknown> = {
      status: "ok",
      service: "agent",
      consumer: cfg.consumerName,
      group: cfg.consumerGroup,
      workers: cfg.workerCount,
      timestamp: new Date().toISOString(),
    };

    try {
      await consumer.healthCheck();
      await publisher.healthCheck();
      status.redis = "connected";
    } catch {
      status.status = "degraded";
      status.redis = "unreachable";
      res.writeHead(503, { "Content-Type": "application/json" });
      res.end(JSON.stringify(status));
      return;
    }

    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(status));
  });

  healthServer.listen(cfg.healthPort, () => {
    console.log(`[HEALTH] Listening on :${cfg.healthPort}/health`);
  });

  // Set timeouts to match Go services
  healthServer.setTimeout(5000);
  healthServer.keepAliveTimeout = 5000;

  // --------------------------------------------------------
  // Step 6: Set up graceful shutdown
  // --------------------------------------------------------
  const ac = new AbortController();

  // Phase 1: Signal received → stop accepting new work
  // Phase 2: Wait for in-flight processing to finish
  // Phase 3: Close connections and exit
  const shutdown = async (signal: string): Promise<void> => {
    console.log(`\n[SHUTDOWN] ${signal} received — starting graceful shutdown`);

    // Signal the consumer read loop to stop
    ac.abort();
    consumer.stop();

    // Close health server (stop accepting new health checks)
    await new Promise<void>((resolve) => {
      healthServer.close(() => {
        console.log("[SHUTDOWN] Health server closed");
        resolve();
      });
    });

    // Wait a moment for in-flight batch to finish
    // (the consumer.run() Promise.allSettled handles this)
    await new Promise((r) => setTimeout(r, 500));

    // Close Redis connections
    await consumer.close();
    await publisher.close();

    console.log("=== NOPEnclaw Agent stopped ===");
    process.exit(0);
  };

  process.on("SIGINT", () => shutdown("SIGINT"));
  process.on("SIGTERM", () => shutdown("SIGTERM"));

  // --------------------------------------------------------
  // Step 7: Start the consumer read loop (blocks until shutdown)
  // --------------------------------------------------------
  console.log("=== NOPEnclaw Agent ready — processing messages ===");

  try {
    await consumer.run((msg) => handler.handle(msg), ac.signal);
  } catch (err) {
    console.error(`[FATAL] Consumer loop crashed: ${err}`);
    process.exit(1);
  }
}

// Run
main().catch((err) => {
  console.error(`[FATAL] Startup failed: ${err}`);
  process.exit(1);
});
