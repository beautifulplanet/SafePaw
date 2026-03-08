# SPEC: Cost Monitoring & Token Usage Tracking

**Status:** DRAFT — needs sign-off before implementation  
**Author:** Copilot  
**Date:** 2026-03-07

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
4. **Minimal overhead.** Parsing response bodies adds latency — keep it under 5ms p99.

---

## Architecture Decision: How to capture LLM traffic

### Option A: Parse OpenClaw's outbound LLM responses (LLM Proxy)

Route OpenClaw's LLM API calls through the gateway. Configure `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL` env vars to point to gateway proxy endpoints.

- **Pro:** Sees exact token counts from provider responses
- **Pro:** Can enforce spend limits by blocking requests
- **Con:** Requires OpenClaw to support base URL overrides (it does via `providerBaseUrl`)
- **Con:** Adds the gateway as an outbound hop → latency on every LLM call
- **Con:** Must handle streaming SSE responses to extract usage from final chunk

### Option B: Poll OpenClaw's built-in usage API

OpenClaw already has `src/gateway/server-methods/usage.ts` with usage tracking, token counts, and cost aggregation. Query it periodically via the internal network.

- **Pro:** Zero changes to traffic flow — no added latency to LLM calls
- **Pro:** OpenClaw already normalizes usage across all providers
- **Pro:** Includes cache read/write tokens, retries, all the details
- **Con:** Depends on OpenClaw's internal API (could change across versions)
- **Con:** Can't enforce spend limits (read-only)

### Option C: Hybrid — Poll usage + gateway-level spend alert

Poll OpenClaw's usage API for data (Option B), but add a circuit breaker in the gateway that can block all traffic to OpenClaw if a spend threshold is exceeded.

- **Pro:** Best of both worlds — accurate data, enforceable limits
- **Pro:** No latency added to normal LLM calls
- **Con:** Blunt instrument — blocks ALL traffic, not just expensive models

### Recommendation: **Option B first, Option C as follow-up**

Start with polling OpenClaw's usage API. It's the simplest path, gives immediate value, and doesn't add latency. Add the gateway circuit breaker later if users need spend caps.

---

## Scope — Phase 1 (this PR)

### 1. Gateway: Usage collector (`middleware/usage.go`)

- New goroutine polls `http://openclaw:18789/api/usage` (or whatever the endpoint is) every 60 seconds
- Parses token counts: input, output, cache read, cache write, total
- Parses cost data if available
- Stores in memory (rolling 24h window, per-model breakdown)
- Exposes as Prometheus metrics:
  - `safepaw_llm_tokens_total{provider, model, type}` (counter) — type: input/output/cache_read/cache_write
  - `safepaw_llm_cost_dollars{provider, model}` (gauge) — estimated cost
  - `safepaw_llm_requests_total{provider, model, status}` (counter)

### 2. Gateway: Usage API endpoint

- `GET /admin/usage` — returns JSON summary:
  ```json
  {
    "period": "24h",
    "total_tokens": 1250000,
    "total_cost_usd": 12.50,
    "by_model": [
      {"provider": "anthropic", "model": "claude-sonnet-4-20250514", "input_tokens": 500000, "output_tokens": 200000, "cost_usd": 4.20},
      {"provider": "openai", "model": "gpt-4o", "input_tokens": 300000, "output_tokens": 250000, "cost_usd": 8.30}
    ],
    "last_updated": "2026-03-07T21:00:00Z"
  }
  ```
- Protected by auth (same as `/admin/revoke`)

### 3. Wizard: Cost dashboard component

- New section in wizard dashboard: "LLM Usage & Cost"
- Shows: total cost today, total tokens, per-model breakdown table
- Simple bar chart or table — no heavy charting library
- Polls `/admin/usage` via gateway every 60s
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
| `services/gateway/middleware/usage.go` | Create | Usage collector, poller, Prometheus metrics |
| `services/gateway/middleware/pricing.go` | Create | Per-model token pricing table |
| `services/gateway/middleware/usage_test.go` | Create | Unit tests |
| `services/gateway/middleware/pricing_test.go` | Create | Pricing lookup tests |
| `services/gateway/main.go` | Modify | Start usage collector, add `/admin/usage` endpoint |
| `services/wizard/ui/src/components/CostDashboard.tsx` | Create | React component for cost display |
| `services/wizard/ui/src/App.tsx` (or equivalent) | Modify | Wire in CostDashboard |
| `services/wizard/internal/api/handler.go` | Modify | Proxy `/admin/usage` from gateway (if needed) |
| `.env.example` | Modify | Add `COST_ALERT_DAILY_WARN` and `COST_ALERT_DAILY_CRIT` |
| `README.md` | Modify | Document cost monitoring feature |

---

## Estimated size

- ~400 lines Go (usage collector + pricing + tests)
- ~150 lines TypeScript/React (dashboard component)
- ~30 lines config/docs changes
