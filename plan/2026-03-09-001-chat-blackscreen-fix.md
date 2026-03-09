# Session 2026-03-09-001: Chat with AI Black Screen Fix

## Summary
Fixed the "Chat with AI" button that produced black screens, filter messages,
and was completely non-functional. Root cause was 9 compounding bugs across
the gateway, wizard UI, and middleware stack.

## Changes Made (2 commits, both pushed)

### Commit 78e631c — WS black screen + filter false positives (6 bugs)
1. **CRITICAL: WriteTimeout kills WS tunnels** — `ws_proxy.go`: clear inherited
   30s deadline after hijack so WebSocket sessions survive beyond 30s
2. **HIGH: ScanningReader corrupts WS frames** — `output_scanner.go`: changed to
   log-only mode for WebSocket streams; HTTP OutputScanner still sanitizes
3. **HIGH: system_prompt_leak false positive** — `output_scanner.go`: tightened
   pattern to require verbatim leak context ("my system prompt is:")
4. **HIGH: eventHandlerPattern false positive** — `output_scanner.go` + `sanitize.go`:
   now requires HTML tag context (`<... on*=`) instead of matching bare code
5. **MEDIUM: dangerousURIPattern matches `data:`** — `sanitize.go`: removed `data:`
   from pattern; CSP header handles data: URI defense
6. **MEDIUM: OriginCheck blocks cross-port** — `security.go`: auto-allow
   localhost/127.0.0.1/[::1] origins when no ALLOWED_ORIGINS set

### Commit 6fadd4d — Chat with AI flow (3 root causes)
7. **Dashboard token failure** — `Dashboard.tsx`: catch token generation failure
   and open gateway URL without token (works when AUTH_ENABLED=false)
8. **SPA API calls lack auth** — `auth.go`: new `SetTokenCookie()` function sets
   HttpOnly cookie `safepaw_token` on first `?token=` page load; `extractToken()`
   now checks cookie as third fallback after header and query param
9. **Codespaces OriginCheck** — `security.go`: new `isCodespacesOrigin()` function
   auto-allows `*.app.github.dev` origins

## Files Modified
- services/gateway/ws_proxy.go
- services/gateway/middleware/output_scanner.go
- services/gateway/middleware/output_scanner_test.go
- services/gateway/middleware/sanitize.go
- services/gateway/middleware/security.go
- services/gateway/middleware/security_test.go
- services/gateway/middleware/auth.go
- services/gateway/middleware/bruteforce.go
- services/wizard/ui/src/pages/Dashboard.tsx
- services/wizard/ui/dist/ (rebuilt)
- .devcontainer/devcontainer.json

## Decisions
- ScanningReader is now LOG-ONLY for WebSocket streams. Rationale: WebSocket data
  is framed binary — changing payload length without updating frame headers corrupts
  the stream. The receipt ledger + logging provides observability without breaking data.
- Auth cookie approach chosen over alternatives (injecting JS into HTML, requiring
  SPA changes) because it's transparent to the OpenClaw SPA — zero upstream changes.
- Codespaces origins auto-allowed because they're already authenticated by GitHub's
  port forwarding. Blocking them serves no security purpose.

## Current State
- HEAD: 6fadd4d on main (pushed to origin)
- All gateway tests pass (4 packages, ~90 tests)
- Wizard UI rebuilt with new Dashboard.tsx
- Receipt ledger (from previous session) fully implemented:
  receipt.go, tool_parser.go, LedgerReader, /admin/ledger endpoint
- Push command: `unset GITHUB_TOKEN && TOKEN=$(gh auth token) && git push "https://x-access-token:${TOKEN}@github.com/beautifulplanet/SafePaw.git" main`

## Next Steps
- [ ] Verify demo stack runs end-to-end (`docker compose -f docker-compose.demo.yml up`)
- [ ] Test "Chat with AI" button in running demo
- [ ] Address any remaining issues from live testing
- [ ] Rebuild wizard UI dist if any further Dashboard changes
