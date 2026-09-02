---
name: orchestration
description: >-
  Coordinates bounded implementation and review work against one live native
  tracker contract. Use when parallel research, implementation, or independent
  review materially improves delivery. Produces consolidated evidence without
  creating local work units or requiring delegation.
user-invocable: false
allowed-tools: 'Read, Write, Edit, Glob, Grep, TodoWrite, TodoRead'
version: 0.5.0
---

# Orchestration

Coordinate agents around the same native reference and work contract. The main agent remains accountable for evidence and decisions; delegation changes execution topology, never tracker authority.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(orchestration): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Re-read the live work contract through [`project-management/v1`](../project-management/SKILL.md) before dividing work or accepting a result.
- Delegate only when the harness exposes the capability, governing instructions allow it, and the task is independently bounded. Delegation is optional.
- Give every agent the native reference, relevant criteria, allowed actions, file or subsystem boundary, and required verification.
- Carry the rideable-increment answers in every relevant delegation packet. Execution may split commits and handoffs, but it must not fragment the end-to-end value contract into local work authorities or future-only layers.
- Assign one writer to a shared change at a time. Use independent read-only reviewers after implementation.
- Keep provider operations with the main agent through the selected provider skill. Dedicated provider profiles are deferred until every target can package and enforce them without broken links or broader authority.
- Treat agent results as claims. Inspect delivered diffs and rerun the contract's gates before accepting them.
- Consolidate reviewer perspectives by evidence and defect class rather than vote count.
- Read [Delegation Contract](references/delegation.md) before coordinating multiple writers or review rounds.
- Return through the harness by default. Persist a result only when it must survive the immediate response because work is asynchronous, large, cross-conversation, multi-consumer, or explicitly requested.
- When persistence is warranted, use `.agents/reports/YYYYMMDDHHMMSS-slug.md` with a UTC timestamp and a descriptive slug. Use the purpose-specific [background result](templates/background-result.md) or [review convergence](templates/review-convergence.md) template; never invent report status or a universal report schema.
- Read [Background Agents](references/background-agents.md) before assigning non-interactive work.

## Verification

- Every delegated task is traceable to a live completion criterion or explicit review lens.
- The coordinated result preserves the named rider's complete journey, and foundation packets remain tied to the immediate path that exercises them.
- Exactly one writer owned each overlapping change at any moment.
- Delivered commits, diffs, and test claims were independently inspected.
- Provider mutations used the same common contract as main-agent execution and were verified by native readback.
- The consolidated result names disagreements, accepted findings, rejected findings with evidence, and remaining risk.
- Any persisted report uses a skill-owned template, does not overwrite an existing file, and is referenced in the harness return.

## Quick Reference

| Need | Execution shape |
|------|-----------------|
| Small cohesive change | Main agent directly |
| Independent research questions | Parallel read-only agents |
| Disjoint implementation areas | Bounded writers with non-overlapping ownership |
| Quality gate | Fresh read-only reviewers |
| Tracker operation | Main agent or optional connector-only project manager |
| Immediate bounded result | Return through the harness only |
| Result needed after this response | Persist with the matching orchestration template |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Delegation | [delegation.md](references/delegation.md) | Defining agent boundaries, writer ownership, and review convergence |
| Background agents | [background-agents.md](references/background-agents.md) | Running bounded non-interactive work and deciding whether its result needs a file |
| Background result | [background-result.md](templates/background-result.md) | Persisting a background result that must outlive its harness return |
| Review convergence | [review-convergence.md](templates/review-convergence.md) | Persisting multi-round review evidence and dispositions |
