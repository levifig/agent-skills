# Report Template

**Location:** `.agents/reports/{YYYYMMDD}-{HHMMSS}-{type}-{slug}.md` for
authored long-form prose. Generated report output should come from `loaf report
generate ...` and does not need to be committed as a Markdown file.

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
- **archived** — Report has been processed and archived in state; authored
  Markdown reports may also be moved to `.agents/reports/archive/` as a
  compatibility artifact.

In SQLite-backed projects, lifecycle status is mutated with `loaf report
create`, `loaf report finalize`, and `loaf report archive`. Frontmatter mirrors
or documents authored prose; it is not the operational state authority.
