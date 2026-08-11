<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Spec Conversion and Guidance Sweep — Retiring the Legacy Work Surface

## Problem

The Change model is the bounded-work contract for new work, but the legacy spec/task workflow it replaced is still fully alive, not deprecated: `loaf spec` (9 subcommands) and `loaf task` (8) plus markdown-compat machinery span roughly 3,600 lines of `internal/cli/cli.go` and 3,800 lines of `internal/state/`, all with active write paths. `loaf init` still scaffolds `.agents/specs` and `.agents/tasks` into new projects, the fenced AGENTS.md block installed into user projects still advertises `loaf task/spec/kb`, and a shipping post-tool hook still runs `loaf task refresh` to regenerate a `TASKS.md` that no longer exists anywhere. The guidance contradicts itself: the shape skill documents breakdown as "retired" while the breakdown skill ships wholesale to every target, and `cmd/loaf/content_hygiene_test.go` pins the compatibility stance as exact substrings across 12 files, making the contradiction load-bearing.

Live work intent is trapped in the legacy stores: Loaf's own database holds 15 open specs and 17 open tasks (several naming real unresolved problems, such as TASK-406's secrets-indexed-in-FTS finding), and `.agents/specs/` holds 24 top-level SPEC files with no marker linking any of them to the Change model. There is no conversion mechanism of any kind — "deliberately converted" is aspirational prose.

This Change is the terminal carrier of the `change-model-hard-cut` lineage promise, materialized by change-work-model TASK-004 and unpinned from `0.3.0` by the ADR-026 arc-boundary revision (2026-08-10, the first pin-late application).

## Hypothesis

If the public spec/task/breakdown surface is hard-removed, trapped work intent migrates mechanically into the intake queue, and every live and generated guidance surface converges on the Change-first flow, then no conversation can be routed into a retired workflow, guidance stops contradicting itself and the code, and no open work goes dark — completing the arc the lineage promised, as the sweep's own X-bump release.

## Scope

**In**

- A mechanical `loaf migrate work-records` migration: open/draft specs (each carrying its open tasks) and orphan open tasks become Intents preserving full text and provenance; completed/archived rows stay quarantined; a migrate-on-open nudge gates projects holding open legacy rows.
- Removal of the `loaf spec` and `loaf task` command surfaces, the markdown-compat machinery (including the `loaf migrate markdown` spec/task importers), and their argument parsers and help surfaces.
- State-layer quarantine: legacy writers and readers removed, the migration becomes the tables' only reader; entity registry, lifecycle vocabulary, export, housekeeping, render-sweep, and the transitional-tasks digest layer converge.
- Init/install/hooks convergence: no legacy scaffolding, fenced-block text updated, the `generate-task-board` hook removed, an install-deprecation entry retires the breakdown skill on installed harnesses.
- Skills convergence: the breakdown skill deleted; every referencing skill, template, agent profile, and hook instruction rewritten; the CLI reference regenerated.
- Docs convergence: README, AGENTS.md, ARCHITECTURE, STRATEGY, VISION, PR template, knowledge files, and schema docs; dated in-place revision notes on ADR-013 and ADR-016 under the living-record convention.
- The hygiene gate flipped: `TestPlanningVocabularyConverged` pins the new stance — legacy workflow references forbidden outside historical evidence.
- Dogfood: Loaf's own migration runs, the 24 top-level SPEC files move to archive, and `loaf change verify` writes the receipt.

**Out** (deferred, not rejected)

- Physical deletion of the `specs`/`tasks` schemas — the zero-row cleanup remains a later Change, unlocked once no project retains rows.
- The fate of legacy-layout `change.md` folders — owned by the removal boundary named in change-work-model, not this sweep.
- `linear-native-coordination` — the predecessor this sweep now sequences behind; captured at `docs/changes/20260811-linear-native-coordination/` with the operator mandate that Linear's ontology informs the internal model. It owns ADR-011's supersession.
- A lighter-than-Change work container ("loose plans") — captured as a spark for its own future pitch; vocabulary collision with Change-internal `plan.md` noted.
- `suggestReleaseBump` realignment to arc evidence — named pending by ADR-026, not this sweep's business.
- Migrating GridSight, mvault, dots, or any other project's rows — the mechanism ships here; their runs happen when those projects next open. Triage of the migrated intake items likewise happens later, in the queue.

**Cut** (explicitly rejected)

- Public read-only compatibility commands — inherited stub no-go, reaffirmed at interview.
- Machine-authored briefs: the migration never writes into `docs/changes/` — SQLite is the unattended landing zone; the authored surface stays human.
- Global deletion of legacy rows while any project retains them.
- A broad skill-quality or routing audit riding along with the guidance rewrite.
- Implementing from the parked `59fbcdcf` draft without revalidation against current main.

## Observable Workflow

`loaf spec` and `loaf task` are unknown commands; root help, agent help, and the generated CLI reference carry no trace of them. Opening a project whose database holds open legacy rows produces a migration nudge naming `loaf migrate work-records`; running it reports each converted record, and `loaf intake list` shows the resulting Intents with their origin provenance. At triage, an item still wanted is promoted with `loaf change init <slug> --brief` — the same promotion path every capture uses. `loaf init` scaffolds no `.agents/specs` or `.agents/tasks`. After `loaf upgrade`, no harness offers a breakdown command, and the installed fenced block no longer advertises the retired surface. Historical evidence — archived SPEC files, old Changes, the changelog, journal history — still reads naturally with its legacy vocabulary intact.

## Rabbit Holes and No-Gos

- **The 792-line test file.** `internal/cli/cli_test.go`'s legacy coverage is remediation, not redesign — delete and replace with refusal tests; do not refactor the surviving suite's structure while in there.
- **Entity-registry ripples.** Journal entries, findings, and git history reference `SPEC-*`/`TASK-*` ids as text; those references stay as text. Only resolution machinery goes — do not chase historical mentions.
- **Guidance rewrite scope creep.** Each skill edit removes/redirects legacy references only; improving unrelated prose in the same files is the audit this Change cuts.
- **The removal boundary.** Legacy `change.md` folders surface constantly during this work (11 show `state=executing` forever in `loaf change list`); their fate is a separately named decision — resist folding it in.
- **Quarantine is not cleanup.** No schema drops, no row deletion, no `0001_initial.sql` edits beyond what the migration itself requires.

## Decisions

Provenance: shaping interview 2026-08-11 (four grilling rounds, recorded in this session's journal entries); inherited decisions from the `journal-reliability-foundation` terminal stub; the ADR-026 arc-boundary revision and its unpin commit (`8eac0210`).

1. **Full hard cut, no read-only compat.** The inherited stub decision reaffirmed at interview: the public surface disappears entirely; the migration is the tables' only remaining reader. Forecloses a demoted `list/show` surface and the maintenance tail it would carry.
2. **Tables quarantine physically; the zero-row cleanup is a later Change.** Inherited: no global deletion while any project retains rows.
3. **Hybrid disposition.** The migration is mechanical and unattended — open/draft specs (with their open tasks folded in) and orphan open tasks become Intents with full text and provenance; promotion to a brief is a human act at triage via `loaf change init --brief`. The boundary is ADR-016's: SQLite is the safe unattended landing zone, `docs/changes/` is an authored surface where everything implies a human chose it.
4. **Markdown spec files are never trapped.** Projects in markdown-compat mode keep their readable `.md` files in Git; the migration reads SQLite rows only and never ingests markdown. Files, unlike rows, need no liberation mechanism.
5. **The strong gate is retired; the promise survives as arc semantics.** The brief's "target_release: 0.3.0 / stable cannot cut" framing predates the ADR-026 revision and its unpin. The sweep stays unpinned per pin-late discipline: when executed it derives as an arc of one and bumps X at its own cut (stated via explicit `--bump` until the suggestion realignment lands). Execution still ends with `loaf change verify`, so the receipt exists with nothing demanding it.
6. **Linear designs first; the sweep executes after.** Operator directive: Linear-native mode is critical and its ontology informs the internal model. `linear-native-coordination` (captured 2026-08-11) lands before this sweep executes and owns ADR-011's supersession; this restores the recorded 2026-07-17 successor order. Implement's preflight re-checks whether the landed Linear model shifts any convergence target here.
7. **No new work container.** The "loose plan" idea is a spark for a future pitch, not a disposition target — the Change already scales down to zero ceremony unpinned, and below it the journal+commit path needs no container.
8. **ADRs follow the living-record convention.** ADR-013 and ADR-016 get dated in-place revision notes where the artifact-kind lists and SPEC framing go stale; ADR-011 is not this Change's to touch (Decision 6). No new ADR: the hard cut is the execution of decisions ADR-022 and this lineage already recorded, and reversal would be a code change, not a lost rationale.
9. **The hygiene gate flips inside the sweep.** `TestPlanningVocabularyConverged` is rewritten to pin the new stance — legacy workflow references forbidden on current-guidance surfaces, required convergence sentences asserted — so the stance change is atomic and enforced from the first commit after the flip.
10. **`loaf kb` is not legacy.** The knowledge-base/glossary surface stays untouched.
11. **Historical evidence keeps its vocabulary.** Inherited: `docs/changes/`, `.agents/specs/archive/`, CHANGELOG, ADR history, and journal renders retain SPEC/TASK mentions; enforcement scopes to current-guidance surfaces only.

## Planning Contract

### Approach

Removal proceeds data-path-first: the migration lands before anything that reads or writes legacy state disappears, so no window exists where rows are unreachable. CLI and state removal follow as two slices, then the install/init surface, then content and docs convergence, then the gate flip, then the dogfood run. Every content-touching commit rebuilds and commits the `dist/` + `plugins/` mirrors alongside sources per house rules, and every commit touching a hygiene-pinned file updates the pinned strings in the same commit so the suite never reds between slices.

### Migration semantics

`loaf migrate work-records` converts, per project: each open/draft spec row to one Intent carrying the spec body verbatim plus its open task rows as structured content; each open task without a parent spec to its own Intent; provenance fields record origin ids and timestamps. Completed and archived rows are untouched. The operation is idempotent by operation key — re-running creates nothing new (precedent: the deferred-intent capture from journal-reliability-foundation). The nudge follows the ADR-013 worktree-storage pattern but triggers only when open/draft rows exist — work that would go dark; projects holding only completed rows quarantine silently with no ceremony. Nudge wording, exempt commands, and exit code are TASK-001's to fix.

### Sequencing

This Change executes after `linear-native-coordination` lands (Decision 6) — implement's preflight confirms it and re-checks convergence targets against the landed Linear model. Internally: TASK-001 precedes TASK-002, which precedes TASK-003; TASK-004 follows TASK-002; TASK-005 and TASK-006 can run in parallel after TASK-004; TASK-007 requires both; TASK-008 runs last and writes the receipt.

### Risks

The `cli_test.go` remediation is the largest single hazard — 792 matching lines interleaved with surviving coverage; the refusal-test replacement must not silently drop unrelated assertions. Entity-registry removal ripples into `trace`, `link`, and alias machinery; the boundary is Decision text-vs-resolution (Rabbit Holes). Installed harnesses carry stale guidance until `loaf upgrade` runs; install markers and the deprecation entry make the drift visible rather than silent. The receipt-staleness warnings currently shown by `loaf change list` for older cohort members are pre-existing noise, not this Change's regression.

## Implementation Units

Ordered by likelihood-of-change; packets in `tasks/`.

- **TASK-001 — Rows-to-intake migration.** `loaf migrate work-records`, the open-rows nudge, idempotency, round-trip tests.
- **TASK-002 — CLI surface removal.** `runSpec`/`runTask`, markdown-compat machinery, parsers, help surfaces, `cli_test.go` remediation, refusal tests.
- **TASK-003 — State-layer quarantine.** Legacy writers/readers, entity registry, lifecycle vocabulary, export/housekeeping/render paths, transitional-tasks digest layer, schema docs.
- **TASK-004 — Init, install, and hooks convergence.** Init scaffolding, fenced block, hook catalog, breakdown deprecation entry.
- **TASK-005 — Skills convergence.** Breakdown deleted; ~25 referencing content files rewritten; CLI reference regenerated.
- **TASK-006 — Docs convergence.** Public docs, knowledge files, schema diagrams, ADR-013/016 revision notes.
- **TASK-007 — Hygiene gate flip.** `TestPlanningVocabularyConverged` pins the new stance; straggler sweep.
- **TASK-008 — Dogfood migration and archive.** Loaf's own run, SPEC files archived, `loaf change verify` receipt.

## Verification Contract

- **V1.** The full suite is green. Command: `go test ./...`. Expect: exit 0.
- **V2.** The legacy surface refuses. Command: `go test ./internal/cli -run TestLegacyWorkSurfaceRemoved -v`. Expect: exit 0 and contains `TestLegacyWorkSurfaceRemoved`.
- **V3.** Guidance pins the new stance. Command: `go test ./cmd/loaf -run TestPlanningVocabularyConverged -v`. Expect: exit 0 and contains `TestPlanningVocabularyConverged`.
- **V4.** Build and artifact parity hold. Command: `npm run build`. Expect: exit 0.
- **V5.** The migration round-trips. Command: `go test ./internal/state -run TestWorkRecordsMigration -v`. Expect: exit 0 and contains `TestWorkRecordsMigration`.

- **H1.** Migrated intake items read faithfully against their source rows — spot-checked at the first triage session that consumes them.
- **H2.** After `loaf upgrade`, no installed harness surface offers breakdown or advertises `loaf spec`/`loaf task`.

## Definition of Done

- All eight task packets' checkboxes flipped in their delivering commits.
- V1–V5 green through `loaf change verify` with the receipt committed.
- Loaf's own database holds zero open legacy rows; `.agents/specs/` top level holds only `archive/`.
- `dist/` and `plugins/` mirrors committed alongside every content-changing source commit.
- The X-bump cut itself is out of scope — releases decouple from merges (ADR-026); this Change is done when it is merged, verified, and receipt-carried.

## Durable Outputs

After execution proves what is true: the `docs/knowledge/task-system.md` record retitles/rescopes to the post-sweep work model (or folds into `work-model.md`); CHANGELOG carries the removal narrative at the arc cut; no new ADR anticipated (Decision 8) — if implementation surfaces a constraint passing the reversal test, promote it at reflect time.

## Open Questions

- [KU] Exact new-stance strings and forbidden-pattern scoping for the flipped hygiene gate → TASK-007.
- [KU] Which of the 24 top-level SPEC files hold unharvested content deserving an Intent beyond the mechanical migration → TASK-008 review step.
- [KU] Nudge wording, exempt-command list, and exit code for the migrate-on-open gate → TASK-001.
- [KU] Whether the landed `linear-native-coordination` model shifts any convergence target here → implement preflight, per Decision 6.
