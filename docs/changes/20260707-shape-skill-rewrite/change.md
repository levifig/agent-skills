---
change: shape-skill-rewrite
created: 2026-07-07
branch: shape-skill-rewrite
---

# Shape Skill Rewrite — Fog-Routed Shaping for the Change Model

## Problem

The primary shaping surface still teaches the retired model. `content/skills/shape/SKILL.md` walks agents through SPEC-NNN ID generation, `.agents/specs/` file creation, and a breakdown handoff — all superseded by the TRANSITIONAL note, which means every `/shape` invocation starts from guidance that loses to a footnote. Three gaps compound it:

- **Technique selection is unowned judgment.** Shaping has grilling-style interviews, brainstorms, research spikes, and prototypes available, but nothing says which unknown gets which technique. Agents choose by vibes.
- **No step surfaces unknown unknowns.** Interviews can only clear questions someone already knows to ask. Shaping in unfamiliar territory has no named reconnaissance move, so blind spots survive into implementation where they are expensive.
- **The CLI boundary is untaught.** The pilot shipped `loaf change init/check`, but no skill explains when to call them, how to read violations vs. derived executability, or that `--require-executable` is the implement preflight.

Breakdown independently duplicates judgment (pilot Decision 5): the agent that decomposes the work is the agent that implements it, and decomposition-for-execution ordering buries the decisions a reviewer most needs to see first.

## Hypothesis

A shape skill rewritten around the Change model — with the fog register as a dispatch table, a blindspot pass ahead of the interview, and implementation units ordered by likelihood-of-change — produces Changes whose unknowns are cleared by the cheapest adequate technique and whose review hits the tweakable decisions first. If that holds, the largest piece of guidance debt left by the pilot is retired, and the narrowing protocol becomes something an agent executes rather than a convention it must remember.

## Scope

**In**

- Rewrite `content/skills/shape/SKILL.md` (and sidecars/templates guidance) around the hybrid Change model: produce a Change, never a numbered spec; teach the `loaf change init/check` boundary; offer the draft PR at shaping (opt-in, pilot Decision 21a).
- The fog register: Open Questions entries carry a quadrant tag and a route (see Planning Contract). Guidance convention only — never parsed by `check`.
- Blindspot pass as the named, conditional opening move, sequenced before grilling.
- Grilling import from the mattpocock review evidence: one question at a time via `AskUserQuestion`, recommended answer per question, architecture-changing answers first.
- Absorb breakdown's decomposition judgment as a shaping step, re-principled as ordering-for-review: likelihood-of-change first, mechanical work collapsed at the bottom.
- Reaction-artifact guidance for unknown-known routing (design variants, mocks — HTML in `research/`, pilot Decisions 20/21e).
- Instantiate the pilot's Critique Gate as the final shaping step, before `check` and the PR offer.
- Delete shape's spec-first machinery — SPEC-ID generation, the spec lifecycle state machine, the breakdown handoff — per the dead-weight adjudication in the blast-radius findings.
- Ship `loaf-reference` with this branch (Decision 10): rename from `cli-reference`, thin the generated catalog into an operational router with a compact command index, and add static `references/` — including the guided config-maintenance workflow (`loaf config check --json` → explain findings → `--fix` safe repairs → targeted questions for project-owned choices → re-validate → log) that answers "is this project's Loaf config up to date, and what do I fix?". Old name retired via the install deprecation manifest.

**Out** (deferred, not rejected)

- Quiz merge gate and deviations harvest — ride the conversion-and-guidance-sweep Change, which owns ship. Recorded here so harvest picks them up: the walkthrough (passive) and quiz (active) are one artifact with two reading modes; deviations are `decision(<scope>)` journal entries collected at ship.
- Deleting the breakdown skill file and sweeping spec-first language repo-wide — the sweep's job (pilot Decision 4 ordering); this rewrite merely stops shape from referencing breakdown.
- Routing-target gaps in brainstorm/idea/triage/research surfaced by this shaping's blindspot pass — recorded in `research/blindspot-routing-targets.md`, owned by skill-surface-tightening.
- Implement/ship interface wiring for the Change model — input detection for change slugs, the `--require-executable` preflight, the Verification Contract as ship's merge gate, deviation-logging conventions — surfaced by this shaping's blindspot pass (`research/blindspot-implement-ship.md`), owned by the sweep; captured as sparks so they survive this Change's fate (pilot D17 two-lane rule).
- The full 34-skill blindspot audit — the tightening Change's opening move.

**Cut** (explicitly rejected)

- A mandatory blindspot pass. It is offered when territory is unfamiliar, skipped when the shaper is the domain expert — a conditional default was already rejected as hidden judgment (pilot Decision 21a's reasoning, applied consistently).
- New CLI surface (`loaf fog`, quadrant linting). The register is document structure; no ceremony exists that a command would serve — the name-the-ceremony rule holds.
- Check-enforced quadrant tags. `loaf change check` never parses Open Questions prose.
- Any technique producing decisions instead of fog entries — reconnaissance, not orders.

## Observable Workflow

```text
/shape <messy input>
  → gather context: journal, sparks, strategic docs, prior Changes
  → unfamiliar territory? offer blindspot pass → fog entries (tagged UU→KU as they get named)
  → fog register routes each entry:
      [KU] known unknown        → grilling (architecture-changing answers first) or research spike
      [UK] unknown known        → reaction artifact: variants/mock in research/, react and pick
      [UU] suspected blind spot → blindspot pass / reconnaissance
  → loaf change init <slug> on branch <slug>
  → Planning Contract + Implementation Units, ordered by likelihood-of-change
  → loaf change check   (violations gate; executability derived, reported, not forced)
  → offer draft PR (opt-in)
```

The shaping session ends with a Change whose remaining fog entries each name their owner: a section, a spike, or a follow-up.

## Rabbit Holes and No-Gos

- **Do not rebuild the interview as a form.** Grilling is one question at a time with a recommended answer, not a questionnaire artifact. The moment it becomes a template to fill, it stops finding anything.
- **Do not let the fog register become a status board.** Entries resolve into Decisions or Planning Contract sections; the diff is the record. No counts, no burndown, no lifecycle.
- **Do not expand into ship's territory.** Walkthrough/quiz/harvest guidance belongs to the sweep. This Change touches ship zero times.
- **Do not gold-plate the SKILL.md.** The <500-line rule stands; routing tables that read once belong inline, procedures that read repeatedly may earn `references/`.

## Decisions

Provenance: 1–7 accepted 2026-07-06/07 in the Field Guide evaluation conversation (user pre-approved the folded proposal); they refine the converged narrowing protocol (journal decisions `379b76abd8691ba5502256ed`, `a5184c5d77bbe0d12b4590fd`) and the pilot's Follow-ups entry. 8–9 accepted 2026-07-07 from this shaping's own blindspot pass (reports in `research/`).

1. **The fog register is a dispatch table, not documentation.** Each named unknown carries a quadrant class that routes it to a technique deterministically. This forecloses technique-by-vibes and is the skill-level application of the pilot's skill/CLI seam: the taxonomy owns routing so the agent doesn't re-derive it.
2. **The blindspot pass precedes grilling.** Interviews clear only known unknowns; a pass that names blind spots must run first or its findings arrive too late to interview about. Offered, never imposed (Cut list).
3. **Grilling orders questions by architectural impact.** "Prioritize questions where the answer would change the architecture" — the Field Guide heuristic sharpens the mattpocock import's one-question-at-a-time mechanic.
4. **Implementation units are ordered by likelihood-of-change.** Data models, interfaces, and user-facing flows lead; mechanical refactors collapse at the bottom. This is breakdown's absorbed judgment re-principled for the draft→ready review (pilot Decision 15): the reviewer's attention lands on what is most likely to need changing.
5. **Techniques produce fog entries, not decisions.** Blindspot passes, spikes, and reaction artifacts return named unknowns and evidence for the human to adjudicate — reconnaissance-not-orders survives the rewrite intact.
6. **Quadrant tags are convention, never contract.** `loaf change check` stays out of prose; a stale tag costs nothing and polices nothing.
7. **Imports are implemented against committed evidence.** The grilling and prototype semantics come from pointing the implementing agent at `research/mattpocock-review/` in the pilot's folder — source as reference, not re-description in a prompt (the Field Guide's references pattern, applied to this Change's own implementation).
8. **Fog routes name in-session techniques, never skill invocations.** The blindspot pass proved the destination skills' contracts don't fit the handoff: research re-interviews an already-scoped question and writes evidence to `.agents/reports/` instead of the Change's `research/`; brainstorm forces a strategic frame onto Change-local questions and sends resolutions forward into intake instead of back into the session; no skill is the blindspot-pass destination. Grilling, reaction artifacts, spikes, and blindspot passes are moves shape executes itself; fixing the skills' contracts belongs to tightening.
9. **The critique gate becomes a shaping step.** The pilot's Critique Gate section is target behavior with no owner in the current skill — an agent won't know to interrogate its own scope, boundary placement, or smuggled status words before finalizing. The rewrite instantiates it as the last move before `loaf change check` and the PR offer.
10. **`loaf-reference` ships with this Change** *(user + Codex converged, session 019f2cdf tail, 2026-07-06/07; user direction to ride this branch)*. The acceptance contract: rename `cli-reference` → `loaf-reference` ("cli" is too generic — this is the Loaf operating manual for agents); keep it non-user-invocable; thin the ~1,450-line generated command catalog into a short operational router — discovery via `loaf --help` / `loaf <cmd> --help`, prefer `--json` surfaces for diagnosis, `--fix` only for safe repairs, ask the user for project-owned choices, never hand-edit managed hook files, re-run checks after changes, log decisions — with a compact generated command index for drift-proofing and static `references/` for configuration, command routing, and troubleshooting; retire the old name through the install deprecation manifest; never let it become a user workflow skill. Two review fixes landed en route (missing `loaf doctor` metadata, Change-blind decision guide) and survive inside the thinned output.

## Planning Contract

### Fog register format

Open Questions entries take the form:

```text
- [KU] <the unknown> → <route: grilling | research spike | owner section>
- [UK] <the recognize-it-when-seen criterion> → reaction artifact in research/
- [UU] <the suspected blind area> → blindspot pass over <territory>
```

Lifecycle: an entry resolves by becoming a Decision, a Planning Contract subsection, or a follow-up — visible in the diff, never silently deleted. A `[UU]` that gets named becomes a `[KU]` or `[UK]` and re-routes. The rewrite documents this format in shape's guidance; the change template gains at most a one-line comment pointing at it.

### Blindspot pass

Trigger: shaping in territory where the shaper lacks fluency — a new domain, an unfamiliar subsystem, a first collaboration. The skill offers it; the user declines when they are the expert. Prompt shape: *"what would I not know to ask here — codebase history, domain conventions, prior art, potholes"* against the specific territory, with the shaper's disclosed starting point (the Field Guide's context-disclosure requirement). Deliverable: fog entries with quadrant tags — never conclusions, never scope decisions.

### Grilling

One question at a time through `AskUserQuestion`, each with a recommended answer (mattpocock semantics), ordered by architectural impact (Decision 3). Stop condition: no unrouted `[KU]` entries remain, or answers stop changing the contract. Grilling never opens the session — context gathering and the blindspot-pass offer come first (Decision 2). Harnesses without `AskUserQuestion` degrade to one inline question per message, same semantics — the current skill assumes the tool everywhere and never says so (blindspot finding, shape/SKILL.md:30).

### Decomposition (breakdown absorbed)

What survives from breakdown: dependency awareness, granularity judgment (made autonomously, not deferred to the user), and acceptance-criteria thinking — now expressed as Implementation Units and the Verification Contract inside the Change. What dies with it: task-file minting, ID allocation, estimate fields, and ordering-for-execution as the default presentation. Units are commit-boundary guides ordered by likelihood-of-change; sequencing constraints that genuinely exist are stated in prose, not implied by list order.

### CLI boundary teaching

The rewritten skill documents, tersely: `loaf change init <slug>` scaffolds on the branch named by the slug; `loaf change check` output splits violations (always fail) from derived executability (report, not gate); `--require-executable` is implement's preflight and CI's non-draft gate; the folder date comes from creation day, the branch carries the bare slug. The skill teaches *reading* the CLI, not wrapping it.

### Blast-radius findings

Three read-only reviewers swept the blast radius on 2026-07-07; raw reports live in this folder's `research/` (`blindspot-shape-breakdown.md`, `blindspot-routing-targets.md`, `blindspot-implement-ship.md`). Adjudication:

**Changes this rewrite** (beyond the pre-approved scope):

- *Source polymorphism.* Step 1 recognizes only "idea file, problem description, or requirement area"; the rewrite teaches the pilot's full input set — journal entries, sparks, ideas, brainstorms, Linear issues, PR conversations, plain conversation.
- *Interview stop condition.* The current interview has none; the fog register supplies it (Grilling subsection above).
- *Misalignment selector.* On strategic misalignment the current text offers surface/adjust/note with no selector — the rewrite pins it: surface to the user; never silently adjust the idea.
- *Absent strategic docs.* The skill assumes VISION/STRATEGY/ARCHITECTURE exist at bare paths; most consumer projects have none. The rewrite states the degraded path: shape against the journal, recent Changes, and the conversation, and say so in the Change's Source Inputs.
- *Defined terms.* "Rabbit holes" and "no-gos" are load-bearing and undefined; the rewrite defines them where first used (one line each — full glossary work stays with the sweep, pilot D21d).
- *Template orphan repair.* `templates/change.md` exists in the skill folder but the skill body never references it; `templates/spec.md` carries V1-banned status frontmatter. The rewrite points at the change template and drops the spec path from the body.
- *Surviving value imported, not re-derived.* Breakdown's Right Size Test and right-sizing rules, per-unit verification discipline, own-the-decisions granularity, shape's solution-direction altitude, and note-tensions-don't-fix all survive verbatim in spirit — U3's checklist.

**Recorded for other Changes** (Out; sparks capture the abandonment-surviving ones):

- Implement cannot receive a Change: input detection mints a bogus `loaf task` from a change slug, the wave planner reads `depends_on` that Units don't have, close-out runs `loaf spec archive`, and the pilot-named `--require-executable` preflight is absent. Ship's evidence review false-positives on change.md as "docs describing future work," and its verification never reads the Change's Verification Contract. → sweep.
- Routing-target contract gaps and undefined SQLite-vs-fallback assumptions across brainstorm/idea/triage/research. → tightening.
- Spec-residue inventory across idea, triage, research, and their templates (SPEC-terminal lifecycles, `loaf spec list` calls, status frontmatter in report templates). → conversion/sweep.

### Sequencing

This rewrite lands before the conversion pass and sweep — the TRANSITIONAL note already routes new work to Changes, so the primary surface should teach the model it routes to. The sweep later removes breakdown, sweeps spec language repo-wide, and owns the convergence grep; nothing here strands in-flight `loaf spec` work.

## Implementation Units

Ordered by likelihood-of-change (Decision 4, dogfooded):

- **U1 — Fog register and routing table.** The new SKILL.md sections most likely to be tweaked in review: register format, quadrant routing, technique-produces-fog rule.
- **U2 — Blindspot pass and grilling.** Trigger conditions, prompt shapes, ordering heuristic, stop conditions, `AskUserQuestion` fallback; implemented against `research/mattpocock-review/` per Decision 7.
- **U3 — Decomposition guidance.** Breakdown absorption: units, ordering-for-review, verification-contract authoring; the surviving-value checklist from the blast-radius findings imported verbatim in spirit.
- **U4 — Change contract and CLI boundary.** The model teaching: source polymorphism, init/check reading, evidence lands in the Change's `research/` (never `.agents/reports/` for shaping work), critique gate, opt-in draft PR, branch-at-shaping, durable-outputs timing, absent-strategic-docs path.
- **U5 — Mechanical close-out.** Frontmatter description with negative routing (not brainstorm, not idea), sidecar review, related-skills links (breakdown reference dropped), `hooks.yaml` check, `loaf build`, routing eval, `templates/spec.md` disposition per the fog entry.
- **U6 — `loaf-reference` rider (Decision 10).** Landed at shaping time, ahead of U1–U5, in three slices: (a) generator review fixes — `loaf doctor` metadata, Change-first decision-guide entry; (b) the rename — generator name/path, sidecar, README/eval/release-verification references, `config/deprecations.json` retirement of `cli-reference`, rebuilt targets with stale copies removed; (c) the thinning — short operational SKILL.md plus compact generated command index, static `references/configuration.md` (guided config-maintenance workflow), `command-routing.md`, `troubleshooting.md`.

## Verification Contract

Executable (machine-checkable):

- **V1.** `loaf change check docs/changes/20260707-shape-skill-rewrite` exits zero and reports this document executable before implementation begins.
- **V2.** After the rewrite: `loaf build` succeeds; `npm run eval:routing` passes with the new shape description (the routing-eval infrastructure is the measurement tool the pilot preserved for exactly this).
- **V3.** `grep -n "SPEC-" content/skills/shape/` returns no instruction that mints or references numbered specs — the shape-produces-a-Change-without-a-spec criterion this Change inherited from the pilot's Verification Contract.
- **V4.** `npm run test` (Go suite) passes — no CLI behavior changes expected; the gate proves it.

Human review:

- **H1.** The rewritten skill explains the CLI boundary accurately (inherited from the pilot) — a reader who has never seen the Change model can run init/check and read the output correctly.
- **H2.** Grilling and blindspot-pass guidance produce fog entries, not decisions, in a dogfood shaping run.
- **H3.** The next real Change shaped with the new skill reaches its draft→ready review with tweakable decisions leading the Planning Contract — the ordering rule observed working, not asserted.

## Definition of Done

- SKILL.md and sidecars rewritten; V1–V4 pass; H1–H2 confirmed in review; the PR is reviewed and squash-merged with this folder on main.
- H3 is confirmed on the first Change shaped after merge — a trailing check, not a merge gate.
- Deferred items (quiz gate, deviations harvest, routing-target gaps) are recorded in Out/fog for the sweep and tightening Changes to harvest — nothing lives only in conversation.
- Journal carries the shaping and implementation decision trail.

## Durable Outputs

- The rewritten `content/skills/shape/SKILL.md` and its built targets — the skill itself is the distributable artifact.
- A `docs/knowledge/` note on fog routing is deliberately **not** created here: vocabulary pinning belongs to the sweep; until then this Change's Decisions section is the reference.

## Open Questions

- ~~[UK] Right altitude for the new SKILL.md — inline routing table vs. `references/` split~~ → resolved in implementation: register, routing table, and process stayed inline (164 lines); technique mechanics went to six `references/` files. Deviation noted honestly: the implementer judged directly instead of producing both drafts as the entry asked — a fully-inline draft would have breached the 500-line rule, and orchestrator review confirmed the altitude. The dual-draft rite was skipped, not the judgment it protects.
- ~~[KU] Does `eval:routing` cover shape's confusable neighbors (brainstorm, idea)?~~ → resolved: it didn't; U5 added idea-shape and brainstorm-shape conflict probes (115 → 119 cases), and shape's own cases updated from spec to Change vocabulary.
- ~~[KU] Should the change template's Open Questions comment mention quadrant tags?~~ → resolved with the recommended answer: one-line fog-register comment added to the template, explicitly marked convention-never-parsed. PR review can veto.
- ~~[KU] Delete `templates/spec.md` with this rewrite, or leave it for the conversion pass?~~ → resolved with the recommended answer: deleted; `shared-templates` and build config verified silent on it, and the conversion pass does not consume shape's template.
- ~~[UU] What agents actually misread in the current skill text~~ → resolved by the 2026-07-07 blindspot pass; adjudicated in Blast-radius findings, raw reports in `research/`.

The register is empty; remaining review is the human pass at the draft→ready flip (H1–H2) and the trailing H3 check on the next shaped Change.

## Source Inputs

- The pilot Change `docs/changes/20260704-shape-first-change-workflow/change.md` — its Follow-ups entry for this rewrite; Decisions 5, 12, 15, 20, 21a/e inherited here.
- Narrowing-protocol journal decisions `379b76abd8691ba5502256ed` and `a5184c5d77bbe0d12b4590fd` (authority gradient, metamorphosis checklist, reconnaissance-not-orders).
- Thariq, "A Field Guide to Fable: Finding Your Unknowns" (x.com/trq212/status/2073100352921215386, 2026-07-03) and its worked-artifacts page — the quadrant taxonomy, blindspot pass, architecture-first interview ordering, and tweakable-plan ordering. **Confirming input, flagged as such** per the pilot's survivorship-bias meta-finding; it is the third independent convergence on map/fog after Wayfinder and the SDD Blueprint.
- `research/mattpocock-review/` in the pilot's folder — grilling and prototype semantics (Decision 7's implementation reference).
- The 2026-07-07 blindspot pass over the blast-radius skills (three read-only reviewers) — raw reports in `research/blindspot-shape-breakdown.md`, `research/blindspot-routing-targets.md`, `research/blindspot-implement-ship.md`; adjudicated in the Planning Contract.
- The 2026-07-06/07 conversation where the Field Guide harvest was evaluated, mapped, and approved.
- Codex session `019f2cdf-22d5-7551-9c04-152c25a9bc82` (tail, 2026-07-06) — the `loaf-reference` acceptance contract: rename rationale, thin-router shape with `references/{configuration,command-routing,troubleshooting}.md`, the guided config-maintenance workflow, the no-workflow-skill boundary, and the ship-with-this-Change decision (also logged to the journal from that session).
