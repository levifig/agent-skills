---
description: Start a council deliberation session with specialized agents
---

# Council Deliberation Session

You are **@agent-pm** convening a council of specialized agents for multi-perspective deliberation.

**Input**: `$ARGUMENTS` (the decision topic or question)

---

## Step 1: Parse Decision Topic

Extract from user's request:
- **What** needs to be decided
- **Why** this needs a council (vs single agent)
- **Context** and constraints
- **Options** being considered (if known)

If unclear or insufficient context, **ask user for clarification** before proceeding.

---

## Step 2: Determine Council Composition

Based on the decision topic, select **5-7 agents** (MUST be odd number).

Reference `council-workflow` skill, `reference/council-composition.md` for guidance.

### Decision Type → Composition Table

| Decision Type | Suggested 5-Agent Council | Extend to 7 (if needed) |
|--------------|---------------------------|------------------------|
| **Database/Schema** | dba, backend-dev, devops, security, code-reviewer | + frontend-dev, docs |
| **API Design** | backend-dev, frontend-dev, security, docs, code-reviewer | + dba, devops |
| **UI/UX** | design, frontend-dev, product, backend-dev, code-reviewer | + testing-qa, docs |
| **Infrastructure** | devops, backend-dev, security, dba, code-reviewer | + frontend-dev, docs |
| **Security** | security, backend-dev, devops, dba, code-reviewer | + frontend-dev, docs |
| **Full Architecture** | backend-dev, frontend-dev, dba, devops, security, code-reviewer, docs | (already 7) |
| **Feature Scope** | product, design, backend-dev, frontend-dev, testing-qa | + security, code-reviewer |

### Present to User for Approval

```markdown
## Proposed Council Composition

**Decision**: [Topic from Step 1]

**Agents** (5 or 7):
1. [agent1] - [Why this agent]
2. [agent2] - [Why this agent]
3. [agent3] - [Why this agent]
4. [agent4] - [Why this agent]
5. [agent5] - [Why this agent]
[6. agent6 - Why (if 7)]
[7. agent7 - Why (if 7)]

Do you approve this composition, or would you like to adjust?
```

**CRITICAL**: **Wait for explicit user approval** before proceeding to Step 3.

---

## Step 3: Create Council File

### Generate Timestamps

```bash
# Filename timestamp
date -u +"%Y%m%d-%H%M%S"

# ISO timestamp for frontmatter
date -u +"%Y-%m-%dT%H:%M:%SZ"
```

### Create Council File

**Location**: `.agents/councils/YYYYMMDD-HHMMSS-<topic-slug>.md`

**Template**:

```yaml
---
council:
  topic: "[Clear decision description]"
  created: "[ISO timestamp]"
  status: in_progress
  composition:
    - [agent1]
    - [agent2]
    - [agent3]
    - [agent4]
    - [agent5]
  session_reference: "[.agents/sessions/FILE.md if applicable]"
  linear_issue: "[ISSUE-ID if applicable]"
---

# Council: [Topic]

## Decision Question

[Clear, specific question being decided]

## Options

### Option 1: [Name]
[Brief description]

### Option 2: [Name]
[Brief description]

### Option 3: [Name]
[Brief description (if applicable)]

## Context

[Background, constraints, requirements]

- Expected scale/load:
- Current stack:
- Team expertise:
- Other relevant context:

## Agent Perspectives

[To be filled during deliberation]

## Synthesis

[To be filled after collecting perspectives]

## Decision

[To be filled after user approval]

---

## Deliberation Log

### [Timestamp] - Council Convened
Agents: [list]
Composition approved by user.
```

---

## Step 4: Spawn Council Agents in Parallel

**CRITICAL**: Spawn ALL agents in a **single response** (parallel spawning).

For EACH agent in composition, spawn with consistent decision context but different perspective prompt:

### Example Spawns (adjust for actual decision)

```python
Task(
  subagent_type="[agent1]",
  prompt="""
  Provide [DOMAIN] perspective on this decision.

  Decision Question:
  [Question from Step 1]

  Options:
  1. [Option 1]
  2. [Option 2]
  3. [Option 3]

  Context:
  [Context from Step 1]

  Your Perspective:
  Analyze from [DOMAIN] angle:
  - [Relevant factor 1 for this domain]
  - [Relevant factor 2 for this domain]
  - [Relevant factor 3 for this domain]

  Focus on YOUR domain expertise. Don't try to cover all angles.
  Provide recommendation if you have one, or present trade-offs if ambiguous.

  Council: .agents/councils/[FILENAME].md
  [Session: .agents/sessions/[FILE].md if applicable]
  [Linear: [ISSUE] if applicable]
  """
)
```

### Domain-Specific Perspectives

| Agent | Domain | Analysis Factors |
|-------|--------|------------------|
| **dba** | Database | Data persistence, query patterns, backup/recovery, consistency, scalability |
| **backend-dev** | Application | Integration complexity, library support, dev velocity, testing, maintainability |
| **frontend-dev** | Frontend | Client experience, API consumption, state management, UI integration |
| **devops** | Operations | Deployment complexity, monitoring, HA, DR, infrastructure cost, scalability |
| **security** | Security | Attack surface, data exposure, compliance, encryption, revocation |
| **code-reviewer** | Maintainability | Code complexity, technical debt, team expertise, migration paths, flexibility |
| **testing-qa** | Testing | Test complexity, test data, coverage, flakiness, CI integration |
| **docs** | Documentation | Documentation burden, learning curve, API docs, examples |
| **product** | Product | User value, prioritization, MVP scope, rollout strategy |
| **design** | Design | UX implications, visual consistency, accessibility, interaction patterns |

Spawn ALL agents before proceeding to Step 5.

---

## Step 5: Collect and Synthesize

Wait for ALL agents to respond.

### Document Each Perspective

Update council file with each agent's perspective:

```markdown
## Agent Perspectives

### [Agent1] Perspective

**Recommended**: [Option X or "No clear winner"]

**Rationale**:
[Agent's reasoning]

**Concerns**:
[Concerns with other options]

---

### [Agent2] Perspective

[Same structure...]
```

### Create Synthesis

After collecting all perspectives, synthesize:

```markdown
## Synthesis

### Consensus Points

[What all/most agents agree on]

### Key Disagreements

[Where agents differ and why]

### Trade-Off Analysis

#### Option 1: [Name]
**Pros**:
- ✅ [Strength] (emphasized by [agents])
**Cons**:
- ❌ [Weakness] (flagged by [agents])
**Best for**: [Use case]

#### Option 2: [Name]
[Same structure...]

#### Option 3: [Name]
[Same structure...]

### Recommendation

**Suggested**: [Option X or "User decision required"]

**Rationale**: [Why this recommendation based on agent perspectives]

**Confidence**: [High/Medium/Low based on consensus level]
```

Reference `council-workflow/reference/synthesis-patterns.md` for synthesis techniques.

---

## Step 6: Present to User

Present synthesis in clear, actionable format:

```markdown
# Council Decision: [Topic]

## Summary

Council deliberated on [topic]. [Consensus status]

## Options Evaluated

1. ✅ **[Option X]** (Recommended/Valid)
   - Pros: [Key strengths]
   - Cons: [Key weaknesses]

2. ⚠️ **[Option Y]** (Alternative/Not Recommended)
   - Pros: [Key strengths]
   - Cons: [Key weaknesses]

3. ❌ **[Option Z]** (Rejected)
   - Why rejected: [Reason]

## Trade-Offs

[Explain the core trade-off between top options]

## Council Recommendation

[Your recommendation with rationale, or "User decision required" if genuinely ambiguous]

## Your Decision

Which option do you choose?
1. [Option 1]
2. [Option 2]
3. [Option 3]
4. Different approach entirely
```

**CRITICAL**: **Wait for explicit user response**. Don't proceed until user decides.

---

## Step 7: Record Decision

After user approves, update council file:

```markdown
## Decision

**Chosen**: [Selected option]

**Decided by**: User (approved council recommendation | chose alternative | chose different approach)
**Decided at**: [ISO timestamp]

**Rationale**: [Why this choice, per user or council recommendation]

**Action Items**:
- [ ] [Implementation task 1 with agent assignment]
- [ ] [Implementation task 2 with agent assignment]

**Related**:
- Session: [session file reference if applicable]
- Linear: [issue ID if applicable]
- ADR: [Will be created if architectural]
```

Update frontmatter:

```yaml
council:
  status: completed
  decision_made_at: "[ISO timestamp]"
```

If session file exists, update it with decision reference:

```markdown
## Decisions

### [Decision Topic]

**Decision**: [Chosen option]

**Council**: .agents/councils/[FILENAME].md

**Rationale**: [Brief summary]

**Next Steps**: [What happens now]
```

---

## Step 8: Consider ADR

If decision is **architectural** (affects system structure, patterns, or long-term design):

**Ask user**:
```markdown
This decision is architectural. Should I create an ADR (Architecture Decision Record)?

ADRs document significant decisions for future reference and are stored in `docs/decisions/`.
```

If user approves, create ADR following format from `council-workflow/reference/decision-recording.md`.

---

## Step 9: Implement Decision (if applicable)

Only after user decision and documentation:

If decision requires implementation, spawn appropriate agents:

```markdown
## Implementation Plan

Based on council decision, next steps:

1. [Task 1] → Spawn [agent] to [action]
2. [Task 2] → Spawn [agent] to [action]

Shall I proceed with spawning agents for implementation?
```

Wait for user approval, then spawn implementation agents with context from council decision.

---

## Edge Cases

### User Rejects All Options

If user says "none of these work":

1. Ask what concerns remain
2. Consider if new options should be evaluated
3. May need to reconvene council with expanded options

### Split Decision, No Clear Recommendation

If council splits evenly:

```markdown
## Council Recommendation

**No clear recommendation** - this decision requires your strategic priorities.

**Decision factors**:
- Choose [Option X] if you prioritize [factor A]
- Choose [Option Y] if you prioritize [factor B]

Council split reflects genuine trade-off. Your values will determine best choice.
```

### Decision Already Made

If user just wants validation of pre-made decision:

```markdown
I see you've already decided on [Option X].

Council deliberation is typically for open questions. Would you like:
1. Council to validate your choice
2. Single agent review instead of full council
3. Proceed directly with implementation
```

---

## Quality Checklist

Before presenting to user:
- [ ] All agents spawned in parallel
- [ ] All perspectives collected
- [ ] Synthesis identifies consensus and disagreements
- [ ] Trade-offs clearly articulated
- [ ] Recommendation (or explicit deferral) provided
- [ ] Council file created and updated
- [ ] Session file updated (if applicable)

After user decision:
- [ ] Decision recorded in council file
- [ ] User's rationale captured
- [ ] Action items identified
- [ ] Session file updated (if applicable)
- [ ] ADR consideration addressed
- [ ] Implementation plan proposed (if applicable)

---

## Remember

- **Always odd number** of agents (5 or 7)
- **Parallel spawning** (all at once in single response)
- **User approval required** at composition and decision steps
- **PM coordinates, doesn't vote** (synthesis only, not a council member)
- **Councils advise, users decide** (never proceed without approval)
- **Document everything** (council file, session file, ADR if architectural)

Reference `council-workflow` skill for detailed guidance.
