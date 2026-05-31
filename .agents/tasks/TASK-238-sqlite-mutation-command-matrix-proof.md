---
id: TASK-238
title: Prove initialized mutation commands write through SQLite
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:01:49Z'
updated: '2026-05-28T22:06:23Z'
completed_at: '2026-05-28T22:06:23Z'
depends_on:
  - TASK-237
files:
  - internal/cli/
  - .agents/tasks/TASK-238-sqlite-mutation-command-matrix-proof.md
verify: >-
  go test ./internal/cli -run TestRunnerInitializedMutationCommandsWriteThroughSQLite -count=1 && go test ./internal/cli
done: >-
  A public CLI mutation matrix proves initialized task, idea, spark,
  brainstorm, spec, session, tag, bundle, and link mutations write through
  SQLite state instead of Markdown fallback paths.
---

# TASK-238: Prove initialized mutation commands write through SQLite

## Description

Close the broad SPEC-040 mutation-write-through-SQLite test condition with an
aggregate command-level proof. Individual tests already cover specific mutation
families; this task adds one matrix that exercises all currently Go-native
mutation families after SQLite initialization and checks the resulting database
state directly.

This task should not add new command behavior. It is verification hardening for
the initialized-state dispatch contract.

## Acceptance Criteria

- [x] The test runs through the public `Runner` command path.
- [x] The matrix covers task, idea, spark, brainstorm, spec, session, tag,
  bundle, and link mutations.
- [x] The matrix verifies SQLite table state directly after the commands run.
- [x] The matrix uses initialized SQLite state, not Markdown-only delegation.

## Verification

```bash
go test ./internal/cli -run TestRunnerInitializedMutationCommandsWriteThroughSQLite -count=1
go test ./internal/cli
```
