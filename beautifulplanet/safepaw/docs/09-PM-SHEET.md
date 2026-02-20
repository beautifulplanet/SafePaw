# Safepaw: Master Project Management (PM) Sheet

## Methodology
As a team of CTOs, we operate on strict risk mitigation. Every task is broken down into atomic units. 
For each task, we define:
- **Plan A:** The optimal, Ivy-League standard approach.
- **Plan B:** The fallback approach if Plan A hits technical limitations or takes too long.
- **Plan C:** The "We need to figure this out together" escape hatch.

---

## Phase 1: Foundation & Infrastructure Setup
*Goal: Establish the local development environment, monorepo structure, and core databases with absolute security and isolation.*

### Task 1.1: Initialize Monorepo Structure (Secure)
- **Description:** Create the directory structure for all microservices to ensure clean boundaries and prevent accidental execution of untrusted code.
- **Plan A:** Use a standard directory structure (`/services/gateway`, `/services/router`, etc.) with a top-level `docker-compose.yml` for local orchestration. Ensure no global execution permissions are set.
  - *Tech Plan A:* Write a bash script to scaffold directories, initialize empty Git repo, and create placeholder `README.md` files for each service.
- **Plan B:** Use a polyglot monorepo tool like Bazel or Pants if standard directories become too hard to manage for cross-service protobufs.
  - *Tech Plan B:* Initialize Bazel workspace and define `BUILD` files for each language.
- **Plan C:** If monorepo tooling is too heavy, split into multiple repositories (not recommended initially due to overhead).

### Task 1.2: Setup Local Redis (Message Queue - Isolated)
- **Description:** Stand up Redis for inter-service communication (Redis Streams) with strict network isolation.
- **Plan A:** Use official Redis Docker image via `docker-compose` on an internal Docker network. Do NOT expose port 6379 to the host machine.
  - *Tech Plan A:* Write `docker-compose.yml` with `redis:7-alpine`, configure an internal network, and require authentication (requirepass).
- **Plan B:** Use a lightweight alternative like NATS if Redis Streams proves too complex for our specific pub/sub needs, maintaining the same internal network isolation.
  - *Tech Plan B:* Swap Redis for `nats:latest` in compose file, update architecture docs.
- **Plan C:** Figure out a managed cloud queue (AWS SQS) for local dev if local containers fail on the host machine.

### Task 1.3: Setup Local Postgres (State & Config - Isolated)
- **Description:** Stand up PostgreSQL for durable state (sessions, user configs) with strict network isolation.
- **Plan A:** Use official Postgres Docker image on an internal Docker network. Do NOT expose port 5432 to the host machine.
  - *Tech Plan A:* Add `postgres:16-alpine` to `docker-compose.yml`, set up init scripts (`/docker-entrypoint-initdb.d/`) to create the `safepaw` database and user. Enforce strong passwords even locally.
- **Plan B:** Use SQLite locally for development speed, migrating to Postgres only for production, ensuring the `.sqlite` file is strictly `.gitignore`d.
  - *Tech Plan B:* Configure ORMs in services to use SQLite dialect locally.
- **Plan C:** TBD based on database performance needs.

### Task 1.4: Define Protobuf Schemas (The API Boundary)
- **Description:** Define the exact data structures that will pass between Gateway, Router, and Agent.
- **Plan A:** Create a `/shared/proto` directory and write `.proto` files for `Message`, `User`, and `ChannelConfig`.
  - *Tech Plan A:* Write `message.proto`. Set up a script to compile these into Rust, Go, and Python bindings automatically.
- **Plan B:** Use JSON over Redis if Protobuf compilation becomes a bottleneck for rapid iteration.
  - *Tech Plan B:* Define strict JSON Schemas (OpenAPI/Zod style) and share them as documentation.
- **Plan C:** Figure out a dynamic schema registry if message formats change too frequently.

---

## Phase 2: The Go Gateway & Router (Connection Management)
*Goal: Handle 10k+ concurrent WebSockets safely and route messages.*

### Task 2.1: Initialize Go Project
- **Description:** Scaffold the Go-based WebSocket server and Router.
- **Plan A:** Use `go mod init` and set up `gorilla/websocket` for connections.
  - *Tech Plan A:* Write a basic `main.go` that binds to port 8080 and accepts HTTP requests.
- **Plan B:** Use a higher-level framework like Fiber if raw `net/http` is too verbose.
  - *Tech Plan B:* Rewrite `main.go` using `gofiber/fiber/v2`.
- **Plan C:** TBD if Go proves too complex; fallback to TypeScript/Node.js for Gateway.

### Task 2.2: Implement WebSocket Handshake
- **Description:** Accept incoming WS connections and upgrade them.
- **Plan A:** Use Axum's WebSocket upgrade extractor.
  - *Tech Plan A:* Create a route `/ws`, extract the connection, and spawn a Tokio task for each connected client.
- **Plan B:** Handle raw HTTP upgrades manually if Axum's abstraction hides too much control.
  - *Tech Plan B:* Parse `Connection: Upgrade` headers manually using `hyper`.
- **Plan C:** TBD.

### Task 2.3: Connect Gateway to Redis Streams
- **Description:** Forward incoming WS messages to the Redis Queue.
- **Plan A:** Use the `redis-rs` crate with async support.
  - *Tech Plan A:* Establish a Redis connection pool (`deadpool-redis`). On WS message receive, execute `XADD` to a `safepaw_inbound` stream.
- **Plan B:** Use standard Redis Pub/Sub if Streams (XADD/XREAD) are overkill for MVP.
  - *Tech Plan B:* Use `PUBLISH` command instead of `XADD`.
- **Plan C:** TBD.

---

*(Note: Phases 3-8 (Router, Agent, Media, Extraction, Integration, Deployment) will be expanded into atomic tasks as we complete Phases 1 & 2 to maintain strict focus and avoid overwhelming the active memory context.)*
