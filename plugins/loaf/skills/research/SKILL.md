---
name: research
description: >-
  Conducts project assessment and topic investigation. Use when stepping back to
  understand the big picture or when the user asks "what's the current state?"
  Produces state assessments, research findings with ranked options, or vision
  change proposa...
user-invocable: true
argument-hint: '[topic]'
version: 2.0.0-dev.11
---

# Research

Patterns for zooming out, investigating topics, and evolving project direction.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Input Parsing
- Confidence Hierarchy
- Research Modes
- Related Skills

**Input:** $ARGUMENTS

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

## Verification

- Interview step was completed before research began
- All findings cite sources with confidence levels (High/Medium/Low)
- VISION.md was not modified without explicit user approval

## Quick Reference

| Input Pattern | Mode |
|---------------|------|
| Empty / "project state" / "catch me up" | State Assessment |
| Topic or question | Topic Investigation |
| "let's brainstorm" / "ideas for X" | Brainstorming |
| "should we change direction?" / "update VISION" | Vision Evolution |

## Topics

| Topic | Template | Use When |
|-------|----------|----------|
| State Assessment | [state-assessment.md](templates/state-assessment.md) | Producing a project state overview |
| Topic Investigation | [findings.md](templates/findings.md) | Documenting research findings |

## Input Parsing

Parse `$ARGUMENTS` to determine mode:

| Input Pattern | Mode |
|---------------|------|
| Empty / "project state" / "catch me up" | State Assessment |
| Topic or question | Topic Investigation |
| "let's brainstorm" / "ideas for X" | Brainstorming |
| "should we change direction?" / "update VISION" | Vision Evolution |

## Confidence Hierarchy

Prioritize sources in this order:

1. **Project context** (highest) -- VISION.md, ARCHITECTURE.md, session files, codebase patterns
2. **Authoritative docs** -- Context7, official docs, RFCs
3. **Community knowledge** -- Stack Overflow (verified), GitHub issues, expert blogs
4. **General web** (lowest) -- Search results, unverified sources

Always check project context first. Rate findings: **High** (official/verified), **Medium** (authoritative, consistent), **Low** (community, single reference).

## Research Modes

### State Assessment

**Trigger:** Empty input, "project state", "catch me up"

1. Read project documents: VISION.md, STRATEGY.md, ARCHITECTURE.md
2. Check ideas (`.agents/ideas/`) and specs (`docs/specs/`)
3. Review recent sessions (`.agents/sessions/`)
4. Check recent commits: `git log --oneline -20`
5. Synthesize following [state-assessment template](templates/state-assessment.md)

### Topic Investigation

**Trigger:** Specific topic or question

1. **Interview** with AskUserQuestion: what are you trying to understand? What context do you have? What decision will this inform?
2. Check project context first (ADRs, ARCHITECTURE, sessions)
3. Apply confidence hierarchy for external sources
4. Create findings document following [findings template](templates/findings.md)

**Output file:** `.agents/drafts/{YYYYMMDD}-{HHMMSS}-research-{slug}.md`

### Brainstorming

**Trigger:** "Let's brainstorm" / "Ideas for X"

1. Interview to understand constraints and goals
2. Generate diverse options (quantity first)
3. Filter through constraints
4. Refine promising directions
5. Present shaped options with pros/cons

### Vision Evolution

**Trigger:** "Should we change direction?" / "Update VISION"

1. Gather evidence (sessions, feedback, market changes)
2. Identify what's changed since last VISION update
3. Propose specific changes with rationale
4. **Get user approval before any edits**

## Related Skills

- **orchestration** - For acting on research findings
- **brainstorm** - For generative exploration
- **reflect** - For updating strategy post-shipping
- **architecture** - For making technical decisions
- **strategy** - For discovering strategic context
