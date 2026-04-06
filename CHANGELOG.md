# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0-dev.15] - 2026-04-07

### Added
- `Suggests Next` section in 8 pipeline skills for workflow continuity (triage→shape→breakdown→implement→release→wrap→housekeeping→reflect)

### Fixed
- 4 pre-tool hooks (`validate-commit`, `validate-push`, `workflow-pre-pr`, `detect-linear-magic`) fired on every Bash command — added `if:` conditions
- Hooks errored on unparseable stdin instead of passing silently

### Changed
- Session filenames simplified to fixed `YYYYMMDD-HHMMSS-session.md` — descriptions in frontmatter, not filenames

## [2.0.0-dev.14] - 2026-04-07

### Added
- `/wrap` skill — responsible session shutdown with journal flush, loose end prompts, and housekeeping check
- `/triage` skill added to README pipeline
- `skill()` journal entry type for self-logging skill invocations
- Skill self-logging convention in CLAUDE.md

### Fixed
- `decide` keyword references in source-of-truth templates (`fenced-section.ts`, `session.md`, `hooks.yaml`) not updated to `decision`
- Session template still using old `- TIMESTAMP` format instead of `[TIMESTAMP]`

### Changed
- `workflow-pre-pr` hook warns when base branch has unpushed commits that would be absorbed into squash merge
- `loaf release` now auto-detects `.claude-plugin/marketplace.json` as a version file

## [2.0.0-dev.13] - 2026-04-06

### Fixed
- Session journal blank line between every entry — `trimEnd()` made separator condition unreachable
- Session resume replaying commits already logged in journal

### Changed
- `session start` archives paused sessions and creates fresh ones by default; `--resume` flag for explicit continuation
- `session end` writes `--- PAUSE ---` separator header between sessions
- Journal entry format: `[YYYY-MM-DD HH:MM]` brackets replace `- YYYY-MM-DD HH:MM` prefix
- `decide` entry type renamed to `decision`

### Removed
- Dead `formatEntry` function, unused `timestamp` parameter, filesystem sync retry loop
- Unnecessary `lockAcquired` flags, session variable aliases, multiline entry display handling

## [2.0.0-dev.12] - 2026-04-06

### Fixed
- Three advisory hooks (pre-merge, pre-push, post-merge) broken since SPEC-020 — `json-parser.sh` dependency deleted but hooks not migrated

### Changed
- New `instruction:` field in hooks.yaml — hooks that output static files now use native `if` conditions instead of bash JSON parsing
- Removed 3 bash hook scripts and shared `json-parser.sh` library (-491 lines)
- Swap `tsx` for `bun` in build script — tsx was declared but not installed; bun is available natively via mise
- `validate-push` and `workflow-pre-pr` hooks downgraded from blocking to advisory — safety nets, not gates
- Release skill now creates PR before version bump when no PR exists (fixes `[Unreleased]` empty conflict)
- All three target builders (Claude Code, Cursor, OpenCode) generate `cat` commands for instruction-file hooks

## [2.0.0-dev.11] - 2026-04-04

### Added
- MCP detection library — detects Linear and Serena across Claude Code and Cursor configurations
- Interactive MCP recommendation flow during `loaf install` with scope choice (global/project)
- `.agents/loaf.json` integration toggles for runtime feature gating without rebuilding

### Changed
- Bundled MCP servers (sequential-thinking, Linear, Serena) removed from Claude Code plugin manifest
- Session magic-word detection gated on `.agents/loaf.json` integration state
- `loaf install --upgrade` skips MCP recommendations
- Integration config merged from `.agents/config.json` into `.agents/loaf.json` per ADR-007
- `AgentsConfig`/`readAgentsConfig` renamed to `LoafConfig`/`readLoafConfig`
- `/cleanup` skill and `loaf cleanup` CLI command renamed to `/housekeeping` and `loaf housekeeping`
- Session journal nudge hooks changed from advisory to imperative ("REQUIRED" before responding)
- 4 knowledge base files rewritten for post-SPEC-020 architecture (hook-system, build-system, task-system, skill-architecture)

### Removed
- `mcpServers` section from plugin.json and Claude Code build target
- `linear-mcp.sh` wrapper script
- `.agents/config.json` (merged into `.agents/loaf.json`)

## [2.0.0-dev.9] - 2026-04-03

### Added
- Amp target (experimental) — skills + runtime plugin for the Amp editor
- `loaf check` CLI — unified TypeScript enforcement backend replacing ~30 shell hook scripts
- `loaf session` subcommands — `start`, `end`, `log`, `list`, `archive` replace resume-session/reference-session skills
- CLI reference skill — non-user-invocable knowledge skill with per-target command substitution
- `council` skill (renamed from council-session) — user-invocable council workflow
- Codex Bash-matching enforcement hooks via generated `.codex/hooks.json`
- Runtime plugins generated for OpenCode (`hooks.ts`) and Amp (`loaf.js`)
- Self-contained `loaf` binary bundled in Claude Code plugin
- Fenced-section management for `loaf install` target project files
- Vulnerability scanner integration in security-audit (trivy, semgrep, npm audit) gated behind VALIDATION_LEVEL

### Changed
- Shared skill intermediate layer (`dist/skills/`) eliminates duplicated build logic across 7 targets
- All 25 skills reordered to structural convention (Critical Rules → Verification → Quick Reference → Topics)
- 16 skills gained Critical Rules sections; all skills now have Verification sections
- Hook payloads normalize both flat (`tool_input`) and nested (`tool.input`) shapes for cross-harness compatibility
- `failClosed` enforcement across Claude Code, Cursor, and Codex hooks
- Signal-killed hook subprocesses now fail closed (`code ?? 1` instead of `code || 0`)
- Session archival uses atomic rename-first to prevent corruption on crash
- Journal entries use proper EntryType values (`resume`/`conclude` instead of invalid `context`)
- Cursor post-tool hook timeouts read from config instead of hardcoded 30s

### Removed
- ~30 legacy shell hook scripts (`content/hooks/pre-tool/`, `post-tool/`)
- 4 shared bash libraries (`json-parser.sh`, `config-reader.sh`, `agent-detector.sh`, `timeout-manager.sh`)
- `resume-session` and `reference-session` skills (absorbed by `loaf session`)

## [2.0.0-dev.8] - 2026-03-31

### Changed
- All 30 skill descriptions rewritten to fit Claude Code's 250-char truncation budget (SPEC-014 follow-up)
- Removed `/ship` alias skill — `/release` already triggers on "ship it"

## [2.0.0-dev.7] - 2026-03-30

### Added
- `/release` skill — orchestrates squash merge ritual: pre-flight, docs freshness, housekeeping, version bump, merge, cleanup (SPEC-019)
- `/ship` alias for `/release` — ergonomic "ship it" invocation
- `loaf release --bump <type>` — skip interactive bump prompt for non-interactive use
- `loaf release --base <ref>` — scope commits to a branch instead of last tag
- `loaf release --no-tag` — skip git tag creation (implies `--no-gh`)
- `loaf release --yes` — skip confirmation prompt for non-interactive use
- Release library test suite: version, changelog, commits, options, and command integration tests

### Changed
- Option validation and skip-flag logic extracted to `cli/lib/release/options.ts`
- `/release` skill detects curated changelog entries under `[Unreleased]` and preserves them instead of regenerating from commits

## [2.0.0-dev.6] - 2026-03-30

### Added
- 4 focused skills extracted from foundations: git-workflow, debugging, security-compliance, documentation-standards (SPEC-014)
- 3 functional profile agents: implementer (Smith), reviewer (Sentinel), researcher (Ranger) with enforced tool boundaries (SPEC-014)
- SOUL.md — Warden identity (Arandil) for coordinator sessions (SPEC-014)
- Self-healing SessionStart hook that restores SOUL.md from canonical template if missing (SPEC-014)

### Changed
- Foundations skill slimmed to code style, TDD, verification, review, and production readiness (SPEC-014)
- All 29 skill descriptions rewritten with action verb openers, user-intent phrases, negative routing, and success criteria (SPEC-014)
- Hook `skill:` fields reassigned to match new skill boundaries (SPEC-014)
- Hook agent predicates updated from role-agent IDs to profile names across 12 hook scripts (SPEC-014)
- OpenCode session hooks now stored as arrays, fixing collision where only the last hook per event survived (SPEC-014)
- ARCHITECTURE.md updated to document profile model and Warden identity (SPEC-014)

### Removed
- 8 role-based agents: pm, backend-dev, frontend-dev, dba, devops, qa, design, power-systems (SPEC-014)
- `{{AGENT:...}}` substitution system from build pipeline (SPEC-014)
- Legacy `plugin-groups` section from hooks.yaml (SPEC-014)

## [2.0.0-dev.5] - 2026-03-29

### Added
- `loaf cleanup` command — scan `.agents/` artifacts and recommend cleanup actions (SPEC-012)
  - Covers all 7 artifact types: sessions, tasks, specs, plans, drafts, councils, reports
  - `--dry-run` and `--sessions`/`--specs`/`--plans`/`--drafts` filters
  - Non-TTY pipe-safe output (behaves like `--dry-run` when piped)
  - Interactive per-item confirmation with delete previews
  - Nested frontmatter support (`session.*`, `council.*`, `report.*`)
  - Dual council schema support (council-session + orchestration formats)
  - Detects drafts promoted to specs via `source` field cross-reference
- Shared prompt helpers (`askYesNo`, `askChoice`, `isTTY`) in `cli/lib/prompts.ts`
- Pre-merge prompt hook for squash merge conventions (clean body, no auto-dump)
- Prompt hook support in build system (Claude Code target; filtered for other targets)
- Advisory `/reflect` suggestion in `/implement` AFTER phase when session has extractable learnings (SPEC-011)
- Post-implementation reflection flag in `/shape` Step 9 for sessions with strategic tensions (SPEC-011)
- `/reflect` recommendation in `/cleanup` extraction checks before archiving decision-rich sessions (SPEC-011)

### Changed
- Spec cleanup (task archival, spec archival) moved to pre-merge on the feature branch instead of post-merge on main
- Post-merge housekeeping reduced to: pull main, delete branch, suggest reflection
- `/cleanup` skill updated to reference CLI as execution engine (skill = policy + Linear, CLI = filesystem)

### Fixed
- Pre-push hook changed from unconditionally blocking (exit 2) to advisory (exit 0)
- Stale `docs/specs/` paths in `/reflect`, `/shape`, and spec template — now `.agents/specs/`

## [2.0.0-dev.4] - 2026-03-27

### Added
- `loaf task archive` command — move completed tasks to archive and update TASKS.json atomically
- `loaf spec archive` command — same for completed specs
- `loaf task sync --push` — push JSON metadata to .md frontmatter (reverse sync)
- Tasks section in `/cleanup` skill with drift detection and CLI-based archival
- Archive step in post-merge housekeeping hook
- SPEC-016 draft: Council Advisory Redesign
- `loaf version` subcommand showing version, Node.js, built targets, and content stats (TASK-020)

### Changed
- Post-merge hook split into pre-merge checklist (changelog, version, build) and post-merge housekeeping (archival, cleanup)
- `/cleanup` archival process now uses CLI commands instead of raw `mv`
- Skills and references replaced `.agents/` path references with CLI commands and IDs
- `council-session` skill changed to model-invoked (not user-invocable)

## [2.0.0-dev.3] - 2026-03-27

### Added
- Workflow enforcement hooks: pre-PR (conditional blocker), post-merge (housekeeping checklist), pre-push (branch safety) (SPEC-015)
- Project-level CHANGELOG.md in Keep a Changelog format with retroactive entries
- Hook library functions `parse_command` and `parse_exit_code` in json-parser.sh

## [2.0.0-dev.2] - 2026-03-27

### Added
- `/bootstrap` skill and `loaf setup` CLI command for 0-to-1 project setup (SPEC-013)

## [2.0.0-dev.1] - 2026-03-25

### Added
- Knowledge management system with staleness tracking and lifecycle hooks (SPEC-009)
- `loaf task` and `loaf spec` CLI commands with managed markdown data model
- `loaf task list --active` flag for filtering in-progress tasks
- `loaf release` command with pre-release versioning support
- `loaf init` command with safe project scaffolding
- `loaf install` command replacing the shell-based installer
- Vitest test infrastructure and task management tests
- TypeScript build system replacing the shell-based builder (SPEC-008)
- Loaf CLI v2.0.0 skeleton and source reorganization

### Fixed
- Post-merge housekeeping steps added to implement skill
- Code review findings from SPEC-008 implementation addressed
- Redundant root CLAUDE.md symlink removed
