---
captured: 2026-04-07T19:15:00Z
status: raw
tags: [reports, research, templates, lifecycle]
related: [research, housekeeping]
origin: housekeeping review — noticed research outputs go to drafts/ not reports/, and reports/ has no template or frontmatter
---

# Report template and lifecycle

## Problem

1. The research skill routes all output to `.agents/drafts/` — finished research findings should go to `.agents/reports/`
2. `.agents/reports/` has no template, no frontmatter schema, and no lifecycle tracking
3. The one existing report (TASK-074 sidecar audit) has no frontmatter — just a raw markdown heading

## Proposed Fix

- Create a report template with frontmatter (title, type, created, status, source spec/task, archived_at)
- Update the research skill to output finished findings to `reports/`, keep in-progress work in `drafts/`
- Update housekeeping skill to scan `reports/` with lifecycle awareness
- Define the `drafts/` → `reports/` promotion path (when does a draft become a report?)

## Scope

Small — one template, two skill updates (research output path, housekeeping scan).
