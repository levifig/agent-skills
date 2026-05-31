---
id: TASK-246
title: Manage report lifecycle in SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:47:20Z'
updated: '2026-05-28T22:56:58Z'
completed_at: '2026-05-28T22:56:58Z'
depends_on:
  - TASK-245
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-246-state-backed-report-lifecycle.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestReportLifecycle|TestRunnerReportLifecycle' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf report create`, `loaf report finalize`, and `loaf report archive` write
  SQLite report rows and status events when state is initialized, delegate in
  Markdown-only mode, and do not create repository files.
---

# TASK-246: Manage report lifecycle in SQLite state

## Description

Close the SPEC-040 report lifecycle mutation gap by wiring native Go command
paths for:

- `loaf report create <slug>`
- `loaf report finalize <report>`
- `loaf report archive <report>`

When SQLite state is initialized, these commands should write report rows and
status-change events in SQLite. Markdown-only projects should keep delegating to
the TypeScript compatibility command.

This task should not create Markdown report files or implement report body
editing.

## Acceptance Criteria

- [x] `loaf report create <slug> --type <kind> --source <source>` creates a
  draft report row with a stable report alias and creation event in SQLite.
- [x] `loaf report finalize <report>` transitions a draft report to final and
  records a status-change event.
- [x] `loaf report archive <report>` transitions a final report to archived and
  records a status-change event.
- [x] Invalid lifecycle transitions fail without mutating the report.
- [x] SQLite-backed report lifecycle commands do not create repository files.
- [x] Markdown-only state delegates create/finalize/archive to the TypeScript
  compatibility command.
- [x] Missing and invalid SQLite state errors are clear and consistent with
  other SQLite-backed commands.
- [x] Tests cover state package behavior and public `Runner` command paths.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestReportLifecycle|TestRunnerReportLifecycle' -count=1
go test ./internal/state ./internal/cli
```
