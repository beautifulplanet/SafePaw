# Plan — Session Logs & Context

This folder is an append-only log of every development session.
Each file captures what was done, what was decided, and what's next.

**Purpose**: AI agents lose context between sessions. These logs let
the agent pick up exactly where it left off without re-investigating.

**Rules**:
- One file per session, named `YYYY-MM-DD-NNN-<slug>.md`
- Append only — never edit previous session files
- Each file has: Summary, Changes Made, Decisions, Current State, Next Steps
- Keep it factual and concise — this is for machines, not prose
