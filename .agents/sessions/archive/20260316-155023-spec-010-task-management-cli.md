---
session:
  title: "SPEC-010: Task & Spec Management CLI"
  status: archived
  created: "2026-03-16T15:50:24Z"
  last_updated: "2026-03-17T00:09:00Z"
  branch: "feat/spec-010-task-management-cli"
  spec: "SPEC-010"
  archived_at: "2026-03-27T23:06:02Z"
  archived_by: cleanup

traceability:
  requirement: "Surface existing task/spec system through CLI and structured data"
  architecture:
    - "Managed .md data model"
    - "TASKS.json schema"
  decisions:
    - "Council: Managed .md model (JSON source of truth, .md for body content)"
    - "TASKS.json committed to git, .agents/specs/ tracked"
    - "CLI-mediated mutations for both agents and humans"
    - "blocked as first-class task status"

plans:
  - "20260316-155023-spec-010-implementation.md"
transcripts: []

orchestration:
  current_task: "Complete"
  spawned_agents:
    - "task-031-types (backend-dev): TASKS.json schema + TypeScript types"
    - "task-032-parser-migration (backend-dev): frontmatter parser + migration"
    - "task-033-task-commands (backend-dev): loaf task list/status"
    - "task-034-spec-command (backend-dev): loaf spec list"
    - "task-035-create (backend-dev): loaf task create"
    - "task-036-update (backend-dev): loaf task update"
    - "task-037-show-sync (backend-dev): loaf task show + sync"
    - "task-038-skill-updates (backend-dev): update task-related skills"
---

# Session: SPEC-010 — Task & Spec Management CLI

## Context

Implemented the Task & Spec Management CLI per SPEC-010. The "Managed .md" data model was decided via council deliberation (3 perspectives: agent-optimized, human-first, pragmatist). A fourth "Managed .md" model emerged during the interview — JSON is source of truth for metadata, .md files hold body content, CLI mediates all mutations.

## Current State

**Complete.** Committed as `cce51fd` on `feat/spec-010-task-management-cli`.

### Delivered

- 8 CLI commands: `loaf task list/status/show/create/update/sync` + `loaf spec list`
- 6 new source files in `cli/lib/tasks/` and `cli/commands/`
- TASKS.json auto-generated with 30 tasks and 10 specs
- 4 skills updated (breakdown, implement, orchestration, resume-session)
- `.gitignore` updated to track `.agents/TASKS.json` and `.agents/specs/`
- Code review findings fixed: `completed_at` roundtrip, `--json` on task show, recursive archive scanning

### Also in this session
- Renamed `review-sessions` skill → `cleanup` with expanded scope (specs, plans, drafts)
- Shaped SPEC-011 (Knowledge Lifecycle) and SPEC-012 (Cleanup Refinement)
- Captured idea: Spec Lifecycle CLI + AI-Assisted Project Intelligence

## Key Decisions

- **Managed .md model** — council deliberation converged on JSON for metadata, .md for body, CLI mediates
- **Map-keyed schema** — TASKS.json uses `Record<string, TaskEntry>` for O(1) lookup
- **`blocked` status** — added as first-class for "what can I work on?" queries
- **TASKS.json committed** — it's authoritative data, not a cache
- **CLI-mediated creation** — both agents and humans use `loaf task create/update`
- **Transition bridge** — `loaf task sync --import` catches orphan .md files
- **Skill rename** — `review-sessions` → `cleanup` with broader scope

## Resumption Prompt

SPEC-010 is complete. Branch: `feat/spec-010-task-management-cli` — PR #2 open, needs rebase onto main before squash merge. Next specs shaped: SPEC-011 (Knowledge Lifecycle), SPEC-012 (Cleanup Refinement). TASK-031 (test infrastructure, P1) is the most urgent standalone task. Idea parked: Spec Lifecycle CLI + AI intelligence.
