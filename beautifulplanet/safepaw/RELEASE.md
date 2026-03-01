# Going public without regretting it

This is the **single checklist** to run before flipping the repo to public and cutting a first release. Use it as the “last place” to confirm you’re ready.

---

## 1. Clear public timestamp

- [ ] **Create a tag and GitHub Release** (e.g. `v0.1.0`) with release notes that describe core features (gateway, wizard, scanning, auth, runbooks, threat model).
- [ ] Ensure the **default branch** (e.g. `master`) has the commit you want as the public proof point.
- [ ] This gives an easy-to-cite, time-stamped artifact and starts accumulating evidence of authorship (stars, forks, mentions).

---

## 2. Raise the bar (“someone can copy it in a weekend”)

You can’t prevent copying; you can make a faithful clone harder:

- [ ] **Docs are excellent** — README, SECURITY.md, RUNBOOK.md, BACKUP-RECOVERY.md, THREAT-MODEL.md, CONTRIBUTING. ✅ Already in place.
- [ ] **Opinionated architecture and runbooks** — Documented request flow, playbooks, secret rotation. ✅ Already in place.
- [ ] **“Why we made these design decisions”** — The root [README Key design decisions](../../README.md#key-design-decisions) and [Security posture](../../README.md#security-posture) tables explain rationale (e.g. OpenClaw not exposed, heuristic-only scanning, server-generated request IDs). Consider expanding in a DESIGN.md or ADR if you want more narrative. This is hard to clone quickly and is valuable.

---

## 3. Public-readiness checklist (highest risk)

Before flipping visibility, confirm:

| Check | What to verify |
|-------|----------------|
| **No secrets in git history** | No API keys, tokens, real `.env`, or test creds in any commit. Use `git log -p` or a secret-scanning tool; consider `git filter-repo` if you ever committed secrets. |
| **GitHub Actions logs** | Workflows do not print secrets (no `echo $SECRET`, no log of masked values). Use `${{ secrets.* }}` and avoid logging env vars that might hold secrets. |
| **`.env.example` only placeholders** | All values are examples or `CHANGE_ME_*`; no real keys or passwords. ✅ Current `.env.example` uses placeholders. |
| **Wizard .env API** | GET `/api/v1/config` returns **masked** secret values; PUT accepts only the allowed key list. Wizard never logs secret values—only key names in audit. ✅ See SECURITY.md “Editable config keys” and “never logs secret values.” |
| **Docker images** | Images do not bake secrets into layers (no `ENV` with real keys, no COPY of `.env` with secrets). Use build args or runtime env only. ✅ Compose uses env_file: .env at runtime, not baked in. |

**Optional:** Paste your `.gitignore` and list of env vars for a quick leak-path review. Current `.gitignore` already excludes `.env`, `.env.*` (except `.env.example`), `*.key`, `secrets.json`, and other sensitive patterns.

---

## 4. Positioning

Current line: *“Secure, one-click deployer for OpenClaw.”*

If OpenClaw is niche, you may get more traction with:

- **“A security perimeter and deploy wizard for self-hosted AI assistants (OpenClaw first).”**

That frames it as an extensible pattern, not a one-off. The root README already mentions “self-hosted AI assistants” and “InstallerClaw” as the product name; you can tighten the tagline in the repo description when you go public.

---

## 5. Licensing

- **MIT** — Maximizes adoption and contributors; also maximizes the risk that someone clones and monetizes without sharing back.
- **AGPL** — “Network use must publish source” — more protective if you care about copyleft.
- **Dual-license later** — Possible if you want to offer a commercial exception.

No change required; just be aware that **MIT favors fast copying**. If your main worry is “someone did it first / someone will clone and monetize,” consider whether MIT is still the right choice before going public.

---

## 6. Recommendation

If the repo is as strong as the README suggests (tests, CI, runbooks, threat model, no secrets in history, checklist above satisfied):

1. **Go public** and **cut a release** (tag + GitHub Release `v0.1.0` with notes).
2. That gives you a **credible timestamp** and starts **accumulating evidence of authorship**.

Use this doc as the last place to confirm you’re ready before flipping visibility.
