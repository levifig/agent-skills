---
id: TASK-020
title: Richer version output for loaf and skill-level version detection
status: done
priority: P3
created: '2026-01-24T15:50:00.000Z'
updated: '2026-03-27T18:16:58.589Z'
files:
  - cli/commands/version.ts
verify: loaf version && loaf --version
done: >-
  `loaf version` shows rich version info; stale session detection still works
  via skill frontmatter
completed_at: '2026-03-27T18:16:58.588Z'
---

# TASK-020: Version command — richer output

## Description

`loaf --version` already exists and shows the version. This task adds a richer `loaf version` subcommand that shows more context (build date, installed targets, etc.) and considers whether the skill-level `/loaf:version` command is still needed now that the CLI handles versioning.

## Current State

- `loaf --version` works (from SPEC-008)
- Build system injects version into skill frontmatter
- Plugin.json has version
- No `loaf version` subcommand with richer output

## Open Questions

- Is a skill-level `/loaf:version` still needed, or does `loaf --version` cover it?
- What richer info would `loaf version` show? (installed targets, build date, content stats?)

## Acceptance Criteria

- [ ] `loaf version` shows version + additional context
- [ ] Determine if skill-level version command is still warranted

## Work Log

<!-- Updated by session as work progresses -->
