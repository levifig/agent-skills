---
description: >-
  Reviews and maintains agent artifacts in .agents/ plus issue hygiene —
  reports, handoffs, councils, archived issues, and stale started worktrees. Use
  when the user asks "housekeeping," "clean up," or "tidy up .agents/." Provides
  hygiene recommendations, archives completed work, and ensures extracted
  knowledge is preserved. Not for strategic reflection (use reflect) or
  knowledge management (use knowledge-base).
subtask: false
version: 0.3.0
---

# Housekeeping

## Contents
- Critical Rules
- Verification
- Quick Reference
- Mode-Aware Checks
- Suggests Next
- Topics
- Artifact Naming

Systematic review of `.agents/` artifacts and issue workspaces.

## Critical Rules

**Always**
- Log invocation as the first action: `loaf journal log "skill(housekeeping): <scope or trigger>"`
- Review EVERY file individually — never sample or average
- Check Loaf issue status (and Linear overlay, if enabled) before archiving linked artifacts
- Extract lessons learned and decisions before archiving
- Use CLI (`loaf housekeeping`, `loaf report archive`, `loaf issue status` / `loaf issue stop`) — never raw `mv`
- Treat `.agents/handoffs/` as first-class but disposable: keep active/final handoffs, delete only after confirmed deprecated status
- Check report `status` is `done` (or `final`) before archiving reports (see [templates/report.md](templates/report.md))
- In SQLite-backed projects, verify lifecycle through `loaf issue list --json`, `loaf issue list --started`, `loaf issue list --archived`, and `loaf report list --json`
- When delegated subagents are available, use the `librarian` profile for
  `.agents/`-scoped durable artifact tending: report/handoff hygiene,
  staleness notes, and lifecycle-safe cleanup recommendations.
  Housekeeping still owns user confirmation and final archive decisions.
- Log outcome to the project journal: `loaf journal log "decision(housekeeping): archived N reports; stopped M stale worktrees"`

**Never**
- Auto-archive without user confirmation for each artifact
- Skip spark extraction before deleting brainstorm drafts
- Leave `archived_at` or `archived_by` fields empty in archived files
- Run `loaf issue stop` from inside the started worktree
- Dispatch cleanup agents into a live started worktree another agent occupies

## Verification

After work completes, verify:
- Reports archived via `loaf report archive` after processing
- Archived issues reviewed via `loaf issue list --archived` (`cancelled` / `duplicate` archive through `loaf issue status`)
- Stale started worktrees reviewed via `loaf issue list --started` (a `(missing)` marker means the recorded path is gone)
- SQLite-backed report/issue state reflects lifecycle changes when initialized
- Drafts checked for unprocessed sparks before deletion
- Handoffs deleted only after explicit deprecation is confirmed
- Summary table presented showing all actions taken

## Quick Reference

### CLI Commands

```bash
loaf housekeeping --dry-run          # Preview recommendations
loaf housekeeping                    # Run artifact scanner
loaf issue list --started            # Started worktrees (alias, title, branch, path)
loaf issue list --archived           # cancelled / duplicate rows
loaf issue stop <ref>                # Remove worktree; keeps branch; does not change status
loaf issue status <ref> cancelled    # Archive an abandoned issue
loaf issue status <ref> duplicate --duplicate-of <surviving>
loaf report archive <report>         # Archive a processed report
loaf issue absorb --all --dry-run    # Preview leftover TASK/INTENT to issue
loaf issue absorb --all --history --dry-run  # Include done/archived leftover rows
```

`loaf housekeeping` still prints leftover `specs` / `tasks` sections when those
SQLite tables have rows. Preview leftover TASK/INTENT work with
`loaf issue absorb --all --dry-run`. Add `--history` when done or archived
leftover rows should move too. Persist with the same command without `--dry-run`.
A single leftover row is `loaf issue absorb <ref>` (or `--dismiss` to archive
without minting). Do not invent a bulk import. Change-local
`docs/changes/**/tasks/` are never imported. Do not create new records on the
leftover tables. The `loaf task` / `loaf spec` CLI is legacy.

The project journal is append-only and never archived — it is not a housekeeping
target. It is the canonical record housekeeping reads when extracting decisions
before archiving other artifacts.

### Artifact Lifecycle

| Artifact | Active Location | Archive | Action |
|----------|-----------------|---------|--------|
| Issues | SQLite (`loaf issue list`) | `cancelled` / `duplicate` via `loaf issue status` | Confirm, then status; `done` is ship, not housekeeping |
| Started worktrees | `loaf issue list --started` | `loaf issue stop <ref>` | Stop stale or `(missing)` trees after confirmation |
| Drafts / brainstorms | SQLite state | SQLite resolved/archived status | User decision (spark extraction first) |
| Handoffs | `.agents/handoffs/` | delete | Delete after status is confirmed `deprecated` |
| Reports | SQLite state + generated/authored report Markdown | `archive/` | `loaf report archive` after processing |

## Cross-Branch Reconciliation

If a stale branch reintroduces `.agents/{tasks,ideas,sparks,sessions,brainstorms,drafts}/`
or `.agents/TASKS.json`, keep the deletion from the cutover branch and rerun
`loaf check --hook ephemeral-provenance`. Use `loaf state restore-ephemerals
<backup-id>` only for an intentional rollback, followed by a forward re-import.

## Mode-Aware Checks

### Started worktrees

For each row from `loaf issue list --started`:

1. If `(missing)`, flag as **stale started workspace** — the row still records a path that is gone. Offer `loaf issue stop <ref>` after confirmation. Stop does not mark the issue `done`.
2. If the path exists but the issue is `done` / `cancelled` / `duplicate`, flag as **worktree outlived the issue** — same offer.
3. If the path exists and status is `active`, leave it unless the user asks to stop.

Treat these as **warnings**, not auto-fixes.

### Linear overlay

When `integrations.linear.enabled` is `true` in `.agents/loaf.json`, the tracker
adapter is not shipped. If a report or journal entry names a Linear id next to
a Loaf alias, you may `get_issue` and flag an obvious mismatch (Linear Done vs
Loaf still `active`, or the reverse). Warnings only. Do not drive Loaf status
from Linear.

### Leftover board rows

If `loaf housekeeping --dry-run` still reports `tasks` or leftover TASK/INTENT
rows, or `loaf doctor` names leftover-absorb, migrate those rows — do not leave
them as "readable, mint nothing."

1. Preview: `loaf issue absorb --all --dry-run`
2. For done/archived leftover rows: `loaf issue absorb --all --history --dry-run`
3. Persist the previewed command without `--dry-run` after confirmation
4. Change-local `docs/changes/**/tasks/` are never imported

Do not invent a bulk import. Do not create new task or spec records.

## Suggests Next

After housekeeping, suggest reflect if the session produced key decisions or learnings worth integrating into strategic docs.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Report Template | [templates/report.md](templates/report.md) | Creating cleanup reports |
| Linear Integration | `orchestration/references/linear.md` | Checking external tracker overlay |
| Journal Continuity | `orchestration/references/journal.md` | Understanding the project journal model |

## Artifact Naming

Name every artifact you create for what it is, never for the work unit that produced it: the containing directory or the issue already records that provenance. Put the source in a front-matter field (`source: LOAF-42`), not the filename. Versions and timestamps are identity and stay. See the `foundations` skill for the full rule; `loaf check --hook artifact-names` enforces it at commit.
