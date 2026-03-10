# Development Guide

Configuration reference, build commands, testing, and project structure for SafePaw (InstallerClaw).

**Related:** [CONTRIBUTING.md](../CONTRIBUTING.md) — coding standards and PR process · [PITFALLS.md](../PITFALLS.md) — known gotchas · [README.md](../README.md) — project overview

---

## Configuration

All configuration via environment variables (`.env` in the repo root). See [.env.example](../.env.example) for the full list with comments.

### Essential

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key for OpenClaw |
| `OPENAI_API_KEY` | OpenAI API key (optional) |

### Channel tokens (optional)

| Variable | Description |
|----------|-------------|
| `DISCORD_BOT_TOKEN` | Discord bot token |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` | Slack bot and app tokens |

### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_ENABLED` | `true` (Docker) | Enable gateway HMAC auth |
| `AUTH_SECRET` | — | HMAC signing key (min 32 bytes) |
| `TLS_ENABLED` | `false` | Enable TLS on gateway |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | /certs/... | TLS cert and key paths |
| `RATE_LIMIT` | 60 | Requests per minute per IP (global default) |
| `RATE_LIMIT_WINDOW_SEC` | 60 | Rate limit window in seconds |
| `LOG_FORMAT` | `text` | `json` for SIEM-style structured logs |
| `WIZARD_ADMIN_PASSWORD` | auto-generated | Wizard admin password |
| `WIZARD_TOTP_SECRET` | — | Optional base32 TOTP secret for wizard MFA |

### Per-endpoint rate limits

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_HEALTH` | 120 | `/health` endpoint |
| `RATE_LIMIT_METRICS` | 30 | `/metrics` endpoint |
| `RATE_LIMIT_ADMIN` | 10 | `/admin/*` endpoints |
| `RATE_LIMIT_WS` | 5 | WebSocket upgrade |

---

## Building

```bash
# Gateway
cd services/gateway
go build -o gateway .
PROXY_TARGET=http://localhost:18789 ./gateway

# Generate a token (when auth enabled)
export AUTH_SECRET=$(openssl rand -base64 48)
go run tools/tokengen/main.go -sub admin -scope proxy -ttl 24h

# Wizard backend
cd services/wizard
go build -o wizard ./cmd/wizard
WIZARD_ADMIN_PASSWORD=dev ./wizard

# Wizard UI (hot reload)
cd services/wizard/ui
npm install && npm run dev
```

From the repo root: `make lint`, `make vulncheck`, `make fuzz`.

---

## Testing

| Suite | Location | Command |
|-------|----------|---------|
| Gateway unit + integration | `services/gateway` | `go test ./... -race` |
| Wizard unit + integration | `services/wizard` | `go test ./... -race` |
| Fuzz (gateway) | `services/gateway` | `go test -fuzz=...` or `make fuzz` |
| Vulnerability check | repo root | `make vulncheck` |
| E2E (live stack) | repo root | `make test-e2e` or `./scripts/verify-deployment.sh` |
| Playwright (wizard login) | repo root | `npx playwright test` (after `docker compose up -d`) |

### Coverage

| Service | Current | CI gate |
|---------|---------|---------|
| Gateway | 80.5% | >65% |
| Wizard | 64.2% | ≥60% |

CI runs build, test with `-race`, lint (golangci-lint), gosec, govulncheck, coverage gates, fuzz seed corpus, and Docker build on every push.

### Key test areas

- **Gateway middleware:** Auth, rate limiting, brute-force, body scanner, output scanner, CORS, headers, request ID
- **Gateway integration:** Full proxy chain, Redis revocation, WebSocket, token lifecycle
- **Wizard middleware:** CSRF protection, RBAC (RequireRole), LoginGuard — all at 92% coverage
- **Wizard API:** Session management, TOTP, audit log, config CRUD, container health
- **Fuzz targets:** Prompt injection patterns, sanitizer, channel parsing, output scanner, token validation, KV operations

---

## Project structure

```
SafePaw/
├── docker-compose.yml         # 5+ services, health checks, resource limits
├── Makefile                   # build, test, lint, vulncheck, fuzz, Docker, e2e
├── start.sh / start.bat       # One-command setup (generates .env, starts stack)
├── .env.example               # All env vars with comments
├── go.work                    # Go workspace (gateway, wizard, mockbackend)
│
├── services/
│   ├── gateway/               # Go reverse proxy + middleware + tools/tokengen
│   ├── wizard/                # Go backend + React 19 UI (cmd/, internal/, ui/)
│   ├── mockbackend/           # Test backend for integration tests
│   ├── openclaw/              # OpenClaw Dockerfile
│   └── postgres/init/         # DB init scripts
│
├── monitoring/                # Prometheus config, Grafana dashboards, alert rules
├── shared/proto/              # Shared protocol definitions
│
├── docs/
│   ├── ARCHITECTURE.md        # Full technical architecture with diagrams
│   ├── COMPLIANCE.md          # SOC 2 & GDPR control mapping
│   ├── PATCHING-POLICY.md     # Dependency update SLAs
│   ├── PENTEST-POLICY.md      # Penetration testing policy
│   ├── SECRETS-MIGRATION.md   # Vault migration guide
│   ├── adr/                   # 9 architecture decision records
│   └── scope/                 # SOW-001, CO-001, CO-002
│
├── SECURITY.md                # Incident response, hardening, defense-in-depth
├── RUNBOOK.md                 # 6 incident playbooks
├── BACKUP-RECOVERY.md         # Backup and restore procedures
├── THREAT-MODEL.md            # STRIDE threat model (48 threats)
├── CONTRIBUTING.md            # Dev workflow and coding standards
├── PITFALLS.md                # Known gotchas
├── CHANGELOG.md               # Release history
└── SAFEGUARDS.md              # CI and testing safeguards
```

---

## Troubleshooting

| Issue | What to do |
|-------|------------|
| Lost wizard admin password | `docker compose logs wizard` (first lines) or set `WIZARD_ADMIN_PASSWORD` in `.env` and restart. See [SECURITY.md](../SECURITY.md) § Recovery. |
| Prerequisites fail | Install Docker and Compose V2; ensure ports 3000 and 8080 are free. |
| Dashboard shows no services | Wizard needs read-only Docker socket; check compose mount for `/var/run/docker.sock` (or npipe on Windows). |
| Gateway 502 / backend unreachable | OpenClaw may still be starting. `docker compose logs openclaw`; `curl http://localhost:8080/health`. |
| Auth required | Set `AUTH_ENABLED=true` and `AUTH_SECRET`; use `tools/tokengen` to create tokens. |
| CI coverage gate fails | Run `go test -coverprofile=c.out ./...` locally and check with `go tool cover -func=c.out \| grep total`. Gateway needs >65%, wizard needs ≥60%. |

---

## Production hardening

Before exposing anything beyond localhost, work through the checklist in [SECURITY.md](../SECURITY.md):

- Strong `WIZARD_ADMIN_PASSWORD` + TOTP enabled
- `AUTH_ENABLED=true` with strong `AUTH_SECRET` (min 32 bytes)
- TLS enabled with valid certs
- Wizard bound to `127.0.0.1` only (default)
- Rate limits tuned for your load
- `LOG_FORMAT=json` feeding your SIEM
- Secrets rotated on schedule ([RUNBOOK.md](../RUNBOOK.md))
- `./scripts/verify-deployment.sh` passes
