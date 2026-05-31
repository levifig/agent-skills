---
id: TASK-231
title: Export external-safe triage Markdown from SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:25:15Z'
updated: '2026-05-28T21:30:20Z'
completed_at: '2026-05-28T21:30:20Z'
depends_on:
  - TASK-230
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-231-state-export-triage-markdown.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  When SQLite state is initialized, `loaf state export triage --format markdown`
  writes a deterministic external-safe Markdown summary of idea, spark, and
  brainstorm triage state to stdout, strips SPEC-038-banned Loaf internals, and
  refuses missing or invalid state.
---

# TASK-231: Export external-safe triage Markdown from SQLite state

## Description

Add the first external-targeted Markdown export for SPEC-040 Track D:
`loaf state export triage --format markdown`.

The export should summarize SQLite-backed triage state across ideas, sparks,
and brainstorms without exposing Loaf-private artifact IDs, paths, tracks, or
phases. It should write to stdout only and remain deterministic for unchanged
state.

This task does not implement spec/session/release-readiness exports, file output,
or recording export rows in the `exports` table.

## Acceptance Criteria

- [x] `loaf state export triage --format markdown` parses as a native Go state command.
- [x] Output is Markdown written to stdout, not a repository file.
- [x] Output is explicitly external-safe and does not include `SPEC-*`, `TASK-*`, `.agents/`, `Track N`, or `Phase N`.
- [x] Output summarizes ideas, sparks, and brainstorms by status using SQLite state.
- [x] Output is deterministic for unchanged triage rows.
- [x] Generated content is validated against the SPEC-038 banned-pattern set before returning.
- [x] Missing SQLite state is refused with a clear initialization/migration message.
- [x] Invalid SQLite state is refused with a `loaf state doctor` message.
- [x] Unsupported export kinds and formats still return clear errors.
- [x] Tests cover state-layer Markdown shape, leak stripping/validation, CLI output, missing/invalid state, unsupported kind/format errors, and no repository writes.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run typecheck
npm run build
```

Built-binary smoke:

```bash
XDG_STATE_HOME=<temp> bin/loaf state migrate markdown --apply
XDG_STATE_HOME=<temp> bin/loaf state export triage --format markdown
```
