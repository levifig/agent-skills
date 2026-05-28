---
id: TASK-182
title: Verify release build artifacts after post-bump commits
status: done
priority: P2
created: '2026-05-19T18:56:00.763Z'
updated: '2026-05-19T19:02:46.238Z'
completed_at: '2026-05-19T19:02:46.237Z'
---

# TASK-182: Verify release build artifacts after post-bump commits

## Description

Update release workflow guidance so generated artifacts are rebuilt and checked
again if commits land after the release bump.

## Acceptance Criteria

- [ ] Release skill instructs a post-bump artifact freshness check.
- [ ] Guidance tells the user to commit regenerated artifacts when stale.

## Verification

```bash
npm run build
```
