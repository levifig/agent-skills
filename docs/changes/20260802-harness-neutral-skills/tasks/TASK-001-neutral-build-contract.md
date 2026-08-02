---
change: harness-neutral-skills
id: TASK-001
title: Harness-neutral build contract
blocks:
  - TASK-002
  - TASK-003
---

# TASK-001 — Harness-neutral build contract

## Objective

The build stops rewriting authored prose. Blind string replacement is gone, each of the five tokens has an explicit disposition, whatever survives lives somewhere named for what it does, and a test asserts that skill bodies are byte-identical across every target.

## Scope boundaries

**In:** `substituteNativeBuildHarnessLanguage` and `nativeBuildHarnessLanguages` in `internal/cli/build_codex.go`, their callers across the target builders, and the new identity test.

**Out:** Rewriting skill content — that is TASK-003, and this task's test is expected to fail until it lands. The literal-config mechanism is TASK-002.

## Context pointers

- Contract: `shape.md` — Decisions 2 and 3, Planning Contract / Placement
- The substitution table and the 17-pair replacer are at `internal/cli/build_codex.go:502`–`590`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — harness-neutral build contract"
```

## Steps

- [x] Delete the second-stage replacer (`Claude Code`→harness name, `CLAUDE.md`→agents file, `subagent`→mechanism, `TodoWrite`/`TodoRead`→todo tool, `/loaf:`→`/`, and the rest), along with the `{{LOAF_CLAUDE_COMPAT_PATH}}` stash it needs
- [x] Decide and record the disposition of each token: `{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`, `{{SUBAGENT_MECHANISM}}`, `{{TODO_TOOL}}` are expected to retire in favour of behavioural prose; `{{AGENTS_FILE}}` is the open question — resolve it against ADR-020 and note the answer in `shape.md` Decisions
- [x] Move whatever survives out of the Codex-specific file into a home named for the cross-target concern it serves
- [x] Add `TestSkillTreeIsTargetInvariant`, comparing the full built tree across all targets — frontmatter included, excepting only fields a target sidecar legitimately owns
- [x] Add `TestNoHarnessProseSubstitution`, asserting no replacer operates on rendered markdown at all

## Token dispositions (TASK-001)

| Token | Disposition | Reasoning |
|-------|-------------|-----------|
| `{{HARNESS_NAME}}` | Retired | Behavioural prose; models know their harness. No content usages remained. |
| `{{INTERVIEW_TOOL}}` | Retired | Behavioural prose ("structured question tool if available"). No content usages remained. |
| `{{SUBAGENT_MECHANISM}}` | Retired | Behavioural prose / labeled sections for product-specific spawn APIs. No content usages remained. |
| `{{TODO_TOOL}}` | Retired | Behavioural prose; the old substitution corrupted fenced allowlists. No content usages remained. |
| `{{AGENTS_FILE}}` | Retired (Decision 9) | ADR-020: root `AGENTS.md` is canonical; `.claude/CLAUDE.md` is only a symlink. Author `AGENTS.md`; label the compat path when the symlink itself is the fact. |
| `{{IMPLEMENT_CMD}}` / `{{RESUME_CMD}}` / `{{ORCHESTRATE_CMD}}` | Retired | All three already expanded to `/implement` for every target. Author `/implement` or name the implement workflow. Five residual `{{IMPLEMENT_CMD}}` sites remain in content until TASK-003. |

Nothing survived to relocate: the first-stage table and second-stage replacer are deleted. Cross-target skill invariance helpers live in `internal/cli/build_skill_invariance.go` (named for the contract that replaced substitution). Claude plugin slash-command scoping of skill bodies (`nativeClaudeScopeCommands`) was also removed because it rewrote rendered markdown per target and broke the same invariant.

## Verification

- `go test ./internal/cli/ -run TestNoHarnessProseSubstitution` passes
- `go test ./internal/cli/ -run TestSkillTreeIsTargetInvariant` — **passes after TASK-001 alone.** Removing substitution (and Claude's per-target description truncation) made skill trees target-identical immediately; residual Claude-first *wording* is the same on every target and is TASK-003's concern (H1 / neutral authoring), not cross-target identity (V1). Five literal `{{IMPLEMENT_CMD}}` sites remain in content and built output until TASK-003.
