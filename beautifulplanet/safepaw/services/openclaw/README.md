# OpenClaw service (SafePaw)

Dockerfile for the **OpenClaw** AI assistant, used by the InstallerClaw/SafePaw stack.

- **Build-time install** — `openclaw` is installed when the image is built, not at container start. Faster startup and reproducible builds.
- **Non-root** — Runs as the `node` user (from the official Node image).
- **Pinned base** — Uses the same `node:22-alpine@sha256:...` digest as in docker-compose for supply-chain consistency.
- **Version pinning** — Set `OPENCLAW_VERSION` (e.g. `1.2.3`) in `.env` or as a build-arg to lock the openclaw version.

OpenClaw is never exposed to the host; all traffic reaches it through the SafePaw gateway.
