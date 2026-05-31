---
id: TASK-239
title: Add state-backed spec show command
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:08:17Z'
updated: '2026-05-28T22:11:32Z'
completed_at: '2026-05-28T22:11:32Z'
depends_on:
  - TASK-238
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-239-state-backed-spec-show-read-path.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestShowSpec|TestRunnerSpecShow' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf spec show <spec>` reads initialized SQLite state, including spec
  metadata, source provenance, imported body, task counts, and immediate
  relationships, while delegating in Markdown-only mode.
---

# TASK-239: Add state-backed spec show command

## Description

Close the SPEC-040 command-surface gap for `loaf spec show <spec>`. The Go
front controller already handles `spec list` and `spec archive`; `spec show`
should also become state-backed once SQLite is initialized.

This task should not change Markdown source files. Imported Markdown remains
source/provenance and optional body display data.

## Acceptance Criteria

- [x] `loaf spec show <spec> --json` returns SQLite spec metadata, source
  provenance, imported body, task counts, and immediate relationships.
- [x] Human `loaf spec show <spec>` prints the same useful fields without JSON.
- [x] Markdown-only mode still delegates to the legacy TypeScript CLI.
- [x] Invalid SQLite state fails with the standard doctor hint.
- [x] Tests cover the state package read model and public `Runner` command path.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestShowSpec|TestRunnerSpecShow' -count=1
go test ./internal/state ./internal/cli
```
