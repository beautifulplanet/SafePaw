# Changelog

All notable changes to SafePaw are documented here.  
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
This project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

*Nothing yet.*

## [0.2.0] — 2026-03-20

### Added
- **RBAC** — Three-tier role-based access control for wizard API (admin, operator,
  viewer). Login returns role; `RequireRole()` middleware enforces per-endpoint
  permissions. Env vars: `WIZARD_OPERATOR_PASSWORD`, `WIZARD_VIEWER_PASSWORD`.
- **Architecture Decision Records** — 6 ADRs documenting key design choices:
  HMAC tokens (not JWT), zero external middleware deps, Go for gateway, Docker
  socket proxy, heuristic scanning (not ML), embedded frontend.
- **Per-request audit trail** — `SecurityContext` struct rides on request context;
  `AuditEmitter` emits single JSON line per request with auth, rate-limit,
  scan results, and timing.
- **Trivy container scanning** in CI — Scans gateway and wizard Docker images
  for CRITICAL vulnerabilities.
- **Backup cron installer** — `scripts/backup-cron-install.sh` with configurable
  retention (default 30 days), `--remove` flag, logging.
- **API test collection** — `scripts/api-test-collection.sh` with 7 suites:
  health, auth, RBAC, security headers, invalid payloads, rate limiting,
  prompt injection.
- **Pitfalls checklist** — `PITFALLS.md` consolidating 40+ gotchas from all
  project documentation.
- **Penetration testing policy** — `docs/PENTEST-POLICY.md` with scope, testing
  types, rules of engagement, responsible disclosure, severity SLAs.
- **SOC 2 & GDPR compliance mapping** — `docs/COMPLIANCE.md` mapping controls
  to Trust Service Criteria (CC1–CC9) and GDPR articles with honest gap analysis.
- **Patching policy** — `docs/PATCHING-POLICY.md` with Dependabot workflow,
  severity SLAs, container base image strategy, dependency freeze process.
- **SecretsProvider interface** — `shared/secrets/` Go package defining pluggable
  secret backend interface with `EnvProvider` default implementation.
- **Secrets migration guide** — `docs/SECRETS-MIGRATION.md` with step-by-step
  Vault migration path and Docker Compose integration examples.
- **Encrypted backups** — `scripts/backup-encrypted.sh` wraps backup with GPG
  AES-256 symmetric encryption.
- **Restore verification** — `scripts/restore-verify.sh` non-destructive integrity
  check validating decryption, extraction, artifact presence, and .env keys.

### Changed
- Gateway coverage pushed to 65.2% (from 55%).
- Session token `Create()` now accepts `role` parameter for RBAC support.
  Backward compatible: tokens without role default to "admin".
- `SessionValidator` function signature changed from `func(string) bool` to
  `func(string) (string, bool)` — returns role and validity.

## [0.1.0] — 2026-03-01

First tagged release. Core security gateway and admin wizard operational.

### Added
- **Security gateway** — Go reverse proxy on :8080 with HMAC-SHA256 auth,
  per-IP rate limiting, brute-force detection with escalating bans, prompt
  injection scanning (14 patterns, v2.0.0), output scanning (XSS, secret
  leakage, encoding evasion), Prometheus metrics, structured JSON logging.
- **Admin wizard** — Go + React 19 dashboard on :3000 with login (HMAC session
  tokens), MFA (TOTP), service health monitoring, config editor, gateway
  metrics, prerequisite checks, service restart controls.
- **Docker Compose stack** — 6 services (gateway, wizard, OpenClaw, Redis,
  Postgres, Docker socket proxy) with pinned images.
- **Demo mode** — `docker-compose.demo.yml` with mock backend (no API keys
  needed).
- **Cost monitoring** — WebSocket-based usage collector querying OpenClaw
  for LLM token costs.
- **WebSocket proxy** — Full-duplex proxying with per-message output scanning.
- **Token revocation** — In-memory + Redis persistence for revoked tokens.
- **Mock backend** — `services/mockbackend` with /health, /echo, /status/:code,
  /payload/injection, /payload/xss, /delay endpoints.
- **Integration test suite** — `scripts/integration-gateway-mock.sh` for
  gateway testing without OpenClaw.
- **CI pipeline** — GitHub Actions with 6 jobs: gateway, wizard, lint, security,
  docker, OpenClaw compat. 65% coverage threshold.
- **Pre-commit hooks** — gofmt, go vet, unit tests.
- **Documentation** — README, SECURITY.md, THREAT-MODEL.md, RUNBOOK.md,
  BACKUP-RECOVERY.md, CONTRIBUTING.md, SAFEGUARDS.md, RELEASE.md,
  SCOPE-IMPROVEMENTS.md, SPEC-COST-MONITORING.md.

### Security
- Docker socket replaced with scoped proxy (tecnativa/docker-socket-proxy:0.3)
  allowing only container list/inspect/restart.
- CI actions pinned by SHA256 digest.
- Constant-time password comparison (`crypto/subtle`).
- CSP, X-Frame-Options, HSTS, Permissions-Policy headers on all responses.
- CSRF double-submit cookie protection on wizard API.
- Session invalidation on credential or TOTP change (generation counter).
- Log sanitization — secrets never appear in logs.
- Dependabot enabled for Go modules, npm, and GitHub Actions.

---

[Unreleased]: https://github.com/beautifulplanet/SafePaw/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/beautifulplanet/SafePaw/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/beautifulplanet/SafePaw/releases/tag/v0.1.0
