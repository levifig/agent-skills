---
name: implement
description: Implements a shaped native tracker work contract in Git with evidence-led verification. Use when a canonical record is complete and ready to build. Produces code, atomic history when authorized, and verified implementation evidence for ship.
---

# Implement

Read the live canonical work contract through [`project-management/v1`](../project-management/SKILL.md), then build the smallest coherent change in Git. The tracker defines the work; repository instructions and code define the implementation surface.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Harness Integration
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(implement): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Re-read the native work record, completion criteria, hierarchy, dependencies, status, and recent relevant comments before planning.
- Confirm the live contract makes Rider, complete Journey, Entry point, observable Outcome, real Dogfood, Safety/integrity proof, Learning sought, and explicit Deferrals concrete. If it describes layers or future-only machinery instead, return it to shape.
- Inspect repository instructions and affected code before editing. Treat existing changes as user-owned.
- If the live contract is incomplete or implementation changes its intended outcome, stop and return it to shape instead of silently redefining work.
- Start each testable behavior with a focused failing test when practical, implement the minimum passing change, and keep refactoring behavior-neutral.
- Preserve the rideable journey across commits and delegation. Include only machinery exercised by its end-to-end path; reduce breadth before integrity.
- Keep commits cohesive and atomic only when the user or governing workflow authorizes commits. Never infer permission to push, merge, or publish.
- Use orchestration only when delegation is available, authorized, and materially useful; a single agent remains a valid execution path.
- Post a [tracker update](../../templates/tracker-update.md) only when progress, a blocker, or verification evidence benefits collaborators. Comments never modify the work contract.
- Re-read native state after any status transition or comment append and report indeterminate effects honestly.

## Verification

- Every completion criterion is mapped to current evidence or an explicit remaining gap.
- The named rider can complete the journey through its real entry point, and observed dogfood plus safety/integrity evidence match the live contract.
- No added foundation or abstraction waits on an unspecified future consumer.
- Focused tests, affected package tests, formatting, lint or static analysis, and the relevant build were actually run and their outputs read.
- The implementation diff contains no unrequested authority, dependency, schema, or public-interface expansion.
- Commit references and working-tree state are reported exactly; uncommitted or unverified work is labeled.
- The live tracker record was read again before handoff to ship.

## Quick Reference

| Condition | Action |
|-----------|--------|
| Contract gap | Return to shape with the exact gap |
| External blocker | Preserve code state and report observed blocker evidence |
| Independent bounded tasks | Coordinate through orchestration when authorized |
| Review finding | Add a regression test, fix, and re-run affected gates |
| Criteria satisfied | Hand live reference, diff, and evidence to ship |

## Harness Integration

### Amp

When delegating through the managed `loaf_delegate` tool, read [Amp native delegation](../orchestration/references/amp-native-delegation.md). Keep tracker operations, shell commands, tests, and acceptance with the main agent. An incompatible delegation result does not authorize selecting another mode or broadening child tools.

## Topics

Load the repository's language, testing, and domain guidance for the implementation itself.
