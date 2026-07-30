---
id: ADR-025
title: "Entry stage — pitch as human problem-discovery, authored briefs, landing matrix, and capture promotion"
status: Accepted
date: 2026-07-30
supersedes: null
superseded_by: null
amends:
  - ADR-022
related:
  - ADR-020
  - ADR-022
  - ADR-023
---

# ADR-025: Entry stage — pitch and authored briefs

## Context

ADR-022 gave `brief.md` a seat in the role-named set — "the pre-shaping ask" — but no authoring ceremony. Capture seeded a pasted spark; `/shape` absorbed raw vision and double-duty interviewed problem and solution; project-scale discovery lived only in bootstrap; explore and brainstorm still answered "where do I start?" as user-facing surfaces. The change-work-model Cut already refused a `pitch.md` document and relation fields; what was missing was the ceremony that makes the brief/shape boundary semantic (problem-space vs solution-space) rather than mechanical (pasted vs authored).

This decision records what shipped on the pitch-entrypoint branch: the pitch skill, shared brief skeleton, capture-folder promotion, bootstrap series-prep, and flow-seam demotions — distilled from landed behavior, not aspiration.

## Decision

**Pitch is the human entry ceremony.** `/pitch` grills the problem register (problem, who has it, current alternatives / competitive landscape, value proposition, constraints) one question at a time, recommendation-first, and authors a brief at the matching scale: change-scale `brief.md` after `loaf change init <slug> --brief`, or project-scale `docs/BRIEF.md` with `source: pitch`. Agents never initiate a pitch — Claude Code enforces `disable-model-invocation: true`; every built target carries the behavioral rule "Agents never initiate a pitch." Agent legwork *inside* a human-opened pitch (scans, file writes the skill directs) is delegated by the skill.

**The brief is the pitch artifact — no new document.** Shared problem-space skeleton at both scales: problem statement, who has it, current alternatives, value proposition, constraints, sequencing and relationships (prose), sources and research links, open questions. Three surfaces implement it: shape's `templates/brief.md` (change-scale authority), the byte-identical Go embed, and bootstrap's project-scale brief template. Solution design never lands in a brief.

**Brief semantics amend ADR-022.** Authored at pitch (or seeded at capture); may accrete parked problem-space concepts while the change is captured; freezes when `shape.md` exists; superseded by `shape.md`; never mechanically load-bearing. Brief-only still derives `captured` regardless of richness — no pitched-state machinery, no prose critic in `loaf change check`.

**Shape owns solution-space.** With a brief present, `/shape` restates the problem from it and grills how, boundaries, and verification. Without a brief, full narrowing remains — pitch is optional at change scale, never a gate. Capture-only folders promote through ordinary `loaf change init <slug>` (no `--brief`): brief and `change.json` values preserved verbatim; `shape.md` and seeded `tasks/` instantiated; fully materialized folders still reject as duplicates. Skills never hand-copy scaffold templates.

**Landing matrix.** Park and series-prep share one matrix: targeted captures (`target_release` set) commit docs-only on the default branch (the shipped promise-carrier exception); untargeted captures commit docs-only on the slug branch or stay intake — never on main. One docs-only commit per capture, never a batch. Every landing validates with explicit-path `loaf change check <folder>` plus a direct `change.json` read-back of intended target presence/absence before the commit. Pitch and bootstrap prepare commits; push and PR stay human.

**Project scale.** Pitched `docs/BRIEF.md` feeds `/bootstrap` as discovery-already-done (`source: pitch` → gap interview only). Bootstrap series-prep mints the initial target-version series as captured changes from BRIEF concepts, each user-confirmed, each landed per the matrix; concepts without even a coarse target stay BRIEF lines, sparks, or Intents.

**Explore and brainstorm demote to agent techniques.** Explore is `user-invocable: false` with description routing user entry intent to `/pitch`; brainstorm was already non-invocable and reroutes the same way. Exploration durable records and the four-field checkpoint contract stay intact — who reaches for them changes. Deep rewrite of those skills is deferred.

**Discovery, not registration.** Skills are auto-discovered from `content/skills/` at build time. Pitch ships no hooks; `config/hooks.yaml` holds hook instances with an owning skill, never a skill roster. Journal self-logging is skill text.

**No new CLI verbs and no new machine state.** No `loaf pitch`, no derived "pitched" state. Promotion is idempotent completion of the existing `init` verb.

## Consequences

The Loaf Flow reads as one front door: pitch → shape → implement → ship → release, at both scales. Briefs become reviewable problem documents; shape sessions stop re-grilling the problem when a brief exists; greenfield projects and new changes share one skeleton. Cost accepted: pitch adds ceremony that small well-understood changes may skip (Decision: optional at change scale; triage retains direct hand-to-shape). Dogfood of the pitch→shape seam (H4) remains a human-invoked checkpoint after the brief-aware shape skill lands.

ADR-022's anatomy, cohorts, commitments, and coexistence rules stand; only brief mutability and authorship semantics are refined here. Operating account: [loaf-flow.md](../knowledge/loaf-flow.md) and [work-model.md](../knowledge/work-model.md).

Provenance: `docs/changes/20260728-pitch-entrypoint/` shape.md Decisions 1–12; landed commits on `pitch-entrypoint` for brief contract, capture promotion, pitch skill, bootstrap series-prep, flow seams, and guidance sweep.
