---
id: SPEC-011
title: Enhance /reflect with workflow-integrated suggestions
source: direct
created: '2026-03-16T23:50:30.000Z'
status: complete
---

# SPEC-011: Enhance /reflect with workflow-integrated suggestions

## Problem Statement

`/reflect` exists and works — it reads sessions, identifies learnings, proposes diffs to strategic docs, and waits for human approval. But nobody remembers to run it. Session learnings stay trapped in ephemeral session files because there's no nudge at the right moment.

The skill itself is solid. The gap is discoverability: nothing suggests running `/reflect` when a session contains extractable decisions or insights.

## Strategic Alignment

- **Vision:** Directly supports the "living knowledge base" pillar. `/reflect` is how knowledge flows from sessions into durable docs — but only if it gets invoked.
- **Architecture:** Follows ADR-006 (agents create, humans curate) — workflows suggest, humans decide.

## Solution Direction

### Suggestion placement: inside deliberate workflows

**Not** at SessionStart/SessionEnd — those fire on every session, including quick chats and one-off questions. A `/reflect` suggestion there would be jarring and ignored.

Instead, embed suggestions inside workflow skills where the user is already in a process mindset and reflection is natural:

#### 1. After `/implement` completes a spec

When `/implement` finishes its AFTER phase (post-merge housekeeping), check if the session has extractable learnings. If so, suggest `/reflect` as a final step before closing out.

This is the highest-value placement — learnings are freshest right after shipping.

#### 2. After `/shape` produces a spec

Shaping often surfaces strategic shifts, architectural constraints, or vision changes worth capturing. After the spec is written, check if the shaping session produced key decisions and suggest `/reflect`.

#### 3. During `/cleanup` session review

When cleanup reviews sessions for archival, flag sessions with unextracted learnings as "Extract & Archive" and suggest running `/reflect` before archiving.

### Detection signals

Simple checks — any of these trigger the suggestion:

- Session frontmatter has non-empty `decisions` list
- Session body contains `## Key Decisions` with content (not just the heading)
- Session body contains `## Lessons Learned` or `lessons_learned` in frontmatter
- Session is linked to a completed spec

### Suggestion format

A brief, non-blocking note at the end of the workflow output:

```
This session produced key decisions. Consider running /reflect to update strategic docs.
```

No gates, no blocking, no extra prompts. The user can act on it or ignore it.

### Reflect skill hardening

Review the existing `/reflect` SKILL.md for gaps:
- Ensure it handles the case where strategic docs don't exist yet
- Verify doc paths reference `docs/` not `.agents/`
- Update any stale references in the skill's templates

## Scope

### In Scope
- Add conditional `/reflect` suggestion to `/implement` AFTER phase
- Add conditional `/reflect` suggestion to `/shape` completion
- Add "extract before archive" flag to `/cleanup` session review
- Review and fix any stale paths in `/reflect` SKILL.md
- Build verification

### Out of Scope
- Rewriting `/reflect` (it works — we're adding nudges, not rebuilding)
- Hook-based suggestions (SessionStart/SessionEnd — too jarring)
- Auto-running `/reflect` (always suggested, never automatic)
- CLI command for reflect
- Creating a new `/crystallize` skill (reflect covers this)

### Rabbit Holes
- Over-engineering detection heuristics — simple string/frontmatter checks are enough
- Adding a "did you reflect?" gate to archival or merging — the suggestion is sufficient
- Trying to parse YAML frontmatter robustly inside skill markdown — grep patterns are fine

### No-Gos
- Don't auto-write to strategic docs without human approval
- Don't block workflow completion on reflect
- Don't suggest reflect on every session — only when detection signals are present

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Detection heuristics produce false positives | Medium | Low | Advisory only — worst case is a skipped suggestion |
| Users still skip reflect despite suggestions | Medium | Low | Acceptable — opt-in by design. The nudge is the improvement. |
| Adding suggestions to skills makes their output noisier | Low | Low | One conditional line, only when signals are present |

## Open Questions

- [x] New skill or enhance existing? → Enhance `/reflect` (no new skill)
- [x] Where to suggest? → Inside workflows (implement, shape, cleanup), not session hooks
- [x] Blocking or advisory? → Advisory only

## Test Conditions

- [ ] `/implement` suggests `/reflect` after completing a spec when session has key decisions
- [ ] `/implement` stays silent when session has no extractable learnings
- [ ] `/shape` suggests `/reflect` after producing a spec when decisions were made
- [ ] `/shape` stays silent when no decisions to extract
- [ ] `/cleanup` flags sessions with learnings as "Extract & Archive"
- [ ] `/cleanup` doesn't flag sessions without learnings
- [ ] `/reflect` SKILL.md references `docs/` paths (not `.agents/`)
- [ ] `loaf build` succeeds

## Circuit Breaker

At 50%: Ship `/implement` suggestion only (highest value). Skip shape and cleanup.

At 75%: Add `/shape` suggestion. Skip cleanup integration.
