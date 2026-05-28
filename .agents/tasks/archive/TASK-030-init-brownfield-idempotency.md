---
id: TASK-030
title: '`loaf init` — brownfield support + idempotency polish'
spec: SPEC-007
status: done
priority: P2
created: '2026-03-16T16:27:15.461Z'
updated: '2026-03-17T00:16:46.349Z'
depends_on:
  - TASK-028
files:
  - cli/commands/init.ts
verify: 'loaf init && loaf init (second run shows all-green, creates nothing)'
done: >-
  Re-running `loaf init` shows status with checkmarks, reports brownfield
  patterns, suggests consolidation
completed_at: '2026-03-17T00:16:46.349Z'
---

# TASK-030: loaf init — brownfield support + idempotency

## Description

Polish `loaf init` for brownfield projects and repeated runs. Detect existing docs, report status, suggest (don't auto-migrate).

## Brownfield detection

- Existing README.md, architecture docs, ADRs
- Existing `.agents/` structure (partial setup)
- Existing CLAUDE.md or .claude/ directory
- Report what exists and suggest how it maps to Loaf structure

## Idempotent output

Second run of `loaf init` should show:
```
loaf init

  ✓ .agents/ (exists)
  ✓ .agents/AGENTS.md (exists)
  ✓ docs/knowledge/ (exists)
  ✓ docs/VISION.md (exists)
  ...
  All set — nothing to create.
```

## Acceptance Criteria

- [ ] Reports existing patterns in brownfield projects
- [ ] Suggests consolidation without auto-migrating
- [ ] Re-running shows status with checkmarks, creates nothing new
- [ ] Output is clean, colored, consistent with loaf build/install

## Context

See SPEC-007 for full context. Circuit breaker 100%.

## Work Log

<!-- Updated by session as work progresses -->
