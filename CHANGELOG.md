# Changelog

This project follows [Common Changelog](https://common-changelog.org/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). `## [Unreleased]`
is a Loaf workflow staging section for curated entries before release.

## [Unreleased]

- _No unreleased changes yet._

## [2.0.0-pre.20260625190923] - 2026-06-25

### Added

- Added SPEC-052 install-destination parity for Codex, Cursor, OpenCode, and Amp, including a documented `~/.agents/skills` capability table and install records for relocated targets.
- Added the SPEC-049 reversible `loaf state migrate lifecycle-statuses` migration with copy-run dry-run, live-backup apply, rollback manifest, and top-level `loaf migrate lifecycle-statuses` alias.
- Added the SPEC-049 canonical lifecycle status registry for Loaf state entities, including per-entity validators and explicit exclusions for finding and run domain vocabularies.

### Changed

- SPEC-052 updates `loaf install` to write OpenCode and Amp skills to the shared `~/.agents/skills` convention, preserve foreign shared skills, and relocate old Loaf-owned per-harness skill homes through the SPEC-053 upgrade manifest.
- Trimmed duplicated skill guidance and stale references in SPEC-050, including orchestration authority handoffs, ADR-source de-duplication, helper-script contract checks, and generated CLI/session reference coverage.
- Refreshed the SPEC-051 skill routing eval harness and description rewrite validation scaffolding, including dry-run suite checks and conflict-pair probes for measured routing work.
- SPEC-049 lifecycle write paths now emit canonical statuses such as `done`, `paused`, and `in_progress` while tolerating legacy current rows until migration.
- SPEC-049 lifecycle list, show, export, and help surfaces now display canonical statuses while accepting legacy status filters during the migration window.
- SPEC-049 generated CLI reference output and report templates now document canonical lifecycle statuses, including report `done` and the lifecycle-status migration command.

## [2.0.0-pre.20260625183349] - 2026-06-25

### Added

- Added the SPEC-054 rich artifact entity model for reports, findings, verdicts, and provenance runs, including row-shaped JSON imports and multi-format finding exports.
- Added SPEC-046 docs Tier-2 indexing and cross-project search, including `loaf docs index`, docs search locators, stale-index refresh, and branch-aware docs results.
- Added deterministic durable document rendering and finalization in SPEC-044, including scratch/final render commands, self-consistency drift checks, and CI/build drift validation.
- Added SQLite-native artifact bodies and Tier-1 FTS search in SPEC-043, including artifact body schema, dual-source Markdown fallback, body write verbs, direct-write guardrails, and generated CLI reference coverage.

### Changed

- Completed SPEC-047 build integrity and parity hardening, including real JS/TS output validation, first-class Amp TypeScript plugin output, Gemini target removal, Codex hook semantics, OpenCode command reachability, and cross-harness parity linting.
- Converged session workflow guidance in SPEC-048 around SQLite-backed session state, native session journal commands, and render-on-demand Markdown artifacts across skills, templates, agents, and generated targets.
- SPEC-053 adds the breaking-change migration mechanism: `loaf install --upgrade` now reports externalized vendor skills and requires `--yes` before destructive deprecation cleanup, while `librarian` is available as the durable artifact handler across supported harnesses.

## [2.0.0-pre.20260614235428] - 2026-06-14

### Changed

- Added Homebrew-ready release packaging and CI/CD so tagged Loaf releases can build native archives, upload checksummed assets, and update `levifig/homebrew-tap`.
- Completed the boring-reliable state/CLI audit, tying the single global SQLite database contract, durable project identity, repair guidance, backup/export/restore evidence, backend/Linear diagnostics, human help, and agent JSON surfaces to tests, docs, SPEC-040, native cutover guardrails, and live primary-checkout dogfood.
- State, project, repair, backup, and migration terminal help now names the JSON contract fields instead of using generic `Output JSON`, including readiness, diagnostics, repair plans, backup restore guidance, migration context, durable project identity, and applied status.
- Utility and knowledge-base help surfaces now describe `kb`, `check`, `housekeeping`, and `trace` JSON output in terms of knowledge metadata, hook results, cleanup sections/signals, traced entities, global database scope, and project identity across agent help, command help, and generated CLI reference output.
- Entity-family help surfaces now describe `brainstorm`, `idea`, `spark`, `tag`, `bundle`, and `link` JSON output in terms of global database scope, project identity, relationships, events, tags, and bundle membership across agent help, command help, and generated CLI reference output.
- `loaf session report --json` now returns the same session Markdown export contract as state/report generation aliases instead of advertising `--json` and rejecting it; session, task, spec, and report help now describe their JSON scope, project identity, diagnostics, events, and compatibility summaries precisely.
- Agent help and generated CLI reference output now describe critical state JSON contracts precisely for `state path|init|status|doctor`, guarded repairs, backups, top-level migration aliases, restore guidance, global database scope, and project identity instead of using generic raw/details wording.
- Agent help, command help, and generated CLI reference output now describe migration/report JSON contracts consistently, including state migration aliases, project context, global database paths, and report command metadata.
- `loaf state migrate storage-home --dry-run --json` now includes the durable project ID, friendly project name, and current project path when the global data-home database already contains the current project.
- `loaf report generate ... --json` success payloads now include the JSON contract version, report command, global database scope, project export scope, and durable project identity; external reports omit local database and project paths while internal session reports retain them for agent routing.
- Human missing-state errors from `loaf state backup` and Markdown `loaf state export ...` commands now include the global database scope, target database path, and safe next actions while preserving concise JSON errors for agents.
- `loaf state migrate markdown --dry-run --json` now includes the global database scope, target database path, project import scope, project name/path, and `applied: false` without creating SQLite state.
- `loaf state doctor --json` and exported state snapshots now classify local Markdown import and stale compatibility export warnings with structured category, policy, and details fields for safer agent routing.
- `loaf state export all --format json` now carries current state diagnostics and repair-plan actions alongside the raw project tables, so backend/Linear repair follow-up exports preserve the reason and policy that led to the export.
- `loaf state migrate markdown --apply|--resume --json` now includes an explicit `action` field, and human output prints the same action so agents and humans can distinguish fresh imports from resumed imports without relying on argv context.
- `loaf state backup verify --json` now includes the current checkout's restore target, preserve path, and validation commands without reading or recreating live SQLite state; human verify output prints the same concrete restore paths.
- `loaf state doctor --json` backend and Linear diagnostics now include structured `details` fields, so agents can route invalid local backend rows, drift warnings, and external sync gaps without parsing prose.
- Project-specific commands now reject invalid project path invariants before showing or mutating one identity, while `project list --json` remains available for doctor-recommended inspection.
- Project commands now reject schema checksum drift before reading identity state, matching `state doctor` invalid-state behavior and pointing users at the affected global database path.
- Project command human errors for missing SQLite state now include the global database path, scope, and safe `state status` / `state init` next actions instead of a terse missing-database message.
- `loaf project move` now accepts positional absolute paths (`loaf project move <from> [to]`) in addition to `--from/--to`, preserving the same dry-run, JSON, and path-safety checks.
- `loaf state doctor` now rejects backend mapping rows with sensitive-looking external identity values, keeping Linear/backend metadata to identifiers and URLs instead of credentials.
- `loaf state export all --json --format markdown` now returns the same machine-readable flag-conflict error as the reverse flag order instead of falling through to a generic unsupported-format message.
- `loaf report generate` now accepts its documented `--format markdown` option and supports `--json`, returning the same markdown export wrapper used by state exports with machine-readable errors for unsupported formats and missing state.
- Markdown exports from `loaf state export triage|release-readiness|spec|session` now include explicit project context; external-safe exports name the global/project scope, stable project ID, and friendly project name without exposing local paths, while internal exports also include project and database paths.
- `loaf state init|status|doctor` human output now uses the same durable project identity labels as the rest of the SQLite CLI: `project` for ID and `project name` for the friendly name.
- `loaf state backup` human output now ends with a concrete `state backup verify <backup>` next action, and backup help/reference text names the global data-home backups directory.
- `loaf state path --verbose` now provides human-oriented command, scope, project root, and database path context while preserving raw-path default output for shell substitution and restore workflows.
- `loaf project show|identity` and `loaf project list` human output now use the same command, scope, database, project ID, friendly name, and project path labels as project identity mutations.
- `loaf state migrate markdown` and `loaf state migrate storage-home` human output now report command, global database scope, project import/migration scope, database path, project context, applied status, and dry-run next actions consistently.
- `loaf project rename|move` human output now reports command, scope, database, project identity, from/to values, applied status, and dry-run next actions consistently.
- `loaf state doctor` diagnostics now label backend mapping and Linear sync findings by policy so local data fixes, drift audits, and external sync work are easier to distinguish.
- `loaf state doctor` repair-plan commands now have regression coverage proving suggested follow-up commands run in the diagnostic mode that produced them.
- `loaf state doctor` repair plans now classify local database, backend mapping, and external sync actions for clearer human and agent follow-up.
- Added safe next-action guidance to backup verification output after dogfooding the manual restore flow, so users know how to preserve the current DB, restore the verified backup, and rerun health checks.
- Documented and verified a manual SQLite backup restore flow so users can recover the global database by verifying a backup, preserving the current DB, copying the backup into place, and running health checks.
- Completed the Gate 1 control-plane evidence pass with regression coverage for project rename/move safeguards and repair dry-runs, including durable project identity, single current path, dry-run table stability, and legacy archive preview safety.
- Added command-matrix regression coverage for critical state/project/migration JSON success contracts, including read-only no-mutation checks, migration dry-run no-copy/no-database checks, and backup verification without live state access.
- Refocused the boring-reliable state/CLI plan into gated execution criteria so future work progresses through control-plane proof, recovery confidence, and UX/policy normalization instead of broad edge chasing.
- Added command-matrix regression coverage for critical state/project/migration JSON failure contracts, including contract version, command name, silent exit code, and no database creation for pre-open failures.
- Added a focused boring-reliable state/CLI plan that turns the remaining SQLite hardening work into an explicit reliability contract, command matrix, and prioritized audit tracks.
- `loaf state export all --json` is now accepted as an agent-friendly alias for `loaf state export all --format json`, while markdown export kinds continue to require explicit `--format markdown`.
- `loaf state doctor` repair plans now route invalid backend-mapping diagnostics to `loaf state doctor --json` instead of suggesting `state export`, which refuses to run while state is invalid.
- `loaf state doctor --json` now includes non-mutating repair plans whenever diagnostics are present, even without `--dry-run`, so agents receive next actions alongside health failures.
- `loaf state backup verify --json` now includes `backup_path` in verification failure payloads after a path has been parsed, making invalid-backup diagnostics easier for agents to correlate.
- `loaf state path --json` now reports the resolved global SQLite path with contract version, project root, and database scope without creating the database.
- `loaf state doctor` now accepts project-level backend mappings, allowing a Loaf project to be linked to a Linear/external project while still rejecting mismatched project mapping IDs.
- `loaf state doctor --json` now exits nonzero for invalid SQLite state while still returning the machine-readable status payload.
- `loaf state export all --format json` now includes `project_paths` rows so project-scoped snapshots preserve checkout path history alongside durable project identity.
- `npm run build` now rebuilds the Go CLI before regenerating the CLI reference so agent-facing docs do not lag behind command metadata changes.
- `loaf state backup verify <backup> [--json]` now verifies existing SQLite backups without live-state access and reports all project identities captured in the global backup.
- `loaf task refresh|sync --json` and `loaf session enrich|housekeeping --json` compatibility summaries now include `contract_version` for agentic consumers.
- `loaf housekeeping` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown fallback output keeps repository-local artifact context.
- `loaf trace` and `loaf spec show` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown `spec show` fallback output keeps repository-local spec context.
- `loaf task list|show|status` and `loaf spec list` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown fallback output keeps repository-local task/spec context.
- `loaf brainstorm promote|archive|list|show` JSON and human output now report global database scope and durable project identity details.
- `loaf state doctor` and SQLite-backed `loaf report list` now warn when the global database is ready but the current repo still has importable local `.agents` Markdown that has not been migrated.
- `loaf session start|end|archive|list|show|log` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown fallback output keeps repository-local session context.
- `loaf spark capture|promote|resolve|list|show` JSON and human output now report global database scope and durable project identity details.
- `loaf idea capture|promote|resolve|archive|list|show` JSON and human output now report global database scope and durable project identity details.
- `loaf report create|finalize|archive|list` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown fallback output keeps repository-local report context.
- `loaf bundle create|update|add|remove|list|show` JSON and human output now report global database scope and durable project identity details.
- `loaf tag add|remove|list|show` JSON and human output now report global database scope and durable project identity details.
- `loaf link create|remove|list` JSON and human output now report global database scope and durable project identity details.
- `loaf spec archive` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown fallback JSON includes the contract version without database context.
- `loaf task create|update|archive` JSON and human output now report global database scope and durable project identity details when backed by SQLite, while Markdown fallback JSON includes the contract version without database context.
- `loaf project show|list|rename|move` JSON and human output now identify project metadata as global database state.
- `loaf state repair ...` JSON and human output now report global database scope and durable project identity details for guarded repair previews and applies.
- `loaf state init|status|doctor` now report global database scope consistently in JSON and human output, and human diagnostics include durable project identity details when available.
- `loaf migrate storage-home --json` and human output now report global database scope, project migration scope, and applied project identity details.
- `loaf migrate markdown --apply|--resume --json` now reports global database scope, project import scope, and durable project identity details.
- `loaf state doctor` now warns when backend mapping rows use an unknown `sync_status`, helping catch misspelled integration state without invalidating the database.
- `loaf state export all --format json` now reports `database_scope` and `export_scope` in the snapshot and manifest, making project-scoped exports from the global database explicit.
- `loaf state backup` JSON and human output now report the number of project identities captured in the global database backup.
- `loaf state backup` JSON and human output now identify backups as global database backups.
- `loaf project move` now rejects missing or non-directory target paths before previewing or recording a checkout move.
- `loaf state doctor` now flags backend mapping rows with empty backend, local entity, external entity, or sync-status fields.
- `loaf state repair ...` human output now shows `--dry-run` or `--apply` in the command header and suppresses apply guidance when no rows or files match.
- `loaf migrate markdown --json`, `loaf migrate storage-home --json`, and `loaf state repair ... --json` success payloads now include `contract_version`.
- `loaf project identity` now works as a discoverable alias for `loaf project show`.
- `loaf project show|list|rename|move --json` now include `contract_version` for agentic consumers.
- `loaf state init|status|doctor --json` now include `contract_version` for agentic consumers.
- JSON error payloads now include `contract_version` for agentic consumers.
- `loaf state backup --json` and `loaf state export all --format json` now include `contract_version` for agentic JSON consumers.
- `loaf state backup --json` and human output now include the backup file's SHA-256 digest for artifact verification.
- `loaf state backup --json` and `loaf state export all --format json` now surface project name and current project path alongside the durable project ID.
- `loaf state status` and `loaf state doctor` now inspect existing SQLite databases through read-only connections.
- SQLite backup and export verification errors now include the first foreign-key violation's table, row, parent table, and constraint details.
- `loaf state export all --format json` manifest now reports SQLite integrity and foreign-key verification checks.
- `loaf state backup` now verifies and reports backup foreign-key integrity alongside SQLite integrity checks.
- `loaf state doctor` now reports SQLite `quick_check` failures and foreign-key violations as explicit invalid-state diagnostics.
- `loaf project rename --json` now requires an existing registered project identity and no longer initializes missing SQLite state as a side effect.
- `loaf project move --json` now validates against an existing SQLite database before opening a writable handle, so rejected moves no longer create empty state.
- `loaf project show|list` now open the global SQLite database read-only and no longer initialize missing state as a side effect.
- `loaf state status` now distinguishes durable SQLite `project_id` from the path-derived `legacy_project_key`, avoiding pre-init identity confusion.
- `--agent-help` and the generated `cli-reference` skill now document the generic `loaf state export --format <format>` contract.
- `--agent-help` now documents `loaf build`/`install` short aliases and non-interactive install confirmation flags consistently with native help.
- `--agent-help` now documents housekeeping's legacy-compatible `--plans` and `--handoffs` filters.
- `loaf report create --help` now matches the parser by documenting `--source` and no longer advertising unsupported `--title`.
- `--agent-help` and the generated `cli-reference` skill now document `loaf migrate worktree-storage` dry-run/apply and conflict-resolution flags.
- `loaf kb ... --help` now works for knowledge-base subcommands and `--agent-help` documents KB JSON/path options for agentic use.
- `loaf report list --help`, `--agent-help`, and the generated `cli-reference` skill now document Loaf's report lifecycle statuses for `--status` filters.
- `loaf report generate --help`, `--agent-help`, and the generated `cli-reference` skill now state that `--format` expects Markdown output.
- `--agent-help` and the generated `cli-reference` skill now document concrete `loaf state export` subcommands and required `--format` contracts.
- `loaf state export all --format json` now includes a verified manifest with table order, per-table row counts, and total exported rows.
- `loaf state export all --format json` manifest now includes an explicit `table_count` for agentic consumers.
- `loaf state export ...` generation now reads SQLite through read-only connections.
- `loaf project rename|move --json` validation and safeguard failures now return machine-readable JSON error payloads instead of plain text.
- `loaf state init|status|doctor --json` validation failures now return machine-readable JSON error payloads instead of plain text.
- `loaf state migrate|repair --json` validation and safeguard failures now return machine-readable JSON error payloads instead of plain text.
- `loaf state backup --json` and `loaf state export all --format json` failures now return machine-readable JSON error payloads instead of plain text.
- `loaf trace --json` and `loaf idea capture --json` validation failures now return machine-readable JSON error payloads instead of plain text.
- `loaf link create|remove` now accepts the documented `--from` and `--to` flags, and `--json` validation failures return machine-readable JSON error payloads.
- `loaf --json` command paths now apply a central fallback so unwrapped validation failures still return machine-readable JSON error payloads.
- `--agent-help` and the generated `cli-reference` skill now document task mutation and compatibility `--json` options.
- `--agent-help` now has a regression guard against live help drift for documented `--json` options, and documents state/session/housekeeping JSON output options consistently.
- `loaf trace --help` now shows trace usage instead of reporting `--help` as an unknown option, and `--agent-help` documents trace JSON output.
- `loaf check --help` now shows registered hook usage instead of reporting `--help` as an unknown option.
- `loaf migrate markdown|storage-home --help` now shows top-level migration usage instead of reporting `--help` as an unknown option, and `--agent-help` documents their migration options.
- `loaf state doctor` now warns when Linear integration is enabled but active local task rows have no Linear backend mapping.
- `--agent-help` now documents state-backed brainstorm, idea, spark, tag, bundle, and link subcommands instead of exposing them as bare top-level command names.
- The generated `cli-reference` skill now documents top-level command options plus state-backed trace, brainstorm, idea, spark, tag, bundle, and link commands.
- `loaf task create|list|update --json` validation failures now return machine-readable JSON error payloads instead of plain text.
- `loaf task list|update` help, invalid-status errors, and agent help now name the valid task statuses.
- `loaf task create|update` help, invalid-priority errors, and agent help now name the valid task priorities.
- The generated `cli-reference` skill now uses the same task status and priority values as the native CLI help and agent help.
- `--agent-help` and the generated `cli-reference` skill now document `loaf project` identity commands and their dry-run safeguards.
- `--agent-help` and the generated `cli-reference` skill now describe `loaf project list` global database JSON fields.
- `--agent-help` and the generated `cli-reference` skill now document guarded `loaf state repair` targets and safety flags.
- `loaf state doctor` now validates backend mapping drift for Linear and other external integrations, including orphaned local entities, unknown entity kinds, and ambiguous local-to-external mappings.
- `loaf state doctor` repair plans now deduplicate repeated repair actions while preserving distinct diagnostic causes.
- `loaf state backup` now verifies backup integrity, schema version, and project identity before returning, and reports those checks in JSON and human output.
- `loaf state backup` now verifies created backups through a read-only SQLite connection so verification does not mutate backup files or create sidecars.
- `loaf project move` now supports `--dry-run` for validated path-move previews without mutating the global project identity index.
- Project rename and move dry-runs now open SQLite read-only, avoid initializing missing databases, and `loaf project rename` supports `--dry-run` previews.
- State doctor and repair JSON now keeps empty repair/archive fields as arrays instead of omitting them or returning `null`.
- `loaf state repair legacy-project-database` now previews and archives migrated per-project SQLite leftovers without deleting them.
- `loaf state repair relationship-origin` now previews and applies guarded relationship provenance backfills, creating a SQLite backup before writes.
- `loaf state doctor` now checks operational SQLite invariants for project path identity and relationship provenance, with manual repair guidance for unsafe drift.
- `loaf state doctor --dry-run` now reports an explicit repair plan in human and JSON output without mutating SQLite state or legacy databases.
- `loaf project list` now shows registered projects from the global SQLite database with stable IDs, friendly names, current paths, and JSON output.
- Native Go is now the shipped Loaf runtime, with cross-platform binaries replacing the transitional TypeScript delegation path.
- Existing Markdown-only Loaf projects now have a documented dry-run and apply path for adopting SQLite-backed state without rewriting source artifacts.
- SQLite project identity now uses generated stable project IDs in one global database, plus friendly names and path mappings managed by `loaf project show|rename|move`.
- `agents-config` now documents and pins the fall-back-to-`projectRoot` behavior when a linked worktree's `.git` pointer file is malformed (missing `gitdir:` line or non-matching shape). This is the deliberate Case-4 fallback in `resolveEffectiveRoot` — distinct from the "main removed" case fixed in #53, which still throws. Closes a Codex review follow-up on #53.

### Fixed

- Markdown migration apply no longer requires legacy `.agents/TASKS.json` when importing Markdown-only task files.
- State-backed CLI commands now handle parent and nested `--help` consistently before parsing options or opening SQLite state.
- SQLite-backed state commands now fail on project identity mapping errors instead of silently falling back to path-derived legacy project IDs.
- Storage-home migration now preserves pending SQLite writes when copying legacy state into XDG data-home storage.
- Markdown migration relationship imports now ignore empty dependency arrays, prune stale imported links by structured origin, and record imported/manual relationship provenance.
- Storage-home migration now upgrades copied legacy databases before readiness checks and rekeys legacy path-hash project rows into generated stable identities in the global database.
- Project path moves now reject unknown source paths without creating a stray project row, and SQLite enforces one current path per project.

### Removed

- Removed the bundled TypeScript command runtime and obsolete TypeScript build/test toolchain from the shipped CLI.

## [2.0.0-dev.49] - 2026-05-31

### Fixed

- `findActiveSessionForBranch` now applies a deterministic `filePath` tiebreaker on both the branch-match and most-recent-active fallback paths, so candidates with byte-identical effective timestamps resolve to the same session across repeated calls regardless of `readdirSync` order.

## [2.0.0-dev.48] - 2026-05-29

### Changed

- Document Go as the accepted runtime direction for Loaf's stateful core and shape SQLite work around that runtime foundation.
- Add the initial dependency-free Go runtime skeleton for future state commands without changing the shipped TypeScript CLI.
- Add Go-native `loaf state path` dispatch with XDG-backed, hashed project-state paths shared by main and linked Git worktrees.
- Add a transitional Go-to-TypeScript legacy delegation bridge so unmigrated commands keep using the bundled CLI while `state` commands remain Go-native.
- Use a portable `bin/loaf` launcher with native runtime lookup under `bin/native/<platform>-<arch>/`, keeping legacy TypeScript delegation installable on unsupported native platforms during the Go port.
- Select `github.com/ncruces/go-sqlite3/driver` as the SQLite driver, with cgo-free, dependency, vulnerability, and testability guardrails documented before adding the dependency.
- Package the Go front controller as the public `loaf` command while bundling TypeScript fallback assets for delegated commands.
- Define the initial SQLite operational-state schema as Go-owned migration metadata with dependency-free guardrail tests.
- Add the approved cgo-free SQLite driver and Go-native `state init/status/doctor` storage lifecycle commands.
- Add a Go-native markdown migration dry-run that previews `.agents/` import counts, skipped files, inferred relationships, and spark entries without creating SQLite state.
- Add a Go-native markdown migration apply path that initializes SQLite and imports current `.agents/` specs, tasks, ideas, brainstorms, sessions, reports, sparks, relationships, and source provenance without mutating Markdown.
- Add a Go-native `loaf trace` read model for imported SQLite state, resolving aliases to source provenance plus inbound and outbound task/spec relationships.
- Add a Go-native `loaf task list` read path for imported SQLite state, with JSON and human output, active/status filters, dependency aliases, source provenance, and Markdown compatibility fallback.
- Add a Go-native `loaf spec list` read path for imported SQLite state, with JSON and human output, task counts, source provenance, and Markdown compatibility fallback.
- Add a Go-native `loaf session list` read path for imported SQLite state, with JSON and human output, archived-session import, journal counts, source provenance, and Markdown compatibility fallback.
- Add a Go-native `loaf report list` read path for imported SQLite state, with JSON and human output, type/status filters, archived-report import, source provenance, and Markdown compatibility fallback.
- Import explicit lineage relationships and spark promotion links into SQLite so `loaf trace` can show spark-to-idea-to-spec/task provenance from migrated `.agents/` artifacts.
- Import shaping draft artifacts into SQLite so `loaf trace` can show brainstorm-to-idea-to-shaping-draft-to-spec/task provenance from migrated `.agents/` drafts.
- Add a Go-native `loaf session log` write path for initialized SQLite state, preserving unresolved journal context as nullable fields while keeping hook-mode and Markdown-only compatibility delegation.
- Add Go-native `loaf idea list` and `loaf idea resolve ... --by ...` paths so imported ideas can be marked resolved in SQLite and stay out of the default triage list.
- Add Go-native `loaf spark list` and `loaf spark resolve ... --by ...` paths so imported sparks can be marked resolved in SQLite and stay out of the default triage list.
- Add Go-native `loaf tag list/show/add/remove` paths for SQLite-backed many-to-many classification across imported state rows and journal entries.
- Add Go-native `loaf bundle create/show/add/remove` paths for SQLite-backed related sets assembled from tag queries and explicit membership.
- Add Go-native `loaf link create/list/remove` paths for SQLite-backed explicit relationships that are visible through `loaf trace`.
- Add Go-native `loaf task update <task> --status <status>` for SQLite-backed task status changes with status-change events.
- Add Go-native `loaf task show <task>` for SQLite-backed task inspection with dependencies, source provenance, and imported body output.
- Add Go-native `loaf task create --title ...` for SQLite-backed task creation with generated aliases, spec/dependency relationships, and creation events.
- Add Go-native `loaf task update <task>` metadata updates for SQLite-backed priority, spec, dependency, and session relationship changes.
- Add Go-native `loaf spec archive <spec>` for SQLite-backed spec archival with status-change events and skipped-ref reporting.
- Add Go-native `loaf task archive` for SQLite-backed task archival by task ID or spec, with status-change events and active-list filtering.
- Add Go-native `loaf idea capture --title ...` for SQLite-backed idea creation with generated aliases, status-change events, and list/trace visibility.
- Add Go-native `loaf idea archive` for SQLite-backed idea archival with optional reason notes, status-change events, skipped-ref reporting, and default triage-list filtering.
- Record optional `loaf spark resolve --reason` rationale on SQLite-backed spark resolution relationships and status-change events.
- Add Go-native `loaf spark capture --scope ... --text ...` for SQLite-backed spark creation with generated aliases, status-change events, and list/trace visibility.
- Add Go-native `loaf spark promote --to-idea ...` for SQLite-backed spark-to-idea promotion relationships visible through trace and link reads.
- Add Go-native `loaf idea show` for SQLite-backed idea inspection with source provenance, imported body output, and immediate relationships.
- Add Go-native `loaf idea promote --to-spec ...` for SQLite-backed idea-to-spec promotion relationships visible through trace, link, and idea-show reads.
- Add Go-native `loaf brainstorm list` for SQLite-backed brainstorm triage reads with status filters and source provenance.
- Add Go-native `loaf brainstorm show` for SQLite-backed brainstorm inspection with source provenance, imported body output, and immediate relationships.
- Add Go-native `loaf brainstorm promote --to-idea ...` for SQLite-backed brainstorm-to-idea promotion relationships visible through trace, link, and brainstorm-show reads.
- Add Go-native `loaf brainstorm archive` for SQLite-backed brainstorm archival with optional reason notes, status-change events, and default triage-list filtering.
- Add Go-native `loaf state backup` for repository-external SQLite database backups with JSON and human output.
- Add Go-native `loaf state export all --format json` for internal SQLite state snapshots.
- Add Go-native `loaf state export triage --format markdown` for external-safe SQLite triage summaries.
- Add command-level linked-worktree coverage proving Go-native SQLite state commands share the same project database.
- Add `loaf state doctor` diagnostics for schema mismatch, migration checksum drift, and stale generated export records.
- Add state-init safety coverage proving SQLite state is repository-external and schema columns avoid secret storage.
- Add public binary bridge coverage proving one packaged `loaf` command can run Go-native state commands and delegate unmigrated commands to the TypeScript fallback.
- Add public CLI dry-run coverage proving markdown migration previews all required counts and leaves SQLite state uncreated.
- Add SQLite secret-boundary coverage proving backend mappings store only external identity metadata and native state code does not introduce token or credential storage.
- Harden Go artifact verification so release builds compare the launcher, platform native runtime, plugin mirror, and TypeScript fallback layout.

### Fixed

- Return clear SQLite initialization errors for Go-only commands in Markdown-only projects instead of delegating to missing TypeScript commands.
- Resolve installed TypeScript fallback assets from the namespaced `~/.local/share/loaf/dist-cli` layout used by `loaf install`.
- Keep SQLite writes transactional for tag creation and apply per-connection SQLite pragmas for foreign keys, WAL journaling, and busy timeout handling.

## [2.0.0-dev.47] - 2026-05-28

### Fixed

- Branch-fallback session routing no longer rewrites the adopted session's `branch:` frontmatter. Previously, every `loaf session log` against a branch with no dedicated session would overwrite the resolved session's origin branch, breaking subsequent routing. The session's origin is now preserved across every adoption.
- Multi-worktree branch routing now resolves correctly. When several sessions are active simultaneously (orchestrator on one branch, agents on others) and the current branch has no dedicated session, `loaf session log` adopts the most-recently-updated active session instead of dropping the entry. Previous behavior only fell back when exactly one active session existed.
- Branch-fallback WARN now names the resolved session file and its origin branch (e.g., `WARN: no session for branch 'release/v0.16.0'; logging to most-recent active session '20260101-120000-session.md' (origin branch 'cwt/foo'). Pass --session-id <id> to silence.`), so misroutes are visible at a glance. A distinct WARN fires when no active session exists to fall back to.
- Branch-fallback WARN now distinguishes rename-link adoption (`WARN: branch '<new>' appears to be a rename of '<old>'; logging to its session ...`) from most-recent-active adoption, so operators can tell why a log landed where it did instead of seeing the inaccurate "most-recent active" wording on every adoption.

## [2.0.0-dev.46] - 2026-05-28

### Changed

- Adopt Common Changelog as Loaf's changelog writing standard ([Common Changelog](https://common-changelog.org/))
- Let council workflows state the selected composition and spawn directly, while still reserving the final decision for the user

### Fixed

- `.agents/loaf.json` reads and writes from a linked git worktree now follow the SPEC-036 centralization to the main worktree, so `loaf release --pre-merge` (and other consumers of `loaf.json`) no longer fail with "No version files found" when invoked from a migrated linked worktree.
- `agents-config` now throws an actionable error (instead of silently writing a stale shadow config) when a linked worktree's recorded main has been removed, mirroring the diagnostic surfaced by `loaf migrate worktree-storage`.
- `workflow-pre-pr` no longer treats backtick-quoted `## [Unreleased]` mentions in CHANGELOG prose as the real section header, so PRs whose intro text references the staging area no longer false-block with "empty Unreleased section".

## [2.0.0-dev.45] - 2026-05-27

### Fixed

- `loaf release` refreshes uv-managed Python release artifacts with package-local `uv sync`, and refuses to commit unignored `.venv` files created during release artifact refresh.

## [2.0.0-dev.44] - 2026-05-22

### Added
- Add dedicated handoff skill (b9a97b51)

## [2.0.0-dev.43] - 2026-05-20

### Added

- `loaf task list --status <status>` filters task output by lifecycle state.

### Changed

- `loaf release --post-merge` now validates release readiness from version files and `CHANGELOG.md`, so conventional squash subjects like `feat:` and `fix:` are accepted.
- Release workflow guidance now calls out post-bump build-artifact verification and stricter changelog curation before publishing.

### Fixed

- `loaf migrate worktree-storage` treats identical file contents as already resolved before considering mtimes or overwrite conflicts.
- Worktree migration diagnostics now surface directory-read failures under `LOAF_DEBUG_RESOLVE`, and migration output respects `NO_COLOR` and non-TTY output.
- Release tags are created with explicit signed-tag mode.
- Task index rebuild and frontmatter sync preserve valid concurrent entries and unknown spec metadata.
- Task and session lock staleness detection share one PID/host-aware policy, avoiding false eviction of live same-host lock holders.
- Linked-worktree migration refusal preserves unknown-command feedback while still nudging users toward `loaf migrate worktree-storage`.

## [2.0.0-dev.42] - 2026-05-19

### Added

- **Worktree-aware `.agents/` storage.** Loaf now treats `.agents/` as project-scoped state rather than branch-scoped content. Running any `loaf` command from a linked git worktree (a `git worktree add`-style checkout) resolves `.agents/` to the **main worktree's directory** instead of the worktree's own copy. Sessions, IDs, knowledge, and ideas converge across all worktrees of a project. Single-checkout repositories see no behavior change. See [ADR-013](docs/decisions/ADR-013-agentic-state-storage-model.md) for the decision rationale and rejected alternatives.
- **`loaf migrate worktree-storage`.** New command that moves a linked worktree's local `.agents/` content into the main worktree's `.agents/`. Dry-run by default; `--apply` performs the move. Conflict policy prefers the most-recently-modified version when a file exists in both locations, with `--force-from-worktree` and `--force-from-main` overrides. Writes a `.moved-to` back-pointer in the source location and is idempotent on re-run. EXDEV (cross-filesystem) moves fall back to `fs.cpSync` with `preserveTimestamps`.
- **Pre-A3 refusal nudge.** Loaf invocations in a linked worktree with un-migrated content are refused with exit code `2` and a single-action instruction: run `loaf migrate worktree-storage`. The migrate command itself, `help` / `-h` / `--help`, and `-v` / `--version` are always allowed; single-checkout repositories and the main worktree never trigger the refusal.
- **`LOAF_DEBUG_RESOLVE` environment variable.** When set to `1` / `true` / `yes` / `on` (case-insensitive), surfaces git probe diagnostics from `findAgentsDir` that are otherwise suppressed. Useful for diagnosing unexpected refusal nudge fires.

### Changed

- **Retired convention: "spec on main, tasks+code on branch".** Under the worktree-aware storage model, `.agents/` content always lives in the main worktree's directory regardless of which branch's PR is in flight. PR templates and project docs that referenced this convention should be updated.

### Migration

Users with active `git worktree add` linked worktrees containing `.agents/` content must run `loaf migrate worktree-storage --apply` once after upgrading. Single-checkout repositories require no action.

## [2.0.0-dev.40] - 2026-05-02

### Added

- `git-workflow` skill — new "Changelog Discipline" section in `references/commits.md`. Codifies the rule that user-facing CHANGELOG entries describe what changed from a user/operator's perspective, not how the work was tracked or organized internally. Drops internal terms (spec/task IDs, internal session references, hook IDs that aren't user-facing); keeps references to public artifacts (`ADR-NNN`, public CLI flags, documented file paths); requires curating auto-generated `loaf release --pre-merge` output before bumping.

## [2.0.0-dev.39] - 2026-05-02

### Added

- ADR lifecycle now supports `Rejected` as a fifth status. Full lifecycle: `Proposed | Accepted | Rejected | Deprecated | Superseded`. A `Rejected` ADR records "the team weighed this option and explicitly chose against it" — useful when the same idea resurfaces.

### Changed

- `architecture` skill — Lifecycle section codifies body-section requirements by status. `## Deprecated` is required for `Deprecated`, `## Rejected` is required for `Rejected`, `## Superseded` is optional for `Superseded` (the `superseded_by:` linkage suffices).
- ADR frontmatter schema finalized as structured what+when: `status`, `date`, `accepted_date` (optional), `rejected_date`, `deprecated_date`, `supersedes`, `superseded_by`. The `deprecated_reason` and `migrated_to` fields introduced during the previous deprecation pass are dropped — context belongs in the body section's prose, not duplicated in frontmatter.
- ADR template (`content/templates/adr.md`) updated with the new schema and a header note that `Rejected` and `Deprecated` ADRs require a body section.
- `ADR-004`, `ADR-006`, `ADR-009` frontmatter cleaned up to match the new schema; body sections preserve all migration content.
- `docs/ARCHITECTURE.md` Operating Principles section gains two new subsections:
  - **Adversarial Review for Substantive Guidance Changes** — `loaf:reviewer` is the baseline (internal-consistency auditor); `codex:rescue` or equivalent adversarial reviewer is recommended when available, since the two readers catch different defect classes. Codex is plugin-dependent and optional.
  - **Recategorization as a General Lifecycle Pattern** — distinguishes supersession (the answer changed; new artifact replaces old) from recategorization (the artifact's classification was wrong; the underlying rule still holds; deprecate-in-place and point to new home). Generalizes beyond ADRs.

## [2.0.0-dev.38] - 2026-05-02

### Changed

- `architecture` skill — tightened bar. ADRs are now reserved for architecturally significant decisions (those affecting the system's structure, key quality attributes, dependencies, interfaces, or construction techniques) OR decisions that are difficult to reverse, per Microsoft Well-Architected canonical phrasing. The skill includes a structured Triage Gate that operationalizes the bar with explicit routing for non-ADR decisions to `/shape` (SPEC), `ARCHITECTURE.md` / `VISION.md`, the owning skill's docs, or session-log.
- `architecture` skill — "decisions are choices" filter. ADRs require at least one credible alternative considered. Catches principle/manifesto-shaped artifacts at write time and routes them to `ARCHITECTURE.md` or `VISION.md` instead.
- `architecture` skill — cost-of-divergence framing. The skill evaluates decisions by the consequence of casual divergence (now: security regression, contract or interface break, multi-PR coordination; later: foundational shape commitments whose future reversal cost is the reason to record now) rather than by the cost of change alone. Captures security-boundary decisions reversible-by-code and foundational early-project commitments.
- `architecture` skill — Lifecycle nuance. Original `Decision`/`Context`/`Rationale`/`Consequences` sections are immutable post-acceptance; status transitions, frontmatter additions, and append-only `## Deprecated` / `## Superseded` sections are the supported lifecycle mechanism. Distinguishes recategorization (deprecate-in-place, content moved elsewhere) from supersession (new ADR replaces old, both linked).
- `architecture` skill — maturity-aware bar. The bar is constant; the number of decisions clearing it scales with project maturity. Early/exploratory phases pass foundational shape commitments via the cost-of-divergence framing's "later" prong.
- ADR template (`content/templates/adr.md`) — HTML-comment header surfaces the bar to agents reading the template; propagates to the `reflect` skill's shared template via the build system.
- `docs/ARCHITECTURE.md` — new Operating Principles section, with the `Authorship Model — Agents Create, Humans Curate` subsection as its first principle.
- `docs/knowledge/knowledge-management-design.md` — new Naming Conventions section.
- `docs/decisions/README.md` — index updated; missing ADR-012 row added.

### Deprecated

- ADR-004 (Knowledge Naming Convention) — recategorized as a project naming convention. Active source: `docs/knowledge/knowledge-management-design.md` Naming Conventions section.
- ADR-006 (Agent-Creates, Human-Curates Model) — recategorized as a guiding principle (philosophical/operational rationale, not architectural). Active source: `docs/ARCHITECTURE.md` Operating Principles section.
- ADR-009 (Sparks Convention in Brainstorm Documents) — recategorized as workflow lore for the `brainstorm` skill. Owning skill is the canonical source.

## [2.0.0-dev.37] - 2026-05-02

### Added

- `/refactor-deepen` skill — surfaces refactoring opportunities through a deepening lens (modules that hide complexity behind narrow interfaces). Vocabulary discipline is load-bearing: the skill uses an eight-term taxonomy (Module, Interface, Implementation, Depth, Seam, Adapter, Leverage, Locality) ported verbatim from Matt Pocock's `improve-codebase-architecture` skill, with `references/language.md`, `references/deepening.md`, and `references/interface-design.md` providing the vocabulary's full semantics. Default INTERFACE-DESIGN phase spawns 3 sub-agents with identical briefs (no opposing-constraint priming) — variety emerges from sampling, not manufactured opposition. Terminates by writing a PLAN file. Not for renames, extractions, or generic restructuring (use `/loaf:implement`).
- `loaf kb glossary` CLI subcommand with five verbs: `upsert` writes or updates a canonical term; `check` resolves a term to canonical, avoided-alias, or unknown; `list` enumerates entries (one line per term, scriptable); `stabilize` promotes a candidate to canonical; `propose` writes a candidate (low-commitment, exploratory). Mutation policy lives in the verb names themselves rather than skill prose. Write commands (`upsert`, `stabilize`, `propose`) fail fast in Linear-native mode with the exact spec error verbatim; read commands (`list`, `check`) work in both modes.
- Domain glossary KB convention at `docs/knowledge/glossary.md` with `type: glossary` frontmatter and four sections: `## Canonical Terms`, `## Candidates`, `## Relationships`, `## Flagged ambiguities`. Lazy creation — the file is written only on the first successful `upsert`/`stabilize`/`propose`, never on `check` or `list`.
- `content/templates/grilling.md` shared interview-protocol template covering the relentless-interview / decision-tree / recommend-per-question / explore-when-answerable mechanics. Distributed by `targets.yaml` to `architecture` and `refactor-deepen` skills (NOT `shape` — deferred per separate idea). Mutation policy is delegated to the consuming skill; this template defines interview shape only.
- Plan artifact convention at `.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md`. Plans use temporal-record naming (same family as sessions, ideas, drafts, councils) — write-once snapshots of a `/refactor-deepen` interview, never updated. No `id` frontmatter field; the filename is the identity.

### Changed

- `/architecture` skill evolved to integrate with the glossary: reads existing glossary at interview start, challenges drifted/fuzzy language inline, offers `loaf kb glossary upsert` or `stabilize` when load-bearing terms surface during ADR interviews. Glossary side-effects are additive — never gating ADR creation. The `templates/adr.md` artifact format is preserved byte-identical.
- `cli/lib/kb/glossary.ts` parser is fence-aware and strict: tracks ``` and `~~~` code-fence state so heading-like content inside fences is preserved verbatim; rejects files missing required sections; rejects preamble prose before the first `## ` header; lossless parse/serialize round-trip on any accepted input.

### Internal

- 96 new tests in `cli/lib/kb/glossary.test.ts` and `cli/commands/kb-glossary.test.ts` covering lossless round-trip, fence handling (backtick + tilde), Linear-native gating in all three write verbs, and read-time-no-creation regressions.

## [2.0.0-dev.36] - 2026-04-30

### Fixed
- Validate flags early in release and let dry-run preview when no commits (4083f362)

## [2.0.0-dev.35] - 2026-04-30

### Added
- Add artifact journal entry types (TASK-103) (9443c355)

## [2.0.0-dev.34] - 2026-04-30

### Added

- Pre-commit `validate-commit` guard against bundled build-artifact leakage. Detects when staged paths include `plugins/`, `dist/`, `.claude-plugin/`, or root lockfiles (`package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lock`, `bun.lockb`) on commits whose subject does not indicate a build/release/deps/lockfile scope. Block message names the offending paths and shows the exact `git reset` + split-commit recipe. Bypass with `git commit --no-verify` when intentional.

### Changed

- `loaf release` now runs the project's full build script (`npm run build` for Node projects with a `build` script in `package.json`) instead of the content-only `loaf build`. Refreshes the bundled CLI (`plugins/loaf/bin/loaf`) so the version baked into the bundle matches the version in `package.json` after a release commit. Falls back to `loaf build` for non-Node projects.

### Fixed

- `extractUnreleasedEntries` (renamed to `extractUnreleasedBody`) preserves curated `[Unreleased]` body verbatim — including `### Added`, `### Changed`, `### Removed`, `### Fixed`, `### Internal` subsection headers — under the new versioned section. Previously filtered to list-item lines only, flattening the categorical structure. Caught when the comprehensive 6-section CHANGELOG drafted for v2.0.0-dev.33 was reduced to a single bulleted list.

## [2.0.0-dev.33] - 2026-04-30

- `loaf release --pre-merge` flag bundling `--no-tag --no-gh --base <auto-detected>` with 4-step base detection (explicit `--base` → open-PR base via `gh pr view` → `git config loaf.release.base` → default branch).
- `loaf release --post-merge` flag with 8-point guardrail checklist that finalizes a release after squash-merge: tag → push tag → GH release from CHANGELOG section → pull base → best-effort feature-branch cleanup. Light idempotency: each guardrail is rerun-safe; partial-failure aborts produce actionable manual-fix messages naming the exact recovery command.
- `loaf release --version-file <path>` repeatable CLI flag for ad-hoc version-file selection, complementing declared `release.versionFiles` in `.agents/loaf.json` for monorepo layouts (e.g., `["backend/pyproject.toml", "frontend/package.json"]`).
- Release-only PR classifier in `workflow-pre-pr`: a PR whose diff is exactly version-file paths + `CHANGELOG.md` with a non-empty `## [<version>]` section bypasses the empty-`[Unreleased]` block. Enables release-only PRs on repos with protected default branches.
- `loaf release` commit subject is now `chore: release v<semver>` (was `release: vX.Y.Z`). Conventional-Commits compliant; passes `@commitlint/config-conventional` without rewording. `workflow-pre-pr` and `validate-push` accept the new shape as a pre-merge escape hatch (shape-validated, not prefix-only — `chore: release notes draft` is still rejected).
- `loaf release` preserves curated `[Unreleased]` entries when present: existing list items are copied verbatim under the new `## [X.Y.Z]` header and auto-generation does not run. Resolves the recurring overwrite/jargon friction observed in dev.31 and dev.32.
- `loaf release` re-inserts the `- _No unreleased changes yet._` stub under fresh `[Unreleased]` after each release, so subsequent `gh pr create` does not block on an "empty" section.
- `/loaf:release` skill collapses Step 4 to `loaf release --pre-merge --bump <type> --yes` and Step 6 to `loaf release --post-merge`. Replaces the prior manual `git tag` / `git push --tags` / `gh release create` / `git checkout` / `git pull` / `git branch -d` sequence.
- CI `Build Distributions` workflow now verifies build-artifact freshness instead of auto-committing to `main`. Fails loudly when `dist/`, `plugins/`, or `.claude-plugin/` are out of sync with source. Also runs on `pull_request` so drift is caught during PR review, not only after merge. Removes the `GH013` auto-push rejection that had been failing every push to `main`.
- `release` from the accepted Conventional Commits types in `validate-commit` (Loaf-specific extension; not commitlint-compatible). The `chore: release v<semver>` shape replaces it cleanly.
- Orphan `content/hooks/pre-tool/workflow-pre-pr.sh` — no longer wired; `loaf check --hook workflow-pre-pr` auto-dispatches to the TS path.
- `loaf check workflow-pre-pr`: empty-section detector under `[Unreleased]` now mirrors `extractUnreleasedEntries` and discards the `- _No unreleased changes yet._` stub before checking for curated entries. Previously, the stub (a markdown list item by design) was counted as a real entry, which would have allowed feature PRs that forget to add changelog entries to silently pass.
- Unified base-branch resolver via `skipPRLookup?` option in `cli/lib/release/base.ts`. Replaces the divergent `resolveBaseForPostMerge` that had drifted into `post-merge.ts`. One resolver now serves `--pre-merge`, `--post-merge` (skips PR tier — the PR is closed/merged at that point), and the release-only PR classifier.
- Regression coverage added across the spec: `validate-commit` AI-attribution path-token pass cases + structured-attribution reject cases, `loaf release` end-to-end commit subject assertion (real commit, not `--dry-run`), post-merge guardrails 4/5/7/8 + idempotency rerun, base-detection 4-step precedence, monorepo declared-file resolution, release-only PR classifier mixed-diff disqualification.

## [2.0.0-dev.32] - 2026-04-29

Note: An earlier iteration of this release explored a configurable soul catalog with a `loaf soul` CLI; that work was reviewed in-flight and pivoted away from before merge — the lore decoupling stands, the soul layer does not. See the SPEC-033 archive for the full exploration.

### Changed
- Agent profile prompts (`implementer`, `reviewer`, `researcher`, `librarian`) describe themselves functionally — no Warden/Fellowship lore in profile bodies.
- Council references and skill prose now use profile types (`implementer`/`reviewer`/`researcher`/`librarian`/`orchestrator`); fellowship vocabulary is stripped from agent-facing skill content.
- `ARCHITECTURE.md` and `docs/knowledge/skill-architecture.md` reframed around the two-layer model: profiles for mechanics, skills for knowledge.

### Removed
- The deprecated `content/templates/soul.md` template.

## [2.0.0-dev.31] - 2026-04-28

### Added
- `--session-id <id>` flag on `loaf session log`, `loaf session archive`, and `loaf session enrich` for explicit session targeting independent of git branch.

### Fixed
- Session journal misrouting: `loaf session log` now routes by `claude_session_id` first, then hook stdin payload, then branch fallback. Resolves silent corruption observed during the v2.0.0-dev.30 release where post-merge wrap entries landed in stopped sessions instead of the active one.
- `loaf session log --from-hook --session-id <id>` with empty stdin now honors the explicit `--session-id` override instead of silently no-opping.

### Changed
- Branch-fallback session routing emits a stderr warning so misroutes are visible instead of silent. Pass `--session-id` to silence the warning.

### Internal
- Session lookup helpers extracted to a new `cli/lib/session/` module (`store.ts` for persistence primitives, `find.ts` for finders, `resolve.ts` for the 3-tier resolution chain).

## [2.0.0-dev.30] - 2026-04-24

### Fixed
- Escape regex literals in opencode runtime plugin (b9357605)
- Post-ADR-010 doctor + version followups (7ef8ab1b)

## [2.0.0-dev.29] - 2026-04-22

### Added
- Linear-native routing in implement skill (parent + sub-issue) (1c12a442)
- Mode-aware linear reconciliation checks in housekeeping (ae130564)
- Linear-native mode in breakdown skill (parent + sub-issues) (2ad67e30)

## [2.0.0-dev.28] - 2026-04-22

### Added
- Enforce project symlinks and migrate user content on loaf install (0abf44bd)
- Add loaf doctor command for alignment checks (23b787e0)

### Changed
- Remap fenced-section targets to canonical .agents/AGENTS.md (4ff26006)

### Fixed
- Isolate check.test.ts fixtures and serialize vitest file runs (89f62d5d)

## [2.0.0-dev.27] - 2026-04-11

### Added
- `loaf session enrich` CLI command — reviews JSONL conversation logs via librarian agent, fills in missing journal entries (decisions, discoveries, context)
- JSONL extractor module (`cli/lib/journal/extractor.ts`) — filters conversation logs, discovers subagent transcripts, enforces 100KB summary cap
- `LOAF_ENRICHMENT` hook isolation — prevents enrichment agent from creating spurious session files
- Wrap skill Step 0: enrichment before wrap-up generation
- Housekeeping enrichment pass for stopped/done sessions + `.agents/tmp/` cleanup

### Changed
- Session status `complete` renamed to `done`, `paused` removed (stopped covers it)
- Session statuses: `active | stopped | done | blocked | archived`

## [2.0.0-dev.26] - 2026-04-10

### Added
- `loaf session housekeeping` command — orphan detection, split consolidation, age-based archival, spec linkage repair
- `.loaf-state` trigger mechanism — `SessionEnd` flags housekeeping due, `SessionStart` surfaces nudge
- `/wrap` skill — interactive+scripted session close with `loaf session end --wrap`
- `loaf session context for-compact` — PreCompact journal flush + nudge instructions (replaces `compact.sh`)
- `loaf session context for-resumption` — PostCompact rich resumption context
- Librarian agent profile — Ent lore, behavioral contract, `Read + Edit (.agents/)` tool scope
- `TaskCompleted` session hook — auto-logs task completions to session journal
- `UserPromptSubmit` hook — injects Implementation Principles on every prompt
- `claude_session_id`-first session lookup with split consolidation on start

### Changed
- All hooks moved from `plugin.json` to `hooks/hooks.json` (`plugin.json` silently drops non-matcher events)
- Absorb `context-archiver` agent into Librarian profile (decisions persist to spec changelog)
- Journal `PostToolUse` hooks consolidated: `git commit` + `gh pr` (specific `if` conditions)
- `UserPromptSubmit` hook uses `type: command` (not `type: prompt` — prompt type acts as gate/validator)
- Implementation Principles: question-guard, task-before-tool rule
- Journal Discipline: git events auto-logged by hooks, manual logging removed
- Release skill: `/wrap` runs after version bump, `AskUserQuestion` for all decisions, `/reflect` always post-merge

### Fixed
- `TaskCompleted` hook handler — uses `hook_event_name` (not `tool_name`), logs `task_description` for richer context
- `claude_session_id` priority over branch for session lookup
- `appendEntry` blank line handling after `session(stop)` markers

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
