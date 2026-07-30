# Pitch Interview Guide

Problem-discovery interview for `/loaf:pitch`. Borrows shape's grilling mechanics (one question at a time, recommendation-first, architectural-impact ordering) and bootstrap's interview anti-patterns. The register is problem-space only — approach, architecture, and decomposition belong to `/loaf:shape`.

## Contents
- How This Guide Works
- Destination Pinning
- Problem-Discovery Dimensions
- Applicability Judgment
- Interview Mechanics
- Open Questions: Specifiability and HITL/AFK
- Exit Criteria
- Anti-Patterns
- Brief Cold-Read

## How This Guide Works

This is a **builder interview** pointed at the problem register. The agent helps the human crystallize what hurts, who feels it, what they do today, why solving it is worth it, and what bounds the answer — not how to build it.

The interview is adaptive, not a form. Strong answers skip dimensions; weak answers dig deeper. Follow the energy. Every question carries a recommended answer with rationale so the human can accept, override, or refine rather than invent from a blank page.

---

## Destination Pinning

Before dimension grilling, converge on a **destination**: one or two lines naming the end state this pitch is aiming at — what becomes true in the world if the work succeeds. Pin it early; it fixes the brief's scope for the rest of the interview.

| Scale | Destination feeds |
|-------|-------------------|
| **Project** | VISION success criteria (bootstrap extracts it; pitch keeps it as the project's north star in the BRIEF) |
| **Change** | The eventual Hypothesis when `/loaf:shape` promotes the capture — a sharper "what good looks like" than a feature list |

**How to pin:** offer a recommendation-first draft from the human's opening words ("Destination: operators can ship a release without a manual config audit"). Confirm, tighten, or rewrite until both parties can restate it. Do not start deep dimension probes until the destination is on the table.

If the destination keeps shifting mid-interview, pause and re-pin — a moving destination means the brief's scope is still open.

---

## Problem-Discovery Dimensions

Grill these five dimensions. Order by what would change the brief most — a wrong "who" invalidates value and alternatives; cosmetic naming goes last.

### 1. Problem

What friction, gap, or unmet need exists? Push past "I want to build X" to the problem behind X. Prefer past, specific moments over hypothetical futures ("Tell me about the last time this hurt" beats "Would people use…?").

**Must resolve before exit:** a one-sentence problem statement a stranger could restate.

### 2. Who Has It

Who experiences this problem? Role, context, and how often the pain shows up. "Developers" or "users" alone is too thin — name a concrete person or role-in-situation.

**Applicability:** a bug fix or internal plumbing pitch may name a single operator ("anyone who runs the release command") without a persona cast. A product or multi-audience pitch needs at least one concrete affected party.

### 3. Current Alternatives / Competitive Landscape

What do they do today? Existing tools, manual workarounds, cobbled scripts, or "nothing" are all valid. Understanding the status quo clarifies what "better" means and whether the pain is real (evidence of spend, time, or workarounds).

**Applicability:** competitive analysis is for concepts that compete with external products or obvious substitutes. Skip a formal landscape scan for pure internal refactors, bug fixes, and infra chores — "current alternative" can be one sentence ("we live with the flaky path"). When a scan is warranted, delegate a researcher subagent (see SKILL.md Evidence Delegation); do not invent competitor claims.

### 4. Value Proposition

Why is solving this worth it? What becomes true for the people who have the problem if this lands? One line: different AND better relative to the alternative — not a feature list, not an architecture sketch.

**Applicability:** always name value, but depth scales. A product pitch needs a crisp UVP; a small change can be "removes the weekly fire-drill so release day is boring."

### 5. Constraints

Non-negotiable bounds: technical, legal, organizational, philosophical, sequencing. Things that limit the solution space before design begins. Capture as problem-space limits ("must not break the promise-carrier exception"), never as chosen designs ("use Postgres").

**Always ask lightly:** at least one real constraint or an explicit "none known yet."

### Secondary (only when signal demands)

- **Sequencing and relationships** — how this hangs with other work, release cohort as prose, series order. No machine relation fields.
- **Open questions** — unresolved problem-space items that pass the [specifiability test](#open-questions-specifiability-and-hitlafk); each tagged HITL or AFK.
- **Evidence of pain** — money, time, workarounds (Mom Test lens). When absent and the claim is large, challenge gently.

---

## Applicability Judgment

| Pitch kind | Personas / who | Competitive scan | Value depth |
|------------|----------------|------------------|-------------|
| Product / multi-audience concept | Specific roles required | Usually yes — researcher subagent | Full UVP sentence |
| Feature on an existing product | One concrete affected role | Only if a substitute is obvious | Why-now + better-than-today |
| Bug fix / reliability | Operator or user of the broken path | No | Cost of the failure |
| Internal refactor / chore | Team or subsystem owner | No | Why now (risk, drag, blocked work) |
| Greenfield project | Target users required | Usually yes | Project-altitude UVP |

When unsure, ask one applicability question first: "Is this competing with something people already use, or is it pure internal friction?" Then expand or skip.

---

## Interview Mechanics

### One question at a time

Use `AskUserQuestion` when available; otherwise one inline question per message with the same semantics. Never batch dimensions into a multi-field form — forms invite skimmed defaults and are the failure mode this ceremony exists to prevent.

### Recommendation-first

Every question includes a recommended answer and a short rationale. The human overrides freely; the recommendation forces a position so the conversation is not "what do you think?" into empty space.

Example shape:

> **Who has this problem most often?**  
> Recommendation: mid-size platform engineers who already run multi-service deploys — they feel config drift weekly. Rationale: your examples all came from that world; broader "all developers" would dilute the brief.

### Ordering

Prioritize answers that would rewrite the brief. Typical order: **destination pin** → problem → who → alternatives → value → constraints → sequencing/open questions. Reorder when the human's first utterance already settles an early dimension.

Before asking, check whether reading resolves it — journal, prior Change, intake item body, BRIEF. Only ask what reading could not answer.

### Adaptive depth

| Signal | Response |
|--------|----------|
| Crisp, specific answers | Confirm, move on; skip expand-if-needed probes |
| Category answers ("developers need better tools") | Ask for a concrete story or last painful moment |
| Solution-first ("I want a CLI that…") | Pause; reframe to problem and who |
| Energy dropping | Cut to synthesis; a brief with named gaps beats an exhausted interrogation |
| Direction genuinely undecided | Offer the explore technique from inside pitch; do not force a false brief |

### Scenario stress-testing

Probe the problem with **concrete lived scenarios**, not abstract categories. Prefer "walk me through the last time this bit you" over "how often does this happen?" Invent edge cases deliberately to force boundary precision: "What if the operator is on-call at 2am with half the logs missing — is that still this problem, or a different one?" Scenarios that collapse the framing expose missing constraints or a second concept that should stay out of this brief.

### Challenge stance

Demand specificity over generalization. Probe the rationale behind claims ("why weekly, not monthly?"). Push back on vague answers until the gap closes or is named as an open question — do not paper over mush with polite summary.

Also enforce **canonical language** during the interview:

- Flag terms that conflict with `docs/knowledge/glossary.md` usage (e.g. calling a Change a "spec," a release cohort a "milestone," a skill a "module"). Offer the glossary term and get the human to accept the swap before it lands in the brief.
- Sharpen fuzzy project-local terms to a single canonical choice mid-interview ("you said both 'capture' and 'ticket' — pick one and stick to it"). Ambiguous vocabulary becomes solution fog later.

### Mid-interview evidence

When competitive landscape or external facts would change the brief and the human lacks them, pause the grill, delegate a researcher subagent, land evidence (change-scale: `research/` in the change folder; project-scale: links in Sources), then resume with a recommendation informed by the scan. Evidence supports framing; it does not become solution design.

---

## Open Questions: Specifiability and HITL/AFK

### Specifiability test

An open question earns a **precise entry** in the brief only if it can be **stated precisely now** — not answered now. The bar is: a later reader (human or agent) could work the question without inventing what was meant.

| Passes (precise entry) | Fails (coarse note only) |
|------------------------|--------------------------|
| "Does the operator need multi-region failover in v1, or is single-region acceptable for the first cohort?" | "Figure out reliability stuff" |
| "Which existing CLI command is the migration source of truth for config paths?" | "TBD on integration" |

Everything vaguer stays a **coarse note** in Open Questions or Sequencing prose — never pre-sliced into fake precision. Do not invent enumerated options the human did not surface.

### HITL / AFK tags

Mark each precise open question with one of:

| Tag | Meaning | Example |
|-----|---------|---------|
| **HITL** | Needs the human live — judgment, taste, organizational call, or access only they hold | "Will legal accept the data-retention tradeoff?" |
| **AFK** | Runnable by an agent without the human in the loop — research, codebase scan, competitive lookup | "What do the top three substitutes charge for the free tier?" |

Briefs carry the tag inline in the Open Questions section, e.g. `- [HITL, blocking] …` or `- [AFK, deferrable] …`. Blocking vs deferrable remains orthogonal: a HITL question can be deferrable; an AFK question can still block shaping if the answer rewrites scope.

---

## Exit Criteria

Stop interviewing when all of the following hold (or the human explicitly wants to land with named open questions):

1. **Destination** is pinned in one or two lines.
2. **Problem** is restatable in one sentence.
3. **Who** is concrete enough for the pitch kind (see Applicability).
4. **Current alternative** is named (tool, workaround, or nothing) — even if competitive scan was skipped.
5. **Value** is nameable without listing features.
6. **Constraints** are listed or explicitly empty.
7. Answers have stopped changing the framing — the last questions confirmed rather than rewrote.
8. Open questions that remain pass the specifiability test (or are coarse notes) and carry HITL/AFK tags when precise.
9. A cold reader could pass the brief cold-read test below.

Open questions may remain; mark each blocking or deferrable, and HITL or AFK when precise. Blocking problem-space unknowns belong in the brief's Open Questions, not as invented answers.

### The pivot

Do not announce "the interview is over." Shift: "I think I have enough to draft the brief — tell me what I got wrong." Author the brief against the shared skeleton, then section-review with the human before any init or landing step. The **closing ceremony** (recap, path, next step) is owned by SKILL.md after landing — this guide owns interview depth only.

---

## Anti-Patterns

Adopted from bootstrap's interview guide; binding on pitch.

**The Form.** Running dimensions mechanically like a survey. If answer 2 covers dimension 4, confirm and skip.

**The 45-Minute Interrogation.** If energy drops, cut to draft. Gaps in the brief are honest; drained enthusiasm is not recoverable in the same session.

**The Therapist.** Do not explore the builder's feelings about the product. User emotions (switching forces, pain) matter; builder therapy does not.

**Premature Architecture.** No "what database," "what framework," or decomposition during pitch. Solution-space content is a bounce — redirect to constraints or value if the human offers designs.

**The Echo Chamber.** Reflecting without challenge. Constructive pressure: "That's a big claim for a weekly chore — is the pain really that frequent?"

**Solution-First Questioning.** "What features should it have?" is shape's job. "What job fails today?" is pitch's.

**Asking Permission to Proceed.** Transition when signals are strong; the human will pull back if more remains.

**Over-Indexing on Frameworks.** JTBD, Mom Test, Lean Canvas are interviewer lenses — never vocabulary dumped on the builder ("Let's do a JTBD analysis").

**Third Interview Idiom.** Do not invent pitch-specific interview machinery. Destination pinning, scenario stress-testing, and challenge stance deepen the same grilling mechanics — they are not a parallel framework.

**Pseudo-Shape in the Brief.** Approach, architecture, task breakdown, or verification design must not enter `brief.md` / `docs/BRIEF.md`. If it appears while drafting, move it out and note it for `/loaf:shape`.

**Fake Precision.** Pre-slicing vague unknowns into numbered open questions that cannot yet be stated precisely. Coarse notes beat counterfeit clarity.

**Trailing Off.** Ending the session without the SKILL.md closing ceremony. The interview pivot is not the ceremony close.

---

## Brief Cold-Read

Before offering shape-now or park, cold-read the authored brief. A stranger should name, in one pass:

1. The **destination** (or success end-state)
2. The **problem**
3. The **affected user** (or operator)
4. The **current alternative**
5. The **value** of solving it

…and find **zero solution-space content** (no approach, stack, API shape, or task list). Precise open questions should carry HITL/AFK tags. If any of the five is missing or solution prose creeps in, revise before landing.
