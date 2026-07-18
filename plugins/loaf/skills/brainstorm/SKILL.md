---
name: brainstorm
description: >-
  Conducts structured brainstorming with divergent thinking and trade-off
  analysis. Use when the user asks "help me think through this," "what are the
  options," or is exploring tradeoffs. Produces docs with sparks. Not for quick
  ideas or shaping.
user-invocable: true
argument-hint: '[idea or problem]'
version: 2.0.0-alpha.7
---

# Brainstorm

Generative thinking — expanding possibilities before narrowing through structured exploration.

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
- Delete brainstorm documents — archive them for context preservation
- Process sparks during the main brainstorm — capture only, expand later
- Turn brainstorm into an interview — keep it exploratory

## Verification

After work completes, verify:
- Brainstorm captured in SQLite or summarized in an explicitly durable report
- Sparks captured with `loaf spark capture` and optionally summarized in `## Sparks`
- Spark lifecycle documented: unprocessed → promoted/discarded
- Brainstorm references strategic context from VISION/STRATEGY

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

Sparks are: lightweight, byproducts, worth remembering. Mark as `*(promoted)*` or `*(abandoned)*` after processing.

Brainstorm documents are archived after sparks are processed — never deleted, since the exploration context has lasting value. SQLite spark state is the lifecycle source; draft markdown is a projection or narrative summary.

## Suggests Next

After brainstorming, suggest `/loaf:shape` if a clear idea emerged, or `/loaf:idea` to capture sparks for later. `/loaf:idea` invoked without arguments scans brainstorm docs for unprocessed sparks, bridging the brainstorm → idea pipeline.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Brainstorm Template | [templates/brainstorm.md](templates/brainstorm.md) | Creating structured brainstorm documents |
| Strategic Context | `strategy/references/` | Grounding exploration in project direction |
