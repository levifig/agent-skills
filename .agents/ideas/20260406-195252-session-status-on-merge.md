---
status: raw
created: 2026-04-06T19:52:52Z
tags: [housekeeping, sessions, release, hooks]
related: [SPEC-026, release, housekeeping]
origin: post-merge housekeeping gap discovered during SPEC-026 release
---

# Session status not updated on merge

## Problem

The housekeeping scanner relies on session frontmatter `status` to determine if a session is archivable. But nothing in the release/merge flow automatically marks the session as `complete` — it stays `active` even after the branch is merged and deleted. This means `loaf housekeeping` misses sessions that should be archived.

Same issue with plans: the plan scanner checks if its linked session is archived/completed, but doesn't cross-reference the linked spec's status. A plan linked to an active session but a completed/archived spec won't be flagged.

## Two gaps

1. **Session status**: Post-merge hook or release skill Step 6 should run `loaf session end` to mark the session as `complete`. Currently requires manual frontmatter edit.

2. **Plan scanner**: Should also check if the plan's linked spec (via `spec:` frontmatter) is archived/completed. If the spec is done, the plan should be flagged for deletion regardless of session status.

## Constraints

- `loaf session end` already exists — just needs to be called at the right time
- The release skill's Step 6 (post-merge cleanup) is the natural place for session completion
- The plan scanner already reads spec status elsewhere — just needs the cross-reference
