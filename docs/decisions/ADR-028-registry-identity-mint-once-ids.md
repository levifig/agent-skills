# ADR-028: Entity identity lives in the alias registry; derived IDs are mint-once

Decision Date: 2026-08-09
Status: Accepted

## Context

Entity primary keys in the global SQLite database were derived —
`sha256(kind, project_id, alias)` — and the markdown importer resolved
identity by recomputing that derivation. Both halves assumed every input
to the hash was immutable. `project_id` was not: migration 3 rekeyed
projects from legacy path-hash IDs to opaque `proj_` IDs, rewriting the
`project_id` column but necessarily leaving the derived `id` columns
untouched. The next markdown import (2026-06-24) recomputed every ID
under the new salt, found no match, inserted a full second copy of every
artifact, and re-pointed each alias to the new twin via
`ON CONFLICT(project_id, namespace, alias) DO UPDATE SET entity_id`.

The June-13 originals became alias-orphans: invisible to every list
surface (all list queries INNER JOIN through `aliases`) yet counted by
the housekeeping scanner (raw `WHERE project_id = ?`). The same fork
duplicated ~1,020 journal entries, which carry no aliases at all. The
damage was invisible for six weeks and was discovered by accident during
a housekeeping pass (journal `finding(state)`, 2026-08-07).

The entity tables had no unique constraint other than the derived
primary key. The only content-meaningful uniqueness in the schema was
`aliases UNIQUE (project_id, namespace, alias)` — the registry was
already the de-facto identity authority; the importer just didn't
consult it.

## Decision

1. **Derived entity IDs are mint-once opaque keys.** An ID is computed
   at first creation and never recomputed for resolution. No code path
   may locate an existing row by re-deriving its ID; the one exception
   is the alias-orphans repair migration's twin proof, which recomputes
   against *historical* salts precisely to identify rows damaged before
   this decision.
2. **Identity resolution goes through the registry.** Importers and any
   other writer that might encounter an existing entity resolve
   `(project_id, namespace, alias)` against the aliases table first and
   reuse the registered entity's ID. Unaliased kinds resolve by natural
   key (journal entries: entry type, scope, message for markdown-origin
   rows; sources: project and path).
3. **Divergence is detected, not assumed away.** `loaf state doctor`
   carries an alias-parity diagnostic: for every project and aliased
   entity table, raw row counts must equal alias-reachable counts, with
   zero dead aliases. Damage reports as an error diagnostic naming the
   repair; it does not invalidate the database.

## Alternatives rejected

- **Re-deriving entity IDs after a rekey** (rewriting `id` columns
  across every table and reference in one transaction). Rejected: large
  blast radius insuring against a scenario the registry-first importer
  already neutralizes. Recorded as permanently foreclosed in the
  state-dedupe Change's Cut list.
- **Schema-level content uniqueness on entity tables.** Rejected: the
  aliases registry is the identity authority; a second constraint
  surface would drift from it.

## Consequences

- A project rekey, storage-home merge, or import replay can no longer
  fork the identity space; at worst it mints alias-safe duplicates in
  one documented spark corner case (verbatim-duplicated lines under
  rekey re-import), which stays alias-reachable and repairable.
- Repair of pre-decision damage ships as audited migrations
  (`alias-orphans`, `journal-duplicates`) on the repair-migration
  pattern (see ARCHITECTURE.md, Repair Migrations).
- Executed 2026-08-09 against the production database: 191 alias
  orphans and 1,190 duplicated journal rows retired; scanner and list
  surfaces agree exactly (233/26/11); doctor parity clear across 27
  projects.

## Evidence

- PR #159 (squash `34761008`) and PR #158 (the captured brief)
- `docs/changes/20260807-state-dedupe/` — shape.md Decisions 2, 5, 8;
  receipts/ceremony.md
- Journal: `decision(identity)` 2026-08-08, `decision(state)` and
  `wrap(state-dedupe)` 2026-08-09
