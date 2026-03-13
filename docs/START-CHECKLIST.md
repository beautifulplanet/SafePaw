# SafePaw Start Checklist — Risk Analysis

Use this checklist before starting the stack (full or demo). All boxes should be satisfied before production use. If a risk cannot be checked, stop and plan mitigation or acceptance before proceeding.

**How to use:** Run the launcher (e.g. `LAUNCH.bat` or `start.bat`). For automated items, the script does the check. For manual items, tick mentally or in your process before first run and after any change that affects them.

---

## Pre-start checks

| # | Check | Automated? | How |
|---|-------|------------|-----|
| 1 | Docker installed and running | **Y** | start.sh / start.bat / LAUNCH.bat — script fails with clear message if Docker is not running. |
| 2 | Ports 3000 and 8080 free | **Y** (launcher) | LAUNCH.bat checks before [1]/[2]; if in use, warns and asks "Start anyway? [y/N]". Manual fallback: `netstat -ano \| findstr ":3000 :8080"` (Windows). See [VERIFY-LAUNCHER.md](VERIFY-LAUNCHER.md), [SAFEGUARDS.md](../SAFEGUARDS.md) §4. |
| 3 | .env exists or is generated with strong secrets | **Y** | start.sh / start.bat generate .env from .env.example with secure random (openssl/PowerShell). First run only. |
| 4 | No placeholder secrets in .env (e.g. CHANGE_ME) | **N** | Manual: after first run, confirm .env has no CHANGE_ME. Generated .env does not contain placeholders; if you edited .env.example and copied by hand, check. Optional future: script could refuse to start if CHANGE_ME present. |
| 5 | VPN / network conflict | **N** | Manual: disconnect VPN before start or add Docker subnet (172.16.0.0/12) to VPN split-tunnel. See [SAFEGUARDS.md](../SAFEGUARDS.md) §2. |
| 6 | Docker Desktop resource limits (Windows) | **N** | Manual: Settings > Resources — CPUs 2, Memory 4 GB max, Swap 1 GB, Disk 40 GB. See [SAFEGUARDS.md](../SAFEGUARDS.md) §1. |
| 7 | Disk space (e.g. ≥5 GB free) | **N** | Manual: `Get-PSDrive C` (PowerShell) or `df -h` (Unix). See [SAFEGUARDS.md](../SAFEGUARDS.md) §3. |
| 8 | Wizard admin password known or recoverable | **Doc** | First run: script prints it or sets WIZARD_ADMIN_PASSWORD in .env. Lost: [SECURITY.md](../SECURITY.md) § Recovery: lost wizard admin password. |

---

## After start

| # | Check | Automated? | How |
|---|-------|------------|-----|
| 9 | Wizard and gateway respond | **Y** (start.sh) / **Partial** (start.bat) | start.sh waits for wizard health up to 120s. start.bat uses fixed 30s; then open http://localhost:3000 and http://localhost:8080/health. |
| 10 | No crash-looping container | **N** | Manual: if a container keeps restarting, run `docker compose stop <service>` and fix before restart. See [SAFEGUARDS.md](../SAFEGUARDS.md) §6, [PITFALLS.md](../PITFALLS.md) D5. |

---

## References

- [SAFEGUARDS.md](../SAFEGUARDS.md) — Pre-run and while-running safeguards (VPN, ports, resources, emergency stop).
- [SECURITY.md](../SECURITY.md) — Defense-in-depth, recovery, hardening.
- [RUNBOOK.md](../RUNBOOK.md) — Incident playbooks; includes graceful shutdown verification.

**Rule:** If a new risk is found, add it to this checklist (automated or manual), then plan mitigation or acceptance before the next start.
