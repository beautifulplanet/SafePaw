# ADR-009: Codespaces-Aware URL Routing

**Status:** Accepted  
**Date:** 2026-03-08  
**Deciders:** Project leads  

## Context

SafePaw is developed and demonstrated in GitHub Codespaces. In a standard Docker
deployment, the wizard runs on `:3000` and the gateway on `:8080`. The "Chat with
AI" button in the wizard dashboard generates a URL to open OpenClaw through the
gateway with an authenticated token.

Codespaces uses a subdomain-based port forwarding scheme:

```
Standard:     localhost:8080
Codespaces:   cautious-space-xylophone-abc123-8080.app.github.dev
```

The port number is embedded in the subdomain, not appended with `:port`. This
means the standard URL construction (`hostname:8080`) produces invalid URLs in
Codespaces environments:

```
Wrong:  cautious-space-xylophone-abc123-3000.app.github.dev:8080
Right:  cautious-space-xylophone-abc123-8080.app.github.dev
```

Additionally, the gateway accepts authentication tokens via the `?token=` query
parameter. Originally, `extractToken()` in the auth middleware only read this
parameter for WebSocket upgrade requests. The "Chat with AI" button opens a
regular browser GET request, so the token was ignored — resulting in a
`missing_token` error.

## Decision

Two changes to support Codespaces-aware routing:

### 1. Dashboard URL construction (`Dashboard.tsx`)

Detect the Codespaces environment by checking if the hostname contains
`.app.github.dev`. If detected, swap the port in the subdomain:

```typescript
if (host.includes('.app.github.dev')) {
  // Replace -3000.app.github.dev with -8080.app.github.dev
  gatewayHost = host.replace(/-\d+\.app\.github\.dev/, '-8080.app.github.dev');
}
```

For non-Codespaces environments, use the standard `hostname:8080` pattern.

### 2. Gateway token extraction (`auth.go`)

Accept `?token=` query parameter for all HTTP request types, not just WebSocket
upgrades. The `Authorization` header takes priority if both are present.

```go
func extractToken(r *http.Request) string {
    if auth := r.Header.Get("Authorization"); ... { return bearerToken }
    if tok := r.URL.Query().Get("token"); tok != "" { return tok }
    return ""
}
```

## Consequences

**Good:**
- "Chat with AI" works in both standard Docker and GitHub Codespaces environments
  without manual URL editing
- Token-based browser navigation works for any HTTP request, enabling simple
  shareable links
- Detection is automatic — no configuration needed

**Neutral:**
- The Codespaces detection regex is specific to GitHub's current URL pattern.
  If GitHub changes the pattern, this would need updating. The standard Docker
  path is unaffected.
- `?token=` in URLs means tokens appear in browser history and server logs. This
  is acceptable for short-lived tokens (24h default TTL) and consistent with
  WebSocket token passing.

## References

- `services/wizard/ui/src/pages/Dashboard.tsx` — Codespaces URL detection
- `services/gateway/middleware/auth.go` — `extractToken()` function
- `services/gateway/middleware/auth_test.go` — Updated test for HTTP query param
