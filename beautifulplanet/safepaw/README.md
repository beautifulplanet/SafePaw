# SafePaw

**Secure, one-click deployer for [OpenClaw](https://github.com/nicepkg/openclaw).**

Security perimeter (Go gateway + Go/React wizard) for the OpenClaw personal AI assistant: reverse proxy with auth, rate limiting, prompt-injection and output scanning, TLS, and a guided setup UI. One command brings up the full stack from this directory.

---

### Impact

- **One-command deploy** — `docker compose up -d`; six services (wizard, gateway, openclaw, redis, postgres, docker-socket-proxy) with health checks; only wizard and gateway exposed on 127.0.0.1.
- **All traffic through the gateway** — OpenClaw has no host-exposed ports; auth, rate limiting, brute-force protection, and AI-defense scanning apply first.
- **Wizard for ops** — Admin login (session tokens, optional TOTP), prerequisites, live container status, masked .env edit, audit log.
- **258+ tests** — Go unit and integration tests, 7 fuzz targets; CI: build, test, lint, gosec, govulncheck, coverage gate (60%), Docker build.
- **Operational docs** — Runbooks (6 types), backup/restore, secret rotation, STRIDE threat model.

**Stack:** Go · React 19 · TypeScript · Tailwind · Docker Compose · Redis · PostgreSQL · Prometheus · Grafana

---

### Evidence

| Claim | Proof |
|-------|--------|
| Stack runs | `docker compose up -d` — wizard :3000, gateway :8080 |
| Gateway health | `curl -s http://localhost:8080/health` |
| Prometheus metrics | `curl -s http://localhost:8080/metrics` |
| Gateway tests | `cd services/gateway && go test ./... -race` |
| Wizard tests | `cd services/wizard && go test ./... -race` |
| Fuzz targets | `make fuzz` |
| Vuln check | `make vulncheck` |
| Runbooks | [RUNBOOK.md](RUNBOOK.md) |
| Backup procedures | [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md) |
| Threat model | [THREAT-MODEL.md](THREAT-MODEL.md) |

---

### How to read this README

| If you want… | Go to | Time |
|--------------|--------|------|
| Run the stack | [Quick start](#quick-start) | 2 min |
| Understand architecture | [Architecture](#architecture), [Request flow](#request-flow) | 2 min |
| Configure env vars | [Configuration](#configuration) | 1 min |
| Develop or test locally | [Development](#development), [Testing](#testing) | 5 min |
| Security and runbooks | [SECURITY.md](SECURITY.md), [RUNBOOK.md](RUNBOOK.md) | 2 min |
| Troubleshoot | [Troubleshooting](#troubleshooting) | 1 min |

---

## Quick start

### Prerequisites

- Docker and Docker Compose (v2)
- Ports 3000 and 8080 free
- For production: `AUTH_ENABLED=true`, `AUTH_SECRET`, and optionally `TLS_ENABLED` with certs

### Commands

```bash
# From this directory (safepaw)
cp .env.example .env
# Edit .env: API keys, channel tokens, AUTH_SECRET if using auth

docker compose up -d

# Wizard:  http://localhost:3000   (password in docker compose logs wizard or WIZARD_ADMIN_PASSWORD in .env)
# Gateway: http://localhost:8080
```

First run: wizard prints an admin password once. Use `docker compose logs wizard` or set `WIZARD_ADMIN_PASSWORD` in `.env`.

> **Before going to production**, read the [Production Hardening Checklist](#production-hardening-checklist) below.

---

## Architecture

```
docker-compose.yml (6 services, internal network)
|
+-- wizard    (Go + React)  :3000  Setup UI + health dashboard
+-- gateway   (Go)          :8080  Security reverse proxy
+-- openclaw  (Node.js)     :18789 (internal only)
+-- redis                   :6379  (internal — rate limit + revocation state)
+-- postgres                :5432  (internal — config storage)
+-- docker-socket-proxy     :2375  (internal — scoped Docker API access)
```

Only wizard and gateway bind to the host (127.0.0.1).

### Request flow

```
Request → Metrics → Security headers → Request ID (server UUID only)
        → Origin check → Brute-force guard → Rate limit → Auth (HMAC + Redis revocation)
                → Body scanner (prompt injection) → Output scanner (XSS/secrets)
                        → Reverse proxy → OpenClaw
                        ```

                        ### Gateway (Go)

                        - Body scanner (14 patterns, versioned v2.0.0), output scanner (XSS, secret leaks, exfil)
                        - Rate limiting (per-IP), brute-force guard (IP ban, fed by auth + rate limit)
                        - HMAC auth, Redis-backed revocation, `POST /admin/revoke`
                        - Security headers, origin validation, server-only request IDs, optional TLS
                        - Prometheus `/metrics`, structured JSON logging (`LOG_FORMAT=json`)

                        ### Wizard (Go + React 19 + TypeScript + Tailwind)

                        - HMAC session tokens (jti for replay protection), optional TOTP (MFA)
                        - Docker Engine API over Unix socket via **scoped proxy** (tecnativa/docker-socket-proxy — allows only container list, inspect, restart; blocks exec, create, images, volumes, networks), prerequisite checks, live dashboard
                        - GET/PUT config (allowed keys, masked secrets), audit log, service restart

                        ### AI defense patterns (body scanner)

                        Instruction override, identity hijacking, system delimiter injection, secret extraction, jailbreak keywords, encoding evasion, data exfiltration, role injection. Risk levels: `none`, `low`, `medium`, `high` → `X-SafePaw-Risk` header.

                        ---

                        ## Configuration

                        All via environment variables (`.env`).

                        | Variable | Default | Description |
                        |----------|---------|-------------|
                        | `ANTHROPIC_API_KEY` | — | Anthropic API key for OpenClaw |
                        | `OPENAI_API_KEY` | — | OpenAI API key (optional) |
                        | `AUTH_ENABLED` | `true` (Docker) | Enable gateway auth |
                        | `AUTH_SECRET` | — | HMAC signing key (min 32 bytes) |
                        | `TLS_ENABLED` | `false` | Enable TLS on gateway |
                        | `RATE_LIMIT` | 60 | Requests per minute per IP |
                        | `LOG_FORMAT` | `text` | `json` for SIEM-style logs |
                        | `WIZARD_ADMIN_PASSWORD` | auto-generated | Wizard admin password |
                        | `WIZARD_TOTP_SECRET` | — | Optional base32 TOTP for wizard MFA |

                        Channel tokens: `DISCORD_BOT_TOKEN`, `TELEGRAM_BOT_TOKEN`, `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`. See [.env.example](.env.example) for the full list.

                        ---

                        ## Development

                        ```bash
                        # Gateway
                        cd services/gateway
                        go build -o gateway .
                        PROXY_TARGET=http://localhost:18789 ./gateway

                        # Token (when auth enabled)
                        export AUTH_SECRET=$(openssl rand -base64 48)
                        go run tools/tokengen/main.go -sub admin -scope proxy -ttl 24h

                        # Wizard
                        cd services/wizard
                        go build -o wizard ./cmd/wizard
                        WIZARD_ADMIN_PASSWORD=dev ./wizard

                        # Wizard UI (hot reload)
                        cd services/wizard/ui && npm install && npm run dev
                        ```

                        From this directory: `make lint`, `make vulncheck`, `make fuzz`. See [CONTRIBUTING.md](CONTRIBUTING.md).

                        ---

                        ## Testing

                        | Suite | Command |
                        |-------|---------|
                        | Gateway | `cd services/gateway && go test ./... -race` |
                        | Wizard | `cd services/wizard && go test ./... -race` |
                        | Fuzz | `make fuzz` |
                        | Vuln check | `make vulncheck` |
                        | E2E (live stack) | `./scripts/verify-deployment.sh` after `docker compose up -d` |

                        CI runs build, test with coverage gate (60%), lint, gosec, govulncheck, Docker build.

                        ---

                        ## Project structure

                        ```
                        safepaw/
                        ├── docker-compose.yml       # 6 services, health checks, resource limits
                        ├── Makefile                 # build, test, lint, vulncheck, fuzz, Docker
                        ├── .env.example
                        ├── SECURITY.md              # Incident response, logging, hardening, MFA
                        ├── RUNBOOK.md               # 6 playbooks, secret rotation
                        ├── THREAT-MODEL.md          # STRIDE (27 threats)
                        ├── BACKUP-RECOVERY.md       # Postgres, Redis, volumes, .env
                        ├── CONTRIBUTING.md
                        ├── monitoring/              # Prometheus, Grafana, alerts
                        ├── services/
                        │   ├── gateway/             # Go reverse proxy, middleware, tools/tokengen
                        │   ├── wizard/              # Go + React (cmd, internal, ui)
                        │   └── postgres/init/
                        ├── _archived/               # Legacy / previous architecture
                        └── shared/proto/
                        ```

                        ---

                        ## Troubleshooting

                        | Issue | What to do |
                        |-------|------------|
                        | Lost wizard admin password | `docker compose logs wizard` or set `WIZARD_ADMIN_PASSWORD` in `.env` and restart. [SECURITY.md](SECURITY.md) § Recovery. |
                        | Prerequisites fail | Install Docker and Compose; ensure 3000 and 8080 are free. |
                        | Dashboard shows no services | Check Docker socket mount (`/var/run/docker.sock` or npipe on Windows). |
                        | Gateway 502 | `docker compose logs openclaw`; `curl http://localhost:8080/health`. |
                        | Auth required | Set `AUTH_ENABLED=true` and `AUTH_SECRET`; use `tools/tokengen` for tokens. |

                        ---

                        ## Verify deployment

                        ```bash
                        curl -s http://localhost:3000/api/v1/health | jq .
                        curl -s http://localhost:8080/health | jq .
                        ```

                        Then open http://localhost:3000, sign in, and check the dashboard. Full script: `./scripts/verify-deployment.sh`.

                        ---

                        ## Limitations

                        - **Prompt-injection and output scanning** — Heuristic regex patterns only (not a security boundary). Useful as a tripwire/defense-in-depth layer, but cannot catch novel, obfuscated, or adversarial attacks. No ML/LLM-based detection. See [SECURITY.md](SECURITY.md).
                        - **Output scanning** — Same caveat: heuristic regex for XSS, secret leaks, and exfil. Treat as a helpful early-warning layer, not a guarantee.
                        - **Revocation** — Redis-backed when Redis is configured; in-memory fallback. Brute-force bans are in-memory only.
                        - **Wizard password** — No "forgot password"; recovery via logs or `.env` and restart.
                        - **Wizard compromise** — If an attacker gains wizard access, they can read/write API keys and tokens via PUT `/api/v1/config`. Docker API access is scoped via docker-socket-proxy (container list/inspect/restart only — no exec, create, images, or volumes). The wizard should only be accessible from localhost or behind a VPN.
                        - **Stack** — Docker-first; no generic bare-metal install path.

                        ---

                        ## Production Hardening Checklist

                        Do this **before** exposing anything beyond localhost:

                        - [ ] **Set a strong admin password** — `WIZARD_ADMIN_PASSWORD` in `.env` (don't rely on auto-generated)
                        - [ ] **Enable MFA** — Set `WIZARD_TOTP_SECRET` for two-factor login on the wizard
                        - [ ] **Enable auth on the gateway** — `AUTH_ENABLED=true` + a strong `AUTH_SECRET` (min 32 bytes)
                        - [ ] **Enable TLS** — `TLS_ENABLED=true` with valid certs (`TLS_CERT_FILE`, `TLS_KEY_FILE`)
                        - [ ] **Keep wizard on localhost** — The wizard binds to `127.0.0.1` by default. Never expose it to the internet without a VPN or reverse proxy with auth.
                        - [ ] **Set system profile** — Choose `small`/`medium`/`large`/`very-large` in Settings to match your server's RAM
                        - [ ] **Review rate limits** — Default is 60 req/min/IP. Tune `RATE_LIMIT` and `RATE_LIMIT_WINDOW_SEC` for your load.
                        - [ ] **Rotate secrets on schedule** — See [RUNBOOK.md](RUNBOOK.md) for rotation playbooks
                        - [ ] **Run the verification script** — `./scripts/verify-deployment.sh` after starting the stack
                        - [ ] **Monitor logs** — Set `LOG_FORMAT=json` and feed to your SIEM. Alert on `[AUTH]` failures, `[SCANNER]` high-risk, `[RATELIMIT]` denials.
                        - [ ] **Understand scanning limits** — Prompt-injection and output scanners are heuristic tripwires, not security boundaries. They catch many known attacks but cannot stop novel or obfuscated ones.

                        ---

                        ## Documentation index

                        | Document | Purpose |
                        |----------|---------|
                        | [README.md](README.md) | This file |
                        | [SECURITY.md](SECURITY.md) | Incident response, logging, defense-in-depth, MFA, recovery |
                        | [RUNBOOK.md](RUNBOOK.md) | 6 playbooks, secret rotation |
                        | [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md) | Backup/restore procedures |
                        | [THREAT-MODEL.md](THREAT-MODEL.md) | STRIDE, residual risks |
                        | [SCOPE-IMPROVEMENTS.md](SCOPE-IMPROVEMENTS.md) | Review feedback triage and improvement backlog |
                        | [CONTRIBUTING.md](CONTRIBUTING.md) | Dev workflow, PR process |
                        | [RELEASE.md](RELEASE.md) | Going public: timestamp, checklist, positioning, licensing |
                        | [.env.example](.env.example) | Configuration reference |

                        ---

                        ## License

                        MIT
                        