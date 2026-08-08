---
change: state-dedupe
id: TASK-001
title: Alias-orphan repair migration
blocks:
  - TASK-004
---

# TASK-001 — Alias-orphan repair migration

## Objective

`loaf state migrate alias-orphans` exists with the full preview → backup → manifest → apply → verify → rollback ceremony: it classifies alias-orphaned entity rows across all six entity tables in every project, retires proven duplicates with their reference-table residue, deletes dangling aliases, refuses unproven rows without explicit disposition, and executes named per-row dispositions (the broken-evidence report archives as moot).

## Scope boundaries

**In:** New migration file in `internal/state/` (e.g. `alias_orphan_migration.go`) plus its tests; registration in the `stateMigrateSources` registry (`internal/cli/cli.go:3190` area); a kind-generic retirement sweep derived from `spec_delete.go:90-132`.

**Out:** The importer (TASK-002), doctor (TASK-003), any run against the production database (TASK-004), lifecycle-status semantics (existing migration owns them), list-command queries, schema changes.

## Context pointers

- Contract: `shape.md` — Planning Contract (Approach, Idempotency and safety), Decisions 1/3/5/6
- Template code: `internal/state/lifecycle_status_migration.go` (triad + manifest), `internal/state/spec_delete.go:90-132` (reference-table sweep), `internal/state/markdown_import.go:1120-1127` (`stableMigrationID`, used only for twin proof against historical salts)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — alias-orphan repair migration"
# Read lifecycle_status_migration.go end to end before writing anything;
# the triad, backup call, manifest write, and registry entry are the pattern to follow.
export LOAF_DB="$(mktemp -d)/loaf.sqlite"   # never touch the production DB from tests or smokes
```

## Steps

- [x] Classification: per project, per entity table (tasks, specs, reports, ideas, sparks, brainstorms), find entity rows with no matching `aliases` row; prove twins by recomputing `stableMigrationID(kind, hex(sha256(current_path)), alias)` for the project's aliases, with exact-title match in the event's timestamp cluster as a distinctly-labeled `content-identity` fallback; everything else is `unproven`
- [x] Preview: run classification against a temp copy; report per project and per table — retire/unproven/dangling-alias/orphaned-source counts and the named dispositions
- [x] Apply: mandatory `Backup` first, JSON rollback manifest beside it (every deleted row preserved), retirement sweep per row (bodies, FTS, events, entity_tags, bundle_members, backend_mappings, exports, relationships, then the row) under `PRAGMA defer_foreign_keys = ON`, dangling aliases deleted, unproven rows untouched unless an explicit per-row disposition is supplied via repeatable flags (`--retire <entity-id>`, `--realias <entity-id>=<alias>`) recorded verbatim in the manifest
- [x] Named disposition: the broken-evidence report (`report:7644bb23d2664de93b6cb6a5`) archives as moot — status normalized and an event recording the unrecoverable evidence and SPEC-047 rationale
- [x] Rollback: restore deleted rows from the manifest; verify round-trip in tests
- [x] Tests (`TestAliasOrphan*`): classification correctness on a fixture reproducing the June-24 shape (rekey + re-import), derivation vs content-identity vs unproven labeling, apply/rollback round-trip, idempotency (second preview classifies zero, second apply no-ops), reference-table residue fully removed, unproven rows refused without disposition

## Verification

- `go test ./internal/state -run 'AliasOrphan' -count=1` exits 0
- `go test ./...` exits 0
- Preview against a fixture DB shows per-project classification and touches nothing
