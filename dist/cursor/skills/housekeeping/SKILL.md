---
name: housekeeping
description: >-
  Reviews and maintains agent artifacts in .agents/ — sessions, specs, plans,
  drafts, handoffs, councils, and reports. Use when the user asks
  "housekeeping," "clean up," "review sessions," or "tidy up .agents/." Provides
  hygiene recommendations, archives completed work, and ensures extracted
  knowledge is preserved. Not for strategic reflection (use reflect) or
  knowledge management (use knowledge-base).
version: 2.0.0-pre.20260625183349
---

# Housekeeping

## Contents
- Critical Rules
- Verification
- Quick Reference
- Session Enrichment
- Mode-Aware Checks
- Process
- Guardrails
- Related Skills

Systematic review and archival of all `.agents/` artifacts with Linear-aware checks.

## Critical Rules

**Always**
- Log invocation as the first action: `loaf session log "skill(housekeeping): <scope or trigger>"`
- Review EVERY file individually — never sample or average
- Check Linear issue status before archiving sessions
- Extract lessons learned and decisions before archiving
- Use CLI (`loaf housekeeping`, `loaf session archive`, `loaf task archive`, `loaf spec archive`) — never raw `mv`
- Treat `.agents/handoffs/` as first-class but disposable: keep active/final handoffs, delete only after confirmed deprecated status
- Check report `status` is `processed` and linked session is archived before archiving reports (see [templates/report.md](templates/report.md))
- Check state assessment `session:` field before flagging for cleanup — only flag when linked session is archived
- In SQLite-backed projects, verify lifecycle changes through `loaf task list --json`, `loaf spec list --json`, and `loaf report list --json`; use `loaf task sync` only for Markdown compatibility repair
- When delegated background agent are available, use the `librarian` profile for
  `.agents/`-scoped durable artifact tending: session/report/spec/handoff
  hygiene, staleness notes, and lifecycle-safe cleanup recommendations.
  Housekeeping still owns user confirmation and final archive decisions.
- Log outcome to session journal: `loaf session log "decision(housekeeping): archived N specs, M sessions"`

**Never**
- Auto-archive without user confirmation for each artifact
- Skip spark extraction before deleting brainstorm drafts
- Leave `archived_at` or `archived_by` fields empty in archived files

## Verification

After work completes, verify:
- Session lifecycle state is visible through `loaf session list --json`
- Tasks archived via `loaf task archive`
- Specs archived via `loaf spec archive`
- SQLite-backed task/spec/report state reflects lifecycle changes when initialized
- Drafts checked for unprocessed sparks before deletion
- Handoffs deleted only after explicit deprecation is confirmed
- Summary table presented showing all actions taken

## Quick Reference

### CLI Commands

```bash
loaf housekeeping --dry-run          # Preview recommendations
loaf housekeeping                    # Run artifact scanner
loaf housekeeping --sessions         # Review sessions only
loaf session archive                 # Archive single session
loaf session enrich --json           # Compatibility diagnostic for legacy enrichment
loaf task archive TASK-XXX           # Archive single task
loaf spec archive SPEC-XXX           # Archive single spec
loaf task sync                       # Compatibility: repair Markdown task index drift
```

### Artifact Lifecycle

| Artifact | Active Location | Archive | Action |
|----------|-----------------|---------|--------|
| Sessions | SQLite state + `.agents/sessions/` compatibility views | `archive/` | `loaf housekeeping --sessions` / `loaf session archive` |
| Tasks (local mode only) | SQLite state + `.agents/tasks/` source prose | `archive/` | `loaf task archive` |
| Specs | SQLite state + `.agents/specs/` authored prose | `archive/` | `loaf spec archive` |
| Drafts (state assessments) | `.agents/drafts/` | delete | Flag for cleanup when linked session is archived |
| Drafts (brainstorms) | `.agents/drafts/` | `archive/` | User decision (spark extraction first) |
| Handoffs | `.agents/handoffs/` | delete | Delete after status is confirmed `deprecated` |
| Reports | SQLite state + generated/authored report Markdown | `archive/` | `loaf report archive` after processing + linked session archived |

**Linear-native mode** (when `integrations.linear.enabled` is `true` in
`.agents/loaf.json`): local `TASK-NNN.md` files do not exist for new specs —
Linear issues are the task record. The "Tasks" row above is inert unless the
project has pre-Linear local tasks lingering (see [Mode-Aware Checks](#mode-aware-checks)).
Specs still archive locally — they are the canonical deliberation artifact in
every mode.

## Session Enrichment

In SQLite-backed projects, `loaf session log` is the canonical journal writer and
`wrap` owns end-of-session checks. `loaf session enrich --json` is a compatibility
diagnostic for legacy markdown enrichment; it is not part of the normal
housekeeping flow.

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
| Report Template | [templates/report.md](templates/report.md) | Creating cleanup reports |
| Linear Integration | `orchestration/references/linear.md` | Checking external issue status |
| Session Management | `orchestration/references/sessions.md` | Understanding session lifecycle |
