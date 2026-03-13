# Plan: One-Click Start/Stop + Safety (Two Scopes)

**Date:** 2026-03-10  
**Status:** Implemented (menu, logs, checklist, graceful shutdown doc, START-DEMO.ps1 aligned)  
**Intent:** One-click start (SafePaw + OpenClaw, everything needed, SAFELY) and one-button off with graceful shutdown; optional “little menu.” Use Google PM–style risk analysis to check all security/safety boxes. If we find gaps, stop and plan agile, then next waterfall per scope.

---

## 1. Current state (what exists today)

### 1.1 One-click start

| Item | Exists? | Where | Notes |
|------|---------|-------|-------|
| Start full stack (wizard + gateway + OpenClaw + Redis + Postgres) | Yes | `start.sh`, `start.bat` | No args = full. Generates `.env` if missing (secure random secrets). |
| Start demo (wizard + gateway + mockbackend) | Yes | `start.sh --demo`, `start.bat --demo` | No OpenClaw, no API key. |
| .env generation with secure defaults | Yes | start.sh (openssl/urandom), start.bat (PowerShell) | REDIS, POSTGRES, AUTH_SECRET, WIZARD_ADMIN_PASSWORD, OPENCLAW_GATEWAY_TOKEN. |
| Docker check before start | Yes | start.sh, start.bat | Fail if Docker not running. |
| Health wait after up | Yes (sh) / Partial (bat) | start.sh: wait for wizard healthy up to 120s; start.bat: fixed 30s timeout | Windows has no health-loop. |
| RAM profile detection (full only) | Yes (sh) / No (bat) | start.sh: detect_profile, SYSTEM_PROFILE in .env | start.bat does not set profile. |
| Open browser after start | Yes | start.sh (xdg-open/open), start.bat (start URL) | |
| START-DEMO.ps1 | Yes | Root | Starts **full** compose only (not demo). No --stop. Says “docker compose down” in text. |

### 1.2 One-click stop

| Item | Exists? | Where | Notes |
|------|---------|-------|-------|
| Stop all services | Yes | `start.sh --stop`, `start.bat --stop` | Runs `docker compose down` + `docker-compose.demo.yml down`. |
| Graceful shutdown in services | Yes | gateway/main.go, wizard/cmd/wizard/main.go | Both register SIGINT/SIGTERM and call server.Shutdown(). |
| Compose stop behavior | Yes | Docker Compose default | Sends SIGTERM, then SIGKILL after timeout (default 10s). |

### 1.3 Safety / pre-run guidance

| Item | Exists? | Where | Notes |
|------|---------|-------|-------|
| Pre-run safeguards (VPN, ports, resources) | Prose only | SAFEGUARDS.md | Not a ticked checklist; not automated. |
| Emergency stop instructions | Yes | SAFEGUARDS.md | Task Manager → End Docker/vmmem. |
| Runbooks (incident response) | Yes | RUNBOOK.md | 6 playbooks. |
| Formal risk checklist (Google PM style) | No | — | No “all boxes checked before start” artifact. |

### 1.4 “Little menu”

| Item | Exists? | Where | Notes |
|------|---------|-------|-------|
| Single entry point (Start full / Start demo / Stop / Logs) | No | — | User must run script + flag. No GUI menu, no single “launcher” that offers choices. |

---

## 2. Scope 1: One-click start — SafePaw + OpenClaw, one click, everything needed, SAFELY

**Goal:** One action starts the full stack (or demo) with all prerequisites and safety checks satisfied. Use risk-analysis methods so all security and safety boxes that can be checked are checked.

### 2.1 Acceptance criteria (Scope 1)

- One entry point (script or menu) starts either full stack or demo.
- All required env/secrets are present or generated securely (no weak defaults).
- Pre-start checks: Docker running; ports 3000/8080 free; optional: disk, RAM, VPN guidance.
- Post-start: health checks waited on (or documented wait); user gets Wizard URL + gateway URL + password (or where to find it).
- Risk checklist: a defined list (Google PM–style) of security/safety items, each either automated (script checks) or manual (human ticks before first run or after change). Document where it lives and that “all boxes must be checked before production use.”

### 2.2 Risk-analysis checklist (to define and satisfy)

Conceptually, “check all boxes” should cover at least:

| # | Check | Automated? (Y/N) | Where / how |
|---|-------|------------------|-------------|
| 1 | Docker installed and running | Y | Already in start.sh/bat |
| 2 | Ports 3000, 8080 free | N today | Add to script or doc |
| 3 | .env exists or is generated with strong secrets | Y | Already |
| 4 | No placeholder secrets in .env (e.g. CHANGE_ME) | Could be Y | Add check or doc |
| 5 | VPN / network conflict guidance | N | SAFEGUARDS.md; could add to checklist |
| 6 | Resource limits (Docker Desktop RAM/CPU) | N | SAFEGUARDS.md; manual |
| 7 | Disk space (e.g. ≥5 GB free) | N today | Optional script check |
| 8 | Wizard password known or recoverable | Doc | SECURITY.md recovery; could print or link |

If any box cannot be checked (e.g. new risk found), **stop and plan agile**: document the risk, decide mitigate vs accept, then add to checklist and next waterfall.

### 2.3 Gaps (Scope 1)

- No single “risk checklist” artifact that ties together SAFEGUARDS + env + ports + resources. Either add a `docs/START-CHECKLIST.md` (or similar) or embed a short checklist in start script comments / README.
- start.bat: no health-loop (fixed 30s); no RAM profile. Parity with start.sh or document Windows limitations.
- No automated port check in scripts (only in SAFEGUARDS/PITFALLS as “do this before start”).
- Optional: script check for .env placeholders (CHANGE_ME) and refuse to start or warn.

---

## 3. Scope 2: Graceful shutdown + one-button off + menu

**Goal:** (1) Stack fails and shuts down gracefully when something goes wrong or when user stops it. (2) One-button off. (3) Optional “little menu” for start/stop (and maybe logs).

### 3.1 Acceptance criteria (Scope 2)

- **Graceful shutdown:** When user runs “stop,” all services receive SIGTERM and shut down without leaving orphan processes or corrupt state. Evidence: gateway and wizard already call `server.Shutdown()` on SIGTERM; document that and optionally add a small verification step (e.g. run `down`, inspect logs for clean exit).
- **Fail and shut down gracefully:** If one container or the host is in a bad state, “stop” still runs and brings down the stack (docker compose down is idempotent). Document: “If something is wrong, run stop; then fix; then start.” No requirement that a crashed process “cleans up” on its own—the one-button off is the human/script action.
- **One-button off:** Single command or single menu action stops all services (full and demo). **Exists:** `start.sh --stop` / `start.bat --stop`. Confirm behavior on Windows (START-DEMO.ps1 does not expose --stop; user must run start.bat --stop or `docker compose down`).
- **Little menu (optional):** One entry point (e.g. script or tiny UI) that offers: Start full / Start demo / Stop all / Logs. Not in repo today; either add a small wrapper (e.g. PowerShell menu, or batch menu) or document “use start.bat --stop and start.bat or start.bat --demo” as the “menu” for now.

### 3.2 Gaps (Scope 2)

- **Menu:** No “little menu” exists. Agreed earlier it would be “a little menu or something”—currently only CLI flags. Plan: add a minimal launcher (e.g. PowerShell script that prompts “1=Full 2=Demo 3=Stop 4=Logs” and runs the right command) or document CLI as the interface.
- **Verification of graceful shutdown:** Not automated. Plan: add one verification step (e.g. “run stop, then check gateway/wizard logs for shutdown message”) to docs or to a “verify” script.
- **START-DEMO.ps1:** Does not support Stop or Demo mode; name implies “demo” but it runs full compose. Align with start.bat (e.g. support same flags) or rename and document.

---

## 4. Summary: do we have these features?

| Feature | Have it? | Notes |
|---------|----------|--------|
| One-click start (full stack) | Yes | start.sh, start.bat, LAUNCH.bat [1] |
| One-click start (demo) | Yes | start.sh --demo, start.bat --demo, LAUNCH.bat [2] |
| Safe .env generation | Yes | Strong random; no CHANGE_ME in generated values |
| One-button stop | Yes | start.sh --stop, start.bat --stop, LAUNCH.bat [3], START-DEMO.ps1 --stop |
| Graceful shutdown (services) | Yes | Gateway and wizard handle SIGTERM + Shutdown() |
| Risk checklist (all boxes) | **Yes** | docs/START-CHECKLIST.md (2026-03-10) |
| Little menu (Start full / demo / Stop / processes) | **Yes** | LAUNCH.bat (Civil Zones style); logs to logs\date\action\partN.log |
| Graceful shutdown verification | **Yes** | RUNBOOK.md § Graceful shutdown verification |
| START-DEMO.ps1 aligned | **Yes** | Supports --demo, --stop; LAUNCH.bat preferred entry |
| Windows parity (health wait, profile) | Partial | start.bat: 30s fixed wait; no profile |

---

## 5. Implementation completed (2026-03-10)

- **LAUNCH.bat:** Menu [1]=Full [2]=Demo [3]=Stop [4]=Show processes; logs via Log-Launch.ps1.
- **Log-Launch.ps1:** logs\YYYY-MM-DD\{full|demo|shutdown|processes}\part1.log (part2 when part1 exceeds 100KB).
- **docs/START-CHECKLIST.md:** Risk checklist (automated vs manual); references SAFEGUARDS, SECURITY, RUNBOOK.
- **RUNBOOK.md:** New § “Graceful shutdown verification” — how to confirm clean stop from gateway/wizard logs.
- **START-DEMO.ps1:** Aligned with start.bat (--demo, --stop); comment points to LAUNCH.bat as preferred.

---

## 6. Next steps (optional / future)

- start.bat parity: health-loop and RAM profile on Windows, or document as accepted limitation.
- Optional: script check for .env placeholder (CHANGE_ME) and warn or refuse to start.
- Optional: port check in LAUNCH.bat or start.bat before start (fail fast if 3000/8080 in use).
