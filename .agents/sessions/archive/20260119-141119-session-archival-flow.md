---
session:
  title: "Archival flow update"
  status: archived
  created: "2026-01-19T14:11:19Z"
  last_updated: "2026-01-20T17:45:00Z"
  archived_at: "2026-01-25T00:00:00Z"
  archived_by: "agent-pm"
---

# Session: Archival flow update

**Date:** 2026-01-19
**Owner:** PM (ChatGPT)

## Goal
Update session instructions to archive sessions on close instead of deleting them, and assess impact on existing ADRs.

## Current State
- Updated canonical guidance to archive (status + move) sessions, councils, and reports to `.agents/<type>/archive/` after processing, while preserving audit trail in sessions.
- Sessions now require council outcome summaries and report processing summaries before archive.
- Reports require frontmatter and are treated as unprocessed if missing; deletions only after session archive.
- Review workflow scans active + archive directories and enforces gating before cleanup.
- Added `archived_at` to report frontmatter and `/review-sessions` checks for missing archive timestamps.
- Added report archival cross-reference in councils guidance.
- Added `archived_at` and optional `archived_by` to sessions, councils, and reports; /review-sessions auto-moves and auto-updates links after user confirmation.
- Added explicit ban on `.agents/` links in docs outside `.agents/`.
- OpenCode PM guidance updated to require `question` tool for user interviews, pre-plan questioning, and todowrite/todoread delegation.
- Added OpenCode sidecars for all subagents to allow display-name capitalization.

## Actions
- [x] Review current session lifecycle instructions and archival/deletion guidance
- [x] Determine instruction changes needed for archive-only flow
- [x] Assess impact on existing ADRs or decisions
- [x] Draft updates for user review

## Outcome
Session archival flow updated across all guidance documents.

## Notes
- `archived_by` remains optional in templates, enforced by `/review-sessions`.
