# SafePaw — Threat Model (STRIDE)

> **Version**: 1.1  
> **Last reviewed**: 2026-03-08  
> **Methodology**: [Microsoft STRIDE](https://learn.microsoft.com/en-us/azure/security/develop/threat-modeling-tool-threats)  
> **Scope**: SafePaw Gateway + Wizard + OpenClaw orchestration

---

## 1. System Overview

```
┌─────────────────────────────────────────────────────┐
│  Host Machine                                       │
│                                                     │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐   │
│  │ Browser  │──▶│ Wizard   │   │  Gateway     │   │
│  │          │   │ :3000    │   │  :8080       │   │
│  └──────────┘   └──────────┘   └──────┬───────┘   │
│                                       │            │
│  ┌────────────────────────────────────┼──────────┐ │
│  │ Docker Network (safepaw-internal)  │          │ │
│  │                                    ▼          │ │
│  │  ┌──────────┐  ┌───────┐  ┌──────────────┐  │ │
│  │  │ Postgres │  │ Redis │  │  OpenClaw     │  │ │
│  │  │ (state)  │  │(rate) │  │  (AI engine)  │  │ │
│  │  └──────────┘  └───────┘  └──────────────┘  │ │
│  └──────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### Trust Boundaries

| Boundary | Between | Controls |
|----------|---------|----------|
| **TB1** | Internet → Gateway | TLS, Auth, Rate Limit, IP Ban |
| **TB2** | Browser → Wizard | HTTPS, Session Cookie, CSRF |
| **TB3** | Gateway → OpenClaw | Docker network isolation |
| **TB4** | Wizard → Docker Socket | Read-only mount, allowlisted ops |
| **TB5** | Services → Databases | Network isolation, auth |

---

## 2. STRIDE Analysis

### S — Spoofing

| ID | Threat | Component | Mitigation | Status |
|----|--------|-----------|------------|--------|
| S1 | Attacker impersonates legitimate API user | Gateway | HMAC-SHA256 tokens with expiry + scope | ✅ Implemented |
| S2 | Attacker spoofs client IP via headers | Gateway | Only trusts X-Real-IP from loopback | ✅ Implemented |
| S3 | Attacker forges session cookie | Wizard | HMAC-signed session tokens, HttpOnly, SameSite=Strict | ✅ Implemented |
| S4 | Attacker injects X-Auth-Subject header | Gateway | StripAuthHeaders middleware when auth disabled | ✅ Implemented |
| S5 | Token replay after compromise | Gateway | Token revocation API + subject-level ban | ✅ Implemented |

### T — Tampering

| ID | Threat | Component | Mitigation | Status |
|----|--------|-----------|------------|--------|
| T1 | Modified request body | Gateway | Body size limit (64KB), content-type validation | ✅ Implemented |
| T2 | Prompt injection in user messages | Gateway | Heuristic scanner + risk tagging (X-SafePaw-Risk) | ✅ Implemented |
| T3 | XSS injection in AI responses | Gateway | Output scanner strips `<script>`, event handlers | ✅ Implemented |
| T4 | Config file tampering | Wizard | Allowlisted config keys only, mutex-protected writes | ✅ Implemented |
| T5 | Supply chain compromise (npm) | Docker | Image pinned by SHA256 digest | ✅ Implemented |
| T6 | Docker socket abuse | Wizard | Read-only mount, container ops only | ✅ Implemented |

### R — Repudiation

| ID | Threat | Component | Mitigation | Status |
|----|--------|-----------|------------|--------|
| R1 | Admin denies performing action | Wizard | Structured audit log (JSON) for all mutations | ✅ Implemented |
| R2 | Attacker's actions untracked | Gateway | Request ID tracing, structured JSON logs | ✅ Implemented |
| R3 | No evidence of token revocation | Gateway | Audit events on revocation with actor/reason | ✅ Implemented |
| R4 | Centralized log aggregation | All | LOG_FORMAT=json for SIEM integration | ✅ Implemented |

### I — Information Disclosure

| ID | Threat | Component | Mitigation | Status |
|----|--------|-----------|------------|--------|
| I1 | API keys leaked in AI responses | Gateway | Output scanner detects `sk-`, `ghp_`, API key patterns | ✅ Implemented |
| I2 | System prompt leaked | Gateway | Output scanner detects "system prompt" patterns | ✅ Implemented |
| I3 | Server technology disclosed | Gateway | Server header removed, generic error messages | ✅ Implemented |
| I4 | Secrets exposed in config API | Wizard | Sensitive values masked in GET /config | ✅ Implemented |
| I5 | Internal network topology exposed | Docker | OpenClaw/Redis/Postgres have no host-exposed ports | ✅ Implemented |
| I6 | Error messages reveal internals | Both | Structured errors with codes, not stack traces | ✅ Implemented |

### D — Denial of Service

| ID | Threat | Component | Mitigation | Status |
|----|--------|-----------|------------|--------|
| D1 | Connection flooding | Gateway | Per-IP rate limiting (configurable window) | ✅ Implemented |
| D2 | Repeated abuse after rate limit | Gateway | Brute-force IP banning (escalating duration) | ✅ Implemented |
| D3 | Large request body | Gateway | MaxBodySize enforced (64KB default) | ✅ Implemented |
| D4 | Large response body | Gateway | Output scanner with size limit | ✅ Implemented |
| D5 | Header size attack | Gateway | MaxHeaderBytes = 64KB | ✅ Implemented |
| D6 | Slow loris / connection exhaustion | Gateway | Read/Write/Idle timeouts configured | ✅ Implemented |
| D7 | Memory exhaustion via leaked entries | Gateway | Background cleanup for rate limiter, revocations, bans | ✅ Implemented |
| D8 | Container resource exhaustion | Docker | CPU/memory limits per service | ✅ Implemented |

### E — Elevation of Privilege

| ID | Threat | Component | Mitigation | Status |
|----|--------|-----------|------------|--------|
| E1 | User token used for admin operations | Gateway | Scope-based auth (proxy vs admin) | ✅ Implemented |
| E2 | Container escape to host | Docker | Non-root containers, resource limits | ✅ Implemented |
| E3 | Lateral movement between services | Docker | Network isolation, no host port exposure for internal services | ✅ Implemented |
| E4 | Path traversal in channel names | Gateway | ValidateChannel rejects `..`, `/`, `\` | ✅ Implemented |
| E5 | Docker socket privilege escalation | Wizard | Read-only mount, allowlisted operations | ✅ Implemented |

---

## 3. Data Flow Threats

### HTTP Request Flow (Gateway)

```
Client → [TLS] → SecurityHeaders → RequestID → OriginCheck
       → BruteForceGuard → RateLimit → Auth → BodyScanner
       → OutputScanner → ReverseProxy → OpenClaw
```

Each middleware layer adds a defense. Failure at any layer results in request rejection with appropriate HTTP status code and structured logging.

### WebSocket Flow

```
Client → [TCP Upgrade] → SecurityHeaders → Auth → WS Tunnel
       → OutputScanner(backend→client) → Client
```

WebSocket streams are scanned in real-time via `ScanningReader`.

---

## 4. Residual Risks & Known Gaps

> **Last updated**: 2026-03-08 (post security-gap review)

| # | Risk | Severity | Status | Notes |
|---|------|----------|--------|-------|
| G1 | Prompt injection is heuristic (bypassable by novel techniques) | Medium | **Open** | Regex + heuristic only. Upgrade to ML-based classifier when available. Accepts residual bypass risk. |
| G2 | Output scanner encoding evasion | Medium | **Mitigated** | Added 2-round nested base64 decode + fullwidth unicode normalization (U+FF01–U+FF5E → ASCII). Scan-raw-first optimization skips expensive path when raw scan already triggers. |
| G3 | WebSocket size limits | Low | **Mitigated** | `io.LimitReader` caps client→backend at 100MB total connection bytes. Backend→client scanned by `ScanningReader`. |
| G4 | Unicode confusable character evasion (e.g. ꜱ U+A731 for 's', Cyrillic а U+0430 for 'a') | Low | **Accepted** | Full ICU confusable mapping would require large tables and risks false positives on legitimate multilingual content. Documented as residual risk. |
| G5 | In-memory state (bans, revocations, rate limits) lost on restart | Low | **Accepted** | Acceptable for single-node deployment; use Redis for multi-node. |
| G6 | No MFA for Wizard admin | Low | **Mitigated** | Optional TOTP via `WIZARD_TOTP_SECRET` env var. |
| G7 | Docker socket access grants container management | Medium | **Mitigated** | Replaced raw socket with tecnativa/docker-socket-proxy (read-only, allowlisted endpoints). |
| G8 | No request/response logging of full bodies | Low | **Accepted** | Logging full bodies would store PII/secrets. Structured metadata logging (request ID, risk score, path) considered sufficient. |
| G9 | Supply chain: govulncheck not in CI | Low | **Open** | Tracked in GitHub issue. Add `govulncheck ./...` to CI when stable in Go toolchain. |

---

## 5. Review Schedule

- **Quarterly**: Review STRIDE table against new features
- **On incident**: Update residual risks and mitigations
- **On dependency update**: Re-run `govulncheck` and update supply chain controls
- **Annually**: Full threat model review with external assessment

---

## 6. References

- [SECURITY.md](./SECURITY.md) — Security architecture details
- [RUNBOOK.md](./RUNBOOK.md) — Incident response playbooks
- [CONTRIBUTING.md](./CONTRIBUTING.md) — Development security standards
