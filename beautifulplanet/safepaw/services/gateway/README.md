# SafePaw Gateway

Security-hardened reverse proxy that sits in front of OpenClaw.

## What It Does
- Proxies all HTTP/WebSocket traffic to the OpenClaw backend
- Scans request bodies for prompt injection patterns (AI defense layer)
- Enforces rate limiting, origin validation, and security headers
- Provides optional HMAC-SHA256 token authentication
- TLS termination with modern cipher suites
- Health check endpoint with backend probing

## Tech Stack
- **Language:** Go 1.25+
- **Proxy:** `net/http/httputil.ReverseProxy` (stdlib)
- **UUID:** `google/uuid`
- **No external runtime dependencies** (single static binary)

## Architecture
```
services/gateway/
+-- main.go           # Reverse proxy, body scanner, graceful shutdown
+-- config/
|   +-- config.go     # Env-based configuration (PROXY_TARGET, auth, TLS)
+-- middleware/
|   +-- sanitize.go   # AI defense: prompt injection detection, XSS stripping
|   +-- security.go   # Security headers, origin check, rate limiter, request ID
|   +-- auth.go       # HMAC-SHA256 token auth (stateless, no DB per request)
+-- tools/
|   +-- tokengen/     # CLI tool to generate auth tokens for testing
+-- Dockerfile        # Multi-stage build (build in Go, run in Alpine)
```

## Security Layers
1. **Security Headers** - HSTS, CSP, X-Frame-Options, X-Content-Type-Options
2. **Origin Validation** - Rejects cross-site request forgery
3. **Rate Limiting** - Per-IP request throttle (60/min default)
4. **Request ID** - UUID tracing through the proxy
5. **Authentication** - HMAC-SHA256 stateless tokens (optional)
6. **Body Scanning** - Prompt injection detection on POST/PUT/PATCH bodies
7. **TLS Termination** - TLS 1.2+ with strong cipher suites (production)
8. **Non-root container** - Runs as `safepaw` user in Docker

## AI Defense (Body Scanner)

The gateway scans JSON request bodies for prompt injection patterns before
forwarding to OpenClaw. Results are attached as headers:

- `X-SafePaw-Risk: none|low|medium|high`
- `X-SafePaw-Triggers: instruction_override,jailbreak_keyword,...`

Detects: instruction overrides, identity hijacking, system delimiter attacks,
encoding evasion, DAN/jailbreak attempts, secret extraction, and more.

## Configuration

Key environment variables:

| Variable | Default | Description |
|---|---|---|
| `PROXY_TARGET` | `http://openclaw:18789` | OpenClaw backend URL |
| `GATEWAY_PORT` | `8080` | Listen port |
| `RATE_LIMIT` | `60` | Max requests per window per IP |
| `AUTH_ENABLED` | `false` | Enable token authentication |
| `AUTH_SECRET` | (required if auth on) | HMAC-SHA256 signing key |
| `TLS_ENABLED` | `false` | Enable HTTPS |
| `MAX_BODY_SIZE` | `1048576` | Max body size for scanning (bytes) |

## Token Auth

Generate a token for testing:
```bash
export AUTH_SECRET=$(openssl rand -base64 48)
go run tools/tokengen/main.go -sub "user123" -scope "proxy" -ttl 24h
```

Enable in docker-compose:
```env
AUTH_ENABLED=true
AUTH_SECRET=<your-secret>
```

## Status
- [x] Reverse proxy to OpenClaw backend
- [x] Security middleware (headers, rate limit, origin check)
- [x] AI defense body scanner (prompt injection detection)
- [x] Authentication middleware (HMAC-SHA256 tokens)
- [x] TLS termination (TLS 1.2+, strong ciphers)
- [x] Health check with backend probing
- [x] Multi-stage Dockerfile (non-root Alpine)
- [x] Token generation CLI tool
- [ ] WebSocket upgrade passthrough optimization
- [ ] Response body scanning (output validation)
