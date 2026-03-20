#!/usr/bin/env bash
# SafePaw — Clean Shutdown
# Stops all SafePaw Docker services and closes viewer windows.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

echo ""
echo "  SafePaw — Shutting down..."
echo ""

# ── Check Docker ────────────────────────────────────────────
if ! docker info &>/dev/null; then
    echo "[ERROR] Docker is not running. Nothing to stop."
    exit 1
fi

# ── Stop viewer/monitor processes ───────────────────────────
# Kill background processes started by start.sh (stored in .safepaw-pids)
if [[ -f .safepaw-pids ]]; then
    while IFS= read -r pid; do
        kill "$pid" 2>/dev/null || true
    done < .safepaw-pids
    rm -f .safepaw-pids
    echo "[OK] Viewer processes stopped"
fi

# ── Check if anything is running ────────────────────────────
if ! docker ps --filter "name=safepaw-" --format "{{.Names}}" 2>/dev/null | grep -q "safepaw-"; then
    echo "[INFO] No SafePaw containers found running."
    exit 0
fi

# ── Show what we're stopping ────────────────────────────────
echo "Currently running SafePaw containers:"
docker ps --filter "name=safepaw-" --format "  {{.Names}}  ({{.Status}})"
echo ""

# ── Graceful shutdown ───────────────────────────────────────
echo "Stopping services (30s timeout)..."
docker compose down --timeout 30 2>/dev/null || true
docker compose -f docker-compose.demo.yml down --timeout 30 2>/dev/null || true

# ── Verify nothing remains ──────────────────────────────────
sleep 3
if docker ps --filter "name=safepaw-" --format "{{.Names}}" 2>/dev/null | grep -q "safepaw-"; then
    echo ""
    echo "[WARN] Some containers still running. Force-killing..."
    docker ps --filter "name=safepaw-" --format "{{.Names}}" | while read -r c; do
        echo "  Force-stopping $c"
        docker kill "$c" 2>/dev/null || true
        docker rm "$c" 2>/dev/null || true
    done
fi

# ── Append shutdown event to session log ────────────────────
LATEST_LOG=$(ls -t logs/session-*.txt 2>/dev/null | head -1)
if [[ -n "${LATEST_LOG:-}" ]]; then
    echo "[$(date -Iseconds)] === SAFEPAW SHUTDOWN (stop.sh) ===" >> "$LATEST_LOG"
fi

echo ""
echo "=========================================="
echo "  SafePaw: STOPPED"
echo "  All services shut down."
echo "  Log files preserved in: logs/"
echo "=========================================="
echo ""
