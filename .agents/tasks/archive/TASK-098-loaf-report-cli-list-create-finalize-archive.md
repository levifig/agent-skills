---
id: TASK-098
title: 'loaf report CLI — list, create, finalize, archive'
spec: SPEC-028
status: done
priority: P1
created: '2026-04-07T22:05:30.601Z'
updated: '2026-04-07T22:21:54.789Z'
completed_at: '2026-04-07T22:21:54.789Z'
---

# TASK-098: loaf report CLI — list, create, finalize, archive

## Description

New `loaf report` command with four subcommands, following the `loaf spec`/`loaf task` pattern (Commander.js, ANSI output).

**Files to create/modify:**
- `cli/commands/report.ts` — new command file with list, create, finalize, archive subcommands
- `cli/index.ts` — register the report command
- `cli/commands/report.test.ts` — tests for all subcommands

**Reference implementations:** `cli/commands/spec.ts`, `cli/commands/task.ts`

**Subcommands:**
- `loaf report list` — list reports grouped by status, `--type` and `--status` filters
- `loaf report create <slug>` — scaffold report from template, `--type` (default: research), `--source` (optional)
- `loaf report finalize <file>` — set `status: final`, add `finalized_at` timestamp
- `loaf report archive <file>` — move to `archive/`, set `archived_at`/`archived_by`, validate linked session is archived

**Dependencies:** TASK-097 (needs unified template frontmatter schema)

## Acceptance Criteria

- [ ] `loaf report list` shows reports grouped by status with ANSI formatting
- [ ] `loaf report list --type research` filters by type
- [ ] `loaf report list --status draft` filters by status
- [ ] `loaf report create my-topic --type research` scaffolds report with correct frontmatter and timestamp
- [ ] `loaf report create` defaults to `--type research` and `--source ad-hoc`
- [ ] `loaf report finalize <file>` transitions draft → final, sets `finalized_at`
- [ ] `loaf report finalize` errors on non-draft reports
- [ ] `loaf report archive <file>` moves to `archive/`, validates linked session is archived
- [ ] `loaf report archive` errors if linked session is not archived
- [ ] Command registered in `cli/index.ts`
- [ ] Tests cover all subcommands and error cases

## Verification

```bash
npm run typecheck && npm run test
# Smoke test:
loaf report list
loaf report create test-report --type research
loaf report finalize .agents/reports/*test-report*.md
loaf report archive .agents/reports/*test-report*.md
```
