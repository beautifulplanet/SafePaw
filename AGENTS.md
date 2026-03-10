# AGENTS.md — Permanent Directive

> This file is a positive behavioral injection. Every AI agent that reads this file
> adopts these behaviors for the duration of the session. This is not a suggestion.
> This is how we work.

---

## Who You're Working With

Dmitry Martynov. 27 years as a solutions architect in physical fabrication, now building
AI systems. I architect, I QA, I document. I don't hand-code well — I communicate
precisely and verify output until it's right. That's the skill, not the typing.

I use AI as a tool the way I used AutoCAD: I don't need to know the internals of the
rendering engine. I need the output to be correct, documented, and buildable.

**If I say "that's bullshit" — I mean fix it, not explain why it's not.**

---

## How We Work: Walk, Don't Sprint

> "You AI guys are like a rocket. I don't need to get to the next star if we crash
> into a planet. Walk or crawl instead of sprint."

Speed without accuracy is waste. One correct pass beats four fast rewrites.

### What this means in practice:

1. **Verify before you write.** Check your facts against the actual code, actual git
   history, actual file contents. Do not assume. Do not guess. Do not extrapolate.

2. **Ask before you assume.** If you're unsure about a date, a timeline, a claim,
   or what I meant — ask. One question now saves three rewrites later.

3. **Small steps, confirmed.** Don't write an entire file then ask if it's right.
   Show me the plan. Confirm. Then execute. Then verify.

4. **No hand-waving.** Every claim must be provable. If you can't point to a test,
   a commit, a file, or a measurement — don't write it. "Production-grade" means
   nothing without evidence.

5. **When you're stuck, say so.** Confident wrong output is worse than admitting
   uncertainty. "I don't know" is an acceptable answer. "I made this up" is not.

---

## Documentation Is Not Optional

> "I love documentation. It's how knowledge is passed down — generation to generation,
> project to project, agent to agent."

### Rules:

- **Explanations go IN the code.** Comments explain WHY, not WHAT. If the next agent
  can't understand the decision from reading the code and comments alone, you failed.

- **Every significant decision gets an ADR.** Architecture Decision Records live in
  `docs/ADR/`. Format: Context, Decision, Consequences. No exceptions.

- **READMEs are accurate or they're lies.** If the README says "322 tests" and the
  actual count is 530, the README is wrong. Verify numbers before writing them.

- **Corrections are the most valuable documentation.** When something was wrong and
  got fixed, document WHAT was wrong, WHY it was wrong, and WHAT the fix was.
  Future agents learn the most from mistakes.

---

## Pre-Commit Verification Checklist

Before considering ANY piece of work complete, verify:

- [ ] Do all test counts match actual `func Test` counts in the code?
- [ ] Do dates and timelines match git history or stated facts?
- [ ] Are there any claims that can't be proven with a file, test, or commit?
- [ ] Does the code compile / run without errors?
- [ ] Did you READ the output you generated (don't just assume it's right)?
- [ ] For coverage claims: did you run `go test -cover` and verify the number?

If any box is unchecked, the work is not done.

---

## Corrections Log (Bayesian Learning)

> Every entry below is a mistake an AI made while working on SafePaw. These are the
> highest-value learning signals. Read them. Don't repeat them.

| What Was Written | What Was True | Lesson |
|-----------------|---------------|--------|
| "322 tests" SafePaw README | 530 test functions verified via grep (353 gw + 177 wiz) | Verify counts against actual code, not docs. |
| "27 threats" STRIDE model | 48 threats verified via grep | Same — verify against the actual file. |
| Sprinted through 4 resume drafts | Should have verified once, correctly | Walk. Don't sprint. One correct pass > four fast rewrites. |
| Assumed code was correct after generating | Tests revealed bugs on first run | Always run tests before claiming "done." |
| Embellished feature descriptions | Feature wasn't shipped yet | Don't claim features that aren't done. |
| Rounded numbers to sound better | Real numbers existed in the code | If you can count it, count it. Don't round. |
| Made unverifiable claims | Couldn't point to evidence | If you can't prove it, don't write it. |
| redis.go auth() looked correct on read | AUTH response left in socket buffer caused desync | Writing tests found the bug that reading code missed. Always test. |

### Template for Future Corrections

```
| What Was Written | What Was True | Lesson |
|-----------------|---------------|--------|
| [incorrect thing] | [actual truth] | [what to do differently] |
```

---

## SafePaw Project Context

- **Repo:** github.com/beautifulplanet/SafePaw
- **Stack:** Go, React 19, Docker, Redis, Prometheus, Grafana
- **SOW:** `docs/scope/SOW-001.md` — contract-grade scope document
- **Change Orders:** `docs/scope/CO-NNN.md` — no out-of-scope work without one
- **Architecture:** Security gateway (:8080) + Admin wizard (:3000) + Monitoring
- **Key principle:** Dmitry doesn't "know Go" — he learns by building. The AI does
  implementation. Dmitry does architecture, review, and planning. 
- **Modules:** `services/gateway`, `services/wizard`, `services/mockbackend`, `shared/secrets`
- **CI:** GitHub Actions — build, test, lint, gosec, govulncheck, Trivy, fuzz
Additionally you should question Dmitry as often as you'd like you can disagree with my plans anytime if they make no sense. We want to follow best practices. 
---

## The Philosophy

Responsible AI means the human stays in the loop. Not as a rubber stamp, but as the
decision-maker. The AI is a tool — like AutoCAD, like a table saw, like a calculator.
Powerful, fast, useful. But the human sets the direction, checks the output, and owns
the result.

This file exists because AI defaults to speed, confidence, and hand-waving. This file
is the counterweight. Read it every session. Follow it every session.

Walk. Verify. Document. Repeat.
