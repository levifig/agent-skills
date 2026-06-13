package cli

import "io"

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
				{Flags: "--target <target>", Description: "Build only one target"},
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
				{Flags: "--to <targets>", Description: "Comma-separated target tools or all"},
				{Flags: "--upgrade", Description: "Upgrade already-installed targets"},
				{Flags: "--yes", Description: "Skip confirmation prompts"},
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
				{Name: "path", Description: "Print the native SQLite database path"},
				{Name: "init", Description: "Initialize native SQLite state"},
				{Name: "status", Description: "Show native state status"},
				{Name: "doctor", Description: "Diagnose native state health"},
				{Name: "migrate", Description: "Run state migrations"},
				{Name: "backup", Description: "Create a SQLite database backup"},
				{Name: "export", Description: "Export state data"},
			},
		},
		{
			Name:        "migrate",
			Description: "Run migration workflows",
			Subcommands: []agentHelpSubcommand{
				{Name: "markdown", Description: "Import markdown artifacts into native SQLite state"},
				{Name: "storage-home", Description: "Move durable SQLite state to XDG_DATA_HOME"},
				{Name: "worktree-storage", Description: "Move linked-worktree .agents content to the main checkout"},
			},
		},
		{
			Name:        "session",
			Description: "Manage sessions",
			Subcommands: []agentHelpSubcommand{
				{Name: "start", Description: "Start or resume a session"},
				{Name: "end", Description: "End the current session"},
				{Name: "archive", Description: "Archive completed sessions"},
				{Name: "list", Description: "List sessions"},
				{Name: "show", Description: "Display one session"},
				{Name: "log", Description: "Append a journal entry"},
				{Name: "report", Description: "Generate a session report"},
				{Name: "enrich", Description: "Summarize compatibility enrichment status"},
				{Name: "housekeeping", Description: "Summarize session housekeeping status"},
				{Name: "state", Description: "Manage session current-state metadata"},
				{Name: "context", Description: "Render session context for compaction or resumption"},
			},
		},
		{
			Name:        "task",
			Description: "Manage project tasks",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "Show task board grouped by status", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}, {Flags: "--active", Description: "Hide completed tasks"}, {Flags: "--status <status>", Description: "Filter by task status"}}},
				{Name: "show", Description: "Display a single task's details", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "status", Description: "Show task summary counts"},
				{Name: "create", Description: "Create a new task", Options: []agentHelpOption{{Flags: "--title <title>", Description: "Task title"}, {Flags: "--spec <id>", Description: "Associated spec ID"}, {Flags: "--priority <level>", Description: "Priority level"}, {Flags: "--depends-on <ids>", Description: "Comma-separated dependency task IDs"}}},
				{Name: "update", Description: "Update a task's metadata", Options: []agentHelpOption{{Flags: "--status <status>", Description: "New task status"}, {Flags: "--priority <level>", Description: "New task priority"}, {Flags: "--spec <id>", Description: "Set or clear associated spec"}, {Flags: "--depends-on <ids>", Description: "Replace dependencies"}, {Flags: "--session <file>", Description: "Set or clear session reference"}}},
				{Name: "archive", Description: "Archive completed tasks", Options: []agentHelpOption{{Flags: "--spec <id>", Description: "Archive done tasks for a spec"}}},
				{Name: "refresh", Description: "Rebuild the Markdown task index from task/spec files"},
				{Name: "sync", Description: "Sync the Markdown task index and task files", Options: []agentHelpOption{{Flags: "--import", Description: "Import orphan markdown files"}, {Flags: "--push", Description: "Push index metadata into markdown frontmatter"}}},
			},
		},
		{
			Name:        "spec",
			Description: "Manage project specs",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "Show specs with status and task counts", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "show", Description: "Show spec details", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "archive", Description: "Archive a completed spec", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}}},
			},
		},
		{
			Name:        "report",
			Description: "Manage durable reports",
			Subcommands: []agentHelpSubcommand{
				{Name: "list", Description: "List reports", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Filter by report type"}, {Flags: "--status <status>", Description: "Filter by status"}, {Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "generate", Description: "Generate a report from state", Options: []agentHelpOption{{Flags: "--format <format>", Description: "Output format"}}},
				{Name: "create", Description: "Create a report draft", Options: []agentHelpOption{{Flags: "--type <type>", Description: "Report type"}, {Flags: "--source <source>", Description: "Report source"}, {Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "finalize", Description: "Mark a report draft as final", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "archive", Description: "Archive a finalized report", Options: []agentHelpOption{{Flags: "--json", Description: "Output raw JSON"}}},
			},
		},
		{
			Name:        "kb",
			Description: "Knowledge base management",
			Subcommands: []agentHelpSubcommand{
				{Name: "status", Description: "Show knowledge base status"},
				{Name: "validate", Description: "Validate knowledge file frontmatter"},
				{Name: "check", Description: "Check knowledge staleness"},
				{Name: "review", Description: "Mark knowledge files reviewed"},
				{Name: "init", Description: "Initialize knowledge directories"},
				{Name: "import", Description: "Register external knowledge imports"},
				{Name: "glossary", Description: "Domain glossary mutation and lookup"},
			},
		},
		{
			Name:        "check",
			Description: "Run hook checks",
			Options: []agentHelpOption{
				{Flags: "--hook <id>", Description: "Run one registered hook"},
				{Flags: "--json", Description: "Output raw JSON"},
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
		{Name: "housekeeping", Description: "Scan agent artifacts and summarize housekeeping recommendations"},
		{Name: "trace", Description: "Trace relationships for an entity"},
		{Name: "brainstorm", Description: "Manage brainstorm artifacts"},
		{Name: "idea", Description: "Manage ideas"},
		{Name: "spark", Description: "Manage sparks"},
		{Name: "tag", Description: "Manage tags"},
		{Name: "bundle", Description: "Manage bundles"},
		{Name: "link", Description: "Manage entity relationships"},
	}
}
