---
id: SPEC-051
title: Routing Eval & Validated Description Rewrites
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-E)"
created: 2026-06-22T09:13:21Z
status: drafting
branch: feat/routing-eval-description-rewrites
source_sessions:
  - id: 20260621-001541-session
    role: shaped
---

# SPEC-051: Routing Eval & Validated Description Rewrites

## Problem Statement

Skill routing is the only mechanism that decides which of 35 skills a user
utterance reaches, and Loaf has no working evidence that routing is correct.
The one tool that should measure it — `cli/scripts/eval-skill-routing.mjs` — is
stale: it expects routes to skills that do not exist (`council-session`,
`cleanup`, `resume-session`, `reference-session` at
`cli/scripts/eval-skill-routing.mjs:195-244`), routes some prompts to `idea`
where `triage` now owns queue processing (`:185-189`), and does not test the
known confusable pairs at all. The actual skill set is
`architecture, bootstrap, brainstorm, breakdown, cli-reference, council,
database-design, debugging, documentation-standards, foundations, git-workflow,
go-development, handoff, housekeeping, idea, implement, infrastructure-management,
interface-design, knowledge-base, orchestration, power-systems-modeling,
python-development, refactor-deepen, reflect, release, research, ruby-development,
security-compliance, shape, ship, strategy, thermo-nuclear-code-quality-review,
triage, typescript-development, wrap` — 35 skills, none of which is
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
16 reference/knowledge" while the real count is 35. (The companion drift — that
the user-invocable workflow skills lack a `skill(<name>)` first-action self-log
line — is owned by SPEC-048, which rewrites the skills' session interaction.)

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
   with the current 35 skills and remove the four phantom skills. Add an
   explicit **conflict-pair suite**: utterance → expected-skill probes for
   `idea`/`triage`, `research`/`brainstorm`, `strategy`/`reflect`,
   `ship`/`release`, `architecture`/`shape`, and
   `foundations`/`git-workflow`/`documentation-standards`. The harness keeps its
   current shape (system-prompt listing + single-token model answer +
   pass/fail + per-skill accuracy + cost report) and the `npm run eval:routing`
   entrypoint (`package.json:25`).

2. **Establish a baseline.** Run the refreshed harness against the *current*
   descriptions and record the per-pair accuracy. This is the number every
   rewrite must beat.

3. **Eval-gate the rewrites.** For each candidate skill
   (`foundations`, `research`, `brainstorm`, `interface-design`, `strategy`),
   propose a new `description:`, run the harness on the conflict-pair suite with
   the proposed description swapped in, and **ship the rewrite only if measured
   routing accuracy improves and nothing regresses**. A rewrite that reads
   better but does not move (or worsens) the number is discarded. No blind
   rewrites — this is the explicit lesson from the cli-reference self-correction
   that string inspection is unreliable
   (`.agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md:24`).

4. **Fix the taxonomy doc.** Update `docs/knowledge/skill-architecture.md:88`
   from "33 skills total" to the verified count (35) with the correct
   workflow/reference split, and refresh any other stale figure in that file.

The harness is the deliverable that outlasts this spec: it is the regression
guard so the next description edit or new skill cannot silently degrade routing.

## Scope

### In Scope
- Rewrite `cli/scripts/eval-skill-routing.mjs` test cases to the real 35-skill
  set; remove phantom skills (`council-session`, `cleanup`, `resume-session`,
  `reference-session`).
- Add a conflict-pair probe suite for the six named pairs/groups.
- Capture a recorded baseline routing-accuracy result (committed as a fixture or
  documented in the harness output section of the eval, not as live CI).
- Eval-gated `description:` rewrites for `foundations`, `research`,
  `brainstorm`, `interface-design`, `strategy` — ship only measured wins.
- Fix the skill count and tier figures in
  `docs/knowledge/skill-architecture.md`.
- Rebuild artifacts (`dist/*`, `plugins/loaf/`) for any skill whose
  `description:` changed, committed with the source.

### Out of Scope
- Editing skill *bodies* below the frontmatter (SPEC-050).
- Any change to the build pipeline, harness targets, or hook generation
  (SPEC-047 / WS-A).
- Adding the eval to CI as a blocking gate (requires an API key in CI and has
  per-run cost; see Rabbit Holes). The eval is a developer/pre-merge tool here.
- Renaming, hiding, retiring, or merging any skill (taxonomy decisions are
  SPEC-053 / WS-G; `debugging`/`thermo` disposition is not decided here).
- Session-model rewrites or status-enum work (SPEC-048 / SPEC-049).
- First-action self-logging line (`loaf session log "skill(<name>)"`) — owned by
  SPEC-048, which atomically rewrites the skills' session interaction.
- Rewriting descriptions for skills not measurably collision-prone.

### Rabbit Holes
- **Turning the LLM eval into a hard CI gate.** It costs money per run and needs
  a key; non-determinism means a single run can flap. Keep it a developer tool
  invoked on demand; if a deterministic guard is wanted, that is a separate
  follow-on (a cheaper structural lint, not the LLM eval).
- **Chasing 100% routing accuracy.** Some utterances are genuinely ambiguous
  (e.g. "step back and assess" between `research` and `reflect`). The bar is
  *measured improvement on the named pairs*, not perfection.
- **Rewriting all 35 descriptions.** Only the five named collision-prone skills
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
   repeatability? Proposed: record baseline on the default model, allow
   `--model` override for cheap iteration.
2. How is the baseline persisted for reviewers — a committed JSON fixture under
   `cli/scripts/` or a documented number in the spec/PR? Proposed: committed
   fixture so regressions are diffable.
3. Are `git-workflow` and `documentation-standards` themselves rewrite
   candidates, or only probe-targets that sharpen `foundations`? Proposed:
   probe-targets only unless the eval shows `foundations` cannot win without
   adjusting a sibling's negative routing.
4. Does the eval need to model the two-tier description truncation (≤250 char
   first sentence vs full) the harness already slices at
   `eval-skill-routing.mjs:81`? Confirm the 250-char budget still matches the
   harnesses' real `<available_skills>` truncation.

## Test Conditions

- [ ] `npm run eval:routing` runs end-to-end with no skipped/"not found" skills
      (the phantom-skill warning at `eval-skill-routing.mjs:319` never fires).
- [ ] `rg -n 'council-session|cleanup|resume-session|reference-session' cli/scripts`
      returns no matches.
- [ ] The harness includes explicit conflict-pair probes for all six named
      pairs/groups (`idea`/`triage`, `research`/`brainstorm`, `strategy`/`reflect`,
      `ship`/`release`, `architecture`/`shape`,
      `foundations`/`git-workflow`/`documentation-standards`).
- [ ] A baseline routing-accuracy result is recorded and committed.
- [ ] Each shipped `description:` rewrite (`foundations`, `research`,
      `brainstorm`, `interface-design`, `strategy`) has a recorded
      before/after accuracy showing measured improvement with no per-pair
      regression; any candidate without a win is documented as discarded.
- [ ] `docs/knowledge/skill-architecture.md` states the verified skill count
      (35) and correct workflow/reference split; `rg -n '33 skills' docs`
      returns no matches.
- [ ] `loaf build` succeeds and `git diff --exit-code -- dist plugins` is clean
      (regenerated artifacts committed with source).
- [ ] `npm run typecheck` and `npm run test` pass.

## Priority Order

All tracks are **non-breaking** (content + developer tooling only; no schema,
no harness surface, no install change). No SPEC-053 gate applies.

1. **Track 1 — Refresh the harness** *(non-breaking; enables everything below)*.
   Rewrite `eval-skill-routing.mjs` test cases to the real 35-skill set, drop
   phantom skills, add conflict-pair probes. Go/no-go: harness runs clean with
   zero skipped skills.
2. **Track 2 — Record baseline** *(non-breaking; gate for Track 3)*. Run and
   commit baseline accuracy. Go/no-go: baseline reproducible within run-to-run
   variance.
3. **Track 3 — Eval-gated description rewrites** *(non-breaking; each rewrite
   independently gated)*. Per-skill go/no-go: ship only on measured
   improvement, no regression. Skills that do not win are dropped from scope,
   not forced.
4. **Track 4 — Doc count fix** *(non-breaking; independent)*. Update
   `skill-architecture.md`. Can land anytime.
