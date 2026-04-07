# Research Findings Template (Draft)

Working document for in-progress research. Promote to `.agents/reports/` using the [report template](report.md) when findings are complete.

**Location:** `.agents/drafts/{YYYYMMDD}-{HHMMSS}-research-{slug}.md`

**Filename timestamp:** `date -u +"%Y%m%d-%H%M%S"`

```yaml
---
title: "Research: [Topic]"
type: research
created: YYYY-MM-DDTHH:MM:SSZ
status: active         # active | archived
tags: []
related: []            # Optional: spec IDs, idea files, ADRs
---

# Research: [Topic]

**Question:** [What we're trying to understand]

## Summary

[One-paragraph answer with confidence level]

## Key Findings

1. [Finding 1]
2. [Finding 2]

## Project Context

[How this relates to our existing decisions/loaf:architecture]

## Options Considered

### Option A: [Name]
- **Pros:** ...
- **Cons:** ...
- **Best for:** ...

### Option B: [Name]
- **Pros:** ...
- **Cons:** ...
- **Best for:** ...

## Recommendation

[Which option and why, with caveats]

## Sources

- [Source 1 with confidence level]
- [Source 2 with confidence level]

## Implications

- [What this means for our work]

## Open Questions

- [What's still unclear]
```
