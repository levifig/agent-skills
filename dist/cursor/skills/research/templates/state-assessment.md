# State Assessment Template

State assessments are typically presented in-conversation. If saved, use a
durable report artifact rather than `.agents/drafts/`.

**Filename timestamp:** `date -u +"%Y%m%d-%H%M%S"`

```yaml
---
title: "State Assessment: [YYYY-MM-DD]"
type: state-assessment
created: YYYY-MM-DDTHH:MM:SSZ
status: active         # active | archived
tags: []
---

# Project State Assessment

## Current Position

- [Summary of where the project stands]

## Strategic Context

- **Vision:** [Brief summary]
- **Key personas:** [Who we're building for]
- **Current focus:** [Active issues]

## Recent Progress

- [Key accomplishments from recent sessions]

## In Flight

| Issue | Status | Notes |
|-------|--------|-------|
| LOAF-1 | active | [progress] |
| LOAF-2 | todo | [next up] |

## Ideas Pipeline

- [Idea 1] -- raw
- [Idea 2] -- raw

## Lessons Learned (Recent)

- [Insights from implementation feedback]

## Open Questions

- [Unresolved decisions or gaps]

## Recommendations

1. [Actionable next step]
2. [Actionable next step]
```
