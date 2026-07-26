---
change: change-work-model
created: 2026-07-26
branch: change-work-model
---

<!-- Frontmatter must open the file at byte one — parsers depend on it. No status-like frontmatter (readiness/status/state): readiness is derived — a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Change Work Model — Pitch, Plan, Slices, and Shipped-Not-Shaped Release Gating

## Problem

A Change today is one file serving three lives. `change.md` is simultaneously the pitch (the shaping deliverable), the plan (implementation units and verification contract), and the machine surface (frontmatter carrying `lineage`, `predecessor`, `release-after`). Three failures follow directly from that conflation:

1. **The release gate cannot distinguish shaped from shipped.** The gate's satisfaction test for an arc terminal is node presence in HEAD plus structural executability — which is precisely what shaping produces. Proven 2026-07-25 in an isolated worktree: merging the sole shaping commit of terminal `spec-conversion-and-guidance-sweep` plus one missing frontmatter line takes `loaf release --dry-run --bump minor` from blocked to a complete stable 2.1.0 plan with zero lines of the conversion sweep implemented. The mechanism that exists to prevent releasing mid-arc opens on the *promise* of the terminal, and shape-first makes that collision the expected order of events, not an abuse.
2. **Shaping edits and execution edits are indistinguishable in history.** Every commit that modifies `change.md` looks the same, whether it refines scope or checks off delivered work. Any git-provenance rule keyed on the single file inherits that ambiguity: a late scope tweak that incidentally touches one outside file reads as execution.
3. **Machine metadata hides in markdown frontmatter.** The immutability replay parses prose headers across full history, narrative edits can collide with frozen fields, and the vocabulary (`lineage`/`predecessor`/`release-after`) predates the settled arc model.

Beneath all three sits a work-model ambiguity this Change resolves in vocabulary: `journal-reliability-foundation` executed as one mega-Change across 20+ PRs because discovered problems were absorbed as units instead of spun out, and because "implementation unit" carries no integration discipline in its name.

## Hypothesis

Splitting the Change unit into `shape.md` (the pitch), `plan.md` (slices, their task checklists, and the verification contract), and `change.yml` (machine metadata under the `arc`/`previous`/`terminal` vocabulary) gives each life its own file. Execution evidence becomes machine-derivable — commits that modify `plan.md` while carrying work outside the folder are execution, commits that touch only `shape.md` are shaping — so the release gate can require an arc terminal *shipped* rather than merely materialized, closing the proven defect. Metadata moves out of prose into a file the immutability replay can watch directly, and the settled work model (Change = bet, slices integrate independently as PRs, tasks are checkboxes) gets carried by the artifact structure itself.

## Scope

**In**

- New Change layout: `docs/changes/YYYYMMDD-slug/` containing `shape.md`, `plan.md`, and `change.yml`.
- `change.yml` schema: identity (`change`, `created`, `branch`) plus arc metadata under the new vocabulary — `arc:` (was `lineage:`), `previous:` (was `predecessor:`), `terminal:` (was `release-after:`, declared once on the arc root). No forward pointers: `next` is derived from the graph.
- `loaf change init` scaffolds the new layout; `loaf change check` validates it (product contract sections in `shape.md`, planning contract and verification in `plan.md`, metadata in `change.yml`).
- Progressive deprecation of the single-file layout: old `change.md` Changes stay valid and checkable, emit a deprecation notice naming the removal boundary, and the immutability replay reads both surfaces for as long as both are accepted.
- Execution provenance derivation: a Change counts as executed when at least one commit in HEAD ancestry modifies its `plan.md` (old layout: `change.md`) and also modifies at least one path outside the Change's folder.
- Release gate rewrite: an arc terminal satisfies `terminal:` only when materialized, structurally executable, **and executed** per the provenance rule. The `--bump prerelease` bypass keeps working unchanged, including for the new failure mode.
- Terminal verification receipt: `loaf change verify` executes the executable criteria in the terminal's Verification Contract and writes a committed receipt bound to a digest of `plan.md`'s contract; editing the contract expires the receipt. The gate requires the receipt for arc terminals only — ordinary Changes carry zero new ceremony.
- Slice vocabulary in `plan.md`: slices are vertical, independently integrable, each ideally landing as one PR; tasks are markdown checkboxes inside a slice, never entities.
- Skill and guidance sweep: `shape` writes the new layout, `implement` reads `plan.md`, templates and `.claude/CLAUDE.md`/`AGENTS.md` guidance updated to the new vocabulary and the unit-vs-Change boundary test (same problem, another slice → slice; different problem discovered → Intent).

**Out** (deferred, not rejected)

- OpenSpec-style canonical `specs/` corpus with archive-time reconciliation of deltas into current truth — named future direction for the "current truth" gap; today Durable Outputs remains the reconciliation vehicle.
- Converting historical Changes to the new layout — the deprecation window exists so they never have to convert retroactively; the removal boundary decides their fate.
- Arc retirement or supersession mechanics — decided against for now; the prerelease bypass is the only exit from an unfinished arc, deliberately.
- SQLite mirroring of slice/checkbox state — git is the sole witness of execution.

**Cut** (explicitly rejected)

- Renaming the Change entity (to Pitch or anything else) — the folder is the bet and OpenSpec's "change as delta, resolved at completion" matches its mechanics; `shape.md` *is* the pitch.
- Any status-like field in any of the three files — readiness stays derived; a `verified:` or `done:` key is a status field wearing a hat.
- Task entities or a task command surface — checkboxes in a committed `plan.md` are the record.
- Forward pointers (`next:`) in `change.yml` — they force mutating committed nodes, which the immutability replay exists to forbid.

## Observable Workflow

Shaping: `/shape` confirms scope, `loaf change init <slug>` scaffolds `shape.md` + `plan.md` + `change.yml`, and shaping fills `shape.md` while `plan.md` holds slices and the verification contract. `loaf change check` reads all three and reports violations and derived executability exactly as today.

Executing: an implementer picks a slice, works it on a branch, and the PR that integrates the slice checks its boxes in `plan.md` in the same commits that deliver the work. That single habit — plan and work travel together — is what the gate later reads as execution provenance.

Releasing: on a repo whose arc terminal is materialized but has no execution provenance, `loaf release --bump minor` reports `release blocked: terminal "<slug>" is materialized but not executed`. After the terminal's slices land and `loaf change verify` writes a receipt matching the current contract digest, the same command proceeds. `--bump prerelease` flows in both states.

Deprecation: `loaf change check` against an old single-file Change prints the familiar report plus one notice: the layout is deprecated, the new layout is `shape.md`/`plan.md`/`change.yml`, and support ends at the named boundary.

## Rabbit Holes and No-Gos

- **Do not rebuild `loaf change check` into a workflow engine.** It validates structure and derives executability; it must not start interpreting checkbox counts as progress percentages or emitting burn-down state. Checkbox state is evidence for the gate, not a progress API.
- **Do not let the receipt grow into a general verification framework.** It reuses the hash-binding pattern already proven in the capability contract; it covers arc terminals only. Extending receipts to every Change is scope creep with real ceremony cost.
- **Do not redesign the arc graph.** Multiple-roots, cycle, and duplicate-slug validation carry over as-is; only the vocabulary and the file being read change.
- **Do not touch the SemVer inference or changelog machinery.** The gate decides *whether* stable may cut; everything downstream of that decision stays untouched.
- **Resist unifying with capability receipts.** Same pattern, different domain; a shared "receipt subsystem" abstraction is not worth its coupling.

## Decisions

Provenance: interview and debate 2026-07-25/26 (journal: sequencing decision, work-model finding), NotebookLM corpus debate against Shape Up, Linear Method, SDD, and OpenSpec sources; the gate defect proven mechanically in an isolated worktree.

1. **Split-first sequencing.** The release-gate fix is defined inside this Change rather than built twice — once against `change.md` frontmatter and again after the split. Forecloses shipping the gate fix independently first.
2. **Git provenance is the ship-proof, receipts are terminal-exactness.** Provenance (plan-file commit + outside paths) proves work landed for every node; the committed receipt proves the terminal's contract was met. SQLite facts were rejected because the gate must hold on CI and other machines; a slug-grep heuristic was rejected as trivially satisfiable.
3. **`arc:`/`previous:`/`terminal:` with no `next:`.** Backward pointers keep the graph append-only; forward pointers would require editing committed nodes. `arc:` stays explicit rather than derived because multiple-roots validation depends on declared grouping.
4. **Change stays Change; `shape.md` is the pitch.** In a git-canonical model the diff is native: the folder is the delta's description and contract, and the Change's merged PRs *are* the delta — OpenSpec needs prose delta-specs only because its truth corpus lives outside git. The lifecycle still matches (live while worked, resolved when its slices land), and Shape Up's pitch is the proposal document, not the executing container. Forecloses entity renames and their migration cost.
5. **Slices, not implementation units.** The name carries the discipline: vertical, independently integrable, main stays shippable if the rest is never built. A Change legitimately spans multiple PRs — one slice, one PR, ideally.
6. **The unit-vs-Change boundary is the problem boundary.** Same problem, another slice → slice. Different problem discovered mid-work → Intent, then its own Change. This is the rule `journal-reliability-foundation` broke.
7. **No arc exit besides prerelease.** An unfinished arc blocks stable by design; the pressure to finish the terminal is the incentive working, not a trap.
8. **Tasks are checkboxes.** A checked box in a committed `plan.md` is itself execution evidence; entities would resurrect the retired task model.

## Planning Contract

### Approach

The split lands as a new scaffold plus a compatibility reader, not a migration. `loaf change init` emits the three-file layout; the loader grows a second path that assembles the same in-memory `changeNode` from either layout, so the graph derivation, lineage validation, and immutability replay stay single-sourced. The gate rewrite and receipt build on the loader's layout-agnostic view: provenance keys on the layout's plan file (`plan.md` new, `change.md` old), so the gate closes for both layouts the day it ships.

### Placement

All CLI work stays in `internal/cli`: the loader beside `change_lineage.go`, the provenance derivation with the gate in `change_lineage.go`, receipt verb following the `change` subcommand dispatch, scaffold templates beside the existing change template. Skill updates live in `content/skills/shape` and `content/skills/implement`; guidance in `.agents/AGENTS.md` and `.claude/CLAUDE.md` follows the shipped behavior, not ahead of it.

### Immutability across the layout boundary

`arc`/`previous`/`terminal` freeze at first non-empty value exactly as `lineage`/`predecessor`/`release-after` do today, and the replay must treat the old and new names as one field each across a layout conversion: a Change that declared `lineage: X` in `change.md` history and later carries `arc: X` in `change.yml` is unchanged; carrying `arc: Y` is a violation. Empty-to-set stays legal, which is also the documented ceremony for an arc discovered at the second Change — the root gains `arc:` and `terminal:` then.

### Compatibility and the removal boundary

Old-layout Changes parse, validate, and gate exactly as today plus a deprecation notice. The removal boundary is a named release, decided at ship time and recorded in the deprecation notice itself; until then both layouts are first-class inputs to every reader. Nothing ever rewrites an old Change in place.

### Risks

- The provenance rule rests on the convention that execution touches the plan file. The scaffold makes the convention structural (checkboxes live in `plan.md`), the implement skill instructs it, and the false-negative direction is safe: a Change whose work never touched `plan.md` reads as unexecuted and blocks stable until the boxes are checked — an annoyance with a one-commit remedy, never a wrongly opened gate.
- The old-layout fallback (provenance keyed on `change.md`) keeps the shaping-edit ambiguity for old Changes, including the live arc terminal `spec-conversion-and-guidance-sweep` if it never converts. Accepted: the ambiguity is strictly narrower than today's defect (presence alone), and the terminal receipt still gates exactness.
- Receipt expiry via contract-digest drift is a known canary pattern that fired four times during capability work; the verify command must re-derive and report the digest mismatch plainly so re-verification is mechanical.

### Sequencing

S1 and S2 are ordered (schema before scaffold); S3 depends on both; S4 and S5 depend on S3; S6 follows behavior. Each slice leaves main coherent: layout without gate keeps today's gate behavior; gate without receipt enforces provenance alone for non-terminal nodes.

## Implementation Units

<!-- Slices: vertical, independently integrable, each ideally one PR. Tasks within a slice are commits, not entities. -->

- **S1 — `change.yml` schema and layout-agnostic loader.** Parse the new file, map `arc`/`previous`/`terminal` onto the graph model, accept both layouts in every reader, and extend the immutability replay to treat old/new field names as one field across layout history.
- **S2 — Scaffold and templates.** `loaf change init` emits `shape.md` + `plan.md` + `change.yml`; templates carry the pitch sections, the slice/checkbox structure, and the verification contract split.
- **S3 — `loaf change check` for the new layout.** Product contract read from `shape.md`, planning and verification contracts from `plan.md`, metadata from `change.yml`; deprecation notice on old-layout input naming the removal boundary.
- **S4 — Execution provenance and the gate rewrite.** Derive executed-ness (plan-file commit + outside paths) for every arc node; terminal satisfaction requires materialized + structurally executable + executed; prerelease bypass extended to the new failure; `release blocked` message distinguishes "not materialized" from "materialized but not executed".
- **S5 — Terminal receipt.** `loaf change verify` runs executable criteria, writes the committed receipt bound to the plan-file contract digest, and the gate requires a current receipt for arc terminals.
- **S6 — Skills and guidance sweep.** `shape` writes the new layout and teaches the slice/problem-boundary tests; `implement` reads `plan.md` and checks boxes in delivering commits; CLAUDE.md/AGENTS.md vocabulary updated.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** Regression of the proven defect: in a fixture repo whose arc terminal is materialized by shaping-only commits (no commit touches paths outside the terminal's folder), `loaf release --dry-run --bump minor` exits non-zero with `materialized but not executed`; adding a commit that modifies the terminal's plan file plus an outside path and a current receipt makes the same command succeed.
- **V2.** `loaf release --dry-run --bump prerelease` succeeds in every gate state above.
- **V3.** `loaf change init` scaffolds the three-file layout and `loaf change check` reports zero violations on the fresh scaffold.
- **V4.** `loaf change check` on an old-layout fixture passes with a deprecation notice; on a fixture whose history changes `lineage: X` to `arc: Y` across the layout boundary, the release preflight blocks with an immutable-metadata finding.
- **V5.** `loaf change verify` writes a receipt whose digest matches the current plan-file contract; editing the contract afterwards makes the gate report the receipt expired.
- **V6.** `go test ./...`, `npm run build`, and `loaf check` pass; rebuilt targets show zero drift.

<!-- Human review: -->

- **H1.** The scaffold's `shape.md` reads as a pitch and `plan.md` as a plan — the section split matches how shaping and implementing actually hand off, confirmed by dogfooding this very Change's successor work.
- **H2.** The deprecation notice names a real boundary and reads as an announcement, not a threat; old-layout users can tell exactly what to do and by when.

## Definition of Done

- All six slices integrated on main with V1–V6 green in CI.
- The proven shaping-only-merge attack from 2026-07-25, replayed against the shipped gate, stays blocked.
- A new Change shaped with the shipped scaffold passes `loaf change check` with zero violations and no deprecation notice.
- Journal carries the ship decision; Durable Outputs below created after implementation proves them.

## Durable Outputs

- ADR: the three-file Change unit and the arc vocabulary (`arc`/`previous`/`terminal`), including the no-forward-pointers and no-status-fields commitments.
- ADR or knowledge doc: execution provenance and terminal receipts — what the gate reads, why git is the witness, receipt expiry semantics.
- Knowledge doc: the work model — Change = bet, slices integrate as PRs, tasks are checkboxes, problem-boundary test, when an arc exists.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route — see the shape skill's quadrant table. Tags are convention, never parsed by check. -->

- [KU] Receipt file name and format (`verification.yml` in the Change folder vs `receipts/` subdir; yml vs json) → S5, decided at implementation against the capability-receipt precedent.
- [KU] Exact removal boundary for the old layout (which release) → decided at S3 ship time, recorded in the deprecation notice; candidate: first stable 2.x.
- [KU] Whether `loaf change check --require-executable` should also require every slice checkbox checked (full-plan execution) or provenance only → S4; leaning provenance-only, since checkbox completeness is the receipt's job for terminals.
- [UK] What the scaffolded `plan.md` slice/checkbox structure should feel like to an implementer mid-execution → reaction artifact: draft the template against this Change's own S1–S6 and react.

## Source Inputs

- Journal 2026-07-25: `discover(release-gate)` — the worktree proof that the gate opens on shaping alone; `decision(sequencing)` — split-first; `finding(work-model)` — the NotebookLM corpus debate verdict.
- Intents: `INTENT-20260725-split-the-change-unit-into-shape-md-plan-md-and-change-yml`, `INTENT-20260725-release-gate-must-require-the-terminal-change-shipped-not-merely-shaped`, `INTENT-20260725-progressive-deprecation-as-a-first-class-loaf-capability` (this Change dogfoods the deprecation pattern; the capability itself stays its own Intent).
- Conversation 2026-07-26: nomenclature debate settling Change/pitch/slices/checkboxes and the arc-creation semantics.
- Prior art: OpenSpec change-as-delta model; Shape Up pitches, scopes, and done-means-deployed; `internal/cli/target_capability_contract.go` for hash-bound receipts.
