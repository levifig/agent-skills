---
id: TASK-139
title: >-
  Preserve curated [Unreleased] entries when present (no auto-generation
  override)
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:27:51.986Z'
updated: '2026-04-29T19:02:03.536Z'
completed_at: '2026-04-29T19:02:03.535Z'
---

# TASK-139: Preserve curated [Unreleased] entries when present (no auto-generation override)

## Description

When `[Unreleased]` already contains list items at invocation time, `loaf release` must move those exact items verbatim under the new `## [X.Y.Z]` header instead of overwriting them with auto-generated commit-subject jargon. Auto-generation runs only when `[Unreleased]` is empty. Lives in `cli/commands/release.ts`. Implements SPEC-031 Task 3.

## Acceptance Criteria

- [ ] When `[Unreleased]` contains list items, those exact items appear under the new `## [X.Y.Z]` header after release, byte-for-byte preserved.
- [ ] When `[Unreleased]` is empty (or contains only the stub), the existing auto-generation from commit subjects runs as today.
- [ ] No commit-subject jargon is interleaved with curated entries when both could apply.
- [ ] Fixture test covers a curated `[Unreleased]` with multiple list items and asserts they appear unchanged under the new version header.
- [ ] Stub re-insertion (TASK-138) still applies on both paths after this change.

## Verification

```bash
npm run typecheck && npm run test -- release
```
