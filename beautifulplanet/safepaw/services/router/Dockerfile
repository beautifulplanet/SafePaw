# =============================================================
# NOPEnclaw Router — Multi-stage Dockerfile
# =============================================================
# Same pattern as Gateway: build in golang:1.26-alpine, run in
# alpine:3.19 with non-root user and health check.
# =============================================================

# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy shared proto module first (needed by replace directive)
COPY shared/proto/gen/go/ /shared/proto/gen/go/

# Copy dependency files (layer caching)
COPY services/router/go.mod services/router/go.sum* ./
RUN go mod download 2>/dev/null; true

# Copy source code
COPY services/router/ .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /build/router ./cmd/router

# --- Stage 2: Run ---
FROM alpine:3.19

RUN apk --no-cache add ca-certificates \
    && adduser -D -g '' nopenclaw

WORKDIR /app

COPY --from=builder /build/router /app/router

USER nopenclaw

EXPOSE 8081

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8081/health || exit 1

ENTRYPOINT ["/app/router"]
