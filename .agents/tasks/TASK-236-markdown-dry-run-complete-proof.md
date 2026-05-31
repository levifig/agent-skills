---
id: TASK-236
title: Prove markdown migration dry-run is complete and non-mutating
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:54:32Z'
updated: '2026-05-28T21:56:06Z'
completed_at: '2026-05-28T21:56:06Z'
depends_on:
  - TASK-235
files:
  - internal/cli/
  - .agents/tasks/TASK-236-markdown-dry-run-complete-proof.md
verify: >-
  go test ./internal/cli -run TestRunnerStateMigrateMarkdownJSONDryRunDoesNotCreateDatabase -count=1 && go test ./internal/cli
done: >-
  Public CLI dry-run coverage proves `loaf state migrate markdown --dry-run`
  reports every SPEC-required count and skipped file while leaving SQLite state
  uncreated.
---

# TASK-236: Prove markdown migration dry-run is complete and non-mutating

## Description

Close the SPEC-040 dry-run test condition with a public CLI proof. The underlying
state preview already counts the individual artifact classes, but the SPEC
condition needs command-level evidence that dry-run reports all required counts
and does not initialize or import SQLite state.

This is verification hardening only; no migration behavior should change.

## Acceptance Criteria

- [x] `loaf state migrate markdown --json` reports specs, tasks, ideas, sparks,
  sessions, reports, relationships, and skipped files.
- [x] The dry-run fixture also includes brainstorm coverage so the broader
  migration preview remains exercised at the CLI layer.
- [x] Dry-run leaves the project SQLite database path uncreated.
- [x] The proof runs through the public `Runner` command path, not just the
  state package helper.

## Verification

```bash
go test ./internal/cli -run TestRunnerStateMigrateMarkdownJSONDryRunDoesNotCreateDatabase -count=1
go test ./internal/cli
```
