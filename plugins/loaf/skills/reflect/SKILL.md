---
name: reflect
description: >-
  Integrates learnings from shipped work into strategic documents. Use after
  completing significant work or when the user asks "what did we learn?" Updates
  VISION.md, STRATEGY.md, and ARCHITECTURE.md based on implementation
  experience. Not for pre-i...
user-invocable: true
argument-hint: '[session-file]'
version: 2.0.0-dev.29
---

# Reflect

Update VISION, STRATEGY, and ARCHITECTURE based on proven implementation.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Purpose
- When to Reflect
- Process
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

- **Evidence-based** -- proposals need supporting evidence from shipped work
- **Post-implementation only** -- reflect after shipping, not before or during planning
- **Get explicit approval** -- never update strategic docs (VISION.md, STRATEGY.md, ARCHITECTURE.md) without user confirmation
- **Consolidate** -- batch related learnings into coherent updates, avoid micro-updates
- **Link back** -- always reference the specs/sessions that informed each update
- **Log updates** -- log each strategic document update to session journal: `loaf session log "decision(scope): updated STRATEGY.md with learning"`

## Verification

- Proposals cite specific specs, sessions, or commits as evidence
- No strategic document was modified without explicit user approval
- Completed specs referenced in updates are archived after reflection

## Quick Reference

| Learning Type | Document |
|---------------|----------|
| User behavior / market / problem understanding | STRATEGY.md |
| Direction changes | VISION.md |
| Technical constraints / patterns | ARCHITECTURE.md |
| Decision updates | ADR (new or supersede) |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Update Proposal | [templates/update-proposal.md](templates/update-proposal.md) | Drafting proposals for strategic doc changes |

---

## Purpose

Strategy evolves through **shipping**, not theorizing.

After completing work, `/loaf:reflect` extracts learnings and proposes updates to strategic documents. **Don't update strategy during planning or shaping.** Update after implementation proves (or disproves) assumptions.

---

## When to Reflect

- After completing a spec or shipping a significant feature
- After discovering something unexpected during implementation
- After a series of related sessions
- Periodically (monthly/quarterly) to consolidate learnings

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` can be: a spec ID (`SPEC-001`), a topic ("authentication learnings"), or empty (general reflection on recent work).

### Step 2: Gather Evidence

Sources:
1. **Completed specs** (`.agents/specs/SPEC-*.md` with status `complete`) -- look for "Lessons Learned"
2. **Session files** (`.agents/sessions/`) -- insights, surprises, pivots
3. **Recent commits** (`git log --oneline -30`)
4. **Implementation reality** -- what was harder/easier than expected? What assumptions were wrong?

### Step 3: Interview for Insights

Ask: What surprised you? What would you do differently? Did any assumptions prove wrong? What did we learn about users or technical constraints? Strategic implications?

### Step 4: Identify Implications

Map learnings to documents:

| Learning Type | Document |
|---------------|----------|
| User behavior / market / problem understanding | STRATEGY.md |
| Direction changes | VISION.md |
| Technical constraints / patterns | ARCHITECTURE.md |
| Decision updates | ADR (new or supersede) |

### Step 5: Draft Proposals

For each document needing updates, create proposals following [update-proposal template](templates/update-proposal.md).

### Step 6: Present and Await Approval

Present all proposals grouped by document. **Do NOT update strategic documents without explicit approval.**

User may: approve all, approve some, modify proposals, defer updates, or request more evidence.

### Step 7: Apply Updates

After approval:
1. Update documents with approved changes
2. Create ADRs if needed (see `/loaf:architecture` for format)
3. Archive completed specs if appropriate
4. Announce what was updated

---

## Related Skills

- **shape** -- Notes strategic tensions for later reflection
- **strategy** -- Deep discovery (before reflection validates)
- **architecture** -- Technical decisions (creates ADRs)
- **research** -- Investigation that may inform reflection
