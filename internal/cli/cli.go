package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/legacy"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// Runner owns Go-native command dispatch and delegates unmigrated commands to
// the bundled TypeScript compatibility CLI.
type Runner struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	WorkingDir string
	StateHome  string
	Legacy     legacy.Runner
}

type housekeepingOptions struct {
	jsonOutput bool
	dryRun     bool
	sections   map[string]bool
}

type compatibilityCommandSummary struct {
	Version int            `json:"version"`
	Command string         `json:"command"`
	Mode    string         `json:"mode"`
	Action  string         `json:"action"`
	Reason  string         `json:"reason"`
	Counts  map[string]int `json:"counts,omitempty"`
}

type compatibilityCommandOptions struct {
	jsonOutput bool
}

// Run dispatches a loaf command.
func (r Runner) Run(args []string) error {
	out := r.Stdout
	if out == nil {
		out = os.Stdout
	}

	workingDir, err := project.ResolveWorkingDirectory(r.WorkingDir)
	if err != nil {
		return err
	}
	runtime := state.NewRuntime(workingDir)

	if len(args) == 0 {
		return r.runLegacy(args, runtime.RootPath(), out)
	}

	switch args[0] {
	case "state":
		return r.runState(args[1:], out, runtime)
	case "trace":
		return r.runTrace(args[1:], out, runtime)
	case "brainstorm":
		return r.runBrainstorm(args[1:], out, runtime)
	case "idea":
		return r.runIdea(args[1:], out, runtime)
	case "spark":
		return r.runSpark(args[1:], out, runtime)
	case "tag":
		return r.runTag(args[1:], out, runtime)
	case "bundle":
		return r.runBundle(args[1:], out, runtime)
	case "link":
		return r.runLink(args[1:], out, runtime)
	case "report":
		return r.runReport(args[1:], out, runtime)
	case "spec":
		return r.runSpec(args[1:], out, runtime)
	case "session":
		return r.runSession(args[1:], out, runtime)
	case "task":
		return r.runTask(args[1:], out, runtime)
	case "housekeeping":
		return r.runHousekeeping(args[1:], out, runtime)
	default:
		return r.runLegacy(args, runtime.RootPath(), out)
	}
}

func (r Runner) runHousekeeping(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseHousekeepingArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"housekeeping"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.Housekeeping(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	result = filterHousekeepingSummary(result, options.sections)
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeHousekeepingSummary(out, result, options)
	return nil
}

func (r Runner) stateMode(runtime state.Runtime) (project.Root, string, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, "", err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, "", err
	}
	return projectRoot, status.Mode, nil
}

func sqliteStateRequiredError(command string) error {
	return fmt.Errorf("loaf %s requires initialized SQLite state; run `loaf state init` or `loaf state migrate markdown --apply` first", command)
}

func missingSubcommandError(command string) error {
	return fmt.Errorf("loaf %s requires a subcommand", command)
}

func unknownSubcommandError(command string, subcommand string) error {
	return fmt.Errorf("unknown loaf %s subcommand %q", command, subcommand)
}

func writeHousekeepingSummary(out io.Writer, result state.HousekeepingSummary, options housekeepingOptions) {
	if options.dryRun {
		fmt.Fprint(out, "\n  loaf housekeeping (SQLite state, dry run)\n\n")
	} else {
		fmt.Fprint(out, "\n  loaf housekeeping (SQLite state)\n\n")
	}
	fmt.Fprintf(out, "  database: %s\n\n", result.DatabasePath)
	for _, name := range sortedHousekeepingSections(result) {
		section := result.Sections[name]
		fmt.Fprintf(out, "    %-16s%d total", housekeepingSectionLabel(name), section.Total)
		if section.CleanupCandidate > 0 {
			fmt.Fprintf(out, "  %d cleanup candidate(s)", section.CleanupCandidate)
		}
		fmt.Fprintln(out)
		for _, status := range sortedCountKeys(section.ByStatus) {
			fmt.Fprintf(out, "      %-12s%d\n", status+":", section.ByStatus[status])
		}
	}
	if len(result.Signals) == 0 {
		fmt.Fprint(out, "\n  No SQLite housekeeping signals.\n\n")
		return
	}
	fmt.Fprint(out, "\n  Signals:\n")
	for _, signal := range result.Signals {
		fmt.Fprintf(out, "    %s\n", signal)
	}
	fmt.Fprintln(out)
}

func writeCompatibilityCommandSummary(out io.Writer, summary compatibilityCommandSummary) {
	fmt.Fprintf(out, "\n  loaf %s (SQLite state)\n\n", summary.Command)
	fmt.Fprintf(out, "  mode: %s\n", summary.Mode)
	fmt.Fprintf(out, "  action: %s\n", summary.Action)
	fmt.Fprintf(out, "  %s\n", summary.Reason)
	if len(summary.Counts) > 0 {
		fmt.Fprintln(out)
		for _, key := range sortedCountKeys(summary.Counts) {
			fmt.Fprintf(out, "    %-10s%d\n", key+":", summary.Counts[key])
		}
	}
	fmt.Fprintln(out)
}

func filterHousekeepingSummary(result state.HousekeepingSummary, sections map[string]bool) state.HousekeepingSummary {
	if len(sections) == 0 {
		return result
	}
	filtered := state.HousekeepingSummary{
		Version:      result.Version,
		DatabasePath: result.DatabasePath,
		Sections:     map[string]state.HousekeepingSection{},
	}
	for section := range sections {
		if value, ok := result.Sections[section]; ok {
			filtered.Sections[section] = value
		}
	}
	filtered.Signals = housekeepingSignalsFromSections(filtered.Sections)
	return filtered
}

func housekeepingSignalsFromSections(sections map[string]state.HousekeepingSection) []string {
	var signals []string
	for _, name := range sortedHousekeepingSectionNames(sections) {
		section := sections[name]
		if section.CleanupCandidate > 0 {
			signals = append(signals, fmt.Sprintf("%s:%d", name, section.CleanupCandidate))
		}
	}
	return signals
}

func (r Runner) runBrainstorm(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return missingSubcommandError("brainstorm")
	}
	switch args[0] {
	case "list":
		return r.runBrainstormList(args[1:], out, runtime)
	case "show":
		return r.runBrainstormShow(args[1:], out, runtime)
	case "promote":
		return r.runBrainstormPromote(args[1:], out, runtime)
	case "archive":
		return r.runBrainstormArchive(args[1:], out, runtime)
	default:
		return unknownSubcommandError("brainstorm", args[0])
	}
}

func (r Runner) runBrainstormList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseBrainstormListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("brainstorm list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	brainstorms, err := state.ListBrainstorms(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, brainstorms)
	}
	writeBrainstormList(out, brainstorms, options.filters)
	return nil
}

func (r Runner) runBrainstormShow(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("brainstorm show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	ref, jsonOutput, err := parseSingleRefArgs("brainstorm show", args)
	if err != nil {
		return err
	}
	result, err := state.ShowBrainstorm(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeBrainstormShow(out, result)
	return nil
}

func (r Runner) runBrainstormPromote(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("brainstorm promote")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseBrainstormPromoteArgs(args)
	if err != nil {
		return err
	}
	result, err := state.PromoteBrainstorm(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.promote)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "promoted brainstorm %s to idea %s\n", firstNonEmpty(result.Brainstorm.Alias, result.Brainstorm.ID), firstNonEmpty(result.Idea.Alias, result.Idea.ID))
	fmt.Fprintf(out, "relationship: %s\n", result.Relationship)
	return nil
}

func (r Runner) runBrainstormArchive(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("brainstorm archive")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseBrainstormArchiveArgs(args)
	if err != nil {
		return err
	}
	result, err := state.ArchiveBrainstorms(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.archive)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeBrainstormArchive(out, result)
	return nil
}

func writeBrainstormList(out io.Writer, brainstorms state.BrainstormList, filters state.BrainstormListOptions) {
	if len(brainstorms.Brainstorms) == 0 {
		fmt.Fprint(out, "\n  No brainstorms found.\n\n")
		return
	}

	fmt.Fprint(out, "\n  loaf brainstorm list\n\n")
	for _, alias := range sortedBrainstorms(brainstorms) {
		brainstorm := brainstorms.Brainstorms[alias]
		fmt.Fprintf(out, "    %-32s%s", alias, brainstorm.Title)
		if filters.All || filters.Status != "" {
			fmt.Fprintf(out, "  [%s]", brainstorm.Status)
		}
		fmt.Fprintln(out)
		if brainstorm.SourcePath != "" {
			fmt.Fprintf(out, "      %s\n", brainstorm.SourcePath)
		}
	}
	fmt.Fprintf(out, "\n  %d brainstorm(s)\n\n", len(brainstorms.Brainstorms))
}

func writeBrainstormShow(out io.Writer, result state.BrainstormShow) {
	brainstorm := result.Brainstorm
	fmt.Fprintf(out, "brainstorm %s\n", firstNonEmpty(brainstorm.Alias, brainstorm.ID))
	fmt.Fprintf(out, "title: %s\n", brainstorm.Title)
	fmt.Fprintf(out, "status: %s\n", brainstorm.Status)
	for _, source := range brainstorm.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(brainstorm.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range brainstorm.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintf(out, "created: %s\n", brainstorm.CreatedAt)
	fmt.Fprintf(out, "updated: %s\n", brainstorm.UpdatedAt)
	if brainstorm.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, brainstorm.Body)
	}
}

func writeBrainstormArchive(out io.Writer, result state.BrainstormArchiveResult) {
	fmt.Fprint(out, "\n  loaf brainstorm archive\n\n")
	for _, item := range result.Archived {
		brainstorm := item.Ref
		title := ""
		if item.Brainstorm != nil {
			brainstorm = firstNonEmpty(item.Brainstorm.Alias, item.Ref, item.Brainstorm.ID)
			title = item.Brainstorm.Title
		}
		if title == "" {
			fmt.Fprintf(out, "  archived %s\n", brainstorm)
		} else {
			fmt.Fprintf(out, "  archived %s: %s\n", brainstorm, title)
		}
	}
	for _, item := range result.Skipped {
		brainstorm := item.Ref
		if item.Brainstorm != nil {
			brainstorm = firstNonEmpty(item.Brainstorm.Alias, item.Ref, item.Brainstorm.ID)
		}
		fmt.Fprintf(out, "  skipped %s: %s\n", brainstorm, item.Reason)
	}
	fmt.Fprintln(out)
	if len(result.Archived) > 0 {
		fmt.Fprintf(out, "  Archived %d brainstorm(s)\n", len(result.Archived))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(out, "  Skipped %d brainstorm(s)\n", len(result.Skipped))
	}
	fmt.Fprintln(out)
}

func (r Runner) runState(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		fmt.Fprintln(out, runtime.Name())
		return nil
	}

	switch args[0] {
	case "path":
		projectRoot, err := project.ResolveRoot(runtime.RootPath())
		if err != nil {
			return err
		}
		path, err := state.PathResolver{StateHome: r.StateHome}.DatabasePath(projectRoot)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, path)
		return nil
	case "init":
		return r.runStateInit(args[1:], out, runtime)
	case "status":
		return r.runStateStatus(args[1:], out, runtime)
	case "doctor":
		return r.runStateDoctor(args[1:], out, runtime)
	case "migrate":
		return r.runStateMigrate(args[1:], out, runtime)
	case "backup":
		return r.runStateBackup(args[1:], out, runtime)
	case "export":
		return r.runStateExport(args[1:], out, runtime)
	default:
		return fmt.Errorf("state subcommand %q is not implemented yet", args[0])
	}
}

func (r Runner) runStateInit(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	status, err := r.initializeState(runtime)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, status)
	}
	fmt.Fprintln(out, "loaf state init")
	fmt.Fprintf(out, "project root: %s\n", status.ProjectRoot)
	fmt.Fprintf(out, "database: %s\n", status.DatabasePath)
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	fmt.Fprintf(out, "schema version: %d\n", status.SchemaVersion)
	return nil
}

func (r Runner) runStateStatus(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	status, err := r.inspectState(runtime)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, status)
	}
	fmt.Fprintln(out, "loaf state status")
	fmt.Fprintf(out, "project root: %s\n", status.ProjectRoot)
	fmt.Fprintf(out, "database: %s\n", status.DatabasePath)
	fmt.Fprintf(out, "database exists: %t\n", status.DatabaseExists)
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	fmt.Fprintf(out, "schema version: %d\n", status.SchemaVersion)
	return nil
}

func (r Runner) runStateDoctor(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, fix, err := parseDoctorArgs(args)
	if err != nil {
		return err
	}
	status, err := r.inspectState(runtime)
	if err != nil {
		return err
	}
	if fix && status.Mode == state.ModeMarkdownOnly {
		status, err = r.initializeState(runtime)
		if err != nil {
			return err
		}
		status.Diagnostics = append([]state.Diagnostic{{
			Severity: "info",
			Code:     "database-initialized",
			Message:  "SQLite state database initialized",
		}}, status.Diagnostics...)
	}
	if jsonOutput {
		return writeJSON(out, status)
	}

	fmt.Fprintln(out, "loaf state doctor")
	fmt.Fprintf(out, "project root: %s\n", status.ProjectRoot)
	fmt.Fprintf(out, "database: %s\n", status.DatabasePath)
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	for _, diagnostic := range status.Diagnostics {
		fmt.Fprintf(out, "%s: %s\n", diagnostic.Severity, diagnostic.Message)
	}
	if status.Mode == state.ModeInvalid {
		return fmt.Errorf("state doctor found errors")
	}
	return nil
}

func (r Runner) runStateBackup(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := state.Backup(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintln(out, "loaf state backup")
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	fmt.Fprintf(out, "bytes: %d\n", result.Bytes)
	fmt.Fprintf(out, "created at: %s\n", result.CreatedAt)
	return nil
}

func (r Runner) runStateExport(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseStateExportArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	switch {
	case options.kind == state.ExportKindAll && options.format == state.ExportFormatJSON:
		result, err := state.ExportAllJSON(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	case options.kind == state.ExportKindSpec && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportSpecMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref)
		if err != nil {
			return err
		}
		fmt.Fprint(out, result.Content)
		return nil
	case options.kind == state.ExportKindReleaseReadiness && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportReleaseReadinessMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return err
		}
		fmt.Fprint(out, result.Content)
		return nil
	case options.kind == state.ExportKindSession && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportSessionMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref)
		if err != nil {
			return err
		}
		fmt.Fprint(out, result.Content)
		return nil
	case options.kind == state.ExportKindTriage && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportTriageMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return err
		}
		fmt.Fprint(out, result.Content)
		return nil
	default:
		return fmt.Errorf("state export %s --format %s is not implemented yet", options.kind, options.format)
	}
}

func (r Runner) runStateMigrate(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return fmt.Errorf("state migrate requires a source")
	}
	switch args[0] {
	case "markdown":
		return r.runStateMigrateMarkdown(args[1:], out, runtime)
	default:
		return fmt.Errorf("state migrate source %q is not implemented yet", args[0])
	}
}

func (r Runner) runStateMigrateMarkdown(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseMarkdownMigrationArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	if options.apply || options.resume {
		result, err := state.ApplyMarkdownMigration(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		if options.resume {
			fmt.Fprintln(out, "loaf state migrate markdown --resume")
		} else {
			fmt.Fprintln(out, "loaf state migrate markdown --apply")
		}
		fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
		writeMarkdownMigrationPlan(out, result.MarkdownMigrationPlan)
		return nil
	}

	plan, err := state.PreviewMarkdownMigration(projectRoot)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, plan)
	}

	fmt.Fprintln(out, "loaf state migrate markdown --dry-run")
	writeMarkdownMigrationPlan(out, plan)
	return nil
}

func writeMarkdownMigrationPlan(out io.Writer, plan state.MarkdownMigrationPlan) {
	fmt.Fprintf(out, "agents path: %s\n", plan.AgentsPath)
	fmt.Fprintf(out, "specs: %d\n", plan.Specs)
	fmt.Fprintf(out, "tasks: %d\n", plan.Tasks)
	fmt.Fprintf(out, "ideas: %d\n", plan.Ideas)
	fmt.Fprintf(out, "sparks: %d\n", plan.Sparks)
	fmt.Fprintf(out, "brainstorms: %d\n", plan.Brainstorms)
	fmt.Fprintf(out, "shaping drafts: %d\n", plan.ShapingDrafts)
	fmt.Fprintf(out, "sessions: %d\n", plan.Sessions)
	fmt.Fprintf(out, "reports: %d\n", plan.Reports)
	fmt.Fprintf(out, "relationships: %d\n", plan.Relationships)
	fmt.Fprintf(out, "skipped files: %d\n", len(plan.SkippedFiles))
	for _, path := range plan.SkippedFiles {
		fmt.Fprintf(out, "  - %s\n", path)
	}
	for _, warning := range plan.Warnings {
		fmt.Fprintf(out, "warning: %s\n", warning)
	}
}

func (r Runner) inspectState(runtime state.Runtime) (state.Status, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return state.Status{}, err
	}
	return state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
}

func (r Runner) initializeState(runtime state.Runtime) (state.Status, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return state.Status{}, err
	}
	return state.Initialize(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
}

func (r Runner) runTrace(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseTraceArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := state.Trace(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}

	fmt.Fprintf(out, "%s %s\n", result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID))
	if result.Entity.Title != "" {
		fmt.Fprintf(out, "title: %s\n", result.Entity.Title)
	}
	if result.Entity.Status != "" {
		fmt.Fprintf(out, "status: %s\n", result.Entity.Status)
	}
	for _, source := range result.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(result.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
		return nil
	}
	fmt.Fprintln(out, "relationships:")
	for _, relationship := range result.Relationships {
		target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
		fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
		if relationship.Reason != "" {
			fmt.Fprintf(out, " (%s)", relationship.Reason)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func (r Runner) runTask(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return r.runLegacy(append([]string{"task"}, args...), runtime.RootPath(), out)
	}
	switch args[0] {
	case "create":
		return r.runTaskCreate(args[1:], out, runtime)
	case "list":
		return r.runTaskList(args[1:], out, runtime)
	case "show":
		return r.runTaskShow(args[1:], out, runtime)
	case "update":
		return r.runTaskUpdate(args[1:], out, runtime)
	case "archive":
		return r.runTaskArchive(args[1:], out, runtime)
	case "refresh":
		return r.runTaskRefresh(args[1:], out, runtime)
	case "sync":
		return r.runTaskSync(args[1:], out, runtime)
	default:
		return r.runLegacy(append([]string{"task"}, args...), runtime.RootPath(), out)
	}
}

func (r Runner) runTaskRefresh(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "refresh"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	options, err := parseCompatibilityCommandArgs("task refresh", args, nil)
	if err != nil {
		return err
	}
	tasks, err := state.ListTasks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.TaskListOptions{})
	if err != nil {
		return err
	}
	specs, err := state.ListSpecs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	summary := compatibilityCommandSummary{
		Version: 1,
		Command: "task refresh",
		Mode:    "sqlite",
		Action:  "read",
		Reason:  "SQLite state is canonical; Markdown TASKS.json refresh is not run.",
		Counts: map[string]int{
			"tasks": len(tasks.Tasks),
			"specs": len(specs.Specs),
		},
	}
	if options.jsonOutput {
		return writeJSON(out, summary)
	}
	writeCompatibilityCommandSummary(out, summary)
	return nil
}

func (r Runner) runTaskSync(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "sync"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	options, err := parseCompatibilityCommandArgs("task sync", args, map[string]bool{"--import": true, "--push": true})
	if err != nil {
		return err
	}
	tasks, err := state.ListTasks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.TaskListOptions{})
	if err != nil {
		return err
	}
	specs, err := state.ListSpecs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	summary := compatibilityCommandSummary{
		Version: 1,
		Command: "task sync",
		Mode:    "sqlite",
		Action:  "skipped",
		Reason:  "SQLite state is canonical; Markdown task sync is a compatibility repair path and is not run in SQLite mode.",
		Counts: map[string]int{
			"tasks": len(tasks.Tasks),
			"specs": len(specs.Specs),
		},
	}
	if options.jsonOutput {
		return writeJSON(out, summary)
	}
	writeCompatibilityCommandSummary(out, summary)
	return nil
}

func (r Runner) runTaskCreate(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "create"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseTaskCreateArgs(args)
	if err != nil {
		return err
	}
	result, err := state.CreateTask(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeTaskCreate(out, result)
	return nil
}

func (r Runner) runTaskShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("task show", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "show"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.ShowTask(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeTaskShow(out, result)
	return nil
}

func (r Runner) runTaskList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseTaskListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "list"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	tasks, err := state.ListTasks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, tasks)
	}
	writeTaskList(out, tasks, options.filters)
	return nil
}

func (r Runner) runTaskUpdate(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "update"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseTaskUpdateArgs(args)
	if err != nil {
		return err
	}
	result, err := state.UpdateTask(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.update)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeTaskUpdate(out, result)
	return nil
}

func (r Runner) runTaskArchive(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"task", "archive"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseTaskArchiveArgs(args)
	if err != nil {
		return err
	}
	result, err := state.ArchiveTasks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.archive)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeTaskArchive(out, result)
	return nil
}

func writeTaskCreate(out io.Writer, result state.TaskCreateResult) {
	fmt.Fprintf(out, "created task %s: %s\n", firstNonEmpty(result.Task.Alias, result.Task.ID), result.Task.Title)
	fmt.Fprintf(out, "status: %s\n", result.Task.Status)
	if result.Priority != "" {
		fmt.Fprintf(out, "priority: %s\n", result.Priority)
	}
	if result.Spec.Alias != "" {
		fmt.Fprintf(out, "spec: %s\n", result.Spec.Alias)
	}
	if len(result.Depends) > 0 {
		var dependencies []string
		for _, dependency := range result.Depends {
			dependencies = append(dependencies, firstNonEmpty(dependency.Alias, dependency.ID))
		}
		fmt.Fprintf(out, "depends on: %s\n", strings.Join(dependencies, ", "))
	}
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeTaskUpdate(out io.Writer, result state.TaskStatusUpdateResult) {
	fmt.Fprintf(out, "updated task %s\n", firstNonEmpty(result.Task.Alias, result.Task.ID))
	if result.Previous != result.Status {
		fmt.Fprintf(out, "status: %s -> %s\n", result.Previous, result.Status)
	} else if result.Status != "" {
		fmt.Fprintf(out, "status: %s\n", result.Status)
	}
	if result.Priority != "" {
		fmt.Fprintf(out, "priority: %s\n", result.Priority)
	}
	if result.Spec != nil {
		fmt.Fprintf(out, "spec: %s\n", firstNonEmpty(result.Spec.Alias, result.Spec.ID))
	}
	if len(result.Depends) > 0 {
		var dependencies []string
		for _, dependency := range result.Depends {
			dependencies = append(dependencies, firstNonEmpty(dependency.Alias, dependency.ID))
		}
		fmt.Fprintf(out, "depends on: %s\n", strings.Join(dependencies, ", "))
	}
	if result.Session != nil {
		fmt.Fprintf(out, "session: %s\n", firstNonEmpty(result.Session.Alias, result.Session.ID))
	}
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeTaskArchive(out io.Writer, result state.TaskArchiveResult) {
	fmt.Fprint(out, "\n  loaf task archive\n\n")
	if result.Spec != nil && len(result.Archived) == 0 && len(result.Skipped) == 0 {
		fmt.Fprintf(out, "  No completed tasks found for %s\n\n", firstNonEmpty(result.Spec.Alias, result.Spec.ID))
		return
	}
	for _, item := range result.Archived {
		task := item.Ref
		title := ""
		if item.Task != nil {
			task = firstNonEmpty(item.Task.Alias, item.Ref, item.Task.ID)
			title = item.Task.Title
		}
		if title == "" {
			fmt.Fprintf(out, "  archived %s\n", task)
		} else {
			fmt.Fprintf(out, "  archived %s: %s\n", task, title)
		}
	}
	for _, item := range result.Skipped {
		task := item.Ref
		if item.Task != nil {
			task = firstNonEmpty(item.Task.Alias, item.Ref, item.Task.ID)
		}
		fmt.Fprintf(out, "  skipped %s: %s\n", task, item.Reason)
	}
	fmt.Fprintln(out)
	if len(result.Archived) > 0 {
		fmt.Fprintf(out, "  Archived %d task(s)\n", len(result.Archived))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(out, "  Skipped %d task(s)\n", len(result.Skipped))
	}
	fmt.Fprintln(out)
}

func writeTaskShow(out io.Writer, result state.TaskShow) {
	task := result.Task
	fmt.Fprintf(out, "task %s\n", firstNonEmpty(task.Alias, task.ID))
	fmt.Fprintf(out, "title: %s\n", task.Title)
	fmt.Fprintf(out, "status: %s\n", task.Status)
	if task.Priority != "" {
		fmt.Fprintf(out, "priority: %s\n", task.Priority)
	}
	if task.Spec != "" {
		fmt.Fprintf(out, "spec: %s\n", task.Spec)
	}
	if len(task.DependsOn) == 0 {
		fmt.Fprintln(out, "depends on: none")
	} else {
		fmt.Fprintf(out, "depends on: %s\n", strings.Join(task.DependsOn, ", "))
	}
	if len(task.Sessions) > 0 {
		fmt.Fprintf(out, "sessions: %s\n", strings.Join(task.Sessions, ", "))
	}
	for _, source := range task.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	fmt.Fprintf(out, "created: %s\n", task.CreatedAt)
	fmt.Fprintf(out, "updated: %s\n", task.UpdatedAt)
	if task.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, task.Body)
	}
}

func (r Runner) taskStateMode(runtime state.Runtime) (project.Root, string, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, "", err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, "", err
	}
	return projectRoot, status.Mode, nil
}

func writeTaskList(out io.Writer, tasks state.TaskList, filters state.TaskListOptions) {
	if len(tasks.Tasks) == 0 {
		fmt.Fprint(out, "\n  No tasks found.\n\n")
		return
	}

	fmt.Fprint(out, "\n  loaf task list\n\n")
	specs := map[string]bool{}
	for _, status := range taskStatusDisplayOrder(filters) {
		group := sortedTasksByStatus(tasks, status)
		fmt.Fprintf(out, "  %s (%d)\n", taskStatusLabel(status), len(group))
		for _, entry := range group {
			task := tasks.Tasks[entry]
			if task.Spec != "" {
				specs[task.Spec] = true
			}
			fmt.Fprintf(out, "    %-10s%-4s%s", entry, task.Priority, task.Title)
			if task.Spec != "" {
				fmt.Fprintf(out, "  %s", task.Spec)
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out)
	}

	total := len(tasks.Tasks)
	if filters.Active {
		fmt.Fprintf(out, "  Total: %d active tasks across %d specs\n\n", total, len(specs))
		return
	}
	fmt.Fprintf(out, "  Total: %d tasks across %d specs\n\n", total, len(specs))
}

func (r Runner) runIdea(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return missingSubcommandError("idea")
	}
	switch args[0] {
	case "list":
		return r.runIdeaList(args[1:], out, runtime)
	case "show":
		return r.runIdeaShow(args[1:], out, runtime)
	case "capture":
		return r.runIdeaCapture(args[1:], out, runtime)
	case "promote":
		return r.runIdeaPromote(args[1:], out, runtime)
	case "resolve":
		return r.runIdeaResolve(args[1:], out, runtime)
	case "archive":
		return r.runIdeaArchive(args[1:], out, runtime)
	default:
		return unknownSubcommandError("idea", args[0])
	}
}

func (r Runner) runIdeaList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIdeaListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("idea list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	ideas, err := state.ListIdeas(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, ideas)
	}
	writeIdeaList(out, ideas, options.filters)
	return nil
}

func (r Runner) runIdeaShow(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("idea show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	ref, jsonOutput, err := parseSingleRefArgs("idea show", args)
	if err != nil {
		return err
	}
	result, err := state.ShowIdea(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeIdeaShow(out, result)
	return nil
}

func (r Runner) runIdeaCapture(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("idea capture")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseIdeaCaptureArgs(args)
	if err != nil {
		return err
	}
	result, err := state.CaptureIdea(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.capture)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "captured idea %s\n", firstNonEmpty(result.Idea.Alias, result.Idea.ID))
	fmt.Fprintf(out, "title: %s\n", result.Idea.Title)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
	return nil
}

func (r Runner) runIdeaPromote(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("idea promote")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseIdeaPromoteArgs(args)
	if err != nil {
		return err
	}
	result, err := state.PromoteIdea(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.promote)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "promoted idea %s to spec %s\n", firstNonEmpty(result.Idea.Alias, result.Idea.ID), firstNonEmpty(result.Spec.Alias, result.Spec.ID))
	fmt.Fprintf(out, "relationship: %s\n", result.Relationship)
	return nil
}

func (r Runner) runIdeaResolve(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIdeaResolveArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("idea resolve")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.ResolveIdea(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.idea, options.by)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "resolved idea %s by %s\n", firstNonEmpty(result.Idea.Alias, result.Idea.ID), firstNonEmpty(result.ResolvedBy.Alias, result.ResolvedBy.ID))
	return nil
}

func (r Runner) runIdeaArchive(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("idea archive")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseIdeaArchiveArgs(args)
	if err != nil {
		return err
	}
	result, err := state.ArchiveIdeas(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.archive)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIdeaArchive(out, result)
	return nil
}

func writeIdeaList(out io.Writer, ideas state.IdeaList, filters state.IdeaListOptions) {
	if len(ideas.Ideas) == 0 {
		fmt.Fprint(out, "\n  No ideas found.\n\n")
		return
	}

	fmt.Fprint(out, "\n  loaf idea list\n\n")
	for _, alias := range sortedIdeas(ideas) {
		idea := ideas.Ideas[alias]
		fmt.Fprintf(out, "    %-24s%s", alias, idea.Title)
		if filters.All || filters.Status != "" {
			fmt.Fprintf(out, "  [%s]", idea.Status)
		}
		fmt.Fprintln(out)
		if idea.SourcePath != "" {
			fmt.Fprintf(out, "      %s\n", idea.SourcePath)
		}
	}
	fmt.Fprintf(out, "\n  %d idea(s)\n\n", len(ideas.Ideas))
}

func writeIdeaShow(out io.Writer, result state.IdeaShow) {
	idea := result.Idea
	fmt.Fprintf(out, "idea %s\n", firstNonEmpty(idea.Alias, idea.ID))
	fmt.Fprintf(out, "title: %s\n", idea.Title)
	fmt.Fprintf(out, "status: %s\n", idea.Status)
	for _, source := range idea.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(idea.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range idea.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintf(out, "created: %s\n", idea.CreatedAt)
	fmt.Fprintf(out, "updated: %s\n", idea.UpdatedAt)
	if idea.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, idea.Body)
	}
}

func writeIdeaArchive(out io.Writer, result state.IdeaArchiveResult) {
	fmt.Fprint(out, "\n  loaf idea archive\n\n")
	for _, item := range result.Archived {
		idea := item.Ref
		title := ""
		if item.Idea != nil {
			idea = firstNonEmpty(item.Idea.Alias, item.Ref, item.Idea.ID)
			title = item.Idea.Title
		}
		if title == "" {
			fmt.Fprintf(out, "  archived %s\n", idea)
		} else {
			fmt.Fprintf(out, "  archived %s: %s\n", idea, title)
		}
	}
	for _, item := range result.Skipped {
		idea := item.Ref
		if item.Idea != nil {
			idea = firstNonEmpty(item.Idea.Alias, item.Ref, item.Idea.ID)
		}
		fmt.Fprintf(out, "  skipped %s: %s\n", idea, item.Reason)
	}
	fmt.Fprintln(out)
	if len(result.Archived) > 0 {
		fmt.Fprintf(out, "  Archived %d idea(s)\n", len(result.Archived))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(out, "  Skipped %d idea(s)\n", len(result.Skipped))
	}
	fmt.Fprintln(out)
}

func (r Runner) runSpark(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return missingSubcommandError("spark")
	}
	switch args[0] {
	case "list":
		return r.runSparkList(args[1:], out, runtime)
	case "show":
		return r.runSparkShow(args[1:], out, runtime)
	case "capture":
		return r.runSparkCapture(args[1:], out, runtime)
	case "resolve":
		return r.runSparkResolve(args[1:], out, runtime)
	case "promote":
		return r.runSparkPromote(args[1:], out, runtime)
	default:
		return unknownSubcommandError("spark", args[0])
	}
}

func (r Runner) runSparkList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseSparkListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("spark list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	sparks, err := state.ListSparks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, sparks)
	}
	writeSparkList(out, sparks, options.filters)
	return nil
}

func (r Runner) runSparkShow(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("spark show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	ref, jsonOutput, err := parseSingleRefArgs("spark show", args)
	if err != nil {
		return err
	}
	result, err := state.ShowSpark(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeSparkShow(out, result)
	return nil
}

func (r Runner) runSparkCapture(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("spark capture")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseSparkCaptureArgs(args)
	if err != nil {
		return err
	}
	result, err := state.CaptureSpark(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.capture)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "captured spark %s\n", firstNonEmpty(result.Spark.Alias, result.Spark.ID))
	if result.Scope != "" {
		fmt.Fprintf(out, "scope: %s\n", result.Scope)
	}
	fmt.Fprintf(out, "text: %s\n", result.Spark.Title)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
	return nil
}

func (r Runner) runSparkResolve(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseSparkResolveArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("spark resolve")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.ResolveSparkWithOptions(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.resolve)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "resolved spark %s by %s\n", firstNonEmpty(result.Spark.Alias, result.Spark.ID), firstNonEmpty(result.ResolvedBy.Alias, result.ResolvedBy.ID))
	return nil
}

func (r Runner) runSparkPromote(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("spark promote")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseSparkPromoteArgs(args)
	if err != nil {
		return err
	}
	result, err := state.PromoteSpark(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.promote)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "promoted spark %s to idea %s\n", firstNonEmpty(result.Spark.Alias, result.Spark.ID), firstNonEmpty(result.Idea.Alias, result.Idea.ID))
	fmt.Fprintf(out, "relationship: %s\n", result.Relationship)
	return nil
}

func writeSparkList(out io.Writer, sparks state.SparkList, filters state.SparkListOptions) {
	if len(sparks.Sparks) == 0 {
		fmt.Fprint(out, "\n  No sparks found.\n\n")
		return
	}

	fmt.Fprint(out, "\n  loaf spark list\n\n")
	for _, alias := range sortedSparks(sparks) {
		spark := sparks.Sparks[alias]
		fmt.Fprintf(out, "    %-24s%s", alias, spark.Text)
		if spark.Scope != "" {
			fmt.Fprintf(out, "  (%s)", spark.Scope)
		}
		if filters.All || filters.Status != "" {
			fmt.Fprintf(out, "  [%s]", spark.Status)
		}
		fmt.Fprintln(out)
		if spark.SourcePath != "" {
			fmt.Fprintf(out, "      %s\n", spark.SourcePath)
		}
	}
	fmt.Fprintf(out, "\n  %d spark(s)\n\n", len(sparks.Sparks))
}

func writeSparkShow(out io.Writer, result state.SparkShow) {
	spark := result.Spark
	fmt.Fprintf(out, "spark %s\n", firstNonEmpty(spark.Alias, spark.ID))
	if spark.Scope != "" {
		fmt.Fprintf(out, "scope: %s\n", spark.Scope)
	}
	fmt.Fprintf(out, "status: %s\n", spark.Status)
	fmt.Fprintf(out, "text: %s\n", spark.Text)
	for _, source := range spark.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(spark.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range spark.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
}

func (r Runner) runTag(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return missingSubcommandError("tag")
	}
	switch args[0] {
	case "list":
		return r.runTagList(args[1:], out, runtime)
	case "show":
		return r.runTagShow(args[1:], out, runtime)
	case "add":
		return r.runTagAdd(args[1:], out, runtime)
	case "remove":
		return r.runTagRemove(args[1:], out, runtime)
	default:
		return unknownSubcommandError("tag", args[0])
	}
}

func (r Runner) runTagList(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.tagStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("tag list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	tags, err := state.ListTags(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, tags)
	}
	writeTagList(out, tags)
	return nil
}

func (r Runner) runTagShow(args []string, out io.Writer, runtime state.Runtime) error {
	name, jsonOutput, err := parseSingleRefArgs("tag show", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.tagStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("tag show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ShowTag(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, name)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeTagShow(out, result)
	return nil
}

func (r Runner) runTagAdd(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseTagMutationArgs("tag add", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.tagStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("tag add")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.AddTag(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref, options.name)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "tagged %s %s with %s\n", result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID), result.Name)
	return nil
}

func (r Runner) runTagRemove(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseTagMutationArgs("tag remove", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.tagStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("tag remove")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RemoveTag(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref, options.name)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "removed tag %s from %s %s\n", result.Name, result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID))
	return nil
}

func (r Runner) tagStateMode(runtime state.Runtime) (project.Root, string, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, "", err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, "", err
	}
	return projectRoot, status.Mode, nil
}

func writeTagList(out io.Writer, tags state.TagList) {
	if len(tags.Tags) == 0 {
		fmt.Fprint(out, "\n  No tags found.\n\n")
		return
	}
	fmt.Fprint(out, "\n  loaf tag list\n\n")
	for _, name := range sortedTags(tags) {
		fmt.Fprintf(out, "    %-24s%d\n", name, tags.Tags[name].Count)
	}
	fmt.Fprintf(out, "\n  %d tag(s)\n\n", len(tags.Tags))
}

func writeTagShow(out io.Writer, result state.TagShowResult) {
	fmt.Fprintf(out, "\n  tag %s\n\n", result.Name)
	if len(result.Members) == 0 {
		fmt.Fprint(out, "  No tagged rows found.\n\n")
		return
	}
	for _, member := range result.Members {
		label := firstNonEmpty(member.Alias, member.ID)
		fmt.Fprintf(out, "    %-14s%s", member.Kind, label)
		if member.Title != "" {
			fmt.Fprintf(out, "  %s", member.Title)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "\n  %d tagged row(s)\n\n", len(result.Members))
}

func (r Runner) runBundle(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return missingSubcommandError("bundle")
	}
	switch args[0] {
	case "list":
		return r.runBundleList(args[1:], out, runtime)
	case "create":
		return r.runBundleCreate(args[1:], out, runtime)
	case "update":
		return r.runBundleUpdate(args[1:], out, runtime)
	case "show":
		return r.runBundleShow(args[1:], out, runtime)
	case "add":
		return r.runBundleAdd(args[1:], out, runtime)
	case "remove":
		return r.runBundleRemove(args[1:], out, runtime)
	default:
		return unknownSubcommandError("bundle", args[0])
	}
}

func (r Runner) runBundleList(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.bundleStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("bundle list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ListBundles(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeBundleList(out, result)
	return nil
}

func (r Runner) runBundleCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseBundleCreateArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.bundleStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("bundle create")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.CreateBundle(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.BundleCreateOptions{
		Slug:  options.slug,
		Title: options.title,
		Tags:  options.tags,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "created bundle %s\n", result.Slug)
	return nil
}

func (r Runner) runBundleUpdate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseBundleUpdateArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.bundleStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("bundle update")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.UpdateBundle(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.BundleUpdateOptions{
		Slug:     options.slug,
		Title:    options.title,
		SetTitle: options.setTitle,
		Tags:     options.tags,
		SetTags:  options.setTags,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "updated bundle %s\n", result.Slug)
	if result.Title != "" {
		fmt.Fprintf(out, "title: %s\n", result.Title)
	}
	if len(result.Tags) > 0 {
		fmt.Fprintf(out, "tags: %s\n", strings.Join(result.Tags, ", "))
	} else if options.setTags {
		fmt.Fprintln(out, "tags: none")
	}
	return nil
}

func (r Runner) runBundleShow(args []string, out io.Writer, runtime state.Runtime) error {
	slug, jsonOutput, err := parseSingleRefArgs("bundle show", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.bundleStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("bundle show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ShowBundle(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, slug)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeBundleShow(out, result)
	return nil
}

func (r Runner) runBundleAdd(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseBundleMemberArgs("bundle add", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.bundleStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("bundle add")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.AddBundleMember(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.slug, options.ref)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "added %s %s to bundle %s\n", result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID), result.Slug)
	return nil
}

func (r Runner) runBundleRemove(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseBundleMemberArgs("bundle remove", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.bundleStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("bundle remove")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RemoveBundleMember(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.slug, options.ref)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "removed %s %s from bundle %s\n", result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID), result.Slug)
	return nil
}

func (r Runner) bundleStateMode(runtime state.Runtime) (project.Root, string, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, "", err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, "", err
	}
	return projectRoot, status.Mode, nil
}

func writeBundleList(out io.Writer, result state.BundleList) {
	if len(result.Bundles) == 0 {
		fmt.Fprint(out, "\n  No bundles found.\n\n")
		return
	}
	fmt.Fprint(out, "\n  loaf bundle list\n\n")
	for _, slug := range sortedBundleSlugs(result) {
		bundle := result.Bundles[slug]
		fmt.Fprintf(out, "    %-24s%s", slug, bundle.Title)
		if len(bundle.TagQuery) > 0 {
			fmt.Fprintf(out, "  [%s]", strings.Join(bundle.TagQuery, ", "))
		}
		fmt.Fprintf(out, "  %d member(s)\n", bundle.MemberCount)
	}
	fmt.Fprintf(out, "\n  %d bundle(s)\n\n", len(result.Bundles))
}

func writeBundleShow(out io.Writer, result state.BundleShowResult) {
	fmt.Fprintf(out, "\n  bundle %s\n", result.Slug)
	if result.Title != "" && result.Title != result.Slug {
		fmt.Fprintf(out, "  %s\n", result.Title)
	}
	if len(result.TagQuery) > 0 {
		fmt.Fprintf(out, "  tags: %s\n", strings.Join(result.TagQuery, ", "))
	}
	fmt.Fprintln(out)
	if len(result.Members) == 0 {
		fmt.Fprint(out, "  No bundled rows found.\n\n")
		return
	}
	for _, member := range result.Members {
		label := firstNonEmpty(member.Alias, member.ID)
		fmt.Fprintf(out, "    %-14s%s", member.Kind, label)
		if member.Title != "" {
			fmt.Fprintf(out, "  %s", member.Title)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "\n  %d bundled row(s)\n\n", len(result.Members))
}

func sortedBundleSlugs(result state.BundleList) []string {
	slugs := make([]string, 0, len(result.Bundles))
	for slug := range result.Bundles {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs
}

func sortedHousekeepingSections(result state.HousekeepingSummary) []string {
	return sortedHousekeepingSectionNames(result.Sections)
}

func sortedHousekeepingSectionNames(sections map[string]state.HousekeepingSection) []string {
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedCountKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func housekeepingSectionLabel(name string) string {
	switch name {
	case "shaping_drafts":
		return "drafts"
	default:
		return name
	}
}

func (r Runner) runLink(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return missingSubcommandError("link")
	}
	switch args[0] {
	case "create":
		return r.runLinkCreate(args[1:], out, runtime)
	case "list":
		return r.runLinkList(args[1:], out, runtime)
	case "remove":
		return r.runLinkRemove(args[1:], out, runtime)
	default:
		return unknownSubcommandError("link", args[0])
	}
}

func (r Runner) runLinkCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseLinkMutationArgs("link create", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.linkStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("link create")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.CreateLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.LinkMutationOptions{
		From:   options.from,
		To:     options.to,
		Type:   options.relationshipType,
		Reason: options.reason,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "linked %s %s %s %s %s\n", result.From.Kind, firstNonEmpty(result.From.Alias, result.From.ID), result.Type, result.To.Kind, firstNonEmpty(result.To.Alias, result.To.ID))
	return nil
}

func (r Runner) runLinkList(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("link list", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.linkStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("link list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ListLinks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeLinkList(out, result)
	return nil
}

func (r Runner) runLinkRemove(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseLinkMutationArgs("link remove", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.linkStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("link remove")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RemoveLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.LinkMutationOptions{
		From:   options.from,
		To:     options.to,
		Type:   options.relationshipType,
		Reason: options.reason,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "removed link %s %s %s %s %s\n", result.From.Kind, firstNonEmpty(result.From.Alias, result.From.ID), result.Type, result.To.Kind, firstNonEmpty(result.To.Alias, result.To.ID))
	return nil
}

func (r Runner) linkStateMode(runtime state.Runtime) (project.Root, string, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, "", err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, "", err
	}
	return projectRoot, status.Mode, nil
}

func writeLinkList(out io.Writer, result state.LinkListResult) {
	fmt.Fprintf(out, "\n  links for %s %s\n\n", result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID))
	if len(result.Relationships) == 0 {
		fmt.Fprint(out, "  No links found.\n\n")
		return
	}
	for _, relationship := range result.Relationships {
		target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
		fmt.Fprintf(out, "    %-8s %-14s %-14s%s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
		if relationship.Reason != "" {
			fmt.Fprintf(out, "  %s", relationship.Reason)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "\n  %d link(s)\n\n", len(result.Relationships))
}

func (r Runner) runSpec(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return r.runLegacy(append([]string{"spec"}, args...), runtime.RootPath(), out)
	}
	switch args[0] {
	case "list":
		return r.runSpecList(args[1:], out, runtime)
	case "show":
		return r.runSpecShow(args[1:], out, runtime)
	case "archive":
		return r.runSpecArchive(args[1:], out, runtime)
	default:
		return r.runLegacy(append([]string{"spec"}, args...), runtime.RootPath(), out)
	}
}

func (r Runner) runSpecList(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"spec", "list"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	specs, err := state.ListSpecs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, specs)
	}
	writeSpecList(out, specs)
	return nil
}

func (r Runner) runSpecShow(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"spec", "show"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	ref, jsonOutput, err := parseSingleRefArgs("spec show", args)
	if err != nil {
		return err
	}
	result, err := state.ShowSpec(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeSpecShow(out, result)
	return nil
}

func (r Runner) runSpecArchive(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"spec", "archive"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	refs, jsonOutput, err := parseArchiveArgs("spec archive", args)
	if err != nil {
		return err
	}
	result, err := state.ArchiveSpecs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, refs)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeSpecArchive(out, result)
	return nil
}

func writeSpecList(out io.Writer, specs state.SpecList) {
	if len(specs.Specs) == 0 {
		fmt.Fprint(out, "\n  No specs found.\n\n")
		return
	}

	fmt.Fprint(out, "\n  loaf spec list\n\n")
	for _, status := range specStatusDisplayOrder(specs) {
		group := sortedSpecsByStatus(specs, status)
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(out, "  %s (%d)\n", specStatusLabel(status), len(group))
		for _, entry := range group {
			spec := specs.Specs[entry]
			fmt.Fprintf(out, "    %-10s%s\n", entry, spec.Title)
			fmt.Fprintf(out, "              Tasks: %d todo / %d in_progress / %d done\n", spec.Tasks.Todo, spec.Tasks.InProgress, spec.Tasks.Done)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "  Total: %d specs\n\n", len(specs.Specs))
}

func writeSpecShow(out io.Writer, result state.SpecShow) {
	spec := result.Spec
	fmt.Fprintf(out, "spec %s\n", firstNonEmpty(spec.Alias, spec.ID))
	fmt.Fprintf(out, "title: %s\n", spec.Title)
	fmt.Fprintf(out, "status: %s\n", spec.Status)
	fmt.Fprintf(out, "tasks: %d todo / %d in_progress / %d done\n", spec.Tasks.Todo, spec.Tasks.InProgress, spec.Tasks.Done)
	for _, source := range spec.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(spec.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range spec.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	if spec.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, spec.Body)
	}
}

func writeSpecArchive(out io.Writer, result state.SpecArchiveResult) {
	fmt.Fprint(out, "\n  loaf spec archive\n\n")
	for _, item := range result.Archived {
		spec := item.Ref
		title := ""
		if item.Spec != nil {
			spec = firstNonEmpty(item.Spec.Alias, item.Ref, item.Spec.ID)
			title = item.Spec.Title
		}
		if title == "" {
			fmt.Fprintf(out, "  archived %s\n", spec)
		} else {
			fmt.Fprintf(out, "  archived %s: %s\n", spec, title)
		}
	}
	for _, item := range result.Skipped {
		spec := item.Ref
		if item.Spec != nil {
			spec = firstNonEmpty(item.Spec.Alias, item.Ref, item.Spec.ID)
		}
		fmt.Fprintf(out, "  skipped %s: %s\n", spec, item.Reason)
	}
	fmt.Fprintln(out)
	if len(result.Archived) > 0 {
		fmt.Fprintf(out, "  Archived %d spec(s)\n", len(result.Archived))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(out, "  Skipped %d spec(s)\n", len(result.Skipped))
	}
	fmt.Fprintln(out)
}

func (r Runner) runSession(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return r.runLegacy(append([]string{"session"}, args...), runtime.RootPath(), out)
	}
	switch args[0] {
	case "list":
		return r.runSessionList(args[1:], out, runtime)
	case "show":
		return r.runSessionShow(args[1:], out, runtime)
	case "log":
		return r.runSessionLog(args[1:], out, runtime)
	case "enrich":
		return r.runSessionEnrich(args[1:], out, runtime)
	case "report":
		return r.runSessionReport(args[1:], out, runtime)
	default:
		return r.runLegacy(append([]string{"session"}, args...), runtime.RootPath(), out)
	}
}

func (r Runner) runSessionEnrich(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"session", "enrich"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	options, err := parseCompatibilityCommandArgs("session enrich", args, map[string]bool{"--dry-run": true})
	if err != nil {
		return err
	}
	sessions, err := state.ListSessions(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SessionListOptions{All: true})
	if err != nil {
		return err
	}
	summary := compatibilityCommandSummary{
		Version: 1,
		Command: "session enrich",
		Mode:    "sqlite",
		Action:  "skipped",
		Reason:  "SQLite journal state is written through `loaf session log`; Markdown JSONL enrichment is a compatibility path and is not run in SQLite mode.",
		Counts: map[string]int{
			"sessions": len(sessions.Sessions),
		},
	}
	if options.jsonOutput {
		return writeJSON(out, summary)
	}
	writeCompatibilityCommandSummary(out, summary)
	return nil
}

func (r Runner) runSessionShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("session show", args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"session", "show"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.ShowSession(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeSessionShow(out, result)
	return nil
}

func (r Runner) runSessionList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseSessionListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"session", "list"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	sessions, err := state.ListSessions(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, sessions)
	}
	writeSessionList(out, sessions, options.filters)
	return nil
}

func (r Runner) runSessionLog(args []string, out io.Writer, runtime state.Runtime) error {
	if shouldDelegateSessionLog(args) {
		return r.runLegacy(append([]string{"session", "log"}, args...), runtime.RootPath(), out)
	}
	options, err := parseSessionLogArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"session", "log"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.LogJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalLogOptions{
		Entry:            options.entry,
		ObservedBranch:   state.ObservedGitBranch(runtime.RootPath()),
		ObservedWorktree: runtime.RootPath(),
		HarnessSessionID: options.harnessSessionID,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "logged journal entry: %s\n", result.ID)
	return nil
}

func (r Runner) runSessionReport(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("session report", args)
	if err != nil {
		return err
	}
	if jsonOutput {
		return fmt.Errorf("session report does not support --json")
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := state.ExportSessionMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	fmt.Fprint(out, result.Content)
	return nil
}

func shouldDelegateSessionLog(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--from-hook", "--detect-linear":
			return true
		}
	}
	return false
}

func writeSessionList(out io.Writer, sessions state.SessionList, filters state.SessionListOptions) {
	active := sortedSessionsByArchivedState(sessions, false)
	archived := sortedSessionsByArchivedState(sessions, true)

	fmt.Fprint(out, "\n  loaf session list\n\n")
	if len(active) == 0 {
		fmt.Fprint(out, "  No active sessions found.\n")
	} else {
		fmt.Fprint(out, "  Active Sessions:\n")
		for _, alias := range active {
			session := sessions.Sessions[alias]
			fmt.Fprintf(out, "    %s", firstNonEmpty(session.Branch, alias))
			if session.HarnessSessionID != "" {
				fmt.Fprintf(out, "  %s", session.HarnessSessionID)
			}
			if session.JournalEntries > 0 {
				fmt.Fprintf(out, "  %d entries", session.JournalEntries)
			}
			fmt.Fprintln(out)
			if session.SourcePath != "" {
				fmt.Fprintf(out, "      %s\n", session.SourcePath)
			}
		}
	}

	if filters.All && len(archived) > 0 {
		fmt.Fprintln(out)
		fmt.Fprint(out, "  Archived Sessions:\n")
		for _, alias := range archived {
			session := sessions.Sessions[alias]
			fmt.Fprintf(out, "    %s", firstNonEmpty(session.Branch, alias))
			if session.HarnessSessionID != "" {
				fmt.Fprintf(out, "  %s", session.HarnessSessionID)
			}
			if session.JournalEntries > 0 {
				fmt.Fprintf(out, "  %d entries", session.JournalEntries)
			}
			fmt.Fprintln(out)
			if session.SourcePath != "" {
				fmt.Fprintf(out, "      %s\n", session.SourcePath)
			}
		}
	}

	fmt.Fprintln(out)
	if filters.All {
		fmt.Fprintf(out, "  %d active, %d archived\n\n", len(active), len(archived))
		return
	}
	fmt.Fprintf(out, "  %d active\n\n", len(active))
}

func writeSessionShow(out io.Writer, result state.SessionShow) {
	session := result.Session
	fmt.Fprintf(out, "session %s\n", firstNonEmpty(session.Alias, session.ID))
	if session.Branch != "" {
		fmt.Fprintf(out, "branch: %s\n", session.Branch)
	}
	fmt.Fprintf(out, "status: %s\n", session.Status)
	if session.HarnessSessionID != "" {
		fmt.Fprintf(out, "harness session: %s\n", session.HarnessSessionID)
	}
	for _, source := range session.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(session.JournalEntries) == 0 {
		fmt.Fprintln(out, "journal entries: none")
	} else {
		fmt.Fprintln(out, "journal entries:")
		for _, entry := range session.JournalEntries {
			if entry.Scope != "" {
				fmt.Fprintf(out, "  - %s(%s): %s\n", entry.EntryType, entry.Scope, entry.Message)
			} else {
				fmt.Fprintf(out, "  - %s: %s\n", entry.EntryType, entry.Message)
			}
		}
	}
	if len(session.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range session.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintf(out, "created: %s\n", session.CreatedAt)
	fmt.Fprintf(out, "updated: %s\n", session.UpdatedAt)
}

func (r Runner) runReport(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return r.runLegacy(append([]string{"report"}, args...), runtime.RootPath(), out)
	}
	switch args[0] {
	case "list":
		return r.runReportList(args[1:], out, runtime)
	case "generate":
		return r.runReportGenerate(args[1:], out, runtime)
	case "create":
		return r.runReportCreate(args[1:], out, runtime)
	case "finalize":
		return r.runReportFinalize(args[1:], out, runtime)
	case "archive":
		return r.runReportArchive(args[1:], out, runtime)
	default:
		return r.runLegacy(append([]string{"report"}, args...), runtime.RootPath(), out)
	}
}

func (r Runner) runReportList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseReportListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"report", "list"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	reports, err := state.ListReports(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, reports)
	}
	writeReportList(out, reports)
	return nil
}

func (r Runner) runReportGenerate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseReportGenerateArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	var result state.MarkdownExport
	switch options.kind {
	case state.ExportKindSession:
		result, err = state.ExportSessionMarkdown(context.Background(), projectRoot, resolver, options.ref)
	case state.ExportKindTriage:
		result, err = state.ExportTriageMarkdown(context.Background(), projectRoot, resolver)
	case state.ExportKindReleaseReadiness:
		result, err = state.ExportReleaseReadinessMarkdown(context.Background(), projectRoot, resolver)
	default:
		return fmt.Errorf("report generate kind %q is not implemented yet", options.kind)
	}
	if err != nil {
		return err
	}
	fmt.Fprint(out, result.Content)
	return nil
}

func (r Runner) runReportCreate(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.reportStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"report", "create"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseReportCreateArgs(args)
	if err != nil {
		return err
	}
	result, err := state.CreateReport(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeReportCreate(out, result)
	return nil
}

func (r Runner) runReportFinalize(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("report finalize", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.reportStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"report", "finalize"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.FinalizeReport(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeReportStatus(out, "finalized", result)
	return nil
}

func (r Runner) runReportArchive(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("report archive", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.reportStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runLegacy(append([]string{"report", "archive"}, args...), runtime.RootPath(), out)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result, err := state.ArchiveReport(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeReportStatus(out, "archived", result)
	return nil
}

func (r Runner) reportStateMode(runtime state.Runtime) (project.Root, string, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, "", err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, "", err
	}
	return projectRoot, status.Mode, nil
}

func writeReportCreate(out io.Writer, result state.ReportCreateResult) {
	fmt.Fprintf(out, "created report %s: %s\n", firstNonEmpty(result.Report.Alias, result.Report.ID), result.Report.Title)
	fmt.Fprintf(out, "status: %s\n", result.Report.Status)
	fmt.Fprintf(out, "type: %s\n", result.Kind)
	fmt.Fprintf(out, "source: %s\n", result.Source)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeReportStatus(out io.Writer, action string, result state.ReportStatusResult) {
	fmt.Fprintf(out, "%s report %s: %s\n", action, firstNonEmpty(result.Report.Alias, result.Report.ID), result.Report.Title)
	fmt.Fprintf(out, "previous: %s\n", result.Previous)
	fmt.Fprintf(out, "status: %s\n", result.Status)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeReportList(out io.Writer, reports state.ReportList) {
	if len(reports.Reports) == 0 {
		fmt.Fprint(out, "\n  No reports found.\n\n")
		return
	}

	fmt.Fprint(out, "\n  loaf report list\n\n")
	for _, status := range reportStatusDisplayOrder(reports) {
		group := sortedReportsByStatus(reports, status)
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(out, "  %s:\n", reportStatusLabel(status))
		for _, alias := range group {
			report := reports.Reports[alias]
			fmt.Fprintf(out, "    %s [%s]\n", report.Title, report.Kind)
			if report.SourcePath != "" {
				fmt.Fprintf(out, "      %s\n", report.SourcePath)
			}
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  %d report(s) total\n\n", len(reports.Reports))
}

func parseJSONOnly(args []string) (bool, error) {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			return false, fmt.Errorf("unknown option %q", arg)
		}
	}
	return jsonOutput, nil
}

func parseCompatibilityCommandArgs(command string, args []string, allowedFlags map[string]bool) (compatibilityCommandOptions, error) {
	var options compatibilityCommandOptions
	positional := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--model", "--session-id":
			if command != "session enrich" {
				return compatibilityCommandOptions{}, fmt.Errorf("%s does not support %s", command, arg)
			}
			if _, err := consumeFlagValue(args, &i, arg); err != nil {
				return compatibilityCommandOptions{}, err
			}
		default:
			if strings.HasPrefix(arg, "--") {
				if allowedFlags != nil && allowedFlags[arg] {
					continue
				}
				return compatibilityCommandOptions{}, fmt.Errorf("unknown %s option %q", command, arg)
			}
			if command != "session enrich" {
				return compatibilityCommandOptions{}, fmt.Errorf("%s accepts no positional arguments", command)
			}
			positional++
			if positional > 1 {
				return compatibilityCommandOptions{}, fmt.Errorf("%s accepts at most one session file", command)
			}
		}
	}
	return options, nil
}

func parseHousekeepingArgs(args []string) (housekeepingOptions, error) {
	var options housekeepingOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--dry-run":
			options.dryRun = true
		case "--sessions":
			options.addHousekeepingSection("sessions")
		case "--specs":
			options.addHousekeepingSection("specs")
		case "--drafts":
			options.addHousekeepingSection("shaping_drafts")
		case "--plans", "--handoffs":
			if options.sections == nil {
				options.sections = map[string]bool{}
			}
		default:
			return housekeepingOptions{}, fmt.Errorf("unknown housekeeping option %q", arg)
		}
	}
	return options, nil
}

func (o *housekeepingOptions) addHousekeepingSection(section string) {
	if o.sections == nil {
		o.sections = map[string]bool{}
	}
	o.sections[section] = true
}

type stateExportOptions struct {
	kind   string
	ref    string
	format string
}

func parseStateExportArgs(args []string) (stateExportOptions, error) {
	if len(args) == 0 {
		return stateExportOptions{}, fmt.Errorf("state export requires a kind")
	}
	options := stateExportOptions{kind: args[0]}
	var positional []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--format":
			value, err := consumeFlagValue(args, &i, "state export --format")
			if err != nil {
				return stateExportOptions{}, err
			}
			options.format = value
		case strings.HasPrefix(arg, "--format="):
			options.format = strings.TrimPrefix(arg, "--format=")
		default:
			if strings.HasPrefix(arg, "-") {
				return stateExportOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	switch options.kind {
	case state.ExportKindAll, state.ExportKindReleaseReadiness, state.ExportKindSpec, state.ExportKindSession, state.ExportKindTriage:
	default:
		return stateExportOptions{}, fmt.Errorf("state export kind %q is not implemented yet", options.kind)
	}
	switch options.kind {
	case state.ExportKindSpec:
		if len(positional) != 1 {
			return stateExportOptions{}, fmt.Errorf("state export spec requires exactly one spec")
		}
		options.ref = positional[0]
	case state.ExportKindSession:
		if len(positional) != 1 {
			return stateExportOptions{}, fmt.Errorf("state export session requires exactly one session")
		}
		options.ref = positional[0]
	default:
		if len(positional) > 0 {
			return stateExportOptions{}, fmt.Errorf("state export %s does not accept positional arguments", options.kind)
		}
	}
	if options.format == "" {
		return stateExportOptions{}, fmt.Errorf("state export %s requires --format", options.kind)
	}
	switch {
	case options.kind == state.ExportKindAll && options.format == state.ExportFormatJSON:
		return options, nil
	case options.kind == state.ExportKindSpec && options.format == state.ExportFormatMarkdown:
		return options, nil
	case options.kind == state.ExportKindReleaseReadiness && options.format == state.ExportFormatMarkdown:
		return options, nil
	case options.kind == state.ExportKindSession && options.format == state.ExportFormatMarkdown:
		return options, nil
	case options.kind == state.ExportKindTriage && options.format == state.ExportFormatMarkdown:
		return options, nil
	default:
		return stateExportOptions{}, fmt.Errorf("state export format %q is not implemented yet", options.format)
	}
}

func parseDoctorArgs(args []string) (bool, bool, error) {
	jsonOutput := false
	fix := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--fix":
			fix = true
		default:
			return false, false, fmt.Errorf("unknown option %q", arg)
		}
	}
	return jsonOutput, fix, nil
}

func parseTraceArgs(args []string) (string, bool, error) {
	ref := ""
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, fmt.Errorf("unknown option %q", arg)
			}
			if ref != "" {
				return "", false, fmt.Errorf("trace accepts exactly one id")
			}
			ref = arg
		}
	}
	if ref == "" {
		return "", false, fmt.Errorf("trace requires an id")
	}
	return ref, jsonOutput, nil
}

type taskListOptions struct {
	jsonOutput bool
	filters    state.TaskListOptions
}

type taskCreateOptions struct {
	jsonOutput bool
	create     state.TaskCreateOptions
}

type taskUpdateOptions struct {
	jsonOutput bool
	update     state.TaskUpdateOptions
}

type taskArchiveOptions struct {
	jsonOutput bool
	archive    state.TaskArchiveOptions
}

type ideaListOptions struct {
	jsonOutput bool
	filters    state.IdeaListOptions
}

type brainstormListOptions struct {
	jsonOutput bool
	filters    state.BrainstormListOptions
}

type brainstormPromoteOptions struct {
	jsonOutput bool
	promote    state.BrainstormPromoteOptions
}

type brainstormArchiveOptions struct {
	jsonOutput bool
	archive    state.BrainstormArchiveOptions
}

type ideaResolveOptions struct {
	jsonOutput bool
	idea       string
	by         string
}

type ideaCaptureOptions struct {
	jsonOutput bool
	capture    state.IdeaCaptureOptions
}

type ideaPromoteOptions struct {
	jsonOutput bool
	promote    state.IdeaPromoteOptions
}

type ideaArchiveOptions struct {
	jsonOutput bool
	archive    state.IdeaArchiveOptions
}

type sparkListOptions struct {
	jsonOutput bool
	filters    state.SparkListOptions
}

type sparkCaptureOptions struct {
	jsonOutput bool
	capture    state.SparkCaptureOptions
}

type sparkResolveOptions struct {
	jsonOutput bool
	resolve    state.SparkResolveOptions
}

type sparkPromoteOptions struct {
	jsonOutput bool
	promote    state.SparkPromoteOptions
}

type tagMutationOptions struct {
	jsonOutput bool
	ref        string
	name       string
}

type bundleCreateOptions struct {
	jsonOutput bool
	slug       string
	title      string
	tags       []string
}

type bundleUpdateOptions struct {
	jsonOutput bool
	slug       string
	title      string
	setTitle   bool
	tags       []string
	setTags    bool
}

type bundleMemberOptions struct {
	jsonOutput bool
	slug       string
	ref        string
}

type linkMutationOptions struct {
	jsonOutput       bool
	from             string
	to               string
	relationshipType string
	reason           string
}

type sessionListOptions struct {
	jsonOutput bool
	filters    state.SessionListOptions
}

type sessionLogOptions struct {
	jsonOutput       bool
	entry            string
	harnessSessionID string
}

type reportListOptions struct {
	jsonOutput bool
	filters    state.ReportListOptions
}

type reportCreateOptions struct {
	jsonOutput bool
	create     state.ReportCreateOptions
}

type reportGenerateOptions struct {
	kind string
	ref  string
}

func parseTaskListArgs(args []string) (taskListOptions, error) {
	var options taskListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--active":
			options.filters.Active = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return taskListOptions{}, err
			}
			if !state.ValidTaskListStatus(value) {
				return taskListOptions{}, fmt.Errorf("invalid status %q", value)
			}
			options.filters.Status = value
		default:
			return taskListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseTaskCreateArgs(args []string) (taskCreateOptions, error) {
	options := taskCreateOptions{create: state.TaskCreateOptions{Priority: "P2"}}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return taskCreateOptions{}, err
			}
			options.create.Title = value
		case "--spec":
			value, err := consumeFlagValue(args, &i, "--spec")
			if err != nil {
				return taskCreateOptions{}, err
			}
			options.create.Spec = value
		case "--priority":
			value, err := consumeFlagValue(args, &i, "--priority")
			if err != nil {
				return taskCreateOptions{}, err
			}
			if !state.ValidTaskPriority(value) {
				return taskCreateOptions{}, fmt.Errorf("invalid priority %q", value)
			}
			options.create.Priority = value
		case "--depends-on":
			value, err := consumeFlagValue(args, &i, "--depends-on")
			if err != nil {
				return taskCreateOptions{}, err
			}
			options.create.DependsOn = splitCommaList(value)
		default:
			return taskCreateOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.create.Title) == "" {
		return taskCreateOptions{}, fmt.Errorf("task create requires --title")
	}
	return options, nil
}

func parseTaskUpdateArgs(args []string) (taskUpdateOptions, error) {
	var options taskUpdateOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return taskUpdateOptions{}, err
			}
			if !state.ValidTaskStatus(value) {
				return taskUpdateOptions{}, fmt.Errorf("invalid status %q", value)
			}
			options.update.Status = value
			options.update.SetStatus = true
		case "--priority":
			value, err := consumeFlagValue(args, &i, "--priority")
			if err != nil {
				return taskUpdateOptions{}, err
			}
			if !state.ValidTaskPriority(value) {
				return taskUpdateOptions{}, fmt.Errorf("invalid priority %q", value)
			}
			options.update.Priority = value
			options.update.SetPriority = true
		case "--spec":
			value, err := consumeFlagValue(args, &i, "--spec")
			if err != nil {
				return taskUpdateOptions{}, err
			}
			options.update.Spec = value
			options.update.SetSpec = true
		case "--depends-on":
			value, err := consumeFlagValue(args, &i, "--depends-on")
			if err != nil {
				return taskUpdateOptions{}, err
			}
			if strings.EqualFold(strings.TrimSpace(value), "none") {
				options.update.DependsOn = nil
			} else {
				options.update.DependsOn = splitCommaList(value)
			}
			options.update.SetDependsOn = true
		case "--session":
			value, err := consumeFlagValue(args, &i, "--session")
			if err != nil {
				return taskUpdateOptions{}, err
			}
			options.update.Session = value
			options.update.SetSession = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return taskUpdateOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return taskUpdateOptions{}, fmt.Errorf("task update requires exactly one task")
	}
	options.update.Ref = positional[0]
	if !options.update.SetStatus && !options.update.SetPriority && !options.update.SetSpec && !options.update.SetDependsOn && !options.update.SetSession {
		return taskUpdateOptions{}, fmt.Errorf("task update requires at least one update")
	}
	return options, nil
}

func parseTaskArchiveArgs(args []string) (taskArchiveOptions, error) {
	var options taskArchiveOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--spec":
			value, err := consumeFlagValue(args, &i, "--spec")
			if err != nil {
				return taskArchiveOptions{}, err
			}
			options.archive.Spec = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return taskArchiveOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			options.archive.Refs = append(options.archive.Refs, args[i])
		}
	}
	if options.archive.Spec != "" && len(options.archive.Refs) > 0 {
		return taskArchiveOptions{}, fmt.Errorf("task archive accepts task ids or --spec, not both")
	}
	if options.archive.Spec == "" && len(options.archive.Refs) == 0 {
		return taskArchiveOptions{}, fmt.Errorf("task archive requires task ids or --spec")
	}
	return options, nil
}

func splitCommaList(value string) []string {
	var values []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func consumeFlagValue(args []string, index *int, flag string) (string, error) {
	next := *index + 1
	if next >= len(args) || strings.HasPrefix(args[next], "--") {
		return "", fmt.Errorf("%s requires a value", flag)
	}
	*index = next
	return args[next], nil
}

func parseIdeaListArgs(args []string) (ideaListOptions, error) {
	var options ideaListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--all":
			options.filters.All = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return ideaListOptions{}, err
			}
			options.filters.Status = value
		default:
			return ideaListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseBrainstormListArgs(args []string) (brainstormListOptions, error) {
	var options brainstormListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--all":
			options.filters.All = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return brainstormListOptions{}, err
			}
			options.filters.Status = value
		default:
			return brainstormListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseBrainstormPromoteArgs(args []string) (brainstormPromoteOptions, error) {
	var options brainstormPromoteOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--to-idea":
			value, err := consumeFlagValue(args, &i, "--to-idea")
			if err != nil {
				return brainstormPromoteOptions{}, err
			}
			options.promote.ToIdea = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return brainstormPromoteOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.promote.Brainstorm != "" {
				return brainstormPromoteOptions{}, fmt.Errorf("brainstorm promote accepts exactly one brainstorm")
			}
			options.promote.Brainstorm = args[i]
		}
	}
	if options.promote.Brainstorm == "" {
		return brainstormPromoteOptions{}, fmt.Errorf("brainstorm promote requires a brainstorm")
	}
	if options.promote.ToIdea == "" {
		return brainstormPromoteOptions{}, fmt.Errorf("brainstorm promote requires --to-idea")
	}
	return options, nil
}

func parseBrainstormArchiveArgs(args []string) (brainstormArchiveOptions, error) {
	var options brainstormArchiveOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--reason":
			value, err := consumeFlagValue(args, &i, "--reason")
			if err != nil {
				return brainstormArchiveOptions{}, err
			}
			options.archive.Reason = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return brainstormArchiveOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			options.archive.Refs = append(options.archive.Refs, args[i])
		}
	}
	if len(options.archive.Refs) == 0 {
		return brainstormArchiveOptions{}, fmt.Errorf("brainstorm archive requires at least one brainstorm")
	}
	return options, nil
}

func parseIdeaCaptureArgs(args []string) (ideaCaptureOptions, error) {
	var options ideaCaptureOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return ideaCaptureOptions{}, err
			}
			options.capture.Title = value
		default:
			return ideaCaptureOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.capture.Title) == "" {
		return ideaCaptureOptions{}, fmt.Errorf("idea capture requires --title")
	}
	return options, nil
}

func parseIdeaResolveArgs(args []string) (ideaResolveOptions, error) {
	var options ideaResolveOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--by":
			value, err := consumeFlagValue(args, &i, "--by")
			if err != nil {
				return ideaResolveOptions{}, err
			}
			options.by = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return ideaResolveOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.idea != "" {
				return ideaResolveOptions{}, fmt.Errorf("idea resolve accepts exactly one idea")
			}
			options.idea = args[i]
		}
	}
	if options.idea == "" {
		return ideaResolveOptions{}, fmt.Errorf("idea resolve requires an idea")
	}
	if options.by == "" {
		return ideaResolveOptions{}, fmt.Errorf("idea resolve requires --by")
	}
	return options, nil
}

func parseIdeaPromoteArgs(args []string) (ideaPromoteOptions, error) {
	var options ideaPromoteOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--to-spec":
			value, err := consumeFlagValue(args, &i, "--to-spec")
			if err != nil {
				return ideaPromoteOptions{}, err
			}
			options.promote.ToSpec = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return ideaPromoteOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.promote.Idea != "" {
				return ideaPromoteOptions{}, fmt.Errorf("idea promote accepts exactly one idea")
			}
			options.promote.Idea = args[i]
		}
	}
	if options.promote.Idea == "" {
		return ideaPromoteOptions{}, fmt.Errorf("idea promote requires an idea")
	}
	if options.promote.ToSpec == "" {
		return ideaPromoteOptions{}, fmt.Errorf("idea promote requires --to-spec")
	}
	return options, nil
}

func parseIdeaArchiveArgs(args []string) (ideaArchiveOptions, error) {
	var options ideaArchiveOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--reason":
			value, err := consumeFlagValue(args, &i, "--reason")
			if err != nil {
				return ideaArchiveOptions{}, err
			}
			options.archive.Reason = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return ideaArchiveOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			options.archive.Refs = append(options.archive.Refs, args[i])
		}
	}
	if len(options.archive.Refs) == 0 {
		return ideaArchiveOptions{}, fmt.Errorf("idea archive requires at least one idea")
	}
	return options, nil
}

func parseSparkListArgs(args []string) (sparkListOptions, error) {
	var options sparkListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--all":
			options.filters.All = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return sparkListOptions{}, err
			}
			options.filters.Status = value
		default:
			return sparkListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseSparkCaptureArgs(args []string) (sparkCaptureOptions, error) {
	var options sparkCaptureOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--scope":
			value, err := consumeFlagValue(args, &i, "--scope")
			if err != nil {
				return sparkCaptureOptions{}, err
			}
			options.capture.Scope = value
		case "--text":
			value, err := consumeFlagValue(args, &i, "--text")
			if err != nil {
				return sparkCaptureOptions{}, err
			}
			options.capture.Text = value
		default:
			return sparkCaptureOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.capture.Text) == "" {
		return sparkCaptureOptions{}, fmt.Errorf("spark capture requires --text")
	}
	return options, nil
}

func parseSparkResolveArgs(args []string) (sparkResolveOptions, error) {
	var options sparkResolveOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--by":
			value, err := consumeFlagValue(args, &i, "--by")
			if err != nil {
				return sparkResolveOptions{}, err
			}
			options.resolve.By = value
		case "--reason":
			value, err := consumeFlagValue(args, &i, "--reason")
			if err != nil {
				return sparkResolveOptions{}, err
			}
			options.resolve.Reason = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return sparkResolveOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.resolve.Spark != "" {
				return sparkResolveOptions{}, fmt.Errorf("spark resolve accepts exactly one spark")
			}
			options.resolve.Spark = args[i]
		}
	}
	if options.resolve.Spark == "" {
		return sparkResolveOptions{}, fmt.Errorf("spark resolve requires a spark")
	}
	if options.resolve.By == "" {
		return sparkResolveOptions{}, fmt.Errorf("spark resolve requires --by")
	}
	return options, nil
}

func parseSparkPromoteArgs(args []string) (sparkPromoteOptions, error) {
	var options sparkPromoteOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--to-idea":
			value, err := consumeFlagValue(args, &i, "--to-idea")
			if err != nil {
				return sparkPromoteOptions{}, err
			}
			options.promote.ToIdea = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return sparkPromoteOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.promote.Spark != "" {
				return sparkPromoteOptions{}, fmt.Errorf("spark promote accepts exactly one spark")
			}
			options.promote.Spark = args[i]
		}
	}
	if options.promote.Spark == "" {
		return sparkPromoteOptions{}, fmt.Errorf("spark promote requires a spark")
	}
	if options.promote.ToIdea == "" {
		return sparkPromoteOptions{}, fmt.Errorf("spark promote requires --to-idea")
	}
	return options, nil
}

func parseSingleRefArgs(command string, args []string) (string, bool, error) {
	ref := ""
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, fmt.Errorf("unknown option %q", arg)
			}
			if ref != "" {
				return "", false, fmt.Errorf("%s accepts exactly one argument", command)
			}
			ref = arg
		}
	}
	if ref == "" {
		return "", false, fmt.Errorf("%s requires an argument", command)
	}
	return ref, jsonOutput, nil
}

func parseArchiveArgs(command string, args []string) ([]string, bool, error) {
	var refs []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, false, fmt.Errorf("unknown option %q", arg)
			}
			refs = append(refs, arg)
		}
	}
	if len(refs) == 0 {
		return nil, false, fmt.Errorf("%s requires at least one ref", command)
	}
	return refs, jsonOutput, nil
}

func parseTagMutationArgs(command string, args []string) (tagMutationOptions, error) {
	var options tagMutationOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return tagMutationOptions{}, fmt.Errorf("%s requires an entity and tag", command)
	}
	options.ref = positional[0]
	options.name = positional[1]
	return options, nil
}

func parseBundleCreateArgs(args []string) (bundleCreateOptions, error) {
	var options bundleCreateOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return bundleCreateOptions{}, err
			}
			options.title = value
		case "--tag":
			value, err := consumeFlagValue(args, &i, "--tag")
			if err != nil {
				return bundleCreateOptions{}, err
			}
			options.tags = append(options.tags, value)
		default:
			if strings.HasPrefix(args[i], "-") {
				return bundleCreateOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.slug != "" {
				return bundleCreateOptions{}, fmt.Errorf("bundle create accepts exactly one slug")
			}
			options.slug = args[i]
		}
	}
	if options.slug == "" {
		return bundleCreateOptions{}, fmt.Errorf("bundle create requires a slug")
	}
	return options, nil
}

func parseBundleUpdateArgs(args []string) (bundleUpdateOptions, error) {
	var options bundleUpdateOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return bundleUpdateOptions{}, err
			}
			options.title = value
			options.setTitle = true
		case "--tag":
			value, err := consumeFlagValue(args, &i, "--tag")
			if err != nil {
				return bundleUpdateOptions{}, err
			}
			options.tags = append(options.tags, value)
			options.setTags = true
		case "--clear-tags":
			options.tags = nil
			options.setTags = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return bundleUpdateOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.slug != "" {
				return bundleUpdateOptions{}, fmt.Errorf("bundle update accepts exactly one slug")
			}
			options.slug = args[i]
		}
	}
	if options.slug == "" {
		return bundleUpdateOptions{}, fmt.Errorf("bundle update requires a slug")
	}
	if !options.setTitle && !options.setTags {
		return bundleUpdateOptions{}, fmt.Errorf("bundle update requires --title, --tag, or --clear-tags")
	}
	return options, nil
}

func parseBundleMemberArgs(command string, args []string) (bundleMemberOptions, error) {
	var options bundleMemberOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return bundleMemberOptions{}, fmt.Errorf("%s requires a bundle slug and entity", command)
	}
	options.slug = positional[0]
	options.ref = positional[1]
	return options, nil
}

func parseLinkMutationArgs(command string, args []string) (linkMutationOptions, error) {
	var options linkMutationOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--type":
			value, err := consumeFlagValue(args, &i, "--type")
			if err != nil {
				return linkMutationOptions{}, err
			}
			options.relationshipType = value
		case "--reason":
			value, err := consumeFlagValue(args, &i, "--reason")
			if err != nil {
				return linkMutationOptions{}, err
			}
			options.reason = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return linkMutationOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 2 {
		return linkMutationOptions{}, fmt.Errorf("%s requires a source entity and target entity", command)
	}
	if options.relationshipType == "" {
		return linkMutationOptions{}, fmt.Errorf("%s requires --type", command)
	}
	options.from = positional[0]
	options.to = positional[1]
	return options, nil
}

func parseSessionListArgs(args []string) (sessionListOptions, error) {
	var options sessionListOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--all":
			options.filters.All = true
		default:
			return sessionListOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	return options, nil
}

func parseSessionLogArgs(args []string) (sessionLogOptions, error) {
	var options sessionLogOptions
	var entryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--session-id", "--harness-session-id":
			value, err := consumeFlagValue(args, &i, args[i])
			if err != nil {
				return sessionLogOptions{}, err
			}
			options.harnessSessionID = value
		default:
			entryParts = append(entryParts, args[i])
		}
	}
	if len(entryParts) != 1 {
		return sessionLogOptions{}, fmt.Errorf("session log accepts exactly one entry")
	}
	options.entry = entryParts[0]
	return options, nil
}

func parseReportListArgs(args []string) (reportListOptions, error) {
	var options reportListOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--type":
			value, err := consumeFlagValue(args, &i, "--type")
			if err != nil {
				return reportListOptions{}, err
			}
			options.filters.Type = value
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return reportListOptions{}, err
			}
			options.filters.Status = value
		default:
			return reportListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseReportCreateArgs(args []string) (reportCreateOptions, error) {
	options := reportCreateOptions{create: state.ReportCreateOptions{Kind: "research", Source: "ad-hoc"}}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--type":
			value, err := consumeFlagValue(args, &i, "--type")
			if err != nil {
				return reportCreateOptions{}, err
			}
			options.create.Kind = value
		case "--source":
			value, err := consumeFlagValue(args, &i, "--source")
			if err != nil {
				return reportCreateOptions{}, err
			}
			options.create.Source = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return reportCreateOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return reportCreateOptions{}, fmt.Errorf("report create requires exactly one slug")
	}
	options.create.Slug = positional[0]
	return options, nil
}

func parseReportGenerateArgs(args []string) (reportGenerateOptions, error) {
	if len(args) == 0 {
		return reportGenerateOptions{}, fmt.Errorf("report generate requires a kind")
	}
	options := reportGenerateOptions{kind: args[0]}
	positional := args[1:]
	for _, arg := range positional {
		if strings.HasPrefix(arg, "-") {
			return reportGenerateOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	switch options.kind {
	case state.ExportKindSession:
		if len(positional) != 1 {
			return reportGenerateOptions{}, fmt.Errorf("report generate session requires exactly one session")
		}
		options.ref = positional[0]
	case state.ExportKindTriage, state.ExportKindReleaseReadiness:
		if len(positional) != 0 {
			return reportGenerateOptions{}, fmt.Errorf("report generate %s does not accept positional arguments", options.kind)
		}
	default:
		return reportGenerateOptions{}, fmt.Errorf("report generate kind %q is not implemented yet", options.kind)
	}
	return options, nil
}

func taskStatusDisplayOrder(filters state.TaskListOptions) []string {
	if filters.Status != "" {
		return []string{filters.Status}
	}
	statuses := []string{"in_progress", "blocked", "todo", "review", "done", "archived"}
	if !filters.Active {
		return statuses
	}
	return statuses[:len(statuses)-2]
}

func sortedTasksByStatus(tasks state.TaskList, status string) []string {
	var ids []string
	for id, task := range tasks.Tasks {
		if task.Status == status {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func sortedIdeas(ideas state.IdeaList) []string {
	var aliases []string
	for alias := range ideas.Ideas {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func sortedBrainstorms(brainstorms state.BrainstormList) []string {
	var aliases []string
	for alias := range brainstorms.Brainstorms {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func sortedSparks(sparks state.SparkList) []string {
	var aliases []string
	for alias := range sparks.Sparks {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func sortedTags(tags state.TagList) []string {
	var names []string
	for name := range tags.Tags {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func taskStatusLabel(status string) string {
	switch status {
	case "in_progress":
		return "In Progress"
	case "blocked":
		return "Blocked"
	case "todo":
		return "Todo"
	case "review":
		return "Review"
	case "done":
		return "Done"
	case "archived":
		return "Archived"
	default:
		return status
	}
}

func specStatusDisplayOrder(specs state.SpecList) []string {
	statuses := state.SpecStatusOrder()
	seen := map[string]bool{}
	for _, status := range statuses {
		seen[status] = true
	}
	var extra []string
	for _, spec := range specs.Specs {
		if !seen[spec.Status] {
			seen[spec.Status] = true
			extra = append(extra, spec.Status)
		}
	}
	sort.Strings(extra)
	return append(statuses, extra...)
}

func sortedSpecsByStatus(specs state.SpecList, status string) []string {
	var ids []string
	for id, spec := range specs.Specs {
		if spec.Status == status {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func specStatusLabel(status string) string {
	switch status {
	case "implementing":
		return "Implementing"
	case "approved":
		return "Approved"
	case "drafting":
		return "Drafting"
	case "complete":
		return "Complete"
	case "archived":
		return "Archived"
	default:
		return status
	}
}

func sortedSessionsByArchivedState(sessions state.SessionList, archived bool) []string {
	var aliases []string
	for alias, session := range sessions.Sessions {
		if (session.Status == "archived") == archived {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return aliases
}

func reportStatusDisplayOrder(reports state.ReportList) []string {
	statuses := []string{"draft", "final", "archived"}
	seen := map[string]bool{}
	for _, status := range statuses {
		seen[status] = true
	}
	var extra []string
	for _, report := range reports.Reports {
		if !seen[report.Status] {
			seen[report.Status] = true
			extra = append(extra, report.Status)
		}
	}
	sort.Strings(extra)
	return append(statuses, extra...)
}

func sortedReportsByStatus(reports state.ReportList, status string) []string {
	var aliases []string
	for alias, report := range reports.Reports {
		if report.Status == status {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return aliases
}

func reportStatusLabel(status string) string {
	switch status {
	case "draft":
		return "Drafts"
	case "final":
		return "Final"
	case "archived":
		return "Archived"
	default:
		return status
	}
}

type markdownMigrationOptions struct {
	jsonOutput bool
	apply      bool
	dryRun     bool
	resume     bool
}

func parseMarkdownMigrationArgs(args []string) (markdownMigrationOptions, error) {
	var options markdownMigrationOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		case "--resume":
			options.resume = true
		default:
			return markdownMigrationOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return markdownMigrationOptions{}, fmt.Errorf("state migrate markdown cannot combine --apply and --dry-run")
	}
	if options.resume && options.dryRun {
		return markdownMigrationOptions{}, fmt.Errorf("state migrate markdown cannot combine --resume and --dry-run")
	}
	if options.resume && options.apply {
		return markdownMigrationOptions{}, fmt.Errorf("state migrate markdown cannot combine --resume and --apply")
	}
	return options, nil
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (r Runner) runLegacy(args []string, cwd string, out io.Writer) error {
	legacyRunner := r.Legacy
	legacyRunner.Stdin = firstReader(legacyRunner.Stdin, r.Stdin, os.Stdin)
	legacyRunner.Stdout = firstWriter(legacyRunner.Stdout, out, os.Stdout)
	legacyRunner.Stderr = firstWriter(legacyRunner.Stderr, r.Stderr, os.Stderr)
	if legacyRunner.Cwd == "" {
		legacyRunner.Cwd = cwd
	}
	return legacyRunner.Run(args)
}

func firstReader(readers ...io.Reader) io.Reader {
	for _, reader := range readers {
		if reader != nil {
			return reader
		}
	}
	return nil
}

func firstWriter(writers ...io.Writer) io.Writer {
	for _, writer := range writers {
		if writer != nil {
			return writer
		}
	}
	return nil
}
