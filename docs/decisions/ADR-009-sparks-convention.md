---
id: ADR-009
title: Sparks Convention in Brainstorm Documents
status: Deprecated
date: 2026-03-14
deprecated_date: 2026-05-02
deprecated_reason: 'Recategorized — workflow lore for brainstorm skill, not architecturally significant.'
migrated_to: content/skills/brainstorm/SKILL.md
---

# ADR-009: Sparks Convention in Brainstorm Documents

## Decision

Speculative ideas from brainstorming sessions ("sparks") live as a `## Sparks` section at the end of brainstorm documents. They are promoted to ideas via `/idea` or abandoned. Brainstorm documents are archived, never deleted.

## Context

Brainstorming produces speculative byproducts — ideas that aren't ready for `/shape` but are worth remembering. These need a home that doesn't create file sprawl or a separate tracking system.

## Rationale

- Sparks belong with their origin context (the brainstorm that produced them)
- No new artifact type needed — just a section convention
- `/idea` (no args) scans brainstorm docs for unprocessed sparks, bridging brainstorm → idea pipeline
- Brainstorm documents are historical records of exploration — archiving preserves the reasoning
- Lifecycle is simple: unprocessed → promoted (`*(promoted)*`) or abandoned (`~~strikethrough~~`)

## Alternatives Considered

- Standalone `SPARKS.md` file — tried and abandoned; disconnects sparks from their context
- Individual spark files in `.agents/sparks/` — too heavy for lightweight captures
- Backlog items — too formal; sparks are pre-backlog

## Consequences

- Brainstorm skill includes "Capture Sparks" step
- Idea skill scans brainstorm docs when invoked without arguments
- Idea files get `origin:` field for traceability
- Brainstorm docs are retained while they hold unprocessed sparks

## Deprecated

This ADR was recategorized on 2026-05-02 against the tightened architecture-skill bar (see [content/skills/architecture/SKILL.md](../../content/skills/architecture/SKILL.md#the-bar)).

**ADRs capture a *choice* between credible architectural alternatives.** This record codifies workflow lore for the brainstorm skill — the `## Sparks` section convention. The original includes alternatives ("standalone SPARKS.md", "individual spark files"), but they were workflow-design alternatives, not architectural ones (no canonical-domain effect, no cost-of-divergence beyond the brainstorm skill). Microsoft Well-Architected: *"avoid making decision records design guides."*

**The convention itself remains in force.** The active source is now [content/skills/brainstorm/SKILL.md](../../content/skills/brainstorm/SKILL.md) — where the workflow can evolve alongside the skill itself, the appropriate mechanism for skill-specific lore.

This record is retained per the append-only-log discipline ("_was_ the decision, _no longer_ the decision" — Nygard) but is no longer the operative source.
