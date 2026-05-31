---
id: TASK-250
title: Make housekeeping state-aware in SQLite mode
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T23:25:55Z'
updated: '2026-05-28T23:30:27Z'
completed_at: '2026-05-28T23:30:27Z'
depends_on:
  - TASK-249
files:
  - internal/state/housekeeping.go
  - internal/state/housekeeping_test.go
  - internal/cli/cli.go
  - internal/cli/cli_test.go
  - .agents/tasks/TASK-250-state-aware-housekeeping-read-path.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestHousekeeping|TestRunnerHousekeeping' -count=1
  && go test ./internal/state ./internal/cli
done: >-
  `loaf housekeeping` reads initialized SQLite state, delegates in Markdown-only
  mode, rejects invalid state, and reports lifecycle/triage cleanup signals
  without mutating repository files.
---

# TASK-250: Make housekeeping state-aware in SQLite mode

## Description

Final SPEC-040 audit found that top-level `loaf housekeeping` remains delegated
to the Markdown scanner even after SQLite state is initialized. Track B says
`housekeeping` reads from SQLite when initialized, and the in-scope command
family explicitly includes `loaf housekeeping`.

## Acceptance Criteria

- [x] `loaf housekeeping` delegates to the TypeScript compatibility bridge in
  Markdown-only mode.
- [x] `loaf housekeeping` rejects invalid SQLite state with the same diagnostic
  used by other state-aware commands.
- [x] `loaf housekeeping --json` reads initialized SQLite state and reports
  task/spec/session/report/triage lifecycle counts.
- [x] Human output makes cleanup signals visible without mutating repository
  files.
- [x] Tests cover state behavior, Markdown-only delegation, and invalid-state
  rejection.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestHousekeeping|TestRunnerHousekeeping' -count=1
go test ./internal/state ./internal/cli
```
