---
id: TASK-240
title: Add state-backed spark show command
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:12:50Z'
updated: '2026-05-28T22:16:55Z'
completed_at: '2026-05-28T22:16:55Z'
depends_on:
  - TASK-239
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-240-state-backed-spark-show-read-path.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestShowSpark|TestRunnerSparkShow' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf spark show <spark>` reads initialized SQLite state, including spark
  text, scope, status, source provenance, and immediate relationships, while
  delegating in Markdown-only mode.
---

# TASK-240: Add state-backed spark show command

## Description

Close the SPEC-040 command-surface gap for `loaf spark show <spark>`. The Go
front controller already handles spark list, capture, resolve, and promote;
show should become state-backed once SQLite is initialized.

This task should not add new spark lifecycle behavior.

## Acceptance Criteria

- [x] `loaf spark show <spark> --json` returns SQLite spark text, scope, status,
  source provenance, and immediate relationships.
- [x] Human `loaf spark show <spark>` prints the same useful fields without JSON.
- [x] Markdown-only mode still delegates to the legacy TypeScript CLI.
- [x] Invalid SQLite state fails with the standard doctor hint.
- [x] Tests cover the state package read model and public `Runner` command path.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestShowSpark|TestRunnerSparkShow' -count=1
go test ./internal/state ./internal/cli
```
