---
topics:
  - tasks
  - specs
  - shape-up
  - sessions
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
last_reviewed: '2026-05-22'
---

# Task System

Loaf implements a Shape Up-inspired task management system: specs define bounded work, tasks break it down, sessions track execution.

## Key Rules

- **Complexity-based sizing.** Size by complexity (small/medium/large), not time. Agents don't have time budgets.
- **Priority ordering.** Ship tracks in order; drop from the end if scope tightens.
- **Go/no-go gates.** Binary checks between priority tracks using test conditions.
- **One concern per task.** One agent type, one concern, fits in context with room for exploration.
- **Tasks are agent-authored, human-reviewed.** Agents create tasks via `/breakdown`, humans approve.

## Pipeline

```
/shape → SPEC file → /breakdown → SQLite tasks → /implement → SQLite sessions → Done
```

## Structure

| Artifact | Location | Purpose |
|----------|----------|---------|
| Specs | `.agents/specs/SPEC-XXX-slug.md` | Bounded work definitions (scope, test conditions, priority order) |
| Tasks | SQLite (`loaf task show/list`) | Individual work items (acceptance criteria, verification) |
| Sessions | SQLite (`loaf session show/list`) | Execution context (linked to branch/spec) |

`findAgentsDir()` remains relevant for durable `.agents/` content such as
specs, reports, councils, handoffs, and project config. Ephemeral task,
session, idea, spark, brainstorm, and draft records live in the global SQLite
store after the SPEC-045 cutover.

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
| `update <id>` | Update metadata (status, priority, depends_on, session, spec) |
| `archive [ids...]` | Archive completed tasks in SQLite state |
| `refresh` | Compatibility diagnostic; no-op in SQLite-backed projects |
| `sync` | Compatibility diagnostic; no-op in SQLite-backed projects |

### `loaf spec`

| Subcommand | Purpose |
|------------|---------|
| `list` | Show specs with status |
| `archive [ids...]` | Move completed specs to `archive/` |

### `loaf session`

| Subcommand | Purpose |
|------------|---------|
| `start` | Start/resume session for current branch |
| `end` | End session with progress summary |
| `log [entry]` | Log entry to session journal |
| `archive` | Archive completed session |
| `housekeeping` | Detect orphan/split sessions and archive old done sessions |
| `enrich [file]` | Record a native SQLite enrichment checkpoint; no markdown edit |
| `list` | List all active and archived sessions |

## Session Lifecycle

Sessions track execution context per branch in SQLite. Key behaviors:

- **One session per harness session ID, not per branch.** `loaf session start` routes on the harness conversation id from the SessionStart hook. One conversation = one SQLite session record, regardless of branches visited.
- **3-tier session routing (SPEC-032).** Session-mutating commands (`loaf session log`, `archive`, `enrich`, `end --wrap`) resolve their target through the native Go session router: `--session-id <id>` flag → hook stdin payload (`--from-hook` opt-in only) → branch-fallback (Tier 3 emits a visible stderr WARN so misroutes surface immediately).
- **New-conversation detection.** When a new session starts with an id differing from the stored harness session id, the session writes resume entries. `loaf session end --wrap` persists wrapped state in SQLite.
- **Subagent detection.** `agent_id` in hook JSON is only present for subagents. `session start` exits early when `agent_id` is set, preventing subagent sessions from polluting the parent journal.
- **Branch rename recovery.** If a branch is renamed via `git branch -m`, session start detects the rename via reflog and updates both session and spec frontmatter.
- **Session status values:** `active`, `stopped`, `done`, `blocked`, `archived`

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
