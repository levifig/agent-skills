---
topics:
  - workflow
  - pitch
  - changes
  - bootstrap
covers:
  - content/skills/pitch/**/*
  - content/skills/shape/**/*
  - content/skills/triage/SKILL.md
  - content/skills/bootstrap/**/*
  - content/skills/explore/**/*
  - content/skills/brainstorm/**/*
  - content/skills/implement/**/*
  - content/skills/ship/**/*
  - content/skills/release/**/*
consumers:
  - implementer
  - reviewer
  - researcher
last_reviewed: '2026-07-30'
---

# The Loaf Flow

## Contents
- The pipeline
- Two scales
- Problem-space vs solution-space
- Where explore and brainstorm sit
- Capture, park, and promote
- Operating links

How work enters and leaves Loaf after the pitch-entrypoint change. This is the ceremony view; [work-model.md](work-model.md) is the anatomy and evidence view. They describe one pipeline, not two. Rationale: [ADR-025](../decisions/ADR-025-entry-stage-pitch.md) (entry stage; amends brief semantics in [ADR-022](../decisions/ADR-022-change-anatomy-and-release-cohorts.md)).

## The pipeline

```
pitch → shape → implement → ship → release
```

| Stage | Owns | Ships as |
|-------|------|----------|
| **pitch** | Problem discovery; authors the brief | Human ceremony only (`/pitch`); agents never open one |
| **shape** | Solution-space contract + task packets | `/shape`; promotes capture-only folders in place |
| **implement** | Task commits that flip packet checkboxes | `/implement` orchestrating implementers |
| **ship** | Land the change PR | `/ship` |
| **release** | Publish a version from already-landed work; gate stable cuts on cohort receipts | `/release` |

Reflect distills durable outputs on the branch before merge. The project journal carries decisions across every stage. Pitch is the recommended front door for raw concepts; it is **optional** at change scale — shape still runs full narrowing when no brief exists.

## Two scales

| Scale | When | Pitch output | Next |
|-------|------|--------------|------|
| **Change** | Existing project + a concept | `loaf change init <slug> --brief` then authored `brief.md` | Shape-now (slug branch, hand to `/shape`) or park per landing matrix |
| **Project** | Greenfield / empty or minimal directory | `docs/BRIEF.md` with `source: pitch` | Hand to `/bootstrap` (gap interview + series-prep of captured changes) |

Both scales share one problem-space brief skeleton (problem, who, alternatives, value, constraints, sequencing prose, sources, open questions). Change-scale evidence lands under the change's `research/`; project-scale evidence is inline source links in the BRIEF.

## Problem-space vs solution-space

- **Pitch** grills what, who, and why-valuable. A brief that reads like a pseudo-shape is a failure.
- **Shape** grills how, boundaries, decomposition, and verification. With a brief present it restates the problem from the brief and does not re-discover it; without a brief it narrows from the raw ask as before.
- The brief may **accrete** parked problem-space sentences until shaping starts; it **freezes** when `shape.md` exists and is then archeological. It is never mechanically load-bearing — brief-only folders still derive `captured` regardless of richness.

## Where explore and brainstorm sit

They are **agent-side techniques**, not user slash front doors.

- **Explore** remains the durable Exploration / checkpoint / Intent machinery; its Claude Code sidecar is `user-invocable: false`, and its description routes human "explore this" intent to `/pitch`. Agents reach for it from inside pitch (or other work) when the direction is genuinely undecided.
- **Brainstorm** is the divergent stance consumed by explore (and shape's internal techniques); already non-invocable; description routes user entry intent to `/pitch`.

Triage dispositions: items needing problem discovery hand to `/pitch`; well-understood directions hand to `/shape`; "explore" means the agent technique after (or instead of) forcing a false brief.

## Capture, park, and promote

**Landing matrix** (park and bootstrap series-prep):

| Intent | Where it lands | `target_release` |
|--------|----------------|------------------|
| Shape now | Slug branch; hand to `/shape` (no park-commit first) | Optional stamp when known |
| Park targeted | Docs-only commit on the **default** branch (promise-carrier exception) | Present |
| Park untargeted | Docs-only on the **slug** branch, or stay intake (Intent/spark) | Absent — never on main |

Every capture landing: explicit-path `loaf change check <folder>` (zero violations, captured state) plus direct `change.json` read-back of the intended target, then one docs-only commit per capture. Pitch and bootstrap prepare commits; they never push or open PRs.

**Promotion.** A capture-only folder (`change.json` + `brief.md`) becomes shaped when `/shape` (or the human) runs ordinary `loaf change init <slug>`: brief and metadata preserved verbatim; `shape.md` and seeded `tasks/` materialize; skills never copy templates by hand. Fully materialized folders still reject as duplicates.

**Series-prep.** After bootstrap populates operating documents from a pitched BRIEF, it enumerates initial work as captured changes (user-confirmed, one commit each, coarse target when ready). Concepts without a target binding stay BRIEF lines, sparks, or Intents — not minted Changes.

## Operating links

| Topic | Where |
|-------|--------|
| Anatomy, task evidence, cohorts, receipts | [work-model.md](work-model.md) |
| CLI surfaces and capture `init` promotion | [task-system.md](task-system.md) |
| Term definitions | [glossary.md](glossary.md) |
| Entry-stage decision | [ADR-025](../decisions/ADR-025-entry-stage-pitch.md) |
| Change anatomy (brief semantics amended) | [ADR-022](../decisions/ADR-022-change-anatomy-and-release-cohorts.md) |
