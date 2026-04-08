# Report Frontmatter Template

**Location:** `.agents/reports/YYYYMMDD-HHMMSS-{type}-{slug}.md`

```yaml
---
title: "Report: [Topic]"
type: research | audit | analysis | council
created: YYYY-MM-DDTHH:MM:SSZ
status: draft | final | archived
source: SPEC-XXX | TASK-XXX | ad-hoc
finalized_at: YYYY-MM-DDTHH:MM:SSZ    # Set when status → final
archived_at: YYYY-MM-DDTHH:MM:SSZ     # Set when status → archived
archived_by: orchestrator
tags: []
---
```

**Archive location:** `.agents/reports/archive/`

**Archive prerequisites:**
- Status is `final`
- Linked session is archived
- `archived_at` and `archived_by` are set
