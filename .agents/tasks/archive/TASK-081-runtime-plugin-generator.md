---
id: TASK-081
title: Create shared runtime plugin generator (OpenCode + Amp)
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
completed_at: '2026-04-04T16:41:22.296Z'
---

# TASK-081: Create shared runtime plugin generator (OpenCode + Amp)

Unified runtime plugin generator for TypeScript-plugin harnesses.

## Implementation

Created `cli/lib/build/lib/hooks/runtime-plugin.ts` with:
- `RuntimePlatform` interface supporting OpenCode and Amp adapters
- `generateRuntimePlugin()` function with shared core + platform-specific adapters
- Event wiring for preTool, postTool, sessionStart, sessionEnd
- Hook command execution via subprocess calls to `loaf check` and `loaf session`
- Platform-specific `extraEvents` callback for OpenCode's extended event surface

## Verification

- [x] Runtime plugin generates valid TypeScript for OpenCode
- [x] Runtime plugin generates valid TypeScript for Amp with experimental header
- [x] Plugins call `loaf check` and `loaf session` via subprocess
- [x] Session lifecycle events mapped correctly
- [x] All tests pass

**Shared core:** `runHook()` (calls `loaf check`/`loaf session` as subprocess), `matchesTool()`, hook grouping by matcher, exit code interpretation (0=allow, 2=block, 1=error), `failClosed` handling.

**OpenCode adapter:** Maps `tool.execute.before/after`, `session.created/ended`. `extraEvents` handles `session.compacting` -> `loaf session log "compact: context compaction triggered"`.

**Rewrite OpenCode target:** Replace `generateHooks()`/`generateHooksJs()` in `opencode.ts` with call to shared `generateRuntimePlugin()`. Output becomes `hooks.ts` (TypeScript).

## Constraints

- Amp adapter created here but Amp target wiring is TASK-083
- OpenCode `hooks.js` -> `hooks.ts` filename change (document in release notes)
- `extraEvents` callback for platform-specific events beyond shared core
- Each runtime plugin calls `loaf check`/`loaf session` as subprocess, not inline logic

## Verification

- [ ] `loaf build --target opencode` produces valid `dist/opencode/plugins/hooks.ts`
- [ ] Plugin calls `loaf check` as subprocess
- [ ] Plugin handles `session.compacting` via `extraEvents`
- [ ] Generated TypeScript is valid
- [ ] `failClosed` handling in subprocess error path
- [ ] `npm run typecheck` passes
