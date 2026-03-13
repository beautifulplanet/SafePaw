# Next steps (after launcher SOW-002)

Launcher work (SOW-002 A–D) is complete. Below are the next optional improvements, in order of size/value.

---

## 1. Launcher [5] Quick health check (small) ✅ Done

- **What:** Add menu option [5] that hits wizard `http://localhost:3000/api/v1/health` and gateway `http://localhost:8080/health`, then reports OK or fail.
- **Where:** LAUNCH.bat (PowerShell Invoke-WebRequest) and LAUNCH.sh (curl); option [5] in both.
- **Value:** User can confirm stack is healthy without opening a browser or running curl manually.

---

## 2. .env CHANGE_ME check (small) ✅ Done

- **What:** Before [1] or [2] start, if `.env` contains `CHANGE_ME`, warn and ask "Start anyway? [y/N]"; only **y** continues.
- **Where:** LAUNCH.bat (findstr) and LAUNCH.sh (grep); runs only when .env exists. No check when .env is missing (start generates it without placeholders).
- **Value:** START-CHECKLIST "No placeholder secrets" is now automated at launcher start.

---

## 3. start.bat parity (medium) ✅ Documented as accepted

- **What:** On Windows, (a) wait for wizard/gateway health (loop like start.sh) instead of fixed 30s; (b) set SYSTEM_PROFILE in .env from RAM (like start.sh detect_profile).
- **Decision:** Documented as **accepted limitation** in **docs/ACCEPTED-LIMITATIONS.md**. start.bat stays simple; launcher [5] and START-CHECKLIST cover the risk. Users can increase the 30s timeout in start.bat or set SYSTEM_PROFILE in .env manually if needed.

---

## 4. Other project work (Phase 2, etc.)

- Cost monitoring Phase 2 (historical usage, trends).
- Gateway/wizard features per SOW-001, SCOPE-IMPROVEMENTS.md, SPEC-COST-MONITORING.md.
- Use WORKSTATE.md to set "current task" and "next task" per session (see AGENTS.md).

---

**Recommendation:** **1**, **2**, and **3** are done (3 = documented as accepted). Next: **4** (Phase 2 — cost monitoring, SCOPE-IMPROVEMENTS, or set current task in WORKSTATE.md).
