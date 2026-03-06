#!/usr/bin/env bash
# Pre-commit hook for SafePaw — mirrors CI checks locally.
# Install:  make install-hooks   (or:  ln -sf ../../beautifulplanet/safepaw/scripts/pre-commit.sh .git/hooks/pre-commit)
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
SP="$ROOT/beautifulplanet/safepaw"
GATEWAY="$SP/services/gateway"
WIZARD="$SP/services/wizard"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

pass() { printf "${GREEN}✓ %s${NC}\n" "$1"; }
fail() { printf "${RED}✗ %s${NC}\n" "$1"; exit 1; }
info() { printf "${YELLOW}● %s${NC}\n" "$1"; }

# ── 1. Build ───────────────────────────────────────────
info "Building gateway..."
(cd "$GATEWAY" && go build ./...) || fail "Gateway build failed"
pass "Gateway builds"

info "Building wizard..."
(cd "$WIZARD" && go build ./...) || fail "Wizard build failed"
pass "Wizard builds"

# ── 2. Vet ─────────────────────────────────────────────
info "Vetting gateway..."
(cd "$GATEWAY" && go vet ./...) || fail "Gateway vet failed"
pass "Gateway vet"

info "Vetting wizard..."
(cd "$WIZARD" && go vet ./...) || fail "Wizard vet failed"
pass "Wizard vet"

# ── 3. Test (race detector) ───────────────────────────
info "Testing gateway..."
(cd "$GATEWAY" && go test -race -count=1 ./...) || fail "Gateway tests failed"
pass "Gateway tests"

info "Testing wizard..."
(cd "$WIZARD" && go test -race -count=1 ./...) || fail "Wizard tests failed"
pass "Wizard tests"

# ── 4. Lint (if golangci-lint is installed) ────────────
if command -v golangci-lint &>/dev/null; then
  info "Linting gateway..."
  (cd "$GATEWAY" && golangci-lint run --timeout=5m) || fail "Gateway lint failed"
  pass "Gateway lint"

  info "Linting wizard..."
  (cd "$WIZARD" && golangci-lint run --timeout=5m) || fail "Wizard lint failed"
  pass "Wizard lint"
else
  info "golangci-lint not found — skipping lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)"
fi

# ── 5. Security scan (if gosec is installed) ───────────
if command -v gosec &>/dev/null; then
  info "Security scan gateway..."
  (cd "$GATEWAY" && gosec -quiet -exclude=G104,G706 -exclude-dir=tools ./...) || fail "Gateway gosec failed"
  pass "Gateway gosec"

  info "Security scan wizard..."
  (cd "$WIZARD" && gosec -quiet -exclude=G704,G706 ./...) || fail "Wizard gosec failed"
  pass "Wizard gosec"
else
  info "gosec not found — skipping security scan (install: go install github.com/securego/gosec/v2/cmd/gosec@latest)"
fi

printf "\n${GREEN}All pre-commit checks passed.${NC}\n"
