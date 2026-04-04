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
last_reviewed: '2026-04-04'
---

# Task System

Loaf implements a Shape Up-inspired task management system: specs define bounded work, tasks break it down, sessions track execution.

## Key Rules

- **Appetite over estimates.** Decide how much time work is worth, not how long it'll take.
- **Fixed time, variable scope.** Time is fixed; scope flexes when needed.
- **Circuit breakers.** Checkpoint at 50% appetite to re-evaluate.
- **One concern per task.** One agent type, one concern, fits in context with room for exploration.
- **Tasks are agent-authored, human-reviewed.** Agents create tasks via `/breakdown`, humans approve.

## Pipeline

```
/shape → SPEC file → /breakdown → TASK files → /implement → Sessions → Done
```

## Structure

| Artifact | Location | Purpose |
|----------|----------|---------|
| Specs | `.agents/specs/SPEC-XXX-slug.md` | Bounded work definitions (appetite, scope, test conditions) |
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
      "session": null,
      "created": "2026-04-04T16:41:22Z",
      "completed_at": null,
      "file": "TASK-065-extract-shared-content-modules.md"
    }
  },
  "specs": {
    "SPEC-020": {
      "title": "Cross-Harness Skills, Hook Consolidation & Target Convergence",
      "status": "complete",
      "file": "archive/SPEC-020-target-convergence-amp.md"
    }
  }
}
```

Tasks keyed by ID (Record, not array). Specs section tracks spec lifecycle. `next_id` ensures unique IDs across creates.

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

## Linear Integration

Optional sync layer. Local tasks are the primary interface. Linear pushes for team visibility when configured. Integration toggled via `integrations.linear.enabled` in `.agents/loaf.json` (set by `loaf install`). When disabled, skills use local `loaf task` commands exclusively.

## Cross-References

- [cli-design.md](cli-design.md) — CLI design philosophy and command patterns
- [knowledge-management-design.md](knowledge-management-design.md) — knowledge system uses similar Shape Up patterns
