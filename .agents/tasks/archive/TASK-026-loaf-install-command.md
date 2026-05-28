---
id: TASK-026
title: '`loaf install` command + tool detection'
spec: SPEC-008
status: done
priority: P2
created: '2026-03-16T16:27:15.466Z'
updated: '2026-03-16T16:27:15.466Z'
depends_on:
  - TASK-021
files:
  - cli/lib/detect/tools.ts
  - cli/lib/install/installer.ts
  - cli/commands/install.ts
  - install.sh
verify: loaf install --to cursor && loaf install --upgrade
done: >-
  `loaf install` replaces install.sh for all target installations. install.sh is
  a thin bootstrap.
completed_at: '2026-03-16T16:27:15.466Z'
---

# TASK-026: `loaf install` command + tool detection

## Description

Port install.sh logic into the TypeScript CLI as `loaf install`. Create a reusable tool detection module and target-specific installers. Reduce install.sh to a thin bootstrap wrapper.

## Two components

### 1. Tool detection module (`cli/lib/detect/`)

Port from install.sh:
- macOS PATH normalization (path_helper, homebrew, npm prefix)
- Detect Claude Code (cli), Cursor (cli/app/config), OpenCode (config dir), Codex (cli/config), Gemini (cli/config)
- Config directory resolution per target
- Dev mode detection (running from cloned repo)
- Loaf marker file handling (`.loaf-version`)

### 2. Install command (`cli/commands/install.ts`)

Port from install.sh:
- `loaf install --to <target>` — install specific target
- `loaf install --to all` — auto-detect and install all
- `loaf install` (no flags) — interactive target selection
- `loaf install --upgrade` — update only already-installed targets
- Target-specific installers (OpenCode, Cursor, Codex, Gemini dirs)
- Colored output, status reporting
- Dev mode: special Claude Code instructions

### 3. Bootstrap install.sh

Reduce to: curl-pipe → npm install -g loaf → loaf install

## Acceptance Criteria

- [ ] `loaf install --to cursor` installs to Cursor config directory
- [ ] `loaf install --to all` detects and installs to all available targets
- [ ] `loaf install` (no flags) shows interactive target selection
- [ ] `loaf install --upgrade` updates only already-installed targets
- [ ] Dev mode detected when running from the repo
- [ ] Claude Code shows marketplace instructions (not installed via loaf install)
- [ ] macOS PATH normalization works correctly
- [ ] install.sh reduced to thin bootstrap
- [ ] All 5 targets installable (matching current install.sh behavior)

## Implementation Notes

- install.sh has careful PATH normalization for macOS — port faithfully, don't simplify
- Interactive selection uses terminal escape codes — consider Node.js prompts library or hand-roll
- rsync vs cp fallback logic — port as-is
- Dev mode detection: check if running from git repo with package.json + src/skills/
- Claude Code uses marketplace (`/plugin marketplace add`), not file copying

## Context

See SPEC-008 for full context. Depends on TASK-021 (CLI skeleton). Can run in parallel with TASK-024/025 (no dependency on TypeScript conversion).
Circuit breaker stage: 75%. Completing this task = third circuit breaker milestone.

## Work Log

<!-- Updated by session as work progresses -->
