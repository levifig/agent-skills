---
topics:
  - tasks
  - specs
  - shape-up
  - journal
  - orchestration
covers:
  - .agents/specs/**/*.md
  - internal/cli/cli.go
  - internal/state/task_*.go
  - content/skills/breakdown/**/*
  - content/skills/implement/**/*
  - content/skills/orchestration/**/*
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-07-03'
---

# Task System

Loaf implements a Shape Up-inspired task management system: specs define bounded work, tasks break it down, the project journal records execution.

## Key Rules

- **Complexity-based sizing.** Size by complexity (small/medium/large), not time. Agents don't have time budgets.
- **Priority ordering.** Ship tracks in order; drop from the end if scope tightens.
- **Go/no-go gates.** Binary checks between priority tracks using test conditions.
- **One concern per task.** One agent type, one concern, fits in context with room for exploration.
- **Tasks are agent-authored, human-reviewed.** Agents create tasks via `/breakdown`, humans approve.

## Pipeline

```
/shape → SPEC file → /breakdown → SQLite tasks → /implement → project journal → Done
```

## Structure

| Artifact | Location | Purpose |
|----------|----------|---------|
| Specs | `.agents/specs/SPEC-XXX-slug.md` | Bounded work definitions (scope, test conditions, priority order) |
| Tasks | SQLite (`loaf task show/list`) | Individual work items (acceptance criteria, verification) |
| Journal | SQLite (`loaf journal recent/show`) | Project-scoped execution record across every conversation |

`findAgentsDir()` remains relevant for durable `.agents/` content such as
specs, reports, councils, handoffs, and project config. Ephemeral task,
idea, spark, brainstorm, and draft records live in the global SQLite
store after the SPEC-045 cutover; the journal is SQLite-native as well.

## TASKS.json

`.agents/TASKS.json` was removed by the SPEC-045 cutover. It is rollback
material only, restored by `loaf state restore-ephemerals <backup-id>` when
explicitly undoing the cutover. Do not recreate it as an index or compatibility
mirror. `loaf check --hook ephemeral-provenance` fails if `TASKS.json` or
tracked ephemeral markdown returns.

## CLI Commands

### `loaf task`

| Subcommand | Purpose |
|------------|---------|
| `list` | Show task board grouped by status |
| `list --status <status>` | Filter tasks to one status |
| `show <id>` | Display single task details |
| `status` | Summary counts |
| `create` | Create new task |
| `update <id>` | Update metadata (status, priority, depends_on, spec) |
| `archive [ids...]` | Archive completed tasks in SQLite state |
| `refresh` | Compatibility diagnostic; no-op in SQLite-backed projects |
| `sync` | Compatibility diagnostic; no-op in SQLite-backed projects |

### `loaf spec`

| Subcommand | Purpose |
|------------|---------|
| `list` | Show specs with status |
| `archive [ids...]` | Move completed specs to `archive/` |

### `loaf journal`

| Subcommand | Purpose |
|------------|---------|
| `log [entry]` | Append a project-scoped journal entry |
| `recent` | Show the recent journal timeline (`--branch`, `--since-last-wrap`) |
| `search <query>` | Full-text search across the project journal |
| `show <id>` | Read one journal entry |
| `context` | Emit the layered continuity digest (latest wrap + branch entries + open tasks) |
| `export` | Export the journal to Markdown or JSONL |

## Journal Model (SPEC-056)

The project journal is the only session-related structure. There is no session
entity, status, or lifecycle. Key behaviors:

- **Project-scoped events, correlated by harness id.** `journal_entries` rows carry `project_id NOT NULL` and an opaque `harness_session_id` that groups one conversation's entries. Nobody opens, closes, or transitions anything.
- **Concurrency-safe by construction.** Two conversations logging at once — across branches, worktrees, or harnesses — interleave rows with different `harness_session_id` tags, which is correct rather than corrupt. There is no router to misroute and no branch-fallback tier (SPEC-032's session router was superseded).
- **Wrap is an optional checkpoint.** `loaf journal log "wrap(scope): …"` records synthesis worth saving; nothing is ever "unwrapped," and a conversation that ends without one leaves a valid journal.
- **Continuity is derived and ephemeral.** The SessionStart hook runs `loaf journal context --from-hook` to emit a read-time digest that is shown, then discarded. Subagent invocations (`agent_id` present in hook JSON) exit silently and write nothing.

### Cross-Branch Reconciliation

After SPEC-045, stale branches may still carry deleted ephemeral files. Rebase
or merge main, keep the deletion side for `.agents/{tasks,ideas,sparks,sessions,brainstorms,drafts}/`
and `.agents/TASKS.json`, then run `loaf check --hook ephemeral-provenance`.
If rollback is intentionally needed, use `loaf state restore-ephemerals
<backup-id>` and re-import forward; do not hand-edit restored files as a mirror.

## Linear Integration

Optional sync layer. Local tasks are the primary interface. Linear pushes for team visibility when configured. Integration toggled via `integrations.linear.enabled` in `.agents/loaf.json` (set by `loaf install`). When disabled, skills use local `loaf task` commands exclusively.

## Cross-References

- [cli-design.md](cli-design.md) — CLI design philosophy and command patterns
- [knowledge-management-design.md](knowledge-management-design.md) — knowledge system uses similar Shape Up patterns
