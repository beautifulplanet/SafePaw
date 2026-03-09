#!/usr/bin/env bash
# =============================================================
# SafePaw — One-Command Start
# =============================================================
# Usage:
#   ./start.sh               Full stack (wizard + gateway + AI + redis + postgres)
#   ./start.sh --demo        Demo mode  (wizard + gateway + mockbackend, no API key needed)
#   ./start.sh --stop        Stop all services
#   ./start.sh --logs        Tail service logs
# =============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ── Colors ────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info()  { echo -e "${CYAN}▸${NC} $*"; }
ok()    { echo -e "${GREEN}✓${NC} $*"; }
warn()  { echo -e "${YELLOW}!${NC} $*"; }
fail()  { echo -e "${RED}✗${NC} $*"; exit 1; }

# ── Parse args ────────────────────────────────────────────────

MODE="full"
for arg in "$@"; do
  case "$arg" in
    --demo)  MODE="demo" ;;
    --stop)
      info "Stopping SafePaw services..."
      docker compose down 2>/dev/null || true
      docker compose -f docker-compose.demo.yml down 2>/dev/null || true
      ok "All services stopped."
      exit 0
      ;;
    --logs)
      docker compose logs -f --tail=50
      exit 0
      ;;
    --help|-h)
      echo "Usage: ./start.sh [--demo] [--stop] [--logs]"
      echo ""
      echo "  (no args)   Full stack: wizard + gateway + AI + redis + postgres"
      echo "  --demo      Demo mode: wizard + gateway + mockbackend (no API key)"
      echo "  --stop      Stop all services"
      echo "  --logs      Tail service logs"
      exit 0
      ;;
    *)
      warn "Unknown option: $arg (use --help)"
      ;;
  esac
done

# ── Banner ────────────────────────────────────────────────────

echo ""
echo -e "${BOLD}🐾 SafePaw — Starting up...${NC}"
echo ""

# ── Check Docker ──────────────────────────────────────────────

if ! command -v docker &>/dev/null; then
  fail "Docker not found. Install Docker Desktop: https://docs.docker.com/get-docker/"
fi

if ! docker info &>/dev/null; then
  fail "Docker daemon not running. Start Docker Desktop first."
fi
ok "Docker running"

# ── Generate .env if missing ──────────────────────────────────

generate_secret() {
  openssl rand -base64 "$1" 2>/dev/null || head -c "$1" /dev/urandom | base64 | tr -d '=/+' | head -c "$1"
}

# Portable sed -i (macOS requires '' argument, GNU sed does not)
sed_inplace() {
  if [[ "$OSTYPE" == darwin* ]]; then
    sed -i '' "$@"
  else
    sed -i "$@"
  fi
}

if [ "$MODE" = "full" ] && [ ! -f .env ]; then
  info "Generating .env with secure defaults..."

  REDIS_PW=$(generate_secret 24)
  POSTGRES_PW=$(generate_secret 24)
  AUTH_SECRET=$(generate_secret 48)
  WIZARD_PW=$(generate_secret 12 | tr -d '=/+' | head -c 14)
  GW_TOKEN=$(openssl rand -hex 24 2>/dev/null || head -c 48 /dev/urandom | xxd -p | head -c 48)

  cp .env.example .env

  # Replace placeholder values with generated secrets
  sed_inplace "s|REDIS_PASSWORD=CHANGE_ME_generate_a_random_password|REDIS_PASSWORD=${REDIS_PW}|" .env
  sed_inplace "s|POSTGRES_PASSWORD=CHANGE_ME_generate_a_random_password|POSTGRES_PASSWORD=${POSTGRES_PW}|" .env
  sed_inplace "s|AUTH_SECRET=CHANGE_ME_run_openssl_rand_base64_48|AUTH_SECRET=${AUTH_SECRET}|" .env
  sed_inplace "s|OPENCLAW_GATEWAY_TOKEN=CHANGE_ME_run_openssl_rand_hex_24|OPENCLAW_GATEWAY_TOKEN=${GW_TOKEN}|" .env
  # Set wizard password (uncomment and set)
  sed_inplace "s|# WIZARD_ADMIN_PASSWORD=|WIZARD_ADMIN_PASSWORD=${WIZARD_PW}|" .env

  ok "Generated .env with secure defaults"
elif [ "$MODE" = "full" ]; then
  ok ".env already exists (keeping existing config)"
fi

# ── Detect system RAM and set profile ─────────────────────────

detect_profile() {
  local mem_kb
  if [ -f /proc/meminfo ]; then
    mem_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
  elif command -v sysctl &>/dev/null; then
    mem_kb=$(( $(sysctl -n hw.memsize 2>/dev/null || echo 0) / 1024 ))
  else
    mem_kb=0
  fi

  local mem_gb=$(( mem_kb / 1024 / 1024 ))

  if [ "$mem_gb" -ge 128 ]; then
    echo "very-large"
  elif [ "$mem_gb" -ge 64 ]; then
    echo "large"
  elif [ "$mem_gb" -ge 16 ]; then
    echo "medium"
  else
    echo "small"
  fi
}

if [ "$MODE" = "full" ]; then
  PROFILE=$(detect_profile)
  if [ -f /proc/meminfo ]; then
    MEM_KB=$(grep MemTotal /proc/meminfo | awk '{print $2}')
  elif command -v sysctl &>/dev/null; then
    MEM_KB=$(( $(sysctl -n hw.memsize 2>/dev/null || echo 0) / 1024 ))
  else
    MEM_KB=0
  fi
  MEM_GB=$(( MEM_KB / 1024 / 1024 ))
  ok "Detected ${MEM_GB}GB RAM → ${PROFILE} profile"

  # Write SYSTEM_PROFILE to .env if not already set
  if ! grep -q "^SYSTEM_PROFILE=" .env 2>/dev/null; then
    echo "" >> .env
    echo "SYSTEM_PROFILE=${PROFILE}" >> .env
  fi
fi

# ── Launch ────────────────────────────────────────────────────

if [ "$MODE" = "demo" ]; then
  info "Starting demo mode (no API key needed)..."
  COMPOSE_FILE="docker-compose.demo.yml"
else
  info "Building and starting services (first run takes ~90s)..."
  COMPOSE_FILE="docker-compose.yml"
fi

docker compose -f "$COMPOSE_FILE" up -d --build 2>&1 | while IFS= read -r line; do
  # Show progress but filter noise
  case "$line" in
    *"pulling"*|*"Building"*|*"Created"*|*"Started"*|*"running"*)
      echo -e "  ${CYAN}${line}${NC}"
      ;;
  esac
done
# Check if docker compose failed (PIPESTATUS[0] holds its exit code)
COMPOSE_EXIT=${PIPESTATUS[0]}
if [ "${COMPOSE_EXIT:-0}" -ne 0 ]; then
  fail "docker compose exited with status $COMPOSE_EXIT — run 'docker compose -f $COMPOSE_FILE logs' for details"
fi

# ── Wait for healthchecks ─────────────────────────────────────

info "Waiting for services to become healthy..."

MAX_WAIT=120
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
  WIZARD_HEALTH=$(docker inspect --format='{{.State.Health.Status}}' safepaw-wizard 2>/dev/null || echo "starting")
  if [ "$WIZARD_HEALTH" = "healthy" ]; then
    break
  fi
  sleep 3
  ELAPSED=$((ELAPSED + 3))
  printf "\r  Waiting... %ds" "$ELAPSED"
done
printf "\r"

if [ "$WIZARD_HEALTH" = "healthy" ]; then
  ok "All services healthy"
else
  warn "Wizard still starting after ${MAX_WAIT}s — check: docker compose logs wizard"
fi

# ── Print summary ─────────────────────────────────────────────

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  ${BOLD}Wizard:${NC}   http://localhost:3000"

if [ "$MODE" = "demo" ]; then
  echo -e "  ${BOLD}Password:${NC} DemoPassword123!"
else
  # Extract password from .env
  PW=$(grep "^WIZARD_ADMIN_PASSWORD=" .env 2>/dev/null | cut -d= -f2- || echo "")
  if [ -n "$PW" ]; then
    echo -e "  ${BOLD}Password:${NC} ${PW}"
  else
    echo -e "  ${BOLD}Password:${NC} (auto-generated, check: docker compose logs wizard)"
  fi
fi

echo -e "  ${BOLD}Gateway:${NC}  http://localhost:8080"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── Open browser ──────────────────────────────────────────────

open_browser() {
  local url="$1"
  if command -v xdg-open &>/dev/null; then
    xdg-open "$url" 2>/dev/null &
  elif command -v open &>/dev/null; then
    open "$url" 2>/dev/null &
  elif [ -n "${BROWSER:-}" ]; then
    "$BROWSER" "$url" 2>/dev/null &
  else
    return 1
  fi
}

if open_browser "http://localhost:3000"; then
  info "Opening browser..."
else
  info "Open http://localhost:3000 in your browser"
fi

echo ""
echo -e "To stop:  ${CYAN}./start.sh --stop${NC}"
echo -e "Logs:     ${CYAN}./start.sh --logs${NC}"
echo ""
