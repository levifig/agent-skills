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
			Description: "Install Loaf into detected or selected agent tools",
			Options: []agentHelpOption{
				{Flags: "--to <target>", Description: "Target to install to, or all"},
				{Flags: "--upgrade", Description: "Upgrade already-installed targets"},
				{Flags: "-y, --yes", Description: "Assume yes to safe project-file symlink migrations"},
				{Flags: "--no-yes", Description: "Force prompt-style declines in non-interactive mode"},
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
				{Name: "doctor", Description: "Diagnose native state health", Options: []agentHelpOption{{Flags: "--fix", Description: "Apply safe repairs"}, {Flags: "--dry-run", Description: "Preview repairs without writing"}, {Flags: "--json", Description: "Output diagnostics, repair plan, global database scope, and project identity as JSON"}}},
				{Name: "repair", Description: "Repair guarded SQLite data drift"},
				{Name: "repair legacy-project-database", Description: "Archive migrated per-project SQLite leftovers", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview archive paths without writing"}, {Flags: "--apply", Description: "Move legacy SQLite files into the archive directory"}, {Flags: "--json", Description: "Output archive plan/result, global database scope, and project identity as JSON"}}},
				{Name: "repair relationship-origin", Description: "Backfill missing relationship provenance", Options: []agentHelpOption{{Flags: "--origin <imported|manual>", Description: "Provenance value to set"}, {Flags: "--dry-run", Description: "Preview affected rows without writing"}, {Flags: "--apply", Description: "Apply the backfill"}, {Flags: "--json", Description: "Output repair plan/result, global database scope, and project identity as JSON"}}},
				{Name: "migrate", Description: "Run state migrations"},
				{Name: "migrate markdown", Description: "Import markdown artifacts into native SQLite state", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview import work without creating SQLite state"}, {Flags: "--apply", Description: "Initialize SQLite and apply the import"}, {Flags: "--resume", Description: "Resume an interrupted import"}, {Flags: "--json", Description: "Output migration contract, scope, project context, and counts as JSON"}}},
				{Name: "migrate storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into the global XDG_DATA_HOME database", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview migration work without copying"}, {Flags: "--apply", Description: "Copy or merge eligible legacy state without deleting the source"}, {Flags: "--json", Description: "Output migration contract, global database paths, action, and project identity when available"}}},
				{Name: "backup", Description: "Create a SQLite database backup under the global data-home backups directory", Options: []agentHelpOption{{Flags: "--json", Description: "Output backup verification, checksum, schema version, project count, and current project identity as JSON"}}},
				{Name: "backup verify", Description: "Verify an existing SQLite database backup", Options: []agentHelpOption{{Flags: "--json", Description: "Output backup verification, restore guidance, schema version, and captured project identities as JSON"}}},
				{Name: "export", Description: "Export state data", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format for the selected export kind"}}},
				{Name: "export all", Description: "Export a complete project-scoped SQLite snapshot", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: json"}, {Flags: "--json", Description: "Alias for --format json"}}},
				{Name: "export triage", Description: "Export a triage summary from SQLite state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
				{Name: "export session", Description: "Export one session from SQLite state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}}},
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
			Name:        "migrate",
			Description: "Run migration workflows",
			Subcommands: []agentHelpSubcommand{
				{Name: "markdown", Description: "Import markdown artifacts into native SQLite state", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview import work without creating SQLite state"}, {Flags: "--apply", Description: "Initialize SQLite and apply the import"}, {Flags: "--resume", Description: "Resume an interrupted import"}, {Flags: "--json", Description: "Output migration contract, scope, project context, and counts as JSON"}}},
				{Name: "storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into the global XDG_DATA_HOME database", Options: []agentHelpOption{{Flags: "--dry-run", Description: "Preview migration work without copying"}, {Flags: "--apply", Description: "Copy or merge eligible legacy state without deleting the source"}, {Flags: "--json", Description: "Output migration contract, global database paths, action, and project identity when available"}}},
				{Name: "worktree-storage", Description: "Move linked-worktree .agents content to the main checkout", Options: []agentHelpOption{{Flags: "--apply", Description: "Perform the migration; dry-run is the default"}, {Flags: "--force-from-worktree", Description: "On conflict, keep the worktree-local copy"}, {Flags: "--force-from-main", Description: "On conflict, keep the main-worktree copy"}}},
			},
		},
		{
			Name:        "session",
			Description: "Manage sessions",
			Subcommands: []agentHelpSubcommand{
				{Name: "start", Description: "Start or resume a session", Options: []agentHelpOption{{Flags: "--resume", Description: "Resume if possible"}, {Flags: "--session-id <id>", Description: "Harness session ID"}, {Flags: "--force", Description: "Ignore hook agent adoption guard"}, {Flags: "--json", Description: "Output action, session, journal IDs, global database scope, and project identity as JSON"}}},
				{Name: "end", Description: "End the current session", Options: []agentHelpOption{{Flags: "--if-active", Description: "No-op when no active session exists"}, {Flags: "--wrap", Description: "Mark as wrapped"}, {Flags: "--from-hook", Description: "Read hook input"}, {Flags: "--session-id <id>", Description: "Harness session ID"}, {Flags: "--json", Description: "Output action/noop, session, journal IDs, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive completed sessions", Options: []agentHelpOption{{Flags: "--branch <branch>", Description: "Branch to archive"}, {Flags: "--session-id <id>", Description: "Harness session ID"}, {Flags: "--json", Description: "Output archive result, affected sessions, global database scope, and project identity as JSON"}}},
				{Name: "list", Description: "List sessions", Options: []agentHelpOption{{Flags: "--all", Description: "Include archived sessions"}, {Flags: "--json", Description: "Output sessions, diagnostics, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Display one session", Options: []agentHelpOption{{Flags: "--json", Description: "Output session details, journal entries, relationships, global database scope, and project identity as JSON"}}},
				{Name: "log", Description: "Append a journal entry", Options: []agentHelpOption{{Flags: "--from-hook", Description: "Read hook input"}, {Flags: "--session-id <id>", Description: "Harness session ID"}, {Flags: "--json", Description: "Output journal entry, linked session, global database scope, and project identity as JSON"}}},
				{Name: "report", Description: "Generate a session report", Options: []agentHelpOption{{Flags: "--json", Description: "Output export contract, command, project context, and markdown content as JSON"}}},
				{Name: "enrich", Description: "Summarize compatibility enrichment status", Options: []agentHelpOption{{Flags: "--json", Description: "Output compatibility mode, action, reason, and counts as JSON"}}},
				{Name: "housekeeping", Description: "Summarize session housekeeping status", Options: []agentHelpOption{{Flags: "--json", Description: "Output compatibility mode, action, reason, and counts as JSON"}}},
				{Name: "state", Description: "Manage session current-state metadata"},
				{Name: "context", Description: "Render session context for compaction or resumption"},
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
				{Name: "update", Description: "Update a task's metadata", Options: []agentHelpOption{{Flags: "--status <status>", Description: "New task status: " + strings.Join(state.TaskStatuses(), ", ")}, {Flags: "--priority <level>", Description: "New task priority: " + strings.Join(state.TaskPriorities(), ", ")}, {Flags: "--spec <id>", Description: "Set or clear associated spec"}, {Flags: "--depends-on <ids>", Description: "Replace dependencies"}, {Flags: "--session <file>", Description: "Set or clear session reference"}, {Flags: "--json", Description: "Output updated task, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive completed tasks", Options: []agentHelpOption{{Flags: "--spec <id>", Description: "Archive done tasks for a spec"}, {Flags: "--json", Description: "Output archive result, archived tasks, global database scope, and project identity as JSON"}}},
				{Name: "refresh", Description: "Rebuild the Markdown task index from task/spec files", Options: []agentHelpOption{{Flags: "--json", Description: "Output compatibility mode, action, reason, and counts as JSON"}}},
				{Name: "sync", Description: "Sync the Markdown task index and task files", Options: []agentHelpOption{{Flags: "--import", Description: "Import orphan markdown files"}, {Flags: "--push", Description: "Push index metadata into markdown frontmatter"}, {Flags: "--json", Description: "Output compatibility mode, action, reason, and counts as JSON"}}},
			},
		},
		{
			Name:        "spec",
			Description: "Manage project specs",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "Show specs with status and task counts", Options: []agentHelpOption{{Flags: "--json", Description: "Output specs, diagnostics, task counts, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show spec details", Options: []agentHelpOption{{Flags: "--json", Description: "Output spec details, task counts, relationships, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a completed spec", Options: []agentHelpOption{{Flags: "--json", Description: "Output archive result, archived specs, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "report",
			Description: "Manage durable reports",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List reports", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Filter by report type"}, {Flags: "--status <status>", Description: "Filter by status; Loaf lifecycle statuses: draft, final, archived"}, {Flags: "--json", Description: "Output reports, diagnostics, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one report", Options: []agentHelpOption{{Flags: "--json", Description: "Output report details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "generate", Description: "Generate a report from state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format: markdown"}, {Flags: "--json", Description: "Output contract, command, project context, and markdown content as JSON"}}},
				{Name: "create", Description: "Create a report draft", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Report type"}, {Flags: "--source <source>", Description: "Report source"}, {Flags: "--body-file <path>", Description: "Read Markdown body from a UTF-8 file"}, {Flags: "--body -", Description: "Read Markdown body from stdin"}, {Flags: "--message <text>", Description: "Use inline Markdown body text"}, {Flags: "--json", Description: "Output created report, event, global database scope, and project identity as JSON"}}},
				{Name: "finalize", Description: "Mark a report draft as final", Options: []agentHelpOption{{Flags: "--json", Description: "Output report status transition, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive a finalized report", Options: []agentHelpOption{{Flags: "--json", Description: "Output report status transition, event, global database scope, and project identity as JSON"}}},
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
		{Name: "doctor", Description: "Diagnose project alignment", Options: []agentHelpOption{{Flags: "--fix", Description: "Apply safe fixes"}, {Flags: "--verbose", Description: "Show details"}}},
		{
			Name:        "release",
			Description: "Create a new release with changelog, version bump, and tag",
			Options: []agentHelpOption{
				{Flags: "--dry-run", Description: "Preview release without making changes"},
				{Flags: "--bump <type>", Description: "Skip interactive bump choice"},
				{Flags: "--base <ref>", Description: "Use commits since ref"},
				{Flags: "--no-tag", Description: "Skip git tag creation"},
				{Flags: "--tag", Description: "Force git tag creation"},
				{Flags: "--no-gh", Description: "Skip GitHub release draft"},
				{Flags: "--gh", Description: "Force GitHub release draft"},
				{Flags: "--version-file <path>", Description: "Override version file path"},
				{Flags: "--pre-merge", Description: "Prepare release artifacts before squash-merge"},
				{Flags: "--post-merge", Description: "Finalize release after squash-merge"},
				{Flags: "-y, --yes", Description: "Skip confirmation prompt"},
			},
		},
		{Name: "version", Description: "Show version and content counts"},
		{Name: "housekeeping", Description: "Scan agent artifacts and summarize housekeeping recommendations", Options: []agentHelpOption{{Flags: "--json", Description: "Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON"}, {Flags: "--dry-run", Description: "Show recommendations without applying actions"}, {Flags: "--sessions", Description: "Only review sessions"}, {Flags: "--specs", Description: "Only review specs"}, {Flags: "--drafts", Description: "Only review shaping drafts"}, {Flags: "--plans", Description: "Accept legacy plans filter for compatibility"}, {Flags: "--handoffs", Description: "Accept legacy handoffs filter for compatibility"}}},
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
				{Name: "list", Description: "List ideas from SQLite state", Options: []agentHelpOption{{Flags: "--all", Description: "Include resolved and archived ideas"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output ideas, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one idea from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output idea details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "capture", Description: "Capture an idea in SQLite state", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Idea title"}, {Flags: "--json", Description: "Output created idea, event, global database scope, and project identity as JSON"}}},
				{Name: "promote", Description: "Record idea-to-spec promotion", Options: []agentHelpOption{{Flags: "--to-spec <spec>", Description: "Target spec"}, {Flags: "--json", Description: "Output promotion relationship, global database scope, and project identity as JSON"}}},
				{Name: "resolve", Description: "Resolve an idea by linking it to another entity", Options: []agentHelpOption{{Flags: "--by <entity>", Description: "Resolving entity"}, {Flags: "--json", Description: "Output resolution relationship, event, global database scope, and project identity as JSON"}}},
				{Name: "archive", Description: "Archive one or more ideas", Options: []agentHelpOption{{Flags: "--reason <text>", Description: "Archive reason"}, {Flags: "--json", Description: "Output archive result, archived ideas, global database scope, and project identity as JSON"}}},
			},
		},
		{
			Name:        "spark",
			Description: "Manage sparks",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List sparks from SQLite state", Options: []agentHelpOption{{Flags: "--all", Description: "Include resolved sparks"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output sparks, global database scope, and project identity as JSON"}}},
				{Name: "show", Description: "Show one spark from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output spark details, relationships, global database scope, and project identity as JSON"}}},
				{Name: "capture", Description: "Capture a spark in SQLite state", Options: []agentHelpOption{{Flags: "--scope <scope>", Description: "Spark scope"}, {Flags: "--text <text>", Description: "Spark text"}, {Flags: "--json", Description: "Output created spark, event, global database scope, and project identity as JSON"}}},
				{Name: "resolve", Description: "Resolve a spark", Options: []agentHelpOption{{Flags: "--reason <text>", Description: "Resolution reason"}, {Flags: "--json", Description: "Output resolution relationship, event, global database scope, and project identity as JSON"}}},
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
			agentHelpOption{Flags: "--session <session>", Description: "Optional related session"},
			agentHelpOption{Flags: "--task <task>", Description: "Optional related task"},
		)
	}
	options = append(options, agentHelpOption{Flags: "--json", Description: "Output created artifact, event, global database scope, and project identity as JSON"})
	return agentHelpCommand{
		Name:        kind,
		Description: "Manage " + kind + "s in native SQLite state",
		Subcommands: []agentHelpSubcommand{
			{Name: "new", Description: "Create a " + kind + " in SQLite state", Options: options},
			{Name: "show", Description: "Show one " + kind + " from SQLite state", Options: []agentHelpOption{{Flags: "--json", Description: "Output artifact details, relationships, global database scope, and project identity as JSON"}}},
			{Name: "list", Description: "List " + kind + "s from SQLite state", Options: []agentHelpOption{{Flags: "--all", Description: "Include archived artifacts"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output artifacts, global database scope, and project identity as JSON"}}},
			{Name: "link", Description: "Link a " + kind + " to another entity", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Relationship type; defaults to related_to"}, {Flags: "--reason <text>", Description: "Relationship reason"}, {Flags: "--json", Description: "Output relationship ID, source/target, global database scope, and project identity as JSON"}}},
		},
	}
}
