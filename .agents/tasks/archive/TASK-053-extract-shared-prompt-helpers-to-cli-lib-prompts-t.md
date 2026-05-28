---
id: TASK-053
title: Extract shared prompt helpers to cli/lib/prompts.ts
spec: SPEC-012
status: done
priority: P1
created: '2026-03-28T23:36:06.000Z'
updated: '2026-03-29T02:21:17.749Z'
completed_at: '2026-03-29T02:21:17.748Z'
---

# TASK-053: Extract shared prompt helpers to cli/lib/prompts.ts

## Description

Extract `askYesNo()` and `askChoice()` from `cli/commands/release.ts` into a shared `cli/lib/prompts.ts` module. Add non-TTY detection so piped invocations skip prompts automatically.

## What to do

1. Create `cli/lib/prompts.ts` with:
   - `askYesNo(question: string): Promise<boolean>` — existing logic from release.ts
   - `askChoice<T>(question: string, options: T[], format: (t: T) => string, defaultChoice: T): Promise<T>` — generalized from release.ts's BumpType-specific version
   - `isTTY(): boolean` — wraps `process.stdout.isTTY` check
2. Update `cli/commands/release.ts` to import from the shared module
3. Tests for the shared module (non-TTY returns default, TTY behavior)

## Acceptance Criteria

- [ ] `cli/lib/prompts.ts` exports `askYesNo`, `askChoice`, `isTTY`
- [ ] `release.ts` imports from shared module (no duplicate implementations)
- [ ] `isTTY()` returns false when stdout is not a TTY
- [ ] `askYesNo()` returns false in non-TTY mode
- [ ] `askChoice()` returns default in non-TTY mode
- [ ] `npm run typecheck` passes
- [ ] Existing release command still works

## Verification

```bash
npm run typecheck && npm run test
```
