---
id: SPEC-031
title: Release Flow Hardening
source: direct
created: 2026-04-24T21:30:00Z
status: drafting
---

# SPEC-031: Release Flow Hardening

## Problem Statement

The standard `bump → push → PR → squash-merge → tag` release path requires manual choreography around hook and CLI edge cases: ceremony commits, commit-message rewording, and multi-step post-merge orchestration. Five concrete friction points were observed shipping v2.0.0-dev.30 (PR #35):

1. `validate-commit` hook regex false-positives on legitimate path tokens (e.g., `.claude/CLAUDE.md` in a commit body mentioning the AGENTS.md-consolidation work). Required dancing around the regex to get the commit through.
2. `loaf release` moves entries from `[Unreleased]` into a versioned `## [X.Y.Z]` section but does not re-insert the `_No unreleased changes yet._` stub. `workflow-pre-pr` then blocks `gh pr create` on an empty `[Unreleased]`. Required a 5th ceremony commit to unblock PR creation.
3. Pre-merge bump flags (`--no-tag --no-gh --base main`) are non-obvious. The canonical pre-merge invocation lives in skill docs, not in CLI ergonomics.
4. `workflow-pre-pr` and `validate-push` have escape-hatch logic for tagged HEAD commits but no way to recognize a legitimate pre-merge `release:` commit — the exact artifact `loaf release` produces before merge.
5. Post-merge tag + GitHub release + branch cleanup is a manual sequence of commands. The release skill orchestrates it, but there is no single CLI invocation that closes the loop.

## Strategic Alignment

- **Vision:** Loaf's "structured execution" pillar — every change flows through a deliberate pipeline without friction. A frictionless release path is load-bearing for this pillar.
- **Personas:** Primarily the solo developer (the release flow is the high-frequency surface they touch). For the team lead persona, this lowers the onboarding cost of Loaf's release discipline — the CLI enforces the canonical path instead of relying on skill knowledge.
- **Architecture:** Maps to STRATEGY.md priority 3 (Release flow hardening). Reinforces the new STRATEGY theme "Diagnosis and repair must share the same state taxonomy" — hooks and the release skill must share the same release-in-progress state model. Shape-validated `release: v<semver>` becomes the shared token across hook validation and CLI mode detection.
- No strategic tensions surfaced. This is infrastructure cleanup, single-user project, no backwards-compat cost.

## Solution Direction

Fix each friction point with minimal surgery. Hooks become path- and release-commit-aware; CLI makes pre-/post-merge paths first-class via new `--pre-merge` and `--post-merge` flags; release skill documents the updated canonical flow.

The release skill's three-layer separation — skills describe, CLI executes, hooks enforce — stays intact. Skill docs shrink (fewer special cases), CLI surface grows modestly (two new flags), hook logic becomes version-shape-aware instead of AI-word-naive.

## Scope

### In Scope

**Hook fixes**

- `validate-commit` regex: replace `/\b(claude|gpt|copilot|chatgpt|gemini|anthropic)\b/i` with `/(?<=^|\s)(claude|gpt|copilot|chatgpt|gemini|anthropic)(?=[\s.,!?:;]|$)/i`. Path tokens (`.claude/`, `dist/claude-code/`) pass; real attribution (`Claude wrote`, `Co-Authored-By: Claude`) still caught.
- `workflow-pre-pr` + `validate-push`: accept HEAD commits matching `^release: v<semver>( \(#\d+\))?$` as valid pre-merge escape hatch, alongside existing tagged-HEAD logic. **Shape-validated, not prefix-only** — the existing test at `cli/commands/check.test.ts:592` must continue to reject `release: prep docs` (a legitimate non-release use of the `release:` type).

**CLI fixes**

- `loaf release` always re-inserts `_No unreleased changes yet._` stub under `[Unreleased]` after moving entries into a versioned section. Works in both the curated-entries path (user wrote entries before running release) and the generated-entries path (loaf release auto-generates from commits).
- New `loaf release --pre-merge` flag: bundles `--no-tag --no-gh --base <auto-detected>`. Works in both curated and generated changelog paths.
- New `loaf release --post-merge` flag: verifies HEAD state against an 8-point guardrail checklist, tags, creates GH release from CHANGELOG section, pulls base branch, best-effort deletes local+remote feature branch.

**Skill updates**

- `/loaf:release` step 4 (both curated and generated branches) invokes `--pre-merge`.
- `/loaf:release` step 6 invokes `--post-merge`.

### Out of Scope

- Redesigning the hook primitive model (already addressed in SPEC-026, SPEC-030).
- Changing Conventional Commits policy.
- Overhauling release-notes generation or adding CHANGELOG curation UX.
- Multi-spec release rollups (one spec = one release, as today).
- Non-GitHub remotes (gh CLI remains required).
- Fixing other `validate-commit` false-positives beyond AI-attribution terms.
- Reflog-based or multi-candidate merge-base base detection (see Rabbit Holes).

### Rabbit Holes

- Generalizing hook validation into a "hook contract system" — strategic-layer concern, not this spec.
- Making `--pre-merge` and `--post-merge` handle every possible release shape. Optimize for the standard path.
- Interactive CHANGELOG curation UX ("prompt to edit entries before commit").
- Session/task lifecycle integration — the release skill already handles that via wrap.
- Reflog heuristics for base detection — fragile (reflog is GC'd) and solves a problem that does not exist in this single-user repo.
- Merge-base scoring across candidate refs (default + develop + release/* + hotfix/*) — YAGNI; if multi-release-branch workflow emerges, revisit.

### No-Gos

- Do not skip hooks via `--no-verify`. Hooks are the enforcement layer; bypassing them undermines the invariant.
- Do not make `--post-merge` tag-and-release a destructive action on failure. Failures in branch deletion or pull should warn and exit 0 once the tag + GH release have succeeded. Tag/release themselves must verify before acting.
- Do not rely on parsing the squash merge body to identify release commits. Use subject-shape validation on HEAD.

## Implementation Notes

### Base detection for `--pre-merge`

Priority order, first match wins:

1. Explicit `--base <ref>` flag.
2. Open PR's base branch via `gh pr view --head <current> --json baseRefName` (if a PR exists and is OPEN).
3. User override via `git config loaf.release.base <ref>`.
4. Default branch via `gh repo view --json defaultBranchRef -q .defaultBranchRef.name`, falling back to `origin/HEAD`.

This covers: fresh branch not yet pushed, pushed branch without PR, branch with open PR, and non-default base via explicit config. Reflog-based and multi-candidate merge-base scoring were considered and rejected as over-engineering.

### Post-merge guardrails for `--post-merge`

All checks required, ordered; any failure aborts before tag/release:

1. Clean worktree (no uncommitted changes).
2. On detected base branch, or branch is cleanly fast-forwardable to it.
3. HEAD subject matches `^release: v<semver>( \(#\d+\))?$`.
4. Extracted version matches detected version files at HEAD (`package.json`, `.claude-plugin/marketplace.json`, etc.).
5. `git diff HEAD^ HEAD --name-only` includes `CHANGELOG.md` and at least one version file.
6. `CHANGELOG.md` at HEAD contains a non-empty `## [<version>]` section.
7. No existing local tag, remote tag, or GitHub release for `v<version>`.
8. HEAD itself is not already tagged.

After all checks pass: tag, create GH release from CHANGELOG section, pull base branch. Branch deletion (local + remote) is best-effort — a failure here warns and exits 0 with tag + release already successful.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| `--pre-merge` base auto-detection picks wrong ref | Low | Medium | `--base` override remains first-class; PR-base lookup is deterministic when PR exists |
| Shape-validated escape hatch rejects a legitimate edge case | Low | Low | Fixture test on existing `release: prep docs` rejection; single-user repo means the user can adjust regex if needed |
| `--post-merge` aborts on a transient state the user can't fix | Medium | Low | Every abort has an actionable error message; user can retry once state is clean |
| Regex change breaks existing commit-validation tests | Low | Medium | Task 1 includes regression tests for both the new pass cases (path tokens) and the preserved reject cases (real attribution) |
| Skill and CLI drift if skill docs aren't updated in lockstep | Medium | Medium | Task 6 explicitly updates `/loaf:release` skill; task 7 is an end-to-end smoke test that exercises both layers |

## Open Questions

- [x] **Q1 resolved:** 4-step base detection (explicit → PR base → git-config override → default branch). Reflog and merge-base scoring deferred.
- [x] **Q2 resolved:** 8-point post-merge guardrail list (worktree, branch, subject shape, version match, diff files, CHANGELOG section, tag collision, HEAD untagged).
- [x] **Q3 resolved:** Shape-validated escape hatch `^release: v<semver>( \(#\d+\))?$`, not prefix-only — preserves the intentional `release: prep docs` rejection at `cli/commands/check.test.ts:592`.

## Test Conditions

- [ ] A fresh release (fix/feat/chore) completes via exactly 5 human steps: `loaf release --pre-merge` → `gh pr create` → review → `gh pr merge --squash` → `loaf release --post-merge`.
- [ ] No ceremony commits in the shipped branch: no stub-restore chore, no hand-patched CHANGELOG.
- [ ] Commit messages referencing path tokens (`.claude/CLAUDE.md`, `dist/codex/`, `.agents/`) pass `validate-commit` without rewording.
- [ ] `gh pr create` after `--pre-merge` succeeds without touching `[Unreleased]` manually.
- [ ] `loaf release --post-merge` on a valid squash-merged HEAD: tags, creates GH release, pulls, best-effort deletes branch, exits clean.
- [ ] **Regression gate:** `cli/commands/check.test.ts:592` still rejects `release: prep docs` (escape hatch is version-shape-validated).
- [ ] **Regression gate:** `loaf release` changelog test explicitly asserts the `_No unreleased changes yet._` stub is re-inserted under `[Unreleased]` (not just that the `[Unreleased]` header is preserved).
- [ ] `loaf doctor` on a freshly released project: no warnings.
- [ ] Task 5 test suite: fixture-driven integration tests for `--post-merge` guardrails — dirty worktree, wrong branch, bad subject shape, missing CHANGELOG section, tag collision. Each aborts with the expected message and does not tag/release.

## Priority Order

Single spec, seven tasks with explicit dependencies:

1. **Task 1** — Tighten `validate-commit` regex + regression tests for both path-token pass cases and preserved-reject cases. Rolls up TASK-109. Independent.
2. **Task 2** — `loaf release` re-inserts `[Unreleased]` stub (curated + generated paths). Test explicitly asserts stub restoration, not just header preservation. Rolls up TASK-110. Independent.
3. **Task 3** — Hook shape-validated `release:` escape hatch in `workflow-pre-pr` and `validate-push`. Preserves existing `release: prep docs` rejection. Independent.
4. **Task 4** — `loaf release --pre-merge` flag with 4-step base-detection algorithm. Blocked by Tasks 2 and 3 (produces commits Tasks 2 and 3 must shape and accept).
5. **Task 5** — `loaf release --post-merge` flag with 8-point guardrails, fixture integration tests, best-effort branch delete. Independent of 1–4.
6. **Task 6** — Update `/loaf:release` skill: step 4 invokes `--pre-merge` in both curated and generated paths; step 6 invokes `--post-merge`. Blocked by Tasks 4 and 5.
7. **Task 7** — End-to-end documented smoke test (manual procedure, run once per release-flow change). Blocked by Tasks 1–6.

**Go/No-Go:**

- Tasks 1, 2, 3, 5 can each merge as separate PRs (independent).
- Task 4 requires Tasks 2 and 3 merged first (otherwise it produces commits the hooks will still reject).
- Task 6 requires Tasks 4 and 5 merged.
- Task 7 is a final verification pass, not a merge gate.

## Success Metric

Every future release — regardless of type (fix, feat, chore) — requires exactly five human-facing steps:

1. `loaf release --pre-merge`
2. `gh pr create …`
3. (review)
4. `gh pr merge --squash`
5. `loaf release --post-merge`

No CHANGELOG hand-patching. No hook-dodging commit messages. No manual tag. No manual GitHub release. No manual branch cleanup.
