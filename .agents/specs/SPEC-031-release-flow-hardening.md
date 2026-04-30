---
id: SPEC-031
title: Release Flow Hardening
source: direct
created: '2026-04-24T21:30:00.000Z'
status: implementing
branch: feat/release-flow-hardening
---

# SPEC-031: Release Flow Hardening

## Problem Statement

The standard `bump → push → PR → squash-merge → tag` release path requires manual choreography around hook and CLI edge cases: ceremony commits, commit-message rewording, and multi-step post-merge orchestration. Five concrete friction points were observed shipping v2.0.0-dev.30 (PR #35), and three more surfaced in the `enline-gridsight-gds` session on 2026-04-27:

1. *(Resolved on `main` in commit `ccc265e8`, 2026-04-29.)* `validate-commit` hook regex false-positives on legitimate path tokens (e.g., `.claude/CLAUDE.md` in a commit body mentioning the AGENTS.md-consolidation work). Required dancing around the regex to get the commit through. The fix replaced the bare `\b(claude|gpt|copilot|chatgpt|gemini|anthropic)\b` pattern with structured attribution-context patterns (`co-authored-by:` trailers, attribution verbs near AI names, bot-emoji footers). Retained as a regression test in this spec; no new code required.
2. `loaf release` moves entries from `[Unreleased]` into a versioned `## [X.Y.Z]` section but does not re-insert the `_No unreleased changes yet._` stub. `workflow-pre-pr` then blocks `gh pr create` on an empty `[Unreleased]`. Required a 5th ceremony commit to unblock PR creation.
3. Pre-merge bump flags (`--no-tag --no-gh --base main`) are non-obvious. The canonical pre-merge invocation lives in skill docs, not in CLI ergonomics.
4. `workflow-pre-pr` and `validate-push` have escape-hatch logic for tagged HEAD commits but no way to recognize a legitimate pre-merge release commit — the exact artifact `loaf release` produces before merge.
5. Post-merge tag + GitHub release + branch cleanup is a manual sequence of commands. The release skill orchestrates it, but there is no single CLI invocation that closes the loop.
6. `loaf release` and the `/loaf:release` skill commit as `release: vX.Y.Z`, which is not a Conventional Commits type and is rejected by standard `@commitlint/config-conventional`. Downstream projects that adopt commitlint cannot use Loaf's release flow until they reword the commit by hand.
7. `detectVersionFiles` only inspects the repository root. Monorepo layouts (`backend/pyproject.toml`, `frontend/package.json`) are invisible to `loaf release`, which fails with "No version files found" and forces a manual bump.
8. On repos with protected default branches, the existing flow works only when a feature branch is being merged. There is no first-class support for a *release-only PR* — a branch whose only diff is a version bump and `[Unreleased]` → `[X.Y.Z]` move — and `workflow-pre-pr` blocks it because the empty `[Unreleased]` looks like a missing changelog.

## Strategic Alignment

- **Vision:** Loaf's "structured execution" pillar — every change flows through a deliberate pipeline without friction. A frictionless release path is load-bearing for this pillar.
- **Personas:** Primarily the solo developer (the release flow is the high-frequency surface they touch). For the team lead persona, this lowers the onboarding cost of Loaf's release discipline — the CLI enforces the canonical path instead of relying on skill knowledge.
- **Architecture:** Maps to STRATEGY.md priority 3 (Release flow hardening). Reinforces the new STRATEGY theme "Diagnosis and repair must share the same state taxonomy" — hooks and the release skill must share the same release-in-progress state model. Shape-validated `chore: release v<semver>` becomes the shared token across hook validation and CLI mode detection.
- **Cross-project applicability:** Loaf is increasingly used in projects with commitlint and monorepo layouts (e.g., `enline-gridsight-gds`). The release flow must work in those repos without bespoke patches.
- No strategic tensions surfaced. This is infrastructure cleanup, single-user project (Loaf itself), with explicit downstream-project ergonomics goals.

## Solution Direction

Fix each friction point with minimal surgery. Hooks become path- and release-commit-aware; CLI makes pre-/post-merge paths first-class via new `--pre-merge` and `--post-merge` flags; the release commit subject moves to a Conventional-Commits-compliant `chore: release v<semver>` shape; CLI version-file discovery learns about monorepo layouts via declarative config; release skill documents the updated canonical flow.

The release skill's three-layer separation — skills describe, CLI executes, hooks enforce — stays intact. Skill docs shrink (fewer special cases), CLI surface grows modestly (three new flags: `--pre-merge`, `--post-merge`, `--version-file`), hook logic becomes version-shape-aware instead of AI-word-naive.

## Scope

### In Scope

**Hook fixes**

- `validate-commit` regex change is **already shipped in `ccc265e8`** at `cli/commands/check.ts:657-664`. This spec adds regression tests asserting both the new pass cases (path tokens like `.claude/`, `dist/codex/`, `.agents/`) AND the preserved reject cases (`Co-Authored-By: Claude`, `Generated by Claude`, bot-emoji footer). Test fixtures live in `cli/commands/check.test.ts`.
- `workflow-pre-pr` + `validate-push`: accept HEAD commits matching `^chore: release v<semver>( \(#\d+\))?$` as valid pre-merge escape hatch, alongside existing tagged-HEAD logic. **Shape-validated, not prefix-only** — must reject e.g. `chore: release notes draft` (a non-release use of `chore:`). Drop the prior `release:` shape entirely; it never made it past commitlint.
- `workflow-pre-pr` *release-only PR* recognition: when the branch's diff against the PR base is *only* version-file edits + a `[Unreleased]` → `[X.Y.Z]` block move in `CHANGELOG.md`, allow the empty `[Unreleased]` and pass without warning. Implemented as a narrow shape check, not a heuristic.

**CLI fixes**

- `loaf release` always re-inserts `_No unreleased changes yet._` stub under `[Unreleased]` after moving entries into a versioned section. Works in both the curated-entries path (user wrote entries before running release) and the generated-entries path (loaf release auto-generates from commits).
- `loaf release` commits as `chore: release v<semver>` (drop the non-Conventional `release:` form). Update `cli/commands/release.ts:468` plus all skill docs that show example commits.
- `loaf release` version-file discovery learns monorepo layouts: `detectVersionFiles` reads optional declared paths from `.agents/loaf.json` at `release.versionFiles: ["backend/pyproject.toml", "frontend/package.json"]`. Repeatable `--version-file <path>` CLI override for ad-hoc invocation. When neither is set, fall back to the current root-only auto-detect.
- New `loaf release --pre-merge` flag: bundles `--no-tag --no-gh --base <auto-detected>`. Works in both curated and generated changelog paths.
- New `loaf release --post-merge` flag: verifies HEAD state against an 8-point guardrail checklist, tags, creates GH release from CHANGELOG section, pulls base branch, best-effort deletes local+remote feature branch.

**Skill updates**

- `/loaf:release` step 4 (both curated and generated branches) invokes `--pre-merge`.
- `/loaf:release` step 6 invokes `--post-merge`.
- All `/loaf:release` references to `release: vX.Y.Z` updated to `chore: release vX.Y.Z`.

### Out of Scope

- Redesigning the hook primitive model (already addressed in SPEC-026, SPEC-030).
- Changing Conventional Commits policy.
- Overhauling release-notes generation or adding CHANGELOG curation UX.
- Multi-spec release rollups (one spec = one release, as today).
- Non-GitHub remotes (gh CLI remains required).
- Fixing other `validate-commit` false-positives beyond AI-attribution terms.
- Reflog-based or multi-candidate merge-base base detection (see Rabbit Holes).
- **Bundled build-artifact leakage in unrelated commits** (lockfiles, `plugins/loaf/bin/loaf`, generated outputs). Tracked separately as `TASK-136`. Different problem domain (commit hygiene, not release flow); recurrence in dev.31 and dev.32 is real but warrants its own scoped fix.

### Rabbit Holes

- Generalizing hook validation into a "hook contract system" — strategic-layer concern, not this spec.
- Making `--pre-merge` and `--post-merge` handle every possible release shape. Optimize for the standard path.
- Interactive CHANGELOG curation UX ("prompt to edit entries before commit").
- Session/task lifecycle integration — the release skill already handles that via wrap.
- Reflog heuristics for base detection — fragile (reflog is GC'd) and solves a problem that does not exist in this single-user repo.
- Merge-base scoring across candidate refs (default + develop + release/* + hotfix/*) — YAGNI; if multi-release-branch workflow emerges, revisit.
- Auto-globbing `**/pyproject.toml` etc. for monorepo discovery. Too magical; pulls in vendored copies. Declarative config + flag override is the bound.
- Re-detecting all consumers of the `release:` shape across downstream Loaf-using projects. They will be updated as they bump to the new Loaf version; no migration tooling.
- Cross-validating monorepo version files for agreement (e.g., `package.json` and `backend/pyproject.toml` having the same version). Out of scope; user's responsibility.

### No-Gos

- Do not skip hooks via `--no-verify`. Hooks are the enforcement layer; bypassing them undermines the invariant.
- Do not make `--post-merge` tag-and-release a destructive action on failure. Failures in branch deletion or pull should warn and exit 0 once the tag + GH release have succeeded. Tag/release themselves must verify before acting.
- Do not rely on parsing the squash merge body to identify release commits. Use subject-shape validation on HEAD.
- Do not preserve dual-shape acceptance (`release:` *and* `chore: release`). Cut over cleanly to `chore: release`; nothing in the wild has shipped against the old shape because the old shape was rejected by commitlint everywhere it mattered.
- Do not auto-glob the filesystem for monorepo version files. Declarative paths only.

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
3. HEAD subject matches `^chore: release v<semver>( \(#\d+\))?$`.
4. Extracted version (from commit subject) matches the version recorded in each detected version file at HEAD. This verifies the bump landed in every declared file (the work `loaf release` was supposed to do); it does **not** enforce cross-file agreement on the *input* state — disagreement before the bump is the user's responsibility per the Risks table.
5. `git diff HEAD^ HEAD --name-only` includes `CHANGELOG.md` and at least one version file.
6. `CHANGELOG.md` at HEAD contains a non-empty `## [<version>]` section.
7. No existing local tag, remote tag, or GitHub release for `v<version>`.
8. HEAD itself is not already tagged.

After all checks pass: tag, create GH release from CHANGELOG section, pull base branch. Branch deletion (local + remote) is best-effort — a failure here warns and exits 0 with tag + release already successful.

### Monorepo version-file discovery

Two-tier resolution, declarative-first:

1. **Declared paths** — `.agents/loaf.json` reads an optional `release.versionFiles` array of repo-relative paths. Each entry is validated against the existing format detection (json/toml-regex). When set, this list *replaces* root auto-detection (no merge — explicit beats implicit).
2. **CLI override** — `--version-file <path>` is repeatable. When present on the CLI, it replaces both declared and auto-detected paths for that invocation (useful for one-off bumps and dry-runs).
3. **Fallback** — when neither is set, `detectVersionFiles` behaves exactly as today: scan the root for `package.json`, `pyproject.toml`, `Cargo.toml`, `.agents/loaf.json`, `.claude-plugin/marketplace.json`.

Validation: each declared/overridden path must exist and contain a parseable version. If any declared path is missing or malformed, abort with a precise error before any version writes — partial monorepo bumps are worse than no bump.

### Release-only PR detection

Hook-side classifier in `workflow-pre-pr`. A PR is *release-only* if `git diff <base>..HEAD --name-only` returns exclusively:

- `CHANGELOG.md`, AND
- One or more known version-file paths (the same set `loaf release` would write to, including monorepo declarations from `.agents/loaf.json`).

Any other changed file disqualifies the classification. When the diff qualifies AND `CHANGELOG.md` at HEAD has a non-empty `## [<version>]` section matching the version files, the empty-`[Unreleased]` block does not block PR creation.

### Conventional Commits compliance

The `chore: release v<semver>` shape is the smallest change that satisfies `@commitlint/config-conventional`. Scope intentionally omitted to match Loaf's project convention (no scopes in any commit type — see git log). Test fixtures must include a project that pins commitlint and a project that does not, to confirm the new shape passes both.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| `--pre-merge` base auto-detection picks wrong ref | Low | Medium | `--base` override remains first-class; PR-base lookup is deterministic when PR exists |
| Shape-validated escape hatch rejects a legitimate edge case | Low | Low | Fixture tests for both the new accept (`chore: release v1.2.3`) and the preserved reject (`chore: release notes draft`) cases |
| `--post-merge` aborts on a transient state the user can't fix | Medium | Low | Every abort has an actionable error message; user can retry once state is clean |
| Regex change breaks existing commit-validation tests | Low | Medium | Task 1 includes regression tests for both the new pass cases (path tokens) and the preserved reject cases (real attribution); regex change already shipped in `ccc265e8` |
| Removing `release` from `validateCommit` accepted-types missed during cutover, breaking the release flow self-referentially | Low | High | Task 5 explicit touch-point list (CLI + hook + 3 test fixtures + skill + STRATEGY.md) is grep-verified during implementation. CI runs `loaf check` on the release commit itself as part of the smoke test. |
| Skill and CLI drift if skill docs aren't updated in lockstep | Medium | Medium | Task 10 explicitly updates `/loaf:release` skill; task 11 is an end-to-end smoke test that exercises both layers |
| Release-only PR classifier misclassifies (false positive lets a non-release PR through with empty `[Unreleased]`) | Low | Medium | Classifier is a strict allowlist of paths AND requires non-empty `## [<version>]` section in CHANGELOG that matches the version files. False positive requires both conditions to coincide accidentally |
| Declared monorepo paths disagree with each other (e.g., `package.json` says 1.0.0, `backend/pyproject.toml` says 0.9.0) | Medium | Low | Out of scope to validate; document in skill that user is responsible for keeping declared files aligned. `--dry-run` shows current → new for every declared file before committing |
| Downstream Loaf consumers pinned to old `release:` shape break on upgrade | Low | Low | Single-user-base today; no public release; no migration path needed beyond release notes for the bump that ships SPEC-031 |

## Open Questions

- [x] **Q1 resolved:** 4-step base detection (explicit → PR base → git-config override → default branch). Reflog and merge-base scoring deferred.
- [x] **Q2 resolved:** 8-point post-merge guardrail list (worktree, branch, subject shape, version match, diff files, CHANGELOG section, tag collision, HEAD untagged).
- [x] **Q3 resolved (revised):** Shape-validated escape hatch is `^chore: release v<semver>( \(#\d+\))?$`, not the prior `release:` form (which fails commitlint). Test fixture must reject `chore: release notes draft` to confirm shape validation, not prefix matching. The `release: prep docs` rejection at `cli/commands/check.test.ts:592` remains valid as a regression test — after Task 5's `validateCommit` change, `release:` is an unknown type, so `release: prep docs` is rejected at the type-check layer instead of the changelog-empty layer. Test must be updated to assert the new error path.
- [x] **Q4 resolved:** Monorepo discovery is declarative-only via `.agents/loaf.json` `release.versionFiles` plus `--version-file` CLI override. No filesystem auto-glob.
- [x] **Q5 resolved:** Release-only PR classification is a strict diff allowlist (only `CHANGELOG.md` + known version files), not a heuristic. Release-only PRs bypass the empty-`[Unreleased]` block in `workflow-pre-pr`.

## Test Conditions

- [ ] A fresh release (fix/feat/chore) completes via exactly 5 human steps: `loaf release --pre-merge` → `gh pr create` → review → `gh pr merge --squash` → `loaf release --post-merge`.
- [ ] No ceremony commits in the shipped branch: no stub-restore chore, no hand-patched CHANGELOG.
- [ ] Commit messages referencing path tokens (`.claude/CLAUDE.md`, `dist/codex/`, `.agents/`) pass `validate-commit` without rewording.
- [ ] `gh pr create` after `--pre-merge` succeeds without touching `[Unreleased]` manually.
- [ ] `loaf release --post-merge` on a valid squash-merged HEAD: tags, creates GH release, pulls, best-effort deletes branch, exits clean.
- [ ] **Regression gate:** `cli/commands/check.test.ts` rejects `release: prep docs` AND rejects `chore: release notes draft` (shape-validation, not prefix matching).
- [ ] **Regression gate:** `loaf release` changelog test explicitly asserts the `_No unreleased changes yet._` stub is re-inserted under `[Unreleased]` (not just that the `[Unreleased]` header is preserved).
- [ ] **Regression gate:** `loaf release` commits with subject `chore: release v<semver>` — explicitly tested, not just observed in fixtures.
- [ ] **Commitlint compatibility:** Fixture project pinned to `@commitlint/config-conventional` accepts the `loaf release` commit on its first commit-msg hook.
- [ ] **Monorepo:** Project with `.agents/loaf.json` declaring `release.versionFiles: ["backend/pyproject.toml"]` runs `loaf release --dry-run` and shows the backend file in the bump preview. Without the declaration, the same project errors with "No version files found."
- [ ] **`--version-file` override:** `loaf release --version-file frontend/package.json --dry-run` previews exactly that file, ignoring root and declared paths.
- [ ] **Release-only PR:** A branch whose only diff is a version bump and `[Unreleased]` → `[X.Y.Z]` move passes `workflow-pre-pr` without a stub restore, on a repo with `_No unreleased changes yet._` style.
- [ ] `loaf doctor` on a freshly released project: no warnings.
- [ ] Task 7 test suite: fixture-driven integration tests for `--post-merge` guardrails — dirty worktree, wrong branch, bad subject shape, missing CHANGELOG section, tag collision. Each aborts with the expected message and does not tag/release.
- [ ] **Base detection precedence:** Fixture tests for `--pre-merge` covering all four steps in order — explicit `--base` flag wins over PR-base lookup; PR-base wins over `git config loaf.release.base`; git config wins over default-branch fallback. Each tier tested in isolation and together.
- [ ] **Post-merge guardrail 4 (version match):** Fixture where `package.json` is bumped but `pyproject.toml` is stale aborts with explicit per-file diagnostic.
- [ ] **Post-merge guardrail 5 (diff files):** Fixture where the squash merge somehow lacks a CHANGELOG diff aborts cleanly.
- [ ] **Post-merge guardrail 7 (existing GH release):** Fixture mocking `gh release view v1.2.3` returning success aborts with the "delete release page" message.
- [ ] **Post-merge guardrail 8 (HEAD already tagged):** Fixture where HEAD already has a tag aborts with the "not a fresh post-merge state" message.
- [ ] **Post-merge idempotency:** Fixture where a prior partial run created the tag locally but failed before GH release. Rerun aborts at guardrail 7 with actionable error; user fixes and reruns successfully.
- [ ] **Curated entries preserved (Task 3):** Fixture with curated `[Unreleased]` containing list items asserts those exact items appear under the new `## [X.Y.Z]` header — auto-generation does not overwrite them.

## Priority Order

Single spec, eleven tasks with explicit dependencies:

1. **Task 1** — Regression tests for the `validate-commit` regex (already shipped in `ccc265e8`, 2026-04-29). Tests must cover both new pass cases (path tokens) and preserved reject cases (real attribution). Also: delete orphan `content/hooks/pre-tool/workflow-pre-pr.sh` — the file is not wired by `config/hooks.yaml` (which auto-dispatches via `loaf check --hook workflow-pre-pr` to the TS path at `cli/commands/check.ts:451`); the shell file is stale parallel logic that will rot. Rolls up TASK-109 (test coverage portion). Independent.
2. **Task 2** — `loaf release` re-inserts `[Unreleased]` stub (curated + generated paths). Test explicitly asserts stub restoration, not just header preservation. Rolls up TASK-110. Independent.
3. **Task 3** — Preserve curated `[Unreleased]` entries when present. Currently, `loaf release` overwrites curated entries with auto-generated commit-subject jargon (recurred in dev.31 and dev.32 — see STRATEGY.md:65-66). Behavior must be: if `[Unreleased]` contains list items at invocation time, those entries are preserved verbatim under the new `## [X.Y.Z]` header; auto-generation runs only when `[Unreleased]` is empty. Test: fixture with curated `[Unreleased]` entries asserts those exact entries appear under the new version header post-release. Independent.
4. **Task 4** — Hook shape-validated `chore: release` escape hatch in `workflow-pre-pr` and `validate-push`. Rejects `release:` shape (now an unknown type) and rejects `chore: release notes draft` (shape, not prefix). Independent.
5. **Task 5** — `loaf release` commit subject change: `release: vX.Y.Z` → `chore: release vX.Y.Z`. Touch-points (grep-verified, all in current `main`):
   - `cli/commands/release.ts:468` (the commit message string)
   - `cli/commands/check.ts:642` (remove `release` from accepted Conventional Commits types in the `conventionalCommitRegex`)
   - `cli/commands/check.ts:649` (remove `release` from the user-facing "Valid types: ..." error message)
   - `cli/commands/check.test.ts:632, 658, 890` (three fixtures using `release: v1.1.0` / `release: prep docs` — update to `chore: release v1.1.0` / preserve `release: prep docs` as a *now unknown-type* rejection regression)
   - `content/skills/release/SKILL.md:193, 226` (curated and generated paths showing `Commit: release: vX.Y.Z`)
   - `docs/STRATEGY.md:64` (stale doctrine: "extend `workflow-pre-pr` to accept a `release:` HEAD commit as an escape hatch" — reword to reflect the cutover)

   Test explicitly asserts the subject shape produced by `loaf release`. Independent of Tasks 1–3 mechanically, but **lands together with Task 4 in a single PR** (the hook accept-list and the CLI commit subject must change atomically — splitting them either rejects the new commit or accepts the old one).
6. **Task 6** — `loaf release --pre-merge` flag with 4-step base-detection algorithm. Blocked by Tasks 2, 3, 4, and 5 (produces commits the hooks must accept).
7. **Task 7** — `loaf release --post-merge` flag with 8-point guardrails, fixture integration tests, best-effort branch delete. Subject-shape check uses the new `chore: release` form. **Implementation specifics:**
   - **Feature branch identity:** capture `git symbolic-ref --short HEAD` BEFORE the checkout-to-base step, store in a local variable, use for the delete step (avoids "where does the branch name come from?" ambiguity).
   - **Tag push:** action sequence is `git tag -a v<version> -m "Release <version>"` → `git push origin v<version>` → `gh release create v<version>` → `git checkout <base> && git pull --rebase` → best-effort branch deletion. Tag push happens before GH release create, since `gh release create` will create+push a tag if missing — explicit push first keeps that behavior off our path.
   - **Light idempotency:** each guardrail check is rerun-safe (all stateless reads). Each abort message names the manual fix:
     - Tag exists locally: `"tag v<version> already exists locally — run \`git tag -d v<version>\` and rerun"`
     - Tag exists on remote: `"tag v<version> already exists on remote — run \`git push origin :refs/tags/v<version>\` and rerun"`
     - GH release exists: `"GH release v<version> already exists — visit the release page and delete it manually before rerunning"`
     - HEAD already tagged: `"HEAD is already tagged as <existing-tag>; this is not a fresh post-merge state"`

   Blocked by Task 5 (subject shape).
8. **Task 8** — Monorepo version-file discovery: `.agents/loaf.json` `release.versionFiles` reading, `--version-file` repeatable CLI flag, validation, dry-run preview. Independent.
9. **Task 9** — Release-only PR classifier in `workflow-pre-pr`: strict diff allowlist (CHANGELOG.md + version files only), bypass empty-`[Unreleased]` block when classified. Blocked by Task 6 (`--pre-merge` base resolution) and Task 8 (version-file path set must include declared monorepo paths). Both Tasks 6 and 9 use the same base-ref resolution logic; share the implementation.
10. **Task 10** — Update `/loaf:release` skill: step 4 invokes `--pre-merge` in both curated and generated paths; step 6 invokes `--post-merge`; all `release:` examples updated to `chore: release`. Blocked by Tasks 6 and 7.
11. **Task 11** — End-to-end documented smoke test (manual procedure, run once per release-flow change). Blocked by Tasks 1–10.

**Go/No-Go:**

- Tasks 1, 2, 3, 8 can each merge as separate PRs (independent).
- Tasks 4 + 5 must land in a single PR (atomic shape cutover).
- Task 6 requires Tasks 2, 3, and 4+5 merged first.
- Task 7 requires Task 5 merged.
- Task 9 requires Tasks 6 and 8 merged.
- Task 10 requires Tasks 6 and 7 merged.
- Task 11 is a final verification pass, not a merge gate.

**Bundling:** The user's intent is a single feature branch / single PR for all of SPEC-031, with the chore-shape commit absorbed. The numbered tasks describe internal ordering and atomicity (e.g., Tasks 4+5 atomic), not separate PRs.

## Success Metric

Every future release — regardless of type (fix, feat, chore), regardless of repo layout (root or monorepo), regardless of whether the release ships alongside feature work or standalone — requires exactly five human-facing steps:

1. `loaf release --pre-merge`
2. `gh pr create …`
3. (review)
4. `gh pr merge --squash`
5. `loaf release --post-merge`

No CHANGELOG hand-patching. No hook-dodging commit messages. No commitlint rewording. No manual version edits in `backend/pyproject.toml`. No manual tag. No manual GitHub release. No manual branch cleanup. The flow works identically against protected default branches because the merge IS the PR.
