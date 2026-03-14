# How to test SafePaw — and where we are

**For:** Telling people you have a working product. No rush — use this when you’re ready to verify or demo.

---

## Where we are (short)

| Mode | Works? | What you can say |
|------|--------|-------------------|
| **Demo** | Yes, out of the box | “One-click demo: launcher → Start Demo → browser opens. You get the control panel, security dashboard, and a mock AI backend. No API keys or OpenClaw needed.” |
| **Full stack** | Yes, with one extra step | “Full product: you need OpenClaw (or any HTTP backend) and API keys in `.env`. Same launcher → Start Full. Then you have a real AI behind the gateway with auth, rate limiting, and scanning.” |

So: **we have a working product.** Demo proves the UX and security layer; full stack proves it with a real backend.

---

## How to test it (minimal path)

**Prereqs:** Docker Desktop installed and running. Ports 3000 and 8080 free.

### Option A — Demo (recommended first)

1. Open the SafePaw repo in Explorer; double‑click **LAUNCH.bat**.
2. In the menu, press **2** (Start Demo). Wait for “Done” (and browser may open to http://localhost:3000).
3. If the browser didn’t open, go to **http://localhost:3000**.
4. **First time only:** The wizard will have printed a one-time password in the launcher window. Copy it and sign in. (Or check `docker compose logs wizard` for the line `[INFO] Auto-generated admin password: ...`.)
5. You should see the **Home** dashboard (services, stats if the mock is up). Click **Security** (activity), **Settings** (config). That’s the product.

**Quick health check:** In the launcher menu press **5**. You should see `Wizard :3000: 200`, `Gateway :8080: 200`, and “[OK] Both healthy.”

**Shut down:** In the launcher press **3** (Shut down), then **y**. Or double‑click **STOP-SAFEPAW.bat** for an emergency stop.

---

### Option B — Full stack (real AI)

You need either:

- OpenClaw cloned next to SafePaw (e.g. `.../openclaw` next to `.../SafePaw`) and built, **or**
- Another HTTP backend and set `PROXY_TARGET` in `.env` to point at it.

Then:

1. Put your API keys (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) and any channel tokens in `.env`. (First run: **LAUNCH.bat** → **1** (Full) will generate `.env` from `.env.example`; then edit `.env` and add keys, or use the wizard Settings after first login.)
2. **LAUNCH.bat** → **1** (Start Full). Wait for “Done”.
3. Open http://localhost:3000, sign in (password from first-run log or the one you set in `.env`).
4. From the dashboard you can open “Chat with AI” (admin only), see usage, change Settings, restart services. That’s the full product.

---

## What “working” looks like

- **Launcher:** Menu shows up; [2] Demo or [1] Full starts containers; [4] shows processes; [5] reports both healthy; [3] shuts down cleanly.
- **Wizard:** Login page loads; after sign-in you see Home (dashboard), Security (activity), Settings. No crash; no blank screen.
- **Gateway:** http://localhost:8080/health returns JSON with `"status":"ok"` when the stack is up.
- **Demo:** No real AI, but the perimeter (gateway + wizard) and UI are real. Good for “here’s the product” without API keys or OpenClaw.

If those hold, you can say: **“I have a working product: security gateway and control panel for a self‑hosted AI. Demo runs in one click; full stack runs with OpenClaw and API keys.”**

---

## If something fails

- **Docker not running:** Start Docker Desktop and try again.
- **Ports in use:** Close anything using 3000 or 8080, or run the launcher and choose “Start anyway” when it warns (only for local testing).
- **Password unknown:** See [SECURITY.md](SECURITY.md) § Recovery: lost wizard admin password (logs or set `WIZARD_ADMIN_PASSWORD` in `.env` and restart).
- **Launcher / logs:** See [docs/VERIFY-LAUNCHER.md](VERIFY-LAUNCHER.md) and [docs/START-CHECKLIST.md](START-CHECKLIST.md).

---

## One-line summary

**Where we are:** Working product — demo one-click; full stack with OpenClaw + API keys.  
**How to test:** LAUNCH.bat → [2] Demo → open http://localhost:3000 → sign in → use dashboard and Settings.
