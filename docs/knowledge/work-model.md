---
topics:
  - changes
  - tasks
  - releases
  - workflow
covers:
  - docs/changes/**/*
  - internal/cli/change_*.go
  - content/skills/pitch/**/*
  - content/skills/shape/**/*
  - content/skills/implement/**/*
  - content/skills/triage/SKILL.md
  - content/skills/bootstrap/**/*
consumers:
  - implementer
  - reviewer
  - researcher
last_reviewed: '2026-07-30'
---

# The Work Model

## Contents
- The Unit: a Change
- The Documents
- Tasks: Vertical Slices, Committed as Evidence
- Derived States
- Releases: Cohorts and the Gate
- The Pipeline
- Projections: Served, Never Committed

How bounded work moves through Loaf: what a Change is, which documents carry which role, how tasks become committed evidence, and how releases read that evidence. The ceremony front door and both-scale flow live in [loaf-flow.md](loaf-flow.md). Rationale lives in [ADR-022](../decisions/ADR-022-change-anatomy-and-release-cohorts.md) (anatomy, cohorts, commitments; brief semantics amended by [ADR-025](../decisions/ADR-025-entry-stage-pitch.md)), [ADR-023](../decisions/ADR-023-execution-provenance-and-cohort-receipts.md) (provenance, receipts, the gate), [ADR-024](../decisions/ADR-024-receipt-content-digest-freshness.md) (content-digest freshness), and [ADR-025](../decisions/ADR-025-entry-stage-pitch.md) (pitch entry stage); this document is the operating view.

## The Unit: a Change

A Change is a folder — `docs/changes/YYYYMMDD-slug/` — holding everything one bounded problem generates: identity (`change.json`), narrative, task packets, research inputs, and authored reports. The slug is the identity; `slug/NNN` references a task; PR numbers and tracker keys are shorthand, never identity.

**The problem boundary is the unit boundary.** Same problem, another task inside the change. A different problem discovered mid-work becomes an Intent and its own change — never `TASK-N+1`. This is the rule that keeps a change from becoming a twenty-PR mega-unit.

**One change, one PR.** The PR is the change's whole life: draft anchors shaping, implementation commits flow in, reflect distills durable outputs on the branch, and it merges only when everything is done — main carries only completed changes. The sanctioned exception is a captured promise carrier: brief + `change.json` declaring a `target_release`, merged early to hard-bind a release to future work.

## The Documents

| Document | Role | Mutability |
|----------|------|------------|
| `change.json` | Machine identity + optional `target_release` | Identity fixed; target mutable (retargets are reviewable diffs) |
| `brief.md` | Authored problem-space document (from `/pitch` or a capture seed): problem, who, alternatives, value, constraints | Accretes parked problem-space concepts until shaping; freezes when `shape.md` exists; superseded by the contract; never mechanically load-bearing |
| `shape.md` | The contract: problem, scope, decisions, verification criteria | Settles at shaping; later edits are consequential (criteria changes expire receipts) |
| `plan.md` / `design.md` | The technical route, when the how needs prose | Accretive during shaping |
| `tasks/TASK-NNN-slug.md` | Execution state: delegation packets with checkboxes | Mutates during execution — the only surface that should |
| `research/` | Inputs to shaping: spikes, evidence, reaction artifacts (pitch landscape scans land here at change scale) | Settled with shaping |
| `reports/` | Authored outputs: `YYYYMMDD-HHMMSS-<kind>-<slug>.html`, closed kind registry (`approval`, `review`, `visual`, `audit`, `note`) | Snapshots — never auto-updated; stamp with `loaf change report new` |

Executable verification criteria in `shape.md` declare their command: `- **V1.** What must be true. Command: ` `` `exact command` `` `. Expect: exit 0.` (inline or as sub-bullets). Commands run from the repository root. H-tier entries stay prose and are review material, never gate input.

`Expect` is enforced with a deliberately minimal grammar: atoms join with ` and ` — `exit <N>` is the required exit code (an absent `Expect`, or one with no exit atom, means `exit 0`; a second `exit` atom is a contradiction and fails the criterion) and `` contains `text` `` requires the combined stdout+stderr to contain that backtick-delimited text (repeatable), as in ``Expect: exit 0 and contains `all green`.`` A criterion passes when its command ran, the exit code matched, and every `contains` matched, and the receipt records each atom with its outcome. Any other clause is unenforceable: `loaf change verify` warns naming the criterion and the clause and records it as advisory, never letting it affect the result — a declared expectation is either checked or loudly not.

## Tasks: Vertical Slices, Committed as Evidence

A task file is a self-sufficient delegation packet — objective, scope boundaries, context pointers, acquisition, checkbox steps, verification — complete enough to hand to a subagent as its entire brief. Relations use the closed set `parent`, `blocks`/`blocked-by`, `relates-to`, targeting `TASK-NNN` within the owning change only.

**A task is a commit, not a PR.** Land each task as an atomic commit that flips its checkboxes alongside the work; PRs batch task commits, and a squash still carries the flips in its diff. A pure coordination task's closing box rides any batch commit.

**Commit packets unchecked before executing them.** A packet that first lands already-checked in its delivering commit produces added-checked lines, not transitions — and transitions are what count as evidence. This is the one discipline the model cannot derive its way around.

**Descoping is legible only through verification.** An unchecked task on a verified change is descoped work, cut cleanly; completion displays derive from checkboxes, but doneness for release purposes is the criteria contract, never checkbox count.

## Derived States

Nothing stores a status; every state is computed from artifacts and history:

```
captured → shaped → executable → executing → complete
                                          ↘ verified   (cohort members only)
```

`captured` = brief-only. `shaped` = contract present, gaps remain. `executable` = `loaf change check` derives a complete implementation contract. `executing` = a commit touches `tasks/` plus a path outside `docs/changes/`. `complete` = every checkbox checked. `verified` = fresh receipt with all criteria passing — the only state the release gate reads, and it does not require `complete`. All surfaces (`list`, `show`, `check`, JSON) share one derivation.

`complete` and `executing` derive from the working tree while `verified` derives from committed HEAD — Decision 15 keeps checkboxes non-gating, so the split is deliberate.

## Releases: Cohorts and the Gate

Declaring `target_release: X.Y.Z` in `change.json` (canonical form — no `v`, no leading zeros, no prerelease) opts the change into the strong gate. The cohort — every change sharing that target — is the **target-version bucket**, derived rather than declared. Cutting stable `X.Y.Z` requires the whole cohort executed at flip grade and receipt-verified; one shaped-only member blocks the version. Prerelease candidates always flow. `--bump release` always gates; `--post-merge` keys on the prepared version at HEAD — a prepared prerelease publishes through the valve and tags exactly what the version files carry, while a prepared stable gates that version's cohort before tagging it.

The working loop for a cohort member: land the tasks → `loaf change verify <folder>` → commit `receipts/verify.json` → release. Block messages name their remedy — `structurally invalid (…)` and `not executable (contract gaps: …)` (fix what `loaf change check` reports; a brief-only member reads as `contract gaps: shape.md (missing)`), `not executed` (land a real flip commit), `receipt records failing criteria (V1)` (fix, re-verify, recommit), `content changed since verification` / `criteria changed (receipt expired)` (re-verify after content or criteria drift; an uncommitted receipt reads `not committed at HEAD`; receipts bind to a masked root-tree content digest — ADR-024 — so squash/rebase and cohort peers never machine-split the verdict), `legacy member — convert first` (sanctioned atomic conversion, boxes unchecked).

Slipping a change to a later release is a retarget: a reviewable `change.json` diff, surfaced at check and preflight, never blocked. The roadmap is this projection — cohorts and their derived states — not a planner.

## The Pipeline

```
pitch (problem-space brief) → shape (contract + packets) → implement (task commits) → ship (land the PR) → release (publish the version)
```

`/pitch` is the human front door: it authors a problem-space brief at change scale (`loaf change init <slug> --brief` + authored `brief.md`) or project scale (`docs/BRIEF.md` with `source: pitch`), then offers shape-now or park per the landing matrix below. `/shape` treats an existing brief as primary input (solution-space only) and promotes a capture-only folder in place via ordinary `loaf change init <slug>`; without a brief it still runs full narrowing — pitch is recommended, never mandatory. Implementation picks packets and flips boxes in delivering commits; `/ship` lands the change PR; `/release` publishes already-landed work and gates stable cuts on cohort receipts. Reflect distills durable outputs on the branch before merge; the journal carries decisions throughout. Reviews happen per round, each with its own board under `reports/` — authored snapshots, dispositions decided by the owner, accepted findings becoming task packets in the same change.

**Landing matrix (park and series-prep).** A parked capture *with* `target_release` commits docs-only on the default branch (the promise-carrier exception). An *untargeted* park commits docs-only on its slug branch or stays native intake (Intent/spark) — never on main. Series-prep at bootstrap mints the same way: one docs-only commit per capture, each bound to a coarse target when ready. Every landing is validated first with explicit-path `loaf change check <folder>` plus a direct `change.json` read-back of the intended target. Pitch and bootstrap prepare commits; push and PR stay human.

**Triage.** Intake items needing problem discovery hand to `/pitch`; well-understood directions hand to `/shape`. Capture-before-shape without a full pitch still uses `loaf change init <slug> --brief` and a seeded brief. Explore and brainstorm are agent-side techniques, not user-facing front doors — see [loaf-flow.md](loaf-flow.md).

## Projections: Served, Never Committed

Anything rendering live state — task indexes, gate status, cohort dashboards — is an on-demand CLI projection: `loaf change tasks --json` (stable-ID task index), `loaf change show` (state + derived PR set), `loaf change list [--target X.Y.Z]` (units and cohorts). Committed copies of derived state drift into lies; authored documents with data baked in at writing time are ordinary committed artifacts in `reports/`. The same doctrine governs the journal render and everything downstream.
