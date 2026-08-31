---
name: housekeeping
description: >-
  Reviews and maintains Loaf continuity artifacts, reports, handoffs, and Git
  workspaces while treating the configured tracker as canonical shared work. Use
  when the user asks "housekeeping," "clean up," or "tidy up .agents/." Provides
  hygiene recommendations, archives completed work, and ensures extracted
  knowledge is preserved. Not for strategic reflection (use reflect) or
  knowledge management (use knowledge-base).
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

Systematic review of private continuity and repository hygiene without creating a second work system.

## Critical Rules

**Always**
- Log invocation as the first action: `loaf journal log "skill(housekeeping): <scope or trigger>"`
- Review EVERY file individually — never sample or average
- Read shared work status from the canonical tracker through the selected `project-management/v1` provider skill and harness-native connection
- Extract lessons learned and decisions before archiving
- Use deterministic Loaf commands for Loaf-owned artifacts and Git-native commands for worktrees; never raw `mv`
- Treat `.agents/handoffs/` as first-class but disposable: keep active/final handoffs, delete only after confirmed deprecated status
- Check report `status` is `done` (or `final`) before archiving reports (see [templates/report.md](templates/report.md))
- Preserve the project journal, sparks, wraps, and useful handoffs; the journal is append-only and never a cleanup target
- When delegated subagents are available, use the `librarian` profile for
  `.agents/`-scoped durable artifact tending: report/handoff hygiene,
  staleness notes, and lifecycle-safe cleanup recommendations.
  Housekeeping still owns user confirmation and final archive decisions.
- Log outcome to the project journal: `loaf journal log "decision(housekeeping): archived N reports; stopped M stale worktrees"`

**Never**
- Auto-archive without user confirmation for each artifact
- Skip spark extraction before deleting brainstorm drafts
- Leave `archived_at` or `archived_by` fields empty in archived files
- Infer tracker state from local artifacts or Git state
- Dispatch cleanup agents into a live started worktree another agent occupies

## Verification

After work completes, verify:
- Reports archived via `loaf report archive` after processing
- Provider readback confirmed every tracker state used to justify cleanup
- Git worktrees were checked for branch, dirtiness, reachability, and occupancy before any removal was proposed
- Drafts checked for unprocessed sparks before deletion
- Handoffs deleted only after explicit deprecation is confirmed
- Summary table presented showing all actions taken

## Quick Reference

### CLI Commands

```bash
loaf housekeeping --dry-run          # Preview recommendations
loaf housekeeping                    # Run artifact scanner
git worktree list --porcelain        # Inspect repository-native workspaces
loaf report archive <report>         # Archive a processed report
```

Historical local issue/task/spec rows are frozen migration inputs, not current
work. Normal housekeeping never promotes, reconciles, or synchronizes them.

The project journal is append-only and never archived — it is not a housekeeping
target. It is the canonical record housekeeping reads when extracting decisions
before archiving other artifacts.

### Artifact Lifecycle

| Artifact | Active Location | Archive | Action |
|----------|-----------------|---------|--------|
| Shared work | Canonical native tracker | Provider-native archive/close semantics | Confirm, mutate through the selected provider, then read back |
| Git worktrees | Git repository | Repository-native removal | Inspect branch and dirtiness independently of tracker status |
| Drafts / brainstorms | SQLite state | SQLite resolved/archived status | User decision (spark extraction first) |
| Handoffs | `.agents/handoffs/` | delete | Delete after status is confirmed `deprecated` |
| Reports | SQLite state + generated/authored report Markdown | `archive/` | `loaf report archive` after processing |

## Mode-Aware Checks

Read the configured provider manifest before using optional fields. Never emulate
unsupported hierarchy, status, archive, or assignment behavior. Historical
local rows are reported only as migration candidates for a separate, explicit,
one-time migration; they are never compared with the tracker during routine
housekeeping.

## Suggests Next

After housekeeping, suggest reflect if the session produced key decisions or learnings worth integrating into strategic docs.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Report Template | [templates/report.md](templates/report.md) | Creating cleanup reports |
| Tracker Workflows | [../project-management/SKILL.md](../project-management/SKILL.md) | Selecting a provider and reading canonical work |
| Wrap Continuity | [../wrap/SKILL.md](../wrap/SKILL.md) | Preserving connective narrative before cleanup |

## Artifact Naming

Name every artifact you create for what it is, never for the work unit that produced it: the containing directory or the issue already records that provenance. Put the source in a front-matter field (`source: LOAF-42`), not the filename. Versions and timestamps are identity and stay. See the `foundations` skill for the full rule; `loaf check --hook artifact-names` enforces it at commit.
