# Blindspot pass — execution consumers (implement, ship)

Read-only reviewer report, 2026-07-07. Questions: unknown knowns, interface gaps between Change-model input and current SPEC/task expectations, and deviation-logging anchor points. Findings verbatim; adjudication in `change.md`. These skills are OUT of this rewrite's scope — the findings are recorded for the sweep Change (which owns ship and the guidance sweep) and captured as sparks so they survive independently.

## Unknown knowns

- [unknown-known] implement/SKILL.md:36 — "You are the ORCHESTRATOR" assumes a fully-shaped, decomposed work item already exists upstream; never states where the work definition comes from, so under the Change model an agent won't know a change.md is the entry artifact rather than a SPEC/TASK.
- [unknown-known] implement/SKILL.md:54 — "Spec artifacts closed out on branch before PR creation" assumes the reader knows what a "spec artifact" is; the Change model writes durable specs at finalize (pilot D19, post-merge) and archives the folder post-merge (D2), so this precondition is silently wrong.
- [unknown-known] implement/SKILL.md:102 — "do not recreate .agents/TASKS.json after the SQLite cutover" references a migration event the reader is assumed to know.
- [unknown-known] ship/SKILL.md:145 — "git diff --exit-code -- dist plugins" bakes Loaf's own generated-artifact layout into a distributable skill.
- [unknown-known] ship/SKILL.md:132 — "use the project's review skill or read-only review flow" assumes such a skill exists and is known by name; does not tie back to pilot D15 review rounds.

## Interface gaps

- [interface-gap] implement/SKILL.md:86-96 — Input Detection recognizes only TASK-XXX / SPEC-XXX / Linear IDs; a `docs/changes/YYYYMMDD-slug/` path or change slug is unrecognized and falls through into ad-hoc "loaf task create" — an agent handed a Change would mint a bogus local task.
- [interface-gap] implement/SKILL.md:106-118 — Ad-hoc Task Auto-Creation would fabricate a task entity the Change model explicitly rejects (pilot Cut list; D22 no task entity).
- [interface-gap] implement/SKILL.md:92 — SPEC-XXX "resolve local tasks and build dependency waves" expects tracked tasks with depends_on; a Change carries Implementation Units in-document ("not tracked entities"), so the wave machinery has no input.
- [interface-gap] implement/references/batch-orchestration.md:24-33 — Batch resolution extracts "depends_on from each task" and validates task files exist; a change.md provides neither — the wave planner is inapplicable to Change-model input with no fallback described.
- [interface-gap] implement/SKILL.md:291-304 — No `loaf change check --require-executable` preflight anywhere; pilot V3 names this exact gate as "the implement-skill preflight." Should slot into the Startup Checklist before branch creation/spawning, gating on derived executability (D12).
- [interface-gap] implement/SKILL.md:308-326 — BEFORE/DURING never read the Change's Verification Contract as the source of what "done" means; acceptance evidence should be the change.md Verification Contract, not ad-hoc testing.
- [interface-gap] implement/SKILL.md:327-332 — AFTER close-out runs `loaf task update --status done` / `loaf task archive` / `loaf spec archive`; none apply to a Change — an agent handed a change.md would run loaf spec archive on a non-existent spec.
- [interface-gap] implement/SKILL.md:220-222 — "they decide whether to run /breakdown again or add an ad-hoc sub-issue" contradicts the D17 two-lane harvest rule: change-adjacent deferrals go into change.md Follow-ups/Out; abandonment-surviving work goes to spark capture.
- [interface-gap] implement/SKILL.md:357-360 — Related Skills describe shape as "Spec format and lifecycle" and breakdown as "Turning specs into tasks" — cross-references point at the retired model.
- [interface-gap] ship/SKILL.md:114-134 — Evidence Review flags "docs describe future work as already shipped"; a change.md is intentionally a plan and lives in-diff under docs/changes/, so this drift check false-positives on the Change itself. Ship must distinguish the Change (plan, expected in-diff) from durable specs (post-finalize).
- [interface-gap] ship/SKILL.md:136-151 — Step 3 runs generic "checks the project supports" rather than the Change's Verification Contract; ship can pass a PR whose contract criteria were never checked.
- [interface-gap] ship/SKILL.md:155-174 — Squash body derives from "reviewed diff and PR body" with no notion of change.md as the body source, and no check that handoff.md was removed pre-merge (pilot DoD makes a lingering handoff.md a merge blocker).

## Deviation anchors

- [deviation] implement/SKILL.md:317-322 — DURING logs spawns/outcomes and "keep journal entries handoff-ready" but never says "log deviations from the Change plan" nor writes them back into change.md; the writeback loop (pilot spike step: "write findings back into the Change's planning and verification sections") has no counterpart. Slot an implementation-notes/deviation step here.
- [deviation] implement/references/branch-and-completion.md:184-190 — Handoff Readiness ("log what just happened after every significant action") is the natural home for plan-deviation logging; journal types decision/discover/block already exist, but the reference frames logging around agent-work outcomes, not plan-vs-actual divergence; needs an explicit deviation-entry convention.
- [deviation] ship/SKILL.md:125-129 — The drift check is where accumulated plan deviations would surface, but it treats drift as something to FIX to match the diff rather than harvest (D17); this is the reconciliation point where deviations logged during implement should be checked against the change.md contract. Harvest itself is sweep-owned; this is its interface anchor in ship.
