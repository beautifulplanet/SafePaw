# =============================================================
# SafePaw Gateway - Multi-stage Dockerfile
# =============================================================
# Secure reverse proxy in front of OpenClaw.
# Stage 1: Build the Go binary (large image, discarded)
# Stage 2: Run in minimal Alpine (tiny attack surface)
#
# Multi-stage builds mean the final image has NO compiler,
# NO source code, NO build tools — just the binary + certs.
# =============================================================

# --- Stage 1: Build ---
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy dependency files first (layer caching)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build a static binary (CGO_ENABLED=0 = no C dependencies)
# -ldflags="-s -w" strips debug symbols (smaller binary)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /build/gateway .

# --- Stage 2: Run ---
FROM alpine:3.19

# Security: install CA certs (for proxying TLS) and create non-root user
RUN apk --no-cache add ca-certificates \
    && adduser -D -g '' safepaw

WORKDIR /app

COPY --from=builder /build/gateway /app/gateway

# Run as non-root user (principle of least privilege)
USER safepaw

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/gateway"]
