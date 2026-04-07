---
name: git-workflow
description: >-
  Covers branching strategies, commit conventions, PR creation, and squash merge
  workflow. Use when creating branches, writing commits, creating or merging
  PRs, or managing git history. Provides patterns for collaborative git
  workflows. Not for code style (use foundations) or CI/CD pipelines (use
  infrastructure-management).
version: 2.0.0-dev.16
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
- Commit complete units of work -- don't commit partial or in-progress changes
- Squash merge feature branches -- never merge commits directly
- One branch per spec/feature; branch name format: `feat/{slug}`
- Never force-push to `main` or shared branches
- Never push without explicit user confirmation

## Verification

- Commit messages follow Conventional Commits format (`type(scope): description`)
- Branch is up to date with base branch before creating PR
- PR title is under 70 characters with PR# suffix convention

## Quick Reference

| Action | Command/Pattern |
|--------|----------------|
| Branch naming | `feat/{slug}`, `fix/{slug}`, `chore/{slug}` |
| Commit format | `type(scope): description` |
| Squash merge | `gh pr merge --squash` |
| PR creation | `gh pr create --title "..." --body "..."` |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Commits | `references/commits.md` | Writing commit messages, creating PRs, branching, pre-PR/pre-push/post-merge hooks |
| Release ritual | `/release` skill | Orchestrating the full squash merge workflow (pre-flight, version bump, merge, cleanup) |
