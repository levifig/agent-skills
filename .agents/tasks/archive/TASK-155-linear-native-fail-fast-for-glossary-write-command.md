---
id: TASK-155
title: Linear-native fail-fast for glossary write commands
spec: SPEC-034
status: done
priority: P1
created: '2026-05-02T01:25:29.099Z'
updated: '2026-05-02T01:25:29.099Z'
depends_on:
  - TASK-152
  - TASK-154
completed_at: '2026-05-02T03:35:00.000Z'
---

# TASK-155: Linear-native fail-fast for glossary write commands

## Description

Detect Linear-native mode (read `integrations.linear.enabled` from `.agents/loaf.json`) and gate all glossary write commands with fail-fast behavior per SPEC-034 no-go. Read commands work in both modes. Per ADR-011, glossary mutation belongs to the deferred artifact-taxonomy spec; this guard prevents silent state corruption when someone toggles Linear-native mid-flight.

## File Hints

- `cli/lib/kb/glossary.ts` or `cli/commands/kb.ts` (mode-detection helper, possibly extracted)
- `cli/commands/kb.test.ts`

## Acceptance Criteria

- [ ] When `.agents/loaf.json` has `integrations.linear.enabled: true`: all of `upsert`, `propose`, `stabilize` exit non-zero with the exact message `"Linear-native glossary writes pending artifact-taxonomy spec — local mode only for now."`
- [ ] When Linear disabled or missing: writes work normally
- [ ] `list` and `check` work regardless of mode (no Linear dependency for reads)
- [ ] Mode detection reads the file fresh per invocation (handles toggle without restart)
- [ ] Tests: mock loaf.json in both states; verify each command's behavior

## Verification

```bash
npm test -- cli/commands/kb.test.ts
# Manual: toggle integrations.linear.enabled in .agents/loaf.json, retry upsert, observe fail-fast
```
