<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# [Change Title]

## Problem

[Why this work exists — the friction, gap, or rot being addressed.]

## Hypothesis

[The bet: what becomes true if this ships, and why it is worth making.]

## Scope

**In**

- [What this Change delivers.]

**Out** (deferred, not rejected)

- [What is explicitly postponed, and where it went.]

**Cut** (explicitly rejected)

- [What this Change will not do, ever.]

## Observable Workflow

[What someone sees or does once this ships — commands, flows, UX. Concrete over abstract.]

## Rabbit Holes and No-Gos

[Boundaries: the ways this work could quietly grow into something it must not become.]

## Decisions

Provenance: [how each decision was accepted — interview, review, dogfooding.]

1. **[Decision.]** [Rationale, and what it forecloses.]

## Planning Contract

<!-- The HOW. Prefer plan.md/design.md when the route needs its own file; keep this container. Free-form ### subsections named by the work. -->

### [Approach / Placement / Risks / Sequencing …]

[…]

## Implementation Units

<!-- Task packets live in tasks/TASK-NNN-slug.md; this section may summarize the decomposition. -->

- [**TASK-001 — Unit name.** What it delivers.]

## Verification Contract

<!-- Executable (machine-checkable): each V-entry declares Command and Expect for loaf change verify. Expect is a grammar, not prose: atoms join with " and " — `exit <N>` is the required exit code (omit the atom, or Expect entirely, for exit 0; a second exit atom is a contradiction and fails the criterion) and contains `text` requires the combined stdout+stderr to contain that backtick-delimited text (repeatable). Example: Expect: exit 0 and contains `all green`. Any other clause is unenforceable: verify warns naming the criterion and clause, records it as advisory, and never checks it. -->

- [**V1.** What must be true. Command: `exact command`. Expect: exit 0.]

<!-- Human review (H-tier): review material, never gate input. -->

- [**H1.** What a reviewer confirms that no command can.]

## Definition of Done

- [Derived from gates and review — never a status flag.]

## Durable Outputs

[Specs, ADRs, knowledge docs, or schema updates to create after implementation proves what is now true.]

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. Tags are convention, never parsed by check. -->

- [KU] [Known unknown → route to a task or later change]
