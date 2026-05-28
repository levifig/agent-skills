---
title: "Ralph Loop — Automated spec implementation with TDD"
status: raw
created: 2026-03-27T03:22:40Z
tags: [automation, TDD, implementation, appetite, orchestration]
related: [SPEC-013, orchestration, implement]
origin: brainstorm-session (SPEC-013 breakdown discussion)
---

# Ralph Loop — Automated spec implementation with TDD

An automated implementation mode that takes a spec ID and appetite level, then runs a TDD-driven loop until the spec is complete or appetite is spent.

## Core Idea

`/implement SPEC-013 --appetite 75%` (or similar) kicks off a loop:

1. Pick next task by priority + dependency order
2. Write tests first (TDD)
3. Implement until tests pass
4. Verify against task acceptance criteria
5. Review (self-review or human checkpoint)
6. Mark task done, pick next
7. At circuit breaker thresholds (50%, 75%), pause and report status
8. Repeat until spec is complete or appetite is spent

Like a CI pipeline for spec implementation — automated but with human checkpoints at the right moments.

## Problem/Opportunity

Today, implementing a broken-down spec requires manually invoking `/implement` per task or per spec, with no built-in TDD discipline, no appetite tracking, and no automatic circuit breaker pauses. The user has to manage the loop themselves.

## Constraints

- Needs Claude Code SDK or Codex SDK for true autonomous looping (aligns with Pillar 3: Autonomous Execution)
- Human review checkpoints are non-negotiable — can't ship without eyes on it
- Appetite tracking needs a way to measure "how much have we spent" (sessions? time? tasks completed?)
- TDD requires test infrastructure to exist (or be created as part of the loop)
