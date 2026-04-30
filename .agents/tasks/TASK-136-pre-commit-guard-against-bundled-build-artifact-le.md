---
id: TASK-136
title: Pre-commit guard against bundled build-artifact leakage in unrelated commits
status: todo
priority: P2
created: '2026-04-29T15:00:01.123Z'
updated: '2026-04-29T15:00:01.123Z'
---

# TASK-136: Pre-commit guard against bundled build-artifact leakage in unrelated commits

## Description

Bundled build-artifact files (lockfiles like `package-lock.json`, build outputs like `plugins/loaf/bin/loaf`, generated targets in `dist/`) periodically leak into commits whose primary scope is unrelated to the build. Documented recurrences:

- dev.31: lockfile leaked into PR #36 (required follow-up split — see TASK-123)
- dev.31: `plugins/loaf/bin/loaf` leaked into commit `13ce968d` (required follow-up split — see TASK-126)
- dev.32: pattern recurred (see STRATEGY.md:65-66)

Each occurrence requires a follow-up commit to revert the leak from the unrelated commit and land it in a dedicated build/lockfile-bump commit. Codex review caught both dev.31 instances, but the cost is real (extra commits, extra review cycles, polluted history).

The fix surface is a pre-commit hook (likely a new entry in `config/hooks.yaml` or a refinement of an existing hook) that detects when a commit's *non-build* scope (subject prefix `feat:`, `fix:`, `docs:`, etc.) includes paths that are clearly build outputs (lockfiles, `plugins/`, `dist/`, `node_modules/`). Block-with-explanation; user splits or overrides explicitly.

Out of scope for SPEC-031 (release flow hardening) — different problem domain (commit hygiene, not release flow). Punted from SPEC-031 for separate shaping.

## Acceptance Criteria

- [ ] Pre-commit hook detects build-output paths in commits whose subject does not match build-scope prefixes (`chore: release`, `chore: build`, `chore: deps`, etc.)
- [ ] Hook output names the leaked file(s) and suggests the split (move to a dedicated `chore: build` commit)
- [ ] Hook is overridable when the leak is intentional (e.g., adding a new build script that touches `plugins/`)
- [ ] Existing release-flow commits (`chore: release v…`) are exempt — they intentionally bundle build artifacts
- [ ] Test fixtures cover: clean non-build commit (passes), feat commit with lockfile leak (blocks), explicit `chore: build` commit with lockfile (passes), `chore: release` commit with `plugins/loaf/bin/loaf` (passes)

## Verification

```bash
# Replay both dev.31 leak patterns against the hook in test mode
loaf check --hook detect-artifact-leak --fixture lockfile-in-feat
loaf check --hook detect-artifact-leak --fixture plugins-bin-in-fix
# Both must block with the suggested split
```
