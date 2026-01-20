# Council Workflow

Councils are deliberation mechanisms for decisions with multiple valid approaches. They bring diverse perspectives to reach well-reasoned conclusions.

## When to Convene a Council

**Use councils for:**
- User explicitly requests it
- Multiple valid approaches with unclear winner
- Significant architectural decisions
- Technology selection with trade-offs
- Cross-cutting concerns affecting multiple domains

**Don't use councils for:**
- Simple decisions with clear best practice
- Routine implementation choices
- Single-domain decisions (just ask that expert)
- Time-sensitive decisions (councils add overhead)
- Trivial matters (HTTP status codes, naming)

## Core Principles

1. **Councils are consultative, not executive** - They advise, user decides
2. **Always odd number** - 5 or 7 agents to prevent ties
3. **Domain-matched composition** - Select relevant experts
4. **Parallel deliberation** - Spawn all agents simultaneously
5. **User approval required** - NEVER proceed without explicit approval

## Council Composition

### Decision Type to Agents

| Decision Type | 5-Agent Council | Extended (7) |
|---------------|-----------------|--------------|
| Database/Data | dba, backend-dev, devops, security, code-reviewer | + frontend-dev, docs |
| API Design | backend-dev, frontend-dev, security, docs, code-reviewer | + dba, devops |
| UI/UX | design, frontend-dev, product, backend-dev, code-reviewer | + testing-qa, docs |
| Infrastructure | devops, backend-dev, security, dba, code-reviewer | + frontend-dev, docs |
| Security | security, backend-dev, devops, dba, code-reviewer | + frontend-dev, docs |
| Full Architecture | backend-dev, frontend-dev, dba, devops, security, code-reviewer, docs | N/A |
| Feature Scope | product, design, backend-dev, frontend-dev, testing-qa | + security, code-reviewer |

### Composition Rules

- **5 or 7 agents** (always odd)
- Primary domain expert must be included
- Include `code-reviewer` for maintainability
- Include `security` if security-relevant
- PM coordinates but does NOT vote

## Deliberation Process

### Step 1: Define Decision Question

**Good questions:**
- "Should we use Redis, PostgreSQL, or JWT-only for session storage?"
- "Which frontend framework: React, Vue, or Svelte?"

**Poor questions:**
- "How should we build this?" (too vague)
- "Is this a good idea?" (not a choice)

### Step 2: Compose Council

Present to user for approval:

```markdown
## Proposed Council Composition

**Decision**: Session storage strategy

**Agents** (5):
1. dba - Database implications
2. backend-dev - Application integration
3. devops - Operational complexity
4. security - Session security
5. code-reviewer - Long-term maintenance

Do you approve this composition?
```

### Step 3: Spawn All Agents in Parallel

**Critical**: Spawn ALL agents simultaneously for independent perspectives.

Each agent receives:
- Same decision question
- Same options
- Same context
- Different perspective prompt

```python
Task(
  subagent_type="dba",
  prompt="""
  Provide database perspective on session storage.

  Options: Redis, PostgreSQL, JWT-only

  Your Perspective: Data persistence, query patterns,
  backup/recovery, consistency, operational complexity.

  Council: .agents/councils/20251210-153000-session-storage.md
  """
)
# Spawn other agents with their domain perspectives...
```

### Step 4: Collect Perspectives

Document each agent's:
- Recommended option
- Rationale
- Concerns about alternatives

### Step 5: Synthesize Findings

PM creates synthesis:

```markdown
## Synthesis

### Consensus Points
All agents agree: [common ground]

### Key Disagreements
[Where perspectives differ]

### Trade-Off Analysis

#### Option A
**Pros**: ...
**Cons**: ...
**Best for**: ...

#### Option B
**Pros**: ...
**Cons**: ...
**Best for**: ...

### Recommendation
[PM's synthesis of agent perspectives]
```

### Step 6: Present to User

```markdown
## Options Evaluated

1. **PostgreSQL** (Recommended)
   - Pros: Operational simplicity, existing expertise
   - Cons: Slower than Redis

2. **Redis** (Valid Alternative)
   - Pros: Faster, purpose-built
   - Cons: Additional infrastructure

## Your Decision
Which option do you choose?
```

**WAIT** for explicit user decision.

### Step 7: Record Decision

Update council file with:
- Chosen option
- Who decided (user)
- When decided
- Rationale
- Action items

## Council File Format

**Location**: `.agents/councils/YYYYMMDD-HHMMSS-<topic>.md`
**Archive location:** `.agents/councils/archive/` (move after linked session includes council outcome summary; archive indefinitely).
**Link policy**: Documents outside `.agents/` must not reference `.agents/` files. Keep `.agents/` links contained within `.agents/` artifacts, and update them when files move to `.agents/<type>/archive/`.
**Report capture**: If outputs are captured as a report, follow report frontmatter policy, including `archived_at` and `archived_by` when archived (see Sessions → Reports Processed).

```yaml
---
council:
  topic: "Session storage strategy"
  timestamp: "2025-12-10T15:30:00Z"
  status: approved              # approved | rejected | deferred | archived
  archived_at: "2025-12-10T18:00:00Z"   # Required when archived
  archived_by: "agent-pm"               # Optional; fill when archived (enforced by /review-sessions)
  session: "20251210-140000-user-auth"  # REQUIRED
  participants:
    - dba
    - backend-dev
    - devops
    - security
    - code-reviewer             # Min 5, MUST be odd
  decision: "postgresql"
  linear_issue: "AUTH-45"       # Optional
---

# Council: Session Storage Strategy

## Context
[Why this council was convened]

## Options Considered
[Options with trade-offs]

## Decision
**Chosen**: PostgreSQL
**Rationale**: [Why]

## Implementation Notes
[Follow-up actions]
```

## Council vs Implementation

| Aspect | Council | Implementation |
|--------|---------|----------------|
| Purpose | Consultation | Build things |
| Agent Action | Provide perspectives | Write code |
| File Changes | None (read-only) | Modifications |
| Spawning | All parallel | Sequential/parallel |
| Outcome | Recommendation | Work completed |
| User Role | Approves decision | Reviews results |

After council concludes and user decides, PM spawns implementation agents to execute.

## Archiving Guidance

- Archive councils indefinitely (no deletion policy).
- When moving a council to `.agents/councils/archive/`, update any references inside `.agents/` files to the archived path.
- Documents outside `.agents/` must not reference `.agents/` files.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Council for simple decisions | Single agent or PM judgment |
| Wrong composition | Match agents to decision domain |
| Even number of agents | Always 5 or 7 |
| PM as council member | PM coordinates, doesn't vote |
| Proceed without user approval | Wait for explicit decision |
| Implement during council | Advise only, implement after |
| Skip documentation | Record decision in council file |
| Archive council without session summary | Summarize in session before archive (status + move) |

## Checklist

**Before convening:**
- [ ] Decision genuinely needs council
- [ ] Decision question is clear
- [ ] Composition planned (5-7, odd, relevant)
- [ ] User approved composition

**During deliberation:**
- [ ] All agents spawned in parallel
- [ ] Same context provided to all
- [ ] Each agent focused on their domain
- [ ] All perspectives collected

**After deliberation:**
- [ ] Synthesis created
- [ ] Options presented to user
- [ ] Explicit user decision obtained
- [ ] Council file updated
- [ ] Archive council after linked session summary exists (set status + `archived_at` + `archived_by`, move)
- [ ] Update `.agents/` references to archived paths (no `.agents` links outside `.agents/`)
- [ ] Archive indefinitely (no deletion policy)
- [ ] Consider ADR if architectural
- [ ] Implementation spawned (if applicable)
