# ADR-002: Zero External Middleware Dependencies

**Status:** Accepted  
**Date:** 2026-02-15  
**Deciders:** Project leads  

## Context

The SafePaw gateway sits in front of an AI assistant (OpenClaw) and processes
every request and WebSocket message. It implements:

- Security headers (HSTS, CSP, X-Frame-Options)
- CORS with origin whitelisting
- Per-IP rate limiting with sliding windows
- Brute-force detection with escalating bans
- HMAC-SHA256 token authentication
- Prompt-injection input scanning
- Output scanning (XSS, secret leakage, encoding evasion)
- Prometheus-compatible metrics
- Redis client for token revocation
- Structured logging
- Per-request audit trail

The standard Go approach would be to use:
- `gorilla/mux` or `chi` for routing
- `rs/cors` for CORS
- `prometheus/client_golang` for metrics (15+ transitive deps)
- `go-redis/redis` for Redis (10+ transitive deps)
- `uber-go/zap` for structured logging (5+ deps)

That's 30–50 transitive dependencies, most of which are maintained by
different individuals, each with their own release cadence, CVE exposure,
and supply chain risk.

For a **security product** whose entire purpose is to guard an AI backend, every
dependency is attack surface. The Go standard library provides everything needed:
`net/http`, `crypto/hmac`, `sync`, `encoding/json`, `time`.

## Decision

Write all middleware from scratch using only the Go standard library.

The only two external dependencies in the gateway are:
1. `github.com/coder/websocket` v1.8.14 — WebSocket support (stdlib has none)
2. `github.com/google/uuid` v1.6.0 — RFC 4122 UUIDs for request IDs

Both are single-purpose, widely audited, and maintained by organizations
(Coder and Google), not individual maintainers.

Specific hand-rolled implementations:
- **Rate limiter** — Per-IP `sync.Map` with background cleanup goroutine
- **Brute-force guard** — Strike counter with escalating ban durations
  (5m → 15m → 60m → 240m)
- **Metrics** — Prometheus text format via `atomic.Int64` counters, exposed
  at `/metrics` with zero Prometheus library code
- **Redis client** — 200-line RESP protocol implementation supporting
  SET/GET/DEL/KEYS/AUTH (avoids go-redis's 15+ transitive deps)
- **Structured logger** — `log/slog`-compatible JSON output with prefixed
  categories (`[AUTH]`, `[SCANNER]`, `[SECURITY]`, etc.)
- **CORS** — Map-based origin whitelist with preflight handling

## Consequences

**Good:**
- `go.sum` has 4 entries total (2 deps + 2 checksums). Supply chain audit takes
  minutes, not hours
- No transitive dependency CVEs to monitor (Dependabot only watches 2 packages)
- Faster builds (~2s vs ~8s with typical dependency trees)
- Smaller binary (~12 MB vs ~20 MB)
- Complete understanding of every line — no "what does this library do internally?"
- When a CVE appears in a library we don't use, we can ignore it

**Bad:**
- Higher initial development effort (~2 weeks for middleware vs ~2 days with libs)
- Must write our own tests for functionality that library tests would cover
- The Redis client supports only 5 commands — if we need Redis Streams or Pub/Sub,
  we'd either extend it or reconsider go-redis
- Prometheus text format is simple but lacks histograms and summaries
- No gorilla/mux pattern matching — using Go 1.22+ `http.ServeMux` with method
  patterns instead

**Neutral:**
- The "build vs buy" trade-off is different for security infrastructure than
  for business logic. Libraries save time but add trust assumptions. For a
  security gateway, minimizing trust assumptions is the priority.

## References

- `services/gateway/go.mod` — 2 external dependencies
- `services/gateway/middleware/` — All hand-rolled middleware
- `services/gateway/middleware/redis.go` — 200-line Redis client
- `services/gateway/middleware/metrics.go` — Prometheus text format
