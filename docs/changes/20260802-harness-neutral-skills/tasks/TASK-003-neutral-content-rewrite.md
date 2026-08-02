---
change: harness-neutral-skills
id: TASK-003
title: Harness-neutral content rewrite
blocked-by:
  - TASK-001
  - TASK-002
---

# TASK-003 — Harness-neutral content rewrite

## Objective

Skill prose describes behaviour instead of naming harness-specific tools, so every target's built body is identical. This is the long pole of the Change and the unit that turns TASK-001's failing identity test green.

## Scope boundaries

**In:** Prose in `content/skills/` that currently relies on substitution — roughly 27 files by the pre-change target diff, concentrated in the workflow skills and in `foundations/references/`.

**Out:** Editing skills for quality, structure, taxonomy, or description wording. If a skill reads badly for reasons unrelated to harness coupling, leave it and note it for the skills audit. The labeled-section convention itself is TASK-002; this task applies it.

## Context pointers

- Contract: `shape.md` — Hypothesis, Decision 2, Rabbit Holes and No-Gos
- The affected set is derivable before the rewrite: diff `dist/opencode/skills` against `dist/codex/skills` on the pre-change build

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — harness-neutral content rewrite"
```

## Steps

- [ ] Capture the affected-file list from the pre-change target diff so coverage is provable rather than asserted
- [ ] Rewrite tool references as behaviour: name what the model should accomplish and let it select its own tool, e.g. "ask one question at a time, with a recommendation, using your harness's structured question tool if it has one"
- [ ] Replace harness-name prose with second person or neutral phrasing rather than naming any single harness
- [ ] Convert genuinely product-specific facts into labeled sections per TASK-002's convention, rather than deleting them — `permissions.md`, `background-agents.md`, `bootstrap/SKILL.md`, and `foundations/references/review.md` all contain intentional cross-harness material that must survive
- [ ] Stop hard-coding slash-command forms. TASK-001 removed the Claude body rewriter that turned `/implement` into `/loaf:implement`, so every bare `/name` in content now ships unscoped to Claude Code, where the plugin form is `loaf:name`. Name the workflow in prose, or carry the invocation forms in a labeled section — do not reintroduce a rewriter
- [ ] Clear the residual `{{IMPLEMENT_CMD}}` literals TASK-001 left in `implement/references/batch-orchestration.md` (3), `loaf-reference/references/command-routing.md` (1), and `orchestration/references/local-tasks.md` (1)
- [ ] Leave `{{AGENTS_FILE}}` handling to whatever TASK-001 decided
- [ ] Confirm no paragraph is hard-wrapped — Loaf prose is one continuous line per paragraph

## Verification

- `go test ./internal/cli/ -run TestSkillTreeIsTargetInvariant` passes
- Every file on the captured affected-file list is accounted for, either rewritten or explicitly justified as needing no change
