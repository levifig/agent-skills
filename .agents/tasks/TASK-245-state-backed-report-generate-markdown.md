---
id: TASK-245
title: Generate report Markdown from SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:43:09Z'
updated: '2026-05-28T22:46:06Z'
completed_at: '2026-05-28T22:46:06Z'
depends_on:
  - TASK-244
files:
  - internal/cli/
  - .agents/tasks/TASK-245-state-backed-report-generate-markdown.md
verify: >-
  go test ./internal/cli -run 'TestRunnerReportGenerate|TestRunnerSessionReport' -count=1 && go test ./internal/cli
done: >-
  `loaf report generate session <session>`, `loaf report generate triage`,
  `loaf report generate release-readiness`, and `loaf session report <session>`
  generate Markdown from SQLite state to stdout without mutating the database or
  repository files.
---

# TASK-245: Generate report Markdown from SQLite state

## Description

Close the SPEC-040 generated-report command gap by wiring native Go command
entry points for:

- `loaf report generate session <session>`
- `loaf report generate triage`
- `loaf report generate release-readiness`
- `loaf session report <session>`

These commands should generate Markdown from SQLite state and print it to
stdout. This task should not create persisted report rows or write repository
Markdown.

## Acceptance Criteria

- [x] `loaf report generate session <session>` renders the same SQLite-backed
  session Markdown as the state export path.
- [x] `loaf report generate triage` renders the same external-safe triage
  Markdown as the state export path.
- [x] `loaf report generate release-readiness` renders the same external-safe
  release-readiness Markdown as the state export path.
- [x] `loaf session report <session>` works as a session-report alias.
- [x] Missing and invalid SQLite state fail with the same state-export errors.
- [x] The commands are deterministic and do not mutate SQLite state.
- [x] The commands do not create repository files.
- [x] Tests cover the public `Runner` command paths.

## Verification

```bash
go test ./internal/cli -run 'TestRunnerReportGenerate|TestRunnerSessionReport' -count=1
go test ./internal/cli
```
