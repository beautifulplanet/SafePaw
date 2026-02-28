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
            - Prompt injection scanning
            - Security headers (HSTS, CSP, etc.)
            - TLS 1.2+ termination

[Wizard :3000] --> Setup UI for configuration and health monitoring
```

OpenClaw handles the AI assistant, channels (Discord, Telegram, Slack, etc.),
and LLM integration. SafePaw handles the security perimeter.

---

## Architecture

```
docker-compose.yml
|
+-- wizard    (Go)     :3000  Setup UI + health dashboard
+-- gateway   (Go)     :8080  Security reverse proxy
+-- openclaw  (Node)   :18789 (internal only - never exposed)
+-- redis              :6379  (internal only - rate limiter state)
+-- postgres           :5432  (internal only - config storage)
```

### Gateway (Go)
Security reverse proxy using `net/http/httputil.ReverseProxy`:
- Prompt injection body scanner (heuristic pattern matching)
- Per-IP rate limiting with automatic cleanup
- HMAC-SHA256 stateless token auth
- Security headers (HSTS, CSP, X-Frame-Options, etc.)
- Origin validation (CSRF protection)
- Request ID injection (UUID tracing)
- TLS termination with curated cipher suites
- Streaming response passthrough (SSE/WebSocket)

### Wizard (Go)
Setup UI with embedded React SPA:
- Docker health monitoring
- Service status dashboard
- Admin authentication
- CORS and rate limiting

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
# Setup wizard: http://localhost:3000
# Gateway:      http://localhost:8080
```

**First-run:** The wizard prints an auto-generated admin password once to stdout. To retrieve it: `docker compose logs wizard` or `docker logs safepaw-wizard`. Set `WIZARD_ADMIN_PASSWORD` in `.env` to use a fixed password.

---

## Security

SafePaw is designed with defense in depth: rate limiting, origin validation, HMAC auth, prompt-injection scanning, and TLS. OpenClaw has **no host-exposed ports**—all traffic goes through the gateway.

- **Incident response, logging reference, and hardening:** See [SECURITY.md](SECURITY.md).
- **Production:** Set `AUTH_ENABLED=true`, provide `AUTH_SECRET`, and enable `TLS_ENABLED` with valid certificates. Use a strong `WIZARD_ADMIN_PASSWORD` in `.env`.

---

## Skills Demonstrated

| Skill | Evidence |
|-------|----------|
| **Go systems programming** | Reverse proxy with middleware chain, goroutine-based rate limiter, graceful shutdown |
| **Security engineering** | HMAC-SHA256 auth, TLS 1.2+, constant-time comparison, prompt injection detection, non-root containers |
| **AI defense** | 14-pattern heuristic scanner for prompt injection, identity hijacking, jailbreak attempts, delimiter attacks |
| **Infrastructure as code** | Docker Compose with health checks, resource limits, internal-only networks, volume management |
| **API design** | RESTful wizard API, middleware chain pattern, structured JSON error responses |
| **Build tooling** | Multi-stage Docker builds, stripped binaries, Alpine base images (5MB attack surface) |

### AI Defense Patterns Detected

The body scanner catches:
- **Instruction override** - "ignore previous instructions"
- **Identity hijacking** - "you are now admin"
- **System delimiter injection** - `` ```system ``, `<|system|>`, `[SYSTEM]`
- **Secret extraction** - "reveal your system prompt"
- **Jailbreak keywords** - DAN, developer mode
- **Encoding evasion** - base64/eval tricks
- **Data exfiltration** - external URL injection
- **Role injection** - `ASSISTANT:`, `SYSTEM:` framing

Risk levels: `none`, `low`, `medium`, `high` -- attached as `X-SafePaw-Risk` header.

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
| `AUTH_SECRET` | -- | HMAC signing key (min 32 bytes) |
| `TLS_ENABLED` | `false` | Enable HTTPS |
| `RATE_LIMIT` | `60` | Requests per minute per IP |

See [.env.example](.env.example) for the full list.

---

## Project Structure

```
safepaw/
+-- docker-compose.yml      # Orchestration (5 services)
+-- .env.example             # Configuration template
+-- README.md
+-- SECURITY.md              # Incident response, logging, hardening (see Security section)
+-- services/
|   +-- gateway/             # Go reverse proxy
|   |   +-- main.go          # Proxy + body scanner
|   |   +-- config/          # Env-based config
|   |   +-- middleware/       # Security stack
|   |   |   +-- sanitize.go  # AI defense (404 lines)
|   |   |   +-- security.go  # Headers, rate limit, origin
|   |   |   +-- auth.go      # HMAC-SHA256 tokens
|   |   +-- tools/tokengen/  # Token generation CLI
|   |   +-- Dockerfile
|   +-- wizard/              # Go setup UI
|       +-- cmd/wizard/      # Entry point
|       +-- internal/        # API, config, middleware
|       +-- ui/              # Embedded React SPA
+-- _archived/               # Previous services (portfolio evidence)
|   +-- router/              # Go Redis Streams message router
|   +-- agent/               # TypeScript echo service
|   +-- gateway-redis/       # Redis stream client
|   +-- gateway-ws/          # WebSocket hub + handler
+-- shared/
    +-- proto/               # Protobuf schemas (4 files)
```

---

## Development

```bash
# Build gateway locally
cd services/gateway
go build -o gateway .

# Run with custom target
PROXY_TARGET=http://localhost:18789 ./gateway

# Generate auth token
export AUTH_SECRET=$(openssl rand -base64 48)
go run tools/tokengen/main.go -sub "admin" -scope "proxy" -ttl 24h
```

---

## Status

| Component | Status | Notes |
|-----------|--------|-------|
| Gateway reverse proxy | **Complete** | HTTP + WebSocket proxying |
| Security middleware | **Complete** | Headers, origin, rate limit, auth, body scanner; request ID in logs |
| AI body scanner | **Complete** | 14 prompt injection patterns, 4 risk levels |
| TLS termination | **Complete** | TLS 1.2+, strong ciphers |
| Wizard setup UI | **Complete** | Login, prerequisites, dashboard with live polling, service restart |
| Session tokens | **Complete** | HMAC-SHA256, HttpOnly cookies, 24h TTL |
| Docker Compose | **Complete** | 5-service orchestration, health checks |
| SECURITY.md | **Complete** | Incident response, logging reference, defense-in-depth, Phase 2 roadmap |
| Unit tests | **Complete** | Wizard (session, middleware, api), gateway (WS upgrade) |
| Integration tests | Not started | End-to-end Docker flow optional |

---

## License

MIT
