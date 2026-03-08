# ADR-003: Go for the Security Gateway

**Status:** Accepted  
**Date:** 2026-02-10  
**Deciders:** Project leads  

## Context

SafePaw needs a reverse proxy that sits between clients and OpenClaw (a
Node.js AI assistant). The proxy must:

1. Authenticate every HTTP request and WebSocket upgrade
2. Scan request bodies for prompt-injection patterns
3. Scan responses for XSS and secret leakage
4. Rate-limit per IP
5. Detect brute-force attacks
6. Serve Prometheus metrics
7. Handle WebSocket proxying with message-level scanning
8. Start in <1 second and use minimal memory

Candidate languages:

| Language | Strengths | Weaknesses for this use case |
|----------|-----------|------------------------------|
| **Go** | stdlib `net/http` + `httputil.ReverseProxy`, static binary, goroutine concurrency, fast startup | Less expressive than Python/TS for regex-heavy scanning |
| **Node.js** | Same runtime as OpenClaw, npm ecosystem | Single-threaded event loop, heavier memory footprint, `node_modules` supply chain risk |
| **Python** | Rich NLP/ML ecosystem for future scanning | GIL limits concurrency, slower startup, unsuitable for high-throughput proxy |
| **Rust** | Maximum performance, memory safety | Steeper learning curve, slower iteration, overkill for this throughput level |

## Decision

Use Go (1.24+) for the gateway.

Key implementation choices enabled by Go:

- **`httputil.ReverseProxy`** — Production-grade reverse proxy in stdlib.
  Handles connection pooling, hop-by-hop header stripping, error handling.
  Director function modifies forwarded requests (strips auth headers, injects
  `X-Auth-Subject`).
- **Goroutine-per-connection** — Each WebSocket connection gets its own
  goroutine pair (read + write). No callback spaghetti, no async/await chains.
- **`sync.Map` + `atomic`** — Lock-free counters for metrics and rate limiting.
  No external concurrency library needed.
- **Static binary** — `CGO_ENABLED=0 go build` produces a single 12 MB
  executable. Docker image is `FROM scratch` + one file.
- **Go 1.22+ ServeMux** — Method-aware routing (`GET /health`, `POST /api/...`)
  without gorilla/mux or chi.

## Consequences

**Good:**
- Single static binary, ~12 MB Docker image (vs ~150 MB for Node.js)
- Cold start <500ms (vs 2-3s for Node.js with module loading)
- Memory usage ~15 MB idle (vs ~60 MB for Node.js)
- stdlib reverse proxy is battle-tested (used in Caddy, Traefik, etc.)
- Goroutine model naturally handles 10K+ concurrent WebSocket connections
- Race detector (`go test -race`) catches concurrency bugs at test time
- Cross-compilation for any OS/arch with `GOOS`/`GOARCH`

**Bad:**
- Two languages in the stack (Go gateway + Node.js OpenClaw). Developers need
  both toolchains
- Regex-heavy scanning code is more verbose in Go than Python
- No REPL for quick experimentation (minor — tests serve this role)
- If we wanted ML-based scanning in the future, Go's ML ecosystem is weaker
  than Python's (would need a sidecar or FFI)

**Neutral:**
- Go and Node.js communicate via HTTP — language boundary is clean
- The wizard (admin dashboard) is also Go, so the team already knows the language
- Go's error handling verbosity is a feature for security code — every error is
  explicitly handled

## References

- `services/gateway/main.go` — Entry point, middleware chain, reverse proxy setup
- `services/gateway/ws_proxy.go` — WebSocket proxy with goroutine-per-direction
- `services/gateway/middleware/` — All middleware (stdlib only)
