# SOW-002 — Launcher "Google CTO Bar" (Multi-SOW)

> **Status:** DRAFT — not yet approved  
> **Issued:** 2026-03-10  
> **Parties:** beautifulplanet (Owner) · Implementation (per AGENTS.md workflow)  
> **Parent:** plan/2026-03-10-002-one-click-start-stop-safety.md (launcher implemented); this SOW raises the bar to reliability/ops standard.

---

## Purpose

The SafePaw launcher (LAUNCH.bat, Log-Launch.ps1) is usable and correct. This SOW defines three **reliability/ops** deliverables that bring it to a "Google CTO bar": visible stack status, non-silent logging failure, and pre-start port check. Each deliverable is a separate **work package** with its own acceptance criteria and done-when. A fourth section covers **polish** (README, Unix launcher, CI) as optional/scale.

Work is executed per AGENTS.md: tiny tasks, Plan A/B, strict scope, document artifact. No work outside these SOWs without a Change Order.

---

## SOW-002-A: Stack status on menu ✅ DELIVERED

### Scope

When the launcher menu is drawn, show the user at a glance whether the SafePaw stack is **RUNNING** or **STOPPED** (and optionally which mode: full vs demo). No need to press [4] to discover state.

### Acceptance criteria

| # | Criterion | Done when |
|---|-----------|-----------|
| A1 | On each menu draw (including first open and after returning from an action), the launcher determines stack state. | Logic exists and is invoked before the "Enter [1-4] or Q" prompt. |
| A2 | State is displayed in the menu header or immediately below the title (e.g. "Stack: RUNNING" or "Stack: STOPPED"). | User sees one of these without pressing any option. |
| A3 | "RUNNING" is shown only when at least one SafePaw-related container is running (e.g. `docker ps --filter "name=safepaw"` returns at least one row). | No false positive (e.g. stale state). |
| A4 | Determination is fast (e.g. single `docker ps` filter or one health check). No long blocking. | Menu appears within a few seconds on a normal machine. |
| A5 | If Docker is not running or not in PATH, state is shown as "UNKNOWN" or "Docker not available" (no crash). | Launcher does not exit or hang. |

### Out of scope

- Showing per-service status (wizard up, gateway up, etc.) in the menu. Option [4] remains the way to see process list.
- Changing how start/stop works; only the **display** of status is in scope.

### Evidence

- LAUNCH.bat (or a called script) contains the status check and echo of "Stack: RUNNING | STOPPED | UNKNOWN".
- VERIFY-LAUNCHER.md or equivalent updated with step: "Open menu → confirm Stack: STOPPED or RUNNING is shown."

---

## SOW-002-B: Logging failures not silent ✅ DELIVERED

### Scope

If Log-Launch.ps1 fails (e.g. ExecutionPolicy, path too long, permission, script missing), the user is informed. No silent swallow of errors so that logs appear to work but are not written.

### Acceptance criteria

| # | Criterion | Done when |
|---|-----------|-----------|
| B1 | When the launcher invokes Log-Launch.ps1, failure of that invocation is detectable (e.g. capture exit code or stderr). | LAUNCH.bat or wrapper checks result of the PowerShell call. |
| B2 | On failure, the user sees a clear one-line message (e.g. "Warning: launcher log could not be written — check logs\launcher-errors.txt or run from SafePaw folder."). | Message is visible in the same window (no suppression with >nul for the error path). |
| B3 | Optional but recommended: first failure writes a short error line to a fixed path (e.g. `logs\launcher-errors.txt`) so the user can inspect even if the window was closed. | File exists and contains timestamp + brief reason when a failure occurred. |
| B4 | Successful logging remains silent (no spam). Only failure path produces message and/or file. | Normal runs unchanged. |

### Out of scope

- Retry logic for logging. Single attempt is enough; inform user on failure.
- Changing Log-Launch.ps1 internal behavior (e.g. more robust paths) unless needed to meet B1–B4.

### Evidence

- LAUNCH.bat (or wrapper) implements check and message (and optionally launcher-errors.txt).
- Manual test: temporarily break logging (e.g. rename Log-Launch.ps1); run launcher and choose an option; confirm warning appears (and optionally launcher-errors.txt).

---

## SOW-002-C: Pre-start port check ✅ DELIVERED

### Scope

Before starting the stack (options [1] or [2]), the launcher checks whether ports 3000 and 8080 are already in use. If either is in use, the user is warned (and optionally blocked from starting until they acknowledge or free the port). **Behavior chosen:** warn and ask "Start anyway? [y/N]"; only call start.bat if user confirms. If both ports free, start proceeds with no prompt.

### Acceptance criteria

| # | Criterion | Done when |
|---|-----------|-----------|
| C1 | Immediately before calling start.bat for [1] or [2], the launcher runs a port check for 3000 and 8080 (e.g. `netstat` or PowerShell Test-NetConnection / socket). | Check is implemented and runs only for start actions. |
| C2 | If either port is in use, the user sees a clear message (e.g. "Port 3000 or 8080 is in use. Free the port or stop the other app, then try again."). | Message is visible in the launcher window. |
| C3 | Behavior is one of: (a) block start and return to menu after message + pause, or (b) warn and ask "Start anyway? [y/N]" and only call start.bat if user confirms. | Documented in this SOW and in START-CHECKLIST or VERIFY-LAUNCHER which behavior was chosen. |
| C4 | If both ports are free, start proceeds as today (no extra prompt). | No regression for the normal case. |
| C5 | Port check is lightweight (no long timeout). | Menu/start flow remains responsive. |

### Out of scope

- Checking other ports (e.g. 6379, 5432). Only 3000 and 8080 (host-exposed) are in scope.
- Modifying start.bat or start.sh to do the check; launcher (LAUNCH.bat / helper script) does the check before calling start.bat.

### Evidence

- LAUNCH.bat or a small helper (e.g. PowerShell or batch) contains the port check and message.
- docs/START-CHECKLIST.md or VERIFY-LAUNCHER.md updated to mention that the launcher now checks ports before start.
- Manual test: bind 3000 or 8080 with another process; run launcher [1] or [2]; confirm message and no start (or confirm "Start anyway?" flow).

---

## SOW-002-D: Polish (optional / scale) ✅ DELIVERED

The following are **not** required for "CTO bar" but improve discoverability and cross-platform/CI.

### D.1 README launcher mention ✅

- **Scope:** In README.md Quick start (or equivalent), add one line that tells Windows users to run **LAUNCH.bat** for the menu (start/stop/status).
- **Done when:** README contains the line; new users can find the launcher from the main doc. **Done:** Windows and macOS/Linux (LAUNCH.sh) both mentioned in README.

### D.2 Unix launcher (LAUNCH.sh) ✅

- **Scope:** Provide a shell script (LAUNCH.sh) that offers the same menu (1=Full, 2=Demo, 3=Stop, 4=Show processes) and calls start.sh with the right args. Loop back to menu like LAUNCH.bat.
- **Done when:** LAUNCH.sh exists, is executable, and works on at least one Unix-like environment (e.g. macOS or Linux); documented in README. **Done:** LAUNCH.sh added; README updated.

### D.3 CI smoke test for launcher ✅

- **Scope:** One CI job (e.g. in .github/workflows) that runs the launcher with a non-interactive input (e.g. echo "3" | LAUNCH.bat or equivalent) to verify the script runs and exits 0. No requirement to actually start the stack; only "script runs without crash."
- **Done when:** Job is in CI and green; any launcher regression (e.g. path, syntax) is caught. **Done:** Job `launcher` runs `printf 'Q\n' | ./LAUNCH.sh` on ubuntu-latest and expects exit 0.

---

## Summary table

| SOW       | Deliverable              | Required for CTO bar | Priority | Status   |
|-----------|--------------------------|----------------------|----------|----------|
| SOW-002-A | Stack status on menu     | Yes                  | 1        | Delivered|
| SOW-002-B | Logging failures visible | Yes                  | 2        | Delivered|
| SOW-002-C | Pre-start port check     | Yes                  | 3        | Delivered|
| SOW-002-D | Polish (README, Unix, CI)| No                   | Optional | Delivered|

---

## Change order process

Any change to scope, acceptance criteria, or new work packages under this document follows the same Change Order process as SOW-001 (docs/scope/CO-NNN.md). No out-of-scope work without an approved CO.
