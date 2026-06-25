---
id: TASK-353
title: Implement Tier-1 SQLite search
spec: SPEC-043
status: done
priority: P1
created: '2026-06-24T13:04:19Z'
updated: '2026-06-24T14:16:44Z'
completed_at: '2026-06-24T14:16:44Z'
depends_on:
  - TASK-352
files:
  - internal/state/search.go
  - internal/cli/cli.go
  - internal/cli/cli_reference.go
  - content/skills/cli-reference/SKILL.md
  - dist/
  - plugins/
  - .agents/tasks/TASK-353-tier1-sqlite-search.md
verify: >-
  go test ./internal/cli ./internal/state -run 'Search|ArtifactBody|Journal' -count=1
  && npm run build
done: >-
  `loaf search` queries current-project SQLite artifact bodies and journal entries
  through FTS5, supports explicit all-project scope, returns tiered JSON, and
  redacts secret-like snippets.
---

# TASK-353: Implement Tier-1 SQLite search

## Description

Add SPEC-043 Track 4: `loaf search` over SQLite-resident artifact bodies and
journal entries using FTS5.

## Acceptance Criteria

- [x] `loaf search <query>` returns current-project Tier-1 hits from artifact bodies and journal entries.
- [x] `--all-projects` expands scope explicitly; default search does not cross project boundaries.
- [x] JSON output includes a `tier` discriminator and stable entity addressing.
- [x] Ranking uses SQLite FTS ranking consistently enough for deterministic tests.
- [x] Snippets omit or redact planted secret-like content.
- [x] Editing a body removes old-term matches and adds new-term matches.
- [x] CLI reference output documents `loaf search` and is regenerated.

## Verification

```bash
go test ./internal/cli ./internal/state -run 'Search|ArtifactBody|Journal' -count=1
npm run build
```
