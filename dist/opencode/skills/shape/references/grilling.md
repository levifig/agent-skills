# Grilling

The interview technique for `[KU]` entries — known unknowns precise enough to phrase as a question. Imported from the reviewed `grill-me` / `grill-with-docs` pattern (`docs/changes/20260704-shape-first-change-workflow/research/mattpocock-review/`), sharpened with the Field Guide's architectural-impact ordering.

This is a distinct, shape-specific technique — not the shared `templates/grilling.md` used by `architecture` and `refactor-deepen`. That template's glossary-mutation duty doesn't fit shape's ambiguity-resolving step (see the deferral rationale it documents); shape's grilling stays scoped to fog entries and never writes to the domain glossary.

## The Mechanic

One question at a time, through `AskUserQuestion`. Harnesses without that tool degrade to one inline question per message — same semantics, just without the structured UI. Never batch questions into a form; a form invites the user to skim and accept defaults, which is exactly the quick-capture failure grilling exists to avoid.

Every question carries a recommended answer with rationale — never "what do you think?" alone. The recommendation forces a position; the user is free to override it, but the question isn't complete without one.

## Ordering

Prioritize questions whose answer would change the architecture. Cosmetic questions — naming, ordering, presentation — go last, even when they're easier to answer. An architecture-changing answer received late can invalidate everything decided in between; asking it first avoids that rework.

Before asking, check whether reading resolves the question — an existing ADR, a prior Change, a journal entry. Only ask what reading couldn't answer.

## Stop Condition

Stop when either holds:

- No unrouted `[KU]` entries remain.
- Answers stop changing the contract — the last several questions confirmed direction rather than altering it.

Grilling never opens the session. Context gathering and the blindspot-pass offer come first; an interview without that groundwork asks questions the codebase or journal could have answered for free.

## Mid-Interview Reroute

If a question turns out to need domain fluency the shaper doesn't have — the follow-up can't even be phrased — stop grilling it and route the entry to the blindspot pass instead of guessing at an answer.
