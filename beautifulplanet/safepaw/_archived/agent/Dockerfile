# =============================================================
# NOPEnclaw Agent — Multi-stage Dockerfile
# =============================================================
# Build: node:22-alpine with TypeScript compilation
# Run: node:22-alpine with production deps only
#
# Pattern mirrors Router/Gateway: non-root user, health check,
# minimal final image.
# =============================================================

# --- Stage 1: Build ---
FROM node:22-alpine AS builder

WORKDIR /build

# Copy dependency files first (layer caching)
COPY package.json package-lock.json ./

# Install ALL deps (including devDependencies for tsc)
RUN npm ci

# Copy source code and TypeScript config
COPY tsconfig.json ./
COPY src/ ./src/

# Compile TypeScript → JavaScript
RUN npx tsc

# --- Stage 2: Run ---
FROM node:22-alpine

# Security: non-root user (same pattern as Gateway/Router)
RUN adduser -D -g '' nopenclaw

WORKDIR /app

# Copy package files for production install
COPY package.json package-lock.json ./

# Install production dependencies only (no typescript, no @types/*)
RUN npm ci --omit=dev

# Copy compiled JavaScript from build stage
COPY --from=builder /build/dist/ ./dist/

USER nopenclaw

EXPOSE 8082

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8082/health || exit 1

ENTRYPOINT ["node", "dist/main.js"]
