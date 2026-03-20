# SafePaw Shared: Protobuf Schemas

The **single source of truth** for all data structures flowing between services.

## Schemas

| File | Description | Used By |
|------|-------------|---------|
| `common.proto` | Shared primitives (Timestamp, UserId) | All services |
| `message.proto` | Message envelope (the core data unit) | Gateway, Router, Agent |
| `user.proto` | Unified user identity across platforms | Gateway, Agent |
| `channel_config.proto` | Per-channel settings and credentials | Router, Agent |

## Compilation

### Local (Windows)
```powershell
.\scripts\proto-gen.ps1
```

### Local (Linux/macOS)
```bash
./scripts/proto-gen.sh
```

### Docker (CI — no local tools needed)
```bash
docker build -f shared/proto/Dockerfile.protoc --output shared/proto/gen .
```

### Prerequisites (local only)
```bash
# protoc v29+ — https://github.com/protocolbuffers/protobuf/releases
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
npm install -g ts-proto
```

## Generated Output

- **Go** bindings -> `shared/proto/gen/go/` (module: `nopenclaw/proto` -- legacy name, rename pending)
- **TypeScript** bindings -> `shared/proto/gen/ts/` (package: `@nopenclaw/proto` -- legacy name, rename pending)

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
- [x] Compilation script working (Go + TypeScript)
- [x] Go bindings compile cleanly
- [x] Docker-based CI generation
