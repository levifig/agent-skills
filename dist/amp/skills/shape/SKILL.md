---
name: shape
description: >-
  Shapes messy input into a bounded, reviewable Change under
  docs/changes/YYYYMMDD-slug/, validated by loaf change check. Runs a fog-routed
  narrowing protocol — gather context, an optional blindspot pass, grilling
  for known unknowns, reaction artifacts for unknown-knowns — decomposes
  Implementation Units ordered by likelihood-of-change, runs a critique gate,
  and offers an opt-in draft PR. Use when the user asks "shape this," "turn this
  into a Change," or an idea has accumulated enough constraints to bound.
  Recognizes journal entries, sparks, ideas, brainstorms, Linear issues, PR
  conversations, and plain conversation as input. Produces a Change, never a
  numbered spec or task file. Not for quick capture (use idea) or open-ended
  divergent thinking before scope exists (use brainstorm).
version: 2.0.0-alpha.6
---

# Shape

Turn messy input into a bounded, reviewable Change.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Related Skills
- Topics

**Input:** $ARGUMENTS

---

## Critical Rules

1. **Log invocation first** — `loaf journal log "skill(shape): <input being shaped>"` before doing anything else.
2. **Produces a Change, never a spec** — the artifact is `docs/changes/YYYYMMDD-slug/change.md`. No sequentially-numbered spec file, no task entity, no status-like frontmatter.
3. **The fog register routes, you don't guess** — every named unknown carries a quadrant tag that dispatches it to exactly one technique (see Quick Reference). Technique-by-vibes is the failure mode this replaces.
4. **Blindspot pass precedes grilling, offered not imposed** — run it when the territory is unfamiliar; skip it when the shaper is the domain expert. Never impose it as a mandatory step.
5. **Grill one question at a time, with a recommendation** — via `Amp UI input` (or one inline question per message on harnesses without it — same semantics). Never a form to fill in one pass. Order by architectural impact: questions whose answer would change the architecture go first, cosmetic questions last.
6. **Techniques return fog entries, not decisions** — blindspot passes, grilling, and reaction artifacts hand back named unknowns and evidence for the human to adjudicate. Reconnaissance, not orders.
7. **Own the decomposition** — decide Implementation Unit boundaries and granularity autonomously (absorbed from the retired breakdown step); ask only when two orderings carry genuinely different trade-offs.
8. **Order units by likelihood-of-change** — data models, interfaces, and user-facing flows lead; mechanical work collapses at the bottom, so review attention lands on what's most likely to need changing.
9. **Surface misalignment, never silently adjust** — when the idea conflicts with strategic docs, prior Changes, or the journal, tell the user and let them decide. Don't quietly reshape their idea.
10. **Critique before finalizing** — run the Critique Gate as the last shaping step, before `loaf change check` and the PR offer.
11. **Get approval before `loaf change init`** — don't scaffold the folder without explicit confirmation of scope.
12. **Log the outcome** — `loaf journal log "decision(shape): <slug> shaped — N units, N open fog entries"`.

---

## Verification

- `docs/changes/YYYYMMDD-slug/change.md` exists with Problem, Hypothesis, Scope, Observable Workflow, and Rabbit Holes and No-Gos all non-empty (the Product Contract set `loaf change check` requires)
- Every Open Questions entry carries a quadrant tag (`[KU]`, `[UK]`, or `[UU]`) and a route
- `loaf change check` reports zero violations; its executability report was read, not ignored
- The Critique Gate ran, and its answers changed the document where they applied — not just spoken aloud
- No sequentially-numbered spec or task file, and no status-like frontmatter (`readiness`, `status`, `state`), exists anywhere in the Change

---

## Quick Reference

### Fog register format

Open Questions entries take one of three forms:

```text
- [KU] <the unknown> → <route: grilling | research spike | owner section>
- [UK] <the recognize-it-when-seen criterion> → reaction artifact in research/
- [UU] <the suspected blind area> → blindspot pass over <territory>
```

An entry resolves by becoming a Decision, a Planning Contract subsection, or a named follow-up — visible in the diff, never silently deleted. A `[UU]` that gets named becomes a `[KU]` or `[UK]` and re-routes through the table below.

### Quadrant routing

| Tag | Meaning | Routes to |
|-----|---------|-----------|
| `[KU]` known unknown | A question you can state precisely | [Grilling](references/grilling.md) (architecture-changing answers first) or a research spike |
| `[UK]` unknown known | You'd recognize the right answer if you saw it, but can't state it yet | [Reaction artifact](references/reaction-artifact.md) — a variant or mock in `research/`, react and pick |
| `[UU]` suspected blind spot | Unfamiliar territory; you don't yet know what you don't know | [Blindspot pass](references/blindspot-pass.md) |

No route names a skill invocation. Research re-interviews an already-scoped question and writes to `.agents/reports/`; brainstorm forces a strategic frame onto a Change-local question and sends resolutions to intake. Shape runs all three techniques itself, in-session, and writes evidence into the Change's own `research/` — never `.agents/reports/`.

### Defined terms

- **Rabbit holes** — tempting expansions of scope that would consume disproportionate effort for marginal value; name them so nobody wanders in unknowingly.
- **No-gos** — approaches explicitly forbidden for this Change, stated so they aren't silently reconsidered mid-implementation.

### Source inputs recognized

Step 1 reads whatever `$ARGUMENTS` names, or asks. Recognized sources: a journal entry (cite by ID), a spark, an idea, a brainstorm document, a Linear issue, a PR conversation, a prior Change, or plain conversation with no artifact behind it yet.

---

## Process

### Step 1: Gather Context

Parse `$ARGUMENTS` against the source inputs above. Read the journal (`loaf journal recent` / `search`) for related history, and check for a prior Change touching the same area. When VISION.md, STRATEGY.md, and ARCHITECTURE.md exist, read them for strategic fit. Most consumer projects don't have them yet — when absent, shape against the journal, recent Changes, and the conversation instead, and say so in the Change's Source Inputs.

### Step 2: Evaluate Strategic Fit

When strategic docs exist, check: does this advance the vision, serve the target personas, fit technical constraints, avoid conflicting with in-flight Changes? On misalignment, **surface it to the user — never silently adjust the idea**. The user decides whether to proceed, narrow scope, or defer to `/reflect` after this ships.

### Step 3: Name the Change and Initialize

Once the shape of the work is nameable, confirm scope with the user, then:

```bash
loaf change init <slug>
```

This scaffolds `docs/changes/YYYYMMDD-slug/change.md` from [the Change template](templates/change.md) and prints the branch to use — **it does not switch branches**. Create or switch to it yourself (`git switch -c <slug>`) before committing anything: branch-at-shaping, not deferred to implementation. A Change grows toward executability from here: the flat Product Contract sections (Problem, Hypothesis, Scope, Observable Workflow, Rabbit Holes and No-Gos) fill in as understanding solidifies through the steps below, not all at once. See [references/cli-boundary.md](references/cli-boundary.md) for what `init` writes and how `check` reads it.

### Step 4: Narrow the Unknowns

Offer the blindspot pass when the territory is unfamiliar (a new domain, an unfamiliar subsystem, a first collaboration) — skip it when the shaper is the expert. Its fog entries, and any others surfaced in interview, route by quadrant (Quick Reference above). Loop: grill `[KU]` entries, react to `[UK]` entries, run blindspot reconnaissance on `[UU]` entries until each gets a name and re-routes. Stop grilling when no unrouted `[KU]` entries remain or answers stop changing the contract. Entries still open at the end of the session are fine — each names its owner (a section, a spike, a follow-up).

### Step 5: Decompose into Implementation Units

Absorbed from the retired breakdown step — see [references/decomposition.md](references/decomposition.md) for the Right Size Test and per-unit verification discipline. Order units by likelihood-of-change; state real sequencing constraints in prose, never by list order alone.

### Step 6: Fill the Planning Contract

Write the free-form `###` subsections the work actually needs (approach, placement, risks, sequencing) inside the Planning Contract container. Its subsection names are yours; the container itself, plus Implementation Units, Verification Contract, and Definition of Done, is what `loaf change check` looks for. Durable Outputs stays forward-looking here — name what a final spec, ADR, or knowledge doc will need to capture, but don't write it now. Durable artifacts get created after implementation proves what's true, not during shaping.

### Step 7: Run the Critique Gate

Before finalizing, challenge the draft — see [references/critique-gate.md](references/critique-gate.md). Is scope still bounded, does every new command or state name its ceremony, is a status field creeping back in under another name, is the CLI/skill boundary drawn correctly, and could this be smaller and still deliver the Hypothesis?

### Step 8: Validate

```bash
loaf change check
```

Read violations (always block — fix them) separately from the executability report (derived, informational unless `--require-executable` is passed — that flag is implement's preflight and CI's non-draft gate, not shape's business). See [references/cli-boundary.md](references/cli-boundary.md).

### Step 9: Offer the Draft PR

Offer to push the branch and open a draft PR, using [the PR template](templates/pr.md) — opt-in, never automatic. `loaf change check` (with no `--require-executable`) plus `gh pr list` is the cross-branch index either way.

### Step 10: Log the Outcome

`loaf journal log "decision(shape): <slug> shaped — N units, N open fog entries"`.

---

## Related Skills

- **idea** — Quick capture; feeds into `/shape` once a concept has enough weight
- **brainstorm** — Open-ended divergent thinking before scope narrows enough to shape
- **implement** — Starts execution once a Change is structurally executable; this does not prove implementation completion
- **reflect** — Updates strategic docs after the shipped work proves what changed

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Blindspot pass | [references/blindspot-pass.md](references/blindspot-pass.md) | Deciding whether to offer reconnaissance, and how to prompt it |
| Grilling | [references/grilling.md](references/grilling.md) | Running the one-question-at-a-time interview for `[KU]` entries |
| Reaction artifacts | [references/reaction-artifact.md](references/reaction-artifact.md) | Resolving `[UK]` entries with a variant, mock, or prototype |
| Decomposition | [references/decomposition.md](references/decomposition.md) | Sizing and ordering Implementation Units |
| CLI boundary | [references/cli-boundary.md](references/cli-boundary.md) | Reading `loaf change init`/`check` output, or explaining `--require-executable` |
| Critique Gate | [references/critique-gate.md](references/critique-gate.md) | Self-challenging scope and boundaries before finalizing |
