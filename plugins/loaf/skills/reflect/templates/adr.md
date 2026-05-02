<!--
This template is for architecturally significant decisions only.

ADRs are rare yet binding: they capture the rationale for a choice
that affects the system's structure, key quality attributes,
dependencies, interfaces, or construction techniques — and that is
difficult to reverse in the project's current state.

If the decision is a development pattern, exploration, or implementation
direction, use a SPEC via /loaf:shape instead.
If it's a guiding principle or operating philosophy, update ARCHITECTURE.md
or VISION.md.
If it's workflow lore for a specific skill, document it in that skill.
If it's a local choice changeable in a single PR, session-log it.

Run the architecture skill's Triage Gate if unsure.

A `Rejected` or `Deprecated` ADR REQUIRES a `## Rejected` or `## Deprecated`
body section explaining the transition. A `Superseded` ADR may include a
`## Superseded` section but the linkage in `superseded_by:` is sufficient.
-->

# ADR Template

**Location:** `docs/decisions/ADR-{number}-{slug}.md`

```yaml
---
id: ADR-001
title: "PostgreSQL as Primary Database"
status: Accepted  # Proposed | Accepted | Rejected | Deprecated | Superseded
date: 2026-01-23
# accepted_date:   # optional — only if differs from `date`
# rejected_date:   # required iff status is Rejected
# deprecated_date: # required iff status is Deprecated
supersedes: null   # optional — ADR-NNN if replacing
superseded_by: null  # required iff status is Superseded
---

# ADR-001: PostgreSQL as Primary Database

## Context

[Why this decision was needed. What problem we faced.]

## Decision

[What we decided. Be specific and unambiguous.]

## Consequences

### Positive
- [Benefit 1]
- [Benefit 2]

### Negative
- [Tradeoff 1]
- [Tradeoff 2]

### Neutral
- [Implication that's neither good nor bad]

## Alternatives Considered

### [Alternative 1]
[Why it was rejected]

### [Alternative 2]
[Why it was rejected]
```
