# Product Planning

Guidelines for product specifications, roadmaps, and feature definition. Inspired by Shape Up (Ryan Singer/37signals).

## Contents

- Core Principle
- Shape Up Methodology
- Roadmap: Now / Next / Later
- Feature Specification
- Acceptance Criteria
- Edge Case Categories
- Project Constraints
- Critical Rules
- Roadmap Updates
- Validation Checklist

## Core Principle

**Insist on complete, polished releases - no MVPs or quick hacks.**

**OpenCode requirement:** Interview the user with the `question` tool before drafting any plan or research strategy.

Every release should be:
- Complete (all planned functionality works)
- Polished (good UX, no rough edges)
- Delightful (exceeds expectations)

## Shape Up Methodology

Adapted from [Shape Up](https://basecamp.com/shapeup) for AI agent orchestration.

### Complexity-Based Sizing

**Size work by complexity, not time. Agents don't have time budgets.**

| Size | Complexity | Suitable For |
|------|-----------|--------------|
| Small | Single concern, clear approach | Bug fixes, minor enhancements |
| Medium | Multiple concerns, known patterns | Feature additions, refactors |
| Large | Cross-cutting, needs exploration | Major features (should be split) |

If work is too complex for its size, it needs more shaping or should be split.

### Shaping Before Building

**Shape = define boundaries, not tasks.**

Orchestrator shapes before delegating to implementation agents:

1. **Problem**: What are we solving? (not "build feature X")
2. **Complexity**: Small / medium / large?
3. **Solution sketch**: Rough direction, not detailed design
4. **Rabbit holes**: What to avoid (explicitly list pitfalls)
5. **No-gos**: What's out of scope (be specific)

```markdown
## Shaped Task: Improve Export Performance

**Problem**: CSV exports timeout on large datasets (>10k rows)
**Complexity**: Small (single concern, clear approach)
**Solution sketch**: Stream to temp file, return async download link
**Rabbit holes**:
- Don't rewrite the export format
- Don't add progress indicators (future scope)
**No-gos**:
- Excel format support
- Scheduled exports
```

### Priority Ordering

**Ship tracks in priority order. Drop from the end, not the middle.**

When scope exceeds complexity sizing:
1. **Ship in order** - Deliver the highest-priority tracks first
2. **Go/no-go gates** - Binary check between tracks (does the previous track pass its test conditions?)
3. **Drop from end** - If later tracks won't fit, drop them into follow-up specs

If the core tracks don't fit, the shaping was wrong.

### Go/No-Go Gates

**Know when to stop.**

Set explicit gates between priority tracks:

```markdown
## Go/No-Go Gates

After each priority track completes:
- [ ] Does this track pass its test conditions?
- [ ] Are we still solving the right problem?
- [ ] Is the next track still worth pursuing?

If any "no" answer → STOP, reassess with user before continuing
```

Go/no-go gates prevent sunk cost fallacy. Stopping to reshape is not failure.

### Hill Charts

**Track progress visually: uphill (figuring out) → downhill (executing).**

```
          Figuring out ← → Executing
                    ___
                   /   \
        Unknown   /     \   Known
        territory/       \  territory
               /         \
              /           \
    --------'             '--------
    Start                     Done
```

**In session files:**

```markdown
## Progress

| Task | Hill Position | Notes |
|------|---------------|-------|
| API design | ⬆️ 70% (still exploring) | Auth approach unclear |
| Data model | ⬇️ 30% (executing) | Schema finalized |
| UI components | ⬇️ 80% (nearly done) | Just styling left |
```

When a task is "stuck uphill" for too long, it signals shaping problems.

### Betting Table

**Fresh prioritization, not backlog grooming.**

Each cycle, review what to bet on:

```markdown
## Betting Table

**Shaped and ready:**
1. Export performance fix (small) ✅ Bet
2. Dashboard redesign (medium) ✅ Bet
3. API v2 migration (large) ❌ Too big, re-shape

**Not ready (needs shaping):**
- "Improve search" - too vague
- "Mobile support" - no solution sketch

**Deliberately not betting:**
- Low-impact requests (say no, don't queue)
```

**No backlogs.** If something isn't worth betting on now, let it go. Good ideas resurface.

### Cool-Down

**Build in slack.**

After completing a cycle:
- Fix small bugs
- Address tech debt
- Explore new ideas
- **Rest**

Don't immediately start the next big thing. Sustainable pace produces better work.

## Roadmap: Now / Next / Later

### Philosophy

Roadmaps communicate direction, not dates. The Now/Next/Later framework provides clarity without false precision.

```
+------------+------------+------------+
|    NOW     |    NEXT    |   LATER    |
+------------+------------+------------+
| Committed  | Planned    | Vision     |
| Detailed   | Directional| Aspirational|
| In Progress| Refined    | Exploratory |
+------------+------------+------------+
```

### Now (Active Development)

**What belongs here:**
- Work actively being built
- Fully scoped and estimated
- Assigned resources
- Clear acceptance criteria

**Level of detail:**
- Specific features with user stories
- Technical approach defined
- Dependencies identified

### Next (Planned)

**What belongs here:**
- Committed direction
- Refined enough to estimate roughly
- May have open questions
- Subject to adjustment

**Level of detail:**
- High-level feature areas
- Key outcomes, not implementation

### Later (Vision)

**What belongs here:**
- Aspirational goals
- Market opportunities
- Strategic bets
- Long-term vision

**Level of detail:**
- Themes and directions
- Why it matters

### What NOT to Include

**No version numbers:**
```markdown
# Bad
## v1.2.0 (Q1 2025)

# Good
## Now
```

**No date estimates:**
```markdown
# Bad
- Feature A (2 weeks)

# Good
- Feature A
```

**No timeframes:**
```markdown
# Bad
## Phase 1 (Weeks 1-4)

# Good
## Now
```

## Feature Specification

### Minimum Requirements

Every feature must have:

1. **Clear problem statement** - What problem does this solve?
2. **User persona** - Who benefits?
3. **Acceptance criteria** - How do we know it's done?
4. **Edge cases** - What could go wrong?

### Feature Template

```markdown
# Feature: [Name]

## Overview

**Problem Statement**: What problem does this solve?
**Target Users**: Who benefits from this feature?
**Value Proposition**: Why is this valuable?

## Requirements

### Functional Requirements
1. [Requirement 1]
2. [Requirement 2]

### Non-Functional Requirements
- **Performance**: [Expectations]
- **Security**: [Requirements]
- **Accessibility**: [Considerations]

## User Stories

### Story 1: [Title]

**As a** [user persona]
**I want to** [action]
**So that** [benefit]

#### Acceptance Criteria
- [ ] Given [context], when [action], then [outcome]
- [ ] Given [context], when [action], then [outcome]

## Edge Cases

| Scenario | Expected Behavior |
|----------|-------------------|
| [Edge case 1] | [How system should respond] |
| [Edge case 2] | [How system should respond] |

## Out of Scope
- [What this feature does NOT include]

## Dependencies
- [Dependency 1]

## Open Questions
- [ ] [Question needing resolution]
```

## Acceptance Criteria

### Given-When-Then Format

```markdown
Given [precondition/context]
When [action is performed]
Then [expected outcome]
```

### Good vs Bad Criteria

| Good | Bad |
|------|-----|
| Specific and testable | Vague or subjective |
| Single behavior | Multiple behaviors |
| Observable outcome | Internal implementation |
| User-focused | Developer-focused |

**Good example:**
```markdown
Given I am logged in as an operator,
when I click the Export button,
then a CSV download starts within 2 seconds
```

**Bad example:**
```markdown
- Export should be fast
- Data should be correct
```

## Edge Case Categories

### Input Edge Cases
- Empty input
- Maximum/minimum values
- Invalid formats
- Special characters

### State Edge Cases
- First-time user
- Concurrent access
- Partial completion
- System under load

### Environment Edge Cases
- Network failure
- Timeout
- Service unavailable

### Authorization Edge Cases
- Expired session
- Revoked access
- Cross-tenant access attempt

## Project Constraints

Document in the file configured in `.agents/loaf.json` (default: `docs/CONSTRAINTS.md`):

- **Processing model**: Batch vs real-time, latency expectations
- **Deployment model**: Single/multi-tenant, cloud/on-prem
- **Architecture boundaries**: State management, resource lifecycle
- **Configuration model**: How behavior is customized
- **User personas**: Access levels, primary use cases
- **Licensing/compliance**: Requirements and audit needs

## Critical Rules

### Always
- Define clear acceptance criteria
- Consider all user personas
- Document edge cases
- Prioritize user value

### Never
- Ship incomplete features
- Use dates for estimates
- Over-engineer
- Skip accessibility requirements

## Roadmap Updates

| Trigger | Action |
|---------|--------|
| Feature ships | Move from Now to "Completed" or remove |
| Priorities change | Reorder items, move between buckets |
| New opportunity | Add to appropriate bucket |
| Scope change | Update descriptions |

### Review Cadence
- **Weekly**: Update Now bucket
- **Monthly**: Review Next bucket
- **Quarterly**: Revisit Later bucket

## Validation Checklist

### Roadmap
- [ ] No version numbers in planning sections
- [ ] No date/time estimates
- [ ] Now items are specific and actionable
- [ ] Next items have clear direction
- [ ] Later items focus on outcomes

### Feature Spec
- [ ] Problem statement is clear
- [ ] Target users identified
- [ ] All user stories have acceptance criteria
- [ ] Edge cases documented
- [ ] Out of scope explicitly stated
- [ ] Dependencies identified
- [ ] Open questions captured
