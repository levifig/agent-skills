---
id: SPEC-021
title: Bootstrap Document Hierarchy Redesign
source: conversation — bootstrap skill gap analysis
created: '2026-04-01T00:00:00.000Z'
status: drafting
appetite: M (focused skill rewrite)
---

# SPEC-021: Bootstrap Document Hierarchy Redesign

## Problem Statement

The bootstrap skill treats BRIEF.md, VISION.md, STRATEGY.md, and ARCHITECTURE.md as a flat set of outputs from a single interview pass. In practice, these documents serve fundamentally different purposes at different depths:

- **BRIEF.md** is a lightweight seed — a napkin sketch capturing the spark in 5-10 minutes
- **VISION.md** and **STRATEGY.md** are foundational pillars — the result of deep, extensive interviewing. Together they should be substantial enough to produce a commercial/executive proposal, build an internal knowledge base, define a value system and principles, and inform architecture decisions
- **ARCHITECTURE.md** flows from the pillars — it's how we build what VISION and STRATEGY describe

The bootstrap skill also has a critical template gap: it describes creating 5 documents but only ships 1 template (brief.md). VISION.md and ARCHITECTURE.md have no templates at all. STRATEGY.md has a template but it lives in the strategy/ skill, not bootstrap/.

### Current model (flat)

```
Single interview pass → BRIEF.md, VISION.md, STRATEGY.md, ARCHITECTURE.md
                           ↑ heavy work    ↑ derivative        ↑ derivative
```

All documents treated as peers. Interview doesn't distinguish depth. Template coverage: 1 of 4.

### Target model (layered)

```
Light interview → BRIEF.md           (seed — the "gist")
                      ↓ approved
Deep interview  → VISION.md          (pillar — why, where, for whom)
                → STRATEGY.md        (pillar — who, what market, how we win)
                      ↓ these enable
                  ARCHITECTURE.md    (how we build it)
                  Commercial proposals, KB, principles, first specs
```

Clear phase boundaries. Interview depth matches document weight. Templates for all documents.

## Solution Shape

### Phase structure

Bootstrap becomes a multi-phase workflow with explicit handoff points:

**Phase 1 — Capture (the seed):** Light interview (5-10 questions). Produces BRIEF.md. Quick, conversational, focused on the spark — what, who, why.

**Phase 2 — Foundation (the pillars):** Deep interview (15-25 questions). Produces VISION.md and STRATEGY.md. This is the heavy work: Wardley mapping, persona building, competitive landscape, job-to-be-done, switching forces. These documents should be substantial enough to hand to an investor, a new team member, or use as the basis for an executive proposal.

**Phase 3 — Architecture (conditional):** Only if technical signal exists from Phases 1-2 or from codebase analysis (brownfield). Produces ARCHITECTURE.md.

Each phase has a clear gate: "brief approved → deep dive", "pillars approved → architecture".

### Input-first interview methodology

When an input document exists (brief, README, existing docs), the interview must:
1. Focus on what's there first — follow the document's own structure, sections, and topics
2. Exhaust what the input covers — ask clarifying questions about each section, validate understanding, surface implicit assumptions
3. Only then move to gap-filling questions
4. Don't reshape the input into the expected output structure — let the input's organization guide the conversation

When starting from scratch, use the existing excavation framework (the question library in phases 1-4 is solid).

### Templates to create

1. **vision.md** — Purpose/mission, target users, success criteria, non-goals, what makes this unique, north star
2. **architecture.md** — System overview, technology choices, components, build vs. buy, constraints, deployment
3. **strategy.md** — Copy from strategy/ skill so bootstrap is self-contained for initial creation

### What NOT to change

- strategy/ skill — stays focused on updating/extending existing STRATEGY.md
- architecture/ skill — stays focused on ADRs and ARCHITECTURE.md evolution
- reflect/ skill — stays focused on post-shipping learnings
- The interview question library (phases 1-4 are solid content)

## Rabbit Holes

- Don't redesign the interview question library — just reorganize which questions map to which phase/document
- Don't create a "proposal generator" skill — that's downstream of VISION+STRATEGY, not part of bootstrap
- Don't merge strategy/ or architecture/ skills into bootstrap — they serve different lifecycle stages

## No-Gos

- No changes to strategy/architecture/reflect skills
- No new CLI commands — bootstrap is a skill workflow, not a CLI feature
- No "smart document generation" — templates are structural guidance, not AI-generated boilerplate

## Test Conditions

- [ ] Bootstrap skill has templates for: brief.md, vision.md, architecture.md, strategy.md
- [ ] Interview guide clearly separates light (brief) vs deep (vision/strategy) interviewing
- [ ] SKILL.md has distinct phases with explicit handoff points between them
- [ ] Input-first methodology documented: exhaust input structure before gap-filling
- [ ] `loaf build` succeeds
- [ ] `npm run typecheck` passes
