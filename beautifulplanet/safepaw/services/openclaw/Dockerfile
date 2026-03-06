# =============================================================
# SafePaw OpenClaw — Dedicated Dockerfile
# =============================================================
# Runs the OpenClaw AI assistant inside the InstallerClaw/SafePaw
# perimeter. OpenClaw is NOT exposed to the host; all traffic
# goes through the gateway.
#
# Safety:
#   - Pinned base image (supply-chain integrity)
#   - openclaw installed at BUILD time (reproducible, no runtime npm)
#   - Runs as non-root (node user from base image)
#   - Minimal Alpine base
#   - Version pinning via OPENCLAW_VERSION build-arg
# =============================================================

# Pinned digest for supply-chain safety (match docker-compose)
ARG NODE_IMAGE=node:22-alpine@sha256:e4bf2a82ad0a4037d28035ae71529873c069b13eb0455466ae0bc13363826e34
FROM ${NODE_IMAGE}

# Pin openclaw version at build time (override with --build-arg OPENCLAW_VERSION=x.y.z)
ARG OPENCLAW_VERSION
ENV OPENCLAW_VERSION=${OPENCLAW_VERSION}

# openclaw or its deps may use git (e.g. postinstall); gosu for entrypoint drop-to-node
RUN apk add --no-cache git gosu

# Skip node-llama-cpp native build (optional local LLM); use API providers (Anthropic/OpenAI) instead
ENV NODE_LLAMA_CPP_SKIP_DOWNLOAD=true

# Install openclaw globally at build time (not at container start)
RUN npm install -g openclaw@${OPENCLAW_VERSION} \
    && npm cache clean --force

# Config and workspace are mounted at /home/node/.openclaw; ensure node can write (volume may be root-owned)
COPY entrypoint.sh /entrypoint.sh
RUN sed -i 's/\r$//' /entrypoint.sh && chmod +x /entrypoint.sh

WORKDIR /home/node
EXPOSE 18789

HEALTHCHECK --interval=15s --timeout=10s --retries=5 --start-period=30s \
    CMD wget -qO- http://localhost:18789/health || exit 1

# Entrypoint runs as root, chowns volume, then exec openclaw as node
ENTRYPOINT ["/entrypoint.sh"]
CMD ["openclaw", "gateway", "--port", "18789", "--bind", "lan"]
