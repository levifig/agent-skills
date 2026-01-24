---
description: Understand project state or investigate specific topics
version: 1.13.1
---

# Research Command

Investigate to understand — gather facts, assess state, answer questions.

**Input:** $ARGUMENTS

---

## Purpose

Research is about **understanding**, not generating.

Use `/research` when you need to:
- Catch up on project state
- Investigate a technical question
- Understand how something works
- Gather facts before deciding

For **generating** options or **exploring** ideas, use `/brainstorm`.
For **updating strategy** based on learnings, use `/reflect`.

---

## Mode Detection

Parse input to determine mode:

| Input Pattern | Mode | Action |
|---------------|------|--------|
| "project state" / "catch me up" / empty | State Assessment | Review docs, sessions, recent work |
| Topic or question | Topic Investigation | Structured inquiry |

---

## Mode 1: State Assessment

**Trigger:** Empty input, "project state", "catch me up", "where are we"

### Steps

1. **Read project documents:**
   - `docs/VISION.md` — Current direction
   - `docs/STRATEGY.md` — Personas, market, problem space
   - `docs/ARCHITECTURE.md` — Technical constraints

2. **Check ideas and specs:**
   - `.agents/ideas/*.md` — Pending ideas
   - `docs/specs/SPEC-*.md` — Active specifications

3. **Review recent sessions:**
   - `.agents/sessions/*.md` — Implementation history
   - Note lessons learned, open questions

4. **Check recent commits:**
   ```bash
   git log --oneline -20
   ```

5. **Synthesize findings:**

```markdown
## Project State Assessment

**Date:** [timestamp]

### Current Position
[Where the project is now]

### Strategic Context
- **Vision:** [Brief summary]
- **Key personas:** [Who we're building for]
- **Current focus:** [Active specs/work]

### Recent Progress
- [Accomplishment 1]
- [Accomplishment 2]

### In Flight
| Spec/Task | Status | Notes |
|-----------|--------|-------|
| SPEC-001 | implementing | [progress] |
| SPEC-002 | approved | [next up] |

### Ideas Pipeline
- [Idea 1] — raw
- [Idea 2] — raw

### Lessons Learned (Recent)
- [Insight from sessions]

### Open Questions
- [Unresolved question]

### Recommendations
1. [Suggested next step]
2. [Suggested next step]
```

---

## Mode 2: Topic Investigation

**Trigger:** Specific topic or question

### Steps

1. **Clarify the question:**

   Use `AskUserQuestion` to understand:
   - What specifically are you trying to understand?
   - What context do you already have?
   - What decision will this inform?

2. **Check project context first:**
   - Existing decisions in `docs/decisions/ADR-*.md`
   - Relevant sections in ARCHITECTURE.md
   - Previous session discussions

3. **Apply confidence hierarchy:**
   ```
   Project context > Official docs > Authoritative sources > Community > General web
   ```

4. **For library/framework questions:**
   - Use Context7 MCP if available
   - Cross-reference with official documentation
   - Check for version-specific information

5. **Synthesize findings:**

```markdown
## Research: [Topic]

**Question:** [What we're investigating]

### Summary
[Answer with confidence level: High/Medium/Low]

### Key Findings
1. [Finding 1]
2. [Finding 2]

### Project Context
[How this relates to our existing decisions/architecture]

### Sources
- [Source 1] — [confidence]
- [Source 2] — [confidence]

### Implications
- [What this means for our work]

### Open Questions
- [What's still unclear]
```

---

## Research vs Other Commands

| Need | Command |
|------|---------|
| Understand current state | `/research` |
| Investigate a factual question | `/research` |
| Generate ideas or options | `/brainstorm` |
| Explore a problem space | `/brainstorm` |
| Update strategy from learnings | `/reflect` |
| Make technical decisions | `/architecture` |

---

## Confidence Levels

Rate findings by confidence:

| Level | Meaning |
|-------|---------|
| **High** | Official docs, verified in project context, or tested |
| **Medium** | Authoritative source, consistent with multiple references |
| **Low** | Community source, single reference, or inference |

Always cite sources with confidence levels.

---

## Context7 Usage

For library/framework questions, use Context7 if available:

```
1. mcp__plugin_context7_context7__resolve-library-id
   - Find the library ID

2. mcp__plugin_context7_context7__query-docs
   - Query specific documentation
```

This provides authoritative, version-specific documentation.

---

## Guardrails

1. **Clarify before diving** — Understand what's actually being asked
2. **Check project context** — Respect existing decisions
3. **Cite sources** — With confidence levels
4. **Present findings** — Don't make decisions for the user
5. **Stay investigative** — This is about understanding, not generating

---

## Output Expectations

Research produces **findings**, not decisions.

**Good output:**
- "Based on X, Y, and Z, the options are A and B"
- "The documentation says X with high confidence"
- "Our ADR-003 already decided Y"

**Bad output:**
- "Let's do X" (that's a decision, not research)
- "I think we should..." (presents options, not conclusions)

---

## Related Commands

- `/brainstorm` — When you need to generate options or explore creatively
- `/reflect` — When you need to update strategy based on learnings
- `/architecture` — When you need to make technical decisions
- `/strategy` — When you need to discover strategic context
