---
id: TASK-247
title: Point workflow guidance at SQLite-backed state commands
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:58:43Z'
updated: '2026-05-28T23:06:56Z'
completed_at: '2026-05-28T23:06:56Z'
depends_on:
  - TASK-246
files:
  - cli/commands/spec.ts
  - cli/commands/task.ts
  - cli/lib/cli-reference-generator.ts
  - content/skills/breakdown/SKILL.md
  - content/skills/cli-reference/SKILL.md
  - content/skills/housekeeping/SKILL.md
  - content/skills/idea/SKILL.md
  - content/skills/implement/SKILL.md
  - content/skills/orchestration/references/local-tasks.md
  - content/skills/orchestration/references/sessions.md
  - content/skills/triage/SKILL.md
  - content/skills/research/SKILL.md
  - content/skills/research/templates/report.md
  - .agents/tasks/TASK-247-sqlite-state-guidance-migration.md
verify: >-
  ! rg -n 'TASKS\.json is the source of truth|Move completed tasks to archive
  and update TASKS\.json|Push TASKS\.json metadata|Update frontmatter
  `status:`|Promoted sparks have corresponding idea files|Create file in
  `\.agents/ideas/`|Status starts as `draft`|Set to `final`|Log resolutions in
  the source document|Append `resolve\(spark\)` entry|Mark inline in source'
  content/skills && jq empty .agents/TASKS.json
done: >-
  Workflow guidance for tasks, triage, reports, and housekeeping treats
  SQLite-backed CLI commands as the operational mutation surface and leaves
  Markdown/frontmatter guidance as compatibility or authored-prose context.
---

# TASK-247: Point workflow guidance at SQLite-backed state commands

## Description

Close the SPEC-040 Track E guidance gap. The Go/SQLite command paths now cover
the core task, triage, report, and export operations, but source skills still
describe `TASKS.json`, Markdown frontmatter, and `.agents/reports/` files as the
primary operational mutation surface.

Update the source guidance so agents use SQLite-aware `loaf` commands for
lifecycle and status changes. Markdown files may still be referenced as authored
prose, source links, generated exports, or compatibility views.

## Acceptance Criteria

- [x] CLI reference describes state-backed command behavior without presenting
  `TASKS.json` as the future source of truth.
- [x] Housekeeping guidance stops requiring `TASKS.json` sync as a post-archive
  invariant for SQLite-backed workflows.
- [x] Local task guidance describes SQLite as the operational store and
  Markdown/frontmatter as compatibility/source prose during migration.
- [x] Triage guidance points capture/promote/archive/resolve actions through
  `loaf idea`, `loaf spark`, and `loaf brainstorm` commands instead of manual
  frontmatter edits.
- [x] Research/report guidance distinguishes generated/persisted report state
  from authored Markdown report files and mentions `loaf report generate`.
- [x] Verification confirms the old source-of-truth/manual-frontmatter phrasing
  is removed or explicitly compatibility-scoped.

## Verification

```bash
! rg -n 'TASKS\.json is the source of truth|Move completed tasks to archive and update TASKS\.json|Push TASKS\.json metadata|Update frontmatter `status:`|Promoted sparks have corresponding idea files|Create file in `\.agents/ideas/`|Status starts as `draft`|Set to `final`|Log resolutions in the source document|Append `resolve\(spark\)` entry|Mark inline in source' content/skills
jq empty .agents/TASKS.json
```
