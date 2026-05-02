---
id: ADR-006
title: Agent-Creates, Human-Curates Model
status: Deprecated
date: 2026-03-14
deprecated_date: 2026-05-02
---

# ADR-006: Agent-Creates, Human-Curates Model

## Decision

Agents are the primary authors of knowledge files, ADRs, tasks, and specs. Humans review, approve, and curate. The CLI is for human management and health checks, not for authoring.

## Context

Traditional documentation is human-authored and agent-consumed. This creates a maintenance burden — humans write docs as a separate task, docs drift. The research principle "maintenance as side effect of work" suggests inverting this.

## Rationale

- Agents are already doing the work — they're closest to what's being learned
- Knowledge creation should happen during brainstorming, development, debugging — not as a separate documentation sprint
- Humans are better at judgment (is this worth documenting?) than at the writing itself
- The growth loop: agent discovers insight → proposes knowledge file → human reviews → committed
- Same pattern for ADRs: agent proposes after architectural discussion → human reviews
- Same pattern for tasks: agent creates via `/breakdown` → human approves

## Consequences

- Knowledge-base skill must guide agents on WHEN to create knowledge (not just format)
- Review workflow needed: agents propose, humans accept/edit/reject
- CLI commands are management-oriented (`check`, `validate`, `status`, `review`), not authoring-oriented
- Quality control depends on human review — agents may write redundant or low-quality knowledge
- PostToolUse and SessionEnd hooks prompt agents at the right moments

## Deprecated

This ADR was recategorized on 2026-05-02 against the tightened architecture-skill bar (see [content/skills/architecture/SKILL.md](../../content/skills/architecture/SKILL.md#the-bar)).

**ADRs capture an architectural *choice* between credible alternatives.** This record names an alternative — the traditional "humans author, agents consume" model — but the rejection is on **philosophical and operational grounds** (maintenance burden, growth loop, "maintenance as side effect of work"), not on architectural ones. No specific quality attribute, dependency, interface, or construction technique is being weighed; the rationale is about *how the team operates*, not *how the system is structured*. That makes ADR-006 a guiding principle / operating model, not an architectural decision.

The principle's downstream *implications* (CLI surface as management-not-authoring, hook timing, review workflow) shape the system; those *implications* could be ADRs in their own right, but the principle itself is not.

**The principle itself remains in force.** The active source is now [docs/ARCHITECTURE.md](../ARCHITECTURE.md#authorship-model--agents-create-humans-curate) under the Operating Principles section — where it can evolve via `/reflect`, the appropriate mechanism for guiding principles (mutable, evolves with project learning; ADRs are immutable post-acceptance).

This record is retained per the append-only-log discipline ("_was_ the decision, _no longer_ the decision" — Nygard) but is no longer the operative source.
