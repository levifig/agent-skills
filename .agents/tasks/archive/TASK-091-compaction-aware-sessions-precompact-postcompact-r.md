---
id: TASK-091
title: 'Compaction-aware sessions — PreCompact, PostCompact, retire archive-context'
spec: SPEC-027
status: done
priority: P2
created: '2026-04-07T10:11:01.349Z'
updated: '2026-04-07T10:33:17.300Z'
completed_at: '2026-04-07T10:33:17.300Z'
---

# TASK-091: Compaction-aware sessions — PreCompact, PostCompact, retire archive-context

## Description

Implement SPEC-027 Part C: make sessions compaction-aware. The session journal already captures decisions, discoveries, and progress — connect it to the compaction lifecycle so the journal serves as the resumption protocol.

**PreCompact:** Strengthen the nudge to require both journal flush AND a condensed state summary written to the session file's `## Current State` section. Frame journaling as compaction insurance.

**PostCompact:** Add a new PostCompact prompt nudge that tells the model to re-read the session file for resumption context. The state summary written pre-compaction becomes the resumption prompt.

**Cleanup:** Retire `archive-context.sh` (references stale `.work/` paths, broken). Remove `.context-snapshots/` mechanism.

Depends on TASK-089 (`claude_session_id` enables same-session detection post-compaction).

## Key Files

- `config/hooks.yaml` — add PostCompact hook (~line 209)
- `content/hooks/session/compact.sh` — may need updates
- `content/hooks/session/archive-context.sh` — DELETE
- `content/skills/orchestration/references/context-management.md` — update compaction guidance

## Acceptance Criteria

- [ ] PreCompact nudge requires journal flush + state summary to `## Current State`
- [ ] PostCompact prompt nudge exists in hooks.yaml
- [ ] PostCompact nudge tells model to re-read session file for resumption
- [ ] `archive-context.sh` removed
- [ ] No references to `.context-snapshots/` in active hooks
- [ ] `compact(session)` marker still appears in journal after compaction
- [ ] `context-management.md` updated with journal-as-resumption model
- [ ] `loaf build` succeeds

## Verification

```bash
loaf build && ! test -f content/hooks/session/archive-context.sh
```

## Context

See SPEC-027 Part C. The journal IS the external memory — compaction can be lossy because all important state is already on disk.
