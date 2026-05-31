---
id: TASK-251
title: Guard Markdown compatibility commands in SQLite mode
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T23:31:57Z'
updated: '2026-05-28T23:35:20Z'
completed_at: '2026-05-28T23:35:20Z'
depends_on:
  - TASK-250
files:
  - internal/cli/cli.go
  - internal/cli/cli_test.go
  - .agents/tasks/TASK-251-sqlite-mode-compatibility-command-guards.md
verify: >-
  go test ./internal/cli -run 'TestRunnerTaskRefresh|TestRunnerTaskSync|TestRunnerSessionEnrich' -count=1
  && go test ./internal/cli
done: >-
  `loaf task refresh`, `loaf task sync`, and `loaf session enrich` are
  state-aware: they delegate in Markdown-only mode, reject invalid SQLite state,
  and avoid Markdown compatibility mutation when SQLite state is initialized.
---

# TASK-251: Guard Markdown compatibility commands in SQLite mode

## Description

SPEC-040 scopes `loaf task refresh/sync` and `loaf session enrich` as part of
the task/session command families that must become state-backed or state-aware.
These commands are Markdown compatibility operations today. They should still
delegate before SQLite migration, but initialized SQLite projects must not
silently run Markdown-era mutation paths that bypass the canonical state store.

## Acceptance Criteria

- [x] `loaf task refresh` delegates in Markdown-only mode.
- [x] `loaf task refresh` reads initialized SQLite state and reports task/spec
  counts without mutating repository files.
- [x] `loaf task sync` delegates in Markdown-only mode.
- [x] `loaf task sync` reports that Markdown compatibility sync is unnecessary
  in SQLite mode and does not mutate repository files.
- [x] `loaf session enrich` delegates in Markdown-only mode.
- [x] `loaf session enrich` reports that Markdown JSONL enrichment is a
  compatibility path in SQLite mode and does not mutate repository files.
- [x] All three commands reject invalid SQLite state with the standard
  diagnostic.

## Verification

```bash
go test ./internal/cli -run 'TestRunnerTaskRefresh|TestRunnerTaskSync|TestRunnerSessionEnrich' -count=1
go test ./internal/cli
```
