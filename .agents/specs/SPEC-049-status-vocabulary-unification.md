---
id: SPEC-049
title: "Status-Vocabulary Unification"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-C)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
created: 2026-06-22T09:13:21Z
status: drafting
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

Candidate universal phases (exact set is an Open Question, to be decided *with* SPEC-048, not
bikeshedded):

| Universal phase | Today's spellings it absorbs |
|-----------------|------------------------------|
| `open` / `active` | idea/spark/brainstorm `open`; session `active`; task `todo` |
| `in_progress` | task `in_progress`; spec `implementing` |
| `blocked` | task `blocked`; session `blocked` |
| `review` | task `review`; spec `approved`; report `draft` (under review) |
| `done` | task `done`; spec `complete`; report `final`; idea/spark/brainstorm `resolved`; session `done` |
| `paused` | session `stopped` |
| `archived` | all entities' `archived` |

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
  `events` rows keep their original `from_status`/`to_status` *spellings as historical record* — the
  migration does **not** silently rewrite history in place beyond `to_status` normalization needed
  for current-state queries. (Final in-place-vs-synthetic policy is an Open Question to settle before
  breakdown; the default here is: normalize `to_status` for query correctness, do not invent
  `from_status`, emit one synthetic boundary event.)

Migration runs against a **copy of the DB first**, takes a **backup** via the existing state-backup
mechanism, and is **reversible** (a down-mapping table from canonical → legacy spelling is retained
so the rewrite can be undone). All of this rides the SPEC-053 migration harness.

After migration, update every `include*Status` filter, the human-readable and `--json` list output,
and add a lint asserting the canonical vocabulary lives in exactly one source file (mirroring
SPEC-048's "session schema in one file" lint, generalized).

## Scope

### In Scope
- Canonical lifecycle vocabulary defined once (single Go file) with per-entity allowed subsets.
- `Valid<Entity>Status` + `<Entity>StatusOrder` for **every** entity (spec, task, report, session,
  idea, spark, brainstorm), replacing the ad-hoc literals and missing validators.
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

### Out of Scope
- The session-state semantics themselves (`stopped` vs `done`, `wrap` model) — owned by SPEC-048.
- Bodies-in-SQLite, search, uniform verbs, new entity types — owned by SPEC-043.
- The migration harness itself (deprecation window, backup/restore plumbing) — owned by SPEC-053;
  this spec *consumes* it.
- `TASKS.json` retirement — owned by SPEC-035; this spec only defines the status mapping for it.

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
| `TASKS.json` re-introduces dual-source status drift (SPEC-035) | Med | Med | Define explicit status mapping for `TASKS.json`; coordinate with SPEC-035 |
| Synthetic events confuse downstream history readers | Low | Med | Distinct `event_type = "status_normalized"`; documented; one per entity |
| Spec/report status *meaning* accidentally redefined (SPEC-037) | Low | Med | Only respell/validate; do not change lifecycle semantics |
| Missed call site leaves a legacy spelling in new rows | Med | Med | One-file vocabulary + lint; exhaustive call-site list above; test that no status outside subset is writable |

## Open Questions
- [ ] Exact canonical universal set and per-entity subsets (decide *with* SPEC-048).
- [ ] `events` rewrite policy: normalize `to_status` only + synthetic boundary event (proposed
      default) vs full in-place rewrite vs append-only with a normalization marker.
- [ ] Whether `paused` (session `stopped`) is a universal phase or session-only.
- [ ] How `TASKS.json` task statuses map to the canonical set and who owns keeping them in sync
      (this spec vs SPEC-035).
- [ ] Down-mapping fidelity for reversibility when multiple legacy spellings collapse to one
      canonical phase (e.g. `complete`/`final`/`done`/`resolved` → `done`): does reverse need the
      original spelling per entity, or is entity-type sufficient to recover it?

## Test Conditions
- [ ] A single Go source file defines the canonical vocabulary; a lint fails the build if any
      status literal is defined elsewhere.
- [ ] `Valid<Entity>Status` exists for spec, task, report, session, idea, spark, and brainstorm and
      rejects any status outside that entity's declared subset.
- [ ] Writing an out-of-subset status to any entity is rejected (was silently accepted for report,
      session, idea, spark, brainstorm before).
- [ ] Migration on a DB copy rewrites every entity `status` to its canonical spelling with zero rows
      left at a legacy spelling.
- [ ] After migration, each rewritten entity has exactly one `status_normalized` event whose
      `from_status` is its last legacy status and `to_status` its canonical equivalent.
- [ ] Pre-existing `events` rows retain their original transition record (history not destroyed).
- [ ] The migration is reversible: running the down-migration restores legacy spellings byte-for-byte
      on the entity `status` column.
- [ ] A backup is taken before the live migration runs (verified by the SPEC-053 harness contract).
- [ ] Report list no longer surfaces `active`/`unknown`; it shows canonical statuses only.
- [ ] Session statuses produced match SPEC-048's canonical session set (cross-spec consistency test).

## Priority Order

Tracks ship in this order. If scope needs cutting, drop from the end. Marked non-breaking vs gated.

1. **Define the canonical vocabulary + per-entity subsets + validators** (single file,
   `Valid<Entity>Status`, `<Entity>StatusOrder`). *Non-breaking* (additive; new rows still accept
   legacy spellings until track 3). Go/no-go: validators + one-file lint pass before touching call
   sites.
2. **Rewrite call sites + `include*Status` filters + list/show output to canonical spellings.**
   *Non-breaking to stored data* (no rewrite yet); new rows now use canonical spellings. Go/no-go:
   all 14 events-insert sites and every filter emit/accept canonical only; tests green. Must land
   *with or after* SPEC-048's session changes where `session_*.go` overlaps.
3. **Data migration: rewrite populated entity `status` + events `to_status`, emit synthetic
   normalization events; reversible + backed-up.** **BREAKING — gated on SPEC-053 and user
   sign-off.** Go/no-go: copy-run succeeds, reverse-migration verified, backup taken, sign-off
   obtained before running against the live global DB.
