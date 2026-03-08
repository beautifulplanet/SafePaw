# Dependency & Patching Policy

> How SafePaw keeps dependencies current and vulnerabilities patched.

---

## Philosophy

SafePaw follows a **minimal-dependency** philosophy (see [ADR-002](docs/adr/002-zero-external-middleware-deps.md)):

| Ecosystem | Dependency count | Rationale |
|-----------|-----------------|-----------|
| Gateway Go | 2 (`coder/websocket`, `google/uuid`) | Fewer deps = smaller attack surface |
| Wizard Go | 0 direct middleware deps | Hand-rolled session, rate limiter, Redis client |
| Wizard UI (npm) | ~15 direct (React, Vite, Tailwind) | Frontend toolchain only; not shipped in Go binary |
| GitHub Actions | 4 (checkout, setup-go, setup-node, cache) | Pinned to major version tags |

## Automated Tooling

### Dependabot (configured in `.github/dependabot.yml`)

| Ecosystem | Directory | Schedule | PR Limit |
|-----------|-----------|----------|----------|
| `gomod` | `/services/gateway` | Weekly (Monday) | 5 |
| `gomod` | `/services/wizard` | Weekly (Monday) | 5 |
| `npm` | `/services/wizard/ui` | Weekly (Monday) | 5 |
| `github-actions` | `/` | Weekly (Monday) | 5 |

All Dependabot PRs are labeled (`dependencies`, `go`/`npm`/`ci`) and assigned
to `beautifulplanet` for review.

### govulncheck (CI — every commit)

```yaml
# .github/workflows/ci.yml → security job
- run: go install golang.org/dl/gotip@latest && gotip download
- run: govulncheck ./...    # advisory mode (continue-on-error)
```

Scans Go binaries against the Go vulnerability database. Currently advisory
(non-blocking) — tracked in
[#13](https://github.com/beautifulplanet/SafePaw/issues/13) to promote to
hard-fail once govulncheck stabilizes.

### Trivy (CI — every commit)

```yaml
# .github/workflows/ci.yml → docker job
trivy image --exit-code 1 --severity CRITICAL safepaw-gateway:latest
trivy image --exit-code 1 --severity CRITICAL safepaw-wizard:latest
```

Blocks CI on any CRITICAL CVE in the built Docker images.

---

## Patching SLAs

| Severity | Source | Response time | Action |
|----------|--------|--------------|--------|
| Critical CVE | Trivy, govulncheck, GitHub Advisory | ≤ 24 hours | Patch, rebuild, and deploy immediately |
| High CVE | Trivy, govulncheck, Dependabot | ≤ 72 hours | Merge Dependabot PR or apply manual fix |
| Medium CVE | Dependabot, govulncheck | ≤ 2 weeks | Review and merge in next regular batch |
| Low / Informational | Dependabot | Next scheduled release | Bundle with other updates |
| Go minor/patch release | Go release blog | ≤ 1 week | Update CI matrix and Dockerfiles |
| npm minor/patch | Dependabot PR | ≤ 1 week | Merge after CI passes |

---

## Review Process for Dependency Updates

### Automated checks (CI must pass)

1. `go test -race ./...` — no regressions
2. `go vet ./...` — no new warnings
3. `govulncheck ./...` — no new advisories
4. `trivy image --severity CRITICAL` — no new critical CVEs
5. `npm run build` (wizard UI) — frontend still builds
6. Coverage gate (≥ 65% gateway)

### Manual review checklist (for each Dependabot PR)

- [ ] Read the changelog/release notes for the update
- [ ] Check if the update is a major version bump (breaking changes?)
- [ ] Verify the maintainer is still active and trusted
- [ ] Confirm no new transitive dependencies introduced (`go mod graph`)
- [ ] Check license compatibility (MIT/BSD/Apache 2.0 only)
- [ ] Run `go mod tidy` — no unexpected additions
- [ ] For npm: check `npm audit` output

### Major version bumps

Major version bumps require additional review:

1. Read the migration guide
2. Run full test suite locally (not just CI)
3. Test the demo flow end-to-end (`scripts/api-test-collection.sh all`)
4. Update ADRs if the upgrade changes an architectural assumption
5. Document the change in CHANGELOG.md

---

## Container Base Image Policy

| Image | Current base | Update strategy |
|-------|-------------|----------------|
| Gateway | `golang:1.24-alpine` → `scratch` (multi-stage) | Rebuild on Go release; alpine for build only |
| Wizard | `golang:1.24-alpine` → `scratch` (multi-stage) | Same as gateway |
| Postgres | `postgres:16-alpine` | Dependabot for Docker; manual review for major PG versions |
| Redis | `redis:7-alpine` | Dependabot for Docker |
| Docker Socket Proxy | `tecnativa/docker-socket-proxy:0.3` | Pin version; update quarterly |

**Alpine base**: Minimal attack surface. No shell in production (`scratch` final stage).

---

## Dependency Freeze Policy

Before a tagged release:

1. Freeze dependency updates 48 hours before release
2. Run `govulncheck` and `trivy` one final time
3. Pin all Go dependencies to exact versions in `go.sum`
4. npm: run `npm audit --production` and resolve any high/critical
5. Document frozen versions in CHANGELOG.md release notes

---

## Exceptions

| Scenario | Policy |
|----------|--------|
| Zero-day CVE in a direct dependency | Skip freeze; patch, test, and release immediately |
| Dependabot PR failing CI | Investigate within SLA; if upstream bug, open issue and pin previous version |
| Abandoned upstream dependency | Fork or replace within 30 days; document decision in ADR |
| License change in dependency | Evaluate immediately; replace if non-permissive |

---

## References

- [.github/dependabot.yml](../.github/dependabot.yml) — Dependabot configuration
- [.github/workflows/ci.yml](../.github/workflows/ci.yml) — CI pipeline with security checks
- [docs/adr/002-zero-external-middleware-deps.md](docs/adr/002-zero-external-middleware-deps.md) — Why minimal deps
- [CHANGELOG.md](../CHANGELOG.md) — Release history
- [docs/PENTEST-POLICY.md](PENTEST-POLICY.md) — Severity SLAs
- [docs/COMPLIANCE.md](COMPLIANCE.md) — SOC 2 CC9.2 vendor risk mapping
