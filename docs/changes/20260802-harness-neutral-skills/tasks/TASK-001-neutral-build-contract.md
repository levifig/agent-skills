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

- [ ] Delete the second-stage replacer (`Claude Code`→harness name, `CLAUDE.md`→agents file, `subagent`→mechanism, `TodoWrite`/`TodoRead`→todo tool, `/loaf:`→`/`, and the rest), along with the `{{LOAF_CLAUDE_COMPAT_PATH}}` stash it needs
- [ ] Decide and record the disposition of each token: `{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`, `{{SUBAGENT_MECHANISM}}`, `{{TODO_TOOL}}` are expected to retire in favour of behavioural prose; `{{AGENTS_FILE}}` is the open question — resolve it against ADR-020 and note the answer in `shape.md` Decisions
- [ ] Move whatever survives out of the Codex-specific file into a home named for the cross-target concern it serves
- [ ] Add `TestSkillTreeIsTargetInvariant`, comparing the full built tree across all targets — frontmatter included, excepting only fields a target sidecar legitimately owns
- [ ] Add `TestNoHarnessProseSubstitution`, asserting no replacer operates on rendered markdown at all

## Verification

- `go test ./internal/cli/ -run TestNoHarnessProseSubstitution` passes
- `go test ./internal/cli/ -run TestSkillTreeIsTargetInvariant` fails for content reasons only, and passes once TASK-003 lands — this task and TASK-003 land together, so neither is a standalone commit
