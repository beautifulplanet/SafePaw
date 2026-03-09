# SafePaw Incident Response Runbook

Structured playbooks for operational incidents. Each runbook follows a standard format: **Detect → Assess → Mitigate → Recover → Postmortem**.

**Related:** [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md) — Backup and restore. [monitoring/NOTIFICATIONS.md](monitoring/NOTIFICATIONS.md) — Wire Grafana alerts to Slack, PagerDuty, or email.

---

## INC-1: Token Compromise (Leaked API Token)

### Detect
- Alert: Unusual traffic pattern from a single subject (`[AUTH] Authenticated` logs with unexpected IPs)
- Alert: `safepaw_requests_total` spike from single subject in Grafana
- Manual report: User reports unauthorized access

### Assess
1. Identify the compromised subject: `grep "[AUTH] Authenticated: sub=SUBJECT" gateway.log`
2. Check scope of access: what endpoints were hit, what data was accessed
3. Check `safepaw_auth_failures_total` for brute-force attempts

### Mitigate
```bash
# Step 1: Revoke the compromised subject's tokens immediately
ADMIN_TOKEN=$(go run tools/tokengen/main.go -sub admin -scope admin -ttl 1h)
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"subject":"COMPROMISED_USER","reason":"token_leaked"}' \
  http://gateway:8080/admin/revoke

# Step 2: Verify revocation
curl -H "Authorization: Bearer $LEAKED_TOKEN" http://gateway:8080/
# Should return 401 with "token_revoked"
```

### Recover
1. Issue new token for the legitimate user after identity verification
2. If widespread compromise: rotate `AUTH_SECRET` and restart gateway (invalidates ALL tokens)
3. Review access logs for the compromised period

### Postmortem
- How was the token leaked? (client-side storage, logging, intercepted)
- Action: shorten default TTL, add IP binding (future work), improve client guidance

---

## INC-2: Prompt Injection Attack (Sustained)

### Detect
- Alert: `safepaw_prompt_injection_detected_total{risk="high"}` rate > threshold
- Log: `[SCANNER] Prompt injection risk=high` entries increasing
- Alert: `[OUTPUT-SCAN] risk=high` entries (successful injection affecting outputs)

### Assess
1. Check log volume: `grep "[SCANNER] Prompt injection risk=high" gateway.log | wc -l`
2. Identify source IPs: `grep "[SCANNER]" gateway.log | grep "risk=high" | awk '{print $NF}'`
3. Check if any injections bypassed input scanner (look for `[OUTPUT-SCAN] risk=high`)

### Mitigate
```bash
# Step 1: If from a single IP, rate limit will auto-block
# Check if rate limiting is catching it:
grep "RATELIMIT.*DENIED" gateway.log | tail -20

# Step 2: If needed, tighten rate limit
# Edit .env: RATE_LIMIT=10
docker compose restart gateway

# Step 3: If patterns are novel, add to sanitize.go promptInjectionPatterns
# (requires code change + rebuild)
```

### Recover
1. Review output logs for any successful injections that reached clients
2. If system prompt or API keys were leaked in output, rotate them
3. Notify affected users if sensitive data was exposed

### Postmortem
- What patterns bypassed the scanner?
- Action: add new patterns to `sanitize.go`, update `output_scanner.go`

---

## INC-3: Gateway Down (Backend Unreachable)

### Detect
- Alert: `/health` returns `{"status":"degraded","backend":"unreachable"}`
- Alert: `safepaw_requests_total{status="502"}` increasing
- Grafana: error rate panel shows 502 spike

### Assess
1. Check gateway health: `curl http://localhost:8080/health`
2. Check OpenClaw container: `docker inspect safepaw-openclaw --format='{{.State.Status}}'`
3. Check Docker network: `docker network inspect safepaw_safepaw-internal`

### Mitigate
```bash
# Step 1: Check container status
docker compose ps

# Step 2: Restart OpenClaw
docker compose restart openclaw

# Step 3: If OpenClaw won't start, check logs
docker compose logs openclaw --tail=50

# Step 4: If npm install is stuck (first run), recreate
docker compose up -d --force-recreate openclaw
```

### Recover
1. Verify health: `curl http://localhost:8080/health` returns `{"status":"ok"}`
2. Check gateway metrics for recovery: `/metrics` shows requests succeeding

### Postmortem
- Why did OpenClaw go down? (OOM, npm failure, crash loop)
- Action: adjust resource limits, add restart policies, consider image pre-build

---

## INC-4: Brute Force Attack on Wizard

### Detect
- Alert: Repeated `[WARN] Failed login attempt from IP` in wizard logs
- Alert: `safepaw_rate_limited_total` increasing on wizard (if proxied through gateway)

### Assess
1. Count failed attempts: `docker compose logs wizard | grep "Failed login" | wc -l`
2. Identify source: `docker compose logs wizard | grep "Failed login" | awk '{print $NF}' | sort | uniq -c`
3. Check if wizard is exposed beyond localhost (should only be `127.0.0.1:3000`)

### Mitigate
```bash
# Step 1: Wizard has LoginGuard lockout (5 failures/min → 15-minute IP lockout, already active)
# Step 2: Wizard has 500ms delay per failed login + rate limiting (already active)
# Step 3: If attack is from local network, check firewall rules

# Step 4: Change admin password (invalidates all sessions)
# Edit .env: WIZARD_ADMIN_PASSWORD=new-strong-password
docker compose restart wizard
```

### Recover
1. Verify new password works
2. Review wizard logs for successful login during attack window

---

## INC-5: Secret Rotation

### When
- **Scheduled:** Quarterly rotation of all secrets (see [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md) — back up before rotating).
- **Emergency:** After any compromise (see INC-1) or suspected leak.

### Secrets to rotate (in order)

| Secret | Used by | Notes |
|--------|--------|--------|
| `POSTGRES_PASSWORD` | Postgres, Wizard (config DB) | Change in Postgres first, then .env; Wizard/OpenClaw may use DB |
| `REDIS_PASSWORD` | Redis, Gateway, Wizard | Change in Redis config and .env; all clients must match |
| `AUTH_SECRET` | Gateway (HMAC signing) | Invalidates all API tokens; re-issue after |
| `WIZARD_ADMIN_PASSWORD` | Wizard (admin login) | Invalidates all wizard sessions; users must re-login |
| TLS cert/key | Gateway (if TLS_ENABLED=true) | Replace files, update paths in .env if needed |
| API keys (OpenAI, Anthropic, etc.) | OpenClaw | Rotate in provider, then update .env / Wizard UI |

### Order of operations (minimize downtime)

1. **Back up** (at least Postgres and .env). See [BACKUP-RECOVERY.md](BACKUP-RECOVERY.md).
2. **Generate new values** (do not overwrite .env until ready):
   ```bash
   NEW_AUTH_SECRET=$(openssl rand -base64 48)
   NEW_REDIS_PASSWORD=$(openssl rand -base64 32)
   NEW_POSTGRES_PASSWORD=$(openssl rand -base64 32)
   NEW_WIZARD_PASSWORD=$(openssl rand -base64 24)
   ```
3. **Postgres:** Set new password inside Postgres, then update `.env` with `POSTGRES_PASSWORD`, then restart Postgres and any service that connects to it (Wizard, OpenClaw if it uses DB).
   ```bash
   docker compose exec postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "ALTER USER ${POSTGRES_USER} PASSWORD 'NEW_POSTGRES_PASSWORD';"
   # Update .env POSTGRES_PASSWORD, then:
   docker compose restart postgres
   # Wait for healthy
   docker compose restart wizard
   ```
4. **Redis:** Update `.env` with new `REDIS_PASSWORD`. Restart Redis with new password (Compose passes env); then restart Gateway and Wizard so they reconnect with new password.
   ```bash
   # After editing .env:
   docker compose restart redis
   docker compose restart gateway wizard
   ```
5. **AUTH_SECRET:** Update `.env` with `AUTH_SECRET`. Restart Gateway. All existing API tokens are invalid; issue new tokens (e.g. `go run tools/tokengen/main.go -sub admin -scope admin -ttl 24h`).
   ```bash
   docker compose restart gateway
   ```
6. **WIZARD_ADMIN_PASSWORD:** Update `.env`; restart Wizard. All wizard sessions invalid; admins re-login with new password.
   ```bash
   docker compose restart wizard
   ```
7. **Verify:** `curl http://localhost:8080/health`, wizard login, and one API call with new token.

### One-shot full rotation (downtime acceptable)

```bash
# 1. Generate new secrets (save to .env)
NEW_AUTH_SECRET=$(openssl rand -base64 48)
NEW_REDIS_PASSWORD=$(openssl rand -base64 32)
NEW_POSTGRES_PASSWORD=$(openssl rand -base64 32)
# Update .env: AUTH_SECRET, REDIS_PASSWORD, POSTGRES_PASSWORD

# 2. Postgres password change (from host, before restart)
docker compose exec postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "ALTER USER ${POSTGRES_USER} PASSWORD '${NEW_POSTGRES_PASSWORD}';"

# 3. Rolling restart (order matters)
docker compose restart redis
docker compose restart postgres
docker compose restart openclaw
docker compose restart gateway
docker compose restart wizard

# 4. Verify
curl http://localhost:8080/health
curl http://localhost:3000/api/v1/health

# 5. Re-issue auth tokens (old tokens invalid)
go run tools/tokengen/main.go -sub admin -scope admin -ttl 24h
```

---

## INC-6: Disk Space Exhaustion

### Detect
- Wizard prerequisites page shows "Low disk space" warning
- Docker logs: `no space left on device`
- Grafana not receiving new data

### Mitigate
```bash
# 1. Check disk usage
df -h

# 2. Clean Docker resources
docker system prune -f
docker volume prune -f

# 3. Rotate logs
docker compose logs --no-log-prefix > /tmp/safepaw-archive-$(date +%Y%m%d).log
# Then truncate Docker container logs (requires root)

# 4. Check volume sizes
docker system df -v
```

---

## Incident response timeline

Target times apply from **first detection** (alert or report). Adjust for your team size and SLA commitments.

| Phase | P0 Critical | P1 High | P2 Medium | P3 Low |
|-------|-------------|---------|-----------|--------|
| **Acknowledge** | 5 min | 15 min | 4 hours | 24 hours |
| **Mitigate** (stop the bleed) | 15 min | 1 hour | 8 hours | 3 days |
| **Recover** (service normal) | 1 hour | 4 hours | 24 hours | 1 week |
| **Postmortem** (blameless write-up) | 3 days | 5 days | 2 weeks | Backlog |

- **Acknowledge:** On-call has seen the incident and responded (e.g. “Investigating” in chat or ticket).
- **Mitigate:** Immediate impact stopped (e.g. token revoked, service restarted, attack blocked).
- **Recover:** Service restored and verified (health green, no ongoing errors).
- **Postmortem:** Documented what happened, root cause, and follow-up actions (see each playbook’s Postmortem).

---

## Escalation matrix

| Severity | Response time target | Owner | Examples |
|----------|----------------------|-------|----------|
| **P0 — Critical** | 15 min to mitigate | On-call + team lead | Token compromise, data breach, all services down |
| **P1 — High** | 1 hour to mitigate | On-call | Gateway down, sustained attack, OpenClaw unreachable |
| **P2 — Medium** | Same business day | Assignee | Single service degraded, disk warning, unusual patterns |
| **P3 — Low** | 24 hours | Backlog | Non-critical warnings, performance tuning, log noise |

**Escalation path:** If the on-call cannot meet the mitigate target, escalate to team lead (P0/P1) or next available engineer (P2). Document escalation in the incident ticket or chat.
