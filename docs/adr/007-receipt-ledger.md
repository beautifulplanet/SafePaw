# ADR-007: Append-Only Receipt Ledger for Agent Action Traceability

**Status:** Accepted  
**Date:** 2026-03-08  
**Deciders:** Project leads  

## Context

The gateway audit trail (`SecurityContext`, `AuditEmitter`) answers the question:
"Did this request pass security checks?" It records auth results, rate-limit
decisions, scan verdicts, and request metadata. This is sufficient for security
operations — detecting attacks, investigating incidents, meeting compliance
requirements.

However, once a request passes all security layers and reaches OpenClaw via
WebSocket, the gateway loses visibility into what the AI agent actually _does_.
A single WebSocket session can involve dozens of tool invocations — file reads,
code execution, web searches — each with its own risk profile and duration.

After thousands of completions, operators need to answer a different question:
"What did the agent DO once it got through?" This is the traceability problem.

Without a receipt ledger:
- No record of which tools the agent invoked during a session
- No ability to correlate a specific tool call with a specific user request
- No audit trail for quality assurance or incident investigation
- No way to detect anomalous agent behavior (e.g., unexpected tool patterns)

## Decision

Add an append-only, in-memory receipt ledger to the gateway. The ledger records
immutable entries for every significant agent action that flows through the
WebSocket proxy:

**Entry types:**
- `session_start` — WebSocket tunnel opened (with auth subject)
- `tool_call` — Agent invoked a tool (tool name, truncated summary)
- `tool_result` — Tool returned a result (duration, status)
- `agent_text` — Agent produced text output
- `session_end` — WebSocket tunnel closed (total duration)
- `quality_flag` — Output scanner detected elevated risk

**Design properties:**
- **Monotonic sequence numbers** — No gaps, no reordering. Every entry gets a
  globally unique, strictly increasing sequence number via `atomic.Uint64`.
- **Immutable entries** — Append-only; no updates or deletes. Once recorded, a
  receipt is permanent within the buffer window.
- **Bounded ring buffer** — Fixed capacity (default 10,000 entries) prevents
  unbounded memory growth. When full, oldest entries are overwritten.
- **Zero external dependencies** — No database required. The ledger is purely
  in-memory with `sync.RWMutex` for concurrent access.
- **Queryable** — Filter by request_id, session_id, subject, action type,
  sequence number, or time range. Admin endpoint at `/admin/ledger`.

**Implementation:**
- `middleware/receipt.go` — Ledger struct with `Append()`, `Query()`, `Count()`
- `middleware/tool_parser.go` — Extracts `tool_use`/`tool_result` from WebSocket
  byte streams and records them in the ledger
- `ws_proxy.go` — Instruments the bidirectional WebSocket copy to emit receipts
- `main.go` — Initializes the ledger and exposes `/admin/ledger` (admin scope)

## Consequences

**Good:**
- Every agent action has a traceable, sequenced record tied to the originating
  request and authenticated user
- Operators can investigate "what happened in session X" without parsing raw logs
- The ring buffer bounds memory to a predictable maximum (~10K entries × ~500 bytes
  = ~5 MB worst case)
- No external infrastructure required — works identically in development and production
- Query API enables building dashboards, alerting, and quality review workflows

**Neutral:**
- Ring buffer means very old entries are lost. For long-term retention, operators
  should forward receipts to external storage (e.g., via `/admin/ledger` polling
  or structured log export)
- Ledger is per-instance. Multi-instance deployments would need external
  aggregation (acceptable at current scale)

## References

- `services/gateway/middleware/receipt.go` — Ledger implementation
- `services/gateway/middleware/receipt_test.go` — Tests (concurrent append, query, ring buffer)
- `services/gateway/middleware/tool_parser.go` — WebSocket tool extraction
- `services/gateway/ws_proxy.go` — WebSocket proxy instrumentation
