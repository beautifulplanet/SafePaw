#!/usr/bin/env bash
# OpenClaw Compatibility Test
# Validates that OpenClaw starts successfully with SafePaw's config.
#
# What it tests:
#   1. Build the OpenClaw Docker image
#   2. Start it with SafePaw's exact config (gateway.mode=local, bind=loopback)
#   3. Wait for the /healthz endpoint to respond 200
#   4. If healthy within timeout → PASS, otherwise → FAIL
#
# Usage:
#   ./scripts/openclaw-compat-test.sh                     # default: build from ../../openclaw
#   OPENCLAW_IMAGE=openclaw:latest ./scripts/openclaw-compat-test.sh  # use existing image

set -euo pipefail

CONTAINER_NAME="safepaw-openclaw-compat-test"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-60}"
OPENCLAW_IMAGE="${OPENCLAW_IMAGE:-}"
OPENCLAW_BUILD_CONTEXT="${OPENCLAW_BUILD_CONTEXT:-../../openclaw}"

# Resolve build context relative to script location
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SAFEPAW_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cleanup() {
  echo "--- Cleaning up ---"
  docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
}
trap cleanup EXIT

# Build image if not provided
if [[ -z "$OPENCLAW_IMAGE" ]]; then
  BUILD_PATH="$SAFEPAW_DIR/$OPENCLAW_BUILD_CONTEXT"
  if [[ ! -f "$BUILD_PATH/Dockerfile" ]]; then
    echo "ERROR: OpenClaw Dockerfile not found at $BUILD_PATH/Dockerfile"
    echo "Set OPENCLAW_BUILD_CONTEXT or OPENCLAW_IMAGE"
    exit 1
  fi
  echo "=== Building OpenClaw image from $BUILD_PATH ==="
  OPENCLAW_IMAGE="openclaw-compat-test:latest"
  docker build -t "$OPENCLAW_IMAGE" "$BUILD_PATH"
fi

# Remove any leftover test container
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# Start OpenClaw with SafePaw's exact config (matches docker-compose.yml)
echo "=== Starting OpenClaw with SafePaw config ==="
docker run -d \
  --name "$CONTAINER_NAME" \
  --memory 512m \
  -e NODE_ENV=production \
  "$OPENCLAW_IMAGE" \
  sh -c '
    mkdir -p /home/node/.openclaw &&
    echo "{\"gateway\":{\"bind\":\"loopback\",\"mode\":\"local\",\"controlUi\":{\"enabled\":true}}}" > /home/node/.openclaw/openclaw.json &&
    exec node openclaw.mjs gateway --port 18789 --bind loopback
  '

# Wait for health check
echo "=== Waiting for OpenClaw to become healthy (timeout: ${HEALTH_TIMEOUT}s) ==="
ELAPSED=0
while [[ $ELAPSED -lt $HEALTH_TIMEOUT ]]; do
  # Check if container is still running
  if ! docker inspect --format='{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null | grep -q true; then
    echo "FAIL: Container exited unexpectedly"
    echo "--- Container logs ---"
    docker logs "$CONTAINER_NAME" 2>&1 | tail -30
    exit 1
  fi

  # Check health endpoint
  HEALTH=$(docker exec "$CONTAINER_NAME" node -e "fetch('http://127.0.0.1:18789/healthz').then(r=>{console.log(r.status);process.exit(r.ok?0:1)}).catch(()=>process.exit(1))" 2>/dev/null) || true
  if [[ "$HEALTH" == "200" ]]; then
    echo "PASS: OpenClaw healthy after ${ELAPSED}s"
    echo "--- Config used ---"
    docker exec "$CONTAINER_NAME" cat /home/node/.openclaw/openclaw.json
    exit 0
  fi

  sleep 2
  ELAPSED=$((ELAPSED + 2))
  printf "  %ds...\r" "$ELAPSED"
done

echo "FAIL: OpenClaw did not become healthy within ${HEALTH_TIMEOUT}s"
echo "--- Container logs ---"
docker logs "$CONTAINER_NAME" 2>&1 | tail -30
exit 1
