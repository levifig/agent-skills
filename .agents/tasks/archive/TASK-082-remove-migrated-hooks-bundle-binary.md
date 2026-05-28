---
id: TASK-082
title: 'Remove migrated hooks, shell libs, session skills + bundle binary'
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
completed_at: '2026-04-04T16:41:22.296Z'
---

# TASK-082: Remove migrated hooks, shell libs, session skills + bundle binary

Cleanup task: remove artifacts that have been replaced by `loaf check`, skill instructions, and `loaf session`.

## Implementation

### Removed ~20 hook scripts
Deleted migrated hook scripts from `content/hooks/pre-tool/` and `content/hooks/post-tool/`:
- All language hooks (python-*, typescript-*, rails-*)
- All infra hooks (k8s, dockerfile, terraform)
- All design hooks (a11y, tokens)
- format-check, tdd-advisory, validate-changelog, changelog-reminder

### Removed 4 shared shell libraries
Deleted from `content/hooks/lib/`:
- `json-parser.sh`
- `config-reader.sh`
- `agent-detector.sh`
- `timeout-manager.sh`

### Removed session skills
Deleted skill directories:
- `content/skills/resume-session/` (absorbed by `loaf session start`)
- `content/skills/reference-session/` (absorbed by archived journal reads)

### Bundled CLI binary
Plugin output includes self-contained binary:
- `plugins/loaf/bin/loaf` (self-contained, not a wrapper script)
- Binary is generated during `loaf build` via CLI bundling

## Verification

- [x] Deleted hook scripts no longer exist
- [x] Shared shell libraries deleted
- [x] `resume-session` and `reference-session` directories deleted
- [x] Remaining hooks functional (no broken library deps)
- [x] `plugins/loaf/bin/loaf` exists after build
- [x] `loaf build` succeeds for all targets
- [x] `npm run typecheck` passes
- [x] All 75 tests pass
