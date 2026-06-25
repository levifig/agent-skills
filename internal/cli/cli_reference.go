package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type cliReferenceCommand struct {
	Name        string
	Description string
	Subcommands []cliReferenceSubcommand
	Options     []cliReferenceOption
}

type cliReferenceSubcommand struct {
	Name        string
	Description string
	Options     []cliReferenceOption
}

type cliReferenceOption struct {
	Flags       string
	Description string
}

func (r Runner) runGenerateCLIReference(args []string, out io.Writer, rootPath string) error {
	if len(args) > 0 {
		return fmt.Errorf("__generate-cli-ref does not accept arguments")
	}
	outputPath := filepath.Join(rootPath, "content", "skills", "cli-reference", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create CLI reference directory: %w", err)
	}
	if err := os.WriteFile(outputPath, []byte(generateCLIReferenceSkill(cliReferenceCommands())), 0o644); err != nil {
		return fmt.Errorf("write CLI reference: %w", err)
	}
	fmt.Fprintf(out, "Generated CLI reference: %s\n", outputPath)
	return nil
}

func cliReferenceCommands() []cliReferenceCommand {
	return []cliReferenceCommand{
		{
			Name:        "build",
			Description: "Build skill distributions for agent harnesses",
			Options: []cliReferenceOption{
				{Flags: "-t, --target <name>", Description: "Build a specific target only"},
			},
		},
		{
			Name:        "install",
			Description: "Install Loaf to detected AI tool configurations",
			Options: []cliReferenceOption{
				{Flags: "--to <target>", Description: `Target to install to (or "all")`},
				{Flags: "--upgrade", Description: "Update installed targets and apply deprecation-manifest cleanup"},
				{Flags: "-y, --yes", Description: "Assume 'yes' to safe migrations and destructive deprecation cleanup"},
				{Flags: "--no-yes", Description: "Force interactive prompts even when stdin is not a TTY (testing)"},
			},
		},
		{
			Name:        "init",
			Description: "Initialize a project with Loaf structure",
			Options: []cliReferenceOption{
				{Flags: "--no-symlinks", Description: "Skip symlink creation prompts"},
			},
		},
		{
			Name:        "release",
			Description: "Create a new release with changelog, version bump, and tag",
			Options: []cliReferenceOption{
				{Flags: "--dry-run", Description: "Preview release without making changes"},
				{Flags: "--bump <type>", Description: "Skip interactive bump choice (prerelease, release, major, minor, patch)"},
				{Flags: "--base <ref>", Description: "Use commits since <ref> instead of last tag (e.g. main)"},
				{Flags: "--tag", Description: "Force git tag creation (overrides --pre-merge default)"},
				{Flags: "--no-tag", Description: "Skip git tag creation"},
				{Flags: "--gh", Description: "Force GitHub release draft (overrides --pre-merge default)"},
				{Flags: "--no-gh", Description: "Skip GitHub release draft"},
				{Flags: "--pre-merge", Description: "Shortcut for --no-tag --no-gh --base <auto-detected>"},
				{Flags: "--post-merge", Description: "Finalize release after squash-merge"},
				{Flags: "--version-file <path>", Description: "Override version file path (repeatable). Replaces configured version files and root auto-detection."},
				{Flags: "-y, --yes", Description: "Skip confirmation prompt"},
			},
		},
		{
			Name:        "search",
			Description: "Search SQLite artifact bodies, journal entries, and indexed docs",
			Options: []cliReferenceOption{
				{Flags: "<query>", Description: "Search terms matched through SQLite FTS5"},
				{Flags: "--all-projects", Description: "Search every registered project instead of only the current project"},
				{Flags: "--limit <n>", Description: "Maximum results to return (default: 20)"},
				{Flags: "--json", Description: "Output tiered hits, stable entity addresses, snippets, global database scope, and project identity as JSON"},
			},
		},
		{
			Name:        "docs",
			Description: "Manage docs/ indexing",
			Subcommands: []cliReferenceSubcommand{
				{Name: "index", Description: "Index docs/ Markdown into SQLite FTS", Options: []cliReferenceOption{
					{Flags: "--rebuild", Description: "Rebuild current worktree docs index before scanning"},
					{Flags: "--json", Description: "Output indexed docs, counts, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "render",
			Description: "Maintain committed durable Markdown renders",
			Subcommands: []cliReferenceSubcommand{
				{Name: "sweep", Description: "Upgrade committed durable renders to the current renderer contract", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Report upgrade-needed files without rewriting them"},
					{Flags: "--json", Description: "Output scanned files, upgrade counts, drift counts, and target contract as JSON"},
				}},
			},
		},
		{
			Name:        "state",
			Description: "Manage native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "path", Description: "Print the resolved SQLite database path", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output contract version, database path, scope, and project root as JSON"},
					{Flags: "--verbose", Description: "Output command, scope, project root, and database path"},
				}},
				{Name: "status", Description: "Show SQLite readiness and markdown-only compatibility status", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output readiness mode, diagnostics, global database scope, and project identity as JSON"},
				}},
				{Name: "init", Description: "Initialize an empty SQLite state database", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output initialized status, global database scope, and project identity as JSON"},
				}},
				{Name: "doctor", Description: "Diagnose SQLite state health", Options: []cliReferenceOption{
					{Flags: "--fix", Description: "Initialize missing SQLite state when safe"},
					{Flags: "--dry-run", Description: "Show the repair plan without applying fixes"},
					{Flags: "--json", Description: "Output diagnostics, repair plan, global database scope, and project identity as JSON"},
				}},
				{Name: "repair legacy-project-database", Description: "Archive migrated per-project SQLite leftovers", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview archive paths without writing"},
					{Flags: "--apply", Description: "Move legacy SQLite files into the archive directory"},
					{Flags: "--json", Description: "Output archive plan/result, global database scope, and project identity as JSON"},
				}},
				{Name: "repair relationship-origin", Description: "Preview or apply guarded relationship provenance backfills", Options: []cliReferenceOption{
					{Flags: "--origin <imported|manual>", Description: "Provenance value to backfill"},
					{Flags: "--dry-run", Description: "Preview affected rows without writing"},
					{Flags: "--apply", Description: "Backfill missing origins after creating a SQLite backup"},
					{Flags: "--json", Description: "Output repair plan/result, global database scope, and project identity as JSON"},
				}},
				{Name: "migrate markdown", Description: "Import existing .agents Markdown artifacts into SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview import counts without creating a database"},
					{Flags: "--apply", Description: "Initialize SQLite and import Markdown artifacts"},
					{Flags: "--resume", Description: "Resume the Markdown import after an interrupted attempt"},
					{Flags: "--backup", Description: "Create SQLite and .agents rollback backups during apply or resume"},
					{Flags: "--remove-source", Description: "Remove ephemeral Markdown sources after a rollback backup"},
					{Flags: "--rollback <manifest>", Description: "Restore .agents files from a rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, scope, project context, counts, and rollback fields as JSON"},
				}},
				{Name: "migrate storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview the storage-home migration"},
					{Flags: "--apply", Description: "Copy the legacy database without deleting it"},
					{Flags: "--json", Description: "Output migration contract, global database paths, action, and project identity when available"},
				}},
				{Name: "migrate lifecycle-statuses", Description: "Normalize legacy lifecycle statuses in SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview status normalization on a temporary database copy"},
					{Flags: "--apply", Description: "Normalize live SQLite statuses after creating a backup"},
					{Flags: "--rollback <manifest>", Description: "Restore statuses from a lifecycle-statuses rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, project context, counts, backup, and rollback fields as JSON"},
				}},
				{Name: "backup", Description: "Create a SQLite database backup under the global data-home backups directory", Options: []cliReferenceOption{{Flags: "--json", Description: "Output backup verification, checksum, schema version, project count, and current project identity as JSON"}}},
				{Name: "backup verify", Description: "Verify an existing SQLite database backup", Options: []cliReferenceOption{{Flags: "--json", Description: "Output backup verification, restore guidance, schema version, and captured project identities as JSON"}}},
				{Name: "export", Description: "Export SQLite state for review or migration", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format for the selected export kind"}}},
				{Name: "export all", Description: "Export a complete project-scoped SQLite snapshot", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: json"}, {Flags: "--json", Description: "Alias for --format json"}}},
				{Name: "export triage", Description: "Export a triage summary from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export session", Description: "Export one session from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export spec", Description: "Export one spec from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export release-readiness", Description: "Export a release-readiness report from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
			},
		},
		{
			Name:        "session",
			Description: "Manage session journals and native SQLite session state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "start", Description: "Start or resume a session for the current branch", Options: []cliReferenceOption{
					{Flags: "--resume", Description: "Resume if possible"},
					{Flags: "--session-id <id>", Description: "Harness session ID"},
					{Flags: "--force", Description: "Ignore hook agent adoption guard"},
					{Flags: "--json", Description: "Output action, session, journal IDs, global database scope, and project identity as JSON"},
				}},
				{Name: "end", Description: "End, wrap, or clear a session", Options: []cliReferenceOption{
					{Flags: "--if-active", Description: "No-op when no active session exists"},
					{Flags: "--wrap", Description: "Mark as wrapped"},
					{Flags: "--from-hook", Description: "Read hook input"},
					{Flags: "--session-id <id>", Description: "Harness session ID"},
					{Flags: "--json", Description: "Output action/noop, session, journal IDs, global database scope, and project identity as JSON"},
				}},
				{Name: "archive", Description: "Archive a stopped or targeted session", Options: []cliReferenceOption{
					{Flags: "--branch <branch>", Description: "Branch to archive"},
					{Flags: "--session-id <id>", Description: "Harness session ID"},
					{Flags: "--json", Description: "Output archive result, affected sessions, global database scope, and project identity as JSON"},
				}},
				{Name: "list", Description: "List sessions", Options: []cliReferenceOption{
					{Flags: "--all", Description: "Include archived sessions"},
					{Flags: "--json", Description: "Output sessions, diagnostics, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one session", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output session details, journal entries, relationships, global database scope, and project identity as JSON"},
				}},
				{Name: "log", Description: "Append a session journal entry", Options: []cliReferenceOption{
					{Flags: "--from-hook", Description: "Read hook input"},
					{Flags: "--session-id <id>", Description: "Harness session ID"},
					{Flags: "--json", Description: "Output journal entry, linked session, global database scope, and project identity as JSON"},
				}},
				{Name: "report", Description: "Export a session report", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output export contract, command, project context, and markdown content as JSON"},
				}},
			},
		},
		{
			Name:        "project",
			Description: "Manage durable project identity",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List registered projects in the global SQLite database", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output database path, project IDs, friendly names, and current paths as JSON"},
				}},
				{Name: "show", Description: "Show the current project identity", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output project ID, friendly name, current path, and database path as JSON"},
				}},
				{Name: "identity", Description: "Alias for project show", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output project ID, friendly name, current path, and database path as JSON"},
				}},
				{Name: "rename", Description: "Rename the friendly project name", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Validate and preview without writing"},
					{Flags: "--json", Description: "Output project ID, friendly name, current path, database path, and applied status as JSON"},
				}},
				{Name: "move", Description: "Record a checkout path move", Options: []cliReferenceOption{
					{Flags: "<from> [to]", Description: "Previous and optional new absolute project paths"},
					{Flags: "--from <path>", Description: "Previous absolute project path"},
					{Flags: "--to <path>", Description: "New absolute project path; defaults to the current project root"},
					{Flags: "--dry-run", Description: "Validate and preview without writing"},
					{Flags: "--json", Description: "Output project ID, friendly name, current path, database path, and applied status as JSON"},
				}},
			},
		},
		{
			Name:        "migrate",
			Description: "Run native migration workflows",
			Subcommands: []cliReferenceSubcommand{
				{Name: "markdown", Description: "Import existing .agents Markdown artifacts into SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview import counts without creating a database"},
					{Flags: "--apply", Description: "Initialize SQLite and import Markdown artifacts"},
					{Flags: "--resume", Description: "Resume the Markdown import after an interrupted attempt"},
					{Flags: "--backup", Description: "Create SQLite and .agents rollback backups during apply or resume"},
					{Flags: "--remove-source", Description: "Remove ephemeral Markdown sources after a rollback backup"},
					{Flags: "--rollback <manifest>", Description: "Restore .agents files from a rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, scope, project context, counts, and rollback fields as JSON"},
				}},
				{Name: "storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview the storage-home migration"},
					{Flags: "--apply", Description: "Copy the legacy database without deleting it"},
					{Flags: "--json", Description: "Output migration contract, global database paths, action, and project identity when available"},
				}},
				{Name: "lifecycle-statuses", Description: "Normalize legacy lifecycle statuses in SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview status normalization on a temporary database copy"},
					{Flags: "--apply", Description: "Normalize live SQLite statuses after creating a backup"},
					{Flags: "--rollback <manifest>", Description: "Restore statuses from a lifecycle-statuses rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, project context, counts, backup, and rollback fields as JSON"},
				}},
				{Name: "worktree-storage", Description: "Move linked-worktree .agents state to the main worktree", Options: []cliReferenceOption{
					{Flags: "--apply", Description: "Perform the migration; dry-run is the default"},
					{Flags: "--force-from-worktree", Description: "On conflict, keep the worktree-local copy"},
					{Flags: "--force-from-main", Description: "On conflict, keep the main-worktree copy"},
				}},
			},
		},
		{
			Name:        "task",
			Description: "Manage project tasks",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "Show task board grouped by status", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output tasks, diagnostics, global database scope, and project identity as JSON"},
					{Flags: "--active", Description: "Hide completed tasks"},
					{Flags: "--status <status>", Description: "Only show tasks with status: " + validTaskListStatusText()},
				}},
				{Name: "show", Description: "Display a single task's details", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output task details, relationships, global database scope, and project identity as JSON"},
				}},
				{Name: "status", Description: "Show task summary counts"},
				{Name: "create", Description: "Create a new task", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Task title"},
					{Flags: "--spec <id>", Description: "Associated spec ID (e.g., SPEC-010)"},
					{Flags: "--priority <level>", Description: "Priority level: " + validTaskPriorityText()},
					{Flags: "--depends-on <ids>", Description: "Comma-separated task IDs"},
					{Flags: "--json", Description: "Output created task, event, global database scope, and project identity as JSON"},
				}},
				{Name: "update", Description: "Update a task's metadata", Options: []cliReferenceOption{
					{Flags: "--status <status>", Description: "New status: " + validTaskStatusText()},
					{Flags: "--priority <level>", Description: "New priority: " + validTaskPriorityText()},
					{Flags: "--depends-on <ids>", Description: "Replace depends_on (comma-separated task IDs)"},
					{Flags: "--session <file>", Description: `Set or clear session reference (use "none" to clear)`},
					{Flags: "--spec <id>", Description: "Set or change associated spec"},
					{Flags: "--json", Description: "Output updated task, event, global database scope, and project identity as JSON"},
				}},
				{Name: "archive", Description: "Archive completed tasks through the task lifecycle", Options: []cliReferenceOption{
					{Flags: "--spec <id>", Description: "Archive all done tasks for a spec"},
					{Flags: "--json", Description: "Output archive result, archived tasks, global database scope, and project identity as JSON"},
				}},
				{Name: "refresh", Description: "Compatibility: rebuild the Markdown task index from task/spec files", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output compatibility summary as JSON"},
				}},
				{Name: "sync", Description: "Compatibility: sync the Markdown task index and task files", Options: []cliReferenceOption{
					{Flags: "--import", Description: "Import orphan .md files not in the index"},
					{Flags: "--push", Description: "Push compatibility index metadata into .md frontmatter"},
					{Flags: "--json", Description: "Output compatibility summary as JSON"},
				}},
			},
		},
		{
			Name:        "spec",
			Description: "Manage project specs",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "Show specs with status and task counts", Options: []cliReferenceOption{{Flags: "--json", Description: "Output specs, diagnostics, task counts, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show spec details", Options: []cliReferenceOption{{Flags: "--json", Description: "Output spec details, task counts, relationships, global database scope, and project identity as JSON"}}},
				{Name: "render", Description: "Render deterministic spec Markdown to the XDG cache", Options: []cliReferenceOption{{Flags: "--json", Description: "Output render path, content hash, contract, global database scope, and project identity as JSON"}}},
				{Name: "finalize", Description: "Write deterministic spec Markdown to its tracked git location", Options: []cliReferenceOption{{Flags: "--json", Description: "Output render path, content hash, contract, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a completed spec", Options: []cliReferenceOption{{Flags: "--json", Description: "Output archive result, archived specs, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "report",
			Description: "Manage durable reports (research, audits, investigations)",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List reports", Options: []cliReferenceOption{
					{Flags: "--type <type>", Description: "Filter by report type"},
					{Flags: "--status <status>", Description: "Filter by status; Loaf lifecycle statuses: draft, done, archived"},
					{Flags: "--json", Description: "Output reports, diagnostics, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one report", Options: []cliReferenceOption{{Flags: "--json", Description: "Output report details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "render", Description: "Render deterministic report Markdown to the XDG cache", Options: []cliReferenceOption{{Flags: "--json", Description: "Output render path, content hash, contract, global database scope, and project identity as JSON"}}},
				{Name: "generate", Description: "Generate a report from state", Options: []cliReferenceOption{
					{Flags: "--format <format>", Description: "Output format: markdown"},
					{Flags: "--json", Description: "Output contract, command, project context, and markdown content as JSON"},
				}},
				{Name: "create", Description: "Create a report draft", Options: []cliReferenceOption{
					{Flags: "--type <type>", Description: "Report type"},
					{Flags: "--source <source>", Description: "Report source"},
					{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
					{Flags: "--body -", Description: "Read Markdown body from stdin"},
					{Flags: "--message <text>", Description: "Use inline Markdown body text"},
					{Flags: "--json", Description: "Output created report, event, global database scope, and project identity as JSON"},
				}},
				{Name: "finalize", Description: "Mark a report draft as done and write its deterministic tracked render", Options: []cliReferenceOption{{Flags: "--json", Description: "Output report status transition, render path, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a done report", Options: []cliReferenceOption{{Flags: "--json", Description: "Output report status transition, event, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "finding",
			Description: "Manage report findings and verdicts in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List findings", Options: []cliReferenceOption{
					{Flags: "--report <report>", Description: "Filter by parent report"},
					{Flags: "--run <run>", Description: "Filter by provenance run"},
					{Flags: "--status <status>", Description: "Filter by status: " + strings.Join(state.FindingStatuses(), ", ")},
					{Flags: "--severity <severity>", Description: "Filter by severity: " + strings.Join(state.FindingSeverities(), ", ")},
					{Flags: "--confidence <confidence>", Description: "Filter by confidence: " + strings.Join(state.FindingConfidences(), ", ")},
					{Flags: "--dimension <dimension>", Description: "Filter by freeform finding dimension"},
					{Flags: "--format <format>", Description: "Output format: json, csv, markdown, html"},
					{Flags: "--json", Description: "Alias for --format json"},
				}},
				{Name: "show", Description: "Show one finding", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: json, csv, markdown, html"}, {Flags: "--json", Description: "Alias for --format json"}}},
				{Name: "create", Description: "Create a finding under a report", Options: []cliReferenceOption{
					{Flags: "--report <report>", Description: "Parent report"},
					{Flags: "--run <run>", Description: "Optional run provenance row"},
					{Flags: "--title <title>", Description: "Finding title"},
					{Flags: "--status <status>", Description: "Initial status: " + strings.Join(state.FindingStatuses(), ", ")},
					{Flags: "--severity <severity>", Description: "Severity: " + strings.Join(state.FindingSeverities(), ", ")},
					{Flags: "--confidence <confidence>", Description: "Confidence: " + strings.Join(state.FindingConfidences(), ", ")},
					{Flags: "--dimension <dimension>", Description: "Freeform finding dimension"},
					{Flags: "--path <path>", Description: "File path or artifact location"},
					{Flags: "--line-start <line>", Description: "Starting line number"},
					{Flags: "--line-end <line>", Description: "Ending line number"},
					{Flags: "--symbol <symbol>", Description: "Symbol or object location"},
					{Flags: "--metadata <json>", Description: "JSON metadata"},
					{Flags: "--body-file <path>", Description: "Read finding narrative from a UTF-8 file"},
					{Flags: "--body -", Description: "Read finding narrative from stdin"},
					{Flags: "--message <text>", Description: "Use inline finding narrative text"},
					{Flags: "--json", Description: "Output created finding, event, global database scope, and project identity as JSON"},
				}},
				{Name: "verdict", Description: "Record a finding verdict", Options: []cliReferenceOption{
					{Flags: "--outcome <outcome>", Description: "Verdict outcome: " + strings.Join(state.VerdictOutcomes(), ", ")},
					{Flags: "--rationale <text>", Description: "Verdict rationale"},
					{Flags: "--run <run>", Description: "Optional run provenance row"},
					{Flags: "--notes <text>", Description: "Reproduction notes"},
					{Flags: "--metadata <json>", Description: "JSON metadata"},
					{Flags: "--json", Description: "Output verdict, updated finding, event, global database scope, and project identity as JSON"},
				}},
				{Name: "import-json", Description: "Import row-shaped finding and verdict JSON", Options: []cliReferenceOption{
					{Flags: "--report <report>", Description: "Existing report ref, or slug for a new import report"},
					{Flags: "--report-type <type>", Description: "Report type used when creating a missing report"},
					{Flags: "--source <source>", Description: "Source label used when creating a missing report"},
					{Flags: "--run <run>", Description: "Optional run provenance row for imported rows"},
					{Flags: "--findings <path>", Description: "Row-shaped findings JSON; may be repeated"},
					{Flags: "--verdicts <path>", Description: "Row-shaped verdicts JSON; may be repeated"},
					{Flags: "--json", Description: "Output import counts, files, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "run",
			Description: "Manage provenance runs for generated findings and reports",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List provenance runs", Options: []cliReferenceOption{
					{Flags: "--status <status>", Description: "Filter by status: " + strings.Join(state.RunStatuses(), ", ")},
					{Flags: "--generator <ref>", Description: "Filter by generator reference"},
					{Flags: "--json", Description: "Output runs, filters, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one provenance run", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output run metadata, relationships, global database scope, and project identity as JSON"},
				}},
				{Name: "create", Description: "Create a provenance run row without storing generator code", Options: []cliReferenceOption{
					{Flags: "--generator <ref>", Description: "Generator reference or name"},
					{Flags: "--version <version>", Description: "Generator version"},
					{Flags: "--hash <hash>", Description: "Generator content hash"},
					{Flags: "--status <status>", Description: "Initial status: " + strings.Join(state.RunStatuses(), ", ")},
					{Flags: "--metadata <json>", Description: "JSON metadata"},
					{Flags: "--report <report>", Description: "Optional produced report relationship"},
					{Flags: "--json", Description: "Output created run, event, global database scope, and project identity as JSON"},
				}},
				{Name: "complete", Description: "Complete, fail, or archive a provenance run", Options: []cliReferenceOption{
					{Flags: "--status <status>", Description: "Completion status: completed, failed, archived"},
					{Flags: "--metadata <json>", Description: "Replace run metadata with JSON"},
					{Flags: "--json", Description: "Output run transition, event, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "plan",
			Description: "Manage plans in native SQLite state",
			Subcommands: nativeArtifactReferenceSubcommands("plan"),
		},
		{
			Name:        "handoff",
			Description: "Manage handoffs in native SQLite state",
			Subcommands: nativeArtifactReferenceSubcommands("handoff"),
		},
		{
			Name:        "council",
			Description: "Manage councils in native SQLite state",
			Subcommands: nativeArtifactReferenceSubcommands("council"),
		},
		{
			Name:        "kb",
			Description: "Knowledge base management",
			Subcommands: []cliReferenceSubcommand{
				{Name: "glossary", Description: "Domain glossary mutation and lookup"},
				{Name: "validate", Description: "Validate knowledge file frontmatter", Options: []cliReferenceOption{{Flags: "--json", Description: "Output per-file frontmatter errors and warnings as JSON"}}},
				{Name: "status", Description: "Show knowledge base overview", Options: []cliReferenceOption{{Flags: "--json", Description: "Output knowledge file totals, coverage counts, stale count, review age, and directories as JSON"}}},
				{Name: "check", Description: "Check knowledge file staleness against git history", Options: []cliReferenceOption{
					{Flags: "--file <path>", Description: "Reverse lookup: find knowledge files covering this path"},
					{Flags: "--json", Description: "Output per-file staleness, coverage, commit, and review metadata as JSON"},
				}},
				{Name: "review", Description: "Mark a knowledge file as reviewed today", Options: []cliReferenceOption{{Flags: "--json", Description: "Output updated knowledge frontmatter as JSON"}}},
				{Name: "init", Description: "Initialize knowledge base directories and QMD collections", Options: []cliReferenceOption{{Flags: "--json", Description: "Output directory actions, config status, and QMD collections as JSON"}}},
				{Name: "import", Description: "Import external project knowledge via QMD collection", Options: []cliReferenceOption{
					{Flags: "--path <path>", Description: "Path to the external project's knowledge directory"},
					{Flags: "--json", Description: "Output QMD import collection status or import error as JSON"},
				}},
			},
		},
		{
			Name:        "setup",
			Description: "One-step bootstrap: init + build + install",
		},
		{
			Name:        "version",
			Description: "Show version info and project statistics",
		},
		{
			Name:        "housekeeping",
			Description: "Scan project artifacts and recommend housekeeping actions",
			Options: []cliReferenceOption{
				{Flags: "--dry-run", Description: "Show recommendations without prompting for actions"},
				{Flags: "--json", Description: "Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON"},
				{Flags: "--sessions", Description: "Only review sessions"},
				{Flags: "--specs", Description: "Only review specs"},
				{Flags: "--plans", Description: "Only review plans"},
				{Flags: "--drafts", Description: "Only review drafts"},
				{Flags: "--handoffs", Description: "Only review handoffs"},
			},
		},
		{
			Name:        "trace",
			Description: "Trace relationships for one state entity",
			Options: []cliReferenceOption{
				{Flags: "--json", Description: "Output traced entity, sources, relationships, global database scope, and project identity as JSON"},
			},
		},
		{
			Name:        "brainstorm",
			Description: "Manage brainstorms in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "capture", Description: "Capture a brainstorm in SQLite state", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Brainstorm title"},
					{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
					{Flags: "--body -", Description: "Read Markdown body from stdin"},
					{Flags: "--message <text>", Description: "Use inline Markdown body text"},
					{Flags: "--json", Description: "Output created brainstorm, event, global database scope, and project identity as JSON"},
				}},
				{Name: "list", Description: "List brainstorms from SQLite state", Options: []cliReferenceOption{
					{Flags: "--all", Description: "Include archived brainstorms"},
					{Flags: "--status <status>", Description: "Filter by status"},
					{Flags: "--json", Description: "Output brainstorms, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one brainstorm from SQLite state", Options: []cliReferenceOption{{Flags: "--json", Description: "Output brainstorm details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "promote", Description: "Record brainstorm-to-idea promotion", Options: []cliReferenceOption{
					{Flags: "--to-idea <idea>", Description: "Target idea"},
					{Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"},
				}},
				{Name: "archive", Description: "Archive one or more brainstorms", Options: []cliReferenceOption{
					{Flags: "--reason <text>", Description: "Archive reason"},
					{Flags: "--json", Description: "Output archive result, archived brainstorms, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "idea",
			Description: "Manage ideas in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List ideas from SQLite state", Options: []cliReferenceOption{
					{Flags: "--all", Description: "Include done and archived ideas"},
					{Flags: "--status <status>", Description: "Filter by status"},
					{Flags: "--json", Description: "Output ideas, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one idea from SQLite state", Options: []cliReferenceOption{{Flags: "--json", Description: "Output idea details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "capture", Description: "Capture an idea in SQLite state", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Idea title"},
					{Flags: "--json", Description: "Output created idea, event, global database scope, and project identity as JSON"},
				}},
				{Name: "promote", Description: "Record idea-to-spec promotion", Options: []cliReferenceOption{
					{Flags: "--to-spec <spec>", Description: "Target spec"},
					{Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"},
				}},
				{Name: "resolve", Description: "Resolve an idea by linking it to another entity", Options: []cliReferenceOption{
					{Flags: "--by <entity>", Description: "Resolving entity"},
					{Flags: "--json", Description: "Output resolution relationship, event, global database scope, and project identity as JSON"},
				}},
				{Name: "archive", Description: "Archive one or more ideas", Options: []cliReferenceOption{
					{Flags: "--reason <text>", Description: "Archive reason"},
					{Flags: "--json", Description: "Output archive result, archived ideas, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "spark",
			Description: "Manage sparks in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List sparks from SQLite state", Options: []cliReferenceOption{
					{Flags: "--all", Description: "Include done sparks"},
					{Flags: "--status <status>", Description: "Filter by status"},
					{Flags: "--json", Description: "Output sparks, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one spark from SQLite state", Options: []cliReferenceOption{{Flags: "--json", Description: "Output spark details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "capture", Description: "Capture a spark in SQLite state", Options: []cliReferenceOption{
					{Flags: "--scope <scope>", Description: "Spark scope"},
					{Flags: "--text <text>", Description: "Spark text"},
					{Flags: "--json", Description: "Output created spark, event, global database scope, and project identity as JSON"},
				}},
				{Name: "resolve", Description: "Resolve a spark", Options: []cliReferenceOption{
					{Flags: "--reason <text>", Description: "Resolution reason"},
					{Flags: "--json", Description: "Output resolution relationship, event, global database scope, and project identity as JSON"},
				}},
				{Name: "promote", Description: "Record spark-to-idea promotion", Options: []cliReferenceOption{
					{Flags: "--to-idea <idea>", Description: "Target idea"},
					{Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "tag",
			Description: "Manage tags in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List tags from SQLite state", Options: []cliReferenceOption{{Flags: "--json", Description: "Output tags, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show entities with a tag", Options: []cliReferenceOption{{Flags: "--json", Description: "Output tagged entities, global database scope, and project identity as JSON"}}},
				{Name: "add", Description: "Add a tag to an entity", Options: []cliReferenceOption{{Flags: "--json", Description: "Output tag mutation, entity, global database scope, and project identity as JSON"}}},
				{Name: "remove", Description: "Remove a tag from an entity", Options: []cliReferenceOption{{Flags: "--json", Description: "Output tag mutation, entity, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "bundle",
			Description: "Manage bundles in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List bundles from SQLite state", Options: []cliReferenceOption{{Flags: "--json", Description: "Output bundles, global database scope, and project identity as JSON"}}},
				{Name: "create", Description: "Create a bundle", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Bundle title"},
					{Flags: "--tags <tags>", Description: "Comma-separated tag query"},
					{Flags: "--json", Description: "Output created bundle, tags, global database scope, and project identity as JSON"},
				}},
				{Name: "update", Description: "Update a bundle", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Bundle title"},
					{Flags: "--tags <tags>", Description: "Comma-separated tag query"},
					{Flags: "--json", Description: "Output updated bundle, tags, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one bundle", Options: []cliReferenceOption{{Flags: "--json", Description: "Output bundle details, members, global database scope, and project identity as JSON"}}},
				{Name: "add", Description: "Add an entity to a bundle", Options: []cliReferenceOption{{Flags: "--json", Description: "Output bundle membership result, global database scope, and project identity as JSON"}}},
				{Name: "remove", Description: "Remove an entity from a bundle", Options: []cliReferenceOption{{Flags: "--json", Description: "Output bundle membership result, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "link",
			Description: "Manage explicit relationships in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "create", Description: "Create an explicit relationship", Options: []cliReferenceOption{
					{Flags: "--from <entity>", Description: "Source entity"},
					{Flags: "--to <entity>", Description: "Target entity"},
					{Flags: "--type <type>", Description: "Relationship type"},
					{Flags: "--reason <text>", Description: "Relationship reason"},
					{Flags: "--json", Description: "Output relationship ID, source/target, global database scope, and project identity as JSON"},
				}},
				{Name: "list", Description: "List relationships for one entity", Options: []cliReferenceOption{{Flags: "--json", Description: "Output relationships, global database scope, and project identity as JSON"}}},
				{Name: "remove", Description: "Remove an explicit relationship", Options: []cliReferenceOption{
					{Flags: "--from <entity>", Description: "Source entity"},
					{Flags: "--to <entity>", Description: "Target entity"},
					{Flags: "--type <type>", Description: "Relationship type"},
					{Flags: "--json", Description: "Output removed relationship ID, global database scope, and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "check",
			Description: "Run enforcement hook checks",
			Options: []cliReferenceOption{
				{Flags: "--hook <id>", Description: "Registered hook ID to run"},
				{Flags: "--json", Description: "Output hook result, pass/block status, exit code, warnings, errors, and findings as JSON"},
			},
		},
	}
}

func generateCLIReferenceSkill(commands []cliReferenceCommand) string {
	sections := []string{`---
name: cli-reference
description: >-
  Documents the Loaf CLI commands and when to use them. Reference for
  {{IMPLEMENT_CMD}}, {{ORCHESTRATE_CMD}}, and all loaf
  subcommands. Use when you need to know which CLI command to invoke.
  Not for skill documentation (use the skill's own SKILL.md) or for
  understanding build internals.
---

# Loaf CLI Reference

## Contents
- Global Commands
- Command Reference
- Command Substitution Reference
- Quick Decision Guide

Quick reference for all Loaf CLI commands. Each command includes its purpose, common usage patterns, and when to use it.

**Note:** This file is auto-generated from native CLI reference metadata. Do not edit manually.
`,
		`## Global Commands

### {{IMPLEMENT_CMD}}
Orchestrates implementation sessions through agent delegation and batch execution.

**Use when:**
- User asks "implement this" or "start working on TASK-XXX"
- Starting a new spec implementation
- Resuming work after context loss

**Usage:**
- {{IMPLEMENT_CMD}} TASK-XXX - Load task, auto-create session
- {{IMPLEMENT_CMD}} SPEC-XXX - Resolve all tasks, build dependency waves
- {{IMPLEMENT_CMD}} TASK-XXX..YYY - Expand range, build waves
- {{IMPLEMENT_CMD}} "description" - Ad-hoc session

### {{ORCHESTRATE_CMD}}
Coordinates multi-agent work: agent delegation, session management, Linear integration.

**Use when:**
- Managing sessions and delegating to agents
- Running council workflows
- Coordinating cross-cutting work

---
`,
	}

	for _, cmd := range commands {
		sections = append(sections, generateCLIReferenceCommandSection(cmd))
	}
	for _, cmd := range supplementalCLIReferenceCommands(commands) {
		sections = append(sections, generateCLIReferenceCommandSection(cmd))
	}

	sections = append(sections, strings.Join([]string{
		"## Command Substitution Reference",
		"",
		"The following placeholders are substituted at build time per target:",
		"",
		"| Placeholder | Claude Code | OpenCode | Cursor |",
		"|-------------|-------------|----------|--------|",
		"| `{{IMPLEMENT_CMD}}` | `/implement` | `/implement` | `@loaf/implement` |",
		"| `{{ORCHESTRATE_CMD}}` | `/implement` | `/implement` | `@loaf/implement` |",
		"",
		"---",
		"",
		"## Quick Decision Guide",
		"",
		"**Need to start working?** -> `{{IMPLEMENT_CMD}} TASK-XXX`",
		"",
		"**Need to continue after restart?** -> `loaf session start` then `{{IMPLEMENT_CMD}}`",
		"",
		"**Need to coordinate agents?** -> `{{ORCHESTRATE_CMD}}`",
		"",
		"**Made changes to skills?** -> `loaf build && loaf install --to <target>`",
		"",
		"**Want to see what's in progress?** -> `loaf task list --active`",
		"",
		"**Ready to archive completed work?** -> `loaf task archive TASK-XXX`",
		"",
		"**Need to check knowledge freshness?** -> `loaf kb check`",
		"",
	}, "\n"))

	return strings.Join(sections, "\n")
}

func generateCLIReferenceCommandSection(cmd cliReferenceCommand) string {
	var parts []string
	subcommands := cliReferenceSubcommands(cmd)

	parts = append(parts,
		fmt.Sprintf("## %s Management", capitalizeFirst(cmd.Name)),
		"",
		fmt.Sprintf("### `loaf %s`", cmd.Name),
		cmd.Description,
		"",
	)

	if guidance := cliReferenceCommandGuidance(cmd.Name); guidance != "" {
		parts = append(parts, guidance, "")
	}

	if len(subcommands) > 0 {
		parts = append(parts,
			"**Subcommands:**",
			"",
			"| Subcommand | Purpose |",
			"|------------|---------|",
		)
		for _, sub := range subcommands {
			parts = append(parts, fmt.Sprintf("| `loaf %s %s` | %s |", cmd.Name, sub.Name, sub.Description))
		}
		parts = append(parts, "")

		var withOptions []cliReferenceSubcommand
		for _, sub := range subcommands {
			if len(sub.Options) > 0 {
				withOptions = append(withOptions, sub)
			}
		}
		if len(withOptions) > 0 {
			parts = append(parts, "**Options:**", "")
			for _, sub := range withOptions {
				parts = append(parts, fmt.Sprintf("- `loaf %s %s`:", cmd.Name, sub.Name))
				for _, opt := range sub.Options {
					parts = append(parts, fmt.Sprintf("  - `%s` - %s", opt.Flags, opt.Description))
				}
				parts = append(parts, "")
			}
		}
	}

	if len(cmd.Options) > 0 {
		parts = append(parts, "**Options:**", "")
		for _, opt := range cmd.Options {
			parts = append(parts, fmt.Sprintf("- `%s` - %s", opt.Flags, opt.Description))
		}
		parts = append(parts, "")
	}

	parts = append(parts, "**Usage:**", "```bash")
	if examples := cliReferenceCommandUsageExamples(cmd.Name); len(examples) > 0 {
		parts = append(parts, examples...)
	} else if len(subcommands) > 0 {
		limit := len(subcommands)
		if limit > 3 {
			limit = 3
		}
		for _, sub := range subcommands[:limit] {
			parts = append(parts, fmt.Sprintf("loaf %s %s", cmd.Name, sub.Name))
		}
	} else {
		parts = append(parts, fmt.Sprintf("loaf %s", cmd.Name))
	}
	parts = append(parts, "```", "", "---", "")

	return strings.Join(parts, "\n")
}

func cliReferenceSubcommands(cmd cliReferenceCommand) []cliReferenceSubcommand {
	if cmd.Name != "session" {
		return cmd.Subcommands
	}
	return withMissingCLIReferenceSubcommands(cmd.Subcommands, []cliReferenceSubcommand{
		{Name: "show", Description: "Display one session from state"},
		{Name: "report", Description: "Generate a session report from SQLite state"},
	})
}

func nativeArtifactReferenceSubcommands(kind string) []cliReferenceSubcommand {
	options := []cliReferenceOption{
		{Flags: "--title <title>", Description: "Artifact title"},
		{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
		{Flags: "--body -", Description: "Read Markdown body from stdin"},
		{Flags: "--message <text>", Description: "Use inline Markdown body text"},
	}
	switch kind {
	case "plan", "council":
		options = append(options, cliReferenceOption{Flags: "--spec <spec>", Description: "Optional related spec"})
	case "handoff":
		options = append(options,
			cliReferenceOption{Flags: "--session <session>", Description: "Optional related session"},
			cliReferenceOption{Flags: "--task <task>", Description: "Optional related task"},
		)
	}
	options = append(options, cliReferenceOption{Flags: "--json", Description: "Output created artifact, event, global database scope, and project identity as JSON"})
	return []cliReferenceSubcommand{
		{Name: "new", Description: "Create a " + kind + " in SQLite state", Options: options},
		{Name: "show", Description: "Show one " + kind + " from SQLite state", Options: []cliReferenceOption{{Flags: "--json", Description: "Output artifact details, relationships, global database scope, and project identity as JSON"}}},
		{Name: "list", Description: "List " + kind + "s from SQLite state", Options: []cliReferenceOption{
			{Flags: "--all", Description: "Include archived artifacts"},
			{Flags: "--status <status>", Description: "Filter by status"},
			{Flags: "--json", Description: "Output artifacts, global database scope, and project identity as JSON"},
		}},
		{Name: "link", Description: "Link a " + kind + " to another entity", Options: []cliReferenceOption{
			{Flags: "--type <type>", Description: "Relationship type; defaults to related_to"},
			{Flags: "--reason <text>", Description: "Relationship reason"},
			{Flags: "--json", Description: "Output relationship ID, source/target, global database scope, and project identity as JSON"},
		}},
	}
}

func supplementalCLIReferenceCommands(commands []cliReferenceCommand) []cliReferenceCommand {
	for _, cmd := range commands {
		if cmd.Name == "report" {
			return nil
		}
	}
	return []cliReferenceCommand{{
		Name:        "report",
		Description: "Manage report state and generated report output.",
		Subcommands: []cliReferenceSubcommand{
			{Name: "list", Description: "List reports from SQLite state or Markdown compatibility files"},
			{Name: "create", Description: "Create a draft report row in SQLite state"},
			{Name: "finalize", Description: "Transition a draft report to done"},
			{Name: "archive", Description: "Transition a done report to archived"},
			{Name: "generate", Description: "Generate report Markdown from SQLite state to stdout"},
		},
	}}
}

func withMissingCLIReferenceSubcommands(subcommands []cliReferenceSubcommand, supplemental []cliReferenceSubcommand) []cliReferenceSubcommand {
	seen := map[string]bool{}
	for _, sub := range subcommands {
		seen[sub.Name] = true
	}
	result := append([]cliReferenceSubcommand{}, subcommands...)
	for _, sub := range supplemental {
		if !seen[sub.Name] {
			result = append(result, sub)
		}
	}
	return result
}

func cliReferenceCommandGuidance(commandName string) string {
	switch commandName {
	case "task":
		return "In SQLite-backed projects, task metadata mutations go through the Go-native\nstate store. Markdown task files and `TASKS.json` remain compatibility/source\nartifacts during migration; do not edit them directly for lifecycle changes."
	case "spec":
		return "Spec lifecycle changes go through `loaf spec` commands. Markdown spec files\nremain the authored prose artifact, while SQLite state carries operational\nstatus and relationship data when initialized."
	case "session":
		return "Session list/show/log/report commands are SQLite-aware. Prefer these commands\nover manual session frontmatter edits when changing lifecycle or journal state."
	case "report":
		return "In SQLite-backed projects, report lifecycle state is stored in SQLite. Use\ngenerated report commands for review output; create authored Markdown reports\nonly when a durable prose artifact is explicitly needed."
	case "state":
		return "Existing TypeScript-era projects can keep running supported commands in\nmarkdown-only compatibility mode until SQLite is initialized. Use\n`loaf state migrate markdown --apply` to import `.agents/` Markdown into SQLite\nwithout rewriting the source Markdown files." +
			"\n\nManual restore from a backup is explicit until a guarded restore command exists:\nverify the backup with `loaf state backup verify <backup>`, preserve the current\n`$(loaf state path)` file, copy the verified backup to that path, then run\n`loaf state doctor` and `loaf state status`." +
			"\nFor agents, `loaf state backup verify <backup> --json` also returns\n`restore_database_path`, `restore_preserve_path`, and\n`restore_validation_commands` for the current checkout."
	case "project":
		return "Project IDs are stable SQLite identities, not path or name hashes. Use\n`loaf project rename --dry-run` for display-name previews and\n`loaf project move --dry-run` before recording checkout path moves."
	case "migrate":
		return "`loaf migrate markdown` is the upgrade path for existing `.agents/`\nprojects with no SQLite database. Start with `--dry-run`, then use `--apply`\nwhen the artifact counts and unimported file classifications look right."
	default:
		return ""
	}
}

func cliReferenceCommandUsageExamples(commandName string) []string {
	switch commandName {
	case "state":
		return []string{
			"loaf state status",
			"loaf state migrate markdown --dry-run",
			"loaf state migrate markdown --apply",
			"loaf state migrate lifecycle-statuses --dry-run",
			"loaf state backup",
			"loaf state backup verify /path/to/backup.sqlite",
			"loaf state status",
		}
	case "project":
		return []string{
			"loaf project show",
			"loaf project identity --json",
			"loaf project rename \"Loaf\" --dry-run",
			"loaf project rename \"Loaf\"",
			"loaf project move /old/path/to/loaf /new/path/to/loaf --dry-run",
			"loaf project move --from /old/path/to/loaf --dry-run",
			"loaf project move --from /old/path/to/loaf",
			"loaf project show --json",
		}
	case "migrate":
		return []string{
			"loaf migrate markdown --dry-run",
			"loaf migrate markdown --apply",
			"loaf migrate storage-home --dry-run",
			"loaf migrate lifecycle-statuses --dry-run",
		}
	case "render":
		return []string{
			"loaf render sweep --dry-run",
			"loaf render sweep --json",
			"loaf check --hook render-drift --json",
		}
	case "report":
		return []string{
			"loaf report list",
			"loaf report create release-readiness --type audit --source manual",
			"loaf report finalize report-release-readiness",
			"loaf report archive report-release-readiness",
			"loaf report generate release-readiness",
		}
	default:
		return nil
	}
}

func capitalizeFirst(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
