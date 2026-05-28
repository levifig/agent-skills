---
id: TASK-165
title: loaf release --post-merge should use git tag -s (signed) explicitly
status: done
priority: P1
created: '2026-05-16T18:24:55.387Z'
updated: '2026-05-19T17:57:58.720Z'
completed_at: '2026-05-19T17:57:58.719Z'
---

# TASK-165: loaf release --post-merge should use git tag -s (signed) explicitly

## Context

During the GridSight v0.6.0 release (2026-05-15), `loaf release --post-merge` created an **unsigned** annotated tag despite the project having SSH signing fully configured for commits (`commit.gpgsign=true`, `gpg.format=ssh`, `user.signingkey=ssh-ed25519...`).

Root cause: the skill issues `git tag -a` (plain annotated), not `git tag -s` (signed). Without a user-level `tag.gpgsign=true` override, git does NOT auto-sign annotated tags even when `commit.gpgsign=true` is set — these are two separate config keys.

## Reproduction

1. Repo has `commit.gpgsign=true` (typical SSH-signing project).
2. `tag.gpgsign` is unset (default).
3. Run `loaf release --post-merge` after a release PR merges.
4. Observe: tag is created (`✓ Created tag vX.Y.Z`) but unsigned (`git tag -v vX.Y.Z` → `error: no signature found`).

## Why it matters

Projects that require signed commits typically also require signed tags. Relying on `tag.gpgsign` config to be set is brittle:

- It's a user-level config most developers don't set
- Silent fallback to unsigned tags violates the project's signing discipline
- The skill's brief / docs imply signed tags but don't actually enforce it

## Fix

Change the skill's tag-creation step from:

```bash
git tag -a "$TAG" -m "$MSG" "$COMMIT"
```

to:

```bash
git tag -s "$TAG" -m "$MSG" "$COMMIT"
```

This forces signing using whatever signing format is configured (`gpg.format=ssh` or `gpg.format=openpgp`). If the user has no signing key set up, `git tag -s` will fail loudly — which is the correct behavior for a release-gate skill.

## Alternative

If forcing `-s` is too aggressive for projects that don't sign anything, the skill could:

1. Check `commit.gpgsign` config — if true, use `git tag -s`; otherwise `git tag -a`.
2. OR add a `--unsigned-tag` flag for explicit opt-out.

The default should be signed, matching the project's commit-signing posture.

## Source incident

GridSight v0.6.0 release at <https://github.com/enlinehq/gridsight-core-gds/releases/tag/v0.6.0> had to:

1. Detect the unsigned tag manually (via `git tag -v`)
2. Delete the unsigned local tag (`git tag -d`)
3. Set `git config --global tag.gpgsign true` as a workaround
4. Rerun `loaf release --post-merge` — produced signed tag on rerun

The release succeeded but added two unnecessary steps and a (now-permanent) global config change that was a workaround, not a fix.

## Secondary observation

The push failure (`exit 128`) on the original run was likely correlated — the remote may have rejected the unsigned tag if branch-protection or org policy requires signed tags. The terse skill output didn't capture stderr, so the root cause remains uncharacterized. Worth surfacing more push-stderr in the skill output as a separate improvement.

## Acceptance Criteria

- [ ] `loaf release --post-merge` produces signed tags by default (verified via `git tag -v $TAG`)
- [ ] Works for both `gpg.format=ssh` and `gpg.format=openpgp`
- [ ] Fails loudly with a clear message if signing is required but no key is configured
- [ ] Add a regression test if test infra exists for the release skill

## Verification

```bash
# In a repo with commit signing configured but tag.gpgsign unset:
loaf release --post-merge
git tag -v v$(jq -r .version package.json)
# Should output: "Good 'git' signature with ED25519/RSA key ..."
# Must NOT output: "error: no signature found"
```

## Related: test-isolation finding (SPEC-036 / TASK-166 review, 2026-05-19)

While reviewing TASK-166, the reviewer traced 6 pre-existing test failures in `cli/commands/check.test.ts` to the **inverse** of this bug: tests fail when the *developer* has `tag.gpgsign=true` globally but the test repos don't have a signing key available.

**Concrete details:**

- **Failing tests (line numbers in `cli/commands/check.test.ts`):** 1086, 1937, 1970, 2004, 2034, 2071
- **Pattern:** each spot shells out to `git tag vX.Y.Z` (annotated, no `-m`, no `--no-sign`) against a fresh `git init` repo in a tmp dir
- **Root cause:** with `tag.gpgsign=true` in the user's global config (`~/.config/git/config` line 175-176 in the reviewer's env, with `gpg.program` set to `op-ssh-sign`), git attempts to sign and fails non-interactively → the tag command fails → assertions break
- **Verification:** repo-wide grep for `gpgsign|gpg-sign|--no-sign` shows no test infrastructure overrides this

**Recommended fix (alongside or after the primary fix):**

Make the test-side tag calls in `check.test.ts` hermetic by passing `-c tag.gpgsign=false` (or `--no-sign`):

```
git -c tag.gpgsign=false tag vX.Y.Z   # OR
git tag --no-sign vX.Y.Z
```

Either prevents the tests from picking up the developer's global config. This is independent of the primary `loaf release` fix — but the two share the same `tag.gpgsign` interaction surface, so it's natural to fix both in the same PR.

**Why this matters:** developers who follow signing best practices (the kind most likely to ship Loaf) will have `tag.gpgsign=true` globally for their own work. The current test suite punishes that posture by failing tests that have nothing to do with signing. Hermetic tests should not depend on global git config.
