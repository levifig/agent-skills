---
id: TASK-232
title: Verify linked worktrees share SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:31:57Z'
updated: '2026-05-28T21:34:07Z'
completed_at: '2026-05-28T21:34:07Z'
depends_on:
  - TASK-231
files:
  - internal/cli/
  - internal/project/
  - internal/state/
  - .agents/tasks/TASK-232-linked-worktree-sqlite-state-routing.md
verify: >-
  go test ./internal/cli ./internal/project ./internal/state && go test ./...
done: >-
  Command-level tests prove the main worktree and a linked Git worktree resolve
  to the same SQLite state path and observe the same initialized database and
  state-backed rows.
---

# TASK-232: Verify linked worktrees share SQLite state

## Description

Add command-level coverage for SPEC-040's linked-worktree invariant. Existing
project and state path tests prove the lower-level resolver, but the public
`loaf` command surface also needs to prove that a main checkout and linked Git
worktree use the same SQLite database.

This task should use real temporary Git repositories and `git worktree add`.
It should not migrate or rewrite repository `.agents/` state.

## Acceptance Criteria

- [x] A real main checkout and linked Git worktree fixture are created in tests.
- [x] `loaf state path` returns the same SQLite path from both checkouts.
- [x] `loaf state init` from one checkout is visible through `loaf state status` from the other.
- [x] A state-backed row written from one checkout is readable from the other.
- [x] The test uses temporary XDG/state locations and does not write SQLite state into the repository.
- [x] The SPEC-040 linked-worktree test condition is updated only after verification passes.

## Verification

```bash
go test ./internal/cli ./internal/project ./internal/state
go test ./...
```
