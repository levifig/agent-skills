---
id: SPEC-010
title: Task & Spec Management CLI
source: direct
created: '2026-03-14T19:48:00.000Z'
status: complete
---

# SPEC-010: Task & Spec Management CLI

## Problem Statement

Loaf has a mature Shape Up task/spec system — 20+ task files and 10 specs with consistent YAML frontmatter — but it is only accessible through markdown files and agent skills. Humans lack CLI visibility into project state. Future TUI and web kanban interfaces need structured, programmatic data. And the current approach of agents editing YAML frontmatter inside markdown files is fragile (line-targeted edits can corrupt delimiters, every query requires globbing and parsing N files).

## Strategic Alignment

- **Vision:** Phase 3 (CLI Maturity) in the phased roadmap. Directly advances the goal of "a CLI that manages everything" and provides the data layer for future autonomous execution (Phase 5).
- **Personas:** Framework user gets CLI visibility into project state; agents get reliable, low-token-cost task operations through CLI commands instead of fragile frontmatter editing.
- **Architecture:** Builds on the SPEC-008 CLI skeleton (Commander.js, tsup bundle, ANSI color helpers). Introduces the "Managed .md" data model — a new pattern where structured metadata lives in JSON and rich content lives in markdown.

## Solution Direction

### The "Managed .md" Model

A council deliberation (agent-optimized, human-first, pragmatist perspectives) converged on a hybrid data model:

**TASKS.json** is the source of truth for structured metadata — status, priority, dependencies, dates, and task-to-spec relationships. It is committed to git and written exclusively through CLI commands.

**TASK-XXX-slug.md** files hold body content — descriptions, acceptance criteria, context, work logs. Their YAML frontmatter is a **read-only mirror** synced from TASKS.json for convenience (GitHub browsing, editor viewing). Body content below the frontmatter is authored directly by agents and humans.

**All metadata mutations go through CLI commands.** Both agents and humans use the same interface: `loaf task create`, `loaf task update`. This eliminates the fragile pattern of agents editing YAML frontmatter and creates a single codepath that a future TUI or web kanban can also use.

### TASKS.json Schema

Map-keyed by ID for O(1) lookup and clean git merges:

```json
{
  "version": 1,
  "next_id": 31,
  "tasks": {
    "TASK-019": {
      "title": "Enhance /resume for intelligent project resumption",
      "slug": "intelligent-resume",
      "spec": "SPEC-002",
      "status": "todo",
      "priority": "P1",
      "depends_on": [],
      "files": [],
      "verify": "...",
      "done": "...",
      "session": null,
      "created": "2026-01-24T03:20:00Z",
      "updated": "2026-01-24T03:20:00Z",
      "completed_at": null,
      "file": "TASK-019-intelligent-resume.md"
    }
  },
  "specs": {
    "SPEC-010": {
      "title": "Task & Spec Management CLI",
      "status": "approved",
      "appetite": "Large (1-2 weeks)",
      "requirement": "Surface existing task/spec system through CLI and structured data",
      "created": "2026-03-14T19:48:00Z",
      "file": "SPEC-010-task-management-cli.md"
    }
  }
}
```

### Task Statuses

`todo` | `in_progress` | `blocked` | `review` | `done`

`blocked` is new — distinguishes "waiting on something" from "available to pick up," which is essential for "what can I work on next?" queries and kanban column rendering.

### Transition Bridge

A `loaf task sync --import` command detects orphan .md files (those with frontmatter but no corresponding JSON entry) and imports them. This provides a safety net during the transition period where skills are being updated to use CLI commands.

## Scope

### In Scope

- **`loaf task list`** — formatted task board from TASKS.json, grouped by status
- **`loaf task status`** — summary counts by status (todo, in_progress, blocked, done)
- **`loaf task show <id>`** — single task detail view (JSON metadata + .md body content)
- **`loaf task create`** — create task (JSON entry + .md skeleton with synced frontmatter)
- **`loaf task update <id>`** — update metadata fields (status, priority, depends_on, etc.)
- **`loaf spec list`** — spec overview with per-spec task counts
- **`loaf task sync --import`** — import orphan .md files not yet in TASKS.json
- **TASKS.json schema** — TypeScript types, validation, versioned schema
- **Migration script** — one-time conversion of existing 20+ task files and 10 specs into TASKS.json
- **Skill updates** — update breakdown, implement, orchestration, resume-session skills to use CLI commands for task operations
- **`blocked` status** — added as first-class task status in the schema

### Out of Scope

- TUI kanban board (future spec)
- Web kanban / mini-Trello visualization (future spec)
- Linear bidirectional sync (exists as separate optional integration)
- `loaf spec create` (shaping is a skill-driven workflow, not a CLI command)
- pm agent → harness CLI transformation (future strategic work)
- Task assignment / ownership model
- Notifications or alerts on task state changes

### Rabbit Holes

- **Terminal task editor** — tempting to build an interactive editor for task body content. Just open the .md file in `$EDITOR` or print the path.
- **Bidirectional frontmatter sync** — the sync is one-way only (JSON → .md). If someone edits .md frontmatter, `sync --import` can reconcile, but this is not the happy path.
- **Elaborate filtering/sorting** — start with grouping by status. Filtering by spec, priority, or labels can come later via flags.
- **Concurrent TASKS.json writes** — the single-writer model (CLI commands are the only writer) avoids this. Don't try to solve distributed locking.
- **TASKS.md backward compatibility** — the existing generated `TASKS.md` can be replaced by `loaf task list --format md` output. Don't maintain two generation paths.

### No-Gos

- Don't break existing .md body content (descriptions, criteria, work logs must survive migration)
- Don't require external dependencies (no SQLite, no database engines)
- Don't auto-commit task changes (human decides when to commit)
- Don't build the TUI/GUI in this spec
- Don't make Linear a requirement for any functionality

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Skills don't adopt CLI commands | Medium | Medium | Transition bridge (`sync --import`) catches orphans; update skill instructions with clear guidance |
| TASKS.json merge conflicts in git | Low | Low | Map-keyed structure merges cleanly when different tasks are modified; single-writer model reduces simultaneous edits |
| Migration misses edge cases in frontmatter | Low | Medium | Validation step in migration; original .md files preserved as body content files |
| Schema needs to evolve post-v1 | Medium | Low | `version` field in TASKS.json enables future migrations |

## Open Questions

- [x] Data model: JSON source of truth vs derived index → **Managed .md (JSON for metadata, .md for body)**
- [x] Appetite → **Large (1-2 weeks)**
- [x] Task + spec in one spec or split → **Together**
- [x] Agent integration depth → **Full (update all task-related skills)**
- [x] Git tracking of TASKS.json → **Committed**

## Test Conditions

- [ ] `loaf task list` shows formatted task board grouped by status with colored output
- [ ] `loaf task status` shows correct counts for todo, in_progress, blocked, done
- [ ] `loaf task create --spec SPEC-010 --title "Test task" --priority P2` creates JSON entry + .md skeleton
- [ ] `loaf task update TASK-019 --status in_progress` updates JSON and syncs .md frontmatter
- [ ] `loaf task show TASK-019` displays metadata table + rendered .md body content
- [ ] `loaf spec list` shows all specs with status and per-spec task counts
- [ ] `loaf task sync --import` detects orphan .md files and imports into TASKS.json
- [ ] Migration script correctly converts all existing task and spec files into TASKS.json
- [ ] Existing .md body content is preserved verbatim through migration
- [ ] Updated skills (breakdown, implement, orchestration, resume-session) reference CLI commands for task operations
- [ ] TASKS.json validates against TypeScript schema (no unknown fields, correct types)
- [ ] `blocked` status is available and renders correctly in task list

## Circuit Breaker

**At 50% appetite:** Drop `loaf task show` and `loaf task sync --import`. Focus on the core: list, status, create, update, spec list, and migration. The transition bridge can be a fast-follow.

**At 75% appetite:** Drop skill updates. Ship the CLI commands and migration, update skills in a follow-up spec. The CLI works independently of whether skills know about it.
