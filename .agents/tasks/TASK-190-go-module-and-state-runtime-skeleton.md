---
id: TASK-190
title: Add Go module and state runtime skeleton
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T15:13:09Z'
updated: '2026-05-28T15:43:13Z'
depends_on: []
files:
  - go.mod
  - cmd/loaf/main.go
  - internal/state/
  - internal/project/
verify: go test ./... && go vet ./...
done: >-
  Go module builds and tests with a minimal state/runtime package boundary, no
  SQLite dependency selected yet, and existing TypeScript CLI behavior
  unchanged
completed_at: '2026-05-28T15:43:13Z'
---

# TASK-190: Add Go module and state runtime skeleton

## Description

Introduce the Go runtime foundation for SPEC-040 without changing the shipped
public command yet. This task creates the minimal Go module, command entrypoint,
and internal package boundaries needed for later `loaf state` work.

Keep this intentionally thin. The goal is to establish structure, test harness,
and project-root plumbing, not to implement SQLite behavior or replace existing
TypeScript commands.

## Acceptance Criteria

- [x] `go.mod` exists with the chosen module path and Go version.
- [x] `cmd/loaf/main.go` compiles and has a minimal command-dispatch skeleton.
- [x] `internal/state/` exists as the future SQLite/state boundary with at least one small testable unit.
- [x] `internal/project/` exists for project/worktree identity helpers that later Track 0/A tasks can extend.
- [x] No SQLite driver is added in this task.
- [x] Existing TypeScript CLI entrypoints remain unchanged.
- [x] `go test ./...` passes.
- [x] `go vet ./...` passes.

## Context

SPEC-040 Track 0 starts the Go runtime foundation before SQLite storage.
ADR-014 makes Go the stateful runtime direction, but explicitly rejects a
big-bang CLI rewrite.

## Verification

```bash
go test ./...
go vet ./...
```
