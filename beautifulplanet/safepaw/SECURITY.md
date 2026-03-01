# SafePaw Security

Security operations, incident response, and hardening guidance for SafePaw (NOPEnclaw).

**Related documents:**
- [RUNBOOK.md](RUNBOOK.md) — Operational playbooks for 6 incident types (token compromise, injection, gateway down, brute force, rotation, disk)
- [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md) — Backup and restore procedures for Postgres, Redis, and volumes
- [THREAT-MODEL.md](THREAT-MODEL.md) — STRIDE threat analysis (27 identified threats, all mitigated)
- [SCOPE-IMPROVEMENTS.md](SCOPE-IMPROVEMENTS.md) — Triage of review feedback (what’s done vs. open) and prioritized improvement backlog
- [CONTRIBUTING.md](CONTRIBUTING.md) — Development workflow, coding standards, security review checklist

---

## 1. Incident Response

### Getting the request ID

Every gateway request gets a unique `X-Request-ID` (UUID). Use it to correlate logs across the stack:

- **Gateway:** Log lines include `request_id=` when available (see [Logging](#2-logging)).
- **Client:** If you log the `X-Request-ID` response header from the gateway, you can search logs for that ID.

### Security events to act on

| Event | Log prefix / location | Action |
|-------|------------------------|--------|
| Auth failure (invalid/expired token) | `[AUTH] Rejected` | Check for token leak or misconfiguration; consider revoking token (Phase 2). |
| Auth failure (wrong password) | Wizard: `[WARN] Failed login attempt from <ip>` | Possible brute force; ensure rate limiting and consider blocking IP. |
| Rate limit hit | `[RATELIMIT] DENIED` | Normal under load or abuse; tune `RATE_LIMIT` or add IP allow/block. |
| Prompt injection attempt | `[SCANNER] Prompt injection risk=...` | Request still proxied with `X-SafePaw-Risk`; consider blocking `high` or alerting. |
| Origin rejected | `[SECURITY] Blocked request from unauthorized Origin` | CSRF/CSWSH style request; no further action unless repeated. |
| Backend unreachable | `[PROXY] Backend error` / health `degraded` | Check OpenClaw/network; gateway is up but backend is down. |

### If the gateway is compromised

1. Rotate `AUTH_SECRET` and re-issue all tokens.
2. Enable revocation (Phase 2) and revoke compromised tokens.
3. Rotate Wizard admin password (`WIZARD_ADMIN_PASSWORD`).
4. Review gateway and wizard logs for suspicious requests (search for `[AUTH]`, `[SCANNER]`, `[RATELIMIT]`).

### If OpenClaw is exposed without the gateway

By design, **OpenClaw has no host-exposed ports**. It only listens on the internal Docker network. If the gateway container stops, OpenClaw is still not reachable from the host or internet. The only way to reach OpenClaw is through the gateway. See [Defense in depth](#3-defense-in-depth).

---

## 2. Logging

Security-relevant log lines are structured with a tag prefix for grep:

| Component | Prefixes | What’s logged |
|-----------|----------|----------------|
| Gateway | `[AUTH]` | Token validation success/failure, scope rejection, optional-auth invalid token. |
| Gateway | `[RATELIMIT]` | Allow/deny per IP; denial includes count. |
| Gateway | `[SCANNER]` | Prompt injection risk level, triggers, path, remote IP, body length. |
| Gateway | `[SECURITY]` | Origin blocked, security headers applied, IP bans (brute force). |
| Gateway | `[PROXY]` | Backend errors (with path and remote). |
| Gateway | `[WS]` | WebSocket upgrade, tunnel established/closed, errors. |
| Wizard | `[AUDIT]` | All admin actions: login (success/failure), config changes, service restarts. |
| Wizard | `[WARN] Failed login attempt from <ip>` | Failed admin login (IP for correlation). |

All gateway logs should include `request_id=` where the request ID is available (see [Request ID](#request-id) below) so you can trace a request across layers.

### Structured JSON Logging (SIEM Integration)

Set `LOG_FORMAT=json` to convert all gateway logs to JSON for SIEM ingestion (Splunk, Datadog, ELK):

```json
{"ts":"2024-01-01T00:00:00Z","level":"warn","service":"safepaw-gateway","component":"AUTH","msg":"Rejected: bad token","fields":{"remote":"10.0.0.1","request_id":"abc-123"}}
```

The JSON adapter automatically parses SafePaw's log prefix convention (`[AUTH]`, `[SCANNER]`, etc.) and extracts key-value fields from log messages.

### Request ID

The gateway sets `X-Request-ID` on every request (or preserves the one from the client). This ID is passed along the middleware chain and should be included in security and proxy log lines for incident response.

---

## 3. Defense in Depth

- **OpenClaw is not exposed.** It has no `ports:` on the host; only the gateway and wizard have host ports (and only on `127.0.0.1`). If the gateway fails, OpenClaw does not become reachable from outside.
- **Layers:** Security headers → Request ID → Origin check → Rate limit → Auth (if enabled) → Body scanner → Proxy. A failure in one layer does not bypass the others.
- **Wizard:** Only listens on localhost; admin auth and rate limiting protect the setup UI.

---

## 4. Revocation (Phase 2 — Complete)

Token revocation is now implemented as an in-memory revocation list in the gateway:

- **How it works:** Admin calls `POST /admin/revoke` with `{"subject":"user_id","reason":"compromised"}`. The gateway records the revocation timestamp and rejects all tokens for that subject issued before the revocation time.
- **Scope:** Revocation is subject-level (all tokens for a user), not per-token. This is deliberate — if credentials are compromised, all tokens should be invalid.
- **Persistence:** In-memory only. Revocation entries auto-expire after `AUTH_MAX_TTL` (default 7 days). Entries are lost on gateway restart, which is acceptable for short-lived tokens. Redis/Postgres-backed persistence is future work.
- **Admin endpoint:** `POST /admin/revoke` requires an `admin`-scoped auth token.

**Emergency revocation steps:**
1. Generate an admin token: `go run tools/tokengen/main.go -sub admin -scope admin -ttl 1h`
2. Revoke: `curl -X POST -H "Authorization: Bearer <admin-token>" -d '{"subject":"compromised_user","reason":"leaked"}' http://gateway:8080/admin/revoke`
3. For full rotation: change `AUTH_SECRET` and restart the gateway (invalidates ALL tokens).

---

## 5. Monitoring and Alerting

- **Health dashboard:** The wizard shows service status (healthy/degraded/down). Use it for quick checks; for production, treat it as a view, not the only alert source.
- **Actionable alerts:** Configure alerts on:
  - Gateway health check failing (e.g. `/health` returns 5xx or degraded).
  - OpenClaw (backend) unreachable from gateway.
  - Repeated auth failures or rate-limit denials from the same IP.
  - Log volume or pattern matching `[SCANNER] Prompt injection risk=high` if you decide to treat high risk as an alert.
- **Prometheus/Grafana:** The gateway exposes a `/metrics` endpoint in Prometheus text exposition format. Metrics include:
  - `safepaw_requests_total` — total HTTP requests by method, status, path
  - `safepaw_request_duration_seconds` — request duration histogram
  - `safepaw_prompt_injection_detected_total` — prompt injection detections by risk level
  - `safepaw_tokens_revoked_total` — token revocations
  - `safepaw_rate_limited_total` — rate-limited requests
  - `safepaw_auth_failures_total` — authentication failures by reason
  - `safepaw_active_connections` — current active connections
  
  Scrape with Prometheus (`scrape_configs: [{job_name: safepaw, static_configs: [{targets: ['gateway:8080']}]}]`) and build dashboards/alerts in Grafana.

---

## 6. Prompt Injection and Pattern Updates

- The body scanner uses a fixed set of heuristic patterns (see README “AI Defense Patterns”). New attack vectors can appear over time.
- **Practice:** Review and update patterns in `gateway/middleware/sanitize.go` when new prompt-injection or jailbreak techniques are published; consider a short “Security” section in release notes.
- **ML-based detection:** Not implemented. Consider ML-based anomaly detection as a future enhancement for unknown attack vectors; the current pattern set remains the first line of defense.

---

## 7. Automated Testing

- **Gateway tests (195 tests):** Comprehensive suite covering:
  - Body scanner: prompt injection risk levels (none/low/medium/high), pattern triggers, content type validation, channel validation, metadata sanitization, XSS stripping, control character handling
  - Auth middleware: token creation/validation, expiry, wrong secret, scope enforcement, admin bypass, query param vs Bearer header, optional auth
  - Token revocation: revoke and check, different subjects, revoked token rejection, post-revocation token acceptance
  - Security middleware: headers, origin check, rate limiter, request ID, auth header stripping, IP extraction
  - Output scanner: script tag, iframe, event handler, javascript URI, API key leak, system prompt leak, HTTP middleware, binary passthrough, WebSocket stream scanning
  - Brute-force guard: ban threshold, different IPs, reset, escalation, middleware allow/block
  - Integration/E2E: scope enforcement (proxy→admin blocked, admin→proxy allowed, unknown scope rejected), full chain (rate limit → auth → ban), identity spoofing prevention (StripAuthHeaders)
  - Structured logging: JSON/text output, level inference, KV extraction, component binding, security/audit events
  - Prometheus metrics: recording, serving, connection gauge, path normalization
  - Config loading: defaults, custom port, invalid target, auth enabled/disabled, TLS, origins, rate limit, env helpers
  - **7 fuzz targets** (46 seed corpus entries): prompt injection, XSS sanitize, control chars, channel validation, output scan, token create/validate, KV parser
- **Wizard tests (55+ tests):** Session tokens (JTI nonce), TOTP/MFA (RFC vector, clock skew), middleware (auth 9 cases, CORS 5, rate limit 4), API (health, login with/without MFA, prerequisites, config, restart, SPA fallback), config loading, audit logging
- **Total: 250 tests + 7 fuzz targets across the project**
- **E2E:** `scripts/verify-deployment.sh` validates a live deployment (Docker containers, health endpoints, security headers, rate limiting, body scanner, auth, metrics)
- **CI pipeline (GitHub Actions):** 5 parallel jobs on every push/PR:
  1. **Gateway build + test** — `go test -race` with coverage reporting + coverage threshold (>60%) + fuzz seed corpus
  2. **Wizard build + test** — `go test -race` with coverage reporting + coverage threshold (>60%)
  3. **Lint** — `golangci-lint` (errcheck, govet, staticcheck, gosimple, bodyclose, gosec, prealloc, misspell, gofmt)
  4. **Security scan** — `gosec` + `govulncheck` (known CVE detection in dependencies)
  5. **Docker build** — verifies both images build successfully
- **Makefile:** `make all` runs build + test + lint; `make fuzz` runs fuzz targets; `make vulncheck` runs govulncheck

---

## 8. Documentation

- **Security config:** README and `.env.example` cover `AUTH_ENABLED`, `AUTH_SECRET`, `TLS_*`, `RATE_LIMIT`, `WIZARD_ADMIN_PASSWORD`, `WIZARD_TOTP_SECRET` (optional MFA). Keep these in sync with code defaults.
- **Incident response:** This document (SECURITY.md) is the source for incident response and logging; update it when adding new security features or log formats.
- **First-run password:** README and Quick Start should state that the wizard prints the auto-generated admin password once to stdout and that it can be retrieved with `docker compose logs wizard` (or `docker logs safepaw-wizard`).

### Recovery: lost wizard admin password

If you never saved the auto-generated password and can’t find it in logs:

1. **Retrieve from container logs:** `docker compose logs wizard` or `docker logs safepaw-wizard` (scroll to first startup; the password is printed once).
2. **Set a new password:** Add or edit `WIZARD_ADMIN_PASSWORD=<your-password>` in `.env`, then restart the wizard: `docker compose restart wizard`. Use the new password on the next login.

There is no “forgot password” flow that bypasses the container; this is intentional (no recovery endpoint that could be abused).

---

## 9. User Experience (Wizard)

The setup wizard should guide users toward secure defaults:

- **Strong admin password:** Prefer setting `WIZARD_ADMIN_PASSWORD` in `.env` to a strong value instead of relying on the one-time auto-generated password (which is only in logs).
- **Optional MFA (TOTP):** Set `WIZARD_TOTP_SECRET` in `.env` to a base32 TOTP secret; login will then require password + 6-digit code from an authenticator app. See `.env.example` for generation hints.
- **Enable security layers:** In production, set `AUTH_ENABLED=true`, provide `AUTH_SECRET`, and enable `TLS_ENABLED` with valid certs.
- **Rate limiting:** Document that `RATE_LIMIT` controls gateway request rate per IP and that it can be tuned for abuse protection.

---

## 10. Known limitations and adoption considerations

These limitations are by design or deferred; they affect risk and adoption and should be understood before deployment.

| Limitation | Description | Mitigation / future work |
|------------|-------------|---------------------------|
| **Heuristic-only prompt injection** | The body scanner uses fixed regex/heuristic patterns (see `gateway/middleware/sanitize.go`). It catches many known attacks but **cannot** catch novel, obfuscated, or adversarial prompt-injection variants. No LLM- or ML-based detection is implemented. | Treat as one layer of defense. Update patterns when new techniques are published. Consider LLM/ML-based detection for high-assurance deployments (future work). |
| **Output scanning is heuristic** | Gateway scans HTTP responses and WebSocket streams for XSS, secret leaks, and data exfiltration using regex patterns. Like input scanning, this is heuristic and cannot catch all novel attacks. | Defense-in-depth layer. High-risk output is automatically sanitized. Consider ML-based output analysis for high-assurance deployments. |
| **In-memory revocation list** | Token revocation is in-memory and lost on gateway restart. Acceptable for short-lived tokens but not for long-lived deployments without restart. | Use short TTLs. For persistence, future work will add Redis/Postgres-backed revocation. Emergency: rotate `AUTH_SECRET` to invalidate all tokens. |
| **One-time wizard password** | The wizard prints the auto-generated admin password once to stdout. If logs are lost or missed, the user must use container logs or set `WIZARD_ADMIN_PASSWORD` in `.env` and restart. | See [Recovery: lost wizard admin password](#recovery-lost-wizard-admin-password) above. Prefer setting `WIZARD_ADMIN_PASSWORD` in `.env` before first run. |
| **Docker and socket dependency** | The wizard expects Docker and read-only access to the Docker socket for health and service listing. Version or permission issues can break the dashboard. | Ensure Docker and Compose are installed and the socket is mounted correctly; see Troubleshooting in the README. |
| **Tight coupling** | Stack is Go + React + Docker + Postgres + Redis. No pluggable security modules or non-Docker deployment path. | Acceptable for the current “one-click Docker” goal; bare-metal or alternate backends would require code changes. |
| **E2E tests are script-based** | Integration tests run via scripts/verify-deployment.sh against a live deployment; not yet integrated into CI. | Run the verification script after `docker compose up -d`; integrate into CI pipeline when Docker-in-Docker is available. |

---

## 11. Vulnerability management

SafePaw uses automated dependency and code scanning to find known vulnerabilities. This section defines how often we run checks, who acts on findings, and target response times.

### Scanning

| Tool | When it runs | What it covers |
|------|----------------|----------------|
| **govulncheck** | Every push/PR (CI), and on demand via `make vulncheck` | Go modules: known CVEs in direct and indirect dependencies |
| **gosec** | Every push/PR (CI) | Go code: hardcoded secrets, weak crypto, unsafe patterns |
| **golangci-lint** | Every push/PR (CI) | Go code: errcheck, govet, staticcheck, and other linters |

Before a release, run `make vulncheck` and `make lint` from the safepaw root and fix or document any new findings.

### Who acts on findings

- **CI failures:** The person who opened the PR fixes or mitigates before merge. If a CVE has no fix yet, add a tracked issue and document the risk in the PR or in SECURITY.md.
- **Post-merge / periodic:** Maintainers review govulncheck and gosec output; critical/high issues are triaged within the SLAs below.

### Response SLAs

| Severity | Definition | Target action |
|----------|------------|----------------|
| **Critical** | Exploitable remote code execution, auth bypass, or data exfiltration in a dependency we use | Fix or mitigate within **7 days**. If no patch exists, document workaround or consider temporary pin/removal. |
| **High** | Significant impact (e.g. DoS, privilege escalation) in a dependency we use | Fix or mitigate within **14 days**. Document in issue or SECURITY.md if deferred. |
| **Medium / Low** | Other CVEs or gosec findings | Address in next release or within **30 days**. |

“Mitigate” can mean: upgrade the dependency, replace the dependency, disable the affected code path, or accept risk with explicit documentation. Document the decision (e.g. in an issue or ADR).

### Where to record

- **Open issues** for each CVE or finding that is not fixed immediately, with a label or tag for severity.
- **SECURITY.md** or release notes when a vulnerability is fixed in a release so users can upgrade.

---

## CTO / Red Team Feedback Mapping

| Feedback | Where it’s addressed |
|----------|----------------------|
| Revocation list | §4 Revocation (Phase 2 — complete); in-memory revocation list with admin endpoint. |
| Monitoring & alerting | §5 Monitoring and Alerting; Prometheus `/metrics` endpoint implemented. |
| Logging | §2 Logging; request ID and event table. |
| Defense in depth | §3; OpenClaw not exposed when gateway fails. Output scanning added. |
| Prompt injection updates | §6; pattern review and ML note. |
| Automated testing | §7; 101 tests total. E2E verification script added. |
| Documentation | §8; security config and incident response. |
| Wizard UX / best practices | §9; password, auth, TLS, rate limit. Secure cookies added. |
| Known limitations / adoption | §10 Known limitations and adoption considerations. |
