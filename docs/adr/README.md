# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the SafePaw
project. ADRs document significant technical decisions — the **why** behind the
**what** — so future contributors (and interviewers) can understand the
reasoning without reading every commit.

## Format

Each ADR follows [Michael Nygard's template](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions):

- **Status**: Accepted, Superseded, or Deprecated
- **Context**: The forces at play
- **Decision**: What we chose
- **Consequences**: Trade-offs, good and bad

## Index

| # | Title | Status | Date |
|---|-------|--------|------|
| [001](001-hmac-tokens-not-jwt.md) | HMAC-SHA256 tokens instead of JWT | Accepted | 2026-02-15 |
| [002](002-zero-external-middleware-deps.md) | Zero external middleware dependencies | Accepted | 2026-02-15 |
| [003](003-go-for-gateway.md) | Go for the security gateway | Accepted | 2026-02-10 |
| [004](004-docker-socket-proxy.md) | Docker socket proxy instead of direct mount | Accepted | 2026-03-01 |
| [005](005-heuristic-scanning-not-ml.md) | Heuristic prompt-injection scanning, not ML | Accepted | 2026-02-20 |
| [006](006-embedded-frontend.md) | Monorepo with embedded React frontend | Accepted | 2026-02-10 |
