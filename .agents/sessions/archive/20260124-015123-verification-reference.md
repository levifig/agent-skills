---
session:
  title: "Create verification reference for Loaf self-sufficiency"
  status: archived
  created: "2026-01-24T01:51:23Z"
  last_updated: "2026-01-24T01:53:25Z"
  archived_at: "2026-01-25T00:00:00Z"
  archived_by: "agent-pm"
  task: "TASK-001"
  spec: "SPEC-001"
  branch: null  # Branch merged and deleted

traceability:
  requirement: "Loaf should be independent of superpowers plugin"
  architecture: []
  decisions: []

plans:
  - 20260124-015123-verification-reference.md

transcripts: []

orchestration:
  current_task: "Completed"
  spawned_agents:
    - type: backend-dev
      task: "Create verification reference and update SKILL.md"
      status: completed
      outcome: "Created verification.md with all required content, updated SKILL.md Topics table"
---

# Session: Create Verification Reference

## Context

**Task:** TASK-001 - Create verification reference
**Spec:** SPEC-001 - Loaf Self-Sufficiency
**Priority:** P1

### Background

Loaf currently relies on the superpowers plugin for verification guidance. This task creates a native verification reference document so Loaf is self-sufficient.

### Files Created/Modified

- **Created:** `src/skills/foundations/references/verification.md`
- **Modified:** `src/skills/foundations/SKILL.md` (added verification topic)

## Outcome

**All acceptance criteria met:**

- [x] `src/skills/foundations/references/verification.md` exists
- [x] Reference covers: when to verify, how to verify, common verification commands
- [x] `src/skills/foundations/SKILL.md` references the new verification topic
- [x] `npm run build` succeeds

### Verification Reference Contents

The created reference includes:
- When to Verify table (before done, commits, PRs, after changes)
- What to Verify table (tests, build, types, lint, manual)
- Common Verification Commands by language (Python, TS/JS, Ruby, Go, Rust)
- Verification Mindset section (evidence before assertions)
- Verification Checklist
- Examples for features and bug fixes
- Critical Rules (always/never)

## Decisions Log

1. Placed verification in `foundations` skill (not orchestration) because verification is a universal code quality concern
2. Followed existing reference pattern from `commits.md`
3. Kept guidance language-agnostic with language-specific command tables
