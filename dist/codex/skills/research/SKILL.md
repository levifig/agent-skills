---
name: research
description: >-
  Use for project reflection, brainstorming, and vision evolution. Covers
  assessing project state, exploring problem spaces, generating ideas, and
  evolving strategic direction. Activate when stepping back to think,
  researching topics, or updating VISION.md.
version: 1.13.2
---

# Research

Patterns for zooming out, brainstorming, and evolving project direction.

## Philosophy

**Research is about understanding before doing.**

Research activities:
1. Assess project state and lessons learned
2. Explore problem spaces with structured inquiry
3. Generate and refine ideas through brainstorming
4. Evolve strategic direction when evidence supports it

Research is NOT about:
- Implementing solutions
- Making tactical decisions
- Writing code or specifications

## Quick Reference

| Need | Action |
|------|--------|
| Understand project state | Review sessions, docs, recent changes |
| Explore a topic | Structured inquiry with confidence hierarchy |
| Generate ideas | Brainstorm with interview and synthesis |
| Evolve VISION | Propose changes with evidence |

## Research Modes

### Mode 1: Project State Assessment

**Trigger:** "What's the current state?" / "Catch me up"

**Process:**
1. Read existing docs (VISION, ARCHITECTURE, REQUIREMENTS)
2. Check recent sessions for lessons learned
3. Review recent commits for context
4. Identify gaps, contradictions, or stale information
5. Summarize current state with recommendations

**Output:** State assessment with actionable insights

### Mode 2: Topic Exploration

**Trigger:** "Research X" / "How should we approach Y?"

**Process:**
1. Interview user to understand the question deeply
2. Apply confidence hierarchy for sources
3. Synthesize findings with tradeoffs
4. Present options with recommendations

**Output:** Research findings with options and recommendation

### Mode 3: Brainstorming

**Trigger:** "Let's brainstorm" / "Ideas for X"

**Process:**
1. Interview to understand constraints and goals
2. Generate diverse options (quantity over quality initially)
3. Filter through constraints
4. Refine promising directions
5. Present shaped options for decision

**Output:** Curated options with pros/cons

### Mode 4: Vision Evolution

**Trigger:** "Should we change direction?" / "Update VISION"

**Process:**
1. Gather evidence (sessions, feedback, market changes)
2. Identify what's changed since last VISION update
3. Propose specific changes with rationale
4. Get user approval before any edits

**Output:** VISION.md change proposal (not direct edit)

## Confidence Hierarchy

When researching, prioritize sources in this order:

```
1. Project context (highest confidence)
   - VISION.md, ARCHITECTURE.md, REQUIREMENTS.md
   - Session files and lessons learned
   - Existing codebase patterns

2. Authoritative documentation
   - Context7 for library docs
   - Official documentation sites
   - RFCs and specifications

3. Community knowledge
   - Stack Overflow (verified answers)
   - GitHub issues and discussions
   - Blog posts from recognized experts

4. General web (lowest confidence)
   - Search results
   - Forum posts
   - Unverified sources
```

**Rule:** Always check project context first. External research should supplement, not replace, understanding of existing decisions.

## Interview Patterns

### Before Any Research

Ask clarifying questions:

| Area | Questions |
|------|-----------|
| Scope | What specifically are you trying to understand? |
| Context | What do you already know? What have you tried? |
| Constraints | What constraints should guide the research? |
| Output | What decision will this research inform? |

### For Topic Exploration

```
1. What problem are we trying to solve?
2. What constraints exist (technical, time, team)?
3. What would success look like?
4. What have we already considered and rejected?
5. Are there non-obvious angles to explore?
```

### For Brainstorming

```
1. What's the ideal outcome if constraints didn't exist?
2. What's the minimum viable approach?
3. What approaches have we seen work elsewhere?
4. What would we do if we had 10x the resources? 1/10?
5. What's the contrarian view?
```

## Output Formats

### State Assessment

```markdown
## Project State Assessment

**Date:** YYYY-MM-DD

### Current Position
- [Summary of where the project stands]

### Recent Progress
- [Key accomplishments from recent sessions]

### Lessons Learned
- [Insights from implementation feedback]

### Open Questions
- [Unresolved decisions or gaps]

### Recommendations
1. [Actionable next step]
2. [Actionable next step]
```

### Research Findings

```markdown
## Research: [Topic]

**Question:** [What we're trying to understand]

### Summary
[One-paragraph answer with confidence level]

### Options Considered

#### Option A: [Name]
- **Pros:** ...
- **Cons:** ...
- **Best for:** ...

#### Option B: [Name]
- **Pros:** ...
- **Cons:** ...
- **Best for:** ...

### Recommendation
[Which option and why, with caveats]

### Sources
- [Source 1 with confidence level]
- [Source 2 with confidence level]
```

### Vision Change Proposal

```markdown
## VISION.md Change Proposal

**Proposed by:** [Session/Date]
**Evidence:** [What prompted this]

### Current State
[Quote relevant VISION.md section]

### Proposed Change
[New text]

### Rationale
[Why this change is warranted]

### Impact
- [What this changes downstream]
- [ADRs that may need updating]
- [Requirements that may be affected]

### Approval Required
This proposal requires user approval before implementation.
```

## Integration with Workflow

Research fits at the top of the product development hierarchy:

```
RESEARCH → VISION → ARCHITECTURE → REQUIREMENTS → SPECS → TASKS
    │         │
    └─────────┘
    evolves
```

**Research informs Vision changes.** Vision changes cascade down through Architecture, Requirements, and Specs.

## When to Research

| Situation | Research Mode |
|-----------|---------------|
| Starting new project | Project state + Topic exploration |
| Stuck on approach | Topic exploration + Brainstorming |
| Finishing major work | Project state (capture learnings) |
| External change | Vision evolution |
| Quarterly review | Project state + Vision evolution |

## When NOT to Research

- **When action is clear** - Don't research to avoid doing
- **When already decided** - Check ADRs first
- **When time-critical** - Act, then document learnings
- **For trivial questions** - Just answer them

## Critical Rules

### Always
- Interview before researching
- Check project context first
- Cite sources with confidence levels
- Present options, let user decide
- Get approval before editing VISION

### Never
- Edit VISION without explicit approval
- Research indefinitely (set time bounds)
- Ignore existing project decisions
- Present research as implementation plan
- Skip the interview step

## Scripts

| Script | Usage | Description |
|--------|-------|-------------|
| `check-sessions.sh` | Review recent sessions | Find lessons learned |
| `git-context-summary.sh` | Summarize git history | Understand recent changes |

## Related Skills

- **orchestration** - For acting on research findings
- **foundations** - For documentation standards
