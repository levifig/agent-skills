---
id: TASK-046
title: Pre-PR workflow hook + CHANGELOG.md seed
spec: SPEC-015
status: done
priority: P1
created: '2026-03-27T16:27:29.567Z'
updated: '2026-03-27T16:40:23.792Z'
depends_on:
  - TASK-045
completed_at: '2026-03-27T16:40:23.792Z'
---

# TASK-046: Pre-PR workflow hook + CHANGELOG.md seed

## Description

Create the conditional-blocking pre-PR hook — the most valuable hook in SPEC-015. Three files + seed CHANGELOG:

1. **`content/hooks/pre-tool/workflow-pre-pr.sh`** — Bash script that:
   - Reads JSON stdin via `parse_command` from the hook library
   - Matches commands containing `gh pr create`
   - Checks if `CHANGELOG.md` exists and has entries under `[Unreleased]`
   - **If missing/empty:** Blocks (exit 2), outputs `pre-pr-checklist.md` to stderr
   - **If entries exist:** Passes (exit 0), outputs `pre-pr-format.md` to stdout

2. **`content/hooks/instructions/pre-pr-checklist.md`** — Full blocking checklist:
   - Add CHANGELOG entry under `[Unreleased]` (categorized: Added/Changed/Fixed/etc.)
   - Verify PR title format (conventional commit, <70 chars)
   - Verify PR body format (Summary + Test Plan)
   - If CHANGELOG.md doesn't exist: create with Keep a Changelog header first
   - Source: relevant sections from `foundations/references/commits.md`

3. **`content/hooks/instructions/pre-pr-format.md`** — Lightweight pass-through reminder:
   - PR title format
   - PR body format
   - Merge strategy reminder (squash merge, clean description)

4. **`CHANGELOG.md`** — Seed file at project root with Keep a Changelog header and `[Unreleased]` section populated with entries for work shipped so far (SPEC-008 through SPEC-013).

## Acceptance Criteria

- [ ] Hook script only fires when Bash command contains `gh pr create`
- [ ] Hook exits silently (exit 0, no output) for non-matching commands
- [ ] When CHANGELOG.md missing: blocks (exit 2) with full checklist to stderr
- [ ] When `[Unreleased]` empty (no `### ` category headers): blocks (exit 2)
- [ ] When `[Unreleased]` has entries: passes (exit 0) with format reminder to stdout
- [ ] CHANGELOG.md follows Keep a Changelog format with retroactive entries
- [ ] Instructions directory `content/hooks/instructions/` created
- [ ] Instruction markdown files are concise (not verbose — minimize context waste)

## Context

See SPEC-015 § "Pre-PR Hook" for the conditional blocking logic and instruction content. The `blocking: true` flag in hooks.yaml (TASK-049) is required for exit 2 to work — without it, exit 2 is treated as exit 0.
