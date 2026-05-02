---
title: "loaf release --post-merge guardrail 3 inverts conventional-commits semantics"
captured: 2026-05-02T16:21:40Z
status: raw
tags: [release, cli, conventional-commits, post-merge, guardrails]
related:
  - feedback_squash_subject_reflects_work
---

# `loaf release --post-merge` guardrail 3 inverts conventional-commits semantics

## Nugget

Guardrail 3 of `loaf release --post-merge` requires the squash merge commit on the base branch to have a subject matching `chore: release v<semver>`. This inverts conventional-commits semantics: a squash merge that ships a new feature is a `feat:`, not a `chore:`. `chore:` is reserved for cleanup/housekeeping commits, not for the merges themselves.

Symptom encountered during SPEC-034 release (2026-05-02):
- Branch shipped via PR #45 with squash subject `feat: SPEC-034 — refactor-deepen skill, grilling protocol, glossary KB convention (#45)`
- `loaf release --post-merge` refused to proceed: "guardrail 3 failed: HEAD subject does not match `chore: release v<semver>` shape — this is not a post-merge release commit"
- Recovered by manually tagging + creating GH release + deleting branch (Option 2)

## Problem/Opportunity

Two ways to fix:

1. **Reverse the guardrail expectation.** Drop the subject-shape check entirely, or replace it with a CHANGELOG-section check (`## [<semver>]` exists in CHANGELOG.md and corresponds to the version in `package.json`). The subject of the merge commit is incidental; what matters is that the version metadata is in place.
2. **Detect the version a different way.** Read `package.json` version, look for `[<that version>]` in CHANGELOG, tag at HEAD if both check out. No string-match on the merge subject required.

Option 2 is structurally cleaner — it tests what we actually care about (CHANGELOG + package.json agreement) instead of a proxy (subject shape).

## Initial Context

- **Convention being violated:** Per `feedback_squash_subject_reflects_work.md` (memory): a squash merge keeps the `feat:`/`fix:`/etc type that reflects the work shipped. `chore:` is post-merge bookkeeping. Past releases that landed with `chore: release vX.Y.Z (#N)` subjects were following an inverted convention; the SPEC-034 release deliberately fixed this for new merges.
- **Affected file:** `cli/lib/release/post-merge.ts` (or wherever guardrail 3's regex lives — search for "chore: release" string).
- **Affected guardrail #:** 3 (subject shape). Other guardrails (clean worktree, on base branch, version-file agreement, CHANGELOG section present, tag-not-yet-existing, GH-release-not-yet-existing, HEAD-untagged) remain valid.
- **Test impact:** `cli/lib/release/post-merge.test.ts` likely has fixtures asserting guardrail 3 fires on non-`chore: release` subjects. Those tests should be updated or deleted.
- **Hooks impact:** `validate-push` hook (the one that requires version + CHANGELOG bumped before pushing) is unrelated and stays as-is.

## Possible decomposition

1. Audit guardrail 3's intent (what was it actually trying to catch?)
2. Replace subject check with package.json + CHANGELOG cross-validation
3. Update tests
4. Update `release` skill SKILL.md if it documents the (now-wrong) subject convention
5. Backfill CHANGELOG entry for the fix itself

Estimated scope: small spec, one PR. Could be combined with other release-flow polish into a single SPEC.

## Sequencing

Not urgent — the manual fallback (Option 2: tag + GH release + branch cleanup) works and is what shipped v2.0.0-dev.37. But the guardrail bug will fire on every future PR that follows the (correct) convention, so it should land before the next release at the latest.
