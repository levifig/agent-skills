---
id: TASK-133
title: Update SessionStart hook to restore .agents/SOUL.md from configured soul
spec: SPEC-033
status: done
priority: P1
created: '2026-04-28T22:39:12.619Z'
updated: '2026-04-28T23:42:47.792Z'
depends_on:
  - TASK-131
  - TASK-132
completed_at: '2026-04-28T23:42:47.792Z'
---

# TASK-133: Update SessionStart hook to restore .agents/SOUL.md from configured soul

## Description

The SessionStart hook (or `loaf session start` if the hook delegates) currently restores `.agents/SOUL.md` from the canonical template `content/templates/soul.md` when missing. Generalize this to read the active soul from `loaf.json` and restore from `content/souls/{soul}/SOUL.md` instead.

- Missing `soul:` field defaults to `fellowship` (legacy preservation, matches TASK-131's migration default).
- Present `.agents/SOUL.md` is never touched.
- Restoration uses the souls library from TASK-130.

Once this ships, the standalone `content/templates/soul.md` becomes redundant (the catalog is canonical). Removing the old template is in-scope for this task — the build process should source soul content exclusively from the catalog.

## Acceptance Criteria

- [ ] SessionStart with missing `.agents/SOUL.md` and `soul: none` restores from `content/souls/none/SOUL.md`
- [ ] SessionStart with missing `.agents/SOUL.md` and `soul: fellowship` restores from `content/souls/fellowship/SOUL.md`
- [ ] SessionStart with missing `.agents/SOUL.md` and absent `soul:` field defaults to `fellowship` (legacy)
- [ ] SessionStart with present `.agents/SOUL.md` does not touch it
- [ ] `content/templates/soul.md` is removed (catalog is now canonical) or, if retained for any build-time reason, is no longer the restoration source
- [ ] Tests cover the restoration paths above

## Verification

```bash
npm run build:cli && \
npm run typecheck && \
npm run test -- session && \
loaf build
```

## Context

See SPEC-033 test condition T10. Depends on TASK-131 (`soul:` field in schema) and TASK-132 (profiles read SOUL.md, so restoration must work for them on first session). The deprecation of `content/templates/soul.md` is a small mechanical follow-on — flag it as a discovery in the journal if any reference still points to it after the build passes.
