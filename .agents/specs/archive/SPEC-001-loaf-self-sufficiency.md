---
id: SPEC-001
title: Loaf Self-Sufficiency
created: '2026-01-24T00:51:55.000Z'
status: complete
requirement: Loaf should be independent of superpowers plugin
---

# SPEC-001: Loaf Self-Sufficiency

## Problem Statement

Loaf currently relies on the superpowers plugin for meta-workflow guidance (finishing work, executing plans, verification). Users need both plugins to get full functionality. Loaf should be self-contained.

## Proposed Solution

Add native workflow guidance patterns to Loaf by:
1. Creating reference documents for verification, finishing work, executing plans
2. Integrating these patterns into existing commands
3. Optionally refactoring verbose commands to be lean invokers of skills

## Scope

### In Scope

**P1: Core Workflow Patterns** (Minimum for self-sufficiency)
- Verification before completion reference
- Finishing work workflow reference
- Integration with existing commands

**P2: Execution Patterns**
- Auto-fix rules documentation
- Executing plans reference

**P3: Improvements** (Can be deferred)
- Lean commands refactor (start-session, council-session)
- Goal-backward verification
- Persistent debug sessions

### Out of Scope (Rabbit Holes)

- Wave-based parallelization (complexity > benefit)
- PM as System Harness (adds coupling)
- Full Task Execution Loop (keep auto-fix rules only)
- Spec-reviewer agent (PM + QA sufficient)
- Semantic Task Format (XML)
- Worktree parallelization

### No-Gos

- Don't duplicate superpowers verbatim - adapt to Loaf patterns
- Don't create separate `workflows` skill - use existing structure
- Don't break existing command functionality during refactor

## Test Conditions

- [ ] Verification reference exists and is referenced by foundations skill
- [ ] Finishing work reference exists and is referenced by orchestration skill
- [ ] Auto-fix rules reference exists and is referenced by orchestration skill
- [ ] Executing plans reference exists and is referenced by orchestration skill
- [ ] `npm run build` succeeds
- [ ] Start session can use new patterns without superpowers installed

## Implementation Notes

### Design Decisions

**Workflow guidance location:** `orchestration/references/` (except verification in `foundations/references/`)

**Lean commands approach:** Incremental, largest first
- `start-session.md` (731 lines) → target ~200 lines
- `council-session.md` (569 lines) → target ~200 lines

**Persistent debug state:** File-based in `.agents/debug/`

### Files to Create

| Phase | File |
|-------|------|
| 1 | `src/skills/foundations/references/verification.md` |
| 2 | `src/skills/orchestration/references/finishing-work.md` |
| 3 | `src/skills/orchestration/references/auto-fix-rules.md` |
| 4 | `src/skills/orchestration/references/executing-plans.md` |
| 6 | `src/skills/foundations/references/goal-verification.md` |
| 7 | `src/skills/debugging/references/persistent-sessions.md` |

### Files to Modify

| Phase | File | Changes |
|-------|------|---------|
| 1 | `src/skills/foundations/SKILL.md` | Add verification topic |
| 1, 2 | `src/commands/start-session.md` | Add references to new patterns |
| 2, 3, 4 | `src/skills/orchestration/SKILL.md` | Add new topics |
| 5 | `src/commands/start-session.md` | Refactor to lean |
| 5 | `src/commands/council-session.md` | Refactor to lean |
| 7 | `src/skills/debugging/SKILL.md` | Add persistent sessions topic |

## Circuit Breaker

At 50% appetite (2 sessions): If P1 is not complete, re-evaluate scope. Consider:
- Dropping P3 entirely
- Simplifying verification to just "always verify before claiming done"
- Deferring executing-plans to future spec
