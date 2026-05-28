---
id: TASK-086
title: Verify loaf binary accessibility from all hook environments
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
completed_at: '2026-04-04T16:41:22.296Z'
---

# TASK-086: Verify `loaf` binary accessibility from all hook environments

End-to-end verification of the full SPEC-020 implementation.

## Implementation

### Binary accessibility
Verified `loaf` binary is callable from:
- Claude Code plugin hooks (`${CLAUDE_PLUGIN_ROOT}/bin/loaf`) — via bundled binary in `plugins/loaf/bin/loaf`
- Cursor hooks (PATH-based) — via `loaf` on PATH
- Codex hooks (PATH-based) — via `loaf` on PATH
- OpenCode/Amp runtime plugins (subprocess) — via `loaf` on PATH

### Install verification
`loaf install` now warns if binary not on PATH for targets that need it (opencode, cursor, codex, amp). Claude Code uses plugin-bundled binary and doesn't require PATH.

### Full build + install cycle
- `npm run build:cli` succeeds
- `loaf build` for all 6 targets succeeds
- Generated binaries and configs validated

### Spec test conditions
All test conditions from SPEC-020 across all 4 phases:
- Phase 1: shared intermediate exists, parity checks pass ✓
- Phase 2: descriptions truncated, structural convention applied, hooks migrated ✓
- Phase 3: `loaf check` works, `loaf session` works, direct commands in configs, no bash wrappers ✓
- Phase 4: Amp target builds, Codex hooks installed, fenced sections work, binary accessible ✓

## Verification

- [x] `loaf check --hook check-secrets` callable from each target's hook environment
- [x] `loaf session start` callable from each target's session hook
- [x] `loaf install` warns if binary not on PATH for PATH-dependent targets
- [x] Full `loaf build` for all 6 targets succeeds
- [x] `npm run typecheck` passes
- [x] All 75 tests pass
- [x] All SPEC-020 test conditions pass
