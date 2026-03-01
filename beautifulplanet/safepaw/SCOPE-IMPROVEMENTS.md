# SafePaw — Improvement scope (review feedback)

This document maps **brutally honest** review feedback to reality: what’s already implemented vs. what’s valid remaining work. It then lists all valid improvements as a long, non-rushed scope list for future work.

---

## Part 1: Review feedback vs. current state

Many items in the review are **already addressed**. This section records that so we don’t double-count.

| # | Review claim | Current state | Verdict |
|---|------------------------------|-----------------------------------------------|--------|
| 1 | No token revocation | Redis-backed revocation, `POST /admin/revoke`, in-memory cache | **Done** |
| 2 | No brute-force detection/blocking | BruteForceGuard: IP ban after N auth/rate-limit failures, escalating duration | **Done** |
| 3 | No MFA for admin | Optional TOTP (`WIZARD_TOTP_SECRET`), 6-digit code at login | **Done** |
| 4 | No centralized/structured logging (JSON/SIEM) | Gateway: `LOG_FORMAT=json`, structured logger; Wizard: audit log | **Done** |
| 5 | No Prometheus/Grafana metrics | `/metrics`, Prometheus scrape, Grafana dashboard (8 panels), 6 alert rules | **Done** |
| 6 | No resource limits in Docker Compose | All 5 services have `deploy.resources.limits` (memory + cpus) | **Done** |
| 7 | No audit logs for admin actions | Wizard audit: login success/failure, config change, service restart | **Done** |
| 8 | No fuzz testing | 7 fuzz targets (prompt injection, sanitize, channel, output scan, token, KV) | **Done** |
| 9 | No integration tests for security middleware | integration_test.go: auth→ban, rate limit→ban, scope enforcement, full chain E2E | **Done** |
| 10 | No static analysis / dependency scanning in CI | golangci-lint, gosec, govulncheck in GitHub Actions | **Done** |
| 11 | No automated backup/restore | BACKUP-RECOVERY.md with procedures; no automation (cron/scripts) | **Procedures done; automation open** |
| 12 | No runbooks | RUNBOOK.md: 6 playbooks (token, injection, gateway down, brute force, rotation, disk) | **Done** |
| 13 | No secret rotation | RUNBOOK INC-5: ordered rotation, one-shot block, link to backup | **Procedures done; automation open** |
| 14 | No threat model | THREAT-MODEL.md: STRIDE, 27 threats, mitigations, residual risks | **Done** |

So: **the first 10 “critical” issues are largely already covered.** The review is useful for the remaining gaps and for the longer list (21–100 and OPSEC). Below is the **valid** remaining scope only.

---

## Part 2: Valid improvement scope (prioritized)

Items below are **not** yet fully in place. They are grouped by theme and ordered by impact vs. effort where possible. Treat this as a backlog, not a sprint.

---

### Security

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| S1 | **RBAC in wizard** — Multiple roles (e.g. viewer vs admin) or permission flags so one compromised admin isn’t full control | High | Single admin today; add roles or scoped actions |
| S2 | **Distributed / proxy abuse** — Rate limiting is per-IP; consider fingerprinting, proof-of-work, or abuse signals beyond IP | Medium | Per-IP is first line; document limitation, consider optional extras |
| S3 | **IP allow/block lists** — Configurable allow/deny for gateway (e.g. block known bad actors, allow only office IPs) | Medium | Env or config file list |
| S4 | **Session invalidation on password change** — When admin password or TOTP secret changes, invalidate existing wizard sessions | Medium | Require re-login after rotation |
| S5 | **Vault / secrets manager integration** — Optional integration with HashiCorp Vault or cloud secrets (e.g. AWS Secrets Manager) for production | Medium | Keep .env for dev; doc + optional adapter for prod |
| S6 | **Automated secret rotation** — Scripts or jobs to rotate AUTH_SECRET, Redis, Postgres, wizard password on a schedule | Low | Runbook exists; automate with cron or external scheduler |
| S7 | **Encrypted secrets at rest** — Option to encrypt sensitive values in volumes or backup files | Low | Backup encryption doc; optional tooling |
| S8 | **External auth (OAuth/SSO)** — Optional OAuth or SAML for wizard admin instead of password-only | Low | Larger feature; document as future option |

---

### Monitoring & observability

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| M1 | **Notification channels for alerts** — Document and/or template for wiring Grafana alerts to PagerDuty, Slack, email | High | **Done:** [monitoring/NOTIFICATIONS.md](monitoring/NOTIFICATIONS.md) + contact-points.example.yml |
| M2 | **Incident response timeline** — Document target response times (e.g. P0 in 15 min) and escalation path | High | **Done:** RUNBOOK.md — Incident response timeline + Escalation matrix |
| M3 | **Distributed tracing** — OpenTelemetry or similar for request correlation across gateway → OpenClaw | Medium | Optional; useful for debugging |
| M4 | **Export health/status in standard formats** — e.g. OpenMetrics for health endpoint or dedicated status endpoint | Low | Already have /metrics; extend if needed |
| M5 | **Container resource usage in dashboard** — Show CPU/memory per container in wizard UI (read-only) | Low | Docker stats API or cAdvisor |

---

### Testing & quality

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| T1 | **Regression suite for prompt-injection patterns** — Automated tests that run when patterns change; golden set of attack strings | High | Ensure pattern changes don’t regress |
| T2 | **Chaos / failure-mode tests** — Tests that kill backend, throttle, or corrupt responses to verify gateway/wizard behavior | Medium | Optional chaos runs or integration tests |
| T3 | **UI test automation** — Cypress/Playwright for wizard flows (login, config, dashboard) | Medium | E2E script exists; add browser E2E |
| T4 | **Coverage target increase** — Raise from 60% toward 90% for critical packages | Low | Per FAANG-style feedback |
| T5 | **SonarQube (or equivalent)** — Optional additional static analysis in CI | Low | gosec + govulncheck already; add if desired |

---

### Infrastructure & supply chain

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| I1 | **Automated backup jobs** — Cron or Compose job that runs pg_dump, Redis backup, volume snapshot per BACKUP-RECOVERY.md | High | Document + example script or job |
| I2 | **Container image scanning** — Scan built images for CVEs (e.g. Trivy, Snyk) in CI or before deploy | High | Add to CI or release process |
| I3 | **Image provenance / attestation** — Sign images or attest build provenance (e.g. cosign, in-toto) | Medium | Supply chain hardening |
| I4 | **Runtime security (seccomp/AppArmor)** — Optional hardening profiles for containers | Low | Document or add profiles |
| I5 | **Resource usage reporting** — Export or display per-container resource usage (see M5) | Low | |

---

### Documentation & process

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| D1 | **Vulnerability management policy** — Document how often we run govulncheck, who acts on findings, SLAs | High | **Done:** SECURITY.md §11 Vulnerability management |
| D2 | **Penetration testing** — Document that pentests are run (e.g. annually) and how findings are tracked | High | Policy + placeholder for results |
| D3 | **Compliance playbooks** — SOC2, GDPR, or HIPAA-oriented sections: what we do, what’s in scope, what’s not | Medium | “Compliance considerations” doc |
| D4 | **Onboarding automation** — Script or checklist that validates env, ports, Docker, and first successful request | Low | Extend verify-deployment.sh or add onboarding script |
| D5 | **Self-healing for misconfigurations** — Detect common misconfigs and suggest fixes (or auto-fix where safe) | Low | Wizard could run checks and suggest .env fixes |

---

### User experience (wizard)

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| U1 | **Multiple admin accounts** — Separate credentials and optional roles per admin | Medium | Ties to S1 (RBAC) |
| U2 | **Password rotation on a schedule** — Optional prompt or policy to rotate wizard admin password periodically | Low | Doc + optional enforcement |
| U3 | **Dark/light mode toggle** — Theme preference in wizard UI | Low | |
| U4 | **Mobile-friendly layout** — Responsive wizard for small screens | Low | |
| U5 | **Accessibility (a11y)** — ARIA, keyboard nav, contrast, screen reader support in wizard | Low | |
| U6 | **Internationalization (i18n)** — Localized strings for wizard | Low | |
| U7 | **Custom branding/theming** — Optional logo or theme for wizard | Low | |
| U8 | **User-facing error pages** — Friendly error pages for gateway (e.g. 502, 429) with guidance | Low | |

---

### Gateway & API (optional / future)

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| G1 | **Rate limiting per endpoint** — Different limits for /admin vs. proxy vs. health | Medium | |
| G2 | **API documentation (OpenAPI/Swagger)** — Spec and optional UI for gateway and wizard APIs | Low | |
| G3 | **TLS cipher suite configuration** — Allow operator to restrict ciphers via config | Low | |
| G4 | **Logging to external services** — Optional export to CloudWatch, ELK, etc. | Low | |
| G5 | **DDoS / geo-blocking** — Optional layer (e.g. Cloudflare) or config for geo allow/block | Low | Document; implement only if needed |
| G6–G20 | **Further gateway options** — Caching, compression, custom middleware, per-endpoint quotas, etc. | Low | Backlog; add only when there is a concrete use case |

---

### Advanced OPSEC & operations

| ID | Item | Priority | Notes |
|----|------|----------|--------|
| O1 | **Secrets vault in production** — Document use of Vault/AWS Secrets Manager; optional adapter to pull secrets at startup | High | See S5 |
| O2 | **Ephemeral / short-lived credentials** — Document and encourage short TTLs; optional automatic token rotation | Medium | |
| O3 | **Secure backups** — Encrypt backups and document key management; test restore regularly | Medium | See BACKUP-RECOVERY.md |
| O4 | **Log sanitization** — Ensure no tokens, passwords, or PII in logs; audit and document | Medium | |
| O5 | **Automated patching** — Document process for base image and dependency updates; Dependabot or similar | Medium | |
| O6 | **Incident drills** — Tabletop exercises or red-team runs; document schedule and outcomes | Low | |
| O7 | **Forensics readiness** — Retention policy for logs, metrics, traces; document for post-incident | Low | |
| O8 | **Secure build pipeline** — Signed commits, reproducible builds, CI hardening (already partially in place) | Low | Document and tighten |
| O9 | **External attack surface monitoring** — Optional scanning for exposed endpoints, open ports, misconfigs | Low | |
| O10 | **Data retention policies** — Define how long logs, backups, and audit data are kept | Low | |

---

## Part 3: Suggested implementation order (non-rushed)

**Phase 1 — Quick wins (docs + small code)** ✅ *Completed*  
- M2 Incident response timeline → RUNBOOK.md  
- D1 Vulnerability management policy → SECURITY.md §11  
- M1 Notification channels → monitoring/NOTIFICATIONS.md + contact-points.example.yml  
- T1 Regression suite for prompt-injection patterns → TestPromptInjection_RegressionSuite in gateway/middleware/sanitize_test.go  

**Phase 2 — Security & ops**  
- S1 RBAC or scoped actions in wizard  
- S4 Session invalidation on password/rotation  
- I1 Automated backup (script + doc)  
- I2 Container image scanning in CI  
- O4 Log sanitization audit and doc  

**Phase 3 — Hardening & compliance**  
- D2 Pentest policy  
- D3 Compliance considerations (SOC2/GDPR)  
- S5 Vault/secrets manager doc and optional adapter  
- O3 Secure backups (encryption + restore tests)  
- O5 Automated patching policy  

**Phase 4 — Nice-to-have**  
- S2/S3 Abuse detection or IP lists  
- M3 Distributed tracing  
- T2 Chaos/failure tests  
- T3 UI E2E (Playwright/Cypress)  
- U1 Multiple admins / U2 Password rotation policy  
- Remaining U*, G*, O* items as needed  

---

## Summary

- **Review feedback is partially outdated:** token revocation, brute-force protection, MFA, structured logging, Prometheus/Grafana, resource limits, audit logs, fuzz tests, integration tests, static/dependency scanning, runbooks, backup/restore procedures, secret rotation procedures, and threat model are already in place.
- **Valid remaining work** is captured above: RBAC, abuse detection, notifications, incident timeline, backup automation, image scanning, regression tests for patterns, compliance docs, vault integration, and the rest of the long list. Use this doc as the single scope list and prioritize by your timeline and risk appetite.
