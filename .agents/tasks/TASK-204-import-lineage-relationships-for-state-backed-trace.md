---
id: TASK-204
title: Import lineage relationships for state-backed trace
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T18:07:56Z'
updated: '2026-05-28T18:13:33Z'
completed_at: '2026-05-28T18:13:33Z'
depends_on:
  - TASK-203
files:
  - internal/state/
  - .agents/tasks/TASK-204-import-lineage-relationships-for-state-backed-trace.md
verify: >-
  go test ./internal/state ./internal/cli && go test ./... && npm run build
done: >-
  Markdown migration imports explicit lineage relationships and session spark
  promotion links so `loaf trace` can show spark to idea to spec/task lineage
---

# TASK-204: Import lineage relationships for state-backed trace

## Description

Extend read-only Markdown migration so imported state includes relationship rows
for traceable lineage beyond task/spec edges. This covers explicit relationship
frontmatter on imported artifacts and common session journal spark-promotion
lines.

This task does not implement mutation commands, resolution commands, generated
exports, tags, bundles, or brainstorm-to-shaping lineage.

## Acceptance Criteria

- [x] Imported idea/spec/task/report/brainstorm Markdown can declare relationship fields such as `promoted_to`, `resolved_by`, `derived_from`, and `exported_as`.
- [x] Imported session `spark(...)` journal entries get stable human-facing spark aliases.
- [x] Imported `resolve(spark): <slug> -> promoted to <idea path>` journal entries create `promoted_to` relationships from the matching spark to the idea.
- [x] `loaf trace` can show spark -> idea -> spec/task lineage for an imported fixture.
- [x] Source Markdown remains unmodified.
- [x] Tests cover frontmatter relationship import, spark promotion import, and trace output over the resulting lineage.

## Verification

```bash
go test ./internal/state ./internal/cli
go test ./...
npm run build
```
