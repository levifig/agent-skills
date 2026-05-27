---
name: documentation-standards
description: >-
  Covers ADRs, API documentation, changelogs, documentation review, and Mermaid
  diagrams. Use when writing ADRs, documenting APIs, maintaining changelogs,
  reviewing documentation quality, or creating architecture diagrams. Not for
  inline code comments (use code style guides) or project READMEs (use
  project-specific conventions).
---

# Documentation Standards

Standards for ADRs, API docs, changelogs, and diagrams.

## Contents
- Critical Rules
- Verification
- Topics

## Critical Rules

- **API docs reflect only what is implemented and released** -- never document
  future or planned endpoints in API specifications; update docs AFTER features
  ship
- **Never modify version numbers** in CHANGELOG.md without explicit user approval -- new version headers require user confirmation
- **CHANGELOG.md follows Loaf's Common Changelog profile** -- release notes are
  written for humans, communicate user/operator impact, and are curated from git
  history rather than copied from it
- **CHANGELOG entries must be release-facing** -- imperative, one-line,
  self-describing entries with useful public references; no internal spec/task
  IDs, file lists, or implementation detail without user benefit

## Verification

### After Editing Documentation Files

**CHANGELOG Validation:**
- When editing `CHANGELOG.md`, verify entries follow the format:
  - **Human impact**: What changes for users, operators, or integrators?
  - **Imperative**: Starts with a present-tense verb such as `Add`, `Fix`, `Remove`, or `Document`
  - **Self-describing**: Understandable without relying on the category heading
  - **Referenced**: Links to the best public PR, issue, release, ADR, or commit when available
  - **Concise and aggregated**: One line per meaningful change; related commits merged
- **Anti-patterns to avoid:**
  - File paths (`backend/`, `frontend/`, `.py`, `.ts`)
  - Implementation details without user benefit
  - Lists of file paths
  - Internal spec/task/session IDs
  - Verbatim commit or PR-title dumps
- **Good examples:**
  - "Add `loaf release --post-merge` guardrails for tagged GitHub releases"
  - "Fix session journal routing when hook payloads are empty"
  - "Document worktree-aware `.agents/` storage for linked checkouts"
- **Version protection:**
  - Never modify version numbers (## [X.Y.Z]) without explicit user approval
  - New version headers require user confirmation

**Format Verification:**
- Ensure all entries under `## [Unreleased]` are properly categorized:
  - Changed | Added | Removed | Fixed

### Before Committing

- Review CHANGELOG entries for file-path free, user-focused language
- Verify version numbers haven't been changed without approval
- Ensure CHANGELOG.md follows Loaf's Common Changelog profile
- Consider if commit warrants a CHANGELOG entry

### CHANGELOG Reminder

After committing, consider updating CHANGELOG.md with meaningful release-facing changes. Use Common Changelog categories in this order:
- **Changed** for changes in existing functionality
- **Added** for new functionality
- **Removed** for removed functionality
- **Fixed** for bug fixes

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Documentation | `references/documentation.md` | Writing ADRs, API docs, changelogs |
| Documentation Review | `references/documentation-review.md` | Reviewing documentation quality and completeness |
| Diagrams | `references/diagrams.md` | Creating Mermaid diagrams, visualizing architecture |
