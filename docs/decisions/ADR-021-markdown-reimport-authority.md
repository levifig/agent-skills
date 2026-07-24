---
id: ADR-021
title: "Markdown re-import authority — fingerprint reclaim, insert-only status, snapshot simulation"
status: Accepted
date: 2026-07-24
supersedes: null
superseded_by: null
---

# ADR-021: Markdown re-import authority

## Context

`loaf migrate markdown` is the one-way bridge that imports legacy `.agents`
Markdown into the global SQLite database (ADR-016 / ADR-017). Two independent
failure modes surfaced on re-import: dry-run and apply disagreed because preview
only counted files while apply ran a different pipeline and aborted on the first
non-`migration` journal origin; and import treated Markdown status as authority,
silently overwriting real SQLite lifecycle state (including archived rows).

Schema migration 0011 backfilled every pre-existing origin with mechanism
`unknown`, so any project that imported Markdown before 0011 could preview a
large importable set and then refuse wholesale on apply. Status clobber had no
archived guard and no event trail. The public `state.Backup` API cannot host
dry-run simulation: it Inspect-gates behind-schema databases into a generic
doctor error, rejects OS-temp destinations, owns its filename format, and
completes reservation before verification.

## Decision

Markdown import authority is permanently defined by three rules:

1. **Fingerprint-bounded origin reclaim (information-free-above-journal-row).**
   An existing `journal_origins` row is rewritten as `migration` only when it
   matches the full 0011-compatible fingerprint evaluated against both the
   origin row and its paired journal row: mechanism `unknown`, envelope version
   1, NULL in every field 0011 wrote NULL, and null-safe equality for every
   field 0011 copied from the journal row. The fingerprint does not prove 0011
   authored the row; it proves the row carries no information beyond the journal
   row the import already owns by deterministic ID. Every other non-`migration`
   origin is skipped untouched (entry, origin, and line-derived sparks/aliases/
   relationships), counted, and listed. Apply never aborts on an origin
   collision.

2. **Unknown-is-never-authoritative status disposition.** Stored status
   `unknown` records the absence of lifecycle information — the importer is the
   only writer of `unknown`, and it sits outside both canonical and legacy
   vocabularies on the status-mutation surface. A real normalized incoming
   status may fill a stored `unknown`; any other stored status is insert-only
   (never overwritten). Kept-vs-incoming divergences are reported. Out-of-
   vocabulary input never fills a stored `unknown`. Archived cannot flip back
   because `archived ≠ unknown`. SQLite remains the source of truth once a real
   status exists; import is not sync.

3. **Full-pipeline snapshot simulation via an extracted `VACUUM INTO` primitive.**
   When the database exists and the project is registered, `--dry-run` snapshots
   with a lower-level primitive extracted from backup machinery (caller-owned
   destination and cleanup, hard integrity/FK gate, loaf-owned temp hygiene) and
   runs the identical apply pipeline against the snapshot. Results carry
   `mode: simulation` and an in-transaction `import_report`. When no database or
   registered project exists, dry-run is an honestly labeled file inventory
   (`mode: inventory`, no `import_report`); parity is inapplicable. The public
   `state.Backup` wrapper is not the vehicle. Dry-run/apply report parity holds
   only when the database and `.agents` tree are unchanged between the two
   commands — loaf takes no cross-command lock.

## Rationale

- Reclaiming only information-free origins destroys no provenance evidence while
  unblocking the 0011-backfill false-refusal case.
- Insert-only status (with `unknown` as the sole fillable placeholder) removes
  the silent archived-flip class without inventing merge or timestamp rules.
- Sharing one apply pipeline between simulate and apply eliminates the
  preview/apply asymmetry by construction; inventory mode stays honest when
  there is nothing to collide with.

## Alternatives Considered

- **`--skip-conflicts` flag.** Rejected: deterministic reclaim/skip makes apply
  total with respect to origin collisions; there is no refusal left to skip.
- **Reclaim any `unknown` origin.** Rejected: evidence-bearing or
  copy-mismatched rows must stay untouched.
- **Wrap public `state.Backup` for dry-run.** Rejected: Inspect gate, volatile-
  destination rejection, and reservation-before-verification make it unfit for
  interactive simulation of behind-schema or temp destinations.
- **Timestamp- or merge-based status reconciliation.** Rejected: import stays
  one-way; Markdown is not live status authority after first real status.

## Consequences

- `loaf migrate markdown --dry-run --json` reports `mode` (`simulation` or
  `inventory`); `import_report` is present exactly when `mode` is `simulation`.
- Apply and resume remain aliases for the committed import path; both emit
  `import_report` from the same transaction that writes.
- Doctor's unimported-Markdown diagnostic continues to call the cheap inventory
  function and must not trigger simulation.
- Follow-ups deferred: doctor detection of historical archived-flip damage,
  orphaned-derivative GC, and crash-orphaned snapshot residue sweeps.

## Related

- [Change: markdown-reimport-safety](../changes/20260723-markdown-reimport-safety/change.md)
- [ADR-016](ADR-016-artifact-storage-trichotomy.md) — nouns in SQLite, Markdown is a render
- [ADR-017](ADR-017-ephemeral-agent-markdown-cutover.md) — ephemeral Markdown as migration source
