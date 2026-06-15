# Boring Reliable Completion Audit

Date: 2026-06-14 20:17
Status: Complete
Scope: Current evidence for `docs/reports/2026-06-14-boring-reliable-state-cli-plan.md`.

This audit maps the reliability contract to current evidence from tests, docs, SPEC-040, the native cutover test map, and live primary-checkout dogfood. It closes the current boring-reliable state/CLI goal; future schema or CLI changes still need to re-run the relevant contract rows before release.

## Storage And Identity

| Requirement | Evidence | Status |
|---|---|---|
| One global SQLite file under XDG data home | `docs/ARCHITECTURE.md`, `README.md`, `internal/state/path.go`, `internal/state/status_test.go` | Proven |
| Row-level project isolation via durable `project_id` | `docs/schema/0001_initial.sql`, `docs/schema/0003_project_identity_and_relationship_origin.sql`, `internal/state/schema_test.go`, `internal/state/status_test.go` | Proven |
| Generated project IDs are stable and not path/name derived | `internal/state/status_test.go`, project identity CLI tests | Proven |
| Friendly project name is mutable and independent | project rename tests in `internal/cli/cli_test.go`; schema docs include `friendly_name` | Proven |
| Current path is mutable and historical paths are preserved | project move tests; path-invariant rejection tests in `internal/cli/cli_test.go`; `docs/schema/0004_project_path_current_uniqueness.sql` | Proven |
| Read-only project commands do not create state | control-plane no-mutation tests in `internal/cli/cli_test.go` | Proven |
| Rejected rename/move operations do not create DB rows or repo files | project rename/move safeguard tests in `internal/cli/cli_test.go` | Proven |

## Schema And Migration

| Requirement | Evidence | Status |
|---|---|---|
| Migrations are ordered, checksummed, transactional, and mirrored into schema docs | migration/schema tests in `internal/state`; schema mirror test in `internal/state/schema_test.go` | Proven |
| Storage-home migration copies legacy DBs without deleting source or overwriting destination | storage-home CLI and migration tests in `internal/cli/cli_test.go` | Proven |
| Markdown dry-run does not create SQLite state; apply/resume preserves source Markdown | markdown migration control-plane and apply/resume tests in `internal/cli/cli_test.go` | Proven |
| Invalid schema versions or checksum drift produce doctor guidance | migration/status tests in `internal/state`; control-plane failure matrix; project-command schema-drift rejection test in `internal/cli/cli_test.go` | Proven |
| Future schema changes include data-preservation tests and repair/upgrade UX | Reliability contract and schema mirror/checksum tests establish the release guard; must be rechecked with each future schema commit | Standing guardrail |

## Doctor And Repair

| Requirement | Evidence | Status |
|---|---|---|
| `state doctor` is safe by default | doctor dry-run/no-mutation tests in `internal/cli/cli_test.go` | Proven |
| `state doctor --json` returns machine-readable invalid-state payloads | doctor JSON invalid-state tests in `internal/cli/cli_test.go` | Proven |
| Repair plans expose code, diagnostic code, description, safety, applied state, and command/path | `internal/state/status.go`; repair-plan tests in `internal/state/status_test.go` and `internal/cli/cli_test.go` | Proven |
| Every repair-plan command is executable in the state mode that produced it | `TestRunnerStateDoctorRepairPlanCommandsExecuteInDiagnosticMode` | Proven |
| Unsafe repairs require dry-run/apply separation and JSON output | repair tests for relationship-origin and legacy-project-database in `internal/cli/cli_test.go` | Proven |

## Backup, Export, And Restore

| Requirement | Evidence | Status |
|---|---|---|
| Backup creates repository-external SQLite copies and verifies checksum, bytes, schema, FK, integrity, project count, and current identity | backup tests in `internal/state/backup_test.go` and CLI backup tests in `internal/cli/cli_test.go` | Proven |
| Backup verify checks an existing backup without consulting or mutating live state | `TestRunnerStateControlPlaneJSONSuccessMatrix`; backup verify tests | Proven |
| JSON export snapshots include table order, row counts, identity, scope, and verification manifest | export tests in `internal/state/export_test.go` and CLI matrix tests | Proven |
| Markdown exports are deterministic and boundary-validated when external-safe | markdown export tests in `internal/state/export_test.go` and CLI export tests | Proven |
| Restore has documented manual procedure with validation commands | `README.md`; `TestRunnerStateBackupManualRestoreProcedure`; dogfood notes in the plan | Proven for manual restore |
| Backup verification exposes concrete restore targets without live DB access | `BackupVerificationResult` restore fields; `TestRunnerStateBackupVerifyReportsGlobalProjects`; `TestRunnerStateControlPlaneJSONSuccessMatrix`; live isolated `state backup verify --json` dogfood after removing the live DB | Proven |

## Backend And Linear Consistency

| Requirement | Evidence | Status |
|---|---|---|
| Backend mappings store only external identity metadata, not sensitive values | schema column guard in `internal/state/schema_test.go`; runtime diagnostic `backend-mapping-sensitive-value` in `internal/state/status.go`; `TestInspectReportsSensitiveBackendMappingValues` | Proven in this checkpoint |
| Project-level backend mappings are valid only for the same durable project ID | `TestInspectAcceptsProjectBackendMapping`; `TestInspectRejectsProjectBackendMappingToDifferentProjectID` | Proven |
| Local mappings reject unknown entity kinds and missing local entities | backend mapping invariant tests in `internal/state/status_test.go` | Proven |
| Ambiguous mappings and unknown sync statuses are visible diagnostics | backend mapping warning tests in `internal/state/status_test.go` | Proven |
| Linear-enabled projects warn about active local tasks without Linear mappings | `TestInspectWarnsOnUnmappedLocalTasksWhenLinearEnabled` | Proven |
| Repair guidance separates local DB repair from future backend sync work | diagnostic policy tests in `internal/state/status_test.go` and `internal/cli/cli_test.go`; live isolated `state doctor --json` dogfood for invalid backend rows and Linear-unmapped tasks | Proven |

## Agentic JSON

| Requirement | Evidence | Status |
|---|---|---|
| Commands with `--json` return JSON on success and validation/runtime failure | control-plane success and failure matrix tests in `internal/cli/cli_test.go` | Proven for critical matrix |
| JSON errors include contract version, command, and error | JSON failure matrix tests in `internal/cli/cli_test.go` | Proven |
| SQLite-backed success payloads include contract version and global scope | JSON success matrix and state/project command tests | Proven for critical matrix |
| Project-aware payloads include ID, name, and current path when available | state/project/backup/export tests | Proven for critical matrix |
| Empty collections are stable arrays | repair-plan and export JSON tests | Proven |
| Exit codes are deterministic while preserving JSON | JSON failure matrix tests | Proven for critical matrix |
| Backend/Linear diagnostics include structured routing details | `Diagnostic.Details` in `internal/state/status.go`; backend/Linear detail assertions in `internal/state/status_test.go` and `internal/cli/cli_test.go`; live isolated `state doctor --json` dogfood | Proven |

## Human CLI

| Requirement | Evidence | Status |
|---|---|---|
| Human output names command, scope, database, project name, ID, and path where relevant | human-output tests in `internal/cli/cli_test.go` | Proven for critical matrix |
| Dry-run output says no changes were written | migration/project/repair dry-run tests | Proven |
| Apply output says what changed and where | migration/project/repair apply tests | Proven |
| Repair output labels safe/manual/applied actions | doctor/repair tests | Proven |
| Error text points at next useful command without implying unsafe mutation | JSON/human failure tests and backup verify guidance tests; missing-state project command dogfood fix | Proven for critical matrix |
| Output remains concise and agent-scrapable when JSON is unavailable | human-output matrix tests plus live isolated dogfood of state/project/doctor/backup/export surfaces; positional `project move`, missing-state errors, and terminal-help wording were fixed during the audit | Proven for critical matrix |

## First Weak Proof Point Fixed

The first weak item was the backend policy requirement that mappings store only external identity metadata. Before this checkpoint, schema tests proved there were no dedicated credential columns, but runtime state could still contain sensitive-looking values inside `external_id` or `external_url` and pass doctor checks.

This checkpoint adds a non-mutating doctor diagnostic, `backend-mapping-sensitive-value`, classifies it as `backend-mapping` / `invalid-local-data`, and keeps the repair guidance as a manual local backend-mapping audit rather than external sync work.

## Backend/Linear Checkpoint

The latest backend/Linear sampling pass found that human output and repair plans already separated invalid local backend data from external sync work, but JSON consumers still had to parse diagnostic prose to identify affected rows or counts.

This checkpoint adds structured diagnostic `details` for backend mapping and Linear sync findings, covering affected fields, row counts, mapping IDs, local entity identifiers, external identifiers, and unmapped task counts. Live dogfood through the rebuilt `bin/loaf` confirmed those details appear in `state doctor --json` for both invalid backend mapping rows and Linear-unmapped local tasks.

## Latest Checkpoint

The latest completion-audit pass focused on the last generic terminal-help JSON wording for state, project, repair, backup, and migration commands. Agent help and generated CLI reference had already been tightened, but human `--help` still showed plain `Output JSON` for critical control-plane commands.

This checkpoint makes terminal help name the actual JSON content for `state path|init|status|doctor|backup|backup verify`, `project list|show|rename|move`, guarded state repairs, and state/top-level migrations. `TestRunnerStateAndProjectJSONHelpNamesContracts` guards the human help, `TestRunnerGenerateCLIReferenceWritesSkillNatively` guards the tightened project reference descriptions, and live rebuilt `bin/loaf ... --help` dogfood confirms those surfaces no longer contain generic `Output JSON` wording.

## Final Completion Audit

Final verification from the primary checkout:

- `go test ./...` passed.
- `npm run typecheck` passed.
- Focused CLI/state inventories confirmed the regression harness still covers state control-plane JSON success/failure, mutation safeguards, repair-plan command executability, backup restore guidance, generated CLI reference output, and state/project JSON help wording.
- Live isolated XDG dogfood confirmed `state path --json`, `state status --json`, `state doctor --dry-run --json`, `state init --json`, `project show`, `project list --json`, `state doctor`, `state backup`, `state backup verify --json`, and `state export all --json` expose global database scope, durable project identity, diagnostics, repair plans, backup verification, and export manifest fields.
- Live help scans confirmed critical state/project/migration help no longer uses generic JSON wording, and `bin/loaf --agent-help` no longer advertises raw or generic JSON descriptions.
- `README.md` documents the manual backup restore flow with validation commands, and `TestRunnerStateBackupManualRestoreProcedure` guards it.
- SPEC-040 open questions and test conditions are checked off for global XDG storage, generated project identity, project rename/move semantics, Markdown import preservation, backup/export behavior, traceability, Linear/private mapping policy, and no secret storage.
- The native cutover test map records guardrails for XDG data-home migration, schema checksums/mirror docs, Markdown import dry-run/apply/resume, state health/export, backup, and native command-surface coverage.

## Residual Guardrails

No immediate implementation gap remains in the current audit scope. The standing guardrails are:

- Any new schema migration must add data-preservation tests, update schema docs, and define repair/upgrade UX before release.
- Any new state/project/migration JSON flag must join the critical command matrix or explicitly document why it is outside it.
- Any new backend/Linear integration work must keep local DB repair separate from external sync and must not store tokens, secrets, or sensitive values in SQLite.
