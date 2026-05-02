# Interface Design — Parallel Sub-Agent Pattern

The INTERFACE-DESIGN phase of `/refactor-deepen` proposes the public surface
of a deepened module by sampling three independent design attempts and
presenting all three to the user. Variety must come from sampling, not from
manufactured opposition.

## Contents

- Origin and Source Attribution
- The Default Rule: 3 Identical Briefs
- Why No Opposing-Constraint Priming
- Loaf Agent Profile Mapping
- The Brief
- When to Invoke This Pattern
- Convergence Fallback
- Escalation (Opt-In Only)
- Cost Note
- Presenting the Three Designs
- Anti-Patterns
- Cross-References

## Origin and Source Attribution

This pattern is **Loaf-specific**. It is inspired by Matt Pocock's
`improve-codebase-architecture` skill ([source][matt-source]), but with the
priming choice deliberately reversed.

Matt's source primes three sub-agents with opposing constraints — Agent 1
optimizes for minimalism, Agent 2 for flexibility, Agent 3 for the
common-caller path. That choice is reasonable when the agents are
domain-agnostic and the orchestrator wants guaranteed surface variety.

Loaf rejects that choice. The decision is captured in
[SPEC-034](../../../../.agents/specs/SPEC-034-refactor-deepen-grilling-glossary.md),
specifically in the Rabbit Holes and No-Gos sections (lines 96 and 107). The
short version: priming manufactures diversity rather than letting it emerge,
and manufactured diversity is dishonest signal. If the three designs do
converge, the response is more agents or rerun — not priming. See
[Convergence Fallback](#convergence-fallback) below.

[matt-source]: https://github.com/mattpocock/skills/tree/main/skills/engineering/improve-codebase-architecture/INTERFACE-DESIGN.md

## The Default Rule: 3 Identical Briefs

When the grilling loop reaches interface design for a candidate module, spawn
**exactly 3 sub-agents with identical briefs**. Identical means:

- Same problem statement
- Same module context (the candidate, its callers, its dependencies)
- Same vocabulary constraints (the eight terms from
  [language.md](language.md))
- Same output schema (Interface signature, Implementation sketch,
  Tradeoffs section, Open questions)
- Same model and same temperature settings (whatever the harness defaults
  provide)

No agent receives a "lens," "optimization target," or "design constraint"
the others don't. The only thing that varies is the sampling — three runs
of the same prompt against the same model produce three different design
trajectories, and that is the variety we want.

## Why No Opposing-Constraint Priming

Three reasons, in order of importance:

1. **Honest variety.** If three independent samples of the same brief
   converge on the same design, that convergence is a real signal — the
   problem may have a single best answer. Priming destroys that signal by
   guaranteeing variety regardless of whether the problem warrants it.

2. **No dominant lens upfront.** Picking which constraint each agent
   optimizes for is itself a design decision. Doing it pre-grilling biases
   the entire phase toward whatever lens the orchestrator happened to
   pick. Loaf's grilling protocol is supposed to surface tradeoffs from
   the codebase and the user, not from the orchestrator's pre-commitments.

3. **Easier to escalate, harder to reverse.** If the default is unprimed
   and convergence happens, the user can request a primed run. If the
   default were primed, the user would have to explicitly ask for an
   unprimed sanity check — and most users never know to ask.

## Loaf Agent Profile Mapping

**Use the `researcher` profile** for the design sub-agents.

Justification:

- The design sub-agents produce structured reports (interface signature +
  implementation sketch + tradeoffs), which is exactly the
  researcher contract: "Return findings as structured reports: summary,
  options (ranked with trade-offs), evidence sources, and a
  recommendation." See [content/agents/researcher.md](../../../agents/researcher.md).
- Read-only access matches the phase's purpose. The INTERFACE-DESIGN phase
  proposes designs; it does not implement them. Granting write access
  invites scope creep into "let me just sketch the implementation" and
  pollutes the parallel sampling.
- The researcher's "Cite sources. Every claim from an external source
  needs a URL or reference" rule maps directly to "every design choice
  must reference a caller, a dependency category, or a vocabulary term"
  during the deepening review.

### When `implementer` Could Be an Alternative

If the design phase needs to write probe code — a quick spike to verify
that a proposed interface compiles against the existing call sites, for
example — the `implementer` profile becomes viable. See
[content/agents/implementer.md](../../../agents/implementer.md).

The default remains `researcher` because:

- Probe code at this phase is rarely necessary; the deepening review
  catches most fatal interface mistakes before code is written.
- Three implementers writing probe code in parallel risks three divergent
  partial implementations of the same module — a merge problem the user
  did not ask for.
- If a probe is genuinely needed, the user can request `implementer`
  explicitly, and the brief should constrain the probe to a single
  throwaway file.

## The Brief

Each sub-agent receives the same brief, with these required sections:

```markdown
## Candidate Module

<name and one-line cue from the candidate list>

## Current Shape

<existing interface, callers, dependency category from deepening.md>

## Vocabulary Constraints

Use only the eight source terms (Module, Interface, Implementation,
Depth, Seam, Adapter, Leverage, Locality). Reject "boundary,"
"service," "component," "layer." See references/language.md.

## Required Output

1. **Proposed Interface** — function signatures or type definitions
2. **Implementation Sketch** — how the module hides complexity
3. **Tradeoffs** — what this design optimizes for, what it sacrifices
4. **Open Questions** — what the user must decide
```

The brief is identical across all three agents. No agent gets an extra
"prefer X" or "optimize for Y" instruction.

## When to Invoke This Pattern

Inside the grilling loop, after dependency classification, only when:

- The deepening reveals that the right interface is **non-obvious**.
  Single-agent reasoning would underweight tradeoffs.
- The candidate module has **multiple callers** with **different access
  patterns**, so the interface surface has real degrees of freedom.
- The user (or the grilling itself) has surfaced **explicit tradeoffs**
  (testability vs. ergonomics, locality vs. reuse) that a single design
  attempt would collapse prematurely.

If the interface is obvious from the dependency category and the call
sites — for example, a thin adapter over a true-external dependency —
skip the parallel phase. A single proposed interface is enough.

## Convergence Fallback

Sometimes the three designs converge — same signature, same
implementation strategy, same tradeoffs. Two cases:

1. **Convergence is real.** The problem has a single best answer. Accept
   the convergence as a genuine finding and proceed to the PLAN with that
   single design. Note in the PLAN's "rejected alternatives" section that
   the parallel phase produced no meaningful variation.

2. **Convergence feels accidental.** The designs are suspiciously
   similar, the user senses a missed branch, or the briefing was so
   constrained that variation had no room. The fallback is, in order:

   - **Spawn a 4th sub-agent** with the same identical brief. A fourth
     sample sometimes surfaces a branch the first three missed.
   - **Rerun all three** with different temperature/seed settings if the
     harness exposes that knob.
   - **Re-grill the user** to surface a tradeoff that wasn't in the
     brief, then rerun three fresh agents with the enriched brief.

**Do not introduce priming as the convergence fix.** Priming is the
solution that this entire reference rejects. If the user explicitly opts
into priming, see [Escalation](#escalation-opt-in-only).

## Escalation (Opt-In Only)

The default (3 unprimed agents, identical briefs) is the right answer for
the overwhelming majority of invocations. Two escalations exist for
explicit user opt-in:

### More Agents

The user can request more than three:

> "Spawn 5 design agents instead of 3."

This costs more (see [Cost Note](#cost-note)) but is sometimes warranted
on high-stakes interfaces where the user wants confidence that no design
branch is being missed.

### Specific Lenses

The user can request that specific agents be primed with lenses:

> "Run three agents — one optimizing for testability, one for runtime
> performance, one for caller ergonomics."

This is exactly the pattern the default rejects, but it remains
available when the user has identified the relevant tradeoff axes
explicitly. The skill must surface the tradeoff in writing:

> Lens-primed parallel sampling produces guaranteed variety but loses
> the convergence signal. The three designs will differ because they
> were told to differ, not because the problem is genuinely
> ambiguous.

Both escalations require an **explicit user request**. The skill never
escalates on its own.

## Cost Note

A single invocation of the parallel phase is approximately **3 ×
deep-exploration cost** in tokens — three sub-agents each running a full
design pass against the same context. With more-agents escalation, the
cost scales linearly (4 agents = ~4 ×, 5 agents = ~5 ×).

This pattern is **opt-in inside the grilling loop**. It is not
auto-triggered on every `/refactor-deepen` invocation. The 3-agent default
is the cost ceiling for the default settings — the skill does not scale
agent count silently. If a candidate's interface is obvious, skip the
phase entirely and propose a single design.

The cost is an explicit tradeoff the user accepts when they enter the
INTERFACE-DESIGN phase for a non-obvious interface. The skill should
surface the cost briefly before spawning:

> Spawning 3 design agents in parallel (~3 × token cost). Proceed?

## Presenting the Three Designs

After the three sub-agents return:

1. **Do not pre-rank.** Present the three designs in arbitrary order.
   Pre-ranking biases the user toward whichever design the orchestrator
   placed first.
2. **Side-by-side, not narrative.** Show the proposed interfaces in a
   single comparison table or three parallel sections, not a paragraph
   that summarizes them.
3. **Surface the convergence signal.** If two or more designs are
   substantially similar, say so explicitly: "Designs A and B both
   propose a streaming interface; only C proposes batch."
4. **Let the user pick.** The PLAN file's "proposed deepened module"
   section records the chosen design; the other two go into "rejected
   alternatives" with the reason.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Prime agents with opposing constraints by default | Use identical briefs; let sampling produce variety |
| Auto-trigger the parallel phase on every invocation | Invoke only when interface is non-obvious |
| Use `implementer` profile for design sub-agents | Use `researcher`; switch to `implementer` only on explicit user request |
| Add a "lens" to the brief silently | Surface the tradeoff to the user before priming |
| Pre-rank the three designs in the output | Present in arbitrary order, let the user pick |
| Spawn 5+ agents by default to "be thorough" | Stick to 3; escalate only on explicit user request |
| Treat convergence as failure that needs priming to fix | Treat convergence as signal; fall back to a 4th agent or rerun |
| Skip the cost surfacing before spawning | Surface the ~3 × token cost; let the user opt in |

## Cross-References

- [language.md](language.md) — The eight source terms; vocabulary
  constraints in the brief
- [deepening.md](deepening.md) — Dependency categories the brief must
  cite when justifying interface choices
- [content/agents/researcher.md](../../../agents/researcher.md) — Default
  agent profile for design sub-agents
- [content/agents/implementer.md](../../../agents/implementer.md) —
  Alternative profile when probe code is needed
- [SPEC-034](../../../../.agents/specs/SPEC-034-refactor-deepen-grilling-glossary.md)
  — The shape decision (lines 64, 96, 107, 117) that established the
  identical-brief default
