#!/usr/bin/env bash
# =============================================================
# SafePaw — Demo / Video Prep
# =============================================================
# Creates .env.demo with safe, known values for recording the
# InstallerClaw demo video. No real API keys or secrets.
#
# Usage (from safepaw directory):
#   ./scripts/demo-prep.sh
#   cp .env.demo .env    # use demo env for recording only
#   docker compose up -d
#
# After recording: restore your real .env and restart if needed.
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFEPAW_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_EXAMPLE="$SAFEPAW_ROOT/.env.example"
ENV_DEMO="$SAFEPAW_ROOT/.env.demo"

if [ ! -f "$ENV_EXAMPLE" ]; then
  echo "Error: .env.example not found at $ENV_EXAMPLE. Run from safepaw or scripts directory." >&2
  exit 1
fi

# Demo password: known for video, never use in production
WIZARD_DEMO_PASSWORD="DemoPassword123!"
REDIS_DEMO="demo-redis-password-$(openssl rand -hex 8)"
POSTGRES_DEMO="demo-postgres-password-$(openssl rand -hex 8)"
AUTH_DEMO="demo-auth-secret-$(openssl rand -hex 24)"

# Build .env.demo from .env.example, substituting demo values
sed -e "s/^REDIS_PASSWORD=.*/REDIS_PASSWORD=$REDIS_DEMO/" \
    -e "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$POSTGRES_DEMO/" \
    -e "s/^# WIZARD_ADMIN_PASSWORD=.*/WIZARD_ADMIN_PASSWORD=$WIZARD_DEMO_PASSWORD/" \
    -e "s/^AUTH_ENABLED=.*/AUTH_ENABLED=false/" \
    -e "s/^AUTH_SECRET=.*/AUTH_SECRET=$AUTH_DEMO/" \
    "$ENV_EXAMPLE" > "$ENV_DEMO"

# Ensure WIZARD_ADMIN_PASSWORD is set even if the comment line differed
if ! grep -q "^WIZARD_ADMIN_PASSWORD=" "$ENV_DEMO"; then
  echo "WIZARD_ADMIN_PASSWORD=$WIZARD_DEMO_PASSWORD" >> "$ENV_DEMO"
fi

echo "Created: $ENV_DEMO"
echo ""
echo "Demo wizard login password: $WIZARD_DEMO_PASSWORD"
echo "Gateway auth is DISABLED (AUTH_ENABLED=false) so you can hit :8080 without a token."
echo ""
echo "For recording:"
echo "  1. cp .env.demo .env"
echo "  2. docker compose up -d"
echo "  3. Open http://localhost:3000 and log in with the password above."
echo ""
echo "After recording: restore your real .env (do not commit .env or .env.demo with real secrets)."
