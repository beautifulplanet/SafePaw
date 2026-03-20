#!/bin/bash
# =============================================================
# SafePaw Protobuf Compilation Script (Linux/macOS)
# =============================================================
# Compiles .proto files into Go and TypeScript bindings.
#
# Usage:
#   ./scripts/proto-gen.sh
#
# Prerequisites:
#   - protoc (Protocol Buffer compiler)
#   - protoc-gen-go: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   - ts-proto:      npm install -g ts-proto
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROTO_DIR="$PROJECT_ROOT/shared/proto"
GEN_GO_DIR="$PROTO_DIR/gen/go"
GEN_TS_DIR="$PROTO_DIR/gen/ts"

echo "SafePaw Proto Compiler"
echo "========================="

# Check prerequisites
missing=()
command -v protoc &>/dev/null || missing+=("protoc")
command -v protoc-gen-go &>/dev/null || missing+=("protoc-gen-go")
command -v protoc-gen-ts_proto &>/dev/null || missing+=("protoc-gen-ts_proto")

if [ ${#missing[@]} -gt 0 ]; then
    echo "ERROR: Missing prerequisites: ${missing[*]}"
    exit 1
fi

# Create output directories
mkdir -p "$GEN_GO_DIR" "$GEN_TS_DIR"

# Clean previous generated files (keep go.mod/go.sum/package.json)
find "$GEN_GO_DIR" -name "*.pb.go" -delete 2>/dev/null || true
find "$GEN_TS_DIR" -name "*.ts" -delete 2>/dev/null || true

PROTO_FILES=("$PROTO_DIR"/*.proto)
echo "Found ${#PROTO_FILES[@]} .proto files"

# Compile Go bindings
echo "  -> Go bindings..."
protoc \
    --proto_path="$PROTO_DIR" \
    --go_out="$GEN_GO_DIR" \
    --go_opt=module=nopenclaw/proto \
    "${PROTO_FILES[@]}"

# Compile TypeScript bindings (using ts-proto for better TS ergonomics)
echo "  -> TypeScript bindings..."
protoc \
    --proto_path="$PROTO_DIR" \
    --plugin=protoc-gen-ts_proto="$(which protoc-gen-ts_proto)" \
    --ts_proto_out="$GEN_TS_DIR" \
    --ts_proto_opt=esModuleInterop=true \
    --ts_proto_opt=outputEncodeMethods=false \
    --ts_proto_opt=outputJsonMethods=true \
    "${PROTO_FILES[@]}"

# Verify Go compiles
echo "  -> Verifying Go compilation..."
pushd "$GEN_GO_DIR" >/dev/null
if [ ! -f go.mod ]; then
    go mod init nopenclaw/proto
    go mod tidy
fi
go build ./...
popd >/dev/null

# Summary
GO_COUNT=$(find "$GEN_GO_DIR" -name "*.pb.go" | wc -l)
TS_COUNT=$(find "$GEN_TS_DIR" -name "*.ts" | wc -l)
echo ""
echo "Proto compilation complete!"
echo "  Go:         $GO_COUNT files in shared/proto/gen/go/"
echo "  TypeScript: $TS_COUNT files in shared/proto/gen/ts/"
