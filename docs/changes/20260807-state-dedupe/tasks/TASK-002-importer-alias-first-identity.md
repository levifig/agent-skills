---
change: state-dedupe
id: TASK-002
title: Importer alias-first identity resolution
blocks:
  - TASK-004
---

# TASK-002 — Importer alias-first identity resolution

## Objective

The markdown importer resolves entity identity through the aliases table before deriving an ID: when `(project_id, namespace, alias)` already names an entity of the imported kind, the importer reuses that entity's ID so `ON CONFLICT(id) DO UPDATE` fires, making alias re-pointing and orphan creation impossible regardless of project-ID changes.

## Scope boundaries

**In:** `internal/state/markdown_import.go` upsert paths for specs, tasks, reports, ideas, sparks, brainstorms, and sources where they key off derived IDs; regression tests.

**Out:** The repair migration (TASK-001), doctor (TASK-003), `rekeyLegacyProjectTx` (re-derivation is Cut by the contract), alias derivation rules (`firstNonEmpty(frontmatter id, path alias, stem)` stay as they are — a renamed file is a new artifact by design).

## Context pointers

- Contract: `shape.md` — Planning Contract (Approach), Decision 2, Cut list
- Root cause mechanics: `internal/state/markdown_import.go:218-261` (importSpecs), `:724-734` (task upsert `ON CONFLICT(id)`), `:967-974` (alias upsert that re-points `entity_id`), `:1120-1127` (`stableMigrationID`)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-002 — importer alias-first identity resolution"
export LOAF_DB="$(mktemp -d)/loaf.sqlite"
```

## Steps

- [x] Add an alias-first lookup on the import path: resolve `(project_id, namespace, alias)` to an existing entity ID of the imported kind before falling back to `stableMigrationID` for genuinely new entities
- [x] Ensure the alias upsert can no longer re-point an alias away from a live entity row as a side effect of import (with alias-first resolution the `entity_id` it writes is the resolved one; assert this in tests rather than trusting it)
- [x] Apply the same resolution to `sources` rows so source doubling cannot recur
- [x] Regression test (`TestImportAliasFirst*`): import a markdown tree under one project ID, rewrite `project_id` columns exactly as `rekeyLegacyProjectTx` does, re-import — assert zero new entity rows, zero new source rows, zero alias-orphans, and stable alias→entity mappings
- [x] Idempotency test: re-import with no changes is byte-stable (no row churn, `updated_at` semantics preserved as today)

## Verification

- `go test ./internal/state -run 'ImportAliasFirst' -count=1` exits 0
- `go test ./...` exits 0
