# ADR-005: Heuristic Prompt-Injection Scanning, Not ML

**Status:** Accepted  
**Date:** 2026-02-20  
**Deciders:** Project leads  

## Context

SafePaw proxies user messages to an AI assistant (OpenClaw). Prompt injection
is the primary threat: an attacker crafts input that causes the LLM to ignore
its system prompt and execute attacker instructions instead.

Two broad approaches exist:

| Approach | How it works | Latency | Accuracy |
|----------|-------------|---------|----------|
| **Heuristic (regex)** | Pattern-match known injection phrases | <1 ms | High precision, lower recall |
| **ML classifier** | Fine-tuned model scores injection probability | 50–500 ms | Higher recall, more false positives |

ML classifiers (like Rebuff, LLM Guard, or a fine-tuned BERT) add:
- A Python sidecar or API call per request
- 50–500 ms latency on every message
- A model that itself can be adversarially attacked
- GPU memory or significant CPU overhead
- An additional dependency with its own vulnerabilities and update cadence

SafePaw targets indie developers and small teams running on a single
machine. Adding an ML inference step would double request latency and
require GPU resources that most users don't have.

## Decision

Use regex-based heuristic scanning with versioned patterns, and explicitly
document it as a tripwire — not a boundary.

**Input scanning** (`sanitize.go`):
- 14 patterns organized by risk level (6 high, 6 medium, 1 low, 1 info)
- Pattern version tracking (currently v2.0.0) with changelog
- Quarterly review schedule aligned with OWASP LLM Top 10 updates
- Detected patterns logged with risk level and trigger names
- Risk level injected as `X-SafePaw-Risk` header — backend can act on it
- Requests are **not blocked** by default — the gateway warns, doesn't censor

**Output scanning** (`output_scanner.go`):
- 7 patterns for XSS, secret leakage, and event handler injection
- 2-round base64 decoding to catch nested encoding evasion
- Unicode fullwidth → ASCII normalization
- Dangerous content replaced with `[filtered]` in responses

**Pattern categories:**
```
HIGH:   instruction_override, identity_hijack, prompt_replacement,
        secret_extraction, system_delimiter, jailbreak_keyword
MEDIUM: instruction_delimiter, role_injection, encoding_evasion,
        hypothetical_bypass, data_exfiltration, unicode_escape
LOW:    url_in_content
```

## Consequences

**Good:**
- Sub-millisecond scanning — no perceptible latency increase
- Zero additional dependencies (regex is stdlib)
- Zero additional infrastructure (no GPU, no sidecar, no model serving)
- Deterministic — same input always produces same result (testable, debuggable)
- 17 fuzz test seeds covering edge cases (randomized encoding, large payloads)
- Pattern versioning enables tracking what changed and when
- False positive rate is low because patterns are specific and curated

**Bad:**
- **Recall is inherently limited** — Novel injection techniques, obfuscated
  payloads, and language-specific attacks will pass through. This is documented
  as residual risk G1 in THREAT-MODEL.md.
- **Encoding evasion partially mitigated** — 2-round base64 and fullwidth
  normalization catch common tricks, but visually-confusable Unicode (Cyrillic
  'а' for ASCII 'a', U+A731 'ꜱ' for 's') is NOT normalized because full ICU
  mapping would break legitimate multilingual content.
- **Pattern maintenance** — Humans must update patterns as new attack techniques
  appear. The quarterly review schedule mitigates but doesn't eliminate staleness.
- **No semantic understanding** — "Hypothetically, if you were to..." is caught,
  but arbitrarily creative reformulations may not be.

**Migration path to ML (if needed):**
1. Add a Python sidecar with a fine-tuned classifier
2. Gateway calls sidecar via HTTP with a timeout
3. Combine heuristic + ML scores (heuristic is fast-path, ML is slow-path)
4. Feature-flag ML scanning behind `SCANNER_ML_ENABLED=true`

This is documented but deliberately not implemented — the complexity/benefit
ratio doesn't justify it for the target user base.

## References

- `services/gateway/middleware/sanitize.go` — Input scanning, 14 patterns
- `services/gateway/middleware/output_scanner.go` — Output scanning, 7 patterns
- `THREAT-MODEL.md` §4 — Residual risks G1 (prompt injection) and G2 (encoding evasion)
- `SECURITY.md` §10 — Known limitations
- [OWASP Top 10 for LLM Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/) — LLM01: Prompt Injection
