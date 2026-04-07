---
description: >-
  Shapes ideas into implementable specs with scope boundaries and test
  conditions. Use when the user asks "shape this" or "write a spec," or when an
  idea has accumulated enough constraints to bound. Produces specs with
  acceptance criteria. Not for brainstorming (use brainstorm) or task breakdown
  (use breakdown).
subtask: false
version: 2.0.0-dev.16
---

# Shape

Develop ideas into bounded, buildable specifications.

## Contents
- Critical Rules
- Verification
- Purpose
- Process
- Spec Lifecycle
- Strategic Tensions
- Guardrails
- Related Skills

**Input:** $ARGUMENTS

---

## Critical Rules

Unlike `/idea` (quick capture), shaping requires **deep understanding**. Ask about edge cases, conflicts with existing patterns, hidden complexity, and scope boundaries. Use `AskUserQuestion` throughout.

1. **Interview extensively** -- not quick capture
2. **Evaluate strategic fit** -- don't shape in a vacuum
3. **Document rabbit holes** -- prevent scope creep
4. **Clear test conditions** -- observable outcomes
5. **Priority order** -- for multi-part specs, define track order and go/no-go gates
6. **Get approval** -- don't create spec file without explicit confirmation
7. **Note tensions, don't fix** -- strategy evolves via /reflect
8. **Log outcome** -- log spec creation to session journal: `loaf session log "decision(shape): SPEC-NNN created for feature"`

---

## Verification

- Spec has a clear problem statement, solution direction, hard boundaries, and identified rabbit holes
- All test conditions are observable and measurable outcomes
- User has explicitly approved the spec before the file is created

---

## Purpose

Shaping transforms raw ideas into **well-defined SPECs** with clear problem statement, solution direction (not blueprint), hard boundaries, identified rabbit holes, and enough direction without too much constraint.

Evaluates ideas against strategic context (VISION, STRATEGY, ARCHITECTURE) to ensure alignment.

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` should reference an idea file, problem description, or requirement area. If unclear, ask what to shape.

### Step 2: Gather Strategic Context

Read in order: VISION.md, STRATEGY.md, ARCHITECTURE.md, existing specs.

### Step 3: Evaluate Strategic Fit

| Question | Source |
|----------|--------|
| Does this advance our vision? | VISION.md |
| Does this serve our target personas? | STRATEGY.md |
| Does this fit our technical constraints? | ARCHITECTURE.md |
| Does this conflict with existing work? | .agents/specs/ |

If misaligned: surface the tension, adjust idea, or note for `/reflect` later.

### Step 4: Interview for Shaping

Define boundaries, not tasks:
- Core problem, in/out scope, rabbit holes, no-gos
- Test conditions, risks, open questions
- For multi-part specs: priority ordering and go/no-go gates between tracks

### Step 5: Draft the Spec

Create spec following [spec template](../skills/shape/templates/spec.md).

### Step 6: Generate Spec ID

```bash
ls .agents/specs/SPEC-*.md .agents/specs/archive/SPEC-*.md 2>/dev/null | \
  grep -oE 'SPEC-[0-9]+' | sort -t- -k2 -n | tail -1 | awk -F- '{print $2 + 1}'
```

If none exist, start with `SPEC-001`.

### Step 7: Present for Approval

Present full spec. **Do NOT create spec file without explicit approval.** User may adjust scope, add constraints, request splitting, or decide not to proceed.

### Step 8: Create Spec File

After approval: create `.agents/specs/SPEC-{id}-{slug}.md`, update idea file status if applicable.

### Step 9: Flag for Post-Implementation Reflection

If shaping surfaced strategic tensions (noted in the spec's "Strategic Alignment" section per the Strategic Tensions guidelines below), remind the user: *"This spec has strategic tensions. After implementation ships, run `/reflect` to update strategic docs."* Don't suggest running `/reflect` now — strategy updates come from shipped work, not planning.

---

## Spec Lifecycle

```
drafting -> approved -> implementing -> complete -> archived
```

### Splitting Large Specs

When scope exceeds a single track, split into sub-specs or use priority ordering within the spec. Each sub-spec can be worked independently and references the parent.

---

## Strategic Tensions

**Don't fix strategy during shaping.** Instead:
1. Note the tension in the spec's "Strategic Alignment" section
2. Document what might need to change
3. Proceed with shaping (or pause if blocking)
4. Use `/reflect` after implementation to evolve strategy

---

## Guardrails

1. **Interview extensively** -- not quick capture
2. **Evaluate strategic fit** -- don't shape in a vacuum
3. **Document rabbit holes** -- prevent scope creep
4. **Clear test conditions** -- observable outcomes
5. **Priority order** -- for multi-part specs, define track order and go/no-go gates
6. **Get approval** -- don't create without confirmation
7. **Note tensions, don't fix** -- strategy evolves via /reflect

---

## Suggests Next

After a spec is approved, suggest `/breakdown` to decompose it into tasks.

## Related Skills

- **idea** -- Quick capture (feeds into /shape)
- **brainstorm** -- Deep thinking before shaping
- **breakdown** -- Decompose spec into work items
- **implement** -- Start implementation session
- **reflect** -- Update strategy after shipping
