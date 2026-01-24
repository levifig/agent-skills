# Reflect Command

Update VISION, STRATEGY, and ARCHITECTURE based on proven implementation.

**Input:** $ARGUMENTS

---

## Purpose

Strategy evolves through **shipping**, not theorizing.

After completing work, `/reflect` extracts learnings and proposes updates to strategic documents. This keeps strategy grounded in reality.

**Key principle:** Don't update strategy during planning or shaping. Update it after implementation proves (or disproves) your assumptions.

---

## When to Reflect

- After completing a spec
- After shipping a significant feature
- After discovering something unexpected during implementation
- After a series of related sessions
- Periodically (monthly/quarterly) to consolidate learnings

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` can be:
- A spec ID: `SPEC-001` or `SPEC-002`
- A topic: "authentication learnings"
- Empty: General reflection on recent work

**If empty:** Review recent sessions and completed specs.

### Step 2: Gather Evidence

What did we learn? Sources:

1. **Completed specs:**
   - `docs/specs/SPEC-*.md` with status `complete`
   - Check for "Lessons Learned" sections

2. **Session files:**
   - `.agents/sessions/*.md`
   - Look for insights, surprises, pivots

3. **Recent commits:**
   ```bash
   git log --oneline -30
   ```

4. **Implementation reality:**
   - What was harder than expected?
   - What was easier than expected?
   - What assumptions were wrong?

### Step 3: Interview for Insights

Ask:
- What surprised you during implementation?
- What would you do differently next time?
- Did any assumptions prove wrong?
- What did we learn about our users?
- What did we learn about our technical constraints?
- Any strategic implications?

### Step 4: Identify Implications

Map learnings to strategic documents:

| Learning Type | Document | Section |
|---------------|----------|---------|
| User behavior insights | STRATEGY.md | Personas |
| Market/competitive insights | STRATEGY.md | Market Landscape |
| Problem understanding | STRATEGY.md | Problem Space |
| Direction changes | VISION.md | Direction |
| Technical constraints | ARCHITECTURE.md | Constraints |
| Pattern discoveries | ARCHITECTURE.md | Patterns |
| Decision updates | ADR (new or supersede) | - |

### Step 5: Draft Proposals

For each document that needs updating:

```markdown
## Proposed Update: [DOCUMENT].md

### Section: [Section Name]

**Current:**
> [Quote current text]

**Proposed:**
> [New text]

**Evidence:**
- [What we learned that justifies this]
- [Reference to spec/session]

**Impact:**
- [How this affects future work]
```

### Step 6: Present Proposals

```markdown
## Reflection: [Topic/Spec]

### Summary
[Brief summary of what we learned]

### Proposed Updates

#### VISION.md
[Proposals or "No changes needed"]

#### STRATEGY.md
[Proposals or "No changes needed"]

#### ARCHITECTURE.md
[Proposals or "No changes needed"]

#### New ADRs
[Proposals or "None needed"]

---

**Approve these updates?**
```

### Step 7: Await Approval

**Do NOT update strategic documents without explicit approval.**

User may:
- Approve all
- Approve some, reject others
- Modify proposals
- Defer updates
- Request more evidence

### Step 8: Apply Updates

After approval:

1. **Update documents** with approved changes
2. **Create ADRs** if needed
3. **Archive completed specs** if appropriate
4. **Announce:**
   ```
   Updated:
   - STRATEGY.md (Personas section)
   - ARCHITECTURE.md (Authentication patterns)
   - Created ADR-005-session-management.md

   Archived:
   - SPEC-001-user-auth.md → docs/specs/archive/
   ```

---

## Reflection Patterns

### Post-Spec Reflection

After completing a spec:

1. Read the completed spec
2. Read associated session files
3. Compare original assumptions to reality
4. Extract learnings
5. Propose updates

### Periodic Reflection

Monthly or quarterly:

1. List specs completed since last reflection
2. Read session files from period
3. Look for patterns across multiple specs
4. Consolidate into strategic updates

### Incident Reflection

After something went wrong:

1. Document what happened
2. Identify root cause
3. Determine strategic implications
4. Propose preventive updates

---

## What to Look For

### In Session Files

- "This was harder than expected because..."
- "We discovered that..."
- "The original assumption was wrong..."
- "Users actually need..."
- "The right pattern for this is..."

### In Completed Specs

- Scope changes during implementation
- Test conditions that changed
- Rabbit holes that were hit anyway
- Approaches that worked/didn't work

### In Code and Commits

- Patterns that emerged
- Refactors that happened
- Technical debt created/paid

---

## VISION.md Updates

Vision updates are rare and significant. Triggers:

- Market shift that affects direction
- User needs fundamentally different than assumed
- Strategic pivot based on learnings

**Template:**

```markdown
## VISION.md Update Proposal

**Section:** [Section name]

**Trigger:** [What prompted this]

**Current:**
> [Current vision statement]

**Proposed:**
> [New vision statement]

**Evidence:**
- [Spec/session that proved this]
- [User feedback/market signal]

**Implications:**
- [What this changes about our direction]
```

---

## STRATEGY.md Updates

More frequent than vision. Triggers:

- Persona understanding evolved
- Market landscape changed
- Problem space better understood
- Competitive positioning shifted

**Template:**

```markdown
## STRATEGY.md Update Proposal

**Section:** [Personas/Market/Problem Space]

**Evidence:** [What we learned from SPEC-XXX]

**Current:**
> [Current text]

**Proposed:**
> [New text]

**Impact on future shaping:**
- [How this changes how we evaluate ideas]
```

---

## ARCHITECTURE.md Updates

Technical learnings. Triggers:

- Pattern discovered/validated
- Constraint identified
- Approach proven/disproven
- Technical debt acknowledged

**Template:**

```markdown
## ARCHITECTURE.md Update Proposal

**Section:** [Patterns/Constraints/Decisions]

**Evidence:** [Technical discovery from implementation]

**Current:**
> [Current text]

**Proposed:**
> [New text]

**Why this matters:**
- [How this affects future implementation]
```

---

## ADR Creation

When reflection surfaces a decision worth documenting:

1. Check if ADR already exists
2. If updating existing decision → supersede old ADR
3. If new decision → create new ADR

See `/architecture` for ADR format.

---

## Guardrails

1. **Evidence-based** — Proposals need supporting evidence
2. **Post-implementation** — Reflect after shipping, not before
3. **Get approval** — Don't update strategic docs without confirmation
4. **Consolidate** — Don't create micro-updates; batch related learnings
5. **Link back** — Reference the specs/sessions that informed the update
6. **Archive completed work** — Move done specs to archive

---

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Update strategy during shaping | Note tensions, reflect after shipping |
| Make speculative updates | Base updates on evidence |
| Create one ADR per commit | Consolidate related decisions |
| Skip reflection | Make it a habit post-spec |
| Update without approval | Always get explicit sign-off |

---

## Related Commands

- `/shape` — Notes strategic tensions for later reflection
- `/strategy` — Deep discovery (before reflection validates)
- `/architecture` — Technical decisions (creates ADRs)
- `/research` — Investigation that may inform reflection
---
version: 1.12.0
