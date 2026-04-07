# Product Discovery Interview Guide

A structured interview framework for helping builders go from a vague idea to a clear vision, strategy, and architecture. Synthesized from Shape Up, Jobs to Be Done, Lean Canvas, The Mom Test, Design Sprint, Wardley Mapping, and First Principles Thinking.

## Contents
- How This Guide Works
- Framework Foundations
- Phase 1: Excavation (The Spark)
- Phase 2: Sharpening (The Shape)
- Phase 3: Grounding (The Architecture)
- Phase 4: Synthesis (The Documents)
- Adapting Interview Depth
- Anti-Patterns
- Transitioning to Document Drafting

## How This Guide Works

This is a **builder interview**, not a user research interview. The interviewer (the agent) is helping the builder crystallize their own thinking -- not extracting requirements from a stakeholder. The builder has context, intuition, and taste that need to be surfaced, challenged, and structured.

The interview flows through four phases, each producing progressively sharper artifacts:

| Phase | Focus | Primary Output | Frameworks |
|-------|-------|----------------|------------|
| 1. Excavation | What exists, why it matters | Raw problem + context | Mom Test, JTBD, First Principles |
| 2. Sharpening | Who, what, boundaries | Bounded scope + complexity | Shape Up, Lean Canvas, Design Sprint |
| 3. Grounding | How, where, what evolves | Technical direction + landscape | Wardley Mapping, First Principles |
| 4. Synthesis | Drafting documents | BRIEF, VISION, STRATEGY, ARCHITECTURE | All, integrated |

Phases are not rigid walls. A strong answer in Phase 1 might skip half of Phase 2. A weak answer in Phase 2 might loop back to Phase 1. Follow the energy.

---

## Framework Foundations

What each framework contributes and what to skip.

### Shape Up (Basecamp / Ryan Singer)

**What to use:**
- **Complexity sizing** -- Size work by complexity (small / medium / large), not time. Agents don't have time budgets; complexity determines whether to split or reshape.
- **Fat marker sketches** -- Solutions at the right altitude: rough enough to leave room, specific enough to be buildable. Not wireframes, not words -- the space in between.
- **Rabbit holes and no-gos** -- Explicitly naming what NOT to build and where complexity lurks. Forces the builder to confront scope honestly.
- **Pitch format** -- Problem, Complexity, Solution, Rabbit Holes, No-Gos. A proven structure for packaging shaped work.

**What to skip:** Betting tables, cool-down cycles, hill charts. These are team process mechanics, not 0-to-1 discovery tools.

**Connection to documents:** Complexity sizing and boundaries flow directly into the spec format that `/loaf:shape` produces. The pitch format IS the spec.

### Jobs to Be Done (Christensen, Ulwick, Klement)

**What to use:**
- **The job statement** -- "When [situation], I want to [motivation], so I can [desired outcome]." Forces the builder to articulate the user's world, not their own feature list.
- **Competing against non-consumption** -- The real competition is often "doing nothing" or "cobbling together a workaround." Ask what people do TODAY without your product.
- **Functional, emotional, social dimensions** -- A job has all three. The functional job is obvious; the emotional and social jobs reveal why people actually switch.
- **Switching forces** -- Push (current pain), pull (new attraction), anxiety (fear of new), inertia (habit of old). Map all four to understand adoption.

**What to skip:** Outcome-Driven Innovation scoring matrices, quantitative job importance surveys. These require existing users and market data -- neither exists at 0-to-1.

**Connection to documents:** Job statements and switching forces map directly to VISION.md (purpose, target users) and STRATEGY.md (personas, problem space).

### Lean Canvas (Ash Maurya)

**What to use:**
- **Problem / existing alternatives / customer segments** -- The top-left triangle of the canvas. Forces articulation of the problem, who has it, and what they do about it today.
- **Unique Value Proposition** -- One sentence: why is this different AND better? If the builder can't answer this, the idea isn't ready.
- **Unfair advantage** -- What do you have that can't be easily copied or bought? Honest answer might be "nothing yet" -- that's important to know.
- **Key metrics** -- How will you know this is working? Not vanity metrics. What number would make you confident?

**What to skip:** Revenue streams, cost structure, channels. These matter for businesses but are premature noise at the spark stage of a technical project. They can be explored later if the project is a product.

**Connection to documents:** Problem + UVP + unfair advantage feed VISION.md. Customer segments and existing alternatives feed STRATEGY.md.

### The Mom Test (Rob Fitzpatrick)

**What to use:**
- **Ask about their life, not your idea** -- "Tell me about the last time you dealt with [problem]" beats "Would you use a tool that does X?" The builder's idea isn't special until proven otherwise.
- **Ask about specifics in the past, not generics about the future** -- "What happened last time?" not "What would you do if...?" Past behavior predicts; hypotheticals lie.
- **Look for evidence of pain** -- Have they spent money on this problem? Time? Built workarounds? If no one has done anything about this problem, it might not be a real problem.
- **The deflection test** -- If someone says "that's interesting" and then changes the subject, it's not interesting. Real interest produces follow-up questions, offers to test, or wallet-opening.

**What to skip:** Customer interview logistics, meeting scheduling advice. The Mom Test is about question quality, not interview operations.

**Connection to documents:** Informs the quality of every answer in every phase. The Mom Test isn't a phase -- it's a lens applied to ALL questioning. It keeps the interview honest.

### Design Sprint - Understand Phase (Google Ventures)

**What to use:**
- **Long-term goal** -- "Where do you want to be in [timeframe]?" Then work backward. What has to be true for that to happen?
- **Sprint questions** -- "What questions do we need to answer to know if this is worth building?" Surface the riskiest assumptions explicitly.
- **How Might We (HMW)** -- Reframe problems as opportunities: "How might we make [painful thing] effortless?" Opens creative space without committing to solutions.
- **Map the journey** -- Draw the user's path from discovering the product to getting value. Where are the drops? Where is the magic?

**What to skip:** Lightning demos, Crazy 8s sketching, the full 5-day process. These are team exercises for existing organizations, not solo builder discovery.

**Connection to documents:** Long-term goal IS the vision statement. Sprint questions become open questions in specs. The journey map informs ARCHITECTURE.md's system overview.

### Wardley Mapping (Simon Wardley)

**What to use:**
- **Value chain question** -- "What does the user need, and what do you need to provide it?" Trace dependencies from user need down to infrastructure. Reveals hidden complexity.
- **Evolution axis** -- For each component, ask: "Is this novel (genesis), custom-built, a product you'd buy, or a commodity/utility?" Build the novel parts; buy or use the rest.
- **Movement awareness** -- "What's changing in this space? What's becoming commoditized? What new capabilities are emerging?" Prevents building on shifting sand.
- **Build vs. buy clarity** -- Position each component on the evolution axis. If it's a commodity, don't build it. If it's genesis, that's your moat.

**What to skip:** Full Wardley Map creation, doctrine assessment, gameplay patterns. These require strategic maturity that a 0-to-1 project doesn't have yet.

**Connection to documents:** Value chain and evolution directly inform ARCHITECTURE.md (build vs. buy, technology choices). Movement awareness feeds STRATEGY.md (market landscape, positioning).

### First Principles Thinking (Aristotle, popularized by Musk)

**What to use:**
- **Assumption excavation** -- "What are we assuming is true? What if it isn't?" List every assumption, then challenge each one. The interesting ideas live where assumptions break.
- **Root cause questioning** -- "Why does this problem exist? Why does THAT exist? Why?" (Five whys, adapted.) Get to the structural cause, not the symptom.
- **Physical/logical limits** -- "What's the actual constraint here? Is it physics, regulation, economics, convention, or just habit?" Convention and habit are not real constraints.
- **Inversion** -- "What would make this definitely fail?" Then avoid those things. Often clearer than asking what success looks like.

**What to skip:** The Elon Musk mythologizing. First principles is an ancient technique, not a Silicon Valley invention. Use it as a questioning discipline, not a brand.

**Connection to documents:** Assumption excavation creates the "rabbit holes" and "risks" sections of specs. Root cause questioning deepens the problem statement in VISION.md. Inversion produces no-gos.

---

## Phase 1: Excavation (The Spark)

**Goal:** Understand what exists, why it matters, and whether the problem is real.

**Mood:** Curious, non-judgmental, following threads. The builder may not have clear language for their idea yet. Help them find it.

**Duration:** Varies wildly. A builder with a crisp problem statement might need 3-4 questions. A builder with "I want to build something for developers" needs 10-15.

### Must-Ask Questions

**1. "Tell me about the problem. What's happening in the world that made you think about this?"**
*(Mom Test: ask about their life. JTBD: understand the situation.)*

Don't accept "I want to build X." Push to the problem behind X. "Why X? What breaks if X doesn't exist?"

**2. "Who has this problem? Be specific -- give me a person, not a segment."**
*(JTBD: customer segments. Lean Canvas: customer segments.)*

"Developers" is not specific enough. "A platform engineer at a mid-size company who manages 15+ microservices and spends 3 hours a week on config drift" is specific enough.

**3. "What do they do about it today?"**
*(Mom Test: evidence of pain. JTBD: competing against non-consumption. Lean Canvas: existing alternatives.)*

This is the most diagnostic question in the entire interview. If the answer is "nothing," probe further -- is the problem not painful enough, or is there truly no solution? If the answer is "they use a spreadsheet / a bash script / a competitor," that's gold.

**4. "What have YOU tried? What did you learn?"**
*(Mom Test: past behavior. First Principles: what assumptions formed.)*

The builder's own experience with the problem is the richest source of insight. Whether they've built a prototype, written a doc, or just been frustrated -- their experience shapes the solution.

### Expand-If-Needed Questions

- "Why does this problem exist? What's the root cause?" *(First Principles)*
- "What would happen if no one ever solved this?" *(Inversion)*
- "Have you seen anyone spend money or significant time on this problem?" *(Mom Test: evidence of pain)*
- "Is this problem getting worse or better over time? Why?" *(Wardley: movement awareness)*

### Phase 1 Signals

**Strong signal (move to Phase 2):** Builder can describe a specific person with a specific problem and knows what they do about it today.

**Weak signal (dig deeper):** Builder describes a category ("developers need better tools") without specifics. Ask for a story: "Tell me about a specific moment when this problem was most painful."

**Red flag (challenge directly):** Builder describes a solution without a problem. "I want to build a CLI that..." -- pause. "For whom? Why would they care?"

---

## Phase 2: Sharpening (The Shape)

**Goal:** Define who this is for, what it does, and -- critically -- what it does NOT do. Set complexity and boundaries.

**Mood:** Constructive pressure. Phase 1 was expansive; Phase 2 is reductive. The builder will want to include everything. Your job is to help them cut.

**Duration:** This is typically the longest phase. 8-12 questions for a fresh idea, 4-6 for an idea with an existing brief.

### Must-Ask Questions

**5. "If this works perfectly, what does the user's life look like? What job have you done for them?"**
*(JTBD: job statement. Design Sprint: long-term goal.)*

Construct a job statement together: "When [situation], I want to [motivation], so I can [outcome]." If the builder can't fill this in, the idea is still too vague.

**6. "What's your one-line pitch? Why is this different AND better than what exists?"**
*(Lean Canvas: unique value proposition.)*

Not a tagline -- a value proposition. "Unlike [existing alternative], this [key differentiator] so that [outcome]." If they can't say it in one sentence, they haven't found the core yet.

**7. "What is this NOT? What should we explicitly refuse to build?"**
*(Shape Up: no-gos. First Principles: inversion.)*

This question relieves enormous pressure. Builders carry anxiety about everything the product "should" eventually do. Naming the no-gos creates freedom to focus. Push for at least 3 concrete no-gos.

**8. "How complex is this? If you had to rate it small, medium, or large, where does it land?"**
*(Shape Up: complexity sizing.)*

This is complexity sizing. A small project (single concern, clear approach) produces a fundamentally different spec than a large one (cross-cutting, needs exploration). If the builder says large, it probably needs splitting.

**9. "What questions do we need to answer before we're confident this is worth building?"**
*(Design Sprint: sprint questions.)*

Surface the riskiest assumptions. "Will users actually switch from their current workflow?" "Can we get the data we need?" "Is the performance requirement even achievable?" These become the open questions and risks in specs.

### Expand-If-Needed Questions

- "Walk me through the user's journey from discovery to value. Where could they get stuck?" *(Design Sprint: map the journey)*
- "What's the smallest version that would be genuinely useful? Not an MVP -- the smallest GOOD version." *(Shape Up: fat marker sketch)*
- "What's your unfair advantage here? What do you have that others don't?" *(Lean Canvas: unfair advantage)*
- "How might we make [the hardest part] effortless?" *(Design Sprint: HMW)*
- "What would definitely make this fail?" *(First Principles: inversion)*

### Phase 2 Signals

**Strong signal (move to Phase 3):** Builder can state the job, the differentiator, and at least 3 no-gos. Complexity is sized.

**Weak signal (iterate):** Builder keeps expanding scope. "It should also..." is a signal to tighten. Ask: "If you could only do ONE thing, what would it be?"

**Red flag (loop back to Phase 1):** The "who" keeps shifting. If the user persona changes every other answer, the problem isn't defined yet. Go back to "Who has this problem?"

---

## Phase 3: Grounding (The Architecture)

**Goal:** Establish technical direction, identify what to build vs. buy, and surface hidden complexity.

**Mood:** Pragmatic. Not designing the system -- establishing constraints and key decisions. The builder may have strong opinions or none; adapt.

**Duration:** 4-8 questions. Skip or abbreviate if the builder has a clear brief with technical direction already stated. Expand if the builder is non-technical or the domain is unfamiliar.

### Must-Ask Questions

**10. "What does the user need, and what do you need to provide that? Trace the chain."**
*(Wardley: value chain.)*

Start from the user's need and trace dependencies downward. "The user needs to see their dashboard. That requires a frontend. The frontend needs an API. The API needs a database. The database needs..." This reveals hidden complexity fast.

**11. "For each piece: is this novel, something you'd customize, something you'd buy off the shelf, or a commodity?"**
*(Wardley: evolution axis.)*

The builder should only build the novel/custom parts. Everything else should be bought, rented, or used as a utility. If they want to build their own auth system, challenge that: "Is auth the thing that makes this product special?"

**12. "What's changing in this space? What technology is maturing, what's emerging, what's dying?"**
*(Wardley: movement. First Principles: assumptions about the landscape.)*

If the builder is betting on a technology that's being commoditized, or ignoring something that's emerging, surface it. "You're planning to build X, but Y just shipped last month and does 80% of it."

**13. "What are the hardest technical problems? Where will you spend the most time being confused?"**
*(Shape Up: rabbit holes. First Principles: assumption excavation.)*

These become the rabbit holes in the spec. The builder often knows where the dragons are -- they just haven't been asked directly.

### Expand-If-Needed Questions

- "What constraints are non-negotiable? Performance, privacy, cost, platform, licensing?" *(First Principles: real vs. perceived constraints)*
- "What's the deployment story? Where does this run?" *(Wardley: value chain, infrastructure layer)*
- "If you had to ship something in one week, what would you cut?" *(Shape Up: scope pressure test)*
- "Are there regulatory, legal, or compliance considerations?" *(Often forgotten at 0-to-1)*

### Phase 3 Signals

**Strong signal (move to Phase 4):** Builder can trace a rough value chain, knows what's novel vs. commodity, and has identified the hard problems.

**Weak signal (iterate):** Builder wants to build everything from scratch. Challenge: "What's the thing ONLY YOU can build? Build that. Use existing solutions for the rest."

**Not applicable:** If the project is purely conceptual or non-technical at this stage, skip Phase 3 and note that ARCHITECTURE.md will be populated later when technical decisions are made.

---

## Phase 4: Synthesis (The Documents)

**Goal:** Transform interview insights into draft documents. This is NOT a phase of the interview -- it's the transition from interviewing to drafting.

**Mood:** Collaborative, iterative. The agent drafts, the builder reacts and refines.

### The Transition Moment

The interview ends when you have enough to draft. Don't announce "the interview is over" -- just shift naturally: "I think I have a good picture. Let me draft the vision statement and we can iterate on it."

### Document Mapping

| Interview Insight | Document | Section |
|-------------------|----------|---------|
| Problem + why it matters | BRIEF.md | Problem statement |
| User persona + their current world | BRIEF.md, STRATEGY.md | Target users, personas |
| Job statement + switching forces | VISION.md | Purpose, what makes this unique |
| Long-term goal + differentiator | VISION.md | Where we're going |
| No-gos + boundaries | VISION.md | Non-goals |
| Existing alternatives + market | STRATEGY.md | Landscape, positioning |
| Unfair advantage | STRATEGY.md | Competitive advantage |
| Complexity sizing | STRATEGY.md | Current focus, priorities |
| Key metrics | STRATEGY.md | Success criteria |
| Value chain + build vs. buy | ARCHITECTURE.md | Components, technology choices |
| Novel vs. commodity | ARCHITECTURE.md | Build vs. buy decisions |
| Rabbit holes + hard problems | ARCHITECTURE.md | Risks, open questions |
| Constraints + deployment | ARCHITECTURE.md | Constraints, deployment |

### Draft Order

1. **BRIEF.md** -- Synthesize the interview into a canonical brief. Present for review first. This is the "what are we doing and why" document that everything else derives from.
2. **VISION.md** -- Purpose, target users, success criteria, non-goals. Draft from BRIEF + interview.
3. **STRATEGY.md** -- Only if enough signal exists. Personas, landscape, positioning. If the builder is still figuring this out, note it as an open area and suggest `/loaf:strategy` or `/loaf:research` later.
4. **ARCHITECTURE.md** -- Only if technical decisions were made. If the builder hasn't decided on a stack yet, don't force it. Capture constraints and known decisions only.

### Structured Review

Present each document section by section. Use specific prompts:
- "Is this problem statement accurate?"
- "Did I capture the right non-goals?"
- "Anything missing from the target user description?"
- "Are these the right technical constraints, or did I add assumptions?"

Allow the builder to approve all remaining sections at once if they're satisfied ("Looks good, approve all" is a valid response).

---

## Adapting Interview Depth

The interview adapts to three contexts, per SPEC-013's mode detection:

### Greenfield + Empty (Full Interview)

Run all four phases in full. The builder has minimal clarity and needs the most support. Expect 15-25 questions total across all phases. This is where the interview guide earns its keep.

**Key adaptation:** Be patient. The builder may circle back, contradict themselves, or get stuck. That's normal. Let them think out loud. Silence after a question is productive, not awkward.

### Greenfield + Brief (Gap-Filling Interview)

Read and analyze the brief first. Then run a compressed interview that:
- Confirms understanding ("Here's what I extracted -- is this right?")
- Challenges assumptions ("Your brief says X, but have you considered Y?")
- Fills gaps (which phases have missing information?)

Expect 8-12 questions total, concentrated in whichever phases the brief is weakest.

**Key adaptation:** Don't re-ask what the brief already answers well. Quote the brief back and ask "Is this still accurate?" to confirm, then move to gaps.

### Brownfield (Nuance-Capturing Interview)

The project exists. Code exists. Docs may exist. The interview focuses on:
- What's NOT in the code (intentions, frustrations, future direction)
- What the builder wants to CHANGE (current pain, technical debt, strategic shifts)
- What conventions and preferences exist but aren't documented

Expect 6-10 questions total, mostly in Phases 1-2. Phase 3 is largely answered by the existing codebase.

**Key adaptation:** Show the builder what you learned from their code. "I see a Python/FastAPI project with PostgreSQL and Docker. The test suite uses pytest. Is that the intended stack going forward?" Let the codebase speak first, then fill gaps.

---

## Anti-Patterns

### Do NOT Do This

**The Form.** Running through questions mechanically, one after another, like a survey. An interview is a conversation. If a builder's answer to question 3 partially answers question 7, skip 7 or just confirm.

**The 45-Minute Interrogation.** If the builder is losing energy, you've gone too long. Cut to synthesis. A draft document with gaps is better than an exhaustive interview that drains enthusiasm.

**The Therapist.** "And how does that make you feel about your product?" No. This is a builder interview, not a feelings exploration. Emotions matter (JTBD switching forces), but ask about user emotions, not builder emotions.

**Premature Architecture.** Jumping to "What database should we use?" in Phase 1. Technical decisions come after the problem and scope are clear. Architecture without clarity is random technology selection.

**The Echo Chamber.** Reflecting back everything the builder says without challenging anything. The Mom Test exists because people are too polite. Be constructive, not agreeable. "That's a big scope. Are you sure you need ALL of that for v1?"

**Solution-First Questioning.** "What features should it have?" is the wrong question. "What job does it do for the user?" is the right one. Features are an output of the interview, not an input.

**Asking for Permission to Proceed.** Don't ask "Should we move to the next phase?" Just move when the signals are strong. If the builder has more to say, they'll say it.

**Over-Indexing on Frameworks.** Don't say "Let's do a JTBD analysis" or "Let me apply Wardley Mapping here." The frameworks are lenses for the interviewer, not vocabulary for the builder. Just ask good questions.

---

## Transitioning to Document Drafting

The transition from interview to drafting should be seamless, not announced. Here's how:

### Signals You Have Enough

- Builder can state the problem in one sentence
- At least one specific user persona exists
- No-gos have been named
- Complexity is sized (even roughly)
- The builder isn't generating new information with each question

### The Pivot

Don't say: "The interview is complete. I will now generate documents."

Do say: "I think I have a solid picture. Let me draft the brief and you can tell me what I got wrong."

### Iterative Drafting

Draft BRIEF.md first. It's the smallest, most concrete document. If the brief is wrong, everything downstream will be wrong too. Get it right, then expand.

After BRIEF.md is approved, draft VISION.md. Then conditionally draft STRATEGY.md and ARCHITECTURE.md based on available signal.

Each document gets section-by-section review. Don't dump 4 documents at once.

### When the Builder Wants to Keep Talking

If the builder is energized and wants to explore further after the core documents are drafted, suggest:
- `/loaf:brainstorm` for divergent exploration
- `/loaf:research` for topic investigation
- `/loaf:strategy` for deep persona/market work
- `/loaf:shape` for bounding a specific feature

The bootstrap interview creates the foundation. Other skills deepen specific areas.

---

## Framework Attribution

| Framework | Primary Contribution | Phase(s) |
|-----------|---------------------|----------|
| Shape Up | Complexity sizing, no-gos, rabbit holes, pitch format | 2, 3 |
| Jobs to Be Done | Job statements, switching forces, competing with non-consumption | 1, 2 |
| Lean Canvas | Problem/UVP/unfair advantage triangle, existing alternatives | 1, 2 |
| The Mom Test | Question quality lens, evidence of pain, past behavior over future prediction | All phases |
| Design Sprint | Long-term goal, sprint questions, HMW reframing, journey mapping | 2, 3 |
| Wardley Mapping | Value chain, evolution axis, build vs. buy, movement awareness | 3 |
| First Principles | Assumption excavation, root cause questioning, inversion, real vs. perceived constraints | 1, 3 |
