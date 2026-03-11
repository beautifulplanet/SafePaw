---
description: How to manage SafePaw work using the SOW → CO → WO → Session Log system
---

# Work Order Workflow

This workflow defines how to start a SafePaw development session and manage
scoped work using work orders.

## Session Start Checklist

1. Read `AGENTS.md` — understand behavioral directives and scope rules
2. Read `docs/scope/SOW-001.md` — understand the contract and what's delivered
3. Scan `docs/scope/` for active work orders (`WO-*.md`) and change orders (`CO-*.md`)
4. Read the most recent `plan/` session log to understand current state
5. Confirm session scope with Dmitry before writing any code

## Creating a Work Order

Work orders are filed as `docs/scope/WO-NNN.md` using sequential numbering.
Use the template at `docs/scope/WO-TEMPLATE.md`.

### When to create a new WO:
- New session with a defined scope of work
- Multi-session phase that groups related items
- Any work not already covered by an existing active WO

### When NOT to create a new WO:
- Bug fix to existing delivered baseline (just fix it, log in session plan)
- Security vulnerability (fix immediately, document after — per SOW-001 §6)

## During a Session

// turbo-all

1. Mark the WO scope items as `[/]` (in progress) when you start them
2. Verify each item against acceptance criteria before marking `[x]` (done)
3. Follow AGENTS.md rules: walk don't sprint, verify before claiming done
4. If scope grows beyond the WO, stop and discuss with Dmitry

## Session End Checklist

1. Update the WO with final item status
2. Create a session log in `plan/YYYY-MM-DD-NNN-<slug>.md`
3. If WO is complete, change status to DONE
4. If WO spans multiple sessions, note "Next Steps" in the session log
