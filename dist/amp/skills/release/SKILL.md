---
name: release
description: >-
  Orchestrates releases: pre-flight checks, version bump via `loaf release`,
  squash merge, and post-merge cleanup. Use when the user says "release this,"
  "merge this PR," "ready to merge," or "ship it." Produces version bumps,
  changelog updates, and merged code. Not for creating PRs (use git-workflow) or
  reflection (use reflect).
version: 2.0.0-dev.17
---

# Release

Orchestrate a squash merge with correct version ordering, documentation checks, and cleanup.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Context Detection
- Step 1: Pre-Flight Checks
- Step 2: Documentation Freshness
- Step 3: Housekeeping Verification
- Step 3b: Create PR (if needed)
- Step 4: Version Bump + Changelog
- Step 5: Squash Merge
- Step 6: Post-Merge Cleanup
- Hook Interaction
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

- **Block on pre-flight failure** -- do not offer to skip any failing check
- **User confirms every destructive action** -- version bump commit, push, merge, branch deletion
- **Version bump before merge** -- this is the skill's reason for existing; never defer to post-merge
- **Clean squash body** -- never use the auto-generated commit dump
- **Verify, don't do** -- housekeeping is the implementer's job; this skill only verifies it was done
- **Detect-first** -- auto-detect PR from branch before asking for a number
- **Never push without confirmation** -- even after successful version bump
- **Log release** -- log to session journal after merge: `loaf session log "decision(release): vX.Y.Z shipped via PR #N"`

## Verification

- Pre-flight checks (typecheck, test, build) all pass before proceeding
- Version bump commit exists on the feature branch before merge
- Squash merge body is a clean 2-4 sentence summary, not a commit dump
- Post-merge cleanup completed: base branch pulled, feature branch deleted

## Quick Reference

| Step | Gate | Blocking? |
|------|------|-----------|
| Pre-Flight | typecheck + test + build pass | Yes |
| Doc Freshness | User reviews stale docs | No (user decides) |
| Housekeeping | Spec/tasks archived, CHANGELOG ready | No (user decides) |
| PR Creation | Only if no PR exists yet | Yes (if needed) |
| Version Bump | User confirms bump type | Yes |
| Squash Merge | User approves body text | Yes |
| Post-Merge | Branch cleanup | Yes |

## Topics

| Topic | Use When |
|-------|----------|
| [Context Detection](#context-detection) | Determining current branch and PR state |
| [Hook Interaction](#hook-interaction) | Understanding coexistence with git hooks |

---

## Context Detection

Before anything, detect where we are:

1. **Get current branch and repo default branch:**
   ```bash
   git branch --show-current
   gh repo view --json defaultBranchRef -q .defaultBranchRef.name
   ```
2. **If on the default branch**, STOP — there is no PR to merge. Offer post-merge cleanup only (Step 6), using the default branch as `baseRefName`.
3. **Parse `$ARGUMENTS`**: may be a PR number (`42`), a PR URL, or empty.
4. If `$ARGUMENTS` is empty, auto-detect from the current branch:
   ```bash
   gh pr view --json number,title,url,headRefName,baseRefName
   ```
5. If no PR exists for this branch, note `pr_exists = false` and set `baseRefName` to the repo default branch. Continue — the PR will be created in Step 3b.
6. If PR exists, **save `baseRefName`** from the PR metadata (e.g., `main`, `release/1.0`, `develop`). All subsequent steps use this as the base reference for diffs and changelog scoping. Do NOT hardcode `main`.
7. Confirm the PR identity (or intent to create one) with the user before proceeding.

---

## Step 1: Pre-Flight Checks (BLOCKING)

Run these and **BLOCK on any failure** — do not offer to skip.

### Detect project type

Inspect the repo to determine available checks:
- **Node** (`package.json`): look for `typecheck`, `test`, `build` scripts
- **Python** (`pyproject.toml`): look for `pytest`, `mypy`, `ruff` in dev dependencies or tool config
- **Rust** (`Cargo.toml`): `cargo check`, `cargo test`
- **Go** (`go.mod`): `go vet`, `go test`

### Run checks

Run whichever checks the project supports. Examples for a Node project:
1. `npm run typecheck` (if the script exists)
2. `npm run test` (if the script exists)
3. `npm run build` (preferred) or `loaf build` if `npm run build` is not available

For Python: `pytest`, `mypy .` (if configured). For Rust: `cargo check`, `cargo test`.

**If no checks are detected**, WARN the user explicitly: *"No pre-flight checks found (no test runner, type checker, or build script detected). Proceeding without verification — consider adding checks before merging."* Do NOT silently skip.

On failure: show the error, STOP, explain what needs fixing. Do not proceed to Step 2.

---

## Step 2: Documentation Freshness

Check whether documentation is stale relative to the branch's changes:

1. Run `git diff <baseRefName>..HEAD --name-only -- README.md ARCHITECTURE.md docs/` to identify changed doc files (use the `baseRefName` from Context Detection).
2. Run `git diff <baseRefName>..HEAD --stat` to understand the scope of code changes.
3. Read README.md and ARCHITECTURE.md. Look for references to concepts, features, agents, or APIs that the branch may have changed or removed.
4. If the branch introduced significant changes (new features, removed components, renamed concepts) but docs weren't updated, flag specific sections that may be stale.

Present findings to the user. They decide whether to fix now or note for later. Do NOT silently skip.

---

## Step 3: Housekeeping Verification

**Verify** that implementation housekeeping was done. Do NOT do it — that's the implement skill's job. Each check produces pass/fail:

1. **Spec archived**: If a spec is associated with this branch (check `.agents/specs/` for matching spec), verify its status is `complete`.
2. **Tasks archived**: Check `.agents/tasks/` for tasks related to the spec that aren't archived.
3. **CHANGELOG ready**: Verify `CHANGELOG.md` exists and has the `[Unreleased]` marker (Step 4 will generate the actual entries).
4. **Session file**: If a session file exists, check that its status reflects completion.

On gaps: present them to the user. Offer to fix (delegate to `loaf task archive`, `loaf spec archive`). The user decides. Do NOT silently fix or silently skip.

---

## Step 3b: Create PR (if needed)

**Only run this step if `pr_exists = false` from Context Detection.**

The PR must be created BEFORE the version bump so that `[Unreleased]` changelog entries are still present (advisory hooks check for this).

1. Push the branch if not already pushed.
2. Create the PR following the format in [git-workflow/references/commits.md](../git-workflow/references/commits.md) (Pull Request Format section).
3. Save the PR number and `baseRefName` for subsequent steps.

---

## Step 4: Version Bump + Changelog (on feature branch)

This step handles versioning and changelog. The approach depends on whether curated changelog entries already exist.

### Check for existing changelog entries

Read `CHANGELOG.md` and check if `[Unreleased]` has content (entries written during development, often required by pre-PR hooks).

- **If curated entries exist** → Use the **Curated path** (preserve them)
- **If `[Unreleased]` is empty** → Use the **Generated path** (auto-generate from commits)

### Curated path (preferred when entries exist)

The pre-PR workflow requires writing CHANGELOG entries before creating a PR. These curated entries are typically better than auto-generated ones (grouped by category, human-written descriptions). Preserve them.

1. Run `loaf release --base <baseRefName> --dry-run` to get the **suggested bump type** and **current version**
2. Present the bump suggestion to the user. They may accept or override.
3. Once confirmed, perform the version bump manually:
   - Bump version in `package.json` (or other version files)
   - Convert the `[Unreleased]` header to `## [X.Y.Z] - YYYY-MM-DD` (preserving the curated entries beneath it)
   - Add a fresh empty `## [Unreleased]` section above it
   - Run the project's build command (e.g., `npm run build` or `loaf build`)
   - Commit: `release: vX.Y.Z`

### Generated path (when no entries exist)

When `[Unreleased]` is empty, use `loaf release` to auto-generate changelog entries from branch commits.

1. Run `loaf release --base <baseRefName> --dry-run` to preview:
   - Current version and suggested bump type
   - Generated changelog section from **this branch's commits only**
   - Which version files will be updated

   The `--base` flag scopes the commit analysis to `<baseRefName>..HEAD`, so only commits on the feature branch are considered.

2. Present the preview to the user. They may:
   - Accept the suggested bump type
   - Override with a different type (`prerelease`, `release`, `major`, `minor`, `patch`)
   - Edit the changelog content conversationally

3. Once the user confirms, run:
   ```bash
   loaf release --base <baseRefName> --bump <type> --no-gh --yes
   ```

   This will:
   1. Bump version in all detected files (package.json, pyproject.toml, etc.)
   2. Generate and insert changelog section from branch commits (adding fresh `[Unreleased]`)
   3. Run `loaf build` to rebuild all targets with new version
   4. Commit: `release: vX.Y.Z`
   5. Create git tag `vX.Y.Z`

### After either path

Push to the feature branch (**with user confirmation**). Push tags with `git push --tags`.

### Why these flags?

- `--base <baseRefName>` — Scopes changelog and bump suggestion to this PR's work, not everything since the last tag
- `--no-gh` — GitHub release drafts belong to stable releases, not pre-merge bumps
- `--yes` — Skip the CLI confirmation prompt (the skill already confirmed with the user conversationally)

Tags are created on every version bump (including prereleases) so that future `loaf release` runs can auto-scope to the previous version.

---

## Step 5: Squash Merge

1. Draft a clean squash body from the branch's commit history and PR description. Descriptive, not verbose:
   - One-line summary, then bullet points grouped by feature area
   - Plain text — no bold, no headings, only backticks for `code`
   - Not a paragraph dump, not a commit log
2. Present the draft to the user for review. They may edit it.
3. Execute (after user confirms):
   ```bash
   gh pr merge <N> --squash --body "$(cat <<'EOF'
   <body>
   EOF
   )"
   ```
4. Let GitHub default the title (`PR title (#N)`).
5. NEVER use `--auto` or the automatic squash description that dumps all commits.

---

## Step 6: Post-Merge Cleanup

After successful merge:

1. Switch to the base branch and pull:
   ```bash
   git checkout <baseRefName> && git pull --rebase
   ```
2. Delete the merged feature branch locally and remotely:
   ```bash
   git branch -d <branch>
   git push origin --delete <branch>
   ```
3. **Suggest `/reflect`** if the session has extractable learnings:
   - Check session file for `## Key Decisions` with content
   - Check `traceability.decisions` for ADR entries
   - If signal present: *"This session produced key decisions. Consider running `/reflect` to update strategic docs."*
   - If none: stay silent.

---

## Hook Interaction

This skill coexists with existing hooks. Git workflow hooks are **advisory** (warn but don't block); security hooks remain blocking.

| Hook | Type | When `/release` Runs |
|------|------|---------------------|
| `workflow-pre-pr` (command) | Advisory | Fires on `gh pr create`. Reminds about CHANGELOG — redundant when release skill manages entries. |
| `workflow-pre-merge` (instruction) | Advisory | Fires on `gh pr merge`. Reminds about squash conventions — redundant since body is already crafted. |
| `workflow-post-merge` (instruction) | Advisory | Fires after merge. Outputs checklist the skill already handled. |
| `validate-push` (command) | Advisory | Fires on push. Cross-validates version bump — should pass since Step 4 did both. |
| `check-secrets` (command) | **Blocking** | Security gate. Always fires, always respected. |

Do not modify, disable, or skip these hooks.

---

## Suggests Next

After a successful release, suggest `/wrap` to summarize the session.

## Related Skills

- **implement** — Does the work and housekeeping; `/release` handles the merge afterward
- **reflect** — Suggested post-merge if session has learnings
- **git-workflow** — Conventions this skill enforces
- **housekeeping** — Handles artifact hygiene; `/release` verifies it was done
