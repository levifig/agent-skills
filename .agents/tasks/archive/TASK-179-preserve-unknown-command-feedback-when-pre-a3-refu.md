---
id: TASK-179
title: Preserve unknown-command feedback when pre-A3 refusal fires
status: done
priority: P2
created: '2026-05-19T18:55:59.371Z'
updated: '2026-05-19T19:02:44.909Z'
completed_at: '2026-05-19T19:02:44.908Z'
---

# TASK-179: Preserve unknown-command feedback when pre-A3 refusal fires

## Description

When the pre-A3 refusal nudge blocks a linked worktree command, unknown
top-level commands should still show an unknown-command diagnostic so users can
distinguish typo feedback from migration feedback.

## Acceptance Criteria

- [ ] Unknown command output appears before the SPEC-036 refusal message.
- [ ] Known commands still get the existing refusal behavior.

## Verification

```bash
npm test -- cli/commands/migrate.e2e.test.ts
```
