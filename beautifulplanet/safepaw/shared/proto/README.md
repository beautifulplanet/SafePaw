# Safepaw Shared: Protobuf Schemas

The **single source of truth** for all data structures flowing between services.

## Schemas

| File | Description | Used By |
|------|-------------|---------|
| `common.proto` | Shared primitives (Timestamp, UserId) | All services |
| `message.proto` | Message envelope (the core data unit) | Gateway, Router, Agent |
| `user.proto` | Unified user identity across platforms | Gateway, Agent |
| `channel_config.proto` | Per-channel settings and credentials | Router, Agent |

## Compilation

Run from the `safepaw/` root:

```bash
./scripts/proto-gen.sh
```

This generates:
- **Go** bindings → `shared/proto/gen/go/`
- **TypeScript** bindings → `shared/proto/gen/ts/`

## Rules

1. **NEVER** modify generated files in `gen/` — they are overwritten on compile
2. All schema changes require a review (breaking changes affect all services)
3. Field numbers are permanent — never reuse a deleted field number
4. Use `reserved` keyword when deprecating fields

## Status
- [x] `common.proto` defined
- [x] `message.proto` defined
- [x] `user.proto` defined
- [x] `channel_config.proto` defined
- [ ] Compilation script working
