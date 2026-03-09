# ADR-008: CSRF Double-Submit Cookie Pattern

**Status:** Accepted  
**Date:** 2026-03-07  
**Deciders:** Project leads  

## Context

The wizard admin UI is a React SPA served from the same origin as its API
(`/api/v1/*`). Authentication uses HMAC-signed session cookies set with
`HttpOnly`, `SameSite=Strict`, and `Secure` (when TLS is enabled).

Cookie-based auth is vulnerable to Cross-Site Request Forgery (CSRF): a
malicious page can trigger state-changing requests to the wizard API, and the
browser will automatically attach the session cookie.

Common CSRF defenses:

| Approach | How | Trade-offs |
|----------|-----|-----------|
| **Synchronizer token** | Server generates a random token, embeds in HTML form, validates on submit | Requires server-rendered forms or a token-fetch endpoint; adds server state |
| **Double-submit cookie** | Server sets a JS-readable cookie; client reads it and sends as a header | Stateless on the server; works with SPAs; requires same-origin |
| **SameSite cookie only** | Rely on `SameSite=Strict` to block cross-origin requests | Browser-dependent; older browsers may not enforce it; defense-in-depth recommends layering |
| **Custom header check** | Require a custom header (e.g., `X-Requested-With`) | Simple but doesn't prove possession of a secret |

## Decision

Implement the **double-submit cookie** pattern:

1. **On login success**, the wizard API sets two cookies:
   - `session` — HttpOnly, SameSite=Strict (the auth token)
   - `csrf` — JS-readable (not HttpOnly), SameSite=Strict (random value)

2. **On every mutating request** (POST, PUT, DELETE), the React client reads the
   `csrf` cookie and sends its value as the `X-CSRF-Token` header.

3. **The CSRF middleware** validates that `X-CSRF-Token` header matches the
   `csrf` cookie value. If they don't match, the request is rejected with 403.

**Why this works:**
- A cross-origin attacker can trigger the browser to _send_ the csrf cookie, but
  cannot _read_ it (same-origin policy prevents cookie access from other domains)
- The attacker therefore cannot set the `X-CSRF-Token` header to the correct value
- Combined with `SameSite=Strict`, this provides two independent layers of CSRF
  protection

**Implementation:**
- `services/wizard/internal/middleware/csrf.go` — Middleware that validates the
  double-submit pattern on POST/PUT/DELETE
- `services/wizard/internal/api/handlers.go` — Login handler sets the csrf cookie
- `services/wizard/ui/src/api.ts` — `getCsrfToken()` reads the cookie;
  `request()` auto-attaches `X-CSRF-Token` on mutating requests

## Consequences

**Good:**
- Stateless — no server-side CSRF token store; validation is a simple string
  comparison between header and cookie
- Works naturally with SPA architecture — the React client reads the cookie and
  attaches the header automatically via the `api.ts` request wrapper
- Two independent layers: SameSite=Strict (browser-enforced) + double-submit
  (application-enforced)
- Zero additional dependencies

**Neutral:**
- Requires the frontend to explicitly read the cookie and set the header. This is
  handled once in `api.ts` and applied to all requests automatically.
- The csrf cookie must not be HttpOnly (the client needs to read it). This is by
  design — the cookie value is not a secret; it's a proof-of-same-origin.

## References

- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- `services/wizard/internal/middleware/csrf.go` — Server-side validation
- `services/wizard/ui/src/api.ts` — Client-side token attachment
