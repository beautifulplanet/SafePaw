# Next steps (after launcher SOW-002)

Launcher work (SOW-002 A–D) is complete. Below are the next optional improvements, in order of size/value.

---

## 1. Launcher [5] Quick health check (small) ✅ Done

- **What:** Add menu option [5] that hits wizard `http://localhost:3000/api/v1/health` and gateway `http://localhost:8080/health`, then reports OK or fail.
- **Where:** LAUNCH.bat (PowerShell Invoke-WebRequest) and LAUNCH.sh (curl); option [5] in both.
- **Value:** User can confirm stack is healthy without opening a browser or running curl manually.

---

## 2. .env CHANGE_ME check (small)

- **What:** Before [1] or [2] start, if `.env` contains `CHANGE_ME`, warn and optionally refuse to start (or ask "Start anyway? [y/N]").
- **Where:** LAUNCH.bat / LAUNCH.sh (and optionally start.bat / start.sh) before calling compose.
- **Value:** START-CHECKLIST already lists "No placeholder secrets"; this automates the check.

---

## 3. start.bat parity (medium)

- **What:** On Windows, (a) wait for wizard/gateway health (loop like start.sh) instead of fixed 30s; (b) set SYSTEM_PROFILE in .env from RAM (like start.sh detect_profile).
- **Alternative:** Document as accepted limitation (start.bat stays simple; launcher + START-CHECKLIST cover risk).
- **Value:** Closer parity with start.sh; fewer "started but not ready" cases on Windows.

---

## 4. Other project work (Phase 2, etc.)

- Cost monitoring Phase 2 (historical usage, trends).
- Gateway/wizard features per SOW-001, SCOPE-IMPROVEMENTS.md, SPEC-COST-MONITORING.md.
- Use WORKSTATE.md to set "current task" and "next task" per session (see AGENTS.md).

---

**Recommendation:** Do **1** (health check) next — small, self-contained, improves launcher UX. Then **2** if you want automated safety; **3** only if you want full Windows parity.
