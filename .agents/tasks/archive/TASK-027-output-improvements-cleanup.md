---
id: TASK-027
title: Output improvements + final cleanup
spec: SPEC-008
status: done
priority: P3
created: '2026-03-16T16:27:15.467Z'
updated: '2026-03-16T16:27:15.467Z'
depends_on:
  - TASK-025
  - TASK-026
files:
  - cli/lib/build/targets/
  - dist/
  - CLAUDE.md
  - .claude/CLAUDE.md
  - .agents/ARCHITECTURE.md
verify: loaf build && loaf install --to all && loaf --help
done: >-
  dist/ is cleaner, old directories removed, all documentation updated, all spec
  test conditions pass
completed_at: '2026-03-16T16:27:15.467Z'
---

# TASK-027: Output improvements + final cleanup

## Description

Final polish: improve dist/ output structure, remove old source directories, update all documentation to reflect the new project structure.

## Output improvements

- Analyze dist/ for duplication across targets
- Reduce redundant skill copies where target formats allow
- Simpler, flatter dist/ structure within target constraints
- Clear separation of what each target receives
- Clean, minimal build output

## Cleanup

- Remove old `src/` directory (content already in `content/`)
- Remove old `build/` directory (logic already in `cli/lib/build/`)
- Verify no references to old paths remain

## Documentation updates

- Update root CLAUDE.md for new project structure
- Update .claude/CLAUDE.md for new project structure
- Update .agents/ARCHITECTURE.md
- Update docs/knowledge/ files that reference old paths
- Update README.md

## Final verification

Run through all SPEC-008 test conditions:
- [ ] `loaf --help` shows version and available commands
- [ ] `loaf --version` shows current version
- [ ] `loaf build` builds all 5 targets
- [ ] `loaf build --target claude-code` builds only Claude Code
- [ ] `loaf build --target nonexistent` shows helpful error
- [ ] Colored build output with timing
- [ ] `loaf install --to cursor` works
- [ ] `loaf install --to all` works
- [ ] Interactive install selection works
- [ ] `loaf install --upgrade` works
- [ ] Dev mode detection works
- [ ] `npm link` works
- [ ] Source in `cli/` and `content/`
- [ ] Old `build/` and `src/` removed
- [ ] No npm build scripts remain
- [ ] TypeScript compiles clean
- [ ] package.json publish-ready
- [ ] `loaf` with no subcommand shows help
- [ ] dist/ output functionally equivalent
- [ ] install.sh is thin bootstrap

## Context

See SPEC-008 for full context. Depends on all previous tasks.
Circuit breaker stage: 100%.

## Work Log

<!-- Updated by session as work progresses -->
