# H3 — Series-prep dogfood

**Date:** 2026-07-30  
**Runtime:** branch-built `bin/native/darwin-arm64/loaf` (2.0.0-alpha.16)  
**Scratch project:** isolated temp git repo (discarded after this note)  
**Method:** exercise the series-prep procedure against a sample pitched `docs/BRIEF.md` (`source: pitch`), not a live human bootstrap session — simulates the mint/landing matrix with explicit-path check, `change.json` target read-back, and one docs-only commit per capture on `main`.

## Sample BRIEF (input)

Synthetic project BRIEF with three independently shippable problem threads in Sequencing (shared brief skeleton, pitch ceremony, bootstrap series-prep), all bound to coarse cohort `0.1.0`. One concept deliberately left un-minted in the narrative (exact cohort label open question) to exercise "stays BRIEF line" — not minted as a folder.

## Granularity judgment applied

| Candidate | Decision | Why |
|-----------|----------|-----|
| Shared problem-space brief skeleton | **Mint** `brief-skeleton` | Independently shippable; problem states without siblings |
| Human problem-discovery ceremony | **Mint** `pitch-ceremony` | Independently shippable; problem states alone |
| Bootstrap series-prep for pitched arcs | **Mint** `bootstrap-series-prep` | Independently shippable; problem states alone |
| Exact first public version label | **Stay BRIEF open question** | Not a shippable problem boundary |

## Pre-landing results (each mint)

Every capture: `loaf change check <folder> --json` → `passed: true`, `state: "captured"`, zero findings; direct `change.json` read-back confirmed `target_release: "0.1.0"`. Expected warning only: current branch `main` does not match change `branch` field (promise carriers land on default; no slug branch created). Branches after series: `main` only.

## Commit log (one docs-only commit per capture)

```
97e1247 docs(change): capture bootstrap-series-prep brief
docs/changes/20260730-bootstrap-series-prep/brief.md
docs/changes/20260730-bootstrap-series-prep/change.json
4d83ad8 docs(change): capture pitch-ceremony brief
docs/changes/20260730-pitch-ceremony/brief.md
docs/changes/20260730-pitch-ceremony/change.json
8670b57 docs(change): capture brief-skeleton brief
docs/changes/20260730-brief-skeleton/brief.md
docs/changes/20260730-brief-skeleton/change.json
3ee24ea docs: sample pitched BRIEF for series-prep dogfood
docs/BRIEF.md
```

## Cold-read check

Each minted brief names problem, who, alternative, and value without referring to the bootstrap session or requiring `docs/BRIEF.md`. No "as discussed" / "per the project brief" language. Problem-space only (no architecture or task lists).

## Minted briefs (verbatim)

### docs/changes/20260730-brief-skeleton/

`change.json`:

```json
{
  "branch": "brief-skeleton",
  "change": "brief-skeleton",
  "created": "2026-07-30",
  "target_release": "0.1.0"
}
```

`brief.md`:

```markdown
<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# Shared problem-space brief skeleton

## Problem Statement

Change-scale and project-scale briefs use different skeletons and quality bars, so problem discovery quality depends on which door the builder used. Agents and humans lack one shared problem-space contract (problem, who, alternatives, value, constraints, sequencing prose, sources, open questions).

## Who Has It

Builders and agents authoring Loaf briefs at either scale who need a cold-readable problem statement before shaping.

## Current Alternatives

Paste a spark into brief.md, or rely on bootstrap interview notes that never become a change-scale brief. Trackers hold issues without a problem-space skeleton.

## Value Proposition

One shared skeleton on all brief surfaces so a cold reader can name the problem, who has it, the alternative, and the value in one pass — with zero solution design.

## Constraints

- Problem-space only; no approach or architecture in the brief
- Project-scale frontmatter may mark source: pitch when discovery already ran

## Sequencing and Relationships

First in the 0.1.0 arc: every later capture and pitch ceremony consumes this skeleton. Does not depend on series-prep or flow seams to be valuable alone.

## Sources and Research Links

- Seeded from the project BRIEF problem threads for H3 dogfood (synthetic)

## Open Questions

- [ ] Whether build-time single-sourcing of the three surfaces is needed later — deferrable
```

### docs/changes/20260730-pitch-ceremony/

`change.json`:

```json
{
  "branch": "pitch-ceremony",
  "change": "pitch-ceremony",
  "created": "2026-07-30",
  "target_release": "0.1.0"
}
```

`brief.md`:

```markdown
<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# Human problem-discovery ceremony

## Problem Statement

There is no human-invoked ceremony that grills the problem space and authors a brief before implementation narrowing. Shape absorbs raw vision, so problem framing and solution design compete in one session.

## Who Has It

Builders starting a new concept on an existing Loaf project, or standing up a greenfield product, who need a front door that is not triage paste and not full shape.

## Current Alternatives

Jump straight to /shape, dump text into a tracker, or improvise an interview with no durable brief artifact.

## Value Proposition

A single /pitch entry that produces an authored problem-space brief at change or project scale and offers shape-now or park without writing shape.md or tasks.

## Constraints

- Human-invoked only; agents never open a pitch
- Pitch never writes shape.md, never seeds tasks/, never pushes or opens PRs

## Sequencing and Relationships

Second in the 0.1.0 arc after the shared brief skeleton exists. Hands project-scale BRIEFs to bootstrap; change-scale captures land per the landing matrix. Independent of series-prep machinery but enables the pitched BRIEF series-prep consumes.

## Sources and Research Links

- Seeded from the project BRIEF value proposition for H3 dogfood (synthetic)

## Open Questions

- [ ] How much competitive scan is default vs opt-in — deferrable
```

### docs/changes/20260730-bootstrap-series-prep/

`change.json`:

```json
{
  "branch": "bootstrap-series-prep",
  "change": "bootstrap-series-prep",
  "created": "2026-07-30",
  "target_release": "0.1.0"
}
```

`brief.md`:

```markdown
<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# Bootstrap series-prep for pitched project arcs

## Problem Statement

A project-scale pitched BRIEF describes an initial arc, but bootstrap ends without minting that arc as captured changes. Builders leave bootstrap with operating docs only and no promise-carrier folders bound to a release cohort.

## Who Has It

Builders who just finished /pitch at project scale and /bootstrap, holding a BRIEF with multiple scoped concepts and no durable change series.

## Current Alternatives

Hand-create change folders later, paste sparks, or jump into one mega-shape that re-does discovery. Trackers hold a backlog without git promise carriers.

## Value Proposition

Bootstrap closes by enumerating independently shippable concepts, minting each as loaf change init --brief with a coarse target_release and problem-space seed, one docs-only commit per capture — ready for later /shape without re-opening the BRIEF.

## Constraints

- Every mint user-confirmed; no auto-shape; no branch creation for the series
- Coarse target required to mint; untargeted concepts stay BRIEF lines, sparks, or Intents
- No priority, date, or dependency fields — sequencing is prose

## Sequencing and Relationships

Third in the 0.1.0 arc: consumes a pitched BRIEF after operating docs are populated. Depends on the brief skeleton existing so seeded change briefs match the shared contract. Does not implement pitch or shape seams.

## Sources and Research Links

- Seeded from the project BRIEF sequencing prose for H3 dogfood (synthetic)

## Open Questions

- [ ] Default recommendation for how many captures in a first series — deferrable
```

## Guidance adjustments from the run

1. **Branch-mismatch warning is expected on promise carriers.** Landing on `main` while `change.json` records the slug as `branch` produces a non-blocking warning. Series-prep skill text already forbids creating slug branches; no change needed beyond treating that warning as normal for targeted parks on default.
2. **Do not parse `ls` colorized paths** when automating dogfood or agent scripts — use literal folder paths returned by `loaf change init` prose (`docs/changes/YYYYMMDD-<slug>/`) or `find`/`printf`.
3. **Granularity held:** three folders that each stand alone cold-read; collapsing them would force a mega-brief that fails the "problem states without the others" test. Over-splitting was not a temptation on this sample.
4. No skill-text rewrites required beyond what TASK-003 already specifies; the dogfood confirmed the landing matrix (coarse target required, one commit per capture, check + read-back) is executable with the branch-built CLI.
