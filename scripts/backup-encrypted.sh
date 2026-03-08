#!/usr/bin/env bash
# =============================================================
# SafePaw — Encrypted Backup Script
# =============================================================
# Wraps scripts/backup.sh and encrypts the output with GPG
# (AES-256 symmetric encryption). Produces a single encrypted
# archive from all backup artifacts.
#
# Usage:
#   BACKUP_PASSPHRASE="<secret>" ./scripts/backup-encrypted.sh
#   Or: export BACKUP_PASSPHRASE and run without args.
#
# Environment:
#   BACKUP_PASSPHRASE  — Required. Passphrase for symmetric encryption.
#   BACKUP_DIR         — Optional. Override backup output directory.
#   KEEP_UNENCRYPTED   — Optional. Set to "true" to keep raw files.
#
# Output: backups/safepaw_backup_<timestamp>.tar.gz.gpg
# Exit:   0 on success, 1 on failure
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEPAW_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_DIR="${BACKUP_DIR:-$SAFEPAW_ROOT/backups}"
TIMESTAMP="$(date +%Y%m%d_%H%M)"
KEEP_UNENCRYPTED="${KEEP_UNENCRYPTED:-false}"

# --- Pre-flight checks ---

if [ -z "${BACKUP_PASSPHRASE:-}" ]; then
  echo "ERROR: BACKUP_PASSPHRASE is required."
  echo "  export BACKUP_PASSPHRASE='your-secret-passphrase'"
  echo "  ./scripts/backup-encrypted.sh"
  exit 1
fi

if ! command -v gpg &>/dev/null; then
  echo "ERROR: gpg is not installed. Install with: apt install gnupg"
  exit 1
fi

# --- Run the base backup script ---

echo "[$(date -Iseconds)] Starting base backup..."

# Use a temp directory for the raw backup to avoid mixing with encrypted output
RAW_DIR="$BACKUP_DIR/raw_${TIMESTAMP}"
mkdir -p "$RAW_DIR"

BACKUP_DIR="$RAW_DIR" "$SCRIPT_DIR/backup.sh"

# --- Create compressed archive ---

ARCHIVE_NAME="safepaw_backup_${TIMESTAMP}.tar.gz"
ARCHIVE_PATH="$BACKUP_DIR/$ARCHIVE_NAME"

echo "[$(date -Iseconds)] Creating archive from $(ls "$RAW_DIR" | wc -l) files..."
tar czf "$ARCHIVE_PATH" -C "$RAW_DIR" .

# --- Encrypt with GPG (AES-256, symmetric) ---

ENCRYPTED_PATH="${ARCHIVE_PATH}.gpg"

echo "[$(date -Iseconds)] Encrypting with AES-256..."
gpg --batch --yes --symmetric --cipher-algo AES256 \
    --passphrase-fd 3 \
    --output "$ENCRYPTED_PATH" \
    "$ARCHIVE_PATH" \
    3<<< "$BACKUP_PASSPHRASE"

# --- Cleanup ---

rm -f "$ARCHIVE_PATH"  # Remove unencrypted archive

if [ "$KEEP_UNENCRYPTED" != "true" ]; then
  rm -rf "$RAW_DIR"
  echo "[$(date -Iseconds)] Raw backup files removed (set KEEP_UNENCRYPTED=true to keep)"
else
  echo "[$(date -Iseconds)] Raw backup files kept in: $RAW_DIR"
fi

# --- Verify encrypted file ---

FILE_SIZE=$(stat -c%s "$ENCRYPTED_PATH" 2>/dev/null || stat -f%z "$ENCRYPTED_PATH" 2>/dev/null)
echo "[$(date -Iseconds)] Encrypted backup: $ENCRYPTED_PATH ($FILE_SIZE bytes)"
echo "[$(date -Iseconds)] Decrypt with: gpg --decrypt --output backup.tar.gz $ENCRYPTED_PATH"

exit 0
