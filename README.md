# NOPEnclaw

A security-hardened, self-hosted messaging infrastructure. Routes messages between platforms (Discord, Telegram, Slack, etc.) through a unified pipeline with end-to-end observability.

## Architecture

```
User ──► Gateway (WebSocket) ──► Redis Streams ──► Router ──► Agent ──► Redis Streams ──► Gateway ──► User
```

| Service | Language | Status |
|---------|----------|--------|
| **Gateway** | Go | Working — WebSocket server, Redis Streams bridge, auth middleware, TLS |
| **Router** | Go | In progress — Redis consumer, message routing |
| **Agent** | TypeScript | Not started — echo service first, then LLM integration |
| **Wizard** | Go + React | Scaffolded — paused until the core loop is reliable |

## What works today

- **Gateway** accepts WebSocket connections, authenticates via HMAC-SHA256 tokens, and bridges messages to/from Redis Streams
- **Protobuf pipeline** generates Go and TypeScript bindings from 4 schema files (`common`, `message`, `user`, `channel_config`)
- **Docker Compose** runs Gateway + Redis + Postgres with security defaults (localhost-only ports, non-root containers, pinned image digests)
- **Security middleware**: rate limiting, origin checking, security headers, request ID tracing

## What doesn't work yet

- The full message loop (Gateway → Router → Agent → Gateway) — Router and Agent are in progress
- Auth token revocation (stateless tokens only, no revocation cache)
- Egress filtering on agent containers
- Integration tests

## Quick start

```bash
cp beautifulplanet/safepaw/.env.example beautifulplanet/safepaw/.env
# Edit .env with your passwords

cd beautifulplanet/safepaw
docker compose up -d
```

Gateway will be available at `ws://localhost:8080/ws`.

## Project structure

```
beautifulplanet/safepaw/
├── services/
│   ├── gateway/        # WebSocket entry point (Go)
│   ├── router/         # Message routing engine (Go) — in progress
│   ├── agent/          # Message processor (TS) — not started
│   ├── wizard/         # Setup UI (Go + React) — paused
│   └── postgres/       # DB init scripts
├── shared/
│   └── proto/          # Protobuf schemas + generated code
│       ├── *.proto     # Source schemas
│       └── gen/        # Generated Go + TypeScript bindings
├── scripts/            # Build & code generation scripts
├── docker-compose.yml
└── .env.example
```

## Regenerating proto bindings

```powershell
# Windows
.\scripts\proto-gen.ps1

# Linux/macOS
./scripts/proto-gen.sh

# Docker (no local tools needed)
docker build -f shared/proto/Dockerfile.protoc --output shared/proto/gen .
```

## Security posture

See individual service READMEs for details. Key decisions:
- All inter-service traffic stays on a Docker bridge network (no host-exposed ports except Gateway)
- Redis commands `FLUSHALL`, `FLUSHDB`, `DEBUG` are disabled
- Container images pinned by SHA256 digest
- Gateway binds to `127.0.0.1` only (not `0.0.0.0`)
- Pre-commit hook blocks secrets from entering git history

## License

Private — not yet open source.
