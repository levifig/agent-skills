---
change: state-dedupe
id: TASK-005
title: Journal-duplicates repair migration
blocks:
  - TASK-004
---

# TASK-005 — Journal-duplicates repair migration

## Objective

`loaf state migrate journal-duplicates` exists as a sibling of alias-orphans on the same preview/backup/manifest/apply/verify/rollback triad: it classifies the ~1,020 journal rows duplicated by the June-24 identity fork (exact `(entry_type, scope, message)` twins across the two import instants), retires the June-13 copies, and leaves the journal timeline single-voiced without touching any legitimately repeated entry.

## Scope boundaries

**In:** New migration in `internal/state/` (e.g. `journal_duplicate_migration.go`) plus tests; registration in `stateMigrateSources`; sweep of journal-adjacent reference surfaces (`journal_search` FTS parity, `journal_origins`, `journal_deferrals`, and any FK reference to `journal_entries.id` — enumerate from the schema before writing).

**Out:** The alias-orphans migration (TASK-001 owns aliased entities), the importer identity fix (landed with the consolidated fix round), any deduplication of rows that repeat legitimately at different times, and any rewrite of `created_at` semantics.

## Plan

The damage signature, verified against production: 1,348 rows created in the June-13 import window (`2026-06-13T01:39`) and 1,156 in the June-24 reimport window (`2026-06-24T13:03`); ~1,020 `(entry_type, scope, message)` triples appear in both windows. Journal rows carry no aliases, so classification is by natural key and window membership, not alias reachability:

1. **Duplicate pair** = identical `(entry_type, scope, message)` where one row's `created_at` falls in the June-13 window and the other's in the June-24 window (named constants shared with the alias-orphans migration's cluster gates). The June-24 row survives — its ID derives from the current project ID, consistent with the canonical-twin rule for aliased entities. Rows matching more than one candidate on either side classify `unproven` and are refused by default (same refuse-by-default posture and `--retire` disposition flag as alias-orphans; `--realias` has no meaning here and is rejected).
2. **Retirement** deletes the June-13 row and its reference residue. Before coding, enumerate every table referencing `journal_entries.id` from the migrations schema; known candidates: `journal_search` (FTS — delete or rebuild via the existing `RepairJournalSearch` parity machinery), `journal_origins`, `journal_deferrals`, `intent_*` tables if they cite entry IDs. The manifest preserves every deleted row and reference edge for rollback.
3. **Ceremony wiring:** TASK-004 runs this migration's preview → apply immediately after alias-orphans and before lifecycle-statuses, in the same backup session.
4. **Acceptance:** post-apply, zero `(entry_type, scope, message)` triples duplicated across the two windows; `loaf journal recent`/`search` return single copies; journal-search parity check green; total row count drops by exactly the retired count; second apply is a no-op.

## Context pointers

- Contract: `shape.md` — Planning Contract, Decisions (journal-dupes expansion)
- Pattern: `internal/state/alias_orphan_migration.go` (post-fix-round state: cluster-gate constants, manifest fsync, verbatim flag recording, fixed-point retirement), `internal/state/lifecycle_status_migration.go` (triad origin), `internal/state/journal_search_integrity.go` and `RepairJournalSearch` (FTS parity)
- Damage evidence: journal `finding(state)` entry of 2026-08-08 (the ~1,020-pair discovery, with window timestamps)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — journal-duplicates repair migration"
export LOAF_DB="$(mktemp -d)/loaf.sqlite"   # tests and smokes never touch the production DB
```

## Steps

- [x] Enumerate every schema reference to `journal_entries.id` and record the sweep list in the migration's doc comment
- [x] Classification: window-gated natural-key pairing with `unproven` refusal for ambiguous matches; preview reports pair counts, unproven counts, and per-window row totals
- [x] Apply: mandatory backup, fsynced JSON rollback manifest, June-13-copy retirement with full reference sweep, `--retire` dispositions recorded verbatim, `--realias` rejected
- [x] Rollback: restore rows and reference edges from the manifest; round-trip test
- [x] FTS: keep `journal_search` consistent (targeted deletes or a post-apply `RepairJournalSearch` rebuild — pick one, justify in the code)
- [x] Tests (`TestJournalDuplicate*`): fixture reproducing the two-window duplication, pairing correctness, ambiguity refusal, apply/rollback round-trip, idempotency, FTS parity after apply
- [x] Wire into TASK-004's ceremony sequence (alias-orphans → journal-duplicates → lifecycle-statuses)

## Verification

- `go test ./internal/state -run 'JournalDuplicate' -count=1` exits 0
- `go test ./...` exits 0
- Preview against a copy of the production database reports ~1,020 retire pairs and touches nothing
