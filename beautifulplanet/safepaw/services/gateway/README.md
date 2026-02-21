# NOPEnclaw Gateway (Go)

The WebSocket entry point for all client connections.

## Responsibilities
- Accept and manage 10k+ concurrent WebSocket connections
- Authenticate incoming connections
- Forward validated messages to Redis Streams (`nopenclaw_inbound`)
- Receive outbound messages from Redis and push to connected clients

## Tech Stack
- **Language:** Go
- **Framework:** `gorilla/websocket` or `gofiber/fiber`
- **Message Queue:** Redis Streams (via `go-redis`)

## Status
- [ ] Project scaffolded
- [ ] WebSocket handshake working
- [ ] Connected to Redis Streams
- [ ] Authentication middleware
