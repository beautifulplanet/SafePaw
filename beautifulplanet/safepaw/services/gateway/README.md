# NOPEnclaw Gateway (Go)

The WebSocket entry point for all client connections.

## Responsibilities
- Accept and manage 10k+ concurrent WebSocket connections
- Authenticate incoming connections
- Forward validated messages to Redis Streams (`nopenclaw_inbound`)
- Receive outbound messages from Redis and push to connected clients

## Tech Stack
- **Language:** Go 1.22+
- **WebSocket:** `gorilla/websocket`
- **Message Queue:** Redis Streams (via `go-redis/v9`)
- **UUID:** `google/uuid`

## Architecture
```
services/gateway/
├── main.go           # Entry point, wires everything, graceful shutdown
├── config/
│   └── config.go     # Env-based configuration (zero hardcoded secrets)
├── ws/
│   └── handler.go    # WebSocket upgrade, read/write pumps, connection hub
├── redis/
│   └── stream.go     # Redis Streams XADD/XREAD bridge
├── middleware/
│   └── security.go   # Security headers, origin check, rate limiter, request ID
└── Dockerfile         # Multi-stage build (build in Go, run in Alpine)
```

## Security Layers
1. **Security Headers** — HSTS, CSP, X-Frame-Options, X-Content-Type-Options
2. **Origin Validation** — Rejects cross-site WebSocket hijacking
3. **Rate Limiting** — Per-IP connection throttle (30/min default)
4. **Request ID** — UUID tracing across the pipeline
5. **Non-root container** — Runs as `nopenclaw` user in Docker
6. **Localhost binding** — Host port bound to 127.0.0.1 only

## Status
- [x] Project scaffolded (go mod init, dependencies)
- [x] WebSocket handshake working (/ws endpoint)
- [x] Connected to Redis Streams (XADD inbound, XREAD outbound)
- [x] Security middleware (headers, rate limit, origin check)
- [x] Dockerfile (multi-stage, non-root)
- [x] Health check endpoint (/health)
- [ ] Authentication middleware
- [ ] TLS termination
