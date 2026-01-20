# Product Planning

Guidelines for product specifications, roadmaps, and feature definition.

## Core Principle

**Insist on complete, polished releases - no MVPs or quick hacks.**

**OpenCode requirement:** Interview the user with the `question` tool before drafting any plan or research strategy.

Every release should be:
- Complete (all planned functionality works)
- Polished (good UX, no rough edges)
- Delightful (exceeds expectations)

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

Document in the file configured in `.agents/config.json` (default: `docs/CONSTRAINTS.md`):

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
