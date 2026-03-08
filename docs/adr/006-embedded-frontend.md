# ADR-006: Monorepo with Embedded React Frontend

**Status:** Accepted  
**Date:** 2026-02-10  
**Deciders:** Project leads  

## Context

The SafePaw wizard is an admin dashboard that provides:

- Login with RBAC (admin/operator/viewer)
- Service health monitoring via Docker API
- Configuration editor (`.env` file management)
- Gateway metrics visualization
- Service restart controls
- Prerequisite checking (Docker, Compose, ports, disk)

This UI needs to be served somehow. Options:

| Approach | How | Trade-offs |
|----------|-----|-----------|
| **Separate frontend service** | Nginx + React, served as Docker container | Extra container, CORS complexity, separate deployment pipeline |
| **CDN-hosted SPA** | React built to S3/CloudFront | Requires internet access, CORS, separate CI/CD |
| **Embedded in Go binary** | `go:embed` compiles React build into the server binary | Single artifact, no CORS, no extra containers |
| **Server-rendered (Go templates)** | HTML templates in Go | No reactivity, poor UX for dashboard, harder to iterate |

SafePaw's target deployment is a single `docker compose up`. Adding a
separate frontend container or CDN dependency contradicts the one-command
deployment goal.

## Decision

Embed the React SPA in the Go wizard binary using `go:embed`.

**Build pipeline:**
1. React app lives at `services/wizard/ui/`
2. `npm run build` compiles TypeScript → `dist/` (Vite + React 19 + Tailwind v4)
3. `embed.go` makes it available to Go:
   ```go
   //go:embed all:dist
   var DistFS embed.FS
   ```
4. `go build` bakes the `dist/` directory into the binary
5. The handler serves static files from the embedded FS
6. Non-API paths fall through to `index.html` (SPA routing)

**Auth boundary:**
- Static files (JS, CSS, images) served **without** auth — the login page must load
- API endpoints (`/api/v1/*`) require a valid session token
- CSP headers prevent XSS even if static files are publicly accessible

**Middleware chain applied to the single binary:**
```
SecurityHeaders → CORS → AdminAuth → CSRFProtect → RateLimit → Router
```

## Consequences

**Good:**
- **Single deployment artifact** — One Docker image, one binary, one port (3000).
  No nginx sidecar, no CDN, no CORS configuration.
- **No CORS issues** — Frontend and API share the same origin. The CORS
  middleware exists only for development (Vite dev server on a different port).
- **Atomic deployments** — Frontend and backend are always in sync. No version
  mismatch between API and UI.
- **Smaller infrastructure** — One fewer container in `docker-compose.yml`,
  one fewer health check, one fewer thing to monitor.
- **Offline capability** — The UI works without internet access (no CDN, no
  external font loading blocked by CSP).

**Bad:**
- **Build dependency** — Go build requires `dist/` to exist. The CI pipeline
  must run `npm install && npm run build` before `go build`.
- **Binary size increase** — The embedded UI adds ~2 MB to the binary.
  Acceptable for a server binary (total ~15 MB).
- **Frontend iteration speed** — During development, frontend changes require
  rebuilding the Go binary to test in production mode. Mitigated by Vite's
  dev server with hot reload during development.
- **Monorepo coupling** — Frontend and backend PRs are in the same repo.
  For a small team this is an advantage (atomic changes). For a large team
  it could slow down CI.

**Neutral:**
- The `all:` prefix in `//go:embed all:dist` includes hidden files like
  `.vite/`, which ensures the embedded FS is complete.
- TypeScript strict mode is enforced — no `any` types allowed.
- Tailwind v4 is used for styling (utility-first, no runtime CSS-in-JS).

## References

- `services/wizard/ui/embed.go` — Embed directive
- `services/wizard/ui/package.json` — React 19, Vite 7, Tailwind v4
- `services/wizard/cmd/wizard/main.go` — HTTP server, middleware chain
- `services/wizard/internal/api/handler.go` — SPA fallback handler
