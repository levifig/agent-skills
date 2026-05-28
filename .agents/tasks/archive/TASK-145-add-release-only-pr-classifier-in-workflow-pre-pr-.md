---
id: TASK-145
title: Add release-only PR classifier in workflow-pre-pr (strict diff allowlist)
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:28:14.853Z'
updated: '2026-04-29T19:55:12.727Z'
depends_on:
  - TASK-143
  - TASK-144
completed_at: '2026-04-29T19:55:12.726Z'
---

# TASK-145: Add release-only PR classifier in workflow-pre-pr (strict diff allowlist)

## Description

Add a release-only PR classifier in `workflow-pre-pr` (`cli/commands/check.ts`) that recognizes PRs whose diff against the base contains only `CHANGELOG.md` plus known version-file paths (including monorepo declarations from `.agents/loaf.json` per TASK-143). When the diff qualifies AND `CHANGELOG.md` at HEAD has a non-empty `## [<version>]` section matching the version-file values, the empty `[Unreleased]` block does not block PR creation. Shares base-ref resolution logic with TASK-144. Implements SPEC-031 Task 9.

## Acceptance Criteria

- [ ] PR with diff containing exclusively `CHANGELOG.md` + known version-file paths AND a non-empty `## [<version>]` matching the version files passes `workflow-pre-pr` without warning.
- [ ] Any other file in the diff disqualifies the release-only classification (strict allowlist, not a heuristic).
- [ ] Monorepo version files declared in `.agents/loaf.json` are recognized by the classifier (shared path-set with TASK-143).
- [ ] Base-ref resolution in the classifier reuses the resolver implemented for `--pre-merge` (TASK-144) — no duplicated logic.
- [ ] When the version in `## [<version>]` does not match the version-file values, the empty-`[Unreleased]` bypass does not apply.
- [ ] Fixture: branch whose only diff is a version bump and `[Unreleased]` → `[X.Y.Z]` move passes `workflow-pre-pr` without a stub-restore commit.

## Verification

```bash
npm run typecheck && npm run test -- check.test.ts
```
