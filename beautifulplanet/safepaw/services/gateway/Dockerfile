# =============================================================
# NOPEnclaw Gateway — Multi-stage Dockerfile
# =============================================================
# Stage 1: Build the Go binary (large image, discarded)
# Stage 2: Run in minimal Alpine (tiny attack surface)
#
# OPSEC: Multi-stage builds mean the final image has NO compiler,
# NO source code, NO build tools — just the binary + certs.
# An attacker who somehow gets into the container finds nothing useful.
# =============================================================

# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

# Security: don't run build as root
RUN adduser -D -g '' builder

WORKDIR /build

# Copy dependency files first (layer caching — saves rebuild time)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build a static binary (CGO_ENABLED=0 = no C dependencies)
# -ldflags="-s -w" strips debug symbols (smaller binary, harder to reverse)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /build/gateway .

# --- Stage 2: Run ---
FROM alpine:3.19

# Security: install CA certs (for TLS) and create non-root user
RUN apk --no-cache add ca-certificates \
    && adduser -D -g '' nopenclaw

WORKDIR /app

# Copy ONLY the binary from the build stage
COPY --from=builder /build/gateway /app/gateway

# Run as non-root user (OPSEC: principle of least privilege)
USER nopenclaw

# Expose the gateway port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# Start the gateway
ENTRYPOINT ["/app/gateway"]
