---
name: documentation-standards
description: >-
  Covers ADRs, API documentation, changelogs, documentation review, and Mermaid
  diagrams. Use when writing ADRs, documenting APIs, maintaining changelogs,
  reviewing documentation quality, or creating architecture diagrams. Not for
  inline code comments (use code style guides) or project READMEs (use
  project-specific conventions).
version: 2.0.0-dev.33
---

# Documentation Standards

Standards for ADRs, API docs, changelogs, and diagrams.

## Contents
- Critical Rules
- Verification
- Topics

## Critical Rules

- **API docs reflect only what is implemented and released** -- never document future or planned endpoints in API specifications; update docs AFTER features ship
- **Never modify version numbers** in CHANGELOG.md without explicit user approval -- new version headers require user confirmation
- **CHANGELOG entries must be user-focused** -- no file paths, no implementation details without user benefit, no references to ADR files
- **Keep a Changelog format** -- entries categorized under Added, Changed, Fixed, Removed, Deprecated, or Security

## Verification

### After Editing Documentation Files

**CHANGELOG Validation:**
- When editing `CHANGELOG.md`, verify entries follow the format:
  - **User-focused**: What value does this provide to users?
  - **File-path free**: No paths or extensions in entries
  - **Concise**: One line, under 80 characters preferred
  - **Aggregated**: Related changes combined
- **Anti-patterns to avoid:**
  - File paths (`backend/`, `frontend/`, `.py`, `.ts`)
  - Implementation details without user benefit
  - Lists of file paths
  - References to ADR files
- **Good examples:**
  - "Circuit breaker pattern for API rate limiting"
  - "Improved pipeline documentation with architecture patterns"
  - "Consolidated documentation (redundant design docs removed)"
- **Version protection:**
  - Never modify version numbers (## [X.Y.Z]) without explicit user approval
  - New version headers require user confirmation

**Format Verification:**
- Ensure all entries under `## [Unreleased]` are properly categorized:
  - Added | Changed | Fixed | Removed | Deprecated | Security

### Before Committing

- Review CHANGELOG entries for file-path free, user-focused language
- Verify version numbers haven't been changed without approval
- Ensure CHANGELOG.md follows Keep a Changelog format
- Consider if commit warrants a CHANGELOG entry

### CHANGELOG Reminder

After committing, consider updating CHANGELOG.md with the changes made. Use categories:
- **Added** for new features
- **Changed** for changes in existing functionality
- **Fixed** for bug fixes
- **Removed** for removed features
- **Deprecated** for soon-to-be-removed features
- **Security** for security-related changes

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Documentation | `references/documentation.md` | Writing ADRs, API docs, changelogs |
| Documentation Review | `references/documentation-review.md` | Reviewing documentation quality and completeness |
| Diagrams | `references/diagrams.md` | Creating Mermaid diagrams, visualizing architecture |
