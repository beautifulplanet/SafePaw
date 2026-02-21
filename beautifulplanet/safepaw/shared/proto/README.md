# Safepaw Shared: Protobuf Schemas

Shared data contracts between Gateway, Router, and Agent.

## Purpose
These `.proto` files define the exact structure of every message that flows between services. 
This is the **single source of truth** — if it's not in a `.proto` file, it doesn't exist.

## Files (To Be Created)
- `message.proto` — Core message envelope (sender, channel, content, timestamp)
- `user.proto` — User identity and session info
- `channel_config.proto` — Channel routing configuration

## Compilation
Proto files will be compiled into:
- **Go** bindings (for Gateway + Router)
- **TypeScript** bindings (for Agent)

## Status
- [ ] `message.proto` defined
- [ ] `user.proto` defined
- [ ] `channel_config.proto` defined
- [ ] Compilation script working
