---
id: SPEC-006
title: loaf release — version management workflow
source: direct
created: '2026-01-24T22:25:35.000Z'
status: complete
---

# SPEC-006: loaf release

## Problem Statement

Releasing software involves multiple coordinated steps: analyzing changes, determining version bump, updating version numbers across ecosystem files, generating changelog entries, building, creating tags, and drafting releases. Without a structured workflow, steps get forgotten, versions are inconsistent, and releases happen with incomplete documentation.

## Strategic Alignment

| Question | Assessment |
|----------|------------|
| Advances vision? | Yes — disciplined workflows are a Loaf pillar |
| Fits CLI architecture? | Yes — `loaf release` as a new CLI command in `cli/commands/release.ts` |
| Depends on? | SPEC-008 (CLI skeleton) — done; SPEC-007 (loaf init) — done (provides CHANGELOG.md scaffold) |

## Solution Direction

`loaf release` as a CLI command that orchestrates the full release workflow in four phases:

1. **Gather** (no side effects) — analyze commits, detect version files, check task board
2. **Generate** — auto-generate changelog from conventional commits, open `$EDITOR` for polish
3. **Present** — summarize changes, suggest version bump, show what will happen
4. **Execute** — bump version, update changelog, build, tag, GitHub release draft

### Flow

```
loaf release [--dry-run]

  Analyzing commits since v2.0.0...

  3 commits found:
    feat: add init command (7b49dd4)
    fix: address review findings (74332e6)
    chore: spec housekeeping (db7fb53)

  Generated changelog:

    ## [2.1.0] - 2026-03-15

    ### Added
    - Add init command for project bootstrapping

    ### Fixed
    - Address code review findings from SPEC-008

  Opening $EDITOR for review...

  Version files detected:
    • package.json (2.0.0 → 2.1.0)

  Incomplete tasks: 2
    ⚠ TASK-005: Knowledge management setup
    ⚠ TASK-008: Release automation

  Suggested bump: minor (new features detected)
  New version: 2.1.0

  Actions:
    1. Update package.json version → 2.1.0
    2. Write changelog section to CHANGELOG.md
    3. Run loaf build
    4. Create git tag v2.1.0
    5. Create GitHub release draft (gh available)

  Proceed? [y/N]
```

### Implementation

New files:
- `cli/commands/release.ts` — command registration and orchestration
- `cli/lib/release/commits.ts` — conventional commit parsing and classification
- `cli/lib/release/changelog.ts` — changelog generation and CHANGELOG.md manipulation
- `cli/lib/release/version.ts` — multi-ecosystem version detection and update

Registered in `cli/index.ts`.

### Conventional Commit Mapping

| Commit prefix | Changelog section | Version impact |
|---------------|-------------------|----------------|
| `feat:` | Added | minor |
| `fix:` | Fixed | patch |
| `refactor:` | Changed | patch |
| `perf:` | Changed | patch |
| `BREAKING CHANGE` (footer) | Breaking Changes | major |
| `!` after type (e.g. `feat!:`) | Breaking Changes | major |
| `docs:`, `chore:`, `ci:`, `test:`, `build:` | Filtered out | none |

Non-conventional commits are listed under "Other" for manual triage in the editor step.

### Multi-Ecosystem Version Detection

Auto-detect version files in project root (update all found):

| File | Read/Write pattern |
|------|-------------------|
| `package.json` | JSON parse, update `version` field |
| `pyproject.toml` | Regex: `version = "X.Y.Z"` under `[project]` |
| `Cargo.toml` | Regex: `version = "X.Y.Z"` under `[package]` |
| `.agents/loaf.json` | JSON parse, update `version` field (fallback when no ecosystem file) |

### Editor Integration

When `$VISUAL` or `$EDITOR` is set and stdin is a TTY:
- Write generated changelog to a temp file
- Open editor, wait for exit
- Read back the edited content

When no editor or non-interactive:
- Print generated changelog to terminal
- Proceed to confirmation step (user can manually edit CHANGELOG.md before confirming)

### Dry Run (`--dry-run`)

Shows the full gather/generate/present output but skips the execute phase entirely. Exits with a summary of what *would* happen. Useful for CI validation and cautious users.

## Scope

### In Scope

- `loaf release` CLI command with `--dry-run` flag
- Conventional commit parsing (prefix + breaking change detection)
- Auto-generated changelog in Keep-a-Changelog format
- `$EDITOR`/`$VISUAL` integration with non-interactive fallback
- Multi-ecosystem version detection and update (package.json, pyproject.toml, Cargo.toml, loaf.json)
- Task board review (warning, not blocking)
- Changelog section insertion into CHANGELOG.md
- `loaf build` execution
- Git tag creation (annotated, `vX.Y.Z` format)
- GitHub release draft (via `gh` CLI, optional)
- Single confirmation before any side effects

### Out of Scope

- npm/pypi/crates publishing
- Deployment triggers
- Monorepo releases (multiple packages, independent versions)
- Pre-release/beta versions (e.g. `1.0.0-rc.1`)
- Rollback
- E2E test execution (projects configure their own test commands)
- Commit scope parsing (`feat(cli):` → just use message after colon, ignore scope)

### Rabbit Holes

- **TOML parsing library** — use regex for version field extraction in pyproject.toml/Cargo.toml, don't add a TOML parser dependency
- **Commit body/footer parsing** — only scan for `BREAKING CHANGE:` in footers and `!` after type, don't parse full conventional commit spec (scopes, multi-paragraph bodies)
- **Changelog deduplication** — don't try to match commits to existing [Unreleased] entries; the generated changelog replaces the [Unreleased] section
- **Version validation** — basic semver format check only, don't validate ranges or pre-release identifiers

### No-Gos

- Don't auto-release without confirmation
- Don't modify any files before confirmation (or in `--dry-run`)
- Don't publish packages to registries
- Don't block release on incomplete tasks (warn only)

## Test Conditions

- [ ] `loaf release` parses conventional commits since last git tag
- [ ] Maps commit types to correct changelog sections (Added, Fixed, Changed, Breaking Changes)
- [ ] Filters out chore/ci/docs/test/build commits from changelog
- [ ] Lists non-conventional commits under "Other"
- [ ] Opens `$EDITOR` when set and TTY is available
- [ ] Falls back to terminal preview when no editor or non-interactive
- [ ] Auto-detects version files (package.json, pyproject.toml, Cargo.toml)
- [ ] Falls back to loaf.json when no ecosystem version file found
- [ ] Suggests correct version bump (major for breaking, minor for feat, patch for fix)
- [ ] Warns about incomplete tasks without blocking
- [ ] `--dry-run` shows full output but makes no changes
- [ ] Requires explicit confirmation before any file modifications
- [ ] Updates all detected version files
- [ ] Inserts generated changelog into CHANGELOG.md (replaces [Unreleased], adds versioned section)
- [ ] Creates new empty [Unreleased] section after release
- [ ] Runs `loaf build`
- [ ] Creates annotated git tag (`vX.Y.Z`)
- [ ] Creates GitHub release draft when `gh` is available
- [ ] Gracefully handles: missing `gh`, missing CHANGELOG.md, no commits since last tag, no version files found

## Circuit Breaker

At 50%: Ship with changelog generation + package.json version + git tag only (skip multi-ecosystem, $EDITOR, GitHub release).
At 75%: Add multi-ecosystem and $EDITOR. GitHub release can be follow-up.
