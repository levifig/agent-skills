---
description: >-
  Reviews and maintains agent artifacts in .agents/ — sessions, specs, plans,
  drafts, councils, and reports. Use when the user asks "housekeeping," "clean
  up," "review sessions," or "tidy up .agents/." Provides hygiene
  recommendations, archives completed work, and ensures extracted knowledge is
  preserved. Not for strategic reflection (use reflect) or knowledge management
  (use knowledge-base).
subtask: false
version: 2.0.0-dev.34
---

# Housekeeping

Systematic review and archival of all `.agents/` artifacts with Linear-aware checks.

## Critical Rules

**Always**
- Review EVERY file individually — never sample or average
- Check Linear issue status before archiving sessions
- Extract lessons learned and decisions before archiving
- Use CLI (`loaf session housekeeping`, `loaf task archive`, `loaf spec archive`) — never raw `mv`
- Check report `status` is `processed` and linked session is archived before archiving reports (see [templates/report.md](../skills/housekeeping/templates/report.md))
- Check state assessment `session:` field before flagging for cleanup — only flag when linked session is archived
- Verify `TASKS.json` sync after archival with `loaf task sync`
- Log outcome to session journal: `loaf session log "decision(housekeeping): archived N specs, M sessions"`

**Never**
- Auto-archive without user confirmation for each artifact
- Skip spark extraction before deleting brainstorm drafts
- Leave `archived_at` or `archived_by` fields empty in archived files

## Verification

After work completes, verify:
- Session files archived with proper metadata (status, archived_at, archived_by)
- Tasks archived via `loaf task archive` (updates TASKS.json)
- Specs archived via `loaf spec archive` (updates TASKS.json)
- Drafts checked for unprocessed sparks before deletion
- Summary table presented showing all actions taken

## Quick Reference

### CLI Commands

```bash
loaf session housekeeping --dry-run  # Preview all actions
loaf session housekeeping            # Run all checks and fixes
loaf session archive                 # Archive single session
loaf session enrich <file>           # Enrich a session's journal from JSONL
loaf task archive TASK-XXX           # Archive single task
loaf spec archive SPEC-XXX           # Archive single spec
loaf task sync                       # Fix TASKS.json drift
```

### Artifact Lifecycle

| Artifact | Active Location | Archive | Action |
|----------|-----------------|---------|--------|
| Sessions | `.agents/sessions/` | `archive/` | Move with metadata |
| Tasks (local mode only) | `.agents/tasks/` | `archive/` | `loaf task archive` |
| Specs | `.agents/specs/` | `archive/` | `loaf spec archive` |
| Drafts (state assessments) | `.agents/drafts/` | delete | Flag for cleanup when linked session is archived |
| Drafts (brainstorms) | `.agents/drafts/` | `archive/` | User decision (spark extraction first) |
| Reports | `.agents/reports/` | `archive/` | Archive after processing + linked session archived |

**Linear-native mode** (when `integrations.linear.enabled` is `true` in
`.agents/loaf.json`): local `TASK-NNN.md` files do not exist for new specs —
Linear issues are the task record. The "Tasks" row above is inert unless the
project has pre-Linear local tasks lingering (see [Mode-Aware Checks](#mode-aware-checks)).
Specs still archive locally — they are the canonical deliberation artifact in
every mode.

## Session Enrichment

For each session with status `stopped` or `done` that has a `claude_session_id` in frontmatter, run `loaf session enrich <file>` to catch up on journal entries that weren't logged during the session.

Do NOT enrich `active` sessions — those are handled by the wrap skill when the session ends.

**Never enrich or archive the current conversation's session** — it is actively being written to. The current session's enrichment is handled by the wrap skill, not housekeeping.

Treat enrichment failures as non-fatal — log a warning and continue with other housekeeping tasks.

## Archival Cleanup

When archiving a session, delete its enrichment temp file if one exists at `.agents/tmp/<session-id>-enrichment.txt`. This prevents stale temp files from accumulating across sessions.

## Mode-Aware Checks

When `integrations.linear.enabled` is `true` in `.agents/loaf.json`, apply
these additional checks:

### Spec / Linear parent reconciliation

For each spec file (active and archive) with a `linear_parent:` frontmatter key:

1. Call `get_issue` with the issue identifier. If it 404s or returns
   archived/deleted, flag as **orphaned linear_parent** — the local spec
   references a Linear issue that no longer exists.
2. If the spec's local status is `complete` or `archived`, verify the Linear
   parent issue is in a `completed`-type state. If not (e.g., still "In
   Progress"), flag as **status mismatch** — "Spec marked complete locally
   but Linear parent ENG-198 is still 'In Progress'."
3. If the spec's local status is `implementing` and the Linear parent is
   already `completed`, flag the inverse — spec likely needs to be moved to
   `complete` and archived.

Treat all three as **warnings**, not auto-fixes. The user decides resolution.

### Pre-Linear local task detection

If Linear is enabled but local `TASK-NNN.md` files exist in `.agents/tasks/`,
surface them with context: "Pre-Linear local tasks detected. These aren't
auto-migrated. Either continue using them, run a manual migration, or
archive if superseded by Linear issues."

Do NOT auto-migrate. Migration is user-initiated and out of scope for
housekeeping.

## Suggests Next

After housekeeping, suggest `/reflect` if the session produced key decisions or learnings worth integrating into strategic docs.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Report Template | [templates/report.md](../skills/housekeeping/templates/report.md) | Creating cleanup reports |
| Linear Integration | `orchestration/references/linear.md` | Checking external issue status |
| Session Management | `orchestration/references/sessions.md` | Understanding session lifecycle |
