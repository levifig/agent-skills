---
title: "Triage skill — problem-first decomposition before shaping"
captured: 2026-04-02T16:28:57Z
status: raw
tags: [skill, product-management, shaping, problem-framing]
related: [shape, brainstorm, strategy]
origin: https://x.com/karrisaarinen/status/2039727222374981983
---

# Product Thinking Skill — Problem-First Analysis Before Shaping

## Nugget

Karri Saarinen (Linear CEO) shared a Claude skill that enforces product thinking
discipline: treat every request as a signal about an unmet need, not an instruction
to implement literally. Decompose the underlying problem before proposing solutions.
Push back on shallow or solution-shaped requests.

## Core Principles (from Karri's skill)

- Identify the **underlying problem** instead of accepting proposed solutions at face value
- Treat customer requests as **signals about unmet needs**, not literal instructions
- Before suggesting work, evaluate: who is affected, confidence level, what happens if we do nothing, whether the fix is local for a broader problem
- **Separate problem framing from solution design** — restate problem first, then propose 1-3 directions with tradeoffs
- Push back on overly solution-shaped requests
- Prefer strong opinions informed by customer reality over counting requests

## Direction: `/loaf:triage` skill

A user-invocable workflow skill for **analytical processing of external signals** —
feature requests, customer feedback, tickets, stakeholder asks. Distinct from `idea`
(which captures your own sparks) — triage processes what's coming *in*.

```
[external signals] → triage (analyze/validate) → idea or shape
```

### Two operating modes

**Validate mode** — "I think we need X. What are people actually asking for?"
Pull related signals (Linear issues, feedback), find patterns across requests,
confirm or challenge the hypothesis. Output: validated concept or counter-evidence.

**Batch mode** — "What's in the inbox? What patterns do you see?"
Scan incoming requests, cluster by underlying need, surface what's real vs. noise,
rank by impact. Output: prioritized list with underlying needs identified.

### Analytical lens (from Karri's principles)

The Karri principles are the *how*, not the *what*:
- Treat requests as signals about unmet needs, not instructions
- Infer what's unsaid, look for patterns across feedback
- Evaluate: who's affected, confidence, what if we do nothing, local fix vs. broader problem
- Push back on solution-shaped requests — find the cleaner abstraction
- Strong opinions informed by customer reality, not request counting

### Output format

Structured assessment per concept/cluster:
- Underlying need (one sentence)
- Why the literal request may be insufficient
- Recommended direction
- Open questions / what needs validation
- Verdict: shape / park / reject / needs-research

## Constraints

- Loaf is a developer framework, not a PM tool — the skill should augment technical
  decision-making, not replicate a PM workflow
- The structured response format (h2 title, underlying need, recommended direction,
  open questions) is valuable and could become a template
- Linear-specific framing should be generalized (not everyone uses Linear for tracking)

## Next Steps

- `/loaf:shape` to spec out the triage skill
