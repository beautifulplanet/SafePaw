#!/usr/bin/env bash
# =============================================================
# SafePaw Launcher — menu (same as LAUNCH.bat on Windows)
# =============================================================
# Options: [1] Full stack  [2] Demo  [3] Shut down  [4] Show processes  [Q] Quit
# =============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

GREEN='\033[0;32m'
NC='\033[0m'

stack_status() {
  if ! docker info &>/dev/null; then
    echo "Docker not available"
    return
  fi
  if docker ps --filter "name=safepaw" -q 2>/dev/null | grep -q .; then
    echo "RUNNING"
  else
    echo "STOPPED"
  fi
}

show_menu() {
  clear
  echo ""
  echo "  ============================================================"
  echo "    SAFEPAW - LAUNCHER"
  echo "    Stack: $(stack_status)"
  echo "    SafePaw + OpenClaw or Demo. One click."
  echo "  ============================================================"
  echo ""
  echo "  Start:"
  echo "    [1]  Full stack (SafePaw + OpenClaw)"
  echo "    [2]  Demo (SafePaw only, no API key)"
  echo ""
  echo "  Stop:"
  echo "    [3]  Shut down all"
  echo ""
  echo "  Status:"
  echo "    [4]  Show processes (SafePaw, OpenClaw, support)"
  echo "    [5]  Quick health check (wizard + gateway)"
  echo "  ============================================================"
  echo ""
}

health_check() {
  echo ""
  echo "  Quick health check (localhost)..."
  WIZ_STATUS="unreachable"
  GW_STATUS="unreachable"
  if command -v curl &>/dev/null; then
    WIZ_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 http://127.0.0.1:3000/api/v1/health 2>/dev/null || echo "unreachable")
    GW_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 http://127.0.0.1:8080/health 2>/dev/null || echo "unreachable")
  fi
  echo "    Wizard  :3000: $WIZ_STATUS"
  echo "    Gateway :8080: $GW_STATUS"
  echo ""
  if [ "$WIZ_STATUS" = "200" ] && [ "$GW_STATUS" = "200" ]; then
    echo "  [OK] Both healthy."
  else
    echo "  [--] One or both not ready."
  fi
  echo ""
  echo "  Press Enter to return to menu..."
  read -r
}

run_full() {
  if docker ps --filter "name=safepaw" -q 2>/dev/null | grep -q .; then
    echo ""
    echo "  Stack is already running. Use [3] to stop first, or [4] to see processes."
    echo "  Press Enter to return to menu..."
    read -r
    return
  fi
  echo ""
  echo "  Starting full stack (SafePaw + OpenClaw)..."
  ./start.sh
  echo ""
  echo "  Press Enter to return to menu..."
  read -r
}

run_demo() {
  if docker ps --filter "name=safepaw" -q 2>/dev/null | grep -q .; then
    echo ""
    echo "  Stack is already running. Use [3] to stop first, or [4] to see processes."
    echo "  Press Enter to return to menu..."
    read -r
    return
  fi
  echo ""
  echo "  Starting demo (SafePaw only)..."
  ./start.sh --demo
  echo ""
  echo "  Press Enter to return to menu..."
  read -r
}

do_shutdown() {
  if docker ps --filter "name=safepaw" -q 2>/dev/null | grep -q .; then
    echo ""
    read -r -p "  Stack is running. Shut down all? [y/N]: " confirm
    if [[ ! "${confirm:-}" =~ ^[yY]$ ]]; then
      return
    fi
  fi
  echo ""
  echo "  Shutting down all services..."
  docker compose down 2>/dev/null || true
  docker compose -f docker-compose.demo.yml down 2>/dev/null || true
  echo "  [OK] All stopped."
  echo ""
  echo "  Press Enter to return to menu..."
  read -r
}

show_processes() {
  echo ""
  echo "  ============================================================"
  echo "    SAFEPAW / OPENCLAW - RUNNING PROCESSES"
  echo "  ============================================================"
  echo ""
  if ! docker ps --filter "name=safepaw" -q 2>/dev/null | grep -q .; then
    echo "  No SafePaw or OpenClaw containers running."
    echo "  Start the stack with [1] or [2] to see them here."
    echo ""
  else
    docker ps --filter "name=safepaw"
    echo ""
  fi
  echo "  --- Legend ---"
  echo "    SafePaw:   wizard :3000, gateway :8080"
  echo "    Backend:   openclaw or mockbackend (internal)"
  echo "    Support:   redis, postgres, docker-socket-proxy"
  echo "  ============================================================"
  echo ""
  echo "  Press Enter to return to menu..."
  read -r
}

main() {
  while true; do
    show_menu
    echo -n "Enter [1-5] or [Q] to quit: "
    read -r pick || { echo ""; exit 0; }  # EOF (e.g. piped input) => exit 0
    pick="${pick:-}"
    pick_upper="$(printf '%s' "$pick" | tr 'a-z' 'A-Z')"
    case "$pick_upper" in
      Q)  echo "Bye."; exit 0 ;;
      1)  run_full ;;
      2)  run_demo ;;
      3)  do_shutdown ;;
      4)  show_processes ;;
      5)  health_check ;;
      "") ;;
      *)  echo "  Invalid choice. Try 1, 2, 3, 4, 5, or Q."; sleep 2 ;;
    esac
  done
}

main "$@"
