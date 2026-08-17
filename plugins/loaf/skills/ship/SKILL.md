---
name: ship
description: >-
  Reviews, verifies, and lands one pull request — the sole quality gate before
  work can appear in a later release cut. Use when the user says "ship it,"
  "merge this PR," "ready to merge," "land this branch," or asks for a final
  merge gate. Binds the PR to an issue: the body is `loaf issue render` output,
  definition-of-done criteria are the review checklist, and landing marks the
  issue done and stops its worktree. Produces a reviewed, squash-merged PR and
  post-merge cleanup. Not for version bumps, tags, GitHub Releases, or install
  verification (use release).
user-invocable: true
argument-hint: '[PR number or URL]'
version: 0.3.1
---

# Ship

Review, verify, and land one PR. Ship's review is the quality gate for everything that will ever ship. Releases are retroactive — they cut a version from already-landed work. Nothing between merge and the next release cut re-checks the diff, the tests, or the issue. A rubber-stamped PR ships in the next cut with no second net.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Context Detection
- Step 1: PR Readiness
- Step 1b: Stacked PR Detection
- Step 2: Definition-of-Done Review
- Step 3: Local Verification
- Step 4: Squash Merge
- Step 5: Post-Merge Cleanup
- Step 6: Release Suggestion
- Hook Interaction
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

1. **Log invocation first** — `loaf journal log "skill(ship): shipping <ref or PR or current branch>"` before doing anything else. After merge, log `loaf journal log "decision(ship): PR #N landed via squash merge; <ref> done"`.
2. **Rigor is load-bearing** — this review is the only quality gate. Releases cut from landed work; they do not re-check. If the review is thin, the next cut still publishes it.
3. **Ship is not release** — do not bump versions, create tags, publish GitHub Releases, or verify package installation here. Use the release skill for that.
4. **Bind the PR to an issue** — the PR body is `loaf issue render <ref>` (paste-ready, no manual editing). The issue's definition-of-done criteria are the review checklist. `loaf issue verify <ref>` runs the executable (V-tier) rows and writes nothing. Landing means `loaf issue status <ref> done`. Then `loaf issue stop <ref>` removes the started worktree.
5. **Detect-first** — auto-detect the PR from the current branch, and the issue from `$ARGUMENTS` or the started workspace, before asking for a PR number or issue ref.
6. **Review before merge** — inspect code, docs, tests, changelog, the rendered issue body, definition of done, and CI state before approval.
7. **Never merge without explicit confirmation** — present the PR, checks, review notes, and squash body first, using your harness's structured question tool if it has one.
8. **Detect the stack before merging** — another open PR may use this PR's head branch as its base. Find out before merge, never after, and never delete a head branch while a child PR still points at it.
9. **Clean squash body** — write an intentional squash commit body; never accept the automatic commit dump.
10. **Keep landed and released distinct** — after merge, describe the PR as landed or shipped. It is not released until release publishes a version.

---

## Verification

- Invocation is logged to the project journal before review work begins
- PR identity, base branch, and head branch are confirmed
- The PR is bound to one issue; `loaf issue show <ref>` is the issue surface
- PR body matches `loaf issue render <ref>` with no manual editing
- Every definition-of-done criterion was reviewed against the diff; H-tier by reading, V-tier by `loaf issue verify <ref>` (writes nothing; exit non-zero blocks)
- CI status is passing or the user explicitly accepts named non-blocking checks
- Relevant local checks pass or failures are fixed before merge
- PR body and durable docs do not overclaim relative to the diff
- Squash commit title/body are clean, conventional, and user-facing
- Child PRs stacked on this PR's head branch are enumerated before merge, and each is retargeted, rebased, and re-verified after it
- After merge: `loaf issue status <ref> done`, then `loaf issue stop <ref>` if a worktree was started, base branch updated, feature-branch cleanup state known
- A `decision(ship)` journal entry records the landing

## Quick Reference

| Step | Gate | Blocking? |
|------|------|-----------|
| Context Detection | PR and issue bound | Yes |
| PR Readiness | PR exists, target base known, CI state reviewed | Yes |
| Stacked PR Detection | child PRs on this head branch are enumerated | Yes |
| Definition-of-Done Review | every criterion reviewed; `loaf issue verify` passes or reports no V-tier rows | Yes |
| Local Verification | relevant project checks pass | Yes |
| Squash Merge | user approves body text | Yes |
| Cleanup | issue marked done, started worktree stopped, base pulled, children retargeted and rebased, branch deletion handled | Yes when a child PR exists; done + stop always |
| Release Suggestion | enough landed work may justify a later cut | No |

## Topics

| Topic | Use When |
|-------|----------|
| [Context Detection](#context-detection) | Binding the current branch to a PR and an issue |
| [Definition-of-Done Review](#step-2-definition-of-done-review) | Using issue criteria as the merge checklist |
| [Hook Interaction](#hook-interaction) | Understanding coexistence with git hooks |

---

## Context Detection

Log the invocation, then detect the PR and the issue.

### PR

1. Get current branch and repo default branch:
   ```bash
   git branch --show-current
   gh repo view --json defaultBranchRef -q .defaultBranchRef.name
   ```
2. Parse `$ARGUMENTS`: may be an issue ref (`LOAF-42`), a PR number, a PR URL, a branch name, or empty.
3. If `$ARGUMENTS` is empty or is not a PR identity, auto-detect from the current branch:
   ```bash
   gh pr view --json number,title,url,headRefName,baseRefName,state,mergeStateStatus,isDraft
   ```
4. If no PR exists for the current branch, stop and offer to create one via `git-workflow` rather than silently merging a branch.
5. If already on the default branch, stop. There is no PR to ship from the current branch.

### Issue

Issue commands require initialized SQLite state. Bind exactly one issue:

1. If `$ARGUMENTS` (or a remaining token) is an issue ref, load it:
   ```bash
   loaf issue show <ref>
   ```
2. Otherwise match the PR's `headRefName` to a started workspace:
   ```bash
   loaf issue list --started
   ```
   Columns are alias, title, `started_branch`, `started_worktree`. The started branch from `loaf issue start` is `issue/<alias-or-id>` in lowercase (`issue/loaf-42`), disambiguated with an id suffix when that name is already claimed.
3. Confirm with `loaf issue show <ref>` — `started_branch` / `started_worktree` should match this PR when the issue was started.

If nothing binds, stop and ask for the issue ref. Do not invent a row during ship. If `loaf issue show` reports the issue archived (`cancelled` or `duplicate`), stop.

Confirm PR identity and the bound issue with the user before merge actions.

---

## Step 1: PR Readiness

Inspect the PR's declared state:

```bash
gh pr view <N> --json number,title,body,url,headRefName,baseRefName,state,isDraft,mergeStateStatus,reviewDecision,statusCheckRollup
```

The body must be `loaf issue render <ref>` output — title, issue body, definition-of-done checkboxes (checked only when status is already `done`), and children. No project headers, no hand-edited summary. If the live PR body differs, replace it:

```bash
gh pr edit <N> --body "$(loaf issue render <ref>)"
```

Do not rewrite the markdown by hand. Checkboxes are unchecked until `loaf issue status <ref> done`; do not tick them in the PR body to fake completion.

Block or pause when:

- PR is draft
- merge state is dirty or blocked
- required checks are failing or pending
- review decision is changes requested
- branch is out of date and the project requires update before merge

If checks are unavailable, say so explicitly and compensate with local verification.

---

## Step 1b: Stacked PR Detection

Before merging anything, find out whether another open PR uses this PR's head branch as its base:

```bash
gh pr list --state open --base <headRefName> --json number,title,headRefName
```

Any result is a **child PR**, and this PR is the base of a stack. Record the list now, because after the merge the relationship is harder to see and easier to break. Child PRs are GitHub stacking, not child issues.

When a child PR exists:

- **Do not pass `--delete-branch` to the merge.** Removing the head branch while a child still points at it can close the child outright. Delete it only after every child has been retargeted, in Step 5.
- Tell the user the stack exists and name the children before asking for merge confirmation. A stack changes what "merge this" means.

When no child PR exists, say so, and `--delete-branch` is safe.

---

## Step 2: Definition-of-Done Review

The issue's definition of done is the merge checklist. Load it from the issue, not from memory:

```bash
loaf issue dod list <ref>
loaf issue show <ref>
```

`show` prints each criterion as `position. [V|H] text` with `command=` / `expect=` when present. Walk every row against the landing diff.

1. Gather diff context:
   ```bash
   git fetch origin <baseRefName>
   git diff --stat origin/<baseRefName>...HEAD
   git diff --name-only origin/<baseRefName>...HEAD
   ```
2. For each **H-tier** criterion, read the diff and durable prose and decide whether the text is met. `loaf issue verify` skips H-tier rows — that skip is not a pass; you are the check.
3. Run the **V-tier** rows:
   ```bash
   loaf issue verify <ref>
   ```
   Commands run from the **repository root**. The command honors `exit <N>` and `` contains `text` ``. It writes nothing — it does not tick checkboxes, does not set status, and does not record a run. Non-zero exit blocks merge. `no executable V-tier criteria on <ref>` is not a failure; H-tier review still is. Unenforceable expect clauses print as advisory warnings and are never quietly decorative.
4. Check for drift:
   - Rendered issue body claims features that are not in the diff
   - changelog entries mention unrelated behavior
   - docs describe future work as already shipped
   - comments or runbooks use stale vocabulary
5. If `loaf issue show` lists child issues that are not `done`, name them before asking to merge. Do not mark those children done unless this PR is theirs.
6. Fix blocking drift and unmet criteria before merge. For non-blocking polish, name it and let the user decide.

For high-risk PRs, use the project's review skill or read-only review flow before proceeding.

---

## Step 3: Local Verification

Run the checks the project supports, in addition to `loaf issue verify`. Examples:

- Node: `npm run typecheck`, `npm run test`, `npm run build`
- Go: `go vet ./...`, `go test ./...`
- Python: `pytest`, `mypy .`, `ruff check .`
- Rust: `cargo check`, `cargo test`

If generated artifacts are tracked, verify they are current before merge:

```bash
git diff --exit-code -- dist plugins
```

Use the repo's documented pre-commit or pre-PR checklist when present. Stop on failures.

---

## Step 4: Squash Merge

Draft a clean squash body from the reviewed diff and the rendered issue:

- One-line summary, then bullet points grouped by feature area
- Plain text; use backticks only for code identifiers
- No commit dump
- No agent attribution
- No release-note overclaiming

Present the body and ask for confirmation. Then run:

```bash
gh pr merge <N> --squash --body "$(cat <<'EOF'
<body>
EOF
)"
```

Let GitHub default the title from the PR title so the squash subject remains `type: summary (#N)`.

---

## Step 5: Post-Merge Cleanup

After a successful merge, leave the started worktree before removing it. Do not run `loaf issue stop` from inside that worktree.

1. Switch to the PR base branch in the repository checkout:
   ```bash
   git checkout <baseRefName>
   git pull --ff-only origin <baseRefName>
   ```
2. Mark the bound issue done — this is what "done" means; `loaf issue stop` does not change status:
   ```bash
   loaf issue status <ref> done
   ```
   Write statuses (`triage`, `backlog`, `todo`, `active`, `done`) update in place. Do not use `cancelled` or `duplicate` here.
3. Stop the started worktree if one exists. `loaf issue stop` removes the worktree, clears `started_branch` / `started_worktree` on the row, and **keeps the branch**:
   ```bash
   loaf issue stop <ref>
   ```
   If the issue was never started, the command errors with `issue <ref> is not started` — treat that as already clean and continue. If the worktree is dirty, do not pass `--force` without user confirmation.
4. Delete the local feature branch when safe:
   ```bash
   git branch -d <headRefName>
   ```
5. Confirm the remote branch deletion state from GitHub output or run:
   ```bash
   gh pr view <N> --json headRefName,state
   ```
6. Log the landing:
   ```bash
   loaf journal log "decision(ship): PR #N landed via squash merge; <ref> done"
   ```

If cleanup fails, report the exact residual state (issue status, whether the worktree is still started, which branches remain). Do not force-delete without user confirmation.

### Stacked child PRs

Every child PR recorded in Step 1b needs three repairs before it can ship, and none of them happen on their own.

**Retarget the base.** GitHub does not reliably move a child's base when its base branch merges. Check, and move it explicitly:

```bash
gh pr view <child> --json baseRefName -q .baseRefName    # still the merged head branch?
gh pr edit <child> --base <baseRefName>                  # the parent's base, normally main
```

**Rebase the child onto the new base.** A squash merge replaces the parent's commits with one new commit, so the child still carries the originals and its diff will re-apply everything the parent already landed. Rebasing drops them:

```bash
git rebase <baseRefName>   # skips the already-applied commits, reports which
```

**Assert the diff collapsed.** This is the deterministic signal that the rebase actually took. Compare the child's file count before and after; it must fall to the child's own scope. A count that does not shrink means the rebase did not drop the parent's content, and merging would re-land it.

```bash
git diff --stat <baseRefName>...HEAD | tail -1
gh pr view <child> --json changedFiles -q .changedFiles
```

Then force-push with a lease, wait for CI to finish on the **rebased head** rather than trusting the previous run, and only then ship the child. Delete the parent's head branch once no child PR points at it.

Report the numbers rather than asserting success: the before and after file counts, the commits the rebase skipped, and the CI conclusion on the new head.

---

## Step 6: Release Suggestion

After landing, decide whether to suggest the release skill. That cut will not re-run this review.

- Suggest release when the landed PR completes a coherent batch, user-facing feature, or fix train.
- Do not suggest release for every small PR by default.
- If more related PRs are expected, say this PR is landed and can wait for a later cut.

Use language carefully: the PR is **landed** or **shipped**; it is not **released** until release publishes a version.

---

## Hook Interaction

This skill coexists with existing hooks.

| Hook | Type | When ship Runs |
|------|------|------------------|
| `github-account` | Force-switch | Switches to the configured GitHub account before `gh` PR operations; blocks only if the switch fails |
| `workflow-pre-merge` | Advisory | Fires on `gh pr merge`; use it as a final squash reminder |
| `workflow-post-merge` | Advisory | Fires after merge; use it as a cleanup reminder |
| `validate-push` | Advisory | May fire if branch updates are needed before merge |
| `check-secrets` | Blocking | Always respected before writes or shell actions |

Do not disable hooks to force a PR through.

---

## Suggests Next

After a successful ship, suggest release only when the landed work forms a coherent batch or the user asks to publish. Release will not re-check the landed PR.

## Related Skills

- **release** — Publishes a version from already-landed work; does not re-review those PRs
- **git-workflow** — Branching, PR, commit, and squash merge conventions
- **foundations** — Verification, code review, and production readiness
- **documentation-standards** — Changelog, docs, and durable prose quality
- **reflect** — Updates strategy from significant shipped work
