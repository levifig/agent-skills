---
name: git-workflow
description: >-
  Covers branching strategies, commit conventions, PR creation, and squash merge
  workflow. Use when creating branches, writing commits, creating or merging
  PRs, or managing git history. Provides patterns for collaborative git
  workflows. Not for code style (use foundations) or CI/CD pipelines (use
  infrastructure-management).
---

# Git Workflow

Git conventions for branching, commits, PRs, and merge workflow.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- Use Conventional Commits format for all commit messages
- Working-branch commits are complete implementation checkpoints. Keep each one atomic and buildable enough to support review, diagnosis, and safe continuation; do not commit partial or knowingly broken work.
- A pull request is one shippable unit. Squash merge a reviewed feature or shippable-root PR into the default branch with a deliberately authored Conventional Commit title and useful extended description; never use an automatic dump of branch commit messages.
- Related child or stacked branches join their shippable root without synthetic topology. Verify ancestry with `git merge-base --is-ancestor <root> <child>`, then use `git merge --ff-only <child>` from the root branch. If ancestry diverged, stop and reassess instead of creating a merge commit by default.
- Merge commits are exceptions. Preserve one only when topology, provenance, or a long-lived integration is itself durable project information, and record the explicit rationale. Never create one merely to assemble related feature work.
- Independent shippable roots must not disappear inside one giant squash solely because they share an implementation branch. Split them into separate reviewed PRs, or obtain an explicit human decision that one atomic landing is safer.
- One branch per shippable root. Read the canonical native tracker record through the selected provider, then use the repository's branch convention (normally `feat/<slug>`, `fix/<slug>`, or `chore/<slug>`). Related child work shares the parent's branch and PR when the tracker hierarchy says it is one shipping unit.
- Never force-push to `main` or shared branches
- Never push without explicit user confirmation

## Verification

- Commit messages follow unscoped Conventional Commits format (`type: description`)
- Every working-branch commit is a complete checkpoint, and the candidate diff represents exactly one shippable root
- Stacked work was verified as ancestral and assembled with `--ff-only`; no incidental merge commit exists
- Independent shippable roots have separate PRs unless an explicit human decision documents why one atomic landing is safer
- Branch is up to date with its base branch before creating the PR
- PR title is under 70 characters with PR# suffix convention
- The squash title and extended description describe the shipped outcome rather than replaying implementation commits

## Quick Reference

| Action | Command/Pattern |
|--------|----------------|
| Branch naming | Repository convention, normally `feat/{slug}`, `fix/{slug}`, or `chore/{slug}`; include the native tracker key only when the repository convention calls for it |
| Commit format | `type: description` |
| Assemble stacked child | Verify `git merge-base --is-ancestor <root> <child>`, then run `git merge --ff-only <child>` from the root |
| Squash shippable PR | `gh pr merge --squash`; author the final title and description deliberately |
| Preserve merge topology | Exceptional only; require explicit durable rationale before using a merge commit |
| PR creation | `gh pr create --title "..." --body "..."` |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Commits | `references/commits.md` | Writing commit messages, creating PRs, branching, curating CHANGELOG entries, pre-PR/pre-push/post-merge hooks |
| PR shipping | ship skill | Reviewing, verifying, and squash-merging one PR |
| Release ritual | release skill | Publishing a version from already-landed work |
