# SafePaw — Demo Video Script (LinkedIn)

Use this script to record a short, professional demo showing **SafePaw** running in real time: one-command deploy, Wizard UI, live service dashboard, and the security perimeter in action.

---

## Before You Record

### 1. Pre-flight (do this once)

- **Docker Desktop** running (or Docker Engine + Compose v2).
- **Ports free:** 3000 (Wizard), 8080 (Gateway).  
  Check: `netstat -an | findstr "3000 8080"` (Windows) or `lsof -i :3000 -i :8080` (macOS/Linux).
- **Demo env (recommended):** Use the demo-prep script so you have a known login and no real secrets on screen:

  ```bash
  cd SafePaw
  ./scripts/demo-prep.sh
  # Then: cp .env.demo .env   (only for recording; use your real .env otherwise)
  ```
  **Windows:** Run `demo-prep.sh` in Git Bash or WSL, or manually: copy `.env.example` to `.env`, set `WIZARD_ADMIN_PASSWORD=DemoPassword123!` and `AUTH_ENABLED=false` so you can log in and hit the gateway without tokens.

  Or manually: set `WIZARD_ADMIN_PASSWORD=YourDemoPassword` in `.env` so you can log in without reading logs. For a simpler gateway segment, you can set `AUTH_ENABLED=false` in `.env` **only for the demo** (so you can hit the gateway without generating a token).

### 2. Start the stack (before hitting Record)

```bash
cd SafePaw
docker compose up -d
```

Wait ~30–60 seconds for all services (especially OpenClaw) to become healthy. Optional check:

```bash
docker compose ps
# All should show "healthy" or "running"
```

Leave this terminal available to show `docker compose ps` or `docker compose logs -f` briefly if you want.

---

## Scene-by-Scene Script (~2–3 minutes)

### Scene 1 — Hook (5–10 s)

**Screen:** Your face or a simple title card.

**Say (example):**  
*“Self-hosted AI assistants are powerful—but you don’t want them exposed without a security layer. SafePaw adds a one-command security perimeter around OpenClaw: auth, rate limiting, and prompt-injection scanning. Here’s it running in real time.”*

---

### Scene 2 — One command (15–20 s)

**Screen:** Terminal in `safepaw` directory.

**Do:**

1. Show the directory: `cd SafePaw` (or already there).
2. Run: `docker compose up -d`
3. Optional: `docker compose ps` to show 5 services (wizard, gateway, openclaw, redis, postgres).

**Say (example):**  
*“From the SafePaw repo, one command brings up the full stack: setup wizard, security gateway, OpenClaw, Redis, and Postgres. Only the wizard and gateway are exposed on localhost; OpenClaw stays internal.”*

---

### Scene 3 — Wizard login (10–15 s)

**Screen:** Browser.

**Do:**

1. Open **http://localhost:3000**
2. Show the SafePaw welcome / login screen.
3. Enter the demo admin password (from `WIZARD_ADMIN_PASSWORD` in `.env` or from the demo-prep script).
4. Click Login.

**Say (example):**  
*“The setup wizard is the single place to configure and monitor the deployment. I log in with the admin password.”*

---

### Scene 4 — Prerequisites (5–10 s)

**Screen:** Wizard — Prerequisites page.

**Do:**

1. After login you land on **System Prerequisites** (Docker, compose file, etc.).
2. Show that checks are green (all pass).
3. Click **“Continue to Dashboard →”**.

**Say (example):**  
*“Prerequisites are validated—Docker, compose file, and env are in place. Then we go to the dashboard.”*

---

### Scene 5 — Live dashboard (25–35 s)

**Screen:** Wizard — Service Dashboard.

**Do:**

1. Show the **Service Dashboard** with the five services and their status (Running / Healthy).
2. Point out: Wizard, Gateway, OpenClaw, Redis, Postgres.
3. Optionally click **Refresh** to show it’s live.
4. Optionally open **Configuration** and briefly show masked secrets (no real keys on screen if using .env.demo).

**Say (example):**  
*“The dashboard shows live status for all five services. Everything is running and healthy. Configuration is managed here with secrets masked. The important part: OpenClaw has no host port—it’s only reachable through the gateway.”*

---

### Scene 6 — Gateway in action (15–25 s)

**Screen:** Browser or terminal.

**Do (pick one or both):**

- **Browser:** Open **http://localhost:8080/health**  
  Show the JSON health response (e.g. `"status":"ok"` and optionally `"backend":"reachable"`).
- **Terminal:**  
  `curl -s http://localhost:8080/health`  
  Then: `curl -s http://localhost:8080/metrics | head -20`  
  to show Prometheus metrics.

**Say (example):**  
*“All traffic to the AI assistant goes through the gateway. Here’s the health endpoint and, if we want, metrics—rate limiting, auth, and scanning are in front of OpenClaw.”*

---

### Scene 7 — Security proof (optional, 10–15 s)

**Screen:** Terminal.

**Do:**

1. Show that OpenClaw is **not** bound on the host:  
   `curl -s --connect-timeout 2 http://localhost:18789/health`  
   (should fail or timeout—port 18789 is not exposed).
2. Contrast: `curl -s http://localhost:8080/health` succeeds.

**Say (example):**  
*“If we try to hit OpenClaw directly on its port, we can’t—it’s not exposed. The only way in is through the gateway. That’s the perimeter SafePaw adds.”*

---

### Scene 8 — Outro & CTA (5–10 s)

**Screen:** Your face or a closing slide.

**Say (example):**  
*“SafePaw: a security perimeter and one-command deploy for OpenClaw. If you run self-hosted AI, check the repo link in the description. Thanks for watching.”*

---

## LinkedIn-Specific Tips

| Tip | Why |
|-----|-----|
| **Keep it 1–2 min** | LinkedIn favors short, native video; 2–3 min is fine if every scene earns its place. |
| **Captions** | Many watch without sound; add captions or a short text summary in the post. |
| **First line of post** | Put the main benefit in the first line (e.g. “One-command security perimeter for self-hosted AI”). |
| **Hashtags** | e.g. `#SelfHosted #AI #DevSecOps #OpenSource` — use 3–5 that fit your audience. |
| **Link in comment** | Put the repo link in the first comment so the post stays clean and the algorithm is less likely to throttle. |
| **No real secrets** | Use `.env.demo` / demo-prep and never show real API keys or passwords. |

---

## Quick Reference — URLs & Commands

| What | URL or command |
|------|-----------------|
| Wizard UI | http://localhost:3000 |
| Gateway health | http://localhost:8080/health |
| Gateway metrics | http://localhost:8080/metrics |
| Start stack | `docker compose up -d` |
| Service status | `docker compose ps` |
| OpenClaw not exposed | `curl -s --connect-timeout 2 http://localhost:18789/health` (expect fail) |

---

## If Something Fails During the Demo

- **Wizard won’t load:** Confirm wizard container is running: `docker compose ps`, then `docker compose logs wizard --tail=30`.
- **Login fails:** If you didn’t set `WIZARD_ADMIN_PASSWORD`, get the one-time password: `docker compose logs wizard | findstr "Auto-generated"` (Windows) or `docker compose logs wizard 2>&1 | grep "Auto-generated"` (macOS/Linux).
- **Gateway 502 / unhealthy:** OpenClaw may still be starting. Wait 30–60 s and check: `docker compose logs openclaw --tail=20`.
- **Port in use:** Stop other apps using 3000 or 8080, or change ports in `docker-compose.yml` for the demo (and mention it on screen).

Use this script as a checklist and adjust the wording to your style. Good luck with the LinkedIn demo.
