# Workflow Convergence Research

## Purpose

This file materializes the relevant body of the multi-conversation workflow exploration into Git so `intent-exploration-foundation` remains understandable in a fresh checkout. It records the evidence and synthesis that shaped the Change without copying raw transcripts or making machine-local conversation handles authoritative.

## Source Provenance

- Primary source conversation: Codex desktop thread `019f62e6-88d2-7630-96ce-527652fd9a0b`, observed locally across multiple compactions. The opaque ID is provenance only; this document carries the portable context.
- Comparative inputs previously reviewed in that conversation: [OpenSpec OPSX](https://github.com/Fission-AI/OpenSpec/blob/main/docs/opsx.md), [Every Compound Engineering Plugin](https://github.com/everyinc/compound-engineering-plugin), [Matt Pocock Skills](https://github.com/mattpocock/skills), and NotebookLM notebook `9723f7a6-263d-426b-91f0-7fad35fd7567` (“Agile Product Management: From Shaping to Implementation”).
- Durable predecessor: `docs/changes/20260710-journal-reliability-foundation/change.md` and its journal-origin/deferral/lineage implementation evidence.
- Current-source audit: SQLite schema/migrations, journal deferral, trace/relationship resolution, Change CLI, workflow Skills, hooks, ADRs, README/strategy/architecture, routing evaluations, and generated target output as of commit `340ea967`.
- Accepted journal decisions and discoveries are cited in `change.md`; they provide stable semantic IDs while this file supplies the coherent body those entries alone cannot express.

The comparative sources are not normative dependencies. They helped expose useful workflow shapes and naming/routing patterns; Loaf's authority and continuity model is selected from project evidence and user decisions.

## What the Comparative Review Reinforced

### Explicit transitions beat artifact accumulation

OpenSpec/OPSX demonstrates the value of keeping a bounded change proposal and making workflow transitions legible. Loaf keeps that clarity but does not adopt a shadow copy of every durable artifact: the Change branch is the actual delta, canonical project documents are edited in place, and the Change records affected paths plus relevant local research/reports.

### Explore, Shape, and Plan are materially different kinds of thinking

The product-management research and dogfood discussion converged on a useful funnel. Explore expands and interrogates possibilities; Shape narrows one direction into a Product Contract; Plan defines the route from current to desired state. Combining all three creates either premature commitment during discovery or implementation detail inside the PRD. Loaf therefore targets separate `/explore`, `/shape`, and `/plan` workflows, even though the current checker still expects planning sections inside `change.md`.

### Techniques should not become competing lifecycle owners

The compound-engineering and Matt Pocock skill sets demonstrate the leverage of focused techniques, review loops, and memorable routing names. Loaf keeps Brainstorm, Scout, research, prototype, spike, council, and critique as techniques available where useful, while giving lifecycle ownership to a smaller public workflow. Unique user-facing names may improve routing later; `/explore` remains provisional until dogfood provides evidence.

### Plans need enough structure for independent execution without task-file bureaucracy

One-line units are too weak for reliable handoff, while one Markdown file per Task recreates allocation/status ceremony and duplicates tracker identity. The selected middle is `plan.md` with stable named unit slugs, explicit dependencies, acceptance criteria, rollback, and verification. A later Linear adapter may publish meaningful units as subissues without making tracker IDs intrinsic local identity.

### Learning belongs after delivery, not as speculative durable truth

Compound/reflection loops are valuable when they distill proven behavior. Change decisions are the delivery-scoped ledger; specs, knowledge, and ADRs describe durable reality after implementation/ship/reflect proves it. A Change may name expected durable effects, but it should not manufacture final documentation before the behavior exists.

## Current Repository Evidence

| Surface | Current behavior | Consequence |
|---------|------------------|-------------|
| `internal/state/migrations/0001_initial.sql` | Separate Ideas, Sparks, Brainstorms, Specs, Tasks, and generic relationships | No tracked Intent or resumable Exploration focal entity |
| `internal/state/migrations/0011_journal_origins_and_deferrals.sql` | Optional journal-origin envelope and `journal_deferrals` linking one decision to one Spark | Useful provenance/idempotency seam, but no substantial deferred body or Exploration relation |
| `internal/state/journal_defer.go` | Serializable operation-key write of a four-field journal decision and open Spark | Strong compound-write pattern; wrong long-term authority/projection |
| `internal/state/trace.go` | Entity support distributed through fixed lists/switches; relationships are polymorphic | New kinds require centralized registration and validated endpoints/types to avoid drift |
| `internal/cli/change.go` | `change init` writes `change.md` in the current checkout and prints a branch hint | Worktree/branch/PR mechanization remains a later Change |
| `content/skills/triage/SKILL.md` | Intake covers Sparks/Brainstorms/Ideas and “defer” may mean no operation | Cannot distinguish capture, tracked Intent, and deferred disposition |
| `content/skills/shape/SKILL.md` | Shape owns decomposition and planning inside `change.md` | Conflicts with the accepted Product Contract/`plan.md` separation |
| `content/skills/breakdown/SKILL.md` and Implement guidance | Public Spec/Task execution vocabulary remains | Terminal hard cut and Skill simplification are still required |
| ADR-011/013/016 and current architecture/strategy | Linear mode branching and SQLite/render authority reflect earlier models | Historical evidence remains; current guidance must be superseded/converged after implementation |
| `plugins/` and `dist/` | Tracked generated Skills repeat current source behavior | Every workflow change needs source/generated/install convergence, not source-only edits |

The known global `journal_search` divergence proves why derived views need parity contracts and why this Change must never infer that “no search result” means “no prior context.” Isolated migration tests and canonical recent reads remain the safe evidence path until separately consented repair.

## Selected Concept Taxonomy

| Concept | Meaning | Authority before tracker integration | Creates a worktree? |
|---------|---------|--------------------------------------|---------------------|
| Spark | Incidental thought worth preserving when it cannot be addressed immediately | SQLite | No |
| Idea | Explicit proposition retained for consideration | SQLite | No |
| Intent | Deliberately tracked direction worth revisiting, investigating, or delivering | SQLite; later published/adopted tracker issue when configured | No |
| Deferred | Append-only disposition of an Intent with immutable self-sufficient payload | SQLite | No |
| Exploration | Relational inquiry spanning checkpoints, conversations, evidence, Intents, and later Changes | SQLite, with relevant synthesis materialized into Git on Change promotion | No |
| Research | Evidence gathering for a known question; may support an Exploration or Change | SQLite relationship plus Git report/research when team/delivery relevant | No by itself |
| Change | Bounded approved delivery contract | Git `docs/changes/<date>-<slug>/`; tracker issue later mirrors collaborative work state | Yes, at approved materialization |
| Plan unit | Named self-contained implementation/review packet inside `plan.md` | Git; later meaningful units map to tracker subissues | Reuses the Change worktree |
| Handoff | Immutable continuation bookmark/snapshot | SQLite, related to the relevant Exploration/Intent/Change/unit | No |
| Wrap | Immutable conversation synthesis/report checkpoint | Journal in SQLite, related where useful | No |

Deferred work may arise during Explore, Shape, Plan, Implement, Reflect, Ship, or Release. Its discovery stage is provenance, not a separate defer mechanism. One Exploration may yield multiple Intent and multiple simultaneously shapeable Changes; choosing one first does not implicitly defer every other candidate.

## Authority Boundary

| Information | Local-only mode | Linear-enabled target after the coordination Change |
|-------------|-----------------|-----------------------------------------------------|
| Capture, Exploration, checkpoints, conversations/log refs, journal, wraps, handoffs, mappings/reconciliation evidence | SQLite | SQLite |
| Tracked Intent and deferred disposition | SQLite | Linear owns published collaborative state; SQLite retains local provenance, mappings, checkpoints, and reconciliation history |
| Change/plan/spec/ADR/knowledge/report/code bodies | Git | Git |
| Change and meaningful unit coordination state | Git/SQLite deterministic local view | Linear |
| Review, merge, publication evidence | PR/Git/release surfaces | PR/Git/release surfaces linked from Linear |
| Disposable scratch/cache | `.agents/tmp/` or OS cache when safe | Same |

SQLite is not a hidden Markdown repository. Git is not the right home for every conversation checkpoint. Linear is not the authority for durable project truth. The boundaries are complementary rather than a single universal source of truth.

## CLI and Skill Boundary

The accepted sentence is: **humans and Skills choose; the CLI deterministically proves and performs.**

The CLI owns validation, reads, explicit mutations, append-only persistence, idempotency, migrations, mappings, provider adapters, worktree/branch/PR mechanization, hooks, machine-readable output, and health checks. It may enforce invariants around an explicitly requested operation; it does not infer that operation from ambiguous prose.

Skills own interpretation, questions, recommendations, scope judgment, semantic classification, prose authoring, and deciding which CLI operation to request. A Skill should not branch on backend configuration, call provider MCP directly, maintain lifecycle status, or reproduce Git/SQLite/worktree mechanics.

Hooks are deterministic callers/gates over the same CLI surface. They may attach observable event/provenance data or enforce a machine-checkable invariant; they must not smuggle LLM judgment into a supposedly deterministic gate.

### Config-aware Loaf maintenance

Dogfooding the alpha.8 release exposed a related operator boundary. Upgrading the executable, refreshing installed harness adapters, reconciling project guidance/config, and migrating SQLite are separate mechanisms today; an agent needs one coherent way to determine which apply without embedding another version-specific shell recipe in every workflow Skill.

The shaping session produced direct failure evidence: Homebrew had installed alpha.8 and current `loaf` resolved through `PATH`, but the project-managed Codex safe-command guidance still required the retired alpha.6 Cellar executable. Both mandated journal writes failed before reaching Loaf because that path no longer existed, and the agent correctly refused to substitute an unauthorized bare command. This proves that package upgrade and managed project/harness reconciliation are independent and that stale command ownership must be diagnosed before a maintenance Skill attempts an operation.

The existing non-user-invocable `loaf-reference` Skill is the right operator layer. `.agents/loaf.json` supplies durable team-owned choices such as integration election, while executable provenance, installed targets, ownership digests, state readiness, and checkout alignment are machine-local observed facts. The Skill combines CLI-provided facts and actions, asks only for missing project choices, and verifies convergence. It does not infer installed harness intent from Git config, call a package manager, or replace deterministic target/config/state mechanics.

This requires complete non-mutating planning surfaces underneath it. `loaf doctor --json` must expose project-alignment facts without entering repair flow, and `loaf install --upgrade --dry-run --json` must describe target selection, owned effects, preserved conflicts, deprecations, consent requirements, and project-file reconciliation without writing. State diagnostics retain their backup-first migration actions. A top-level `loaf upgrade` and public `/maintain` remain deferred until this hidden protocol is dogfooded; adding another public Skill now would contradict the simplification goal.

## Target Skill Surface

Core public flow:

```text
/triage → /explore → /shape → /plan → /implement → /ship → /release → /reflect
```

`/research`, `/handoff`, `/wrap`, `/housekeeping`, and `/council` remain optional/supporting workflows. Idea and Spark are natural-language-triggered capture primitives routed through Triage. Brainstorm preserves its full divergent stance inside Explore. Scout, prototype, and spike are internal techniques usable from the stage that needs them. Defer is an Intent disposition, not a user-facing Skill. Breakdown, render, and merge do not survive the completed hard cut.

## Successor Decomposition

```text
journal-reliability-foundation
  → intent-exploration-foundation
  → change-native-execution-migration
  → linear-native-coordination
  → spec-conversion-and-guidance-sweep
```

The separation is deliberate:

- This Change establishes local relational memory and deterministic capture/resumption without Git workspace or tracker dependencies.
- Change-native execution can then promote Intent, allocate the worktree before durable writes, create `change.md`/`plan.md`, open a draft PR, and run local implementation/ship semantics.
- Linear coordination can bind stable local concepts to remote collaborative authority without defining the concepts or leaking provider behavior into Skills.
- The terminal sweep can migrate/remove legacy public authority and converge every guidance/generated/install surface using landed evidence rather than speculative compatibility.

Cross-worktree activity scanning remains separate from this serial chain unless later dogfood proves it blocks correctness. The journal plus explicit workflow-stage scans are sufficient for the current foundation.

## Research Conclusion

The workflow does not need more artifact types or more public Skills. It needs a precise relational bridge between fleeting capture and bounded delivery, plus an equally precise separation between judgment and mechanism. Intent and Exploration provide that bridge only if they remain append-only operational facts rather than mutable lifecycle objects. The five-node lineage lets Loaf prove that foundation locally, then mechanize Git delivery, then add Linear collaboration, and only then remove the legacy surfaces comprehensively.
