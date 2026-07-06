# External inputs evaluation — feeding Decisions 17–20

Write-once evidence (Decision 18): what each external input contributed to this Change, evaluated 2026-07-06.

## The SDD Blueprint (NotebookLM report)

Synthesis of Shape Up, GitHub Spec Kit, Compound Engineering, RAC/Lore, and product-spec practice, generated from the "Agile Product Management" notebook.

- **Independent convergence on our architecture** (its §4, "The Delta Strategy"): self-contained change folders holding delta specs; at launch, deltas merge back into main specs and the folder archives into a chronological log. Sources that have never seen Loaf describe `docs/changes/` → ship → `docs/specs/` → `docs/changes/archive/`. This became Decision 19 and retroactively strengthens Decision 2 (archive-over-delete is half of a cycle, not a storage preference).
- **Feeds the Verification Contract format** (SPEC-017 revival, spike-harness follow-up): RFC 2119 requirement keywords and Given/When/Then scenarios.
- **Validates**: shaping as rough/solved/bounded; appetite over estimate; No-Gos; trackers as "calendar context" — our projection model in different words.
- **Conscious divergences kept**: it prescribes Tasks as a separate artifact (we collapsed them into in-doc units — litigated at Decision 5/13); its Spec→Plan→Tasks cascade is our single growing artifact.

The companion video ("How Spec-Driven Development Works", 74s) is a communications artifact — useful as an onboarding-explainer pattern, no design content beyond the report.

## Thariq — "The Unreasonable Effectiveness of HTML"

HTML as agent output format: information density (tables, SVG, interaction), "the chance of someone actually reading your spec, report or PR writeup is much much higher if it's in HTML," attach an HTML code explainer to every PR, spec-exploration webs, throwaway editors with copy-as-prompt exports.

- **Motivates Decision 20's format split** — including its own limiting caveat: "HTML diffs are noisy and hard to review compared to Markdown." Contracts stay markdown (diffable, checkable); evidence goes HTML (read rendered, never diffed).
- This Change already dogfooded the pattern: the worked-examples decision aid in this directory resolved D13/D14 in one round after prose stalled.

## Delba Oliveira — architectural walkthroughs first

"You don't need to read code anymore. Read architectural changes first, then code, if necessary" — HTML before/after walkthroughs (modules, dependencies, seams, signatures, mermaid), conditioned on verification paths existing and architecture being iterated upfront.

- **Names the review entry point** for implementation PRs (Decision 20): the walkthrough is the aid at Decision 15's stability-point review.
- Her two preconditions map to existing structure: "ways to verify" = the Verification Contract; "iterate architecture upfront" = shaping and the Planning Contract.

## research/change-structure-decision-aid.html

The comparison artifact (three section structures × two worked Changes, three check semantics × three scenarios) that settled Decisions 13 and 14 — preserved here as the first resident of `research/` and a live example of Decision 20's evidence format.
