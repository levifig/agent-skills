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

**In:** Every surface that carries a skill's identity — canonical directory name, frontmatter `name`, the managed-skills manifest, agent `skills:` lists, bodies that instruct an agent to load another skill by name, permission patterns that name a skill, the link rewriting that points generated OpenCode commands back at their skill directory, and Claude plugin skill names.

**Out:** Whether a given skill should exist at all, or under a different name — that is the skills audit. Typed command surfaces keep their bare names.

**Lands atomically with TASK-006.** Adding retirement entries for the unprefixed names is unsafe until ownership hardening exists; see that task and `shape.md` Decision 7.

## Context pointers

- Contract: `shape.md` — Decisions 4, 6, 7; Open Questions (retirement window, plugin names)
- OpenCode requires frontmatter `name` to match the containing directory, so a renamed directory with an unrenamed `name` is invalid. The build currently preserves the source `name` and only merges sidecars and version fields (`build_codex.go:245`–`273`)
- Bare names resolve outside installation: `content/agents/background-runner.opencode.yaml:5-8` loads `foundations`, `content/agents/librarian.opencode.yaml:6-9` loads `orchestration`, and bodies name `debugging` directly (`content/skills/implement/SKILL.md:269-276`, `content/skills/foundations/references/tdd.md:45-50`)
- The typed-command exception is verified: OpenCode emits `commands/<bare-name>.md` independently, and the parity check treats command reachability separately (`build_opencode.go:73-124`, `build_test.go:1356-1371`). The command *name* stays bare
- But the command body is only partly self-contained. `build_opencode.go:118-119` rewrites relative links to `../skills/<name>/…`, which from the install location `~/.config/opencode/commands/` resolves to `~/.config/opencode/skills/` — absent since ADR-018. That is 60 dangling links across 19 installed commands today, and the prefix work must fix both halves: the name and the path
- 17 of 35 skills are `user-invocable: false`, but prefixing is free for all 35 — the invocable ones reach users through the generated command file or plugin scoping, neither of which reads the directory name

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
- [ ] Fix the generated-command link rewriting so it targets the prefixed directory *and* a path that resolves from the installed command location — the current form has been dangling since the canonical relocation
- [ ] Add the unprefixed `retired_skills` entries, and choose the window with a stated reason — the one-release rule was written for one or two entries, not 35
- [ ] Add `TestSkillIdentityContract` and `TestBareCommandsResolveToPrefixedSkills`

## Verification

- `go test ./internal/cli/ -run TestSkillIdentityContract` passes: `name` matches directory for every built skill, and every agent reference and cross-skill load resolves
- `go test ./internal/cli/ -run TestBareCommandsResolveToPrefixedSkills` passes, including that every rewritten link in a generated command resolves to a real file from the installed command location
- A sandbox install leaves zero dangling reference or template links in `commands/`
- Installing into a sandbox HOME produces only `loaf-` prefixed Loaf directories, and Loaf no longer claims `orchestration`
