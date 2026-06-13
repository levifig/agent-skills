package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
				{Flags: "--upgrade", Description: "Update only already-installed targets"},
				{Flags: "-y, --yes", Description: "Assume 'yes' to safe migrations (merge content, back up, and replace real files with symlinks)"},
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
			Name:        "state",
			Description: "Manage native SQLite state",
			Subcommands: []cliReferenceSubcommand{
				{Name: "path", Description: "Print the resolved SQLite database path"},
				{Name: "status", Description: "Show SQLite readiness and markdown-only compatibility status", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output status as JSON"},
				}},
				{Name: "init", Description: "Initialize an empty SQLite state database", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output initialized status as JSON"},
				}},
				{Name: "doctor", Description: "Diagnose SQLite state health", Options: []cliReferenceOption{
					{Flags: "--fix", Description: "Initialize missing SQLite state when safe"},
					{Flags: "--json", Description: "Output diagnostics as JSON"},
				}},
				{Name: "migrate markdown", Description: "Import existing .agents Markdown artifacts into SQLite", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview import counts without creating a database"},
					{Flags: "--apply", Description: "Initialize SQLite and import Markdown artifacts"},
					{Flags: "--resume", Description: "Resume the Markdown import after an interrupted attempt"},
					{Flags: "--json", Description: "Output migration details as JSON"},
				}},
				{Name: "migrate storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview the storage-home migration"},
					{Flags: "--apply", Description: "Copy the legacy database without deleting it"},
					{Flags: "--json", Description: "Output migration details as JSON"},
				}},
				{Name: "backup", Description: "Create a SQLite database backup", Options: []cliReferenceOption{{Flags: "--json", Description: "Output backup details as JSON"}}},
				{Name: "export", Description: "Export SQLite state for review or migration"},
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
					{Flags: "--json", Description: "Output migration details as JSON"},
				}},
				{Name: "storage-home", Description: "Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME", Options: []cliReferenceOption{
					{Flags: "--dry-run", Description: "Preview the storage-home migration"},
					{Flags: "--apply", Description: "Copy the legacy database without deleting it"},
					{Flags: "--json", Description: "Output migration details as JSON"},
				}},
				{Name: "worktree-storage", Description: "Move linked-worktree .agents state to the main worktree"},
			},
		},
		{
			Name:        "task",
			Description: "Manage project tasks",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "Show task board grouped by status", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output raw JSON"},
					{Flags: "--active", Description: "Hide completed tasks"},
					{Flags: "--status <status>", Description: "Only show tasks with status: in_progress, blocked, todo, review, done"},
				}},
				{Name: "show", Description: "Display a single task's details", Options: []cliReferenceOption{
					{Flags: "--json", Description: "Output task entry as JSON"},
				}},
				{Name: "status", Description: "Show task summary counts"},
				{Name: "create", Description: "Create a new task", Options: []cliReferenceOption{
					{Flags: "--title <title>", Description: "Task title"},
					{Flags: "--spec <id>", Description: "Associated spec ID (e.g., SPEC-010)"},
					{Flags: "--priority <level>", Description: "Priority level (P0/P1/P2/P3)"},
					{Flags: "--depends-on <ids>", Description: "Comma-separated task IDs"},
				}},
				{Name: "update", Description: "Update a task's metadata", Options: []cliReferenceOption{
					{Flags: "--status <status>", Description: "New status: todo, in_progress, blocked, review, done"},
					{Flags: "--priority <level>", Description: "New priority: P0, P1, P2, P3"},
					{Flags: "--depends-on <ids>", Description: "Replace depends_on (comma-separated task IDs)"},
					{Flags: "--session <file>", Description: `Set or clear session reference (use "none" to clear)`},
					{Flags: "--spec <id>", Description: "Set or change associated spec"},
				}},
				{Name: "archive", Description: "Archive completed tasks through the task lifecycle", Options: []cliReferenceOption{
					{Flags: "--spec <id>", Description: "Archive all done tasks for a spec"},
				}},
				{Name: "refresh", Description: "Compatibility: rebuild the Markdown task index from task/spec files"},
				{Name: "sync", Description: "Compatibility: sync the Markdown task index and task files", Options: []cliReferenceOption{
					{Flags: "--import", Description: "Import orphan .md files not in the index"},
					{Flags: "--push", Description: "Push compatibility index metadata into .md frontmatter"},
				}},
			},
		},
		{
			Name:        "spec",
			Description: "Manage project specs",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "Show specs with status and task counts", Options: []cliReferenceOption{{Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "show", Description: "Show spec details", Options: []cliReferenceOption{{Flags: "--json", Description: "Output raw JSON"}}},
				{Name: "archive", Description: "Archive a completed spec", Options: []cliReferenceOption{{Flags: "--json", Description: "Output raw JSON"}}},
			},
		},
		{
			Name:        "report",
			Description: "Manage durable reports (research, audits, investigations)",
			Subcommands: []cliReferenceSubcommand{
				{Name: "list", Description: "List reports", Options: []cliReferenceOption{
					{Flags: "--type <type>", Description: "Filter by report type"},
					{Flags: "--status <status>", Description: "Filter by status"},
					{Flags: "--json", Description: "Output as JSON"},
				}},
				{Name: "generate", Description: "Generate a report from state", Options: []cliReferenceOption{{Flags: "--format <format>", Description: "Output format"}}},
				{Name: "create", Description: "Create a report draft", Options: []cliReferenceOption{
					{Flags: "--type <type>", Description: "Report type"},
					{Flags: "--source <source>", Description: "Report source"},
					{Flags: "--json", Description: "Output as JSON"},
				}},
				{Name: "finalize", Description: "Mark a report draft as final", Options: []cliReferenceOption{{Flags: "--json", Description: "Output as JSON"}}},
				{Name: "archive", Description: "Archive a finalized report", Options: []cliReferenceOption{{Flags: "--json", Description: "Output as JSON"}}},
			},
		},
		{
			Name:        "kb",
			Description: "Knowledge base management",
			Subcommands: []cliReferenceSubcommand{
				{Name: "glossary", Description: "Domain glossary mutation and lookup"},
				{Name: "validate", Description: "Validate knowledge file frontmatter", Options: []cliReferenceOption{{Flags: "--json", Description: "Output results as JSON"}}},
				{Name: "status", Description: "Show knowledge base overview", Options: []cliReferenceOption{{Flags: "--json", Description: "Output status as JSON"}}},
				{Name: "check", Description: "Check knowledge file staleness against git history", Options: []cliReferenceOption{
					{Flags: "--file <path>", Description: "Reverse lookup: find knowledge files covering this path"},
					{Flags: "--json", Description: "Output results as JSON"},
				}},
				{Name: "review", Description: "Mark a knowledge file as reviewed today", Options: []cliReferenceOption{{Flags: "--json", Description: "Output updated frontmatter as JSON"}}},
				{Name: "init", Description: "Initialize knowledge base directories and QMD collections", Options: []cliReferenceOption{{Flags: "--json", Description: "Output results as JSON"}}},
				{Name: "import", Description: "Import external project knowledge via QMD collection", Options: []cliReferenceOption{
					{Flags: "--path <path>", Description: "Path to the external project's knowledge directory"},
					{Flags: "--json", Description: "Output results as JSON"},
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
				{Flags: "--json", Description: "Output as JSON"},
				{Flags: "--sessions", Description: "Only review sessions"},
				{Flags: "--specs", Description: "Only review specs"},
				{Flags: "--plans", Description: "Only review plans"},
				{Flags: "--drafts", Description: "Only review drafts"},
				{Flags: "--handoffs", Description: "Only review handoffs"},
			},
		},
		{
			Name:        "check",
			Description: "Run enforcement hook checks",
			Options: []cliReferenceOption{
				{Flags: "--hook <id>", Description: "Registered hook ID to run"},
				{Flags: "--json", Description: "Output JSON format"},
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
			{Name: "finalize", Description: "Transition a draft report to final"},
			{Name: "archive", Description: "Transition a final report to archived"},
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
		return "Existing TypeScript-era projects can keep running supported commands in\nmarkdown-only compatibility mode until SQLite is initialized. Use\n`loaf state migrate markdown --apply` to import `.agents/` Markdown into SQLite\nwithout rewriting the source Markdown files."
	case "migrate":
		return "`loaf migrate markdown` is the upgrade path for existing `.agents/`\nprojects with no SQLite database. Start with `--dry-run`, then use `--apply`\nwhen the artifact counts and skipped files look right."
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
			"loaf state status",
		}
	case "migrate":
		return []string{
			"loaf migrate markdown --dry-run",
			"loaf migrate markdown --apply",
			"loaf migrate storage-home --dry-run",
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
