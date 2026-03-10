# SOC 2 & GDPR Compliance Mapping

> Mapping SafePaw's security controls to SOC 2 Type II Trust Service Criteria
> and GDPR articles. Honest about gaps — this is a posture assessment, not a
> certification claim.

**Status:** Self-assessed | **Last reviewed:** 2025-07-14

---

## How to Read This Document

- **✅ Implemented** — Control exists in code or configuration today.
- **📄 Documented** — Policy/procedure exists but is not enforced by code.
- **⚠️ Partial** — Some aspects covered, gaps noted.
- **❌ Gap** — Not yet addressed. Remediation noted.

---

## SOC 2 Trust Service Criteria

### CC1 — Control Environment

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC1.1 Commitment to integrity and ethics | CONTRIBUTING.md coding standards, security review checklist | [CONTRIBUTING.md](../CONTRIBUTING.md) | 📄 Documented |
| CC1.2 Board/management oversight | Single-maintainer project; all PRs require review | CI branch protection rules | ⚠️ Partial |
| CC1.3 Organizational structure & authority | RBAC: admin/operator/viewer roles | `services/wizard/internal/middleware/auth.go` | ✅ Implemented |
| CC1.4 Competency commitment | ADRs documenting rationale for every architectural choice | [docs/adr/](adr/) | 📄 Documented |
| CC1.5 Accountability for controls | Audit trail logs all admin mutations with actor, IP, timestamp | `services/wizard/internal/audit/emitter.go` | ✅ Implemented |

### CC2 — Communication and Information

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC2.1 Internal communication of objectives | README, SECURITY.md, THREAT-MODEL.md, this document | Project documentation | 📄 Documented |
| CC2.2 Internal communication of responsibilities | CONTRIBUTING.md, RUNBOOK.md with role assignments | [RUNBOOK.md](../RUNBOOK.md) | 📄 Documented |
| CC2.3 External communication | Responsible disclosure in PENTEST-POLICY.md | [docs/PENTEST-POLICY.md](PENTEST-POLICY.md) | 📄 Documented |

### CC3 — Risk Assessment

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC3.1 Risk identification | STRIDE threat model (48 threats identified) | [THREAT-MODEL.md](../THREAT-MODEL.md) | ✅ Implemented |
| CC3.2 Risk analysis and severity | Residual risk table with severity ratings | THREAT-MODEL.md §4 | ✅ Implemented |
| CC3.3 Fraud risk consideration | Prompt injection detection, output scanning | `services/gateway/middleware/sanitize.go`, `output_scanner.go` | ✅ Implemented |
| CC3.4 Change-driven risk assessment | Quarterly threat model review schedule | THREAT-MODEL.md §5 | 📄 Documented |

### CC4 — Monitoring Activities

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC4.1 Ongoing monitoring | Prometheus metrics: auth, rate limit, scanner, proxy | `services/gateway/middleware/metrics.go` | ✅ Implemented |
| CC4.2 Evaluation of deficiencies | CI: go test, race detector, govulncheck, Trivy | `.github/workflows/ci.yml` | ✅ Implemented |
| CC4.3 Communication of deficiencies | Automated Dependabot PRs, govulncheck in CI | `.github/dependabot.yml` | ✅ Implemented |

### CC5 — Control Activities

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC5.1 Selection and development of controls | Defense-in-depth middleware chain (7 layers) | SECURITY.md §3 | ✅ Implemented |
| CC5.2 Technology controls | HMAC-SHA256 auth, rate limiting, IP banning, body size limits | Gateway middleware stack | ✅ Implemented |
| CC5.3 Policy deployment | Conventional commits, branch protection, CI gate | `.github/workflows/ci.yml`, CONTRIBUTING.md | ✅ Implemented |

### CC6 — Logical and Physical Access Controls

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC6.1 Logical access security | HMAC tokens with scope + expiry; RBAC for wizard | `middleware/auth.go`, `wizard/internal/middleware/auth.go` | ✅ Implemented |
| CC6.2 User provisioning | Admin-only token generation; role-based env vars | `tools/tokengen/`, `.env` configuration | ✅ Implemented |
| CC6.3 User deprovisioning | Token revocation API (subject-level ban) | `middleware/revocation.go` | ✅ Implemented |
| CC6.4 Access review | In-memory revocation list; configurable TTL | Gateway startup log, `AUTH_MAX_TTL` | ⚠️ Partial — no periodic access review process |
| CC6.5 Authentication mechanisms | HMAC-SHA256 tokens + optional TOTP (wizard) | `session.go`, TOTP configuration | ✅ Implemented |
| CC6.6 Encryption in transit | TLS termination supported; `SECURE_COOKIE` for wizard | Config: `TLS_CERT`, `TLS_KEY` | ⚠️ Partial — TLS optional, not enforced |
| CC6.7 Encryption at rest | Postgres data in Docker volume (not encrypted by default) | docker-compose.yml | ❌ Gap — see O3 remediation |
| CC6.8 Data destruction | No automated data purge procedure | N/A | ❌ Gap |

### CC7 — System Operations

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC7.1 Detection of unauthorized changes | govulncheck, Trivy scanning in CI | `.github/workflows/ci.yml` | ✅ Implemented |
| CC7.2 Monitoring for anomalies | Brute-force detection with escalating bans | `middleware/bruteforce.go` | ✅ Implemented |
| CC7.3 Incident response procedures | 6 runbook playbooks (detect → recover → postmortem) | [RUNBOOK.md](../RUNBOOK.md) | ✅ Implemented |
| CC7.4 Recovery from incidents | Backup/restore procedures for all persistent data | [BACKUP-RECOVERY.md](../BACKUP-RECOVERY.md) | ✅ Implemented |
| CC7.5 Restoration testing | Backup cron installer with retention | `scripts/backup-cron-install.sh` | ⚠️ Partial — automated restore test not yet implemented |

### CC8 — Change Management

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC8.1 Change authorization | Branch protection, CI-required checks | GitHub repo settings, `ci-pass` job | ✅ Implemented |
| CC8.2 Change testing | 319 tests, race detector, fuzz seeds, 65% coverage gate | CI pipeline | ✅ Implemented |
| CC8.3 Change deployment | CHANGELOG.md, conventional commits, semantic versioning | [CHANGELOG.md](../CHANGELOG.md) | ✅ Implemented |

### CC9 — Risk Mitigation

| Criterion | SafePaw Control | Evidence | Status |
|-----------|----------------|----------|--------|
| CC9.1 Risk mitigation for third parties | Docker socket proxy restricts API surface | ADR-004, docker-compose.yml | ✅ Implemented |
| CC9.2 Vendor risk assessment | Dependabot for dependency freshness; minimal deps | ADR-002 (2 Go deps total) | ✅ Implemented |

---

## GDPR Mapping

> SafePaw processes AI prompts that may contain personal data. As a gateway,
> it does not store user content — but the controls below apply to the
> operational data it does handle.

### Chapter II — Principles (Articles 5–11)

| Article | Requirement | SafePaw Control | Status |
|---------|------------|----------------|--------|
| Art. 5(1)(a) | Lawfulness, fairness, transparency | Documented data flows in THREAT-MODEL.md | 📄 Documented |
| Art. 5(1)(b) | Purpose limitation | Gateway proxies only; no secondary use of prompts | ✅ By design |
| Art. 5(1)(c) | Data minimisation | No prompt/response bodies stored; only metadata logged | ✅ Implemented |
| Art. 5(1)(d) | Accuracy | N/A — SafePaw does not maintain user profile data | N/A |
| Art. 5(1)(e) | Storage limitation | In-memory state with TTL expiry; no persistent PII store | ✅ Implemented |
| Art. 5(1)(f) | Integrity and confidentiality | HMAC auth, TLS support, network isolation, output scanning | ✅ Implemented |
| Art. 5(2) | Accountability | This document + audit trail + THREAT-MODEL.md | 📄 Documented |
| Art. 6 | Lawful basis for processing | Legitimate interest (security gateway); consent delegated to AI provider | ⚠️ Partial — needs Data Processing Agreement template |
| Art. 7 | Conditions for consent | Not directly applicable (gateway, not end-user SaaS) | N/A |

### Chapter III — Rights of the Data Subject (Articles 12–23)

| Article | Requirement | SafePaw Control | Status |
|---------|------------|----------------|--------|
| Art. 12–14 | Transparent information | SECURITY.md documents what is logged and why | 📄 Documented |
| Art. 15 | Right of access | Admin can query audit logs for specific subjects | ⚠️ Partial — no self-service portal |
| Art. 17 | Right to erasure | No persistent PII stored; in-memory TTLs auto-expire | ✅ By design |
| Art. 20 | Data portability | Audit logs exportable as JSON (LOG_FORMAT=json) | ✅ Implemented |
| Art. 25 | Data protection by design | Zero stored prompts, minimal deps, sanitization | ✅ Implemented |

### Chapter IV — Controller and Processor (Articles 24–43)

| Article | Requirement | SafePaw Control | Status |
|---------|------------|----------------|--------|
| Art. 25 | Data protection by design and default | Network isolation, minimal logging, HttpOnly cookies | ✅ Implemented |
| Art. 28 | Processor obligations | SafePaw is middleware — DPA template needed for deployment | ❌ Gap — DPA template not provided |
| Art. 30 | Records of processing | Audit trail (all mutations); structured logs for SIEM | ✅ Implemented |
| Art. 32 | Security of processing | Full middleware chain, encryption in transit, RBAC | ✅ Implemented |
| Art. 33 | Breach notification | Incident response runbooks with timelines | 📄 Documented — no 72-hour automation |
| Art. 35 | DPIA (Data Protection Impact Assessment) | THREAT-MODEL.md serves as risk assessment | ⚠️ Partial — formal DPIA template not created |

---

## Identified Gaps and Remediation Plan

| ID | Gap | SOC 2 / GDPR | Severity | Remediation | Timeline |
|----|-----|-------------|----------|-------------|----------|
| G1 | No encryption at rest for Postgres volumes | CC6.7 | Medium | Phase 3 item O3: GPG-encrypted backups + LUKS volume docs | This phase |
| G2 | No automated data purge procedure | CC6.8 | Low | Document data lifecycle; add Redis key expiry audit | Next phase |
| G3 | TLS not enforced by default | CC6.6 | Medium | Document TLS setup in deployment guide; consider `REQUIRE_TLS` flag | Next phase |
| G4 | No Data Processing Agreement template | Art. 28 | Medium | Create template DPA for deployers | Next phase |
| G5 | No formal DPIA template | Art. 35 | Low | Extend THREAT-MODEL.md with DPIA section | Next phase |
| G6 | No periodic access review process | CC6.4 | Low | Document quarterly review checklist | Next phase |
| G7 | Automated restore testing not implemented | CC7.5 | Medium | Phase 3 item O3: restore verification script | This phase |
| G8 | Breach notification not automated | Art. 33 | Low | Runbook covers manual process; consider webhook alert | Future |

---

## Auditor Notes

### What We Do Well
- **Defense in depth**: 7-layer middleware chain with independent failure modes
- **Minimal attack surface**: 2 Go dependencies in gateway, zero in wizard middleware
- **Threat modeling**: 27 STRIDE threats identified and mitigated before deployment
- **Audit trail**: Every admin mutation logged with actor, IP, action, timestamp
- **Automated security testing**: Race detector, fuzz, govulncheck, Trivy in CI

### What We Don't Claim
- SafePaw is **not SOC 2 certified** — this is a control mapping, not a report
- SafePaw is **not GDPR certified** — this maps relevant controls for deployers
- External audit has **not been performed** — recommended in PENTEST-POLICY.md
- Full body logging is **deliberately omitted** to avoid storing PII (THREAT-MODEL.md G8)

### Why Gaps Are Acceptable (For Now)
SafePaw is designed for **localhost/VPN deployment** behind an existing enterprise perimeter. The documented gaps (TLS enforcement, DPA template, formal DPIA) become relevant when SafePaw is deployed as a public-facing service. The remediation plan above provides a path for that transition.

---

## References

- [SECURITY.md](../SECURITY.md) — Security architecture
- [THREAT-MODEL.md](../THREAT-MODEL.md) — STRIDE threat analysis
- [RUNBOOK.md](../RUNBOOK.md) — Incident response playbooks
- [BACKUP-RECOVERY.md](../BACKUP-RECOVERY.md) — Backup and restore
- [PENTEST-POLICY.md](PENTEST-POLICY.md) — Penetration testing policy
- [CONTRIBUTING.md](../CONTRIBUTING.md) — Development standards
- [docs/adr/](adr/) — Architecture decision records
- [CHANGELOG.md](../CHANGELOG.md) — Change history
