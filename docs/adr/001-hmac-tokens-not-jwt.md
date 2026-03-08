# ADR-001: HMAC-SHA256 Tokens Instead of JWT

**Status:** Accepted  
**Date:** 2026-02-15  
**Deciders:** Project leads  

## Context

SafePaw needs two kinds of signed tokens:

1. **Gateway tokens** — authenticate API clients and WebSocket connections to the
   security proxy. These are issued by the wizard or `tokengen` CLI and validated
   on every request.
2. **Wizard session tokens** — authenticate browser sessions after login. Stored
   in HttpOnly cookies.

The industry default for signed tokens is JSON Web Tokens (JWT), typically using
a library like `golang-jwt/jwt` or `lestrrat-go/jwx`. However, JWT has a long
history of implementation vulnerabilities:

- **"alg: none" attacks** — Attacker strips the signature and sets the algorithm
  to `none`. Libraries that don't reject unsigned tokens silently accept them.
- **Algorithm confusion** — Switching from RS256 to HS256 tricks the verifier
  into using the public key as the HMAC secret.
- **Header injection** — The JWT header is attacker-controlled JSON. Libraries
  that interpret `kid`, `jku`, or `x5u` fields can be tricked into fetching
  attacker keys.
- **Dependency sprawl** — `golang-jwt/jwt` v5 alone pulls in 3 modules.
  `lestrrat-go/jwx` pulls in 15+.

SafePaw's token needs are deliberately simple: sign a JSON payload, verify the
signature, check expiry. There is no need for multiple algorithms, key rotation
via headers, or JWK discovery.

## Decision

Use hand-rolled HMAC-SHA256 tokens with a minimal `payload.signature` format:

```
base64url(json_payload) "." base64url(hmac_sha256(payload, secret))
```

**Gateway token payload:**
```json
{"sub": "user-id", "iat": 1709000000, "exp": 1709086400, "scope": "proxy"}
```

**Wizard session payload:**
```json
{"sub": "admin", "iat": 1709000000, "exp": 1709086400, "jti": "a1b2c3", "gen": 1, "role": "admin"}
```

Implementation:
- Signing: `hmac.New(sha256.New, secret)` — stdlib only
- Verification: `hmac.Equal()` — constant-time comparison
- Secret requirement: ≥32 bytes (enforced at startup)
- Wizard adds `jti` (crypto/rand nonce) for replay protection
- Wizard adds `gen` for session invalidation on credential change

## Consequences

**Good:**
- Zero external dependencies for auth — no transitive supply chain risk
- No "alg: none" or algorithm confusion possible (algorithm is hardcoded)
- No attacker-controlled header (there is no header — just payload + signature)
- Complete control over validation logic (~80 lines total)
- Constant-time comparison via `hmac.Equal` prevents timing attacks
- Smaller binary, faster compilation

**Bad:**
- No ecosystem tooling (can't paste tokens into jwt.io for debugging)
- Must hand-write token generation CLI (`tools/tokengen`)
- No built-in support for asymmetric keys (RSA/ECDSA) — if we ever need
  third-party verification without sharing the secret, we'd need to change format
- Token rotation requires manual `AUTH_SECRET` change (no `kid` header for
  key rotation)

**Neutral:**
- Format is visually similar to JWT (base64.base64) so devs understand it instantly
- Migration to JWT is straightforward if needed — just change the encoding layer

## References

- [Critical vulnerabilities in JSON Web Token libraries](https://auth0.com/blog/critical-vulnerabilities-in-json-web-token-libraries/) — Auth0, 2015
- [JWT Security Best Practices](https://datatracker.ietf.org/doc/html/rfc8725) — RFC 8725
- `services/gateway/middleware/auth.go` — Gateway token implementation
- `services/wizard/internal/session/session.go` — Wizard session implementation
