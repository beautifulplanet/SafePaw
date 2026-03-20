# We don't collect your data

**Short version:** SafePaw does not collect, store, or send any of your data to us or any third party. There is no analytics, no tracking, no phone-home. Your config, your prompts, your usage, and your logs stay on your machine.

This isn't a policy promise — it's how the code is built. You can verify it yourself.

---

## How to verify (scan the repo)

**No telemetry or analytics:**  
Search for SDKs or endpoints that send data to external services:

```bash
# No Segment, Mixpanel, Amplitude, Sentry, etc.
grep -ri "segment\|mixpanel\|amplitude\|sentry\|analytics\|telemetry\|tracking\.\|phoning\|phone.home" --include="*.go" --include="*.ts" --include="*.tsx" services/
```

You'll find:
- **"telemetry"** only in `gateway/middleware/metrics.go` — that's *local* Prometheus-style counters (request counts, latency). They're exposed at `/metrics` for *your* Prometheus to scrape. Nothing is sent by SafePaw to any external server.
- **"tracking"** only in UI text (e.g. "tracking-tight" for font spacing) and brute-force *IP* tracking (per-IP rate limits and bans). No user or usage data is sent anywhere.
- **"collector"** = the *usage collector* that pulls cost data from *your* OpenClaw over WebSocket on *your* Docker network. It doesn't send that data to us; it stays in your gateway and wizard (and optional Postgres).

**No outbound HTTP from the app to our servers (or any analytics):**  
All `fetch()` calls in the wizard UI go to the same origin (`/api/v1/...`). The gateway and wizard only talk to:
- Each other (on your Docker network or localhost)
- Your OpenClaw (or whatever you set as `PROXY_TARGET`)
- Your Redis and Postgres (if you use them)
- Your LLM provider (Anthropic, OpenAI, etc.) — using *your* API key, from OpenClaw, not from SafePaw

**Optional font load (only outbound to the internet):**  
The wizard UI’s `index.html` can load fonts from `fonts.googleapis.com`. That’s a font CDN request from the browser; no data about you or your usage is sent. If you want zero outbound at all, you can self-host the fonts or remove the link.

---

## What we mean by "no data collection"

| We do **not** | We **do** (all local to you) |
|---------------|------------------------------|
| Send your config, prompts, or usage to our servers | Store config in your `.env` and optional Postgres |
| Use any analytics or tracking SDK | Use local metrics (Prometheus) and logs you control |
| Phone home or check in with a license server | Run entirely on your machine and your network |
| Sell or share any data (we never have it) | Let you export or back up your own data (see BACKUP-RECOVERY.md) |

---

## Why we're explicit

So you don’t have to take our word for it. You can grep the repo, read the code, and see there’s no data collection. No corporate privacy policy speak — just “we don’t collect it, and here’s how to check.”
