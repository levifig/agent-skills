---
id: TASK-172
title: 'ADR: agentic state storage model (shared vs branch-local)'
spec: SPEC-036
status: done
priority: P3
created: '2026-05-18T23:59:01.537Z'
updated: '2026-05-19T10:21:08.192Z'
depends_on:
  - TASK-166
  - TASK-170
session: .agents/sessions/20260519-101320-session.md
completed_at: '2026-05-19T10:21:08.191Z'
---

# TASK-172: ADR: agentic state storage model (shared vs branch-local)

## Description

Author the ADR documenting the agentic state storage model decision (**A3**). Written **after** TASK-166 and TASK-170 complete so the narrative reflects what actually shipped.

Required sections (per the project ADR convention in `docs/decisions/`):

- **Title** and **decision date** (ISO 8601)
- **Status:** Accepted
- **Context:** the worktree fragmentation problem — session misrouting, ID clashes, fragmented project knowledge; the category error of treating `.agents/` as branch-scoped content when it's project/process state
- **Decision:** `.agents/` is project-scoped, not branch-scoped. `findAgentsDir()` resolves to the main worktree's `.agents/` from any worktree. No artifact kind is exempt; specs/tasks/plans included.
- **Alternatives Considered:**
  - A1 (sessions-only routing) — rejected: doesn't address ID clashes or fragmented knowledge
  - A2 (per-artifact-kind split, specs/tasks/plans branch-local) — rejected: per-call-site refactor, dual-view scanning, and the bought property (PR-diff visibility for specs/tasks) isn't load-bearing under squash-merge workflows
  - Symlinks — rejected: fragile across `git worktree add`
  - Storage under `.git/loaf/` — rejected: not inspectable as a normal directory
- **Consequences:**
  - Positive: cross-worktree session continuity, collision-free IDs, single shared knowledge view, no per-call-site refactor, single resolver
  - Negative: sessions/specs/tasks no longer appear in PR diffs; "spec on main, tasks+code on branch" convention is retired
- **Compliance:** ADR + migration command represent a hard cut; pre-A3 layouts are refused, not silently fallback'd
- **Follow-on:** update `AGENTS.md` and project memory to reflect the retired "spec on main, tasks+code on branch" convention

## Acceptance Criteria

- [ ] ADR file exists under `docs/decisions/` with the project's standard naming
- [ ] All required sections present
- [ ] ADR status set to `Accepted`
- [ ] Decision date in ISO 8601
- [ ] SPEC-036 frontmatter or body updated with a link to the ADR
- [ ] `.agents/AGENTS.md` references to "spec on main, tasks+code on branch" updated or removed
- [ ] Project memory feedback file `feedback_branch_at_breakdown.md` (or equivalent) updated/superseded

## Files

- New: `docs/decisions/ADR-NNN-agentic-state-storage-model.md`
- Edit: `.agents/specs/SPEC-036-worktree-aware-agents-storage-routing.md` (add ADR link)
- Edit: `.agents/AGENTS.md` (convention update)

## Verification

```bash
ls docs/decisions/*agentic-state-storage*
# Manual review of section completeness
```

## Context

See SPEC-036. Track C. Depends on TASK-166 and TASK-170 so the ADR reflects shipped reality.
