---
id: TASK-135
title: Add interactive soul selection prompt to loaf install
spec: SPEC-033
status: done
priority: P2
created: '2026-04-28T22:39:12.730Z'
updated: '2026-04-28T23:25:36.461Z'
depends_on:
  - TASK-130
completed_at: '2026-04-29T00:25:00.000Z'
---

# TASK-135: Add interactive soul selection prompt to loaf install

## Description

Add a soul-selection step to `loaf install`'s interactive flow. When TTY is detected (or `--interactive` is passed), prompt the user with the catalog listing (from `loaf soul list`) and let them pick. Selection persists to `loaf.json` via the existing config writer.

- Non-interactive (CI, `--yes`, no TTY): default to `none` without prompting.
- Bootstrap skill calls `loaf install` directly — the prompt surfaces naturally; no separate bootstrap-level soul prompt needed.

Droppable per the spec's priority order: tracks 1+2 deliver the mechanism; this task is UX polish. Users can always run `loaf soul use <name>` post-install.

## Acceptance Criteria

- [x] Fresh `loaf install --interactive` (or TTY-detected interactive) prompts for soul choice with catalog listing
- [x] User selection persists to `loaf.json` (`soul:` field)
- [x] Non-interactive installs (CI, `--yes`, no TTY) default to `none` without prompt
- [x] Bootstrap skill (calls `loaf install`) inherits the prompt without separate logic
- [x] Unit tests cover prompt logic; manual smoke test of interactive flow in tmpdir

## Verification

```bash
npm run build:cli && \
npm run typecheck && \
npm run test -- install
# Plus manual: cd $(mktemp -d) && loaf install --interactive
```

## Context

See SPEC-033 test condition T14. Track 3 — droppable if Tracks 1+2 deliver value alone. Depends on TASK-130 (souls library exposes catalog listing for the prompt).
