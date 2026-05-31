---
id: TASK-241
title: Add state-backed session show command
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:19:40Z'
updated: '2026-05-28T22:23:39Z'
completed_at: '2026-05-28T22:23:39Z'
depends_on:
  - TASK-240
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-241-state-backed-session-show-read-path.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestShowSession|TestRunnerSessionShow' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf session show <session>` reads initialized SQLite state, including
  session metadata, source provenance, journal entries, and immediate
  relationships, while delegating in Markdown-only mode.
---

# TASK-241: Add state-backed session show command

## Description

Close the SPEC-040 command-surface gap for `loaf session show <session>`. The
Go front controller already handles session list and log; show should become
state-backed once SQLite is initialized.

This task should not add transcript enrichment or generated report behavior.

## Acceptance Criteria

- [x] `loaf session show <session> --json` returns SQLite session metadata,
  source provenance, journal entries, and immediate relationships.
- [x] Human `loaf session show <session>` prints the same useful fields without JSON.
- [x] Markdown-only mode still delegates to the legacy TypeScript CLI.
- [x] Invalid SQLite state fails with the standard doctor hint.
- [x] Tests cover the state package read model and public `Runner` command path.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestShowSession|TestRunnerSessionShow' -count=1
go test ./internal/state ./internal/cli
```
