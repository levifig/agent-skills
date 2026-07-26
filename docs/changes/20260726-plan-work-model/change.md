---
change: plan-work-model
created: 2026-07-26
branch: plan-work-model
---

<!-- Frontmatter must open the file at byte one — parsers depend on it. No status-like frontmatter (readiness/status/state): readiness is derived — a draft PR is shaping; `loaf change check` derives structural executability from the sections below. This unit itself ships in the old single-file layout: the tooling that validates the new one does not exist until T3, making this the last major unit shaped old-style. -->

# Plan Work Model — Narrative in plan.md, Execution in Task Files, Shipped-Not-Shaped Release Gating

## Problem

The work unit today is one `change.md` serving three lives — the pitch (shaping deliverable), the plan (units and verification), and the machine surface (frontmatter carrying `lineage`, `predecessor`, `release-after`). Four failures follow:

1. **The release gate cannot distinguish shaped from shipped.** The gate's satisfaction test for an arc terminal is node presence in HEAD plus structural executability — precisely what shaping produces. Proven 2026-07-25 in an isolated worktree: merging the sole shaping commit of terminal `spec-conversion-and-guidance-sweep` plus one missing frontmatter line takes `loaf release --dry-run --bump minor` from blocked to a complete stable 2.1.0 plan with zero lines of the conversion sweep implemented. Shape-first makes that collision the expected order of events, not an abuse.
2. **Shaping edits and execution edits are indistinguishable in history.** Every commit that modifies `change.md` looks the same, whether it refines scope or records delivered work, so any git-provenance rule keyed on the single file inherits the ambiguity.
3. **The unit's name does no discriminating work.** In a git repository everything is a change — a commit, a diff, unstaged edits, a PR — and the word names the output (a delta) rather than the intent. Register-testing alternatives exposed the same class of problem: bet/hypothesis language strains on shaped bugfixes, while "plan the fix" and "plan the feature" strain nowhere.
4. **Nothing in the structure supports tracker parity.** The unit maps naturally to a parent issue with sub-issues in Linear or GitHub, but prose sections give a sync adapter no stable task identity to target — one reason integrations still hardcode tracker calls.

Beneath these sits a work-model ambiguity this unit resolves: `journal-reliability-foundation` executed as one mega-unit across 20+ PRs because discovered problems were absorbed as units instead of spun out as Intents, and because nothing in the vocabulary carried the integration discipline.

## Hypothesis

Renaming the unit to **Plan** and splitting **narrative from state** — `plan.md` (pitch and contracts, written by shape, settles after shaping) plus `plan.json` (frozen machine metadata) plus `tasks/NNN-slug.md` (identified task files whose subtask checkboxes are checked in the commits that deliver the work) — makes execution evidence machine-derivable along the true boundary: prose is shaped, state is executed. The release gate can then require an arc terminal *executed* rather than merely materialized, closing the proven defect; stable task identity gives tracker sync a structural mapping (plan → parent issue, task files → sub-issues); and the vocabulary finally matches how the work is spoken about for every work type.

## Scope

**In**

- New unit layout: `docs/plans/YYYYMMDD-slug/` containing `plan.md`, `plan.json`, and `tasks/NNN-slug.md` files.
- `plan.json` schema: identity (`plan`, `created`, `branch`) plus arc metadata — `arc:`, `previous:`, `terminal:` (terminal declared once, on the arc root). No forward pointers: `next` is derived from the graph.
- `plan.md` carries the pitch and contracts: problem, hypothesis, scope, observable workflow, boundaries, decisions, approach, and the verification criteria. It is the parent-issue body under tracker sync.
- Task files: `NNN-slug.md` numbered-record identity inside the owning `tasks/` directory (the artifact-names guard's existing identity exemption), optional `depends-on` relations in frontmatter, subtask checkboxes in the body. Completion is derived from checkbox state — never stored.
- `loaf plan init` scaffolds the layout; `loaf plan check` validates both new and old layouts; `loaf plan tasks --json` projects a stable-ID task index on demand for integrations.
- CLI rename with progressive deprecation: `loaf change …` keeps working over both roots, announces the rename, and names its removal boundary; `loaf change init` scaffolds the new layout with a notice.
- Old-layout compatibility: single-file units under `docs/changes/` stay first-class readable and gate-relevant behind a deprecation notice, never rewritten in place; the immutability replay treats old and new metadata names as one field each across surfaces and history.
- Execution provenance: a plan counts as executed when at least one commit in HEAD ancestry modifies its `tasks/` files and also modifies at least one path outside the plan's folder. Old-layout fallback: keyed on `change.md`.
- Release gate rewrite: an arc terminal satisfies `terminal:` only when materialized, structurally valid, **and executed** per the provenance rule; the `--bump prerelease` bypass keeps working unchanged, including for the new failure mode.
- Terminal verification receipt: `loaf plan verify` executes the executable verification criteria and writes a committed receipt bound to a digest of `plan.md`'s verification section; editing the criteria expires the receipt. Required for arc terminals only — ordinary plans carry zero new ceremony.
- Skill and guidance sweep: `shape` produces a Plan and teaches the task discipline; `implement` picks task files and checks boxes in the delivering commits; vocabulary updated across templates, CLAUDE.md, and AGENTS.md.

**Out** (deferred, not rejected)

- `loaf web serve` and any HTML/GUI projection — its own Intent; the Kanban-style GUI consumes the same derived model through the CLI or a daemon.
- Committed `tasks.json` — revisit only if a sync adapter proves on-demand projection insufficient.
- Tracker sync adapters themselves (Linear/GitHub) — this unit builds the structure they consume; the mapping work is the SPEC-023 successor.
- `docs/specs/` as a current-truth corpus — reflect-owned, created or amended after implementation proves what is true, deliberately conservative to avoid staleness.
- Converting historical `docs/changes/` units — the deprecation window exists so they never have to convert; the removal boundary decides their fate.
- Arc retirement or supersession — `--bump prerelease` remains the only exit from an unfinished arc, deliberately.

**Cut** (explicitly rejected)

- Status-like fields anywhere — no `status:` in `plan.json`, no lifecycle in task frontmatter; completion and readiness are always derived.
- SQLite task entities or a mutable task lifecycle — task files are git-canonical; the retired task model stays retired. IDs return for parity; lifecycle does not.
- Forward pointers (`next:`) — they force mutating committed nodes, which the immutability replay exists to forbid.
- Committed rendered projections (`plan.html`, generated indexes) — rendered output is a projection, never a source surface, per the journal-render doctrine.
- A separate `shape.md` — the split is narrative vs state, not pitch vs plan prose; one settled document beats an editorial boundary between two.

## Observable Workflow

Shaping: `/shape` confirms scope, `loaf plan init <slug>` scaffolds `plan.md` + `plan.json` + `tasks/`, shaping fills `plan.md` and seeds the initial task decomposition. `loaf plan check` reports violations and derived executability.

Executing: an implementer picks `tasks/002-scaffold.md`, works it on a branch, and the PR that integrates it checks its boxes in the same commits that deliver the work — plan and work travel together, and that habit is what the gate later reads as execution provenance. A discovered new problem becomes an Intent, never task `007`.

Releasing: on a repo whose arc terminal is materialized but has no execution provenance, `loaf release --dry-run --bump minor` reports `release blocked: arc terminal plan "<slug>" is materialized but not executed`. After its tasks land and `loaf plan verify` writes a receipt matching the current criteria digest, the same command proceeds. `--bump prerelease` flows in both states.

Deprecation: `loaf plan check` (or the deprecated `loaf change check`) against an old single-file unit prints the familiar report plus one notice naming the new layout and the removal boundary.

Integration preview: `loaf plan tasks --json` emits the parent/children index — plan identity, task IDs, titles, relations, derived completion — shaped for a sync adapter or GUI to consume without parsing markdown.

## Rabbit Holes and No-Gos

- **`loaf plan check` is not a workflow engine.** It validates structure and derives executability; no progress percentages, no burn-down state, no checkbox analytics.
- **The receipt stays terminal-only.** Extending receipts to every plan is ceremony creep; the hash-binding pattern is proven but its scope is the arc terminal.
- **Do not redesign the arc graph.** Multiple-roots, cycle, and duplicate-slug validation carry over; only vocabulary and the file being read change.
- **Do not build the GUI or sync adapters "while we're here."** This unit ships the structure they consume, nothing more.
- **Task files do not grow toward tracker parity in git.** No assignee, estimate, priority, or state fields; parity beyond identity/relations/completion is the adapter's projection job.

## Decisions

Provenance: interviews and debate 2026-07-25 through 2026-07-27 (journal: `discover(release-gate)`, `decision(sequencing)`, `finding(work-model)`, `decision(work-model)`), the NotebookLM corpus debate against Shape Up, Linear Method, SDD, and OpenSpec sources, the gate defect proven mechanically in an isolated worktree, and register-testing candidate names across feature, bugfix, and refactor work.

1. **Split-first sequencing.** The release-gate fix is defined inside this unit rather than built twice. Forecloses shipping the gate fix independently against the old frontmatter.
2. **Git provenance is the ship-proof; the receipt is terminal exactness.** Provenance (task-file commit + outside paths) proves work landed for every arc node; the committed receipt proves the terminal's criteria were met. SQLite facts were rejected because the gate must hold on CI and other machines; slug-grep heuristics were rejected as trivially satisfiable.
3. **`arc:`/`previous:`/`terminal:`, no `next:`.** Backward pointers keep the graph append-only; `arc` stays explicit because multiple-roots validation depends on declared grouping.
4. **Plan replaces Change.** "Change" collides with git's everyday vocabulary and names the output rather than the intent; "bet" fails the register test on non-feature work; "plan" strains nowhere — "plan the fix" and "plan the feature" are both plain English — and the gate's own vocabulary ("materialized but not executed") was already plan language. Supersedes the earlier keep-Change decision. Loaf's Plan is also the durable replacement for harness-native throwaway plan modes.
5. **Narrative/state split, not pitch/plan prose split.** `plan.md` settles at shaping; `tasks/` mutates during execution. Supersedes the `shape.md`/`plan.md` design, whose boundary was editorial (which sentences are pitch?) rather than structural (which files change when?).
6. **Tasks are identified files with derived completion.** `NNN-slug.md` numbered-record identity, `depends-on` relations, subtask checkboxes checked in delivering commits. IDs return for tracker parity; a checkbox flip is committed evidence bound to the delivering commit, not a mutable assertion that rots.
7. **Task/subtask vocabulary; the slice survives as discipline.** Tracker-native names beat bespoke terms, and the law moves into the definition: a task must be a vertical slice — integrable alone, main stays shippable if the rest never lands.
8. **The task-vs-plan boundary is the problem boundary.** Same problem, another task; different problem discovered mid-work, an Intent and its own Plan. This is the rule `journal-reliability-foundation` broke.
9. **No arc exit besides prerelease.** An unfinished arc blocking stable is the incentive working, not a trap.
10. **Projections are served, never committed.** HTML views and JSON indexes are on-demand CLI projections (`loaf plan tasks --json` now, `loaf web serve` later), per the doctrine that rendered output is never a source surface.

## Planning Contract

### Approach

The rename and split land as a new scaffold plus a layout-agnostic loader, not a migration. The loader assembles the same in-memory node from either surface — `docs/plans/<folder>/plan.json` + `plan.md` + `tasks/`, or legacy `docs/changes/<folder>/change.md` — so graph derivation, arc validation, and the immutability replay stay single-sourced. The gate and receipt build on the loader's layout-agnostic view: provenance keys on the layout's execution surface (`tasks/` new, `change.md` old), so the gate closes for both layouts the day it ships.

### Placement

CLI work stays in `internal/cli`: the loader beside `change_lineage.go`, provenance with the gate, the `plan` command family beside the existing `change` dispatch (which becomes a deprecated alias), scaffold templates beside the existing change template. Skill updates live in `content/skills/shape` and `content/skills/implement`; guidance in `.agents/AGENTS.md` and `.claude/CLAUDE.md` follows shipped behavior, never ahead of it.

### Immutability across surfaces

`arc`/`previous`/`terminal` freeze at first non-empty value exactly as `lineage`/`predecessor`/`release-after` do today, and the replay treats old and new names as one field each across a layout conversion: a unit that declared `lineage: X` in `change.md` history and later carries `"arc": "X"` in `plan.json` is unchanged; `"arc": "Y"` is a violation. Empty-to-set stays legal, which is also the documented ceremony for an arc discovered late: the root gains `arc` and `terminal` when its first successor is shaped.

### Provenance precision

The path-level rule ships first: executed means a commit modifies `tasks/` and at least one path outside the plan folder. Scaffold commits are folder-local and can never read as execution. If mixed commits (a task check-off squashed with unrelated shaping edits elsewhere) prove noisy in practice, the named tightening is content-level detection — count only commits whose diff flips a checkbox — which is strictly stronger evidence at the cost of diff parsing.

### Compatibility and the removal boundary

Both roots are first-class inputs to every reader until a named removal boundary (candidate: first stable 2.x), recorded in the deprecation notice itself. Nothing rewrites an old unit in place. The live arc terminal `spec-conversion-and-guidance-sweep` remains old-layout and keeps gating through the `change.md`-keyed fallback; its residual shaping-edit ambiguity is accepted as strictly narrower than the presence-only defect being closed, with the terminal receipt still gating exactness.

### Risks

- The provenance convention requires execution to touch task files. The scaffold makes it structural (checkboxes live in `tasks/`), the implement skill instructs it, and the failure direction is safe: work that never touched its task file reads as unexecuted and blocks stable until a one-commit remedy, never a wrongly opened gate.
- Receipt expiry via criteria-digest drift is a known canary pattern that fired four times during capability work; `loaf plan verify` must report a digest mismatch plainly so re-verification is mechanical.
- Renaming a remote branch under an open PR closes the PR rather than retargeting it (learned on PR #139, closed by the `change-work-model` → `plan-work-model` rename); a mid-shaping rename therefore means superseding with a fresh PR, and folder rename, slug update, and branch rename must still move together with `loaf change check` green before and after.

### Sequencing

T1 and T2 are ordered (schema before scaffold); T3 depends on both; T4 and T5 depend on T3; T6 follows shipped behavior. Each task leaves main coherent: layout without gate keeps today's gate behavior; gate without receipt enforces provenance alone.

## Implementation Units

<!-- Tasks in the new vocabulary — in-document work packets and review anchors, not tracked entities. Each is a vertical slice: integrable alone, main stays shippable. -->

- **T1 — `plan.json` schema and layout-agnostic loader.** Parse the new surface, map `arc`/`previous`/`terminal` onto the graph model, read both roots and both layouts in every reader, and extend the immutability replay to treat old/new field names as one field across surfaces and history.
- **T2 — Scaffold and templates.** `loaf plan init` emits `plan.md` + `plan.json` + a seeded `tasks/`; templates carry the pitch-and-contract structure, the task-file format (`NNN-slug.md`, `depends-on`, subtask checkboxes), and the verification criteria section.
- **T3 — `loaf plan check` and the task projection.** Validation over both layouts with the deprecation notice naming the removal boundary; `loaf plan tasks --json` emits the stable-ID parent/children index; `loaf change …` becomes the deprecated alias.
- **T4 — Execution provenance and the gate rewrite.** Derive executed-ness for every arc node; terminal satisfaction requires materialized + structurally valid + executed; `release blocked` messages distinguish "not materialized" from "materialized but not executed"; prerelease bypass extended to the new failure.
- **T5 — Terminal receipt.** `loaf plan verify` runs executable criteria and writes the committed receipt bound to the criteria digest; the gate requires a current receipt for arc terminals.
- **T6 — Skills and guidance sweep.** `shape` produces a Plan and teaches the problem-boundary and vertical-slice tests; `implement` picks task files and checks boxes in delivering commits; vocabulary swept across templates and guidance.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** Regression of the proven defect: in a fixture repo whose arc terminal is materialized by folder-local commits only, `loaf release --dry-run --bump minor` exits non-zero with `materialized but not executed`; adding a commit that modifies a task file plus an outside path, and a current receipt, makes the same command succeed.
- **V2.** `loaf release --dry-run --bump prerelease` succeeds in every gate state above.
- **V3.** `loaf plan init` scaffolds the layout and `loaf plan check` reports zero violations on the fresh scaffold.
- **V4.** `loaf plan check` on an old-layout fixture passes with a deprecation notice; a fixture whose history moves `lineage: X` to `"arc": "Y"` across the layout boundary blocks release with an immutable-metadata finding.
- **V5.** `loaf plan verify` writes a receipt whose digest matches the current criteria; editing the criteria afterwards makes the gate report the receipt expired.
- **V6.** `loaf plan tasks --json` emits the plan identity and every task's ID, title, relations, and derived completion, stable across runs.
- **V7.** `go test ./...`, `npm run build`, and `loaf check` pass; rebuilt targets show zero drift.

<!-- Human review: -->

- **H1.** The scaffolded `plan.md` reads as one coherent pitch-and-contract, and a task file feels right mid-execution — confirmed by dogfooding this unit's own successors.
- **H2.** The deprecation notice names a real boundary and reads as an announcement, not a threat; old-layout users can tell exactly what to do and by when.

## Definition of Done

- T1–T6 integrated on main with V1–V7 green in CI.
- The proven shaping-only-merge attack from 2026-07-25, replayed against the shipped gate, stays blocked.
- A new plan shaped with the shipped scaffold passes `loaf plan check` with zero violations and no deprecation notice.
- Journal carries the ship decision; Durable Outputs below created via reflect after implementation proves them.

## Durable Outputs

- ADR: the Plan unit anatomy (`plan.md` + `plan.json` + `tasks/`), the arc vocabulary, and the no-status / no-forward-pointer commitments.
- ADR or knowledge doc: execution provenance and terminal receipts — what the gate reads, why git is the witness, receipt expiry semantics.
- Knowledge doc: the work model — plan as the unit, tasks as vertical slices, the problem-boundary test, when an arc exists, and the projection doctrine.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route — see the shape skill's quadrant table. Tags are convention, never parsed by check. -->

- [KU] Receipt file name and format → T5, decided against the capability-receipt precedent.
- [KU] Removal boundary for the old layout and the `loaf change` alias → named at T3 ship time, recorded in the notice; candidate: first stable 2.x.
- [KU] Whether `--require-executable` should demand full checkbox completion or provenance only → T4; leaning provenance-only, since completeness is the receipt's job for terminals.
- [KU] Task relation vocabulary in frontmatter → T2; start matrix-minimal with `depends-on` only.
- [UK] What the scaffolded `plan.md` and task-file structure feel like mid-execution → reaction artifact: draft the templates against this unit's own T1–T6 and react.

## Source Inputs

- Journal 2026-07-25: `discover(release-gate)` — the worktree proof that the gate opens on shaping alone; `decision(sequencing)` — split-first; `finding(work-model)` — the corpus-debate verdict.
- Journal 2026-07-26/27: `decision(shape)` — the unit shaped and renamed to change-work-model; `decision(work-model)` — Plan replaces Change with the narrative/state split.
- Conversation 2026-07-26/27: the register test failing bet/hypothesis language on shaped bugfixes; task IDs for tracker parity; the projection doctrine; `docs/specs/` deferred to reflect.
- Intents: `INTENT-20260725-split-the-change-unit-into-shape-md-plan-md-and-change-yml`, `INTENT-20260725-release-gate-must-require-the-terminal-change-shipped-not-merely-shaped`, `INTENT-20260725-progressive-deprecation-as-a-first-class-loaf-capability` (this unit dogfoods the pattern; the capability stays its own Intent).
- Prior art: OpenSpec change-as-delta and specs-as-truth; Shape Up pitches, scopes, and done-means-deployed; the Linear Method's small-integrations discipline; `internal/cli/target_capability_contract.go` for hash-bound receipts; the journal-render projection doctrine.
