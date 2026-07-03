# Handoff Template

**Location:** `.agents/handoffs/YYYYMMDD-HHMMSS-{slug}.md`

```yaml
---
title: "[Handoff Title]"
created: YYYY-MM-DDTHH:MM:SSZ
status: draft | final | deprecated
source: conversation | branch | task | ad-hoc
branch: branch-name
deprecated_at: YYYY-MM-DDTHH:MM:SSZ  # Set only when confirmed obsolete
deprecated_by: orchestrator
tags: []
---
```

# [Handoff Title]

## Purpose

[What this handoff transfers and who or what should consume it.]

## Current State

[The concise state a future agent needs before acting.]

## Suggested Skills

- [Skill name and why the next agent should load it]

## Existing Artifacts

- [Relevant specs, tasks, ADRs, reports, issues, journal entries, commits, branches, or diffs]

## Decisions

- [Decision and rationale]

## Next Actions

- [Concrete next action]

## Open Questions and Risks

- [Question or risk the next agent should resolve]

## Deprecation Criteria

[When this handoff should be marked deprecated and deleted by housekeeping.]
