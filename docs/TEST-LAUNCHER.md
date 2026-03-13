# One-page: How to test the launcher

**One place. No digging.** Run the steps below to verify the launcher works. Option A is automated (script); Option B is the manual checklist.

---

## Option A: Automated smoke test (run one command)

**Windows (PowerShell, from SafePaw root):**
```powershell
.\scripts\Test-Launcher.ps1
```
The script runs the launcher with piped input "Q" (quit) and checks exit code 0 and that menu text appears. It does not start Docker. Allow up to ~25 seconds (launcher may run `docker info`). If it hangs, run Option B instead.

**macOS/Linux:** CI already runs `printf 'Q\n' | ./LAUNCH.sh`; locally you can run the same. A shell equivalent of Test-Launcher.ps1 can be added if you want.

---

## Option B: Manual test (full verification, ~5 min)

Do this once after changes, or when you want to confirm everything end-to-end.

| Step | What to do | Pass when |
|------|------------|-----------|
| 1 | Open **LAUNCH.bat** (double-click or `LAUNCH.bat` from repo root). | Menu appears with Stack: STOPPED/RUNNING, options [1]–[5], Q. |
| 2 | Press **5** (Quick health check). | You see "Wizard :3000: unreachable", "Gateway :8080: unreachable", "[--] One or both not ready." (stack is stopped). |
| 3 | Press **4** (Show processes). | You see "No SafePaw or OpenClaw containers running." and the legend. |
| 4 | Press **2** (Demo). Wait for build and "Done". | Demo stack starts; browser or :3000 works. |
| 5 | Press **4** again. | Table shows safepaw containers (wizard, gateway, mockbackend, etc.). |
| 6 | Press **5** again. | You see "Wizard :3000: 200", "Gateway :8080: 200", "[OK] Both healthy." |
| 7 | Press **3** → type **y**. | "[OK] All stopped." |
| 8 | Run **STOP-SAFEPAW.bat** once. | "[OK] All SafePaw services stopped." (no-op if already stopped). |

If any step fails, see **docs/VERIFY-LAUNCHER.md** (§ If something fails).

---

## Project stats (lines, tests, team estimate)

See **docs/PROJECT-STATS.md** for:
- Total lines of code (~22.8k Go + scripts)
- Test count (530 Go tests; 7 fuzz) and how to verify
- Rough estimate: launcher work = 1–2 weeks solo; full SafePaw = many months

---

## Run Go tests (gateway + wizard)

From repo root:

```powershell
cd services\gateway; go test ./... -count=1 -short
cd ..\wizard; go test ./... -count=1 -short
```

Or use **Makefile** targets if present (e.g. `make test`). README documents the exact verification commands for test count and coverage.
