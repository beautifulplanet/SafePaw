# =============================================================
# SafePaw Wizard - Multi-stage Dockerfile
# =============================================================
# Setup wizard UI: guided OpenClaw configuration on :3000.
# The React SPA is baked into the Go binary via go:embed,
# so the final image has NO Node.js, NO nginx — just one binary.
#
# Stage 1: Build the Go binary (with embedded UI assets)
# Stage 2: Run in minimal Alpine (tiny attack surface)
# =============================================================

# --- Stage 1: Build ---
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy dependency files first (layer caching)
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null; true

# Copy source code (including ui/dist/ for go:embed)
COPY . .

# Build a static binary (CGO_ENABLED=0 = no C dependencies)
# -ldflags="-s -w" strips debug symbols (smaller binary)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /build/wizard ./cmd/wizard

# --- Stage 2: Run ---
FROM alpine:3.19

# Security: install CA certs (for health check HTTP calls) and create non-root user
RUN apk --no-cache add ca-certificates \
    && adduser -D -g '' safepaw

WORKDIR /app

COPY --from=builder /build/wizard /app/wizard

# Run as non-root user (principle of least privilege)
USER safepaw

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:3000/api/v1/health || exit 1

ENTRYPOINT ["/app/wizard"]
