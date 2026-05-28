---
id: SPEC-041
title: Brief-shaped specs and optional implementation plans
source: direct -- conversation about NotebookLM research, SPEC-040 SQLite state, and Loaf artifact taxonomy
created: '2026-05-26T16:11:55Z'
status: drafting
branch: feat/sqlite-operational-state
source_sessions:
  - id: 019e424f-01bb-7962-b830-d5aac0245fb7
    role: shaped
    note: NotebookLM-informed brief/spec taxonomy shaping
related_specs:
  - SPEC-034
  - SPEC-037
  - SPEC-038
  - SPEC-040
---

# SPEC-041: Brief-shaped specs and optional implementation plans

## Problem Statement

Loaf's current `/shape -> SPEC -> /breakdown -> TASKS` model treats every shaped artifact like a feature-sized requirements document. That creates ceremony for fixes, refactors, research, and small side projects, while still failing to encode the exact constraints agents need: context, boundaries, appetite, deliverable shape, and verification.

Recent design work moved back toward keeping `SPEC-*` as the internal unit, but with changed semantics: a spec is not immutable public truth, not a PRD by default, and not necessarily an implementation plan. SPEC-040 also moves operational state into SQLite, which means Markdown no longer has to carry lifecycle, relationship, alias, and export state by itself.

Loaf needs to redefine what a spec is and when a separate plan exists.

## Strategic Alignment

- **Vision:** Preserves Loaf's grounding and auditability while reducing ceremony.
- **Personas:** Solo developers get lighter shaping for small work. Team leads keep reviewable intent, task boundaries, and external artifact hygiene.
- **Architecture:** Reinforces the CLI/state boundary from SPEC-040: SQLite owns operational truth; Markdown is authored prose or generated export.

## Solution Direction

Redefine a Loaf spec as a shaped internal work artifact:

> A spec specifies the desired outcome, context, constraints, appetite, boundaries, and verification bar. It does not necessarily specify the implementation.

`/shape` becomes an artifact router. It may produce:

- no durable artifact, for tiny obvious work;
- a brief-shaped `SPEC-*`, for most shaped work;
- a `PLAN-*`, when implementation strategy is the main artifact;
- a `SPEC-*` plus one or more `PLAN-*` artifacts, when intent and implementation strategy both need durable review.

A plan is optional and contextual:

> A plan captures implementation strategy, sequencing, rejected alternatives, affected surfaces, and verification strategy when those details need separate review or reuse.

SQLite-backed state records relationships among shaping drafts, specs, plans, tasks, tags, bundles, exports, and external mappings. Markdown specs/plans remain readable projections or authored bodies.

## Scope

### In Scope

- Update `/shape` guidance so it routes to the smallest useful artifact.
- Define brief-shaped spec semantics and minimum fields.
- Define when `PLAN-*` is required, optional, or unnecessary.
- Define how SQLite state tracks spec/plan/task lineage and export metadata.
- Update `/breakdown` expectations so it can decompose from a spec, a plan, or both.
- Keep `SPEC-*` as the internal identity; do not introduce `WORK-*`.

### Out of Scope

- Implementing the full SQLite store from SPEC-040.
- Renaming existing specs.
- Rewriting all historical specs into the new shape.
- Making generated Markdown the source of truth.
- Exposing `SPEC-*`, `TASK-*`, `.agents/...`, tracks, or phases in external/public artifacts.
- Replacing ADRs or knowledge files.

### Rabbit Holes

- Designing a perfect universal artifact ontology.
- Making every `/shape` output both a spec and a plan.
- Treating "brief" as a new ID namespace.
- Reintroducing PRD ceremony under a lighter name.
- Letting task files duplicate plans.

### No-Gos

- Do not require a separate plan unless implementation strategy is genuinely load-bearing.
- Do not make users choose a brief type up front; infer intent from the conversation.
- Do not let external artifacts leak Loaf-local IDs.
- Do not treat specs as immutable truth after implementation; durable truth is extracted into code, tests, docs, KB, changelog, release notes, and rare ADRs.

## Artifact Rules

A brief-shaped spec must contain:

- context;
- problem or opportunity;
- desired outcome;
- constraints;
- out of scope / no-gos;
- appetite;
- unknowns;
- verification / stop condition.

A plan is created when any are true:

- sequencing is non-obvious;
- multiple implementation approaches are plausible;
- the work crosses shared contracts or risky modules;
- rejected alternatives need to be preserved;
- several tasks need a common implementation strategy.

Tasks remain execution slices with concrete acceptance criteria and verification commands.

## Open Questions

- [ ] Should brief-shaped specs add explicit frontmatter such as `shape: brief`, or should shape be inferred from sections?
- [ ] Should `PLAN-*` become a first-class command family before or after SPEC-040 SQLite state lands?
- [ ] Should `/shape` be allowed to emit only a `PLAN-*` for refactors and bug investigations?
- [ ] How should existing `SPEC-*` lifecycle states map to SQLite-backed state transitions?

## Test Conditions

- [ ] `/shape` guidance states that it does not always create both a spec and a plan.
- [ ] `/shape` defines spec as shaped intent/constraints, not implementation blueprint.
- [ ] `/shape` includes decision rules for no artifact, spec-only, plan-only, and spec-plus-plan outputs.
- [ ] `/breakdown` can explain whether it is decomposing from a spec, a plan, or both.
- [ ] Documentation preserves `SPEC-*` and rejects `WORK-*`.
- [ ] External artifact guidance continues to forbid Loaf-local IDs in public surfaces.
- [ ] SQLite state model can represent shaping draft -> spec -> optional plan -> tasks -> exports lineage.

## Priority Order

1. **Source contract** -- update source skills and architecture docs. Go/no-go: the new artifact rules are internally consistent.
2. **Command semantics** -- align `loaf spec`, `loaf task`, and future `loaf plan` command expectations. Go/no-go: no command assumes every spec has a plan.
3. **SQLite integration** -- map the model into SPEC-040 state tables and relationships. Go/no-go: lineage and exports are queryable without Markdown grep.
4. **Generated exports** -- produce Markdown snapshots that are reviewable but not canonical state.
