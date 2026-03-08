#!/usr/bin/env bash
# =============================================================
# SafePaw — Automated Backup Script
# =============================================================
# Backs up Postgres, Redis, OpenClaw volumes, and .env per
# BACKUP-RECOVERY.md. Intended for cron or manual runs.
#
# Usage:
#   From safepaw directory: ./scripts/backup.sh
#   Or: BACKUP_DIR=/path/to/backups ./scripts/backup.sh
#
# Requires: docker compose, .env (or set POSTGRES_USER, POSTGRES_DB, REDIS_PASSWORD)
# Exit: 0 on success, 1 on any backup failure
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEPAW_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_DIR="${BACKUP_DIR:-$SAFEPAW_ROOT/backups}"
TIMESTAMP="$(date +%Y%m%d_%H%M)"

# Load .env if present (for POSTGRES_USER, POSTGRES_DB, REDIS_PASSWORD)
if [ -f "$SAFEPAW_ROOT/.env" ]; then
  set -a
  # shellcheck source=/dev/null
  source "$SAFEPAW_ROOT/.env"
  set +a
fi

POSTGRES_USER="${POSTGRES_USER:-safepaw}"
POSTGRES_DB="${POSTGRES_DB:-safepaw}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"

mkdir -p "$BACKUP_DIR"
cd "$SAFEPAW_ROOT"

log() { echo "[$(date -Iseconds)] $*"; }
err() { echo "[$(date -Iseconds)] ERROR: $*" >&2; }

# Project name for volume names (default from directory name)
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-safepaw}"
VOLUME_PREFIX="${COMPOSE_PROJECT_NAME}_"

# ----- Postgres -----
log "Backing up Postgres..."
if docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl -F c 2>/dev/null > "$BACKUP_DIR/safepaw_pg_${TIMESTAMP}.dump"; then
  log "Postgres backup: safepaw_pg_${TIMESTAMP}.dump"
else
  err "Postgres backup failed (is postgres running?)"
  exit 1
fi

# ----- Redis -----
log "Backing up Redis..."
if [ -n "${REDIS_PASSWORD:-}" ]; then
  docker compose exec -T redis redis-cli -a "$REDIS_PASSWORD" BGSAVE 2>/dev/null || true
else
  docker compose exec -T redis redis-cli BGSAVE 2>/dev/null || true
fi
sleep 2
if docker compose exec -T redis cat /data/dump.rdb 2>/dev/null > "$BACKUP_DIR/safepaw_redis_${TIMESTAMP}.rdb"; then
  log "Redis backup: safepaw_redis_${TIMESTAMP}.rdb"
else
  err "Redis backup failed (is redis running?)"
  exit 1
fi

# ----- OpenClaw home (optional) -----
VOLUME_NAME="${VOLUME_PREFIX}openclaw-home"
if docker volume inspect "$VOLUME_NAME" &>/dev/null; then
  log "Backing up OpenClaw home volume..."
  if docker run --rm -v "$VOLUME_NAME:/data" -v "$BACKUP_DIR:/backup" alpine tar czf "/backup/safepaw_openclaw_home_${TIMESTAMP}.tar.gz" -C /data . 2>/dev/null; then
    log "OpenClaw home: safepaw_openclaw_home_${TIMESTAMP}.tar.gz"
  else
    err "OpenClaw home backup failed"
    exit 1
  fi
else
  log "Skipping OpenClaw home (volume not found)"
fi

# ----- .env (secrets) -----
if [ -f "$SAFEPAW_ROOT/.env" ]; then
  cp "$SAFEPAW_ROOT/.env" "$BACKUP_DIR/env_${TIMESTAMP}.env"
  log ".env copied to backups (store encrypted; do not commit)"
else
  log "No .env found; skipping"
fi

log "Backup completed: $BACKUP_DIR"
exit 0
