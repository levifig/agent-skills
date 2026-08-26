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
			Description: "Onboard Loaf into a folder or a not-yet-installed AI tool configuration",
			Options: []cliReferenceOption{
				{Flags: "--to <target>", Description: `Target to install to (or "all")`},
				{Flags: "--codex-basic-commands", Description: "Explicitly install the least-privilege Codex basic command policy (requires --to codex or --to all)"},
				{Flags: "-y, --yes", Description: "Assume 'yes' to safe migrations and destructive deprecation cleanup"},
				{Flags: "--no-yes", Description: "Force interactive prompts even when stdin is not a TTY (testing)"},
			},
		},
		{
			Name:        "upgrade",
			Description: "Refresh Loaf in place: harness content sync plus deprecation cleanup anywhere, and project-surface refresh only inside a detected Loaf repo",
			Options: []cliReferenceOption{
				{Flags: "--to <target>", Description: `Filter the harness sync to one already-installed target (or "all"); an uninstalled target is an error pointing at loaf install --to`},
				{Flags: "--dry-run", Description: "Report the deterministic non-mutating plan: per-artifact actions, preserved conflicts, deprecations, project-file effects, whether the project part is in scope, and consent requirements"},
				{Flags: "--json", Description: "With --dry-run, emit the plan as one JSON document with exact follow-up commands, project_part, and consent_required"},
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
			Name:        "hooks",
			Description: "Inspect and set which Loaf hooks project into an installed harness's hooks file",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "Show every hook this version ships per installed target with its event, enablement, whether the live file carries it, and absorption provenance; retired ids and entries Loaf does not own are never listed", Options: []cliReferenceOption{
					{Flags: "--target <target>", Description: "Restrict the listing to one installed target (cursor, codex)"},
				}},
				{Name: "enable", Description: "Record a hook as enabled for one target, reconcile that target's hooks file, and report every action the reconcile took", Options: []cliReferenceOption{
					{Flags: "<hook-id>", Description: "Hook id from the target's built catalog, as loaf hooks list reports it"},
					{Flags: "--target <target>", Description: "Target whose hooks file to reconcile (cursor, codex); required, because enablement is recorded per target"},
				}},
				{Name: "disable", Description: "Record a hook as disabled for one target, reconcile that target's hooks file, and report every action the reconcile took", Options: []cliReferenceOption{
					{Flags: "<hook-id>", Description: "Hook id from the target's built catalog, as loaf hooks list reports it"},
					{Flags: "--target <target>", Description: "Target whose hooks file to reconcile (cursor, codex); required, because enablement is recorded per target"},
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
			Description: "Cut a retroactive release from already-landed work",
			Subcommands: []cliReferenceSubcommand{
				{Name: "suggest", Description: "Report landed work since the last version tag. Writes nothing.", Options: []cliReferenceOption{
					{Flags: "--base <ref>", Description: "Use commits since <ref> instead of last tag"},
					{Flags: "--json", Description: "Output the suggestion as JSON"},
				}},
				{Name: "cut", Description: "Cut a retroactive release from landed work. Records members as facts.", Options: []cliReferenceOption{
					{Flags: "--base <ref>", Description: "Use commits since <ref> instead of last tag"},
					{Flags: "--bump <type>", Description: "Override the suggested bump"},
					{Flags: "--includes <version|tag>", Description: "Reference a prior release (repeatable)"},
					{Flags: "--no-tag", Description: "Skip git tag creation (tag v<version> must already exist)"},
					{Flags: "--no-gh", Description: "Skip GitHub release draft"},
					{Flags: "--dry-run", Description: "Print the plan and write nothing"},
				}},
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
				{Name: "doctor", Description: "Diagnose SQLite state health, including leftover SQLite work and leaked issue prefixes", Options: []cliReferenceOption{
					{Flags: "--fix", Description: "Initialize missing SQLite state when safe"},
					{Flags: "--dry-run", Description: "Show the repair plan without applying fixes"},
					{Flags: "--json", Description: "Output diagnostics, repair plan, global database scope, and project identity as JSON"},
				}},
				{Name: "repair legacy-project-database", Description: "Archive migrated per-project SQLite leftovers", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview archive paths without writing"},
					{Flags: "--apply", Description: "Move legacy SQLite files into the archive directory"},
					{Flags: "--json", Description: "Output archive plan/result, global database scope, and project identity as JSON"},
				}},
				{Name: "repair relationship-origin", Description: "Reclassify retired legacy origins to 'command'; foreign origins are reported, never rewritten. Bare invocation is reclassify-only and leaves missing origins untouched; --origin adds the backfill", Options: []cliReferenceOption{
					{Flags: "--origin <imported|manual>", Description: "Enable the missing-origin backfill with this provenance value; omit for reclassify-only"},
					{Flags: "--dry-run", Description: "Preview affected rows without writing"},
					{Flags: "--apply", Description: "Reclassify retired legacy origins, and backfill missing origins when --origin is given, after creating a SQLite backup"},
					{Flags: "--json", Description: "Output repair mode, plan/result, global database scope, and project identity as JSON"},
				}},
				{Name: "repair journal-search", Description: "Preview or apply a backup-first rebuild of the derived journal search index", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview canonical/index parity counts without writing"},
					{Flags: "--apply", Description: "Create a verified backup, rebuild the index, and verify exact parity"},
					{Flags: "--json", Description: "Output parity counts, backup verification, and repair result as JSON"},
				}},
				{Name: "migrate markdown", Description: "Import existing .agents Markdown artifacts into SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Simulate import on a disposable DB snapshot when registered; otherwise inventory-only"},
					{Flags: "--apply", Description: "Initialize SQLite and import Markdown artifacts"},
					{Flags: "--resume", Description: "Resume the Markdown import after an interrupted attempt"},
					{Flags: "--backup", Description: "Create SQLite and .agents rollback backups during apply or resume"},
					{Flags: "--remove-source", Description: "Remove ephemeral Markdown sources after a rollback backup"},
					{Flags: "--rollback <manifest>", Description: "Restore .agents files from a rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, scope, project context, counts, mode, import_report when simulated, and rollback fields as JSON"},
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
				{Name: "migrate deferrals", Description: "Convert historical journal deferrals into canonical deferred Intents; apply is backup-first, provenance-linking, legacy-preserving, and rerunnable", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Report the project-specific conversion manifest without writing"},
					{Flags: "--apply", Description: "Convert after creating and verifying a whole-database backup"},
					{Flags: "--json", Description: "Output the conversion manifest, counts, backup, and project identity as JSON"},
				}},
				{Name: "migrate lifecycle-statuses", Description: "Normalize legacy lifecycle statuses in SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview status normalization on a temporary database copy"},
					{Flags: "--apply", Description: "Normalize live SQLite statuses after creating a backup"},
					{Flags: "--rollback <manifest>", Description: "Restore statuses from a lifecycle-statuses rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, project context, counts, backup, and rollback fields as JSON"},
				}},
				{Name: "migrate alias-orphans", Description: "Retire alias-orphaned entity rows across every project with a backup and rollback manifest", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview classification on a temporary database copy (default)"},
					{Flags: "--apply", Description: "Apply the repair after creating a backup"},
					{Flags: "--rollback <manifest>", Description: "Restore deleted rows from an alias-orphans rollback manifest"},
					{Flags: "--retire <entity-id>", Description: "Force-retire an unproven orphan (repeatable)"},
					{Flags: "--realias <entity-id>=<alias>", Description: "Attach an alias to an unproven orphan (repeatable)"},
					{Flags: "--json", Description: "Output migration contract, per-project classification, counts, backup, and rollback fields as JSON"},
				}},
				{Name: "migrate journal-duplicates", Description: "Retire June-13/June-24 journal natural-key twins across every project with a backup and rollback manifest", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview classification on a temporary database copy (default)"},
					{Flags: "--apply", Description: "Apply the repair after creating a backup"},
					{Flags: "--rollback <manifest>", Description: "Restore deleted rows from a journal-duplicates rollback manifest"},
					{Flags: "--retire <entry-id>", Description: "Force-retire an unproven multi-candidate journal row (repeatable)"},
					{Flags: "--json", Description: "Output migration contract, per-project classification, counts, backup, and rollback fields as JSON"},
				}},
				{Name: "migrate journal-first", Description: "Transform the global database to the journal-first model: purge lifecycle noise, drop the session entity, rekey journal search; destructive by consent", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview counts against a temporary database copy without mutation or backup"},
					{Flags: "--apply", Description: "Take a mandatory backup, then apply the migration to the live database"},
					{Flags: "--json", Description: "Output migration contract, counts, backup path, and schema version as JSON"},
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
					{Flags: "--from-hook", Description: "Read a target lifecycle-hook payload on stdin; exits silently when the normalized payload identifies a subagent"},
					{Flags: "--cursor-hook", Description: "Read Cursor sessionStart JSON and emit its additional_context envelope"},
					{Flags: "--claude-code", Description: "Read Claude Code SessionStart JSON and emit its native hook envelope"},
					{Flags: "--codex-hook", Description: "Read Codex SessionStart JSON and emit its native hook envelope"},
					{Flags: "--opencode-hook", Description: "Read the OpenCode session lifecycle payload and emit the digest as plain-text system context"},
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
					{Flags: "--dry-run", Description: "Simulate import on a disposable DB snapshot when registered; otherwise inventory-only"},
					{Flags: "--apply", Description: "Initialize SQLite and import Markdown artifacts"},
					{Flags: "--resume", Description: "Resume the Markdown import after an interrupted attempt"},
					{Flags: "--backup", Description: "Create SQLite and .agents rollback backups during apply or resume"},
					{Flags: "--remove-source", Description: "Remove ephemeral Markdown sources after a rollback backup"},
					{Flags: "--rollback <manifest>", Description: "Restore .agents files from a rollback manifest"},
					{Flags: "--json", Description: "Output migration contract, scope, project context, counts, mode, import_report when simulated, and rollback fields as JSON"},
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
				{Name: "journal-first", Description: "Transform the global database to the journal-first model: purge lifecycle noise, drop the session entity, rekey journal search; destructive by consent", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview counts against a temporary database copy without mutation or backup"},
					{Flags: "--apply", Description: "Take a mandatory backup, then apply the migration to the live database"},
					{Flags: "--json", Description: "Output migration contract, counts, backup path, and schema version as JSON"},
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
			Description: "Manage project tasks; superseded by loaf issue for new work",
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
			Name:        "issue",
			Description: "Manage issues in native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "new", Description: "Create an issue", Options: []cliReferenceOption{
					{Flags: "--body <text>", Description: "Inline issue body, or '-' to read from stdin"},
					{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
					{Flags: "--message <text>", Description: "Use inline Markdown body text"},
					{Flags: "--kind <kind>", Description: "Issue kind: delivery or decision"},
					{Flags: "--parent <ref>", Description: "Parent issue ref"},
					{Flags: "--fog <text>", Description: "Questions not yet sharp enough to be issues"},
					{Flags: "--status <status>", Description: "Write status after create: " + strings.Join(state.IssueWriteStatuses(), ", ") + "; still records the initial triage event"},
					{Flags: "--json", Description: "Output the created issue, global database scope, and project identity as JSON"},
				}},
				{Name: "absorb", Description: "Mint an issue from leftover SQLite work, or dismiss the source", Options: []cliReferenceOption{
					{Flags: "--all", Description: "Project every leftover row in scope for the current project"},
					{Flags: "--history", Description: "Include done and archived tasks, and ordinarily resolved intents"},
					{Flags: "--dry-run", Description: "Rehearse --all without writing"},
					{Flags: "--dismiss", Description: "Archive the source as superseded without minting an issue"},
					{Flags: "--json", Description: "Output the absorb result, global database scope, and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one issue", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output issue details, parent, children, bucket, global database scope, and project identity as JSON"},
				}},
				{Name: "list", Description: "List project issues", Options: []cliReferenceOption{
					{Flags: "--status <status>", Description: "Filter by status"},
					{Flags: "--kind <kind>", Description: "Filter by kind"},
					{Flags: "--archived", Description: "Include archived issues"},
					{Flags: "--started", Description: "List issues with a recorded started worktree"},
					{Flags: "--json", Description: "Output issues, global database scope, and project identity as JSON"},
				}},
				{Name: "tree", Description: "Print a recursive issue tree", Options: []cliReferenceOption{
					{Flags: "--archived", Description: "Include archived issues"},
					{Flags: "--json", Description: "Output the tree, global database scope, and project identity as JSON"},
				}},
				{Name: "frontier", Description: "List unblocked pick-up-next issues", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output frontier issues, global database scope, and project identity as JSON"},
				}},
				{Name: "start", Description: "Start or join the shippable root workspace", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the root issue, requested ref, joined flag, branch, worktree, base, global database scope, and project identity as JSON"},
				}},
				{Name: "stop", Description: "Remove a started worktree; descendants must stop the root", Options: []cliReferenceOption{
					{Flags: "--force", Description: "Remove a dirty worktree"},
					{Flags: "--json", Description: "Output the stopped issue, branch, worktree, already-gone flag, global database scope, and project identity as JSON"},
				}},
				{Name: "edit", Description: "Replace an issue body through the shared body-edit path", Options: []cliReferenceOption{
					{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
					{Flags: "--body -", Description: "Read Markdown body from stdin"},
					{Flags: "--message <text>", Description: "Use inline Markdown body text"},
					{Flags: "--json", Description: "Output the edited issue, global database scope, and project identity as JSON"},
				}},
				{Name: "retitle", Description: "Replace an issue title", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the retitled issue, global database scope, and project identity as JSON"},
				}},
				{Name: "status", Description: "Set an issue status", Options: []cliReferenceOption{
					{Flags: "--duplicate-of <ref>", Description: "Surviving issue required when status is duplicate"},
					{Flags: "--json", Description: "Output the updated issue, global database scope, and project identity as JSON"},
				}},
				{Name: "dod", Description: "Manage definition-of-done criteria"},
				{Name: "dod add", Description: "Add a criterion", Options: []cliReferenceOption{
					{Flags: "--command <cmd>", Description: "Verification command (implies tier V)"},
					{Flags: "--expect <expect>", Description: "Verification expect grammar (exit N, contains <text>)"},
					{Flags: "--tier <V|H>", Description: "Override criterion tier: V or H"},
					{Flags: "--serves <parent-position>", Description: "Parent criterion position this child criterion claims"},
					{Flags: "--json", Description: "Output the updated issue, global database scope, and project identity as JSON"},
				}},
				{Name: "dod list", Description: "List criteria", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the issue and criteria, global database scope, and project identity as JSON"},
				}},
				{Name: "dod remove", Description: "Remove a criterion by position", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the updated issue, global database scope, and project identity as JSON"},
				}},
				{Name: "dod claim", Description: "Claim a child criterion against a parent criterion", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the updated child issue, global database scope, and project identity as JSON"},
				}},
				{Name: "dod unclaim", Description: "Remove a child-to-parent criterion claim", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the updated child issue, global database scope, and project identity as JSON"},
				}},
				{Name: "promote", Description: "Promote a criterion into a child issue", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the new child issue, global database scope, and project identity as JSON"},
				}},
				{Name: "check", Description: "Derive readiness from the issue row", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output issue, kind, shaped, covered, ready, failures, orphans, publication, global database scope, and project identity as JSON"},
					{Flags: "--human <reason>", Description: "Publish ready-for-human instead of ready-for-agent, with this reason"},
				}},
				{Name: "verify", Description: "Run V-tier criteria from the repository root", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output per-criterion results, issue, ok, global database scope, and project identity as JSON"},
				}},
				{Name: "bucket", Description: "Set an advisory Now/Next/Later label", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the issue and bucket, global database scope, and project identity as JSON"},
				}},
				{Name: "link", Description: "Create or remove an issue relationship", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the relationship mutation, global database scope, and project identity as JSON"},
				}},
				{Name: "render", Description: "Emit a paste-ready PR body", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the markdown, issue, global database scope, and project identity as JSON"},
				}},
				{Name: "identity", Description: "Show, define, or align issue identity and persist it to .agents/loaf.json", Options: []cliReferenceOption{
					{Flags: "--prefix <prefix>", Description: "Define the issue prefix or Linear team key and persist it to .agents/loaf.json"},
					{Flags: "--authority <local|linear|github>", Description: "Set issue authority and persist it to .agents/loaf.json"},
					{Flags: "--align", Description: "Rewrite a leaked LOAF prefix to the project slug"},
					{Flags: "--all", Description: "With --align, rewrite every leaked project in the global database"},
					{Flags: "--dry-run", Description: "Rehearse --prefix, --authority, or --align without writing"},
					{Flags: "--json", Description: "Output identity or rewrite result as JSON"},
				}},
				{Name: "export", Description: "Export issues, identity, criteria, claims, and relationships as JSON", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the export snapshot"},
				}},
				{Name: "pull", Description: "Adopt an existing Linear issue", Options: []cliReferenceOption{
					{Flags: "--tree", Description: "Also adopt the sub-issue tree with parent edges intact"},
					{Flags: "--json", Description: "Output the adopted issue and tree as JSON"},
				}},
				{Name: "push", Description: "Write the local render and status to Linear", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output the push result as JSON"},
				}},
				{Name: "reconcile", Description: "Compare local and Linear and surface conflicts", Options: []cliReferenceOption{
					{Flags: "--take-local", Description: "Write the local status to Linear"},
					{Flags: "--take-tracker", Description: "Write the Linear status to local through the events path"},
					{Flags: "--json", Description: "Output the reconcile result as JSON"},
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
				{Name: "edit", Description: "Replace a report's SQLite body; run report finalize to update the tracked render", Options: []cliReferenceOption{
					{Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"},
					{Flags: "--body -", Description: "Read Markdown body from stdin"},
					{Flags: "--message <text>", Description: "Use inline Markdown body text"},
					{Flags: "--force", Description: "Proceed when the legacy source file diverges from the SQLite body"},
					{Flags: "--json", Description: "Output the edited report, imported flag, content hash, event, global database scope, and project identity as JSON"},
				}},
				{Name: "finalize", Description: "Mark a report draft as done and write its deterministic tracked render", Options: []cliReferenceOption{{Flags: "--json", Description: "Output report status transition, render path, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a done report", Options: []cliReferenceOption{{Flags: "--json", Description: "Output report status transition, event, global database scope, and project identity as JSON"}}},
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
			Name:        "intent",
			Description: "Manage tracked Intent in native SQLite state; disposition is derived from append-only facts; superseded by loaf issue for new work",
			Subcommands: []cliReferenceSubcommand{
				{Name: "create", Description: "Create a tracked or deferred Intent in one transaction", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Bounded single-line title"},
					{Flags: "--body <body>", Description: "Self-sufficient body"},
					{Flags: "--disposition <disposition>", Description: "tracked (default) or deferred"},
					{Flags: "--why <why>", Description: "Why the deferred direction matters"},
					{Flags: "--boundary <boundary>", Description: "What excluded it now"},
					{Flags: "--trigger <trigger>", Description: "When to revisit"},
					{Flags: "--operation-id <key>", Description: "Retry-safe operation key; required when deferred"},
					{Flags: "--from <source>", Description: "Source spark, idea, brainstorm, or journal entry; repeatable"},
					{Flags: "--reason <reason>", Description: "Optional reason recorded with the initial disposition"},
					{Flags: "--json", Description: "Output the created or reused Intent, digests, and project identity as JSON"},
				}},
				{Name: "defer", Description: "Append an immutable deferral to an existing Intent", Options: []cliReferenceOption{
					{Flags: "--why <why>", Description: "Why the direction matters"},
					{Flags: "--boundary <boundary>", Description: "What excluded it now"},
					{Flags: "--trigger <trigger>", Description: "When to revisit"},
					{Flags: "--operation-id <key>", Description: "Retry-safe operation key"},
					{Flags: "--json", Description: "Output the deferred Intent, digests, and project identity as JSON"},
				}},
				{Name: "resume", Description: "Append a tracked disposition linked to the deferral it supersedes", Options: []cliReferenceOption{
					{Flags: "--reason <why now>", Description: "Why the Intent is tracked again"},
					{Flags: "--json", Description: "Output the resumed Intent and project identity as JSON"},
				}},
				{Name: "resolve", Description: "Append a reasoned terminal disposition without overwriting history", Options: []cliReferenceOption{
					{Flags: "--reason <outcome>", Description: "Resolution outcome"},
					{Flags: "--json", Description: "Output the resolved Intent and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one Intent with latest snapshot, derived disposition, deferral payload, and sources", Options: []cliReferenceOption{{Flags: "--json", Description: "Output Intent detail, sources, and project identity as JSON"}}},
				{Name: "list", Description: "List Intents with derived dispositions in deterministic order", Options: []cliReferenceOption{
					{Flags: "--disposition <disposition>", Description: "Filter by derived disposition: tracked, deferred, or resolved"},
					{Flags: "--json", Description: "Output Intents and project identity as JSON"},
				}},
			},
		},
		{
			Name:        "intake",
			Description: "Read the deterministic local intake projection; triage judgment stays with humans and Skills",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "Project each unresolved spark, idea, brainstorm, intent, and unmigrated legacy deferral exactly once with provenance and exact read commands", Options: []cliReferenceOption{{Flags: "--json", Description: "Output intake items and project identity as JSON"}}},
			},
		},
		{
			Name:        "exploration",
			Description: "Manage relational Exploration continuity: immutable portable checkpoints, no lifecycle status, no current pointer",
			Subcommands: []cliReferenceSubcommand{
				{Name: "create", Description: "Create an Exploration identity; sources map to explores or uses-source edges by kind", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Bounded exploration title"},
					{Flags: "--from <source>", Description: "Intent, journal entry, handoff, report, or finding reference; repeatable"},
					{Flags: "--json", Description: "Output the created Exploration and project identity as JSON"},
				}},
				{Name: "checkpoint", Description: "Append one immutable checkpoint; the four core fields are required, trimmed, and capped at 4096 UTF-8 bytes without truncation", Options: []cliReferenceOption{
					{Flags: "--purpose <text>", Description: "Current framing"},
					{Flags: "--conclusions <text>", Description: "Conclusions or constraints so far"},
					{Flags: "--unresolved <text>", Description: "Unresolved question or decision"},
					{Flags: "--next <text>", Description: "Recommended next action"},
					{Flags: "--item <type>:<content>", Description: "Ordered typed item (candidate or evidence); repeatable"},
					{Flags: "--operation-id <key>", Description: "Retry-safe operation key"},
					{Flags: "--json", Description: "Output the appended checkpoint and project identity as JSON"},
				}},
				{Name: "list", Description: "List Explorations with checkpoint counts and portable-context presence", Options: []cliReferenceOption{{Flags: "--json", Description: "Output Explorations and project identity as JSON"}}},
				{Name: "context", Description: "Project portable context: the four-field core returns whole; every optional layer reports counts, truncation, and an exact expansion command", Options: []cliReferenceOption{
					{Flags: "--layer <name>", Description: "Select one layer: items, intents, evidence, or conversations"},
					{Flags: "--cursor <cursor>", Description: "Continue the selected layer (requires --layer)"},
					{Flags: "--limit <n>", Description: "Maximum 1..100 items for the selected layer (requires --layer)"},
					{Flags: "--json", Description: "Output the portable context projection as JSON"},
				}},
				{Name: "conversation", Description: "Associate a logical conversation explicitly: loaf exploration conversation add <exploration> <conversation-id>", Options: []cliReferenceOption{{Flags: "--json", Description: "Output the membership result as JSON"}}},
			},
		},
		{
			Name:        "conversation",
			Description: "Manage logical conversations and machine-local provenance handles; handles never imply portable context",
			Subcommands: []cliReferenceSubcommand{
				{Name: "create", Description: "Create a logical conversation that may carry multiple harness-local handles", Options: []cliReferenceOption{
					{Flags: "--title <label>", Description: "Conversation label"},
					{Flags: "--operation-id <key>", Description: "Retry-safe operation key"},
					{Flags: "--json", Description: "Output the created conversation and project identity as JSON"},
				}},
				{Name: "show", Description: "Show one conversation with handles, log refs, and latest observed availability", Options: []cliReferenceOption{{Flags: "--json", Description: "Output the conversation and project identity as JSON"}}},
				{Name: "list", Description: "List logical conversations deterministically", Options: []cliReferenceOption{{Flags: "--json", Description: "Output conversations and project identity as JSON"}}},
				{Name: "handle", Description: "Attach a machine-local handle: loaf conversation handle add <conversation-id> --harness <h> --handle <id>", Options: []cliReferenceOption{
					{Flags: "--harness <harness>", Description: "Harness name, e.g. codex or claude-code"},
					{Flags: "--handle <id>", Description: "Opaque machine-local conversation identifier"},
					{Flags: "--locality <scope>", Description: "Machine or namespace scope for the handle"},
					{Flags: "--log-ref <locator>", Description: "Bounded log locator, never transcript content"},
					{Flags: "--hash <sha256>", Description: "Optional SHA-256 of the referenced log range"},
					{Flags: "--range <range>", Description: "Optional bounded range within the log"},
					{Flags: "--json", Description: "Output the handle result and project identity as JSON"},
				}},
				{Name: "observe", Description: "Append an immutable timestamped availability observation; the observed row never mutates", Options: []cliReferenceOption{
					{Flags: "--handle <handle-id>", Description: "Observed conversation handle ID"},
					{Flags: "--log-ref <log-ref-id>", Description: "Observed log reference ID"},
					{Flags: "--available", Description: "Record that the source was reachable"},
					{Flags: "--unavailable", Description: "Record that the source was not reachable"},
					{Flags: "--observer <name>", Description: "Observing agent or probe"},
					{Flags: "--locality <scope>", Description: "Machine or namespace of the observation"},
					{Flags: "--note <text>", Description: "Bounded observation note"},
					{Flags: "--json", Description: "Output the observation result and project identity as JSON"},
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
				{Name: "resolve", Description: "Resolve a spark by linking it to the entity that resolves it", Options: []cliReferenceOption{
					{Flags: "--by <entity>", Description: "Resolving entity reference (required)"},
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
			Description: "Diagnose Loaf project alignment (symlinks, stale files, leftover SQLite work, issue prefix and config)",
			Options: []cliReferenceOption{
				{Flags: "--fix", Description: "Offer each safe repair and prompt y/N before applying it"},
				{Flags: "--force", Description: "With --fix, apply every offered repair without prompting"},
				{Flags: "--verbose", Description: "Print each check name even when passing"},
				{Flags: "--json", Description: "Output the identical check set as read-only JSON; never prompts or repairs"},
			},
		},
	}
}

func generateCLIReferenceSkill(commands []cliReferenceCommand) string {
	header := `---
name: loaf-reference
description: >-
  Documents how agents operate the Loaf CLI: command discovery via loaf --help, JSON diagnosis surfaces, config-aware maintenance, and troubleshooting. Use when unsure which loaf command to invoke, how to validate project state, or when asked to upgrade, diagnose, repair, configure, or bring a Loaf project current. Not for workflow guidance (workflow skills own their CLI contracts) or build internals.
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
		"- Prefer `--json` surfaces when diagnosing: `loaf config check --json`, `loaf state doctor --json`. Parse the structured output instead of scraping human-readable text.",
		"- Run the deterministic CLI command before hand-editing anything it manages; the command owns its files.",
		"- Use `--fix` only for safe, mechanical repairs, and review what it changed.",
		"- Ask the user for project-owned choices — GitHub account, issue identity (authority and prefix), tracker or integration election, which harnesses to install — never guess them.",
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
		"| `transitional-tasks` | Open task-board records retained for compatibility. |",
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
		"| Config-aware maintenance protocol | [references/maintenance.md](references/maintenance.md) | Upgrading, diagnosing, repairing, or bringing a project current: diagnose, plan, ask, apply, verify |",
		"| Command routing | [references/command-routing.md](references/command-routing.md) | Deciding which command a task needs; locating the JSON diagnosis surfaces |",
		"| Markdown migration | [references/markdown-migration.md](references/markdown-migration.md) | Running `loaf migrate markdown`: simulation vs inventory mode, import_report, origin/status authority |",
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
