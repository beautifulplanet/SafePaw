<p align="center">
  <strong>InstallerClaw (SafePaw)</strong><br/>
  Secure, one-click deployer for OpenClaw
</p>

<p align="center">
  <a href="#quickstart"><strong>Quickstart</strong></a> &middot;
  <a href="#part-2-tech-stack--architecture"><strong>Architecture</strong></a> &middot;
  <a href="https://github.com/beautifulplanet/SafePaw"><strong>GitHub</strong></a> &middot;
  <a href="beautifulplanet/safepaw/SECURITY.md"><strong>Security</strong></a> &middot;
  <a href="beautifulplanet/safepaw/RUNBOOK.md"><strong>Runbook</strong></a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT License" /></a>
  <a href="https://github.com/beautifulplanet/SafePaw/stargazers"><img src="https://img.shields.io/github/stars/beautifulplanet/SafePaw?style=flat" alt="Stars" /></a>
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go" alt="Go 1.24" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react" alt="React 19" />
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker" alt="Docker Compose" />
</p>

<br/>

## What is InstallerClaw?

# Secure perimeter for self-hosted AI assistants

**If OpenClaw is the _AI assistant_, InstallerClaw is the _security guard + setup crew_ around it.**

InstallerClaw is a Go gateway and React wizard that wraps [OpenClaw](https://github.com/nicepkg/openclaw) in a hardened Docker environment. Auth, rate limiting, prompt-injection scanning, TLS, and guided setup — one command to deploy, one UI to manage.

We call it **InstallerClaw** because it is the install and deploy layer around OpenClaw. The repo and paths still use **SafePaw** / `safepaw` for historical reasons; treat "InstallerClaw" as the product name and "SafePaw" as the codebase/organizational name.

**The trend:** Self-hosted AI assistants (OpenClaw, and similar local/private LLM front-ends) are growing fast. InstallerClaw is a safe, deployable perimeter — auth, scanning, and ops in one place — so you never expose the assistant without a guardrail layer.

<br/>

## InstallerClaw is right for you if

- ✅ You run **OpenClaw** (or any self-hosted AI assistant) and want a security layer in front of it
- ✅ You want **one command** to deploy a hardened stack with auth, rate limiting, and scanning
- ✅ You're **not a security expert** but want defense-in-depth without building it yourself
- ✅ You need **operational tooling** — health dashboard, config UI, audit logs, incident runbooks
- ✅ You want **guided setup** instead of editing YAML files and hoping for the best
- ✅ You care about **prompt injection defense** and want heuristic scanning on every request

<br/>

## Features

<table>
<tr>
<td align="center" width="33%">
<h3>🛡️ Security Gateway</h3>
HMAC auth, per-IP rate limiting, brute-force banning, origin validation, TLS termination. Every request authenticated before it reaches the AI.
</td>
<td align="center" width="33%">
<h3>🔍 AI Defense Scanning</h3>
14-pattern prompt injection body scanner + output scanner (XSS, secret leaks). Heuristic, versioned, auditable. Risk headers on every response.
</td>
<td align="center" width="33%">
<h3>🧙 Setup Wizard</h3>
React UI with guided setup: prerequisite checks, API key config, service health dashboard, masked secrets. No YAML editing required.
</td>
</tr>
<tr>
<td align="center">
<h3>🔐 Auth & Sessions</h3>
HMAC-SHA256 tokens, Redis-backed revocation, scope enforcement. Wizard has optional TOTP MFA, signed session cookies, audit trail.
</td>
<td align="center">
<h3>📊 Observability</h3>
Prometheus metrics, Grafana dashboards, 6 alert rules, structured JSON logging. SIEM-ready out of the box.
</td>
<td align="center">
<h3>🐳 One-Command Deploy</h3>
<code>docker compose up -d</code> — 5 services with health checks, resource limits, and internal-only backends. Only wizard and gateway exposed.
</td>
</tr>
<tr>
<td align="center">
<h3>📋 Incident Runbooks</h3>
6 playbooks: token compromise, injection detected, gateway down, brute force, secret rotation, disk full. Copy-paste commands.
</td>
<td align="center">
<h3>🧪 258+ Tests</h3>
Go unit and integration tests, 7 fuzz targets, coverage gate (60%) in CI. Lint, gosec, govulncheck, Docker build on every push.
</td>
<td align="center">
<h3>📐 Threat Model</h3>
STRIDE analysis with 27 documented threats, mitigations, and residual risks. Defense-in-depth, not security theater.
</td>
</tr>
</table>

<br/>

## Problems InstallerClaw solves

| Without InstallerClaw | With InstallerClaw |
|---|---|
| ❌ OpenClaw is exposed directly — no auth, no rate limiting, no scanning. | ✅ Gateway sits in front with HMAC auth, brute-force protection, and prompt-injection scanning on every request. |
| ❌ You manually configure `.env` files, hope Docker ports are right, and debug health checks. | ✅ Wizard UI walks you through setup: prerequisites, API keys, service health — all from a browser. |
| ❌ When something breaks, you dig through logs trying to figure out what happened. | ✅ Structured logging, Prometheus metrics, Grafana dashboards, and 6 incident runbooks with copy-paste commands. |
| ❌ You have no audit trail — who logged in, what changed, when services restarted. | ✅ Wizard audit log records every login, config change, restart, and token creation with timestamps and IPs. |
| ❌ Prompt injection attacks go straight to the AI with no filtering. | ✅ 14-pattern body scanner + output scanner catch common injection and exfiltration attempts before they reach OpenClaw. |
| ❌ Secret rotation means editing files, restarting services, and hoping nothing breaks. | ✅ Runbook has ordered rotation procedures; wizard UI edits `.env` and restarts services in place. |

<br/>

## What InstallerClaw is not

| | |
|---|---|
| **Not the AI itself.** | OpenClaw handles the assistant, channels, and LLM integration. InstallerClaw handles the perimeter. |
| **Not a guarantee.** | Scanning is heuristic-only (regex/patterns). It **reduces risk** but does not prevent all attacks. See [THREAT-MODEL.md](beautifulplanet/safepaw/THREAT-MODEL.md). |
| **Not enterprise-grade (yet).** | Suited for indie/small-team use. Enterprise deployments may need additional controls (WAF, external IdP, etc.). |
| **Not a cloud service.** | Fully self-hosted. No accounts, no telemetry, no external dependencies at runtime. |
| **Not a workflow builder.** | No drag-and-drop pipelines. InstallerClaw secures and operates a single AI stack. |

<br/>

<div align="center">
<table>
  <tr>
    <td align="center"><strong>Wraps</strong></td>
    <td align="center"><strong>OpenClaw</strong><br/><sub>AI Assistant</sub></td>
    <td align="center"><strong>Discord</strong><br/><sub>Channel</sub></td>
    <td align="center"><strong>Telegram</strong><br/><sub>Channel</sub></td>
    <td align="center"><strong>Slack</strong><br/><sub>Channel</sub></td>
    <td align="center"><strong>Redis</strong><br/><sub>State</sub></td>
    <td align="center"><strong>Postgres</strong><br/><sub>Config</sub></td>
  </tr>
</table>
</div>

<br/>

---

### Impact

- **Deploy in one command** — `docker compose up -d` from the [safepaw](beautifulplanet/safepaw) directory; five services (wizard, gateway, openclaw, redis, postgres) with health checks and internal-only backends.
- **All traffic through the gateway** — OpenClaw has no host-exposed ports. Auth, rate limiting, brute-force protection, and AI-defense scanning apply before any request reaches the assistant.
- **Wizard for ops** — React UI for admin login (session tokens, optional TOTP), prerequisites, live container status, and masked .env editing. Audit log for all admin actions.
- **258+ tests across gateway and wizard** — Go unit and integration tests, 7 fuzz targets, coverage gate (60%) in CI. Lint, gosec, govulncheck, Docker build on every push.
- **Operational docs** — Incident runbooks (6 types), backup/restore procedures, secret rotation, STRIDE threat model (27 threats). This readme is written for anyone running or evaluating the system.

**Stack:** Go · React 19 · TypeScript · Tailwind · Docker Compose · Redis · PostgreSQL · Prometheus · Grafana

---

### Evidence

| Claim | Proof |
|-------|--------|
| Stack runs locally | `cd beautifulplanet/safepaw && docker compose up -d` — wizard :3000, gateway :8080 |
| Gateway health | `curl -s http://localhost:8080/health` — returns status + backend reachability |
| Prometheus metrics | `curl -s http://localhost:8080/metrics` — counters, histograms, gauges |
| 258+ Go tests | `cd beautifulplanet/safepaw/services/gateway && go test ./... -race`; same under `services/wizard` |
| 7 fuzz targets | `cd beautifulplanet/safepaw && make fuzz` (gateway middleware) |
| Vulnerability scan | `cd beautifulplanet/safepaw && make vulncheck` — govulncheck on both services |
| Incident runbooks | [RUNBOOK.md](beautifulplanet/safepaw/RUNBOOK.md) — token compromise, injection, gateway down, brute force, rotation, disk |
| Backup & recovery | [BACKUP-RECOVERY.md](beautifulplanet/safepaw/BACKUP-RECOVERY.md) — Postgres, Redis, volumes, .env |
| Threat model | [THREAT-MODEL.md](beautifulplanet/safepaw/THREAT-MODEL.md) — STRIDE, 27 threats, mitigations |

---

### Quality bar

- **Defense in depth** — Request path: metrics → headers → request ID → origin check → brute-force guard → rate limit → auth (HMAC + Redis revocation) → body scanner → output scanner → proxy. Each layer documented in [SECURITY.md](beautifulplanet/safepaw/SECURITY.md).
- **STRIDE threat model** — Documented threats and mitigations; residual risks called out (e.g. heuristic-only scanning, in-memory brute-force bans).
- **Backup and secret rotation** — Procedures for Postgres, Redis, volumes, and .env; RUNBOOK includes ordered secret rotation and one-shot rotation block.
- **CI pipeline** — Build, test (`-race`), lint (golangci-lint), gosec, govulncheck, coverage gate (60%), fuzz seed corpus, Docker build.
- **Structured logging** — Gateway: JSON or text via `LOG_FORMAT`; wizard: audit log for login, config changes, restarts. SIEM-ready.

---

### How to read this README

**If you're evaluating the system**

| What you want | Where to find it | Time |
|---------------|------------------|------|
| What it is and why it exists | [What is InstallerClaw?](#what-is-installerclaw) + [Part 1: Summary](#part-1-summary) | 1 min |
| Proof (tests, metrics, docs) | [Evidence](#evidence) | 1 min |
| Security and ops posture | [Part 2: Tech stack & architecture](#part-2-tech-stack--architecture), [Security posture](#security-posture) | 2 min |
| Limitations | [Limitations](#limitations) | 30 sec |

**If you're reviewing security & operations**

| What you want | Where to find it | Time |
|---------------|------------------|------|
| Request flow and boundaries | [Architecture](#architecture), [Request flow](#request-flow) | 1 min |
| Threat model and mitigations | [THREAT-MODEL.md](beautifulplanet/safepaw/THREAT-MODEL.md), [Security posture](#security-posture) | 3 min |
| Incident and backup procedures | [RUNBOOK.md](beautifulplanet/safepaw/RUNBOOK.md), [BACKUP-RECOVERY.md](beautifulplanet/safepaw/BACKUP-RECOVERY.md) | 2 min |
| Config and env vars | [Configuration](#configuration) | 1 min |

**If you want to run it**

| What you want | Where to find it | Time |
|---------------|------------------|------|
| Clone and run in a few minutes | [Part 3: Quick start](#part-3-quick-start) | 2 min |
| Full setup and dev commands | [Development](#development), [Testing](#testing) | 5 min |
| Troubleshooting | [Troubleshooting](#troubleshooting), [Verify deployment](#verify-deployment) | 2 min |

---

# Part 1: Summary

_What this is, what it does, why it matters._

### What

SafePaw combines:

- **Go gateway** — Reverse proxy with HMAC auth, Redis-backed revocation, per-IP rate limiting, brute-force IP banning, 14-pattern prompt-injection body scanner, output scanner (XSS, secret leaks), security headers, origin validation, server-generated request IDs, optional TLS, Prometheus metrics, structured JSON logging.
- **Go + React wizard** — Single-binary setup UI (React embedded via `go:embed`): admin login with HMAC session tokens and optional TOTP, prerequisite checks (Docker, Compose, ports, disk), live container dashboard, masked .env edit, service restart, audit log.
- **Five-service Compose stack** — Wizard, gateway, OpenClaw, Redis, Postgres; only wizard and gateway exposed on the host (127.0.0.1). OpenClaw and data stores are internal only.

OpenClaw handles the AI assistant, channels (Discord, Telegram, Slack, etc.), and LLM integration. SafePaw handles the perimeter.

### Why it's interesting (for reviewers)

| Topic | Detail |
|-------|--------|
| Security layering | Defense in depth with brute-force guard fed by auth failures and rate-limit hits; Redis-backed revocation; STRIDE threat model. |
| AI defense | Heuristic body scanner (14 patterns, versioned) + response/output scanner; risk levels and `X-SafePaw-Risk` header. |
| Zero-dependency choices | Gateway: no JWT library (custom HMAC tokens), no Redis client lib (minimal RESP client), no Docker SDK (raw Engine API over Unix socket). |
| Testing | 258+ tests, 7 fuzz targets, integration tests for auth/rate-limit/revocation/scope, coverage gate and govulncheck in CI. |
| Operations | Runbooks, backup/restore, secret rotation order, Grafana alerts, structured logging. |
| Wizard UX | Session tokens + optional TOTP, audit trail, prerequisite checks, live Docker status. |

### Key numbers

| Metric | Value |
|--------|--------|
| Go tests (gateway) | 195+ (middleware, config, integration, fuzz) |
| Go tests (wizard) | 63+ (session, TOTP, middleware, API, config, audit) |
| Fuzz targets | 7 (prompt injection, sanitize, channel, output scan, token, KV) |
| CI jobs | 5 (gateway build+test, wizard build+test, lint, security, Docker) |
| Runbook playbooks | 6 (token compromise, injection, gateway down, brute force, rotation, disk) |
| Threat model entries | 27 (STRIDE, all mitigated with residual risks noted) |
| Prometheus metrics | Request counts, durations, auth failures, injections, connection gauge, path normalization |

---

# Part 2: Tech stack & architecture

### Stack

| Layer | Technology | Why |
|-------|------------|-----|
| Gateway | Go, net/http, httputil.ReverseProxy | Single binary, minimal deps (uuid only), full control over request/response path |
| Wizard backend | Go, go:embed | Serves React build + REST API; one binary, no nginx |
| Wizard frontend | React 19, TypeScript, Tailwind, Vite | Typed API client, SPA routing, modern tooling |
| Auth | HMAC-SHA256 (custom), Redis (optional) | Stateless tokens, scope enforcement, persistent revocation without JWT lib |
| Session (wizard) | HMAC-SHA256, optional TOTP (stdlib) | Signed cookies, replay-resistant (jti), optional MFA |
| Orchestration | Docker Compose | Five services, health checks, resource limits, internal network |
| Observability | Prometheus, Grafana | /metrics, dashboards, 6 alert rules; LOG_FORMAT=json for SIEM |
| Testing | go test, Vitest (wizard UI optional), fuzz | Unit, integration, fuzz; CI with race detector and coverage gate |

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Host (127.0.0.1 only)                    │
│                                                                  │
│   :3000 (Wizard)                    :8080 (Gateway)              │
│   ┌──────────────┐                  ┌──────────────────────────┐ │
│   │ Go + React   │                  │ Go reverse proxy         │ │
│   │ session+TOTP │                  │ auth · rate limit · scan │ │
│   │ audit · .env │                  │ metrics · logging        │ │
│   └──────┬───────┘                  └─────────────┬──────────────┘ │
│          │ Docker socket (ro)                     │               │
└──────────┼───────────────────────────────────────┼───────────────┘
           │                                       │
           ▼                                       ▼
┌─────────────────────── Internal network ────────────────────────┐
│  ┌─────────┐   ┌─────────┐   ┌─────────────┐   ┌──────────────┐  │
│  │ Redis   │   │Postgres │   │ OpenClaw    │   │ OpenClaw     │  │
│  │ :6379   │   │ :5432   │   │ :18789      │   │ (no host     │  │
│  │ state   │   │ config  │   │ (AI asst)   │   │  ports)      │  │
│  └─────────┘   └─────────┘   └─────────────┘   └──────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

### Request flow

```
Request → Metrics → Security headers → Request ID (server UUID only)
        → Origin check → Brute-force guard → Rate limit → Auth (HMAC + revocation)
        → Body scanner (prompt injection) → Output scanner (XSS/secrets)
        → Reverse proxy → OpenClaw
```

### Key design decisions

| Decision | Rationale |
|----------|-----------|
| OpenClaw not exposed to host | Single security perimeter; all traffic is authenticated, rate-limited, and scanned. |
| Auth and revocation in gateway | Stateless HMAC tokens; revocation in Redis so it survives restarts and is shared across instances. |
| Wizard reads Docker socket read-only | Health and container list only; no privilege escalation from UI. |
| Heuristic-only AI scanning | No ML/LLM dependency; patterns versioned and auditable; documented as a limitation. |
| Server-generated request IDs only | Client X-Request-ID ignored to avoid log injection and guarantee unique correlation IDs. |

### Security posture

| Boundary | Threat | Mitigation | Status |
|----------|--------|------------|--------|
| Gateway API | Unauthenticated access | AUTH_ENABLED=true default in Docker; HMAC tokens, scope enforcement | ✅ |
| Gateway API | Token reuse after compromise | Redis-backed revocation; POST /admin/revoke | ✅ |
| Gateway API | Brute force / abuse | Rate limit + brute-force guard (IP ban after N failures) | ✅ |
| Gateway API | Prompt injection / exfil | Body scanner (14 patterns) + output scanner | ✅ Heuristic |
| Gateway API | Spoofed identity when auth off | StripAuthHeaders removes X-Auth-Subject/Scope | ✅ |
| Wizard admin | Weak or missing password | Auto-generated secret if unset; strong password in .env recommended | ✅ |
| Wizard admin | Session replay | HMAC session tokens with jti (nonce) | ✅ |
| Wizard admin | Credential stuffing | Optional TOTP (WIZARD_TOTP_SECRET); rate limit + delay on fail | ✅ |
| Config | Secret leak in UI | .env keys masked in GET; PUT restricted to allowed list | ✅ |
| Operations | Data loss | BACKUP-RECOVERY.md; RUNBOOK secret rotation | ✅ Documented |

Full threat model: [THREAT-MODEL.md](beautifulplanet/safepaw/THREAT-MODEL.md). Incident response: [RUNBOOK.md](beautifulplanet/safepaw/RUNBOOK.md).

---

# Part 3: Quick start

### Prerequisites

- Docker and Docker Compose (v2)
- Ports 3000 (wizard) and 8080 (gateway) free
- For production: set `AUTH_ENABLED=true`, `AUTH_SECRET`, and optionally `TLS_ENABLED` with certs

### Quickstart

```bash
# Clone
git clone https://github.com/beautifulplanet/SafePaw.git
cd SafePaw/beautifulplanet/safepaw

# Configure
cp .env.example .env
# Edit .env: API keys, channel tokens, AUTH_SECRET if using auth

# Launch
docker compose up -d

# Access
# Wizard:  http://localhost:3000   (password in docker compose logs wizard or set WIZARD_ADMIN_PASSWORD in .env)
# Gateway: http://localhost:8080  (proxies to OpenClaw)
```

First run: wizard prints an admin password once to stdout. Use `docker compose logs wizard` or set `WIZARD_ADMIN_PASSWORD` in `.env`.

---

# Part 4: Configuration, development, and reference

## Configuration

All configuration via environment variables (e.g. `.env` in the safepaw directory).

### Essential

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key for OpenClaw |
| `OPENAI_API_KEY` | OpenAI API key (optional) |

### Channel tokens (optional)

| Variable | Description |
|----------|-------------|
| `DISCORD_BOT_TOKEN`, `TELEGRAM_BOT_TOKEN`, `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` | Channel bot tokens |

### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_ENABLED` | `true` (Docker) | Enable gateway HMAC auth |
| `AUTH_SECRET` | — | HMAC signing key (min 32 bytes) |
| `TLS_ENABLED` | `false` | Enable TLS on gateway |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | /certs/... | TLS cert and key paths |
| `RATE_LIMIT` | 60 | Requests per minute per IP |
| `LOG_FORMAT` | `text` | `json` for SIEM-style logs |
| `WIZARD_ADMIN_PASSWORD` | auto-generated | Wizard admin password |
| `WIZARD_TOTP_SECRET` | — | Optional base32 TOTP for wizard MFA |

See [.env.example](beautifulplanet/safepaw/.env.example) for the full list.

## Development

```bash
# Gateway
cd beautifulplanet/safepaw/services/gateway
go build -o gateway .
PROXY_TARGET=http://localhost:18789 ./gateway

# Token (when auth enabled)
export AUTH_SECRET=$(openssl rand -base64 48)
go run tools/tokengen/main.go -sub admin -scope proxy -ttl 24h

# Wizard
cd beautifulplanet/safepaw/services/wizard
go build -o wizard ./cmd/wizard
WIZARD_ADMIN_PASSWORD=dev ./wizard

# Wizard UI (hot reload)
cd beautifulplanet/safepaw/services/wizard/ui
npm install && npm run dev
```

From safepaw root: `make lint`, `make vulncheck`, `make fuzz`. See [CONTRIBUTING.md](beautifulplanet/safepaw/CONTRIBUTING.md).

## Testing

| Suite | Location | Command |
|-------|----------|---------|
| Gateway | `services/gateway` | `go test ./... -race` |
| Wizard | `services/wizard` | `go test ./... -race` |
| Fuzz (gateway) | `services/gateway` | `go test -fuzz=...` or `make fuzz` |
| Vuln check | safepaw root | `make vulncheck` |
| E2E (live stack) | safepaw/scripts | `./verify-deployment.sh` (after `docker compose up -d`) |

CI runs build, test with coverage gate (gateway 60%, wizard 55%), lint, gosec, govulncheck, and Docker build.

## Project structure

> **Layout note:** Code lives under `beautifulplanet/safepaw/` (org/project namespace). A `go.work` workspace file at that level ties the Go modules together so standard tooling (`gopls`, `go test ./...`, etc.) works from the project root. CI and Dockerfiles reference paths relative to this directory.

```
beautifulplanet/safepaw/
├── docker-compose.yml       # 5 services, health checks, resource limits
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

## Troubleshooting

| Issue | What to do |
|-------|------------|
| Lost wizard admin password | `docker compose logs wizard` (first lines) or set `WIZARD_ADMIN_PASSWORD` in `.env` and restart. [SECURITY.md](beautifulplanet/safepaw/SECURITY.md) § Recovery. |
| Prerequisites fail | Install Docker and Compose; ensure 3000 and 8080 are free. |
| Dashboard shows no services | Wizard needs read-only Docker socket; check compose mount for `/var/run/docker.sock` (or npipe on Windows). |
| Gateway 502 / backend unreachable | OpenClaw may still be starting. `docker compose logs openclaw`; `curl http://localhost:8080/health`. |
| Auth required | Set `AUTH_ENABLED=true` and `AUTH_SECRET`; use `tools/tokengen` to create tokens. |

## Verify deployment

```bash
curl -s http://localhost:3000/api/v1/health | jq .
curl -s http://localhost:8080/health | jq .
```

Then open http://localhost:3000, sign in, and check the dashboard. Full script: `scripts/verify-deployment.sh` in the safepaw directory.

## FAQ

**What does a typical setup look like?**
Clone the repo, copy `.env.example` to `.env`, fill in your API keys, run `docker compose up -d`. The wizard at `:3000` guides you through the rest.

**Can I use this without OpenClaw?**
The gateway can proxy to any HTTP backend — change `PROXY_TARGET` in `.env`. The wizard's health checks and service management are OpenClaw-specific, but the gateway is generic.

**How do I add TOTP (MFA) to the wizard?**
Set `WIZARD_TOTP_SECRET` in `.env` to a base32-encoded secret (e.g. from Google Authenticator setup). The login page will prompt for a TOTP code automatically.

**Is this production-ready?**
For indie/small-team deployments behind localhost or a VPN, yes. For public-facing production, add TLS (`TLS_ENABLED=true`), set strong `AUTH_SECRET`, and review [SECURITY.md](beautifulplanet/safepaw/SECURITY.md) and the [Production Hardening Checklist](#production-hardening-checklist) in the safepaw README.

**What if scanning misses an attack?**
It will. Scanning is heuristic-only and documented as such. Treat it as one layer of defense-in-depth, not a guarantee. See [THREAT-MODEL.md](beautifulplanet/safepaw/THREAT-MODEL.md) for residual risks.

## Limitations

- **Prompt-injection and output scanning** — Heuristic (regex/patterns) only; **reduces risk**, does not guarantee prevention. No ML/LLM. See [SECURITY.md](beautifulplanet/safepaw/SECURITY.md).
- **Token revocation** — Redis-backed when Redis is configured; in-memory fallback. Brute-force bans are in-memory only.
- **Wizard password** — No "forgot password" flow; recovery via logs or `.env` and restart.
- **Stack** — Docker-first; wizard expects Docker socket for health. No generic bare-metal install path.

**Release / packaging:** No versioned releases or installers are published yet. When going public: plan for versioned tags (e.g. `v0.1.0`), checksums for binaries, and optionally a single-command installer or `docker compose` image pinning. See [CONTRIBUTING.md](beautifulplanet/safepaw/CONTRIBUTING.md) for build and test commands.

---

## Documentation index

| Document | Purpose |
|----------|---------|
| [README.md](README.md) | This file — summary through reference |
| [SECURITY.md](beautifulplanet/safepaw/SECURITY.md) | Incident response, logging, defense-in-depth, MFA, recovery |
| [RUNBOOK.md](beautifulplanet/safepaw/RUNBOOK.md) | 6 incident playbooks, secret rotation |
| [BACKUP-RECOVERY.md](beautifulplanet/safepaw/BACKUP-RECOVERY.md) | Backup/restore for Postgres, Redis, volumes, .env |
| [THREAT-MODEL.md](beautifulplanet/safepaw/THREAT-MODEL.md) | STRIDE threat model, residual risks |
| [SCOPE-IMPROVEMENTS.md](beautifulplanet/safepaw/SCOPE-IMPROVEMENTS.md) | Review feedback triage (done vs. open) and improvement backlog |
| [CONTRIBUTING.md](beautifulplanet/safepaw/CONTRIBUTING.md) | Dev workflow, coding standards, PR process |
| [RELEASE.md](beautifulplanet/safepaw/RELEASE.md) | Going public: timestamp, checklist, positioning, licensing |
| [.env.example](beautifulplanet/safepaw/.env.example) | All configuration variables with comments |

---

## License

MIT
