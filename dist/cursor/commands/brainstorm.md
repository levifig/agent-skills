# Brainstorm Command

Think deeply about an existing idea or explore a new problem space.

**Input:** $ARGUMENTS

---

## Purpose

Brainstorming is **generative thinking** — expanding possibilities before narrowing.

Unlike `/idea` (quick capture) or `/shape` (rigorous bounding), brainstorming is exploratory:
- Process an existing idea to deepen understanding
- Explore a problem space without a specific solution
- Generate options before committing to an approach

---

## Mode Detection

Parse input to determine mode:

| Input Pattern | Mode | Action |
|---------------|------|--------|
| Idea file reference | Idea Processing | Deep dive on captured idea |
| Problem/question | Problem Exploration | Explore problem space |
| Empty | Open Brainstorm | "What should we be thinking about?" |

Examples:
- `20260124-keyboard-shortcuts` → Process this idea
- `how should we handle offline mode?` → Explore this problem
- (empty) → Open exploration

---

## Process

### Mode: Idea Processing

When given an existing idea file.

#### Step 1: Read the Idea

Load from `.agents/ideas/{filename}.md`

#### Step 2: Gather Context

- Read VISION.md — Where does this fit?
- Read STRATEGY.md — Who benefits?
- Read ARCHITECTURE.md — What constraints apply?
- Check related ideas — Any connections?

#### Step 3: Deep Exploration

Ask expansive questions:

| Category | Questions |
|----------|-----------|
| **User value** | Who benefits most? What's the ideal outcome for them? |
| **Problem depth** | Why does this matter? What's the cost of not doing it? |
| **Alternatives** | What else could solve this? What if we did the opposite? |
| **Risks** | What could go wrong? What are we assuming? |
| **Dependencies** | What needs to exist first? What does this enable? |
| **Scope** | What's the minimal version? What's the maximal version? |

#### Step 4: Generate Options

Explore multiple approaches:

1. **Conventional approach** — How would most solve this?
2. **Minimal approach** — Smallest thing that provides value
3. **Ambitious approach** — If resources were unlimited
4. **Contrarian approach** — What if the opposite were true?

#### Step 5: Refine and Document

```markdown
## Brainstorm: [Idea Title]

### Core Insight
[One sentence capturing the essential value]

### Explored Directions

#### Direction A: [Name]
- **Approach:** ...
- **Pros:** ...
- **Cons:** ...
- **Would work if:** ...

#### Direction B: [Name]
- **Approach:** ...
- **Pros:** ...
- **Cons:** ...
- **Would work if:** ...

### Open Questions
- [Question surfaced during exploration]
- [Question surfaced during exploration]

### Recommendation
[Which direction seems most promising and why]

### Next Steps
- [ ] Ready to shape → `/shape {idea}`
- [ ] Needs more research → `/research {topic}`
- [ ] Park for later → Keep in ideas/
- [ ] Discard → Archive idea
```

#### Step 6: Update Idea File

If user wants to proceed:
- Update idea status to `shaping` if moving to `/shape`
- Add brainstorm notes to idea file
- Or archive if decided not to pursue

---

### Mode: Problem Exploration

When given a problem or question without a specific idea.

#### Step 1: Understand the Problem

Interview to clarify:
- What problem are we trying to solve?
- Who experiences this problem?
- What's the impact of not solving it?
- What have we tried before?
- What constraints exist?

#### Step 2: Gather Context

- Read STRATEGY.md — Is this in our problem space?
- Read VISION.md — Does solving this advance our direction?
- Check existing ideas — Related thinking?

#### Step 3: Divergent Thinking

Generate possibilities without judgment:

| Technique | Prompt |
|-----------|--------|
| **First principles** | If we started from scratch, what would we build? |
| **Inversion** | What would make this problem worse? (Avoid those) |
| **Analogy** | How do others solve similar problems? |
| **Extreme constraints** | What if we had 1 day? 1 year? $100? $1M? |
| **Persona lens** | How would [persona] want this solved? |

#### Step 4: Convergent Thinking

Filter and prioritize:
- Which options align with our strategy?
- Which are feasible given constraints?
- Which provide the most value?

#### Step 5: Document Exploration

```markdown
## Brainstorm: [Problem]

### Problem Statement
[Clear articulation of the problem]

### Who's Affected
[Personas or user types]

### Options Explored

#### Option 1: [Name]
...

#### Option 2: [Name]
...

### Analysis
[Comparison, trade-offs]

### Recommendation
[Which direction to pursue]

### Next Steps
- [ ] Capture as idea → `/idea {summary}`
- [ ] Shape directly → `/shape {topic}`
- [ ] Research first → `/research {topic}`
- [ ] Park → Note in session, revisit later
```

---

### Mode: Open Brainstorm

When input is empty — "What should we be thinking about?"

#### Step 1: Assess Current State

- List ideas in `.agents/ideas/`
- Check recent sessions for open threads
- Review VISION.md for gaps

#### Step 2: Surface Opportunities

Ask:
- What's blocking progress?
- What opportunities are we not pursuing?
- What assumptions haven't we tested?
- What would 10x our impact?

#### Step 3: Prioritize Exploration

Present options:
```markdown
## Open Brainstorm

### Existing Ideas Worth Exploring
1. [idea-1] — Last touched [date]
2. [idea-2] — Related to recent work

### Emerging Opportunities
1. [Opportunity based on recent sessions]
2. [Gap identified in strategy]

### What should we explore?
```

---

## Brainstorming Techniques

### For Expanding Options

| Technique | Description |
|-----------|-------------|
| **Yes, and...** | Build on each idea without criticism |
| **What if...** | Remove constraints temporarily |
| **Worst idea** | Generate intentionally bad ideas, then invert |
| **Random input** | Introduce unrelated concept, find connections |

### For Filtering Options

| Technique | Description |
|-----------|-------------|
| **Dot voting** | Which resonate most? |
| **2x2 matrix** | Plot on impact vs effort |
| **Must/Should/Could** | Categorize by importance |
| **Kill criteria** | What would eliminate an option? |

---

## Output Artifacts

Brainstorming can produce:

| Artifact | When |
|----------|------|
| Updated idea file | Processing an existing idea |
| New idea file | Problem exploration surfaces a nugget |
| Research question | Need more information |
| Direct to shaping | Direction is clear enough |
| Session notes | Exploratory, nothing concrete yet |

---

## Guardrails

1. **Diverge before converging** — Generate options before judging
2. **Stay exploratory** — Don't prematurely commit
3. **Document the thinking** — Even discarded options are valuable
4. **Connect to strategy** — Ground exploration in context
5. **Know when to stop** — Brainstorming can be endless; set boundaries

---

## When to Use This vs Other Commands

| Situation | Command |
|-----------|---------|
| Have a nugget, want to capture quickly | `/idea` |
| Have an idea, want to think deeply | `/brainstorm` |
| Have a problem, want to explore | `/brainstorm` |
| Ready to bound and define | `/shape` |
| Need facts or understanding | `/research` |

---

## Related Commands

- `/idea` — Quick capture (may precede brainstorming)
- `/shape` — Rigorous bounding (often follows brainstorming)
- `/research` — Fact-finding (complements brainstorming)
- `/strategy` — Strategic context (grounds brainstorming)
---
version: 1.13.1
