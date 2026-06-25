---
id: SPEC-051
title: Routing Eval & Validated Description Rewrites
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-E)"
created: 2026-06-22T09:13:21Z
status: complete
branch: feat/routing-eval-description-rewrites
source_sessions:
  - id: 20260621-001541-session
    role: shaped
---

# SPEC-051: Routing Eval & Validated Description Rewrites

## Problem Statement

Skill routing is the only mechanism that decides which of 34 skills a user
utterance reaches, and Loaf has no working evidence that routing is correct.
The one tool that should measure it — `cli/scripts/eval-skill-routing.mjs` — is
stale: it expects routes to skills that do not exist (`council-session`,
`cleanup`, `resume-session`, `reference-session` at
`cli/scripts/eval-skill-routing.mjs:195-244`), routes some prompts to `idea`
where `triage` now owns queue processing (`:185-189`), and does not test the
known confusable pairs at all. The actual skill set on the implementation
branch is
`architecture, bootstrap, brainstorm, breakdown, cli-reference, council,
database-design, debugging, documentation-standards, foundations, git-workflow,
go-development, handoff, housekeeping, idea, implement, infrastructure-management,
interface-design, knowledge-base, orchestration, power-systems-modeling,
python-development, refactor-deepen, reflect, release, research, ruby-development,
security-compliance, shape, ship, strategy, triage, typescript-development,
wrap` — 34 skills, none of which is
`council-session`/`cleanup`/`resume-session`/`reference-session`.

The deep-evaluation report (`report-loaf-skills-deep-audit`,
`.agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md:262-278`) names
both the eval drift and a set of description collisions
(`foundations`/`research`/`brainstorm`/`interface-design`/`strategy`) but warns
that string-inspection of descriptions is unreliable — the same failure mode the
cli-reference self-correction exposed. So the rewrites must be **measured**, not
asserted: a description "reads better" tells us nothing about whether the model
routes to it correctly. Without a routing-eval gate, every description edit is a
blind change to the most behaviorally-sensitive string in the system.

One adjacent drift rides here because it shares the same surface:
`docs/knowledge/skill-architecture.md:88` claims "33 skills total: 17 workflow,
16 reference/knowledge" while the real count is 34: 19 workflow/default-invocable
skills and 15 reference skills with `user-invocable: false`. (The companion
drift — that the user-invocable workflow skills lack a `skill(<name>)`
first-action self-log line — is owned by SPEC-048, which rewrites the skills'
session interaction.)

## Strategic Alignment

**Vision/Architecture.** Routing accuracy is the load-bearing property of the
"skills-first" model documented in `.claude/CLAUDE.md` ("Descriptions drive
routing... The model uses the description to choose from 100+ skills"). This
spec makes that property *testable* rather than assumed. It touches no runtime
backend and no harness build — it is a content + tooling workstream.

**Supersedes.** Nothing. This is purely additive (a refreshed eval harness,
measured description edits, doc-count fix).

**Coordinates-with.**
- **SPEC-050 (Skill De-bloat & Content Hygiene)** runs in parallel. Both edit
  skill content. SPEC-050 owns *body* de-bloat (cutting reference mass, fixing
  stale cross-refs, `## Contents` headers); SPEC-051 owns *frontmatter
  `description`* and the routing harness. The seam is the YAML frontmatter:
  SPEC-051 is the only workstream permitted to edit a skill's `description:`
  block; SPEC-050 edits everything below the closing `---`. This avoids
  merge collisions on the same files. The roadmap (WS-E) explicitly sequences
  D ∥ E for this reason.
- **SPEC-048 (Session-Model Convergence)** adds the self-logging first-action
  line to workflow skills. **Ownership decision:** the self-log line is owned by
  SPEC-048 because it is session-journal behavior, and SPEC-048 atomically
  rewrites the skills' session interaction. SPEC-051 does **not** edit the
  self-log line; it touches only the frontmatter `description:` block.
- **SPEC-049 (Status-Vocabulary Unification)** is unaffected — descriptions
  reference statuses only incidentally.
- This spec depends on no other spec to begin. The deep-evaluation report it
  draws from is `report-loaf-skills-deep-audit`
  (`.agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md`).

## Solution Direction

Build a skill-creator-style routing-eval harness as the gate, then let the gate
decide which description rewrites ship.

1. **Refresh the harness** (`cli/scripts/eval-skill-routing.mjs`). Drive the
   skill list and the `<available_skills>` listing from the real
   `content/skills/*` set (the loader already does this at `:54-84`; the stale
   part is the hard-coded `TEST_CASES` map at `:89-250`). Replace `TEST_CASES`
   with the current 34 skills and remove the four phantom skills. Add an
   explicit **conflict-pair suite**: utterance → expected-skill probes for
   `idea`/`triage`, `research`/`brainstorm`, `strategy`/`reflect`,
   `ship`/`release`, `architecture`/`shape`, and
   `foundations`/`git-workflow`/`documentation-standards`. The harness keeps its
   current shape (system-prompt listing + single-token model answer +
   pass/fail + per-skill accuracy + cost report) and the `npm run eval:routing`
   entrypoint (`package.json:25`).

2. **Establish a no-key structural gate now.** Run the refreshed harness in
   dry-run mode against the *current* skill set and conflict probes so the suite
   itself is trustworthy without `ANTHROPIC_API_KEY`. The implementation
   environment currently has no `ANTHROPIC_API_KEY`, and the user-approved scope
   is description-only / harness-ready now, so live baseline capture is deferred
   until a key-backed run is available.

3. **Prepare eval-gated rewrites without shipping blind edits.** For each
   future candidate skill (`foundations`, `research`, `brainstorm`,
   `interface-design`, `strategy`), propose a new `description:`, run the
   harness on the conflict-pair suite with the proposed description swapped in,
   and **ship the rewrite only if measured routing accuracy improves and nothing
   regresses**. A rewrite that reads
   better but does not move (or worsens) the number is discarded. No blind
   rewrites — this is the explicit lesson from the cli-reference self-correction
   that string inspection is unreliable
   (`.agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md:24`).

4. **Fix the taxonomy doc.** Update `docs/knowledge/skill-architecture.md:88`
   from "33 skills total" to the verified count (34) with the correct
   workflow/reference split, and refresh any other stale figure in that file.

The harness is the deliverable that outlasts this spec: it is the regression
guard so the next description edit or new skill cannot silently degrade routing.

## Scope

### In Scope
- Rewrite `cli/scripts/eval-skill-routing.mjs` test cases to the real 34-skill
  set; remove phantom skills (`council-session`, `cleanup`, `resume-session`,
  `reference-session`).
- Add a conflict-pair probe suite for the six named pairs/groups.
- Add a no-key structural validation mode for the suite, plus key-backed JSON
  output support for future live baseline runs.
- Add description-budget and baseline-output scaffolding so future
  `description:` rewrites can be measured instead of shipped by inspection.
- Fix the skill count and tier figures in
  `docs/knowledge/skill-architecture.md`.

### Out of Scope
- Editing skill *bodies* below the frontmatter (SPEC-050).
- Any change to the build pipeline, harness targets, or hook generation
  (SPEC-047 / WS-A).
- Adding the eval to CI as a blocking gate (requires an API key in CI and has
  per-run cost; see Rabbit Holes). The eval is a developer/pre-merge tool here.
- Renaming, hiding, retiring, or merging any skill (taxonomy decisions are
  SPEC-053 / WS-G; `debugging` disposition is not decided here).
- Session-model rewrites or status-enum work (SPEC-048 / SPEC-049).
- First-action self-logging line (`loaf session log "skill(<name>)"`) — owned by
  SPEC-048, which atomically rewrites the skills' session interaction.
- Rewriting descriptions for skills not measurably collision-prone.
- Capturing the live routing baseline or shipping the candidate
  `description:` rewrites without `ANTHROPIC_API_KEY`; this is deferred until a
  key-backed run can prove measured wins.

### Rabbit Holes
- **Turning the LLM eval into a hard CI gate.** It costs money per run and needs
  a key; non-determinism means a single run can flap. Keep it a developer tool
  invoked on demand; if a deterministic guard is wanted, that is a separate
  follow-on (a cheaper structural lint, not the LLM eval).
- **Chasing 100% routing accuracy.** Some utterances are genuinely ambiguous
  (e.g. "step back and assess" between `research` and `reflect`). The bar is
  *measured improvement on the named pairs*, not perfection.
- **Rewriting all 34 descriptions.** Only the five named collision-prone skills
  are in scope; touching others invites churn without evidence.
- **Building a bespoke eval framework.** Reuse the existing harness shape; do
  not import a heavyweight eval dependency (CLAUDE.md: ask before adding deps).

### No-Gos
- No description ships without a measured routing improvement behind it.
- No edits to skill bodies (SPEC-050's surface) — frontmatter `description:`
  only.
- No new third-party dependency for the harness without explicit approval.
- No taxonomy/visibility changes (those are gated under SPEC-053).

## Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| LLM eval non-determinism flaps pass/fail between runs | Medium | High | Report accuracy as aggregate over the suite, not single-prompt pass/fail; run N>1 and require the *delta* to hold; never wire as a hard CI gate |
| Eval requires `ANTHROPIC_API_KEY` + cost; not all contributors can run it | Medium | High | Keep as opt-in developer tool; commit baseline result so reviewers see the number without re-running |
| Improving one pair regresses another (descriptions interact) | High | Medium | Always run the *full* conflict suite per candidate, not just the target pair; reject any net or per-pair regression |
| File collision with SPEC-050 editing the same SKILL.md | Medium | Medium | Hard seam: SPEC-051 edits only the `description:` block; SPEC-050 edits below frontmatter; sequence D ∥ E with this boundary |
| Rebuilt `dist/*` drift if descriptions change but artifacts not regenerated | Medium | Medium | `loaf build` + `git diff --exit-code -- dist plugins` before commit (existing contract) |

## Open Questions

1. What model should the recorded baseline use — Opus (highest fidelity, matches
   `DEFAULT_MODEL` at `eval-skill-routing.mjs:22`) or a cheaper model for
   repeatability? Decision: record baseline on the default model when a key is
   available; allow `--model` override for cheap iteration.
2. How is the baseline persisted for reviewers — a committed JSON fixture under
   `cli/scripts/` or a documented number in the spec/PR? Decision: committed
   JSON fixture so regressions are diffable.
3. Are `git-workflow` and `documentation-standards` themselves rewrite
   candidates, or only probe-targets that sharpen `foundations`? Proposed:
   probe-targets only unless the eval shows `foundations` cannot win without
   adjusting a sibling's negative routing.
4. Does the eval need to model the two-tier description truncation (≤250 char
   first sentence vs full) the harness already slices at
   `eval-skill-routing.mjs:81`? Decision: keep the explicit 250-character
   simulation parameter and report it in dry-run/live JSON output.

## Test Conditions

- [x] `npm run eval:routing -- --dry-run` validates the suite with no
      skipped/"not found" skills and without requiring `ANTHROPIC_API_KEY`.
- [x] With no `ANTHROPIC_API_KEY`, `npm run eval:routing` validates the suite
      before failing with the expected key-required message.
- [x] `rg -n 'council-session|cleanup|resume-session|reference-session' cli/scripts`
      returns no matches.
- [x] The harness includes explicit conflict-pair probes for all six named
      pairs/groups (`idea`/`triage`, `research`/`brainstorm`, `strategy`/`reflect`,
      `ship`/`release`, `architecture`/`shape`,
      `foundations`/`git-workflow`/`documentation-standards`).
- [x] Baseline-output scaffolding exists for a future key-backed run; the
      actual live baseline fixture is deferred until `ANTHROPIC_API_KEY` is
      available.
- [x] No candidate `description:` rewrite shipped without measured routing
      improvement; future rewrites remain gated by the harness.
- [x] `docs/knowledge/skill-architecture.md` states the verified skill count
      (34) and correct workflow/reference split; `rg -n '33 skills' docs`
      returns no matches.
- [x] `loaf build` succeeds and `git diff --exit-code -- dist plugins` is clean
      (regenerated artifacts committed with source).
- [x] `npm run typecheck` and `npm run test` pass.

## Priority Order

All tracks are **non-breaking** (content + developer tooling only; no schema,
no harness surface, no install change). No SPEC-053 gate applies.

1. **Track 1 — Refresh the harness** *(non-breaking; enables everything below)*.
   Rewrite `eval-skill-routing.mjs` test cases to the real 34-skill set, drop
   phantom skills, add conflict-pair probes, and add no-key structural
   validation. Go/no-go: dry-run harness runs clean with zero skipped skills.
2. **Track 2 — Baseline scaffolding** *(non-breaking; keyless now, live later)*.
   Keep live baseline recording available through `--output` while deferring the
   actual fixture until `ANTHROPIC_API_KEY` exists.
3. **Track 3 — Eval-gated description rewrites** *(deferred)*. Per-skill
   go/no-go: ship only on measured improvement, no regression. Skills that do
   not win are dropped from scope, not forced.
4. **Track 4 — Doc count fix** *(non-breaking; independent)*. Update
   `skill-architecture.md`. Can land anytime.
