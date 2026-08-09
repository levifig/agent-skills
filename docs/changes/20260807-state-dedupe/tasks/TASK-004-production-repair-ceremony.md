---
change: state-dedupe
id: TASK-004
title: Production repair ceremony
blocked-by:
  - TASK-001
  - TASK-002
  - TASK-003
  - TASK-005
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
- [ ] Disposition the unproven rows explicitly — expected: 23 rows (rehearsed on a production copy during review). 3 task orphans without title twins (`--retire` or `--realias` per row), and 20 sparks forming 10 both-orphan message pairs: one copy per import instant, neither holding an alias, no surviving twin to bind to. Each pair takes one `--realias` (the member that lives on) and one `--retire`; which member survives is ceremony judgment — the June-24-survives convention from entity twins is the sensible default
- [ ] Rehearse the exact apply invocation as a preview first: `loaf state migrate alias-orphans --retire … --realias …` (dispositions are accepted in preview and reflected in its totals) — the rehearsed and applied invocations must be identical
- [ ] `loaf state migrate alias-orphans --apply --retire … --realias …`; record the manifest path; first run must exit 0 with post-apply verification passing and a truthful non-zero `orphaned_sources` figure
- [ ] `loaf state migrate journal-duplicates` (preview): read pair counts and ambiguous matches; disposition ambiguities via `--retire`; then `--apply`; record the manifest path
- [ ] `loaf state doctor`: alias-parity section green — raw == reachable for every project and table, zero dead aliases
- [ ] Confirm the broken-evidence report is archived with its moot-rationale event
- [ ] `loaf state migrate lifecycle-statuses` preview, then `--apply`; record OOV statuses it could not map, if any
- [ ] Demonstrate count agreement: `loaf housekeeping` totals equal list-command counts for all seven aliased tables; `loaf task list --status done --json` returns exactly the done rows that exist
- [ ] Journal the ceremony: `decision(state)` with counts and dispositions; `discover(state)` for anything the preview revealed about other projects

## Verification

- Doctor alias-parity green on the production database
- Housekeeping scanner counts equal canonical list counts (the brief's acceptance signal)
- Backup + both manifests retained; receipts sufficient for H1–H3 review
