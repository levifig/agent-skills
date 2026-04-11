---
id: TASK-102
spec: SPEC-029
title: Lifecycle integration (wrap + housekeeping)
priority: P2
status: done
---

# TASK-102: Lifecycle integration (wrap + housekeeping)

## Objective

Integrate `loaf session enrich` into the wrap and housekeeping lifecycle points so enrichment happens automatically at natural session boundaries.

## Acceptance Criteria

- [ ] Wrap skill calls `loaf session enrich` before generating wrap-up report
- [ ] Housekeeping skill calls `loaf session enrich <file>` on sessions with status `stopped` or `done` that have a `claude_session_id`
- [ ] Housekeeping does NOT enrich `active` sessions (those are handled by wrap)
- [ ] Housekeeping cleans up `.agents/tmp/<session-id>-enrichment.txt` when archiving sessions
- [ ] Both handle enrich failures gracefully (warn to stderr, don't block primary workflow)
- [ ] Manual `loaf session enrich` still works independently

## Implementation Notes

- **Wrap skill** (`content/skills/wrap/SKILL.md`): Add step before wrap-up: "Run `loaf session enrich` to fill in missing journal entries from the conversation log."
- **Housekeeping skill** (`content/skills/housekeeping/SKILL.md`): Add enrichment pass for stopped/done sessions + temp file cleanup on archival.
- Both treat enrich failure as non-fatal — log a warning and continue
- No hook changes — purely skill instruction updates + housekeeping cleanup addition

## Dependencies

- TASK-101 (enrich command must exist first)
