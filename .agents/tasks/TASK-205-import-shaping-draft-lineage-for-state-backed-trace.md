---
id: TASK-205
title: Import shaping draft lineage for state-backed trace
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:15:57Z'
updated: '2026-05-28T18:20:31Z'
completed_at: '2026-05-28T18:20:31Z'
depends_on:
  - TASK-204
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-205-import-shaping-draft-lineage-for-state-backed-trace.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Markdown migration imports shaping draft artifacts and lineage relationships so
  `loaf trace` can show brainstorm to idea to shaping draft to spec/task
  provenance
---

# TASK-205: Import shaping draft lineage for state-backed trace

## Description

Extend the read-only Markdown migration and trace read model so shaping draft
artifacts are first-class imported entities. This closes the remaining Track B
lineage fixture for brainstorm -> idea -> shaping draft -> finalized spec ->
task.

This task does not implement mutation commands, generated exports, tags,
bundles, or triage closure behavior.

## Acceptance Criteria

- [x] Imported `.agents/drafts/*.md` files that are explicitly shaping drafts become `shaping_draft` rows with source provenance.
- [x] Shaping draft aliases can be resolved by `loaf trace`.
- [x] Relationship frontmatter can point to shaping drafts and from shaping drafts to specs/tasks.
- [x] `loaf trace` can show brainstorm -> idea -> shaping draft -> finalized spec -> task lineage for an imported fixture.
- [x] Markdown migration preview recognizes shaping draft files as known imported artifacts instead of skipped files.
- [x] Source Markdown remains unmodified.
- [x] Tests cover shaping draft import, trace output, preview counts/skips, and source immutability.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
