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
	outputPath := filepath.Join(rootPath, "content", "skills", "loaf-reference", "SKILL.md")
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
				{Flags: "--codex-basic-commands", Description: "Explicitly install the least-privilege Codex basic command policy (requires --to codex or --to all)"},
				{Flags: "-y, --yes", Description: "Assume 'yes' to safe migrations and destructive deprecation cleanup"},
				{Flags: "--no-yes", Description: "Force interactive prompts even when stdin is not a TTY (testing)"},
			},
		},
		{
			Name:        "config",
			Description: "Validate and refresh project Loaf config",
			Subcommands: []cliReferenceSubcommand{
				{Name: "check", Description: "Validate .agents/loaf.json and installed Loaf-managed hook config", Options: []cliReferenceOption{
					{Flags: "--fix", Description: "Create missing safe defaults and refresh stale installed target config"},
					{Flags: "--json", Description: "Output config status, target hook status, warnings, and errors as JSON"},
				}},
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
				{Flags: "--bump <type>", Description: "Skip interactive bump choice (prerelease, release, major, minor, patch); only explicit prerelease advances an existing prerelease during a lineage freeze; --post-merge may finalize that prepared prerelease"},
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
			Name:        "change",
			Description: "Shape-first Change artifacts: git-canonical work context under docs/changes/",
			Subcommands: []cliReferenceSubcommand{
				{Name: "init", Description: "Scaffold a new Change folder from the template", Options: []cliReferenceOption{
					{Flags: "<slug>", Description: "Change slug: lowercase letters, digits, and single hyphens"},
				}},
				{Name: "check", Description: "Validate a Change and report derived executability", Options: []cliReferenceOption{
					{Flags: "[folder]", Description: "Change folder path; an explicit path wins, otherwise resolves from the current branch"},
					{Flags: "--require-executable", Description: "Exit non-zero unless the Change is structurally executable; this does not prove implementation completion (CI gate for non-draft PRs)"},
					{Flags: "--json", Description: "Output folder, passed, executable, findings, warnings, and gaps as JSON"},
				}},
				{Name: "list", Description: "List a retained lineage after merge or branch deletion", Options: []cliReferenceOption{
					{Flags: "--lineage <key>", Description: "Required lineage key"},
					{Flags: "--json", Description: "Output derived nodes, gaps, and optional journal enrichment"},
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
				{Name: "repair journal-search", Description: "Preview or apply a backup-first rebuild of the derived journal search index", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview canonical/index parity counts without writing"},
					{Flags: "--apply", Description: "Create a verified backup, rebuild the index, and verify exact parity"},
					{Flags: "--json", Description: "Output parity counts, backup verification, and repair result as JSON"},
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
				{Name: "migrate schema", Description: "Preview or apply pending SQLite schema upgrades with a verified backup before mutation", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview pending schema upgrades without writing"},
					{Flags: "--apply", Description: "Apply pending schema upgrades after creating and verifying a backup"},
					{Flags: "--json", Description: "Output schema upgrade action, versions, pending migrations, backup, and verification as JSON"},
				}},
				{Name: "migrate lifecycle-statuses", Description: "Normalize legacy lifecycle statuses in SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview status normalization on a temporary database copy"},
					{Flags: "--apply", Description: "Normalize live SQLite statuses after creating a backup"},
					{Flags: "--rollback <manifest>", Description: "Restore statuses from a lifecycle-statuses rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, project context, counts, backup, and rollback fields as JSON"},
				}},
				{Name: "backup", Description: "Create a SQLite database backup with local rollback or operator-selected non-temporary external destination classification", Options: []cliReferenceOption{{Flags: "--to <DIRECTORY>", Description: "Operator-selected non-temporary external destination directory; not proof of off-device protection"}, {Flags: "--json", Description: "Output backup verification, classification, readiness, checksum, journal watermark, and current project identity as JSON"}}},
				{Name: "backup verify", Description: "Verify an existing SQLite database backup and report retrieval/recovery readiness", Options: []cliReferenceOption{{Flags: "--json", Description: "Output schema version, SQLite validity, journal retrieval readiness, recovery readiness, watermark, and captured project identities as JSON"}}},
				{Name: "backup restore", Description: "Run an isolated disposable restore rehearsal without activating or replacing the live database", Options: []cliReferenceOption{{Flags: "<backup>", Description: "Verified backup path"}, {Flags: "--to <absolute-empty-database-path>", Description: "Required empty disposable restore target; never the live database"}, {Flags: "--json", Description: "Output isolated disposable rehearsal, exact-copy, integrity, retrieval, watermark, and live-database safety evidence; never activates the live database"}}},
				{Name: "restore-ephemerals", Description: "Restore and stage .agents ephemeral Markdown from a rollback manifest or backup id", Options: []cliReferenceOption{
					{Flags: "<manifest|backup-dir|backup-id>", Description: "Rollback manifest path, directory containing manifest.json, or backup id under the global backups directory"},
					{Flags: "--json", Description: "Output rollback contract, project path, manifest path, restored file list, and restored status as JSON"},
				}},
				{Name: "verify-ephemerals", Description: "Verify .agents ephemeral Markdown before SQLite cutover", Options: []cliReferenceOption{
					{Flags: "<manifest|backup-dir|backup-id>", Description: "Rollback manifest path, directory containing manifest.json, or backup id under the global backups directory"},
					{Flags: "--json", Description: "Output verification contract, project context, per-file checks, and failures as JSON"},
				}},
				{Name: "export", Description: "Export SQLite state for review or migration", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format for the selected export kind"}}},
				{Name: "export all", Description: "Export a complete project-scoped SQLite snapshot", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: json"}, {Flags: "--json", Description: "Alias for --format json"}}},
				{Name: "export triage", Description: "Export a triage summary from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export spec", Description: "Export one spec from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export release-readiness", Description: "Export a release-readiness report from SQLite state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
			},
		},
		{
			Name:        "journal",
			Description: "Record and read the project-scoped journal (the durable record across all conversations)",
			Subcommands: []cliReferenceSubcommand{
				{Name: "log", Description: "Append a project-scoped journal entry", Options: []cliReferenceOption{
					{Flags: "--execpolicy-safe", Description: "Codex Auto mode: place immediately after journal log; require the registered project and derive database/provenance from the current runtime or hook payload"},
					{Flags: "--harness-session-id <id>", Description: "Opaque conversation correlation tag"},
					{Flags: "--branch <branch>", Description: "Observed branch (defaults to current git branch)"},
					{Flags: "--worktree <path>", Description: "Observed worktree path"},
					{Flags: "--from-hook", Description: "Derive the entry from a harness hook payload on stdin; exits silently for subagents"},
					{Flags: "--detect-linear", Description: "Scan recent commits for Linear magic words and log a discovery entry"},
					{Flags: "--json", Description: "Output the written entry and project identity as JSON"},
				}},
				{Name: "recent", Description: "Show the recent project journal timeline", Options: []cliReferenceOption{
					{Flags: "--branch <branch>", Description: "Restrict to entries observed on one branch"},
					{Flags: "--since-last-wrap", Description: "Trim to entries logged after the most recent wrap"},
					{Flags: "--limit <n>", Description: "Maximum entries to return"},
					{Flags: "--json", Description: "Output the timeline and project identity as JSON"},
				}},
				{Name: "search", Description: "Full-text search journal entries", Options: []cliReferenceOption{
					{Flags: "--all", Description: "Search across all projects"},
					{Flags: "--limit <n>", Description: "Maximum hits to return"},
					{Flags: "--json", Description: "Output hits and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one journal entry by id", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the entry and project identity as JSON"},
				}},
				{Name: "context", Description: "Emit the contract-v2 active-truth continuity digest", Options: []cliReferenceOption{
					{Flags: "--branch <branch>", Description: "Select branch-recency scope and bind state cursors; active Change provenance remains derived from the actual Git branch"},
					{Flags: "--layer <name>", Description: "Select one canonical layer: project-synthesis, scoped-checkpoint, active-lineage, unresolved-blockers, deferred-intent, active-changes, branch-recency, or transitional-tasks"},
					{Flags: "--limit <n>", Description: "Maximum 1..100 items for the selected layer; requires --layer"},
					{Flags: "--cursor <token>", Description: "Continue the selected layer; requires --layer and is unavailable for intrinsic one-item project-synthesis and scoped-checkpoint"},
					{Flags: "--from-hook", Description: "Read the harness hook payload on stdin; exits silently for subagents (SessionStart/PostCompact)"},
					{Flags: "--json", Description: "Output contract-v2 project metadata, named layers with availability/counts/truncation/expansion, and diagnostics as JSON"},
					{Flags: "for-prompt|for-compact|for-resumption", Description: "Hook subcommands: inject implementation principles, journal-flush guidance, or the resumption digest"},
				}},
				{Name: "export", Description: "Export the project journal to markdown or JSONL", Options: []cliReferenceOption{
					{Flags: "--format <format>", Description: "Output format: markdown (default) or jsonl"},
				}},
				{Name: "defer", Description: "Capture a self-sufficient deferred intent as a decision and open spark pair; stable operation IDs make first writes idempotent and reworded retries visible", Options: []cliReferenceOption{
					{Flags: "--why <text>", Description: "Why this intent was deferred"},
					{Flags: "--boundary <text>", Description: "What remains outside this packet"},
					{Flags: "--trigger <text>", Description: "What should cause revisit"},
					{Flags: "--operation-id <id>", Description: "Stable retry/idempotency key"},
					{Flags: "--change <slug|path>", Description: "Optional retained Change local evidence"},
					{Flags: "--json", Description: "Output the state result as JSON"},
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
				{Name: "delete", Description: "Permanently delete a project and every dependent row across all entity tables", Options: []cliReferenceOption{
					{Flags: "<project-id>", Description: "Project id, friendly name, or current path"},
					{Flags: "--yes", Description: "Confirm the destructive delete (required)"},
					{Flags: "--json", Description: "Output removed-row counts and global database scope as JSON"},
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
				{Name: "schema", Description: "Preview or apply pending SQLite schema upgrades with a verified backup before mutation", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview pending schema upgrades without writing"},
					{Flags: "--apply", Description: "Apply pending schema upgrades after creating and verifying a backup"},
					{Flags: "--json", Description: "Output schema upgrade action, versions, pending migrations, backup, and verification as JSON"},
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
				{Name: "new", Description: "Create a spec in SQLite state", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Spec title (defaults to a title derived from the slug)"},
					{Flags: "--id <SPEC-NNN>", Description: "Explicit spec id; auto-allocated when omitted"},
					{Flags: "--source <source>", Description: "Provenance label recorded on the spec and creation event (default: ad-hoc)"},
					{Flags: "--branch <name>", Description: "Implementation branch recorded on the spec for breakdown/implement handoff"},
					{Flags: "--related <SPEC-A,SPEC-B>", Description: "Comma-separated spec refs to link as related"},
					{Flags: "--body-file <path>", Description: "Read the spec body from a file"},
					{Flags: "--body -", Description: "Read the spec body from stdin"},
					{Flags: "--message <text>", Description: "Use the given text as the spec body"},
					{Flags: "--json", Description: "Output the created spec, global database scope, and project identity as JSON"},
				}},
				{Name: "list", Description: "Show specs with status and task counts", Options: []cliReferenceOption{{Flags: "--json", Description: "Output specs, diagnostics, task counts, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show spec details", Options: []cliReferenceOption{{Flags: "--json", Description: "Output spec details, branch, source, resolved related specs, task counts, relationships, global database scope, and project identity as JSON"}}},
				{Name: "status", Description: "Set a spec's lifecycle status (draft, todo, in_progress, done, archived)", Options: []cliReferenceOption{{Flags: "--json", Description: "Output spec status transition, event, global database scope, and project identity as JSON"}}},
				{Name: "render", Description: "Render deterministic spec Markdown to the XDG cache", Options: []cliReferenceOption{{Flags: "--json", Description: "Output render path, content hash, contract, global database scope, and project identity as JSON"}}},
				{Name: "finalize", Description: "Write deterministic spec Markdown to its tracked git location", Options: []cliReferenceOption{{Flags: "--json", Description: "Output render path, content hash, contract, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a completed spec", Options: []cliReferenceOption{{Flags: "--json", Description: "Output archive result, archived specs, global database scope, and project identity as JSON"}}},
				{Name: "delete", Description: "Permanently delete a spec and every dependent row (aliases, bodies, search index, events, sources); leaves the on-disk render in place", Options: []cliReferenceOption{
					{Flags: "<spec>", Description: "Spec ref to delete"},
					{Flags: "--yes", Description: "Confirm the destructive delete (required)"},
					{Flags: "--json", Description: "Output removed-row counts, global database scope, and project identity as JSON"},
				}},
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
		{
			Name:        "doctor",
			Description: "Diagnose Loaf project alignment (symlinks, stale files, version drift)",
			Options: []cliReferenceOption{
				{Flags: "--fix", Description: "Apply safe auto-fixes for failing checks"},
				{Flags: "--verbose", Description: "Print each check name even when passing"},
			},
		},
	}
}

func generateCLIReferenceSkill(commands []cliReferenceCommand) string {
	header := `---
name: loaf-reference
description: >-
  Documents how agents operate the Loaf CLI: command discovery via loaf --help, JSON diagnosis surfaces, guided config maintenance, and troubleshooting. Use when unsure which loaf command to invoke or how to validate project state. Not for workflow guidance (workflow skills own their CLI contracts) or build internals.
---

# Loaf Reference

## Contents
- Operating Rules
- Journal Context (contract v2)
- Command Index
- Topics

The Loaf operating manual for agents: how to discover commands, diagnose project state, and keep configuration current. It teaches reading the CLI, not memorizing it.

**Note:** This file is auto-generated from native CLI reference metadata. Do not edit manually.
`

	lines := []string{
		"",
		"## Operating Rules",
		"",
		"- Get exact, current syntax live: `loaf --help` lists every command, `loaf <command> --help` details one. This index is a map, not the contract.",
		"- Prefer `--json` surfaces when diagnosing: `loaf config check --json`, `loaf state doctor --json`, `loaf change check --json`. Parse the structured output instead of scraping human-readable text.",
		"- Run the deterministic CLI command before hand-editing anything it manages; the command owns its files.",
		"- Use `--fix` only for safe, mechanical repairs, and review what it changed.",
		"- Ask the user for project-owned choices — GitHub account, tracker or integration election, which harnesses to install — never guess them.",
		"- Never hand-edit Loaf-managed hook files; regenerate them through `loaf build` and `loaf install`.",
		"- Re-run the relevant check after any change and confirm it passes.",
		"- Log meaningful decisions to the journal: `loaf journal log \"decision(scope): ...\"`.",
		"",
		"## Journal Context (contract v2)",
		"",
		"`loaf journal context` is an active-truth read model, not the former latest-arbitrary-wrap plus branch entries plus open tasks summary. Consume its named layers and diagnostics rather than inferring state from an omitted layer.",
		"",
		"| Layer | Truth it supplies |",
		"|-------|-------------------|",
		"| `project-synthesis` | The latest `wrap(project)` synthesis. |",
		"| `scoped-checkpoint` | The latest non-project wrap, only when no project synthesis exists; it is explicitly labeled as a fallback. |",
		"| `active-lineage` | Journal evidence associated with active Change lineage. |",
		"| `unresolved-blockers` | Blocks that do not have a later exact-scope unblock. |",
		"| `deferred-intent` | Open deferred-intent decision and spark pairs. |",
		"| `active-changes` | Git-derived active Change evidence and worktree state. |",
		"| `branch-recency` | Recent entries on the selected branch after entries already surfaced as active truth are removed. |",
		"| `transitional-tasks` | Open task-board records during the Markdown-to-native transition. |",
		"",
		"Each layer reports `source_available`, `available_count`, `shown_count`, `truncated`, and an exact `expand_command`; paginated layers also return a cursor. `source_available: false` means the source could not be derived and is not an empty result. In particular, an unavailable Change source marks both `active-changes` and `active-lineage` unavailable and emits a diagnostic.",
		"",
		"Use `--branch` to select `branch-recency` scope and bind state cursors. It does not override active Change provenance or reasons, which always use the actual Git branch. Use `--layer` to request one canonical layer. `--limit` accepts 1 through 100 only with `--layer`; `--cursor` also requires `--layer` and cannot expand the intrinsic one-item `project-synthesis` or `scoped-checkpoint` layers. Reuse the returned `expand_command` verbatim: cursors are bound to their layer, project, branch, snapshot, and limit. `--json` is the stable machine surface; human output retains the same counts, unavailable markers, and expansion command.",
		"",
		"## Command Index",
		"",
		"Names and one-line purposes only. Run `loaf <command> --help` for options, arguments, and current usage.",
		"",
		"| Command | Purpose | Subcommands |",
		"|---------|---------|-------------|",
	}

	for _, cmd := range commands {
		lines = append(lines, cliReferenceIndexRow(cmd))
	}
	for _, cmd := range supplementalCLIReferenceCommands(commands) {
		lines = append(lines, cliReferenceIndexRow(cmd))
	}

	lines = append(lines,
		"",
		"## Topics",
		"",
		"| Topic | Reference | Use When |",
		"|-------|-----------|----------|",
		"| Configuration maintenance | [references/configuration.md](references/configuration.md) | Checking whether a project's Loaf config is current and repairing it; wiring project-owned choices |",
		"| Command routing | [references/command-routing.md](references/command-routing.md) | Deciding which command a task needs; locating the JSON diagnosis surfaces |",
		"| Troubleshooting | [references/troubleshooting.md](references/troubleshooting.md) | Diagnosing state, config, or alignment failures; isolating a throwaway database |",
		"",
	)

	return header + strings.Join(lines, "\n")
}

func cliReferenceIndexRow(cmd cliReferenceCommand) string {
	subcommands := "—"
	if len(cmd.Subcommands) > 0 {
		names := make([]string, 0, len(cmd.Subcommands))
		for _, sub := range cmd.Subcommands {
			names = append(names, sub.Name)
		}
		subcommands = strings.Join(names, ", ")
	}
	return fmt.Sprintf("| `loaf %s` | %s | %s |", cmd.Name, cmd.Description, subcommands)
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
			cliReferenceOption{Flags: "--harness-session-id <id>", Description: "Optional conversation correlation tag"},
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
