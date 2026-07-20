---
description: >-
  Orchestrates standalone releases from already-landed work: release readiness,
  version selection, changelog curation, release commit, tag, GitHub Release,
  install verification, and post-release follow-up. Use when the user says "cut
  a release," "publish a version," "release from main," or asks whether enough
  landed work should become a release. Not for reviewing or merging a PR (use
  ship).
version: 2.0.0-alpha.12
---

# Release

Publish a coherent version from work that has already landed.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Context Detection
- Step 1: Release Readiness
- Step 2: Change Collection
- Step 3: Version + Changelog
- Step 4: Release Execution
- Step 5: Protected-Branch Handoff
- Step 6: Publication Verification
- Step 7: Post-Release Follow-Up
- Hook Interaction
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

- **Release is not merge** -- do not use `/release` to review, approve, or land a feature PR. Use `/ship` for PR correctness and landing.
- **Release from landed work** -- collect changes from the release base branch, normally the repo default branch, since the last release tag.
- **Batch by intent** -- group release notes by user-facing outcome, `CR-*` change bundle, spec, or related PRs; do not mirror individual commits mechanically.
- **Keep landed and released distinct** -- a PR may be landed without being released; a release may contain multiple landed PRs.
- **Block on release-readiness failure** -- do not publish if build, tests, version files, changelog, tag, or GitHub release state is inconsistent.
- **Never push, tag, or publish without confirmation** -- present the exact actions first.
- **Use `prompt the user in chat` for release decisions when available** -- version bump type, release PR handoff, push/tag/GitHub Release confirmation.
- **Log release** -- after publication, run `loaf journal log "decision(release): vX.Y.Z published from <base> with <summary>"`.

## Verification

- Release base branch is clean, current, and contains the intended landed PRs
- Pre-flight checks pass before versioning or publication
- Changelog entries are curated user-facing prose, not commit or PR-title dumps
- Version files, changelog heading, git tag, and GitHub Release all agree
- Tag points at the released base-branch commit or release commit, not an abandoned feature branch
- Downstream install path is verified when applicable, especially Homebrew for Loaf releases

## Quick Reference

| Step | Gate | Blocking? |
|------|------|-----------|
| Readiness | clean/current base branch, no unresolved release collisions | Yes |
| Change Collection | landed work since last tag grouped into release themes | Yes |
| Version + Changelog | bump selected, notes curated, files updated | Yes |
| Execution | release commit/tag/GitHub Release created or release PR prepared | Yes |
| Verification | release and install paths checked | Yes |
| Follow-Up | reflect/housekeeping suggested when useful | No |

## Topics

| Topic | Use When |
|-------|----------|
| [Context Detection](#context-detection) | Determining release base, last tag, and current branch |
| [Protected-Branch Handoff](#step-5-protected-branch-handoff) | Branch protection requires a release PR |
| [Hook Interaction](#hook-interaction) | Understanding coexistence with git hooks |

---

## Context Detection

Before anything, establish the release surface:

1. Get current branch and repo default branch:
   ```bash
   git branch --show-current
   gh repo view --json defaultBranchRef -q .defaultBranchRef.name
   ```
2. Parse `$ARGUMENTS` for an explicit base, tag, or version. If omitted, use the repo default branch as the release base.
3. Verify the current branch:
   - If already on the release base, continue.
   - If on a feature branch, stop and explain that `/release` publishes from landed work. Offer `/ship` if the active PR needs landing first.
   - If preparing a release branch because direct base-branch pushes are blocked, continue only after the user confirms that release-PR strategy.
4. Find the previous release tag:
   ```bash
   git describe --tags --abbrev=0
   ```
5. Gather the candidate release range:
   ```bash
   git log --oneline <last-tag>..HEAD
   git diff --stat <last-tag>..HEAD
   ```

---

## Step 1: Release Readiness

Run release pre-flight checks before editing release files:

1. Ensure worktree is clean:
   ```bash
   git status --short
   ```
2. Ensure the release base is current:
   ```bash
   git fetch --tags origin
   git status --branch --short
   ```
3. Check for existing tag or GitHub Release collisions for the target version once known:
   ```bash
   git tag --list vX.Y.Z
   gh release view vX.Y.Z
   ```
4. Run project checks:
   - Node: `npm run typecheck`, `npm run test`, `npm run build` when scripts exist
   - Go: `go vet ./...`, `go test ./...` when `go.mod` exists
   - Python: `pytest`, `mypy .`, `ruff check .` when configured
   - Rust: `cargo check`, `cargo test` when `Cargo.toml` exists

If no checks are detected, warn explicitly. If a check fails, stop and fix before release.

---

## Step 2: Change Collection

Collect landed work since the last release and group it for release notes.

1. Inspect commits:
   ```bash
   git log --first-parent --oneline <last-tag>..HEAD
   git log --oneline <last-tag>..HEAD
   ```
2. Inspect merged PRs when GitHub is available:
   ```bash
   gh pr list --state merged --base <base> --json number,title,mergedAt,url
   ```
3. Group changes by user-facing outcome:
   - `CR-*` change bundle, when referenced
   - spec or task family, when public enough to be useful
   - feature/fix/documentation/build themes
   - operational release work, when it affects users or maintainers
4. Drop noise:
   - purely internal task labels
   - reverted work that is not present in `HEAD`
   - individual commit mechanics that collapse into one user-facing change

Present the grouped release contents before choosing the bump.

---

## Step 3: Version + Changelog

Choose the bump and curate the changelog from the grouped landed work.

1. Run a dry run:
   ```bash
   loaf release --dry-run
   ```
   Use `--base <ref>` when the project expects a non-default release base.
2. Present:
   - current version
   - proposed next version
   - detected version files
   - release actions the CLI would perform
   - draft changelog entries
3. Curate `CHANGELOG.md` before publishing:
   - write from the upgrading user's perspective
   - group under Common Changelog categories: `Changed`, `Added`, `Removed`, `Fixed`
   - use one self-describing line per meaningful change
   - include public PR, issue, ADR, release, or commit links when helpful
   - avoid dumping commit subjects, task IDs, session mechanics, or internal gate language
4. Confirm the bump type: `prerelease`, `release`, `major`, `minor`, or `patch`.

---

## Step 4: Release Execution

For a direct release from the base branch, run:

```bash
loaf release --bump <type> --yes
```

This should:

1. Update version files
2. Convert `[Unreleased]` into `## [X.Y.Z] - YYYY-MM-DD`
3. Reinsert a fresh empty `[Unreleased]` section
4. Run configured release artifact commands
5. Create the release commit
6. Create/push the release tag
7. Create the GitHub Release when enabled

After execution, verify generated artifacts are current:

```bash
npm run build
git diff --exit-code -- dist plugins content/skills/loaf-reference/SKILL.md
```

Adjust the path list to the project. For Loaf itself, tracked generated outputs under `dist/`, `plugins/`, and native binaries must match the source changes.

---

## Step 5: Protected-Branch Handoff

If branch protection prevents direct release commits on the base branch:

1. Create a dedicated release branch.
2. Run the release command in a mode that creates the version/changelog/artifact commit but does not publish final tags or GitHub Release artifacts until the release commit lands on the base branch.
3. Open a release PR with a concise release-focused body.
4. Hand the PR to `/ship` for review and landing.
5. After `/ship` lands the release PR, resume `/release` on the base branch to tag, publish the GitHub Release, and verify installability.

Do not hide this handoff inside `/release`: `/ship` remains the PR correctness and merge gate.

---

## Step 6: Publication Verification

After publishing, verify the public release state:

1. Confirm tag location:
   ```bash
   git show --stat vX.Y.Z
   ```
2. Confirm GitHub Release:
   ```bash
   gh release view vX.Y.Z
   ```
3. Confirm package or installer availability when applicable:
   - npm: `npm view <package> version`
   - Homebrew: `brew update && brew info <tap>/<formula>`
   - project-specific deploy or artifact registry checks
4. For Loaf/Homebrew, report readiness only after the GitHub release exists, assets are uploaded, the tap formula is updated, and tap CI has passed.

If publication partially completes, do not retag casually. Name the exact state and continue with the smallest repair or patch release path.

---

## Step 7: Post-Release Follow-Up

After verification:

1. Log the release decision to the project journal:
   ```bash
   loaf journal log "decision(release): vX.Y.Z published from <base> with <summary>"
   ```
2. Suggest `/reflect` when the release produced durable product or workflow learnings.
3. Suggest `/housekeeping` when release branches or temporary reports need cleanup.
4. Keep future-work discoveries out of the release notes; capture them as tasks, ideas, or sparks instead.

---

## Hook Interaction

This skill coexists with existing hooks. Git workflow hooks are advisory unless
configured otherwise; security and secret-scanning hooks remain blocking.

| Hook | Type | When `/release` Runs |
|------|------|---------------------|
| `github-account` | Force-switch | Switches to the configured GitHub account before `gh` release operations; blocks only if the switch fails |
| `validate-push` | Advisory | Cross-checks version bump, changelog, and build on push |
| `workflow-pre-pr` | Advisory | May fire only for protected-branch release PRs |
| `workflow-pre-merge` | Advisory | Belongs to `/ship` when a release PR must land |
| `workflow-post-merge` | Advisory | Belongs to `/ship` after PR landing |
| `check-secrets` | Blocking | Always respected before writes or shell actions |

Do not disable hooks to force a release through.

---

## Suggests Next

After a successful release, suggest `/reflect` for durable learnings and `/housekeeping` if temporary release artifacts need attention.

## Related Skills

- **ship** -- Reviews, verifies, and lands a PR before it becomes release input
- **git-workflow** -- Branching, PR, commit, and squash merge conventions
- **documentation-standards** -- Changelog and release-note quality
- **reflect** -- Updates strategy from shipped/released learnings
- **housekeeping** -- Cleans up completed spec, report, and handoff artifacts
