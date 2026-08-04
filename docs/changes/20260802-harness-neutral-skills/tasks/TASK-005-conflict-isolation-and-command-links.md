---
change: harness-neutral-skills
id: TASK-005
title: Per-skill conflict isolation and command-link repair
blocked-by:
  - TASK-004
---

# TASK-005 — Per-skill conflict isolation and command-link repair

## Objective

A directory Loaf cannot prove it owns costs exactly that one skill. Everything else installs, the skipped are named, and the links inside generated OpenCode commands resolve from the place those commands are actually installed.

## Scope boundaries

**In:** The ownership preflight and conflict handling in `syncManagedSkillsDirIfExists` and its planning mirror, the reporting that names skipped skills, and the link rewriting in `generateNativeOpenCodeCommands`.

**Out:** Skill naming of any kind. Decision 4 rejected the `loaf-` prefix and sent naming to the skills audit, including whether `orchestration` becomes `orchestrate`. Retirement and migration remain TASK-006. Claude Code keeps its plugin channel.

## Context pointers

- Contract: `shape.md` — Decisions 4, 5; Planning Contract / Risks
- The blast radius is the defect. `syncManagedSkillsDirIfExists` completes its ownership preflight before staging anything and returns on the first unowned or digest-mismatched entry, so one foreign directory aborts the sync for every skill and every target. The two preflight loops are the modified-managed check and the unowned-collision check
- The store's real state, measured: thirty-five Loaf skills, four third-party skills coexisting without incident (`helmor-cli`, `i-have-adhd`, `improve`, `thermo-nuclear-code-quality-review`), and one manifest entry — `orchestration` — with no directory on disk
- Skipping is not the same as forgetting. A skill skipped for conflict must not be silently dropped from the manifest, or the next run treats the destination as never-managed and the ownership story changes underneath the user
- `generateNativeOpenCodeCommands` rewrites `](templates/` and `](references/` to `](../skills/<name>/…` (`build_opencode.go:118`–`119`). From the installed location `~/.config/opencode/commands/` that resolves to `~/.config/opencode/skills/`, which has not existed since ADR-018 moved skills to `~/.agents/skills`. 60 dangling links across 19 installed commands
- The command *name* stays bare and the command *body* is inlined and self-contained; only the reference and template links are broken

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — conflict isolation and command links"
```

## Steps

- [x] Make both preflight checks collect conflicts rather than return on the first, and install every non-conflicting skill
- [x] Decide and state what happens to a conflicted skill's manifest entry, so a skip does not silently change ownership on the next run
- [x] Report skipped skills by name and reason, distinguishing "not ours" from "modified since we wrote it"
- [x] Keep the planning mirror in agreement: a skill the plan calls conflicted must be exactly the skill apply skips
- [x] Fix the generated-command link rewriting so it resolves from the installed command location to the canonical store, deriving the relative path rather than assuming a config-directory depth — or use a form that resolves regardless of where the config directory lives, and say which and why
- [x] Add `TestSkillConflictIsolation` and `TestGeneratedCommandLinksResolve`

## Verification

- `go test ./internal/cli/ -run TestSkillConflictIsolation` passes: with a foreign directory present, every other skill installs, the conflict is reported once by name, and nothing unowned is touched
- `go test ./internal/cli/ -run TestGeneratedCommandLinksResolve` passes, checking every rewritten link in every generated command against a sandbox install
- A sandbox install with a foreign `orchestration` present leaves zero dangling reference or template links in `commands/` and installs the other thirty-four skills
