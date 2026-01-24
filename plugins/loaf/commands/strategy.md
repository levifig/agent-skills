---
description: Discover and maintain strategic context in STRATEGY.md
version: 1.12.0
---

# Strategy Command

Deep discovery for personas, market landscape, and problem space.

**Input:** $ARGUMENTS

---

## Purpose

STRATEGY.md captures the **landscape** — who we're building for, what we understand about the problem space, and how we're positioned.

This is distinct from:
- **VISION.md** — Where we're going (north star)
- **ARCHITECTURE.md** — How we build (technical constraints)

Strategy informs shaping by providing context about users and market.

---

## Mode Detection

Parse input to determine mode:

| Input Pattern | Mode | Action |
|---------------|------|--------|
| Empty or "discover" | Full Discovery | Comprehensive strategy session |
| "personas" | Persona Discovery | Define/refine user personas |
| "market" or "landscape" | Market Analysis | Competitive positioning, trends |
| "problems" or "problem-space" | Problem Space | Core problems we solve |
| "glossary" | Domain Glossary | Key terms and definitions |
| Specific topic | Targeted Update | Update specific section |

---

## CRITICAL: Interview Deeply

Strategy discovery requires **extensive interviewing**. You're capturing domain knowledge that lives in the user's head.

Use `AskUserQuestion` frequently. Ask non-obvious questions:
- What do users complain about with existing solutions?
- Who are we explicitly NOT building for?
- What adjacent problems should we avoid?
- What trends might make this irrelevant in 3 years?

---

## Process

### Step 1: Gather Context

1. **Read VISION.md** — Strategy must align with vision
2. **Read existing STRATEGY.md** — Don't duplicate, extend
3. **Check recent sessions** — Implementation learnings inform strategy

### Step 2: Mode-Specific Discovery

#### Mode: Full Discovery

For new projects or comprehensive refresh.

Interview sequence:

1. **Users & Personas**
   - Who are the primary users?
   - What are their goals, frustrations, behaviors?
   - Who are we explicitly NOT building for?

2. **Problem Space**
   - What core problems do we solve?
   - What adjacent problems are tempting but out of scope?
   - What's the cost of the problem today?

3. **Market Landscape**
   - Who else solves this problem?
   - How are we different?
   - What trends affect this space?

4. **Domain Language**
   - What terms need definition?
   - What's ambiguous or overloaded?

#### Mode: Persona Discovery

Focus on user understanding.

| Question | Purpose |
|----------|---------|
| Who are the distinct user types? | Identify personas |
| What role does each play? | Context |
| What are they trying to accomplish? | Goals |
| What frustrates them today? | Pain points |
| How do they currently solve this? | Existing behavior |
| What would delight them? | Opportunities |
| Who explicitly isn't our user? | Anti-personas |

**Persona template:**

```markdown
### [Persona Name]

**Role:** [Job title or function]

**Goals:**
- [Primary goal]
- [Secondary goal]

**Frustrations:**
- [Pain point 1]
- [Pain point 2]

**Current Behavior:**
- [How they solve this today]

**Key Insight:**
[One sentence capturing the essential truth about this persona]
```

#### Mode: Market Analysis

Focus on positioning and landscape.

| Question | Purpose |
|----------|---------|
| Who else addresses this problem? | Competitors |
| What do they do well? | Learn from |
| What do they do poorly? | Differentiation opportunity |
| How are we different? | Positioning |
| What's our unfair advantage? | Moat |
| What market trends matter? | Context |
| What could disrupt this space? | Risks |

#### Mode: Problem Space

Focus on the problems we solve.

| Question | Purpose |
|----------|---------|
| What's the core problem? | Foundation |
| Why does it matter? | Stakes |
| Who feels this pain most? | Target |
| What's the cost of not solving it? | Value |
| What adjacent problems exist? | Boundaries |
| Which adjacent problems should we avoid? | Focus |
| What would "solved" look like? | Success criteria |

#### Mode: Domain Glossary

Focus on shared language.

| Question | Purpose |
|----------|---------|
| What terms are ambiguous? | Clarify |
| What's domain-specific jargon? | Define |
| What terms mean different things to different people? | Align |
| What abbreviations do we use? | Document |

### Step 3: Draft Updates

Based on discovery, draft additions/updates to STRATEGY.md.

**STRATEGY.md structure:**

```markdown
# Strategy

## Personas

### [Persona 1]
...

### [Persona 2]
...

### Anti-Personas
[Who we're NOT building for]

## Problem Space

### Core Problems
- [Problem 1]
- [Problem 2]

### Adjacent Problems (Not Solving)
- [Related but out of scope]

### Success Criteria
[What "solved" looks like]

## Market Landscape

### Competitive Positioning
[How we're different]

### Competitors
| Competitor | Strengths | Weaknesses | Our Differentiation |
|------------|-----------|------------|---------------------|
| ... | ... | ... | ... |

### Market Trends
- [Trend 1]
- [Trend 2]

### Risks
- [What could disrupt us]

## Glossary

| Term | Definition |
|------|------------|
| ... | ... |
```

### Step 4: Present for Approval

```markdown
## Proposed STRATEGY.md Updates

### New/Updated Sections:

[Draft content]

---

**Before approval, confirm:**
1. Are the personas accurate?
2. Is the problem space well-defined?
3. Is our positioning clear?
4. Any missing context?
```

### Step 5: Await Approval

**Do NOT update STRATEGY.md without explicit approval.**

User may:
- Approve as-is
- Adjust details
- Add missing context
- Request more discovery

### Step 6: Update STRATEGY.md

After approval:

1. **If STRATEGY.md doesn't exist:** Create it with full structure
2. **If updating:** Merge new content with existing
3. **Announce:**
   ```
   Updated: docs/STRATEGY.md

   Sections updated:
   - [Section 1]
   - [Section 2]

   This context will inform future /shape sessions.
   ```

---

## Creating STRATEGY.md from Scratch

If no STRATEGY.md exists, run full discovery mode and create the complete document.

Minimum viable STRATEGY.md:

```markdown
# Strategy

## Personas

### [Primary Persona]
**Role:** ...
**Goals:** ...
**Frustrations:** ...

## Problem Space

### Core Problems
- [Core problem we solve]

## Market Landscape

### Positioning
[One paragraph on how we're different]

---

*Last updated: YYYY-MM-DD*
```

---

## Guardrails

1. **Interview deeply** — Strategy is domain knowledge extraction
2. **Align with VISION** — Strategy serves the north star
3. **Define anti-personas** — Who we're NOT building for is as important
4. **Adjacent problems** — Document what's out of scope
5. **Get approval** — Don't update without confirmation
6. **Keep it current** — Outdated strategy is worse than none

---

## When to Use This vs Other Commands

| Situation | Command |
|-----------|---------|
| Understanding users and market | `/strategy` |
| Quick idea capture | `/idea` |
| Developing an idea into spec | `/shape` |
| Updating strategy after shipping | `/reflect` |
| Technical decisions | `/architecture` |

---

## Related Commands

- `/shape` — Uses STRATEGY.md for context during shaping
- `/reflect` — Updates STRATEGY.md based on shipping learnings
- `/research` — Investigation that may inform strategy
- `/brainstorm` — Deep thinking that may surface strategy insights
