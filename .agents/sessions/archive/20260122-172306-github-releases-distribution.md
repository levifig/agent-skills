---
session:
  title: "GitHub Releases Distribution Migration"
  status: abandoned
  created: "2026-01-22T17:23:00Z"
  last_updated: "2026-01-22T17:23:00Z"
  archived_at: "2026-03-27T18:30:00Z"
  archived_by: "agent-pm"
---

# Session: GitHub Releases Distribution Migration

**Created**: 2026-01-22 17:23
**Status**: Abandoned — never progressed past planning

## Objective

Migrate from committing build outputs (`dist/`, `plugins/`, `.claude-plugin/`) to main branch to using GitHub Releases for distribution.

## Problem Statement

Current CI workflow commits build outputs to main after every push, causing:
- Local/remote divergence during development
- Constant need for `git pull --rebase`
- Polluted commit history

## Proposed Solution

Use GitHub Releases for distribution:
1. Build outputs not tracked in main (added to `.gitignore`)
2. CI validates build on push to main (no commit)
3. CI creates GitHub Release on tag push (e.g., `v*`)
4. Installer downloads from latest GitHub Release
5. Local dev workflow unchanged

## Critical Issue Identified

**Claude Code Marketplace Constraint**: Claude Code expects plugins at repo root when using:
```
/plugin marketplace add levifig/agent-skills
```

The marketplace.json references paths like `./plugins/orchestration` which must exist at the repository root in the **main branch** for Claude Code to fetch them.

**Problem**: If we move `plugins/` and `.claude-plugin/` to GitHub Releases only, the marketplace path breaks.

## Options Analysis

### Option 1: Keep Claude Code at Root, Release Others
- Keep `plugins/` and `.claude-plugin/` committed to main
- Move only `dist/` to releases
- **Pros**: Claude Code works unchanged, partial improvement
- **Cons**: Still commits to main (smaller payoff), split distribution model

### Option 2: GitHub Releases with Source References
- Use releases for all distributions
- Change marketplace.json to reference source files directly
- **Investigation needed**: Does Claude Code support building from source? Can marketplace.json reference `src/` paths?
- **Pros**: Clean main branch
- **Cons**: Unknown if Claude Code supports this

### Option 3: Git Tags Only (No Releases)
- Tag versions but don't use GitHub Releases
- Keep everything committed to main
- Installer checks out specific tags
- **Pros**: Simple, known to work
- **Cons**: Doesn't solve commit pollution

### Option 4: Separate Claude Code Repository
- Create `agent-skills-claude` repo with only built plugins
- CI pushes built plugins there on tag
- Main repo stays clean
- **Pros**: Clean separation, each tool optimized
- **Cons**: Maintenance overhead, split documentation

### Option 5: Branch-Based Distribution
- Keep main clean (no build outputs)
- CI pushes build outputs to `dist` branch on tag
- Claude Code marketplace points to `dist` branch
- Installer downloads from `dist` branch
- **Pros**: Clean main, single repo, known Git patterns
- **Cons**: GitHub UI shows both branches, slight complexity

## Recommended Approach

**Option 5: Branch-Based Distribution** appears most pragmatic:

1. `.gitignore`: Add `dist/`, `plugins/`, `.claude-plugin/`
2. CI on push to main: Validate build only
3. CI on tag (v*):
   - Build all targets
   - Force push to `dist` branch (or create if not exists)
   - Tag the `dist` branch with same tag
4. Claude Code marketplace: Reference `dist` branch
   - User runs: `/plugin marketplace add levifig/agent-skills@dist`
5. Installer: Downloads from `dist` branch (latest tag or HEAD)
6. Local dev: Unchanged (builds locally)

## Questions for User

1. Does Option 5 (branch-based) align with your goals?
2. Are you okay with users needing to specify `@dist` for Claude Code?
3. Alternative: Investigate if Claude Code supports source-based builds (Option 2)?
4. Would you prefer Option 4 (separate repo) for complete isolation?

## Current State

- Analyzed existing build system and CI workflow
- Identified Claude Code marketplace constraint
- Session created, awaiting user direction

## Next Steps

Pending user decision on approach:
- [ ] Update `.gitignore`
- [ ] Modify CI workflow
- [ ] Update installer script
- [ ] Update documentation
- [ ] Test with each target
- [ ] Create migration guide

## Files Analyzed

- `.github/workflows/build.yml` - Current CI workflow
- `.gitignore` - Current exclusions
- `install.sh` - Installation logic
- `.claude-plugin/marketplace.json` - Marketplace manifest
- `build/targets/claude-code.js` - Claude Code build logic
- `AGENTS.md` - Distribution documentation

## Notes

- Build system generates to repo root for Claude Code by design (line 49, claude-code.js)
- Marketplace uses relative paths (`./plugins/...`)
- Installer already has dev vs remote detection
- Other targets (OpenCode, Cursor, Copilot, Codex) don't have marketplace constraints
