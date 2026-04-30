---
id: TASK-143
title: >-
  Add monorepo version-file discovery (.agents/loaf.json release.versionFiles +
  --version-file flag)
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:27:52.204Z'
updated: '2026-04-29T18:50:20.077Z'
completed_at: '2026-04-29T18:50:20.076Z'
---

# TASK-143: Add monorepo version-file discovery (.agents/loaf.json release.versionFiles + --version-file flag)

## Description

Teach `detectVersionFiles` in `cli/commands/release.ts` about monorepo layouts via two-tier declarative resolution. Read an optional `release.versionFiles` array of repo-relative paths from `.agents/loaf.json`; when set, this list replaces root auto-detection (no merge — explicit beats implicit). Add a repeatable `--version-file <path>` CLI flag which, when present, replaces both declared and auto-detected paths for that invocation. When neither is set, fall back to today's root-only auto-detect. Implements SPEC-031 Task 8.

## Acceptance Criteria

- [ ] `.agents/loaf.json` `release.versionFiles` array is read and used as the version-file set when present.
- [ ] `--version-file <path>` is a repeatable CLI flag that replaces both declared and auto-detected paths for the invocation.
- [ ] Declared paths replace (not merge with) root auto-detection.
- [ ] CLI override replaces both declared and auto-detected paths.
- [ ] Each declared/overridden path is validated (must exist, must contain a parseable version) BEFORE any version writes; missing/malformed paths abort with a precise error and no partial bump.
- [ ] When neither config nor flag is set, behavior matches today: scan repo root for `package.json`, `pyproject.toml`, `Cargo.toml`, `.agents/loaf.json`, `.claude-plugin/marketplace.json`.
- [ ] Fixture: project with `.agents/loaf.json` declaring `backend/pyproject.toml` runs `loaf release --dry-run` and shows the backend file in the bump preview.
- [ ] Fixture: `loaf release --version-file frontend/package.json --dry-run` previews exactly that file, ignoring root and declared paths.

## Verification

```bash
npm run typecheck && npm run test -- release
```
