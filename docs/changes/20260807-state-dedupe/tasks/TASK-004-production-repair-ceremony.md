---
change: state-dedupe
id: TASK-004
title: Production repair ceremony
blocked-by:
  - TASK-001
  - TASK-002
  - TASK-003
---

# TASK-004 — Production repair ceremony

## Objective

The production database is repaired and proven: backup taken, preview read for all projects, unproven rows dispositioned explicitly, dedupe applied, doctor parity green, the lifecycle-statuses migration run for the first time, and scanner-vs-list count agreement demonstrated — with receipts journaled.

## Scope boundaries

**In:** Operator ceremony against `~/.local/share/loaf/loaf.sqlite` using the built binary from this branch; journal entries; ceremony receipts for H1–H3 review.

**Out:** Any code changes (if the ceremony reveals a defect, it routes back to TASK-001/002/003); dispositions of lifecycle OOV statuses beyond recording them (set-status verbs are TASK-408 territory).

## Context pointers

- Contract: `shape.md` — Observable Workflow, Decisions 3/4/6, Definition of Done, Open Questions (all three resolve here)
- Recovery discipline: `docs/ARCHITECTURE.md` — Recovery Tiers and Restore Safety

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — production alias-orphan repair ceremony"
npm run build   # ceremony runs the binary built from this branch — no LOAF_DB isolation, deliberately
```

## Steps

- [ ] `loaf state backup` and record the backup ID (Recovery Tier: local rollback)
- [ ] `loaf state migrate alias-orphans` (preview): read per-project classification for all projects; record counts
- [ ] Disposition the unproven rows (expected: the 3 task orphans without title twins) explicitly via `--retire` / `--realias` flags — each recorded in the manifest
- [ ] `loaf state migrate alias-orphans --apply`; record the manifest path
- [ ] `loaf state doctor`: alias-parity section green — raw == reachable for every project and table, zero dangling aliases
- [ ] Confirm the broken-evidence report is archived with its moot-rationale event
- [ ] `loaf state migrate lifecycle-statuses` preview, then `--apply`; record OOV statuses it could not map, if any
- [ ] Demonstrate count agreement: `loaf housekeeping` totals equal list-command counts for all six tables; `loaf task list --status done --json` returns exactly the done rows that exist
- [ ] Journal the ceremony: `decision(state)` with counts and dispositions; `discover(state)` for anything the preview revealed about other projects

## Verification

- Doctor alias-parity green on the production database
- Housekeeping scanner counts equal canonical list counts (the brief's acceptance signal)
- Backup + both manifests retained; receipts sufficient for H1–H3 review
