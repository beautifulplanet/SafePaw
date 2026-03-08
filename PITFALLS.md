# SafePaw — Pitfalls & Gotchas Checklist

> **One-stop reference** for every known pitfall, common mistake, and operational
> gotcha. Sourced from README, SAFEGUARDS, SECURITY, THREAT-MODEL, RUNBOOK,
> BACKUP-RECOVERY, CONTRIBUTING, and SCOPE-IMPROVEMENTS.

---

## 1 · Setup & Docker

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| D1 | Docker Desktop resource limits not set | Machine freeze (OOM/CPU pin) | Set CPU=2, Memory=4 GB, Swap=1 GB, Disk=40 GB |
| D2 | VPN active during `docker compose up` | Network failures, DNS black-hole | Disable VPN **or** split-tunnel Docker subnet `172.16.0.0/12` |
| D3 | Ports 3000/8080 already in use | Bind conflict, container won't start | `lsof -i :3000 -i :8080` before starting |
| D4 | Repeated `--no-cache` builds | 4.6 GB+ cache growth, disk full | Build once, iterate on source, build once more |
| D5 | Crash-looping container restarted blindly | Infinite loop, log flood | Fix root cause first, then restart |
| D6 | Docker socket not mounted | Wizard can't list services | Verify `/var/run/docker.sock` volume in compose |
| D7 | Docker Compose version mismatch | `services` syntax errors | Ensure Compose ≥ 2.20 |

## 2 · Authentication & Sessions

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| A1 | Wizard admin password lost (printed once to stdout) | Locked out | Check `docker compose logs wizard` or set `WIZARD_ADMIN_PASSWORD` in `.env` and restart |
| A2 | `AUTH_SECRET` shorter than 32 bytes | Gateway token generation fails (400) | Use `openssl rand -base64 48` |
| A3 | `AUTH_SECRET` rotation invalidates **all** existing tokens | Clients get 401 until re-authed | Warn operators; coordinate rotation window |
| A4 | Token revocation list is in-memory | Lost on gateway restart | Use short TTLs (≤24 h); rotate `AUTH_SECRET` for mass invalidation |
| A5 | Session generation bump on credential change | All active wizard sessions invalidated | Expected behavior — document for operators |
| A6 | TOTP required for admin/operator but not viewer | Viewer MFA gap if secrets warrant it | Acceptable trade-off; document policy |
| A7 | Same password used for multiple roles | Admin trumps (constant-time check all three) | Use distinct passwords per role |

## 3 · Security Scanning & Injection

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| S1 | Prompt-injection scanning is **heuristic only** | Novel/obfuscated payloads pass through | Treat gateway scanner as tripwire, not boundary; update patterns; consider ML |
| S2 | Output scanning can be evaded via encoding | Base64, fullwidth Unicode, homoglyphs | 2-round normalization mitigates; document residual risk |
| S3 | Unicode confusable characters (U+A731 'ꜱ') | False negatives in pattern matching | Full ICU mapping risks false positives; accepted residual |
| S4 | `X-SafePaw-Risk: high` still proxies the request | Backend sees potentially malicious input | Scanner warns, doesn't block by default; tune policy per use case |
| S5 | WebSocket messages capped at 100 MB | Legitimate large payloads truncated | Adjust `io.LimitReader` if needed; 100 MB is generous |
| S6 | govulncheck advisory-only in CI | Known CVEs don't break the build | Track for hard-fail when dependency set stabilizes |

## 4 · Configuration & Secrets

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| C1 | Secrets in git history | Credential leak post-push | Run `git log -p \| grep -i 'secret\|password\|key'` before going public |
| C2 | GitHub Actions logs may print env vars | Credentials visible in CI log | Mask with `::add-mask::` or ensure `echo` never sees secrets |
| C3 | `.env.example` with real values | Credentials exposed in repo | Example values only (`sk-test-xxx`); review pre-release |
| C4 | Docker image bakes secrets into layer | Accessible via `docker history` | Use runtime env vars, never `COPY .env` |
| C5 | `PUT /api/v1/config` disallowed keys silently skipped | Operator thinks key was saved | Response lists applied keys; check returned payload |
| C6 | `SYSTEM_PROFILE` expansion overwrites resource limits | Loss of manual tuning | Setting profile resets all per-service memory/CPU limits |
| C7 | Wizard compromise = full secret access | All API keys readable/writable | Run wizard localhost only; enable MFA; strong password |

## 5 · Backup & Recovery

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| B1 | Docker volume names include project prefix | `docker volume ls` may surprise | Filter: `docker volume ls \| grep safepaw` |
| B2 | Restore order matters | Dependent services crash on stale data | Stop dependents → restore → start dependents |
| B3 | Redis persistence defaults to RDB | Up to 60 s of data loss on crash | Use `redis-cli SAVE` before backup; or enable AOF |
| B4 | `.env` file must be backed up encrypted | Plaintext secrets on disk | `gpg -c .env` before copying offsite |
| B5 | Backup cron silently fails if Docker down | Stale backups | Monitor `/var/log/safepaw-backup.log`; alert on age |

## 6 · Testing & Development

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| T1 | Mocks ≠ real backends | False confidence from perfect mocks | Periodically test against staging; fuzzing/mutation |
| T2 | Overfitting to happy paths | Miss rare/adversarial failures | Include slowloris, fragmented payloads, malformed JSON |
| T3 | Test payloads go stale | Outdated attack patterns pass | Review/update `api-test-collection.sh` payloads quarterly |
| T4 | Heuristic scanner false positives | Benign input blocked | Track FP rate; whitelist known patterns |
| T5 | Race conditions invisible in unit tests | Data races only appear under load | Use `-race` flag; add concurrency stress tests |
| T6 | Error messages leak internals | Attacker learns stack layout | Assert error responses are generic; never expose paths/versions |
| T7 | No `any` types in TypeScript frontend | Type-safety regression | `strict: true` in tsconfig; CI lint enforces |

## 7 · Operations & Monitoring

| # | Pitfall | Impact | Fix |
|---|---------|--------|-----|
| O1 | Log sensitive data (passwords, tokens) | Security breach via log access | Log sanitization active; never log request bodies |
| O2 | Missing structured log prefix | Hard to grep/filter incidents | Use `[AUTH]`, `[SCANNER]`, `[SECURITY]`, `[PROXY]`, etc. |
| O3 | Alert fatigue from warning-level alerts | Operators ignore real incidents | Tune thresholds; separate info/warning/critical channels |
| O4 | Disk space exhaustion from Docker | Stack crashes | Cron `docker system prune`; monitor free space |
| O5 | Gateway 502 on cold start | Users see errors during boot | Health check endpoint + readiness probe; document startup time |
| O6 | IP auto-ban escalation after brute force | Legitimate users may be banned | Provide unban mechanism; document ban durations |
| O7 | Cost monitoring unavailable if OpenClaw down | Usage endpoint returns "unavailable" | Clean degradation — no crash, no stale data |

## 8 · Incident Response Quick Reference

| Incident | Detect | Immediate Action | Recovery |
|----------|--------|-------------------|----------|
| Token compromise | Unusual `[AUTH] Rejected` spike | Revoke token or rotate `AUTH_SECRET` | Re-issue tokens to legitimate clients |
| Prompt injection detected | `[SCANNER] Prompt injection risk=high` | Request still proxied; review logs | Update scanner patterns; consider blocking |
| Brute force | `[WARN] Failed login attempt` repeated | Rate limiter auto-triggers 429 | IP auto-banned with escalating backoff |
| Gateway down (502) | Health check fails on `:8080/health` | Check OpenClaw container logs | Restart stack; check network/resources |
| Secret rotation needed | Scheduled or post-incident | Block → rotate → re-issue (ordered) | Verify all clients re-authenticated |
| Disk space exhaustion | Wizard prerequisite warning | `docker system prune -f` | Add monitoring alert; cron cleanup |

---

*Last updated: 2026-03-08 · Sources: README, SAFEGUARDS, SECURITY, THREAT-MODEL,
RUNBOOK, BACKUP-RECOVERY, CONTRIBUTING, SCOPE-IMPROVEMENTS*
