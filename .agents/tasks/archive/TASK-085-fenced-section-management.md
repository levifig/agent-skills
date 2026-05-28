---
id: TASK-085
title: Fenced-section management for CLAUDE.md/AGENTS.md
spec: SPEC-020
status: done
priority: P2
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
completed_at: '2026-04-04T16:41:22.296Z'
---

# TASK-085: Fenced-section management for CLAUDE.md/AGENTS.md

Install and upgrade Loaf framework conventions into user project instruction files.

## Implementation

### New Module: `cli/lib/install/fenced-section.ts`

Created comprehensive fenced-section management with:
- `installFencedSection(targetFile, upgrade)` - Install or upgrade fenced section
- `getTargetFile(target, projectRoot)` - Get correct path per target
- `getFencedVersion(targetFile)` - Check existing fence version
- `installFencedSectionsForTargets(targets, projectRoot, upgrade)` - Batch install

### Updated: `cli/commands/install.ts`

Integrated fenced-section installation:
- After tool content installation, installs fenced section to project files
- Respects `--upgrade` flag for version-aware updates
- Reports status per target (created/appended/updated/skipped/error)

### Test Coverage: `cli/lib/install/fenced-section.test.ts`

23 tests covering:
- Creating new files
- Appending to existing files
- Updating existing fenced sections
- Skipping when version matches in upgrade mode
- Preserving user content before and after fences
- Version parsing (including prerelease versions)
- Per-target file resolution (including Cursor's multiple options)
- Batch installation to multiple targets
- Content format validation

## Verification

- [x] `loaf install` creates fenced section in target file
- [x] `loaf install --upgrade` replaces only between fences
- [x] User content outside fences untouched
- [x] Version marker works (skip if current, refresh if outdated)
- [x] Missing fences = append new section
- [x] Fenced content is compact (~20-30 lines)

### Test Results
- Type check: ✓ Pass
- Build: ✓ Pass
- Unit tests: ✓ 23/23 pass
- Full test suite: ✓ 440/440 pass
- Manual verification: ✓ Working as expected
