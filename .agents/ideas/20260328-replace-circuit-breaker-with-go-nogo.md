---
captured: 2026-03-28T15:32:34Z
status: absorbed
absorbed_by: SPEC-025
tags: [methodology, shape-up, specs, agentic-development]
related: [SPEC-014]
---

# Replace Circuit Breaker with Priority Ordering + Go/No-Go Gates

## Idea

Circuit breakers are a human-energy management tool from Shape Up — they answer "what
do we ship if the team runs out of steam at 75%?" Agents don't run out of steam. The
concept doesn't map to agentic development where the scarce resource is scope clarity
and context window, not motivation.

Replace with two concepts:
- **Priority ordering** — tracks ship in explicit order (A → B → C → D)
- **Go/no-go gates** — binary R checks between tracks (verify R3-R6 pass before starting Track B)

The binary test conditions (Rs) already serve as the gate criteria — no separate
circuit breaker section needed.

## Scope

Cross-cutting change to the spec/planning methodology. Touches 7 files:
- `content/skills/shape/templates/spec.md` (spec template section)
- `content/skills/shape/SKILL.md` (shaping process)
- `content/skills/orchestration/references/specs.md` (spec format definition)
- `content/skills/orchestration/references/planning.md` (planning references)
- `content/templates/plan.md` (plan template section)
- `content/skills/orchestration/SKILL.md` (orchestration references)
- `content/skills/breakdown/SKILL.md` (breakdown alignment reference)

Note: `production-readiness.md` and `foundations-validate-changelog.sh` reference the
software pattern "circuit breaker" (not Shape Up) — leave those untouched.

## Source

Discussion during SPEC-014 reshape session.
