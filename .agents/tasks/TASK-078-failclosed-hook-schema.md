---
id: TASK-078
title: Add failClosed to hook schema and build output
spec: SPEC-020
status: todo
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
---

# TASK-078: Add `failClosed` to hook schema and build output

Add `failClosed` field to the hook system for security-critical hooks.

## Scope

**Type system:** Add `failClosed?: boolean` to `HookDefinition` in `cli/lib/build/types.ts`.

**Hook config:** Set `failClosed: true` on security-critical hooks in `config/hooks.yaml`:
- `check-secrets`
- `security-audit`
- `validate-push`

**Build output:** Emit `failClosed` field in generated hook configs:
- Claude Code `plugin.json`: native support
- Cursor `hooks.json`: native support
- Codex `.codex/hooks.json`: native support
- OpenCode runtime plugin: translate via subprocess error handling (non-zero, non-2 exit -> block when failClosed is true)

**Semantics:** When `failClosed: true`, hook crash/timeout/unexpected exit blocks the action instead of allowing it. Default: `false` (fail-open).

## Verification

- [ ] `failClosed` in `HookDefinition` type
- [ ] `hooks.yaml` has `failClosed: true` on security hooks
- [ ] Claude Code `plugin.json` emits `failClosed` field
- [ ] Cursor `hooks.json` emits `failClosed` field
- [ ] `npm run typecheck` passes
