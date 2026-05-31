---
id: TASK-249
title: Add state-backed bundle list and update commands
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T23:16:03Z'
updated: '2026-05-28T23:23:32Z'
completed_at: '2026-05-28T23:23:32Z'
depends_on:
  - TASK-248
files:
  - internal/state/bundle.go
  - internal/state/bundle_test.go
  - internal/cli/cli.go
  - internal/cli/cli_test.go
  - .agents/tasks/TASK-249-state-backed-bundle-list-update.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestBundles|TestRunnerBundle' -count=1
  && go test ./internal/state ./internal/cli
done: >-
  `loaf bundle list` and `loaf bundle update` are Go-native when SQLite state is
  initialized, delegate in Markdown-only mode, and cover SPEC-040's bundle
  command scope.
---

# TASK-249: Add state-backed bundle list and update commands

## Description

Final SPEC-040 audit found that the spec explicitly names
`loaf bundle list/show/create/update`, but the Go command surface only covers
`bundle create/show/add/remove`. Add native SQLite-backed `bundle list` and
`bundle update` support while preserving Markdown-only delegation.

## Acceptance Criteria

- [x] `loaf bundle list` reads bundle rows from SQLite when initialized.
- [x] `loaf bundle update <slug>` can update title and tag query for an existing
  bundle.
- [x] Both commands support `--json` and human output.
- [x] Both commands delegate to the TypeScript compatibility bridge in
  Markdown-only mode.
- [x] Tests cover state behavior, Markdown-only delegation, and invalid-state
  rejection.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestBundles|TestRunnerBundle' -count=1
go test ./internal/state ./internal/cli
```
