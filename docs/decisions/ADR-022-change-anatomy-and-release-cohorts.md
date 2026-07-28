---
id: ADR-022
title: "Change anatomy — role-named narrative, task-file state, and release cohorts via target_release"
status: Accepted
date: 2026-07-28
supersedes: null
superseded_by: null
---

# ADR-022: Change anatomy and release cohorts

## Context

The bounded-work unit was one `docs/changes/YYYYMMDD-slug/change.md` serving three lives at once — the pitch, the plan, and the machine surface (frontmatter carrying `lineage`, `predecessor`, `release-after`). Four failures followed, one of them proven mechanically: the release gate's satisfaction test was node presence in HEAD plus structural executability, which is precisely what shaping produces, so on 2026-07-25 an isolated worktree took `loaf release --dry-run --bump minor` from blocked to a complete stable plan by merging a single shaping commit with zero lines of the promised work implemented. Shaping edits and execution edits were indistinguishable in history; the document conflated roles the agentic corpus keeps separate (spec, plan, brief); and nothing supported tracker parity or stable task reference, because the retired `SPEC-XXX` counter could not return — a global sequence needs a mint authority, and a branch-native, offline-capable, multi-worktree model deliberately has none.

The lineage graph (`arc`/`previous`/`terminal` chains between changes) collapsed under review: forward pointers forced mutating committed nodes, frozen designations fought roadmap reality, and the terminal-change proxy hung the release on a designated node rather than on the release itself. A corpus debate (Shape Up, Linear Method, SDD, OpenSpec) settled the naming: the bucket must be the word with no reserved inner meaning, and "Change" is it — a Plan containing a `plan.md` makes "update the plan" unresolvable.

## Decision

**Anatomy.** A change is a folder `docs/changes/YYYYMMDD-slug/` containing `change.json` (identity: `change`, `created`, `branch`, plus optional `target_release`), role-named narrative documents — `shape.md` required (the contract: problem, scope, decisions, verification criteria), `brief.md` optional (archeological pre-shaping ask, never updated after shaping, never mechanically load-bearing), `plan.md`/`design.md` optional and accretive (the technical route) — and `tasks/TASK-NNN-slug.md` state files. Change-scoped artifacts live inside the folder: `research/` (inputs to shaping) and `reports/` (authored outputs named `YYYYMMDD-HHMMSS-<kind>-<slug>.html` from a closed kind registry shipped in the binary: ceremony kinds `approval`/`review`, informational `visual`/`audit`/`note`).

**Narrative/state split.** Narrative documents settle at shaping; `tasks/` mutates during execution. This boundary is evidentiary, not stylistic: which files change when is what provenance rules read, so prose is shaped and state is executed. Shape edits during execution remain legal but consequential — criteria changes expire receipts and appear as reviewable diffs.

**Task files.** `TASK-NNN-slug.md` is numbered-record identity local to its owning change; `slug/NNN` is the external reference; no global counter exists and none may be minted. Frontmatter relations use the tracker-native closed set — `parent` (containment, declared on the child), `blocks`/`blocked-by` (sequencing, inverses derived), `relates-to` — with cross-change relations forbidden. Each file is a self-sufficient delegation packet: objective, scope boundaries, context pointers, acquisition instructions, checkbox steps, task-level verification. Completion is derived from checkbox state, never stored. A task is one atomic commit's worth of work; PRs are integration batches.

**Release cohorts.** A change optionally declares `target_release` in one canonical literal form (`MAJOR.MINOR.PATCH`, no `v`, no leading zeros, no prerelease or build suffix). The cohort — all changes sharing a target — *is* the arc: derived, never declared as a graph. Cutting stable `X.Y.Z` requires every cohort member materialized, structurally valid, executed at flip grade, and receipt-verified (ADR-023); prerelease candidates bypass; `--bump release` always gates, and `--post-merge` keys on the prepared version at HEAD — a prepared prerelease publishes through the valve, a prepared stable gates that version's cohort, and the tag always equals the version files. The field is mutable by design: a retarget, including removal, is a visible reviewable roadmap decision surfaced from history and at preflight, never blocked. Changes without a target never gate anything and carry zero verification ceremony.

**Commitments.** No status-like fields anywhere — `change.json` and task frontmatter are closed schemas that reject `status`, `readiness`, `state`, `completion`, `done`, `assignee`, `estimate`, `priority`; readiness and completion are always derived (the display ladder: captured → shaped → executable → executing → complete, plus verified for cohort members). No forward pointers between changes — they force mutating committed nodes. No local ID minting — slug is identity, tracker IDs (PR numbers, Linear keys) are shorthand and sync linkage, and the PR set is derived from squash subjects rather than stored.

**Coexistence and conversion.** The loader detects the new layout by `change.json` presence and falls back to legacy single-file `change.md` in the same root; malformed `change.json` fails closed. Both layouts are first-class until a named removal boundary. Sanctioned conversion is an atomic same-commit replace — `change.json` values carried verbatim, `change.md` retired, all task checkboxes unchecked (pre-checked boxes are a check violation, because flip-grade execution must come from later delivering commits). Retention is unit-keyed (slug + folder identity) with mutable-event replay across both surfaces, replacing the path-keyed deletion detection and immutable-field freeze replay; deleting or renaming away any unit whose history ever declared a target (or legacy `lineage`/`release-after`) is blocked.

**Lifecycle.** The change PR is the change's whole life: draft as the shaping anchor, implementation commits, pre-merge reflect, merged only when everything is done — so main carries only completed changes. The sole sanctioned early merge is a captured promise carrier (brief + `change.json` with a target) hard-binding a release to future work.

## Consequences

Execution evidence became machine-derivable along the true boundary, closing the proven shaping-only-merge defect — replayed against the shipped gate, the 2026-07-25 attack now blocks with `targets 2.0.0 but is not executed`. Tracker parity gets a coordination-free identity scheme (change → project or tracking issue, tasks → sub-issues, relations mapping natively) without reintroducing a counter. The roadmap is a projection of cohort state, never a planner: no milestone entities, no date fields.

Costs accepted: legacy cohort members must convert before they can verify (prose criteria cannot execute), and one dogfood wrinkle is grandfathered — the conversion commit `acbea950` was simultaneously TASK-003's delivering commit, so its boxes landed checked; the exemption is pinned by commit hash with `INTENT-20260727-dogfood-conversion-manufactured-task-003-execution` tracking the cleanup. The removal boundary for the legacy layout is owed at the first stable release after the new layout has shipped one minor, recorded in the deprecation notice.

Provenance: `docs/changes/20260726-change-work-model/` (shape.md Decisions 3–12 and 16–17; review boards `reports/20260727-160756-review-codex-r1.html`, `…-192707-review-codex-r2.html`, `…-201619-review-codex-r3.html`, `…-234403-review-implementation-round-1.html`, `20260728-021901-review-implementation-round-2.html`), PR #141, journal `decision(work-model)` 2026-07-25→28.
