---
title: "Shape-spawned ideas harness — discipline + linkage for adjacent concepts surfaced during shaping"
captured: 2026-05-01T22:53:35Z
status: raw
tags: [shape, idea, triage, reflect, workflow, linkage]
related:
  - SPEC-034-refactor-deepen-grilling-glossary
  - 20260501-225251-spec-plan-tasks-artifact-taxonomy
---

# Shape-spawned ideas harness

## Nugget

When `/shape` (or `/architecture`, or any deliberation skill) interviews the user, adjacent concepts and tensions frequently surface that don't belong in the current spec but are too important to lose. Today there is no discipline or mechanism to capture them durably with a clear path back to the spec that birthed them and forward to eventual exploration. Add three pieces:

1. **Discipline rule** in `/shape`, `/architecture`, and `/refactor-deepen`: when an in-conversation tension is out-of-scope for the current artifact, capture it as `/idea` immediately with `related: SPEC-NNN` (or the plan filename `YYYYMMDD-HHMMSS-{slug}`).
2. **Forward link** in spec/plan frontmatter: `spawned_ideas: [...]` listing the idea IDs surfaced during shaping/grilling. The artifact knows what threads it left open.
3. **Promotion gate** in `/triage` and `/reflect`: ideas with `blocked_by: SPEC-NNN` are surfaced separately ("post-implementation review queue") rather than treated as ready-to-shape. `/reflect` reviews `spawned_ideas` of the just-shipped spec and decides per-idea: promote to spec, defer, or close as no-longer-relevant.

## Problem/Opportunity

Concrete failure modes today:

- During SPEC-034 shaping, three adjacent concepts surfaced (artifact taxonomy, plan-handoff mechanics, glossary mutation policy per skill). None were captured durably until I (the user) asked. If I hadn't, all three would have died with the conversation or lived only inside the spec's prose — coupled to SPEC-034's fate, undiscoverable from a triage queue.
- `.agents/ideas/` and `/idea` exist, but `/shape` doesn't tell the orchestrator "capture this now" when an off-topic concept emerges. The discipline is implicit and easily skipped.
- Ideas with `related: SPEC-NNN` exist (the monorepo-version-files idea references SPEC-031), but the spec doesn't know what ideas it spawned. There's no inverse pointer. `/reflect` can't review "what threads did this spec leave open" because the spec doesn't track them.
- `/triage` treats all raw ideas as equally ready to shape. An idea blocked on SPEC-034 shipping shouldn't appear in the same queue as a fresh independent idea — they need different treatment.

## Initial Context

- **Mechanism exists; discipline doesn't.** `/idea`, `.agents/ideas/`, `related:` field, `/triage`, `/reflect` are all in place. The gap is the workflow that connects them across a shaping session.
- **Pattern is generic.** Not specific to refactor work. Affects every shaping session, every architecture interview, every deep deliberation. A small spec applied across `/shape`, `/architecture`, `/refactor-deepen`, and any future deliberation skill.
- **Three concrete changes, narrow surface:**
  - `templates/spec.md` and `templates/plan.md` (when defined): add `spawned_ideas: [list-of-idea-ids]` frontmatter field.
  - `templates/idea.md`: add optional `blocked_by:` frontmatter field accepting a SPEC ID (`SPEC-NNN`) or a plan filename (`YYYYMMDD-HHMMSS-{slug}`), distinct from `related:`.
  - `/shape`, `/architecture`, `/refactor-deepen` SKILL.md: add Critical Rule "if an out-of-scope concept surfaces, capture as `/idea` with `blocked_by: <current-artifact>` before continuing" plus a Step in the process for it.
  - `/triage` SKILL.md: surface blocked ideas in a separate "post-implementation review" section, not the main queue.
  - `/reflect` SKILL.md: at start, read `spawned_ideas` of the spec being reflected on; for each, prompt: promote / defer / close.
- **Inspired by the pattern that prompted it.** SPEC-034's shaping itself surfaced this gap; the gap got captured (this idea); the harness will eventually formalize the loop. Self-instantiating.

- **Open questions for shaping:**
  - Should `blocked_by` be a single ID or a list? (One spec spawning many ideas is one-to-many; one idea blocked on multiple specs is rare but possible.)
  - Does `/reflect` reading `spawned_ideas` belong in the existing `/reflect` skill, or does it warrant its own sub-step / sub-skill?
  - What's the right format for capturing during a shape session — full `/idea` interview, or a lightweight inline-capture that gets promoted later?
  - Should `/shape` automatically write the `spawned_ideas` field as ideas are captured during the session, or is that a manual update before final spec write?
  - When ideas are captured mid-shape, should the orchestrator pause to interview for them, or capture a stub and continue (deferring the interview to triage)?

- **Sequencing:** sized as a small, contained workflow refinement. Could ship after SPEC-034 (informed by one round of manual practice during this and adjacent sessions), or in parallel if scope stays narrow. Independent of the artifact-taxonomy idea — the harness works regardless of whether SPECs become PRDs.
