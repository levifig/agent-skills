---
name: ship
description: >-
  Reviews, verifies, and lands one pull request. Use when the user says "ship
  it," "merge this PR," "ready to merge," "land this branch," or asks for a
  final merge gate. Produces a reviewed, squash-merged PR and post-merge
  cleanup. Not for version b...
user-invocable: true
argument-hint: '[PR number or URL]'
version: 2.0.0-alpha.11
---

# Ship

Review, verify, and land one PR. Shipping is the PR gate; releasing is the version-publication gate.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Context Detection
- Step 1: PR Readiness
- Step 2: Evidence Review
- Step 3: Local Verification
- Step 4: Squash Merge
- Step 5: Post-Merge Cleanup
- Step 6: Release Suggestion
- Hook Interaction
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

- **Ship is not release** -- do not bump versions, create tags, publish GitHub Releases, or verify package installation here.
- **Keep PR quality local** -- smaller PRs are welcome, but `/loaf:ship` must still verify correctness before merge.
- **Detect-first** -- auto-detect the PR from the current branch before asking for a PR number.
- **Review before merge** -- inspect code, docs, tests, changelog, PR body, and CI state before approval.
- **Never merge without explicit confirmation** -- present the PR, checks, findings, and squash body first.
- **Clean squash body** -- write an intentional squash commit body; never accept the automatic commit dump.
- **Keep landed and released distinct** -- after merge, describe the PR as landed or shipped, not necessarily released.
- **Log shipping** -- after merge, run `loaf journal log "decision(ship): PR #N landed via squash merge"`.

## Verification

- PR identity, base branch, and head branch are confirmed
- CI status is passing or the user explicitly accepts named non-blocking checks
- Relevant local checks pass or failures are fixed before merge
- PR body and durable docs do not overclaim relative to the diff
- Squash commit title/body are clean, conventional, and user-facing
- Base branch is updated after merge and the feature branch cleanup state is known

## Quick Reference

| Step | Gate | Blocking? |
|------|------|-----------|
| PR Readiness | PR exists, target base known, CI state reviewed | Yes |
| Evidence Review | findings resolved or explicitly accepted | Yes |
| Local Verification | relevant checks pass | Yes |
| Squash Merge | user approves body text | Yes |
| Cleanup | base pulled, branch deletion handled | No |
| Release Suggestion | enough landed work may justify `/loaf:release` | No |

## Topics

| Topic | Use When |
|-------|----------|
| [Context Detection](#context-detection) | Determining current branch and PR state |
| [Hook Interaction](#hook-interaction) | Understanding coexistence with git hooks |

---

## Context Detection

Before anything, detect the PR surface:

1. Get current branch and repo default branch:
   ```bash
   git branch --show-current
   gh repo view --json defaultBranchRef -q .defaultBranchRef.name
   ```
2. Parse `$ARGUMENTS`: may be a PR number, PR URL, branch name, or empty.
3. If `$ARGUMENTS` is empty, auto-detect from the current branch:
   ```bash
   gh pr view --json number,title,url,headRefName,baseRefName,state,mergeStateStatus,isDraft
   ```
4. If no PR exists for the current branch, stop and offer to create one via `git-workflow` rather than silently merging a branch.
5. If already on the default branch, stop. There is no PR to ship from the current branch.
6. Confirm PR identity with the user before merge actions.

---

## Step 1: PR Readiness

Inspect the PR's declared state:

```bash
gh pr view <N> --json number,title,body,url,headRefName,baseRefName,state,isDraft,mergeStateStatus,reviewDecision,statusCheckRollup
```

Block or pause when:

- PR is draft
- merge state is dirty or blocked
- required checks are failing or pending
- review decision is changes requested
- branch is out of date and the project requires update before merge

If checks are unavailable, say so explicitly and compensate with local verification.

---

## Step 2: Evidence Review

Review the landing diff and durable prose together:

1. Gather diff context:
   ```bash
   git fetch origin <baseRefName>
   git diff --stat origin/<baseRefName>...HEAD
   git diff --name-only origin/<baseRefName>...HEAD
   ```
2. Read the PR title/body and changed docs that make behavior claims.
3. Check for drift:
   - PR body claims features that are not in the diff
   - changelog entries mention unreleased or unrelated behavior
   - docs describe future work as already shipped
   - comments or runbooks use stale internal vocabulary
4. Fix blocking drift before merge. For non-blocking polish, name it and let the user decide.

For high-risk PRs, use the project's review skill or read-only review flow before proceeding.

---

## Step 3: Local Verification

Run the checks the project supports. Examples:

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

Draft a clean squash body from the reviewed diff and PR body:

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

After a successful merge:

1. Switch to the PR base branch:
   ```bash
   git checkout <baseRefName>
   git pull --ff-only origin <baseRefName>
   ```
2. Delete the local feature branch when safe:
   ```bash
   git branch -d <headRefName>
   ```
3. Confirm the remote branch deletion state from GitHub output or run:
   ```bash
   gh pr view <N> --json headRefName,state
   ```
4. Log the landing to the project journal:
   ```bash
   loaf journal log "decision(ship): PR #N landed via squash merge"
   ```

If cleanup fails, report the exact residual state. Do not force-delete without user confirmation.

---

## Step 6: Release Suggestion

After landing, decide whether to suggest `/loaf:release`:

- Suggest `/loaf:release` when the landed PR completes a coherent batch, user-facing feature, fix train, or release branch.
- Do not suggest `/loaf:release` for every small PR by default.
- If multiple related PRs are expected, say the PR is landed and can wait for a later batched release.

Use language carefully: the PR is **landed** or **shipped**; it is not **released** until `/loaf:release` publishes a version.

---

## Hook Interaction

This skill coexists with existing hooks.

| Hook | Type | When `/loaf:ship` Runs |
|------|------|------------------|
| `github-account` | Force-switch | Switches to the configured GitHub account before `gh` PR operations; blocks only if the switch fails |
| `workflow-pre-merge` | Advisory | Fires on `gh pr merge`; use it as a final squash reminder |
| `workflow-post-merge` | Advisory | Fires after merge; use it as a cleanup reminder |
| `validate-push` | Advisory | May fire if branch updates are needed before merge |
| `check-secrets` | Blocking | Always respected before writes or shell actions |

Do not disable hooks to force a PR through.

---

## Suggests Next

After a successful ship, suggest `/loaf:release` only when the landed work forms a coherent release batch or the user asks to publish.

## Related Skills

- **release** -- Publishes a version from already-landed work
- **git-workflow** -- Branching, PR, commit, and squash merge conventions
- **foundations** -- Verification, code review, and production readiness
- **documentation-standards** -- Changelog, docs, and durable prose quality
- **reflect** -- Updates strategy from significant shipped work
