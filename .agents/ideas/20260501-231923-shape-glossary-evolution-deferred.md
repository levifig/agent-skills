---
title: "/shape glossary evolution — revisit after /architecture and /refactor-deepen prove the convention"
captured: 2026-05-01T23:19:23Z
status: raw
tags: [shape, glossary, vocabulary, deferred]
related:
  - SPEC-034-refactor-deepen-grilling-glossary
blocked_by: SPEC-034
---

# /shape glossary evolution (deferred)

## Nugget

SPEC-034 originally proposed Track D — `/shape` flags candidate terms to glossary as they surface during shaping. Removed during shape review. Reasoning: `/shape` is an *upstream ambiguity funnel* — the convergent step where fuzzy concepts get bounded. Terms surfaced during shaping are at peak ambiguity; capturing them as glossary candidates risks polluting the canonical vocabulary with terms that get renamed, redefined, or dropped as the spec firms up. Glossary's value is stability; writing to it from inside an ambiguity-resolving step undermines that.

After SPEC-034 ships and `/architecture` (stabilizes terms) + `/refactor-deepen` (pressure-tests terms) have a few real uses under their belt, revisit whether `/shape` should participate in glossary mutation, and if so, *how*.

## Problem/Opportunity

Today (post-SPEC-034 shipping), `/shape` does not write to the glossary. This means:

- Terms genuinely introduced for the first time during shaping (rare but real — a new feature may name a new domain concept) have no path into the glossary until much later, when `/architecture` or `/refactor-deepen` happens to encounter them.
- Re-shaping a spec doesn't benefit from the glossary built up by other skills — `/shape` reads the glossary (per the shared `grilling.md` template's read/challenge rules) but cannot contribute back.

The frictions may be small in practice. We won't know until we observe actual shape sessions running against a populated glossary.

## Initial Context

- **Removed from SPEC-034 by deliberate decision.** Codex's "upstream ambiguity funnel" framing made the case: `/shape` is the wrong skill to author glossary state because shape-stage language is still in flux.
- **Possible quieter patterns to consider when revisiting:**
  - **Silent batch capture.** Shape session silently appends candidates to a private "pending" queue; presents "5 terms surfaced — review for glossary?" once at session end. Quieter than inline surfacing.
  - **Read-only enforcement.** `/shape` reads glossary, challenges term drift, but never writes — even candidates. Glossary stays authored by `/architecture` + `/refactor-deepen`; `/shape` just enforces against existing entries.
  - **Promote-on-spec-completion.** Terms that survive into the final spec body (i.e., made it through the shaping ambiguity) auto-promote to glossary candidates when the spec is approved. Tied to the spec lifecycle, not the shape session.
- **Validation gates before re-shaping:**
  - Has the glossary actually been populated by `/architecture` / `/refactor-deepen`? If empty, the question is moot.
  - Have shape sessions drifted from the glossary (using "boundary" when the glossary says "seam")? If yes, read-only enforcement is the priority. If no, silent batch may be enough.
  - Are there cases where genuinely new terms were introduced during shape and *should* have been in the glossary earlier? Concrete examples will inform the design.
- **Open questions for shaping (when revisited):**
  - Which of the three patterns (silent batch / read-only / promote-on-completion) wins?
  - Does this become its own SKILL.md rule, or a Critical Rule added to all skills that read the glossary?
  - Does the answer change in Linear-native mode where shape sessions may produce Linear issues with team-visible terminology?
- **Sequencing:** explicitly deferred. Revisit after SPEC-034 ships *and* at least one real `/architecture` invocation + one real `/refactor-deepen` invocation have used the glossary mechanic. Likely 1–2 sprints of dogfooding before the question becomes answerable.
