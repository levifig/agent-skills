# Product Planning

Guidelines for product specifications, roadmaps, and feature definition. Inspired by Shape Up (Ryan Singer/37signals).

## Core Principle

**Insist on complete, polished releases - no MVPs or quick hacks.**

**OpenCode requirement:** Interview the user with the `question` tool before drafting any plan or research strategy.

Every release should be:
- Complete (all planned functionality works)
- Polished (good UX, no rough edges)
- Delightful (exceeds expectations)

## Shape Up Methodology

Adapted from [Shape Up](https://basecamp.com/shapeup) for AI agent orchestration.

### Appetite Over Estimates

**Don't estimate how long work will take. Decide how much time it's worth.**

```markdown
# Bad (estimate-driven)
"How long will this feature take?"
→ "About 2 weeks"
→ Scope creeps → Takes 4 weeks

# Good (appetite-driven)
"We're willing to invest 2 days on this."
→ Shape to fit the appetite
→ Cut scope if needed, not time
```

**Appetite levels:**
| Appetite | Size | Suitable For |
|----------|------|--------------|
| Small | 1-2 days | Bug fixes, minor enhancements |
| Medium | 3-5 days | Feature additions, refactors |
| Large | 1-2 weeks | Major features (rare) |

If work can't be shaped to fit the appetite, it's not ready - needs more shaping or should be broken down.

### Shaping Before Building

**Shape = define boundaries, not tasks.**

PM shapes before delegating to implementation agents:

1. **Problem**: What are we solving? (not "build feature X")
2. **Appetite**: How much is it worth?
3. **Solution sketch**: Rough direction, not detailed design
4. **Rabbit holes**: What to avoid (explicitly list pitfalls)
5. **No-gos**: What's out of scope (be specific)

```markdown
## Shaped Task: Improve Export Performance

**Problem**: CSV exports timeout on large datasets (>10k rows)
**Appetite**: 1 day (small batch)
**Solution sketch**: Stream to temp file, return async download link
**Rabbit holes**:
- Don't rewrite the export format
- Don't add progress indicators (future scope)
**No-gos**:
- Excel format support
- Scheduled exports
```

### Fixed Time, Variable Scope

**Time is fixed. Scope flexes.**

When running out of appetite:
1. **Cut scope** - Remove nice-to-haves, keep must-haves
2. **Simplify** - Reduce complexity, accept rougher edges
3. **Document what's left** - Create follow-up issues for cut scope

Never extend time. If it doesn't fit, the shaping was wrong.

### Circuit Breakers

**Know when to stop.**

Set explicit checkpoints for re-evaluation:

```markdown
## Circuit Breaker Points

After 50% of appetite spent:
- [ ] Is the core approach working?
- [ ] Are we still solving the right problem?
- [ ] Should we stop and reshape?

If two "no" answers → STOP, reassess with user
```

Circuit breakers prevent sunk cost fallacy. Stopping early to reshape is not failure.

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
1. Export performance fix (1 day appetite) ✅ Bet
2. Dashboard redesign (1 week appetite) ✅ Bet
3. API v2 migration (2 week appetite) ❌ Too big, re-shape

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

## Plan File Storage

Implementation plans are stored in `.agents/plans/` for persistence across context resets.

### Location & Naming

```
.agents/plans/YYYYMMDD-HHMMSS-{plan-slug}.md
```

**Generate timestamps:**
```bash
date -u +"%Y%m%d-%H%M%S"  # Filename: 20251204-143500
date -u +"%Y-%m-%dT%H:%M:%SZ"  # Frontmatter: 2025-12-04T14:35:00Z
```

**Good**: `20251204-143500-api-auth-design.md`
**Bad**: `api-auth-design.md` (missing timestamp)

### Plan File Format

```yaml
---
session: 20251204-140000-feature-auth       # Parent session ID
council: 20251204-142000-api-approach       # If plan came from council (optional)
created: 2025-12-04T14:35:00Z               # When plan was created
status: pending                              # pending | approved | superseded
---

# [Plan Title]

## Overview

Brief description of what this plan covers.

## Context

What led to this plan. Reference session, council, or user requirements.

## Implementation Steps

1. **Step 1**: Description
   - Details
   - Expected outcome

2. **Step 2**: Description
   - Details
   - Expected outcome

## Dependencies

- What must be complete before this plan can execute

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Risk 1 | How to address |

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2

## Out of Scope

What this plan explicitly does NOT cover.
```

### Plan Lifecycle

| Status | Description |
|--------|-------------|
| `pending` | Created, awaiting user approval |
| `approved` | User approved, ready for implementation |
| `superseded` | Replaced by a newer plan |

### PM Workflow

1. **Receive plan** from Task(Plan) or exploration
2. **Save to `.agents/plans/`** with proper filename
3. **Update session** with plan reference in `plans:` array
4. **Present to user** for approval
5. **Mark approved** when user confirms
6. **Reference in agent prompts** during implementation

### Linking Plans

**In session files:**
```yaml
plans:
  - 20251204-143500-api-auth-design.md
  - 20251204-150000-frontend-components.md
```

**In council files:**
```yaml
council:
  implementation_plan: "../plans/20251204-143500-api-auth-design.md"
```

**In agent prompts:**
```
Reference the implementation plan at .agents/plans/20251204-143500-api-auth-design.md
for the approved approach.
```

### Superseding Plans

When a plan needs revision:

1. Create new plan file with fresh timestamp
2. Update old plan status to `superseded`
3. Add note in old plan: `Superseded by: 20251205-100000-revised-approach.md`
4. Update session `plans:` array with new plan
