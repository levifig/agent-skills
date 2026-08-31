package cli

import (
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type agentHelpOption struct {
	Flags        string `json:"flags"`
	Description  string `json:"description"`
	DefaultValue any    `json:"defaultValue,omitempty"`
}

type agentHelpSubcommand struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Options     []agentHelpOption `json:"options,omitempty"`
}

type agentHelpCommand struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Subcommands []agentHelpSubcommand `json:"subcommands,omitempty"`
	Options     []agentHelpOption     `json:"options,omitempty"`
}

type agentHelpDocument struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Commands    []agentHelpCommand `json:"commands"`
}

func writeAgentHelpJSON(out io.Writer) error {
	return writeJSON(out, agentHelpDocument{
		Name:        "loaf",
		Description: "Loaf — An Opinionated Agentic Framework",
		Commands:    agentHelpCommands(),
	})
}

func agentHelpCommands() []agentHelpCommand {
	return []agentHelpCommand{
		{
			Name:        "build",
			Description: "Build Loaf content targets",
			Options: []agentHelpOption{
				{Flags: "-t, --target <name>", Description: "Build a specific target only"},
			},
		},
		{
			Name:        "init",
			Description: "Scaffold project agent files",
			Options: []agentHelpOption{
				{Flags: "--no-symlinks", Description: "Skip project instruction symlink setup"},
			},
		},
		{
			Name:        "install",
			Description: "Onboard Loaf into a folder or a not-yet-installed agent tool; refreshing an existing install is loaf upgrade",
			Options: []agentHelpOption{
				{Flags: "--to <target>", Description: "Target to install to, or all"},
				{Flags: "-y, --yes", Description: "Assume yes to safe project-file symlink migrations and destructive deprecation cleanup"},
				{Flags: "--no-yes", Description: "Force prompt-style declines in non-interactive mode"},
			},
		},
		{
			Name:        "upgrade",
			Description: "Refresh Loaf in place: harness content sync plus deprecation cleanup anywhere, and project-surface refresh only inside a detected Loaf repo",
			Options: []agentHelpOption{
				{Flags: "--to <target>", Description: "Filter the harness sync to one already-installed target, or all; an uninstalled target is an error pointing at loaf install --to"},
				{Flags: "--dry-run", Description: "Report the deterministic non-mutating plan, including whether the project part is in scope, without writing files, manifests, config, or state"},
				{Flags: "--json", Description: "With --dry-run, emit the plan as one JSON document with exact follow-up commands, project_part, and consent_required"},
				{Flags: "-y, --yes", Description: "Assume yes to safe project-file symlink migrations and destructive deprecation cleanup"},
				{Flags: "--no-yes", Description: "Force prompt-style declines in non-interactive mode"},
			},
		},
		{
			Name:        "config",
			Description: "Validate and refresh project Loaf config",
			Subcommands: []agentHelpSubcommand{
				{Name: "check", Description: "Validate .agents/loaf.json and installed Loaf-managed hook config", Options: []agentHelpOption{{Flags: "--fix", Description: "Create missing safe defaults and refresh stale installed target config"}, {Flags: "--json", Description: "Output config status, target hook status, warnings, and errors as JSON"}}},
			},
		},
		{
			Name:        "hooks",
			Description: "Inspect and set which Loaf hooks project into an installed harness's hooks file; enablement is user-scoped per target and the file is its projection",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "Show every hook this version ships per installed target with its event, enablement, whether the live file carries it, and absorption provenance; retired ids and entries Loaf does not own are never listed", Options: []agentHelpOption{{Flags: "--target <target>", Description: "Restrict the listing to one installed target (cursor, codex)"}}},
				{Name: "enable", Description: "Record a hook as enabled for one target, reconcile that target's hooks file, and report every action the reconcile took", Options: []agentHelpOption{{Flags: "<hook-id>", Description: "Hook id from the target's built catalog"}, {Flags: "--target <target>", Description: "Target whose hooks file to reconcile (cursor, codex); required"}}},
				{Name: "disable", Description: "Record a hook as disabled for one target, reconcile that target's hooks file, and report every action the reconcile took", Options: []agentHelpOption{{Flags: "<hook-id>", Description: "Hook id from the target's built catalog"}, {Flags: "--target <target>", Description: "Target whose hooks file to reconcile (cursor, codex); required"}}},
			},
		},
		{
			Name:        "harness",
			Description: "Diagnose or reconcile Loaf-owned installed harness content without updating the CLI, project config, human instructions, authentication material, or tracker state",
			Subcommands: []agentHelpSubcommand{
				{Name: "reconcile", Description: "Use one initiating harness to reconcile the complete installed shared-skills cohort and write a receipt", Options: []agentHelpOption{{Flags: "--target <target>", Description: "Initiating installed target; the reconciliation scope is the complete shared-skills cohort"}, {Flags: "--json", Description: "Output the cohort reconcile receipt as JSON"}}},
			},
		},
		{
			Name:        "setup",
			Description: "One-step bootstrap: init + build + install",
		},
		{
			Name:        "state",
			Description: "Manage native SQLite state",
			Subcommands: []agentHelpSubcommand{
				{Name: "path", Description: "Print the native SQLite database path", Options: []agentHelpOption{{Flags: "--json", Description: "Output contract version, database path, scope, and project root as JSON"}, {Flags: "--verbose", Description: "Output command, scope, project root, and database path"}}},
				{Name: "init", Description: "Initialize native SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output initialized status, global database scope, and project identity as JSON"}}},
				{Name: "status", Description: "Show native state status", Options: []agentHelpOption{{Flags: "--json", Description: "Output readiness mode, diagnostics, global database scope, and project identity as JSON"}}},
				{Name: "doctor", Description: "Diagnose native state health, including leftover SQLite work and leaked issue prefixes", Options: []agentHelpOption{{Flags: "--fix", Description: "Apply safe repairs"}, {Flags: "--dry-run", Description: "Preview repairs without writing"}, {Flags: "--json", Description: "Output diagnostics, repair plan, global database scope, and project identity as JSON"}}},
				{Name: "repair", Description: "Repair guarded SQLite data drift"},
				{Name: "repair legacy-project-database", Description: "Archive migrated per-project SQLite leftovers", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview archive paths without writing"}, {Flags: "--apply", Description: "Move legacy SQLite files into the archive directory"}, {Flags: "--json", Description: "Output archive plan/result, global database scope, and project identity as JSON"}}},
				{Name: "repair relationship-origin", Description: "Reclassify retired legacy origins to 'command'; foreign origins are reported, never rewritten. Bare invocation is reclassify-only and leaves missing origins untouched; --origin adds the backfill", Options: []agentHelpOption{{Flags: "--origin <imported|manual>", Description: "Enable the missing-origin backfill with this provenance value; omit for reclassify-only"}, {Flags: "--dry-run", Description: "Preview affected rows without writing"}, {Flags: "--apply", Description: "Apply the reclassification, and the backfill when --origin is given, after a backup"}, {Flags: "--json", Description: "Output repair mode, plan/result, global database scope, and project identity as JSON"}}},
				{Name: "repair journal-search", Description: "Rebuild the derived journal search index from canonical journal entries", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview canonical/index parity counts without writing"}, {Flags: "--apply", Description: "Create a verified backup, rebuild the index, and verify exact parity"}, {Flags: "--json", Description: "Output parity counts, backup verification, and repair result as JSON"}}},
				{Name: "migrate", Description: "Run state migrations"},
				{Name: "migrate markdown", Description: "Import markdown artifacts into native SQLite state", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Simulate import on a disposable DB snapshot when registered; otherwise inventory-only"}, {Flags: "--apply", Description: "Initialize SQLite and apply the import"}, {Flags: "--resume", Description: "Resume an interrupted import"}, {Flags: "--backup", Description: "Create SQLite and .agents rollback backups during apply or resume"}, {Flags: "--remove-source", Description: "Remove ephemeral Markdown sources after a rollback backup"}, {Flags: "--rollback <manifest>", Description: "Restore .agents files from a rollback manifest"}, {Flags: "--json", Description: "Output migration contract, scope, project context, counts, mode, import_report when simulated, and rollback fields as JSON"}}},
				{Name: "migrate storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into the global XDG_DATA_HOME database", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview migration work without copying"}, {Flags: "--apply", Description: "Copy or merge eligible legacy state without deleting the source"}, {Flags: "--json", Description: "Output migration contract, global database paths, action, and project identity when available"}}},
				{Name: "migrate schema", Description: "Preview or apply pending SQLite schema upgrades with a verified backup before mutation", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview pending schema upgrades without writing"}, {Flags: "--apply", Description: "Apply pending schema upgrades after creating and verifying a backup"}, {Flags: "--json", Description: "Output schema upgrade action, versions, pending migrations, backup, and verification as JSON"}}},
				{Name: "migrate lifecycle-statuses", Description: "Normalize legacy lifecycle statuses in SQLite with a backup and rollback manifest", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview status normalization on a temporary database copy"}, {Flags: "--apply", Description: "Normalize live SQLite statuses after creating a backup"}, {Flags: "--rollback <manifest>", Description: "Restore statuses from a lifecycle-statuses rollback manifest"}, {Flags: "--json", Description: "Output migration contract, project context, counts, backup, and rollback fields as JSON"}}},
				{Name: "migrate journal-first", Description: "Transform the global database to the journal-first model: purge lifecycle noise, drop the session entity, rekey journal search; destructive by consent", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview counts against a temporary database copy without mutation or backup"}, {Flags: "--apply", Description: "Take a mandatory backup, then apply the migration to the live database"}, {Flags: "--json", Description: "Output migration contract, counts, backup path, and schema version as JSON"}}},
				{Name: "migrate deferrals", Description: "Convert historical journal deferrals into canonical deferred Intents; apply is backup-first, provenance-linking, legacy-preserving, and rerunnable", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Report the project-specific conversion manifest without writing"}, {Flags: "--apply", Description: "Convert after creating and verifying a whole-database backup"}, {Flags: "--json", Description: "Output the conversion manifest, counts, backup, and project identity as JSON"}}},
				{Name: "backup", Description: "Create a SQLite database backup with local rollback or operator-selected non-temporary external destination classification", Options: []agentHelpOption{{Flags: "--to <DIRECTORY>", Description: "Operator-selected non-temporary external destination directory; not proof of off-device protection"}, {Flags: "--json", Description: "Output backup verification, classification, readiness, checksum, journal watermark, and current project identity as JSON"}}},
				{Name: "backup verify", Description: "Verify an existing SQLite database backup and report retrieval/recovery readiness", Options: []agentHelpOption{{Flags: "--json", Description: "Output schema version, SQLite validity, journal retrieval readiness, recovery readiness, watermark, and captured project identities as JSON"}}},
				{Name: "backup restore", Description: "Run an isolated disposable restore rehearsal without activating or replacing the live database", Options: []agentHelpOption{{Flags: "<backup>", Description: "Verified backup path"}, {Flags: "--to <absolute-empty-database-path>", Description: "Required empty disposable restore target; never the live database"}, {Flags: "--json", Description: "Output isolated disposable rehearsal, exact-copy, integrity, retrieval, watermark, and live-database safety evidence; never activates the live database"}}},
				{Name: "restore-ephemerals", Description: "Restore and stage .agents ephemeral Markdown from a rollback manifest or backup id", Options: []agentHelpOption{{Flags: "<manifest|backup-dir|backup-id>", Description: "Rollback manifest path, directory containing manifest.json, or backup id under the global backups directory"}, {Flags: "--json", Description: "Output rollback contract, project path, manifest path, restored file list, and restored status as JSON"}}},
				{Name: "verify-ephemerals", Description: "Verify .agents ephemeral Markdown before SQLite cutover", Options: []agentHelpOption{{Flags: "<manifest|backup-dir|backup-id>", Description: "Rollback manifest path, directory containing manifest.json, or backup id under the global backups directory"}, {Flags: "--json", Description: "Output verification contract, project context, per-file checks, and failures as JSON"}}},
				{Name: "export", Description: "Export state data", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format for the selected export kind"}}},
				{Name: "export all", Description: "Export a complete project-scoped SQLite snapshot", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: json"}, {Flags: "--json", Description: "Alias for --format json"}}},
				{Name: "export triage", Description: "Export a triage summary from SQLite state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export spec", Description: "Export one spec from SQLite state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export release-readiness", Description: "Export a release-readiness report from SQLite state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
			},
		},
		{
			Name:        "project",
			Description: "Manage durable project identity",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List registered projects in the global SQLite database", Options: []agentHelpOption{{Flags: "--json", Description: "Output database path, project IDs, friendly names, and current paths as JSON"}}},
				{Name: "show", Description: "Show the current project identity", Options: []agentHelpOption{{Flags: "--json", Description: "Output project ID, friendly name, current path, and database path as JSON"}}},
				{Name: "identity", Description: "Alias for project show", Options: []agentHelpOption{{Flags: "--json", Description: "Output project ID, friendly name, current path, and database path as JSON"}}},
				{Name: "rename", Description: "Rename the friendly project name", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Validate and preview without writing"}, {Flags: "--json", Description: "Output project ID, friendly name, current path, and database path as JSON"}}},
				{Name: "move", Description: "Record a checkout path move", Options: []agentHelpOption{{Flags: "<from> [to]", Description: "Previous and optional new absolute project paths"}, {Flags: "--from <path>", Description: "Previous absolute project path"}, {Flags: "--to <path>", Description: "New absolute project path; defaults to the current project root"}, {Flags: "--dry-run", Description: "Validate and preview without writing"}, {Flags: "--json", Description: "Output project ID, friendly name, current path, and database path as JSON"}}},
			},
		},
		{
			Name:        "docs",
			Description: "Manage docs/ indexing",
			Subcommands: []agentHelpSubcommand{
				{Name: "index", Description: "Index docs/ Markdown into SQLite FTS", Options: []agentHelpOption{{Flags: "--rebuild", Description: "Rebuild current worktree docs index before scanning"}, {Flags: "--json", Description: "Output indexed docs, counts, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "migrate",
			Description: "Run migration workflows",
			Subcommands: []agentHelpSubcommand{
				{Name: "markdown", Description: "Import markdown artifacts into native SQLite state", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Simulate import on a disposable DB snapshot when registered; otherwise inventory-only"}, {Flags: "--apply", Description: "Initialize SQLite and apply the import"}, {Flags: "--resume", Description: "Resume an interrupted import"}, {Flags: "--json", Description: "Output migration contract, scope, project context, counts, mode, and import_report when simulated as JSON"}}},
				{Name: "storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into the global XDG_DATA_HOME database", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview migration work without copying"}, {Flags: "--apply", Description: "Copy or merge eligible legacy state without deleting the source"}, {Flags: "--json", Description: "Output migration contract, global database paths, action, and project identity when available"}}},
				{Name: "schema", Description: "Preview or apply pending SQLite schema upgrades with a verified backup before mutation", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview pending schema upgrades without writing"}, {Flags: "--apply", Description: "Apply pending schema upgrades after creating and verifying a backup"}, {Flags: "--json", Description: "Output schema upgrade action, versions, pending migrations, backup, and verification as JSON"}}},
				{Name: "lifecycle-statuses", Description: "Normalize legacy lifecycle statuses in SQLite with a backup and rollback manifest", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview status normalization on a temporary database copy"}, {Flags: "--apply", Description: "Normalize live SQLite statuses after creating a backup"}, {Flags: "--rollback <manifest>", Description: "Restore statuses from a lifecycle-statuses rollback manifest"}, {Flags: "--json", Description: "Output migration contract, project context, counts, backup, and rollback fields as JSON"}}},
				{Name: "journal-first", Description: "Transform the global database to the journal-first model: purge lifecycle noise, drop the session entity, rekey journal search; destructive by consent", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview counts against a temporary database copy without mutation or backup"}, {Flags: "--apply", Description: "Take a mandatory backup, then apply the migration to the live database"}, {Flags: "--json", Description: "Output migration contract, counts, backup path, and schema version as JSON"}}},
				{Name: "vnext-rehearsal", Description: "Import a verified legacy backup into a brand-new isolated vNext database without activation or cutover", Options: []agentHelpOption{{Flags: "--backup <ABSOLUTE-SQLITE>", Description: "Absolute, clean path to a verified standalone legacy SQLite backup"}, {Flags: "--to <ABSOLUTE-NEW-DIRECTORY>", Description: "Absolute, clean path to a brand-new isolated destination directory"}, {Flags: "--json", Description: "Output structured source, archive, projection, and destination evidence as JSON"}}},
				{Name: "worktree-storage", Description: "Move linked-worktree .agents content to the main checkout", Options: []agentHelpOption{{Flags: "--apply", Description: "Perform the migration; dry-run is the default"}, {Flags: "--force-from-worktree", Description: "On conflict, keep the worktree-local copy"}, {Flags: "--force-from-main", Description: "On conflict, keep the main-worktree copy"}}},
			},
		},
		{
			Name:        "journal",
			Description: "Record and read the project journal",
			Subcommands: []agentHelpSubcommand{
				{Name: "log", Description: "Append a project-scoped journal entry", Options: []agentHelpOption{{Flags: "--execpolicy-safe", Description: "Codex Auto mode: place immediately after journal log; require the registered project and derive database/provenance from the current runtime or hook payload"}, {Flags: "--harness-session-id <id>", Description: "Opaque conversation correlation tag"}, {Flags: "--branch <branch>", Description: "Observed branch (defaults to current git branch)"}, {Flags: "--worktree <path>", Description: "Observed worktree path"}, {Flags: "--from-hook", Description: "Derive the entry from a harness hook payload; exits silently for subagents"}, {Flags: "--detect-linear", Description: "Scan recent commits for Linear magic words and log a discovery entry"}, {Flags: "--json", Description: "Output the written entry and project identity as JSON"}}},
				{Name: "recent", Description: "Show the recent project journal timeline", Options: []agentHelpOption{{Flags: "--branch <branch>", Description: "Restrict to entries observed on one branch"}, {Flags: "--since-last-wrap", Description: "Trim to entries logged after the most recent wrap"}, {Flags: "--limit <n>", Description: "Maximum entries to return"}, {Flags: "--json", Description: "Output the timeline and project identity as JSON"}}},
				{Name: "search", Description: "Full-text search journal entries", Options: []agentHelpOption{{Flags: "--all", Description: "Search across all projects"}, {Flags: "--limit <n>", Description: "Maximum hits to return"}, {Flags: "--json", Description: "Output hits and project identity as JSON"}}},
				{Name: "show", Description: "Show one journal entry by id", Options: []agentHelpOption{{Flags: "--json", Description: "Output the entry and project identity as JSON"}}},
				{Name: "defer", Description: "Capture a self-sufficient deferred intent as a decision and open spark pair; stable operation IDs make first writes idempotent and reworded retries visible", Options: []agentHelpOption{{Flags: "--why <text>", Description: "Why this intent was deferred"}, {Flags: "--boundary <text>", Description: "What remains outside this packet"}, {Flags: "--trigger <text>", Description: "What should cause revisit"}, {Flags: "--operation-id <id>", Description: "Stable retry/idempotency key"}, {Flags: "--json", Description: "Output the state result as JSON"}}},
				{Name: "context", Description: "Emit the contract-v2 active-truth continuity digest", Options: []agentHelpOption{{Flags: "--branch <branch>", Description: "Select branch-recency scope and bind state cursors; active Change provenance remains derived from the actual Git branch"}, {Flags: "--layer <name>", Description: "Select one canonical layer: project-synthesis, scoped-checkpoint, active-lineage, unresolved-blockers, deferred-intent, active-changes, branch-recency, or transitional-tasks"}, {Flags: "--limit <n>", Description: "Maximum 1..100 items for the selected layer; requires --layer"}, {Flags: "--cursor <token>", Description: "Continue the selected layer; requires --layer and is unavailable for intrinsic one-item project-synthesis and scoped-checkpoint"}, {Flags: "--from-hook", Description: "Read the harness hook payload on stdin; exits silently for subagent invocations"}, {Flags: "--cursor-hook", Description: "Read Cursor sessionStart JSON and emit its additional_context envelope"}, {Flags: "--claude-code", Description: "Read Claude Code SessionStart JSON and emit its native hook envelope"}, {Flags: "--codex-hook", Description: "Read Codex SessionStart JSON and emit its native hook envelope"}, {Flags: "--opencode-hook", Description: "Read the OpenCode session lifecycle payload and emit the digest as plain-text system context"}, {Flags: "--json", Description: "Output the contract-v2 continuity digest with project identity, named layers with availability/counts/truncation/expansion, and diagnostics as JSON"}, {Flags: "for-prompt|for-compact|for-resumption", Description: "Hook subcommands: inject implementation principles, journal-flush guidance, or the resumption digest"}}},
				{Name: "export", Description: "Export the project journal to markdown or JSONL", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown (default) or jsonl"}}},
			},
		},
		{
			Name:        "task",
			Description: "Manage project tasks",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "Show task board grouped by status", Options: []agentHelpOption{{Flags: "--json", Description: "Output tasks, diagnostics, global database scope, and project identity as JSON"}, {Flags: "--active", Description: "Hide completed tasks"}, {Flags: "--status <status>", Description: "Filter by task status: " + strings.Join(state.TaskListStatuses(), ", ")}}},
				{Name: "show", Description: "Display a single task's details", Options: []agentHelpOption{{Flags: "--json", Description: "Output task details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "status", Description: "Show task summary counts"},
				{Name: "create", Description: "Create a new task", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Task title"}, {Flags: "--spec <id>", Description: "Associated spec ID"}, {Flags: "--priority <level>", Description: "Task priority: " + strings.Join(state.TaskPriorities(), ", ")}, {Flags: "--depends-on <ids>", Description: "Comma-separated dependency task IDs"}, {Flags: "--json", Description: "Output created task, event, global database scope, and project identity as JSON"}}},
				{Name: "update", Description: "Update a task's metadata", Options: []agentHelpOption{{Flags: "--status <status>", Description: "New task status: " + strings.Join(state.TaskStatuses(), ", ")}, {Flags: "--priority <level>", Description: "New task priority: " + strings.Join(state.TaskPriorities(), ", ")}, {Flags: "--spec <id>", Description: "Set or clear associated spec"}, {Flags: "--depends-on <ids>", Description: "Replace dependencies"}, {Flags: "--json", Description: "Output updated task, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive completed tasks", Options: []agentHelpOption{{Flags: "--spec <id>", Description: "Archive done tasks for a spec"}, {Flags: "--json", Description: "Output archive result, archived tasks, global database scope, and project identity as JSON"}}},
				{Name: "refresh", Description: "Rebuild the Markdown task index from task/spec files", Options: []agentHelpOption{{Flags: "--json", Description: "Output compatibility mode, action, reason, and counts as JSON"}}},
				{Name: "sync", Description: "Sync the Markdown task index and task files", Options: []agentHelpOption{{Flags: "--import", Description: "Import orphan markdown files"}, {Flags: "--push", Description: "Push index metadata into markdown frontmatter"}, {Flags: "--json", Description: "Output compatibility mode, action, reason, and counts as JSON"}}},
			},
		},
		{
			Name:        "issue",
			Description: "Frozen local-work compatibility for explicit one-time migration; never ongoing tracker-native Flow authority",
			Subcommands: []agentHelpSubcommand{
				{Name: "new", Description: "Create a ref-keyed work contract or a legacy issue", Options: []agentHelpOption{{Flags: "--ref <authority-ref>", Description: "Provider-qualified authority ref; writes a work contract instead of an internal issue row"}, {Flags: "--body <text>", Description: "Inline issue body, or '-' to read from stdin"}, {Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"}, {Flags: "--message <text>", Description: "Use inline Markdown body text"}, {Flags: "--kind <kind>", Description: "Issue kind: delivery or decision"}, {Flags: "--parent <ref>", Description: "Parent issue ref"}, {Flags: "--fog <text>", Description: "Questions not yet sharp enough to be issues"}, {Flags: "--status <status>", Description: "Write status after create: " + strings.Join(state.IssueWriteStatuses(), ", ") + "; still records the initial triage event"}, {Flags: "--json", Description: "Output the created issue, global database scope, and project identity as JSON"}}},
				{Name: "absorb", Description: "Mint an issue from leftover SQLite work, or dismiss the source", Options: []agentHelpOption{{Flags: "--all", Description: "Project every leftover row in scope for the current project"}, {Flags: "--history", Description: "Include done and archived tasks, and ordinarily resolved intents"}, {Flags: "--dry-run", Description: "Rehearse --all without writing"}, {Flags: "--dismiss", Description: "Archive the source as superseded without minting an issue"}, {Flags: "--json", Description: "Output the absorb result, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one work contract or legacy issue by ref", Options: []agentHelpOption{{Flags: "--json", Description: "Output issue details, parent, children, bucket, global database scope, and project identity as JSON"}}},
				{Name: "list", Description: "List project issues", Options: []agentHelpOption{{Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--kind <kind>", Description: "Filter by kind"}, {Flags: "--archived", Description: "Include archived issues"}, {Flags: "--started", Description: "List issues with a recorded started worktree"}, {Flags: "--json", Description: "Output issues, global database scope, and project identity as JSON"}}},
				{Name: "tree", Description: "Print a recursive issue tree", Options: []agentHelpOption{{Flags: "--archived", Description: "Include archived issues"}, {Flags: "--json", Description: "Output the tree, global database scope, and project identity as JSON"}}},
				{Name: "frontier", Description: "List unblocked pick-up-next authority refs", Options: []agentHelpOption{{Flags: "--json", Description: "Output frontier refs, legacy issues, global database scope, and project identity as JSON"}}},
				{Name: "start", Description: "Start or join the shippable root workspace", Options: []agentHelpOption{{Flags: "--json", Description: "Output the root issue, requested ref, joined flag, branch, worktree, base, global database scope, and project identity as JSON"}}},
				{Name: "stop", Description: "Remove a started worktree; descendants must stop the root", Options: []agentHelpOption{{Flags: "--force", Description: "Remove a dirty worktree"}, {Flags: "--json", Description: "Output the stopped issue, branch, worktree, already-gone flag, global database scope, and project identity as JSON"}}},
				{Name: "edit", Description: "Replace an issue body through the shared body-edit path", Options: []agentHelpOption{{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"}, {Flags: "--body -", Description: "Read Markdown body from stdin"}, {Flags: "--message <text>", Description: "Use inline Markdown body text"}, {Flags: "--json", Description: "Output the edited issue, global database scope, and project identity as JSON"}}},
				{Name: "retitle", Description: "Replace an issue title", Options: []agentHelpOption{{Flags: "--json", Description: "Output the retitled issue, global database scope, and project identity as JSON"}}},
				{Name: "status", Description: "Set an issue status", Options: []agentHelpOption{{Flags: "--duplicate-of <ref>", Description: "Surviving issue required when status is duplicate"}, {Flags: "--json", Description: "Output the updated issue, global database scope, and project identity as JSON"}}},
				{Name: "dod", Description: "Manage definition-of-done criteria"},
				{Name: "promote", Description: "Promote a criterion into a child issue", Options: []agentHelpOption{{Flags: "--json", Description: "Output the new child issue, global database scope, and project identity as JSON"}}},
				{Name: "bucket", Description: "Set an advisory Now/Next/Later label", Options: []agentHelpOption{{Flags: "--json", Description: "Output the issue and bucket, global database scope, and project identity as JSON"}}},
				{Name: "link", Description: "Create or remove an issue relationship", Options: []agentHelpOption{{Flags: "--json", Description: "Output the relationship mutation, global database scope, and project identity as JSON"}}},
				{Name: "render", Description: "Emit a paste-ready PR body", Options: []agentHelpOption{{Flags: "--json", Description: "Output the markdown, issue, global database scope, and project identity as JSON"}}},
				{Name: "identity", Description: "Show, define, or align the local issue prefix", Options: []agentHelpOption{{Flags: "--prefix <prefix>", Description: "Define the local issue prefix and rewrite existing aliases"}, {Flags: "--align", Description: "Rewrite a leaked LOAF prefix to the project slug"}, {Flags: "--all", Description: "With --align, rewrite every leaked project in the global database"}, {Flags: "--dry-run", Description: "Rehearse --prefix or --align without writing"}, {Flags: "--json", Description: "Output identity or rewrite result as JSON"}}},
				{Name: "export", Description: "Export issues, identity, criteria, claims, and relationships as JSON", Options: []agentHelpOption{{Flags: "--json", Description: "Output the export snapshot"}}},
				{Name: "pull", Description: "Legacy adapter only; not a tracker-native Flow operation", Options: []agentHelpOption{{Flags: "--tree", Description: "Also adopt the sub-issue tree with parent edges intact"}, {Flags: "--json", Description: "Output the legacy adapter result as JSON"}}},
				{Name: "push", Description: "Legacy adapter only; local-to-tracker synchronization is not supported by current Flow", Options: []agentHelpOption{{Flags: "--json", Description: "Output the legacy adapter result as JSON"}}},
				{Name: "reconcile", Description: "Legacy adapter only; local-to-tracker synchronization is not supported by current Flow", Options: []agentHelpOption{{Flags: "--take-local", Description: "Legacy adapter option"}, {Flags: "--take-tracker", Description: "Legacy adapter option"}, {Flags: "--json", Description: "Output the legacy adapter result as JSON"}}},
			},
		},
		{
			Name:        "report",
			Description: "Manage durable reports",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List reports", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Filter by report type"}, {Flags: "--status <status>", Description: "Filter by status; Loaf lifecycle statuses: draft, done, archived"}, {Flags: "--json", Description: "Output reports, diagnostics, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one report", Options: []agentHelpOption{{Flags: "--json", Description: "Output report details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "generate", Description: "Generate a report from state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}, {Flags: "--json", Description: "Output contract, command, project context, and markdown content as JSON"}}},
				{Name: "create", Description: "Create a report draft", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Report type"}, {Flags: "--source <source>", Description: "Report source"}, {Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"}, {Flags: "--body -", Description: "Read Markdown body from stdin"}, {Flags: "--message <text>", Description: "Use inline Markdown body text"}, {Flags: "--json", Description: "Output created report, event, global database scope, and project identity as JSON"}}},
				{Name: "edit", Description: "Replace a report Markdown body under .agents/reports/; run report finalize to update the tracked render", Options: []agentHelpOption{{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"}, {Flags: "--body -", Description: "Read Markdown body from stdin"}, {Flags: "--message <text>", Description: "Use inline Markdown body text"}, {Flags: "--force", Description: "Proceed when the on-disk report diverges from the expected body"}, {Flags: "--json", Description: "Output the edited report, imported flag, content hash, event, global database scope, and project identity as JSON"}}},
				{Name: "finalize", Description: "Mark a report draft as done", Options: []agentHelpOption{{Flags: "--json", Description: "Output report status transition, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a done report", Options: []agentHelpOption{{Flags: "--json", Description: "Output report status transition, event, global database scope, and project identity as JSON"}}},
			},
		},
		nativeArtifactAgentHelpCommand("plan"),
		nativeArtifactAgentHelpCommand("handoff"),
		nativeArtifactAgentHelpCommand("council"),
		{
			Name:        "kb",
			Description: "Knowledge base management",
			Subcommands: []agentHelpSubcommand{
				{Name: "status", Description: "Show knowledge base status", Options: []agentHelpOption{{Flags: "--json", Description: "Output knowledge file totals, coverage counts, stale count, review age, and directories as JSON"}}},
				{Name: "validate", Description: "Validate knowledge file frontmatter", Options: []agentHelpOption{{Flags: "--json", Description: "Output per-file frontmatter errors and warnings as JSON"}}},
				{Name: "check", Description: "Check knowledge staleness", Options: []agentHelpOption{{Flags: "--file <path>", Description: "Reverse lookup: find knowledge files covering this path"}, {Flags: "--json", Description: "Output per-file staleness, coverage, commit, and review metadata as JSON"}}},
				{Name: "review", Description: "Mark knowledge files reviewed", Options: []agentHelpOption{{Flags: "--json", Description: "Output updated knowledge frontmatter as JSON"}}},
				{Name: "init", Description: "Initialize knowledge directories", Options: []agentHelpOption{{Flags: "--json", Description: "Output directory actions, config status, and QMD collections as JSON"}}},
				{Name: "import", Description: "Register external knowledge imports", Options: []agentHelpOption{{Flags: "--path <path>", Description: "Path to the external project's knowledge directory"}, {Flags: "--json", Description: "Output QMD import collection status or import error as JSON"}}},
				{Name: "glossary", Description: "Domain glossary mutation and lookup"},
			},
		},
		{
			Name:        "check",
			Description: "Run hook checks",
			Options: []agentHelpOption{
				{Flags: "--hook <id>", Description: "Run one registered hook"},
				{Flags: "--json", Description: "Output hook result, pass/block status, exit code, warnings, errors, and findings as JSON"},
			},
		},
		{Name: "doctor", Description: "Diagnose project alignment, managed content, and leftover migration work", Options: []agentHelpOption{{Flags: "--fix", Description: "Offer safe repairs with y/N confirmation"}, {Flags: "--force", Description: "With --fix, accept every repair without prompting"}, {Flags: "--verbose", Description: "Show details"}, {Flags: "--json", Description: "Output the identical check set as read-only JSON; never prompts or repairs"}}},
		{
			Name:        "release",
			Description: "Cut a retroactive release from already-landed work",
			Subcommands: []agentHelpSubcommand{
				{Name: "suggest", Description: "Report landed work since the last version tag. Writes nothing.", Options: []agentHelpOption{
					{Flags: "--base <ref>", Description: "Use commits since <ref> instead of last tag"},
					{Flags: "--json", Description: "Output the suggestion as JSON"},
				}},
				{Name: "cut", Description: "Cut a retroactive release from landed work. Records members as facts.", Options: []agentHelpOption{
					{Flags: "--base <ref>", Description: "Use commits since <ref> instead of last tag"},
					{Flags: "--bump <type>", Description: "Override the suggested bump"},
					{Flags: "--includes <version|tag>", Description: "Reference a prior release (repeatable)"},
					{Flags: "--no-tag", Description: "Skip git tag creation (tag v<version> must already exist)"},
					{Flags: "--no-gh", Description: "Skip GitHub release draft"},
					{Flags: "--dry-run", Description: "Print the plan and write nothing"},
				}},
			},
		},
		{Name: "version", Description: "Show version and content counts"},
		{Name: "housekeeping", Description: "Scan agent artifacts and summarize housekeeping recommendations", Options: []agentHelpOption{{Flags: "--json", Description: "Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON"}, {Flags: "--dry-run", Description: "Show recommendations without applying actions"}, {Flags: "--specs", Description: "Only review specs"}, {Flags: "--drafts", Description: "Only review shaping drafts"}, {Flags: "--plans", Description: "Accept legacy plans filter for compatibility"}, {Flags: "--handoffs", Description: "Accept legacy handoffs filter for compatibility"}}},
		{Name: "trace", Description: "Trace relationships for an entity", Options: []agentHelpOption{{Flags: "--json", Description: "Output traced entity, sources, relationships, global database scope, and project identity as JSON"}}},
		{
			Name:        "brainstorm",
			Description: "Manage brainstorm artifacts",
			Subcommands: []agentHelpSubcommand{
				{Name: "capture", Description: "Capture a brainstorm in SQLite state", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Brainstorm title"}, {Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"}, {Flags: "--body -", Description: "Read Markdown body from stdin"}, {Flags: "--message <text>", Description: "Use inline Markdown body text"}, {Flags: "--json", Description: "Output created brainstorm, event, global database scope, and project identity as JSON"}}},
				{Name: "list", Description: "List brainstorms from SQLite state", Options: []agentHelpOption{{Flags: "--all", Description: "Include archived brainstorms"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output brainstorms, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one brainstorm from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output brainstorm details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "promote", Description: "Record brainstorm-to-idea promotion", Options: []agentHelpOption{{Flags: "--to-idea <idea>", Description: "Target idea"}, {Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive one or more brainstorms", Options: []agentHelpOption{{Flags: "--reason <text>", Description: "Archive reason"}, {Flags: "--json", Description: "Output archive result, archived brainstorms, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "idea",
			Description: "Manage ideas",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List ideas from SQLite state", Options: []agentHelpOption{{Flags: "--all", Description: "Include done and archived ideas"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output ideas, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one idea from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output idea details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "capture", Description: "Capture an idea in SQLite state", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Idea title"}, {Flags: "--json", Description: "Output created idea, event, global database scope, and project identity as JSON"}}},
				{Name: "promote", Description: "Record idea-to-spec promotion", Options: []agentHelpOption{{Flags: "--to-spec <spec>", Description: "Target spec"}, {Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"}}},
				{Name: "resolve", Description: "Resolve an idea by linking it to another entity", Options: []agentHelpOption{{Flags: "--by <entity>", Description: "Resolving entity"}, {Flags: "--json", Description: "Output resolution relationship, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive one or more ideas", Options: []agentHelpOption{{Flags: "--reason <text>", Description: "Archive reason"}, {Flags: "--json", Description: "Output archive result, archived ideas, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "intent",
			Description: "Manage tracked Intent; disposition is derived from append-only facts with no mutable lifecycle status",
			Subcommands: []agentHelpSubcommand{
				{Name: "create", Description: "Create a tracked or deferred Intent snapshot plus its initial disposition in one transaction", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Bounded single-line title"}, {Flags: "--body <body>", Description: "Self-sufficient body"}, {Flags: "--disposition <disposition>", Description: "tracked (default) or deferred"}, {Flags: "--why <why>", Description: "Why the deferred direction matters"}, {Flags: "--boundary <boundary>", Description: "What excluded it now"}, {Flags: "--trigger <trigger>", Description: "When to revisit"}, {Flags: "--operation-id <key>", Description: "Retry-safe operation key; required when deferred"}, {Flags: "--from <source>", Description: "Source spark, idea, brainstorm, or journal entry; repeatable"}, {Flags: "--reason <reason>", Description: "Optional reason recorded with the initial disposition"}, {Flags: "--json", Description: "Output the created or reused Intent, digests, and project identity as JSON"}}},
				{Name: "defer", Description: "Append an immutable four-field deferral payload to an existing Intent through the shared operation mapping", Options: []agentHelpOption{{Flags: "--why <why>", Description: "Why the direction matters"}, {Flags: "--boundary <boundary>", Description: "What excluded it now"}, {Flags: "--trigger <trigger>", Description: "When to revisit"}, {Flags: "--operation-id <key>", Description: "Retry-safe operation key"}, {Flags: "--json", Description: "Output the deferred Intent, digests, and project identity as JSON"}}},
				{Name: "resume", Description: "Append a tracked disposition linked to the deferral it supersedes; history is never overwritten", Options: []agentHelpOption{{Flags: "--reason <why now>", Description: "Why the Intent is tracked again"}, {Flags: "--json", Description: "Output the resumed Intent and project identity as JSON"}}},
				{Name: "resolve", Description: "Append a reasoned terminal disposition", Options: []agentHelpOption{{Flags: "--reason <outcome>", Description: "Resolution outcome"}, {Flags: "--json", Description: "Output the resolved Intent and project identity as JSON"}}},
				{Name: "show", Description: "Show one Intent with latest snapshot, derived disposition, deferral payload, and sources", Options: []agentHelpOption{{Flags: "--json", Description: "Output Intent detail, sources, and project identity as JSON"}}},
				{Name: "list", Description: "List Intents with derived dispositions in deterministic order", Options: []agentHelpOption{{Flags: "--disposition <disposition>", Description: "Filter by derived disposition: tracked, deferred, or resolved"}, {Flags: "--json", Description: "Output Intents and project identity as JSON"}}},
			},
		},
		{
			Name:        "intake",
			Description: "Read the deterministic local intake projection of unresolved capture and tracked work; the CLI never ranks, promotes, or chooses a disposition",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "Project each unresolved spark, idea, brainstorm, intent, and unmigrated legacy deferral exactly once with provenance and exact read commands", Options: []agentHelpOption{{Flags: "--json", Description: "Output intake items and project identity as JSON"}}},
			},
		},
		{
			Name:        "exploration",
			Description: "Manage relational Exploration continuity through immutable portable checkpoints; no lifecycle status or current pointer exists",
			Subcommands: []agentHelpSubcommand{
				{Name: "create", Description: "Create an Exploration identity; sources map to explores or uses-source edges by kind", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Bounded exploration title"}, {Flags: "--from <source>", Description: "Intent, journal entry, handoff, report, or finding reference; repeatable"}, {Flags: "--json", Description: "Output the created Exploration and project identity as JSON"}}},
				{Name: "checkpoint", Description: "Append one immutable checkpoint with the four required portable fields, each capped at 4096 UTF-8 bytes and never truncated", Options: []agentHelpOption{{Flags: "--purpose <text>", Description: "Current framing"}, {Flags: "--conclusions <text>", Description: "Conclusions or constraints so far"}, {Flags: "--unresolved <text>", Description: "Unresolved question or decision"}, {Flags: "--next <text>", Description: "Recommended next action"}, {Flags: "--item <type>:<content>", Description: "Ordered typed item (candidate or evidence); repeatable"}, {Flags: "--operation-id <key>", Description: "Retry-safe operation key"}, {Flags: "--json", Description: "Output the appended checkpoint and project identity as JSON"}}},
				{Name: "list", Description: "List Explorations with checkpoint counts and portable-context presence", Options: []agentHelpOption{{Flags: "--json", Description: "Output Explorations and project identity as JSON"}}},
				{Name: "context", Description: "Project portable context: the four-field core returns whole while every optional layer reports counts, truncation, a stable cursor, and an exact expansion command", Options: []agentHelpOption{{Flags: "--layer <name>", Description: "Select one layer: items, intents, evidence, or conversations"}, {Flags: "--cursor <cursor>", Description: "Continue the selected layer; requires --layer"}, {Flags: "--limit <n>", Description: "Maximum 1..100 items for the selected layer; requires --layer"}, {Flags: "--json", Description: "Output the portable context projection as JSON"}}},
				{Name: "conversation", Description: "Associate a logical conversation explicitly with add; membership is never inferred from branch, worktree, or recency", Options: []agentHelpOption{{Flags: "--json", Description: "Output the membership result as JSON"}}},
			},
		},
		{
			Name:        "conversation",
			Description: "Manage logical conversations and machine-local provenance handles; a handle is optional evidence and never implies portable context",
			Subcommands: []agentHelpSubcommand{
				{Name: "create", Description: "Create a logical conversation that may carry multiple harness-local handles", Options: []agentHelpOption{{Flags: "--title <label>", Description: "Conversation label"}, {Flags: "--operation-id <key>", Description: "Retry-safe operation key"}, {Flags: "--json", Description: "Output the created conversation and project identity as JSON"}}},
				{Name: "show", Description: "Show one conversation with handles, log refs, and latest observed availability", Options: []agentHelpOption{{Flags: "--json", Description: "Output the conversation and project identity as JSON"}}},
				{Name: "list", Description: "List logical conversations deterministically", Options: []agentHelpOption{{Flags: "--json", Description: "Output conversations and project identity as JSON"}}},
				{Name: "handle", Description: "Attach a machine-local handle with add: harness, opaque local ID, optional locality, and an optional bounded log locator with hash and range", Options: []agentHelpOption{{Flags: "--harness <harness>", Description: "Harness name, e.g. codex or claude-code"}, {Flags: "--handle <id>", Description: "Opaque machine-local conversation identifier"}, {Flags: "--locality <scope>", Description: "Machine or namespace scope"}, {Flags: "--log-ref <locator>", Description: "Bounded log locator, never transcript content"}, {Flags: "--hash <sha256>", Description: "Optional SHA-256 of the referenced log range"}, {Flags: "--range <range>", Description: "Optional bounded range"}, {Flags: "--json", Description: "Output the handle result and project identity as JSON"}}},
				{Name: "observe", Description: "Append an immutable timestamped availability observation for a handle or log ref; the observed row never mutates", Options: []agentHelpOption{{Flags: "--handle <handle-id>", Description: "Observed conversation handle ID"}, {Flags: "--log-ref <log-ref-id>", Description: "Observed log reference ID"}, {Flags: "--available", Description: "Source was reachable"}, {Flags: "--unavailable", Description: "Source was not reachable"}, {Flags: "--observer <name>", Description: "Observing agent or probe"}, {Flags: "--locality <scope>", Description: "Machine or namespace of the observation"}, {Flags: "--note <text>", Description: "Bounded observation note"}, {Flags: "--json", Description: "Output the observation result and project identity as JSON"}}},
			},
		},
		{
			Name:        "spark",
			Description: "Manage sparks",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List sparks from SQLite state", Options: []agentHelpOption{{Flags: "--all", Description: "Include done sparks"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output sparks, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one spark from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output spark details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "capture", Description: "Capture a spark in SQLite state", Options: []agentHelpOption{{Flags: "--scope <scope>", Description: "Spark scope"}, {Flags: "--text <text>", Description: "Spark text"}, {Flags: "--json", Description: "Output created spark, event, global database scope, and project identity as JSON"}}},
				{Name: "resolve", Description: "Resolve a spark by linking it to the entity that resolves it", Options: []agentHelpOption{{Flags: "--by <entity>", Description: "Resolving entity reference (required)"}, {Flags: "--reason <text>", Description: "Resolution reason"}, {Flags: "--json", Description: "Output resolution relationship, event, global database scope, and project identity as JSON"}}},
				{Name: "promote", Description: "Record spark-to-idea promotion", Options: []agentHelpOption{{Flags: "--to-idea <idea>", Description: "Target idea"}, {Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "tag",
			Description: "Manage tags",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List tags from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output tags, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show entities with a tag", Options: []agentHelpOption{{Flags: "--json", Description: "Output tagged entities, global database scope, and project identity as JSON"}}},
				{Name: "add", Description: "Add a tag to an entity", Options: []agentHelpOption{{Flags: "--json", Description: "Output tag mutation, entity, global database scope, and project identity as JSON"}}},
				{Name: "remove", Description: "Remove a tag from an entity", Options: []agentHelpOption{{Flags: "--json", Description: "Output tag mutation, entity, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "bundle",
			Description: "Manage bundles",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List bundles from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output bundles, global database scope, and project identity as JSON"}}},
				{Name: "create", Description: "Create a bundle", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Bundle title"}, {Flags: "--tags <tags>", Description: "Comma-separated tag query"}, {Flags: "--json", Description: "Output created bundle, tags, global database scope, and project identity as JSON"}}},
				{Name: "update", Description: "Update a bundle", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Bundle title"}, {Flags: "--tags <tags>", Description: "Comma-separated tag query"}, {Flags: "--json", Description: "Output updated bundle, tags, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one bundle", Options: []agentHelpOption{{Flags: "--json", Description: "Output bundle details, members, global database scope, and project identity as JSON"}}},
				{Name: "add", Description: "Add an entity to a bundle", Options: []agentHelpOption{{Flags: "--json", Description: "Output bundle membership result, global database scope, and project identity as JSON"}}},
				{Name: "remove", Description: "Remove an entity from a bundle", Options: []agentHelpOption{{Flags: "--json", Description: "Output bundle membership result, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "link",
			Description: "Manage entity relationships",
			Subcommands: []agentHelpSubcommand{
				{Name: "create", Description: "Create an explicit relationship", Options: []agentHelpOption{{Flags: "--from <entity>", Description: "Source entity"}, {Flags: "--to <entity>", Description: "Target entity"}, {Flags: "--type <type>", Description: "Relationship type"}, {Flags: "--reason <text>", Description: "Relationship reason"}, {Flags: "--json", Description: "Output relationship ID, source/target, global database scope, and project identity as JSON"}}},
				{Name: "list", Description: "List relationships for one entity", Options: []agentHelpOption{{Flags: "--json", Description: "Output relationships, global database scope, and project identity as JSON"}}},
				{Name: "remove", Description: "Remove an explicit relationship", Options: []agentHelpOption{{Flags: "--from <entity>", Description: "Source entity"}, {Flags: "--to <entity>", Description: "Target entity"}, {Flags: "--type <type>", Description: "Relationship type"}, {Flags: "--json", Description: "Output removed relationship ID, global database scope, and project identity as JSON"}}},
			},
		},
	}
}

func nativeArtifactAgentHelpCommand(kind string) agentHelpCommand {
	options := []agentHelpOption{
		{Flags: "--title <title>", Description: "Artifact title"},
		{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
		{Flags: "--body -", Description: "Read Markdown body from stdin"},
		{Flags: "--message <text>", Description: "Use inline Markdown body text"},
	}
	switch kind {
	case "plan", "council":
		options = append(options, agentHelpOption{Flags: "--spec <spec>", Description: "Optional related spec"})
	case "handoff":
		options = append(options,
			agentHelpOption{Flags: "--harness-session-id <id>", Description: "Optional conversation correlation tag"},
			agentHelpOption{Flags: "--task <task>", Description: "Optional related task"},
		)
	}
	options = append(options, agentHelpOption{Flags: "--json", Description: "Output created artifact, event, global database scope, and project identity as JSON"})
	desc := "Manage " + kind + "s in native SQLite state"
	createDesc := "Create a " + kind + " in SQLite state"
	showDesc := "Show one " + kind + " from SQLite state"
	listDesc := "List " + kind + "s from SQLite state"
	if kind == "council" {
		desc = "Manage councils as Markdown files under .agents/councils/"
		createDesc = "Create a council Markdown file under .agents/councils/"
		showDesc = "Show one council from .agents/councils/"
		listDesc = "List councils from .agents/councils/"
	}
	return agentHelpCommand{
		Name:        kind,
		Description: desc,
		Subcommands: []agentHelpSubcommand{
			{Name: "new", Description: createDesc, Options: options},
			{Name: "show", Description: showDesc, Options: []agentHelpOption{{Flags: "--json", Description: "Output artifact details, relationships, global database scope, and project identity as JSON"}}},
			{Name: "list", Description: listDesc, Options: []agentHelpOption{{Flags: "--all", Description: "Include archived artifacts"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output artifacts, global database scope, and project identity as JSON"}}},
			{Name: "link", Description: "Link a " + kind + " to another entity", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Relationship type; defaults to related_to"}, {Flags: "--reason <text>", Description: "Relationship reason"}, {Flags: "--json", Description: "Output relationship ID, source/target, global database scope, and project identity as JSON"}}},
		},
	}
}
