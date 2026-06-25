---
id: TASK-255
title: Remove Gemini from in-repo build and install surfaces
spec: SPEC-047
status: done
priority: P1
created: '2026-06-24T12:03:41Z'
updated: '2026-06-24T12:25:48Z'
completed_at: '2026-06-24T12:25:48Z'
depends_on:
  - TASK-254
files:
  - config/targets.yaml
  - internal/cli/build.go
  - internal/cli/build_test.go
  - internal/cli/install_fenced.go
  - internal/cli/install_mcp.go
  - dist/gemini/
  - .agents/tasks/TASK-255-remove-gemini-in-repo-build-install-surfaces.md
verify: >-
  ! rg -n 'gemini|Gemini' config/targets.yaml internal/cli/build.go
  internal/cli/build_test.go internal/cli/install.go internal/cli/install_target.go
  internal/cli/install_target_test.go internal/cli/install_fenced.go
  internal/cli/install_mcp.go internal/cli/install_mcp_test.go
  internal/cli/install_command_test.go internal/cli/install_symlink.go
  internal/cli/version.go package.json README.md docs/ARCHITECTURE.md
  docs/knowledge/build-system.md docs/knowledge/glossary.md .agents/AGENTS.md &&
  test ! -e dist/gemini && npm run build && npm run test
done: >-
  Gemini is absent from source build/install wiring and generated in-repo output
  while user-side cleanup remains deferred to SPEC-053.
---

# TASK-255: Remove Gemini from in-repo build and install surfaces

## Description

Shrink the first-class harness matrix from six targets to five by removing Gemini
from repo build output, build wiring, install wiring, generated artifacts, and
tests.

This task must not remove already-installed user Gemini files. User-side cleanup
is a breaking migration action and remains gated on SPEC-053.

## Acceptance Criteria

- [x] `config/targets.yaml` no longer declares a Gemini target.
- [x] `defaultBuildTargets`, target validation, and target dispatch enumerate
  exactly Claude Code, OpenCode, Cursor, Codex, and Amp.
- [x] `dist/gemini/` is no longer produced or tracked.
- [x] Install fencing/MCP logic no longer advertises or installs Gemini.
- [x] Tests and fixtures no longer include Gemini expectations.
- [x] User-side orphan cleanup is not implemented in this task.

## Verification

```bash
! rg -n 'gemini|Gemini' config/targets.yaml internal/cli/build.go internal/cli/build_test.go internal/cli/install.go internal/cli/install_target.go internal/cli/install_target_test.go internal/cli/install_fenced.go internal/cli/install_mcp.go internal/cli/install_mcp_test.go internal/cli/install_command_test.go internal/cli/install_symlink.go internal/cli/version.go package.json README.md docs/ARCHITECTURE.md docs/knowledge/build-system.md docs/knowledge/glossary.md .agents/AGENTS.md
test ! -e dist/gemini
npm run build
npm run test
```
