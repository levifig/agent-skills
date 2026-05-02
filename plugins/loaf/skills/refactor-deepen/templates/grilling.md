# Grilling Protocol

Shared interview-protocol template for skills that need to grill a user (or themselves) before producing an artifact. Distributed by `targets.yaml` to skills that conduct relentless interviews — currently `architecture` and `refactor-deepen`. `shape` is intentionally **not** a consumer (see deferral rationale in `20260501-231923-shape-glossary-evolution-deferred`).

## Contents
- The Four Mechanics
- Glossary Discipline
- What This Template Does Not Own
- When to Stop Grilling

## The Four Mechanics

These four rules are the load-bearing parts of the protocol. Drop any of them and the interview degrades into "ask a few questions and accept the first answer."

### 1. Relentless Interview

Do not accept underspecified answers. When a user response leaves a load-bearing decision implicit, ask the follow-up. Treat "use your best judgment" as a signal that you have not yet exposed the constraint, not a license to proceed.

Stop only when one of:
- The decision is fully specified.
- The user has explicitly named the unknowns and accepted the risk of proceeding without them.
- Continuing would only resolve cosmetic detail (naming, ordering, color) that the user can correct after the artifact lands.

### 2. Walk the Decision Tree

Each question opens branches. Walk all of them before returning to the parent. If a branch is closed by an existing decision (ADR, glossary entry, prior `/loaf:refactor-deepen` plan), surface that closure to the user — do not silently re-derive.

Order branches by **load-bearing weight**, not by ease of answering. Cosmetic questions go last, even when they're easy.

### 3. Recommend Per Question

For every question with more than one viable answer, the skill must offer a recommendation **with rationale** — not "what do you think?" but "I recommend X because Y; the costs are Z." The user is free to override, but the recommendation forces the skill to take a position.

If the recommendation depends on information the skill doesn't have, escalate to a follow-up question rather than naming "it depends" as the answer.

### 4. Explore When You Can Answer

When a question can be resolved by reading the codebase, an existing ADR, the glossary, or any other authoritative source, **explore first and answer second**. Do not ask the user to answer something the skill could have looked up.

The exception: when reading would consume substantial budget (deep multi-file analysis) and the user can answer in seconds, ask. Be honest about the tradeoff.

## Glossary Discipline

The grilling protocol is the natural place where vocabulary drift surfaces. These three rules apply to every skill that imports this template:

### Read at Start

Before the first question, read `docs/knowledge/glossary.md` via:

```bash
loaf kb glossary list
```

If no glossary exists yet, the file is created lazily on first mutation — there's nothing to read at start, but the skill should still attempt the call so empty-state messaging is consistent.

### Challenge Drift Inline

When the user (or the skill itself) reaches for a term that has a canonical replacement in the glossary, surface the canonical term and ask the user to confirm. Example:

> You said "service" — the glossary canonicalizes this as `Module` and lists `service` under `_Avoid_`. Switch the framing, or is the user describing something different?

Do not silently substitute. Drift awareness is the entire point.

### Surface Candidates

When the interview produces a load-bearing term that does not yet exist in the glossary — canonical or candidate — surface the term and ask whether to add it. **The consuming skill decides which mutation verb to call** (and whether to call one at all). This template intentionally does not name verbs or prescribe commitment levels — see the consuming skill's SKILL.md for its mutation policy.

## What This Template Does Not Own

The template defines the *interview shape*, not the *output shape* or the *mutation policy*. Specifically:

- **The artifact format** (ADR, plan, brief, etc.) belongs to the consuming skill's own template.
- **The mutation policy** for new glossary terms — which CLI verb to call, when, and with what commitment — is decided by the consuming skill. See each skill's Critical Rules section for its policy.
- **Whether to grill at all** is the consuming skill's call. Some invocations (e.g., trivial decisions, follow-up clarifications) skip the protocol entirely.

If a skill needs to extend this template (add a fifth mechanic, override a rule), it should do so in its own SKILL.md — do not modify this file in place. The template is shared across all consumers.

## When to Stop Grilling

Three terminating conditions, in priority order:

1. **The decision is converged.** Every load-bearing branch has an answer with rationale. The artifact can be written.
2. **The user names the residual unknowns and accepts the risk.** This is a deliberate ship-with-known-gaps, captured in the artifact (e.g., as Open Questions in a spec, or "Out of Scope" in a plan).
3. **The interview reveals the decision is the wrong shape.** The user wanted A; grilling exposed that they need B. Stop, surface the reframe, and ask whether to switch tracks.

Do not stop because the user got tired of questions. If fatigue is real, take it as a signal to consolidate (group the remaining questions) — not to skip them.
