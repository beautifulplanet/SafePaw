#!/bin/bash
# =============================================================
# NOPEnclaw Protobuf Compilation Script
# =============================================================
# Compiles .proto files into Go and TypeScript bindings.
#
# Prerequisites:
#   - protoc (Protocol Buffer compiler)
#   - protoc-gen-go (Go plugin)
#   - protoc-gen-ts (TypeScript plugin via ts-proto or protobuf-ts)
#
# Install (inside devcontainer):
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   npm install -g ts-proto
# =============================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROTO_DIR="$PROJECT_ROOT/shared/proto"
GEN_GO_DIR="$PROTO_DIR/gen/go"
GEN_TS_DIR="$PROTO_DIR/gen/ts"

echo "🔧 NOPEnclaw Proto Compiler"
echo "========================="

# Check prerequisites
if ! command -v protoc &> /dev/null; then
    echo "❌ protoc not found. Install: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

# Create output directories
mkdir -p "$GEN_GO_DIR" "$GEN_TS_DIR"

# Clean previous generated files
rm -rf "${GEN_GO_DIR:?}/"* "${GEN_TS_DIR:?}/"*

echo "📦 Compiling .proto files..."

# Compile Go bindings
echo "  → Go bindings..."
protoc \
    --proto_path="$PROTO_DIR" \
    --go_out="$GEN_GO_DIR" \
    --go_opt=paths=source_relative \
    "$PROTO_DIR"/*.proto

# Compile TypeScript bindings (using ts-proto for better TS ergonomics)
echo "  → TypeScript bindings..."
protoc \
    --proto_path="$PROTO_DIR" \
    --plugin=protoc-gen-ts_proto="$(which protoc-gen-ts_proto)" \
    --ts_proto_out="$GEN_TS_DIR" \
    --ts_proto_opt=esModuleInterop=true \
    --ts_proto_opt=outputEncodeMethods=false \
    --ts_proto_opt=outputJsonMethods=true \
    "$PROTO_DIR"/*.proto

echo ""
echo "✅ Proto compilation complete!"
echo "   Go:         $GEN_GO_DIR/"
echo "   TypeScript: $GEN_TS_DIR/"
echo ""
echo "📋 Generated files:"
find "$GEN_GO_DIR" "$GEN_TS_DIR" -type f 2>/dev/null | sed 's|'"$PROJECT_ROOT"'/||'
