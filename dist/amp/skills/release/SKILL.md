---
name: release
description: >-
  Orchestrates releases: pre-flight checks, version bump via `loaf release`,
  squash merge, and post-merge cleanup. Use when the user says "release this,"
  "merge this PR," "ready to merge," or "ship it." Produces version bumps,
  changelog updates, and merged code. Not for creating PRs (use git-workflow) or
  reflection (use reflect).
version: 2.0.0-dev.33
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
- **Use `AskUserQuestion` for all decisions and confirmations** -- version bump type, push approval, merge approval, branch deletion. Never use inline text questions for permission/decision prompts
- **Version bump before merge** -- this is the skill's reason for existing; never defer to post-merge
- **Wrap after version bump** -- wrap needs PR# and version to produce a complete session summary
- **Clean squash body** -- never use the auto-generated commit dump
- **Orchestrate the full lifecycle** -- if housekeeping or reflect haven't been run, trigger them (with user confirmation via `AskUserQuestion`) rather than just flagging gaps
- **Detect-first** -- auto-detect PR from branch before asking for a number
- **Never push without confirmation** -- even after successful version bump
- **Log release** -- log to session journal after merge: `loaf session log "decision(release): vX.Y.Z shipped via PR #N"`

## Verification

- Pre-flight checks (typecheck, test, build) all pass before proceeding
- Version bump commit exists on the feature branch before merge
- Squash merge body is a one-line summary followed by bullet points grouped by feature area, not a commit dump
- Git tag points to the squash merge commit on the base branch, not a branch commit
- GitHub Release exists with changelog body (not auto-generated notes)
- Post-merge cleanup completed: base branch pulled, feature branch deleted

## Quick Reference

| Step | Gate | Blocking? |
|------|------|-----------|
| Pre-Flight | typecheck + test + build pass | Yes |
| Doc Freshness | User reviews stale docs | No (user decides) |
| Housekeeping | Spec/tasks archived, CHANGELOG ready | No (user decides) |
| PR Creation | Only if no PR exists yet | Yes (if needed) |
| Version Bump | User confirms bump type; `loaf release --pre-merge` | Yes |
| Wrap | Session summary (after version bump, has PR# + version) | Yes |
| Squash Merge | User approves body text | Yes |
| Post-Merge | `loaf release --post-merge` (tag, GH Release, cleanup) | Yes |

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

## Step 3: Housekeeping

Check each item. If missing, **offer to run it** (with user confirmation) rather than just flagging it.

1. **Spec status**: If a spec is associated with this branch (check `.agents/specs/` for matching spec), verify its status is `complete`. If not, offer to update it.
2. **Tasks archived**: Check `.agents/tasks/` for tasks related to the spec that aren't archived. If found, offer to run `loaf task archive`.
3. **CHANGELOG ready**: Verify `CHANGELOG.md` exists and has the `[Unreleased]` marker (Step 4 will generate the actual entries).
4. **Journal flushed**: Review conversation for unrecorded decisions/discoveries. Log them now — this is the last chance before wrap.

Present the results as a checklist. Use `AskUserQuestion` for any decisions or approvals.

**Note:** Wrap (`/wrap`) runs AFTER version bump (Step 4b) so it can reference the PR# and version in the session summary. Reflection runs post-merge (Step 6).

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

**Review curated entries for quality before bumping:**
- Use backticks for code references (file names, commands, config keys, hook names)
- Remove internal tracking terms (tracks, phases, stages, task IDs, spec IDs)
- Write from the user's perspective — what changed, not how it was tracked

1. Run `loaf release --pre-merge --dry-run` to get the **suggested bump type** and **current version** (the `--pre-merge` flag auto-detects the base branch — see [Why `--pre-merge`?](#why---pre-merge) below).
2. Present the bump suggestion to the user. They may accept or override.
3. Once confirmed, run:
   ```bash
   loaf release --pre-merge --bump <type> --yes
   ```

   This will:
   1. Bump version in all detected files (package.json, pyproject.toml, etc.)
   2. Convert the `[Unreleased]` header to `## [X.Y.Z] - YYYY-MM-DD` (preserving the curated entries beneath it) and re-insert a fresh empty `## [Unreleased]` section above
   3. Run `loaf build` to rebuild all targets with the new version
   4. Commit: `chore: release vX.Y.Z`

### Generated path (when no entries exist)

When `[Unreleased]` is empty, use `loaf release` to auto-generate changelog entries from branch commits.

**After generation, review and rewrite entries before committing:**
- Use backticks for code references (file names, commands, config keys, hook names)
- Remove internal tracking terms (tracks, phases, stages, task IDs, spec IDs)
- Write from the user's perspective — what changed, not how it was tracked
- Keep entries concise but descriptive enough to understand the change without reading the diff

1. Run `loaf release --pre-merge --dry-run` to preview:
   - Current version and suggested bump type
   - Generated changelog section from **this branch's commits only**
   - Which version files will be updated

   The `--pre-merge` flag scopes the commit analysis to `<auto-detected base>..HEAD`, so only commits on the feature branch are considered.

2. Present the preview to the user. They may:
   - Accept the suggested bump type
   - Override with a different type (`prerelease`, `release`, `major`, `minor`, `patch`)
   - Edit the changelog content conversationally

3. Once the user confirms, run:
   ```bash
   loaf release --pre-merge --bump <type> --yes
   ```

   This will:
   1. Bump version in all detected files (package.json, pyproject.toml, etc.)
   2. Generate and insert changelog section from branch commits (adding a fresh `[Unreleased]`)
   3. Run `loaf build` to rebuild all targets with the new version
   4. Commit: `chore: release vX.Y.Z`

### After either path

Push to the feature branch (**with user confirmation**).

### Why `--pre-merge`?

`--pre-merge` bundles the canonical pre-merge flag set so the skill no longer needs to spell each one out:

- Equivalent to `--no-tag --no-gh --base <auto-detected>` (tag and GH release land post-merge in Step 6, not on the soon-to-be-squashed feature commit).
- Auto-detects the base ref via a 4-step priority: explicit `--base <ref>` → open PR's `baseRefName` → `git config loaf.release.base` → repo default branch. Run `loaf release --help` to see the full priority order.
- Pass `--base <ref>` explicitly to override auto-detection (useful for non-default base branches like `release/1.0` when no PR exists yet).
- `--yes` skips the CLI confirmation prompt — the skill already confirmed with the user conversationally.

---

## Step 4b: Wrap Session

Run `/wrap` AFTER version bump so the session summary can reference the PR# and final version.

The wrap skill will:
1. Flush any remaining journal entries
2. Gather git state (commits, working tree, unpushed)
3. Generate the `## Session Wrap-Up` report
4. Write it to the session file (replaces `## Current State`)
5. Run `loaf session end --wrap` to set status to `complete`

When called from `/release`, wrap skips the version bump and changelog prompts (already handled).

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

After successful merge, run a single command:

```bash
loaf release --post-merge
```

This verifies HEAD state against an 8-point guardrail checklist (clean worktree, on the base branch with the merge commit at HEAD, `chore: release v<semver>` subject shape, version-file agreement, CHANGELOG section present, no pre-existing tag or GH release, HEAD untagged). Once all guardrails pass it tags the squash merge commit, pushes the tag, creates the GitHub Release from the matching `## [X.Y.Z]` CHANGELOG section, pulls the base branch, and best-effort deletes the local + remote feature branch. Manual `git tag`, `git push --tags`, `gh release create`, `git checkout`, `git pull --rebase`, and `git branch -d` are no longer needed — the flag subsumes them.

If anything aborts:

1. Read the actionable error message — each guardrail failure names the exact manual fix (e.g., "tag v1.2.3 already exists locally — run `git tag -d v1.2.3` and rerun").
2. Perform the named fix.
3. Rerun `loaf release --post-merge`. The command is idempotent on stateless reads, so reruns after a transient `gh` failure or a half-completed prior run pick up exactly where they left off.

After the release finalizes, **run `/reflect`** — already on the base branch. Reflect looks back at the shipped work and updates strategic docs (VISION.md, STRATEGY.md, ARCHITECTURE.md) with learnings. Use `AskUserQuestion` to confirm before running.

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

After a successful release, suggest `/housekeeping` if session artifacts need archiving.

## Related Skills

- **implement** — Does the work and housekeeping; `/release` handles the merge afterward
- **reflect** — Suggested post-merge if session has learnings
- **git-workflow** — Conventions this skill enforces
- **housekeeping** — Handles artifact hygiene; `/release` verifies it was done
