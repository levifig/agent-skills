# Boring Reliable State And CLI Plan

Date: 2026-06-14 20:17
Status: Completed checkpoint
Scope: Single global SQLite state, project identity, migrations, repair UX, backup/export, backend mappings, and human/agent CLI contracts.

This plan paused opportunistic hardening and turned the remaining work into a measurable reliability contract. The current codebase has reached the checkpoint: the native Go runtime is authoritative, state lives in one global SQLite database, project identity is durable and path/name independent, and critical state/project/migration commands have JSON contracts, human output, repair guidance, docs, tests, and live dogfood evidence. Future work should use this contract as a regression gate rather than reopening the whole audit by default.

## Current Baseline

| Area | Current Evidence | Remaining Concern |
|---|---|---|
| Native runtime | `docs/reports/2026-06-10-native-go-cutover-test-map.md` maps public command coverage and native-only guards. | The map proves migration breadth, not the final reliability contract for state and CLI UX. |
| Global DB location | `README.md` and SPEC-040 document one global SQLite file under XDG data home, partitioned by stable `project_id`. | Need final audit that every state/project command reports this consistently in JSON and human output. |
| Project identity | `loaf project show/list/rename/move` support stable ID, friendly name, current path, and path move safeguards. | Need command-matrix verification that rename/move never creates accidental identities and gives actionable errors. |
| State health | `loaf state status/path/doctor` expose global scope, project details, diagnostics, and repair plans. | Need repair-plan command validation so every suggested command is executable in the diagnosed state mode. |
| Backup/export | `state backup`, `backup verify`, and `export all` verify integrity and include project identity context. | Need restore/import confidence story: backup verification is strong, but restore procedure and test coverage are not yet first-class. |
| Backend/Linear mappings | Doctor validates backend mapping drift, project-level mappings, unknown sync statuses, and Linear unmapped local tasks. | Need a fuller backend consistency policy: what is invalid, warning-only, repairable, exportable, or delegated to future Linear sync. |
| Agent JSON | Many commands include `contract_version`, scope, project identity, machine-readable errors, and stable empty arrays. | Need a command matrix that proves all critical state/project/migration paths return JSON when `--json` is requested, including failures. |
| Human CLI | Human output has improved scope/project/path/safety labels. | Need a deliberate UX pass for consistency, next actions, and terse but useful repair guidance. |

## Reliability Contract

A state/CLI surface is boring reliable only when all applicable checks below are true and verified by current tests or dogfood output.

### Storage And Identity

- The database path is one global SQLite file under XDG data home: `$XDG_DATA_HOME/loaf/loaf.sqlite`, with platform fallback handled by `PathResolver`.
- Project isolation is row-level via durable `project_id`; no new code treats per-project database files as the primary storage model.
- New project IDs are generated and stable; they are not derived from path, friendly name, remote URL, or branch.
- Friendly project name is mutable and independent of path and ID.
- Current path is mutable via `loaf project move`, with historical paths preserved in `project_paths`.
- Read-only project commands do not create databases or project identities.
- Rejected rename/move operations do not create databases, project rows, path rows, sidecars, or repo files.

### Schema And Migration

- Migrations are ordered, checksummed, transactional, and mirrored into schema docs when schema changes.
- Storage-home migration copies legacy DBs to XDG data home without deleting the source or overwriting an existing destination.
- Markdown migration dry-run does not create SQLite state; apply/resume imports without mutating source Markdown.
- Existing state commands reject invalid schema versions or checksum drift with doctor guidance.
- Any future schema change must include data-preservation tests and explicit repair/upgrade UX.

### Doctor And Repair

- `state doctor` is always safe to run and does not mutate unless an explicit safe `--fix` path is selected.
- `state doctor --json` returns a machine-readable status payload even when exiting nonzero for invalid state.
- Repair plans are non-mutating by default and include `code`, `diagnostic_code`, `description`, `safe`, `applied`, and any relevant `command` or `path`.
- Every repair-plan command is valid in the state mode that produced it. Invalid-state diagnostics must not suggest commands that refuse invalid state.
- Unsafe repairs require dry-run/apply separation, clear human labels, and JSON output.

### Backup, Export, And Restore

- `state backup` creates repository-external `.sqlite` backups, verifies them before success, and reports checksum, bytes, schema version, foreign-key check, integrity check, project count, and current project identity.
- `state backup verify` verifies an existing backup without consulting or mutating live state.
- `state export all --format json` and `state export all --json` produce internal project-scoped snapshots with table order, row counts, project identity, scope, and verification manifest.
- Markdown exports are explicit, deterministic, and boundary-validated when external-safe.
- Restore remains incomplete until there is either a documented manual restore procedure with validation commands or a guarded `state restore` command.

### Backend And Linear Consistency

- Backend mappings store only external identity metadata, not tokens or secrets.
- Project-level backend mappings are valid only when they point at the same durable project ID.
- Task/spec/session/etc. mappings reject unknown local entity kinds and missing local entities.
- Ambiguous mappings and unknown sync statuses are visible diagnostics.
- Linear-enabled projects warn about active local tasks without Linear mappings.
- Repair guidance distinguishes local database repairs from backend sync work that requires future Linear integration.

### Agentic JSON

- Any command that accepts `--json` returns JSON for success and validation/runtime failures.
- JSON errors include `contract_version`, `command`, and `error`.
- State/project/migration JSON success payloads include `contract_version` and global database scope when backed by SQLite.
- Critical project-aware JSON payloads include `project_id`, `project_name`, and `project_current_path` when available.
- Empty collections are stable arrays, not `null` or omitted, unless the field is explicitly optional.
- Exit codes are deterministic: success is `0`; invalid state or validation failures are nonzero while preserving JSON when requested.

### Human CLI

- Human output names the command, scope, database path, project name, project ID, and project path when relevant.
- Dry-run output says no changes were written.
- Apply output says exactly what changed and where.
- Repair output labels safe/manual/applied actions.
- Error text points at the next useful command without implying an unsafe mutation.
- Output is concise enough for humans to scan and structured enough for agents to scrape if JSON is not available.

## Command Matrix

This is the focused audit set for the next iteration. Each row should eventually have tests or dogfood evidence for success, JSON success, JSON failure, and no unintended mutation where applicable.

| Surface | Human Success | JSON Success | JSON Failure | No-Mutation Proof | Priority |
|---|---|---|---|---|---|
| `state path` | Known good | Known good | Needs sampled matrix | Does not create DB | Medium |
| `state status` | Known good | Known good | Needs sampled matrix | Does not create DB | High |
| `state init` | Covered | Covered | Needs sampled matrix | Re-run idempotent | Medium |
| `state doctor` | Improving | Improving | Invalid state returns JSON payload | Does not mutate unless `--fix` safe path | Critical |
| `state repair relationship-origin` | Covered | Covered | Needs sampled matrix | Dry-run/apply split | High |
| `state repair legacy-project-database` | Covered | Covered | Needs sampled matrix | Dry-run/apply split | High |
| `state backup` | Covered | Covered | Covered for missing/invalid state | Creates backup only outside repo | Critical |
| `state backup verify` | Covered | Covered | Covered for invalid/missing path | Does not read live state | Critical |
| `state export all` | Covered; `--json` alias added | Covered | Covered for invalid state and markdown alias misuse | Does not mutate DB or repo files | Critical |
| `state export triage` | Covered | Not applicable | Needs JSON-format misuse checks | Does not mutate DB or repo files | High |
| `state export spec/session/release-readiness` | Covered | Not applicable | Needs JSON-format misuse checks | Does not mutate DB or repo files | High |
| `migrate markdown --dry-run` | Covered | Covered | Needs sampled matrix | Does not create DB | Critical |
| `migrate markdown --apply/--resume` | Covered | Covered | Needs interrupted/resume confidence review | Preserves source Markdown | Critical |
| `migrate storage-home --dry-run` | Covered | Covered | Needs sampled matrix | Does not copy | High |
| `migrate storage-home --apply` | Covered | Covered | Covered for overwrite/partial copy | Preserves legacy source DB | High |
| `project show/list` | Covered | Covered | Covered for missing DB and unknown path | Does not create identity | Critical |
| `project rename --dry-run/apply` | Covered | Covered | Covered for missing DB and unknown path | Preserves project ID | Critical |
| `project move --dry-run/apply` | Covered | Covered | Covered for missing DB, unknown from, missing target | Preserves project ID; one current path | Critical |
| Backend mapping diagnostics | Human via doctor | JSON via doctor | Invalid state returns JSON payload | Diagnostics only | Critical |

Final status: all critical rows have current test or dogfood evidence in `docs/reports/2026-06-14-boring-reliable-completion-audit.md`.

## Focused Execution Plan

The previous shape was directionally right but broad enough to invite edge chasing. From here, use three gates and avoid starting later work until the current gate has evidence.

### Gate 1: Prove The Control Plane

Status: Complete as of 2026-06-14 via `TestRunnerStateControlPlaneJSONFailureMatrix`, `TestRunnerStateControlPlaneJSONSuccessMatrix`, and `TestRunnerStateControlPlaneMutationAndRepairSafeguards`.

Finish the command matrix for the commands that can damage trust fastest: `state status`, `state doctor`, `state export all`, `state backup verify`, `project show/list`, `project rename/move`, `migrate markdown --dry-run`, and `migrate storage-home --dry-run`.

Exit criteria:

- JSON failures are covered for validation errors, invalid state, and missing-state cases.
- JSON successes are covered for read-only and dry-run paths.
- Read-only and dry-run commands prove they do not create databases, project identities, sidecars, or repo files.
- Each failure includes `contract_version`, `command`, deterministic exit code, and useful error text.

### Gate 2: Make Recovery Boring

Status: Complete as of 2026-06-14. Manual restore is acceptable for pre-release because it is documented, covered by `TestRunnerStateBackupManualRestoreProcedure`, dogfooded against an isolated XDG data home from the primary checkout, and reinforced by `state backup verify` next-action guidance. A guarded `state restore` command can wait until repeated use proves the manual flow too clumsy.

Close the restore-confidence gap before polishing lower-risk CLI surfaces. Start with a documented manual restore procedure unless command-level restore proves necessary.

Exit criteria:

- A user can verify a backup, preserve the current global DB, restore the verified backup into the XDG data-home path, and run doctor/status checks without guessing.
- The procedure is backed by tests where practical and by dogfood output from the primary checkout.
- Restore guidance is visible from backup/doctor docs or output when state is unhealthy.

### Gate 3: Normalize Repair, Backend, And Human UX

Only after the core control plane and recovery path are proven, run the UX/policy pass across repair plans, backend/Linear diagnostics, and human output.

Exit criteria:

- Every repair-plan command is executable in the state mode that suggested it.
- Backend diagnostics clearly separate invalid local data, warning-only drift, and future Linear sync work.
- Human output consistently names scope, database path, project name, project ID, project path, mutation status, and next action when relevant.
- The completion audit maps each reliability-contract bullet to current tests, docs, or dogfood output.

## Remaining Work Tracks

### Track 1: Prove The Matrix

Add a focused regression harness or table-driven CLI test that runs the critical command matrix against temporary state. It should assert JSON parseability, command names, contract version, scope fields, exit codes, and no repo mutation for read-only commands.

Progress:

- 2026-06-14: `TestRunnerStateControlPlaneJSONFailureMatrix` covers sampled JSON failure contracts across `state`, `state repair`, `state backup verify`, `state export`, `project`, and `migrate` control-plane commands, including contract version, command name, error text, silent exit code, and no state database creation for pre-open/read-only failures.
- 2026-06-14: `TestRunnerStateControlPlaneJSONSuccessMatrix` covers JSON success and no-mutation contracts for initialized read-only surfaces (`state status`, `state doctor`, `state export all`, `project show`, `project list`), migration dry-runs (`state migrate markdown --dry-run`, `state migrate storage-home --dry-run`), and `state backup verify` without live state access.
- 2026-06-14: `TestRunnerStateControlPlaneMutationAndRepairSafeguards` covers project rename/move dry-run and apply safeguards plus repair dry-runs for relationship origins and legacy project databases, including durable project ID preservation, single current path after moves, dry-run table stability, and no archive writes during legacy repair previews.

Go/no-go: the matrix can be re-run with one command and failures identify the exact command contract that regressed.

### Track 2: Restore Confidence

Backup verification is strong, but restore is not yet a first-class story. Decide between:

- documented manual restore: copy verified backup to the global DB path after backing up the current DB, then run `state doctor`;
- or a guarded `loaf state restore <backup> --dry-run|--apply`.

Recommendation: start with documented manual restore plus tests around backup verification and doctor compatibility. Add a command only when the manual procedure proves too clumsy.

Progress:

- 2026-06-14: README and generated CLI reference guidance document the manual restore flow: verify the backup, preserve the current global DB, copy the verified backup into the XDG data-home DB path, then run `state doctor` and `state status`. `TestRunnerStateBackupManualRestoreProcedure` backs the flow by verifying a backup, preserving a changed live DB, copying the backup into the global path, and proving doctor/status report the restored project identity.
- 2026-06-14: Dogfooded the documented restore flow from the primary checkout with isolated `XDG_DATA_HOME`/`XDG_STATE_HOME`: `state backup verify` returned `verified: true`, `integrity_check: ok`, and `foreign_key_check: ok`; the live DB was preserved as `.before-restore`; copying the backup into the global DB path restored the baseline project identity; and both `state doctor --json` and `state status --json` returned `sqlite-ready`.
- 2026-06-14: Human `state backup verify` output now includes the safe restore next action: preserve the current database, copy the verified backup to `loaf state path`, then run `state doctor` and `state status`.
- 2026-06-14: Dogfooded `state backup verify --json` from the primary checkout after removing the isolated live DB. Verification remained read-only and now returns `restore_database_path`, `restore_preserve_path`, and `restore_validation_commands` for the current checkout, while human output prints the concrete restore target and preserve path. `TestRunnerStateBackupVerifyReportsGlobalProjects`, `TestRunnerStateBackupManualRestoreProcedure`, and the control-plane success matrix cover the contract without requiring a live DB.

Go/no-go: a user can recover from a bad global DB using a verified backup without guessing which files to copy or which checks to run.

### Track 3: Repair UX

Audit every `RepairAction` and human repair-plan line. Repair commands should be executable in the current state mode, dry-run first unless explicitly safe, and clear about whether they inspect, preview, apply, or require external sync.

Progress:

- 2026-06-14: `TestRunnerStateDoctorRepairPlanCommandsExecuteInDiagnosticMode` now turns doctor repair actions into executable CLI checks. It covers missing DB initialization, storage-home dry-run, legacy DB archive preview, schema drift inspection, SQLite invariant inspection, project path invariant listing, relationship-origin repair preview, invalid backend mapping inspection, backend drift export, Linear task mapping export, and local Markdown import preview. Invalid-state diagnostic commands are allowed to return the expected JSON exit code 1, but parser/refusal failures are caught.
- 2026-06-14: Repair actions now expose a `category` and `requires_external_sync` policy in JSON, while human `state doctor` output labels the action category. Backend mapping diagnostics are split between local backend-mapping audit work and Linear/external sync reconciliation, with `TestRepairPlanClassifiesBackendAndExternalSyncActions` proving that invalid local mappings do not masquerade as external sync and Linear-unmapped tasks are marked as external sync work.

Go/no-go: no repair plan points to a command that immediately fails for the same diagnostic state. Current command-bearing repair actions are covered; future repair actions need to be added to the executability matrix when introduced.

### Track 4: Backend/Linear Policy

Write and enforce a small policy for backend mappings:

- local DB invariants that make state invalid;
- warning diagnostics that indicate sync drift;
- external sync gaps that are future Linear work;
- fields that must never store secrets.

Progress:

- 2026-06-14: Backend-related diagnostics now expose `category`, `policy`, and `requires_external_sync` where relevant. Invalid backend mapping rows use `backend-mapping` / `invalid-local-data`, warning-only drift uses `backend-mapping` / `warning-drift`, and Linear task mapping gaps use `external-sync` / `external-sync-gap` with `requires_external_sync: true`. Human `state doctor` output renders these labels inline, and `TestRunnerStateDoctorLabelsBackendDiagnosticPolicy` verifies both human output and JSON fields.
- 2026-06-14: Dogfooded invalid backend mapping rows and Linear-unmapped local tasks from the primary checkout with isolated XDG homes. Human output already separated invalid local data from external sync work, but JSON diagnostics still forced agents to parse prose for affected rows. Backend and Linear diagnostics now include structured `details` payloads for fields such as `mapping_id`, `entity_kind`, `entity_id`, `external_id`, `row_count`, and `unmapped_task_count`; `TestInspectReportsInvalidBackendMappingMissingEntity`, backend warning tests, Linear warning tests, and `TestRunnerStateDoctorLabelsBackendDiagnosticPolicy` cover the public contract.

Go/no-go: doctor diagnostics and repair/export guidance make it obvious whether the next action is local repair, export/audit, or backend sync.

### Track 5: Human Output Pass

Run the human form of the command matrix and normalize output shape. Prefer terse blocks with command name, scope, project, path, status, and next action. Avoid feature explanations and avoid presenting dangerous operations as casual fixes.

Progress:

- 2026-06-14: `loaf project rename` and `loaf project move` human output now use the same high-risk mutation shape: command header, global database scope, database path, durable project ID, friendly name, current path, from/to values, and `applied: true|false`. Dry-run output includes a next action; apply output does not. `TestRunnerProjectRenameDryRunDoesNotWrite`, `TestRunnerProjectMoveDryRunDoesNotWrite`, and `TestRunnerProjectRenameAndMoveHumanApplyOutput` cover the new shape.
- 2026-06-14: `loaf state migrate markdown` and `loaf state migrate storage-home` human output now use the same migration shape: command header, global database scope, project import/migration scope, database path, project context, `applied: true|false`, and dry-run next actions. `TestRunnerStateMigrateMarkdownHumanDryRun`, `TestRunnerStateMigrateMarkdownApplyHuman`, `TestRunnerStateMigrateMarkdownResumeHuman`, `TestRunnerMigrateMarkdownUsesNativeAlias`, `TestRunnerStateMigrateStorageHomeCopiesLegacyDatabase`, and `TestRunnerMigrateStorageHomeUsesNativeAlias` cover the shape.
- 2026-06-14: Dogfooded the human-output matrix with isolated XDG homes and found `loaf project show|identity` and `loaf project list` still used older identity labels. They now share the normalized project identity shape: command header, global database scope, database path, durable project ID, friendly name, and project path. `TestRunnerProjectShowRenameAndMoveUseStableIdentity` covers the shape and alias command header.
- 2026-06-14: Dogfooded `loaf state path` and backup output with isolated XDG homes. `loaf state path` intentionally keeps raw-path default output for shell substitution and manual restore workflows, and now adds `--verbose` for human-oriented command, global scope, project root, and database path context. `TestRunnerDispatchesStatePathNatively`, `TestRunnerStateControlPlaneJSONFailureMatrix`, `TestRunnerStateHelpIsNative`, `TestRunnerAgentHelpIsNative`, and `TestGenerateCLIReferenceIncludesCurrentCommands` cover the behavior and docs surfaces.
- 2026-06-14: Continued backup dogfood across creation, verify, help, and failure output. Backup creation already reported global scope, database, backup path, checksum, verification, and project identity; it now also gives a concrete `loaf state backup verify <backup>` next action, and help/reference text names the global data-home backups directory. `TestRunnerStateBackupHumanOutput`, `TestRunnerNestedStateBackedHelpDoesNotParseAsOption`, `TestRunnerAgentHelpIsNative`, and `TestGenerateCLIReferenceIncludesCurrentCommands` cover the output and docs surfaces.
- 2026-06-14: Dogfooded `loaf state init`, `loaf state status`, and `loaf state doctor` in missing and healthy modes. Initialized state output now uses the same durable identity labels as project/migration surfaces: `project` for the stable ID, `project name` for the friendly name, and `project path` for the checkout path. `TestRunnerStateInitStatusAndDoctor`, `TestRunnerStateInitHumanOutputPrintsRepositoryExternalDatabaseWithoutSecrets`, `TestRunnerStateDoctorFixInitializesMissingDatabase`, and `TestRunnerStateDoctorRepairPlanCommandsExecuteInDiagnosticMode` cover the normalized output and repair flow.
- 2026-06-14: Dogfooded `loaf state export all`, markdown export surfaces, and report-generation aliases. JSON export already carried global database/project identity context; markdown exports now render a `Project Context` block so exported artifacts are self-identifying. External exports (`triage`, `release-readiness`) include scope, durable project ID, and friendly name without local project/database paths, while internal exports (`spec`, `session`) include local project and database paths. `TestExportTriageMarkdownReturnsExternalSafeSummary`, `TestExportReleaseReadinessMarkdownReturnsExternalSafeSummary`, `TestExportSpecMarkdownRendersSpecSnapshot`, `TestExportSessionMarkdownRendersSessionSummary`, `TestRunnerStateExportTriageMarkdown`, `TestRunnerStateExportReleaseReadinessMarkdown`, `TestRunnerStateExportSpecMarkdown`, `TestRunnerStateExportSessionMarkdown`, and report/export equality tests cover the shape.
- 2026-06-14: Sampled export/report failure modes and found `loaf report generate` advertised `--format markdown` but rejected all `--format` flags, and had no structured success contract for agents. `report generate` now accepts `--format markdown`, rejects other formats with machine-readable errors when `--json` is requested, and returns the existing `MarkdownExport` wrapper for `--json` success. `TestRunnerReportGenerateTriageAndReleaseReadinessMatchStateExports`, `TestRunnerReportGenerateJSONContracts`, `TestRunnerReportGenerateHelpNamesMarkdownFormat`, `TestRunnerAgentHelpIsNative`, and `TestRunnerGenerateCLIReferenceWritesSkillNatively` cover the parser, help, agent-help, and generated reference surfaces.
- 2026-06-14: Finished the narrow `state export` failure-mode pass by sampling missing state, invalid state, unsupported kinds, unsupported formats, and `--json` misuse across export kinds. The pass found an order-dependent parser error where `state export all --json --format markdown` fell through to a generic unsupported-format message while the reverse flag order reported the intended conflict. `TestRunnerStateExportJSONErrorsAreMachineReadable`, `TestRunnerStateExportRejectsMissingInvalidUnsupportedState`, and `TestRunnerStateControlPlaneJSONFailureMatrix` now cover the relevant JSON error contracts and flag-order conflict.

Go/no-go: a user can run the state/project/migration commands without reading docs and understand whether anything changed.

### Track 6: Completion Audit

Only after Tracks 1-5, run a requirement-by-requirement completion audit against:

- this reliability contract;
- SPEC-040 acceptance items;
- native cutover test map guardrails;
- current command output from the primary checkout and temporary fixtures.

Go/no-go: every requirement has current evidence, not just a historical changelog entry.

Progress:

- 2026-06-14: Started the completion audit in `docs/reports/2026-06-14-boring-reliable-completion-audit.md`, mapping the reliability contract to current tests, docs, and dogfood evidence. The first weak proof point was the backend mapping sensitive-value boundary: schema tests proved Loaf does not define dedicated sensitive storage columns, but doctor did not reject sensitive-looking values inside external identity fields. `backend-mapping-sensitive-value` now classifies those rows as invalid local backend-mapping data, and `TestInspectReportsSensitiveBackendMappingValues` covers the repair policy.
- 2026-06-14: Continued human-output dogfood from the primary checkout with isolated XDG homes. Missing and initialized state surfaces were readable, but a natural `loaf project move <from> <to> --dry-run` invocation failed as an unknown option because absolute paths were not accepted positionally. `loaf project move` now accepts positional absolute paths in addition to `--from/--to`, with the same preview/apply safeguards and JSON contracts. `TestRunnerProjectMoveAcceptsPositionalPaths` covers the new shape.
- 2026-06-14: Sampled human failure output for project commands against missing SQLite state. `project show`, `project list`, `project rename --dry-run`, and `project move --dry-run` all protected state correctly, but returned a terse missing-database message without the global database path or an inspect-first next action. Missing-state project errors now name the global database path and point to `loaf state status` or `loaf state init`, with `TestRunnerProjectMissingDatabaseHumanErrorsIncludeContext` covering the shared failure path.
- 2026-06-14: Sampled invalid schema state by drifting a migration checksum in an isolated primary-checkout database. `state doctor` correctly reported invalid state, but project commands still read identity data because their read-only opener only checked max schema version. Project commands now validate migration checksums before reading identity state and reject drift with the global database path plus `loaf state doctor` guidance. `TestRunnerProjectCommandsRejectSchemaChecksumDrift` covers human and JSON failures.
- 2026-06-14: Sampled project path invariant drift by making `projects.current_path` disagree with the current `project_paths` row. `state doctor` correctly marked the DB invalid, but `project show` displayed the stale path while `project list` showed the path-history row. Project-specific commands now reject invalid path invariants before showing or mutating a single identity, while `project list --json` remains available for doctor-recommended inspection. `TestRunnerProjectCommandsRejectPathInvariantMismatch` covers human and JSON failures plus the inspection path.
- 2026-06-14: Dogfooded Markdown import apply/resume from the primary checkout with isolated XDG homes. Dry-run did not create the global database, apply and repeated resume kept one spec/task/idea/relationship row each, and `.agents` source file hashes were unchanged. The pass found that apply and resume JSON payloads were indistinguishable once separated from argv context; `MarkdownMigrationResult.action` now reports `apply` or `resume`, human output prints the same action, and `TestRunnerStateMigrateMarkdownApplyJSON`, `TestRunnerStateMigrateMarkdownApplyHuman`, `TestRunnerStateMigrateMarkdownResumeJSON`, and `TestRunnerStateMigrateMarkdownResumeHuman` cover the contract and source preservation.
- 2026-06-14: Dogfooded backend/Linear repair follow-up exports from the primary checkout with isolated XDG homes. `state doctor --json` correctly reported backend mapping drift, ambiguous Linear mappings, and unmapped active local tasks, but the recommended `state export all --format json` snapshot dropped the diagnostic and repair-plan context that explained why the export existed. `ExportSnapshot` now carries `diagnostics` and `repair_plan`, the manifest reports their counts, and `TestRunnerStateDoctorRepairPlanCommandsExecuteInDiagnosticMode` verifies backend drift and Linear reconciliation exports preserve that context.
- 2026-06-14: Dogfooded stale compatibility export and local Markdown import warnings from the primary checkout with isolated XDG homes. The warnings were preserved in doctor and export JSON, but they had no category, policy, or structured details, unlike backend/Linear diagnostics. `local-markdown-not-imported` now reports `markdown-import/import-pending` with importable artifact counts and preview/apply commands, while `stale-compatibility-export` reports `compatibility-export/stale-export` with export/source identifiers and timestamps. `TestInspectWarnsWhenGlobalDatabaseHasNotImportedCurrentMarkdown`, `TestInspectWarnsWhenInitializedProjectHasUnimportedMarkdown`, `TestInspectWarnsOnStaleCompatibilityExports`, `TestRunnerStateExportAllCarriesWarningDiagnosticDetails`, and `TestRunnerReportListWarnsWhenGlobalDatabaseHasUnimportedMarkdown` cover the contract.
- 2026-06-14: Dogfooded backup/export/import JSON output from the primary checkout with isolated XDG homes. Backup, backup verify, export all, and Markdown apply already carried global/project context, but `state migrate markdown --dry-run --json` only returned artifact counts. Dry-run JSON now uses `MarkdownMigrationPreviewResult` to report global database scope, target database path, project import scope, project name/path, and `applied: false` while still leaving the SQLite database missing. `TestRunnerStateMigrateMarkdownJSONDryRunDoesNotCreateDatabase` and the control-plane success matrix cover the contract.
- 2026-06-14: Dogfooded human and JSON failures for backup/export/import command surfaces from the primary checkout with isolated XDG homes. JSON failures stayed concise and machine-readable, but human missing-state errors from `state backup` and Markdown `state export ...` commands were too terse to show the global DB target. Human missing-state output now includes global database scope, target database path, and safe next actions to inspect state or import local Markdown. `TestRunnerStateBackupRejectsMissingAndInvalidState` and `TestRunnerStateExportRejectsMissingInvalidUnsupportedState` cover the shape.
- 2026-06-14: Re-dogfooded release-readiness/triage/session report-generation aliases from the primary checkout with isolated XDG homes. Markdown output and alias equality were stable, but `loaf report generate ... --json` success wrappers only exposed Markdown metadata and content, unlike their machine-readable failure payloads. `MarkdownExport` now carries contract version, report command, global database scope, project export scope, and durable project identity. External report JSON omits local project/database paths, while internal session JSON retains them for agent routing. `TestRunnerReportGenerateJSONContracts`, `TestRunnerReportGenerateSessionAndSessionReportMatchStateExport`, and `TestRunnerReportGenerateRejectsMissingInvalidUnsupportedState` cover the contract.
- 2026-06-14: Dogfooded migration and repair JSON success envelopes from the primary checkout with isolated XDG homes. Markdown migration and repair envelopes already carried contract/scope/project context where available, but `state migrate storage-home --dry-run --json` did not report project identity after the global data-home database had already been initialized for the current project. Storage-home preview now performs a read-only status inspection when the destination DB exists and records the verified durable project ID, friendly name, and current path. `TestRunnerStateMigrateStorageHomeCopiesLegacyDatabase` covers the after-apply dry-run shape.
- 2026-06-14: Audited command aliases and generated help/reference output against the tightened JSON contracts. Runtime alias output was consistent, but `--agent-help` advertised top-level `migrate markdown|storage-home` without equivalent `state migrate ...` leaves, and report/migration JSON descriptions underspecified contract and project context fields. Agent help now exposes state migration leaf commands, migration/report JSON descriptions name contract/scope/project context, generated CLI reference output matches, and `TestRunnerAgentHelpIsNative`, `TestRunnerReportGenerateHelpNamesMarkdownFormat`, and `TestRunnerGenerateCLIReferenceWritesSkillNatively` guard the surface.
- 2026-06-14: Audited generated `dist/` and plugin CLI-reference artifacts plus install-facing runtime metadata for stale state-command descriptions. Top-level migration aliases still used vague generated-reference wording, and critical state control-plane agent-help/reference entries still used raw/details JSON descriptions. Agent help and generated CLI reference output now name the actual contract fields for `state path|init|status|doctor`, repair previews/applies, backups, backup verification restore guidance, top-level migration aliases, global database scope, and project identity. `TestRunnerAgentHelpIsNative` and `TestRunnerGenerateCLIReferenceWritesSkillNatively` guard the descriptions, while `npm run build` refreshed `content/skills/cli-reference`, all `dist/*/skills/cli-reference` copies, and `plugins/loaf/skills/cli-reference`.
- 2026-06-14: Continued the agent-help/raw-JSON audit across SQLite-backed session, task, spec, and report surfaces. The pass found a behavioral contract bug where `loaf session report --json` was advertised but rejected. `session report --json` now returns the local session `MarkdownExport` wrapper with `command: session report`, and session/task/spec/report agent help, command help, and generated CLI reference output name the relevant contract fields: global database scope, project identity, diagnostics, journal entries, relationships, events, compatibility counts, status transitions, and Markdown export content. `TestRunnerReportGenerateSessionAndSessionReportMatchStateExport`, `TestRunnerAgentHelpIsNative`, and `TestRunnerGenerateCLIReferenceWritesSkillNatively` guard the behavior and descriptions.
- 2026-06-14: Continued the agent-help/raw-JSON audit across SQLite-backed entity families: `brainstorm`, `idea`, `spark`, `tag`, `bundle`, and `link`. Their payload structs already exposed global/project context plus relationships, events, tag mutations, bundle membership, and source/target relationship IDs, but help surfaces still used vague JSON wording. Agent help, command help, and generated CLI reference output now name those fields directly; `TestRunnerAgentHelpIsNative`, `TestRunnerGenerateCLIReferenceWritesSkillNatively`, and rebuilt `bin/loaf --agent-help` dogfood guard the descriptions.
- 2026-06-14: Finished the remaining `--agent-help` raw-JSON pass across `kb`, `check`, `housekeeping`, and `trace`. These surfaces now name their concrete JSON content across agent help, command help, and generated CLI reference output: knowledge totals, coverage/staleness metadata, frontmatter diagnostics, QMD import status, hook pass/block results, cleanup sections/signals, traced entities, sources, relationships, and SQLite-backed project identity. `TestRunnerAgentHelpIsNative`, `TestRunnerGenerateCLIReferenceWritesSkillNatively`, and rebuilt `bin/loaf --agent-help` dogfood guard the descriptions.
- 2026-06-14: Ran the final terminal-help JSON wording sweep for critical state/project control-plane commands. Human `--help` for `state path|init|status|doctor|backup|backup verify`, `project list|show|rename|move`, guarded state repairs, and state/top-level migrations now names the same contract fields already present in agent help and generated CLI reference output. `TestRunnerStateAndProjectJSONHelpNamesContracts`, `TestRunnerGenerateCLIReferenceWritesSkillNatively`, and rebuilt `bin/loaf ... --help` dogfood guard the descriptions.
- 2026-06-14: Completed the final requirement-by-requirement audit in `docs/reports/2026-06-14-boring-reliable-completion-audit.md`. The audit ties the reliability contract to `go test ./...`, `npm run typecheck`, focused CLI/state test inventories, isolated XDG dogfood, SPEC-040 checked decisions/test conditions, native cutover guardrails, README restore guidance, and generated help/reference scans.

## Next Best Commit

No immediate implementation checkpoint is identified by this audit. Keep the reliability contract as the regression gate for future schema, state/project/migration CLI, JSON, backup/export/restore, and backend/Linear changes.
