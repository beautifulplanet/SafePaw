# Safepaw Router (Go)

The message routing engine that reads from Redis and dispatches to the correct Agent.

## Responsibilities
- Consume messages from `safepaw_inbound` Redis Stream
- Determine the target channel/agent based on message metadata
- Route messages to the appropriate Agent service
- Handle dead-letter routing for undeliverable messages

## Tech Stack
- **Language:** Go
- **Message Queue:** Redis Streams (consumer groups)
- **Config Store:** PostgreSQL (channel routing rules)

## Status
- [ ] Project scaffolded
- [ ] Redis Stream consumer working
- [ ] Routing logic implemented
- [ ] Dead-letter handling
