<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# State Dedupe

## Problem

The 2026-06-24 markdown import ran after migration 3 had rekeyed the project from its legacy path-hash ID to an opaque `proj_…` ID. Entity primary keys are derived — `sha256(kind \0 project_id \0 alias)` — so every re-derived ID missed its existing row, the importer inserted a full second copy of every artifact, and the alias upsert (`ON CONFLICT(project_id, namespace, alias) DO UPDATE SET entity_id`) re-pointed each alias to the new twin. The June-13 originals became alias-orphans: invisible to every list command (all list queries INNER JOIN through `aliases`) yet fully visible to the housekeeping scanner (raw `WHERE project_id = ?`).

The two surfaces now disagree about how much work exists — 299/38/14 scanner vs 233/26/11 canonical for tasks/specs/reports — and the blast radius extends beyond the brief: ideas (51 orphans), sparks (56), brainstorms (3), ~230 duplicated `sources` rows, and one dangling alias (literal `[]`, pointing at a nonexistent task row). `loaf task list --status done --json` returned zero rows while 66 done tasks existed, because the done rows were exactly the orphans. The 2026-08-07 housekeeping pass archived ghosts: all 66 `done → archived` events were written against orphan rows reached by raw internal-ID resolution.

Both halves of the defect remain live in code: the importer still trusts derived IDs alone, and nothing — no schema constraint, no doctor check — detects alias-orphaning. Separately, one report row ("Transitional TypeScript Surfaces — Do Not Deepen", status `active`, out-of-vocabulary) holds its alias but has no body and cites a source file with no git history — unrecoverable evidence.

## Hypothesis

If alias-orphans are classified and retired by an audited migration, the importer resolves identity through aliases before deriving IDs, and doctor checks alias parity, then the scanner and the list commands agree on counts for every entity table in every project, `--json` list output can be trusted by agents and workflows again, and no future rekey, merge, or import can silently fork the identity space — divergence becomes detectable the day it happens instead of discoverable by accident at housekeeping.

## Scope

**In**

- An alias-orphan repair migration (`loaf state migrate alias-orphans`) with the full preview → backup → manifest → apply → verify → rollback ceremony, covering all seven aliased entity tables (tasks, specs, reports, ideas, sparks, brainstorms, shaping drafts — the seventh added in review, because the housekeeping scanner counts it and the importer aliases it), orphaned `sources` rows, dangling alias rows, and the reference-table sweep (events, entity_tags, bundle_members, backend_mappings, exports, relationships, artifact bodies/FTS) for every retired row.
- A journal-duplicates repair migration (`loaf state migrate journal-duplicates`) on the same triad, retiring the ~1,020 June-13 journal rows whose `(entry_type, scope, message)` twins were re-minted at the June-24 instant — journal rows carry no aliases, so this rides natural-key window classification instead of alias reachability (expansion by operator direction; see Decision 7).
- Importer identity fix: markdown import resolves `(project_id, namespace, alias)` against the aliases table first and reuses the existing entity ID; derivation only mints IDs for genuinely new entities. Journal entries, which have no aliases, resolve by natural identity (`entry_type`, scope, message) before deriving; sparks whose message normalizes to an empty slug receive a deterministic content-hash alias so no row is born orphaned.
- `loaf state doctor` gains an alias-parity diagnostic: per-project, per-table raw counts vs alias-reachable counts, plus dangling-alias detection.
- Explicit disposition of the broken-evidence report row: archive as moot with an event recording why (evidence unrecoverable; SPEC-047 already shipped the simplification this report guarded against deepening).
- The production repair ceremony, including the never-run `loaf state migrate lifecycle-statuses --apply` sequenced after the dedupe.

**Out** (deferred, not rejected)

- Write-time status-vocabulary enforcement and set-status verbs — TASK-408 / SPEC-049 territory.
- Sanctioned body-edit paths — TASK-407 / SPEC-055 territory.
- `specTaskCounts` joining only the spec alias (spec_list.go) and raw status literals in task_archive.go/spec_list.go — post-dedupe these count truthfully; revisit if the doctor parity check ever shows drift.
- Stale source *paths* (rows citing files renamed on disk, e.g. the sidecar-audit report) — path drift is not duplication damage.

**Cut** (explicitly rejected)

- Re-derivation of entity IDs anywhere — not in `rekeyLegacyProjectTx`, not as a one-time repair. Derived IDs are mint-once opaque keys; identity lives in the aliases table. All 27 project rows already carry opaque IDs, so the rekey trigger is extinct in this database, and the importer fix neutralizes it everywhere else.
- Fabricating replacement content for the broken-evidence report.
- Changing list-surface JOIN semantics (e.g. LEFT JOIN to display orphans) — an alias-orphan is damage to repair, not a display state.
- New schema-level unique constraints on entity tables — the aliases table's `UNIQUE (project_id, namespace, alias)` is the identity registry.

## Observable Workflow

```
$ loaf state migrate alias-orphans            # preview (default), all projects
  project proj_7afeb3fc… (loaf):
    tasks:   66 orphans — 63 retire (twin proven), 3 unproven (operator disposition required)
    specs:   12 orphans — 12 retire
    reports:  3 orphans —  3 retire
    ideas: 51 retire (derivation); sparks: 46 retire + 10 unproven (June-24-born collision victims — realias, not retire); brainstorms: 3 retire
    sources: N orphan-referenced rows to retire; aliases: 1 dead to delete
  dispositions: report:7644bb23… → archive-as-moot (evidence unrecoverable)

$ loaf state migrate alias-orphans --retire … --realias … # dispositions rehearse in preview too
$ loaf state migrate alias-orphans --apply --retire … --realias …   # backup first, manifest written, verify after

$ loaf state migrate journal-duplicates            # preview: ~1,020 June-13/June-24 twin pairs
$ loaf state migrate journal-duplicates --apply    # same backup/manifest/verify ceremony

$ loaf state doctor                           # alias-parity section green: raw == reachable, 0 dead aliases
$ loaf housekeeping                           # scanner counts now equal list counts
$ loaf task list --status done --json         # returns every done task that exists

$ loaf state migrate lifecycle-statuses --apply   # existing tool, first run, after dedupe
```

## Rabbit Holes and No-Gos

- The migration is a fixed-classification repair for this damage class, not a general database fsck. New damage classes get new migrations.
- Twin-ship is proven by legacy-salt ID recomputation, with content identity (title equality within the event's timestamp cluster) as a distinctly-labeled fallback — never timestamp alone, per the brief. Unproven rows are refused, surfaced, and left for explicit operator disposition; the migration never guesses.
- Do not chase stale source paths, missing bodies on live rows, or status semantics beyond what the existing lifecycle-statuses migration already implements.
- Do not add code that resolves entities by recomputing `stableMigrationID` — that pattern is the root cause. Recomputation appears exactly once, inside the migration's twin-proof, against historical salts.
- The lifecycle-statuses run may surface out-of-vocabulary free-text statuses it cannot map; record their handling in the ceremony and leave any needed set-status verb to TASK-408. Do not extend the vocabulary migration here.

## Decisions

Provenance: operator interview during shaping (2026-08-07, four structured questions), on top of the captured brief and a full code/database investigation (see Problem).

1. **Full blast radius.** One migration repairs all six entity tables plus sources and dangling aliases. Same mechanism, same surgery, one backup — splitting would mean operating on the production database twice. Forecloses a tasks/specs/reports-only partial repair.
2. **Prevention is importer alias-first resolution plus doctor parity; re-derivation is rejected.** With the importer resolving identity through aliases, a rekey can no longer cause orphaning at the next import, so re-deriving IDs (in rekey or as a one-time repair) buys insurance against a neutralized scenario at the cost of rewriting IDs across ~10 tables in one transaction. Forecloses ID-rewriting sweeps permanently.
3. **The broken-evidence report is archived as moot.** Status normalized from out-of-vocabulary `active` and archived by the migration as a named per-row disposition, with an event recording that the evidence is unrecoverable and the guardrail moot (SPEC-047 shipped the simplification it guarded). Forecloses both fabricated replacement content and a permanently-`active` bodyless row.
4. **The lifecycle-statuses migration runs as part of the ceremony**, after dedupe so no effort is spent normalizing rows about to be retired. Zero new code; closes the vocabulary half of the housekeeping finding.
5. **Canonical rows are the alias-holders** (confirmed from the brief with a correction: status vocabulary is *not* a discriminator — both copies carry raw vocab because lifecycle normalization never ran). The orphans retire; the twins survive.
6. **The migration sweeps every project in the global database, not just this one.** All 27 projects were rekeyed by migration 3; any of them with pre-rekey markdown imports carries the same damage. Preview reports per project before any apply, so the blast radius is visible first. (Shaper's decomposition call — flagged for review rather than interviewed.)
7. **The ~1,020 duplicated journal rows fold into this Change as TASK-005** (operator direction, 2026-08-08): the June-24 fork also re-minted journal entries, invisible to the alias-orphan lens because journal rows carry no aliases. Repair work of this kind takes a plan, not a shaping ceremony — the packet carries the plan; a separate `journal-duplicates` migration rides the same triad and runs in the same TASK-004 ceremony. Forecloses both leaving the duplicates permanently and bolting journal classification onto the alias-orphans migration.
8. **Two review-proven calibrations bind the proofs** (round-4 confirmation, reproduced on a production copy): retirement classification iterates to a fixed point, because content-identity and source proofs are orphan-count-sensitive and a retirement can unlock a proof mid-run; and the derivation proof must never gain a body-fingerprint guard — all 132 production derivation-proven pairs have mismatched fingerprints (June-13 originals and June-24 re-imports genuinely differ in content), so title + recency is the calibrated design, recorded as a load-bearing code comment.

## Planning Contract

### Approach

Model the migration on `lifecycle_status_migration.go` — the existing preview/apply/rollback triad: preview runs against a temp copy (`copySQLiteDatabase`), apply takes a mandatory `Backup` first, writes a JSON rollback manifest next to the backup, applies in one transaction, verifies after, and `rollback` restores from the manifest. Register it in the `stateMigrateSources` registry (cli.go:3190) as `alias-orphans`.

Classification, per project, per entity table:

- **Orphan** = entity row with no `aliases` row matching `(project_id, entity_kind, entity_id, namespace)`.
- **Retire (twin proven):** recompute `stableMigrationID(kind, legacy_project_id, alias)` for every alias in the project, where `legacy_project_id = hex(sha256(current_path))`; an orphan whose ID matches proves the alias-holder is its twin. Fallback proof: exact title match against an alias-holder within the June-24 event cluster — recorded in the manifest as `content-identity`, distinctly from `derivation`.
- **Unproven:** orphans with neither proof are listed, refused by default, and require explicit per-row operator disposition supplied as repeatable apply flags — `--retire <entity-id>` and `--realias <entity-id>=<alias>` — recorded verbatim in the manifest. No disposition, no touch.
- **Dangling aliases** are deleted when they are dead: the entity row is missing *and* nothing in the project still names that entity. An alias the importer forward-declares for a referenced-but-unimported artifact (a `depends_on` naming a task with no file) keeps a live relationship edge and is a reference, not damage — the detector and the repair both pass over it, or import → repair → import never converges. The edge goes when the reference leaves the markdown, and the alias it left behind is then collected. (Refinement discovered in review: the production `[]` alias is dead by exactly this test.)
- **Orphaned sources:** `sources` rows referenced only by retired rows retire with them.
- **Named dispositions:** special-cased rows (the broken-evidence report) carry their disposition in the plan and manifest.

Retirement reuses the reference-table sweep from `spec_delete.go:90-132` (artifact bodies, FTS, events, entity_tags, bundle_members, backend_mappings, exports, relationships, then the row) under `PRAGMA defer_foreign_keys = ON`, generalized across entity kinds. Events written against retired rows are deleted with them, consistent with `spec delete`; the manifest preserves every deleted row for rollback and audit.

The importer fix inverts identity resolution in `markdown_import.go`: before using a derived ID, look up `(project_id, namespace, alias)`; if the alias names an existing entity of that kind, use that entity's ID so `ON CONFLICT(id) DO UPDATE` fires. Regression test simulates the full historical sequence — import under one project ID, rekey, re-import — and asserts zero new entity rows and zero orphans.

The doctor diagnostic is read-only: for each project and entity table, compare raw counts to alias-joined counts, count dangling aliases, and report per-table parity. It detects; the migration repairs. No `--fix`.

### Idempotency and safety

The migration is re-runnable: a second preview after apply classifies zero orphans; a second apply is a no-op. Rides Recovery Tiers (ARCHITECTURE.md): mandatory backup, isolated preview, manifest rollback, post-apply verification. All tests isolate via temp DBs (`t.Setenv`/`LOAF_DB`); only the ceremony (TASK-004) touches the production database, deliberately.

### Journal duplicates

`loaf state migrate journal-duplicates` (TASK-005) repairs the unaliased half of the same event: pairs are identical `(entry_type, scope, message)` triples with one row in the June-13 import window and one in the June-24 reimport window (the same named window constants as the alias-orphans cluster gates); the June-24 row survives, ambiguous multi-candidate matches classify unproven and are refused without an explicit `--retire`. The full plan lives in the TASK-005 packet.

### Sequencing

TASK-001 (migration) → TASK-002 (importer), TASK-003 (doctor), and TASK-005 (journal duplicates) are independent of each other; TASK-004 (ceremony) is blocked by all four — it runs the shipped code against the production database and uses the doctor check as its verification surface.

## Implementation Units

- **TASK-001 — Alias-orphan repair migration.** `loaf state migrate alias-orphans` preview/apply/rollback with classification, twin proofs, reference-table sweep, named dispositions, manifest, and tests.
- **TASK-002 — Importer alias-first identity resolution.** Markdown import resolves aliases before deriving IDs; simulated-rekey regression test.
- **TASK-003 — Doctor alias-parity diagnostic.** Read-only per-project, per-table parity section in `loaf state doctor`, with tests.
- **TASK-004 — Production repair ceremony.** Backup, rehearsed preview with dispositions, alias-orphans apply, journal-duplicates apply, doctor verification, lifecycle-statuses run, count-agreement receipts, journal entries.
- **TASK-005 — Journal-duplicates repair migration.** `loaf state migrate journal-duplicates` on the same triad: window-gated natural-key pairing, June-13-copy retirement with reference sweep and FTS parity, refuse-by-default ambiguity handling, and tests.

## Verification Contract

- **V1.** Migration classification, apply, rollback, and idempotency tests pass. Command: `go test ./internal/state -run 'AliasOrphan' -count=1`. Expect: exit 0.
- **V2.** Importer resolves identity through aliases; simulated rekey + re-import creates zero new rows. Command: `go test ./internal/state -run 'ImportAliasFirst' -count=1`. Expect: exit 0.
- **V3.** Doctor reports alias parity and dangling aliases. Command: `go test ./... -run 'AliasParity' -count=1`. Expect: exit 0.
- **V4.** The whole suite stays green. Command: `go test ./...`. Expect: exit 0.
- **V5.** Journal-duplicates pairing, refusal, apply/rollback, and FTS-parity tests pass. Command: `go test ./internal/state -run 'JournalDuplicate' -count=1`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** Ceremony receipts: backup ID, preview output for all projects, apply manifest path, post-apply doctor parity green, scanner-vs-list equality for all seven aliased tables, lifecycle-statuses manifest, journal entries.
- **H2.** The broken-evidence report row is archived with its moot-rationale event; the unrecoverable evidence is documented, not fabricated.
- **H3.** The three unproven task orphans (66 orphans vs 63 title twins) received explicit manifest-recorded dispositions.

## Definition of Done

- V1–V5 green in CI.
- On the production database: for every project and every entity table, raw row counts equal alias-reachable counts, and zero dead aliases remain (doctor parity green).
- Housekeeping scanner counts equal canonical list counts — the brief's acceptance signal.
- Zero `(entry_type, scope, message)` journal twins remain across the June-13/June-24 import windows.
- The broken-evidence report is archived with recorded rationale.
- Backup and rollback manifests retained per Recovery Tiers; ceremony receipts journaled.

## Durable Outputs

- ADR candidate: entity identity lives in the aliases table; derived entity IDs are mint-once opaque keys, never recomputed for resolution. (Supersedes the implicit stable-derivation assumption that caused this event.)
- CHANGELOG entry for the 0.2.x line covering the repair migration, importer fix, and doctor diagnostic.
- Post-ceremony journal synthesis: final per-table counts, dispositions of unproven rows, lifecycle-statuses OOV leftovers if any.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. Tags are convention, never parsed by check. -->

- [KU] Do the other 26 projects carry alias-orphans, and does each have a recomputable legacy ID (`sha256(current_path)`)? → **Resolved by the production-copy rehearsal:** all 191 orphans belong to this project; post-apply doctor reports alias-parity clear across all 27 projects and 189 table checks. The other projects carry no alias-orphan damage.
- [KU] What are the three task orphans without title twins? → TASK-001 preview classifies them as unproven; operator dispositions in TASK-004, manifest-recorded.
- [KU] Which out-of-vocabulary free-text statuses can the lifecycle-statuses migration not map? → surfaced by its preview in TASK-004; handling recorded in ceremony receipts; any needed set-status verb routes to TASK-408, not this Change.
- [KU] How many of the ~1,020 journal twin pairs are ambiguous (multi-candidate) and need explicit `--retire` dispositions? → **Resolved by the production-copy rehearsal:** 1,019 duplicated triples decompose into 866 clean 1:1 pairs (retired automatically) and 153 ambiguous groups spanning 614 rows, which refuse by default. The Definition of Done's zero-twins line is reachable only through ceremony dispositions — scriptable from the preview JSON — not a bare apply.
