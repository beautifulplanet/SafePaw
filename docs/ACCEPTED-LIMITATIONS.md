# Accepted limitations (Windows / start.bat)

Decisions we made to keep the Windows launcher and start flow simple. Revisit if requirements change.

---

## 1. start.bat: fixed 30s wait (no health loop)

**Gap:** On Unix, `start.sh` waits up to **120 seconds** in a loop, checking `docker inspect safepaw-wizard` health until "healthy". On Windows, `start.bat` uses a **fixed 30-second** `timeout` and does not check container health.

**Why accepted:** Implementing a health loop in batch (polling `docker inspect`, parsing output) is brittle and verbose. The launcher already offers **[5] Quick health check** so the user can confirm wizard/gateway are up. START-CHECKLIST and VERIFY-LAUNCHER document the flow. If 30s is too short on slow machines, the user can wait and press [5] or open the wizard URL after.

**Mitigation:** Use **[5]** after start to confirm both healthy; or increase the fixed delay in `start.bat` (e.g. to 45–60s) if needed.

---

## 2. start.bat: no SYSTEM_PROFILE / RAM detection

**Gap:** `start.sh` detects system RAM and sets `SYSTEM_PROFILE` (small / medium / large / very-large) in `.env`, which drives Compose resource limits. `start.bat` does **not** set `SYSTEM_PROFILE`; Compose uses defaults from `.env.example` or existing `.env`.

**Why accepted:** RAM detection on Windows (e.g. via PowerShell `Get-CimInstance Win32_PhysicalMemory`) and writing into `.env` adds complexity and edge cases (WMI permissions, format). Defaults are safe for typical dev/demo use. Users with large machines can set `SYSTEM_PROFILE=large` (or `very-large`) in `.env` manually if they want higher limits.

**Mitigation:** Document in README or START-CHECKLIST that Windows users can set `SYSTEM_PROFILE` in `.env` for resource tuning; defaults are suitable for most setups.

---

## Summary

| Limitation | Mitigation |
|------------|------------|
| No health loop on Windows (fixed 30s) | Use launcher [5] after start; or increase timeout in start.bat if needed. |
| No RAM / SYSTEM_PROFILE on Windows | Set `SYSTEM_PROFILE` in `.env` manually for resource tuning; defaults are OK for typical use. |

These are **accepted** rather than deferred tech debt: the launcher and docs provide a clear path for users; full parity would add cost for limited benefit.
