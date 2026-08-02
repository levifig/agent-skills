---
change: harness-neutral-skills
id: TASK-005
title: Prefix identity contract
blocked-by:
  - TASK-004
relates-to:
  - TASK-006
---

# TASK-005 — Prefix identity contract

## Objective

A Loaf skill has one identity, `loaf-<name>`, and every surface that names it moves together. Sharing a canonical store with other vendors then stops being a collision risk, and Orca's `orchestration` is vacated rather than contested.

## Scope boundaries

**In:** Every surface that carries a skill's identity — canonical directory name, frontmatter `name`, the managed-skills manifest, agent `skills:` lists, bodies that instruct an agent to load another skill by name, permission patterns that name a skill, and Claude plugin skill names.

**Out:** Whether a given skill should exist at all, or under a different name — that is the skills audit. Typed command surfaces keep their bare names.

**Lands atomically with TASK-006.** Adding retirement entries for the unprefixed names is unsafe until ownership hardening exists; see that task and `shape.md` Decision 7.

## Context pointers

- Contract: `shape.md` — Decisions 4, 6, 7; Open Questions (retirement window, plugin names)
- OpenCode requires frontmatter `name` to match the containing directory, so a renamed directory with an unrenamed `name` is invalid. The build currently preserves the source `name` and only merges sidecars and version fields (`build_codex.go:245`–`273`)
- Bare names resolve outside installation: `content/agents/background-runner.opencode.yaml:5-8` loads `foundations`, `content/agents/librarian.opencode.yaml:6-9` loads `orchestration`, and bodies name `debugging` directly (`content/skills/implement/SKILL.md:269-276`, `content/skills/foundations/references/tdd.md:45-50`)
- The typed-command exception is verified: OpenCode emits `commands/<bare-name>.md` independently from self-contained bodies (`build_opencode.go:73-124`), and the parity check treats command reachability separately (`build_test.go:1356-1371`)

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — prefix identity contract"
```

## Steps

- [ ] Enumerate every identity surface before changing any of them, so the contract is provable rather than remembered
- [ ] Apply the prefix to the canonical directory, frontmatter `name`, and the managed-skills manifest together
- [ ] Update agent `skills:` lists and any permission pattern naming a skill
- [ ] Resolve cross-skill loads: either prefix the names bodies use, or stop naming sibling skills in bodies entirely — decide which, since the second is simpler and may be better regardless, and it interacts with Claude's plugin channel
- [ ] Confirm typed commands still resolve under bare names
- [ ] Add the unprefixed `retired_skills` entries, and choose the window with a stated reason — the one-release rule was written for one or two entries, not 35
- [ ] Add `TestSkillIdentityContract` and `TestBareCommandsResolveToPrefixedSkills`

## Verification

- `go test ./internal/cli/ -run TestSkillIdentityContract` passes: `name` matches directory for every built skill, and every agent reference and cross-skill load resolves
- `go test ./internal/cli/ -run TestBareCommandsResolveToPrefixedSkills` passes
- Installing into a sandbox HOME produces only `loaf-` prefixed Loaf directories, and Loaf no longer claims `orchestration`
