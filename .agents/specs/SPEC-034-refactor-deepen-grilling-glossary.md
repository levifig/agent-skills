---
id: SPEC-034
title: Refactor-deepen skill + grilling protocol + glossary convention
source: direct
created: '2026-05-01T22:27:43.000Z'
status: done
branch: feat/refactor-deepen-grilling-glossary
session: 20260502-025928-session.md
---

# SPEC-034: Refactor-deepen skill + grilling protocol + glossary convention

## Problem Statement

Loaf has no skill for proposing structural code improvements grounded in a consistent architectural vocabulary. Today, refactoring requests are handled by `/implement` or ad-hoc conversation, with vocabulary drift across sessions ("boundary," "service," "component," "layer" used interchangeably). Loaf also lacks a domain glossary mechanic — there's no canonical place where "we call this `Order intake module`, never `OrderHandler`" lives, and no skill enforces such terminology even when it's been agreed.

Matt Pocock's `improve-codebase-architecture` skill ports cleanly into Loaf as a refactoring-by-deepening workflow, but doing so usefully requires three adjacent gaps to be filled simultaneously:

1. The grilling-as-interview-protocol mechanic (currently embedded inconsistently in `/shape` and `/architecture`)
2. A domain glossary surface with vocabulary discipline (no equivalent exists today)
3. A lighter-than-spec output artifact for refactoring work (specs are feature-shaped; refactors want a leaner shape)

## Strategic Alignment

- **Vision:** "Domain expertise that loads automatically." A deepening lens with consistent vocabulary fits this — but only if the vocabulary is enforced. The glossary convention closes that loop.
- **Personas:** Solo developer benefits from a tighter refactoring workflow. Team lead benefits from vocabulary consistency across developers and AI tools.
- **Architecture:**
  - Skills are the universal knowledge layer — `/refactor-deepen` works across all 6 targets without target-specific code.
  - "CLI is the protocol layer" — glossary mutations route through `loaf kb glossary {propose, stabilize, upsert, list, check}`. CLI verbs encode mutation policy; skills choose intent. Removes the prose-clarity dependency of "which skill writes what" — the CLI enforces it.
  - Skill breadth tension is real (already at 31 skills) — this spec adds **+1** skill (`/refactor-deepen`) and reuses existing surfaces (shared templates, KB conventions) for everything else.
- **Tensions surfaced during shaping (captured as ideas):**
  - **`20260501-225251-spec-plan-tasks-artifact-taxonomy`** — Specs are feature-shaped; refactors and bug fixes want lighter artifacts. SPEC-034 ships PLAN as a minimal ad-hoc shape; the broader SPEC-as-PRD / PLAN-as-strategy / TASKS-as-agent-native taxonomy is a follow-up shaping session, informed by SPEC-034's first plan in the wild.
  - **`20260501-225335-shape-spawned-ideas-harness`** — `/shape` has no discipline for capturing adjacent concepts that surface during interviews. The mechanism exists; the workflow connecting `/idea`, `related:`, `/triage`, and `/reflect` across a shaping session does not. Affects every deliberation skill, not just this spec.
  - **`20260501-231922-plan-lifecycle-cli-doctor-housekeeping`** — Plans need lifecycle infrastructure (list, archive, doctor recognition, housekeeping awareness) parallel to specs. Originally Track C of this spec; extracted as its own follow-up because lifecycle is a distinct product surface from refactoring-skill scope.
  - **`20260501-231923-shape-glossary-evolution-deferred`** — `/shape` evolution to participate in glossary mutation was originally Track D. Removed because `/shape` is an upstream ambiguity funnel (the convergent step where fuzzy concepts get bounded), and writing to a stability-focused glossary from inside an ambiguity-resolving step risks polluting canonical vocabulary. Revisit after `/architecture` and `/refactor-deepen` validate the convention.

## Solution Direction

Port Matt Pocock's `improve-codebase-architecture` skill as `/refactor-deepen` (first member of a `/refactor-{flavor}` family). Extract the grilling-as-interview-protocol from his `grill-with-docs` skill as a Loaf-shared template, distributed to `/architecture` and `/refactor-deepen` only. (`/shape` distribution was originally planned but deferred per `20260501-231923-shape-glossary-evolution-deferred` — see Test Conditions.) Add a domain glossary KB convention so vocabulary discipline has a home, with mutation policy encoded in CLI verbs (`propose` / `stabilize` / `upsert`) rather than skill prose. Evolve `/architecture` to absorb the with-docs side-effects (glossary stabilization inline during interviews, not just ADR creation). Output of `/refactor-deepen` is a minimal plan artifact, with the artifact-taxonomy question deliberately deferred and lifecycle infrastructure deferred to a follow-up spec.

The skill preserves Matt's vocabulary discipline (Module, Interface, Implementation, Depth, Seam, Adapter, Leverage, Locality) verbatim — substituting "boundary" or "service" defeats the entire point.

## Scope

### In Scope

- **Track A (foundation: grilling + glossary CLI + `/architecture` evolution):**
  - `content/templates/grilling.md` — shared template documenting the interview protocol (relentless interview, walk decision tree, recommend per question, prefer exploration when it can answer). Owns: read existing glossary at start, challenge term drift inline, surface candidate terms during interview. Does **not** own mutation — that's the CLI's job.
  - `targets.yaml` `shared-templates` registration distributing `grilling.md` to `architecture` and `refactor-deepen` only (NOT `shape` — see deferral rationale in `20260501-231923-shape-glossary-evolution-deferred`)
  - KB convention: `docs/knowledge/glossary.md` with `type: glossary` frontmatter; lazy creation pattern (CLI creates file on first canonical term, never upfront)
  - Glossary format borrowed from Matt's `CONTEXT-FORMAT.md`: term + definition + `_Avoid_:` aliases + `## Relationships` + `## Flagged ambiguities`. The file also holds a `## Candidates` section for proposed-but-not-yet-canonical terms.
  - `loaf kb glossary {propose, stabilize, upsert, list, check}` subcommand:
    - `propose <term> --definition <d>` writes to candidates section. Caller-agnostic, low-stability commitment. (Reserved for future use; SPEC-034 itself does not invoke `propose`.)
    - `stabilize <term>` promotes a candidate to canonical. Caller is `/architecture`.
    - `upsert <term> --definition --avoid <list>` writes/updates a canonical term directly. Callers are `/architecture` and `/refactor-deepen`.
    - `list [--canonical | --candidates | --all]` enumerates entries.
    - `check <term>` returns canonical definition if known, "avoided alias for X" if it's a non-canonical term in any `_Avoid_:` list, or "unknown" if absent.
    - All write commands fail fast with explicit error in Linear-native mode (per ADR-011): `"Linear-native glossary writes pending artifact-taxonomy spec — local mode only for now."` Read commands (`list`, `check`) work in both modes.
  - `/architecture` skill evolution — *stabilizes* terms: reads existing glossary at start; offers `stabilize` or `upsert` inline when fuzzy language surfaces during ADR interviews; references shared `grilling.md`
- **Track B (deepening skill + plan artifact):**
  - `content/skills/refactor-deepen/SKILL.md` — Critical Rules, Verification, Quick Reference, Topics
  - `references/language.md` — Module, Interface, Implementation, Depth, Seam, Adapter, Leverage, Locality (verbatim from Matt's LANGUAGE.md, with Loaf-context mapping notes)
  - `references/deepening.md` — dependency categories (in-process, local-substitutable, ports-and-adapters, true-external) and seam discipline (verbatim from DEEPENING.md)
  - `references/interface-design.md` — parallel sub-agent design pattern. Default rule: **spawn exactly 3 sub-agents with the same brief** (no opposing-constraint priming) so variety emerges from sampling rather than from manufactured opposition. User can opt to add more agents or to add specific lenses on request, but the default is 3-identical-briefs.
  - `templates/plan.md` — minimal PLAN artifact shape (candidate / dependency category / proposed deepened module / what survives in tests / rejected alternatives)
  - Plan artifact location: `.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md` (same temporal-record naming as sessions/ideas/drafts/councils — plans are write-once snapshots, not sequentially-numbered contracts; no `id` frontmatter field)
  - `/refactor-deepen` *pressure-tests* terms: uses existing glossary entries via `loaf kb glossary check`; calls `loaf kb glossary upsert` when a deepening clearly names a structural module
  - Skill terminates with: *"Plan saved to `.agents/plans/<filename>.md`. Workflow handoff is pending the SPEC/PLAN/TASKS artifact taxonomy spec — for now, decide manually."* — no `/breakdown` or `/implement` recommendation, deferring to the taxonomy work.
- **Track C (Codex review opt-in):**
  - `/refactor-deepen` final step: if `codex` plugin detected, offer "Codex review of this deepening before commit?" — opt-in only, plugin-gated.

### Out of Scope

- Full SPEC/PLAN/TASKS artifact taxonomy reconciliation — captured as `20260501-225251-spec-plan-tasks-artifact-taxonomy`, follow-up shaping session
- Shape-spawned ideas harness (forward links, blocked_by semantics, automated capture) — captured as `20260501-225335-shape-spawned-ideas-harness`, follow-up
- Plan artifact lifecycle (`loaf plan list/archive`, doctor recognition, housekeeping awareness) — captured as `20260501-231922-plan-lifecycle-cli-doctor-housekeeping`, blocked-on this spec
- `/shape` evolution to participate in glossary mutation — captured as `20260501-231923-shape-glossary-evolution-deferred`, blocked-on this spec; revisit after `/architecture` and `/refactor-deepen` validate the convention
- Other `/refactor-{flavor}` skills (extract, rename, decompose, etc.)
- A standalone `/grill` user-invocable skill (use cases are covered by `/shape`, `/architecture`, `/refactor-deepen` invoking the shared template)
- Linear-native glossary or plan storage (both stay local; reconciliation belongs to the artifact-taxonomy spec). Write commands fail fast in Linear-native mode rather than silently degrade.
- Migrating existing ADR or KB content to use the new vocabulary
- Auto-detecting refactoring opportunities (the skill is invoked, not proactive)
- Hook-based glossary enforcement (PreToolUse hook flagging avoided aliases) — discipline is informational this spec; mechanical enforcement is a future spec if drift observed

### Rabbit Holes

- **Defining the full PLAN taxonomy here.** Tempting; out of scope. Minimal shape is enough.
- **Building a glossary-enforcement hook.** Tempting (PreToolUse flagging avoided aliases). Out of scope — discipline is informational here.
- **Porting the rest of Matt's pack.** Tempting; out of scope. Other Matt skills (`triage`, `to-prd`, `tdd`, `zoom-out`, `diagnose`) evaluated separately.
- **Renaming existing skills to match Matt's vocabulary.** Out of scope. Vocabulary discipline applies forward, not retroactively.
- **Building an interactive grilling chat-mode UI.** Out of scope.
- **Folding `/grill-me` (the bare productivity primitive) into Loaf.** Decided against during shaping — every concrete use case has a destination skill.
- **Auto-handoff from PLAN to /breakdown or /implement.** Out of scope — that's the artifact-taxonomy spec.
- **Building plan lifecycle commands here.** Tempting (the artifact exists, why not?). Out of scope — extracted to its own follow-up spec to keep this scope tight.
- **Priming the parallel sub-agents with opposing constraints.** Tempting — Matt's source skill does this (Agent 1 minimal, Agent 2 flexible, Agent 3 common-caller). Decided against in shape review: priming manufactures diversity rather than letting it emerge. If accidental convergence shows up in practice, the fallback is more agents or rerun, not priming.

### No-Gos

- Substituting "boundary," "service," "component," "layer" for the source skill's vocabulary. Discipline is the whole point.
- Making `/architecture`, `/shape`, or `/refactor-deepen` write directly to glossary file with raw filesystem ops. Glossary mutations route through `loaf kb glossary` CLI verbs.
- Writing the glossary file upfront before any term is resolved. Lazy creation only.
- Letting Codex review be on-by-default for `/refactor-deepen`. Opt-in, plugin-gated.
- Making `/refactor-deepen` produce a SPEC. The artifact mismatch is the entire reason the PLAN shape exists.
- Recommending `/breakdown` or `/implement` as next step from a finished plan — the handoff design is downstream of the deferred taxonomy spec.
- Silent degradation in Linear-native mode. Glossary write commands and plan creation must fail fast with an explicit error referencing the artifact-taxonomy spec, not produce partial state.
- Priming parallel sub-agents with opposing design constraints by default. Default brief is identical for all 3 agents; priming is opt-in only.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Vocabulary discipline doesn't stick — Claude drifts to "boundary/service/component" | Medium | High | Sentinel test as Track B's go/no-go: pick one shallow Loaf module, run `/refactor-deepen` on it, manually grade vocabulary fidelity (correct terms vs. drifted terms in output). Iterate Critical Rules until grade passes. Vocabulary docs (`references/language.md`) read on every invocation. Skill description leads with vocabulary. Hook-based enforcement is a future spec if drift observed in production. |
| Glossary KB convention competes with knowledge-management thinking, breaks existing `loaf kb` | Low | Medium | Explicit `type: glossary` frontmatter; existing KB queries unchanged. Validate with one round-trip (write → read → use during a `/refactor-deepen` invocation) before declaring done. |
| Plan artifact shape proves wrong on first real refactor | Medium | Low | Deliberately minimal. First in-the-wild use is the validation. Iteration expected. |
| Adding `/refactor-deepen` degrades skill routing | Medium | Medium | Razor-sharp lead-sentence ("deepening lens, not generic refactoring"). Negative routing: "Not for renames, extractions, or generic restructuring (use `/implement`)." |
| Parallel sub-agents converge accidentally without opposing-constraint priming | Medium | Low | Accept as a tradeoff for honest variety. Fallback if observed: spawn a 4th agent or rerun with different seeds, not introduce priming. Document this fallback in `references/interface-design.md`. |
| INTERFACE-DESIGN parallel sub-agents run expensive on large codebases | Low | Medium | Pattern is opt-in inside grilling loop, not auto-triggered. 3-agent default is the cost ceiling. Cost documented in skill body. |
| `/architecture` evolution conflicts with existing ADR generation flow | Low | Medium | Glossary side-effects are additive and opt-in mid-interview, not on critical path of ADR creation. ADR template unchanged. |
| Codex review hook duplicates work users already do via `/codex:rescue` | Low | Low | Opt-in; skip if user prefers manual. |
| CLI verb policy (propose/stabilize/upsert) confuses skill authors | Low | Medium | Each verb's intent is documented in CLI help text and in `references/glossary-mutation-policy.md` (one-page reference). Skills explicitly link to it. |

## Open Questions

- [ ] Should `loaf kb glossary check <term>` return all known aliases-to-avoid, or only when the queried term *is* an avoided alias? **Working assumption:** return both — definition if canonical, "this is an avoided alias for X" if not, "unknown" if absent.
- [ ] Should the shared `grilling.md` template be read by skills as a reference, or expanded inline at build time into each consuming skill? **Working assumption:** read as reference (mirrors Loaf's existing shared-template pattern, e.g., session.md).
- [ ] Should the sentinel vocabulary test be automated (e.g., a script that diffs `/refactor-deepen` output against a known-good fixture) or manual (human grades fidelity once per Track B)? **Working assumption:** manual once for go/no-go; automation deferred until drift observed in production.
- [ ] When `loaf kb glossary upsert` is called with `--avoid <list>` that conflicts with an existing canonical term in that list, fail or auto-rewrite? **Working assumption:** fail with explicit "term X already canonical; use a different surface or `stabilize`/`demote` first."

## Test Conditions

- [ ] `loaf build` succeeds with new skill, shared template, sidecar files, glossary CLI subcommands
- [ ] `npm run typecheck` passes
- [ ] `loaf kb glossary upsert <term> --definition <d> --avoid <a>` writes valid frontmatter; second upsert for same term updates rather than duplicates
- [ ] `loaf kb glossary stabilize <term>` promotes a term from `## Candidates` to canonical section; fails if term not in candidates
- [ ] `loaf kb glossary check <known-term>` returns canonical definition and aliases; `check <avoided-alias>` returns "avoided, use <canonical>"; `check <unknown>` returns "unknown"
- [ ] `loaf kb glossary upsert/stabilize/propose` fail fast in Linear-native mode with explicit error message; `list/check` work in both modes
- [ ] `/refactor-deepen` is invokable and produces a numbered candidate list within a single conversation turn
- [ ] `/refactor-deepen` reads `docs/knowledge/glossary.md`, `docs/decisions/ADR-*.md`, and `ARCHITECTURE.md` before presenting candidates (verifiable via session journal)
- [ ] `/refactor-deepen`'s grilling loop produces a `.agents/plans/<YYYYMMDD-HHMMSS>-*.md` file with the minimal shape filled out
- [ ] **Sentinel vocabulary test (Track B go/no-go):** Pick one shallow Loaf module, invoke `/refactor-deepen`, manually grade output for vocabulary fidelity. Pass condition: zero drifted-term occurrences (no "boundary/service/component/layer" where the source taxonomy has a precise term). Fail condition triggers Critical Rules iteration before declaring Track B done.
- [ ] When `/refactor-deepen` introduces a new domain term, it is added via `loaf kb glossary upsert`
- [ ] `/refactor-deepen`'s INTERFACE-DESIGN phase spawns exactly 3 sub-agents with identical briefs (no priming) and presents three distinct designs
- [ ] `/architecture` reads existing glossary at start; challenges term drift if used inconsistently during interview (manual review of one full invocation)
- [ ] `/architecture` invokes `loaf kb glossary stabilize` or `upsert` when a load-bearing term surfaces during an ADR interview
- [ ] Shared `grilling.md` template distributed to `architecture` and `refactor-deepen` skills at build time (verifiable in `dist/` and `plugins/loaf/skills/`). `/shape` distribution deferred per `20260501-231923-shape-glossary-evolution-deferred`.
- [ ] Glossary roundtrip: term added by `/refactor-deepen` → read by next `/architecture` invocation → referenced in subsequent ADR
- [ ] When `codex` plugin present, `/refactor-deepen` offers Codex review at end of grilling loop; when absent, skill terminates cleanly without offer
- [ ] All new reference files >100 lines have `## Contents` TOC
- [ ] Skill description fits in 250 chars first sentence with action-verb start, includes negative routing
- [ ] Strategic tensions documented in `STRATEGY.md` referencing the four captured idea IDs

## Priority Order

Tracks ship in this order. If scope needs cutting, drop from the end.

1. **Track A — Grilling protocol + glossary KB + `loaf kb glossary` CLI verbs + `/architecture` evolution.** Foundation. Go/no-go: `/architecture` invocation produces a glossary entry via `loaf kb glossary upsert`, then a subsequent `/architecture` reads it via `check` and challenges drift correctly. CLI commands work in local mode and fail fast in Linear-native. Shared template distribution verified in `dist/`.
2. **Track B — `/refactor-deepen` skill + plan artifact template.** Depends on A for glossary mutation. Go/no-go: one full `/refactor-deepen` invocation on Loaf itself produces a usable PLAN file with vocabulary discipline maintained (sentinel test passes), including parallel sub-agent INTERFACE-DESIGN phase with 3 unprimed agents producing distinct designs.
3. **Track C — Codex review opt-in for `/refactor-deepen`.** Polish. Drop if scope tightens. Go/no-go: plugin presence detection works; opt-in offer fires on one invocation with `codex` plugin installed.

## Changelog

- 2026-05-02 — Session feat/refactor-deepen-grilling-glossary archived: 8 decision(s) extracted
  [2026-05-02 13:00] decision(refactor-deepen): plans use YYYYMMDD-HHMMSS-slug naming, not PLAN-NNN — plans are temporal records like sessions/ideas, not contracts like specs/tasks; eliminates race-condition follow-up
  [2026-05-02 15:39] decision(release): pause to find and fix Loaf bug that sets local git user.name = Test, then rewrite authorship on this branch
  [2026-05-02 15:51] decision(testing): replace git config writes in tests with -c flag per-command identity (Option 2 — command-focused, not env-focused)
  [2026-05-02 02:11] decision(glossary): grilling.md template must NOT name verb-specific commitment levels — consuming skill owns mutation policy per SPEC-034; remove propose/stabilize/upsert from shared template
  [2026-05-02 02:11] decision(glossary): make splitSections and parseTerms fence-aware (track ``` and ~~~ state) and reject sparse files / non-empty preamble strictly to preserve lossless round-trip
  [2026-05-02 02:23] decision(refactor-deepen): INTERFACE-DESIGN sub-agents use researcher profile by default — read-only matches design-exploration use case, structured-report contract maps to "interface + sketch + tradeoffs" output; implementer documented as opt-in alternative when probe code is needed
  [2026-05-02 02:25] decision(glossary): list and check verbs must not call ensureGlossaryExists — file creation is lazy on first canonical write only; missing glossary surfaces "no entries" / "unknown" without side effects
  [2026-05-02 17:04] decision(release): v2.0.0-dev.37 — PR #45 opened with curated CHANGELOG, awaiting squash merge
