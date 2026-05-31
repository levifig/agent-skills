---
id: TASK-242
title: Export session Markdown from SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:24:49Z'
updated: '2026-05-28T22:28:12Z'
completed_at: '2026-05-28T22:28:12Z'
depends_on:
  - TASK-241
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-242-state-export-session-markdown.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestExportSessionMarkdown|TestRunnerStateExportSessionMarkdown' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf state export session <session> --format markdown` renders deterministic
  session Markdown from SQLite metadata, source provenance, journal entries, and
  relationships without mutating the database or repository files.
---

# TASK-242: Export session Markdown from SQLite state

## Description

Close the SPEC-040 generated-export gap for
`loaf state export session <session> --format markdown`. The export should be
generated from SQLite state and printed to stdout.

This task should not create persisted report rows, write repository Markdown, or
try to solve transcript enrichment/redaction policy.

## Acceptance Criteria

- [x] `loaf state export session <session> --format markdown` renders session
  metadata, source provenance, journal entries, and immediate relationships.
- [x] The export is deterministic and does not mutate SQLite state.
- [x] The export does not create repository files.
- [x] Missing and invalid SQLite state fail with the same state-export errors as
  existing export commands.
- [x] Tests cover the state package exporter and public `Runner` command path.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestExportSessionMarkdown|TestRunnerStateExportSessionMarkdown' -count=1
go test ./internal/state ./internal/cli
```
