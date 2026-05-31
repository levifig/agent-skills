---
id: TASK-243
title: Export spec Markdown from SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:29:11Z'
updated: '2026-05-28T22:32:44Z'
completed_at: '2026-05-28T22:32:44Z'
depends_on:
  - TASK-242
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-243-state-export-spec-markdown.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestExportSpecMarkdown|TestRunnerStateExportSpecMarkdown' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf state export spec <spec> --format markdown` renders deterministic spec
  Markdown from SQLite metadata, task counts, source provenance, imported body,
  and relationships without mutating the database or repository files.
---

# TASK-243: Export spec Markdown from SQLite state

## Description

Close the SPEC-040 generated-export gap for
`loaf state export spec <spec> --format markdown`. The export should be
generated from SQLite state and printed to stdout.

This task should not create persisted report rows or write repository Markdown.

## Acceptance Criteria

- [x] `loaf state export spec <spec> --format markdown` renders spec metadata,
  task counts, source provenance, imported body, and immediate relationships.
- [x] The export is deterministic and does not mutate SQLite state.
- [x] The export does not create repository files.
- [x] Missing and invalid SQLite state fail with the same state-export errors as
  existing export commands.
- [x] Tests cover the state package exporter and public `Runner` command path.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestExportSpecMarkdown|TestRunnerStateExportSpecMarkdown' -count=1
go test ./internal/state ./internal/cli
```
