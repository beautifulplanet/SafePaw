<div align="center">

<img src="beautifulplanet/android-chrome-192x192.png" alt="InstallerClaw" width="80" />

# InstallerClaw

**The security perimeter for self-hosted AI assistants.**

No cloud. No accounts. No telemetry. **We don't collect your data — at all.** You can scan the repo to verify; see [docs/NO-DATA-COLLECTION.md](docs/NO-DATA-COLLECTION.md). Just a gateway, a wizard, and 534 tests.

[![tests](https://img.shields.io/badge/tests-534-blue)](services/) [![coverage-gw](https://img.shields.io/badge/gateway_coverage-80%25-brightgreen)](#evidence) [![coverage-wiz](https://img.shields.io/badge/wizard_coverage-64%25-green)](#evidence) [![fuzz](https://img.shields.io/badge/fuzz_targets-7-blueviolet)](#evidence) [![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev) [![License: MIT](https://img.shields.io/badge/license-MIT-yellow)](LICENSE)

</div>

---

## What is this?

InstallerClaw wraps [OpenClaw](https://github.com/nicepkg/openclaw) (or any HTTP backend) in a hardened local perimeter: a **Go reverse-proxy gateway** with auth, rate limiting, and prompt-injection scanning, plus a **React wizard** for setup, monitoring, and admin. Everything runs in Docker Compose on your machine. Nothing phones home.

**Think of it as a firewall for your AI stack** — if you self-host an assistant and don't want it naked on the network, this is the missing layer.

## Who needs this?

| You are… | Your problem | InstallerClaw gives you |
|----------|-------------|----------------------|
| **Tinkerer** who just got OpenClaw running | It's exposed on a port with no auth, no rate limiting, no scanning | Gateway with HMAC auth, brute-force protection, 14-pattern prompt-injection scanner |
| **Small-team admin** sharing an AI assistant | No audit trail, no MFA, users share one password | Wizard with session tokens, optional TOTP, per-action audit log |
| **Security-conscious dev** who reads threat models | You want proof, not promises | 48 STRIDE threats documented, 534 tests, 7 fuzz targets, govulncheck in CI |
| **Ops person** who gets paged at 3 AM | Something broke, no runbook | 6 incident playbooks, backup/restore procedures, Grafana dashboards |

## What you get

| Layer | Highlights |
|-------|-----------|
| **Gateway** | HMAC-SHA256 auth · Redis-backed revocation · per-IP rate limits · brute-force IP bans · 14-pattern body scanner · output scanner (XSS, secret leaks) · Prometheus metrics · structured JSON logging |
| **Wizard** | React 19 SPA embedded via `go:embed` · admin login with optional TOTP · prerequisite checks · live container dashboard · masked `.env` editing · audit log |
| **Compose stack** | 5 services (wizard, gateway, OpenClaw, Redis, Postgres) · health checks · resource limits · only wizard + gateway exposed on `127.0.0.1` |
| **Ops docs** | STRIDE threat model (48 threats) · 6 incident runbooks · backup/restore · secret rotation · Grafana alerts |
| **CI pipeline** | Build · test (`-race`) · lint · gosec · govulncheck · fuzz seeds · coverage gates · Docker build |

---

## Quick start

**New here? Try the demo in 3 steps:** (1) Clone the repo, (2) run **LAUNCH.bat** (Windows) or **./LAUNCH.sh** (Mac/Linux), (3) press **2** for Demo → browser opens at http://localhost:3000. Sign in with the password shown in the launcher window. No API keys or OpenClaw required. [Full steps →](docs/HOW-TO-TEST-AND-WHERE-WE-ARE.md)

```bash
git clone https://github.com/beautifulplanet/SafePaw.git
cd SafePaw
# Windows: LAUNCH.bat  |  macOS/Linux: ./LAUNCH.sh  → press 2 for Demo
./LAUNCH.sh
```

**Windows:** Use **LAUNCH.bat** for a one-click menu (Full / Demo / Shut down / Show processes). Use **STOP-SAFEPAW.bat** for an emergency stop (can be shortcut on desktop).  
**macOS/Linux:** Use **LAUNCH.sh** for the same menu (`./LAUNCH.sh`). See `docs/VERIFY-LAUNCHER.md` to verify.

**First time testing or demoing?** See [docs/HOW-TO-TEST-AND-WHERE-WE-ARE.md](docs/HOW-TO-TEST-AND-WHERE-WE-ARE.md) for a minimal test path and what “working product” means.

That's it. The script checks Docker, generates secrets, picks a memory profile for your system, starts everything, and opens your browser at `http://localhost:3000`.

### What happens

1. `start.sh` generates `.env` with secure random passwords (Redis, Postgres, auth secret, wizard password)
2. Detects RAM → sets `SYSTEM_PROFILE` (small / medium / large / very-large)
3. Runs `docker compose up -d --build`
4. Waits for health checks, prints your wizard password, opens the wizard

**Prerequisites:** Docker + Compose V2 · ports 3000 and 8080 free

### Verify it works

```bash
curl -s http://localhost:3000/api/v1/health | jq .   # wizard
curl -s http://localhost:8080/health | jq .           # gateway
```

Open http://localhost:3000, sign in, check the dashboard. Full verification script: `./scripts/verify-deployment.sh`.

---

## Evidence

Every number in this README is provable. Run the commands yourself.

| Claim | How to verify |
|-------|--------------|
| 534 Go tests (346 gateway + 188 wizard) | `grep -rE '^func Test' services/gateway --include='*.go' | wc -l` (gateway); same path for wizard. Sum = 534. |
| 7 fuzz targets | `make fuzz` or `grep -r "^func Fuzz" services/gateway/` |
| Gateway coverage 80.5% (CI gate: >65%) | `cd services/gateway && go test -coverprofile=c.out ./... && go tool cover -func=c.out \| grep total` |
| Wizard coverage 64.2% (CI gate: ≥60%) | Same command under `services/wizard` |
| 48 STRIDE threats modeled | `grep -cE "^\| [A-Z][0-9]" THREAT-MODEL.md` |
| 6 incident runbooks | `grep -c "^## INC-" RUNBOOK.md` |
| Zero CVEs in deps | `make vulncheck` (runs govulncheck on both services) |
| Stack runs entirely local | `docker compose ps` — no outbound connections except your LLM API key |

---

## How the pieces fit

```
Internet ─→ [Gateway :8080] ─→ [OpenClaw :18789]
                │                       │
            Auth · Rate Limit      AI Assistant
            Scanner · Metrics      Channels · LLM
                │
            [Redis :6379]          [Postgres :5432]
             Revocation              Config · Audit
             Brute-force

Browser ──→ [Wizard :3000]
             Setup · Dashboard
             .env · TOTP · Audit
```

Only the gateway and wizard are exposed on `127.0.0.1`. OpenClaw, Redis, and Postgres live on an internal Docker network with no host ports. Full architecture with mermaid diagrams: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## Documentation

Everything deep lives in its own doc. The README is the routing table.

### Architecture & design

| Document | What's in it |
|----------|-------------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Full architecture — mermaid diagrams, middleware pipeline, auth flows, WebSocket proxy, scanning, deployment topology, technology decisions |
| [docs/adr/](docs/adr/) | 9 architecture decision records — HMAC over JWT, zero deps, Go choice, socket proxy, heuristic scanning, embedded UI, receipt ledger, CSRF, Codespaces routing |

### Security & threats

| Document | What's in it |
|----------|-------------|
| [SECURITY.md](SECURITY.md) | Defense-in-depth walkthrough, incident response, hardening checklist, MFA setup, logging for SIEM, vulnerability management, password recovery |
| [THREAT-MODEL.md](THREAT-MODEL.md) | STRIDE analysis — 48 threats across 6 categories, mitigations, residual risks, trust boundaries |
| [docs/PENTEST-POLICY.md](docs/PENTEST-POLICY.md) | Penetration testing scope, methodology, responsible disclosure |

### Operations

| Document | What's in it |
|----------|-------------|
| [docs/PHONE-ACCESS.md](docs/PHONE-ACCESS.md) | Let people use their phone with OpenClaw — expose gateway, auth, tokens, CORS, TLS |
| [RUNBOOK.md](RUNBOOK.md) | 6 incident playbooks — token compromise, injection detected, gateway down, brute force, secret rotation, disk full |
| [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md) | Backup and restore for Postgres, Redis, Docker volumes, `.env` |
| [docs/SECRETS-MIGRATION.md](docs/SECRETS-MIGRATION.md) | Migration guide from env vars to Vault / external secrets |

### Development & configuration

| Document | What's in it |
|----------|-------------|
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Configuration reference, build commands, testing guide, project structure, troubleshooting |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Dev workflow, coding standards, PR process, pre-commit hooks |
| [PITFALLS.md](PITFALLS.md) | Known gotchas and edge cases |
| [CHANGELOG.md](CHANGELOG.md) | Release history (Keep a Changelog format) |

### Compliance & policy

| Document | What's in it |
|----------|-------------|
| [docs/COMPLIANCE.md](docs/COMPLIANCE.md) | SOC 2 & GDPR control mapping with gap analysis |
| [docs/PATCHING-POLICY.md](docs/PATCHING-POLICY.md) | Dependency update SLAs, Dependabot workflow, freeze policy |

### Scope & delivery

| Document | What's in it |
|----------|-------------|
| [docs/scope/SOW-001.md](docs/scope/SOW-001.md) | Original statement of work — all items delivered |
| [docs/scope/CO-001.md](docs/scope/CO-001.md) | Change order: per-endpoint rate limits — delivered |
| [docs/scope/CO-002.md](docs/scope/CO-002.md) | Change order: Playwright E2E login tests — delivered |
| [SCOPE-IMPROVEMENTS.md](SCOPE-IMPROVEMENTS.md) | Review feedback triage and improvement backlog |

---

## FAQ

**Can I use this without OpenClaw?**
Yes. The gateway proxies to any HTTP backend — change `PROXY_TARGET` in `.env`. The wizard's container dashboard is OpenClaw-aware, but the gateway is generic.

**How do I add MFA to the wizard?**
Set `WIZARD_TOTP_SECRET` in `.env` to a base32 secret. The login page will prompt for a TOTP code automatically. Details in [SECURITY.md](SECURITY.md).

**Is this production-ready?**
For localhost or VPN deployments, yes. For public-facing setups, enable TLS, set a strong `AUTH_SECRET`, and work through the hardening checklist in [SECURITY.md](SECURITY.md).

**How does the scanner work?**
Heuristic pattern-matching — 14 patterns for prompt injection, plus an output scanner for XSS and secret leaks. It's one layer of defense-in-depth, not a silver bullet. Design rationale: [ADR-005](docs/adr/005-heuristic-scanning-not-ml.md).

**Where's the config reference?**
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) has the full environment variable table. Quick version: copy `.env.example` and fill in your API keys.

---

## Design boundaries

InstallerClaw secures a single AI stack with depth, not breadth:

- **Scanning is heuristic.** Pattern-based tripwires, not ML classifiers. Documented as a scope boundary in [THREAT-MODEL.md](THREAT-MODEL.md).
- **Single-instance.** Designed for indie and small-team deployments. Enterprise can layer WAF and external IdP on top.
- **Local-first.** No cloud dependencies at runtime. Your data stays on your machine.
- **No data collection.** No analytics, no tracking, no phone-home. We don't have your data because we never collect it. [How to verify →](docs/NO-DATA-COLLECTION.md)

---

## License

MIT — see [LICENSE](LICENSE).

## Acknowledgements

Built in collaboration with [Claude](https://claude.ai/) (Opus) by [Anthropic](https://www.anthropic.com/). Architecture decisions, security hardening, and all review by [@beautifulplanet](https://github.com/beautifulplanet).
