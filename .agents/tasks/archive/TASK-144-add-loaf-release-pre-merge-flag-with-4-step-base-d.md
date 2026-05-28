---
id: TASK-144
title: Add loaf release --pre-merge flag with 4-step base detection
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:28:14.798Z'
updated: '2026-04-29T19:29:33.989Z'
depends_on:
  - TASK-138
  - TASK-139
  - TASK-140
  - TASK-141
completed_at: '2026-04-29T19:29:33.988Z'
---

# TASK-144: Add loaf release --pre-merge flag with 4-step base detection

## Description

Add `loaf release --pre-merge` in `cli/commands/release.ts` which bundles `--no-tag --no-gh --base <auto-detected>` and produces a `chore: release v<semver>` commit accepted by both TASK-140's hook and TASK-141's regex. Base detection follows a strict 4-step precedence: explicit `--base <ref>` flag, then open PR base via `gh pr view --head <current> --json baseRefName`, then `git config loaf.release.base`, then default branch via `gh repo view --json defaultBranchRef` (falling back to `origin/HEAD`). Implements SPEC-031 Task 6.

## Acceptance Criteria

- [ ] `loaf release --pre-merge` produces a single commit with subject `chore: release v<semver>` and does not tag, does not push, does not create a GH release.
- [ ] Base detection step 1 wins: explicit `--base <ref>` overrides PR-base, git-config, and default-branch detection.
- [ ] Base detection step 2 wins over steps 3-4: open PR's base (`gh pr view --head <current> --json baseRefName`) is used when no explicit `--base` is set.
- [ ] Base detection step 3 wins over step 4: `git config loaf.release.base` is used when no PR exists and no flag is set.
- [ ] Base detection step 4 fallback: default branch via `gh repo view --json defaultBranchRef` (or `origin/HEAD`) is used when steps 1-3 yield nothing.
- [ ] Each precedence tier is covered by an isolated fixture test plus a combined-precedence test asserting the full ordering.
- [ ] Works in both curated and auto-generated changelog paths.

## Verification

```bash
npm run typecheck && npm run test -- release
```
