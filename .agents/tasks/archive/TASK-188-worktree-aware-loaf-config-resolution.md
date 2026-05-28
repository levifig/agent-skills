---
id: TASK-188
title: Make agents-config.ts worktree-aware (follow .moved-to back-pointer)
spec: SPEC-042
status: done
priority: P1
created: '2026-05-28T01:53:43.449Z'
updated: '2026-05-28T08:59:26.504Z'
files:
  - cli/lib/config/agents-config.ts
  - cli/lib/config/agents-config.test.ts
  - cli/lib/tasks/resolve.ts
  - cli/commands/release.test.ts
verify: >-
  npm run typecheck && npm run test -- cli/lib/config/agents-config.test.ts
  cli/commands/release.test.ts
done: >-
  readLoafConfig and writeLoafConfigRaw follow .moved-to back-pointer to the
  main worktree's .agents/loaf.json; loaf release --pre-merge succeeds in a
  migrated linked worktree without --version-file
completed_at: '2026-05-28T08:59:26.503Z'
---

# TASK-188: Make agents-config.ts worktree-aware (follow .moved-to back-pointer)

## Description

`cli/lib/config/agents-config.ts` reads and writes `.agents/loaf.json` directly via `join(projectRoot, ".agents", "loaf.json")`. Under SPEC-036, a linked worktree's `.agents/` contains only a `.moved-to` back-pointer to the main worktree's `.agents/`. The session/task code path uses `findAgentsDir` (which is SPEC-036-aware via `findMainWorktreeRoot`), but the config-loader path does not. This causes `loaf release --pre-merge` to fail with "No version files found" in centralized linked worktrees.

Fix both reads and writes:

1. Add an internal helper in `agents-config.ts` that resolves the effective `loaf.json` path. In a linked worktree, return `findMainWorktreeRoot(projectRoot) + "/.agents/loaf.json"`. In a single-checkout repo, return the existing path. Fall through to current behavior on probe-null.
2. Route `loafConfigPath`, `readLoafConfig`, `writeLoafConfigRaw`, and `mergeLoafConfigIntegrations` through the helper. Reuse `findMainWorktreeRoot` from `cli/lib/tasks/resolve.ts` — do not reimplement.
3. Symmetric write fix: `writeLoafConfigRaw` must `mkdirSync` the main worktree's `.agents/` (not the linked worktree's) when writing.

This heals all six consumers of `readLoafConfig` (release pre-merge, post-merge, release-only-pr, kb-glossary, MCP detection, MCP recommendations) in one change — no per-caller refactor.

## Acceptance Criteria

- [ ] New internal helper resolves effective `loaf.json` path via `findMainWorktreeRoot` reuse.
- [ ] `loafConfigPath`, `readLoafConfig`, `writeLoafConfigRaw`, `mergeLoafConfigIntegrations` all route through the helper.
- [ ] Unit test: `readLoafConfig` from a worktree containing only `.moved-to` returns the main worktree's config (tmp fixture).
- [ ] Unit test: `writeLoafConfigRaw` / `mergeLoafConfigIntegrations` from a centralized worktree write to the main worktree's `loaf.json` and do NOT create a stray `loaf.json` next to `.moved-to`.
- [ ] Unit test: single-checkout repo behavior unchanged (existing `agents-config.test.ts` cases pass without modification).
- [ ] Integration test: `loaf release --pre-merge --base <prev-tag>` in a migrated linked worktree completes the pre-merge phase without `--version-file` override. Replicates the GridSight v0.16.0 repro.
- [ ] `npm run typecheck` passes.
- [ ] `npm run test` passes.

## Verification

```bash
npm run typecheck
npm run test -- cli/lib/config/agents-config.test.ts cli/commands/release.test.ts
```

Manual repro check (in a scratch repo):

```bash
git worktree add -b release/test .claude/worktrees/release-test origin/main
cd .claude/worktrees/release-test
loaf migrate worktree-storage --apply
loaf release --pre-merge --base <prev-tag>   # should succeed
```

## Context

See [SPEC-042](../specs/SPEC-042-worktree-aware-config-and-session-fallback.md) — Track A.

Related: SPEC-036 (worktree centralization), SPEC-032 (session resolution). The fix pattern mirrors what `findAgentsDir` already does in `cli/lib/tasks/resolve.ts:168`.

Out of scope (do not include in this task):
- Track B (session fallback) — separate task TASK-189.
- Changes to `findMainWorktreeRoot` itself.
- Other `.agents/`-relative paths in release commands.
