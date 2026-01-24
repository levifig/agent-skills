---
description: Shape ideas into well-defined specs through rigorous evaluation
version: 1.13.1
---

# Shape Command

Develop ideas into bounded, buildable specifications.

**Input:** $ARGUMENTS

---

## Purpose

Shaping transforms raw ideas into **well-defined SPECs** with:

- Clear problem statement
- Solution direction (not blueprint)
- Hard boundaries (what's explicitly out)
- Identified rabbit holes and risks
- Enough direction, not too much constraint

Shaping evaluates ideas against strategic context (VISION, STRATEGY, ARCHITECTURE) to ensure alignment and surface tensions.

---

## CRITICAL: Interview Extensively

Unlike `/idea` (quick capture), shaping requires **deep understanding**.

Ask questions the user hasn't thought about:
- Edge cases and failure modes
- Conflicts with existing patterns
- Hidden complexity
- Scope boundaries

Use `AskUserQuestion` throughout.

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` should reference:
- An idea file: `20260124-keyboard-shortcuts` or `.agents/ideas/20260124-keyboard-shortcuts.md`
- A problem description: "user onboarding flow"
- A requirement area: "authentication"

**If unclear:** Ask what to shape.

### Step 2: Gather Strategic Context

Read in order:

1. **VISION.md** — Does this align with where we're going?
2. **STRATEGY.md** — Does this serve our personas? Fit our positioning?
3. **ARCHITECTURE.md** — What technical constraints apply?
4. **Existing specs** — Avoid duplication, identify dependencies

If shaping from an idea file, read it first.

### Step 3: Evaluate Strategic Fit

Before diving into details, assess alignment:

| Question | Source |
|----------|--------|
| Does this advance our vision? | VISION.md |
| Does this serve our target personas? | STRATEGY.md |
| Does this fit our technical constraints? | ARCHITECTURE.md |
| Does this conflict with existing work? | docs/specs/ |

**If misaligned:** Surface the tension. Options:
- Adjust the idea to fit strategy
- Note that strategy may need evolution (for `/reflect` later)
- Decide not to proceed

### Step 4: Interview for Shaping

Shape Up methodology — define boundaries, not tasks.

| Question | Purpose |
|----------|---------|
| What's the core problem we're solving? | Problem statement |
| What's definitely in scope? | Core functionality |
| What's definitely out of scope? | Prevent creep |
| What seems related but should be avoided? | Rabbit holes |
| What approaches are forbidden? | No-gos |
| How do we know it works? | Test conditions |
| How much time is it worth? | Appetite |
| What if we run out of time? | Circuit breaker |
| What could go wrong? | Risks |
| What don't we know yet? | Open questions |

### Step 5: Determine Appetite

| Appetite | Size | Suitable For |
|----------|------|--------------|
| Small | 1-2 days | Bug fixes, minor enhancements |
| Medium | 3-5 days | Feature additions, refactors |
| Large | 1-2 weeks | Major features (rare, consider splitting) |

**If work can't fit the appetite:** Needs more shaping or splitting.

### Step 6: Draft the Spec

Use this template:

```yaml
---
id: SPEC-XXX
title: "[Clear, descriptive title]"
source: "[idea file or 'direct']"
created: YYYY-MM-DDTHH:MM:SSZ
status: drafting
appetite: "[time budget]"
---

# SPEC-XXX: [Title]

## Problem Statement

[What problem are we solving? Why does it matter? Who does it affect?]

## Strategic Alignment

- **Vision:** [How this advances our north star]
- **Personas:** [Which personas benefit, how]
- **Architecture:** [Relevant constraints or patterns]

## Solution Direction

[High-level approach — direction, not blueprint. Enough for an implementer to make good decisions, not so much that it's prescriptive.]

## Scope

### In Scope
- [Core functionality 1]
- [Core functionality 2]

### Out of Scope
- [Explicitly excluded 1]
- [Explicitly excluded 2]

### Rabbit Holes
- [Tempting complexity to avoid 1]
- [Tempting complexity to avoid 2]

### No-Gos
- [Forbidden approach 1]
- [Forbidden approach 2]

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| [Risk 1] | Low/Med/High | Low/Med/High | [How to handle] |

## Open Questions

- [ ] [Unresolved question 1]
- [ ] [Unresolved question 2]

## Test Conditions

- [ ] [Observable outcome 1]
- [ ] [Observable outcome 2]
- [ ] [Observable outcome 3]

## Circuit Breaker

At 50% appetite: [What to do if not on track]

At 75% appetite: [Hard decision point]
```

### Step 7: Generate Spec ID

Find next available ID:

```bash
ls docs/specs/SPEC-*.md docs/specs/archive/SPEC-*.md 2>/dev/null | \
  grep -oE 'SPEC-[0-9]+' | \
  sort -t- -k2 -n | \
  tail -1 | \
  awk -F- '{print $2 + 1}'
```

If no specs exist, start with `SPEC-001`.

### Step 8: Present for Approval

```markdown
## Proposed Specification

[Full spec content]

---

**Before approval, confirm:**
1. Is the problem statement accurate?
2. Is the scope correct?
3. Are the boundaries clear?
4. Is the appetite realistic?
5. Any missing risks or rabbit holes?
```

### Step 9: Await Approval

**Do NOT create spec file without explicit approval.**

User may:
- Approve as-is
- Adjust scope
- Change appetite
- Add constraints
- Request splitting
- Decide not to proceed

### Step 10: Create Spec File

After approval:

1. **Generate timestamp:**
   ```bash
   date -u +"%Y-%m-%dT%H:%M:%SZ"
   ```

2. **Create file:**
   - Location: `docs/specs/SPEC-{id}-{slug}.md`
   - Slug: kebab-case of title

3. **Update idea file** (if shaping from idea):
   - Set status to `shaped`
   - Add reference to spec

4. **Announce:**
   ```
   Created: docs/specs/SPEC-001-keyboard-shortcuts.md

   Next steps:
   - Use `/breakdown SPEC-001` to create atomic work items
   - Or `/implement SPEC-001` to start a session directly
   ```

---

## Splitting Large Specs

When a spec is too big for its appetite:

```
SPEC-001-user-auth.md (large, 2 weeks)
        ↓ split into
SPEC-001a-oauth-integration.md (medium, 3 days)
SPEC-001b-session-management.md (small, 2 days)
SPEC-001c-login-ui.md (medium, 3 days)
```

Each sub-spec:
- Has its own appetite
- Can be worked independently
- References parent spec

---

## Spec Lifecycle

```
drafting → approved → implementing → complete → archived
```

| Status | Meaning |
|--------|---------|
| `drafting` | Being shaped, not ready |
| `approved` | Ready for breakdown/implementation |
| `implementing` | Work in progress |
| `complete` | All work done |
| `archived` | Moved to docs/specs/archive/ |

---

## Strategic Tensions

When shaping reveals misalignment with strategic docs:

**Don't try to fix strategy during shaping.** Instead:

1. Note the tension in the spec's "Strategic Alignment" section
2. Document what might need to change
3. Proceed with shaping (or pause if tension is blocking)
4. Use `/reflect` after implementation to evolve strategy

This keeps shaping focused and strategy changes deliberate.

---

## Guardrails

1. **Interview extensively** — This is not quick capture
2. **Evaluate strategic fit** — Don't shape in a vacuum
3. **Respect appetite** — Fixed time, variable scope
4. **Document rabbit holes** — Prevent scope creep
5. **Clear test conditions** — Observable outcomes
6. **Circuit breaker** — Plan for running out of time
7. **Get approval** — Don't create without confirmation
8. **Note tensions, don't fix** — Strategy evolves via /reflect

---

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Implementation details | Keep high-level, direction not blueprint |
| No appetite | Always set time budget |
| Missing no-gos | Explicitly forbid bad approaches |
| Vague test conditions | Make observable and verifiable |
| Too ambitious | Split or increase appetite |
| Trying to fix strategy | Note tension, use /reflect later |
| Skipping strategic context | Always read VISION/STRATEGY/ARCHITECTURE |

---

## Related Commands

- `/idea` — Quick capture (feeds into /shape)
- `/brainstorm` — Deep thinking before shaping
- `/breakdown` — Decompose spec into work items
- `/implement` — Start implementation session
- `/reflect` — Update strategy after shipping
