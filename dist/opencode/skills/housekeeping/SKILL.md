---
name: housekeeping
description: >-
  Reviews and maintains agent artifacts in .agents/ — sessions, specs, plans,
  drafts, councils, and reports. Use when the user asks "housekeeping," "clean
  up," "review sessions," or "tidy up .agents/." Provides hygiene
  recommendations, archives completed work, and ensures extracted knowledge is
  preserved. Not for strategic reflection (use reflect) or knowledge management
  (use knowledge-base).
subtask: false
version: 2.0.0-dev.11
---

# Housekeeping

Systematic review and archival of all `.agents/` artifacts with Linear-aware checks.

## Critical Rules

**Always**
- Review EVERY file individually — never sample or average
- Check Linear issue status before archiving sessions
- Extract lessons learned and decisions before archiving
- Use CLI (`loaf housekeeping`, `loaf task archive`, `loaf spec archive`) — never raw `mv`
- Verify `TASKS.json` sync after archival with `loaf task sync`

**Never**
- Auto-archive without user confirmation for each artifact
- Delete plans — they are ephemeral and should be deleted, not archived
- Skip spark extraction before deleting brainstorm drafts
- Leave `archived_at` or `archived_by` fields empty in archived files

## Verification

After work completes, verify:
- Session files archived with proper metadata (status, archived_at, archived_by)
- Tasks archived via `loaf task archive` (updates TASKS.json)
- Specs archived via `loaf spec archive` (updates TASKS.json)
- Drafts checked for unprocessed sparks before deletion
- Plans deleted (not archived) when linked sessions are archived
- Summary table presented showing all actions taken

## Quick Reference

### CLI Commands

```bash
loaf housekeeping --dry-run        # Preview all actions
loaf housekeeping --sessions       # Sessions only
loaf housekeeping --specs          # Specs only
loaf task archive TASK-XXX    # Archive single task
loaf spec archive SPEC-XXX    # Archive single spec
loaf task sync                # Fix TASKS.json drift
```

### Artifact Lifecycle

| Artifact | Active Location | Archive | Action |
|----------|-----------------|---------|--------|
| Sessions | `.agents/sessions/` | `archive/` | Move with metadata |
| Tasks | `.agents/tasks/` | `archive/` | `loaf task archive` |
| Specs | `.agents/specs/` | `archive/` | `loaf spec archive` |
| Plans | `.agents/plans/` | N/A | Delete when done |
| Drafts | `.agents/drafts/` | `archive/` | User decision |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Report Template | [templates/report.md](templates/report.md) | Creating cleanup reports |
| Linear Integration | `orchestration/references/linear.md` | Checking external issue status |
| Session Management | `orchestration/references/sessions.md` | Understanding session lifecycle |
