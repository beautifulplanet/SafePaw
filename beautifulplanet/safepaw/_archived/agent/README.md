# NOPEnclaw Agent (Node.js / TypeScript)

The AI-powered message processor and channel integration layer.

## Responsibilities
- Receive routed messages from the Router via Redis
- Execute LLM calls (OpenAI, Anthropic) in a sandboxed environment
- Manage channel integrations (Discord, Telegram, WhatsApp, etc.)
- Return responses back through Redis → Gateway → Client

## Tech Stack
- **Language:** TypeScript / Node.js
- **LLM Integration:** OpenAI SDK, Anthropic SDK
- **Channel Plugins:** Extracted from OpenClaw reference
- **Message Queue:** Redis Streams

## Security
- Runs in a locked-down container with read-only filesystem
- Egress filtered to approved LLM API endpoints only
- Ephemeral state — no cross-session contamination

## Status
- [ ] Project scaffolded
- [ ] Redis consumer working
- [ ] LLM integration (placeholder)
- [ ] First channel plugin extracted (Discord)
