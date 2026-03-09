#!/usr/bin/env bash
# =============================================================
# SafePaw Wizard — Full UI Endpoint Smoke Test
# =============================================================
# Tests every API endpoint the React UI calls. Uses cookie-based
# auth with CSRF, exactly like the browser does.
#
# Usage:  ./scripts/smoke-test.sh [password]
#         Password defaults to WIZARD_ADMIN_PASSWORD from .env
# =============================================================
set -uo pipefail

BASE="http://localhost:3000/api/v1"
COOKIE_JAR=$(mktemp)
RESP_FILE=$(mktemp)
trap 'rm -f "$COOKIE_JAR" "$RESP_FILE"' EXIT

PASS=0; FAIL=0; WARN=0

pass() { ((PASS++)); printf "  \033[32m✓\033[0m %s\n" "$1"; }
fail() { ((FAIL++)); printf "  \033[31m✗\033[0m %s — %s\n" "$1" "$2"; }
warn() { ((WARN++)); printf "  \033[33m⚠\033[0m %s — %s\n" "$1" "$2"; }
header() { printf "\n\033[1m── %s ──\033[0m\n" "$1"; }

get_csrf() { grep $'\tcsrf\t' "$COOKIE_JAR" 2>/dev/null | awk '{print $NF}' || true; }

do_request() {
  local method="$1" path="$2" body="${3:-}" csrf
  csrf=$(get_csrf)
  local -a extra=()
  [[ "$method" != "GET" && -n "$csrf" ]] && extra=(-H "X-CSRF-Token: ${csrf}")
  [[ -n "$body" ]] && extra+=(-d "$body")
  HTTP_CODE=$(curl -s -o "$RESP_FILE" -w '%{http_code}' \
    -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -X "$method" -H "Content-Type: application/json" \
    "${extra[@]}" "${BASE}${path}")
  HTTP_BODY=$(cat "$RESP_FILE")
}

assert_status() {
  if [[ "$HTTP_CODE" == "$2" ]]; then pass "$1 (HTTP $HTTP_CODE)"
  else fail "$1" "expected $2, got $HTTP_CODE — $HTTP_BODY"; fi
}

assert_contains() {
  if [[ "$HTTP_CODE" != "$2" ]]; then fail "$1" "expected $2, got $HTTP_CODE — $HTTP_BODY"
  elif echo "$HTTP_BODY" | grep -q "$3"; then pass "$1 (HTTP $HTTP_CODE)"
  else fail "$1" "body missing '$3': $HTTP_BODY"; fi
}

# ── Resolve password ──────────────────────────────────────────
PASSWORD="${1:-}"
if [[ -z "$PASSWORD" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  ENV_FILE="${SCRIPT_DIR}/../.env"
  [[ -f "$ENV_FILE" ]] && PASSWORD=$(grep -oP 'WIZARD_ADMIN_PASSWORD=\K.*' "$ENV_FILE" 2>/dev/null || true)
fi
if [[ -z "$PASSWORD" ]]; then
  echo "Usage: $0 <password>  (or set WIZARD_ADMIN_PASSWORD in .env)" >&2; exit 1
fi

printf "\033[1;36mSafePaw Smoke Test\033[0m\n"
printf "Target: %s  |  Time: %s\n" "$BASE" "$(date -Iseconds)"

# ── 1. Public ─────────────────────────────────────────────────
header "Public Endpoints"
do_request GET /health
assert_contains "GET /health" 200 '"status":"ok"'

# ── 2. Login ──────────────────────────────────────────────────
header "Authentication"
do_request POST /auth/login '{"password":"wrong-password-12345"}'
assert_status "POST /auth/login (bad pw → 401)" 401

do_request POST /auth/login "{\"password\":\"${PASSWORD}\"}"
assert_contains "POST /auth/login (good pw → 200)" 200 '"role"'

if grep -q $'\tsession\t' "$COOKIE_JAR" && grep -q $'\tcsrf\t' "$COOKIE_JAR"; then
  pass "Session + CSRF cookies set"
else
  fail "Cookie check" "session or csrf cookie missing"
fi

# ── 3. CSRF protection ───────────────────────────────────────
header "CSRF Protection"
csrf_code=$(curl -s -o /dev/null -w '%{http_code}' \
  -b "$COOKIE_JAR" -X POST -H 'Content-Type: application/json' \
  -d '{"subject":"csrf-test","scope":"proxy","ttl_hours":1}' \
  "${BASE}/gateway/token")
if [[ "$csrf_code" == "403" ]]; then pass "POST without X-CSRF-Token → 403"
else fail "CSRF rejection" "expected 403, got $csrf_code"; fi

# ── 4. GET endpoints ──────────────────────────────────────────
header "Read-Only Endpoints (GET)"
do_request GET /prerequisites;          assert_status "GET /prerequisites" 200
do_request GET /status;                 assert_contains "GET /status" 200 '"services"'
do_request GET /config;                 assert_contains "GET /config" 200 '"config"'

do_request GET /gateway/metrics
[[ "$HTTP_CODE" == "200" ]] && pass "GET /gateway/metrics (HTTP 200)" || warn "GET /gateway/metrics" "HTTP $HTTP_CODE"

do_request GET /gateway/activity
[[ "$HTTP_CODE" == "200" ]] && pass "GET /gateway/activity (HTTP 200)" || warn "GET /gateway/activity" "HTTP $HTTP_CODE"

do_request GET /gateway/usage
[[ "$HTTP_CODE" == "200" ]] && pass "GET /gateway/usage (HTTP 200)" || warn "GET /gateway/usage" "HTTP $HTTP_CODE"

do_request GET "/cost/history?days=30"; assert_status "GET /cost/history" 200
do_request GET "/cost/models?days=30";  assert_status "GET /cost/models" 200
do_request GET "/cost/trends?days=7";   assert_status "GET /cost/trends" 200

# ── 5. Mutating endpoints ────────────────────────────────────
header "Mutating Endpoints (POST/PUT with CSRF)"

do_request POST /gateway/token '{"subject":"smoke-test","scope":"proxy","ttl_hours":1}'
assert_contains "POST /gateway/token" 200 '"token"'
GATEWAY_TOKEN=$(echo "$HTTP_BODY" | grep -oP '"token"\s*:\s*"\K[^"]+' || true)

do_request PUT /config '{"SAFEPAW_LOG_LEVEL":"info"}'
assert_contains "PUT /config" 200 '"status"'

do_request POST /services/redis/restart ''
[[ "$HTTP_CODE" == "200" || "$HTTP_CODE" == "202" ]] \
  && pass "POST /services/redis/restart (HTTP $HTTP_CODE)" \
  || warn "POST /services/redis/restart" "HTTP $HTTP_CODE — $HTTP_BODY"

# ── 6. Gateway integration ───────────────────────────────────
header "Gateway Integration"
if [[ -n "${GATEWAY_TOKEN:-}" ]]; then
  gw_code=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer $GATEWAY_TOKEN" \
    http://localhost:8080/health 2>/dev/null || echo "000")
  [[ "$gw_code" == "200" ]] \
    && pass "Gateway token works at :8080/health (HTTP 200)" \
    || warn "Gateway token at :8080/health" "HTTP $gw_code"
else
  warn "Gateway token test" "no token was generated"
fi

# ── 7. Negative tests ────────────────────────────────────────
header "Negative / Edge Cases"

do_request POST /services/fakename/restart ''
[[ "$HTTP_CODE" == "400" || "$HTTP_CODE" == "404" ]] \
  && pass "POST /services/fakename/restart → rejected ($HTTP_CODE)" \
  || fail "Invalid service name" "expected 400/404, got $HTTP_CODE"

do_request POST /gateway/token '{"subject":"","scope":"proxy","ttl_hours":999}'
if [[ "$HTTP_CODE" == "400" ]]; then pass "POST /gateway/token (bad TTL) → 400"
elif [[ "$HTTP_CODE" == "200" ]]; then warn "POST /gateway/token (TTL=999)" "server accepted (may cap internally)"
else fail "Bad TTL rejection" "expected 400, got $HTTP_CODE"; fi

# ── Summary ───────────────────────────────────────────────────
header "Summary"
TOTAL=$((PASS + FAIL + WARN))
printf "  Total: %d  |  " "$TOTAL"
printf "\033[32mPassed: %d\033[0m  |  " "$PASS"
printf "\033[33mWarnings: %d\033[0m  |  " "$WARN"
printf "\033[31mFailed: %d\033[0m\n" "$FAIL"

if [[ "$FAIL" -gt 0 ]]; then
  printf "\n\033[31m✗ %d test(s) FAILED\033[0m\n" "$FAIL"; exit 1
else
  printf "\n\033[32m✓ All tests passed\033[0m"
  [[ "$WARN" -gt 0 ]] && printf " \033[33m(%d warnings)\033[0m" "$WARN"
  printf "\n"; exit 0
fi
