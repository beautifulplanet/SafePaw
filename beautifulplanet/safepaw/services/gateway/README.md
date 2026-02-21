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
│   ├── security.go   # Security headers, origin check, rate limiter, request ID
│   └── auth.go       # HMAC-SHA256 token auth (stateless, no DB per request)
├── tools/
│   └── tokengen/     # CLI tool to generate auth tokens for testing
└── Dockerfile         # Multi-stage build (build in Go, run in Alpine)
```

## Security Layers
1. **Security Headers** — HSTS, CSP, X-Frame-Options, X-Content-Type-Options
2. **Origin Validation** — Rejects cross-site WebSocket hijacking
3. **Rate Limiting** — Per-IP connection throttle (30/min default)
4. **Request ID** — UUID tracing across the pipeline
5. **Authentication** — HMAC-SHA256 stateless tokens (no DB hit per connection)
6. **TLS Termination** — TLS 1.2+ with strong cipher suites (production)
7. **Non-root container** — Runs as `nopenclaw` user in Docker
8. **Localhost binding** — Host port bound to 127.0.0.1 only

## Authentication

Tokens use HMAC-SHA256 (stateless, no DB lookups on every connection).

**Token format:** `<base64url_payload>.<base64url_signature>`

**Generate a token for dev:**
```bash
# Set your auth secret (must match AUTH_SECRET in .env)
export AUTH_SECRET=$(openssl rand -base64 48)

# Generate a token
go run tools/tokengen/main.go -sub "user123" -scope "ws" -ttl 24h
```

**Connect with token:**
```bash
wscat -c "ws://localhost:8080/ws?token=<token>"
```

**Enable auth in docker-compose:**
```env
AUTH_ENABLED=true
AUTH_SECRET=<your-secret-from-openssl>
```

## Status
- [x] Project scaffolded (go mod init, dependencies)
- [x] WebSocket handshake working (/ws endpoint)
- [x] Connected to Redis Streams (XADD inbound, XREAD outbound)
- [x] Security middleware (headers, rate limit, origin check)
- [x] Dockerfile (multi-stage, non-root)
- [x] Health check endpoint (/health)
- [x] Authentication middleware (HMAC-SHA256 tokens)
- [x] TLS termination (TLS 1.2+, strong ciphers)
- [x] Token generation CLI tool
- [x] Postgres auth schema (users, tokens, revocation)
- [ ] Token revocation sync (gateway ↔ Postgres)
- [ ] OAuth2/OIDC integration
