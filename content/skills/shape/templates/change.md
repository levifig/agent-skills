---
change: [slug]
created: [YYYY-MM-DD]
branch: [slug]
---

<!-- Frontmatter must open the file at byte one — parsers depend on it. No status-like frontmatter (readiness/status/state): readiness is derived — a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

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

[The HOW. Free-form `###` subsections named by the work — the container is the contract; the subsection names are yours.]

### [Approach / Placement / Risks / Sequencing / Spike findings …]

[...]

## Implementation Units

<!-- In-document work packets — commit-boundary guides and review anchors, not tracked entities. -->

- [**U1 — Unit name.** What it delivers.]

## Verification Contract

<!-- Executable (machine-checkable): -->

- [**V1.** Criterion bound to a command and an expected result.]

<!-- Human review: -->

- [**H1.** What a reviewer confirms that no command can.]

## Definition of Done

- [Derived from gates and review — never a status flag.]

## Durable Outputs

[Specs, ADRs, knowledge docs, or schema updates to create after implementation proves what is now true. A final spec describes reality, not a plan.]

## Open Questions

- [Known unknowns, each owned by a section, a spike, or a follow-up.]

## Source Inputs

- [Where this Change came from: journal entries (cite by ID), sparks, ideas, brainstorms, issues, conversations, prior Changes.]

<!-- Optional sections, added when they earn their place: Background, Success Metrics (when validation matters), Follow-ups, Critique Gate. -->
