# Report Frontmatter Template

**Location:** `.agents/reports/YYYYMMDD-HHMMSS-subject.md`

```yaml
---
report:
  status: processed
  session_reference: ".agents/sessions/YYYYMMDD-HHMMSS-title.md"
  processed_at: "YYYY-MM-DDTHH:MM:SSZ"
  archived_at: "YYYY-MM-DDTHH:MM:SSZ"
  archived_by: "orchestrator"
---
```

**Archive location:** `.agents/reports/archive/`

**Archive prerequisites:**
- Report is processed
- Linked session is archived
- Session captures key conclusions and action points
- `archived_at` and `archived_by` are set
