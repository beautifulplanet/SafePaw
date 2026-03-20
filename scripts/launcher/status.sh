#!/usr/bin/env bash
# SafePaw — Service Status Check
# Shows per-service state, health, resource usage, and overall summary.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

echo ""
echo "  SafePaw — Service Status"
echo "  ========================"
echo ""

# ── Check Docker ────────────────────────────────────────────
if ! docker info &>/dev/null; then
    echo "[ERROR] Docker is not running."
    echo ""
    echo "  SafePaw: UNKNOWN (Docker not available)"
    exit 1
fi

# ── Check if any SafePaw containers exist ───────────────────
if ! docker ps -a --filter "name=safepaw-" --format "{{.Names}}" 2>/dev/null | grep -q "safepaw-"; then
    echo "  SafePaw: NOT DEPLOYED"
    echo "  No SafePaw containers found."
    echo ""
    echo "  To start: run ./start.sh or ./START-DEMO.bat"
    exit 0
fi

# ── Per-service status ──────────────────────────────────────
printf "  %-25s %-12s %s\n" "Container" "State" "Status"
printf "  %-25s %-12s %s\n" "-------------------------" "----------" "-------------------"
docker ps -a --filter "name=safepaw-" --format "  {{.Names}}|{{.State}}|{{.Status}}" | while IFS='|' read -r name state status; do
    printf "  %-25s %-12s %s\n" "$name" "$state" "$status"
done

echo ""

# ── Summary ─────────────────────────────────────────────────
TOTAL=$(docker ps -a --filter "name=safepaw-" --format "{{.Names}}" | wc -l)
RUNNING=$(docker ps --filter "name=safepaw-" --filter "status=running" --format "{{.Names}}" | wc -l)
HEALTHY=$(docker ps --filter "name=safepaw-" --filter "health=healthy" --format "{{.Names}}" | wc -l)

echo "  -----------------------------------------"
if [[ "$RUNNING" -eq "$TOTAL" ]]; then
    echo "  SafePaw: RUNNING ($RUNNING/$TOTAL up, $HEALTHY healthy)"
elif [[ "$RUNNING" -eq 0 ]]; then
    echo "  SafePaw: STOPPED ($RUNNING/$TOTAL up)"
else
    echo "  SafePaw: DEGRADED ($RUNNING/$TOTAL up, $HEALTHY healthy)"
fi
echo "  -----------------------------------------"

# ── Port bindings ───────────────────────────────────────────
echo ""
echo "  Endpoints:"
docker ps --filter "name=safepaw-" --format "  {{.Names}}: {{.Ports}}" 2>/dev/null

# ── Resource usage ──────────────────────────────────────────
echo ""
echo "  Resources:"
docker stats --no-stream --filter "name=safepaw-" --format "  {{.Name}}: CPU {{.CPUPerc}} / Mem {{.MemUsage}}" 2>/dev/null

# ── Recent session log ──────────────────────────────────────
echo ""
echo "  Session logs:"
LATEST_LOG=$(ls -t logs/session-*.txt 2>/dev/null | head -1)
if [[ -n "${LATEST_LOG:-}" ]]; then
    echo "  Latest: $LATEST_LOG"
else
    echo "  No session logs found in logs/"
fi
echo ""
