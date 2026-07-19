---
name: brainstorm
description: >-
  Preserves the structured divergent-thinking stance consumed by the explore
  workflow: option generation before judgment, trade-off analysis, and spark
  capture. Explore owns the user-facing inquiry lifecycle; reference this
  technique from it. Not a ...
user-invocable: false
argument-hint: '[idea or problem]'
version: 2.0.0-alpha.11
---

# Brainstorm

Generative thinking — expanding possibilities before narrowing. This stance is an internal technique consumed by `/loaf:explore`, which owns inquiry continuity through Explorations and portable checkpoints; invoke the technique from there rather than as a standalone workflow.

## Critical Rules

**Always**
- Diverge before converging — generate options before judging
- Connect exploration to VISION.md and STRATEGY.md context
- Document discarded options — they hold valuable reasoning
- Log invocation first: `loaf journal log "skill(brainstorm): <topic>"`
- Capture sparks (speculative byproducts) with `loaf spark capture --scope <scope> --text <text>`; brainstorm summaries may render or summarize them
- Set boundaries on exploration time
- Log outcome to the project journal: `loaf journal log "decision(scope): direction chosen and rationale"`

**Never**
- Prematurely commit to an option before full exploration
- Create documents, reports, or any Git artifact from this technique — the surrounding Explore workflow owns checkpoints and any durable writes
- Process sparks during the divergent pass — capture only, expand later
- Turn the divergence into an interview — keep it exploratory

## Verification

After a divergent pass, verify:
- Sparks captured with `loaf spark capture` as they arose
- The surrounding Exploration checkpointed the conclusions, discarded options, and open question (`loaf exploration checkpoint`)
- The divergence referenced strategic context from VISION/STRATEGY

## Quick Reference

### Mode Detection

| Input Pattern | Mode | Output |
|---------------|------|--------|
| Idea file reference | Idea Processing | Deep dive on captured idea |
| Problem/question | Problem Exploration | Exploratory options |
| Empty | Open Brainstorm | "What should we think about?" |

### Spark Format

```markdown
## Sparks

- **Title** -- one-line description
- **Title** -- one-line description
```

Sparks are lightweight byproducts worth remembering; their dispositions belong to triage. SQLite spark state is the source; any summary inside a checkpoint item is narrative, not lifecycle.

## Suggests Next

After a divergent pass, checkpoint the surrounding Exploration (`loaf exploration checkpoint`), then suggest `/loaf:shape` if a clear direction emerged or `/loaf:triage` to disposition captured sparks and ideas.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Strategic Context | `strategy/references/` | Grounding exploration in project direction |
