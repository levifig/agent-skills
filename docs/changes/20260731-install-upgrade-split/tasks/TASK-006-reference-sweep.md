---
change: install-upgrade-split
id: TASK-006
title: Reference sweep and rebuilt artifacts
blocked-by:
  - TASK-003
  - TASK-004
  - TASK-005
---

# TASK-006 — Reference sweep and rebuilt artifacts

## Objective

No shipped surface teaches the removed flag: loaf-reference skill docs, doctor remediation strings, help text, README, and the CLI reference all describe the install/upgrade split; `dist/` and `plugins/` artifacts are rebuilt from the updated content.

## Scope boundaries

**In:** The complete live command-surface inventory: `content/skills/loaf-reference/references/maintenance.md` and `configuration.md` (the documented `--dry-run --json` planning flow moves to `loaf upgrade`, honoring TASK-002's schema-compat decision), remediation strings in `doctor.go`, `agent_help.go` and `cli_reference.go` metadata, `install_plan.go` doc strings (if any survive TASK-002), `README.md`, the managed fenced content that projects into `AGENTS.md`, changelog note for the breaking removal, `loaf build` + committed `dist/`/`plugins/` artifacts, and the closing `loaf intent resolve` step below.

**Out:** Prior Changes, journal history, and the changelog's historical entries keep the old phrase — history is not rewritten. No source exception is needed for the tombstone: TASK-003 words it to avoid the literal phrase.

## Context pointers

- Contract: `shape.md` — V5, Definition of Done, Durable Outputs.
- Known reference sites from shaping: `content/skills/loaf-reference/references/maintenance.md` (lines 17, 19, 30, 35, 42, 47, 49), `content/skills/loaf-reference/references/configuration.md`, `internal/cli/doctor.go` (fenced-content check details), `internal/cli/install_plan.go` docs strings.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-006 — install/upgrade reference sweep"
grep -rn -- "install --upgrade" content/ internal/cli/ README.md docs/ | grep -v _test | grep -v docs/changes
```

## Steps

- [ ] Sweep every live reference to `install --upgrade` onto the new command split; re-run V5's grep until it exits 1 with only test files excluded.
- [ ] Update doctor remediation strings to name `loaf upgrade`.
- [ ] Changelog entry for the breaking removal; README/CLI-reference regeneration; refresh this repo's `AGENTS.md` managed section from the rebuilt content.
- [ ] `loaf build`; commit rebuilt `dist/` and `plugins/` with the content changes.
- [ ] Close the loop on provenance: `loaf intent resolve INTENT-20260725-split-install-and-upgrade-by-scope-and-detect-a-newer-binary` with this Change as the resolving disposition (and, if TASK-005 was cut, confirm its follow-up capture exists before resolving).

## Verification

- `bash -c 'grep -rq -- "install --upgrade" --exclude="*_test.go" internal/cli/ content/ dist/ plugins/ README.md AGENTS.md'` exits 1 (V5).
- `loaf build` succeeds; `npm run typecheck` and `go test ./...` green.
