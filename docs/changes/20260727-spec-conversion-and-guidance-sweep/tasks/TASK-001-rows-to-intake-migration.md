---
change: spec-conversion-and-guidance-sweep
id: TASK-001
title: Rows-to-intake migration
blocks:
  - TASK-002
---

# TASK-001 — Rows-to-intake migration

## Objective

`loaf migrate work-records` exists and is the legacy tables' sanctioned exit: open/draft specs (each carrying its open tasks) and orphan open tasks become Intents preserving full text and provenance; completed/archived rows stay untouched; a nudge gates projects holding open legacy rows until they run it.

## Scope boundaries

**In:** New migrate source under the existing `loaf migrate` umbrella; Intent creation with provenance (origin spec/task ids, timestamps); idempotency by operation key; the open-rows nudge (ADR-013 worktree-storage pattern — exempting `migrate`, `help`, `--version`); round-trip tests (`TestWorkRecordsMigration` in `internal/state`).

**Out:** Removing any legacy command or reader (TASK-002/003). No markdown ingestion (Decision 4). No row deletion, and no mutation of source rows — idempotency lives in the operation key, never in a converted-marker column (a status field in disguise).

## Context pointers

- Contract: `shape.md` — Decisions 3–4, Planning Contract "Migration semantics"
- Precedent: idempotent deferred-intent capture (journal-reliability-foundation); nudge gate in ADR-013

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — rows-to-intake migration"
```

## Steps

- [ ] Converter: open/draft spec → one Intent (spec body verbatim + open tasks as structured content); orphan open task → own Intent; provenance recorded
- [ ] Idempotency: operation-keyed — re-run creates nothing new
- [ ] Nudge: triggers only on open/draft rows; completed-only projects quarantine silently; wording, exempt commands, exit code fixed here
- [ ] `TestWorkRecordsMigration` round-trip: source rows → Intents → content and provenance verified faithful

## Verification

- `go test ./internal/state -run TestWorkRecordsMigration -v` green
- Re-run on a migrated fixture creates zero new Intents
