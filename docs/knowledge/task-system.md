---
topics:
  - tasks
  - specs
  - shape-up
  - sessions
  - orchestration
covers:
  - .agents/specs/**/*.md
  - .agents/tasks/**/*.md
  - .agents/sessions/**/*.md
  - content/skills/breakdown/**/*
  - content/skills/implement/**/*
  - content/skills/orchestration/**/*
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-04-07'
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
/shape → SPEC file → /breakdown → TASK files → /implement → Sessions → Done
```

## Structure

| Artifact | Location | Purpose |
|----------|----------|---------|
| Specs | `.agents/specs/SPEC-XXX-slug.md` | Bounded work definitions (scope, test conditions, priority order) |
| Tasks | `.agents/tasks/TASK-XXX-slug.md` | Individual work items (acceptance criteria, verification) |
| Task index | `.agents/tasks/TASKS.json` | Programmatic index (CLI readable) |
| Sessions | `.agents/sessions/YYYYMMDD-HHMMSS-slug.md` | Execution context (linked to branch/spec) |

## TASKS.json

Programmatic index alongside individual task .md files. CLI reads/writes it.

```json
{
  "version": 1,
  "next_id": 89,
  "tasks": {
    "TASK-065": {
      "title": "Extract shared content modules",
      "slug": "extract-shared-content-modules",
      "spec": "SPEC-020",
      "status": "todo",
      "priority": "P0",
      "depends_on": [],
      "files": [],
      "verify": null,
      "done": null,
      "session": null,
      "created": "2026-04-04T16:41:22Z",
      "updated": "2026-04-04T16:41:22Z",
      "completed_at": null,
      "file": "TASK-065-extract-shared-content-modules.md"
    }
  },
  "specs": {
    "SPEC-020": {
      "title": "Cross-Harness Skills, Hook Consolidation & Target Convergence",
      "status": "complete",
      "requirement": null,
      "source": null,
      "created": "2026-04-04T00:00:00Z",
      "file": "archive/SPEC-020-target-convergence-amp.md"
    }
  }
}
```

Tasks keyed by ID (Record, not array). Specs section tracks spec lifecycle. `next_id` ensures unique IDs across creates.

### Task Entry Fields

| Field | Type | Notes |
|-------|------|-------|
| `title` | string | Task description |
| `slug` | string | Derived from filename |
| `spec` | string\|null | Associated spec ID |
| `status` | enum | `todo`, `in_progress`, `blocked`, `review`, `done` |
| `priority` | enum | `P0` (critical) through `P3` (nice-to-have) |
| `depends_on` | string[] | Task IDs this depends on |
| `files` | string[] | Hint files relevant to task |
| `verify` | string\|null | Shell command to verify completion |
| `done` | string\|null | Observable done condition |
| `session` | string\|null | Session filename when picked up |
| `created` | ISO 8601 | Creation timestamp |
| `updated` | ISO 8601 | Last-updated timestamp |
| `completed_at` | ISO 8601\|null | Set when status becomes `done` |
| `file` | string | Relative path to task .md file |

## CLI Commands

### `loaf task`

| Subcommand | Purpose |
|------------|---------|
| `list` | Show task board grouped by status |
| `show <id>` | Display single task details |
| `status` | Summary counts |
| `create` | Create new task |
| `update <id>` | Update metadata (status, priority, depends_on, session, spec) |
| `archive [ids...]` | Move completed tasks to `archive/` and update TASKS.json |
| `refresh` | Rebuild TASKS.json from .md files |
| `sync` | Sync between TASKS.json and .md frontmatter |

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
| `list` | List all active and archived sessions |

## Session Lifecycle

Sessions track execution context per branch. Key behaviors:

- **One session per branch.** `loaf session start` finds existing or creates new. Atomic creation via file locking prevents concurrent duplicates.
- **New-conversation detection.** `claude_session_id` in frontmatter tracks the Claude session. When a new session starts on a branch with an existing session, the ID mismatch triggers a resume with PAUSE header.
- **Subagent detection.** `agent_id` in hook JSON is only present for subagents. `session start` exits early when `agent_id` is set, preventing subagent sessions from polluting the parent journal.
- **Branch rename recovery.** If a branch is renamed via `git branch -m`, session start detects the rename via reflog and updates both session and spec frontmatter.
- **Session status values:** `active`, `paused`, `blocked`, `complete`, `archived`

### Session Frontmatter Fields

| Field | Purpose |
|-------|---------|
| `branch` | Git branch name |
| `status` | Session lifecycle state |
| `spec` | Linked spec ID |
| `claude_session_id` | Harness session ID for new-conversation detection |
| `created`, `last_updated`, `last_entry` | Timestamps |
| `archived_at`, `archived_by` | Archive metadata |

## Linear Integration

Optional sync layer. Local tasks are the primary interface. Linear pushes for team visibility when configured. Integration toggled via `integrations.linear.enabled` in `.agents/loaf.json` (set by `loaf install`). When disabled, skills use local `loaf task` commands exclusively.

## Cross-References

- [cli-design.md](cli-design.md) — CLI design philosophy and command patterns
- [knowledge-management-design.md](knowledge-management-design.md) — knowledge system uses similar Shape Up patterns
