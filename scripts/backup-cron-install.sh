#!/usr/bin/env bash
# =============================================================
# SafePaw — Install automated backup cron job
# =============================================================
# Adds a daily 2:00 AM cron entry that runs scripts/backup.sh,
# rotates backups older than RETENTION_DAYS (default 30), and
# logs output to /var/log/safepaw-backup.log.
#
# Usage:
#   sudo ./scripts/backup-cron-install.sh            # install
#   sudo ./scripts/backup-cron-install.sh --remove    # uninstall
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEPAW_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_SCRIPT="$SAFEPAW_ROOT/scripts/backup.sh"
LOG_FILE="/var/log/safepaw-backup.log"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
CRON_SCHEDULE="${CRON_SCHEDULE:-0 2 * * *}"
CRON_MARKER="# safepaw-backup"

if [ "${1:-}" = "--remove" ]; then
  crontab -l 2>/dev/null | grep -v "$CRON_MARKER" | crontab -
  echo "Removed SafePaw backup cron job"
  exit 0
fi

# Verify backup script exists and is executable
if [ ! -x "$BACKUP_SCRIPT" ]; then
  chmod +x "$BACKUP_SCRIPT"
fi

# Build the cron command: run backup, prune old files, log everything
CRON_CMD="$CRON_SCHEDULE cd $SAFEPAW_ROOT && $BACKUP_SCRIPT >> $LOG_FILE 2>&1 && find $SAFEPAW_ROOT/backups -type f -mtime +$RETENTION_DAYS -delete >> $LOG_FILE 2>&1 $CRON_MARKER"

# Remove existing entry if present, then add new one
(crontab -l 2>/dev/null | grep -v "$CRON_MARKER"; echo "$CRON_CMD") | crontab -

echo "Installed SafePaw backup cron job:"
echo "  Schedule: $CRON_SCHEDULE (default: daily at 2:00 AM)"
echo "  Retention: $RETENTION_DAYS days"
echo "  Log: $LOG_FILE"
echo ""
echo "Verify with: crontab -l | grep safepaw"
echo "Remove with: $0 --remove"
