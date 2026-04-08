# Report Template

**Location:** `.agents/reports/{YYYYMMDD}-{HHMMSS}-{type}-{slug}.md`

**Filename timestamp:** `date -u +"%Y%m%d-%H%M%S"`

**Type slugs:** `research`, `audit`, `analysis`, `council`

```yaml
---
title: "Report: [Topic]"
type: research | audit | analysis | council
created: YYYY-MM-DDTHH:MM:SSZ
status: draft | final | archived
source: SPEC-XXX | TASK-XXX | ad-hoc
tags: []
---

# [Title]

**Question:** [What we investigated]

## Summary

[One-paragraph answer with confidence level]

## Key Findings

1. [Finding with confidence: High/Medium/Low]
2. [Finding with confidence: High/Medium/Low]

## Methodology

[How the investigation was conducted — sources checked, tools used]

## Detailed Analysis

[Full analysis, organized by topic or finding]

## Recommendations

[Actionable next steps, prioritized]

## Sources

- [Source with confidence level]

## Open Questions

- [What remains unclear or needs follow-up]
```

## Status Lifecycle

- **draft** — Work in progress. Findings are being gathered and have not been fully validated.
- **final** — Research concludes and findings are validated. Conclusions are ready for consumption.
- **archived** — Report has been processed and moved to `.agents/reports/archive/`.
