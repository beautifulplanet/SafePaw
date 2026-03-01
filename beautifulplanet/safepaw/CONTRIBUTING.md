# Contributing to SafePaw

Thank you for your interest in contributing to SafePaw. This guide covers the development workflow, coding standards, and review process.

---

## Prerequisites

- **Go 1.25+** — [install](https://go.dev/dl/)
- **Node.js 22+** — [install](https://nodejs.org/) (for Wizard UI)
- **Docker** and **Docker Compose V2** — [install](https://docs.docker.com/get-docker/)
- **golangci-lint** — `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- **gosec** — `go install github.com/securego/gosec/v2/cmd/gosec@latest`

---

## Development Setup

```bash
# Clone
git clone https://github.com/beautifulplanet/SafePaw.git
cd SafePaw/beautifulplanet/safepaw

# Gateway
cd services/gateway
go build -v ./...
go test -v ./...

# Wizard
cd ../wizard
go build -v ./...
go test -v ./...

# Wizard UI (hot reload)
cd ui
npm install
npm run dev
```

---

## Project Structure

```
services/
├── gateway/          # Go security reverse proxy
│   ├── middleware/    # Security, auth, sanitize, output scanner, metrics
│   ├── config/       # Environment-based configuration
│   └── tools/        # CLI utilities (token generator)
├── wizard/           # Go setup wizard + embedded React SPA
│   ├── internal/     # API handlers, config, Docker client, middleware, sessions
│   └── ui/           # React 19 + TypeScript + Tailwind
└── postgres/         # Database init scripts
```

---

## Coding Standards

### Go

- **No external dependencies** unless absolutely necessary. The project maintains a zero-dep philosophy for security and auditability. If you need functionality, implement it in `<100` lines before reaching for a library.
- **Error handling:** always handle errors explicitly. Never ignore errors silently.
- **Logging:** use structured log prefixes: `[AUTH]`, `[SCANNER]`, `[SECURITY]`, `[PROXY]`, `[WS]`, `[RATELIMIT]`, `[REVOKE]`, `[OUTPUT-SCAN]`. Always include `request_id=` where available.
- **Comments:** explain *why*, not *what*. Don't narrate obvious code.
- **Testing:** every new feature needs tests. Every security feature needs positive AND negative tests. Aim for table-driven tests where applicable.

### TypeScript / React

- **Strict mode** enabled. No `any` types.
- **Functional components** with hooks. No class components.
- **API calls** go through `src/api.ts` (typed fetch wrapper).

### Git

- **Conventional commits:** `feat:`, `fix:`, `test:`, `docs:`, `refactor:`, `chore:`
- **One concern per commit.** Don't mix features with refactoring.
- **Branch naming:** `feat/token-revocation`, `fix/rate-limit-edge-case`, `test/config-loading`

---

## Testing

### Running Tests

```bash
# All gateway tests (including race detector)
cd services/gateway && go test -race -v ./...

# All wizard tests
cd services/wizard && go test -race -v ./...

# With coverage
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

### Writing Tests

- Place test files next to the code they test (`foo.go` → `foo_test.go`)
- Use table-driven tests for functions with multiple input/output cases
- Security tests must include both **allow** and **deny** cases
- Use `t.Helper()` in test helper functions
- Don't sleep in tests unless testing time-dependent behavior (and document why)

### E2E Verification

```bash
# Start the stack
docker compose up -d

# Run the verification script
./scripts/verify-deployment.sh
```

---

## Static Analysis

```bash
# Lint (gateway)
cd services/gateway && golangci-lint run

# Lint (wizard)
cd services/wizard && golangci-lint run

# Security scan
cd services/gateway && gosec ./...
cd services/wizard && gosec ./...
```

CI runs all of these automatically on every PR.

---

## Pull Request Process

1. **Fork** the repo and create a branch from `main`
2. **Write code** following the standards above
3. **Add tests** for new functionality
4. **Run the full test suite** locally before pushing
5. **Run lint and security scans** — fix all issues
6. **Open a PR** with:
   - Clear title (conventional commit format)
   - Description of *what* and *why*
   - Test plan (what you tested, how to verify)
7. **CI must pass** — build, test, lint, security scan, Docker build
8. **Code review** — at least one approval required

### Review Checklist

Reviewers should verify:

- [ ] Tests cover new functionality (positive + negative cases)
- [ ] No new dependencies added without justification
- [ ] Security implications considered (auth, input validation, output sanitization)
- [ ] Logging includes request_id where applicable
- [ ] Error messages don't leak internal details to clients
- [ ] Documentation updated (README, SECURITY.md, RUNBOOK.md as needed)
- [ ] No secrets, credentials, or PII in code or comments

---

## Security Reporting

If you find a security vulnerability, **do not open a public issue**. Instead:

1. Email the maintainers directly (see repository contact info)
2. Include: description, reproduction steps, impact assessment
3. Allow 72 hours for initial response

See [SECURITY.md](SECURITY.md) for the full security policy and incident response procedures.

---

## Architecture Decisions

Before making significant changes, open a discussion issue first. Key principles:

- **Zero-dependency preference** — implement small utilities rather than importing libraries
- **Defense in depth** — every security feature should be one layer in a chain, not a single point of trust
- **Stateless where possible** — gateway auth, session tokens, rate limiting all work without shared state
- **Docker-first** — the deployment target is Docker Compose; bare-metal is not a priority
- **Observable** — every security decision should be loggable and measurable
