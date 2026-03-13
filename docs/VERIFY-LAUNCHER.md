# Verify Launcher and Logging

Quick checks that LAUNCH.bat (Windows), LAUNCH.sh (macOS/Linux), STOP-SAFEPAW.bat, logging, START-DEMO.ps1, and docs work.

---

## 0. Emergency stop (STOP-SAFEPAW.bat)

1. In the SafePaw repo root, double-click **STOP-SAFEPAW.bat**.
2. Window should show "Stopping all SafePaw containers..." then "[OK] All SafePaw services stopped." (red title bar = emergency stop).
3. Run it again when the stack is already stopped — it should still say "[OK] All stopped" (harmless).
4. **Desktop shortcut:** Right-click **STOP-SAFEPAW.bat** → **Send to** → **Desktop (create shortcut)**. Move the shortcut to the desktop. Double-click the shortcut — it must run from the SafePaw folder (script uses `%~dp0`). If you moved the shortcut and it fails with "SafePaw folder not found", edit the shortcut: **Target** = full path to `SafePaw\STOP-SAFEPAW.bat`, **Start in** = full path to the `SafePaw` folder (e.g. `C:\Users\You\...\SafePaw`).

---

## 1. LAUNCH.bat menu

1. Open a command prompt or Explorer, go to the SafePaw repo root.
2. Double-click **LAUNCH.bat** (or run `LAUNCH.bat` from cmd).
3. You should see the green menu with **Stack: STOPPED** or **Stack: RUNNING** at the top, then [1]–[5] and Q.
4. Confirm stack status: if no containers are running, it must say STOPPED; if you started the stack, it must say RUNNING.
5. Press **4** (Show processes). You should see either an empty table or a table of safepaw containers, then "Snapshot logged to logs\...".
6. Press **5** (Quick health check). When stack is running: Wizard :3000: 200, Gateway :8080: 200, "[OK] Both healthy." When stopped: unreachable and "[--] One or both not ready."
7. Press **3** (Shut down). It runs `docker compose down` for both compose files and says "[OK] All stopped."
8. Press **Q** to quit.

---

## 1b. LAUNCH.sh (macOS / Linux)

1. In the SafePaw repo root: `chmod +x LAUNCH.sh` (if needed), then `./LAUNCH.sh`.
2. You should see the same menu as LAUNCH.bat (Stack: RUNNING/STOPPED, [1]–[5], Q).
3. Press **4** to show processes, **5** for quick health check (wizard + gateway), **3** to shut down, **Q** to quit. Menu loops after each action.
4. **CI:** The workflow runs `printf 'Q\n' | ./LAUNCH.sh` and expects exit 0.

---

## 2. Log files created

1. In the SafePaw repo, open the **logs** folder.
2. You should see a date folder (e.g. `2026-03-10`).
3. Inside it: **processes** and **shutdown** (and **full** or **demo** if you ran [1] or [2]).
4. Open `logs\YYYY-MM-DD\processes\part1.log` — should contain a timestamp line and "Show processes" plus a docker ps snapshot (or "docker ps" output).
5. Open `logs\YYYY-MM-DD\shutdown\part1.log` — should contain "Shut down requested" and "Shut down completed" with timestamps.

---

## 3. START-DEMO.ps1 (PowerShell)

From the SafePaw repo root in PowerShell:

```powershell
.\START-DEMO.ps1 --stop
```
Should print "Stopping SafePaw services..." and "[OK] All stopped."

```powershell
.\START-DEMO.ps1 --demo
```
Should start the demo stack (Docker must be running). Then:

```powershell
.\START-DEMO.ps1 --stop
```
Should stop it.

---

## 4. Docs exist

| Doc | What to check |
|-----|----------------|
| **docs/START-CHECKLIST.md** | Open the file. You should see a table of pre-start checks (Docker, ports, .env, VPN, etc.) with Automated? Y/N. |
| **RUNBOOK.md** | Open RUNBOOK.md. Near the top, after "Related:", there should be a section **"Graceful shutdown verification"** with steps to check gateway/wizard logs after stop. |

---

## 5. Full stack start/stop (optional)

Only if you have Docker running and want to test the full flow:

1. **LAUNCH.bat** → press **1** (Full) or **2** (Demo). Wait for "Done" and browser to open.
2. **LAUNCH.bat** → press **4** to see processes. Check `logs\date\full\part1.log` or `logs\date\demo\part1.log` has an entry.
3. **LAUNCH.bat** → press **3** to shut down.
4. (Optional) Verify graceful shutdown:  
   `docker compose logs gateway --tail=5` and `docker compose logs wizard --tail=5` — look for `[SHUTDOWN]` / "Gateway stopped" (see RUNBOOK.md § Graceful shutdown verification).

---

## 6. Full verification order (recommended once)

Run in this order to confirm everything works:

| Step | Action | What to check |
|------|--------|----------------|
| 1 | Start demo: **LAUNCH.bat** → [2] | Build finishes, "Done", browser opens or :3000 works |
| 2 | **LAUNCH.bat** → [4] | Table shows safepaw containers (wizard, gateway, mockbackend, etc.) |
| 3 | **LAUNCH.bat** → [3] → type **y** | "[OK] All stopped." |
| 4 | **STOP-SAFEPAW.bat** (from repo or desktop shortcut) | "[OK] All SafePaw services stopped." (no-op if already stopped) |
| 5 | **LAUNCH.bat** → [4] | "No SafePaw or OpenClaw containers running." |
| 6 | Check logs under `logs\YYYY-MM-DD\` | shutdown and processes folders have part1.log entries |

---

## 7. Logging failure and port check (SOW-002-B, SOW-002-C)

- **Logging failure:** If Log-Launch.ps1 fails, the launcher shows "Warning: launcher log could not be written. Check logs\launcher-errors.txt" and appends a line to `logs\launcher-errors.txt`. **Manual test:** rename `Log-Launch.ps1` to `Log-Launch.ps1.bak`, run LAUNCH.bat and choose any option; you should see the warning and a new line in `logs\launcher-errors.txt`. Restore the script afterward.
- **Port check:** Before [1] or [2], the launcher checks if port 3000 or 8080 is in use (`netstat`). If so, it shows "Port 3000 or 8080 is in use..." and asks "Start anyway? [y/N]"; only **y** continues. **Manual test:** bind one port (e.g. run `python -m http.server 3000` in another window), then LAUNCH.bat → [2]; you should see the warning and prompt. Press **N** to return to menu.

## 8. What's missing / how to make it better

| Gap | Status / idea |
|-----|----------------|
| **Emergency stop** | Done — **STOP-SAFEPAW.bat** + desktop shortcut (§0). |
| **Pre-start port check** | Done — SOW-002-C; launcher warns and asks "Start anyway? [y/N]" (§7). |
| **Logging failures not silent** | Done — SOW-002-B; warning + `logs\launcher-errors.txt` (§7). |
| **Health check from launcher** | Done — option [5] in LAUNCH.bat and LAUNCH.sh; hits wizard :3000 and gateway :8080, reports 200 or unreachable. |
| **Unix launcher** | Done — **LAUNCH.sh** (SOW-002-D). Run `./LAUNCH.sh` on macOS/Linux for same menu as LAUNCH.bat. |
| **CI smoke test** | Done — job `launcher` in CI runs `printf 'Q\n' | ./LAUNCH.sh` and expects exit 0 (SOW-002-D). |

To verify the launcher "works fully": run section **6** above, then §7 manual tests if you want to confirm log-failure and port-check behavior.

---

## If something fails

- **LAUNCH.bat doesn't open / errors:** Run from cmd so you see the error. Ensure you're in the SafePaw repo root (where start.bat and LAUNCH.bat live).
- **No logs folder:** Log-Launch.ps1 runs via PowerShell. If PowerShell is disabled or missing, the batch file may still run start/stop but logging might fail. Check that `Log-Launch.ps1` exists in the repo root.
- **START-DEMO.ps1 --stop fails:** Ensure Docker is installed; the script runs `docker compose down`. If no stack is running, it's harmless.
