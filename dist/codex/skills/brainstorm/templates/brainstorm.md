# Brainstorm Document Template

**Location:** Markdown-only compatibility archive. SQLite-backed projects keep
brainstorm state in SQLite or summarize outcomes in an explicitly durable
report.

**Filename timestamp:** `date -u +"%Y%m%d-%H%M%S"`

```yaml
---
title: "Brainstorm: [Topic]"
type: brainstorm
created: YYYY-MM-DDTHH:MM:SSZ
status: active         # active (has unprocessed sparks) | archived
tags: []
related: []            # Optional: idea aliases, spec IDs, session aliases, report refs
---

# Brainstorm: [Topic]

**Date:** YYYY-MM-DD
**Session:** [Brief description of session scope]

## Session Journey

[How the brainstorm evolved -- pivots, reframes, key realizations]

## [Main Content Sections]

[Organized by the brainstorm's natural structure -- options explored,
analysis, trade-offs, convergence points]

## Recommendation

[If applicable -- the direction that emerged]

## Open Questions

- [Unresolved questions for further exploration]

## Sparks

- **[Title]** -- [one-line description]
- **[Title]** -- [one-line description]
```
