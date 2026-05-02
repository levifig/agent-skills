---
title: "SPEC/PLAN/TASKS artifact taxonomy — specs as PRDs, plans as strategy, tasks as agent-native"
captured: 2026-05-01T22:52:51Z
status: raw
tags: [pipeline, artifacts, refactor, taxonomy, linear]
related:
  - SPEC-034-refactor-deepen-grilling-glossary
  - SPEC-023-cli-backend-abstraction
---

# SPEC/PLAN/TASKS artifact taxonomy

## Nugget

Loaf's pipeline is `/shape → SPEC → /breakdown → tasks → /implement`. Specs are feature-shaped (problem statement, in/out scope, rabbit holes, risks, test conditions). For refactors, bug fixes, and short explorations, half those fields are vacuous. The pipeline implicitly assumes feature work; non-feature work either gets force-fit into spec ceremony or runs without ceremony at all (loose `/implement` invocations).

Reframe the artifact taxonomy:

- **SPEC** → PRD-shape: *what* and *why*. Problem statement, scope, success criteria, strategic alignment. Read by humans deciding whether to do the work.
- **PLAN** → implementation strategy: *how*. Approach, dependencies, sequencing, what survives, what gets deleted, alternatives rejected. Read by implementers before they start.
- **TASKS** → agent-native execution: the atomic work units, lived in the agent's task system (TodoWrite locally, Linear in team mode).

A SPEC may produce one or more PLANs. A PLAN may decompose into TASKS or be small enough to execute directly. A bug fix might skip SPEC entirely and start at PLAN. A refactor (per SPEC-034's `/refactor-deepen`) starts at PLAN, no SPEC needed.

## Problem/Opportunity

Concrete frictions today:

- `/shape` produces specs even when the work is a refactor or fix that doesn't need full PRD ceremony. Half the spec template is filler.
- `/breakdown` produces TASK files (`.agents/tasks/TASK-XXX-*.md`) regardless of context. Linear-native mode duplicates this into Linear sub-issues. There's no clean way to use only the agent's task system (TodoWrite for solo, Linear for teams) without dragging a separate task-file convention along.
- New artifact types (the PLAN introduced in SPEC-034) get bolted on without a coherent home. The artifact-taxonomy gap surfaces every time a new workflow doesn't fit "spec."
- Linear is *both* a tracker (where the team-lead persona's value lives) and an agent-native task system. The current SPEC-023 (CLI backend abstraction) handles the storage side but doesn't address the artifact-level question of what belongs in the tracker vs. in code.

## Initial Context

- **Triggered during SPEC-034 shaping** (port of Matt Pocock's `improve-codebase-architecture`). The deepening skill produces plans, not specs — surfaced the question "what artifact shape?" and exposed that Loaf has no answer for non-feature work.
- **SPEC-034 ships a minimal PLAN shape** (`.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md` with candidate / dependency category / proposed deepened module / what survives in tests / rejected alternatives). Plans use the temporal-record naming convention (same as sessions/ideas/drafts/councils) — write-once snapshots, not sequentially-numbered contracts. This is deliberately under-specified — first concrete example informs the abstraction, rather than designing the abstraction first.
- **Key tension:** the team-lead persona benefits from Linear (tracker, dashboards, blocking graphs, notifications). The solo persona benefits from local files (no external dependency, git-versioned). Any taxonomy must work in both modes — local-tasks and Linear-native (per ADR-011).
- **Possible decomposition into shapeable questions:**
  1. What goes in SPEC vs PLAN vs TASKS? (Define each artifact's load-bearing fields and what's allowed to be empty.)
  2. Which workflows produce which artifacts? (`/shape` produces SPEC; what produces PLAN — `/refactor-deepen`, `/architecture` for a multi-step decision, future `/debug-rca`, future `/design-it-twice`?)
  3. How does PLAN hand off to TASKS? (Auto-decompose, manual `/breakdown`, or skip directly to `/implement` for atomic plans?)
  4. How do TASKS reconcile with Linear sub-issues? (Are `.agents/tasks/TASK-XXX.md` files redundant in Linear-native mode? Should they be?)
  5. How does TodoWrite (the AI's native task system) fit in? (Today it's invisible to Loaf. Could it be the canonical "agent's task list" layer, with SPEC/PLAN supplying the human-readable layer?)

- **Open questions for shaping:**
  - Should plans live in `.agents/plans/` (current SPEC-034 plan) or in `.agents/specs/` with frontmatter discriminator? SPEC-034 chose new dir; the taxonomy spec may revisit.
  - Is "fix" a third sibling artifact (alongside spec/plan), or just a thin plan with extra fields?
  - Plans currently identify by filename timestamp (no `id:` frontmatter). Should they ever earn a stable canonical ID — e.g. for cross-referencing from a SPEC's `linked_plans:` field, or for promotion-to-SPEC tracking? Or is the filename-as-identity contract sufficient, mirroring how sessions and ideas are referenced?
  - What does this mean for `/loaf:reflect`? Does it review only specs, or specs+plans+fixes?

- **Sequencing:** wait for SPEC-034 to ship and produce its first real plan in the wild. Use that concrete example (and the friction it surfaces) to inform the taxonomy spec rather than designing in the abstract.
