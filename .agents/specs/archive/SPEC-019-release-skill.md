---
id: SPEC-019
title: Release Skill — orchestrated merge ritual with CLI-backed versioning
source: idea/20260330-idea-merge-command
created: '2026-03-30T15:20:00.000Z'
status: complete
appetite: Small (1–2 sessions)
---

# SPEC-019: Release Skill

## Problem Statement

The merge ritual is scattered across manual steps, advisory hooks, and skill documentation with no single coordination point:

1. **Version bump timing is wrong**: SPEC-014's version bump happened post-merge in a separate commit, creating a stale-version window on main. The bump must be on the feature branch so the squash commit carries it.

2. **`[Unreleased]` section lost**: Manual changelog editing converted `[Unreleased]` to a versioned header without adding a fresh one back. `loaf release`'s `insertIntoChangelog` does this correctly, but it wasn't used.

3. **Documentation staleness unchecked**: After major features (e.g., replacing 8 agents with 3 profiles), README and ARCHITECTURE.md may reference removed concepts. Nothing in the merge flow flags this.

4. **No enforcement, only reminders**: The `workflow-pre-merge` hook is a non-blocking prompt. The `workflow-post-merge` hook emits an informational checklist. The implement skill's AFTER section documents the flow but doesn't enforce ordering.

## Solution Direction

### Part 1: Enhance `loaf release` CLI with non-interactive flags

Add flags to `cli/commands/release.ts` so the skill can drive versioning mechanically:

| Flag | Purpose |
|------|---------|
| `--bump <type>` | Skip interactive bump choice. Values: `prerelease`, `release`, `major`, `minor`, `patch` |
| `--no-tag` | Skip git tag creation |
| `--no-gh` | Skip GitHub release draft |

When `--bump` is provided, skip `askChoice`. Non-TTY already defaults bump choice and skips the editor. Combined with `--no-tag --no-gh`, this gives the skill a clean version+changelog+build+commit pipeline without release artifacts.

**What stays the same**: All library code (`version.ts`, `changelog.ts`, `commits.ts`), the full interactive flow (no flags), `--dry-run`.

### Part 2: Create `/release` skill

A user-invocable skill (`/loaf:release`) that orchestrates the full merge ritual in 7 steps:

1. **Detect context** — Current branch, associated PR (auto-detect via `gh pr view` or accept argument)
2. **Pre-flight checks** (BLOCKING) — `npm run typecheck`, `npm run test`, `npx loaf build`
3. **Documentation freshness** — Check README.md, ARCHITECTURE.md, docs/ for stale references using branch diff
4. **Housekeeping verification** — Spec archived, tasks archived, session updated (verify only, offer to fix)
5. **Version bump + changelog** — Call `loaf release --bump <type> --no-tag --no-gh` on the feature branch
6. **Squash merge** — Draft clean body, confirm with user, execute `gh pr merge --squash`
7. **Post-merge cleanup** — Pull main, delete branch, suggest `/reflect`

**Separation of concerns:**
- CLI handles: version detection, semver calculation, changelog generation/insertion, version file updates, build, commit
- Skill handles: pre-flight, docs freshness, housekeeping, merge orchestration, cleanup

## Scope

### In Scope

- Add `--bump`, `--no-tag`, `--no-gh` flags to `loaf release`
- Create `content/skills/release/SKILL.md` with 7-step orchestration prompt
- Create `content/skills/release/SKILL.claude-code.yaml` sidecar
- Update cross-references in implement, git-workflow, and post-merge instructions
- Fix missing `[Unreleased]` section in CHANGELOG.md

### Out of Scope

- Replacing `loaf release` CLI — it stays for terminal-only workflows
- Modifying existing hooks — they stay as safety nets for manual merges
- Changing the semver library code
- Non-Claude Code targets (the skill is Claude Code-only; CLI flags benefit everyone)

### Rabbit Holes

- Don't build a "smart" diff analyzer for documentation freshness — just flag files that changed and let the user review
- Don't try to auto-generate the squash body — draft it, but the user must review
- Don't add `--yes` auto-confirm flag — force the user to confirm in the terminal too (the skill handles confirmation conversationally)

## Dependencies

| Dependency | Type | Notes |
|---|---|---|
| SPEC-014 | Hard (resolved) | Profile model and skill conventions are in place |

## Test Conditions

- [ ] `loaf release --bump prerelease --no-tag --no-gh --dry-run` shows correct preview without tag/gh steps
- [ ] `loaf release --bump prerelease --no-tag --no-gh` executes version+changelog+build+commit only (no tag, no GH release)
- [ ] `loaf release --dry-run` (no new flags) behaves identically to before
- [ ] `npx loaf build` produces `plugins/loaf/skills/release/SKILL.md` in output
- [ ] `/loaf:release` appears in Claude Code skill list
- [ ] CHANGELOG.md has `[Unreleased]` section after version bump
