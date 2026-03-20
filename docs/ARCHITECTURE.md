# Architecture Guide

> Complete technical architecture of InstallerClaw (SafePaw) — the security perimeter for self-hosted AI assistants.

## System Overview

InstallerClaw is a Go-based security gateway and administration wizard that wraps [OpenClaw](https://github.com/nicepkg/openclaw) in a hardened Docker environment. It provides authentication, rate limiting, prompt-injection scanning, observability, and guided setup through a single-command deployment.

```mermaid
graph TB
    subgraph host["Host (127.0.0.1 only)"]
        wizard["<b>Wizard :3000</b><br/>Go + React 19 SPA<br/>Admin Dashboard · Config · Audit"]
        gateway["<b>Gateway :8080</b><br/>Go Reverse Proxy<br/>Auth · Scan · Rate Limit · Metrics"]
    end

    subgraph internal["Internal Docker Network"]
        openclaw["<b>OpenClaw :18789</b><br/>AI Assistant<br/>LLM · Channels · Tools"]
        redis["<b>Redis :6379</b><br/>Rate Limit State<br/>Token Revocation<br/>Brute-Force Bans"]
        postgres["<b>Postgres :5432</b><br/>Config Store<br/>Cost Analytics<br/>Audit Log"]
        sockproxy["<b>Docker Socket Proxy</b><br/>Read-Only Container API"]
    end

    client(("Client")) -->|"Authenticated traffic"| gateway
    admin(("Admin")) -->|"Session + TOTP"| wizard
    gateway -->|"Proxied HTTP/WS"| openclaw
    gateway -->|"Revocation & bans"| redis
    wizard -->|"Container health"| sockproxy
    wizard -->|"Config & analytics"| postgres
    wizard -->|"Rate limit state"| redis

    style host fill:#f0f9ff,stroke:#0284c7,stroke-width:2px
    style internal fill:#f0fdf4,stroke:#16a34a,stroke-width:2px
```

## Components

### Gateway (Go)

The gateway is a security-hardened reverse proxy. Every request to OpenClaw passes through it.

| Capability | Implementation |
|-----------|---------------|
| Authentication | HMAC-SHA256 tokens (custom format, no JWT) |
| Token revocation | Redis-backed persistent revocation list |
| Rate limiting | Per-IP sliding window with configurable threshold |
| Brute-force protection | IP banning with escalating durations (5m → 15m → 60m → 240m) |
| Prompt injection scanning | 14 heuristic patterns on HTTP `POST/PUT/PATCH` with `application/json` body only. WebSocket chat messages are **not** scanned by the body scanner — they bypass this layer after the initial HTTP upgrade. |
| Output scanning | XSS, secret leak, encoding evasion detection on **HTTP responses only**. WebSocket streams are scanned and risk-logged but passed through unmodified — modifying payload bytes without updating binary frame headers would corrupt the stream and cause client failures. See `output_scanner.go` for the documented trade-off. |
| Security headers | HSTS, CSP, X-Frame-Options, X-Content-Type-Options |
| Request IDs | Server-generated UUID (client headers ignored) |
| Metrics | Prometheus-compatible text format at `/metrics` |
| WebSocket proxy | Full-duplex tunneling with receipt ledger instrumentation |
| Receipt ledger | Append-only agent action traceability (tool calls, results, sessions) |
| TLS | Optional TLS 1.2+ with strong cipher suites |

**Source:** `services/gateway/`  
**Dependencies:** 2 external (github.com/google/uuid, github.com/coder/websocket)

### Wizard (Go + React 19)

The wizard is a single-binary admin dashboard. The React SPA is embedded via `go:embed`.

| Capability | Implementation |
|-----------|---------------|
| Authentication | HMAC session cookies with optional TOTP MFA |
| CSRF protection | Double-submit cookie pattern |
| RBAC | Three roles: admin, operator, viewer |
| Config management | Allowlisted `.env` editing with masked secrets |
| Service health | Real-time Docker container status via socket proxy |
| Cost analytics | Postgres-backed daily rollup with per-model breakdown |
| Audit log | Login, config changes, restarts, token creations |
| Gateway integration | Token generation, metrics display, usage monitoring |

**Source:** `services/wizard/`  
**Dependencies:** 1 external (github.com/lib/pq)

## Middleware Pipeline

Every gateway request passes through this ordered middleware chain:

```mermaid
graph LR
    req(("Request")) --> M["Metrics<br/>Counter"]
    M --> SH["Security<br/>Headers"]
    SH --> RID["Request ID<br/>UUID"]
    RID --> AE["Audit<br/>Emitter"]
    AE --> OC["Origin<br/>Check"]
    OC --> BF["Brute-Force<br/>Guard"]
    BF --> RL["Rate<br/>Limiter"]
    RL --> AUTH["HMAC<br/>Auth"]
    AUTH --> BS["Body<br/>Scanner"]
    BS --> OS["Output<br/>Scanner"]
    OS --> PROXY["Reverse<br/>Proxy"]
    PROXY --> Backend["OpenClaw"]

    style req fill:#fbbf24,stroke:#d97706
    style AUTH fill:#3b82f6,stroke:#1d4ed8,color:#fff
    style BS fill:#f97316,stroke:#ea580c,color:#fff
    style OS fill:#f97316,stroke:#ea580c,color:#fff
    style Backend fill:#22c55e,stroke:#16a34a,color:#fff
```

Each middleware is a standard `http.Handler` wrapper. The chain is composed in `main.go`:

```go
handler = middleware.AuthRequiredWithGuard(auth, "proxy", revocations, bruteForce, handler)
handler = middleware.RateLimitWithGuard(rateLimiter, bruteForce, handler)
handler = middleware.BruteForceMiddleware(bruteForce, handler)
handler = middleware.OriginCheck(cfg.AllowedOrigins, handler)
handler = middleware.AuditEmitter(handler)
handler = middleware.RequestID(handler)
handler = middleware.SecurityHeaders(handler)
handler = middleware.MetricsMiddleware(metrics, handler)
```

## Authentication Flow

### Gateway Tokens (API/WebSocket)

```mermaid
sequenceDiagram
    participant Admin as Wizard Admin
    participant API as Wizard API
    participant Client as API Client
    participant GW as Gateway
    participant Redis

    Note over Admin,API: Token Issuance
    Admin->>API: POST /api/v1/login
    API-->>Admin: Session cookie (HMAC-signed)
    Admin->>API: POST /api/v1/gateway-token
    API->>API: Sign(payload, AUTH_SECRET)
    API-->>Admin: HMAC token {sub, scope, exp}

    Note over Client,Redis: Token Validation
    Client->>GW: Request + Authorization: Bearer <token>
    GW->>GW: HMAC-SHA256 verify (constant-time)
    GW->>Redis: Check revocation(subject)
    Redis-->>GW: Not revoked
    GW->>GW: Validate scope + expiry
    GW-->>Client: Proxied response
```

**Token format:** `base64url(payload).base64url(hmac_sha256(payload, secret))`

**Payload fields:** `sub` (subject), `iat` (issued at), `exp` (expires), `scope` (permissions)

### Wizard Sessions (Browser)

```mermaid
sequenceDiagram
    participant Browser
    participant Wizard as Wizard API
    participant TOTP as TOTP Validator

    Browser->>Wizard: POST /api/v1/login {password, totp_code?}
    Wizard->>Wizard: Validate password (constant-time)
    opt TOTP Enabled
        Wizard->>TOTP: Validate code (±1 window)
        TOTP-->>Wizard: Valid
    end
    Wizard->>Wizard: Sign session {sub, jti, gen, role, exp}
    Wizard-->>Browser: Set-Cookie: session (HttpOnly, Secure, SameSite=Strict)
    Wizard-->>Browser: Set-Cookie: csrf (JS-readable, SameSite=Strict)

    Note over Browser,Wizard: Subsequent Requests
    Browser->>Wizard: POST /api/v1/config + X-CSRF-Token header
    Wizard->>Wizard: Verify session cookie HMAC
    Wizard->>Wizard: Verify CSRF token matches cookie
    Wizard-->>Browser: Response
```

## WebSocket Proxy & Receipt Ledger

The gateway provides full-duplex WebSocket proxying with agent action traceability:

```mermaid
sequenceDiagram
    participant Client
    participant GW as Gateway
    participant Ledger as Receipt Ledger
    participant OC as OpenClaw

    Client->>GW: WebSocket Upgrade + Auth
    GW->>GW: Validate token, rate limit
    GW->>OC: TCP dial + upgrade replay
    GW->>Ledger: Append(session_start, subject, session_id)

    loop Agent Interaction
        Client->>GW: User prompt
        GW->>OC: Forward message
        OC->>GW: tool_use {name, input}
        GW->>Ledger: Append(tool_call, tool_name)
        GW->>Client: Forward tool invocation
        OC->>GW: tool_result {output}
        GW->>Ledger: Append(tool_result, duration)
        GW->>GW: Output scan (XSS, secrets)
        alt Risk Detected
            GW->>Ledger: Append(quality_flag, risk_level)
        end
        GW->>Client: Forward result + X-SafePaw-Risk header
    end

    Client->>GW: Connection close
    GW->>Ledger: Append(session_end, total_duration)
    GW->>OC: Close backend connection
```

**Receipt properties:**
- Monotonic sequence numbers (no gaps)
- Immutable entries (append-only)
- Bounded ring buffer (default 10,000 entries)
- Queryable by request_id, session_id, subject, action, time

## Prompt Injection Scanning

The body scanner inspects POST/PUT/PATCH JSON payloads for prompt injection patterns:

```mermaid
graph TD
    REQ["Incoming Request"] --> CHECK{"POST/PUT/PATCH<br/>+ JSON body?"}
    CHECK -->|No| PASS["Pass through"]
    CHECK -->|Yes| READ["Read body<br/>(size-limited)"]
    READ --> EXTRACT["Extract text fields<br/>from JSON"]
    EXTRACT --> SCAN["Scan against<br/>14 patterns"]
    SCAN --> ASSESS{"Risk level?"}
    ASSESS -->|None| FORWARD["Forward to backend"]
    ASSESS -->|Low| TAG_LOW["Set X-SafePaw-Risk: low<br/>Forward to backend"]
    ASSESS -->|Medium| TAG_MED["Set X-SafePaw-Risk: medium<br/>Forward to backend"]
    ASSESS -->|High| BLOCK["Set X-SafePaw-Risk: high<br/>Log + Forward"]

    FORWARD --> BACKEND["OpenClaw"]
    TAG_LOW --> BACKEND
    TAG_MED --> BACKEND
    BLOCK --> BACKEND

    style BLOCK fill:#ef4444,stroke:#b91c1c,color:#fff
    style TAG_MED fill:#f97316,stroke:#ea580c,color:#fff
    style TAG_LOW fill:#fbbf24,stroke:#d97706
    style FORWARD fill:#22c55e,stroke:#16a34a,color:#fff
```

**14 Detection Patterns:**
1. System prompt override (`ignore previous instructions`)
2. Role hijacking (`you are now`)
3. Data exfiltration (`send to`, `curl`, URL patterns)
4. Jailbreak triggers (`DAN`, `developer mode`)
5. Encoding evasion (base64, hex, unicode)
6. Delimiter injection (markdown code blocks as prompt separators)
7. And more — versioned in `middleware/sanitize.go`

## Output Scanning

Response scanning catches data leakage and injection in backend responses:

```mermaid
graph TD
    RESP["Backend Response"] --> OSCAN["Output Scanner"]
    OSCAN --> DECODE["2-round decode<br/>(base64, hex, URL)"]
    DECODE --> CHECK_XSS["Check XSS patterns"]
    CHECK_XSS --> CHECK_SEC["Check secret patterns<br/>(API keys, tokens)"]
    CHECK_SEC --> CHECK_EXFIL["Check exfiltration<br/>(URLs, IPs)"]
    CHECK_EXFIL --> RESULT{"Findings?"}
    RESULT -->|Clean| DELIVER["Deliver to client"]
    RESULT -->|Flagged| SANITIZE["Sanitize + tag headers<br/>X-SafePaw-Risk"]
    SANITIZE --> DELIVER

    style OSCAN fill:#f97316,stroke:#ea580c,color:#fff
    style SANITIZE fill:#ef4444,stroke:#b91c1c,color:#fff
    style DELIVER fill:#22c55e,stroke:#16a34a,color:#fff
```

## Deployment Architecture

### Docker Compose Stack

```mermaid
graph TB
    subgraph compose["docker-compose.yml"]
        subgraph exposed["Exposed on 127.0.0.1"]
            W["<b>Wizard</b><br/>:3000<br/>Go + React<br/>Admin UI"]
            G["<b>Gateway</b><br/>:8080<br/>Go Proxy<br/>Security Perimeter"]
        end

        subgraph isolated["Internal Network Only"]
            OC["<b>OpenClaw</b><br/>:18789<br/>AI Assistant"]
            R["<b>Redis</b><br/>:6379<br/>State Store"]
            PG["<b>Postgres</b><br/>:5432<br/>Config & Analytics"]
            DSP["<b>Docker Socket Proxy</b><br/>Read-Only API"]
        end
    end

    subgraph monitoring["Optional Monitoring"]
        PROM["Prometheus<br/>Scraping"]
        GRAF["Grafana<br/>Dashboards"]
        ALERTS["6 Alert Rules"]
    end

    G -->|"All traffic proxied"| OC
    G -->|"Revocation + bans"| R
    W -->|"Config + cost analytics"| PG
    W -->|"Container health"| DSP
    W -->|"Rate limit state"| R
    PROM -->|"GET /metrics"| G
    GRAF --> PROM
    ALERTS --> GRAF

    style exposed fill:#dbeafe,stroke:#2563eb,stroke-width:2px
    style isolated fill:#dcfce7,stroke:#16a34a,stroke-width:2px
    style monitoring fill:#fef3c7,stroke:#d97706,stroke-width:1px
```

### Network Isolation

| Service | Exposed Port | Internal Port | Purpose |
|---------|-------------|---------------|---------|
| Wizard | 127.0.0.1:3000 | 3000 | Admin dashboard |
| Gateway | 127.0.0.1:8080 | 8080 | Security proxy |
| OpenClaw | — | 18789 | AI backend (internal only) |
| Redis | — | 6379 | State store (internal only) |
| Postgres | — | 5432 | Config/analytics (internal only) |
| Docker Socket Proxy | — | 2375 | Container API (internal only) |

### Resource Profiles

The stack auto-detects available RAM and applies resource limits:

| Profile | RAM | Gateway | Wizard | OpenClaw | Redis | Postgres |
|---------|-----|---------|--------|----------|-------|----------|
| Small | <4 GB | 256 MB | 256 MB | 512 MB | 128 MB | 256 MB |
| Medium | 4–8 GB | 512 MB | 512 MB | 2 GB | 256 MB | 512 MB |
| Large | 8–16 GB | 1 GB | 1 GB | 4 GB | 512 MB | 1 GB |
| Very Large | >16 GB | 2 GB | 1 GB | 8 GB | 1 GB | 2 GB |

## Security Architecture

### Defense-in-Depth Layers

```mermaid
graph TB
    subgraph L1["Layer 1: Network"]
        TLS["TLS 1.2+"]
        BIND["Localhost binding"]
        INTERNAL["Internal-only backends"]
    end

    subgraph L2["Layer 2: Authentication"]
        HMAC["HMAC-SHA256 tokens"]
        REVOKE["Redis revocation"]
        BRUTE["Brute-force guard"]
    end

    subgraph L3["Layer 3: Rate Limiting"]
        RATE["Per-IP sliding window"]
        ESCALATE["Escalating bans"]
    end

    subgraph L4["Layer 4: Input Scanning"]
        BODY["14-pattern body scanner"]
        RISK["Risk classification"]
    end

    subgraph L5["Layer 5: Output Scanning"]
        XSS["XSS detection"]
        SECRET["Secret leak detection"]
        ENCODE["Encoding evasion detection"]
    end

    subgraph L6["Layer 6: Observability"]
        AUDIT["Structured audit log"]
        METRICS["Prometheus metrics"]
        LEDGER["Receipt ledger"]
    end

    L1 --> L2 --> L3 --> L4 --> L5 --> L6

    style L1 fill:#dbeafe,stroke:#2563eb
    style L2 fill:#fce7f3,stroke:#be185d
    style L3 fill:#fef3c7,stroke:#d97706
    style L4 fill:#fee2e2,stroke:#dc2626
    style L5 fill:#fee2e2,stroke:#dc2626
    style L6 fill:#dcfce7,stroke:#16a34a
```

### STRIDE Threat Coverage

48 threats identified and mitigated across all STRIDE categories. Full analysis: [THREAT-MODEL.md](../THREAT-MODEL.md).

| Category | Threats | Status |
|----------|---------|--------|
| **S**poofing | Token forgery, session hijacking, identity impersonation | Mitigated |
| **T**ampering | Request modification, header injection, log poisoning | Mitigated |
| **R**epudiation | Unattributed actions, missing audit trail | Mitigated |
| **I**nformation Disclosure | Secret leakage, data exfiltration, error verbosity | Mitigated |
| **D**enial of Service | Rate abuse, resource exhaustion, connection flooding | Mitigated |
| **E**levation of Privilege | Scope bypass, Docker socket abuse, admin escalation | Mitigated |

## Technology Decisions

All major architecture decisions are documented as ADRs in [docs/adr/](adr/):

| ADR | Decision | Rationale |
|-----|---------|-----------|
| [001](adr/001-hmac-tokens-not-jwt.md) | HMAC-SHA256 over JWT | Eliminates "alg:none" attacks, zero auth dependencies |
| [002](adr/002-zero-external-middleware-deps.md) | Zero middleware deps | 2 external packages total; minimal supply chain risk |
| [003](adr/003-go-for-gateway.md) | Go for the gateway | Single binary, stdlib controls, optimal for reverse proxy |
| [004](adr/004-docker-socket-proxy.md) | Docker socket proxy | Read-only container access; no privilege escalation |
| [005](adr/005-heuristic-scanning-not-ml.md) | Heuristic scanning | No ML dependency; versioned, auditable patterns |
| [006](adr/006-embedded-frontend.md) | Embedded React SPA | Single binary via go:embed; no CORS, atomic deploys |
| [007](adr/007-receipt-ledger.md) | Receipt ledger | Agent action traceability; append-only, bounded |
| [008](adr/008-csrf-double-submit.md) | CSRF double-submit | Stateless CSRF protection for SPA architecture |
| [009](adr/009-codespaces-url-routing.md) | Codespaces routing | Automatic port-forwarded URL detection |

## Testing Architecture

```mermaid
graph LR
    subgraph unit["Unit Tests (530)"]
        GT["Gateway: 353<br/>middleware, config"]
        WT["Wizard: 177<br/>session, TOTP, API"]
    end

    subgraph fuzz["Fuzz Testing (7 targets)"]
        F1["Prompt injection"]
        F2["Sanitize"]
        F3["Channel parsing"]
        F4["Output scan"]
        F5["Token validation"]
        F6["KV parser"]
        F7["Body scanner"]
    end

    subgraph integration["Integration"]
        SMOKE["smoke-test.sh<br/>20 endpoint tests"]
        VERIFY["verify-deployment.sh<br/>Live stack validation"]
        API_TEST["api-test-collection.sh<br/>7 test suites"]
    end

    subgraph ci["CI Pipeline (5 jobs)"]
        BUILD["Build + Test"]
        LINT["golangci-lint"]
        SEC["gosec + govulncheck"]
        DOCKER["Docker build"]
        COV["Coverage gate<br/>65% / 55%"]
    end

    style unit fill:#dbeafe,stroke:#2563eb
    style fuzz fill:#fef3c7,stroke:#d97706
    style integration fill:#dcfce7,stroke:#16a34a
    style ci fill:#fce7f3,stroke:#be185d
```

## Documentation Map

| Document | Focus Area |
|----------|-----------|
| [README.md](../README.md) | Project overview, quickstart, feature summary |
| [ARCHITECTURE.md](ARCHITECTURE.md) | This document — complete technical architecture |
| [SECURITY.md](../SECURITY.md) | Security posture, incident response, hardening |
| [THREAT-MODEL.md](../THREAT-MODEL.md) | STRIDE analysis, 48 threats, mitigations |
| [RUNBOOK.md](../RUNBOOK.md) | 6 operational playbooks, secret rotation |
| [BACKUP-RECOVERY.md](../BACKUP-RECOVERY.md) | Backup/restore procedures |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Development setup, coding standards |
| [CHANGELOG.md](../CHANGELOG.md) | Release history |
| [docs/adr/](adr/) | 9 Architecture Decision Records |
| [docs/COMPLIANCE.md](COMPLIANCE.md) | SOC 2 & GDPR control mapping |
| [docs/PENTEST-POLICY.md](PENTEST-POLICY.md) | Penetration testing scope |
| [docs/PATCHING-POLICY.md](PATCHING-POLICY.md) | Dependency update SLAs |
