# SPEC: Cost Monitoring & Token Usage Tracking

**Status:** APPROVED — implementation in progress  
**Author:** Copilot  
**Date:** 2026-03-07  
**Updated:** 2026-03-08 (architecture decision finalized)

---

## Problem

OpenClaw calls paid LLM APIs (Anthropic, OpenAI, Google, etc.) continuously.
Users have no visibility into token consumption or cost until the monthly invoice arrives.
Reports from the community show unexpected bills of $300–$800/month from unmonitored usage.

## Goal

Give SafePaw users real-time visibility into LLM token usage and estimated cost,
with optional spend alerts — without modifying OpenClaw.

## Constraints

1. **No OpenClaw modifications.** SafePaw must work with any OpenClaw version.
2. **Gateway-first.** All implementation lives in the Go gateway and wizard.
3. **Accuracy vs availability.** Estimates are acceptable; we don't need billing-grade precision.
4. **Minimal overhead.** Usage polling is async — zero added latency to proxied requests.

---

## Architecture Decision

### Options evaluated

| Option | Description | Verdict |
|--------|-------------|---------|
| **A: Go WS client** | Gateway connects to OpenClaw's WS API, authenticates, calls `usage.cost` | **CHOSEN** |
| **B: Response parsing** | Parse LLM usage fields from HTTP responses flowing through gateway | **ELIMINATED** — OpenClaw makes outbound LLM calls internally; responses never transit the gateway. OpenClaw's HTTP API hardcodes `usage: {0,0,0}`. |
| **C: Node.js sidecar** | Add WS client script inside OpenClaw container, expose via HTTP | **REJECTED** — trades code simplicity for operational complexity (two processes, separate crash domain, health-check blind spot) |

### Why Option A

1. **Data fidelity:** Gets OpenClaw's own processed usage data — same data the Control UI shows, including per-day breakdowns, cache tokens, and cost calculations.
2. **Simple protocol:** The minimal WS handshake is 3 JSON frames (challenge → connect → usage.cost). Device identity not required for `gateway-client`/`backend` mode with token auth from localhost.
3. **Operational simplicity:** One process, one binary, one language. The gateway already owns auth, caching, health checks, and Prometheus.
4. **Clean degradation:** If OpenClaw is down or protocol changes, usage endpoint returns "unavailable" — no crash, no stale data.

### Protocol details (OpenClaw WS gateway, protocol v3)

1. Open WebSocket to `ws://openclaw:18789`
2. Receive `connect.challenge` event with nonce
3. Send `connect` request: `{client: {id: "gateway-client", mode: "backend"}, auth: {token: <TOKEN>}, role: "operator", scopes: ["operator.read"]}`
4. Receive `hello-ok` response
5. Send `usage.cost` request (no params = last 30 days)
6. Receive `CostUsageSummary` response

Token: set via `OPENCLAW_GATEWAY_TOKEN` env var (deterministic) or read from OpenClaw's auto-generated config.

---

## Scope — Phase 1

### 1. Gateway: OpenClaw WS usage collector (`usage_collector.go`)

- Background goroutine connects to OpenClaw via WebSocket
- Authenticates with `gateway-client`/`backend` mode + token
- Calls `usage.cost` every 60 seconds (OpenClaw caches for 30s)
- Stores latest `CostUsageSummary` in memory (atomic pointer swap)
- Handles reconnection on disconnect (backoff: 5s, 15s, 30s, 60s)
- Graceful shutdown via context cancellation
- Exposes Prometheus metrics (hand-rolled, matching existing pattern):
  - `safepaw_llm_cost_dollars_total` (gauge) — total cost
  - `safepaw_llm_tokens_total{type}` (gauge) — input/output/cache_read/cache_write
  - `safepaw_llm_cost_daily_dollars{date}` (gauge) — per-day cost

### 2. Gateway: Pricing table (`pricing.go`)

- Hardcoded per-model token pricing for alert thresholds
- Anthropic: Claude Opus, Sonnet, Haiku
- OpenAI: GPT-4o, GPT-4o-mini, o1, o3-mini
- Not used for cost calculation (OpenClaw already calculates cost) — used only for threshold color classification

### 3. Gateway: Usage API endpoint

- `GET /admin/usage` — returns JSON:
  ```json
  {
    "updated_at": "2026-03-08T12:00:00Z",
    "days": 30,
    "totals": {
      "input_tokens": 500000, "output_tokens": 200000,
      "cache_read_tokens": 50000, "cache_write_tokens": 10000,
      "total_tokens": 760000, "total_cost_usd": 12.50
    },
    "daily": [
      {"date": "2026-03-08", "total_tokens": 25000, "total_cost_usd": 0.45},
      {"date": "2026-03-07", "total_tokens": 31000, "total_cost_usd": 0.62}
    ],
    "status": "connected"
  }
  ```
- Protected by auth (same as `/admin/revoke`) — requires "admin" scope
- Returns `{"status": "unavailable"}` if collector has no data

### 4. Wizard: Cost dashboard component

- New section in Dashboard page: "LLM Usage & Cost"
- Shows: total cost (today + 30d), token counts, daily cost bars
- Color coding: green (<$1/day), yellow ($1–$10/day), red (>$10/day)
- Polls via wizard API proxy every 60s
- Graceful "unavailable" state when gateway has no data

### 5. Configuration

- `OPENCLAW_GATEWAY_TOKEN` — set in `.env` and docker-compose.yml, also passed to OpenClaw
- `COST_ALERT_DAILY_WARN` — daily cost threshold for yellow (default: $1.00)
- `COST_ALERT_DAILY_CRIT` — daily cost threshold for red (default: $10.00)

---

## Out of scope (Phase 1)

- Spend alerts/notifications (push)
- Circuit breaker / spend cap enforcement
- Historical usage (>30d) persisted to Postgres
- Per-channel or per-user usage breakdown
- Heartbeat waste detection (Phase 2)

---

## Acceptance criteria

1. `GET /admin/usage` returns token counts and cost matching OpenClaw's data (within polling interval)
2. Prometheus metrics populated and scrapeable at `/metrics`
3. Wizard dashboard shows cost data with auto-refresh
4. Zero added latency to proxied requests (usage collection is async background goroutine)
5. Graceful degradation: if OpenClaw WS unavailable, endpoint returns `{"status": "unavailable"}`  
6. Unit tests for the usage collector, pricing table, and API endpoint
7. All existing tests still pass
- Color coding: green (<$1/day), yellow ($1–$10/day), red (>$10/day)
- Heartbeat waste callout: flag high-frequency low-token requests hitting expensive models

### 4. Pricing table (`middleware/pricing.go`)

- Hardcoded lookup table of per-token pricing for common models
- Anthropic: Claude Opus 4.6, Sonnet 4, Haiku 3.5
- OpenAI: GPT-4o, GPT-4o-mini, o1, o3-mini
- Google: Gemini 2 Pro, Flash
- Fallback: if model not in table, estimate based on average of known models in that provider
- Version-stamped, easy to update

---

## Out of scope (Phase 1)

- Spend alerts/notifications
- Circuit breaker / spend cap enforcement
- Historical usage (>24h) persisted to Postgres
- Per-channel or per-user usage breakdown
- Model routing recommendations ("switch to Haiku for this task")

---

## Acceptance criteria

1. `GET /admin/usage` returns accurate token counts matching OpenClaw's internal tracking (within 5% due to polling interval)
2. Prometheus metrics populated and scrapeable
3. Wizard dashboard shows cost data with auto-refresh
4. Zero added latency to proxied requests (usage collection is async)
5. Graceful degradation: if OpenClaw usage endpoint unavailable, dashboard shows "Usage data unavailable" — no crash
6. Unit tests for pricing table and usage parsing
7. All existing tests still pass

---

## Resolved questions (validated via community feedback)

1. **OpenClaw usage endpoint** — Confirmed. OpenClaw tracks usage internally (`src/agents/usage.ts`, `src/gateway/server-methods/usage.ts`) and has `max-budget-usd` per job. Will verify exact HTTP endpoint from running container before implementation.

2. **Threshold colors** — Revised based on real user cost data:
   - Green: <$1/day (~$30/month) — disciplined users with model routing report <$20/month
   - Yellow: $1–$10/day (~$30–$300/month) — creeping cost, model routing recommended
   - Red: >$10/day (~$300+/month) — surprise bill territory ($500+/month reports common)
   - Configurable via `COST_ALERT_DAILY_WARN=1.00` and `COST_ALERT_DAILY_CRIT=10.00` in `.env`

3. **Phase 2 priority** — **Historical usage + trend analysis** (Postgres persistence). Community consensus: visibility is the #1 ask ("almost a blackbox", "need more transparency", "token transparency will become an important UX layer"). OpenClaw already has per-job `max-budget-usd` so gateway-level spend caps are lower priority.

4. **Pricing updates** — Hardcoded with version stamp. Community already knows exact rates (Haiku ~$0.25/1M, Sonnet ~$3/1M, Opus ~$15/1M). Easy to update, no config complexity needed.

5. **Heartbeat detection** (added from community feedback) — Multiple users report the #1 cost trap is heartbeat/health checks running on expensive models ($0.70+ per Opus heartbeat). Dashboard should flag high-frequency low-complexity requests as "heartbeat waste" — most actionable single metric a user can fix.

---

## Files to create/modify

| File | Action | Purpose |
|------|--------|---------|
| `services/gateway/usage_collector.go` | Create | WS client, polling, reconnect, Prometheus metrics |
| `services/gateway/usage_collector_test.go` | Create | Unit tests for collector |
| `services/gateway/pricing.go` | Create | Per-model token pricing table |
| `services/gateway/pricing_test.go` | Create | Pricing lookup tests |
| `services/gateway/main.go` | Modify | Start collector, add `/admin/usage` endpoint |
| `services/gateway/config/config.go` | Modify | Add `OPENCLAW_GATEWAY_TOKEN`, alert thresholds |
| `services/gateway/go.mod` | Modify | Add `nhooyr.io/websocket` dependency |
| `services/wizard/ui/src/pages/Dashboard.tsx` | Modify | Add cost section |
| `services/wizard/ui/src/api.ts` | Modify | Add usage types + endpoint |
| `services/wizard/internal/api/handler.go` | Modify | Proxy `/admin/usage` from gateway |
| `docker-compose.yml` | Modify | Add `OPENCLAW_GATEWAY_TOKEN` env var |
| `.env.example` | Modify | Add cost monitoring config |

---

## Estimated size

- ~400 lines Go (usage collector + pricing + tests)
- ~150 lines TypeScript/React (dashboard component)
- ~30 lines config/docs changes
