# SafePaw Backup and Recovery

Procedures for backing up and restoring SafePaw persistent data. Use these to recover from volume loss, node failure, or migration.

**Related:** [RUNBOOK.md](RUNBOOK.md) for incident response; [docker-compose.yml](docker-compose.yml) for volume definitions.

---

## What to Back Up

| Asset | Location | Contains | Backup method |
|-------|----------|---------|----------------|
| **Postgres** | Named volume `postgres-data` | Config store, wizard state, any app tables | `pg_dump` (logical backup) |
| **Redis** | Named volume `redis-data` | Rate-limit state, token revocations (when Redis enabled) | RDB snapshot or `redis-cli SAVE` + copy `/data` |
| **OpenClaw config** | Named volume `openclaw-home` | `~/.openclaw/` — OpenClaw config, env | Tar/copy volume or backup from running container |
| **OpenClaw workspace** | Named volume `openclaw-workspace` | Agent workspace files | Tar/copy volume |
| **Secrets** | `.env` on host | All passwords, API keys, TLS paths | Encrypted copy (do not commit to git) |

---

## Volume Names (Docker Compose default project name)

With project name `safepaw` (directory name or `-p safepaw`):

- `safepaw_postgres-data`
- `safepaw_redis-data`
- `safepaw_openclaw-home`
- `safepaw_openclaw-workspace`

List volumes: `docker volume ls | grep safepaw`

---

## Backup Procedures

### 1. Postgres (recommended: logical backup)

```bash
# From host; container must be running
docker compose exec postgres pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" --no-owner --no-acl -F c -f /tmp/safepaw_pg.dump

# Copy dump out of container
docker compose cp postgres:/tmp/safepaw_pg.dump ./backups/safepaw_pg_$(date +%Y%m%d_%H%M).dump
```

For a plain SQL file (portable, no custom format):

```bash
docker compose exec postgres pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" --no-owner --no-acl > ./backups/safepaw_pg_$(date +%Y%m%d).sql
```

### 2. Redis

```bash
# Trigger RDB snapshot (if appendonly is also used, Redis has both)
docker compose exec redis redis-cli -a "${REDIS_PASSWORD}" BGSAVE

# Wait a few seconds, then copy the volume or the dump file
docker compose exec redis cat /data/dump.rdb > ./backups/safepaw_redis_$(date +%Y%m%d).rdb
# Or copy the whole volume
docker run --rm -v safepaw_redis-data:/data -v $(pwd)/backups:/backup alpine tar czf /backup/safepaw_redis_$(date +%Y%m%d).tar.gz -C /data .
```

### 3. OpenClaw home (config)

```bash
docker run --rm -v safepaw_openclaw-home:/data -v $(pwd)/backups:/backup alpine tar czf /backup/safepaw_openclaw_home_$(date +%Y%m%d).tar.gz -C /data .
```

### 4. OpenClaw workspace (optional, often large)

```bash
docker run --rm -v safepaw_openclaw-workspace:/data -v $(pwd)/backups:/backup alpine tar czf /backup/safepaw_openclaw_workspace_$(date +%Y%m%d).tar.gz -C /data .
```

### 5. .env (secrets)

```bash
# Store encrypted; never commit to git
cp .env ./backups/env_$(date +%Y%m%d).env
# Encrypt, e.g.:
# gpg -c ./backups/env_$(date +%Y%m%d).env
```

---

## Restore Procedures

### 1. Restore Postgres

```bash
# Stop app services that use Postgres (wizard, and any that depend on it)
docker compose stop wizard

# Restore (custom format)
docker compose cp ./backups/safepaw_pg_YYYYMMDD.dump postgres:/tmp/safepaw_pg.dump
docker compose exec postgres pg_restore -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" --no-owner --no-acl --clean --if-exists /tmp/safepaw_pg.dump

# Or plain SQL
docker compose exec -T postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" < ./backups/safepaw_pg_YYYYMMDD.sql

# Restart
docker compose up -d wizard
```

### 2. Restore Redis

```bash
docker compose stop gateway wizard
docker compose stop redis

# Replace volume content (destructive)
docker run --rm -v safepaw_redis-data:/data -v $(pwd)/backups:/backup alpine sh -c "rm -rf /data/* && tar xzf /backup/safepaw_redis_YYYYMMDD.tar.gz -C /data"

# Or if you saved only dump.rdb:
# docker run --rm -v safepaw_redis-data:/data -v $(pwd)/backups:/backup alpine sh -c "cp /backup/safepaw_redis_YYYYMMDD.rdb /data/dump.rdb"

docker compose up -d redis
# Wait for healthy, then
docker compose up -d gateway wizard
```

### 3. Restore OpenClaw home

```bash
docker compose stop openclaw
docker run --rm -v safepaw_openclaw-home:/data -v $(pwd)/backups:/backup alpine sh -c "rm -rf /data/* /data/..?* 2>/dev/null; tar xzf /backup/safepaw_openclaw_home_YYYYMMDD.tar.gz -C /data"
docker compose up -d openclaw
```

### 4. Restore .env

```bash
# Restore from encrypted backup to .env, then
docker compose up -d
```

---

## Recommended Schedule

| Asset | Frequency | Retention |
|-------|-----------|-----------|
| Postgres | Daily (cron) | 7–30 days |
| Redis | Daily or after significant revocation/config | 7 days |
| OpenClaw home | After config changes or weekly | 4 weeks |
| OpenClaw workspace | Optional; weekly if needed | 2 weeks |
| .env | On change (encrypted) | Indefinite (secure store) |

---

## Disaster Recovery (full stack on new host)

1. Install Docker and Docker Compose; clone or copy the SafePaw repo (and any custom config).
2. Restore `.env` from secure backup (decrypt if needed).
3. Create empty volumes if not present: `docker compose up -d postgres redis` (let them create volumes), then stop.
4. Restore Postgres dump and Redis data (and OpenClaw home if backed up) using the procedures above.
5. Start stack: `docker compose up -d`.
6. Verify: `curl http://localhost:8080/health` and wizard login.

---

## RTO / RPO Notes

- **Postgres:** Primary source of truth for config/wizard state. Restore from latest logical backup; RPO = backup interval.
- **Redis:** Revocations and rate-limit state. If lost, revocations are lost (re-issue tokens or rotate AUTH_SECRET); rate limits reset. RPO = backup interval or accept loss.
- **OpenClaw home:** Restore from backup or reconfigure via Wizard; RPO = last backup or last manual config.

For minimal RPO, run Postgres backups at least daily and store them off-host (e.g. S3, NAS, or backup server).
