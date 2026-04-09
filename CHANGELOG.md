# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0-dev.25] - 2026-04-09

### Fixed
- Race condition in auto-resume — check status inside lock (3d539e5)
- Session auto-resume on log to stopped session, soften implement-routing hook (64b3b2a)

## [2.0.0-dev.24] - 2026-04-09

### Changed
- Release skill: tags and GH Releases now created post-merge on `main` instead of pre-merge on feature branch, fixing dangling tag references after squash merge
- Release skill: housekeeping step orchestrates `/wrap`, `/reflect`, and archive instead of just verifying they were done
- Session state: Stop hook changed from CLI command (`loaf session state update`) to agent-written prompt hook — drops redundant journal rehash, writes contextual summary
- Implement skill: description updated to cover all implementation work, not just multi-file tasks

### Added
- `implement-routing` PreToolUse prompt hook on `Edit|Write` — auto-activates `/implement` for implementation work
- `getUncommittedCount()` helper for session state display at startup

### Fixed
- Report and session tests use isolated temp directories (`mkdtempSync` + `realpathSync`) to eliminate flaky failures from cross-file interference in parallel vitest runs
- Session test timeout increased to 15s to accommodate temp directory operations

## [2.0.0-dev.23] - 2026-04-08

### Added
- `/wrap` skill writes Session Wrap-Up report into session file above `## Current State` for archival persistence
- Release skill verifies `/wrap` and `/reflect` were run before merge (wrap required, reflect advisory)
- `/clear` session continuity — `SessionEnd(reason=clear)` logs `session(clear)` marker and keeps session active; `SessionStart(source=clear)` resumes existing session file with new `claude_session_id`
- `## Current State` section in session files, mechanically updated on every Stop event with branch, commit, activity summary, and last 5 journal entries
- Stop hook (`session-state-update`) to trigger Current State updates after each model turn
- Session ID tracking in `session(start)` and `session(resume)` journal entries: `(session {short_id})`
- Current State surfaced in SessionStart output on resume for immediate context recovery
- `source` and `reason` fields in `HookInput` for lifecycle event discrimination
- `clear` entry type in session journal vocabulary

### Removed
- Dead `isNewConversation` variable in session start logic (set but never read)

## [2.0.0-dev.22] - 2026-04-08

### Fixed
- Journal-nudge hook moved from Stop event to PostToolUse(Agent|WebFetch|WebSearch) — Stop forced full-turn retrospection that degraded to only logging commits; PostToolUse gives fresh context per tool result
- Removed Bash from journal-nudge matcher to eliminate noise from routine shell commands
- `validate-commit` hook now correctly parses heredoc-style commit messages instead of capturing raw shell syntax
- `validate-commit` hook skips `-F`/`--file` commits (can't validate file contents from command text)

## [2.0.0-dev.20] - 2026-04-08

### Added
- `loaf report` CLI with `list`, `create`, `finalize`, `archive` subcommands
- Unified report template with status lifecycle (draft → final → archived) and multi-type support (research, audit, analysis, council)
- Drafts lifecycle policy — housekeeping flags state assessments for cleanup when linked session is archived
- `session:` field in state-assessment frontmatter for session linking

### Changed
- Research skill Topic Investigation writes directly to `.agents/reports/` instead of `.agents/drafts/`
- Housekeeping artifact lifecycle table split into state-assessments (session-linked) and brainstorms (user decision)

### Removed
- Findings template (`content/skills/research/templates/findings.md`) — replaced by unified report template

### Fixed
- Report CLI sanitizes path traversal in slug and type arguments
- Report CLI `list --status archived` now scans `archive/` directory
- Report CLI rejects ambiguous substring matches with candidate list

## [2.0.0-dev.19] - 2026-04-07

### Fixed
- `validate-push` no longer false-positives when pushing a release commit (tag at HEAD)
- `workflow-pre-pr` no longer blocks when `[Unreleased]` is empty after release flow moved entries to a version header
- Existing `validate-push` tests fixed to place tag on prior commit (release detection was masking the checks)

### Added
- Report template with frontmatter for `.agents/reports/` (title, type, status, source)
- Research skill promotion path: drafts/ for in-progress, reports/ for final findings
- Wrap skill prompts for missing changelog entries on branches with commits
- 3 new hook tests: release-push pass, tagged-PR pass, spoofed-commit-message block

## [2.0.0-dev.18] - 2026-04-07

### Fixed
- Session end now sets status to `stopped` instead of `paused`
- Same `claude_session_id` always resumes the session (fixes `claude -c` creating duplicate session files)
- Session branch tracking: adopts lone active session when switching branches mid-session
- Commit backfill on resume only includes commits made after the last session entry (no more pre-session noise)
- Journal nudge hook reworded to not hijack model responses

### Changed
- Rename `session(conclude)` entry type to `session(end)` for lifecycle marker
- Rename `conclude(scope)` entry type to `finding(scope)` for analysis results
- Update `EntryType` union and validation script to match new vocabulary
- Release skill post-merge cleanup now ends the session before switching branches

### Added
- Test coverage for branch adoption and same-session-id resume

## [2.0.0-dev.17] - 2026-04-07

### Added
- Add journal logging to workflow skills, broaden nudge hook (0beac80)
- STOP/RESUME separators, merge progress into conclude, remove redundant pause entry (5ab1464)
- PreCompact warns on placeholder Current State, PostCompact prints section content (b1478f6)

### Changed
- Unify session journal entries under session() type (b80fc86)

### Fixed
- Ad-hoc session title and remove Current State placeholder (6a90672)
- Journal amend detection and remove noisy post-commit nudge (bda0074)
- PreCompact warns when Current State timestamp is older than 5 minutes (1da7064)
- PreCompact detects stale Current State via timestamp, nudge requests timestamped heading (e720ca5)
- Resolve all test failures, update 4 stale KB files (e75372b)

## [2.0.0-dev.16] - 2026-04-07

### Added
- Session stability: subagent detection via `agent_id` in hook JSON — subagent spawns no longer create session churn
- `claude_session_id` tagging in session frontmatter for cross-conversation PAUSE/resume detection
- Ad-hoc task auto-creation: `/implement "free text"` creates a task and proceeds without user interaction
- Compaction-aware sessions: PreCompact requires state summary, PostCompact nudges session file re-read
- `## Current State` section seeded in new session files for compaction resilience
- PostCompact prompt nudge in hooks.yaml
- Session management policy (compact vs new session) in orchestration reference
- `/rename` prompt nudge in `/implement` and `loaf session start` output
- `start` journal entry type for new sessions (distinct from `resume`)
- Priority ordering + go/no-go gates as replacement for circuit breaker in spec template and skills

### Removed
- `appetite` field from `SpecEntry`/`SpecFrontmatter` types, parser, CLI display
- `## Circuit Breaker` sections from spec template, shape skill, and all active specs
- `archive-context.sh` hook (referenced stale `.work/` paths, superseded by journal-as-resumption)
- Plan file concept: deleted `content/templates/plan.md`, removed all references from implement, orchestration, housekeeping skills and config
- 5-minute gap heuristic for journal blank lines

### Fixed
- Duplicate commit journal entries: nudge now says "commit auto-logged, log decisions instead"
- Unrecognized Bash commands in `--from-hook` silently exit instead of logging noise
- `process.stdin.unref()` guarded for file-backed stdin (prevents crash on `< hook.json`)
- Cursor PostCompact event mapping added to `mapSessionEvent()`
- `start` entry excluded from `countJournalActivity` system types

### Changed
- Journal markers now all-caps: `SESSION STARTED`, `SESSION RESUMED`, `SESSION PAUSED`
- PAUSE separator written by `session end` only (correct timestamp), not `session start`
- Blank line rules simplified: after PAUSE, before start/resume, nothing else
- Session entry scopes removed from system entries (`pause:` not `pause(branch):`)

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
