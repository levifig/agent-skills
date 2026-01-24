# Architecture Command

Interview about technical decisions, update ARCHITECTURE.md, create ADRs.

**Input:** $ARGUMENTS

---

## Purpose

Technical decisions should be:
1. **Deliberate** - Made consciously, not by accident
2. **Documented** - Captured in ARCHITECTURE.md and ADRs
3. **Traceable** - Future readers understand why

---

## CRITICAL: Interview First

**Before making any architectural decision, understand:**

1. What decision needs to be made?
2. What constraints exist (technical, business, team)?
3. What options have already been considered?
4. What would "good enough" look like vs "ideal"?
5. What's the impact of getting this wrong?

Use `AskUserQuestion` to gather context.

---

## Process

### Step 1: Understand the Decision

Parse `$ARGUMENTS` to understand what decision is needed.

**If unclear:** Ask clarifying questions about:
- The problem being solved
- Technical constraints
- Business constraints
- Time/resource constraints

### Step 2: Gather Context

1. **Read existing documents:**
   - `docs/VISION.md` - Strategic direction
   - `docs/ARCHITECTURE.md` - Current technical design
   - `docs/decisions/ADR-*.md` - Previous decisions

2. **Check for relevant patterns:**
   - How similar problems were solved
   - Established conventions in the codebase
   - Previous attempts and outcomes

3. **Review session history:**
   - Implementation learnings
   - Pain points from previous work

### Step 3: Consider Options

For each viable option:

| Aspect | Evaluate |
|--------|----------|
| Alignment | Does it fit VISION and existing ARCHITECTURE? |
| Complexity | How much does it add to the system? |
| Reversibility | How hard to change later? |
| Team capability | Can the team execute this? |
| Maintenance | Long-term cost? |

### Step 4: Convene Council (If Needed)

For complex or contentious decisions, convene a council:

- **Odd number:** 5 or 7 agents
- **Diverse expertise:** Mix of perspectives
- **Structured deliberation:** Each argues a position

See `orchestration/councils` skill for council workflow.

**Council advises, user decides.**

### Step 5: Present Options

Format:

```markdown
## Decision: [Question]

### Context
[Why this decision is needed now]

### Options

#### Option A: [Name]
- **Pros:** ...
- **Cons:** ...
- **Fits when:** ...

#### Option B: [Name]
- **Pros:** ...
- **Cons:** ...
- **Fits when:** ...

### Recommendation
[Which option and why]

### What do you think?
```

### Step 6: Await User Decision

**Do NOT proceed without explicit user choice.**

User may:
- Accept a recommendation
- Choose a different option
- Ask for more information
- Request a council deliberation

### Step 7: Document the Decision

After user decides:

1. **Update ARCHITECTURE.md** with new decision
2. **Create ADR** in `docs/decisions/`

---

## ADR Format

**Filename:** `ADR-{number}-{slug}.md`

```yaml
---
id: ADR-001
title: "PostgreSQL as Primary Database"
status: Accepted  # Proposed | Accepted | Deprecated | Superseded
date: 2026-01-23
supersedes: null  # ADR-000 if replacing
superseded_by: null  # ADR-002 if replaced
---

# ADR-001: PostgreSQL as Primary Database

## Context

[Why this decision was needed. What problem we faced.]

## Decision

[What we decided. Be specific and unambiguous.]

## Consequences

### Positive
- [Benefit 1]
- [Benefit 2]

### Negative
- [Tradeoff 1]
- [Tradeoff 2]

### Neutral
- [Implication that's neither good nor bad]

## Alternatives Considered

### [Alternative 1]
[Why it was rejected]

### [Alternative 2]
[Why it was rejected]
```

---

## ADR Numbering

Find next available number:

```bash
ls docs/decisions/ADR-*.md 2>/dev/null | \
  grep -oE 'ADR-[0-9]+' | \
  sort -t- -k2 -n | \
  tail -1 | \
  awk -F- '{print $2 + 1}'
```

If no ADRs exist, start with `ADR-001`.

---

## ARCHITECTURE.md Updates

When updating ARCHITECTURE.md:

1. **Find relevant section** or create new one
2. **Add decision** with reference to ADR
3. **Keep it current** - Remove outdated information

Example addition:

```markdown
## Data Storage

We use **PostgreSQL** as our primary database (see [ADR-001](decisions/ADR-001-postgresql.md)).

Key decisions:
- Single database for simplicity
- Connection pooling via PgBouncer
- Read replicas for reporting queries
```

---

## Decision Types

| Type | Scope | ADR Required? |
|------|-------|---------------|
| Strategic | System-wide, hard to reverse | Yes |
| Tactical | Component-level, reversible | Optional |
| Trivial | No significant impact | No |

**When in doubt, create an ADR.** Future you will thank you.

---

## Guardrails

1. **Interview first** - Understand the full context
2. **Check existing decisions** - Don't contradict without superseding
3. **Present options** - User decides, not you
4. **Document thoroughly** - ADRs explain the "why"
5. **Keep ARCHITECTURE.md current** - Update, don't just append

---

## Council Trigger Conditions

Consider convening a council when:

- Decision affects multiple domains (backend + frontend + infra)
- Team has conflicting opinions
- High cost of reversal
- Novel problem without established patterns
- User explicitly requests deliberation

---

## Related Skills

- **orchestration/councils** - Council deliberation workflow
- **orchestration/product-development** - Where architecture fits in hierarchy
- **foundations** - Documentation standards for ADRs
---
version: 1.13.1
