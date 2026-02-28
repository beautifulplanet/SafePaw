# SafePaw Security

Security operations, incident response, and hardening guidance for SafePaw (NOPEnclaw).

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
| Gateway | `[SECURITY]` | Origin blocked, security headers applied. |
| Gateway | `[PROXY]` | Backend errors (with path and remote). |
| Gateway | `[WS]` | WebSocket upgrade, tunnel established/closed, errors. |
| Wizard | `[WARN] Failed login attempt from <ip>` | Failed admin login (IP for correlation). |

All gateway logs should include `request_id=` where the request ID is available (see [Request ID](#request-id) below) so you can trace a request across layers.

### Request ID

The gateway sets `X-Request-ID` on every request (or preserves the one from the client). This ID is passed along the middleware chain and should be included in security and proxy log lines for incident response.

---

## 3. Defense in Depth

- **OpenClaw is not exposed.** It has no `ports:` on the host; only the gateway and wizard have host ports (and only on `127.0.0.1`). If the gateway fails, OpenClaw does not become reachable from outside.
- **Layers:** Security headers → Request ID → Origin check → Rate limit → Auth (if enabled) → Body scanner → Proxy. A failure in one layer does not bypass the others.
- **Wizard:** Only listens on localhost; admin auth and rate limiting protect the setup UI.

---

## 4. Revocation (Phase 2)

Currently, HMAC tokens cannot be revoked before expiry. The TODO in `gateway/middleware/auth.go` and the Postgres schema (`gateway.auth_tokens`, `gateway.token_revocations`) are in place for a future Phase 2:

- **Phase 2:** Gateway (or a sidecar) checks a revocation list (e.g. synced from Postgres or Redis) and rejects tokens that have been revoked. This allows an emergency kill switch for leaked credentials.

Until then: use short TTLs, rotate `AUTH_SECRET` if a token is leaked, and avoid storing long-lived tokens in client code.

---

## 5. Monitoring and Alerting

- **Health dashboard:** The wizard shows service status (healthy/degraded/down). Use it for quick checks; for production, treat it as a view, not the only alert source.
- **Actionable alerts:** Configure alerts on:
  - Gateway health check failing (e.g. `/health` returns 5xx or degraded).
  - OpenClaw (backend) unreachable from gateway.
  - Repeated auth failures or rate-limit denials from the same IP.
  - Log volume or pattern matching `[SCANNER] Prompt injection risk=high` if you decide to treat high risk as an alert.
- **Prometheus/Grafana:** SafePaw does not yet expose Prometheus metrics. To integrate:
  - Add a `/metrics` endpoint on the gateway (and optionally wizard) that exposes request counts, rate-limit denials, auth failures, and scanner triggers.
  - Scrape with Prometheus and build dashboards/alerts in Grafana.

---

## 6. Prompt Injection and Pattern Updates

- The body scanner uses a fixed set of heuristic patterns (see README “AI Defense Patterns”). New attack vectors can appear over time.
- **Practice:** Review and update patterns in `gateway/middleware/sanitize.go` when new prompt-injection or jailbreak techniques are published; consider a short “Security” section in release notes.
- **ML-based detection:** Not implemented. Consider ML-based anomaly detection as a future enhancement for unknown attack vectors; the current pattern set remains the first line of defense.

---

## 7. Automated Testing

- **Current:** Unit tests for session tokens, wizard middleware (auth, CORS, rate limit), wizard API (health, login, prerequisites, SPA fallback), and gateway WebSocket upgrade detection.
- **Gaps:** No dedicated tests for gateway body scanner (pattern matching / risk levels), gateway auth middleware (scope, expiry), or gateway rate limiter. Add tests for:
  - Security features: auth failure/success, rate limit allow/deny, origin check, scanner risk levels and triggers.
  - Edge cases: malformed tokens, expired tokens, missing/invalid headers, body size limits.
- **CI:** Run `go test ./...` for gateway and wizard on every change; add integration tests that hit the gateway and wizard behind Docker if needed.

---

## 8. Documentation

- **Security config:** README and `.env.example` cover `AUTH_ENABLED`, `AUTH_SECRET`, `TLS_*`, `RATE_LIMIT`, `WIZARD_ADMIN_PASSWORD`. Keep these in sync with code defaults.
- **Incident response:** This document (SECURITY.md) is the source for incident response and logging; update it when adding new security features or log formats.
- **First-run password:** README and Quick Start should state that the wizard prints the auto-generated admin password once to stdout and that it can be retrieved with `docker compose logs wizard` (or `docker logs safepaw-wizard`).

---

## 9. User Experience (Wizard)

The setup wizard should guide users toward secure defaults:

- **Strong admin password:** Prefer setting `WIZARD_ADMIN_PASSWORD` in `.env` to a strong value instead of relying on the one-time auto-generated password (which is only in logs).
- **Enable security layers:** In production, set `AUTH_ENABLED=true`, provide `AUTH_SECRET`, and enable `TLS_ENABLED` with valid certs.
- **Rate limiting:** Document that `RATE_LIMIT` controls gateway request rate per IP and that it can be tuned for abuse protection.

---

## CTO / Red Team Feedback Mapping

| Feedback | Where it’s addressed |
|----------|----------------------|
| Revocation list | §4 Revocation (Phase 2); schema and TODO in code. |
| Monitoring & alerting | §5 Monitoring and Alerting; Prometheus/Grafana note. |
| Logging | §2 Logging; request ID and event table. |
| Defense in depth | §3; OpenClaw not exposed when gateway fails. |
| Prompt injection updates | §6; pattern review and ML note. |
| Automated testing | §7; current coverage and gaps. |
| Documentation | §8; security config and incident response. |
| Wizard UX / best practices | §9; password, auth, TLS, rate limit. |
