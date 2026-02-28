# SafePaw

**Secure, one-click deployer for [OpenClaw](https://github.com/nicepkg/openclaw).**

Deploy OpenClaw behind a security-hardened reverse proxy with AI defense,
rate limiting, TLS termination, and a guided setup wizard.

> One command: `docker compose up -d`
> OpenClaw + security perimeter + setup UI. Done.

---

## What It Does

SafePaw wraps OpenClaw in a production-ready security layer:

```
Internet --> [Gateway :8080] --> [OpenClaw :18789 (internal)]
                 |
            Security layers:
            - Rate limiting (per-IP)
            - Origin validation
            - HMAC-SHA256 auth (optional)
            - Prompt injection scanning (14 patterns)
            - Security headers (HSTS, CSP, etc.)
            - WebSocket tunnel (explicit upgrade)
            - TLS 1.2+ termination

[Wizard :3000] --> Setup UI + health dashboard (React + Go)
                   - Admin login with signed session tokens
                   - Live Docker container health monitoring
                   - Prerequisite checks (Docker, Compose, ports, disk)
```

OpenClaw handles the AI assistant, channels (Discord, Telegram, Slack, etc.),
and LLM integration. SafePaw handles the security perimeter.

---

## Architecture

```
docker-compose.yml (5 services, internal network, health checks)
|
+-- wizard    (Go + React)  :3000  Setup UI + health dashboard
+-- gateway   (Go)          :8080  Security reverse proxy
+-- openclaw  (Node.js)     :18789 (internal only - never exposed)
+-- redis                   :6379  (internal only - rate limiter state)
+-- postgres                :5432  (internal only - config storage)
```

Only the Wizard (:3000) and Gateway (:8080) are exposed to the host,
and only on `127.0.0.1` (not `0.0.0.0`). Everything else is internal.

### Gateway (Go, zero external deps except `google/uuid`)
Security reverse proxy using `net/http/httputil.ReverseProxy`:
- **Prompt injection scanner** — 14-pattern heuristic body scanner with risk levels
- **WebSocket tunnel** — explicit HTTP upgrade detection with bidirectional copy
- **Rate limiting** — per-IP with background cleanup goroutine
- **HMAC-SHA256 auth** — stateless tokens with scope enforcement (proxy/admin)
- **Security headers** — HSTS, CSP, X-Frame-Options, Referrer-Policy, Permissions-Policy
- **Origin validation** — prevents CSRF/CSWSH attacks
- **Request ID injection** — UUID tracing across services
- **TLS termination** — TLS 1.2+ with curated cipher suites (X25519, ChaCha20)
- **Streaming passthrough** — SSE/WebSocket via `FlushInterval: -1`
- **Auth header stripping** — when auth disabled, strips spoofed identity headers

### Wizard (Go + React 19 + TypeScript + Tailwind)
Setup UI with single-binary deployment (React baked in via `go:embed`):
- **HMAC-SHA256 session tokens** — no raw passwords in cookies; signed, time-limited tokens
- **Live Docker monitoring** — zero-dependency Docker Engine API client over Unix socket
- **Prerequisite checks** — Docker ping, Compose version, port availability, disk space
- **Service dashboard** — 5-second polling, container health/state/uptime
- **Security middleware** — CSP, CORS (localhost only), rate limiting, admin auth
- **SPA routing** — React Router-style fallback from the Go file server

### OpenClaw
Personal AI assistant with 15+ channel integrations. SafePaw proxies
all traffic to it through the Gateway security layer.

---

## Quick Start

```bash
# Clone
git clone https://github.com/beautifulplanet/SafePaw.git
cd SafePaw

# Configure
cp .env.example .env
# Edit .env with your API keys and channel tokens

# Launch
docker compose up -d

# Access
# Setup wizard: http://localhost:3000  (admin password printed on first launch)
# Gateway:      http://localhost:8080  (proxies to OpenClaw)
```

The wizard auto-generates a secure admin password on first launch and prints
it to stdout. Set `WIZARD_ADMIN_PASSWORD` in `.env` to use a fixed password.

---

## Security Design

### Defense in Depth
Every request passes through multiple security layers. If one layer fails,
the next catches it:

```
Request
  -> Security Headers (HSTS, CSP, X-Frame-Options)
  -> Request ID (UUID tracing)
  -> Origin Check (CSRF/CSWSH protection)
  -> Rate Limit (per-IP throttle)
  -> Auth (HMAC token validation, optional)
  -> Body Scanner (prompt injection detection)
  -> Reverse Proxy -> OpenClaw
```

### Session Tokens (Wizard)
The wizard uses custom HMAC-SHA256 signed tokens (similar to JWT,
zero external dependencies):
- Token format: `base64url(payload).base64url(hmac_signature)`
- Payload: `{"sub":"admin","iat":unix,"exp":unix}`
- Signing key: admin password (never leaves the server)
- HttpOnly, SameSite=Strict cookies for browser sessions
- Bearer tokens for API clients
- 24-hour TTL with server-side expiry validation

### Docker Socket Access
The wizard has read-only access to the Docker socket (`/var/run/docker.sock:ro`)
for health monitoring. It uses a zero-dependency Docker Engine API client (v1.43)
to query container state without pulling in the full Docker SDK.

### AI Defense Patterns Detected

The body scanner catches:
- **Instruction override** — "ignore previous instructions"
- **Identity hijacking** — "you are now admin"
- **System delimiter injection** — `` ```system ``, `<|system|>`, `[SYSTEM]`
- **Secret extraction** — "reveal your system prompt"
- **Jailbreak keywords** — DAN, developer mode
- **Encoding evasion** — base64/eval tricks
- **Data exfiltration** — external URL injection
- **Role injection** — `ASSISTANT:`, `SYSTEM:` framing

Risk levels: `none`, `low`, `medium`, `high` — attached as `X-SafePaw-Risk` header.

---

## Skills Demonstrated

| Skill | Evidence |
|-------|----------|
| **Go systems programming** | Reverse proxy, goroutine rate limiters, graceful shutdown, Unix socket Docker client |
| **Security engineering** | HMAC-SHA256 auth (constant-time `hmac.Equal`), signed session tokens, CSP, CORS, rate limiting, non-root containers |
| **AI defense** | 14-pattern heuristic body scanner for prompt injection, risk levels, trigger logging |
| **React + TypeScript** | React 19, strict mode, typed API client, state machine routing, Tailwind UI |
| **Infrastructure as code** | Docker Compose with health checks, resource limits, internal networks, SHA256-pinned images |
| **Testing** | 30 Go unit tests (session, middleware, handler, WS proxy) |
| **Zero-dep philosophy** | Docker client over raw HTTP (no SDK), session tokens (no JWT library), embed (no nginx) |
| **Build tooling** | Multi-stage Docker builds, stripped Go binaries, Vite + tsc, `go:embed` |

---

## Configuration

All configuration via environment variables (`.env` file):

### Essential
| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key for OpenClaw |
| `OPENAI_API_KEY` | OpenAI API key (optional) |

### Channel Tokens
| Variable | Description |
|----------|-------------|
| `DISCORD_BOT_TOKEN` | Discord bot token |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `SLACK_BOT_TOKEN` | Slack bot token |
| `SLACK_APP_TOKEN` | Slack app-level token |

### Security
| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_ENABLED` | `false` | Enable gateway authentication |
| `AUTH_SECRET` | — | HMAC signing key (min 32 bytes) |
| `TLS_ENABLED` | `false` | Enable HTTPS |
| `RATE_LIMIT` | `60` | Requests per minute per IP |
| `WIZARD_ADMIN_PASSWORD` | auto-generated | Wizard admin password |

See [.env.example](.env.example) for the full list.

---

## Project Structure

```
safepaw/
+-- docker-compose.yml          # 5-service orchestration
+-- .env.example                 # Configuration template
+-- SECURITY.md                  # Incident response, logging, hardening, Phase 2 roadmap
+-- services/
|   +-- gateway/                 # Go reverse proxy (zero deps except uuid)
|   |   +-- main.go              # Proxy + body scanner + WS routing
|   |   +-- ws_proxy.go          # Explicit WebSocket tunnel
|   |   +-- ws_proxy_test.go     # WS upgrade detection tests
|   |   +-- config/              # Env-based config
|   |   +-- middleware/
|   |   |   +-- sanitize.go      # AI defense (14 patterns, 404 lines)
|   |   |   +-- security.go      # Headers, rate limit, origin, request ID
|   |   |   +-- auth.go          # HMAC-SHA256 tokens (scoped, stateless)
|   |   +-- tools/tokengen/      # Token generation CLI
|   |   +-- Dockerfile           # Multi-stage, alpine, non-root
|   +-- wizard/                  # Go setup UI + embedded React SPA
|   |   +-- cmd/wizard/          # Entry point, middleware chain
|   |   +-- internal/
|   |   |   +-- api/             # REST handler + SPA fallback
|   |   |   +-- config/          # Env config, auto-gen password
|   |   |   +-- docker/          # Docker Engine API client (zero deps)
|   |   |   +-- middleware/      # Security headers, CORS, auth, rate limit
|   |   |   +-- session/         # HMAC-SHA256 session tokens
|   |   +-- ui/                  # React 19 + TypeScript 5.8 + Tailwind 3.4
|   |   |   +-- src/pages/       # Login, Prerequisites, Dashboard
|   |   |   +-- src/api.ts       # Typed fetch wrapper
|   |   |   +-- embed.go         # go:embed all:dist
|   |   |   +-- dist/            # Production build (baked into Go binary)
|   |   +-- Dockerfile           # Multi-stage, alpine, non-root
|   +-- postgres/
|       +-- init/                # Schema init scripts
+-- _archived/                   # Previous architecture (portfolio evidence)
|   +-- router/                  # Go Redis Streams message router
|   +-- agent/                   # TypeScript echo service
|   +-- gateway-redis/           # Redis stream client
|   +-- gateway-ws/              # WebSocket hub + handler
+-- shared/
    +-- proto/                   # Protobuf schemas
```

---

## Development

```bash
# Gateway
cd services/gateway
go build -o gateway .
PROXY_TARGET=http://localhost:18789 ./gateway

# Generate auth token
export AUTH_SECRET=$(openssl rand -base64 48)
go run tools/tokengen/main.go -sub "admin" -scope "proxy" -ttl 24h

# Wizard
cd services/wizard
go build -o wizard ./cmd/wizard
WIZARD_ADMIN_PASSWORD=dev ./wizard

# Wizard UI (hot reload)
cd services/wizard/ui
npm install
npm run dev  # Vite dev server on :5173, proxies /api to :3000

# Run all tests
cd services/gateway && go test ./... -v
cd services/wizard && go test ./... -v
```

---

## Test Coverage

| Package | Tests | What's Covered |
|---------|-------|---------------|
| `wizard/internal/session` | 6 | Token create/validate, wrong secret, expired, invalid format, uniqueness, tampered payload |
| `wizard/internal/middleware` | 13 | Security headers, CORS (allowed/disallowed/preflight), AdminAuth (6 cases), rate limit, public paths |
| `wizard/internal/api` | 7 | Health, login success/fail/bad body, SPA fallback, service restart (unknown service, no Docker) |
| `gateway` | 8 | WebSocket upgrade detection (valid, case-insensitive, multi-value, missing headers) |
| **Total** | **34** | |

---

## Status

| Component | Status | Notes |
|-----------|--------|-------|
| Gateway reverse proxy | **Complete** | HTTP + WebSocket proxying |
| Security middleware | **Complete** | 5 layers: headers, origin, rate limit, auth, body scanner |
| AI body scanner | **Complete** | 14 prompt injection patterns, 4 risk levels |
| WebSocket tunnel | **Complete** | Explicit upgrade with bidirectional copy |
| TLS termination | **Complete** | TLS 1.2+, strong ciphers, X25519 |
| Wizard REST API | **Complete** | Health, login, prerequisites, status, service restart, SPA fallback |
| Wizard React UI | **Complete** | Login, prerequisites, dashboard with live polling and service restart |
| Session tokens | **Complete** | HMAC-SHA256 signed, HttpOnly cookies, 24h TTL |
| Docker health client | **Complete** | Zero-dep Engine API client over Unix socket |
| Docker Compose | **Complete** | 5-service orchestration with health checks |
| Unit tests | **Complete** | 34 tests across 4 packages |
| SECURITY.md | **Complete** | Incident response, logging, defense-in-depth, Phase 2 revocation roadmap |

---

## License

MIT
