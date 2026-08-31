---
name: shape
description: >-
  Shapes messy input into a bounded work contract on an authority ref —
  problem body, definition-of-done criteria, out-of-scope statement, and
  children when a criterion earns its own DoD — validated by loaf issue check.
  Use when the user asks "shape this," "turn this into an issue," or a diagnosed
  fix needs a contract. Produces a shaped ref-keyed contract — never a folder,
  plan document, or new internal LOAF-* row. Teaches fog graduation (park, then
  a ledger decision) and one-criterion sizing (verifiable alone, revertible
  alone). Not for quick capture (use idea), problem discovery that should author
  a brief first (use pitch), or open-ended divergent thinking (agent technique:
  explore / brainstorm — user entry routes to pitch).
version: 0.5.0
---

# Shape

Prepare a bounded, reviewable issue.

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

1. **Log invocation first** — Because shaping is a project-scoped event others may audit, `loaf journal log "skill(shape): shaping <topic> into linear:ENG-42"` before doing anything else. If no issue exists yet, log `skill(shape): shaping <topic>` and add the authority ref in the outcome entry.
2. **Produces a ref-keyed contract, never a folder** — Because bounded work keys to a provider-qualified authority ref and ships through PRs, the deliverable is the work contract on that ref: problem in the body, definition of done as `loaf issue dod` criteria, an explicit out-of-scope statement in the body, children via `loaf issue new --ref <child-ref> --parent <ref>` when a criterion earns its own DoD. Do not mint a second internal issue row. No plan document is committed. The PR body, if a PR is opened, is `loaf issue render` output.
3. **The fog register routes, you don't guess** — Because unknowns have different resolution techniques, every named unknown carries a quadrant tag that dispatches it to exactly one technique (see Quick Reference). Technique-by-vibes is the failure mode this replaces.
4. **Fog graduates instead of evaporating** — Because parked questions resurface as scope creep, a question not yet sharp enough is parked in the issue's `fog` field (`loaf issue new --ref <ref> --fog`). When it sharpens it becomes a ledger decision (`loaf issue new --kind decision`, no `--ref`), which is ready when it poses a sharp question (a `?` in the title or body). No plan required.
5. **Blindspot pass precedes grilling, offered not imposed** — Because reconnaissance cost should match unfamiliarity, run it when the territory is unfamiliar; skip it when the shaper is the domain expert. Never impose it as a mandatory step.
6. **Grill one question at a time, with a recommendation** — Because parallel questions hide trade-offs, use your harness's structured question tool if it has one (otherwise one inline question per message — same semantics). Never a form to fill in one pass. Order by architectural impact: questions whose answer would change the architecture go first, cosmetic questions last.
7. **Techniques return fog entries, not decisions** — Because the human adjudicates, blindspot passes, grilling, and reaction artifacts hand back named unknowns and evidence for the human to adjudicate. Reconnaissance, not orders.
8. **Decomposition is the tail** — Because premature splitting obscures the problem, a parent gets children only when its DoD needs more than one coherent slice. A criterion becomes a child the moment it earns its own DoD, via `loaf issue new --ref <child-ref> --parent <parent-ref>`. Own those boundaries autonomously; ask only when two orderings carry genuinely different trade-offs.
9. **Rideability is the higher-order sizing rule** — Every shaped milestone or work unit must be a complete, useful operator journey. Make its Rider, complete Journey, Entry point, observable Outcome, real Dogfood, Safety/integrity proof, Learning sought, and explicit Deferrals concrete in the existing body and criteria; no mandatory headings or storage schema. Then require that slice to be verifiable and revertible alone. Foundation-only layers stay within the immediate consuming slice. See [references/decomposition.md](references/decomposition.md) and [Rideable Increments](../foundations/references/rideable-increments.md).
10. **Window-fit remains a runtime heuristic** — Window-fit ("fits one fresh context window") is owned by implement, which may sub-decompose for execution via handoffs and scratchpad without changing the contract tree. Expand–contract remains the named exception for wide mechanical refactors.
11. **Surface misalignment, never silently adjust** — Because strategic drift hidden in a reshaped issue wastes implementer time, when the idea conflicts with strategic docs, prior issues, or the journal, tell the user and let them decide. Don't quietly reshape their idea.
12. **Critique before finalizing** — Because agents won't interrogate their own scope without a gate, run the Critique Gate as the last shaping step, before `loaf issue check <ref>`.
13. **A diagnosed one-line fix is two commands** — Because ceremony should match uncertainty, `loaf issue new --ref <ref>` with a body that states the problem and `Out of scope: …`, then one `loaf issue dod add <ref>`. No problem-space ceremony. Confirm scope with the user before minting a contract on anything larger.
14. **Log the outcome** — Because shaped issues are audit events, `loaf journal log "decision(shape): linear:ENG-42 shaped — N children, N open fog entries"`.

---

## Verification

- The issue body states the problem and contains an explicit out-of-scope statement (`out of scope`, case-insensitive — that substring is what `loaf issue check` reads)
- At least one definition-of-done criterion exists; V-tier criteria carry `--command` (and `--expect` when the check is more than exit 0); H-tier otherwise
- The contract concretely identifies all eight rideable-increment elements in natural prose and criteria; it is a complete small product rather than a component layer
- Every open unknown is either parked in create-time `fog`, held in the session register until it sharpens, graduated to a `--kind decision` child (or sibling) with a sharp question, or written into the body as a decided answer
- `loaf issue check <ref>` reports the issue shaped (delivery) or ready (decision). When children exist, coverage failures were fixed and containment orphans were filed as sibling backlog issues using the printed remedy
- Problem-boundary test applied: a discovered different problem becomes a new backlog issue, not another criterion on this one
- The Critique Gate ran, and its answers changed the issue where they applied

---

## Quick Reference

### Fog register format

Open unknowns take one of three forms. Keep the register in the session. Park what is still unsharp in `--fog` at create; after create, unsharp entries stay in the session register (edit cannot mutate `fog`). Graduate what is sharp to a decision child or sibling, and write decided answers into the body.

```text
- [KU] <the unknown> → <route: grilling | research spike | owner>
- [UK] <the recognize-it-when-seen criterion> → reaction artifact
- [UU] <the suspected blind area> → blindspot pass over <territory>
```

An entry resolves by becoming a decision child, a body paragraph, a criterion, or remaining parked in `fog` — visible on `loaf issue show`, never silently deleted. A `[UU]` that gets named becomes a `[KU]` or `[UK]` and re-routes through the table below.

### Quadrant routing

| Tag | Meaning | Routes to |
|-----|---------|-----------|
| `[KU]` known unknown | A question you can state precisely | [Grilling](references/grilling.md) (architecture-changing answers first) or a research spike |
| `[UK]` unknown known | You'd recognize the right answer if you saw it, but can't state it yet | [Reaction artifact](references/reaction-artifact.md) — a variant or mock, react and pick |
| `[UU]` suspected blind spot | Unfamiliar territory; you don't yet know what you don't know | [Blindspot pass](references/blindspot-pass.md) |

No route names a skill invocation. Research re-interviews an already-scoped question; brainstorm forces a strategic frame onto an issue-local question and sends resolutions to intake. Shape runs all three techniques itself, in-session, and writes the captured answer onto the issue — never into `.agents/reports/`.

### Defined terms

- **Rabbit holes** — tempting expansions of scope that would consume disproportionate effort for marginal value; name them in the out-of-scope statement so nobody wanders in unknowingly.
- **No-gos** — approaches explicitly forbidden for this issue, stated so they aren't silently reconsidered mid-implementation.

### Source inputs recognized

Step 1 reads whatever `$ARGUMENTS` names, or asks. Recognized sources: a brief from pitch, a journal entry (cite by ID), a spark, an idea, a brainstorm document, a Linear issue, a PR conversation, a prior issue, or plain conversation with no artifact behind it yet.

### One-line entry

A diagnosed fix that already has a problem and a done-check:

```bash
loaf issue new --ref linear:ENG-42 "Fix missing --json in list help" --body "issue list --help omits --json. Out of scope: rewriting other help pages."
loaf issue dod add linear:ENG-42 "issue list help names --json" --command "loaf issue list --help" --expect "contains \`--json\`"
loaf issue check linear:ENG-42
```

Two writes, then the readiness verdict. No grilling, no children, no files.

---

## Process

### Step 1: Gather Context

Parse `$ARGUMENTS` against the source inputs above. When a brief from pitch already frames the problem, restate it, confirm with the user rather than re-discovering, and keep later grilling on solution-space (how, boundaries, verification). When no brief exists, run full narrowing; pitch is the recommended front door for raw concepts, never a gate. Read the journal (`loaf journal recent` / `search`) for related history, and check for prior work touching the same area (`loaf issue frontier`, `loaf issue list`, `loaf issue tree`). When VISION.md, STRATEGY.md, and ARCHITECTURE.md exist, read them for strategic fit. Most consumer projects don't have them yet — when absent, shape against the journal, recent issues, and the conversation instead, and say so in the issue body.

### Step 2: Evaluate Strategic Fit

When strategic docs exist, check: does this advance the vision, serve the target personas, fit technical constraints, avoid conflicting with in-flight issues? On misalignment, **surface it to the user — never silently adjust the idea**. The user decides whether to proceed, narrow scope, or file the conflicting concern as its own backlog issue.

### Step 3: Name the Work and Write the Contract

Once the work is nameable, confirm scope with the user (skip this confirmation on the one-line path), then create the contract on a provider-qualified authority ref. Tracker-backed projects use `linear:<KEY>` after the Linear row exists (create that row through the Linear skill; do not mint an internal LOAF-* alias). Trackerless projects use `branch:<name>` or `pr:<number>`. Prefer creating after the first narrowing pass so `--fog` can carry remaining unsharp questions — the CLI writes `fog` only at create.

```bash
loaf issue new --ref linear:ENG-42 "Rotate auth tokens on a sliding window" \
  --body "Sessions never expire while the tab stays open, so a stolen cookie is valid indefinitely.

Out of scope: migrating existing sessions; third-party IdP support." \
  --fog "[KU] sliding-window length → grill; [UU] existing session-store conventions → blindspot pass"
```

Default kind is `delivery`; default status is `triage`. `--status` accepts `triage`, `backlog`, `todo`, `active`, or `done`. Use `--body -` or `--body-file <path>` for a longer body; `loaf issue edit <ref>` later **replaces** the body, it does not patch it.

A delivery contract is shaped when the body is nonempty (the problem), at least one criterion exists, and the body contains an explicit out-of-scope statement. Fill those as understanding solidifies — create can carry the first body; criteria come next.

Before finalizing, make the [rideable increment](../foundations/references/rideable-increments.md) concrete in that same body and criteria: Rider; complete Journey; Entry point; observable Outcome; real Dogfood; Safety/integrity proof; Learning sought; explicit Deferrals. These are required decisions, not required headings. If the proposed work is only infrastructure, name and include the immediate end-to-end slice that consumes it.

A discovered different problem is a new backlog contract, not a child of this one:

```bash
loaf issue new --ref branch:rewrite-session-store --status backlog "Rewrite the session store"
```

### Step 4: Narrow the Unknowns

Offer the blindspot pass when the territory is unfamiliar (a new domain, an unfamiliar subsystem, a first collaboration) — skip it when the shaper is the expert. Its fog entries, and any others surfaced in interview, route by quadrant (Quick Reference above). Loop: grill `[KU]` entries, react to `[UK]` entries, run blindspot reconnaissance on `[UU]` entries until each gets a name and re-routes. Stop grilling when no unrouted `[KU]` entries remain or answers stop changing the issue.

When a parked question sharpens, graduate it — after the parent's DoD is written (Step 5). Attaching **any** child, including a decision, turns coverage on.

```bash
loaf issue new --kind decision "Should tokens live in httpOnly cookies?"
```

A decision is a ledger question, not a work-contract row: omit `--ref`. It is ready when the title or body contains `?`. It needs no criteria and no out-of-scope statement. Unsharp questions discovered after create stay in the session register until they graduate — there is no `--fog` on edit. See [references/decomposition.md](references/decomposition.md).

### Step 5: Write Definition of Done (decomposition tail)

Add criteria as the interrogation produces observable done-checks. V-tier when a command can disagree with the implementation; H-tier when only a human can tell.

```bash
loaf issue dod add linear:ENG-42 "Sliding-window expiry is covered by tests" --command "go test ./internal/auth/..." --expect "exit 0"
loaf issue dod add linear:ENG-42 "Stolen-cookie writeup is reviewable" --tier H
```

`--command` implies V unless `--tier` overrides. `--expect` uses the verify grammar (`exit <N>`, `` contains `text` ``, joined by ` and `). Commands run from the repository root. See [references/cli-boundary.md](references/cli-boundary.md) and [references/decomposition.md](references/decomposition.md).

A parent gets children only when its DoD needs more than one coherent slice. The moment a criterion earns its own DoD, mint a child contract on its own ref — the parent criterion stays; claim coverage with `loaf issue dod claim`:

```bash
loaf issue new --ref linear:ENG-43 --parent linear:ENG-42 "Child slice title"
loaf issue dod claim linear:ENG-43 1 1
```

Then shape the child the same way (body, out-of-scope, its own criteria). Order children by likelihood-of-change when presenting them; state real sequencing with `loaf issue link <from> blocks <to>`, never by tree order alone.

### Step 6: Run the Critique Gate

Before finalizing, challenge the draft — see [references/critique-gate.md](references/critique-gate.md). Is scope still bounded, does every new command or state name its ceremony, is a second progress flag creeping into the body, is the CLI/skill boundary drawn correctly, and could this be smaller and still be verifiable alone and revertible alone?

### Step 7: Validate

```bash
loaf issue check linear:ENG-42
```

A delivery contract that passes prints that the ref is shaped; a ledger decision prints ready. Failures always block (missing body, missing criterion, missing out-of-scope, no sharp question, uncovered parent criterion). Containment orphans are reported, not failed: each line includes a ready-to-paste remedy that files the orphan as a sibling backlog contract — run that command with `--ref`, do not invent a different disposition.

`loaf issue verify <ref>` runs V-tier commands from the repository root and writes nothing. That is implement's preflight, not shape's gate. See [references/cli-boundary.md](references/cli-boundary.md).

### Step 8: Offer the Review Surface

The contract lives in SQLite keyed to the authority ref. There is no folder to commit and nothing plan-shaped to land. Offer `loaf issue show <ref>` and `loaf issue tree <ref>` as the review surface. If a PR is being opened for the work, its body is `loaf issue render <ref>` — paste-ready, no manual editing. Opt-in, never automatic.

### Step 9: Log the Outcome

`loaf journal log "decision(shape): linear:ENG-42 shaped — N children, N open fog entries"`.

---

## Related Skills

- **pitch** — Problem-discovery ceremony that authors a brief; preferred front door when the problem is not yet framed
- **idea** — Quick capture; feeds into pitch or shape once a concept has enough weight
- **brainstorm** — Agent technique for divergent thinking (route user entry to pitch)
- **implement** — Starts execution once `loaf issue check` reports the issue shaped; this does not prove implementation completion
- **reflect** — Updates strategic docs after the shipped work proves what changed

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Blindspot pass | [references/blindspot-pass.md](references/blindspot-pass.md) | Deciding whether to offer reconnaissance, and how to prompt it |
| Grilling | [references/grilling.md](references/grilling.md) | Running the one-question-at-a-time interview for `[KU]` entries |
| Reaction artifacts | [references/reaction-artifact.md](references/reaction-artifact.md) | Resolving `[UK]` entries with a variant, mock, or prototype |
| Decomposition | [references/decomposition.md](references/decomposition.md) | Sizing slices, promoting criteria, reading coverage and containment |
| Rideable increments | [../foundations/references/rideable-increments.md](../foundations/references/rideable-increments.md) | Making the work contract a complete, useful operator journey |
| CLI boundary | [references/cli-boundary.md](references/cli-boundary.md) | Reading `loaf issue` output, authoring `--command`/`--expect`, or explaining `loaf issue check` |
| Critique Gate | [references/critique-gate.md](references/critique-gate.md) | Self-challenging scope and boundaries before finalizing |

## Artifact Naming

Shape's deliverable is the ref-keyed contract. If a reaction artifact or spike note lands on disk, name it for what it is, never for the work unit that produced it. Put the source in a front-matter field, not the filename. Versions and timestamps are identity and stay. See the `foundations` skill for the full rule; `loaf check --hook artifact-names` enforces it at commit.
