---
change: markdown-reimport-safety
created: 2026-07-23
branch: markdown-reimport-safety
---

# Markdown Re-import Safety

## Problem

Re-importing `.agents` Markdown into SQLite (`loaf migrate markdown`) is broken in two independent ways, both surfaced by the gridsight-core-gds migration on 2026-07-22 (journal findings `journal:453d67cf2bd7bf131a1b5f09`, `journal:8038077b8a7e19e3abe77533`).

First, the dry-run and apply paths disagree because they share no code: `PreviewMarkdownMigration` only counts files and never opens the database, while apply runs the whole import in one transaction and hard-fails on the first journal entry whose `journal_origins` row carries a non-`migration` `capture_mechanism` (`markdown_import.go:740`). Schema migration 0011 backfilled every pre-existing entry with mechanism `unknown`, so any project that imported Markdown before 0011 now fails wholesale on re-import — gridsight previewed 108 importable sessions, then apply refused everything (2,841 `unknown`-origin rows).

Second, the import treats source status as authority: `upsertSpec`/`upsertTask`/`upsertSimpleEntity`/`upsertReport` all overwrite `status` unconditionally on conflict, with no archived guard and no event trail. Gridsight's SPEC-007/008 were archived in SQLite on 2026-07-08 (events intact) and silently flipped back on the 2026-07-22 import, and out-of-vocabulary spellings like `accepted` landed raw in the database. PR #130 closed the archive-gate vocabulary and normalizes spec status at import, but the clobber itself, the missing normalization for other entity kinds, and the preview blindness all remain.

## Hypothesis

If import decisions become deterministic functions of database state (reclaim exactly the origin rows that carry no information beyond their paired journal row, skip all other foreign provenance, never overwrite a real lifecycle status) and the dry-run executes the identical apply pipeline against a verified disposable snapshot of the database, then re-import becomes safe and business-state idempotent, and dry-run and apply cannot disagree whenever both run against the same database and `.agents` state. Where no database or registered project exists, the dry-run is an honestly labeled file inventory and parity is inapplicable — there is no apply outcome to disagree with until state exists.

## Scope

**In**

- Origin-collision semantics in the importer: an origin row is reclaimed as `migration` only when it matches the full 0011-compatible fingerprint — `capture_mechanism = 'unknown'`, `envelope_version = 1`, NULL in every field 0011 wrote NULL (`observed_harness`, `observed_harness_version`, `agent_id`, `source_event`, `head`, `change_path`, `change_sha256`, `dirty`, `reconstructable`, `durable_result_kind`, `durable_result_id`), and null-safe equality with the paired journal row for every field 0011 copied (`harness_session_id`, `branch`↔`observed_branch`, `worktree`↔`observed_worktree`, `created_at`). Every other non-`migration` origin — `manual`, `skill`, `hook`, custom mechanisms, future envelope versions, and any `unknown` row carrying evidence or mismatched copies — causes the entry to be skipped untouched, counted, and listed. Apply never aborts on an origin collision.
- Status authority: stored `unknown` is never authoritative — the importer is the only writer of `unknown` (status mutations are vocabulary-gated in `spec_status.go` and `unknown` is outside both canonical and legacy vocabularies), so it definitionally records the absence of lifecycle information. A real normalized incoming status fills a stored `unknown`; any other stored status is never overwritten, and kept-vs-incoming divergences are reported. Archived can never flip back because `archived ≠ unknown`.
- Lifecycle vocabulary normalization at insert for tasks, reports, ideas, and brainstorms via `CanonicalLifecycleStatus` (specs landed in PR #130), governed by the normative per-kind status table in the Planning Contract; explicit out-of-vocabulary values are stored raw on first insert and surfaced as warnings; absent and explicitly-`unknown` source status are no-opinion — no warning, no divergence.
- Dry-run as snapshot simulation: when the database file exists and the project resolves to a registered row (read-only lookup), `--dry-run` snapshots the database with a lower-level snapshot primitive extracted from the backup machinery (`VACUUM INTO` with a caller-owned destination and cleanup lifecycle — the public `state.Backup` API cannot serve here: it Inspect-gates behind-schema databases into a generic doctor error, rejects OS-temp destinations as volatile, owns its filename format, and completes its reservation before verification), then runs the complete apply pipeline — `Initialize` preflight (`RequireCurrentSchema`, registration refresh) through the FTS rebuild — against the snapshot, reports, and removes the snapshot. Under a quiescent fixture the live main database and `-wal` bytes are unchanged; `-shm` is ephemeral SQLite coordination state and excluded, matching the precedent in `backup_test.go:477`. When no database or registered project exists, the cheap file-inventory preview runs, explicitly labeled inventory-only.
- A new `import_report` object (reclaimed-origin count, skipped-entry list with mechanisms, status-divergence list, and status/provenance outcome warnings) computed inside the importer's own transaction and carried on both simulate and apply results; inventory I/O and parser warnings stay in the existing plan `warnings` field with its meaning unchanged. Dry-run results carry a `mode` discriminant (`simulation` or `inventory`); `import_report` is present exactly when `mode` is `simulation`.
- Apply no longer calls preview: `ApplyMarkdownMigration` computes inventory before opening the import transaction, then produces `import_report` from the committed import, eliminating the current preview-then-import TOCTOU window (`markdown_import.go:70`). Post-commit `.agents` access failures degrade to plan warnings and never fail an already-committed import.
- Spark target-refresh semantics: spark-derived imported relationships are delete-then-reinserted per spark (parity with `deleteImportedRelationships` on artifacts), so a changed promoted-to target no longer strands the old edge while the line remains a spark.
- Inventory counter fixes: the cheap spark counter learns `sessions/archive/*.md` (parity with apply's file set), and counter read failures surface as plan warnings instead of being swallowed.
- Snapshot hygiene: snapshots live in a loaf-owned `0700` temp subdirectory with `0600` files named with pid and timestamp, cleanup is registered before creation so creation failure, cancellation, or ENOSPC leaves no partial file, and removal covers the snapshot's `-wal`/`-shm` siblings; deletion is best-effort on process crash, with the residue pattern named so a future sweep can collect strays.
- Regression coverage shaped like gridsight: 0011-fingerprint backfill rows, evidence-bearing and copy-mismatched `unknown` rows, an archived spec whose Markdown still says a terminal legacy status, foreign-mechanism and future-envelope entries, and a second-apply business-state idempotence check over the full import-touched table set.

**Out** (deferred, not rejected)

- A doctor check that detects historical archived-flip damage in existing databases, and a residue sweep for crash-orphaned snapshots — follow-up candidates once these semantics land.
- Repairing gridsight's data in code: SPEC-007/008 re-archive is an operational step (`loaf spec archive` accepts them post-#130) noted for the gridsight session, not shipped here.
- An alias provenance model: import-created alias rows remain import-refreshable, and hand-curated aliases are not a supported surface today; protecting them needs provenance on `aliases`, which is its own Change.
- Reconciling removed or type-changed session lines: a line that stops being a `spark` (edited to another type, or deleted) leaves its previously derived spark, alias, and relationships in place — orphaned-derivative garbage collection is legacy-content hygiene for the doctor follow-up, not import logic.
- Any markdown-export or rollback-manifest changes beyond carrying the new report fields.

**Cut** (explicitly rejected)

- A `--skip-conflicts` flag: the deterministic reclaim/skip semantics make apply total with respect to origin collisions, so there is no refusal left to skip and no flag to carry forever. (Apply can still fail for real reasons — schema preflight, FTS rebuild, I/O — and those failures are identical in simulation.)
- Reclaiming any origin outside the full 0011-compatible fingerprint — the fingerprint cannot prove 0011 authored the row, and does not try to; it proves the row is information-free above its paired journal row, which is the actual safety requirement. Anything with evidence or mismatched copies is skipped, never rewritten.
- Timestamp- or merge-based status reconciliation between Markdown and SQLite — SQLite is the source of truth once a real status exists; import is a one-way legacy migration, not sync.
- Byte-identical idempotence and `-shm` byte guarantees: `updated_at`/`imported_at` refresh on re-import by design, and the WAL index is mutable coordination state; idempotence is defined over logical business state.

## Observable Workflow

On a pre-0011 project, `loaf migrate markdown --dry-run` reports `mode: simulation`, the file inventory, and an import report: `reclaimed origins: N`, `skipped (foreign provenance): N` with mechanisms, `status divergences: N` naming each kept SQLite status, and the registered project identity — computed by really running the import against a verified throwaway snapshot. `loaf migrate markdown --apply` then reports the same numbers from its committed transaction, provided the database and `.agents` did not change between the two commands (loaf takes no cross-command lock; the parity precondition is stated, not assumed). The gridsight scenario imports its 108 sessions instead of refusing. Archived specs stay archived through any number of re-imports. A second `--apply` reports `reclaimed origins: 0`, the same skipped list, and unchanged business state across every import-touched table. On a fresh machine with no database, `--dry-run` reports `mode: inventory` with an explicit note that conflict detection needs an initialized project, and no `import_report`. `--json` carries the same fields; all pre-existing fields keep their meanings.

## Rabbit Holes and No-Gos

- Two-way Markdown↔SQLite sync: divergence reporting will tempt "why not write the DB status back to frontmatter" — import stays one-way, and export is a different surface.
- Redesigning the journal provenance model: this Change consumes `capture_mechanism` as-is; vocabulary or envelope changes belong to the origin-hygiene lineage (PR #127), not here.
- In-code repair of historical damage: detecting and fixing rows already flipped in other projects' databases is the deferred doctor follow-up, not import logic.
- Reworking `--resume`: with business-state idempotent apply it is behaviorally identical to `--apply`; it stays an accepted alias and its removal is somebody else's cleanup.
- Growing the extracted snapshot primitive into a second Backup: no reservations, recovery tiers, destination policy, or watermark machinery — its whole contract is `VACUUM INTO` a caller-owned path, a hard integrity/FK gate, and caller-owned cleanup.

## Decisions

Provenance: decisions 1–3 were accepted by interview on 2026-07-23 and hardened across two adversarial review rounds (Codex, gpt-5.6 xhigh, 2026-07-23/24 — see Source Inputs); 4–9 were made autonomously during shaping and review disposition.

1. **Reclaim requires the full 0011-compatible fingerprint, evaluated against both the origin row and its paired journal row.** The decision helper receives both rows and requires: mechanism `unknown`, envelope version 1, NULL in all eleven fields 0011 wrote NULL, and null-safe equality for the four values 0011 copied from the journal row. This does not prove 0011 authored the row — nothing can — and the claim is deliberately weaker: a matching row is information-free above the journal row the import already owns by deterministic ID, so rewriting it as `migration` destroys no evidence that is not being deliberately refreshed from the source file. Rows failing any predicate — including `dirty = 1`, populated `observed_harness_version`, or a branch value differing from the journal row — are skipped whole. Hardened per review round 2 finding 1; forecloses `--skip-conflicts` and wholesale-abort behavior.
2. **Stored `unknown` is never authoritative; everything else is insert-only.** The importer is the only writer of status `unknown` — the status-mutation surface is vocabulary-gated (`spec_status.go:32`) and `unknown` is in neither the canonical nor the legacy vocabulary — so a stored `unknown` definitionally records "no lifecycle information", and filling it with a real normalized status is strict information gain on any import. Every other stored status is never overwritten; divergences are reported with both sides. This replaces the structural-placeholder rule (`body_source_id IS NULL` is demonstrably not placeholder-exclusive — review round 2 finding 2, `spec_lifecycle_test.go:535`) and needs no placeholder concept at all: `ensureSpecPlaceholder` rows are covered because they are created at `unknown`. Forecloses status_changed events for import and any frontmatter-wins semantics.
3. **Dry-run runs the real apply pipeline against a snapshot taken by an extracted lower-level primitive.** Simulation is `ApplyMarkdownMigration` pointed at a copy: a snapshot function extracted from the backup machinery runs `VACUUM INTO` to a caller-owned destination with cleanup registered before creation, then hard-fails on snapshot `integrity_check`/`foreign_key_check` failures; journal-search and provenance readiness are informational only, because the apply pipeline's own `rebuildAndVerifyJournalSearch` is the repair path for derived drift and pre-gating on it would block exactly the databases that need repairing (`TestVerifyBackupReportsDivergentJournalSearchWithoutStructuralFailure` shows the backup layer already treats parity as non-structural). The public `state.Backup` API is explicitly not the vehicle: it Inspect-gates `ModeInvalid` (which includes behind-schema) into a generic doctor error (`backup.go:139`), rejects volatile OS-temp destinations (`backup.go:377`), and completes its reservation before verification (`backup.go:214`). Schema preflight runs only inside the shared apply pipeline on the snapshot — simulation performs no Inspect gate of its own — so a behind-schema database produces the same typed `schema-upgrade-required` error from `RequireCurrentSchema` in both modes. Registration and schema writes land on the disposable copy. Live-file preservation is asserted over main database and WAL bytes under a quiescent fixture; `-shm` is excluded as ephemeral coordination state (`backup_test.go:477` precedent). Snapshot-creation failure is its own clearly reported error class, not claimed identical to any apply failure. Hardened per review rounds 2 and 3.
4. **Idempotence means equivalent business state over the canonical business tables, plus parity of the derived indexes.** After a second apply, the logical dump of the thirteen canonical tables — `specs`, `tasks`, `ideas`, `brainstorms`, `shaping_drafts`, `reports`, `sparks`, `journal_entries`, `journal_origins`, `aliases`, `relationships`, `sources`, `artifact_bodies` — all columns except `created_at`/`updated_at`/`imported_at` — is identical, the report shows `reclaimed origins: 0` with a stable skipped list, and the two derived FTS indexes are separately asserted: `journal_search` passes the existing rebuild-verify parity check, and `artifact_search` holds exactly one row per artifact body with matching content (import also writes `artifact_search` via `upsertArtifactSearchTx` and rebuilds `journal_search`, so canonical-table identity alone cannot prove the indexes survived — review round 3 finding 3). Widened from the round-1 four-table comparison per review rounds 2 and 3.
5. **Inventory and effects are separate contracts with separate warning ownership.** Existing plan fields keep their file-inventory meanings and the existing plan `warnings` field keeps inventory I/O/parser warnings; `import_report.warnings` carries status and provenance outcomes. The `mode` discriminant (`simulation`/`inventory`) makes the dry-run response shape deterministic: `import_report` present exactly when simulation ran. Resolves review round 2 finding 8 and round 1 finding 10.
6. **Source status is classified, then dispatched through the normative decision matrix.** Specs and tasks take `TASKS.json` index status over frontmatter (PR #130 precedence); reports derive `archived` from the archive directory as a real directory-derived source status and frontmatter otherwise; ideas and brainstorms insert `open`, their canonical base state, for no-opinion input — a deliberate behavior change from today's importer, which stores an explicit `status: unknown` verbatim (`markdown_import.go:299`); stored `unknown` rows that older imports created that way remain fillable like every other stored `unknown`. The matrix is total: every (stored status × incoming disposition) cell is defined, including out-of-vocabulary input never filling a stored `unknown`. Resolves review round 2 finding 7 and round 3 finding 1.
7. **Spark derivation has target-refresh semantics, not full lifecycle ownership.** Delete-then-reinsert of a spark's imported relationships repairs changed promoted-to targets while the line remains a spark; removed or type-changed lines are explicitly deferred (Scope Out) rather than silently half-handled. Narrowed per review round 2 finding 10.
8. **The inventory fallback is honest, not apply-equivalent.** When no database or registered project exists there is nothing to collide with; the fallback's counters are fixed (archive sparks, surfaced read errors) and its output is labeled, and no parity claim is made for it. Preview never creates the database or project row; simulation requires both via read-only resolution (`LookupProjectIdentityForRoot` errors on unregistered — verified read-only).
9. **`--resume` remains an alias of `--apply`; JSON contract version stays 1.** Idempotent semantics make the distinction cosmetic; report fields are additive under the existing convention.

## Planning Contract

### Approach

The importer produces an `ImportReport` (reclaimed, skipped with mechanisms, divergences, outcome warnings) inside its own transaction, and `ApplyMarkdownMigration` stops calling preview — it computes inventory before opening the import transaction, then carries that inventory plus the in-transaction report, closing the TOCTOU window. Any post-commit file-access failure degrades to a plan warning so a committed import is never reported as failure. `SimulateMarkdownMigration(ctx, root, resolver)` takes the Backup-primitive snapshot, then invokes the same apply entry point with a resolver pointed at the snapshot — simulation and apply are one code path by construction, differing only in which database file they address and in the result envelope (`applied: false`, `action: simulate`, `mode: simulation`, project identity populated from the snapshot's registration). The origin decision is a pure helper over (origin row, paired journal row) implementing the Decision 1 fingerprint; the status decision is a pure helper over (stored status, incoming normalized status) implementing Decision 2. The foreign-origin skip happens before `upsertJournalEntry` and covers the line's derived writes — sparks, spark aliases, and promoted-to relationships from a skipped line are skipped with it. All four `upsertX` statements drop `status = excluded.status`; the insert-only rule reads the existing row first so divergences are recorded with both sides.

### Status decision matrix (normative)

Incoming source status is classified into three dispositions before any write: **no-opinion** (absent, or explicitly `unknown`), **normalized** (canonical or legacy vocabulary, mapped through `CanonicalLifecycleStatus`), or **out-of-vocabulary** (anything else; for shaping drafts, which have no lifecycle vocabulary, every explicit value is carried raw and none of this classification applies beyond the no-opinion default).

Insert values when no row exists:

| Kind | No-opinion input inserts | Normalized input inserts | Out-of-vocabulary input inserts |
|------|--------------------------|--------------------------|--------------------------------|
| spec / task / report | `unknown` (fillable later) | canonical form | raw value + `import_report` warning |
| report in `archive/` directory | `archived` (the directory is a real directory-derived source status, not no-opinion) | canonical form | raw value + warning |
| idea / brainstorm | `open` (canonical base state — explicit `status: unknown` no longer stores `unknown`, changing today's behavior at `markdown_import.go:299`) | canonical form | raw value + warning |
| spark | `open` (existing behavior, unchanged) | n/a | n/a |
| shaping draft | `draft` | raw value (no vocabulary, no warning) | raw value (no vocabulary, no warning) |

Existing-row disposition, uniform across all kinds (stored `unknown` rows exist for any kind in databases written by today's importer — including ideas and brainstorms with explicit `status: unknown` frontmatter — and are all fillable):

| Incoming ↓ / Stored → | `unknown` | Real status |
|-----------------------|-----------|-------------|
| No-opinion | keep, no divergence, no warning | keep, no divergence, no warning |
| Normalized | fill with canonical form | keep; divergence recorded iff canonical(incoming) ≠ canonical(stored) |
| Out-of-vocabulary | keep `unknown` + warning; no divergence (an invalid value never fills) | keep + warning; no divergence |

Divergence comparison canonicalizes both sides, so a stored legacy `complete` against an incoming `done` is not a divergence.

### Placement

`internal/state/markdown_import.go` (decision helpers, report, skip-before-upsert ordering, spark relationship refresh), `internal/state/markdown_migration.go` (inventory function, counter fixes, simulate dispatch and result envelopes), a lower-level snapshot function extracted from the backup machinery in `backup.go` (caller-owned destination and cleanup, loaf-owned `0700` temp dir, `0600` files, pre-registered cleanup), `internal/cli/cli.go` (dry-run dispatch with the resolver passed in — today it resolves the database path only after preview (`cli.go:3565`) — plus human rendering and JSON passthrough). The doctor caller `inspectUnimportedLocalMarkdown` in `internal/state/status.go` keeps calling the cheap inventory function and must not trigger simulation. Tests in `internal/state/markdown_import_test.go`, `markdown_migration_test.go`, and CLI-level parity assertions in `internal/cli/cli_test.go`.

### Snapshot mechanics

The snapshot is produced by a lower-level function extracted from the backup machinery in `backup.go` — `VACUUM INTO` from the live store to a caller-owned destination, with the caller owning the cleanup lifecycle. The public `state.Backup` API cannot serve: it Inspect-gates `ModeInvalid` (including behind-schema) into a generic doctor error before ever reaching the pipeline (`backup.go:139`), rejects OS-temp destinations as volatile (`backup.go:377`), owns its filename format (`backup.go:697`), and completes its reservation before verification so a verification failure deliberately keeps the file (`backup.go:214`). The extracted primitive hard-fails simulation on snapshot `integrity_check`/`foreign_key_check` failures; journal-search and provenance readiness are informational only — the pipeline's own FTS rebuild is the repair path for derived drift, and the backup layer itself already treats parity as non-structural (`TestVerifyBackupReportsDivergentJournalSearchWithoutStructuralFailure`, `backup_test.go:667`). VACUUM rowid renumbering is harmless here because the pipeline rebuilds `journal_search` from the canonical rows on the snapshot regardless. The snapshot and its `-wal`/`-shm` siblings live in a loaf-owned `0700` directory with `0600` files named `markdown-simulate-<pid>-<timestamp>.sqlite`; cleanup is registered before `VACUUM INTO` starts so creation failure, ENOSPC, or cancellation leaves no partial file, and teardown removes siblings. Deletion is best-effort under process crash; the naming pattern is the contract that lets a future sweep identify strays. Concurrent-writer behavior is the Backup primitive's existing busy/timeout semantics; simulation fails with a clear snapshot-creation error rather than retrying indefinitely.

### Risks

- Snapshot cost scales with the global database, not the project; accepted because dry-run is interactive and rare, `VACUUM INTO` compacts, and the failure mode (ENOSPC) is a clean pre-registered-cleanup abort.
- The fingerprint can false-negative (a genuine backfill row that gained evidence or drifted from its journal row is skipped, not reclaimed) — safe direction, visible in the skipped list rather than silent.
- Insert-only status changes semantics for anyone still treating Markdown as live status authority after first import; the divergence report is the mitigation — nothing silently changes, and the kept-vs-incoming pair is printed.
- Per-table conditional status writes are easy to get subtly wrong; the shared decision helper plus the normative table's per-kind tests guard against copy-paste drift.

### Sequencing

U1 defines the report type and semantics that U2 and U4 consume, so it lands first; U3 is independent of U1/U2 and can proceed in parallel; U5 grows alongside each unit rather than trailing; U6 is mechanical once the CLI surface settles.

## Implementation Units

- **U1 — Import decision semantics and report contract.** Fingerprint reclaim over (origin, journal) row pairs, unknown-is-never-authoritative status disposition per the normative table, divergence recording, spark-derived relationship delete-then-reinsert, and the `import_report` object computed in-transaction with apply no longer calling preview.
- **U2 — Snapshot simulation.** Extract the lower-level snapshot primitive from the backup machinery (caller-owned destination and cleanup, hard integrity/FK gate, hygiene contract: permissions, pre-registered cleanup, sibling teardown), `SimulateMarkdownMigration(ctx, root, resolver)` invoking the full apply pipeline against the snapshot with no Inspect pre-gate so typed preflight errors match apply, `mode` discriminant, inventory-fallback dispatch labeled inventory-only, and the inventory counter fixes.
- **U3 — Vocabulary normalization parity.** `CanonicalLifecycleStatus` at insert for tasks, reports, ideas, brainstorms per the normative table; raw-plus-warning for explicit out-of-vocabulary values; absent/explicit-`unknown` as no-opinion. Shaping drafts excluded from normalization (no lifecycle vocabulary); they get the status-disposition rule from U1 and nothing more.
- **U4 — CLI rendering.** Dry-run dispatch passing the resolver, human output for `mode`, `import_report`, and project identity on simulate (no more hardcoded `project: (not initialized)` when a registered project simulated), `--json` passthrough for dry-run, apply, and resume.
- **U5 — Regression fixtures and tests.** Gridsight-shaped fixture (0011-fingerprint rows, `dirty = 1` and copy-mismatched `unknown` rows, archived spec with terminal legacy source status, foreign-mechanism entry, future-envelope entry), simulate/apply parity, second-apply idempotence over the full table dump, per-kind status matrix from the normative table including `TASKS.json`/frontmatter conflicts, changed spark targets, live-file byte preservation (main + WAL, quiescent, `-shm` excluded), FTS-failure parity, snapshot-creation-failure hygiene, and dispatch routing (no DB, DB without project, behind-schema DB, checksum drift, explicit `StateHome`, `LOAF_DB`).
- **U6 — Docs.** CLI reference regeneration and the loaf-reference/skill passages describing `loaf migrate markdown` semantics, the `mode` discriminant, and the inventory-vs-simulation distinction.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** `npm run test` (go test ./...) passes on the branch.
- **V2.** Named state tests pass: fingerprint-reclaim (clean backfill rows reclaimed; `manual`/`skill`/`hook`, custom mechanisms, envelope-v2 rows, `dirty = 1` rows, populated-`observed_harness_version` rows, and journal-copy-mismatched rows all skipped with rows byte-preserved), every cell of the status decision matrix for every kind (fill over stored `unknown` — including a pre-existing `unknown` idea/brainstorm written by today's importer from explicit frontmatter — archived-stays-archived, no-opinion input over a real status producing no divergence, canonical-equality producing no divergence for legacy spellings, out-of-vocabulary input warning without filling stored `unknown`), and spark-target-change cleanup.
- **V3.** `loaf build` and `npm run typecheck` succeed; regenerated CLI reference committed if it changes.
- **V4.** Isolated smoke under `LOAF_DB=$(mktemp -d)/loaf.sqlite`: seed the gridsight-shaped fixture; `--dry-run --json` and `--apply --json` report equal `import_report` values with no intervening state change; a second `--apply` reports `reclaimed_origins: 0`, the logical dump of the thirteen canonical business tables (timestamps excluded) is identical before and after it, `journal_search` passes the rebuild-verify parity check, and `artifact_search` holds exactly one row per artifact body with matching content.
- **V5.** Byte test under a quiescent fixture: sha256 of the live main database and `-wal` are identical before and after `--dry-run`; `-shm` is excluded as ephemeral; the snapshot and its siblings are gone afterward.
- **V6.** FTS-failure parity: with a corrupted `journal_search` table, simulate and apply fail with the same error; the live database is untouched by the failed simulate. Snapshot-creation failure (e.g. unwritable temp dir) reports its own error class and leaves no partial snapshot.
- **V7.** Dispatch routing: no database → inventory fallback (`mode: inventory`, no `import_report`); database without registered project → inventory fallback; behind-schema database → simulation fails with the same schema-preflight error apply gives; registered current-schema project → simulation; explicit `StateHome` and `LOAF_DB` both honored by the resolver passed from the CLI.

<!-- Human review: -->

- **H1.** A reviewer confirms no code path writes `migration` over any origin failing the Decision 1 fingerprint, that the fingerprint check reads both the origin and journal rows, and that skipped entries are never partially mutated (entry, origin, sparks, aliases, relationships all untouched).
- **H2.** No new CLI flags exist; `--resume` behavior is unchanged as an alias.
- **H3.** JSON output changes are additive only — every pre-existing field keeps its name, type, and inventory meaning; plan `warnings` keeps inventory warnings; effects and outcome warnings live in `import_report`.
- **H4.** The doctor diagnostic (`inspectUnimportedLocalMarkdown`) still calls only the cheap inventory function, and the snapshot helper's cleanup registration precedes snapshot creation in the code.

## Definition of Done

- All six units landed on `markdown-reimport-safety`, V1–V7 green, H1–H4 confirmed in review.
- `loaf change check` reports zero violations.
- PR squash-merged per convention, with the gridsight ops note (re-archive SPEC-007/008) and the doctor-check follow-up (archived-flip damage detection, orphaned-derivative GC, snapshot-residue sweep) filed where triage will see them.

## Durable Outputs

- An ADR in `docs/decisions/` recording the import authority model: information-free-above-journal-row origin reclaim, unknown-is-never-authoritative status disposition, and full-pipeline snapshot simulation as the permanent contract for one-way Markdown migration.
- Updated `loaf-reference` skill passage for `loaf migrate markdown` describing the report fields, the `mode` discriminant, the no-abort collision semantics, and the parity precondition.

## Open Questions

- [KU] Do any plugin or skill surfaces parse the migration JSON strictly enough to notice new fields? → owner: U4 (verify consumers during CLI wiring; Go's JSON decoding is tolerant, but rendered docs may enumerate fields).
- [KU] Does the Backup primitive's busy/timeout behavior need a tighter bound for interactive dry-run use? → owner: U2 (measure under the fixture; fail-fast with a clear error beats waiting).

## Source Inputs

- Journal findings `journal:453d67cf2bd7bf131a1b5f09` (origin-collision dry-run/apply asymmetry) and `journal:8038077b8a7e19e3abe77533` (lifecycle status clobber), triaged 2026-07-23 from the gridsight-core-gds session `loaf-migrate-markdown-workflow`.
- Empirical evidence from the global database: 2,841 `unknown`-mechanism origins in gridsight matching the 0011 backfill shape; SPEC-007/008 archived→complete flip with 2026-07-08 archive events intact.
- PR #130 (spec terminal-status vocabulary close) as the landed base this builds on; PR #127 and schema migration 0011 as the origin-vocabulary context.
- Shaping interview in this conversation (2026-07-23), decisions 1–3 as originally accepted.
- Adversarial review round 1 (Codex, gpt-5.6 xhigh, 2026-07-23): 10 findings. Round 2 (same thread, 2026-07-24): 10 findings — the fingerprint completed and re-argued as information-free-above-journal-row, the placeholder rule replaced by unknown-is-never-authoritative, the SHM byte guarantee dropped, simulation unified onto the apply pipeline with shared preflight. Round 3 (same thread, 2026-07-24): 3 findings, 7 of 10 round-2 dispositions passing — the status matrix made total over (stored × incoming), the snapshot method changed from wrapping public `state.Backup` to an extracted lower-level primitive after its API was shown incompatible (Inspect gate, volatile-destination rejection, pre-verification reservation), and derived-index parity added to the idempotence contract.
