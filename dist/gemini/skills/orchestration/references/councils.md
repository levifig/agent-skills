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
6. **Any agent can participate** - All agents (implementation and advisory) can join councils
7. **Ad-hoc specialists allowed** - Create specialist personas when domain expertise is needed

## Council Composition

### Decision Type to Agents

| Decision Type | 5-Agent Council | Extended (7) |
|---------------|-----------------|--------------|
| Database/Data | dba, backend-dev, devops, security, qa | + frontend-dev, docs |
| API Design | backend-dev, frontend-dev, security, docs, qa | + dba, devops |
| UI/UX | design, frontend-dev, product, backend-dev, qa | + docs |
| Infrastructure | devops, backend-dev, security, dba, qa | + frontend-dev, docs |
| Security | security, backend-dev, devops, dba, qa | + frontend-dev, docs |
| Full Architecture | backend-dev, frontend-dev, dba, devops, security, qa, docs | N/A |
| Feature Scope | product, design, backend-dev, frontend-dev, qa | + security |

### Composition Rules

- **5 or 7 agents** (always odd)
- Primary domain expert must be included
- Include domain devs for code review (backend-dev/frontend-dev)
- Include `security` if security-relevant
- PM coordinates but does NOT vote
- Any agent type can participate (implementation agents like backend-dev bring valuable perspectives)

### Ad-Hoc Specialist Personas

When a council needs expertise not covered by existing agents, PM can create a specialist persona:

```markdown
## Proposed Council Composition

**Decision**: Real-time data pipeline architecture

**Agents** (5):
1. backend-dev - Application integration
2. dba - Data storage patterns
3. devops - Infrastructure and scaling
4. **[Ad-hoc] Streaming Specialist** - Kafka/event-driven expertise
5. qa - Testing distributed systems

**Ad-hoc Specialist Prompt**:
> You are a streaming data specialist with deep expertise in Apache Kafka,
> event-driven architectures, and real-time data pipelines. Evaluate options
> from the perspective of throughput, latency, ordering guarantees, and
> operational complexity.
```

Ad-hoc specialists are spawned as general-purpose agents with a specialized prompt.

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
5. backend-dev (or frontend-dev) - Long-term maintainability (domain review)

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

### Step 4: Collect Individual Reports

Each agent provides a structured report with:
- **Recommendation**: Their preferred option
- **Pros**: Benefits from their domain perspective
- **Cons**: Drawbacks and risks they see
- **Suggestions**: Implementation considerations if this option is chosen

### Step 5: Present Individual Perspectives

Display each agent's report separately so the user sees each perspective clearly:

```markdown
## Individual Agent Reports

---

### 🗄️ DBA Perspective

**Recommendation**: PostgreSQL

**Pros**:
- Native session table with existing infrastructure
- ACID compliance for session integrity
- Familiar query patterns for the team

**Cons**:
- Higher latency than in-memory stores
- Connection pool pressure under high load

**Suggestions**:
- Use connection pooling (PgBouncer)
- Consider partitioning by session date
- Index on session_id and expires_at

---

### 🔧 Backend-Dev Perspective

**Recommendation**: Redis

**Pros**:
- Sub-millisecond response times
- Built-in TTL for session expiry
- Pub/sub for session invalidation

**Cons**:
- Additional infrastructure to maintain
- Data persistence requires configuration

**Suggestions**:
- Use Redis Cluster for HA
- Configure AOF persistence
- Implement graceful fallback to DB

---

### 🛡️ Security Perspective

**Recommendation**: PostgreSQL

**Pros**:
- Audit logging built-in
- Row-level security possible
- Encryption at rest standard

**Cons**:
- Session fixation requires careful handling either way

**Suggestions**:
- Rotate session IDs on privilege change
- Implement absolute timeout regardless of storage
- Log session creation/destruction

---

[Continue for all council members...]
```

### Step 6: Synthesize Final Report

After individual reports, PM creates a combined synthesis:

```markdown
## Council Synthesis

### Consensus Points
- All agents agree session security requires ID rotation on auth changes
- All agents recommend connection pooling regardless of storage choice
- Performance is acceptable with either option for current scale

### Key Disagreements
- **DBA & Security** favor PostgreSQL for operational simplicity
- **Backend-Dev & DevOps** favor Redis for performance headroom

### Trade-Off Summary

| Aspect | PostgreSQL | Redis |
|--------|------------|-------|
| Latency | ~5-10ms | <1ms |
| Ops Complexity | Lower (existing) | Higher (new infra) |
| Persistence | Native | Requires config |
| Scaling | Vertical + read replicas | Horizontal (cluster) |

### PM Recommendation
Based on council input: **PostgreSQL** for initial implementation.

**Rationale**: Operational simplicity wins at current scale. The team has existing
PostgreSQL expertise, and latency difference is negligible for session lookups.
Redis can be introduced later if performance becomes a bottleneck.
```

### Step 7: Present Next Steps and Wait for Decision

After the synthesis, prompt the user with concrete next steps:

```markdown
## Your Decision Required

Based on the council deliberation, here are your options:

1. **Accept recommendation (PostgreSQL)**
   → Backend-dev implements session table with connection pooling

2. **Choose alternative (Redis)**
   → DevOps provisions Redis cluster, backend-dev implements client

3. **Hybrid approach**
   → PostgreSQL for persistence, Redis for hot cache
   → Higher complexity but best of both worlds

4. **Request more information**
   → Specify what additional analysis would help

5. **Defer decision**
   → Document current state, revisit when [condition]

**Which option do you choose?**
```

**CRITICAL**: Wait for explicit user response. Do not proceed until user makes a choice.

### Step 8: Record Decision

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
    - backend-dev               # Min 5, MUST be odd
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
