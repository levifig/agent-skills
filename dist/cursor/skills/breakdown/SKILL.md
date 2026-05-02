---
name: breakdown
description: >-
  Decomposes specifications into atomic tasks with dependencies and priorities.
  Use when the user asks "break this down" or "create tasks for this spec."
  Produces task files with estimates, dependencies, and acceptance criteria. Not
  for shaping ideas (use shape) or implementation work (use implement).
version: 2.0.0-dev.39
---

# Breakdown

Decompose specifications into atomic, implementable tasks.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Task Breakdown Philosophy
- Task Backend Detection
- Process
- Linear-Native Mode
- Local-Tasks Mode
- Priority Levels
- Guardrails
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

- **One concern per task** -- never mix unrelated layers (backend + frontend) in a single task
- **Every task includes its own verification** -- no separate "verify" tasks; each task must have an observable done condition
- **Own the decisions** -- decide granularity and priorities autonomously; only ask the user when two equally valid orderings have genuinely different trade-offs
- **Keep tests with the code they test** -- never split implementation and tests into separate tasks
- **Update spec status** -- mark the spec as `implementing` after tasks are created
- **One backend only** -- in Linear-native mode create Linear issues and NO local `TASK-NNN.md`; in local mode create local tasks and make NO Linear calls
- **Spec file is always local** -- in both modes, the spec stays in `.agents/specs/`. The Linear parent issue, when present, is a rollup pointing to the spec, not a re-host of it
- **Log outcome** -- log breakdown to session journal: `loaf session log "decision(breakdown): SPEC-NNN → N tasks created"`

---

## Verification

- Each created task has a clear title, priority, file hints, verification command, and observable done condition
- The dependency graph has no cycles and reflects actual implementation order
- Spec status has been updated to `implementing`
- **Linear-native mode only:** parent issue exists, labeled `spec`, with description pointing to the local spec file; N sub-issues have `parentId` set; zero local `TASK-NNN.md` files and zero new `TASKS.json` entries were created; spec frontmatter has `linear_parent` and `linear_parent_url` populated
- **Local-tasks mode only:** N local `TASK-NNN.md` files exist with matching `TASKS.json` entries; no Linear calls were made

---

## Quick Reference

### Priority Levels

| Priority | Loaf | Linear Priority |
|----------|------|-----------------|
| P0 | Urgent/blocking -- drop everything | Urgent (1) |
| P1 | High -- work next | High (2) |
| P2 | Normal -- scheduled work (default) | Normal (3) |
| P3 | Low -- when time permits | Low (4) |

### Right-Sizing Rules

| Rule | Guideline |
|------|-----------|
| **One agent type** | Completable by a single implementer (after skills narrowing) |
| **One concern** | Touches one layer, service, or component |
| **Context-appropriate** | Fits in model context with room for exploration |
| **Not over-fragmented** | Don't split what naturally belongs together |

### Mode Selection

| `integrations.linear.enabled` in `.agents/loaf.json` | Mode | See |
|------------------------------------------------------|------|-----|
| `true` | Linear-native | [Linear-Native Mode](#linear-native-mode) |
| `false` or absent | Local-tasks | [Local-Tasks Mode](#local-tasks-mode) |

---

## Task Breakdown Philosophy

**Primary principle: separation of concerns.**

### The Right Size Test

1. Can a single implementer complete this? If no, split by concern
2. Does it touch multiple unrelated concerns? If yes, split by concern
3. Will the agent need too much context? If yes, split into phases
4. Am I splitting just to have more tasks? If yes, merge back

### Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Split backend + tests into separate tasks | Keep tests with the code they test |
| Create a task per file | Group files by concern |
| Separate "implement" and "verify" tasks | Every task includes its own verification |
| Copy the full spec text into the Linear parent issue | Summarize + link to the local spec file |
| Create both local `TASK-NNN.md` and Linear sub-issues | Pick one backend; never mix |

---

## Task Backend Detection

Read `.agents/loaf.json`:

```json
{
  "integrations": {
    "linear": { "enabled": true }
  }
}
```

If `integrations.linear.enabled` is `true`, proceed in **Linear-native mode**.
Otherwise, proceed in **Local-tasks mode**.

If `.agents/loaf.json` is missing, default to local-tasks and note the
assumption in the session journal.

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` should reference a spec (e.g., "SPEC-001"). If unclear, list available specs.

### Step 2: Read the Spec

Extract: test conditions, scope, implementation notes, priority ordering, complexity size.

### Step 3: Identify Task Boundaries

Break down by concern (data layer, backend, frontend, infrastructure, etc.). One concern per task. Explicit dependencies for sequential tasks.

### Step 4: Decide Priorities and Granularity

Own the granularity and priority decisions. Apply the Right Size Test, assign priorities
based on dependencies, priority order, and go/no-go gates, and do a self-review pass. Do not
defer these decisions to the user — they trust agent judgment here.

If genuinely uncertain (e.g., two equally valid orderings with different trade-offs),
ask. Otherwise, decide and move on.

### Step 5: Draft Task List

Draft tasks following [task template](templates/task.md). Each task needs: clear title, priority, file hints, verification command, observable done condition, labels (if routing by team).

### Step 6: Present the Plan

Show the dependency graph and task summary for awareness before creating anything.
Present it as "here's what I'm creating" not "which option do you prefer?" The user
can still adjust after creation, but the default is to proceed.

### Step 7: Create Tasks (mode-specific)

Detect the mode (see [Task Backend Detection](#task-backend-detection)) and follow the
matching section below. Do NOT mix modes.

- Linear enabled → [Linear-Native Mode](#linear-native-mode)
- Linear disabled or missing → [Local-Tasks Mode](#local-tasks-mode)

### Step 8: Update Spec and Announce

Set spec status to `implementing`. In Linear-native mode, also write
`linear_parent` and `linear_parent_url` into the spec's frontmatter. Announce
created tasks and next steps.

---

## Linear-Native Mode

Spec files stay local and canonical in `.agents/specs/`. Tasks live in Linear
as sub-issues of a parent rollup issue representing the spec. No local
`TASK-NNN.md` files are created. No new `TASKS.json` entries are created.

### 7a. Ensure the `spec` label exists

The `spec` label groups all spec-parent rollup issues so Linear users can
filter for them.

1. Call `list_issue_labels` to check whether a label named `spec` exists.
2. If missing, create it via `create_issue_label`:
   - `name`: `spec`
   - `color`: `#5e6ad2` (Linear-ish indigo; implementer may adjust)
   - `description`: `Parent rollup issue representing a design spec tracked in the repo at .agents/specs/`
   - Prefer workspace-scoped so all teams can filter uniformly. If the MCP
     only supports team-scoped labels, create on the default team.
3. Log whether the label was created this run or already existed. This
   matters for first-time Loaf setup on a Linear workspace.

### 7b. Resolve team, project, and state

Read from `.agents/loaf.json`:

- **Team:** `linear.default_team` (name) — resolve to team ID via
  `list_teams` if not already cached in `known_teams`.
- **Project:** `linear.project.id`.
- **State:** call `list_issue_statuses` for the team, pick the
  `unstarted`-type state (typically "Backlog" or "To-Do"). States are
  **team-scoped**, not workspace-scoped — always pass the team.

### 7c. Create the parent issue

Use `create_issue` with:

| Field | Value |
|-------|-------|
| `title` | `[SPEC-NNN] <spec title>` |
| `teamId` | from 7b |
| `projectId` | from 7b |
| `stateId` | unstarted state from 7b |
| `priority` | mapped from spec (default High = 2 if unspecified) |
| `labels` | `["spec"]` |
| `description` | Summary synthesized from the spec's Problem Statement + Solution Direction (1–3 paragraphs), ending with: `See .agents/specs/SPEC-NNN-<slug>.md for full text, council references, and strategic tensions.` |

**Do NOT** copy the full spec body into the description. The local file is canonical.

### 7d. Check label-group conflicts (pre-flight per sub-issue)

Linear labels can belong to exclusive groups (e.g., a `type` group where
`feature`, `testing`, `docs`, `bug`, `refactor` are mutually exclusive).
Before creating each sub-issue:

1. Inspect proposed labels against known group membership (from
   `list_issue_labels` group metadata).
2. If a task has more than one label from the same exclusive group, pick the
   most appropriate and drop the others. Warn the user about the drop.
3. Log the resolution so the user can override if desired.

### 7e. Create sub-issues

For each task, use `create_issue` with:

| Field | Value |
|-------|-------|
| `parentId` | parent issue ID from 7c |
| `title` | task title |
| `description` | task description + acceptance criteria |
| `teamId` | routed from `team_keywords` or falling back to `default_team` |
| `projectId` | same as parent unless task explicitly belongs elsewhere |
| `stateId` | unstarted state for the target team |
| `priority` | mapped from task priority (see Priority Levels table) |
| `labels` | task labels after conflict resolution (7d) |

Express dependencies from the spec's Priority Order / dependency graph via
`blockedBy` referencing sibling sub-issue IDs. Create in dependency order so
predecessors exist when referenced.

### 7f. Do NOT create local task files

Skip `loaf task create` entirely. Linear issue IDs are the task record. No
`TASK-NNN.md` files, no new `TASKS.json` entries for this spec's tasks.

### 7g. Update spec frontmatter

Add to the spec file's YAML frontmatter:

```yaml
linear_parent: ENG-198
linear_parent_url: https://linear.app/<workspace>/issue/ENG-198
```

Use the actual parent issue identifier and URL returned from 7c.

---

## Local-Tasks Mode

Spec files and task files both live locally. No Linear calls.

Use `loaf task create --spec SPEC-XXX --title "Task title" --priority P1`
for each task. The CLI creates the `TASKS.json` entry and `TASK-NNN.md`
skeleton file. Then edit the `.md` body content (description, acceptance
criteria) directly.

Dependencies are expressed in `TASKS.json` via the CLI's dependency flags (or
hand-edited into the JSON). Priority Order from the spec maps directly to
task `priority` fields.

See [local-tasks reference](../orchestration/references/local-tasks.md) for
the full local-task model.

---

## Priority Mapping (reference)

| Loaf | Linear API value | Linear label |
|------|------------------|--------------|
| P0 | `1` | Urgent |
| P1 | `2` | High |
| P2 | `3` | Normal |
| P3 | `4` | Low |

---

## Guardrails

1. **One concern per task** -- don't mix backend + frontend
2. **Clear verification** -- how to prove it works
3. **Observable done condition** -- not subjective
4. **File hints** -- help session know where to look
5. **Own the decisions** -- decide granularity and priorities, don't defer
6. **Update spec status** -- mark as implementing
7. **One backend only** -- Linear-native creates Linear issues and no local tasks; local-tasks mode creates local tasks and no Linear calls
8. **Summary not copy** -- the Linear parent description summarizes + links; it does not re-host the spec

---

## Suggests Next

After breakdown completes, suggest `/implement` to start working on the tasks.

## Related Skills

- **shape** -- Create specs that get broken down
- **implement** -- Start session for a task or coordinate multiple tasks

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Linear Integration | `orchestration/references/linear.md` | Working out Linear issue structure, labels, parent/child |
| Local Task Model | `orchestration/references/local-tasks.md` | Local-tasks mode details and CLI flags |
