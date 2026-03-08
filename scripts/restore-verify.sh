#!/usr/bin/env bash
# =============================================================
# SafePaw — Restore Verification Test
# =============================================================
# Verifies that an encrypted backup can be decrypted, extracted,
# and contains the expected artifacts. Does NOT restore into a
# running system — this is a non-destructive integrity check.
#
# Usage:
#   BACKUP_PASSPHRASE="<secret>" ./scripts/restore-verify.sh <backup.tar.gz.gpg>
#   Or with no args: finds the latest .gpg in backups/
#
# Exit: 0 if all checks pass, 1 on any failure
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEPAW_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_DIR="${BACKUP_DIR:-$SAFEPAW_ROOT/backups}"

# --- Locate backup file ---

if [ -n "${1:-}" ]; then
  ENCRYPTED_FILE="$1"
else
  # Find the latest .gpg file in backups/
  ENCRYPTED_FILE=$(find "$BACKUP_DIR" -maxdepth 1 -name "*.tar.gz.gpg" -type f | sort | tail -1)
  if [ -z "$ENCRYPTED_FILE" ]; then
    echo "ERROR: No .tar.gz.gpg file found in $BACKUP_DIR"
    echo "Usage: $0 <path-to-backup.tar.gz.gpg>"
    exit 1
  fi
fi

if [ ! -f "$ENCRYPTED_FILE" ]; then
  echo "ERROR: File not found: $ENCRYPTED_FILE"
  exit 1
fi

if [ -z "${BACKUP_PASSPHRASE:-}" ]; then
  echo "ERROR: BACKUP_PASSPHRASE is required."
  exit 1
fi

echo "=== SafePaw Restore Verification ==="
echo "File: $ENCRYPTED_FILE"
echo "Size: $(stat -c%s "$ENCRYPTED_FILE" 2>/dev/null || stat -f%z "$ENCRYPTED_FILE" 2>/dev/null) bytes"
echo ""

# --- Create temp directory (cleaned up on exit) ---

VERIFY_DIR=$(mktemp -d)
trap 'rm -rf "$VERIFY_DIR"' EXIT

PASS=0
FAIL=0
TOTAL=0

check() {
  TOTAL=$((TOTAL + 1))
  local desc="$1"
  local result="$2"  # "pass" or "fail"
  if [ "$result" = "pass" ]; then
    PASS=$((PASS + 1))
    echo "  [PASS] $desc"
  else
    FAIL=$((FAIL + 1))
    echo "  [FAIL] $desc"
  fi
}

# --- Test 1: Decrypt ---

echo "1. Decrypting..."
DECRYPTED="$VERIFY_DIR/backup.tar.gz"
if gpg --batch --yes --decrypt \
    --passphrase-fd 3 \
    --output "$DECRYPTED" \
    "$ENCRYPTED_FILE" \
    3<<< "$BACKUP_PASSPHRASE" 2>/dev/null; then
  check "GPG decryption with AES-256" "pass"
else
  check "GPG decryption with AES-256" "fail"
  echo ""
  echo "FATAL: Cannot decrypt. Wrong passphrase or corrupted file."
  exit 1
fi

# --- Test 2: Extract ---

echo "2. Extracting..."
EXTRACT_DIR="$VERIFY_DIR/extracted"
mkdir -p "$EXTRACT_DIR"
if tar xzf "$DECRYPTED" -C "$EXTRACT_DIR" 2>/dev/null; then
  check "Archive extraction" "pass"
else
  check "Archive extraction" "fail"
  echo ""
  echo "FATAL: Cannot extract. Corrupted archive."
  exit 1
fi

# --- Test 3: Check expected artifacts ---

echo "3. Checking artifacts..."

# Postgres dump
PG_DUMP=$(find "$EXTRACT_DIR" -name "safepaw_pg_*.dump" -type f | head -1)
if [ -n "$PG_DUMP" ]; then
  PG_SIZE=$(stat -c%s "$PG_DUMP" 2>/dev/null || stat -f%z "$PG_DUMP" 2>/dev/null)
  if [ "$PG_SIZE" -gt 0 ]; then
    check "Postgres dump present and non-empty ($PG_SIZE bytes)" "pass"
  else
    check "Postgres dump present but empty" "fail"
  fi
else
  check "Postgres dump present" "fail"
fi

# Redis RDB
REDIS_RDB=$(find "$EXTRACT_DIR" -name "safepaw_redis_*.rdb" -type f | head -1)
if [ -n "$REDIS_RDB" ]; then
  REDIS_SIZE=$(stat -c%s "$REDIS_RDB" 2>/dev/null || stat -f%z "$REDIS_RDB" 2>/dev/null)
  if [ "$REDIS_SIZE" -gt 0 ]; then
    check "Redis RDB present and non-empty ($REDIS_SIZE bytes)" "pass"
  else
    check "Redis RDB present but empty" "fail"
  fi
else
  # Redis backup is expected but may not exist if Redis wasn't running
  check "Redis RDB present (may be missing if Redis not running)" "fail"
fi

# .env backup
ENV_FILE=$(find "$EXTRACT_DIR" -name "env_*.env" -type f | head -1)
if [ -n "$ENV_FILE" ]; then
  check ".env backup present" "pass"
  # Verify it contains expected keys (without revealing values)
  EXPECTED_KEYS=("AUTH_SECRET" "POSTGRES_USER" "POSTGRES_DB")
  for key in "${EXPECTED_KEYS[@]}"; do
    if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
      check ".env contains $key" "pass"
    else
      check ".env contains $key" "fail"
    fi
  done
else
  check ".env backup present" "fail"
fi

# OpenClaw home (optional — only present if volume existed)
OC_ARCHIVE=$(find "$EXTRACT_DIR" -name "safepaw_openclaw_home_*.tar.gz" -type f | head -1)
if [ -n "$OC_ARCHIVE" ]; then
  check "OpenClaw home archive present (optional)" "pass"
else
  echo "  [SKIP] OpenClaw home archive (optional — volume may not exist)"
fi

# --- Test 4: Wrong passphrase rejection ---

echo "4. Verifying wrong passphrase is rejected..."
if gpg --batch --yes --decrypt \
    --passphrase-fd 3 \
    --output /dev/null \
    "$ENCRYPTED_FILE" \
    3<<< "DEFINITELY_WRONG_PASSPHRASE_$(date +%s)" 2>/dev/null; then
  check "Wrong passphrase rejected" "fail"
else
  check "Wrong passphrase rejected" "pass"
fi

# --- Summary ---

echo ""
echo "=== Results: $PASS/$TOTAL passed, $FAIL failed ==="
echo ""

if [ "$FAIL" -eq 0 ]; then
  echo "All checks passed. Backup is restorable."
  echo ""
  echo "To restore (destructive — read BACKUP-RECOVERY.md first):"
  echo "  1. gpg --decrypt --output backup.tar.gz $ENCRYPTED_FILE"
  echo "  2. tar xzf backup.tar.gz -C /tmp/safepaw-restore/"
  echo "  3. Follow BACKUP-RECOVERY.md restore procedures"
  exit 0
else
  echo "Some checks failed. Review output above."
  exit 1
fi
