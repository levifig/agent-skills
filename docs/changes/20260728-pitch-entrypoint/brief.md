<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     Superseded by shape.md; never updated after shaping; never mechanically load-bearing.
     Captured in the pre-upgrade brief format: this Change ships the richer skeleton, so its own brief predates it. -->

# Pitch — the Loaf Flow entry point

## Original ask

Fine-tune the "Loaf Flow": we need a `/pitch` command that comes either after `/triage` (to start working on an idea/spark) or at the start of a session, on both new and existing projects. On an existing project, `/pitch` is the entrypoint for a change — it grills the concept/idea/problem (problem discovery, competitive analysis if applicable, value proposition if applicable) to create the initial `brief.md` that kickstarts the change; `/shape` then works on top of that brief, grilling further but focused on implementation rather than problem discovery. On a new project, `/pitch` does the initial grilling to get to `docs/BRIEF.md`, which is then bootstrapped into a project, and work carries on via further pitches/changes. Possibly a transformation of the `brainstorm` skill, honed down and made more productive. "This will be a critical skill for the whole Loaf system!! We need to nail it!"

## Context

The ask arrived embedded in the full Loaf Flow vision: pitch → shape (rounds of review and shaping) → final change state pushed under an open PR → implementation delegated to subagents, one task per commit → review rounds until no blocker/major (ideally no high/medium/low) issues remain, capped at 10 rounds, with fixing tasks dispatched between rounds → optional human review → merge → and, per project configuration, beta/RC/stable promotion when a merge completes a target version's arc. Loaf as a full agentic development package for solo devs and teams, dogfooded on Loaf itself and other projects. The same message also asked to refactor/review all skills to fit this model, and to consider dedicated subagents (instead of skills) for mechanical operations like git and knowledge-base management if evidence supports better instruction adherence.

## Notes

The surrounding vision decomposed during shaping: the review-convergence loop, the all-skills audit, and the subagents-vs-skills question routed to their own Intents; the release-promotion arc was already queued as the promotion-model design. This Change delivers the entry stage at both scales.
