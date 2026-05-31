---
id: TASK-191
title: Implement Go front controller with native state path dispatch
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T15:13:09Z'
updated: '2026-05-28T16:06:06Z'
depends_on:
  - TASK-190
files:
  - cmd/loaf/main.go
  - internal/cli/
  - internal/state/
  - internal/project/
  - cli/lib/tasks/resolve.ts
verify: go test ./... && go run ./cmd/loaf state path
done: >-
  Go command can run `state path` natively and resolves the same project-state
  path from main and linked worktrees without invoking the TypeScript CLI
completed_at: '2026-05-28T16:06:06Z'
---

# TASK-191: Implement Go front controller with native state path dispatch

## Description

Add the first Go-native user-facing command: `loaf state path`. This proves the
front-controller shape and anchors the state path/project identity work before
SQLite is introduced.

The command should not initialize a database. It should compute and print the
intended SQLite path using the Track 0 path policy, with linked worktrees
resolving to the same project identity as the main worktree.

## Acceptance Criteria

- [x] Go command dispatch recognizes `state path`.
- [x] `state path` is implemented in Go, not delegated to TypeScript.
- [x] Path resolution stores state outside the repository.
- [x] Linked worktree and main worktree for the same project resolve to the same state path.
- [x] Non-git directories have deterministic fallback behavior documented in tests.
- [x] Tests cover main checkout, linked worktree, and non-git fallback cases.
- [x] `go test ./...` passes.

## Context

This is the go/no-go proof for SPEC-040 Track 0's one-public-command strategy.
Later tasks add delegation and SQLite, but this task should keep the surface
small enough to review independently.

## Verification

```bash
go test ./...
go run ./cmd/loaf state path
```
