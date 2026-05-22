# Brief-First Workflow Handoff

Created: 2026-05-21 18:11
Repo: `/Users/levifig/Code/levifig/projects/loaf`
Suggested branch: `feat/brief-first-workflow`

## Purpose

This handoff package captures the current conversation about replacing Loaf's spec-first workflow with a brief-first, task-contract workflow. It is intended to seed a new branch and a new conversation without losing the reasoning, prior-session context, or open decisions.

## Conversation References

- Current conversation / rollout: `/Users/levifig/.config/codex/sessions/2026/05/21/rollout-2026-05-21T01-43-06-019e47fc-6f8b-7681-922e-c61a4b86a32f.jsonl`
- Prior related session: `019e3fd3-1058-75a1-84d0-03b05be845b7`
- Prior rollout summary: `/Users/levifig/.config/codex/memories/rollout_summaries/2026-05-19T10-40-57-lRkP-loaf_release_prep_and_state_management_discussion.md`
- Prior rollout JSONL: `/Users/levifig/.config/codex/sessions/2026/05/19/rollout-2026-05-19T11-40-57-019e3fd3-1058-75a1-84d0-03b05be845b7.jsonl`
- User's public thread that triggered part of the framing: `https://x.com/levifig/status/2057067449099452736`
- External comparison reference: `https://github.com/Fission-AI/OpenSpec`

## Core Thesis

Loaf should move from:

```text
idea / spark / brainstorm -> /shape -> spec -> /breakdown -> tasks
```

to:

```text
idea / spark / brainstorm -> /shape -> brief -> /breakdown -> task(s)
```

The major change is not just renaming `spec` to `brief`. The model changes the default planning artifact:

- `Brief` is the normal shaped artifact.
- `Spec` is an optional escalation artifact for precision, contracts, APIs, migrations, product semantics, or high-stakes alignment.
- `PRD` is an optional product/stakeholder alignment artifact.
- `ADR` is an optional long-lived architectural decision record, usually created or updated after evidence exists.
- `Task` is the mandatory execution contract produced by `/breakdown`.

## Agreed Direction So Far

### Briefs

Briefs are the default output of `/shape`.

Briefs should be lightweight but sufficient to guide `/breakdown`. They should capture the current intent and strategy without pretending the team already knows everything.

Brief types should be inferred unless the user specifies one. Likely initial types:

- `feature`
- `bug`
- `refactor`
- `docs`
- `ops`
- `research`
- `chore`

Brief type should influence:

- changelog category
- commit-message bias
- required fields/questions during `/shape`
- verification expectations
- whether a spec/ADR/PRD escalation is likely

### Breakdown

Every brief should go through `/breakdown`.

Important nuance: `/breakdown` does not always mean splitting into many tasks. It means normalizing the brief into one or more executable task contracts. A small brief can produce exactly one task.

### Tasks

Tasks should be richer than an OpenSpec checklist, but subtasks should usually not be first-class records.

An atomic task should contain enough guidance for an implement-review loop to run autonomously:

- required end state
- acceptance criteria
- relevant context from the brief, docs, PKB, instructions, and file hints
- implementation guidance and non-goals
- internal checklist of steps
- verification command(s) or review conditions
- delivery notes for changelog/commit/release implications

Subtasks are likely over-granular as durable artifacts. Use checklist items inside the task unless the item needs independent lifecycle, ownership, review, or dependency tracking.

### Backend Rules

Task backend remains exclusive:

- If Linear is active, tasks are Linear-native work items/sub-issues. No local `.agents/tasks/TASK-*.md` and no meaningful `.agents/TASKS.json` state.
- If local mode is active, tasks use Loaf's local backend.
- Future backends should be first-class citizens, not sync targets.

This direction builds on the earlier conclusion that "Linear is optional, but task backends are exclusive."

## Prior-Session Design Constraint

The prior session found that Markdown is currently acting as both human artifact and queryable state database. That is the source of many closure/triage problems.

Relevant prior conclusion:

```text
SQLite / relational state: logs, sparks, ideas, tasks, statuses, relationships, provenance, resolution state, source links, timestamps, branch/session associations.
Markdown: human-authored durable documents such as specs, ADRs, reports, writeups, changelog prose.
Generated reports: session reports, PR audit packets, release readiness, triage closure reports.
```

This matters for the brief-first workflow because briefs and task contracts should be considered product/workflow artifacts, but lifecycle and closure state should eventually live in structured state, not scattered frontmatter and grep conventions.

## Comparison Notes

### Spec vs PRD vs Epic

- PRD: product intent, user outcome, stakeholder alignment.
- Epic: delivery tracking container, roadmap grouping, ownership/dependencies/progress.
- Spec: implementation-relevant behavioral agreement, constraints, contracts, edge cases, verification.
- Brief: default agent-facing shaped artifact: intent, current understanding, strategy, boundaries, verification, and task handoff.

### OpenSpec

OpenSpec is a useful comparison because it has lightweight artifact graphs and checklist-based tasks, but it remains spec/change-folder centered.

Loaf should borrow:

- lightweight artifact graph
- explicit change/task artifacts
- iterative, non-rigid workflow framing

Loaf should avoid:

- making specs the central source of truth for all work
- treating checklist tasks as the only canonical execution model
- duplicating backend-native task state into local files

## Current Repo Surfaces Likely Affected

This is expected to touch a lot of Loaf's skill/template language. Do not start with a broad rewrite without first shaping the transition.

Likely affected source areas:

- `content/skills/idea/SKILL.md`
  - currently says ideas become specs.
- `content/skills/triage/SKILL.md`
  - currently routes raw ideas toward `/shape` as spec creation.
- `content/skills/shape/SKILL.md`
  - currently defines shaping as creating `SPEC-*`.
- `content/skills/shape/templates/spec.md`
  - may need a new brief template or split between brief and spec.
- `content/skills/breakdown/SKILL.md`
  - currently expects a `SPEC-*` input and updates spec status.
- `content/skills/implement/SKILL.md`
  - currently routes `SPEC-*`, `TASK-*`, and Linear issues; should understand brief -> tasks flow.
- `content/skills/orchestration/references/product-development.md`
  - currently encodes `/shape -> SPEC -> /breakdown`.
- `content/skills/orchestration/references/specs.md`
  - should become spec-specific, not general shaping doctrine.
- `content/skills/orchestration/references/local-tasks.md`
  - task contract shape likely needs updating.
- `content/skills/reflect/SKILL.md`
  - should reflect from shipped briefs/tasks/sessions, not only completed specs.
- `docs/ARCHITECTURE.md`
  - current diagram still shows `/shape -> SPEC file -> /breakdown`.
- `docs/STRATEGY.md`
  - has artifact-taxonomy tension already; should be updated only after implementation proves the model.
- `.agents/ideas/20260501-225251-spec-plan-tasks-artifact-taxonomy.md`
  - older idea that should be revisited and likely superseded by this brief-first model.

## Suggested First Branch Scope

Suggested branch: `feat/brief-first-workflow`

Recommended first pass should be a shape/design pass, not immediate full implementation.

Proposed branch goals:

1. Define the artifact taxonomy:
   - brief
   - spec
   - PRD
   - ADR
   - task
   - epic/tracker container
2. Define brief lifecycle and fields.
3. Define inferred brief type behavior.
4. Define `/shape` contract: shape creates briefs by default.
5. Define `/breakdown` contract: every brief becomes one or more task contracts.
6. Decide what remains Markdown now versus what is deferred to structured state / SQLite.
7. Produce a staged migration plan for skills/templates/docs.

Avoid immediately touching every generated `dist/` and `plugins/` artifact until the source docs are stable.

## Open Questions

- Should briefs get stable IDs (`BRIEF-001`) or timestamp filenames like sessions/plans/ideas?
- Should brief type be frontmatter (`type: feature`) with inferred default, or CLI metadata in future structured state?
- Should `/shape` create a brief file immediately, or present it for approval like current specs?
- Should `/breakdown` accept only brief IDs, or also free text / Linear issues / existing specs?
- What is the exact rule for when a brief escalates into a spec?
- Does a spec attach to a brief, or does a spec supersede/replace a brief?
- Should Linear parent issues represent briefs, specs, or epics depending on type/scale?
- How do brief types map to Conventional Commits and changelog sections?
- What is the minimum task contract that keeps agents autonomous without making every task verbose?
- How does `/reflect` ingest completed briefs/tasks into PKB, changelog, and strategic docs?

## Proposed Kickoff Prompt For New Conversation

```text
We need to start a new Loaf branch for the brief-first workflow redesign.

Context:
- Repo: /Users/levifig/Code/levifig/projects/loaf
- Suggested branch: feat/brief-first-workflow
- Read this handoff first: .agents/reports/20260521-181116-brief-taxonomy-handoff.md
- Also check prior related session 019e3fd3-1058-75a1-84d0-03b05be845b7, especially the tail discussion about SQLite/structured state, provenance, session reports, and Markdown as export/view.

Core direction:
- idea/spark/brainstorm -> /shape -> brief -> /breakdown -> task(s)
- /shape creates a brief by default, not a spec.
- Brief types are inferred unless explicitly specified by the user.
- /breakdown should always run on a brief, but may produce exactly one task.
- Tasks are autonomous execution contracts with acceptance criteria, context, verification, and an internal checklist.
- Subtasks should usually remain checklist items inside tasks, not first-class durable records.
- Specs/PRDs/ADRs are escalation artifacts, not the default path.
- Task backend is exclusive: Linear-native tasks if Linear is active, local tasks only in local mode, future backends as first-class citizens.

Task:
1. Read the current relevant skills/templates/docs listed in the handoff.
2. Produce a concise architecture recommendation for the artifact taxonomy and staged migration.
3. Do not implement broad edits yet. First identify the minimal coherent first slice and the files it should touch.
4. Preserve the user's preference for productive iteration: less speculative spec ceremony, stronger briefs and task contracts, and reflection into PKB after shipped work.
```

## Working Recommendation

Move this to a new branch before implementation. The change is cross-cutting enough that it should not ride along with `feat/loose-task-followups`, especially since this branch already has unrelated session-journal modifications.

Start with a design/specification slice that updates the conceptual model and a small number of source skills/templates. Then build out generated targets after the source behavior is coherent.
