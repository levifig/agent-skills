---
change: spec-conversion-and-guidance-sweep
id: TASK-004
title: Init, install, and hooks convergence
blocked-by:
  - TASK-002
blocks:
  - TASK-005
  - TASK-006
---

# TASK-004 — Init, install, and hooks convergence

## Objective

Nothing Loaf writes into a project or harness advertises the retired surface: init scaffolds no legacy directories, the fenced block and hook catalog are converged, and the breakdown skill is retired on installed harnesses through the deprecation machinery.

## Scope boundaries

**In:** `init.go` scaffolding (`.agents/specs`, `.agents/tasks`); `install_fenced.go` block text; `install_target.go` recognized-hook allowlist (`loaf task refresh`); `config/hooks.yaml` — `generate-task-board` removed, `ephemeral-provenance` description converged; `content/hooks/instructions/post-merge.md` checklist; `install_deprecations.go` entry quarantining the breakdown skill on upgrade; associated tests (`install_target_test.go`, `hook_catalog_test.go`, `install_deprecations_report_test.go`).

**Out:** Skill body rewrites (TASK-005); public docs (TASK-006).

## Context pointers

- Contract: `shape.md` — Observable Workflow, Planning Contract "Risks" (installed-harness staleness)
- Inventory anchors: `init.go:45-46`, `install_fenced.go:306-311`, `install_target.go:33`, `config/hooks.yaml:147-153`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — init/install/hooks convergence"
```

## Steps

- [ ] Init scaffolds no legacy directories
- [ ] Fenced block drops `loaf task/spec` (kb stays); existing installs converge on next upgrade
- [ ] `generate-task-board` hook removed from hooks.yaml, catalogs, and built hook manifests
- [ ] Breakdown deprecation entry: upgrade quarantines the installed skill
- [ ] `post-merge.md` instruction rewritten for the Change flow

## Verification

- `go test ./internal/cli -run 'TestInstall|TestHook' -v` green
- A fresh `loaf init` in a fixture creates no `.agents/specs` or `.agents/tasks`
