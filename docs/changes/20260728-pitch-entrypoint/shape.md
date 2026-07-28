<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Pitch Entrypoint — Problem-Discovery Stage, Authored Briefs at Both Scales, Series-Prep at Bootstrap

## Problem

The Loaf Flow has no problem-discovery stage. The work model that shipped in change-work-model gave `brief.md` a seat in the role-named document set — "the original unshaped ask" — but no authoring ceremony, and the project-scale `docs/BRIEF.md` arrives only through bootstrap's own intake. Four failures follow:

1. **Shape's interview does double duty.** `/shape` absorbs raw vision directly, so problem discovery and implementation narrowing compete in one grilling session: either the problem framing gets shortchanged or shaping sprawls across both registers.
2. **Briefs are the weakest artifact in the model.** In practice a brief is a pasted spark — no problem statement, no affected users, no current alternatives, no value proposition — so "targeted shaping later" starts from almost nothing, and the brief/shape boundary is mechanical (pasted vs. authored) rather than semantic.
3. **The two scales enter through different doors with different quality.** A new project gets bootstrap's structured interview; a new change gets whatever triage pasted. Same discovery need, two disconnected mechanisms, no shared skeleton.
4. **The user-facing entry surface is fragmented.** Explore, brainstorm, idea, and shape all answer "where do I start" differently; brainstorm is already demoted to a technique yet still occupies the skill surface, and nothing owns the moment a human commits to working on a concept.

## Hypothesis

A single human-invoked entry ceremony — `/pitch` — that grills the problem space (problem, who has it, current alternatives and competitive landscape, value proposition, constraints) and authors a brief at the matching scale (change `brief.md` with change initialization, or project `docs/BRIEF.md` feeding bootstrap, one shared skeleton) makes the brief/shape boundary semantic: pitch owns problem-space, shape owns solution-space. `/shape` then starts from an already-framed problem and grills implementation only; bootstrap ends by decomposing the pitched BRIEF into a series of captured changes whose briefs act as intent for targeted shaping later; and the Loaf Flow gains one front door — pitch → shape → implement → ship → release — with explore and brainstorm demoted to agent-side techniques.

## Scope

**In**

- New `pitch` skill (workflow, verb-named): scale detection (existing project + concept → change scale; greenfield → project scale), a problem-discovery grilling protocol (one question at a time via `AskUserQuestion`, recommendation-first — shape's interview mechanics pointed at the problem register), applicability judgment for competitive analysis and value proposition (a bug fix pitch skips personas; a product pitch doesn't), and evidence delegation — a researcher subagent for competitive or landscape scans, evidence landing in the change's `research/` at change scale, inline source links in the BRIEF at project scale.
- Human entry only, as a cross-target invariant: mechanically guaranteed on Claude Code (`disable-model-invocation: true` in the sidecar) and held behaviorally on the other built targets — the skill's Critical Rules forbid agent-initiated pitching everywhere, with per-target mechanical exclusion arriving only when a target proves an equivalent surface (evidence-driven target support). Agents never open a pitch; agent legwork inside one (scans, series-prep mechanics) is delegated by the skill.
- Shared brief skeleton at both scales: problem statement, who has it, current alternatives, value proposition, constraints, sequencing and relationships (prose), sources and research links, open questions. Applied to three surfaces: shape's `templates/brief.md` (the change-scale authoring authority), its byte-identical Go-embedded projection (`internal/cli/change_brief_template.md`, drift-gated by `TestChangeScaffoldTemplatesMatchCanonical`), and bootstrap's `templates/brief.md` (project scale, `source:` gains a `pitch` value).
- Captured-folder promotion: `loaf change init <slug>` completes a capture-only folder idempotently — preserving `brief.md` and `change.json` values verbatim while instantiating `shape.md` and the seeded `tasks/` — instead of rejecting the slug as a duplicate (today's behavior, verified: capture then re-init dead-ends). Promotion follows an explicit state matrix: only ordinary `init`, only a structurally valid brief-only folder; repeated `--brief`, missing-brief, hybrid-layout, and invalid-metadata states fail clearly and untouched, existing files are never overwritten, and an interrupted completion is resumable. Duplicate rejection is unchanged for fully-materialized folders; regression tests prove the matrix and the failure protocol, and the behavior ships with the rebuilt runtime and converged CLI guidance in the same commit. Without this, pitch's "shape now" handoff cannot reach the shipped Change anatomy.
- Brief semantics refined, not broken: authored at pitch time, may accrete parked problem-space concepts while the change is captured, freezes when `shape.md` exists; superseded by `shape.md`; never mechanically load-bearing; a brief-only folder still derives "captured".
- Change-scale ceremony: pitch proposes the slug, runs `loaf change init <slug> --brief`, authors `brief.md`, stamps `target_release` in `change.json` when the release binding is known, then offers either "shape now" (create the slug branch, hand to `/shape`, which materializes the full scaffold in place via the promotion path) or "park" per the landing matrix of Decision 11 — a targeted capture commits docs-only on the default branch (the shipped promise-carrier exception), an untargeted capture commits docs-only on its slug branch or stays native intake state; every landing is validated first with explicit-path `loaf change check <folder>` plus a direct `change.json` read-back of the intended target, and pitch prepares commits and never pushes.
- Project-scale ceremony: pitch authors `docs/BRIEF.md` and hands to `/bootstrap`.
- Series-prep at bootstrap: after operating-document population, bootstrap and the builder enumerate the initial arc as captured changes — `loaf change init <slug> --brief` each, briefs seeded problem-space-only from the BRIEF's scoped concepts, sequencing as prose in each brief. The landing matrix applies per mint: a series member carries a coarse `target_release` and lands as its own docs-only commit on the default branch (one commit per capture, never a batch — the promise-carrier pattern), validated first with explicit-path `loaf change check <folder>` plus a direct `change.json` read-back of the stamped target; a concept without even a coarse target binding is not ready to be minted and stays a BRIEF line, spark, or Intent. Every mint is user-confirmed; bootstrap never auto-shapes.
- Flow seams: `/shape` treats an existing `brief.md` as primary input — restate the problem from it, grill solution-space only, fall back to full narrowing when no brief exists (pitch is optional, never mandatory); `/triage` gains the "pitch" disposition for items needing problem discovery ("hand to shape" survives for well-understood directions, and the promoted item resolves against the created change); bootstrap's greenfield+brief mode recognizes `source: pitch` as discovery-already-done and interviews for gaps only; `explore` demotes to `user-invocable: false` with its description rerouting users to `/pitch` (brainstorm is already non-invocable — its description reroutes too).
- Discovery and guidance: skills are auto-discovered from `content/skills/` at build time and pitch ships no hooks — `config/hooks.yaml` holds hook instances, not skill registrations, and journal self-logging lives in Critical Rules. Flow vocabulary (pitch → shape → implement → ship → release) swept through root `AGENTS.md` (canonical per ADR-020; `.claude/CLAUDE.md` follows as the compatibility symlink) and skill cross-references; rebuilt `plugins/` and `dist/` artifacts committed with the source changes that produce them.

**Out** (deferred, not rejected)

- Deep rewrite of explore and brainstorm into smaller agent-technique skills with per-technique model overrides — rides the Loaf Flow skills audit (Intent created by this shaping); this change flips their invocation surface and routing only.
- The review-convergence loop on implement/ship (review rounds until no blocker/major findings, capped at 10, with fixing tasks between rounds) — its own future change; Intent created by this shaping.
- Subagents-vs-skills for mechanical operations (git, knowledge base) — evidence-gathering research, its own Intent from this shaping.
- Release promotion (beta → RC → stable) — already queued as the promotion-model design; the pitch conversation's promotion paragraph routed there, not here.
- Tracker sync of a pitched series — the SPEC-023 successor owns tracker mapping.
- Single-sourcing the brief skeleton across its three surfaces (build-time injection) — revisit if the surfaces drift in practice.

**Cut** (explicitly rejected)

- No `pitch.md` document — the brief IS the pitch output; the role-named narrative set stays brief/shape/plan/design.
- No new CLI verbs and no new machine state — no `loaf pitch`, no "pitched" derived state, no brief-richness detection. A pitched brief and a pasted brief are mechanically identical; richness is prose, and the derived ladder is untouched. Captured-folder promotion is the existing `init` verb gaining idempotent completion, not a new verb.
- No cross-change relation fields — sequencing in a pitched series is prose in each brief plus `target_release` cohorts; the change-work-model Cut stands, and repeated prose-ordering friction during shaping is the "real need" evidence its escape clause names.
- No solution-space content in briefs — approach, architecture, and decomposition belong to shape; the pseudo-shape guard tightens from "don't author here" to "no solution design here."
- `change.json` stays JSON — ".yml" during the shaping interview was read as shorthand for the metadata file, flagged twice without correction; a format migration was neither proposed nor accepted.
- Pitch never writes `shape.md`, never seeds `tasks/`, never opens PRs, never auto-runs shape or bootstrap.

## Observable Workflow

Existing project, new concept: the user runs `/pitch` with a raw idea, or `/triage` dispositions an intake item as "pitch this." The interview grills problem, affected users, current alternatives, value proposition, and constraints — one question at a time, recommendation first — delegating a competitive scan to a researcher subagent when the concept warrants it. Pitch proposes a slug, runs `loaf change init <slug> --brief`, authors `brief.md` (evidence in `research/`, sources linked), stamps `target_release` when known, and offers: shape now (slug branch created, `/shape` picks up the brief, materializes the full scaffold in place through the promotion path, and grills solution-space only) or park (a committed capture per the landing matrix — targeted captures commit docs-only on the default branch as promise carriers, untargeted captures commit docs-only on their slug branch or stay native intake state, push and PR left to the human; parked problem-space concepts may accrete into the brief until shaping starts).

New project: the user runs `/pitch` in an empty or minimal directory. The same interview runs at project altitude and writes `docs/BRIEF.md` (`source: pitch`). `/bootstrap` consumes it — recognizing discovery as done, interviewing for gaps only — populates the operating documents as shipped, then closes with series-prep: builder and bootstrap enumerate the initial arc as captured changes, each minted with `loaf change init <slug> --brief`, seeded with a problem-space brief extracted from the BRIEF, bound to a coarse `target_release`, sequencing stated as prose, and landed as its own docs-only commit (concepts without a target binding stay BRIEF lines or Intents). Work then proceeds change by change: `/shape` picks a captured brief, promotes the folder to the full anatomy, and turns the brief into a contract.

`/shape` without a brief behaves exactly as today — pitch is the recommended front door, never a gate. `/explore` disappears from the user's slash menu; when a pitch interview reveals the direction is genuinely undecided, the agent reaches for the explore technique (Explorations, checkpoints, Intents — machinery untouched) from inside the pitch.

## Rabbit Holes and No-Gos

- **The interview must not become The Form.** One adaptive question at a time with a recommendation, ordered by what would change the brief most; bootstrap's interview anti-patterns (the 45-minute interrogation, the echo chamber, premature architecture) bind pitch too.
- **Series-prep is not roadmap planning.** It mints captured changes and optional targets, nothing more — no milestone entities, no dates, no priorities, no dependency fields. Trackers own planning; Loaf owns evidence.
- **Brief accretion must not become shaping.** Parked concepts are problem-space sentences; the moment solution prose appears, it belongs in a shape session, and review should bounce it.
- **Don't build the review-convergence loop or promotion model "while we're here."** This change ships the entry stage only.
- **Bootstrap's document population is not rewritten.** Series-prep appends a closing phase; VISION/STRATEGY/ARCHITECTURE extraction stays as shipped.
- **Don't invent a third interview idiom.** Pitch borrows shape's grilling mechanics and bootstrap's anti-patterns; the innovation is the register (problem-space) and the output (the brief), not new interview machinery.

## Decisions

Provenance: shaping interview 2026-07-28 (four grilling rounds); change-work-model's shipped contract and Cut list; review of bootstrap, explore, brainstorm, triage, and shape skills as of v2.0.0-alpha.15.

1. **Pitch's output is the brief — no new document.** Enriching `brief.md` keeps the role-named set stable and the supersession chain (brief → shape.md) intact. Forecloses `pitch.md` and pitch-writes-shape-directly. (Interview round 1.)
2. **The brief/shape boundary moves from capture-vs-authoring to problem-space-vs-solution-space.** Pitch grills what, who, and why-valuable; shape grills how, boundaries, and verification. The "brief must not become a pseudo-shape" guard becomes a content rule review can enforce.
3. **Sequencing for a pitched series is prose plus `target_release`; the relation-field Cut stands.** The interview's "combination: metadata + template" adjudicated as: `change.json` carries the release cohort (existing field), the brief's sequencing section carries narrative order. (Interview round 2.)
4. **Pitch is a human ceremony.** `disable-model-invocation: true`; agents never enter the Flow by pitching, while agent legwork inside a pitch is delegated by the skill. (Interview round 2.)
5. **Explore and brainstorm demote to agent techniques; `/pitch` replaces them in the Flow.** Sidecar visibility and description rerouting land now; the deep simplification (smaller skills, per-technique model overrides) is deferred to the skills audit. Exploration durable records and the four-field checkpoint contract stay intact — what changes is who reaches for them. (Interview round 3.)
6. **Series-prep ships in this change.** The both-scale pipeline lands whole: pitch → BRIEF → bootstrap → captured change series. (Interview round 4.)
7. **The brief accretes until shaping, then freezes.** Exercises the pre-shaping mutability already implied by "never updated after shaping"; parking-lot use is legal accretion, always problem-space.
8. **No pitched-state machinery.** Brief-only derives "captured" regardless of richness — `loaf change check` stays a structure validator, never a prose critic.
9. **Pitch is optional at change scale.** Shape retains full narrowing when no brief exists; the flow recommends the front door without gating on it, preserving low-ceremony small changes.
10. **Pitch ships no hooks.** Skills are auto-discovered from `content/skills/` at build time; `config/hooks.yaml` carries hook instances with an owning skill, never skill registrations, and pitch's journal self-logging is skill text. Resolves the shaping-time fog entry about registration. (Codex review round 1.)
11. **Park lands by the matrix, never dangles — and never broadens the shipped exception.** A parked capture *with* a declared `target_release` commits docs-only on the default branch: exactly the shipped promise-carrier exception, unchanged. An *untargeted* park commits docs-only on its slug branch (durable, off-main, the branch shaping will later use) or stays native intake state (Intent or spark) when it is not ready to be a Change at all — untargeted captures never land on main, so "main carries completed changes" holds. Series-prep applies the same matrix, one docs-only commit per capture, never a batch. Every capture landing is validated before it is committed: explicit-path `loaf change check <folder> --json` (bare invocation resolves by branch and would miss a capture landing elsewhere) must report zero violations and the expected captured state, and the folder's `change.json` is then read back directly to confirm the intended `target_release` presence or absence — check validates the schema, the read-back confirms the promise, and no new check output surface is needed. The promise is checked before the promise carrier lands. Pitch and bootstrap prepare commits; push and PR stay human. (Supersedes this decision's round-2 form, which over-broadened the exception; Codex review rounds 2–4.)
12. **Captured folders promote through the CLI, never skill-side template copying — and promotion is a protocol, not a convenience.** `loaf change init <slug>` completes a capture-only folder idempotently — `brief.md` and `change.json` values preserved verbatim, `shape.md` and seeded `tasks/` instantiated — while duplicate rejection stays for fully-materialized folders. The transition is state-matrixed and overwrite-free, with every destination file published atomically (temp-write then rename, refusing existing destinations) and `shape.md` renamed last as the promotion marker: the matrix admits exactly three outcomes — a structurally valid brief-only folder promotes; a partial promotion (no `shape.md`, existing `tasks/` content byte-identical to the would-be seed) resumes by filling only the gaps; everything else fails clearly and untouched — because a half-written promotion that re-creates the dead-end would be worse than the dead-end, and ordering alone cannot prevent one: only atomic publication guarantees a destination is either absent or complete. Deterministic scaffold materialization is protocol-layer work; a skill hand-copying templates is the prose-reimplements-CLI anti-pattern the three-layer separation exists to prevent. Verified gap: today the second `init` rejects the captured slug outright, dead-ending pitch's primary handoff. (Codex review rounds 3–4.)

## Planning Contract

### Approach

Skill-layer work with two CLI touchpoints: the brief template projection and `init`'s captured-folder completion. The pitch skill is authored fresh (SKILL.md + interview-guide reference + sidecars), explicitly borrowing shape's grilling mechanics and bootstrap's interview anti-patterns. The brief skeleton is defined once — in this contract — and propagated to its three surfaces; shape's content template is the change-scale authoring authority, and the Go embed is its byte-identical scaffold projection per the shipped canonical-match rule. Seam edits to shipped skills are section-level, not rewrites.

### Placement

New skill under `content/skills/pitch/` (SKILL.md, `SKILL.claude-code.yaml`, `references/interview-guide.md`, templates as needed). CLI template and tests in `internal/cli/change_scaffold.go` and `internal/cli/change_test.go`. Seam edits in `content/skills/{shape,triage,bootstrap,explore,brainstorm}/`. Guidance in root `AGENTS.md` (canonical per ADR-020; `.claude/CLAUDE.md` follows as the compatibility symlink). Rebuilt `plugins/` and `dist/` committed with the tasks that change their sources.

### Template propagation

Three brief surfaces implement one skeleton: shape's `templates/brief.md` is the change-scale authoring authority, the Go embed is its byte-identical runtime projection (gated by `TestChangeScaffoldTemplatesMatchCanonical`, so drift there fails tests rather than lingering), and bootstrap's project-scale template restates the skeleton at project altitude. The two authored markdown surfaces are kept in agreement by hand this change; drift in practice is the trigger for the deferred single-sourcing.

### Risks

- Pitch could read as mandatory ceremony and add friction to small changes — mitigated by Decision 9 (optional at change scale) and triage retaining direct "hand to shape."
- The demotion of `/explore` removes a user-facing surface some muscle memory may reach for — the description reroutes to `/pitch`, and the technique remains available to the agent; reversal is a one-line sidecar flip if dogfooding rejects it.
- Series-prep quality depends on decomposition judgment the skill text can only guide, not enforce — bounded by user confirmation per mint and the H-tier dogfood.

### Sequencing

TASK-001 (brief contract) and TASK-006 (captured-folder promotion) lead — one defines the skeleton every unit consumes, the other unblocks the capture→shape seam; they are independent commits. TASK-002 (pitch skill) is the centerpiece and depends on TASK-001. TASK-003 (series-prep) depends on TASK-001 only; TASK-004 (seams) depends on TASK-002 (it routes users to the skill it must name) and TASK-006 (shape's materialization instruction relies on the promotion path existing). TASK-005 (guidance sweep) last, following shipped behavior.

## Implementation Units

<!-- Task packets live in tasks/TASK-NNN-slug.md; this section summarizes the decomposition. -->

- **TASK-001 — Brief contract.** The shared problem-space skeleton applied to all three template surfaces, with Go template-content tests; `source: pitch` added to the project-scale frontmatter vocabulary.
- **TASK-002 — Pitch skill.** SKILL.md, interview guide, the human-entry guard (Claude Code sidecar plus the cross-target Critical Rule), journal self-logging; both-scale ceremony including change init and the shape-now/park offer. No hooks (Decision 10).
- **TASK-003 — Bootstrap series-prep.** Pitched-BRIEF consumption (gap-only interview) and the closing series-prep phase minting captured changes.
- **TASK-004 — Flow seams.** Shape consumes briefs; triage gains the pitch disposition; explore demotes; brainstorm and explore descriptions reroute.
- **TASK-005 — Guidance sweep.** Flow vocabulary through AGENTS.md, the shipped work-model knowledge doc and glossary, ADR-022's amendment pointer, and skill cross-references; Durable Outputs distillation; the H4 end-to-end dogfood; final rebuild and drift check.
- **TASK-006 — Captured-folder promotion.** `loaf change init` gains idempotent completion of capture-only folders (brief and metadata preserved verbatim, `shape.md` + seeded `tasks/` instantiated), with regression tests for capture → promote → shaped; duplicate rejection unchanged for materialized folders.

## Verification Contract

<!-- Executable (machine-checkable): each V-entry declares Command and Expect for loaf change verify. -->

- **V1.** Go suite passes, including new template-content tests proving the scaffolded change-scale brief carries the shared skeleton, and promotion regression tests proving capture → `init` completion → `shaped` with `brief.md` and `change.json` preserved verbatim. Command: `go test ./... -count=1`. Expect: exit 0.
- **V2.** Full build succeeds with rebuilt targets and no artifact drift. Command: `npm run build`. Expect: exit 0.
- **V3.** Enforcement hooks pass over the tree. Command: `loaf check`. Expect: exit 0.
- **V4.** The pitch skill exists in every built target projection. Command: `test -f plugins/loaf/skills/pitch/SKILL.md && test -f dist/opencode/skills/pitch/SKILL.md && test -f dist/cursor/skills/pitch/SKILL.md && test -f dist/codex/skills/pitch/SKILL.md && test -f dist/amp/skills/pitch/SKILL.md`. Expect: exit 0.
- **V5.** Explore is demoted to agent-technique surface. Command: `grep -c "user-invocable: false" content/skills/explore/SKILL.claude-code.yaml`. Expect: exit 0 and contains `1`.
- **V6.** This change validates with zero violations and no executability gaps. Command: `loaf change check docs/changes/20260728-pitch-entrypoint --require-executable`. Expect: exit 0.
- **V7.** The human-entry guard ships in the built projections — Claude Code's merged frontmatter carries the mechanical flag, and every projection carries the behavioral rule verbatim. Command: `grep -q "disable-model-invocation: true" plugins/loaf/skills/pitch/SKILL.md && grep -qF "Agents never initiate a pitch." plugins/loaf/skills/pitch/SKILL.md && grep -qF "Agents never initiate a pitch." dist/opencode/skills/pitch/SKILL.md && grep -qF "Agents never initiate a pitch." dist/cursor/skills/pitch/SKILL.md && grep -qF "Agents never initiate a pitch." dist/codex/skills/pitch/SKILL.md && grep -qF "Agents never initiate a pitch." dist/amp/skills/pitch/SKILL.md`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** Interview dogfood (owned by TASK-002, before the skill text finalizes): a real, human-invoked `/pitch` run against a queued intake item produces a brief that passes H2's cold-read — the reaction artifact for the interview design.
- **H2.** A pitched brief read cold names the problem, the affected user, the current alternative, and the value in one pass — and contains zero solution-space content.
- **H3.** Series-prep dogfooded against a sample BRIEF yields captured changes whose briefs stand alone as intent for later shaping.
- **H4.** End-to-end dogfood (owned by TASK-005, after TASK-004's brief-aware shape lands): a fresh `/shape` session consumes a pitched brief without re-asking problem questions — the seam proof that pitch's output is shape's input, sequenced where it is actually satisfiable.

## Definition of Done

- TASK-001 through TASK-006 landed on `pitch-entrypoint` with V1–V7 green and checkboxes flipped in the delivering commits.
- The H1 and H4 dogfoods happened in an isolated scratch project — never on this branch — with the human invoking each ceremony in a session explicitly loaded from the branch-built plugin, and their evidence (including the candidate commit) lives at `research/pitch-interview-dogfood.md`; H3's series-prep evidence lives at `research/series-prep-dogfood.md`.
- The three routed Intents exist: `INTENT-20260728-loaf-flow-skills-audit-refactor-every-skill-to-the-pitch-to-release-model`, `INTENT-20260728-review-convergence-loop-for-implement-and-ship`, `INTENT-20260728-evidence-check-dedicated-subagents-versus-skills-for-mechanical-operations` (each readable via `loaf intent show <ref>`).
- Durable Outputs distilled via reflect on the branch before merge, owned by TASK-005 — the knowledge doc under `docs/knowledge/` and the ADR under `docs/decisions/` named below; the PR merges only when everything is done.

## Durable Outputs

- Knowledge doc: the Loaf Flow — pitch → shape → implement → ship → release, the two scales, the problem-space/solution-space boundary, and where explore/brainstorm techniques sit — converged with `docs/knowledge/work-model.md` rather than layered beside it (TASK-005 updates the shipped doc's front-door and brief narration in the same pass).
- ADR: the entry-stage model — pitch as the human ceremony, invocation-surface demotion of explore/brainstorm, the brief as the pitch artifact, the landing matrix, and the promotion path. Amends ADR-022's brief semantics (authored, accretes-until-shaping) rather than superseding it; ADR-022 gains an amended-by pointer.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. Tags are convention, never parsed by check. -->

- [UK] What a great pitch interview feels like — question dimensions, adaptivity, exit criteria → reaction artifact: the H1 dogfood against a real intake item, with the interview guide revised from the experience before TASK-002 finalizes.
- [KU] Series-prep granularity — when a BRIEF concept earns its own captured change versus staying a BRIEF line → owner: TASK-003 body and the interview guide's series-prep section; evidence via H3.

## Source Inputs

- This conversation, 2026-07-28: the user's Loaf Flow pitch (captured verbatim in `brief.md`) and four grilling rounds — output contract, sequencing adjudication, skill topology, scope.
- `docs/changes/20260726-change-work-model/shape.md` — brief.md semantics, the role-named document set, the Cut list ("pitch" recognized as a lifecycle-stage word; relation fields cut), the captured promise-carrier pattern.
- `content/skills/{bootstrap,explore,brainstorm,triage,shape}/SKILL.md` as shipped at v2.0.0-alpha.15.
- Project memory and journal: the promotion-model queue (the pitch conversation's release paragraph routed there), SPEC-023 successor (tracker sync routed there).
- Prior art, register check: Shape Up's "pitch" is a shaped bet presented for betting; Loaf's pitch is earlier — the problem-discovery ceremony. The word names the act of proposing, not Shape Up's artifact.
- Codex design review 2026-07-28, round 1 (needs-revision: 3 blocking, 3 should-fix, 1 nit) — the hooks-registration phantom, the ADR-020 canonical-path correction, Durable Outputs ownership, dogfood isolation and evidence location, the cross-target qualification of human-entry, Intent identifiability, and V6's `--require-executable`; dispositions at `reports/20260728-220618-review-codex-r1.html`.
- Codex design review 2026-07-28, round 2 (fresh thread: 2 blocking, 4 should-fix, 1 nice-to-have) — the stale hooks-registration line in Implementation Units, H1's unsatisfiable sequencing (split into H1/H4), the dogfood as a user-invoked checkpoint, template authority corrected to shape's content template with the Go embed as byte-identical projection, V4 across all five targets, park's committed-capture ceremony (Decision 11), and redaction of dogfood evidence; dispositions at `reports/20260728-225350-review-codex-r2.html`. Its recheck table confirmed rounds-1 B2/B3/SF1/SF3/N1 resolved.
- Codex design review 2026-07-29, round 3 (fresh thread: 2 blocking, 2 should-fix) — the captured→shaped materialization dead-end (verified empirically: `init` rejects a captured slug; resolved as TASK-006 promotion through the existing verb, Decision 12), the landing matrix bounding park and series-prep to the shipped promise-carrier exception (Decision 11 rewritten), task-local acceptance added to TASK-002/003/004, and work-model/glossary/ADR-022 convergence added to TASK-005; dispositions at `reports/20260729-002332-review-codex-r3.html`. Its recheck confirmed every prior finding resolved except the two it carried forward.
- Codex design review 2026-07-29, round 4 (fresh thread, gpt-5.6-sol at xhigh: 3 blocking, 3 should-fix) — the promotion transition protocol (state matrix, no-overwrite, resumable interruption), the atomic-shipping rule for TASK-006 (branch-built runtime, artifact verifier, mirrors and receipts in the delivering commit), pre-landing validation for every capture (explicit-path `loaf change check`, folded into Decision 11), `init` contract-surface convergence (CLI help, generated reference, cli-boundary, task-system.md), the surviving "optionally bound" contradiction in TASK-003's Objective, and V7's enforcement assertions for the human-entry guard; dispositions at `reports/20260729-011139-review-codex-r4.html`. Its recheck holds rounds 1–3 closed with no re-raises.
- Codex whole-PR review 2026-07-29 (fresh thread, gpt-5.6-sol at xhigh, against draft PR #145: 2 blocking, 3 should-fix, 1 nit) — the landing guard's target confirmation was unexecutable as specified (`change check --json` emits no target; resolved as a direct `change.json` read-back, no new CLI surface), the promotion matrix contradicted its own resumability rule (resolved with `shape.md` as the atomic promotion marker and a template-identical partial state), the TASK-006 smoke used branch-resolved check in a scratch repo (explicit folder path now), V7's guard grep gained its trailing period, the PR body's "no re-raises across rounds" claim corrected to residuals-carried-and-closed, and the anchor's commit message updated to describe the final tree; dispositions at `reports/20260729-015747-review-codex-pr.html`. It confirmed the other 21 recorded dispositions reflected in the committed text and the diff correctly bounded to the Change folder.
- Codex final gate 2026-07-29 (fresh thread, gpt-5.6-sol at xhigh: 2 blocking, 3 should-fix, 1 nit) — verified all six PR-round dispositions landed, then caught the atomicity gap in the promotion marker (ordering alone cannot make a partial write safe → atomic temp-write-and-rename publication with partial-marker and partial-task interruption fixtures), the unpinned dogfood runtime (H1/H4 sessions now load the branch-built plugin via `--plugin-dir`, candidate commit recorded), H3's evidence durability (`research/series-prep-dogfood.md`), the `AGENTS.md` skill-registration guidance contradicting Decision 10 (TASK-005), the cli-boundary JSON envelope omitting the `state` field the guard reads (TASK-006), and the PR body's stale board count; dispositions at `reports/20260729-025423-review-codex-final.html`.
