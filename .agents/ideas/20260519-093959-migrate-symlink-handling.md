---
title: "Pin symlink handling in loaf migrate worktree-storage"
captured: 2026-05-19T09:39:59Z
status: raw
tags: [migrate, symlinks, worktree-storage, follow-up, spec-036]
related: [SPEC-036, TASK-170]
---

# Pin symlink handling in `loaf migrate worktree-storage`

## Nugget

`cli/lib/migrate/worktree-storage.ts` currently has **three different behaviors** for symlinks encountered during migration, depending on which code path executes:

1. **Same-filesystem move** — `renameSync` moves the symlink itself (link, not target). This preserves intent.
2. **EXDEV cross-filesystem fallback** — `fs.cpSync` (post-TASK-170-polish) dereferences symlinks by default unless `verbatimSymlinks: true` is set. Result: copies the target's content to the destination as a regular file; the symlink is gone.
3. **Conflict policy `case "newer":`** — `statSync` follows symlinks to compare the *target's* mtime, not the link's. Determines the "newer wins" result against the wrong inode.

This asymmetry is documented as a code comment in `worktree-storage.ts` (added during TASK-170 polish), but the underlying behavior is not pinned to an explicit design decision.

## Problem/Opportunity

`.agents/` artifacts are usually plain files. Symlinks inside `.agents/` are rare today — but they're plausible for: developer-managed local shortcuts (`.agents/sessions/current → ...`), tool-managed pointers (some session/state systems), and accidental `ln -s` mishaps. The behavior on any of these is currently undefined and may differ from the user's intent.

Three viable designs to choose between:

1. **Preserve symlinks verbatim everywhere.** Use `lstatSync` for the conflict check, `fs.cpSync` with `verbatimSymlinks: true` in the EXDEV path, treat the symlink as the migration unit. Cleanest semantically; matches `renameSync`'s default behavior.
2. **Dereference symlinks everywhere.** Use `statSync` (current) for conflict, `fs.cpSync` default (dereference) for EXDEV, plus explicitly resolve and copy targets in the same-FS path too. Loses link identity but is "what you migrated is what's there." Probably what most users would expect intuitively.
3. **Refuse symlinks.** Detect them via `lstatSync` and error out with a clear message ("symlink at `<path>` — manual handling required, see docs"). Forces an explicit user decision; safest under uncertainty.

## Initial Context

- Surfaced by the TASK-170 review (Q21) on 2026-05-19. Reviewer noted: "Three behaviors for a symlink depending on FS layout. Right now the behavior is unspecified."
- TASK-170 shipped with a code comment marking the gap and pointing here. **Do not extend symlink-touching logic in `worktree-storage.ts` without resolving this idea first.**
- Test coverage today: no test seeds a symlink fixture. The TASK-170 tests use plain files only.
- Choice between (1) / (2) / (3) likely depends on a quick survey of: what tools/skills create symlinks inside `.agents/` in practice (probably none), and whether the migration is expected to be lossless or normalizing.
- Light-weight resolution path: if `.agents/` is empirically symlink-free in the wild, option (3) (refuse + manual handling) is the cheapest and safest until a real use case appears.

---

*Captured via /loaf:idea — shape with /loaf:shape when ready*
