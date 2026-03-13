# SafePaw — project stats

Numbers you can verify. Update this file when counts change.

---

## Lines of code

| Scope | Lines (approx) | How to verify |
|-------|----------------|----------------|
| **Go + scripts (excl. vendor, node_modules, gen)** | ~22,800 | `Get-ChildItem -Recurse -Include *.go,*.bat,*.ps1,*.sh -File \| Where-Object { $_.FullName -notmatch '\\vendor\\|\\node_modules\\|\\gen\\' } \| Get-Content \| Measure-Object -Line` (PowerShell, from repo root). |
| **Go source only (services/gateway, services/wizard, shared)** | ~12,000 | Exclude *_test.go and gen for “production” code only. |

(Counts exclude generated code, vendor, node_modules. Include launcher scripts, start.bat/sh, Log-Launch.ps1, etc.)

---

## Tests

| Claim | How to verify |
|-------|----------------|
| **530 Go tests** (353 gateway + 177 wizard) | `cd services/gateway && go test ./... -count=1 -v 2>&1 | grep -c "=== RUN"` (gateway); same for wizard. See README § Evidence. |
| **7 fuzz targets** | `grep -r "^func Fuzz" services/gateway/` or `make fuzz`. |
| **CI** | GitHub Actions run gateway tests, wizard tests, lint, security, Docker build, OpenClaw compat, launcher smoke (`printf 'Q\n' \| ./LAUNCH.sh`). |

---

## Total time to build SafePaw (from scratch)

Rough estimate for **the full project** (gateway, wizard, Compose stack, STRIDE threat model, 6 runbooks, backup/restore, 530 tests, fuzz, CI, launcher, docs):

| Team size | Total time (order of magnitude) |
|-----------|----------------------------------|
| **Solo dev** | **12–18 months** (design, gateway, wizard, React, Docker, security, tests, docs, launcher, CI). |
| **Small team (2–3)** | **5–8 months** (parallel work on gateway vs wizard, shared ops/docs). |
| **With dedicated PM/security reviewer** | Add **1–2 months** for threat model, runbooks, and sign-off. |

Includes: Go gateway (auth, rate limits, 14-pattern scanner, output scanner, metrics, revocation); Go wizard + embedded React 19 (login, TOTP, dashboard, .env UI, audit); Docker Compose (5 services, health checks); 48-threat STRIDE model; 6 incident runbooks; backup/restore procedures; 530 tests + 7 fuzz targets; CI (build, test, lint, gosec, govulncheck, Docker, launcher smoke); launcher (menu, emergency stop, port/CHANGE_ME checks, health [5], logging).

---

## Launcher work only (for comparison)

**Just** the launcher slice (menu, emergency stop, SOW-002 A–D, health [5], CHANGE_ME, docs, CI launcher job):

- **Solo dev:** 1–2 weeks.
- **Small team (2–3):** ~1 week.
- **With PM (scope, SOW):** add 3–5 days.

---

## How to test the launcher (no digging)

- **One-page steps:** **docs/TEST-LAUNCHER.md** — Option A (automated script), Option B (manual 8-step checklist).
- **Automated (Windows):** From repo root: `.\scripts\Test-Launcher.ps1` — runs launcher with “Q”, expects exit 0 (may take up to 15 s if Docker is slow or not running).
- **Go tests:** From repo root: `cd services\gateway; go test ./... -count=1 -short` then `cd ..\wizard; go test ./... -count=1 -short`.
