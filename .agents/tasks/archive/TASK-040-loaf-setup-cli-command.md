---
id: TASK-040
title: loaf setup CLI command
spec: SPEC-013
status: done
priority: P1
created: '2026-03-27T02:59:43.495Z'
updated: '2026-03-27T11:02:15.585Z'
completed_at: '2026-03-27T11:02:15.584Z'
---

# TASK-040: loaf setup CLI command

## Description

Create a new `loaf setup` CLI command that wraps the mechanical bootstrap into one step. Follows existing Commander.js patterns (`registerSetupCommand()` in `cli/commands/setup.ts`, registered in `cli/index.ts`).

The command runs: optional dir creation → `loaf init` → `loaf build` → `loaf install --to all` → handoff message. All sub-commands are already implemented; this wraps them into a single invocation.

**Files:** `cli/commands/setup.ts`, `cli/index.ts`

## Acceptance Criteria

- [ ] `loaf setup` in a new directory runs init + build + install in sequence
- [ ] `loaf setup ./my-project` creates target directory first, then runs the sequence
- [ ] `loaf setup` in an existing Loaf project (has `loaf.json`) is idempotent
- [ ] Prints handoff message: "Setup complete. Run `/bootstrap` in Claude Code to set up your project."
- [ ] Fails fast: if any step fails, reports the error clearly and stops
- [ ] Unit tests pass

## Verification

```bash
npx tsx cli/index.ts setup --help
npm run typecheck
npm run test
```

## Context
See SPEC-013 — `loaf setup` CLI Command section.
