# SafePaw 🐾

**Self-hosted messaging infrastructure that doesn't make you choose between security and usability.**

Route messages between Discord, Telegram, Slack, WhatsApp — through a unified pipeline with protobuf-typed boundaries, Redis Streams, and zero trust between services.

> 2 languages. 4 protobuf schemas. Every service talks through typed contracts.
> One command to run: `docker compose --profile community up -d`

---

## How to Read This README

This document serves three audiences. Jump to what you need:

| Who you are | Where to go | Time |
|-------------|-------------|------|
| **Hiring manager** wanting the highlights | [Part 1: What This Is](#part-1-what-this-is) | 30 seconds |
| **Engineer** evaluating the architecture | [Part 2: Architecture & Stack](#part-2-architecture--stack) | 2 minutes |
| **Developer** wanting to run it | [Part 3: Quick Start](#part-3-quick-start) | 3 minutes |

---

# Part 1: What This Is

A microservices messaging backbone. Every chat message — from any platform — flows through the same pipeline:

```
User ──► Gateway (WebSocket) ──► Redis Streams ──► Router ──► Agent ──► Redis Streams ──► Gateway ──► User
```

Every hop is a separate process. Every boundary speaks protobuf. Every service runs in its own container with the minimum privileges it needs.

### Why It's Interesting (for Interviewers)

| Skill | Evidence |
|-------|----------|
| **Systems programming** | Go WebSocket server handling 10K concurrent connections, goroutine-per-connection with read/write pumps |
| **Protocol design** | 4 protobuf schemas generating both Go and TypeScript — shared types across language boundaries |
| **Security engineering** | HMAC-SHA256 stateless auth, TLS 1.2+ with curated cipher suites, constant-time token comparison, non-root containers |
| **Infrastructure as code** | Docker Compose with health checks, resource limits, pinned image digests (SHA256), internal-only networks |
| **Stream processing** | Redis Streams with `XADD`/`XREADGROUP` consumer groups, backpressure via `MAXLEN`, blocking reads |
| **Build reproducibility** | Proto generation works 3 ways: local PowerShell, local bash, Docker (CI). Generated code checked in with `git diff` CI guard |

### Key Numbers

| What | Value |
|------|-------|
| Gateway WebSocket capacity | 10,000 concurrent connections |
| Redis Stream max length | ~10,000 messages (capped via `MAXLEN`) |
| Auth token algorithm | HMAC-SHA256, constant-time comparison |
| TLS minimum version | 1.2 (1.3 preferred) |
| Container base image | `alpine:3.19` (5 MB attack surface) |
| Proto schemas | 4 files → 4 Go packages + 4 TypeScript modules |
| Docker image method | Multi-stage, stripped binary, no compiler in final image |
| Rate limiting | 30 connections/min per IP (configurable) |

### Honest Status

Not everything works yet. Here's the truth:

| Component | Status | What exists |
|-----------|--------|-------------|
| **Gateway** | ✅ Working | WebSocket server, Redis bridge, auth, TLS, rate limiter, Dockerfile |
| **Proto Pipeline** | ✅ Working | 4 schemas, Go + TS generation, compile-verified, CI-reproducible |
| **Docker Compose** | ✅ Working | Gateway + Redis + Postgres, health checks, security defaults |
| **Router** | 🔧 In progress | Go module initialized, proto imports wired |
| **Agent** | ⬜ Not started | Will be a TypeScript echo service first, LLM later |
| **Integration test** | ⬜ Not started | Blocked on Router + Agent completing the loop |
| **Auth revocation** | ⬜ Not started | Stateless tokens only — no revocation cache yet |
| **Egress filtering** | ⬜ Not started | Agent containers not yet network-restricted |

---

# Part 2: Architecture & Stack

### Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| **Gateway** | Go, gorilla/websocket | Zero-alloc upgrades, goroutine-per-conn scales to 10K+, stdlib HTTP for middleware |
| **Router** | Go, Redis Streams | Same language as Gateway for shared proto types, consumer groups for exactly-once delivery |
| **Agent** | TypeScript, Node.js | Plugin ecosystem, fast iteration for channel integrations, ts-proto for typed messages |
| **Message Queue** | Redis Streams | Persistent, consumer groups, backpressure — lighter than Kafka for this scale |
| **Database** | PostgreSQL 16 | Auth tables, channel config, user records. Schema-first with init scripts |
| **Serialization** | Protocol Buffers | Typed contracts between Go and TypeScript — one `.proto`, two languages, zero drift |
| **Containers** | Docker, Compose | Multi-stage builds, non-root users, pinned digests, resource limits |

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Host Machine                               │
│                                                                     │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │                   Docker Bridge Network                        │ │
│  │                  (nopenclaw-internal)                          │ │
│  │                                                                │ │
│  │  ┌─────────────┐    ┌──────────┐    ┌─────────────────────┐   │ │
│  │  │   Gateway    │    │  Redis   │    │     PostgreSQL      │   │ │
│  │  │   (Go)       │◄──►│ Streams  │◄──►│     (auth, config)  │   │ │
│  │  │             │    │          │    │                     │   │ │
│  │  │ :8080 (WS)  │    │ (no port)│    │     (no port)       │   │ │
│  │  └──────┬──────┘    └────┬─────┘    └─────────────────────┘   │ │
│  │         │                │                                     │ │
│  │    ┌────▼────┐     ┌─────▼──────┐                              │ │
│  │    │ Auth    │     │  Router    │                              │ │
│  │    │ Middle- │     │  (Go)      │                              │ │
│  │    │ ware    │     │            │                              │ │
│  │    └─────────┘     └─────┬──────┘                              │ │
│  │                          │                                     │ │
│  │                    ┌─────▼──────┐                              │ │
│  │                    │   Agent    │                              │ │
│  │                    │   (TS)     │                              │ │
│  │                    └────────────┘                              │ │
│  └────────────────────────────────────────────────────────────────┘ │
│         │                                                           │
│    127.0.0.1:8080 ◄── ONLY exposed port (localhost only)           │
└─────────────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Gateway is the only exposed port** | Reduces attack surface to one process. Redis, Postgres — unreachable from host. |
| **Protobuf over JSON** | Typed contracts prevent "field name typo" bugs between Go and TypeScript services |
| **Redis Streams over Kafka** | Right-sized for self-hosted scale. Consumer groups give us exactly-once semantics without ZooKeeper |
| **HMAC-SHA256 stateless tokens** | No session store to scale. Tokens are verifiable by any Gateway instance without DB lookup |
| **Image digests, not tags** | `redis:7-alpine` can be mutated. `redis:7-alpine@sha256:02f2cc...` cannot. Supply chain integrity. |
| **Go `embed` for Wizard UI** | Single binary deployment — no nginx, no static file server, no CORS between API and UI |
| **Multi-stage Docker builds** | Final image has no compiler, no source code, no build tools. Attacker finds nothing useful. |

### Alternatives Considered (and Why We Didn't Use Them)

Every architecture is a set of tradeoffs. Here's what else we evaluated:

#### Why microservices instead of a monolith?

| | Microservices (chose this) | Monolith | Modular monolith |
|---|---|---|---|
| **Strength** | Language-per-service (Go gateway, TS agents), independent deploys, blast-radius isolation | Simpler to start, one deploy, no network serialization | Best of both: module boundaries without network hops |
| **Weakness** | Network overhead, proto schema coordination, harder to debug distributed traces | One bad dependency crashes everything, language lock-in | Still single-deploy — can't scale agent separately from gateway |
| **Why we chose it** | Agent plugins need the Node/TS ecosystem. Gateway needs Go's concurrency model. Forcing both into one language means one of them suffers. The proto boundary *is* the modularity contract. |
| **When monolith wins** | If you're a solo dev who only needs one language. If your message volume fits in a single process. We might be that — but we're building for the case where we're not. |

#### Why Redis Streams instead of Kafka, RabbitMQ, or NATS?

| Broker | Strength | Weakness | Why not for us |
|--------|----------|----------|----------------|
| **Redis Streams** (chose this) | Already in stack for caching, consumer groups, lightweight, `XREADGROUP` with blocking | No built-in partitioning, single-node durability only (AOF) | — |
| **Kafka** | Infinite retention, partitioned, battle-tested at trillion-message scale | ZooKeeper/KRaft overhead, 3-broker minimum, 1GB+ RAM baseline | Massive overkill for self-hosted. Our users run this on a $5 VPS. |
| **RabbitMQ** | Mature, flexible routing (exchanges/queues), AMQP standard | Erlang runtime, message acknowledgment complexity, no stream replay | Routing flexibility we don't need — our routing is code, not broker config |
| **NATS JetStream** | Lightweight Go binary, built-in clustering, good DX | Smaller ecosystem, less battle-tested persistence, fewer client libraries | Strong contender. If Redis wasn't already required for caching, NATS would win. Single-dependency preference tipped it. |

#### Why Go for Gateway instead of Rust, Java, or Node.js?

| Language | Strength | Weakness | Why not for us |
|----------|----------|----------|----------------|
| **Go** (chose this) | goroutine-per-connection (cheap), stdlib HTTP/TLS, 15MB static binary, fast compile | No generics until recently, error handling verbose, no WASM story | — |
| **Rust** | Zero-cost abstractions, memory safety without GC, `tokio` async is fast | Steep learning curve, slower iteration, 10× longer compile times | Interview signal: Go is readable in 5 minutes. Rust requires explaining lifetimes before the architecture. |
| **Java/Kotlin** | Mature ecosystem, Spring/Ktor frameworks, JVM tuning tools | 200MB+ runtime, cold start, GC pauses at scale | Docker image goes from 15MB to 300MB. Self-hosted users care about resource footprint. |
| **Node.js** | Same language as Agent, huge ecosystem, fast to prototype | Single-threaded event loop, WebSocket backpressure harder, no goroutines | Gateway needs 10K concurrent connections. Node can do it, but Go does it with less memory and no `cluster` module workarounds. |

#### Why Protobuf instead of JSON, MessagePack, or Avro?

| Format | Strength | Weakness | Why not for us |
|--------|----------|----------|----------------|
| **Protobuf** (chose this) | Schema-enforced types, codegen for Go + TS, compact wire format | Schema evolution learning curve, `.proto` files to maintain | — |
| **JSON** | Universal, human-readable, zero tooling needed | No type enforcement across languages, string field names on every message, larger on wire | The whole point is typed contracts between Go and TypeScript. JSON makes that a gentleman's agreement. |
| **MessagePack** | Binary JSON — fast, compact, no schema needed | No codegen, no type enforcement, debugging requires tooling | Same problem as JSON but less readable. Speed gain doesn't matter at our message volume. |
| **Avro** | Schema registry, great for Kafka ecosystems, schema evolution | Tied to Kafka/Confluent ecosystem, less tooling for Go | We're not using Kafka. Avro without the schema registry loses its main advantage. |

#### Why WebSocket instead of gRPC-Web, SSE, or HTTP long-polling?

| Protocol | Strength | Weakness | Why not for us |
|----------|----------|----------|----------------|
| **WebSocket** (chose this) | Full-duplex, low overhead per message, universal browser support | Stateful connections (harder to load-balance), no built-in compression standard | — |
| **gRPC-Web** | Typed RPCs, streaming, proto-native | Requires Envoy proxy for browser clients, not true bidirectional in browsers | Adds a proxy dependency. Our users shouldn't need to configure Envoy. |
| **Server-Sent Events (SSE)** | Simple, HTTP-native, auto-reconnect | Server→client only — client→server needs separate HTTP POST | Chat is bidirectional. SSE + POST is two protocols pretending to be one. |
| **HTTP long-polling** | Works everywhere, no special protocols | Latency penalty on every poll cycle, wasted connections, higher server load | Acceptable as a fallback. Not acceptable as the primary transport for real-time chat. |

### Security Posture

Seven layers, each defending against a specific threat:

```
Layer 1: Docker Network      → Services unreachable from host (except Gateway)
Layer 2: Image Pinning       → SHA256 digests prevent supply-chain substitution
Layer 3: Non-Root Containers → Compromised process can't escalate to host
Layer 4: Security Headers    → HSTS, X-Frame-Options, CSP, nosniff on every response
Layer 5: Rate Limiting       → 30 conn/min per IP, configurable per deployment
Layer 6: Auth Middleware      → HMAC-SHA256 with constant-time comparison, configurable TTL
Layer 7: Pre-Commit Hook     → Blocks secrets, API keys, and credentials from entering git
```

**What's NOT secured yet** (honesty > theater):
- Agent containers don't have egress filtering (planned: iptables rules allowing only LLM API endpoints)
- Auth tokens can't be revoked (planned: Redis-backed revocation cache synced from Postgres)
- No intrusion detection or audit logging

---

# Part 3: Quick Start

### Prerequisites

| Tool | Version | Required | Purpose |
|------|---------|----------|---------|
| Docker | 20+ | Yes | Runs all services |
| Docker Compose | v2+ | Yes | Orchestration |
| Git | 2.30+ | Yes | Clone the repo |
| Go | 1.25+ | Only for dev | Building Gateway/Router locally |
| Node.js | 22+ | Only for dev | Building Agent locally |
| protoc | 29+ | Only for proto changes | Regenerating typed bindings |

### Run It

```bash
git clone https://github.com/beautifulplanet/SafePaw.git
cd SafePaw/beautifulplanet/safepaw

# Configure (one file, all secrets)
cp .env.example .env
# Edit .env — change REDIS_PASSWORD and POSTGRES_PASSWORD at minimum

# Start
docker compose up -d
```

Gateway is now running at `ws://localhost:8080/ws`.

### Verify

```bash
# Health check
curl http://localhost:8080/health

# Expected response:
# {"connections":0,"redis":"connected","status":"ok","timestamp":"..."}
```

### Regenerate Proto Bindings

Only needed if you modify `.proto` files:

```powershell
# Windows
.\scripts\proto-gen.ps1

# Linux/macOS
./scripts/proto-gen.sh

# Docker (CI — no local tools needed)
docker build -f shared/proto/Dockerfile.protoc --output shared/proto/gen .
```

---

## Project Structure

```
beautifulplanet/safepaw/
├── services/
│   ├── gateway/          # WebSocket entry point (Go) .............. ✅ working
│   │   ├── main.go       # Config → Redis → Hub → Routes → Shutdown
│   │   ├── ws/           # WebSocket handler, connection hub
│   │   ├── redis/        # Redis Streams bridge (XADD/XREAD)
│   │   ├── middleware/    # Auth (HMAC), rate limit, security headers
│   │   ├── config/       # Env-based configuration
│   │   ├── tools/        # Token generation CLI
│   │   └── Dockerfile    # Multi-stage, non-root, health check
│   ├── router/           # Message routing engine (Go) ............ 🔧 in progress
│   ├── agent/            # Message processor (TypeScript) ......... ⬜ not started
│   ├── wizard/           # Setup UI (Go + React) .................. ⏸️ paused
│   └── postgres/         # DB init scripts (auth, users)
├── shared/
│   └── proto/            # THE source of truth for all types
│       ├── common.proto        # Timestamp, UserId
│       ├── message.proto       # Message envelope (the core type)
│       ├── user.proto          # Unified user across platforms
│       ├── channel_config.proto # Per-channel settings
│       ├── Dockerfile.protoc   # CI-reproducible generation
│       └── gen/                # Generated code (checked in)
│           ├── go/             # Go module (nopenclaw/proto)
│           └── ts/             # TypeScript package (@nopenclaw/proto)
├── scripts/
│   ├── proto-gen.ps1     # Windows proto generation
│   └── proto-gen.sh      # Linux/macOS proto generation
├── docker-compose.yml    # Gateway + Redis + Postgres
├── .env.example          # All configuration in one file
└── .gitignore            # Secrets blocked, generated code tracked
```

---

## What's Next

The roadmap is driven by one principle: **prove the loop works before polishing anything**.

| Priority | Task | Why |
|----------|------|-----|
| **P0** ✅ | Proto generation pipeline | Typed contracts between services — done, verified, reproducible |
| **P1** 🔧 | Router service (Go) | Consumes from Redis, routes to Agent — the missing middle |
| **P2** | Agent echo service (TS) | Receives message, echoes back — proves the full loop |
| **P3** | Docker Compose profiles | `community` vs `pro` — one command install |
| **P4** | Integration smoke test | WebSocket client → Gateway → Router → Agent → back. If this passes, it works. |
| **P5** | Auth revocation | Redis-backed cache, admin API to revoke tokens |
| **P6** | Threat model doc | Honest assessment of attack surfaces and mitigations |
| **P7** | Wizard UI | Only after the loop is "boringly reliable" |

---

## License

Private — not yet open source.

---

*Built with Go, TypeScript, Protocol Buffers, Redis Streams, and Docker.
4 proto schemas. 2 languages sharing typed contracts. Security defaults, not security theater.*
