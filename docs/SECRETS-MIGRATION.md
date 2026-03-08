# Secrets Management Migration Guide

> How to move SafePaw from environment-variable secrets to an external secrets
> manager (HashiCorp Vault, AWS Secrets Manager, etc.).

---

## Current State

SafePaw reads all secrets from environment variables at startup:

| Secret | Service | Loaded in |
|--------|---------|-----------|
| `AUTH_SECRET` | Gateway | `config/config.go` via `os.Getenv` |
| `REDIS_PASSWORD` | Gateway | `config/config.go` via `os.Getenv` |
| `OPENCLAW_GATEWAY_TOKEN` | Gateway | `config/config.go` via `os.Getenv` |
| `WIZARD_ADMIN_PASSWORD` | Wizard | `internal/config/config.go` via `os.Getenv` |
| `WIZARD_OPERATOR_PASSWORD` | Wizard | `internal/config/config.go` via `os.Getenv` |
| `WIZARD_VIEWER_PASSWORD` | Wizard | `internal/config/config.go` via `os.Getenv` |
| `WIZARD_TOTP_SECRET` | Wizard | `internal/config/config.go` via `os.Getenv` |
| `POSTGRES_PASSWORD` | Postgres | `docker-compose.yml` → `.env` file |
| `ANTHROPIC_API_KEY` | OpenClaw | `.env` file (managed by wizard) |
| `OPENAI_API_KEY` | OpenClaw | `.env` file (managed by wizard) |

This works well for single-node, localhost deployment. Environment variables
are simple, require no external dependencies, and are supported by every
container runtime.

---

## The Interface

We've defined `secrets.Provider` in `shared/secrets/provider.go`:

```go
type Provider interface {
    Get(ctx context.Context, key string) (string, error)
}
```

The default implementation (`EnvProvider`) wraps `os.Getenv` — zero behavior
change from today.

### Why this interface?

1. **Single method**: `Get(key) → value`. Secrets are read-only from the
   application's perspective.
2. **Context-aware**: External backends need timeouts and cancellation.
3. **Error-aware**: Distinguishes "not set" (`"", nil`) from "backend down"
   (`"", err`).
4. **No write path**: Applications should never write secrets. That's the
   ops team's job (CLI, Terraform, UI).

---

## Migration Steps

### Step 1: Wire EnvProvider (no behavior change)

Replace direct `os.Getenv("AUTH_SECRET")` calls with:

```go
import "safepaw/shared/secrets"

provider := secrets.EnvProvider{}
val, err := provider.Get(ctx, secrets.KeyAuthSecret)
```

This is a refactor, not a feature — tests should continue to pass without
modification.

### Step 2: Add a Vault implementation

Example skeleton (not shipped — write when you need it):

```go
package secrets

import (
    "context"
    "fmt"

    vault "github.com/hashicorp/vault/api"
)

type VaultProvider struct {
    client *vault.Client
    mount  string // e.g. "secret/data/safepaw"
}

func NewVaultProvider(addr, token, mount string) (*VaultProvider, error) {
    cfg := vault.DefaultConfig()
    cfg.Address = addr
    client, err := vault.NewClient(cfg)
    if err != nil {
        return nil, fmt.Errorf("vault client: %w", err)
    }
    client.SetToken(token)
    return &VaultProvider{client: client, mount: mount}, nil
}

func (v *VaultProvider) Get(ctx context.Context, key string) (string, error) {
    secret, err := v.client.KVv2(v.mount).Get(ctx, key)
    if err != nil {
        return "", fmt.Errorf("vault get %s: %w", key, err)
    }
    if secret == nil || secret.Data == nil {
        return "", nil
    }
    val, ok := secret.Data["value"].(string)
    if !ok {
        return "", nil
    }
    return val, nil
}
```

### Step 3: Config-driven provider selection

```go
func NewProvider() (secrets.Provider, error) {
    switch os.Getenv("SECRETS_BACKEND") {
    case "vault":
        return secrets.NewVaultProvider(
            os.Getenv("VAULT_ADDR"),
            os.Getenv("VAULT_TOKEN"),
            os.Getenv("VAULT_MOUNT"),
        )
    case "aws":
        // return NewAWSProvider(...)
    default:
        return secrets.EnvProvider{}, nil
    }
}
```

The `SECRETS_BACKEND` env var is the only bootstrap secret that must remain
in the environment — everything else comes from the provider.

### Step 4: Docker Compose integration

For Vault, add a sidecar or use Vault Agent injector:

```yaml
services:
  vault-agent:
    image: hashicorp/vault:1.15
    command: ["agent", "-config=/etc/vault/agent.hcl"]
    volumes:
      - ./vault-agent.hcl:/etc/vault/agent.hcl:ro
      - secrets-vol:/secrets
```

Or use Docker secrets (Swarm mode):

```yaml
secrets:
  auth_secret:
    external: true
services:
  gateway:
    secrets:
      - auth_secret
    environment:
      SECRETS_BACKEND: docker
```

---

## Decision: Why Not Now?

We deliberately chose **not** to integrate Vault today:

1. **Deployment target**: SafePaw runs on localhost or behind a VPN. The
   threat of env-var exposure is low (requires host access, which is
   game-over anyway).
2. **Dependency cost**: `hashicorp/vault/api` pulls in 50+ transitive deps.
   This contradicts [ADR-002](../docs/adr/002-zero-external-middleware-deps.md).
3. **Complexity budget**: Every external service is another failure mode.
   For a single-node deployment, Vault is more complexity than it solves.
4. **Interface-first**: By defining the `Provider` interface now, we can
   adopt Vault later with zero changes to business logic — only wiring.

### When to migrate

- When SafePaw moves to multi-node deployment
- When secrets need rotation without restart
- When compliance requires a dedicated secrets manager (SOC 2 CC6.1)
- When an ops team is available to manage Vault infrastructure

---

## File Inventory

| File | Purpose |
|------|---------|
| `shared/secrets/provider.go` | Interface definition + key constants |
| `shared/secrets/env.go` | Default `EnvProvider` (wraps `os.Getenv`) |
| `shared/secrets/env_test.go` | Tests for `EnvProvider` |
| `shared/secrets/go.mod` | Module definition |
| `docs/SECRETS-MIGRATION.md` | This document |

---

## References

- [ADR-002: Zero external middleware deps](docs/adr/002-zero-external-middleware-deps.md)
- [COMPLIANCE.md](docs/COMPLIANCE.md) — SOC 2 CC6.1 (logical access)
- [SECURITY.md](SECURITY.md) — Current secret handling
- [THREAT-MODEL.md](THREAT-MODEL.md) — Secret exposure threat (I4)
