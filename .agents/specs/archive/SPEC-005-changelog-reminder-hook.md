---
id: SPEC-005
title: Changelog reminder hook after git commits
source: direct
created: '2026-01-24T22:25:35.000Z'
status: complete
---

# SPEC-005: Changelog reminder hook after git commits

## Problem Statement

When work is completed and committed, there's no automatic reminder to update the changelog. This leads to:
- Forgotten changelog entries
- Batched updates that lose context
- Inconsistent documentation of changes

Developers should be nudged to keep the changelog current at the natural moment when work is fresh in mind—right after committing.

## Strategic Alignment

- **Vision:** Supports Loaf's goal of disciplined, quality-focused development workflows
- **Personas:** Developers benefit from consistent reminders without blocking their flow
- **Architecture:** Fits existing hook infrastructure; complements the existing `foundations-validate-changelog.sh` pre-tool hook

## Solution Direction

Create a **post-tool hook** that triggers after successful `git commit` commands. The hook outputs a reminder to Claude suggesting the changelog be updated, following Keep-a-Changelog format with an `## [Unreleased]` section.

The hook is **informational only**—it doesn't block commits or automatically modify files. Claude receives the reminder and decides:
1. Whether this commit warrants a changelog entry
2. Which category (Added/Changed/Fixed/Removed/Deprecated/Security) is appropriate
3. How to phrase the user-focused entry

## Scope

### In Scope
- Post-tool hook script triggered after `git commit` succeeds
- Output includes:
  - Reminder text about updating CHANGELOG.md
  - Keep-a-Changelog format guidance
  - Reference to the commit just made
- Hook registration in `hooks.yaml` under `foundations` skill
- Works with existing `foundations-validate-changelog.sh` (validation on writes)

### Out of Scope
- Automatic changelog modification
- Blocking commits that lack changelog entries
- Parsing commit messages to auto-generate entries
- Detecting/excluding certain commit types
- Creating CHANGELOG.md if it doesn't exist
- Supporting formats other than Keep-a-Changelog

### Rabbit Holes
- **Auto-categorization from commit prefixes** — Tempting but adds complexity; Claude is better at understanding context
- **Diff analysis** — Significant effort for marginal benefit when Claude can already see the commit
- **Commit message parsing** — The commit message alone may not capture user-facing impact

### No-Gos
- Don't make this blocking — friction should be minimal
- Don't auto-edit the changelog — Claude should make the judgment call
- Don't duplicate validation logic — existing pre-tool hook handles format

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Reminder fatigue | Medium | Low | Keep output concise; Claude can skip when appropriate |
| Duplicate reminders on multi-commit workflows | Low | Low | Hook fires per-commit, which is intentional |

## Test Conditions

- [ ] Hook fires after successful `git commit` (via Bash tool)
- [ ] Hook does NOT fire for non-commit bash commands
- [ ] Output includes reminder text with Keep-a-Changelog guidance
- [ ] Output references the commit just made (or at least acknowledges a commit happened)
- [ ] Hook is registered in `hooks.yaml` under `foundations` skill
- [ ] Hook is non-blocking (exit 0)

## Circuit Breaker

At 50% appetite (half a day): If hook implementation proves complex, simplify to basic text output without commit detection.

At 75% appetite (1+ day): Ship whatever works; refinements can be follow-up work.
