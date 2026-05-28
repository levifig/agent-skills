---
id: TASK-142
title: Add loaf release --post-merge flag with 8-point guardrails + light idempotency
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:27:52.150Z'
updated: '2026-04-29T19:43:16.141Z'
depends_on:
  - TASK-141
completed_at: '2026-04-29T19:43:16.140Z'
---

# TASK-142: Add loaf release --post-merge flag with 8-point guardrails + light idempotency

## Description

Add `loaf release --post-merge` in `cli/commands/release.ts` which verifies HEAD against an 8-point guardrail checklist, then tags, pushes the tag, creates a GH release from the CHANGELOG section, checks out the base branch and pulls, and best-effort deletes the local + remote feature branch. The feature branch name is captured via `git symbolic-ref --short HEAD` before the checkout step. Action sequence is `git tag -a` → `git push origin v<version>` → `gh release create v<version>` → `git checkout <base> && git pull --rebase` → branch deletion. Implements SPEC-031 Task 7.

## Acceptance Criteria

- [ ] All 8 guardrails are checked before any tag/release action: clean worktree, on base branch (or fast-forwardable to it), HEAD subject matches `^chore: release v<semver>( \(#\d+\))?$`, version in commit subject matches version in every detected version file, `git diff HEAD^ HEAD --name-only` includes `CHANGELOG.md` + at least one version file, CHANGELOG has non-empty `## [<version>]` section, no existing local/remote tag or GH release for `v<version>`, HEAD itself not already tagged.
- [ ] Each guardrail has a fixture test that aborts with the expected message and does not tag/release.
- [ ] Feature branch identity is captured via `git symbolic-ref --short HEAD` BEFORE any checkout, stored in a local variable, and reused for the delete step.
- [ ] Action sequence is exactly: `git tag -a v<version> -m "Release <version>"` → `git push origin v<version>` → `gh release create v<version>` → `git checkout <base> && git pull --rebase` → best-effort branch delete (local + remote).
- [ ] Idempotency abort messages match the spec verbatim: tag-local (`run \`git tag -d v<version>\` and rerun`), tag-remote (`run \`git push origin :refs/tags/v<version>\` and rerun`), GH release exists (`visit the release page and delete it manually before rerunning`), HEAD already tagged (`HEAD is already tagged as <existing-tag>; this is not a fresh post-merge state`).
- [ ] Branch-deletion failure warns and exits 0 once tag + GH release have succeeded; tag/release themselves verify before acting.
- [ ] Partial-failure idempotency fixture: prior partial run created tag locally but failed before GH release; rerun aborts at guardrail 7 with actionable error and succeeds after manual cleanup.

## Verification

```bash
npm run typecheck && npm run test -- release
```
