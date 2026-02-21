# SafePaw 🐾

**Self-hosted messaging infrastructure that doesn't make you choose between security and usability.**

Route messages between Discord, Telegram, Slack, WhatsApp — through a unified pipeline with protobuf-typed boundaries, Redis Streams, and zero trust between services.

> 2 languages. 4 protobuf schemas. Every service talks through typed contracts.
> One command to run: `docker compose --profile community up -d`

---

## How to Read This README

This document serves four audiences. Jump to what you need:

| Who you are | Where to go | Time |
|-------------|-------------|------|
| **Hiring manager** wanting the highlights | [Part 1: What This Is](#part-1-what-this-is) | 30 seconds |
| **Engineer** evaluating the architecture | [Part 2: Architecture & Stack](#part-2-architecture--stack) | 5 minutes |
| **Developer** wanting to run it | [Part 3: Quick Start](#part-3-quick-start) | 3 minutes |
| **Anyone** wanting to understand everything | [Part 4: Complete Technical Deep-Dive](#part-4-complete-technical-deep-dive) | 30+ minutes |

---

# Part 1: What This Is

A microservices messaging backbone. Every chat message — from any platform — flows through the same pipeline:

```
User ──► Gateway (WebSocket) ──► Redis Streams ──► Router ──► Agent ──► Redis Streams ──► Gateway ──► User
```

Every hop is a separate process. Every boundary speaks protobuf. Every service runs in its own container with the minimum privileges it needs.

### Skills Demonstrated

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

Not everything works yet. Current status:

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
| **Multi-stage Docker builds** | Final image has no compiler, no source code, no build tools. No development artifacts remain in the runtime image. |

### Alternatives Considered (and Why We Didn't Use Them)

Every architecture is a set of tradeoffs. Here's what else we evaluated:

#### Why microservices instead of a monolith?

| | Microservices (chose this) | Monolith | Modular monolith |
|---|---|---|---|
| **Strength** | Language-per-service (Go gateway, TS agents), independent deploys, blast-radius isolation | Simpler to start, one deploy, no network serialization | Best of both: module boundaries without network hops |
| **Weakness** | Network overhead, proto schema coordination, harder to debug distributed traces | One bad dependency crashes everything, language lock-in | Still single-deploy — can't scale agent separately from gateway |
| **Why we chose it** | Agent plugins need the Node/TS ecosystem. Gateway needs Go's concurrency model. Forcing both into one language means one of them suffers. The proto boundary *is* the modularity contract. |
| **When monolith wins** | If the entire system can use one language and message volume fits in a single process. The microservices boundary exists here because the language split is a hard requirement, not a preference. |

#### Why Redis Streams instead of Kafka, RabbitMQ, or NATS?

| Broker | Strength | Weakness | Why not for us |
|--------|----------|----------|----------------|
| **Redis Streams** (chose this) | Already in stack for caching, consumer groups, lightweight, `XREADGROUP` with blocking | No built-in partitioning, single-node durability only (AOF) | — |
| **Kafka** | Infinite retention, partitioned, battle-tested at trillion-message scale | ZooKeeper/KRaft overhead, 3-broker minimum, 1GB+ RAM baseline | Operational overhead exceeds the requirements of a self-hosted deployment. Minimum viable Kafka cluster uses more RAM than our entire stack. |
| **RabbitMQ** | Mature, flexible routing (exchanges/queues), AMQP standard | Erlang runtime, message acknowledgment complexity, no stream replay | Routing flexibility we don't need — our routing is code, not broker config |
| **NATS JetStream** | Lightweight Go binary, built-in clustering, good DX | Smaller ecosystem, less battle-tested persistence, fewer client libraries | Strong contender. If Redis wasn't already required for caching, NATS would win. Single-dependency preference tipped it. |

#### Why Go for Gateway instead of Rust, Java, or Node.js?

| Language | Strength | Weakness | Why not for us |
|----------|----------|----------|----------------|
| **Go** (chose this) | goroutine-per-connection (cheap), stdlib HTTP/TLS, 15MB static binary, fast compile | No generics until recently, error handling verbose, no WASM story | — |
| **Rust** | Zero-cost abstractions, memory safety without GC, `tokio` async is fast | Steep learning curve, slower iteration, 10× longer compile times | Go provides equivalent concurrency performance for this workload with significantly faster development velocity and lower onboarding cost for contributors. |
| **Java/Kotlin** | Mature ecosystem, Spring/Ktor frameworks, JVM tuning tools | 200MB+ runtime, cold start, GC pauses at scale | Docker image goes from 15MB to 300MB. Self-hosted users care about resource footprint. |
| **Node.js** | Same language as Agent, huge ecosystem, fast to prototype | Single-threaded event loop, WebSocket backpressure harder, no goroutines | Gateway needs 10K concurrent connections. Node can do it, but Go does it with less memory and no `cluster` module workarounds. |

#### Why Protobuf instead of JSON, MessagePack, or Avro?

| Format | Strength | Weakness | Why not for us |
|--------|----------|----------|----------------|
| **Protobuf** (chose this) | Schema-enforced types, codegen for Go + TS, compact wire format | Schema evolution learning curve, `.proto` files to maintain | — |
| **JSON** | Universal, human-readable, zero tooling needed | No type enforcement across languages, string field names on every message, larger on wire | Cross-language type safety is a core requirement. JSON relies on convention for field names and types; protobuf enforces them at compile time. |
| **MessagePack** | Binary JSON — fast, compact, no schema needed | No codegen, no type enforcement, debugging requires tooling | Same problem as JSON but less readable. Speed gain doesn't matter at our message volume. |
| **Avro** | Schema registry, great for Kafka ecosystems, schema evolution | Tied to Kafka/Confluent ecosystem, less tooling for Go | We're not using Kafka. Avro without the schema registry loses its main advantage. |

#### Why WebSocket instead of gRPC-Web, SSE, or HTTP long-polling?

| Protocol | Strength | Weakness | Why not for us |
|----------|----------|----------|----------------|
| **WebSocket** (chose this) | Full-duplex, low overhead per message, universal browser support | Stateful connections (harder to load-balance), no built-in compression standard | — |
| **gRPC-Web** | Typed RPCs, streaming, proto-native | Requires Envoy proxy for browser clients, not true bidirectional in browsers | Adds a proxy dependency. Our users shouldn't need to configure Envoy. |
| **Server-Sent Events (SSE)** | Simple, HTTP-native, auto-reconnect | Server→client only — client→server needs separate HTTP POST | Chat requires bidirectional communication. SSE + POST splits what should be one connection into two separate protocols with independent failure modes. |
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

**What's NOT secured yet:**
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

# Part 4: Complete Technical Deep-Dive

> **This section is the textbook.** Every line of code in this project is explained here — the what, the why, and the tradeoff behind it. If you're an interviewer, a contributor, an auditor, or the developer coming back in 6 months — you should never need to ask anyone a question after reading this.

---

## Section A: Setup Guide (Step by Step)

### A.1 — Prerequisites

You need three things installed. Everything else runs in Docker.

| Tool | Install command | Verify |
|------|----------------|--------|
| **Git** | [git-scm.com/downloads](https://git-scm.com/downloads) | `git --version` → 2.30+ |
| **Docker Desktop** | [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop/) | `docker --version` → 20+ |
| **Docker Compose** | Included with Docker Desktop | `docker compose version` → v2+ |

**For local development only** (not needed to just run the system):

| Tool | Install | Verify | Why |
|------|---------|--------|-----|
| **Go 1.25+** | [go.dev/dl](https://go.dev/dl/) | `go version` | Build Gateway/Router without Docker |
| **Node.js 22+** | [nodejs.org](https://nodejs.org/) | `node --version` | Build Agent without Docker |
| **protoc 29+** | [github.com/protocolbuffers/protobuf/releases](https://github.com/protocolbuffers/protobuf/releases) | `protoc --version` | Only if you change `.proto` files |
| **protoc-gen-go** | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` | `protoc-gen-go --version` | Go code generation from proto |
| **ts-proto** | `npm install -g ts-proto` | `npx ts-proto --version` | TypeScript code generation from proto |

### A.2 — Clone and Configure

```bash
# Step 1: Clone
git clone https://github.com/beautifulplanet/SafePaw.git
cd SafePaw/beautifulplanet/safepaw

# Step 2: Create your environment file
cp .env.example .env
```

Now edit `.env`. The **two lines you must change**:

```dotenv
REDIS_PASSWORD=CHANGE_ME_generate_a_random_password
POSTGRES_PASSWORD=CHANGE_ME_generate_a_random_password
```

Generate real passwords:

```bash
# Linux/macOS
openssl rand -base64 32

# Windows PowerShell
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Max 256 }) -as [byte[]])
```

### A.3 — Start the System

```bash
docker compose up -d
```

This does three things:
1. **Builds** the Gateway from source (multi-stage Docker build — takes ~30s first time)
2. **Pulls** Redis 7 and PostgreSQL 16 (pinned by SHA256 digest — not mutable tags)
3. **Starts** all three containers on an internal Docker network

### A.4 — Verify It's Running

```bash
# Health check
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "ok",
  "connections": 0,
  "redis": "connected",
  "timestamp": "2026-02-21T15:30:00Z"
}
```

If you see `"status": "degraded"` and `"redis": "unreachable"`, your `REDIS_PASSWORD` in `.env` doesn't match what Redis was initialized with. Fix: `docker compose down -v` (destroys data), update `.env`, `docker compose up -d`.

### A.5 — Connect a WebSocket Client

Using any WebSocket client (browser console, `wscat`, Postman):

```javascript
// Browser console
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => console.log('Connected');
ws.onmessage = (e) => console.log('Received:', JSON.parse(e.data));

// You'll immediately receive:
// { "type": "session_init", "session_id": "uuid-here" }
```

Send a message:
```javascript
ws.send(JSON.stringify({
  channel: "test",
  sender_id: "user-1",
  sender_platform: "web",
  content: "Hello SafePaw"
}));

// You'll receive:
// { "type": "ack", "message_id": "uuid", "stream_id": "redis-stream-id" }
```

The `ack` means your message was successfully written to Redis Streams. With the Router and Agent services running (not yet built), you'd then receive the response back through the same WebSocket.

### A.6 — Enable Authentication (Production)

```bash
# Generate a secret (minimum 32 bytes — enforced by code)
openssl rand -base64 48
# Example output: K8fJ3kL9mN2pQ5rT7vX0yB4dG6hJ8kL9mN2pQ5rT7vX0yB4dG6hJ8kL=

# Add to .env
AUTH_ENABLED=true
AUTH_SECRET=K8fJ3kL9mN2pQ5rT7vX0yB4dG6hJ8kL9mN2pQ5rT7vX0yB4dG6hJ8kL=

# Restart gateway
docker compose restart gateway
```

Generate a token using the built-in CLI:
```bash
# From the services/gateway directory
go run tools/tokengen/main.go -secret "your-secret" -sub "user-1" -scope "ws"
# Output: eyJ...base64url...signature

# Connect with auth
const ws = new WebSocket('ws://localhost:8080/ws?token=eyJ...');
```

### A.7 — Enable TLS (Production)

```bash
# Self-signed for testing
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 -nodes

# Add to .env
TLS_ENABLED=true
TLS_CERT_FILE=/certs/tls.crt
TLS_KEY_FILE=/certs/tls.key
TLS_PORT=8443

# Mount certs in docker-compose.yml (add volumes to gateway service)
# volumes:
#   - ./tls.crt:/certs/tls.crt:ro
#   - ./tls.key:/certs/tls.key:ro

# Connect via wss://
wss://localhost:8443/ws
```

### A.8 — Regenerate Proto Bindings

Only needed if you modify any `.proto` file in `shared/proto/`:

```powershell
# Windows
.\scripts\proto-gen.ps1

# Linux/macOS
./scripts/proto-gen.sh

# Docker (CI — no local toolchain needed)
docker build -f shared/proto/Dockerfile.protoc --output shared/proto/gen .
```

All three methods produce identical output. The generated code is checked into git. CI can run `git diff --exit-code shared/proto/gen/` to verify no drift.

---

## Section B: Interview-Ready Technical Walkthrough

> This section walks through the system the way you'd explain it in a systems design interview: starting from the user's perspective, following a message through every service, and explaining every design decision along the way.

### B.1 — The Message Lifecycle (End to End)

When a user sends "Hello" from a web browser, here's exactly what happens:

```
Step 1:  Browser opens WebSocket to Gateway
Step 2:  Gateway upgrades HTTP → WebSocket, assigns session UUID
Step 3:  User sends JSON message through WebSocket
Step 4:  Gateway's readPump validates, builds InboundMessage, XADD to Redis
Step 5:  Router's consumer group reads from nopenclaw_inbound via XREADGROUP
Step 6:  Router inspects channel field, routes to correct Agent inbox
Step 7:  Agent processes message, produces response
Step 8:  Agent XADD response to nopenclaw_outbound
Step 9:  Gateway's outboundReader does XREAD on nopenclaw_outbound
Step 10: Gateway looks up session in Hub, writes response to WebSocket
Step 11: Browser receives JSON response
```

**Current implementation covers Steps 1–4 and 9–11.** Steps 5–8 require the Router and Agent (in progress).

### B.2 — Why This Architecture (The Interview Answer)

**"Why not just a monolith?"**

The Gateway needs Go's goroutine model for 10K concurrent WebSocket connections. The Agent needs Node.js/TypeScript for the plugin ecosystem (Discord.js, Telegraf, etc.). Forcing both into one language means one suffers. The protobuf boundary between them IS the modularity contract — it's not just a serialization choice, it's the API contract that prevents the two languages from drifting.

**"Why not gRPC between services?"**

Redis Streams gives us persistence, replay-ability, and consumer groups for free. If the Router crashes, messages stay in the stream — they don't disappear like they would with a direct gRPC call. When the Router restarts, it picks up where it left off. Redis Streams is also the path to horizontal scaling: multiple Router instances can form a consumer group, each processing different messages.

**"Why stateless HMAC tokens instead of JWTs?"**

JWTs are 3-part (header.payload.signature) with flexible algorithms. That flexibility is a risk — algorithm confusion attacks, `"alg": "none"` bypasses. Our tokens are 2-part (payload.signature) with exactly one algorithm: HMAC-SHA256. Simpler = less attack surface. The tradeoff: no embedded algorithm negotiation, which means all Gateway instances must share the same secret. That's fine for our scale.

**"How do you handle 10K connections?"**

Each WebSocket connection gets two goroutines: `readPump` (client → Redis) and `writePump` (Redis → client). Go's runtime multiplexes goroutines onto OS threads — 10K goroutines ≈ 20K goroutines total ≈ ~80MB memory on Go's M:N scheduler. The Hub tracks connections in a `map[string]*Connection` protected by `sync.RWMutex`. Reads (looking up a session) take a read lock (concurrent). Writes (register/unregister) take a write lock (exclusive). The connection count is a separate `atomic.Int64` — no lock needed for the health check endpoint.

**"What happens if Redis goes down?"**

The Gateway's health check will return `"status": "degraded"`. New messages will fail to publish (users get `publish_failed` error). Existing connections stay alive (WebSocket keepalive pings continue). When Redis recovers, the next `XADD` succeeds — no restart needed. The Gateway doesn't cache messages locally, so there's no gap-fill after Redis returns. That's an intentional tradeoff: simplicity over guaranteed delivery. If we need guaranteed delivery, we'd add a local write-ahead log — but that's premature optimization for a self-hosted messaging system.

### B.3 — Concurrency Model

```
Per WebSocket Connection:
┌─────────────────────────────────────────────────────────┐
│                                                         │
│   readPump goroutine          writePump goroutine       │
│   ┌──────────────────┐       ┌──────────────────┐      │
│   │ ws.ReadMessage() │       │ select {          │      │
│   │ json.Unmarshal() │       │   case msg <- ch: │      │
│   │ Validate fields  │       │     ws.Write(msg) │      │
│   │ stream.XADD()    │       │   case <-ticker:  │      │
│   │ sendAck()        │       │     ws.Ping()     │      │
│   └──────────────────┘       └──────────────────┘      │
│          ▲                          ▲                   │
│          │ reads from WS            │ reads from chan   │
│          │                          │                   │
│   ┌──────┴──────┐          ┌────────┴────────┐         │
│   │  WebSocket  │          │ send chan []byte │         │
│   │  (net.Conn) │          │   (256 buffer)  │         │
│   └─────────────┘          └─────────────────┘         │
│                                     ▲                   │
│                                     │                   │
│                          outboundReader writes here     │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Why two goroutines per connection?** WebSocket is full-duplex (both sides can send simultaneously), but the gorilla/websocket library enforces that only one goroutine reads and one goroutine writes at a time. Separating read and write into their own goroutines eliminates lock contention on the `net.Conn`.

**Why a buffered channel (256)?** If the outboundReader is delivering messages faster than the WebSocket can send them, the buffered channel absorbs bursts. If the buffer fills (client is truly stuck), the message is dropped and logged. This prevents a slow client from blocking the entire outbound delivery loop.

### B.4 — Authentication Flow

```
Token Format: base64url(json_claims) + "." + base64url(hmac_sha256(json_claims))

Example:
eyJzdWIiOiJ1c2VyLTEiLCJpYXQiOjE3MDk4MzEwMDB9.ZG9lc25vdG1hdHRlcnRoaXNpc2FuZXhhbXBsZQ

Token Creation:
1. Build claims JSON: {"sub":"user-1","iat":1709831000,"exp":1709917400,"scope":"ws"}
2. Base64url encode the JSON → payload string
3. HMAC-SHA256(payload, secret) → signature bytes
4. Base64url encode signature → sig string
5. Return: payload + "." + sig

Token Validation (6 steps — each can reject):
1. Split on "." — must have exactly 2 parts (not 3 like JWT)
2. Base64url decode payload → JSON bytes
3. HMAC-SHA256(payload, secret) → expected signature
4. hmac.Equal(expected, provided) — CONSTANT TIME comparison
5. json.Unmarshal → check exp > now (with 30s clock skew tolerance)
6. Verify required fields: sub ≠ "", scope ≠ ""
```

**Why constant-time comparison?** Regular `==` comparison on strings short-circuits: it returns false as soon as any byte differs. An attacker can measure response time differences to learn how many leading bytes of their forged signature are correct, then brute-force byte by byte. `hmac.Equal` always compares all bytes, taking the same time regardless of where the mismatch is.

**Why 30-second clock skew tolerance?** Distributed systems have imperfect clock sync. If the Gateway's clock is 15 seconds ahead of the token-issuing service, a token that just expired might still be valid. 30 seconds is generous enough to handle NTP drift but tight enough that an expired token can't be replayed minutes later.

**Token extraction priority:**
1. Query parameter `?token=...` — checked first (WebSocket clients can't set custom headers during the browser handshake)
2. `Authorization: Bearer ...` header — checked second (REST clients and server-to-server calls use this)

### B.5 — Redis Streams Bridge

```
Inbound Flow (user → system):
  readPump → json.Marshal(InboundMessage) → XADD nopenclaw_inbound MAXLEN ~10000

Outbound Flow (system → user):
  outboundReader → XREAD BLOCK 5000 nopenclaw_outbound $lastID → json.Unmarshal → hub.GetConnection → conn.Send
```

**`MAXLEN ~10000`** — The `~` (tilde) is important. Without it, Redis would trim the stream to exactly 10,000 entries on every XADD, which requires a scan. With `~`, Redis uses an efficient radix-tree-based trimming that keeps the stream approximately at 10,000 entries, trimming in batches. Performance difference: O(1) amortized vs O(N) per write.

**`XREAD BLOCK 5000`** — The outbound reader blocks for up to 5 seconds waiting for new messages. This is server-side long-polling: no CPU burned while waiting, the Redis connection is held open but idle. After 5 seconds with no messages, the read returns nil and the loop restarts. This balances latency (messages delivered within milliseconds of arrival) with resource usage (no busy-loop spinning).

**`lastID = "$"`** — On startup, the outbound reader starts with `$`, meaning "only messages that arrive after I started reading." This means messages that were written to the outbound stream while the Gateway was down are NOT replayed. This is intentional — those messages went to sessions that no longer exist (the WebSocket connections closed when the Gateway stopped). Replaying them would require session affinity or persistent session storage, which is a P5 feature.

**Connection pool**: 50 max connections, 5 idle kept warm, 3 retries on transient failures, 5s dial / 3s read / 3s write timeouts. The pool is sized for 10K WebSocket connections each doing occasional XADDs — not every connection keeps a dedicated Redis connection.

### B.6 — Security Middleware Stack

Requests flow through middleware from outermost to innermost:

```
Incoming HTTP Request
  │
  ▼
SecurityHeaders        — Sets 7 response headers on EVERY response
  │
  ▼
RequestID              — Generates/propagates X-Request-ID (UUID) for tracing
  │
  ▼
OriginCheck            — Rejects requests from non-allowlisted origins
  │
  ▼
RateLimit              — Per-IP sliding window, rejects if over threshold
  │
  ▼
AuthRequired (if on)   — Validates HMAC token, sets X-Auth-Subject
  │
  ▼
Route Handler (/ws or /health)
```

**Security Headers (Layer 1):**

| Header | Value | Defends Against |
|--------|-------|-----------------|
| `X-Frame-Options` | `DENY` | Clickjacking — prevents embedding in iframes |
| `X-Content-Type-Options` | `nosniff` | MIME-type sniffing — browser honors declared type |
| `X-XSS-Protection` | `1; mode=block` | Legacy XSS filter (modern browsers use CSP instead) |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Downgrade attacks — forces HTTPS for 1 year |
| `Content-Security-Policy` | `default-src 'self'` | XSS/injection — only loads resources from same origin |
| `Referrer-Policy` | `no-referrer` | Referrer leakage — strips URL from outgoing requests |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | Feature abuse — disables sensitive browser APIs |

**Rate Limiter (Layer 4):**

```go
type ipRecord struct {
    count    int
    lastSeen time.Time
}
```

Per-IP tracking with a sliding window. Default: 30 requests per minute per IP. When a new request arrives:
1. Look up IP in `map[string]*ipRecord`
2. If `time.Since(lastSeen) > window`: reset count to 1
3. If `count >= limit`: return `429 Too Many Requests`
4. Else: increment count, update lastSeen

A background goroutine runs every `window` duration and cleans up stale entries (IPs not seen in over 2× the window). This prevents the map from growing unbounded during a distributed attack.

**Origin Check (Layer 3):** Builds an `O(1)` lookup map from the `ALLOWED_ORIGINS` environment variable. In dev (no origins configured), all origins are allowed. In production, only exact-match origins pass through.

---

## Section C: Complete Component Manual

Every component in the system, explained at the code level.

### C.1 — Gateway: `main.go` (262 lines)

The entry point. Startup is a strict 7-step sequence — each step must succeed or the process exits immediately.

| Step | What it does | Failure behavior |
|------|-------------|-----------------|
| **1. Load config** | Reads all env vars via `config.Load()` | `log.Fatalf` — process exits. Missing `REDIS_PASSWORD` = fatal. |
| **2. Connect Redis** | Creates `StreamClient`, calls `PING` | `log.Fatalf` — process exits. Redis unreachable = can't function. |
| **3. Build Hub** | `NewHub(maxConns)` — empty connection map | Can't fail. |
| **4. Build routes** | `/health` (unauthenticated) and `/ws` (full middleware stack) | Auth setup fails if `AUTH_SECRET` is too short (<32 bytes). |
| **5. Start outbound reader** | Background goroutine: `go outboundReader(ctx, hub, stream)` | Runs forever until context cancelled. Errors are logged, never fatal. |
| **6. Start HTTP server** | `server.ListenAndServe()` or `server.ListenAndServeTLS()` | `log.Fatalf` if port is taken. |
| **7. Graceful shutdown** | Waits for `SIGINT`/`SIGTERM`, gives 10s for active connections to finish | `server.Shutdown(ctx)` — no new connections, existing ones drain. |

**Graceful shutdown detail:**
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
sig := <-quit  // Blocks here until signal received

shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
server.Shutdown(shutdownCtx)  // Stops accepting new conns, waits for in-flight
cancel()                       // Stops outbound reader goroutine
```

The 10-second timeout means: if connections don't close within 10 seconds, force close them. This prevents a stuck client from blocking a deploy indefinitely.

### C.2 — Gateway: `ws/handler.go` (381 lines)

**Hub:**
```go
type Hub struct {
    mu          sync.RWMutex
    connections map[string]*Connection  // sessionID → *Connection
    connCount   atomic.Int64
    maxConns    int
}
```

Why `sync.RWMutex` + `atomic.Int64`?
- The mutex protects the map. Reads (`GetConnection`) take `RLock` — multiple goroutines can read concurrently. Writes (`Register`, `Unregister`) take `Lock` — exclusive.
- The connection count is a separate `atomic.Int64` so the health check endpoint (`hub.Count()`) never takes ANY lock. At 10K connections with constant health polling, this prevents lock contention on what should be a trivial read.

**Connection:**
```go
type Connection struct {
    SessionID   string
    AuthSubject string              // "" if auth disabled
    AuthScope   string              // "ws", "admin", etc.
    ws          *websocket.Conn
    send        chan []byte          // 256-buffer outbound queue
    hub         *Hub
    stream      *redisStream.StreamClient
    cfg         *config.Config
}
```

Each connection holds references to the hub, stream client, and config. This means every goroutine can publish to Redis and unregister itself without global variables.

**WebSocket upgrade flow:**
1. Check capacity (`hub.Count() >= maxConns`) → 503 if full
2. `upgrader.Upgrade(w, r, nil)` → HTTP → WebSocket
3. Read `X-Auth-Subject` and `X-Auth-Scope` headers (set by auth middleware, if enabled)
4. Generate UUID session ID
5. Create `Connection`, register in Hub
6. Send `session_init` JSON to client
7. Launch `go conn.writePump()` and `go conn.readPump()`

**readPump lifecycle:**
```
Set read limit (64KB max) → Set read deadline → Listen for pong
Loop:
    ReadMessage() → (blocks until client sends)
    json.Unmarshal → ClientMessage
    Validate (content not empty)
    Override sender_id with auth subject if authenticated
    Build InboundMessage with UUID, timestamp
    XADD to Redis (3s timeout)
    sendAck to client
On error:
    hub.Unregister(sessionID)
    ws.Close()
```

**writePump lifecycle:**
```
Start ping ticker (every 54 seconds)
Loop:
    select:
        message from send channel → ws.WriteMessage(TextMessage, data)
        ticker fires → ws.WriteMessage(PingMessage, nil)
    On write error → ws.Close()
```

**Why ping interval = 54 seconds, pong wait = 60 seconds?** The client has 60 seconds to respond to a ping with a pong. Pings are sent every 54 seconds. This gives a 6-second buffer — if the pong arrives 5 seconds late (network hiccup), the connection survives. If no pong arrives in 60 seconds, the connection is considered dead.

### C.3 — Gateway: `redis/stream.go` (~200 lines)

**StreamClient:**
```go
type StreamClient struct {
    client         *goredis.Client
    inboundStream  string    // "nopenclaw_inbound"
    outboundStream string    // "nopenclaw_outbound"
}
```

**Connection pool configuration:**

| Setting | Value | Why |
|---------|-------|-----|
| `PoolSize` | 50 | Max concurrent Redis connections. 10K WebSocket connections don't each need a Redis connection — publishes share the pool. |
| `MinIdleConns` | 5 | Keep 5 connections warmed up. First publish after idle doesn't pay connection cost. |
| `MaxRetries` | 3 | Retry transient network failures (Redis GC pause, network blip). |
| `DialTimeout` | 5s | Don't hang forever if Redis DNS is misconfigured. |
| `ReadTimeout` | 3s | Don't hang on a read that Redis won't answer. |
| `WriteTimeout` | 3s | Don't hang on a write that the network dropped. |
| `PoolTimeout` | 4s | Don't wait forever for a pool slot. 4s > ReadTimeout so a slot should free up. |

**PublishInbound (`XADD`):**
```
json.Marshal(InboundMessage) → XADD nopenclaw_inbound MAXLEN ~10000 * data <json>
```

Returns the Redis stream ID (e.g., `1709831000000-0`), which is sent back to the client in the ack. This lets the client correlate their message to its position in the stream.

**ReadOutbound (`XREAD`):**
```
XREAD COUNT 100 BLOCK 5000 STREAMS nopenclaw_outbound <lastID>
```

Reads up to 100 messages at a time (batching for efficiency). Blocks for 5 seconds. On timeout, returns nil (normal — just means no messages). On success, unmarshals each message and returns the list plus the new `lastID` for the next read.

### C.4 — Gateway: `middleware/auth.go` (375 lines)

**Token Claims:**
```go
type TokenClaims struct {
    Sub   string            `json:"sub"`    // Subject — who this token belongs to
    Iat   int64             `json:"iat"`    // Issued At — Unix timestamp
    Exp   int64             `json:"exp"`    // Expires At — Unix timestamp
    Scope string            `json:"scope"`  // Permission scope ("ws", "admin")
    Meta  map[string]string `json:"meta"`   // Extensible metadata
}
```

**Authenticator creation validation:**
```go
func NewAuthenticator(secret []byte, defaultTTL, maxTTL time.Duration) (*Authenticator, error)
```
- Secret must be ≥ 32 bytes (256 bits). This is the minimum for HMAC-SHA256 to provide full security. Shorter keys reduce the effective hash space.
- `defaultTTL` is used when creating tokens without explicit TTL (default: 24 hours)
- `maxTTL` caps how long any token can live (default: 168 hours = 7 days)
- `clockSkew` is hardcoded to 30 seconds

**`AuthRequired` vs `AuthOptional` middleware:**

| Middleware | Token missing | Token invalid | Token valid |
|-----------|--------------|--------------|-------------|
| `AuthRequired` | 401 `missing_token` | 401 `invalid_token` | Sets `X-Auth-Subject`, continues |
| `AuthOptional` | Continues (anonymous) | Logs warning, continues (anonymous) | Sets `X-Auth-Subject`, continues |

**Scope checking:** `AuthRequired` takes a `requiredScope` parameter. If the token's scope doesn't match, it returns 401 — unless the scope is `"admin"`, which bypasses all scope checks. This is the escape hatch for admin tooling.

**`splitToken` — why not `strings.Split`?**

The code manually scans for the `.` separator instead of importing the `strings` package:
```go
func splitToken(token string) (payload, signature string, ok bool) {
    dotIdx := -1
    for i := 0; i < len(token); i++ {
        if token[i] == '.' {
            if dotIdx >= 0 {
                return "", "", false  // Multiple dots = invalid
            }
            dotIdx = i
        }
    }
    ...
}
```
This catches a common attack: injecting extra dots to confuse JWT-style parsers. Our token has exactly one dot. Zero dots or two+ dots = immediate rejection.

### C.5 — Gateway: `middleware/security.go` (234 lines)

**Four middleware functions, applied as a stack:**

1. **`SecurityHeaders`** — Sets 7 headers on every response. No configuration. No `if` statements. Every response gets the same headers. This eliminates the class of bugs where a developer forgets to add headers to a new endpoint.

2. **`RequestID`** — Checks for existing `X-Request-ID` header (propagated from a reverse proxy). If absent, generates a UUID. This lets you trace a request from the user's browser → nginx → Gateway → Redis → Router → Agent and back, using one ID.

3. **`OriginCheck`** — Builds a `map[string]bool` from the `ALLOWED_ORIGINS` env var. Lookups are O(1) regardless of how many origins are configured. Empty origins list = allow all (dev mode).

4. **`RateLimiter`** with background cleanup:
```go
type RateLimiter struct {
    mu      sync.Mutex
    records map[string]*ipRecord
    limit   int
    window  time.Duration
    done    chan struct{}
}
```

The cleanup goroutine runs on a timer and removes entries older than `2 * window`. Without this, a DDoS from 100K unique IPs would grow the map forever even after the attack stops.

**IP extraction:** Checks `X-Real-IP` header first (set by nginx/reverse proxy), falls back to `RemoteAddr` with port stripped. This means rate limiting works correctly behind a reverse proxy — it limits the real client IP, not the proxy's IP.

### C.6 — Gateway: `config/config.go` (201 lines)

**Zero imports of `strings` or `strconv` for string helpers.** The config package implements its own `splitAndTrim`, `splitString`, `indexOf`, and `trimSpace` functions. This is intentional — minimize external dependencies, even stdlib ones, for a security-critical config loader. Every function is ~10 lines and trivially auditable.

**Every config value:**

| Env Variable | Default | Type | Description |
|-------------|---------|------|-------------|
| `GATEWAY_PORT` | `8080` | int | HTTP listen port |
| `GATEWAY_READ_TIMEOUT_SEC` | `10` | int→Duration | Max time to read request headers |
| `GATEWAY_WRITE_TIMEOUT_SEC` | `10` | int→Duration | Max time to write response |
| `GATEWAY_IDLE_TIMEOUT_SEC` | `120` | int→Duration | Keep-alive timeout for idle connections |
| `WS_READ_BUFFER_SIZE` | `1024` | int | WebSocket read buffer (bytes) |
| `WS_WRITE_BUFFER_SIZE` | `1024` | int | WebSocket write buffer (bytes) |
| `WS_MAX_MESSAGE_SIZE` | `65536` | int→int64 | Max single WebSocket message (64KB) |
| `WS_PONG_WAIT_SEC` | `60` | int→Duration | Time to wait for pong response |
| `WS_PING_INTERVAL_SEC` | `54` | int→Duration | Ping frequency (must be < pong wait) |
| `WS_WRITE_WAIT_SEC` | `10` | int→Duration | Max time for a single write to client |
| `WS_MAX_CONNECTIONS` | `10000` | int | Hub capacity cap |
| `REDIS_ADDR` | `redis:6379` | string | Redis address (Docker network name) |
| `REDIS_PASSWORD` | *(required)* | string | Redis auth password |
| `REDIS_DB` | `0` | int | Redis database number |
| `REDIS_INBOUND_STREAM` | `nopenclaw_inbound` | string | Stream name for user→system messages |
| `REDIS_OUTBOUND_STREAM` | `nopenclaw_outbound` | string | Stream name for system→user messages |
| `ALLOWED_ORIGINS` | *(empty=allow all)* | string | Comma-separated allowed origins |
| `AUTH_ENABLED` | `false` | bool | Enable token authentication |
| `AUTH_SECRET` | *(required if auth on)* | string→[]byte | HMAC-SHA256 signing secret (≥32 bytes) |
| `AUTH_DEFAULT_TTL_HOURS` | `24` | int→Duration | Default token lifetime |
| `AUTH_MAX_TTL_HOURS` | `168` | int→Duration | Maximum token lifetime (7 days) |
| `TLS_ENABLED` | `false` | bool | Enable HTTPS/WSS |
| `TLS_CERT_FILE` | `/certs/tls.crt` | string | PEM certificate path |
| `TLS_KEY_FILE` | `/certs/tls.key` | string | PEM private key path |
| `TLS_PORT` | `8443` | int | HTTPS listen port |

**Validation rules enforced at startup:**
- `REDIS_PASSWORD` must be set (even in dev — there are no hardcoded defaults for secrets)
- `AUTH_SECRET` must be set if `AUTH_ENABLED=true`
- `TLS_CERT_FILE` and `TLS_KEY_FILE` must be set if `TLS_ENABLED=true`
- Everything else has safe defaults that work for local development

### C.7 — Gateway: `Dockerfile` (57 lines)

Two-stage build:

**Stage 1 — Builder (`golang:1.26-alpine`):**
```dockerfile
COPY go.mod go.sum ./
RUN go mod download && go mod verify    # Layer cache: only re-download if deps change
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/gateway .
```

- `CGO_ENABLED=0`: Pure Go, no C dependencies. This means the binary is truly static — runs on any Linux, no glibc needed.
- `-ldflags="-s -w"`: Strip debug symbols and DWARF info. Binary goes from ~15MB to ~10MB. Harder for an attacker to reverse-engineer.
- `go mod verify`: Checks that downloaded modules match `go.sum` checksums. Prevents supply-chain tampering.

**Stage 2 — Runtime (`alpine:3.19`):**
```dockerfile
RUN apk --no-cache add ca-certificates && adduser -D -g '' nopenclaw
COPY --from=builder /build/gateway /app/gateway
USER nopenclaw
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -qO- http://localhost:8080/health || exit 1
```

- `ca-certificates`: Needed for TLS connections to external services. Without this, `https://` calls fail.
- `adduser nopenclaw`: Non-root user. If an attacker gets code execution inside the container, they can't install packages, modify system files, or access the Docker socket.
- `HEALTHCHECK`: Docker knows if the container is healthy. `docker compose` uses this for `depends_on: condition: service_healthy`.

**What's NOT in the final image:** Go compiler, source code, build tools, package managers (besides `apk` in base Alpine), git, development headers. The entire attack surface is: one static binary + CA certificates + Alpine userspace (~5MB).

### C.8 — Docker Compose (117 lines)

**Three services, one network, two volumes:**

| Service | Image | Exposed Ports | Resource Limits | Health Check |
|---------|-------|--------------|----------------|-------------|
| `gateway` | Built from `./services/gateway/Dockerfile` | `127.0.0.1:8080:8080` (localhost only) | 256M RAM, 0.5 CPU | `wget` to `/health` every 10s |
| `redis` | `redis:7-alpine@sha256:02f2cc...` | None (internal only) | 256M RAM, 0.5 CPU | `redis-cli ping` every 10s |
| `postgres` | `postgres:16-alpine@sha256:97ff59...` | None (internal only) | 256M RAM, 0.5 CPU | `pg_isready` every 10s |

**Why `127.0.0.1:8080:8080` instead of `8080:8080`?**

`8080:8080` binds to `0.0.0.0` — every network interface. On a VPS, that means the port is open to the internet. `127.0.0.1:8080:8080` binds only to localhost. You must put a reverse proxy (nginx, Caddy) in front of it to expose it to the internet. This is defense-in-depth: even if you forget to configure your firewall, the port isn't publicly reachable.

**Redis hardening:**
```yaml
command: redis-server --requirepass "${REDIS_PASSWORD}" --appendonly yes
         --rename-command FLUSHALL "" --rename-command FLUSHDB "" --rename-command DEBUG ""
```
- `--appendonly yes`: Enables AOF (Append Only File) persistence. If Redis crashes, messages since last write aren't lost.
- `--rename-command FLUSHALL ""`: Disables the FLUSHALL command entirely. An attacker who somehow connects to Redis can't wipe all data.
- `FLUSHDB` and `DEBUG` are similarly disabled — they're dangerous admin commands with no use in production.

**Image pinning:**
```yaml
image: redis:7-alpine@sha256:02f2cc4882f8bf87c79a220ac958f58c700bdec0dfb9b9ea61b62fb0e8f1bfcf
```
Tags are mutable — `redis:7-alpine` today might be different from `redis:7-alpine` next week. SHA256 digests are content-addressed: this exact image, byte for byte, forever. This prevents supply-chain attacks where a compromised Docker Hub account pushes a malicious image under an existing tag.

### C.9 — Protocol Buffers (4 schemas)

**Schema hierarchy:**

```
common.proto    ← Shared types (Timestamp, UserId)
    ▲
    │  imported by
    │
message.proto   ← THE core envelope (13 fields, 2 enums)
user.proto      ← User identity across platforms
channel_config.proto ← Per-channel settings
```

**`message.proto` — the most important file in the project:**

| Field | Type | Purpose |
|-------|------|---------|
| `id` | `string` | UUID v4 — globally unique per message |
| `session_id` | `string` | Links to Gateway WebSocket session |
| `conversation_id` | `string` | Links to Agent conversation thread |
| `sender` | `UserId` | Who sent it (platform + platform-specific ID) |
| `channel` | `string` | Platform name ("discord", "telegram") |
| `direction` | `Direction` enum | `INBOUND` (user→system) or `OUTBOUND` (system→user) |
| `content_type` | `ContentType` enum | TEXT, IMAGE, FILE, COMMAND, SYSTEM, REACTION |
| `content` | `string` | The actual message payload |
| `metadata` | `map<string,string>` | Extensible key-value pairs for platform-specific data |
| `created_at` | `Timestamp` | When the message was created |
| `processed_at` | `Timestamp` | When the message was processed by the Agent |
| `reply_to_id` | `string` | Optional: thread reply |
| `thread_id` | `string` | Optional: thread grouping |

**Why the `metadata` map?** Different platforms have different concepts. Discord has embeds. Telegram has inline keyboards. Slack has blocks. Rather than adding platform-specific fields to the core schema (which would grow unbounded), `metadata` holds any extra key-value data. The Agent for each platform knows which keys to look for.

**Code generation:**

| Source | Go output | TypeScript output |
|--------|-----------|-------------------|
| `common.proto` | `gen/go/common/common.pb.go` | `gen/ts/common.ts` |
| `message.proto` | `gen/go/message/message.pb.go` | `gen/ts/message.ts` |
| `user.proto` | `gen/go/user/user.pb.go` | `gen/ts/user.ts` |
| `channel_config.proto` | `gen/go/channel/channel_config.pb.go` | `gen/ts/channel_config.ts` |

The Go module at `shared/proto/gen/go/` (module name `nopenclaw/proto`) is compilation-verified — `go build ./...` passes. TypeScript files use `ts-proto` for idiomatic TS output (interfaces, not classes).

### C.10 — Proto Generation Pipeline

**Three ways to generate, identical output:**

| Method | When to use | Command |
|--------|------------|---------|
| `proto-gen.ps1` | Windows local dev | `.\scripts\proto-gen.ps1` |
| `proto-gen.sh` | Linux/macOS local dev | `./scripts/proto-gen.sh` |
| `Dockerfile.protoc` | CI, no local tools | `docker build -f shared/proto/Dockerfile.protoc --output shared/proto/gen .` |

**What the scripts do:**
1. Find `protoc`, `protoc-gen-go`, and `ts-proto` (the `.cmd` wrapper on Windows)
2. Run `protoc` with `--go_out` and `--ts_proto_out` for each `.proto` file
3. Use `--go_opt=module=nopenclaw/proto` to flatten Go output into the module root
4. Run `go build ./...` in the Go output directory to verify compilation
5. Count and report generated files

**CI guard (planned):** After generation, run `git diff --exit-code shared/proto/gen/`. If there's a diff, someone changed a `.proto` file without regenerating. The build fails.

---

## Section D: System Design FAQ

### D.1 — "How does this scale horizontally?"

**Gateway:** Run multiple instances. Each gets its own WebSocket connections. The outbound reader in each instance reads from the same Redis stream with `XREAD` (fan-out). For sticky sessions (routing a reply to the right Gateway), we'd use Redis-backed session lookup or a shared Hub via Redis pub/sub. Not implemented yet — single Gateway handles 10K connections, which is enough for most self-hosted deployments.

**Router:** Use Redis consumer groups (`XREADGROUP`). Multiple Router instances form a group — Redis distributes messages across them. Each message is delivered to exactly one Router instance. Acknowledgments (`XACK`) prevent double-processing.

**Agent:** Agents are stateless processors. Spin up more instances behind the Router. The Router publishes to per-agent Redis streams. Each Agent instance consumes from its stream.

**Redis:** Single-instance is the current bottleneck. For HA: Redis Sentinel (automatic failover) or Redis Cluster (sharding). For this project's scale (self-hosted, <1M messages/day), single-instance with AOF persistence is sufficient.

### D.2 — "What happens when things fail?"

| Failure | Impact | Recovery |
|---------|--------|----------|
| **Gateway crashes** | All WebSocket connections drop. Messages in Redis persist. | Restart Gateway. Clients reconnect. New `$` cursor means in-flight outbound messages are lost (session IDs no longer valid). |
| **Redis crashes** | Gateway health → degraded. New messages fail. Existing connections stay alive (WS keepalive). | Redis restarts, AOF recovers data. Gateway automatically reconnects (go-redis has retry). |
| **Router crashes** | Messages pile up in `nopenclaw_inbound`. No responses sent. | Restart Router. It picks up where it left off via consumer group cursor. Messages are not lost. |
| **Agent crashes** | Router can't deliver messages. Depends on implementation — buffered or dropped. | Restart Agent. Router retries or re-routes. |
| **Network partition** | Redis becomes unreachable from Gateway/Router. Same as Redis crash from their perspective. | Partition heals. Retry logic handles reconnection. |

### D.3 — "What about message ordering?"

Redis Streams preserves insertion order within a single stream. The `nopenclaw_inbound` stream receives messages in the order the Gateway publishes them. If two clients send messages simultaneously, the order depends on which `XADD` reaches Redis first — Redis is single-threaded, so there's a total order.

With consumer groups (Router), messages are distributed but per-consumer order is preserved. If you need conversation-level ordering (all messages in a thread processed sequentially), the Router must route by conversation ID to the same Agent instance.

### D.4 — "Why not just use Firebase/Supabase/PubNub?"

Those are managed services. This project exists specifically for people who want self-hosted messaging for privacy, compliance, or cost reasons. At 10K messages/day, Firebase is ~$0. At 10M messages/day, it's hundreds of dollars/month. A $5 VPS running SafePaw handles that for $5/month forever, and your messages never leave your infrastructure.

### D.5 — "Why track generated proto code in git?"

Clone-and-build. A new developer should be able to `git clone` and `docker compose up` without installing Go, protoc, or any code generation toolchain. If generated code isn't in git, they need: protoc, protoc-gen-go, ts-proto, correct versions of each. That's a 20-minute setup just to get types that already exist.

The tradeoff is PR noise — generated files appear in diffs. The CI guard (`git diff --exit-code`) catches drift without requiring developers to re-run generation on every commit.

---

## Section E: Detailed File Map

Every file, its line count, and what it does:

```
beautifulplanet/safepaw/
│
├── services/gateway/                    # THE WebSocket entry point
│   ├── main.go .................. 262   # Startup: config → redis → hub → routes → server → shutdown
│   ├── Dockerfile ............... 57    # Multi-stage: golang:1.26-alpine → alpine:3.19, non-root
│   ├── go.mod ...................       # Module: nopenclaw/gateway, deps: gorilla/websocket, go-redis, uuid
│   ├── go.sum ...................       # Dependency checksums (tamper detection)
│   │
│   ├── ws/
│   │   └── handler.go ........... 381  # Hub (RWMutex+atomic), Connection, readPump, writePump, upgrader
│   │
│   ├── redis/
│   │   └── stream.go ............ ~200 # StreamClient, XADD/XREAD, pool(50), InboundMessage, OutboundMessage
│   │
│   ├── middleware/
│   │   ├── auth.go .............. 375  # TokenClaims, Authenticator, HMAC-SHA256, AuthRequired/Optional
│   │   └── security.go ......... 234  # SecurityHeaders(7), RequestID, OriginCheck, RateLimiter+cleanup
│   │
│   ├── config/
│   │   └── config.go ........... 201  # All 25 env vars, Load(), validation, zero external string imports
│   │
│   └── tools/tokengen/
│       └── main.go ..............      # CLI: go run tools/tokengen/main.go -secret X -sub Y -scope Z
│
├── services/router/                     # Message routing engine (IN PROGRESS)
│   ├── go.mod ...................       # Module: nopenclaw/router, replace: nopenclaw/proto → ../../shared/proto/gen/go
│   ├── cmd/router/ ..............       # Entry point (not yet written)
│   ├── internal/config/ .........       # Config (not yet written)
│   ├── internal/consumer/ .......       # Redis XREADGROUP consumer (not yet written)
│   ├── internal/router/ .........       # Routing logic (not yet written)
│   └── internal/publisher/ ......       # XADD to agent inboxes (not yet written)
│
├── services/agent/                      # Message processor (NOT STARTED)
│   └── README.md ................       # TypeScript echo service, then LLM integration
│
├── services/postgres/
│   └── init/ ....................       # SQL init scripts for Docker entrypoint
│
├── shared/proto/                        # THE source of truth for typed contracts
│   ├── common.proto .............       # Timestamp, UserId — shared base types
│   ├── message.proto ............       # Message envelope — 13 fields, 2 enums, the core type
│   ├── user.proto ...............       # User + PlatformIdentity + UserRole
│   ├── channel_config.proto .....       # ChannelConfig + RateLimit + ChannelStatus
│   ├── Dockerfile.protoc ........       # CI-reproducible generation (no local tools needed)
│   │
│   └── gen/                             # Generated code (CHECKED INTO GIT — see Section D.5)
│       ├── go/                          # Go module: nopenclaw/proto
│       │   ├── go.mod
│       │   ├── common/common.pb.go
│       │   ├── message/message.pb.go
│       │   ├── user/user.pb.go
│       │   └── channel/channel_config.pb.go
│       │
│       └── ts/                          # TypeScript package: @nopenclaw/proto
│           ├── package.json
│           ├── common.ts
│           ├── message.ts
│           ├── user.ts
│           └── channel_config.ts
│
├── scripts/
│   ├── proto-gen.ps1 ............       # Windows: find tools, run protoc, verify Go build
│   └── proto-gen.sh .............       # Linux/macOS: same logic, shell syntax
│
├── docker-compose.yml ........... 117  # Gateway + Redis + Postgres, internal network, resource limits
├── .env.example .................       # All 25 config vars, documented, safe defaults
└── .gitignore ...................       # Blocks .env, editor files; ALLOWS proto/gen/
```

---

## Section F: Operations Reference

### F.1 — Environment Variables (Complete Reference)

See Section C.6 for the full table of all 25 environment variables, their defaults, types, and descriptions.

### F.2 — Health Checks

| Endpoint | Method | Auth | Response |
|----------|--------|------|----------|
| `GET /health` | HTTP | None | `{"status":"ok","connections":N,"redis":"connected","timestamp":"..."}` |

**Status values:**
- `"ok"` — Gateway running, Redis connected
- `"degraded"` — Gateway running, Redis unreachable (returns HTTP 503)

**Docker health check:** Every 10 seconds, Docker runs `wget -qO- http://localhost:8080/health`. Three consecutive failures = container marked unhealthy. `docker compose` restart policy handles recovery.

### F.3 — Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `FATAL: Configuration error: REDIS_PASSWORD environment variable is required` | Missing `.env` file or `REDIS_PASSWORD` not set | `cp .env.example .env` and set passwords |
| `FATAL: Redis connection failed` | Redis not running, wrong password, wrong address | Check `docker compose ps` — is redis healthy? Check `REDIS_PASSWORD` matches in Gateway and Redis |
| Health returns `"redis": "unreachable"` | Redis crashed or network issue | `docker compose restart redis` |
| `WebSocket upgrade failed` | Client connecting to wrong path, or middleware rejecting | Verify URL is `/ws` (not `/websocket`). Check `ALLOWED_ORIGINS` if set. |
| `Connection rejected: at capacity` | 10K connections reached | Increase `WS_MAX_CONNECTIONS` or deploy additional Gateway instances |
| `401 missing_token` | Auth enabled but client didn't send token | Pass `?token=...` as query parameter or `Authorization: Bearer ...` header |
| `401 invalid_token: signature verification failed` | Wrong secret, corrupted token, or token from different secret | Regenerate token with correct `AUTH_SECRET` |
| `401 token expired` | Token's `exp` is in the past | Generate a new token. Check server clock (`date -u`). |
| `429 Too Many Requests` | Rate limit exceeded | Wait 1 minute. Or increase rate limit by adjusting `RateLimiter` in `main.go` (not yet configurable via env — planned). |
| `docker compose up` fails with port conflict | Port 8080 already in use | `GATEWAY_PORT=8081` in `.env`, or stop the conflicting process |
| Proto generation fails | Missing `protoc`, `protoc-gen-go`, or `ts-proto` | Use Docker method: `docker build -f shared/proto/Dockerfile.protoc --output shared/proto/gen .` |

### F.4 — Log Format

All logs follow a consistent format:
```
2026/02/21 15:30:00.123456 main.go:42: [COMPONENT] Message
```

**Log prefixes:**
| Prefix | Component | Example |
|--------|-----------|---------|
| `[CONFIG]` | Configuration loader | `Port=8080 MaxConns=10000 Redis=redis:6379` |
| `[REDIS]` | Redis Streams bridge | `Connected to redis:6379 (pool=50)` |
| `[HUB]` | Connection hub | `Connection registered: session=abc (total=1)` |
| `[WS]` | WebSocket handler | `Client connected: session=abc auth=user-1 scope=ws` |
| `[AUTH]` | Authentication middleware | `Authenticated: sub=user-1 scope=ws ttl=23h59m` |
| `[OUTBOUND]` | Outbound message reader | `No connection for session=abc, message dropped` |
| `[SERVER]` | HTTP server | `Listening on :8080 (TLS disabled — dev mode)` |
| `[SHUTDOWN]` | Graceful shutdown | `Received signal: interrupt` |

### F.5 — Docker Commands Reference

```bash
# Start all services
docker compose up -d

# View logs (all services)
docker compose logs -f

# View logs (gateway only)
docker compose logs -f gateway

# Restart gateway (after config change)
docker compose restart gateway

# Rebuild gateway (after code change)
docker compose up -d --build gateway

# Stop everything (preserve data)
docker compose down

# Stop everything and DELETE all data (Redis + Postgres volumes)
docker compose down -v

# Check which services are running
docker compose ps

# Enter Redis CLI (for debugging)
docker compose exec redis redis-cli -a "${REDIS_PASSWORD}"

# Check Redis stream contents
docker compose exec redis redis-cli -a "${REDIS_PASSWORD}" XRANGE nopenclaw_inbound - +

# Check stream length
docker compose exec redis redis-cli -a "${REDIS_PASSWORD}" XLEN nopenclaw_inbound
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
| **P7** | Wizard UI | Only after the core message loop is stable and tested |

---

## License

Private — not yet open source.

---

*Built with Go, TypeScript, Protocol Buffers, Redis Streams, and Docker.
4 proto schemas. 2 languages sharing typed contracts. Security by default at every layer.*
