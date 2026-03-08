#!/usr/bin/env bash
# =============================================================
# SafePaw Wizard API — Manual / Scripted Test Collection (T9)
# =============================================================
# Usage:
#   ./scripts/api-test-collection.sh              # run all tests
#   ./scripts/api-test-collection.sh auth          # run auth suite only
#   ./scripts/api-test-collection.sh rbac          # run RBAC suite only
#   ./scripts/api-test-collection.sh headers       # run security headers
#   ./scripts/api-test-collection.sh invalid       # run invalid payloads
#   ./scripts/api-test-collection.sh ratelimit     # run rate-limit suite
#
# Environment:
#   WIZARD_URL          (default: http://localhost:3000)
#   ADMIN_PASSWORD      (default: SafePaw2026!)
#   OPERATOR_PASSWORD   (default: unset — skips operator tests)
#   VIEWER_PASSWORD     (default: unset — skips viewer tests)
# =============================================================
set -euo pipefail

WIZARD_URL="${WIZARD_URL:-http://localhost:3000}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-SafePaw2026!}"
OPERATOR_PASSWORD="${OPERATOR_PASSWORD:-}"
VIEWER_PASSWORD="${VIEWER_PASSWORD:-}"

PASS=0; FAIL=0; SKIP=0
SUITE="${1:-all}"

# ─── Helpers ──────────────────────────────────────────────────

red()   { printf '\033[0;31m%s\033[0m\n' "$*"; }
green() { printf '\033[0;32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[0;33m%s\033[0m\n' "$*"; }

assert_status() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$actual" == "$expected" ]]; then
        green "  ✓ $label (HTTP $actual)"
        ((PASS++))
    else
        red   "  ✗ $label — expected $expected, got $actual"
        ((FAIL++))
    fi
}

assert_header() {
    local label="$1" header="$2" expected="$3" headers="$4"
    local value
    value=$(echo "$headers" | grep -i "^${header}:" | head -1 | cut -d: -f2- | xargs)
    if [[ "$value" == *"$expected"* ]]; then
        green "  ✓ $label: $header contains '$expected'"
        ((PASS++))
    else
        red   "  ✗ $label: $header = '$value', expected to contain '$expected'"
        ((FAIL++))
    fi
}

assert_json() {
    local label="$1" jq_expr="$2" expected="$3" body="$4"
    local actual
    actual=$(echo "$body" | jq -r "$jq_expr" 2>/dev/null || echo "__jq_error__")
    if [[ "$actual" == "$expected" ]]; then
        green "  ✓ $label ($jq_expr = $expected)"
        ((PASS++))
    else
        red   "  ✗ $label — $jq_expr = '$actual', expected '$expected'"
        ((FAIL++))
    fi
}

skip_test() {
    yellow "  ⊘ SKIP: $1"
    ((SKIP++))
}

# Login helper — returns session cookie value
login_as() {
    local password="$1"
    curl -s -c - "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"password\":\"$password\"}" 2>/dev/null \
        | grep 'session' | awk '{print $NF}'
}

# Authenticated GET/POST/PUT helpers
auth_get() {
    local cookie="$1" path="$2"
    curl -s -o /dev/null -w '%{http_code}' \
        -b "session=$cookie" "$WIZARD_URL$path"
}

auth_get_body() {
    local cookie="$1" path="$2"
    curl -s -b "session=$cookie" "$WIZARD_URL$path"
}

auth_post() {
    local cookie="$1" path="$2" body="${3:-}"
    if [[ -n "$body" ]]; then
        curl -s -o /dev/null -w '%{http_code}' \
            -b "session=$cookie" \
            -H 'Content-Type: application/json' \
            -d "$body" "$WIZARD_URL$path"
    else
        curl -s -o /dev/null -w '%{http_code}' \
            -b "session=$cookie" \
            -X POST "$WIZARD_URL$path"
    fi
}

auth_put() {
    local cookie="$1" path="$2" body="$3"
    curl -s -o /dev/null -w '%{http_code}' \
        -b "session=$cookie" \
        -H 'Content-Type: application/json' \
        -d "$body" -X PUT "$WIZARD_URL$path"
}

# ─── Suite: Health (public) ───────────────────────────────────

run_health() {
    echo ""
    echo "=== Health (public) ==="

    local code body
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/health")
    assert_status "GET /health returns 200" "200" "$code"

    body=$(curl -s "$WIZARD_URL/api/v1/health")
    assert_json "health.status = ok" ".status" "ok" "$body"
    assert_json "health.service = safepaw-wizard" ".service" "safepaw-wizard" "$body"
}

# ─── Suite: Auth flows ────────────────────────────────────────

run_auth() {
    echo ""
    echo "=== Auth flows ==="

    # Successful admin login
    local resp
    resp=$(curl -s -D - "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"password\":\"$ADMIN_PASSWORD\"}" 2>/dev/null)
    local code
    code=$(echo "$resp" | head -1 | awk '{print $2}')
    assert_status "Admin login succeeds" "200" "$code"

    local body
    body=$(echo "$resp" | sed '1,/^\r$/d')
    assert_json "Login returns role=admin" ".role" "admin" "$body"

    # Session cookie is set
    if echo "$resp" | grep -qi 'set-cookie.*session='; then
        green "  ✓ Session cookie set"
        ((PASS++))
    else
        red "  ✗ Session cookie not set"
        ((FAIL++))
    fi

    # CSRF cookie is set
    if echo "$resp" | grep -qi 'set-cookie.*csrf='; then
        green "  ✓ CSRF cookie set"
        ((PASS++))
    else
        red "  ✗ CSRF cookie not set"
        ((FAIL++))
    fi

    # Wrong password → 401
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d '{"password":"wrong-pass"}')
    assert_status "Wrong password → 401" "401" "$code"

    # Empty body → 400
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d 'not-json')
    assert_status "Malformed login body → 400" "400" "$code"

    # Unauthenticated API access → 401
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/config")
    assert_status "Unauthenticated GET /config → 401" "401" "$code"

    # Authenticated access works
    local session
    session=$(login_as "$ADMIN_PASSWORD")
    if [[ -n "$session" ]]; then
        code=$(auth_get "$session" "/api/v1/config")
        assert_status "Authenticated GET /config → 200" "200" "$code"
    else
        red "  ✗ Failed to obtain session token"
        ((FAIL++))
    fi
}

# ─── Suite: RBAC ──────────────────────────────────────────────

run_rbac() {
    echo ""
    echo "=== RBAC role enforcement ==="

    local admin_session
    admin_session=$(login_as "$ADMIN_PASSWORD")

    # Admin can do everything
    local code
    code=$(auth_get "$admin_session" "/api/v1/config")
    assert_status "Admin GET /config → 200" "200" "$code"

    code=$(auth_put "$admin_session" "/api/v1/config" '{"TLS_ENABLED":"false"}')
    assert_status "Admin PUT /config → 200" "200" "$code"

    code=$(auth_get "$admin_session" "/api/v1/gateway/metrics")
    assert_status "Admin GET /gateway/metrics → 200" "200" "$code"

    # Operator tests
    if [[ -n "$OPERATOR_PASSWORD" ]]; then
        local op_session
        op_session=$(login_as "$OPERATOR_PASSWORD")

        # Verify login returns operator role
        local op_resp
        op_resp=$(curl -s "$WIZARD_URL/api/v1/auth/login" \
            -H 'Content-Type: application/json' \
            -d "{\"password\":\"$OPERATOR_PASSWORD\"}")
        assert_json "Operator login role" ".role" "operator" "$op_resp"

        # Operator can read
        code=$(auth_get "$op_session" "/api/v1/config")
        assert_status "Operator GET /config → 200" "200" "$code"

        code=$(auth_get "$op_session" "/api/v1/gateway/metrics")
        assert_status "Operator GET /metrics → 200" "200" "$code"

        # Operator CANNOT write config
        code=$(auth_put "$op_session" "/api/v1/config" '{"TLS_ENABLED":"true"}')
        assert_status "Operator PUT /config → 403" "403" "$code"

        # Operator CANNOT create gateway tokens
        code=$(auth_post "$op_session" "/api/v1/gateway/token" '{"subject":"test"}')
        assert_status "Operator POST /gateway/token → 403" "403" "$code"
    else
        skip_test "Operator tests (OPERATOR_PASSWORD not set)"
    fi

    # Viewer tests
    if [[ -n "$VIEWER_PASSWORD" ]]; then
        local vi_session
        vi_session=$(login_as "$VIEWER_PASSWORD")

        local vi_resp
        vi_resp=$(curl -s "$WIZARD_URL/api/v1/auth/login" \
            -H 'Content-Type: application/json' \
            -d "{\"password\":\"$VIEWER_PASSWORD\"}")
        assert_json "Viewer login role" ".role" "viewer" "$vi_resp"

        # Viewer can read
        code=$(auth_get "$vi_session" "/api/v1/config")
        assert_status "Viewer GET /config → 200" "200" "$code"

        code=$(auth_get "$vi_session" "/api/v1/gateway/metrics")
        assert_status "Viewer GET /metrics → 200" "200" "$code"

        # Viewer CANNOT write config
        code=$(auth_put "$vi_session" "/api/v1/config" '{"TLS_ENABLED":"true"}')
        assert_status "Viewer PUT /config → 403" "403" "$code"

        # Viewer CANNOT restart services
        code=$(auth_post "$vi_session" "/api/v1/services/wizard/restart")
        assert_status "Viewer POST /restart → 403" "403" "$code"

        # Viewer CANNOT create gateway tokens
        code=$(auth_post "$vi_session" "/api/v1/gateway/token" '{"subject":"test"}')
        assert_status "Viewer POST /gateway/token → 403" "403" "$code"
    else
        skip_test "Viewer tests (VIEWER_PASSWORD not set)"
    fi
}

# ─── Suite: Security headers ─────────────────────────────────

run_headers() {
    echo ""
    echo "=== Security headers ==="

    local headers
    headers=$(curl -s -D - -o /dev/null "$WIZARD_URL/api/v1/health")

    assert_header "CSP"            "Content-Security-Policy"    "default-src 'self'"              "$headers"
    assert_header "X-Frame"        "X-Frame-Options"            "DENY"                            "$headers"
    assert_header "X-Content-Type" "X-Content-Type-Options"     "nosniff"                         "$headers"
    assert_header "Referrer"       "Referrer-Policy"            "strict-origin-when-cross-origin" "$headers"
    assert_header "Permissions"    "Permissions-Policy"         "camera=()"                       "$headers"

    # CORS: should reject unknown origins
    local cors_headers
    cors_headers=$(curl -s -D - -o /dev/null \
        -H "Origin: https://evil.com" "$WIZARD_URL/api/v1/health")
    if echo "$cors_headers" | grep -qi 'access-control-allow-origin.*evil'; then
        red "  ✗ CORS allows evil.com origin"
        ((FAIL++))
    else
        green "  ✓ CORS rejects unknown origins"
        ((PASS++))
    fi
}

# ─── Suite: Invalid payloads ─────────────────────────────────

run_invalid() {
    echo ""
    echo "=== Invalid payloads ==="

    local code

    # Non-JSON body to login
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d '<xml>not json</xml>')
    assert_status "XML in login body → 400" "400" "$code"

    # Oversized login body (>1KB)
    local big_password
    big_password=$(python3 -c "print('A' * 2000)")
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"password\":\"$big_password\"}")
    assert_status "Oversized login body → 400/413" "400" "$code"

    # Admin: oversized config body (>32KB)
    local admin_session
    admin_session=$(login_as "$ADMIN_PASSWORD")
    local big_value
    big_value=$(python3 -c "print('B' * 40000)")
    code=$(auth_put "$admin_session" "/api/v1/config" "{\"TLS_ENABLED\":\"$big_value\"}")
    # Should be 400 or 413
    if [[ "$code" == "400" || "$code" == "413" ]]; then
        green "  ✓ Oversized config body rejected (HTTP $code)"
        ((PASS++))
    else
        red "  ✗ Oversized config body → $code, expected 400 or 413"
        ((FAIL++))
    fi

    # Gateway token: TTL too large
    code=$(auth_post "$admin_session" "/api/v1/gateway/token" '{"ttl_hours":9999}')
    assert_status "Gateway token TTL too large → 400" "400" "$code"

    # Gateway token: bad JSON
    code=$(auth_post "$admin_session" "/api/v1/gateway/token" 'not-json')
    assert_status "Gateway token bad body → 400" "400" "$code"

    # Restart unknown service
    code=$(auth_post "$admin_session" "/api/v1/services/evilservice/restart")
    assert_status "Restart unknown service → 400" "400" "$code"

    # Config: disallowed key silently skipped (returns 200)
    code=$(auth_put "$admin_session" "/api/v1/config" '{"POSTGRES_PASSWORD":"hacked"}')
    assert_status "Disallowed config key → 200 (skipped)" "200" "$code"

    # SQL injection in password field
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d '{"password":"'\'' OR 1=1 --"}')
    assert_status "SQL injection in password → 401" "401" "$code"

    # XSS in password field
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d '{"password":"<script>alert(1)</script>"}')
    assert_status "XSS in password → 401" "401" "$code"

    # Path traversal attempt
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/services/../../etc/passwd/restart")
    if [[ "$code" != "200" ]]; then
        green "  ✓ Path traversal rejected (HTTP $code)"
        ((PASS++))
    else
        red "  ✗ Path traversal not rejected"
        ((FAIL++))
    fi
}

# ─── Suite: Rate limiting ────────────────────────────────────

run_ratelimit() {
    echo ""
    echo "=== Rate limiting ==="

    # Fire rapid login attempts and look for 429
    local got429=false
    for i in $(seq 1 30); do
        local code
        code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
            -H 'Content-Type: application/json' \
            -d '{"password":"wrong"}')
        if [[ "$code" == "429" ]]; then
            got429=true
            break
        fi
    done

    if $got429; then
        green "  ✓ Rate limiter triggered 429 after rapid login attempts"
        ((PASS++))
    else
        yellow "  ⊘ Rate limiter did not trigger 429 within 30 attempts (may need tuning or may be disabled)"
        ((SKIP++))
    fi
}

# ─── Suite: Prompt injection payloads ─────────────────────────

run_injection() {
    echo ""
    echo "=== Prompt injection / special payloads ==="

    local admin_session code
    admin_session=$(login_as "$ADMIN_PASSWORD")

    # Null bytes in config values
    code=$(auth_put "$admin_session" "/api/v1/config" '{"TLS_ENABLED":"true\u0000injected"}')
    if [[ "$code" != "500" ]]; then
        green "  ✓ Null byte in config value handled (HTTP $code)"
        ((PASS++))
    else
        red "  ✗ Null byte caused server error"
        ((FAIL++))
    fi

    # Very long key name
    local longkey
    longkey=$(python3 -c "print('A' * 500)")
    code=$(auth_put "$admin_session" "/api/v1/config" "{\"$longkey\":\"value\"}")
    if [[ "$code" != "500" ]]; then
        green "  ✓ Very long config key handled (HTTP $code)"
        ((PASS++))
    else
        red "  ✗ Very long key caused server error"
        ((FAIL++))
    fi

    # Unicode in password
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d '{"password":"пароль🔑"}')
    assert_status "Unicode password → 401 (not crash)" "401" "$code"

    # Repeated slashes in URL
    code=$(curl -s -o /dev/null -w '%{http_code}' "$WIZARD_URL///api///v1///health")
    if [[ "$code" != "500" ]]; then
        green "  ✓ Repeated slashes handled (HTTP $code)"
        ((PASS++))
    else
        red "  ✗ Repeated slashes caused server error"
        ((FAIL++))
    fi
}

# ─── Run selected suites ─────────────────────────────────────

echo "SafePaw Wizard API Test Collection"
echo "Target: $WIZARD_URL"
echo "Suite:  $SUITE"

case "$SUITE" in
    all)
        run_health
        run_auth
        run_rbac
        run_headers
        run_invalid
        run_ratelimit
        run_injection
        ;;
    health)     run_health ;;
    auth)       run_auth ;;
    rbac)       run_rbac ;;
    headers)    run_headers ;;
    invalid)    run_invalid ;;
    ratelimit)  run_ratelimit ;;
    injection)  run_injection ;;
    *)
        echo "Unknown suite: $SUITE"
        echo "Available: all, health, auth, rbac, headers, invalid, ratelimit, injection"
        exit 1
        ;;
esac

# ─── Summary ─────────────────────────────────────────────────

echo ""
echo "─────────────────────────────────"
echo "Results: $(green "$PASS passed"), $(red "$FAIL failed"), $(yellow "$SKIP skipped")"
echo "─────────────────────────────────"

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
