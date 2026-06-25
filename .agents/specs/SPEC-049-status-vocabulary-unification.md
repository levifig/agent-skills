---
id: SPEC-049
title: "Status-Vocabulary Unification"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-C)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
created: 2026-06-22T09:13:21Z
status: done
branch: feat/status-vocabulary-unification
---

# SPEC-049: Status-Vocabulary Unification

## Problem Statement

Loaf carries **five incompatible per-entity lifecycle vocabularies**, each defined and ordered
independently, with no shared model and no validation against a common set:

| Entity | Statuses (as coded) | Defined at |
|--------|---------------------|------------|
| spec | `implementing`, `approved`, `drafting`, `complete`, `archived` | `internal/state/spec_list.go:11` |
| task | `todo`, `in_progress`, `blocked`, `review`, `done` (+ `archived` for list filter) | `internal/state/task_list.go:12-13` |
| report | `draft`, `final`, `archived` | `internal/state/report_lifecycle.go:149,158,163` |
| session | `active`, `stopped`, `done`, `blocked`, `archived` | `internal/state/session_start.go:138,159`, `internal/state/session_end.go:14-15,140-153` |
| idea / spark / brainstorm | `open` → `resolved` (→ `archived`) | `internal/state/idea.go:568-579`, `internal/state/spark.go:594-605`, `internal/state/brainstorm.go:109-120` |

The drift is not cosmetic. The same conceptual phase (e.g. "this is done / closed / finalized") is
spelled `complete`, `done`, `final`, and `resolved` across entities. Reports surface `active` and
`unknown` to users where the real states are `draft`/`final`/`archived` (called out as a defect in
the roadmap, WS-B note). There is no `Valid*Status` for report, session, idea, spark, or brainstorm
— only spec (via `specStatusOrder`, `internal/state/spec_list.go:11`) and task
(`ValidTaskStatus`, `internal/state/task_list.go:198`) validate at all, so invalid statuses can be
written silently. Every `include*Status` filter hard-codes literals (`status == "resolved"`,
`status != "active"`) that would all need to change in lockstep.

The lifecycle history compounds the problem: every status transition writes an `events` row with
`from_status`/`to_status` (`internal/state/migrations/0001_initial.sql:156-157`), populated at ~14
call sites across `task_*.go`, `spec_archive.go`, `report_lifecycle.go`, `session_*.go`,
`idea.go`, `spark.go`, `brainstorm_archive.go`. Unifying the vocabulary means rewriting the stored
strings in already-populated tables — a **data migration**, not just a code change. Without this,
`loaf search` (SPEC-043), cross-entity status filtering, and any uniform `list --status` flag are
built on five mutually-unintelligible enums.

## Strategic Alignment

- **Vision:** Advances *Structured Execution* and the "mechanical enforcement, not a prompt
  library" stance — a single validated vocabulary lets `loaf check`, list filters, and search treat
  status as a typed dimension instead of free text.
- **Architecture:** One global SQLite DB is the source of truth (SPEC-040). A coherent status model
  is the cross-entity contract that makes uniform verbs (`list`/`show`/`new`/`edit`/`archive`,
  SPEC-043) and FTS5 status-filtered search meaningful across entities.

**Carved out of SPEC-043 (WS-B) per review.** SPEC-043 defers status-vocabulary unification
(Resolved Decisions #4) to SPEC-049; this spec owns the canonical set, validation, and the `events`
rewrite. SPEC-043 has **zero hard dependency** on this spec: bodies-in-SQLite, search, and uniform
verbs can land with the legacy per-entity vocabularies intact, and this unification can follow
independently.

**Coordinates-with:**
- **SPEC-048 (WS-C, Session-Model Convergence):** the session enum
  (`active`/`stopped`/`done`/`blocked`/`archived`) is reconciled *there* — SPEC-048 owns the session
  schema-in-one-file lint and the `wrap` model. SPEC-049 must adopt whatever canonical session
  states SPEC-048 settles on; the two are **sequenced together** so the session row is migrated once,
  not twice. Where they touch the same `session_*.go` files, SPEC-048 lands first or they land in the
  same change.
- **SPEC-053 (WS-G, Breaking-Change Migration Mechanism):** this is a **breaking data rewrite** of a
  populated table and must run behind SPEC-053's reversible/backed-up migration gate with user
  sign-off.
- **SPEC-035 (TASKS.json staleness):** the legacy `TASKS.json` subsystem is still live. Task status
  has two homes; rewriting the SQLite task vocabulary without reconciling `TASKS.json` re-creates the
  dual-source drift SPEC-035 fights. Name the `TASKS.json` status mapping explicitly (Open Question).
- **SPEC-037 (mutable specs):** spec status semantics (`drafting`/`approved`/`implementing`) interact
  with SPEC-037's "specs are mutable work definitions"; do not redefine spec lifecycle *meaning*,
  only the spelling/validation.

## Solution Direction

Define **one canonical lifecycle vocabulary** as a typed set in a single Go source location, with
per-entity *allowed subsets* (not every entity uses every state) and a shared validator. The model
is a small set of universal phases that each entity maps onto; entity-specific nuance is preserved
where it carries real meaning (e.g. task `blocked`, session `stopped` vs `done`) rather than
collapsed for tidiness.

Canonical lifecycle phases are deliberately small, but not artificially collapsed. `draft`, `open`,
and `todo` stay distinct because they mean different things: an authored durable artifact is being
written, a triage object is unresolved, or committed work is waiting to start.

| Canonical phase | Today's spellings it absorbs | Primary use |
|-----------------|------------------------------|-------------|
| `draft` | spec `drafting`; report/plan/handoff/council `draft` | authored durable artifacts not finalized |
| `open` | idea/spark/brainstorm `open` | unresolved intake/triage objects |
| `todo` | task `todo`; spec `approved` | accepted work not started |
| `in_progress` | task `in_progress`; spec `implementing`; session `active` | active execution |
| `blocked` | task `blocked`; session `blocked` | work waiting on an external condition |
| `review` | task `review` | implementation awaiting review |
| `done` | task `done`; spec `complete`; report/plan/handoff/council `final`; idea/spark/brainstorm `resolved`; session `done` | completed/non-archived work |
| `paused` | session `stopped` | intentionally stopped resumable sessions |
| `archived` | all lifecycle entities' `archived` | hidden historical records |

Domain-specific status vocabularies that are not lifecycle phases stay in their own explicit
registries. Findings (`open`/`confirmed`/`refuted`/`partial`/`archived`) and runs
(`pending`/`running`/`completed`/`failed`/`archived`) from SPEC-054 are not flattened into this
model; SPEC-049 only requires them to be declared outside the lifecycle registry instead of
appearing as anonymous literals.

Each entity declares which subset it permits; `Valid<Entity>Status` validates against that subset;
`<Entity>StatusOrder` returns display order over it. A migration rewrites every populated row in the
entity tables and the `events` table:

- **Entity rows:** rewrite `status` to the canonical spelling.
- **`events` rows — the honest contract:** rewrite `to_status` to the canonical spelling.
  `from_status` for **pre-existing** rows is *not* faithfully reconstructable to the canonical model
  (the prior state may map ambiguously, and older rows predate consistent capture). Rather than
  fabricate history, the migration **appends exactly one synthetic normalization event per rewritten
  entity** (`event_type = "status_normalized"`, `from_status` = the entity's last known legacy
  status, `to_status` = its canonical equivalent, `note` describing the SPEC-049 migration). Existing
  `events` rows keep their original `from_status` as historical record; `to_status` is normalized so
  current-state status queries can treat event destinations as typed lifecycle data.

Migration runs against a **copy of the DB first**, takes a **backup** via the existing state-backup
mechanism, and is **reversible** (a down-mapping table from canonical → legacy spelling is retained
so the rewrite can be undone). All of this rides the SPEC-053 migration harness.

After migration, update every `include*Status` filter, the human-readable and `--json` list output,
and add a lint asserting the canonical vocabulary lives in exactly one source file (mirroring
SPEC-048's "session schema in one file" lint, generalized).

## Scope

### In Scope
- Canonical lifecycle vocabulary defined once (single Go file) with per-entity allowed subsets.
- `Valid<Entity>Status` + `<Entity>StatusOrder` for **every lifecycle entity** (spec, task, report,
  session, idea, spark, brainstorm, plan, handoff, council), replacing the ad-hoc literals and
  missing validators.
- Rewrite of every `insert*StatusEvent` / events-insert call site
  (`task_create.go:149`, `task_update.go:204`, `task_archive.go:178`, `spec_archive.go:130`,
  `report_lifecycle.go:132,208`, `session_start.go:294`, `session_end.go:205`, `idea.go:254,379,552`,
  `spark.go:367,503`, `brainstorm_archive.go:126`) to emit canonical spellings.
- A reversible, backed-up data migration that rewrites populated entity `status` and events
  `to_status`, and emits one synthetic `status_normalized` event per rewritten entity.
- Update every `include*Status` filter and list/show output (human-readable + `--json`).
- A lint/test: the canonical vocabulary is defined in exactly one file; no entity uses a status
  outside its declared subset.
- Reconcile the report `active`/`unknown` display defect against the canonical set.
- Explicitly register domain-specific non-lifecycle vocabularies (finding status, run status,
  verdict outcomes) as excluded from lifecycle canonicalization.

### Out of Scope
- The session-state semantics themselves (`stopped` vs `done`, `wrap` model) — owned by SPEC-048.
- Bodies-in-SQLite, search, uniform verbs, new entity types — owned by SPEC-043.
- The migration harness itself (deprecation window, backup/restore plumbing) — owned by SPEC-053;
  this spec *consumes* it.
- `TASKS.json` retirement — owned by SPEC-035; this spec only defines the status mapping for it.
- Changing finding/verdict semantics from SPEC-054; those statuses are domain outcomes, not the
  core lifecycle vocabulary.

### Rabbit Holes
- **Bikeshedding the exact universal set.** Pick a small set with SPEC-048, migrate, move on. The
  candidate table above is a starting point, not a debate prompt.
- **Reconstructing faithful `from_status` history.** It is not faithfully reconstructable for
  pre-event rows; do not try. The synthetic-boundary-event contract is the honest answer.
- **Collapsing genuinely distinct states for tidiness** (e.g. merging `blocked` into `paused`, or
  `review` into `in_progress`). Preserve meaning that drives behavior.

### No-Gos
- Rewriting `events` history *in place* such that the original transition record is lost.
- Migrating the live DB without a prior copy-run + backup.
- Shipping the rewrite before SPEC-053's migration gate and user sign-off exist.
- Validating only some entities (the current half-validated state is the disease, not a partial fix).

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Data loss / corruption during events + entity rewrite | Med | High | Run on DB copy first; mandatory backup; reversible down-mapping; behind SPEC-053 gate |
| Divergence from SPEC-048's session enum (two rewrites) | Med | High | Sequence with SPEC-048; adopt its canonical session states; land in same change where files overlap |
| `TASKS.json` re-introduces dual-source status drift (SPEC-035) | Med | Med | Define explicit task status mapping for `TASKS.json`; canonical SQLite writes lead; compatibility exports translate where needed |
| Synthetic events confuse downstream history readers | Low | Med | Distinct `event_type = "status_normalized"`; documented; one per entity |
| Spec/report status *meaning* accidentally redefined (SPEC-037) | Low | Med | Only respell/validate; do not change lifecycle semantics |
| Missed call site leaves a legacy spelling in new rows | Med | Med | One-file vocabulary + lint; exhaustive call-site list above; test that no status outside subset is writable |

## Open Questions
- [x] Exact canonical set and per-entity subsets: use `draft`, `open`, `todo`, `in_progress`,
      `blocked`, `review`, `done`, `paused`, `archived`; each entity declares an allowed subset.
- [x] `events` rewrite policy: normalize `to_status`, preserve historical `from_status`, and append
      one `status_normalized` boundary event per rewritten entity.
- [x] `paused` is a canonical phase, with sessions as the first allowed entity subset.
- [x] `TASKS.json` mapping: SQLite canonical task statuses lead. Compatibility exports/imports map
      task statuses directly because task `todo`/`in_progress`/`blocked`/`review`/`done` stay
      canonical; `archived` is list/archive state, not a writable active task update.
- [x] Down-mapping fidelity: the migration records each entity's original legacy status before
      rewrite; rollback restores from that manifest rather than inferring from canonical status
      alone.
- [x] SPEC-054 statuses: finding, verdict, and run vocabularies are domain-specific and must remain
      explicit, but are not lifecycle-canonicalized by SPEC-049.

## Test Conditions
- [x] A single Go source file defines the canonical vocabulary (`internal/state/lifecycle_status.go`);
      regression coverage checks the registry helpers and explicit domain-vocabulary exclusions.
- [x] `Valid<Entity>Status`/registry validation exists for spec, task, report, session, idea, spark,
      brainstorm, plan, handoff, and council, with tests rejecting out-of-subset values.
- [x] Writing an out-of-subset status to lifecycle entities is rejected before SQLite writes.
- [x] Migration on a DB copy rewrites every lifecycle entity `status` to its canonical spelling with
      zero legacy lifecycle statuses remaining in the copy.
- [x] After migration, each rewritten entity has exactly one `status_normalized` event whose
      `from_status` is its last legacy status and `to_status` its canonical equivalent.
- [x] Pre-existing `events` rows preserve historical `from_status`; only `to_status` is normalized
      for query correctness.
- [x] The migration is reversible: rollback restores original entity `status` bytes and event
      `to_status` values from the manifest.
- [x] A backup is taken before live apply and before rollback through the existing state backup
      mechanism.
- [x] Report list no longer surfaces `active`/`unknown`; it shows canonical statuses only.
- [x] Session statuses use the SPEC-048 canonical set: `in_progress`, `paused`, `blocked`, `done`,
      `archived`.
- [x] Plan, handoff, and council statuses use the same lifecycle registry as reports.
- [x] Finding, verdict, and run status tests prove those domain vocabularies remain explicitly
      registered and are not anonymous lifecycle literals.

Evidence: `go test ./internal/state ./internal/cli -count=1`, `npm run build`, `npm run typecheck`,
`npm run test`, and `git diff --check` in TASK-396..TASK-400 implementation slices. No
`docs/schema/*` DDL update was required for TASK-399 because lifecycle-status normalization is a
data migration with JSON rollback manifests, not a schema migration.

## Priority Order

Tracks ship in this order. If scope needs cutting, drop from the end. Marked non-breaking vs gated.

1. **Define the canonical lifecycle contract + registry + validators** (single file,
   `Valid<Entity>Status`, `<Entity>StatusOrder`, explicit domain-vocabulary exclusions).
   *Non-breaking* (additive; call sites can still emit legacy spellings until track 2). Go/no-go:
   validators + registry tests pass before touching write paths.
2. **Rewrite entity write paths and events to canonical spellings.** *Breaking for newly-written
   rows, but gated behind this branch/PR and SPEC-053's migration mechanism.* Go/no-go: all write
   sites emit statuses from the registry, and invalid writable statuses are rejected.
3. **Normalize list filters, displays, housekeeping, exports, and generated help.** Go/no-go:
   human/JSON output uses canonical statuses; compatibility exports name any legacy translation
   explicitly.
4. **Data migration: rewrite populated entity `status` + events `to_status`, emit synthetic
   normalization events; reversible + backed-up.** **BREAKING — gated on SPEC-053 and user
   sign-off.** Go/no-go: copy-run succeeds, reverse-migration verified, backup taken, sign-off
   obtained before running against the live global DB.
5. **Docs/generated-output refresh.** Regenerate CLI reference and tracked build artifacts, update
   schema docs/changelog, and close the PR verification matrix.

## Task Breakdown

Local task rows exist in SQLite for scheduling; markdown task files are intentionally not generated
in SQLite mode.

1. **TASK-396 — Finalize lifecycle status contract and registry** (P1)
   - Depends on: none.
   - File hints: `internal/state/lifecycle_status.go`, `internal/state/*_test.go`, existing
     `finding_vocab.go` tests for explicit non-lifecycle vocabularies.
   - Scope: add the canonical lifecycle registry, per-entity subsets, mapping tables from legacy
     spellings, status-order helpers, validator tests, and domain-vocabulary exclusions for
     finding/verdict/run statuses.
   - Acceptance: every lifecycle entity has a declared subset; validators reject out-of-subset
     values; domain vocabularies remain explicit but outside lifecycle canonicalization.
   - Verification: `go test ./internal/state -run 'LifecycleStatus|FindingVocab|RunStatus' -count=1`.

2. **TASK-397 — Wire lifecycle validators into entity writes** (P1)
   - Depends on: TASK-396.
   - File hints: `internal/state/task_create.go`, `task_update.go`, `task_archive.go`,
     `spec_archive.go`, `report_lifecycle.go`, `session_*.go`, `idea.go`, `spark.go`,
     `brainstorm*.go`, `artifact_entities.go`.
   - Scope: update all lifecycle write paths and status events to emit canonical spellings, and
     reject invalid writable statuses before SQLite writes.
   - Acceptance: new rows and `status_changed` events use registry values only; invalid report,
     session, idea, spark, brainstorm, plan, handoff, and council statuses are rejected.
   - Verification: targeted lifecycle write tests plus `go test ./internal/state -run 'Status|Lifecycle|Task|Report|Session|Idea|Spark|Brainstorm|Artifact' -count=1`.

3. **TASK-398 — Normalize lifecycle list filters and displays** (P1)
   - Depends on: TASK-396.
   - File hints: list/show/export/housekeeping code under `internal/state`, CLI help/reference code
     under `internal/cli`.
   - Scope: update `include*Status` filters, list/show JSON, human output, housekeeping sections,
     and compatibility exports to use canonical statuses or explicit legacy-translation wording.
   - Acceptance: report list never emits `active`/`unknown`; archived/default filters use entity
     subsets; task compatibility status mapping is documented and tested.
   - Verification: `go test ./internal/state ./internal/cli -run 'List|Show|Export|Housekeeping|Status' -count=1`.

4. **TASK-399 — Add reversible lifecycle status migration** (P1)
   - Depends on: TASK-397, TASK-398.
   - File hints: `internal/state/migrations`, migration harness code from SPEC-053, schema docs.
   - Scope: add copy-run, backup-gated, reversible migration that rewrites entity statuses and
     event `to_status`, emits one `status_normalized` event per rewritten entity, and stores
     enough original spelling data for rollback.
   - Acceptance: copy-run leaves zero legacy lifecycle statuses, preserves `from_status`, appends
     boundary events exactly once, and rollback restores entity `status` bytes from the manifest.
   - Verification: migration fixture tests with apply/rollback plus `go test ./internal/state -run 'LifecycleStatusMigration|MarkdownRollback|Backup' -count=1`.

5. **TASK-400 — Refresh status vocabulary docs and generated outputs** (P2)
   - Depends on: TASK-399.
   - File hints: `CHANGELOG.md`, `docs/schema/*`, generated `dist/**`, `plugins/loaf/**`,
     `content/skills/cli-reference/SKILL.md`.
   - Scope: update schema/docs/changelog, regenerate all targets, and close standard verification.
   - Acceptance: tracked generated artifacts match source, changelog has an unreleased entry, and
     spec test conditions are checked off with evidence.
   - Verification: `npm run build`, `npm run typecheck`, `npm run test`, `git diff --check`.
