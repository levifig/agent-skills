---
id: TASK-187
title: Archive release follow-up ideas implemented by this branch
status: done
priority: P2
created: '2026-05-19T19:11:09.003Z'
updated: '2026-05-19T19:19:56.286Z'
completed_at: '2026-05-19T19:19:56.286Z'
---

# TASK-187: Archive release follow-up ideas implemented by this branch

## Description

Two raw release ideas were already handled by this branch's release-flow
follow-up work. Mark them archived with explicit task resolutions.

## Acceptance Criteria

- [x] `release --post-merge` guardrail idea is archived and linked to `TASK-178`.
- [x] Release post-bump rebuild idea is archived and linked to `TASK-182`.

## Verification

```bash
rg -n "status: archived|resolution: implemented in TASK-(178|182)" .agents/ideas/20260502-162140-release-post-merge-guardrail-inverts-conventional-commits.md .agents/ideas/20260411-001500-release-post-bump-rebuild.md
```
