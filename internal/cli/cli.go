package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// Runner owns Go-native command dispatch.
type Runner struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	WorkingDir string
	StateHome  string
}

type housekeepingOptions struct {
	jsonOutput bool
	dryRun     bool
	sections   map[string]bool
}

type compatibilityCommandSummary struct {
	ContractVersion int            `json:"contract_version"`
	Version         int            `json:"version"`
	Command         string         `json:"command"`
	Mode            string         `json:"mode"`
	Action          string         `json:"action"`
	Reason          string         `json:"reason"`
	Counts          map[string]int `json:"counts,omitempty"`
}

type compatibilityCommandOptions struct {
	jsonOutput bool
}

type projectRenameOptions struct {
	name       string
	dryRun     bool
	jsonOutput bool
}

type projectMoveOptions struct {
	fromPath   string
	toPath     string
	dryRun     bool
	jsonOutput bool
}

type backupVerifyOptions struct {
	path       string
	jsonOutput bool
}

type commandErrorJSON struct {
	ContractVersion int    `json:"contract_version"`
	Command         string `json:"command"`
	Error           string `json:"error"`
	BackupPath      string `json:"backup_path,omitempty"`
}

type statePathResult struct {
	ContractVersion int    `json:"contract_version"`
	DatabaseScope   string `json:"database_scope"`
	ProjectRoot     string `json:"project_root"`
	DatabasePath    string `json:"database_path"`
}

type statePathOptions struct {
	jsonOutput    bool
	verboseOutput bool
}

// Run dispatches a loaf command.
func (r Runner) Run(args []string) error {
	out := r.Stdout
	if out == nil {
		out = os.Stdout
	}
	errOut := r.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}

	workingDir, err := project.ResolveWorkingDirectory(r.WorkingDir)
	if err != nil {
		return err
	}
	if shouldRefuseCommandNative(args, workingDir.Path()) {
		if unknown := unknownTopLevelCommandNative(args); unknown != "" {
			fmt.Fprintf(errOut, "error: unknown command '%s'\n\n", unknown)
		}
		if mainMissing := detectMainMissingForRefusalNative(workingDir.Path()); mainMissing != "" {
			fmt.Fprintln(errOut, mainMissing)
		} else {
			fmt.Fprintln(errOut, preA3RefusalMessageNative)
		}
		return ExitError{Code: 2}
	}
	runtime := state.NewRuntime(workingDir)

	if len(args) > 0 && args[0] == "__generate-cli-ref" {
		return r.runGenerateCLIReference(args[1:], out, runtime.RootPath())
	}

	if len(args) == 0 {
		writeRootHelp(out)
		return nil
	}

	var dispatchErr error
	switch args[0] {
	case "--help", "-h", "help":
		writeRootHelp(out)
		return nil
	case "--agent-help":
		dispatchErr = writeAgentHelpJSON(out)
	case "--version", "-v":
		dispatchErr = r.runVersion(out, runtime.RootPath())
	case "build":
		dispatchErr = r.runBuild(args[1:], out, runtime.RootPath())
	case "init":
		dispatchErr = r.runInit(args[1:], out, runtime.RootPath())
	case "install":
		dispatchErr = r.runInstall(args[1:], out, runtime.RootPath())
	case "migrate":
		dispatchErr = r.runMigrate(args[1:], out, runtime)
	case "release":
		dispatchErr = r.runRelease(args[1:], out, runtime.RootPath())
	case "setup":
		dispatchErr = r.runSetup(args[1:], out, runtime.RootPath())
	case "state":
		dispatchErr = r.runState(args[1:], out, runtime)
	case "project":
		dispatchErr = r.runProject(args[1:], out, runtime)
	case "trace":
		dispatchErr = r.runTrace(args[1:], out, runtime)
	case "brainstorm":
		dispatchErr = r.runBrainstorm(args[1:], out, runtime)
	case "idea":
		dispatchErr = r.runIdea(args[1:], out, runtime)
	case "spark":
		dispatchErr = r.runSpark(args[1:], out, runtime)
	case "tag":
		dispatchErr = r.runTag(args[1:], out, runtime)
	case "bundle":
		dispatchErr = r.runBundle(args[1:], out, runtime)
	case "check":
		dispatchErr = r.runCheck(args[1:], out, runtime.RootPath())
	case "doctor":
		dispatchErr = r.runDoctor(args[1:], out, runtime.RootPath())
	case "link":
		dispatchErr = r.runLink(args[1:], out, runtime)
	case "report":
		dispatchErr = r.runReport(args[1:], out, runtime)
	case "spec":
		dispatchErr = r.runSpec(args[1:], out, runtime)
	case "session":
		dispatchErr = r.runSession(args[1:], out, runtime)
	case "task":
		dispatchErr = r.runTask(args[1:], out, runtime)
	case "housekeeping":
		dispatchErr = r.runHousekeeping(args[1:], out, runtime)
	case "kb":
		dispatchErr = r.runKb(args[1:], out, runtime.RootPath())
	case "version":
		dispatchErr = r.runVersion(out, runtime.RootPath())
	default:
		fmt.Fprintf(errOut, "error: unknown command '%s'\n\n", args[0])
		writeRootHelp(errOut)
		return ExitError{Code: 1}
	}
	return writeJSONCommandErrorFallback(out, args, dispatchErr)
}

func writeJSONCommandErrorFallback(out io.Writer, args []string, err error) error {
	if err == nil {
		return nil
	}
	var silent interface {
		ExitCode() int
		Silent() bool
	}
	if errors.As(err, &silent) && silent.Silent() {
		return err
	}
	if !hasFlag(args, "--json") {
		return err
	}
	return writeJSONCommandError(out, jsonErrorCommand(args), err)
}

func jsonErrorCommand(args []string) string {
	if len(args) == 0 {
		return "loaf"
	}
	parts := []string{args[0]}
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") || arg == "help" {
			break
		}
		parts = append(parts, arg)
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func writeRootHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf <command> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Loaf — An Opinionated Agentic Framework")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  build         Build Loaf content targets")
	fmt.Fprintln(out, "  init          Scaffold project agent files")
	fmt.Fprintln(out, "  install       Install Loaf into agent tools")
	fmt.Fprintln(out, "  setup         Initialize, build, and install")
	fmt.Fprintln(out, "  state         Manage native SQLite state")
	fmt.Fprintln(out, "  project       Manage project identity")
	fmt.Fprintln(out, "  migrate       Run migration workflows")
	fmt.Fprintln(out, "  session       Manage sessions")
	fmt.Fprintln(out, "  task          Manage tasks")
	fmt.Fprintln(out, "  spec          Manage specs")
	fmt.Fprintln(out, "  report        Manage reports")
	fmt.Fprintln(out, "  kb            Manage knowledge base")
	fmt.Fprintln(out, "  check         Run hook checks")
	fmt.Fprintln(out, "  doctor        Diagnose project alignment")
	fmt.Fprintln(out, "  release       Create a release")
	fmt.Fprintln(out, "  version       Show version and content counts")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help     Show help")
	fmt.Fprintln(out, "  -v, --version  Show version")
}

func (r Runner) runHousekeeping(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		writeHousekeepingHelp(out)
		return nil
	}
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
		result, err := markdownHousekeepingSummary(projectRoot.Path())
		if err != nil {
			return err
		}
		result = filterHousekeepingSummary(result, options.sections)
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeMarkdownHousekeepingSummary(out, result, options)
		return nil
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

func writeHousekeepingHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf housekeeping [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Scan agent artifacts and summarize housekeeping recommendations.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON")
	fmt.Fprintln(out, "  --dry-run    Show recommendations without applying actions")
	fmt.Fprintln(out, "  --sessions   Only review sessions")
	fmt.Fprintln(out, "  --specs      Only review specs")
	fmt.Fprintln(out, "  --drafts     Only review shaping drafts")
	fmt.Fprintln(out, "  --plans      Accept legacy plans filter for compatibility")
	fmt.Fprintln(out, "  --handoffs   Accept legacy handoffs filter for compatibility")
	fmt.Fprintln(out, "  -h, --help   Show help")
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

func isHelpArg(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")
}

func writeNestedHelp(out io.Writer, args []string, writers map[string]func(io.Writer)) bool {
	if len(args) != 2 || !isHelpArg(args[1:]) {
		return false
	}
	writeHelp, ok := writers[args[0]]
	if !ok {
		return false
	}
	writeHelp(out)
	return true
}

func writeUsageHelp(out io.Writer, usage string, summary string, options ...string) {
	fmt.Fprintf(out, "Usage: %s\n", usage)
	fmt.Fprintln(out)
	fmt.Fprintln(out, summary)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	for _, option := range options {
		fmt.Fprintf(out, "  %s\n", option)
	}
	fmt.Fprintln(out, "  -h, --help   Show help")
}

type subcommandHelpItem struct {
	Name    string
	Summary string
}

func writeCommandGroupHelp(out io.Writer, usage string, summary string, items []subcommandHelpItem) {
	fmt.Fprintf(out, "Usage: %s\n", usage)
	fmt.Fprintln(out)
	fmt.Fprintln(out, summary)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	for _, item := range items {
		fmt.Fprintf(out, "  %-10s%s\n", item.Name, item.Summary)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeHousekeepingSummary(out io.Writer, result state.HousekeepingSummary, options housekeepingOptions) {
	if options.dryRun {
		fmt.Fprint(out, "\n  loaf housekeeping (SQLite state, dry run)\n\n")
	} else {
		fmt.Fprint(out, "\n  loaf housekeeping (SQLite state)\n\n")
	}
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintln(out)
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

func writeMarkdownHousekeepingSummary(out io.Writer, result state.HousekeepingSummary, options housekeepingOptions) {
	if options.dryRun {
		fmt.Fprint(out, "\n  loaf housekeeping (markdown, dry run)\n\n")
	} else {
		fmt.Fprint(out, "\n  loaf housekeeping (markdown)\n\n")
	}
	fmt.Fprintf(out, "  artifacts: %s\n\n", result.DatabasePath)
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
		fmt.Fprint(out, "\n  No markdown housekeeping signals.\n\n")
		return
	}
	fmt.Fprint(out, "\n  Signals:\n")
	for _, signal := range result.Signals {
		fmt.Fprintf(out, "    %s\n", signal)
	}
	fmt.Fprintln(out)
}

func writeCompatibilityCommandSummary(out io.Writer, summary compatibilityCommandSummary) {
	fmt.Fprintf(out, "\n  loaf %s (compatibility)\n\n", summary.Command)
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
		ContractVersion:    result.ContractVersion,
		DatabaseScope:      result.DatabaseScope,
		DatabasePath:       result.DatabasePath,
		ProjectID:          result.ProjectID,
		ProjectName:        result.ProjectName,
		ProjectCurrentPath: result.ProjectCurrentPath,
		Version:            result.Version,
		Sections:           map[string]state.HousekeepingSection{},
	}
	for section := range sections {
		if value, ok := result.Sections[section]; ok {
			filtered.Sections[section] = value
		}
	}
	filtered.Signals = housekeepingSignalsFromSections(filtered.Sections)
	return filtered
}

type markdownHousekeepingSectionSpec struct {
	name            string
	relativeDir     string
	cleanupStatuses map[string]bool
}

var markdownHousekeepingSectionSpecs = []markdownHousekeepingSectionSpec{
	{name: "brainstorms", relativeDir: filepath.Join("drafts", "brainstorms"), cleanupStatuses: cleanupStatusSet("resolved", "archived")},
	{name: "ideas", relativeDir: "ideas", cleanupStatuses: cleanupStatusSet("resolved", "archived")},
	{name: "reports", relativeDir: "reports", cleanupStatuses: cleanupStatusSet("final", "archived")},
	{name: "sessions", relativeDir: "sessions", cleanupStatuses: cleanupStatusSet("done", "stopped", "archived")},
	{name: "shaping_drafts", relativeDir: "drafts", cleanupStatuses: cleanupStatusSet("absorbed", "archived")},
	{name: "sparks", relativeDir: "sparks", cleanupStatuses: cleanupStatusSet("resolved", "archived")},
	{name: "specs", relativeDir: "specs", cleanupStatuses: cleanupStatusSet("complete", "archived")},
	{name: "tasks", relativeDir: "tasks", cleanupStatuses: cleanupStatusSet("done", "archived")},
}

func markdownHousekeepingSummary(rootPath string) (state.HousekeepingSummary, error) {
	agentsDir := filepath.Join(rootPath, ".agents")
	sections := map[string]state.HousekeepingSection{}
	for _, spec := range markdownHousekeepingSectionSpecs {
		section, err := markdownHousekeepingSection(agentsDir, spec)
		if err != nil {
			return state.HousekeepingSummary{}, err
		}
		sections[spec.name] = section
	}
	return state.HousekeepingSummary{
		Version:      1,
		DatabasePath: agentsDir,
		Sections:     sections,
		Signals:      housekeepingSignalsFromSections(sections),
	}, nil
}

func markdownHousekeepingSection(agentsDir string, spec markdownHousekeepingSectionSpec) (state.HousekeepingSection, error) {
	dir := filepath.Join(agentsDir, spec.relativeDir)
	section := state.HousekeepingSection{ByStatus: map[string]int{}}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		status, err := markdownArtifactStatus(path)
		if err != nil {
			return err
		}
		if markdownPathIsArchived(dir, path) {
			status = "archived"
		}
		if status == "" {
			status = "unknown"
		}
		section.Total++
		section.ByStatus[status]++
		if spec.cleanupStatuses[status] {
			section.CleanupCandidate++
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return section, nil
		}
		return state.HousekeepingSection{}, fmt.Errorf("scan markdown housekeeping %s: %w", spec.name, err)
	}
	return section, nil
}

func markdownArtifactStatus(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read markdown artifact %s: %w", path, err)
	}
	fields, ok := parseKnowledgeFrontmatter(body)
	if !ok {
		return "unknown", nil
	}
	return firstFieldValue(fields["status"]), nil
}

func markdownPathIsArchived(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts[:len(parts)-1] {
		if part == "archive" {
			return true
		}
	}
	return false
}

func cleanupStatusSet(statuses ...string) map[string]bool {
	set := map[string]bool{}
	for _, status := range statuses {
		set[status] = true
	}
	return set
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
	if len(args) == 0 || isHelpArg(args) {
		writeBrainstormHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":    writeBrainstormListHelp,
		"show":    writeBrainstormShowHelp,
		"promote": writeBrainstormPromoteHelp,
		"archive": writeBrainstormArchiveHelp,
	}) {
		return nil
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

func writeBrainstormHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf brainstorm <subcommand> [options]", "Manage brainstorms in native SQLite state.", []subcommandHelpItem{
		{Name: "list", Summary: "List brainstorms"},
		{Name: "show", Summary: "Show one brainstorm"},
		{Name: "promote", Summary: "Promote a brainstorm to an idea"},
		{Name: "archive", Summary: "Archive brainstorms"},
	})
}

func writeBrainstormListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf brainstorm list [--all|--status <status>] [--json]", "List brainstorms from SQLite state.", "--all        Include archived brainstorms", "--status     Filter by status", "--json       Output brainstorms, global database scope, and project identity as JSON")
}

func writeBrainstormShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf brainstorm show <brainstorm> [--json]", "Show one brainstorm from SQLite state.", "--json       Output brainstorm details, relationships, global database scope, and project identity as JSON")
}

func writeBrainstormPromoteHelp(out io.Writer) {
	writeUsageHelp(out, "loaf brainstorm promote <brainstorm> --to-idea <idea> [--json]", "Record brainstorm-to-idea promotion.", "--to-idea    Target idea", "--json       Output promotion relationship, global database scope, and project identity as JSON")
}

func writeBrainstormArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf brainstorm archive <brainstorm...> [--reason <text>] [--json]", "Archive one or more brainstorms.", "--reason     Archive reason", "--json       Output archive result, archived brainstorms, global database scope, and project identity as JSON")
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	fmt.Fprint(out, "\n  loaf brainstorm list\n\n")
	writeProjectMutationContext(out, "  ", brainstorms.DatabaseScope, brainstorms.DatabasePath, brainstorms.ProjectID, brainstorms.ProjectName, brainstorms.ProjectCurrentPath)
	if len(brainstorms.Brainstorms) == 0 {
		if brainstorms.DatabaseScope != "" || brainstorms.DatabasePath != "" || brainstorms.ProjectID != "" || brainstorms.ProjectName != "" || brainstorms.ProjectCurrentPath != "" {
			fmt.Fprintln(out)
		}
		fmt.Fprint(out, "  No brainstorms found.\n\n")
		return
	}

	if brainstorms.DatabaseScope != "" || brainstorms.DatabasePath != "" || brainstorms.ProjectID != "" || brainstorms.ProjectName != "" || brainstorms.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
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
	if len(args) == 0 || isHelpArg(args) {
		writeStateHelp(out)
		return nil
	}

	switch args[0] {
	case "path":
		if isHelpArg(args[1:]) {
			writeStatePathHelp(out)
			return nil
		}
		options, err := parseStatePathArgs(args[1:])
		if err != nil {
			return err
		}
		projectRoot, err := project.ResolveRoot(runtime.RootPath())
		if err != nil {
			return err
		}
		path, err := state.PathResolver{StateHome: r.StateHome}.DatabasePath(projectRoot)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, statePathResult{
				ContractVersion: state.StateJSONContractVersion,
				DatabaseScope:   "global",
				ProjectRoot:     projectRoot.Path(),
				DatabasePath:    path,
			})
		}
		if options.verboseOutput {
			fmt.Fprintln(out, "loaf state path")
			fmt.Fprintln(out, "scope: global database")
			fmt.Fprintf(out, "project root: %s\n", projectRoot.Path())
			fmt.Fprintf(out, "database: %s\n", path)
			return nil
		}
		fmt.Fprintln(out, path)
		return nil
	case "init":
		if isHelpArg(args[1:]) {
			writeStateInitHelp(out)
			return nil
		}
		return r.runStateInit(args[1:], out, runtime)
	case "status":
		if isHelpArg(args[1:]) {
			writeStateStatusHelp(out)
			return nil
		}
		return r.runStateStatus(args[1:], out, runtime)
	case "doctor":
		if isHelpArg(args[1:]) {
			writeStateDoctorHelp(out)
			return nil
		}
		return r.runStateDoctor(args[1:], out, runtime)
	case "repair":
		if len(args) == 1 || isHelpArg(args[1:]) {
			writeStateRepairHelp(out)
			return nil
		}
		return r.runStateRepair(args[1:], out, runtime)
	case "migrate":
		if len(args) == 1 || isHelpArg(args[1:]) {
			writeStateMigrateHelp(out)
			return nil
		}
		return r.runStateMigrate(args[1:], out, runtime)
	case "backup":
		if isHelpArg(args[1:]) {
			writeStateBackupHelp(out)
			return nil
		}
		if writeNestedHelp(out, args[1:], map[string]func(io.Writer){
			"verify": writeStateBackupVerifyHelp,
		}) {
			return nil
		}
		return r.runStateBackup(args[1:], out, runtime)
	case "export":
		if len(args) == 1 || isHelpArg(args[1:]) {
			writeStateExportHelp(out)
			return nil
		}
		return r.runStateExport(args[1:], out, runtime)
	default:
		return fmt.Errorf("state subcommand %q is not implemented yet", args[0])
	}
}

func writeStateHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state <command> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage native SQLite state.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  path          Print the resolved SQLite database path")
	fmt.Fprintln(out, "  status        Show SQLite readiness and markdown compatibility status")
	fmt.Fprintln(out, "  init          Initialize native SQLite state")
	fmt.Fprintln(out, "  doctor        Diagnose SQLite state health")
	fmt.Fprintln(out, "  repair        Repair guarded SQLite data drift")
	fmt.Fprintln(out, "  migrate       Run state migrations")
	fmt.Fprintln(out, "  backup        Create a SQLite database backup")
	fmt.Fprintln(out, "  backup verify Verify an existing SQLite backup")
	fmt.Fprintln(out, "  export        Export SQLite state")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help    Show help")
}

func writeStatePathHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state path [--json|--verbose]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Print the resolved native SQLite database path.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output contract version, command, project root, database scope, and database path as JSON")
	fmt.Fprintln(out, "  --verbose    Output command, scope, project root, and database path")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateInitHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state init [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Initialize the native SQLite state database.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output readiness mode, global database scope, database path, and project identity as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateStatusHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state status [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Show SQLite readiness and markdown-only compatibility status.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output readiness mode, diagnostics, global database scope, and project identity as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateDoctorHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state doctor [--fix] [--dry-run] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Diagnose SQLite state health.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --fix        Initialize missing SQLite state when safe")
	fmt.Fprintln(out, "  --dry-run    Show repair plan without applying fixes")
	fmt.Fprintln(out, "  --json       Output diagnostics, repair plan, global database scope, and project identity as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateRepairHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state repair <target> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Repair guarded SQLite data drift.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Targets:")
	fmt.Fprintln(out, "  legacy-project-database  Archive migrated per-project SQLite leftovers")
	fmt.Fprintln(out, "  relationship-origin  Backfill missing relationship provenance")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help           Show help")
}

func writeStateMigrateHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state migrate <source> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run state migrations.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sources:")
	fmt.Fprintln(out, "  markdown      Import .agents Markdown artifacts into SQLite")
	fmt.Fprintln(out, "  storage-home  Copy legacy XDG_STATE_HOME state into XDG_DATA_HOME")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help    Show help")
}

func writeStateBackupHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state backup [verify <backup>] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Create or verify SQLite database backups.")
	fmt.Fprintln(out, "Backups are written under the global data-home backups directory next to the SQLite database.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  verify <backup>  Verify an existing SQLite backup")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output backup verification, checksum, schema version, project count, and current project identity as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateBackupVerifyHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state backup verify <backup> [--json]", "Verify an existing SQLite database backup without reading or mutating live state.", "--json       Output backup verification, restore guidance, schema version, and captured project identities as JSON")
}

func writeStateExportHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state export <kind> --format <format>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Export SQLite state.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Kinds:")
	fmt.Fprintln(out, "  all")
	fmt.Fprintln(out, "  release-readiness")
	fmt.Fprintln(out, "  spec <spec>")
	fmt.Fprintln(out, "  session <session>")
	fmt.Fprintln(out, "  triage")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --format     Output format")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateExportAllHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state export all --format json [--json]", "Export a complete project-scoped SQLite snapshot.", "--format     Output format", "--json       Alias for --format json")
}

func writeStateExportReleaseReadinessHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state export release-readiness --format markdown", "Export a release-readiness report from SQLite state.", "--format     Output format")
}

func writeStateExportSpecHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state export spec <spec> --format markdown", "Export one spec from SQLite state.", "--format     Output format")
}

func writeStateExportSessionHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state export session <session> --format markdown", "Export one session from SQLite state.", "--format     Output format")
}

func writeStateExportTriageHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state export triage --format markdown", "Export a triage summary from SQLite state.", "--format     Output format")
}

func (r Runner) runProject(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeProjectHelp(out)
		return nil
	}

	switch args[0] {
	case "list":
		if isHelpArg(args[1:]) {
			writeProjectListHelp(out)
			return nil
		}
		return r.runProjectList(args[1:], out, runtime)
	case "show", "identity":
		if isHelpArg(args[1:]) {
			writeProjectShowHelp(out)
			return nil
		}
		return r.runProjectShow(args[1:], out, runtime, "loaf project "+args[0])
	case "rename":
		if isHelpArg(args[1:]) {
			writeProjectRenameHelp(out)
			return nil
		}
		return r.runProjectRename(args[1:], out, runtime)
	case "move":
		if isHelpArg(args[1:]) {
			writeProjectMoveHelp(out)
			return nil
		}
		return r.runProjectMove(args[1:], out, runtime)
	default:
		return fmt.Errorf("project subcommand %q is not implemented yet", args[0])
	}
}

func writeProjectHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf project <command> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage durable project identity in the global SQLite database.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  list      List registered projects")
	fmt.Fprintln(out, "  show      Show the current project identity")
	fmt.Fprintln(out, "  identity  Alias for show")
	fmt.Fprintln(out, "  rename    Rename the friendly project name")
	fmt.Fprintln(out, "  move      Record a project path move")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help    Show help")
}

func writeProjectListHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf project list [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "List registered projects in the global SQLite database.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output database path, project IDs, friendly names, and current paths as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeProjectShowHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf project show|identity [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Show the current durable project identity.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output project ID, friendly name, current path, and database path as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeProjectRenameHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf project rename <name> [--dry-run] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Rename the friendly project name without changing its ID.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --dry-run    Validate and preview without writing")
	fmt.Fprintln(out, "  --json       Output project ID, friendly name, current path, database path, and applied status as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeProjectMoveHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf project move <from> [to] [--dry-run] [--json]")
	fmt.Fprintln(out, "       loaf project move --from <path> [--to <path>] [--dry-run] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Record a checkout move without changing the project ID. The target path defaults to the current project root.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --from       Previous absolute project path")
	fmt.Fprintln(out, "  --to         New absolute project path")
	fmt.Fprintln(out, "  --dry-run    Validate and preview without writing")
	fmt.Fprintln(out, "  --json       Output project ID, friendly name, current path, database path, and applied status as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func (r Runner) runProjectList(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	_, store, err := r.openProjectStoreReadOnly(runtime)
	if err != nil {
		return err
	}
	defer store.Close()
	projects, err := store.ListProjects(context.Background())
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, projects)
	}
	writeProjectList(out, projects)
	return nil
}

func (r Runner) runProjectShow(args []string, out io.Writer, runtime state.Runtime, displayCommand string) error {
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		return err
	}
	projectRoot, store, err := r.openProjectStoreReadOnly(runtime)
	if err != nil {
		return err
	}
	if err := validateProjectPathInvariantsForCommand(store); err != nil {
		store.Close()
		return err
	}
	defer store.Close()
	identity, err := store.LookupProjectIdentityForRoot(context.Background(), projectRoot)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("project identity is not registered for %s; run `loaf state init` to register this checkout or `loaf project move --from <old-path>` after moving a registered checkout", projectRoot.Path())
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, identity)
	}
	writeProjectIdentity(out, displayCommand, identity)
	return nil
}

func (r Runner) runProjectRename(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseProjectRenameArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "project rename", err)
		}
		return err
	}
	projectRoot, store, err := r.openProjectStoreReadOnly(runtime)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "project rename", err)
		}
		return err
	}
	if err := validateProjectPathInvariantsForCommand(store); err != nil {
		store.Close()
		if options.jsonOutput {
			return writeJSONCommandError(out, "project rename", err)
		}
		return err
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	if options.dryRun {
		result, err := store.PreviewRenameProject(context.Background(), projectRoot, options.name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = fmt.Errorf("project identity is not registered for %s; run `loaf state init` to register this checkout or `loaf project move --from <old-path>` after moving a registered checkout", projectRoot.Path())
			}
			if options.jsonOutput {
				return writeJSONCommandError(out, "project rename", err)
			}
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeProjectRenameHuman(out, result, false)
		return nil
	}
	preview, err := store.PreviewRenameProject(context.Background(), projectRoot, options.name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("project identity is not registered for %s; run `loaf state init` to register this checkout or `loaf project move --from <old-path>` after moving a registered checkout", projectRoot.Path())
		}
		if options.jsonOutput {
			return writeJSONCommandError(out, "project rename", err)
		}
		return err
	}
	store.Close()
	store = nil
	projectRoot, store, err = r.openProjectStore(runtime)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "project rename", err)
		}
		return err
	}
	identity, err := store.RenameProject(context.Background(), projectRoot, options.name)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "project rename", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, identity)
	}
	writeProjectRenameHuman(out, state.ProjectRenameResult{
		ContractVersion: state.StateJSONContractVersion,
		DatabaseScope:   identity.DatabaseScope,
		Project:         identity,
		FromName:        preview.FromName,
		ToName:          identity.FriendlyName,
		Action:          "renamed",
	}, true)
	return nil
}

func (r Runner) runProjectMove(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseProjectMoveArgs(args, runtime.RootPath())
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "project move", err)
		}
		return err
	}
	projectRoot, store, err := r.openProjectStoreReadOnly(runtime)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "project move", err)
		}
		return err
	}
	if err := validateProjectPathInvariantsForCommand(store); err != nil {
		store.Close()
		if options.jsonOutput {
			return writeJSONCommandError(out, "project move", err)
		}
		return err
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	var result state.ProjectMoveResult
	if options.dryRun {
		result, err = store.PreviewMoveProject(context.Background(), projectRoot, options.fromPath, options.toPath)
	} else {
		if _, err = store.PreviewMoveProject(context.Background(), projectRoot, options.fromPath, options.toPath); err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "project move", err)
			}
			return err
		}
		store.Close()
		store = nil
		projectRoot, store, err = r.openProjectStore(runtime)
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "project move", err)
			}
			return err
		}
		result, err = store.MoveProject(context.Background(), projectRoot, options.fromPath, options.toPath)
	}
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "project move", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeProjectMoveHuman(out, result, !options.dryRun)
	return nil
}

func (r Runner) openProjectIdentityStore(runtime state.Runtime) (project.Root, *state.Store, error) {
	projectRoot, store, err := r.openProjectStore(runtime)
	if err != nil {
		return project.Root{}, nil, err
	}
	if err := store.UpsertProject(context.Background(), projectRoot); err != nil {
		store.Close()
		return project.Root{}, nil, err
	}
	return projectRoot, store, nil
}

func (r Runner) openProjectStoreReadOnly(runtime state.Runtime) (project.Root, *state.Store, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, nil, err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	databasePath, err := resolver.DatabasePath(projectRoot)
	if err != nil {
		return project.Root{}, nil, err
	}
	if info, err := os.Stat(databasePath); err != nil {
		if os.IsNotExist(err) {
			return project.Root{}, nil, fmt.Errorf("project state database does not exist at %s (scope: global database); run `loaf state status` to inspect it or `loaf state init` to create it", databasePath)
		}
		return project.Root{}, nil, fmt.Errorf("stat state database: %w", err)
	} else if info.IsDir() {
		return project.Root{}, nil, fmt.Errorf("state database path is a directory: %s", databasePath)
	}
	store, err := state.OpenStoreReadOnly(databasePath)
	if err != nil {
		return project.Root{}, nil, err
	}
	_, err = store.ValidateCurrentSchema(context.Background())
	if err != nil {
		store.Close()
		return project.Root{}, nil, fmt.Errorf("project state database is invalid at %s (scope: global database): %w; run `loaf state doctor`", databasePath, err)
	}
	return projectRoot, store, nil
}

func validateProjectPathInvariantsForCommand(store *state.Store) error {
	if err := store.ValidateProjectPathInvariants(context.Background()); err != nil {
		return fmt.Errorf("project state path invariants are invalid at %s (scope: global database): %w; run `loaf state doctor`", store.DatabasePath(), err)
	}
	return nil
}

func (r Runner) openProjectStore(runtime state.Runtime) (project.Root, *state.Store, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, nil, err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	databasePath, err := resolver.DatabasePath(projectRoot)
	if err != nil {
		return project.Root{}, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return project.Root{}, nil, fmt.Errorf("create state database directory: %w", err)
	}
	store, err := state.OpenStore(databasePath)
	if err != nil {
		return project.Root{}, nil, err
	}
	if err := store.ApplyMigrations(context.Background()); err != nil {
		store.Close()
		return project.Root{}, nil, err
	}
	return projectRoot, store, nil
}

func writeProjectIdentity(out io.Writer, command string, identity state.ProjectIdentity) {
	fmt.Fprintln(out, command)
	writeProjectMutationHuman(out, identity.DatabaseScope, identity)
}

func writeProjectRenameHuman(out io.Writer, result state.ProjectRenameResult, applied bool) {
	command := "loaf project rename"
	if !applied {
		command += " --dry-run"
	}
	fmt.Fprintln(out, command)
	writeProjectMutationHuman(out, result.DatabaseScope, result.Project)
	fmt.Fprintf(out, "from name: %s\n", result.FromName)
	fmt.Fprintf(out, "to name: %s\n", result.ToName)
	fmt.Fprintf(out, "applied: %t\n", applied)
	if !applied {
		fmt.Fprintln(out, "next: rerun without --dry-run to apply the friendly name change")
	}
}

func writeProjectMoveHuman(out io.Writer, result state.ProjectMoveResult, applied bool) {
	command := "loaf project move"
	if !applied {
		command += " --dry-run"
	}
	fmt.Fprintln(out, command)
	writeProjectMutationHuman(out, result.DatabaseScope, result.Project)
	fmt.Fprintf(out, "from path: %s\n", result.FromPath)
	fmt.Fprintf(out, "to path: %s\n", result.ToPath)
	fmt.Fprintf(out, "applied: %t\n", applied)
	if !applied {
		fmt.Fprintln(out, "next: rerun without --dry-run to record the path move")
	}
}

func writeProjectMutationHuman(out io.Writer, databaseScope string, identity state.ProjectIdentity) {
	fmt.Fprintf(out, "scope: %s database\n", databaseScope)
	fmt.Fprintf(out, "database: %s\n", identity.DatabasePath)
	fmt.Fprintf(out, "project: %s\n", identity.ID)
	fmt.Fprintf(out, "project name: %s\n", firstNonEmpty(identity.FriendlyName, "(unnamed)"))
	fmt.Fprintf(out, "project path: %s\n", firstNonEmpty(identity.CurrentPath, "(none)"))
}

func writeProjectList(out io.Writer, result state.ProjectList) {
	fmt.Fprintln(out, "loaf project list")
	fmt.Fprintf(out, "scope: %s database\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n\n", result.DatabasePath)
	if len(result.Projects) == 0 {
		fmt.Fprintln(out, "No projects registered.")
		return
	}
	for _, project := range result.Projects {
		fmt.Fprintf(out, "project: %s\n", project.ID)
		fmt.Fprintf(out, "project name: %s\n", firstNonEmpty(project.FriendlyName, "(unnamed)"))
		fmt.Fprintf(out, "project path: %s\n", firstNonEmpty(project.CurrentPath, "(none)"))
		if project.LastSeenAt != "" {
			fmt.Fprintf(out, "last seen: %s\n", project.LastSeenAt)
		}
	}
}

func (r Runner) runStateInit(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state init", err)
		}
		return err
	}
	status, err := r.initializeState(runtime)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "state init", err)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, status)
	}
	fmt.Fprintln(out, "loaf state init")
	fmt.Fprintf(out, "project root: %s\n", status.ProjectRoot)
	writeStateProjectIdentity(out, status)
	fmt.Fprintf(out, "scope: %s database\n", status.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", status.DatabasePath)
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	fmt.Fprintf(out, "schema version: %d\n", status.SchemaVersion)
	return nil
}

func (r Runner) runStateStatus(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state status", err)
		}
		return err
	}
	status, err := r.inspectState(runtime)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "state status", err)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, status)
	}
	fmt.Fprintln(out, "loaf state status")
	fmt.Fprintf(out, "project root: %s\n", status.ProjectRoot)
	writeStateProjectIdentity(out, status)
	if status.ProjectID == "" && status.LegacyProjectKey != "" {
		fmt.Fprintf(out, "legacy project key: %s\n", status.LegacyProjectKey)
	}
	fmt.Fprintf(out, "scope: %s database\n", status.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", status.DatabasePath)
	fmt.Fprintf(out, "database exists: %t\n", status.DatabaseExists)
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	fmt.Fprintf(out, "schema version: %d\n", status.SchemaVersion)
	return nil
}

func (r Runner) runStateDoctor(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	jsonOutput, fix, dryRun, err := parseDoctorArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state doctor", err)
		}
		return err
	}
	status, err := r.inspectState(runtime)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "state doctor", err)
		}
		return err
	}
	if dryRun || len(status.Diagnostics) > 0 {
		status.RepairPlan = state.RepairPlanForStatus(status)
	}
	if fix && status.Mode == state.ModeMarkdownOnly {
		if !dryRun {
			status, err = r.initializeState(runtime)
			if err != nil {
				if jsonOutput {
					return writeJSONCommandError(out, "state doctor", err)
				}
				return err
			}
			status.Diagnostics = append([]state.Diagnostic{{
				Severity: "info",
				Code:     "database-initialized",
				Message:  "SQLite state database initialized",
			}}, status.Diagnostics...)
			status.RepairPlan = []state.RepairAction{{
				Code:           "initialize-database",
				DiagnosticCode: "database-missing",
				Category:       state.RepairCategoryLocalDatabase,
				Description:    "Initialized the global SQLite database for this project.",
				Command:        "loaf state doctor --fix",
				Path:           status.DatabasePath,
				Safe:           true,
				Applied:        true,
			}}
		}
	}
	if jsonOutput {
		if err := writeJSON(out, status); err != nil {
			return err
		}
		if status.Mode == state.ModeInvalid {
			return ExitError{Code: 1}
		}
		return nil
	}

	fmt.Fprintln(out, "loaf state doctor")
	fmt.Fprintf(out, "project root: %s\n", status.ProjectRoot)
	writeStateProjectIdentity(out, status)
	fmt.Fprintf(out, "scope: %s database\n", status.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", status.DatabasePath)
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	fmt.Fprintf(out, "schema version: %d\n", status.SchemaVersion)
	for _, diagnostic := range status.Diagnostics {
		fmt.Fprintf(out, "%s\n", formatStateDiagnosticLine("", diagnostic))
	}
	if len(status.RepairPlan) > 0 {
		fmt.Fprintln(out, "repair plan:")
		for _, action := range status.RepairPlan {
			stateLabel := "manual"
			if action.Applied {
				stateLabel = "applied"
			} else if action.Safe {
				stateLabel = "safe"
			}
			if action.Category != "" {
				fmt.Fprintf(out, "- %s [%s/%s]: %s\n", action.Code, stateLabel, action.Category, action.Description)
			} else {
				fmt.Fprintf(out, "- %s [%s]: %s\n", action.Code, stateLabel, action.Description)
			}
			if action.RequiresExternalSync {
				fmt.Fprintln(out, "  external sync: required")
			}
			if action.Command != "" {
				fmt.Fprintf(out, "  command: %s\n", action.Command)
			}
			if action.Path != "" {
				fmt.Fprintf(out, "  path: %s\n", action.Path)
			}
		}
	}
	if status.Mode == state.ModeInvalid {
		return fmt.Errorf("state doctor found errors")
	}
	return nil
}

func writeStateProjectIdentity(out io.Writer, status state.Status) {
	if status.ProjectID != "" {
		fmt.Fprintf(out, "project: %s\n", status.ProjectID)
	}
	if status.ProjectName != "" {
		fmt.Fprintf(out, "project name: %s\n", status.ProjectName)
	}
	if status.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", status.ProjectCurrentPath)
	}
}

func (r Runner) withStateMissingContext(err error, root project.Root) error {
	if err == nil || !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		return err
	}
	databasePath, pathErr := (state.PathResolver{StateHome: r.StateHome}).DatabasePath(root)
	if pathErr != nil {
		return err
	}
	return fmt.Errorf("%v\nscope: global database\ndatabase: %s\nnext: run `loaf state status` to inspect state, or `loaf state migrate markdown --apply` to import local .agents Markdown", err, databasePath)
}

func writeStateDiagnostics(out io.Writer, indent string, diagnostics []state.Diagnostic) {
	for _, diagnostic := range diagnostics {
		fmt.Fprintf(out, "%s\n", formatStateDiagnosticLine(indent, diagnostic))
	}
}

func formatStateDiagnosticLine(indent string, diagnostic state.Diagnostic) string {
	label := diagnostic.Severity
	switch {
	case diagnostic.Category != "" && diagnostic.Policy != "":
		label = fmt.Sprintf("%s [%s/%s]", label, diagnostic.Category, diagnostic.Policy)
	case diagnostic.Category != "":
		label = fmt.Sprintf("%s [%s]", label, diagnostic.Category)
	case diagnostic.Policy != "":
		label = fmt.Sprintf("%s [%s]", label, diagnostic.Policy)
	}
	if diagnostic.RequiresExternalSync {
		label += " [external-sync-required]"
	}
	return fmt.Sprintf("%s%s: %s", indent, label, diagnostic.Message)
}

func stateListWarnings(diagnostics []state.Diagnostic) []state.Diagnostic {
	warnings := []state.Diagnostic{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "warn" || diagnostic.Severity == "error" {
			warnings = append(warnings, diagnostic)
		}
	}
	return warnings
}

func (r Runner) runStateRepair(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return fmt.Errorf("state repair requires a target")
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"legacy-project-database": writeStateRepairLegacyProjectDatabaseHelp,
		"relationship-origin":     writeStateRepairRelationshipOriginHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "legacy-project-database":
		return r.runStateRepairLegacyProjectDatabase(args[1:], out, runtime)
	case "relationship-origin":
		return r.runStateRepairRelationshipOrigin(args[1:], out, runtime)
	default:
		return fmt.Errorf("state repair target %q is not implemented yet", args[0])
	}
}

func writeStateRepairLegacyProjectDatabaseHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state repair legacy-project-database [--dry-run|--apply] [--json]", "Archive migrated legacy per-project SQLite files without deleting them.", "--dry-run    Preview archive paths without writing", "--apply      Move legacy SQLite files into the archive directory", "--json       Output archive plan/result, global database scope, and project identity as JSON")
}

func writeStateRepairRelationshipOriginHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state repair relationship-origin --origin <imported|manual> [--dry-run|--apply] [--json]", "Backfill missing relationship provenance for the current project.", "--origin     Provenance value to set: imported or manual", "--dry-run    Preview affected rows without writing", "--apply      Apply the backfill", "--json       Output repair plan/result, global database scope, and project identity as JSON")
}

func (r Runner) runStateRepairLegacyProjectDatabase(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseLegacyProjectDatabaseRepairArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state repair legacy-project-database", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state repair legacy-project-database", err)
		}
		return err
	}
	result, err := state.ArchiveLegacyProjectDatabase(projectRoot, state.PathResolver{StateHome: r.StateHome}, options.apply)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state repair legacy-project-database", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}

	fmt.Fprintf(out, "loaf state repair legacy-project-database %s\n", repairModeFlag(options.apply))
	fmt.Fprintf(out, "scope: %s database\n", result.DatabaseScope)
	fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	if result.ProjectName != "" {
		fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	}
	if result.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	}
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "legacy database: %s\n", result.LegacyDatabasePath)
	fmt.Fprintf(out, "action: %s\n", result.Action)
	if result.ArchivePath != "" {
		fmt.Fprintf(out, "archive: %s\n", result.ArchivePath)
	}
	fmt.Fprintf(out, "matched files: %d\n", len(result.MatchedPaths))
	fmt.Fprintf(out, "archived files: %d\n", len(result.ArchivedPaths))
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "warn: %s\n", warning)
	}
	if !result.Applied && len(result.MatchedPaths) > 0 {
		fmt.Fprintln(out, "next: rerun with --apply after verifying the global database")
	}
	return nil
}

func (r Runner) runStateRepairRelationshipOrigin(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseRelationshipOriginRepairArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state repair relationship-origin", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state repair relationship-origin", err)
		}
		return err
	}
	result, err := state.RepairMissingRelationshipOrigins(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.RelationshipOriginRepairOptions{
		Origin: options.origin,
		Apply:  options.apply,
	})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state repair relationship-origin", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}

	fmt.Fprintf(out, "loaf state repair relationship-origin %s\n", repairModeFlag(options.apply))
	fmt.Fprintf(out, "scope: %s database\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	if result.BackupPath != "" {
		fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	}
	fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	if result.ProjectName != "" {
		fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	}
	if result.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	}
	fmt.Fprintf(out, "origin: %s\n", result.Origin)
	fmt.Fprintf(out, "matched: %d\n", result.Matched)
	fmt.Fprintf(out, "updated: %d\n", result.Updated)
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	if !result.Applied && result.Matched > 0 {
		fmt.Fprintln(out, "next: rerun with --apply after reviewing the selected origin")
	}
	return nil
}

func repairModeFlag(apply bool) string {
	if apply {
		return "--apply"
	}
	return "--dry-run"
}

func (r Runner) runStateBackup(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) > 0 && args[0] == "verify" {
		return r.runStateBackupVerify(args[1:], out, runtime)
	}
	jsonRequested := hasFlag(args, "--json")
	jsonOutput, err := parseJSONOnly(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state backup", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "state backup", err)
		}
		return err
	}
	result, err := state.Backup(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "state backup", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintln(out, "loaf state backup")
	fmt.Fprintf(out, "scope: %s database\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	fmt.Fprintf(out, "bytes: %d\n", result.Bytes)
	fmt.Fprintf(out, "sha256: %s\n", result.SHA256)
	fmt.Fprintf(out, "verified: %t\n", result.Verified)
	fmt.Fprintf(out, "schema version: %d\n", result.SchemaVersion)
	fmt.Fprintf(out, "projects: %d\n", result.ProjectCount)
	fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	fmt.Fprintf(out, "integrity: %s\n", result.IntegrityCheck)
	fmt.Fprintf(out, "foreign keys: %s\n", result.ForeignKeyCheck)
	fmt.Fprintf(out, "created at: %s\n", result.CreatedAt)
	fmt.Fprintf(out, "next: verify this backup later with `loaf state backup verify %s` before restoring it\n", result.BackupPath)
	return nil
}

func (r Runner) runStateBackupVerify(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseStateBackupVerifyArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state backup verify", err)
		}
		return err
	}
	result, err := state.VerifyBackup(context.Background(), options.path)
	if err != nil {
		if options.jsonOutput {
			return writeStateBackupVerifyJSONError(out, options.path, err)
		}
		return err
	}
	r.addBackupRestoreTargets(&result, runtime)
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintln(out, "loaf state backup verify")
	fmt.Fprintf(out, "scope: %s backup\n", result.DatabaseScope)
	fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	fmt.Fprintf(out, "bytes: %d\n", result.Bytes)
	fmt.Fprintf(out, "sha256: %s\n", result.SHA256)
	fmt.Fprintf(out, "verified: %t\n", result.Verified)
	fmt.Fprintf(out, "schema version: %d\n", result.SchemaVersion)
	fmt.Fprintf(out, "projects: %d\n", result.ProjectCount)
	for _, project := range result.Projects {
		fmt.Fprintf(out, "project: %s\n", project.ID)
		if project.FriendlyName != "" {
			fmt.Fprintf(out, "project name: %s\n", project.FriendlyName)
		}
		if project.CurrentPath != "" {
			fmt.Fprintf(out, "project path: %s\n", project.CurrentPath)
		}
	}
	fmt.Fprintf(out, "integrity: %s\n", result.IntegrityCheck)
	fmt.Fprintf(out, "foreign keys: %s\n", result.ForeignKeyCheck)
	if result.RestoreDatabasePath != "" {
		fmt.Fprintf(out, "restore target: %s\n", result.RestoreDatabasePath)
		fmt.Fprintf(out, "preserve as: %s\n", result.RestorePreservePath)
		fmt.Fprintf(out, "next: if present, preserve current database as %s, copy this verified backup to %s, then run `loaf state doctor` and `loaf state status`\n", result.RestorePreservePath, result.RestoreDatabasePath)
	} else {
		fmt.Fprintln(out, "next: if present, preserve current database, copy this verified backup to `loaf state path`, then run `loaf state doctor` and `loaf state status`")
	}
	return nil
}

func (r Runner) addBackupRestoreTargets(result *state.BackupVerificationResult, runtime state.Runtime) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return
	}
	databasePath, err := (state.PathResolver{StateHome: r.StateHome}).DatabasePath(projectRoot)
	if err != nil {
		return
	}
	result.RestoreDatabasePath = databasePath
	result.RestorePreservePath = databasePath + ".before-restore"
	result.RestoreValidationCommands = []string{"loaf state doctor", "loaf state status"}
}

func (r Runner) runStateExport(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := stateExportJSONRequested(args)
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"all":               writeStateExportAllHelp,
		"release-readiness": writeStateExportReleaseReadinessHelp,
		"spec":              writeStateExportSpecHelp,
		"session":           writeStateExportSessionHelp,
		"triage":            writeStateExportTriageHelp,
	}) {
		return nil
	}
	options, err := parseStateExportArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state export", err)
		}
		return err
	}
	jsonOutput := options.format == state.ExportFormatJSON
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "state export", err)
		}
		return err
	}
	switch {
	case options.kind == state.ExportKindAll && options.format == state.ExportFormatJSON:
		result, err := state.ExportAllJSON(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			if jsonOutput {
				return writeJSONCommandError(out, "state export", err)
			}
			return err
		}
		return writeJSON(out, result)
	case options.kind == state.ExportKindSpec && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportSpecMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref)
		if err != nil {
			return r.withStateMissingContext(err, projectRoot)
		}
		fmt.Fprint(out, result.Content)
		return nil
	case options.kind == state.ExportKindReleaseReadiness && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportReleaseReadinessMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return r.withStateMissingContext(err, projectRoot)
		}
		fmt.Fprint(out, result.Content)
		return nil
	case options.kind == state.ExportKindSession && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportSessionMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref)
		if err != nil {
			return r.withStateMissingContext(err, projectRoot)
		}
		fmt.Fprint(out, result.Content)
		return nil
	case options.kind == state.ExportKindTriage && options.format == state.ExportFormatMarkdown:
		result, err := state.ExportTriageMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return r.withStateMissingContext(err, projectRoot)
		}
		fmt.Fprint(out, result.Content)
		return nil
	default:
		err := fmt.Errorf("state export %s --format %s is not implemented yet", options.kind, options.format)
		if jsonOutput {
			return writeJSONCommandError(out, "state export", err)
		}
		return err
	}
}

func (r Runner) runStateMigrate(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return fmt.Errorf("state migrate requires a source")
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"markdown":     writeStateMigrateMarkdownHelp,
		"storage-home": writeStateMigrateStorageHomeHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "markdown":
		return r.runStateMigrateMarkdown(args[1:], out, runtime)
	case "storage-home":
		return r.runStateMigrateStorageHome(args[1:], out, runtime)
	default:
		return fmt.Errorf("state migrate source %q is not implemented yet", args[0])
	}
}

func writeStateMigrateMarkdownHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate markdown [--dry-run|--apply|--resume] [--json]", "Import .agents Markdown artifacts into SQLite without mutating Markdown.", "--dry-run     Preview import work", "--apply       Apply the import", "--resume      Resume an interrupted import", "--json        Output migration contract, scope, project context, and counts as JSON")
}

func writeStateMigrateStorageHomeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate storage-home [--dry-run|--apply] [--json]", "Copy legacy per-project state into the global XDG data-home SQLite database.", "--dry-run     Preview migration work", "--apply       Apply the migration", "--json        Output migration contract, global database paths, action, and project identity when available")
}

func writeMigrateMarkdownHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate markdown [--dry-run|--apply|--resume] [--json]", "Import .agents Markdown artifacts into SQLite without mutating Markdown.", "--dry-run     Preview import work", "--apply       Apply the import", "--resume      Resume an interrupted import", "--json        Output migration contract, scope, project context, and counts as JSON")
}

func writeMigrateStorageHomeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate storage-home [--dry-run|--apply] [--json]", "Copy legacy per-project state into the global XDG data-home SQLite database.", "--dry-run     Preview migration work", "--apply       Apply the migration", "--json        Output migration contract, global database paths, action, and project identity when available")
}

func (r Runner) runMigrate(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return fmt.Errorf("migrate requires a source")
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"markdown":     writeMigrateMarkdownHelp,
		"storage-home": writeMigrateStorageHomeHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "markdown":
		return r.runMarkdownMigration(args[1:], out, runtime, "loaf migrate markdown")
	case "storage-home":
		return r.runStorageHomeMigration(args[1:], out, runtime, "loaf migrate storage-home")
	case "worktree-storage":
		return r.runMigrateWorktreeStorage(args[1:], out, runtime.RootPath())
	default:
		return fmt.Errorf("migrate source %q is not implemented yet", args[0])
	}
}

func (r Runner) runStateMigrateStorageHome(args []string, out io.Writer, runtime state.Runtime) error {
	return r.runStorageHomeMigration(args, out, runtime, "loaf state migrate storage-home")
}

func (r Runner) runStorageHomeMigration(args []string, out io.Writer, runtime state.Runtime, displayCommand string) error {
	command := strings.TrimPrefix(displayCommand, "loaf ")
	jsonRequested := hasFlag(args, "--json")
	options, err := parseStorageHomeMigrationArgs(args, command)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	var plan state.StorageHomeMigrationPlan
	if options.apply {
		plan, err = state.ApplyStorageHomeMigration(context.Background(), projectRoot, resolver)
	} else {
		plan, err = state.PreviewStorageHomeMigration(projectRoot, resolver)
	}
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, plan)
	}
	writeStorageHomeMigrationHuman(out, displayCommand, plan)
	return nil
}

func (r Runner) runStateMigrateMarkdown(args []string, out io.Writer, runtime state.Runtime) error {
	return r.runMarkdownMigration(args, out, runtime, "loaf state migrate markdown")
}

func (r Runner) runMarkdownMigration(args []string, out io.Writer, runtime state.Runtime, displayCommand string) error {
	command := strings.TrimPrefix(displayCommand, "loaf ")
	jsonRequested := hasFlag(args, "--json")
	options, err := parseMarkdownMigrationArgs(args, command)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	if options.apply || options.resume {
		result, err := state.ApplyMarkdownMigration(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, command, err)
			}
			return err
		}
		if options.resume {
			result.Action = state.MarkdownMigrationActionResume
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		if options.resume {
			writeMarkdownMigrationResultHuman(out, displayCommand+" --resume", result)
		} else {
			writeMarkdownMigrationResultHuman(out, displayCommand+" --apply", result)
		}
		return nil
	}

	plan, err := state.PreviewMarkdownMigration(projectRoot)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	if options.jsonOutput {
		databasePath, err := (state.PathResolver{StateHome: r.StateHome}).DatabasePath(projectRoot)
		if err != nil {
			return writeJSONCommandError(out, command, err)
		}
		return writeJSON(out, state.NewMarkdownMigrationPreviewResult(plan, projectRoot, databasePath))
	}

	databasePath, err := (state.PathResolver{StateHome: r.StateHome}).DatabasePath(projectRoot)
	if err != nil {
		return err
	}
	writeMarkdownMigrationPreviewHuman(out, displayCommand+" --dry-run", projectRoot, databasePath, plan)
	return nil
}

func writeMarkdownMigrationPreviewHuman(out io.Writer, command string, root project.Root, databasePath string, plan state.MarkdownMigrationPlan) {
	fmt.Fprintln(out, command)
	fmt.Fprintln(out, "scope: global database, project import")
	fmt.Fprintf(out, "database: %s\n", databasePath)
	fmt.Fprintln(out, "project: (not initialized)")
	fmt.Fprintf(out, "project name: %s\n", filepath.Base(root.Path()))
	fmt.Fprintf(out, "project path: %s\n", root.Path())
	fmt.Fprintln(out, "applied: false")
	writeMarkdownMigrationPlan(out, plan)
	fmt.Fprintln(out, "next: rerun with --apply to import Markdown into the global database")
}

func writeMarkdownMigrationResultHuman(out io.Writer, command string, result state.MarkdownMigrationResult) {
	fmt.Fprintln(out, command)
	fmt.Fprintf(out, "scope: %s database, %s import\n", result.DatabaseScope, result.ImportScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	fmt.Fprintf(out, "action: %s\n", result.Action)
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	writeMarkdownMigrationPlan(out, result.MarkdownMigrationPlan)
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

func writeStorageHomeMigrationHuman(out io.Writer, displayCommand string, plan state.StorageHomeMigrationPlan) {
	if plan.Applied {
		fmt.Fprintf(out, "%s --apply\n", displayCommand)
	} else {
		fmt.Fprintf(out, "%s --dry-run\n", displayCommand)
	}
	writeStorageHomeMigrationPlan(out, plan)
	if !plan.Applied {
		switch plan.Action {
		case state.StorageHomeActionCopy, state.StorageHomeActionMerge:
			fmt.Fprintln(out, "next: rerun with --apply to copy eligible legacy state into the global database")
		case state.StorageHomeActionAlreadyMigrated:
			fmt.Fprintln(out, "next: no storage-home migration is needed; run `loaf state status` to inspect current state")
		case state.StorageHomeActionNoLegacyState:
			fmt.Fprintln(out, "next: no legacy state was found; run `loaf state init` or `loaf state migrate markdown --apply` if this project still needs SQLite state")
		}
	}
}

func writeStorageHomeMigrationPlan(out io.Writer, plan state.StorageHomeMigrationPlan) {
	fmt.Fprintf(out, "scope: %s database, %s migration\n", plan.DatabaseScope, plan.MigrationScope)
	if plan.ProjectID != "" {
		fmt.Fprintf(out, "project: %s\n", plan.ProjectID)
	} else {
		fmt.Fprintln(out, "project: (not initialized)")
	}
	if plan.ProjectName != "" {
		fmt.Fprintf(out, "project name: %s\n", plan.ProjectName)
	} else if plan.ProjectRoot != "" {
		fmt.Fprintf(out, "project name: %s\n", filepath.Base(plan.ProjectRoot))
	}
	if plan.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", plan.ProjectCurrentPath)
	} else if plan.ProjectRoot != "" {
		fmt.Fprintf(out, "project path: %s\n", plan.ProjectRoot)
	}
	fmt.Fprintf(out, "database: %s\n", plan.DatabasePath)
	fmt.Fprintf(out, "legacy database: %s\n", plan.LegacyDatabasePath)
	fmt.Fprintf(out, "database exists: %t\n", plan.DatabaseExists)
	fmt.Fprintf(out, "legacy database exists: %t\n", plan.LegacyDatabaseExists)
	fmt.Fprintf(out, "action: %s\n", plan.Action)
	fmt.Fprintf(out, "applied: %t\n", plan.Applied)
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
	if isHelpArg(args) {
		writeTraceHelp(out)
		return nil
	}
	jsonRequested := hasFlag(args, "--json")
	ref, jsonOutput, err := parseTraceArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "trace", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "trace", err)
		}
		return err
	}
	result, err := state.Trace(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "trace", err)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}

	fmt.Fprintf(out, "%s %s\n", result.Entity.Kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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

func writeTraceHelp(out io.Writer) {
	writeUsageHelp(out, "loaf trace <entity> [--json]", "Trace relationships for one entity.", "--json       Output traced entity, sources, relationships, global database scope, and project identity as JSON")
}

func (r Runner) runTask(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeTaskHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"create":  writeTaskCreateHelp,
		"list":    writeTaskListHelp,
		"show":    writeTaskShowHelp,
		"status":  writeTaskStatusHelp,
		"update":  writeTaskUpdateHelp,
		"archive": writeTaskArchiveHelp,
		"refresh": writeTaskRefreshHelp,
		"sync":    writeTaskSyncHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "create":
		return r.runTaskCreate(args[1:], out, runtime)
	case "list":
		return r.runTaskList(args[1:], out, runtime)
	case "show":
		return r.runTaskShow(args[1:], out, runtime)
	case "status":
		return r.runTaskStatus(args[1:], out, runtime)
	case "update":
		return r.runTaskUpdate(args[1:], out, runtime)
	case "archive":
		return r.runTaskArchive(args[1:], out, runtime)
	case "refresh":
		return r.runTaskRefresh(args[1:], out, runtime)
	case "sync":
		return r.runTaskSync(args[1:], out, runtime)
	default:
		return unknownSubcommandError("task", args[0])
	}
}

func writeTaskHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf task <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage project tasks in native SQLite state or markdown compatibility mode.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  create   Create a task")
	fmt.Fprintln(out, "  list     List tasks")
	fmt.Fprintln(out, "  show     Show one task")
	fmt.Fprintln(out, "  status   Summarize task and spec statuses")
	fmt.Fprintln(out, "  update   Update task metadata")
	fmt.Fprintln(out, "  archive  Archive tasks")
	fmt.Fprintln(out, "  refresh  Summarize TASKS.json refresh compatibility")
	fmt.Fprintln(out, "  sync     Summarize task sync compatibility")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeTaskCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task create --title <title> [options]", "Create a task.", "--title      Task title", "--spec       Associated spec", "--priority   Task priority: "+validTaskPriorityText(), "--depends-on Comma-separated task refs", "--json       Output created task, event, global database scope, and project identity as JSON")
}

func writeTaskListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task list [--active|--status <status>] [--json]", "List tasks.", "--active     Hide completed tasks", "--status     Filter by status: "+validTaskListStatusText(), "--json       Output tasks, diagnostics, global database scope, and project identity as JSON")
}

func writeTaskShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task show <task> [--json]", "Show one task.", "--json       Output task details, relationships, global database scope, and project identity as JSON")
}

func writeTaskStatusHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task status", "Summarize task and spec statuses.")
}

func writeTaskUpdateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task update <task> [options]", "Update task metadata.", "--status     New task status: "+validTaskStatusText(), "--priority   New task priority: "+validTaskPriorityText(), "--spec       Associated spec", "--depends-on Comma-separated task refs or none", "--session    Session ref or none", "--json       Output updated task, event, global database scope, and project identity as JSON")
}

func writeTaskArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task archive (<task...>|--spec <spec>) [--json]", "Archive done tasks.", "--spec       Archive done tasks for one spec", "--json       Output archive result, archived tasks, global database scope, and project identity as JSON")
}

func writeTaskRefreshHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task refresh [--json]", "Summarize task refresh compatibility.", "--json       Output compatibility mode, action, reason, and counts as JSON")
}

func writeTaskSyncHelp(out io.Writer) {
	writeUsageHelp(out, "loaf task sync [--import|--push] [--json]", "Summarize task sync compatibility.", "--import     Import orphan Markdown tasks", "--push       Push index metadata to Markdown", "--json       Output compatibility mode, action, reason, and counts as JSON")
}

func (r Runner) runTaskRefresh(args []string, out io.Writer, runtime state.Runtime) error {
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		return err
	}
	options, err := parseCompatibilityCommandArgs("task refresh", args, nil)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		summary, err := markdownTaskRefresh(projectRoot.Path())
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, summary)
		}
		writeMarkdownTaskRefreshSummary(out, summary)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
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
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         "task refresh",
		Mode:            "sqlite",
		Action:          "read",
		Reason:          "SQLite state is canonical; Markdown TASKS.json refresh is not run.",
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
	options, err := parseCompatibilityCommandArgs("task sync", args, map[string]bool{"--import": true, "--push": true})
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		summary, err := markdownTaskSync(projectRoot.Path(), args)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, summary)
		}
		writeMarkdownTaskSyncSummary(out, summary)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
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
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         "task sync",
		Mode:            "sqlite",
		Action:          "skipped",
		Reason:          "SQLite state is canonical; Markdown task sync is a compatibility repair path and is not run in SQLite mode.",
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
	jsonRequested := hasFlag(args, "--json")
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "task create", err)
		}
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		options, err := parseTaskCreateArgs(args)
		if err != nil {
			if jsonRequested {
				return writeJSONCommandError(out, "task create", err)
			}
			return err
		}
		result, err := markdownTaskCreate(projectRoot.Path(), options.create)
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "task create", err)
			}
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeTaskCreate(out, result)
		return nil
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonRequested {
			return writeJSONCommandError(out, "task create", err)
		}
		return err
	}

	options, err := parseTaskCreateArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "task create", err)
		}
		return err
	}
	result, err := state.CreateTask(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "task create", err)
		}
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
		result, err := markdownTaskShow(projectRoot.Path(), ref)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeTaskShow(out, result)
		return nil
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
	jsonRequested := hasFlag(args, "--json")
	options, err := parseTaskListArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "task list", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "task list", err)
		}
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "task list", err)
		}
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		tasks, err := markdownTaskList(projectRoot.Path(), options.filters)
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "task list", err)
			}
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, tasks)
		}
		writeTaskList(out, tasks, options.filters)
		return nil
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if options.jsonOutput {
			return writeJSONCommandError(out, "task list", err)
		}
		return err
	}

	tasks, err := state.ListTasks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "task list", err)
		}
		return err
	}
	tasks.Diagnostics = stateListWarnings(status.Diagnostics)
	if options.jsonOutput {
		return writeJSON(out, tasks)
	}
	writeTaskList(out, tasks, options.filters)
	return nil
}

func (r Runner) runTaskStatus(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) > 0 {
		return fmt.Errorf("task status accepts no arguments")
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
		tasks, err := markdownTaskList(projectRoot.Path(), state.TaskListOptions{})
		if err != nil {
			return err
		}
		specs, err := markdownSpecList(projectRoot.Path())
		if err != nil {
			return err
		}
		writeTaskStatus(out, tasks, specs)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	tasks, err := state.ListTasks(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.TaskListOptions{})
	if err != nil {
		return err
	}
	tasks.Diagnostics = stateListWarnings(status.Diagnostics)
	specs, err := state.ListSpecs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	writeTaskStatus(out, tasks, specs)
	return nil
}

func (r Runner) runTaskUpdate(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	projectRoot, mode, err := r.taskStateMode(runtime)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "task update", err)
		}
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		options, err := parseTaskUpdateArgs(args)
		if err != nil {
			if jsonRequested {
				return writeJSONCommandError(out, "task update", err)
			}
			return err
		}
		result, err := markdownTaskUpdate(projectRoot.Path(), options.update)
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "task update", err)
			}
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeTaskUpdate(out, result)
		return nil
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonRequested {
			return writeJSONCommandError(out, "task update", err)
		}
		return err
	}

	options, err := parseTaskUpdateArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "task update", err)
		}
		return err
	}
	result, err := state.UpdateTask(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.update)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "task update", err)
		}
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
		options, err := parseTaskArchiveArgs(args)
		if err != nil {
			return err
		}
		result, err := markdownTaskArchive(projectRoot.Path(), options.archive)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeTaskArchive(out, result)
		return nil
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "status: %s\n", result.Task.Status)
	if result.Priority != "" {
		fmt.Fprintf(out, "priority: %s\n", result.Priority)
	}
	if result.Spec != nil && result.Spec.Alias != "" {
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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

func writeProjectMutationContext(out io.Writer, prefix string, databaseScope string, databasePath string, projectID string, projectName string, projectCurrentPath string) {
	if databaseScope == "" {
		return
	}
	fmt.Fprintf(out, "%sscope: %s database\n", prefix, databaseScope)
	if databasePath != "" {
		fmt.Fprintf(out, "%sdatabase: %s\n", prefix, databasePath)
	}
	if projectID != "" {
		fmt.Fprintf(out, "%sproject: %s\n", prefix, projectID)
	}
	if projectName != "" {
		fmt.Fprintf(out, "%sproject name: %s\n", prefix, projectName)
	}
	if projectCurrentPath != "" {
		fmt.Fprintf(out, "%sproject path: %s\n", prefix, projectCurrentPath)
	}
}

func writeTaskShow(out io.Writer, result state.TaskShow) {
	task := result.Task
	fmt.Fprintf(out, "task %s\n", firstNonEmpty(task.Alias, task.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	fmt.Fprint(out, "\n  loaf task list\n\n")
	writeProjectMutationContext(out, "  ", tasks.DatabaseScope, tasks.DatabasePath, tasks.ProjectID, tasks.ProjectName, tasks.ProjectCurrentPath)
	writeStateDiagnostics(out, "  ", tasks.Diagnostics)
	if taskListHasContext(tasks) {
		fmt.Fprintln(out)
	}

	if len(tasks.Tasks) == 0 {
		fmt.Fprint(out, "  No tasks found.\n\n")
		return
	}

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

func taskListHasContext(tasks state.TaskList) bool {
	return tasks.DatabaseScope != "" ||
		tasks.DatabasePath != "" ||
		tasks.ProjectID != "" ||
		tasks.ProjectName != "" ||
		tasks.ProjectCurrentPath != "" ||
		len(tasks.Diagnostics) > 0
}

func markdownTaskList(rootPath string, options state.TaskListOptions) (state.TaskList, error) {
	files, err := filepath.Glob(filepath.Join(rootPath, ".agents", "tasks", "*.md"))
	if err != nil {
		return state.TaskList{}, fmt.Errorf("find markdown tasks: %w", err)
	}
	sort.Strings(files)
	index := loadMarkdownTaskIndex(rootPath)
	tasks := state.TaskList{Version: 1, Tasks: map[string]state.TaskItem{}}
	for _, path := range files {
		item, alias, err := readMarkdownTask(rootPath, path, index)
		if err != nil {
			return state.TaskList{}, err
		}
		if !taskMatchesFilters(item, options) {
			continue
		}
		tasks.Tasks[alias] = item
	}
	return tasks, nil
}

func markdownTaskShow(rootPath string, ref string) (state.TaskShow, error) {
	files, err := filepath.Glob(filepath.Join(rootPath, ".agents", "tasks", "*.md"))
	if err != nil {
		return state.TaskShow{}, fmt.Errorf("find markdown tasks: %w", err)
	}
	sort.Strings(files)
	index := loadMarkdownTaskIndex(rootPath)
	for _, path := range files {
		item, alias, err := readMarkdownTask(rootPath, path, index)
		if err != nil {
			return state.TaskShow{}, err
		}
		if alias != ref {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return state.TaskShow{}, fmt.Errorf("read markdown task %s: %w", path, err)
		}
		frontmatter, _ := parseKnowledgeFrontmatter(body)
		field := func(keys ...string) string {
			for _, key := range keys {
				if value := firstFieldValue(frontmatter[key]); value != "" {
					return value
				}
			}
			return ""
		}
		hash := sha256.Sum256(body)
		meta := index[alias]
		return state.TaskShow{
			Query: ref,
			Task: state.TaskDetail{
				ID:        alias,
				Alias:     alias,
				Title:     item.Title,
				Status:    item.Status,
				Priority:  item.Priority,
				Spec:      item.Spec,
				DependsOn: item.DependsOn,
				Sessions:  markdownTaskSessions(meta, field("session", "sessions")),
				Sources: []state.TraceSource{{
					Path: item.SourcePath,
					Hash: hex.EncodeToString(hash[:]),
				}},
				Body:      strings.TrimSpace(markdownContentWithoutFrontmatter(string(body))),
				CreatedAt: firstNonEmpty(meta.Created, field("created", "created_at")),
				UpdatedAt: firstNonEmpty(meta.Updated, field("updated", "updated_at"), meta.Created, field("created", "created_at")),
			},
		}, nil
	}
	return state.TaskShow{}, fmt.Errorf("task %q not found in markdown tasks", ref)
}

func markdownTaskCreate(rootPath string, options state.TaskCreateOptions) (state.TaskCreateResult, error) {
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return state.TaskCreateResult{}, fmt.Errorf("task create requires --title")
	}
	priority := strings.TrimSpace(options.Priority)
	if priority == "" {
		priority = "P2"
	}
	if !state.ValidTaskPriority(priority) {
		return state.TaskCreateResult{}, fmt.Errorf("invalid priority %q (valid: %s)", priority, validTaskPriorityText())
	}
	agentsDir := filepath.Join(rootPath, ".agents")
	tasksDir := filepath.Join(agentsDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return state.TaskCreateResult{}, fmt.Errorf("create tasks directory: %w", err)
	}
	indexPath := filepath.Join(agentsDir, "TASKS.json")
	index, err := loadMarkdownTaskIndexObject(indexPath)
	if err != nil {
		return state.TaskCreateResult{}, err
	}
	tasks, err := markdownIndexObject(index, "tasks")
	if err != nil {
		return state.TaskCreateResult{}, err
	}
	specs, err := markdownIndexObject(index, "specs")
	if err != nil {
		return state.TaskCreateResult{}, err
	}

	var specEntity state.TraceEntity
	specRef := strings.TrimSpace(options.Spec)
	if specRef != "" {
		specValue, ok := specs[specRef]
		if !ok {
			return state.TaskCreateResult{}, fmt.Errorf("Spec %q not found in index", specRef)
		}
		specEntity = state.TraceEntity{Kind: "spec", ID: specRef, Alias: specRef}
		if specEntry, ok := specValue.(map[string]any); ok {
			specEntity.Title = jsonObjectString(specEntry, "title")
			specEntity.Status = jsonObjectString(specEntry, "status")
		}
	}

	dependencies := []state.TraceEntity{}
	dependencyIDs := []string{}
	for _, depRef := range options.DependsOn {
		depRef = strings.TrimSpace(depRef)
		if depRef == "" {
			continue
		}
		taskValue, ok := tasks[depRef]
		if !ok {
			return state.TaskCreateResult{}, fmt.Errorf("Dependency %q not found in index", depRef)
		}
		dependencyIDs = append(dependencyIDs, depRef)
		dependency := state.TraceEntity{Kind: "task", ID: depRef, Alias: depRef}
		if taskEntry, ok := taskValue.(map[string]any); ok {
			dependency.Title = jsonObjectString(taskEntry, "title")
			dependency.Status = jsonObjectString(taskEntry, "status")
		}
		dependencies = append(dependencies, dependency)
	}

	nextID := markdownTaskNextID(index)
	taskID := fmt.Sprintf("TASK-%03d", nextID)
	slug := markdownTaskSlug(title)
	if slug == "" {
		slug = strings.ToLower(taskID)
	}
	file := fmt.Sprintf("%s-%s.md", taskID, slug)
	taskPath := filepath.Join(tasksDir, file)
	if _, err := os.Stat(taskPath); err == nil {
		return state.TaskCreateResult{}, fmt.Errorf("task file already exists: .agents/tasks/%s", file)
	} else if err != nil && !os.IsNotExist(err) {
		return state.TaskCreateResult{}, fmt.Errorf("inspect task file %s: %w", file, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry := map[string]any{
		"title":        title,
		"slug":         slug,
		"spec":         nil,
		"status":       "todo",
		"priority":     priority,
		"depends_on":   dependencyIDs,
		"files":        []string{},
		"verify":       nil,
		"done":         nil,
		"session":      nil,
		"created":      now,
		"updated":      now,
		"completed_at": nil,
		"file":         file,
	}
	if specRef != "" {
		entry["spec"] = specRef
	}
	frontmatter := map[string]frontmatterField{
		"id":       markdownReportFrontmatterScalar(taskID),
		"title":    markdownReportFrontmatterScalar(title),
		"status":   markdownReportFrontmatterScalar("todo"),
		"priority": markdownReportFrontmatterScalar(priority),
		"created":  markdownReportFrontmatterScalar(now),
		"updated":  markdownReportFrontmatterScalar(now),
	}
	if specRef != "" {
		frontmatter["spec"] = markdownReportFrontmatterScalar(specRef)
	}
	if len(dependencyIDs) > 0 {
		frontmatter["depends_on"] = frontmatterField{Values: dependencyIDs, Array: true, Set: true}
	}
	if err := os.WriteFile(taskPath, []byte(renderMarkdownTask(frontmatter, markdownTaskBody(taskID, title))), 0o600); err != nil {
		return state.TaskCreateResult{}, fmt.Errorf("write task %s: %w", taskID, err)
	}

	tasks[taskID] = entry
	index["next_id"] = float64(nextID + 1)
	updated, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return state.TaskCreateResult{}, fmt.Errorf("encode TASKS.json: %w", err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(indexPath, updated, 0o600); err != nil {
		return state.TaskCreateResult{}, fmt.Errorf("write TASKS.json: %w", err)
	}

	result := state.TaskCreateResult{
		ContractVersion: state.StateJSONContractVersion,
		Task:            state.TraceEntity{Kind: "task", ID: taskID, Alias: taskID, Title: title, Status: "todo"},
		Priority:        priority,
		Depends:         dependencies,
	}
	if specRef != "" {
		result.Spec = &specEntity
	}
	return result, nil
}

func markdownTaskUpdate(rootPath string, options state.TaskUpdateOptions) (state.TaskStatusUpdateResult, error) {
	if !options.SetStatus && !options.SetPriority && !options.SetSpec && !options.SetDependsOn && !options.SetSession {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("task update requires at least one update")
	}
	if options.SetStatus && !state.ValidTaskStatus(options.Status) {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("invalid status %q", options.Status)
	}
	if options.SetPriority && !state.ValidTaskPriority(options.Priority) {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("invalid priority %q (valid: %s)", options.Priority, validTaskPriorityText())
	}
	indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
	index, err := loadMarkdownTaskIndexObject(indexPath)
	if err != nil {
		return state.TaskStatusUpdateResult{}, err
	}
	tasks, err := markdownIndexObject(index, "tasks")
	if err != nil {
		return state.TaskStatusUpdateResult{}, err
	}
	specs, err := markdownIndexObject(index, "specs")
	if err != nil {
		return state.TaskStatusUpdateResult{}, err
	}
	entryValue, ok := tasks[options.Ref]
	if !ok {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("%s not found in index", options.Ref)
	}
	entry, ok := entryValue.(map[string]any)
	if !ok {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("TASKS.json entry for %s must be an object", options.Ref)
	}

	previousStatus := jsonObjectString(entry, "status")
	if previousStatus == "" {
		previousStatus = "todo"
	}
	finalStatus := previousStatus
	now := time.Now().UTC().Format(time.RFC3339)
	if options.SetStatus {
		finalStatus = options.Status
		if finalStatus == "done" && previousStatus != "done" {
			entry["completed_at"] = now
		} else if finalStatus != "done" && previousStatus == "done" {
			entry["completed_at"] = nil
		}
		entry["status"] = finalStatus
	}
	finalPriority := jsonObjectString(entry, "priority")
	if options.SetPriority {
		finalPriority = options.Priority
		entry["priority"] = finalPriority
	}

	var specEntity *state.TraceEntity
	if options.SetSpec {
		if isMarkdownNoneValue(options.Spec) {
			entry["spec"] = nil
		} else {
			specValue, ok := specs[options.Spec]
			if !ok {
				return state.TaskStatusUpdateResult{}, fmt.Errorf("Unknown spec %q. Use `loaf spec list` to see valid IDs.", options.Spec)
			}
			entry["spec"] = options.Spec
			spec := state.TraceEntity{Kind: "spec", ID: options.Spec, Alias: options.Spec}
			if specEntry, ok := specValue.(map[string]any); ok {
				spec.Title = jsonObjectString(specEntry, "title")
				spec.Status = jsonObjectString(specEntry, "status")
			}
			specEntity = &spec
		}
	} else if specRef := jsonObjectString(entry, "spec"); specRef != "" {
		spec := state.TraceEntity{Kind: "spec", ID: specRef, Alias: specRef}
		if specEntry, ok := specs[specRef].(map[string]any); ok {
			spec.Title = jsonObjectString(specEntry, "title")
			spec.Status = jsonObjectString(specEntry, "status")
		}
		specEntity = &spec
	}

	dependencies := []state.TraceEntity{}
	if options.SetDependsOn {
		dependencyIDs := []string{}
		for _, depRef := range options.DependsOn {
			depRef = strings.TrimSpace(depRef)
			if depRef == "" || isMarkdownNoneValue(depRef) {
				continue
			}
			taskValue, ok := tasks[depRef]
			if !ok {
				return state.TaskStatusUpdateResult{}, fmt.Errorf("Unknown task ID %q in --depends-on", depRef)
			}
			dependencyIDs = append(dependencyIDs, depRef)
			dependency := state.TraceEntity{Kind: "task", ID: depRef, Alias: depRef}
			if taskEntry, ok := taskValue.(map[string]any); ok {
				dependency.Title = jsonObjectString(taskEntry, "title")
				dependency.Status = jsonObjectString(taskEntry, "status")
			}
			dependencies = append(dependencies, dependency)
		}
		entry["depends_on"] = dependencyIDs
	}

	var sessionEntity *state.TraceEntity
	if options.SetSession {
		if isMarkdownNoneValue(options.Session) {
			entry["session"] = nil
		} else {
			entry["session"] = options.Session
			sessionEntity = &state.TraceEntity{Kind: "session", ID: options.Session, Alias: options.Session}
		}
	} else if sessionRef := jsonObjectString(entry, "session"); sessionRef != "" {
		sessionEntity = &state.TraceEntity{Kind: "session", ID: sessionRef, Alias: sessionRef}
	}

	entry["updated"] = now
	if err := syncMarkdownTaskFrontmatter(rootPath, options.Ref, entry); err != nil {
		return state.TaskStatusUpdateResult{}, err
	}
	updated, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("encode TASKS.json: %w", err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(indexPath, updated, 0o600); err != nil {
		return state.TaskStatusUpdateResult{}, fmt.Errorf("write TASKS.json: %w", err)
	}

	result := state.TaskStatusUpdateResult{
		ContractVersion: state.StateJSONContractVersion,
		Task:            state.TraceEntity{Kind: "task", ID: options.Ref, Alias: options.Ref, Title: jsonObjectString(entry, "title"), Status: finalStatus},
		Previous:        previousStatus,
		Status:          finalStatus,
		Priority:        finalPriority,
		Spec:            specEntity,
		Session:         sessionEntity,
	}
	if options.SetDependsOn {
		result.Depends = dependencies
	}
	return result, nil
}

func syncMarkdownTaskFrontmatter(rootPath string, ref string, entry map[string]any) error {
	file := jsonObjectString(entry, "file")
	if file == "" {
		file = markdownTaskFileForAlias(rootPath, ref)
	}
	if file == "" {
		return nil
	}
	path := filepath.Join(rootPath, ".agents", "tasks", filepath.FromSlash(file))
	body := ""
	if content, err := os.ReadFile(path); err == nil {
		body = markdownContentWithoutFrontmatter(string(content))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read markdown task %s: %w", file, err)
	}
	frontmatter := map[string]frontmatterField{
		"id":       markdownReportFrontmatterScalar(ref),
		"title":    markdownReportFrontmatterScalar(jsonObjectString(entry, "title")),
		"status":   markdownReportFrontmatterScalar(firstNonEmpty(jsonObjectString(entry, "status"), "todo")),
		"priority": markdownReportFrontmatterScalar(jsonObjectString(entry, "priority")),
		"created":  markdownReportFrontmatterScalar(jsonObjectString(entry, "created")),
		"updated":  markdownReportFrontmatterScalar(jsonObjectString(entry, "updated")),
	}
	if spec := jsonObjectString(entry, "spec"); spec != "" {
		frontmatter["spec"] = markdownReportFrontmatterScalar(spec)
	}
	if deps := jsonStringSlice(entry["depends_on"]); len(deps) > 0 {
		frontmatter["depends_on"] = frontmatterField{Values: deps, Array: true, Set: true}
	}
	if session := jsonObjectString(entry, "session"); session != "" {
		frontmatter["session"] = markdownReportFrontmatterScalar(session)
	}
	if body == "" {
		body = markdownTaskBody(ref, jsonObjectString(entry, "title"))
	}
	if err := os.WriteFile(path, []byte(renderMarkdownTask(frontmatter, body)), 0o600); err != nil {
		return fmt.Errorf("write markdown task %s: %w", file, err)
	}
	return nil
}

func markdownTaskRefresh(rootPath string) (compatibilityCommandSummary, error) {
	indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
	snapshotBefore, err := loadMarkdownTaskIndexObject(indexPath)
	if err != nil {
		snapshotBefore = emptyMarkdownTaskIndexObject()
	}
	scanStartNextID := markdownTaskNextID(snapshotBefore)
	scanStartSpecs, err := markdownIndexObject(snapshotBefore, "specs")
	if err != nil {
		scanStartSpecs = map[string]any{}
	}

	index, counts, err := buildMarkdownTaskIndexFromFiles(rootPath)
	if err != nil {
		return compatibilityCommandSummary{}, err
	}
	nowIndex, err := loadMarkdownTaskIndexObject(indexPath)
	if err != nil {
		nowIndex = emptyMarkdownTaskIndexObject()
	}
	index = mergeMarkdownTaskRefreshIndex(index, nowIndex, scanStartNextID, scanStartSpecs)
	counts = markdownTaskIndexCounts(index, 0, 0)
	if err := writeMarkdownTaskIndex(rootPath, index); err != nil {
		return compatibilityCommandSummary{}, err
	}
	return compatibilityCommandSummary{
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         "task refresh",
		Mode:            "markdown",
		Action:          "rebuild",
		Reason:          "Rebuilt TASKS.json from task/spec markdown files.",
		Counts:          counts,
	}, nil
}

func emptyMarkdownTaskIndexObject() map[string]any {
	return map[string]any{
		"version": float64(1),
		"next_id": float64(1),
		"tasks":   map[string]any{},
		"specs":   map[string]any{},
	}
}

func mergeMarkdownTaskRefreshIndex(scannedIndex map[string]any, nowIndex map[string]any, scanStartNextID int, scanStartSpecs map[string]any) map[string]any {
	scannedTasks, _ := markdownIndexObject(scannedIndex, "tasks")
	nowTasks, err := markdownIndexObject(nowIndex, "tasks")
	if err == nil {
		for id, entry := range nowTasks {
			if _, exists := scannedTasks[id]; exists {
				continue
			}
			if n := numericID(id); n >= scanStartNextID && n > 0 {
				scannedTasks[id] = entry
			}
		}
	}

	scannedSpecs, _ := markdownIndexObject(scannedIndex, "specs")
	nowSpecs, err := markdownIndexObject(nowIndex, "specs")
	if err == nil {
		for id, entry := range nowSpecs {
			if _, exists := scannedSpecs[id]; exists {
				continue
			}
			if _, existedAtScanStart := scanStartSpecs[id]; !existedAtScanStart {
				scannedSpecs[id] = entry
			}
		}
	}

	nextID := max(markdownTaskNextID(scannedIndex), markdownTaskNextID(nowIndex))
	for id := range scannedTasks {
		if n := numericID(id); n+1 > nextID {
			nextID = n + 1
		}
	}
	scannedIndex["version"] = float64(1)
	scannedIndex["next_id"] = float64(nextID)
	scannedIndex["tasks"] = scannedTasks
	scannedIndex["specs"] = scannedSpecs
	return scannedIndex
}

func markdownTaskSync(rootPath string, args []string) (compatibilityCommandSummary, error) {
	switch {
	case hasFlag(args, "--push"):
		indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
		index, err := loadMarkdownTaskIndexObject(indexPath)
		if err != nil {
			return compatibilityCommandSummary{}, err
		}
		counts, err := syncMarkdownFrontmatterFromIndex(rootPath, index)
		if err != nil {
			return compatibilityCommandSummary{}, err
		}
		return compatibilityCommandSummary{
			ContractVersion: state.StateJSONContractVersion,
			Version:         1,
			Command:         "task sync",
			Mode:            "markdown",
			Action:          "push",
			Reason:          "Pushed TASKS.json metadata into task/spec markdown frontmatter.",
			Counts:          counts,
		}, nil
	case hasFlag(args, "--import"):
		return importMarkdownTaskIndexOrphans(rootPath)
	default:
		summary, err := markdownTaskRefresh(rootPath)
		if err != nil {
			return compatibilityCommandSummary{}, err
		}
		summary.Command = "task sync"
		summary.Reason = "Rebuilt TASKS.json from task/spec markdown files."
		return summary, nil
	}
}

func writeMarkdownTaskRefreshSummary(out io.Writer, summary compatibilityCommandSummary) {
	fmt.Fprint(out, "\n  loaf task refresh\n\n")
	fmt.Fprintln(out, "  ✓ Rebuilt TASKS.json from .md files")
	fmt.Fprintf(out, "    Tasks: %d (%s)\n", summary.Counts["tasks"], markdownTaskStatusCountText(summary.Counts))
	fmt.Fprintf(out, "    Specs: %d\n\n", summary.Counts["specs"])
}

func writeMarkdownTaskSyncSummary(out io.Writer, summary compatibilityCommandSummary) {
	fmt.Fprint(out, "\n  loaf task sync\n\n")
	switch summary.Action {
	case "push":
		fmt.Fprintln(out, "  ✓ Pushed TASKS.json metadata to .md frontmatter")
		fmt.Fprintf(out, "    Tasks: %d, Specs: %d\n\n", summary.Counts["tasks"], summary.Counts["specs"])
	case "import":
		if summary.Counts["imported_tasks"] == 0 && summary.Counts["imported_specs"] == 0 {
			fmt.Fprintln(out, "  No orphan files found.")
			fmt.Fprintln(out)
			return
		}
		parts := []string{}
		if count := summary.Counts["imported_tasks"]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d task(s)", count))
		}
		if count := summary.Counts["imported_specs"]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d spec(s)", count))
		}
		fmt.Fprintf(out, "  ✓ Imported %s into TASKS.json\n\n", strings.Join(parts, " and "))
	default:
		fmt.Fprintln(out, "  ✓ Rebuilt TASKS.json from .md files")
		fmt.Fprintf(out, "    Tasks: %d (%s)\n", summary.Counts["tasks"], markdownTaskStatusCountText(summary.Counts))
		fmt.Fprintf(out, "    Specs: %d\n\n", summary.Counts["specs"])
	}
}

func markdownTaskStatusCountText(counts map[string]int) string {
	parts := make([]string, 0, 5)
	for _, status := range []string{"in_progress", "blocked", "todo", "review", "done"} {
		parts = append(parts, fmt.Sprintf("%d %s", counts["tasks_"+status], status))
	}
	return strings.Join(parts, ", ")
}

func buildMarkdownTaskIndexFromFiles(rootPath string) (map[string]any, map[string]int, error) {
	agentsDir := filepath.Join(rootPath, ".agents")
	tasksDir := filepath.Join(agentsDir, "tasks")
	specsDir := filepath.Join(agentsDir, "specs")
	tasks := map[string]any{}
	specs := map[string]any{}
	maxTaskID := 0

	taskFiles, err := collectMarkdownIndexFiles(tasksDir, filepath.Join(tasksDir, "archive"), "TASK-")
	if err != nil {
		return nil, nil, err
	}
	for _, path := range taskFiles {
		id, entry, err := parseMarkdownTaskIndexFile(tasksDir, path)
		if err != nil {
			return nil, nil, err
		}
		if id == "" {
			continue
		}
		tasks[id] = entry
		if n := numericID(id); n > maxTaskID {
			maxTaskID = n
		}
	}

	specFiles, err := collectMarkdownIndexFiles(specsDir, filepath.Join(specsDir, "archive"), "SPEC-")
	if err != nil {
		return nil, nil, err
	}
	for _, path := range specFiles {
		id, entry, err := parseMarkdownSpecIndexFile(specsDir, path)
		if err != nil {
			return nil, nil, err
		}
		if id == "" {
			continue
		}
		specs[id] = entry
	}

	index := map[string]any{
		"version": float64(1),
		"next_id": float64(maxTaskID + 1),
		"tasks":   tasks,
		"specs":   specs,
	}
	return index, markdownTaskIndexCounts(index, 0, 0), nil
}

func collectMarkdownIndexFiles(activeDir string, archiveDir string, prefix string) ([]string, error) {
	files := []string{}
	active, err := filepath.Glob(filepath.Join(activeDir, prefix+"*.md"))
	if err != nil {
		return nil, fmt.Errorf("find markdown files in %s: %w", activeDir, err)
	}
	files = append(files, active...)
	if _, err := os.Stat(archiveDir); err == nil {
		err = filepath.WalkDir(archiveDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || filepath.Ext(entry.Name()) != ".md" {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("find archived markdown files in %s: %w", archiveDir, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect archive directory %s: %w", archiveDir, err)
	}
	sort.Strings(files)
	return files, nil
}

func parseMarkdownTaskIndexFile(baseDir string, path string) (string, map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read markdown task %s: %w", path, err)
	}
	frontmatter, _ := parseKnowledgeFrontmatter(content)
	id := firstNonEmpty(firstFieldValue(frontmatter["id"]), taskAliasFromPath(path))
	if id == "" {
		return "", nil, nil
	}
	status := normalizeMarkdownTaskStatus(firstFieldValue(frontmatter["status"]))
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return "", nil, fmt.Errorf("resolve markdown task path %s: %w", path, err)
	}
	entry := map[string]any{
		"title":        firstNonEmpty(firstFieldValue(frontmatter["title"]), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))),
		"slug":         markdownTaskSlugFromPath(path, id),
		"spec":         nil,
		"status":       status,
		"priority":     normalizeMarkdownTaskPriority(firstFieldValue(frontmatter["priority"])),
		"depends_on":   frontmatterStringSlice(frontmatter["depends_on"]),
		"files":        frontmatterStringSlice(frontmatter["files"]),
		"verify":       nil,
		"done":         nil,
		"session":      nil,
		"created":      normalizeMarkdownIndexDate(firstFieldValue(frontmatter["created"])),
		"updated":      normalizeMarkdownIndexDate(firstNonEmpty(firstFieldValue(frontmatter["updated"]), firstFieldValue(frontmatter["created"]))),
		"completed_at": nil,
		"file":         filepath.ToSlash(rel),
	}
	if spec := firstFieldValue(frontmatter["spec"]); spec != "" {
		entry["spec"] = spec
	}
	if verify := firstFieldValue(frontmatter["verify"]); verify != "" {
		entry["verify"] = verify
	}
	if done := firstFieldValue(frontmatter["done"]); done != "" {
		entry["done"] = done
	}
	if session := firstFieldValue(frontmatter["session"]); session != "" {
		entry["session"] = session
	}
	if status == "done" {
		entry["completed_at"] = normalizeMarkdownIndexDate(firstNonEmpty(firstFieldValue(frontmatter["completed_at"]), firstFieldValue(frontmatter["updated"]), firstFieldValue(frontmatter["created"])))
	}
	return id, entry, nil
}

func parseMarkdownSpecIndexFile(baseDir string, path string) (string, map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read markdown spec %s: %w", path, err)
	}
	frontmatter, _ := parseKnowledgeFrontmatter(content)
	id := firstNonEmpty(firstFieldValue(frontmatter["id"]), specAliasFromPath(path))
	if id == "" {
		return "", nil, nil
	}
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return "", nil, fmt.Errorf("resolve markdown spec path %s: %w", path, err)
	}
	entry := map[string]any{
		"title":       firstNonEmpty(firstFieldValue(frontmatter["title"]), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))),
		"status":      normalizeMarkdownSpecStatus(firstFieldValue(frontmatter["status"])),
		"requirement": nil,
		"source":      nil,
		"created":     normalizeMarkdownIndexDate(firstFieldValue(frontmatter["created"])),
		"file":        filepath.ToSlash(rel),
	}
	if requirement := firstFieldValue(frontmatter["requirement"]); requirement != "" {
		entry["requirement"] = requirement
	}
	if source := firstFieldValue(frontmatter["source"]); source != "" {
		entry["source"] = source
	}
	return id, entry, nil
}

func importMarkdownTaskIndexOrphans(rootPath string) (compatibilityCommandSummary, error) {
	indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
	index, err := loadMarkdownTaskIndexObject(indexPath)
	if err != nil {
		return compatibilityCommandSummary{}, err
	}
	tasks, err := markdownIndexObject(index, "tasks")
	if err != nil {
		return compatibilityCommandSummary{}, err
	}
	specs, err := markdownIndexObject(index, "specs")
	if err != nil {
		return compatibilityCommandSummary{}, err
	}

	rebuild, _, err := buildMarkdownTaskIndexFromFiles(rootPath)
	if err != nil {
		return compatibilityCommandSummary{}, err
	}
	rebuildTasks, err := markdownIndexObject(rebuild, "tasks")
	if err != nil {
		return compatibilityCommandSummary{}, err
	}
	rebuildSpecs, err := markdownIndexObject(rebuild, "specs")
	if err != nil {
		return compatibilityCommandSummary{}, err
	}

	importedTasks := 0
	maxTaskID := markdownTaskNextID(index) - 1
	for id, entry := range rebuildTasks {
		if _, ok := tasks[id]; ok {
			continue
		}
		tasks[id] = entry
		importedTasks++
		if n := numericID(id); n > maxTaskID {
			maxTaskID = n
		}
	}
	importedSpecs := 0
	for id, entry := range rebuildSpecs {
		if _, ok := specs[id]; ok {
			continue
		}
		specs[id] = entry
		importedSpecs++
	}
	if maxTaskID >= markdownTaskNextID(index) {
		index["next_id"] = float64(maxTaskID + 1)
	}
	if importedTasks > 0 || importedSpecs > 0 {
		if err := writeMarkdownTaskIndex(rootPath, index); err != nil {
			return compatibilityCommandSummary{}, err
		}
	}
	return compatibilityCommandSummary{
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         "task sync",
		Mode:            "markdown",
		Action:          "import",
		Reason:          "Imported orphan task/spec markdown files into TASKS.json.",
		Counts:          markdownTaskIndexCounts(index, importedTasks, importedSpecs),
	}, nil
}

func syncMarkdownFrontmatterFromIndex(rootPath string, index map[string]any) (map[string]int, error) {
	tasks, err := markdownIndexObject(index, "tasks")
	if err != nil {
		return nil, err
	}
	for id, value := range tasks {
		entry, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("TASKS.json entry for %s must be an object", id)
		}
		if err := syncMarkdownTaskIndexFrontmatter(rootPath, id, entry); err != nil {
			return nil, err
		}
	}
	specs, err := markdownIndexObject(index, "specs")
	if err != nil {
		return nil, err
	}
	for id, value := range specs {
		entry, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("TASKS.json entry for %s must be an object", id)
		}
		if err := syncMarkdownSpecIndexFrontmatter(rootPath, id, entry); err != nil {
			return nil, err
		}
	}
	return markdownTaskIndexCounts(index, 0, 0), nil
}

func syncMarkdownTaskIndexFrontmatter(rootPath string, ref string, entry map[string]any) error {
	file := firstNonEmpty(jsonObjectString(entry, "file"), markdownTaskFileForAlias(rootPath, ref))
	if file == "" {
		return nil
	}
	path := filepath.Join(rootPath, ".agents", "tasks", filepath.FromSlash(file))
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read markdown task %s: %w", file, err)
	}
	existing, _ := parseKnowledgeFrontmatter(content)
	frontmatter := mergeMarkdownIndexFrontmatter(existing, markdownTaskEntryFrontmatter(ref, entry), markdownTaskFrontmatterKeys())
	body := markdownContentWithoutFrontmatter(string(content))
	if err := os.WriteFile(path, []byte(renderMarkdownDocument(frontmatter, markdownTaskFrontmatterOrder(), body)), 0o600); err != nil {
		return fmt.Errorf("write markdown task %s: %w", file, err)
	}
	return nil
}

func syncMarkdownSpecIndexFrontmatter(rootPath string, ref string, entry map[string]any) error {
	file := firstNonEmpty(jsonObjectString(entry, "file"), markdownSpecFileForAlias(rootPath, ref))
	if file == "" {
		return nil
	}
	path := filepath.Join(rootPath, ".agents", "specs", filepath.FromSlash(file))
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read markdown spec %s: %w", file, err)
	}
	existing, _ := parseKnowledgeFrontmatter(content)
	frontmatter := mergeMarkdownIndexFrontmatter(existing, markdownSpecEntryFrontmatter(ref, entry), markdownSpecFrontmatterKeys())
	body := markdownContentWithoutFrontmatter(string(content))
	if err := os.WriteFile(path, []byte(renderMarkdownDocument(frontmatter, markdownSpecFrontmatterOrder(), body)), 0o600); err != nil {
		return fmt.Errorf("write markdown spec %s: %w", file, err)
	}
	return nil
}

func markdownTaskEntryFrontmatter(ref string, entry map[string]any) map[string]frontmatterField {
	frontmatter := map[string]frontmatterField{
		"id":       markdownReportFrontmatterScalar(ref),
		"title":    markdownReportFrontmatterScalar(jsonObjectString(entry, "title")),
		"status":   markdownReportFrontmatterScalar(firstNonEmpty(jsonObjectString(entry, "status"), "todo")),
		"priority": markdownReportFrontmatterScalar(jsonObjectString(entry, "priority")),
	}
	if spec := jsonObjectString(entry, "spec"); spec != "" {
		frontmatter["spec"] = markdownReportFrontmatterScalar(spec)
	}
	if created := jsonObjectString(entry, "created"); created != "" {
		frontmatter["created"] = markdownReportFrontmatterScalar(created)
	}
	if updated := jsonObjectString(entry, "updated"); updated != "" {
		frontmatter["updated"] = markdownReportFrontmatterScalar(updated)
	}
	if deps := jsonStringSlice(entry["depends_on"]); len(deps) > 0 {
		frontmatter["depends_on"] = frontmatterField{Values: deps, Array: true, Set: true}
	}
	if files := jsonStringSlice(entry["files"]); len(files) > 0 {
		frontmatter["files"] = frontmatterField{Values: files, Array: true, Set: true}
	}
	if verify := jsonObjectString(entry, "verify"); verify != "" {
		frontmatter["verify"] = markdownReportFrontmatterScalar(verify)
	}
	if done := jsonObjectString(entry, "done"); done != "" {
		frontmatter["done"] = markdownReportFrontmatterScalar(done)
	}
	if session := jsonObjectString(entry, "session"); session != "" {
		frontmatter["session"] = markdownReportFrontmatterScalar(session)
	}
	if completedAt := jsonObjectString(entry, "completed_at"); completedAt != "" {
		frontmatter["completed_at"] = markdownReportFrontmatterScalar(completedAt)
	}
	return frontmatter
}

func markdownSpecEntryFrontmatter(ref string, entry map[string]any) map[string]frontmatterField {
	frontmatter := map[string]frontmatterField{
		"id":     markdownReportFrontmatterScalar(ref),
		"title":  markdownReportFrontmatterScalar(jsonObjectString(entry, "title")),
		"status": markdownReportFrontmatterScalar(firstNonEmpty(jsonObjectString(entry, "status"), "drafting")),
	}
	if source := jsonObjectString(entry, "source"); source != "" {
		frontmatter["source"] = markdownReportFrontmatterScalar(source)
	}
	if created := jsonObjectString(entry, "created"); created != "" {
		frontmatter["created"] = markdownReportFrontmatterScalar(created)
	}
	if requirement := jsonObjectString(entry, "requirement"); requirement != "" {
		frontmatter["requirement"] = markdownReportFrontmatterScalar(requirement)
	}
	return frontmatter
}

func mergeMarkdownIndexFrontmatter(existing map[string]frontmatterField, generated map[string]frontmatterField, known map[string]bool) map[string]frontmatterField {
	merged := map[string]frontmatterField{}
	for key, value := range generated {
		if value.Set {
			merged[key] = value
		}
	}
	for key, value := range existing {
		if !known[key] && value.Set {
			merged[key] = value
		}
	}
	return merged
}

func renderMarkdownDocument(frontmatter map[string]frontmatterField, orderedKeys []string, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	written := map[string]bool{}
	for _, key := range orderedKeys {
		if field := frontmatter[key]; field.Set {
			writeMarkdownReportFrontmatterField(&b, key, field)
			written[key] = true
		}
	}
	extraKeys := []string{}
	for key, field := range frontmatter {
		if field.Set && !written[key] {
			extraKeys = append(extraKeys, key)
		}
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		writeMarkdownReportFrontmatterField(&b, key, frontmatter[key])
	}
	b.WriteString("---\n")
	if !strings.HasPrefix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func writeMarkdownTaskIndex(rootPath string, index map[string]any) error {
	content, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode TASKS.json: %w", err)
	}
	content = append(content, '\n')
	indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return fmt.Errorf("create .agents directory: %w", err)
	}
	if err := os.WriteFile(indexPath, content, 0o600); err != nil {
		return fmt.Errorf("write TASKS.json: %w", err)
	}
	return nil
}

func markdownTaskIndexCounts(index map[string]any, importedTasks int, importedSpecs int) map[string]int {
	counts := map[string]int{
		"imported_tasks": importedTasks,
		"imported_specs": importedSpecs,
	}
	tasks, _ := markdownIndexObject(index, "tasks")
	specs, _ := markdownIndexObject(index, "specs")
	counts["tasks"] = len(tasks)
	counts["specs"] = len(specs)
	for _, value := range tasks {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		status := firstNonEmpty(jsonObjectString(entry, "status"), "todo")
		counts["tasks_"+status]++
	}
	return counts
}

func markdownTaskFrontmatterOrder() []string {
	return []string{"id", "title", "spec", "status", "priority", "created", "updated", "depends_on", "files", "verify", "done", "session", "completed_at"}
}

func markdownTaskFrontmatterKeys() map[string]bool {
	keys := map[string]bool{}
	for _, key := range markdownTaskFrontmatterOrder() {
		keys[key] = true
	}
	return keys
}

func markdownSpecFrontmatterOrder() []string {
	return []string{"id", "title", "source", "created", "status", "requirement"}
}

func markdownSpecFrontmatterKeys() map[string]bool {
	keys := map[string]bool{}
	for _, key := range markdownSpecFrontmatterOrder() {
		keys[key] = true
	}
	return keys
}

func frontmatterStringSlice(field frontmatterField) []string {
	if len(field.Values) == 0 {
		return []string{}
	}
	if field.Array {
		return append([]string{}, field.Values...)
	}
	return []string{}
}

func markdownTaskSlugFromPath(path string, id string) string {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	slug := strings.TrimPrefix(stem, id)
	slug = strings.TrimPrefix(slug, "-")
	return slug
}

func normalizeMarkdownTaskStatus(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "in progress":
		return "in_progress"
	case "complete", "completed", "archived":
		return "done"
	case "pending":
		return "todo"
	case "waiting":
		return "blocked"
	}
	if state.ValidTaskStatus(normalized) {
		return normalized
	}
	return "todo"
}

func normalizeMarkdownTaskPriority(value string) string {
	priority := strings.ToUpper(strings.TrimSpace(value))
	if state.ValidTaskPriority(priority) {
		return priority
	}
	return "P2"
}

func normalizeMarkdownSpecStatus(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "draft":
		return "drafting"
	case "done", "completed", "archived", "implemented":
		return "complete"
	case "in_progress":
		return "implementing"
	}
	switch normalized {
	case "drafting", "approved", "implementing", "complete":
		return normalized
	default:
		return "drafting"
	}
}

func normalizeMarkdownIndexDate(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(trimmed) {
		return trimmed + "T00:00:00Z"
	}
	if _, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return trimmed
	}
	if parsed, err := time.Parse(time.RFC1123, trimmed); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return trimmed
}

func numericID(id string) int {
	match := regexp.MustCompile(`\d+$`).FindString(id)
	if match == "" {
		return 0
	}
	var value int
	for _, char := range match {
		value = value*10 + int(char-'0')
	}
	return value
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func loadMarkdownTaskIndexObject(indexPath string) (map[string]any, error) {
	content, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return map[string]any{
			"version": float64(1),
			"next_id": float64(1),
			"tasks":   map[string]any{},
			"specs":   map[string]any{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read TASKS.json: %w", err)
	}
	var index map[string]any
	if err := json.Unmarshal(content, &index); err != nil {
		return nil, fmt.Errorf("parse TASKS.json: %w", err)
	}
	return index, nil
}

func markdownIndexObject(index map[string]any, key string) (map[string]any, error) {
	value, ok := index[key]
	if !ok || value == nil {
		object := map[string]any{}
		index[key] = object
		return object, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("TASKS.json %s must be an object", key)
	}
	return object, nil
}

func markdownTaskNextID(index map[string]any) int {
	value, ok := index["next_id"]
	if !ok {
		return 1
	}
	switch typed := value.(type) {
	case float64:
		if typed >= 1 {
			return int(typed)
		}
	case int:
		if typed >= 1 {
			return typed
		}
	}
	return 1
}

func markdownTaskSlug(title string) string {
	slug := strings.ToLower(strings.TrimSpace(title))
	slug = strings.NewReplacer("`", "", "'", "", `"`, "").Replace(slug)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug = re.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = strings.Trim(slug[:50], "-")
	}
	return slug
}

func renderMarkdownTask(frontmatter map[string]frontmatterField, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range []string{"id", "title", "status", "priority", "spec", "depends_on", "session", "created", "updated"} {
		if field := frontmatter[key]; field.Set {
			writeMarkdownReportFrontmatterField(&b, key, field)
		}
	}
	b.WriteString("---\n")
	if !strings.HasPrefix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func markdownTaskBody(taskID string, title string) string {
	return fmt.Sprintf(`# %s: %s

## Description

<!-- Describe the task here -->

## Acceptance Criteria

- [ ]

## Verification

`+"```"+`bash
# Add verification command
`+"```"+`
`, taskID, title)
}

func readMarkdownTask(rootPath string, path string, index map[string]markdownTaskIndexEntry) (state.TaskItem, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return state.TaskItem{}, "", fmt.Errorf("read markdown task %s: %w", path, err)
	}
	frontmatter, _ := parseKnowledgeFrontmatter(body)
	field := func(keys ...string) string {
		for _, key := range keys {
			if value := firstFieldValue(frontmatter[key]); value != "" {
				return value
			}
		}
		return ""
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	alias := firstNonEmpty(field("id"), taskAliasFromPath(path), stem)
	meta := index[alias]
	title := firstNonEmpty(meta.Title, field("title"), firstMarkdownHeading(markdownContentWithoutFrontmatter(string(body))), alias)
	status := firstNonEmpty(meta.Status, field("status"), "unknown")
	return state.TaskItem{
		Title:      title,
		Spec:       firstNonEmpty(meta.Spec, field("spec")),
		Status:     status,
		Priority:   firstNonEmpty(meta.Priority, field("priority")),
		DependsOn:  markdownTaskDependencies(meta, firstFieldValue(frontmatter["depends_on"])),
		SourcePath: markdownReportSourcePath(rootPath, path),
	}, alias, nil
}

func taskMatchesFilters(item state.TaskItem, options state.TaskListOptions) bool {
	if options.Active && (item.Status == "done" || item.Status == "archived") {
		return false
	}
	if options.Status != "" && item.Status != options.Status {
		return false
	}
	return true
}

func markdownTaskDependencies(meta markdownTaskIndexEntry, frontmatterDependsOn string) []string {
	if len(meta.DependsOn) > 0 {
		return meta.DependsOn
	}
	if frontmatterDependsOn == "" {
		return nil
	}
	return splitCommaList(frontmatterDependsOn)
}

func markdownTaskSessions(meta markdownTaskIndexEntry, frontmatterSession string) []string {
	if meta.Session != "" {
		return []string{meta.Session}
	}
	if len(meta.Sessions) > 0 {
		return meta.Sessions
	}
	if frontmatterSession == "" {
		return nil
	}
	return splitCommaList(frontmatterSession)
}

func taskAliasFromPath(path string) string {
	re := regexp.MustCompile(`TASK-\d+`)
	return re.FindString(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

func writeTaskStatus(out io.Writer, tasks state.TaskList, specs state.SpecList) {
	taskCounts := countTaskStatuses(tasks)
	specCounts := countSpecStatuses(specs)

	fmt.Fprint(out, "\n  loaf task status\n\n")
	writeProjectMutationContext(out, "  ", tasks.DatabaseScope, tasks.DatabasePath, tasks.ProjectID, tasks.ProjectName, tasks.ProjectCurrentPath)
	writeStateDiagnostics(out, "  ", tasks.Diagnostics)
	if taskListHasContext(tasks) {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  Tasks:  %s  (%d total)\n", formatStatusCounts(taskCounts, []string{"in_progress", "blocked", "todo", "review", "done"}), len(tasks.Tasks))
	fmt.Fprintf(out, "  Specs:  %s  (%d total)\n\n", formatStatusCounts(specCounts, []string{"implementing", "approved", "drafting", "complete"}), len(specs.Specs))
}

func countTaskStatuses(tasks state.TaskList) map[string]int {
	counts := map[string]int{}
	for _, task := range tasks.Tasks {
		counts[task.Status]++
	}
	return counts
}

func countSpecStatuses(specs state.SpecList) map[string]int {
	counts := map[string]int{}
	for _, spec := range specs.Specs {
		counts[spec.Status]++
	}
	return counts
}

func formatStatusCounts(counts map[string]int, order []string) string {
	parts := make([]string, 0, len(order))
	for _, status := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[status], status))
	}
	return strings.Join(parts, " · ")
}

func (r Runner) runIdea(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeIdeaHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":    writeIdeaListHelp,
		"show":    writeIdeaShowHelp,
		"capture": writeIdeaCaptureHelp,
		"promote": writeIdeaPromoteHelp,
		"resolve": writeIdeaResolveHelp,
		"archive": writeIdeaArchiveHelp,
	}) {
		return nil
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

func writeIdeaHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf idea <subcommand> [options]", "Manage ideas in native SQLite state.", []subcommandHelpItem{
		{Name: "list", Summary: "List ideas"},
		{Name: "show", Summary: "Show one idea"},
		{Name: "capture", Summary: "Capture an idea"},
		{Name: "promote", Summary: "Promote an idea to a spec"},
		{Name: "resolve", Summary: "Resolve an idea by another entity"},
		{Name: "archive", Summary: "Archive ideas"},
	})
}

func writeIdeaListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf idea list [--all|--status <status>] [--json]", "List ideas from SQLite state.", "--all        Include resolved and archived ideas", "--status     Filter by status", "--json       Output ideas, global database scope, and project identity as JSON")
}

func writeIdeaShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf idea show <idea> [--json]", "Show one idea from SQLite state.", "--json       Output idea details, relationships, global database scope, and project identity as JSON")
}

func writeIdeaCaptureHelp(out io.Writer) {
	writeUsageHelp(out, "loaf idea capture --title <title> [--json]", "Capture an idea in SQLite state.", "--title      Idea title", "--json       Output created idea, event, global database scope, and project identity as JSON")
}

func writeIdeaPromoteHelp(out io.Writer) {
	writeUsageHelp(out, "loaf idea promote <idea> --to-spec <spec> [--json]", "Record idea-to-spec promotion.", "--to-spec    Target spec", "--json       Output promotion relationship, global database scope, and project identity as JSON")
}

func writeIdeaResolveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf idea resolve <idea> --by <entity> [--json]", "Resolve an idea by linking it to another entity.", "--by         Resolving entity", "--json       Output resolution relationship, event, global database scope, and project identity as JSON")
}

func writeIdeaArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf idea archive <idea...> [--reason <text>] [--json]", "Archive one or more ideas.", "--reason     Archive reason", "--json       Output archive result, archived ideas, global database scope, and project identity as JSON")
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
	jsonRequested := hasFlag(args, "--json")
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "idea capture", err)
		}
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "idea capture", err)
		}
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		err := sqliteStateRequiredError("idea capture")
		if jsonRequested {
			return writeJSONCommandError(out, "idea capture", err)
		}
		return err
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonRequested {
			return writeJSONCommandError(out, "idea capture", err)
		}
		return err
	}

	options, err := parseIdeaCaptureArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "idea capture", err)
		}
		return err
	}
	result, err := state.CaptureIdea(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.capture)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "idea capture", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "captured idea %s\n", firstNonEmpty(result.Idea.Alias, result.Idea.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	fmt.Fprint(out, "\n  loaf idea list\n\n")
	writeProjectMutationContext(out, "  ", ideas.DatabaseScope, ideas.DatabasePath, ideas.ProjectID, ideas.ProjectName, ideas.ProjectCurrentPath)
	if len(ideas.Ideas) == 0 {
		if ideas.DatabaseScope != "" || ideas.DatabasePath != "" || ideas.ProjectID != "" || ideas.ProjectName != "" || ideas.ProjectCurrentPath != "" {
			fmt.Fprintln(out)
		}
		fmt.Fprint(out, "  No ideas found.\n\n")
		return
	}

	if ideas.DatabaseScope != "" || ideas.DatabasePath != "" || ideas.ProjectID != "" || ideas.ProjectName != "" || ideas.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
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
	if len(args) == 0 || isHelpArg(args) {
		writeSparkHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":    writeSparkListHelp,
		"show":    writeSparkShowHelp,
		"capture": writeSparkCaptureHelp,
		"resolve": writeSparkResolveHelp,
		"promote": writeSparkPromoteHelp,
	}) {
		return nil
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

func writeSparkHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf spark <subcommand> [options]", "Manage sparks in native SQLite state.", []subcommandHelpItem{
		{Name: "list", Summary: "List sparks"},
		{Name: "show", Summary: "Show one spark"},
		{Name: "capture", Summary: "Capture a spark"},
		{Name: "resolve", Summary: "Resolve a spark"},
		{Name: "promote", Summary: "Promote a spark to an idea"},
	})
}

func writeSparkListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark list [--all|--status <status>] [--json]", "List sparks from SQLite state.", "--all        Include resolved sparks", "--status     Filter by status", "--json       Output sparks, global database scope, and project identity as JSON")
}

func writeSparkShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark show <spark> [--json]", "Show one spark from SQLite state.", "--json       Output spark details, relationships, global database scope, and project identity as JSON")
}

func writeSparkCaptureHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark capture --scope <scope> --text <text> [--json]", "Capture a spark in SQLite state.", "--scope      Spark scope", "--text       Spark text", "--json       Output created spark, event, global database scope, and project identity as JSON")
}

func writeSparkResolveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark resolve <spark> [--reason <text>] [--json]", "Resolve a spark.", "--reason     Resolution reason", "--json       Output resolution relationship, event, global database scope, and project identity as JSON")
}

func writeSparkPromoteHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark promote <spark> --to-idea <idea> [--json]", "Record spark-to-idea promotion.", "--to-idea    Target idea", "--json       Output promotion relationship, global database scope, and project identity as JSON")
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "relationship: %s\n", result.Relationship)
	return nil
}

func writeSparkList(out io.Writer, sparks state.SparkList, filters state.SparkListOptions) {
	fmt.Fprint(out, "\n  loaf spark list\n\n")
	writeProjectMutationContext(out, "  ", sparks.DatabaseScope, sparks.DatabasePath, sparks.ProjectID, sparks.ProjectName, sparks.ProjectCurrentPath)
	if len(sparks.Sparks) == 0 {
		if sparks.DatabaseScope != "" || sparks.DatabasePath != "" || sparks.ProjectID != "" || sparks.ProjectName != "" || sparks.ProjectCurrentPath != "" {
			fmt.Fprintln(out)
		}
		fmt.Fprint(out, "  No sparks found.\n\n")
		return
	}

	if sparks.DatabaseScope != "" || sparks.DatabasePath != "" || sparks.ProjectID != "" || sparks.ProjectName != "" || sparks.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	if len(args) == 0 || isHelpArg(args) {
		writeTagHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":   writeTagListHelp,
		"show":   writeTagShowHelp,
		"add":    writeTagAddHelp,
		"remove": writeTagRemoveHelp,
	}) {
		return nil
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

func writeTagHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf tag <subcommand> [options]", "Manage tags in native SQLite state.", []subcommandHelpItem{
		{Name: "list", Summary: "List tags"},
		{Name: "show", Summary: "Show tagged entities"},
		{Name: "add", Summary: "Add a tag to an entity"},
		{Name: "remove", Summary: "Remove a tag from an entity"},
	})
}

func writeTagListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf tag list [--json]", "List tags from SQLite state.", "--json       Output tags, global database scope, and project identity as JSON")
}

func writeTagShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf tag show <tag> [--json]", "Show entities with a tag.", "--json       Output tagged entities, global database scope, and project identity as JSON")
}

func writeTagAddHelp(out io.Writer) {
	writeUsageHelp(out, "loaf tag add <entity> <tag> [--json]", "Add a tag to an entity.", "--json       Output tag mutation, entity, global database scope, and project identity as JSON")
}

func writeTagRemoveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf tag remove <entity> <tag> [--json]", "Remove a tag from an entity.", "--json       Output tag mutation, entity, global database scope, and project identity as JSON")
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
		fmt.Fprint(out, "\n  loaf tag list\n\n")
		writeProjectMutationContext(out, "  ", tags.DatabaseScope, tags.DatabasePath, tags.ProjectID, tags.ProjectName, tags.ProjectCurrentPath)
		if tags.DatabaseScope != "" {
			fmt.Fprintln(out)
		}
		fmt.Fprint(out, "  No tags found.\n\n")
		return
	}
	fmt.Fprint(out, "\n  loaf tag list\n\n")
	writeProjectMutationContext(out, "  ", tags.DatabaseScope, tags.DatabasePath, tags.ProjectID, tags.ProjectName, tags.ProjectCurrentPath)
	if tags.DatabaseScope != "" {
		fmt.Fprintln(out)
	}
	for _, name := range sortedTags(tags) {
		fmt.Fprintf(out, "    %-24s%d\n", name, tags.Tags[name].Count)
	}
	fmt.Fprintf(out, "\n  %d tag(s)\n\n", len(tags.Tags))
}

func writeTagShow(out io.Writer, result state.TagShowResult) {
	fmt.Fprintf(out, "\n  tag %s\n\n", result.Name)
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" {
		fmt.Fprintln(out)
	}
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
	if len(args) == 0 || isHelpArg(args) {
		writeBundleHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":   writeBundleListHelp,
		"create": writeBundleCreateHelp,
		"update": writeBundleUpdateHelp,
		"show":   writeBundleShowHelp,
		"add":    writeBundleAddHelp,
		"remove": writeBundleRemoveHelp,
	}) {
		return nil
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

func writeBundleHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf bundle <subcommand> [options]", "Manage bundles in native SQLite state.", []subcommandHelpItem{
		{Name: "list", Summary: "List bundles"},
		{Name: "create", Summary: "Create a bundle"},
		{Name: "update", Summary: "Update a bundle"},
		{Name: "show", Summary: "Show a bundle"},
		{Name: "add", Summary: "Add an entity to a bundle"},
		{Name: "remove", Summary: "Remove an entity from a bundle"},
	})
}

func writeBundleListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf bundle list [--json]", "List bundles from SQLite state.", "--json       Output bundles, global database scope, and project identity as JSON")
}

func writeBundleCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf bundle create <slug> [--title <title>] [--tags <tags>] [--json]", "Create a bundle.", "--title      Bundle title", "--tags       Comma-separated tag query", "--json       Output created bundle, tags, global database scope, and project identity as JSON")
}

func writeBundleUpdateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf bundle update <slug> [--title <title>] [--tags <tags>] [--json]", "Update a bundle.", "--title      Bundle title", "--tags       Comma-separated tag query", "--json       Output updated bundle, tags, global database scope, and project identity as JSON")
}

func writeBundleShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf bundle show <bundle> [--json]", "Show one bundle.", "--json       Output bundle details, members, global database scope, and project identity as JSON")
}

func writeBundleAddHelp(out io.Writer) {
	writeUsageHelp(out, "loaf bundle add <bundle> <entity> [--json]", "Add an entity to a bundle.", "--json       Output bundle membership result, global database scope, and project identity as JSON")
}

func writeBundleRemoveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf bundle remove <bundle> <entity> [--json]", "Remove an entity from a bundle.", "--json       Output bundle membership result, global database scope, and project identity as JSON")
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
		fmt.Fprint(out, "\n  loaf bundle list\n\n")
		writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
		if result.DatabaseScope != "" {
			fmt.Fprintln(out)
		}
		fmt.Fprint(out, "  No bundles found.\n\n")
		return
	}
	fmt.Fprint(out, "\n  loaf bundle list\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" {
		fmt.Fprintln(out)
	}
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
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" {
		fmt.Fprintln(out)
	}
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
	if len(args) == 0 || isHelpArg(args) {
		writeLinkHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"create": writeLinkCreateHelp,
		"list":   writeLinkListHelp,
		"remove": writeLinkRemoveHelp,
	}) {
		return nil
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

func writeLinkHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf link <subcommand> [options]", "Manage explicit relationships in native SQLite state.", []subcommandHelpItem{
		{Name: "create", Summary: "Create a relationship"},
		{Name: "list", Summary: "List relationships for an entity"},
		{Name: "remove", Summary: "Remove a relationship"},
	})
}

func writeLinkCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf link create --from <entity> --to <entity> [--type <type>] [--reason <text>] [--json]", "Create an explicit relationship.", "--from       Source entity", "--to         Target entity", "--type       Relationship type", "--reason     Relationship reason", "--json       Output relationship ID, source/target, global database scope, and project identity as JSON")
}

func writeLinkListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf link list <entity> [--json]", "List relationships for one entity.", "--json       Output relationships, global database scope, and project identity as JSON")
}

func writeLinkRemoveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf link remove --from <entity> --to <entity> [--type <type>] [--json]", "Remove an explicit relationship.", "--from       Source entity", "--to         Target entity", "--type       Relationship type", "--json       Output removed relationship ID, global database scope, and project identity as JSON")
}

func (r Runner) runLinkCreate(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseLinkMutationArgs("link create", args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "link create", err)
		}
		return err
	}
	projectRoot, mode, err := r.linkStateMode(runtime)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "link create", err)
		}
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		if jsonRequested {
			return writeJSONCommandError(out, "link create", sqliteStateRequiredError("link create"))
		}
		return sqliteStateRequiredError("link create")
	case state.ModeInvalid:
		if jsonRequested {
			return writeJSONCommandError(out, "link create", fmt.Errorf("state database is invalid; run `loaf state doctor`"))
		}
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.CreateLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.LinkMutationOptions{
		From:   options.from,
		To:     options.to,
		Type:   options.relationshipType,
		Reason: options.reason,
	})
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "link create", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "linked %s %s %s %s %s\n", result.From.Kind, firstNonEmpty(result.From.Alias, result.From.ID), result.Type, result.To.Kind, firstNonEmpty(result.To.Alias, result.To.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	jsonRequested := hasFlag(args, "--json")
	options, err := parseLinkMutationArgs("link remove", args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "link remove", err)
		}
		return err
	}
	projectRoot, mode, err := r.linkStateMode(runtime)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "link remove", err)
		}
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		if jsonRequested {
			return writeJSONCommandError(out, "link remove", sqliteStateRequiredError("link remove"))
		}
		return sqliteStateRequiredError("link remove")
	case state.ModeInvalid:
		if jsonRequested {
			return writeJSONCommandError(out, "link remove", fmt.Errorf("state database is invalid; run `loaf state doctor`"))
		}
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RemoveLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.LinkMutationOptions{
		From:   options.from,
		To:     options.to,
		Type:   options.relationshipType,
		Reason: options.reason,
	})
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "link remove", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "removed link %s %s %s %s %s\n", result.From.Kind, firstNonEmpty(result.From.Alias, result.From.ID), result.Type, result.To.Kind, firstNonEmpty(result.To.Alias, result.To.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" {
		fmt.Fprintln(out)
	}
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
	if len(args) == 0 || isHelpArg(args) {
		writeSpecHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":    writeSpecListHelp,
		"show":    writeSpecShowHelp,
		"archive": writeSpecArchiveHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "list":
		return r.runSpecList(args[1:], out, runtime)
	case "show":
		return r.runSpecShow(args[1:], out, runtime)
	case "archive":
		return r.runSpecArchive(args[1:], out, runtime)
	default:
		return unknownSubcommandError("spec", args[0])
	}
}

func writeSpecHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf spec <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage project specs in native SQLite state or markdown compatibility mode.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  list     List specs")
	fmt.Fprintln(out, "  show     Show one spec")
	fmt.Fprintln(out, "  archive  Archive completed specs")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeSpecListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec list [--json]", "List specs.", "--json       Output specs, diagnostics, task counts, global database scope, and project identity as JSON")
}

func writeSpecShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec show <spec> [--json]", "Show one spec.", "--json       Output spec details, task counts, relationships, global database scope, and project identity as JSON")
}

func writeSpecArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec archive <spec...> [--json]", "Archive completed specs.", "--json       Output archive result, archived specs, global database scope, and project identity as JSON")
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
		specs, err := markdownSpecList(projectRoot.Path())
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, specs)
		}
		writeSpecList(out, specs)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	specs, err := state.ListSpecs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	specs.Diagnostics = stateListWarnings(status.Diagnostics)
	if jsonOutput {
		return writeJSON(out, specs)
	}
	writeSpecList(out, specs)
	return nil
}

func (r Runner) runSpecShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("spec show", args)
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
		result, err := markdownSpecShow(projectRoot.Path(), ref)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeSpecShow(out, result)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
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
		refs, jsonOutput, err := parseArchiveArgs("spec archive", args)
		if err != nil {
			return err
		}
		result, err := markdownSpecArchive(projectRoot.Path(), refs)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeSpecArchive(out, result)
		return nil
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
	fmt.Fprint(out, "\n  loaf spec list\n\n")
	writeProjectMutationContext(out, "  ", specs.DatabaseScope, specs.DatabasePath, specs.ProjectID, specs.ProjectName, specs.ProjectCurrentPath)
	writeStateDiagnostics(out, "  ", specs.Diagnostics)
	if specListHasContext(specs) {
		fmt.Fprintln(out)
	}

	if len(specs.Specs) == 0 {
		fmt.Fprint(out, "  No specs found.\n\n")
		return
	}

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

func specListHasContext(specs state.SpecList) bool {
	return specs.DatabaseScope != "" ||
		specs.DatabasePath != "" ||
		specs.ProjectID != "" ||
		specs.ProjectName != "" ||
		specs.ProjectCurrentPath != "" ||
		len(specs.Diagnostics) > 0
}

func markdownSpecList(rootPath string) (state.SpecList, error) {
	agentsDir := filepath.Join(rootPath, ".agents")
	files, err := filepath.Glob(filepath.Join(agentsDir, "specs", "*.md"))
	if err != nil {
		return state.SpecList{}, fmt.Errorf("find markdown specs: %w", err)
	}
	sort.Strings(files)
	taskCounts := markdownSpecTaskCounts(rootPath)
	specIndex := loadMarkdownSpecIndex(rootPath)
	specs := state.SpecList{Version: 1, Specs: map[string]state.SpecItem{}}
	for _, path := range files {
		item, alias, err := readMarkdownSpec(rootPath, path, specIndex)
		if err != nil {
			return state.SpecList{}, err
		}
		item.Tasks = taskCounts[alias]
		specs.Specs[alias] = item
	}
	return specs, nil
}

func markdownSpecShow(rootPath string, ref string) (state.SpecShow, error) {
	agentsDir := filepath.Join(rootPath, ".agents")
	files, err := filepath.Glob(filepath.Join(agentsDir, "specs", "*.md"))
	if err != nil {
		return state.SpecShow{}, fmt.Errorf("find markdown specs: %w", err)
	}
	sort.Strings(files)
	specIndex := loadMarkdownSpecIndex(rootPath)
	taskIndex := loadMarkdownTaskIndex(rootPath)
	taskCounts := markdownSpecTaskCounts(rootPath)
	for _, path := range files {
		item, alias, err := readMarkdownSpec(rootPath, path, specIndex)
		if err != nil {
			return state.SpecShow{}, err
		}
		if alias != ref {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return state.SpecShow{}, fmt.Errorf("read markdown spec %s: %w", path, err)
		}
		frontmatter, _ := parseKnowledgeFrontmatter(body)
		field := func(keys ...string) string {
			for _, key := range keys {
				if value := firstFieldValue(frontmatter[key]); value != "" {
					return value
				}
			}
			return ""
		}
		hash := sha256.Sum256(body)
		meta := specIndex[alias]
		return state.SpecShow{
			Query: ref,
			Spec: state.SpecDetail{
				ID:            alias,
				Alias:         alias,
				Title:         item.Title,
				Status:        item.Status,
				Tasks:         taskCounts[alias],
				Sources:       []state.TraceSource{{Path: item.SourcePath, Hash: hex.EncodeToString(hash[:])}},
				Body:          strings.TrimSpace(markdownContentWithoutFrontmatter(string(body))),
				Relationships: markdownSpecRelationships(alias, taskIndex),
				CreatedAt:     firstNonEmpty(meta.Created, field("created", "created_at")),
				UpdatedAt:     firstNonEmpty(meta.Updated, field("updated", "updated_at"), meta.Created, field("created", "created_at")),
			},
		}, nil
	}
	return state.SpecShow{}, fmt.Errorf("spec %q not found in markdown specs", ref)
}

func readMarkdownSpec(rootPath string, path string, index map[string]markdownSpecIndexEntry) (state.SpecItem, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return state.SpecItem{}, "", fmt.Errorf("read markdown spec %s: %w", path, err)
	}
	frontmatter, _ := parseKnowledgeFrontmatter(body)
	field := func(keys ...string) string {
		for _, key := range keys {
			if value := firstFieldValue(frontmatter[key]); value != "" {
				return value
			}
		}
		return ""
	}
	alias := firstNonEmpty(field("id"), specAliasFromPath(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	meta := index[alias]
	title := firstNonEmpty(meta.Title, field("title"), firstMarkdownHeading(markdownContentWithoutFrontmatter(string(body))), alias)
	status := firstNonEmpty(meta.Status, field("status"), "unknown")
	return state.SpecItem{
		Title:      title,
		Status:     status,
		SourcePath: markdownReportSourcePath(rootPath, path),
	}, alias, nil
}

func markdownSpecTaskCounts(rootPath string) map[string]state.SpecTaskCounts {
	counts := map[string]state.SpecTaskCounts{}
	for _, task := range loadMarkdownTaskIndex(rootPath) {
		if task.Spec == "" {
			continue
		}
		specCounts := counts[task.Spec]
		switch task.Status {
		case "done", "archived":
			specCounts.Done++
		case "in_progress":
			specCounts.InProgress++
		default:
			specCounts.Todo++
		}
		counts[task.Spec] = specCounts
	}
	return counts
}

func markdownSpecRelationships(alias string, tasks map[string]markdownTaskIndexEntry) []state.TraceRelationship {
	taskAliases := make([]string, 0)
	for taskAlias, task := range tasks {
		if task.Spec == alias {
			taskAliases = append(taskAliases, taskAlias)
		}
	}
	sort.Strings(taskAliases)
	relationships := make([]state.TraceRelationship, 0, len(taskAliases))
	for _, taskAlias := range taskAliases {
		task := tasks[taskAlias]
		relationships = append(relationships, state.TraceRelationship{
			Direction: "inbound",
			Type:      "implements",
			Entity: state.TraceEntity{
				Kind:   "task",
				ID:     taskAlias,
				Alias:  taskAlias,
				Title:  task.Title,
				Status: task.Status,
			},
		})
	}
	return relationships
}

func markdownSpecArchive(rootPath string, refs []string) (state.SpecArchiveResult, error) {
	if len(refs) == 0 {
		return state.SpecArchiveResult{}, fmt.Errorf("spec archive requires at least one spec")
	}
	indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return state.SpecArchiveResult{}, fmt.Errorf("read TASKS.json: %w", err)
	}
	var index map[string]any
	if err := json.Unmarshal(content, &index); err != nil {
		return state.SpecArchiveResult{}, fmt.Errorf("parse TASKS.json: %w", err)
	}
	specsValue, ok := index["specs"]
	if !ok {
		specsValue = map[string]any{}
		index["specs"] = specsValue
	}
	specs, ok := specsValue.(map[string]any)
	if !ok {
		return state.SpecArchiveResult{}, fmt.Errorf("TASKS.json specs must be an object")
	}

	result := state.SpecArchiveResult{ContractVersion: state.StateJSONContractVersion, Archived: []state.SpecArchiveItem{}, Skipped: []state.SpecArchiveItem{}}
	changed := false
	for _, ref := range refs {
		entryValue, ok := specs[ref]
		if !ok {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Ref: ref, Reason: "not found in index"})
			continue
		}
		entry, ok := entryValue.(map[string]any)
		if !ok {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Ref: ref, Reason: "index entry is not an object"})
			continue
		}
		title := jsonObjectString(entry, "title")
		status := jsonObjectString(entry, "status")
		spec := state.TraceEntity{Kind: "spec", ID: ref, Alias: ref, Title: title, Status: status}
		if status != "complete" {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: status, Status: status, Reason: fmt.Sprintf("status is %s, must be complete", status)})
			continue
		}
		file := jsonObjectString(entry, "file")
		if strings.HasPrefix(file, "archive/") {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: status, Status: status, Reason: "already archived"})
			continue
		}
		if file == "" {
			file = markdownSpecFileForAlias(rootPath, ref)
		}
		if file == "" {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: status, Status: status, Reason: "file not found in index"})
			continue
		}
		srcPath := filepath.Join(rootPath, ".agents", "specs", filepath.FromSlash(file))
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: status, Status: status, Reason: fmt.Sprintf("file not found at %s", file)})
			continue
		} else if err != nil {
			return state.SpecArchiveResult{}, fmt.Errorf("inspect markdown spec %s: %w", file, err)
		}
		archivedFile := filepath.ToSlash(filepath.Join("archive", filepath.FromSlash(file)))
		destPath := filepath.Join(rootPath, ".agents", "specs", filepath.FromSlash(archivedFile))
		if _, err := os.Stat(destPath); err == nil {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: status, Status: status, Reason: fmt.Sprintf("%s already exists", archivedFile)})
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return state.SpecArchiveResult{}, fmt.Errorf("inspect archived markdown spec %s: %w", archivedFile, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return state.SpecArchiveResult{}, fmt.Errorf("create spec archive directory: %w", err)
		}
		if err := os.Rename(srcPath, destPath); err != nil {
			return state.SpecArchiveResult{}, fmt.Errorf("archive markdown spec %s: %w", ref, err)
		}
		entry["file"] = archivedFile
		changed = true
		spec.Status = "archived"
		result.Archived = append(result.Archived, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: "complete", Status: "archived"})
	}
	if changed {
		updated, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return state.SpecArchiveResult{}, fmt.Errorf("encode TASKS.json: %w", err)
		}
		updated = append(updated, '\n')
		if err := os.WriteFile(indexPath, updated, 0o600); err != nil {
			return state.SpecArchiveResult{}, fmt.Errorf("write TASKS.json: %w", err)
		}
	}
	return result, nil
}

func markdownTaskArchive(rootPath string, options state.TaskArchiveOptions) (state.TaskArchiveResult, error) {
	if options.Spec != "" && len(options.Refs) > 0 {
		return state.TaskArchiveResult{}, fmt.Errorf("task archive accepts task ids or --spec, not both")
	}
	if options.Spec == "" && len(options.Refs) == 0 {
		return state.TaskArchiveResult{}, fmt.Errorf("task archive requires task ids or --spec")
	}
	indexPath := filepath.Join(rootPath, ".agents", "TASKS.json")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return state.TaskArchiveResult{}, fmt.Errorf("read TASKS.json: %w", err)
	}
	var index map[string]any
	if err := json.Unmarshal(content, &index); err != nil {
		return state.TaskArchiveResult{}, fmt.Errorf("parse TASKS.json: %w", err)
	}
	tasksValue, ok := index["tasks"]
	if !ok {
		tasksValue = map[string]any{}
		index["tasks"] = tasksValue
	}
	tasks, ok := tasksValue.(map[string]any)
	if !ok {
		return state.TaskArchiveResult{}, fmt.Errorf("TASKS.json tasks must be an object")
	}

	result := state.TaskArchiveResult{ContractVersion: state.StateJSONContractVersion, Archived: []state.TaskArchiveItem{}, Skipped: []state.TaskArchiveItem{}}
	refs := append([]string{}, options.Refs...)
	if options.Spec != "" {
		specs, ok := index["specs"].(map[string]any)
		if !ok {
			return state.TaskArchiveResult{}, fmt.Errorf("TASKS.json specs must be an object")
		}
		specValue, ok := specs[options.Spec]
		if !ok {
			return state.TaskArchiveResult{}, fmt.Errorf("Spec %q not found in index", options.Spec)
		}
		spec := state.TraceEntity{Kind: "spec", ID: options.Spec, Alias: options.Spec}
		if specEntry, ok := specValue.(map[string]any); ok {
			spec.Title = jsonObjectString(specEntry, "title")
			spec.Status = jsonObjectString(specEntry, "status")
		}
		result.Spec = &spec
		for id, entryValue := range tasks {
			entry, ok := entryValue.(map[string]any)
			if !ok {
				continue
			}
			if jsonObjectString(entry, "spec") == options.Spec && jsonObjectString(entry, "status") == "done" {
				refs = append(refs, id)
			}
		}
		sort.Strings(refs)
	}

	changed := false
	for _, ref := range refs {
		entryValue, ok := tasks[ref]
		if !ok {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Ref: ref, Reason: "not found in index"})
			continue
		}
		entry, ok := entryValue.(map[string]any)
		if !ok {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Ref: ref, Reason: "index entry is not an object"})
			continue
		}
		title := jsonObjectString(entry, "title")
		status := jsonObjectString(entry, "status")
		task := state.TraceEntity{Kind: "task", ID: ref, Alias: ref, Title: title, Status: status}
		if status != "done" {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Task: &task, Ref: ref, Previous: status, Status: status, Reason: fmt.Sprintf("status is %s, must be done", status)})
			continue
		}
		file := jsonObjectString(entry, "file")
		if strings.HasPrefix(file, "archive/") {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Task: &task, Ref: ref, Previous: status, Status: status, Reason: "already archived"})
			continue
		}
		if file == "" {
			file = markdownTaskFileForAlias(rootPath, ref)
		}
		if file == "" {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Task: &task, Ref: ref, Previous: status, Status: status, Reason: "file not found in index"})
			continue
		}
		srcPath := filepath.Join(rootPath, ".agents", "tasks", filepath.FromSlash(file))
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Task: &task, Ref: ref, Previous: status, Status: status, Reason: fmt.Sprintf("file not found at %s", file)})
			continue
		} else if err != nil {
			return state.TaskArchiveResult{}, fmt.Errorf("inspect markdown task %s: %w", file, err)
		}
		archivedFile := filepath.ToSlash(filepath.Join("archive", filepath.FromSlash(file)))
		destPath := filepath.Join(rootPath, ".agents", "tasks", filepath.FromSlash(archivedFile))
		if _, err := os.Stat(destPath); err == nil {
			result.Skipped = append(result.Skipped, state.TaskArchiveItem{Task: &task, Ref: ref, Previous: status, Status: status, Reason: fmt.Sprintf("%s already exists", archivedFile)})
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return state.TaskArchiveResult{}, fmt.Errorf("inspect archived markdown task %s: %w", archivedFile, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return state.TaskArchiveResult{}, fmt.Errorf("create task archive directory: %w", err)
		}
		if err := os.Rename(srcPath, destPath); err != nil {
			return state.TaskArchiveResult{}, fmt.Errorf("archive markdown task %s: %w", ref, err)
		}
		entry["file"] = archivedFile
		changed = true
		task.Status = "archived"
		result.Archived = append(result.Archived, state.TaskArchiveItem{Task: &task, Ref: ref, Previous: "done", Status: "archived"})
	}
	if changed {
		updated, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return state.TaskArchiveResult{}, fmt.Errorf("encode TASKS.json: %w", err)
		}
		updated = append(updated, '\n')
		if err := os.WriteFile(indexPath, updated, 0o600); err != nil {
			return state.TaskArchiveResult{}, fmt.Errorf("write TASKS.json: %w", err)
		}
	}
	return result, nil
}

func jsonObjectString(object map[string]any, key string) string {
	value, ok := object[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func jsonStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				items = append(items, text)
			}
		}
		return items
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return splitCommaList(typed)
	default:
		return nil
	}
}

func isMarkdownNoneValue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "none")
}

func markdownSpecFileForAlias(rootPath string, alias string) string {
	files, err := filepath.Glob(filepath.Join(rootPath, ".agents", "specs", "*.md"))
	if err != nil {
		return ""
	}
	sort.Strings(files)
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		frontmatter, _ := parseKnowledgeFrontmatter(body)
		found := firstNonEmpty(firstFieldValue(frontmatter["id"]), specAliasFromPath(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		if found == alias {
			return filepath.Base(path)
		}
	}
	return ""
}

func markdownTaskFileForAlias(rootPath string, alias string) string {
	files, err := filepath.Glob(filepath.Join(rootPath, ".agents", "tasks", "*.md"))
	if err != nil {
		return ""
	}
	sort.Strings(files)
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		frontmatter, _ := parseKnowledgeFrontmatter(body)
		found := firstNonEmpty(firstFieldValue(frontmatter["id"]), taskAliasFromPath(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		if found == alias {
			return filepath.Base(path)
		}
	}
	return ""
}

func specAliasFromPath(path string) string {
	re := regexp.MustCompile(`SPEC-\d+`)
	return re.FindString(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

type markdownSpecIndexEntry struct {
	Title   string `json:"title"`
	Status  string `json:"status"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	File    string `json:"file,omitempty"`
}

type markdownTaskIndexEntry struct {
	Title     string   `json:"title"`
	Spec      string   `json:"spec"`
	Status    string   `json:"status"`
	Priority  string   `json:"priority"`
	DependsOn []string `json:"depends_on"`
	Session   string   `json:"session"`
	Sessions  []string `json:"sessions"`
	Created   string   `json:"created"`
	Updated   string   `json:"updated"`
	File      string   `json:"file,omitempty"`
}

func loadMarkdownTaskIndex(rootPath string) map[string]markdownTaskIndexEntry {
	index := map[string]markdownTaskIndexEntry{}
	content, err := os.ReadFile(filepath.Join(rootPath, ".agents", "TASKS.json"))
	if err != nil {
		return index
	}
	var parsed struct {
		Tasks map[string]markdownTaskIndexEntry `json:"tasks"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return index
	}
	return parsed.Tasks
}

func loadMarkdownSpecIndex(rootPath string) map[string]markdownSpecIndexEntry {
	index := map[string]markdownSpecIndexEntry{}
	content, err := os.ReadFile(filepath.Join(rootPath, ".agents", "TASKS.json"))
	if err != nil {
		return index
	}
	var parsed struct {
		Specs map[string]markdownSpecIndexEntry `json:"specs"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return index
	}
	return parsed.Specs
}

func writeSpecShow(out io.Writer, result state.SpecShow) {
	spec := result.Spec
	fmt.Fprintf(out, "spec %s\n", firstNonEmpty(spec.Alias, spec.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	if len(args) == 0 || isHelpArg(args) {
		writeSessionHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"start":        writeSessionStartHelp,
		"end":          writeSessionEndHelp,
		"archive":      writeSessionArchiveHelp,
		"list":         writeSessionListHelp,
		"show":         writeSessionShowHelp,
		"log":          writeSessionLogHelp,
		"enrich":       writeSessionEnrichHelp,
		"housekeeping": writeSessionHousekeepingHelp,
		"report":       writeSessionReportHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "start":
		return r.runSessionStart(args[1:], out, runtime)
	case "end":
		return r.runSessionEnd(args[1:], out, runtime)
	case "archive":
		return r.runSessionArchive(args[1:], out, runtime)
	case "list":
		return r.runSessionList(args[1:], out, runtime)
	case "show":
		return r.runSessionShow(args[1:], out, runtime)
	case "log":
		return r.runSessionLog(args[1:], out, runtime)
	case "enrich":
		return r.runSessionEnrich(args[1:], out, runtime)
	case "housekeeping":
		return r.runSessionHousekeeping(args[1:], out, runtime)
	case "state":
		return r.runSessionState(args[1:], out, runtime)
	case "context":
		return r.runSessionContext(args[1:], out, runtime)
	case "report":
		return r.runSessionReport(args[1:], out, runtime)
	default:
		return unknownSubcommandError("session", args[0])
	}
}

func writeSessionHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf session <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage session journals and native SQLite session state.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  start         Start or resume a session for the current branch")
	fmt.Fprintln(out, "  end           Stop the current or targeted session")
	fmt.Fprintln(out, "  archive       Archive a stopped or targeted session")
	fmt.Fprintln(out, "  list          List sessions")
	fmt.Fprintln(out, "  show          Show one session")
	fmt.Fprintln(out, "  log           Append a journal entry")
	fmt.Fprintln(out, "  enrich        Summarize markdown enrichment compatibility in SQLite mode")
	fmt.Fprintln(out, "  housekeeping  Summarize markdown housekeeping compatibility in SQLite mode")
	fmt.Fprintln(out, "  state         Compatibility session-state helpers")
	fmt.Fprintln(out, "  context       Render hook context prompts")
	fmt.Fprintln(out, "  report        Export a session report")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help    Show help")
}

func writeSessionStartHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session start [--resume] [--session-id <id>] [--force] [--json]", "Start or resume session state.", "--resume      Resume if possible", "--session-id  Harness session ID", "--force       Ignore hook agent adoption guard", "--json        Output action, session, journal IDs, global database scope, and project identity as JSON")
}

func writeSessionEndHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session end [--if-active] [--wrap] [--from-hook] [--session-id <id>] [--json]", "End, wrap, or clear a session.", "--if-active   No-op when no active session exists", "--wrap        Mark as wrapped", "--from-hook   Read hook input", "--session-id  Harness session ID", "--json        Output action/noop, session, journal IDs, global database scope, and project identity as JSON")
}

func writeSessionArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session archive [--branch <branch>] [--session-id <id>] [--json]", "Archive a stopped or targeted session.", "--branch      Branch to archive", "--session-id  Harness session ID", "--json        Output archive result, affected sessions, global database scope, and project identity as JSON")
}

func writeSessionListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session list [--all] [--json]", "List sessions.", "--all         Include archived sessions", "--json        Output sessions, diagnostics, global database scope, and project identity as JSON")
}

func writeSessionShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session show <session> [--json]", "Show one session.", "--json        Output session details, journal entries, relationships, global database scope, and project identity as JSON")
}

func writeSessionLogHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session log <entry> [--from-hook] [--session-id <id>] [--json]", "Append a session journal entry.", "--from-hook   Read hook input", "--session-id  Harness session ID", "--json        Output journal entry, linked session, global database scope, and project identity as JSON")
}

func writeSessionEnrichHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session enrich [--json]", "Summarize markdown enrichment compatibility.", "--json        Output compatibility mode, action, reason, and counts as JSON")
}

func writeSessionHousekeepingHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session housekeeping [--json]", "Summarize markdown housekeeping compatibility.", "--json        Output compatibility mode, action, reason, and counts as JSON")
}

func writeSessionReportHelp(out io.Writer) {
	writeUsageHelp(out, "loaf session report <session> [--json]", "Export a session report.", "--json        Output export contract, command, project context, and markdown content as JSON")
}

func (r Runner) runSessionStart(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseSessionStartArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runMarkdownSessionStart(options, out, runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	if os.Getenv("LOAF_ENRICHMENT") == "1" {
		return nil
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" && !options.force {
		return nil
	}
	harnessSessionID := firstNonEmpty(options.harnessSessionID, hookInput.SessionID)
	branch := state.ObservedGitBranch(runtime.RootPath())
	if branch == "" {
		return fmt.Errorf("session start requires a git branch")
	}
	result, err := state.StartSession(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SessionStartOptions{
		Branch:           branch,
		HarnessSessionID: harnessSessionID,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSessionStart(out, branch, result)
	return nil
}

func (r Runner) runSessionEnd(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseSessionEndArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runMarkdownSessionEnd(options, out, runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	if os.Getenv("LOAF_ENRICHMENT") == "1" {
		return nil
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	harnessSessionID := firstNonEmpty(options.harnessSessionID, hookInput.SessionID)
	result, err := state.EndSession(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SessionEndOptions{
		Branch:           state.ObservedGitBranch(runtime.RootPath()),
		HarnessSessionID: harnessSessionID,
		IfActive:         options.ifActive,
		Wrap:             options.wrap,
		Clear:            hookInput.Reason == "clear",
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSessionEnd(out, result)
	return nil
}

func (r Runner) runSessionArchive(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseSessionArchiveArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runMarkdownSessionArchive(options, out, runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	branch := options.branch
	if branch == "" {
		branch = state.ObservedGitBranch(runtime.RootPath())
	}
	result, err := state.ArchiveSession(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SessionArchiveOptions{
		Branch:           branch,
		HarnessSessionID: options.harnessSessionID,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSessionArchive(out, result)
	return nil
}

func (r Runner) runMarkdownSessionArchive(options sessionArchiveOptions, out io.Writer, runtime state.Runtime, root project.Root) error {
	branch := options.branch
	if branch == "" {
		branch = state.ObservedGitBranch(runtime.RootPath())
	}
	if branch == "" && options.harnessSessionID == "" {
		return fmt.Errorf("not in a git repository; use --branch <branch>")
	}
	resolution := resolveMarkdownSessionForLog(root.Path(), branch, options.harnessSessionID, "")
	if !resolution.Found {
		if branch != "" {
			fmt.Fprintf(r.sessionLogErrWriter(), "WARN: no session for branch '%s'; no active sessions to fall back to. Pass --session-id <id> to silence.\n", branch)
		}
		return fmt.Errorf("no active session found for branch %s", branch)
	}
	if resolution.Adoption == "most-recent-active" {
		fmt.Fprintf(r.sessionLogErrWriter(), "WARN: no session for branch '%s'; logging to most-recent active session '%s' (origin branch '%s'). Pass --session-id <id> to silence.\n", branch, filepath.Base(resolution.Path), resolution.OriginBranch)
	}
	result, err := archiveMarkdownSession(root.Path(), resolution.Path)
	if err != nil {
		return err
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  loaf session archive")
	fmt.Fprintln(out)
	for _, decision := range result.Decisions {
		fmt.Fprintf(out, "  decision: %s\n", decision)
	}
	if len(result.Decisions) > 0 {
		fmt.Fprintln(out)
	}
	if result.SpecMessage != "" {
		fmt.Fprintf(out, "  %s\n", result.SpecMessage)
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  Archived: %s\n", result.RelativePath)
	fmt.Fprintln(out)
	return nil
}

type markdownSessionArchiveResult struct {
	RelativePath string
	Decisions    []string
	SpecMessage  string
}

func archiveMarkdownSession(rootPath string, sessionPath string) (markdownSessionArchiveResult, error) {
	body, err := os.ReadFile(sessionPath)
	if err != nil {
		return markdownSessionArchiveResult{}, err
	}
	fields, ok := markdownSessionFrontmatter(sessionPath)
	if !ok {
		return markdownSessionArchiveResult{}, fmt.Errorf("frontmatter not found")
	}
	decisions := extractMarkdownDecisionEntries(markdownContentWithoutFrontmatter(string(body)), 10)
	specMessage := ""
	if fields["spec"] != "" && len(decisions) > 0 {
		message, err := persistMarkdownArchiveDecisionsToSpec(rootPath, fields["spec"], firstNonEmpty(fields["branch"], "unknown"), decisions)
		if err != nil {
			specMessage = "Could not persist to spec: " + err.Error()
		} else {
			specMessage = message
		}
	}
	if sessionID := fields["claude_session_id"]; sessionID != "" {
		_ = os.Remove(filepath.Join(rootPath, ".agents", "tmp", sessionID+"-enrichment.txt"))
	}
	archiveDir := filepath.Join(rootPath, ".agents", "sessions", "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return markdownSessionArchiveResult{}, err
	}
	archivePath := filepath.Join(archiveDir, filepath.Base(sessionPath))
	if err := os.Rename(sessionPath, archivePath); err != nil {
		return markdownSessionArchiveResult{}, fmt.Errorf("archive session: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := body
	updated, err = setFrontmatterScalar(updated, "status", "archived")
	if err != nil {
		return markdownSessionArchiveResult{}, err
	}
	updated, err = setFrontmatterScalar(updated, "archived_at", now)
	if err != nil {
		return markdownSessionArchiveResult{}, err
	}
	updated, err = setFrontmatterScalar(updated, "last_updated", now)
	if err != nil {
		return markdownSessionArchiveResult{}, err
	}
	if err := os.WriteFile(archivePath, updated, 0o600); err != nil {
		return markdownSessionArchiveResult{}, err
	}
	return markdownSessionArchiveResult{
		RelativePath: markdownSessionRelativePath(rootPath, archivePath),
		Decisions:    decisions,
		SpecMessage:  specMessage,
	}, nil
}

func persistMarkdownArchiveDecisionsToSpec(rootPath string, specID string, sessionBranch string, decisions []string) (string, error) {
	specsDir := filepath.Join(rootPath, ".agents", "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return "", fmt.Errorf("no specs directory found")
	}
	specPath := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if strings.Contains(entry.Name(), specID) {
			specPath = filepath.Join(specsDir, entry.Name())
			break
		}
	}
	if specPath == "" {
		return "", fmt.Errorf("spec %s not found", specID)
	}
	body, err := os.ReadFile(specPath)
	if err != nil {
		return "", err
	}
	date := time.Now().UTC().Format("2006-01-02")
	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("- %s - Session %s archived: %d decision(s) extracted\n", date, sessionBranch, len(decisions)))
	for _, decision := range decisions {
		entry.WriteString("  ")
		entry.WriteString(decision)
		entry.WriteString("\n")
	}
	entry.WriteString("\n")
	text := string(body)
	if idx := strings.Index(text, "\n## Changelog\n"); idx >= 0 {
		insertAt := idx + len("\n## Changelog\n")
		text = text[:insertAt] + entry.String() + text[insertAt:]
	} else {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n## Changelog\n\n" + entry.String()
	}
	if err := os.WriteFile(specPath, []byte(text), 0o600); err != nil {
		return "", err
	}
	return "Appended decisions to " + filepath.Base(specPath), nil
}

func extractMarkdownDecisionEntries(content string, limit int) []string {
	lines := strings.Split(content, "\n")
	var entries []string
	decisionRE := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] decision\([^)]+\):`)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if decisionRE.MatchString(trimmed) {
			entries = append(entries, trimmed)
		}
	}
	if limit > 0 && len(entries) > limit {
		return entries[:limit]
	}
	return entries
}

func (r Runner) runSessionEnrich(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseCompatibilityCommandArgs("session enrich", args, map[string]bool{"--dry-run": true})
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		summary, err := markdownCompatibilitySummary(projectRoot.Path(), "session enrich", "markdown", "skipped", "Session JSONL enrichment is no longer run through the TypeScript bridge; use native `loaf session log` and `loaf session state update` to maintain markdown journals.")
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, summary)
		}
		writeCompatibilityCommandSummary(out, summary)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	sessions, err := state.ListSessions(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SessionListOptions{All: true})
	if err != nil {
		return err
	}
	summary := compatibilityCommandSummary{
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         "session enrich",
		Mode:            "sqlite",
		Action:          "skipped",
		Reason:          "SQLite journal state is written through `loaf session log`; Markdown JSONL enrichment is a compatibility path and is not run in SQLite mode.",
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

func (r Runner) runSessionHousekeeping(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseCompatibilityCommandArgs("session housekeeping", args, map[string]bool{"--dry-run": true})
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		summary, err := markdownCompatibilitySummary(projectRoot.Path(), "session housekeeping", "markdown", "skipped", "Native markdown session lifecycle commands now own start/end/archive/log/state updates; legacy TypeScript housekeeping is not run.")
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, summary)
		}
		writeCompatibilityCommandSummary(out, summary)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	sessions, err := state.ListSessions(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SessionListOptions{All: true})
	if err != nil {
		return err
	}
	summary := compatibilityCommandSummary{
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         "session housekeeping",
		Mode:            "sqlite",
		Action:          "skipped",
		Reason:          "SQLite session lifecycle is maintained by native `loaf session start/end/archive/log`; markdown session housekeeping is a compatibility cleanup path and is not run in SQLite mode.",
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

func (r Runner) runSessionState(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		writeSessionStateHelp(out)
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		writeSessionStateHelp(out)
		return nil
	}
	switch args[0] {
	case "update":
		return r.runSessionStateUpdate(args[1:], out, runtime)
	default:
		return unknownSubcommandError("session state", args[0])
	}
}

func writeSessionStateHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf session state <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run compatibility helpers for session state maintenance.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  update  Refresh legacy markdown session state in markdown-only mode")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func (r Runner) runSessionStateUpdate(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) > 0 {
		return fmt.Errorf("session state update accepts no arguments")
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runMarkdownSessionStateUpdate(runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	return r.runSQLiteSessionStateUpdate(runtime, projectRoot)
}

func (r Runner) runSQLiteSessionStateUpdate(runtime state.Runtime, root project.Root) error {
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" {
		return nil
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	session, found, err := r.sqliteSessionForContext(root, hookInput.SessionID, branch)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	stateSection := buildMarkdownCurrentStateSection(runtime.RootPath(), firstNonEmpty(branch, session.Session.Branch))
	_, err = state.RecordSessionStateSnapshot(context.Background(), root, state.PathResolver{StateHome: r.StateHome}, state.SessionStateSnapshotOptions{
		SessionRef:       firstNonEmpty(session.Session.Alias, session.Session.ID),
		Content:          stateSection,
		ObservedBranch:   firstNonEmpty(branch, session.Session.Branch),
		ObservedWorktree: runtime.RootPath(),
	})
	return err
}

func (r Runner) runMarkdownSessionStateUpdate(runtime state.Runtime, root project.Root) error {
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" {
		return nil
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	if branch == "" {
		return nil
	}
	sessionPath, found := findMarkdownSessionForContext(root.Path(), hookInput.SessionID, branch)
	if !found {
		return nil
	}
	stateSection := buildMarkdownCurrentStateSection(runtime.RootPath(), branch)
	return writeMarkdownCurrentState(sessionPath, stateSection)
}

func buildMarkdownCurrentStateSection(worktree string, branch string) string {
	now := time.Now().UTC()
	lines := []string{
		fmt.Sprintf("## Current State (%s)", now.Format("2006-01-02 15:04")),
		"",
		fmt.Sprintf("Branch: %s", branch),
	}
	if commits, err := recentGitCommitSubjects(worktree, 1); err == nil && len(commits) > 0 && commits[0].Hash != "" {
		lines = append(lines, fmt.Sprintf("Last commit: %s - %s", commits[0].Hash, commits[0].Subject))
	}
	if uncommitted, err := gitUncommittedFileCount(worktree); err == nil && uncommitted > 0 {
		suffix := "s"
		if uncommitted == 1 {
			suffix = ""
		}
		lines = append(lines, fmt.Sprintf("Uncommitted: %d file%s", uncommitted, suffix))
	}
	return strings.Join(lines, "\n")
}

func gitUncommittedFileCount(worktree string) (int, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktree
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}

func writeMarkdownCurrentState(path string, stateSection string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := setFrontmatterScalar(body, "last_updated", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	frontmatter, markdownBody := splitMarkdownDocument(string(updated))
	markdownBody = upsertMarkdownCurrentState(markdownBody, stateSection)
	return os.WriteFile(path, []byte(frontmatter+markdownBody), 0o600)
}

func splitMarkdownDocument(text string) (string, string) {
	if strings.HasPrefix(text, "---\n") {
		if offset := strings.Index(text[len("---\n"):], "\n---\n"); offset >= 0 {
			end := len("---\n") + offset + len("\n---\n")
			return text[:end], text[end:]
		}
	}
	if strings.HasPrefix(text, "---\r\n") {
		if offset := strings.Index(text[len("---\r\n"):], "\r\n---\r\n"); offset >= 0 {
			end := len("---\r\n") + offset + len("\r\n---\r\n")
			return text[:end], text[end:]
		}
	}
	return "", text
}

func upsertMarkdownCurrentState(body string, stateSection string) string {
	lines := strings.Split(body, "\n")
	currentStateStart := -1
	journalStart := -1
	for i, line := range lines {
		switch {
		case currentStateStart < 0 && strings.HasPrefix(line, "## Current State"):
			currentStateStart = i
		case journalStart < 0 && strings.TrimSpace(line) == "## Journal":
			journalStart = i
		}
	}
	stateLines := append(strings.Split(stateSection, "\n"), "")
	if currentStateStart >= 0 {
		end := len(lines)
		for i := currentStateStart + 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "## ") {
				end = i
				break
			}
		}
		replaced := append([]string{}, lines[:currentStateStart]...)
		replaced = append(replaced, stateLines...)
		replaced = append(replaced, lines[end:]...)
		return strings.Join(trimExcessBlankLinesBeforeHeading(replaced), "\n")
	}
	if journalStart >= 0 {
		inserted := append([]string{}, lines[:journalStart]...)
		inserted = append(inserted, stateLines...)
		inserted = append(inserted, lines[journalStart:]...)
		return strings.Join(trimExcessBlankLinesBeforeHeading(inserted), "\n")
	}
	text := strings.TrimRight(body, "\n")
	if text == "" {
		return stateSection + "\n"
	}
	return text + "\n\n" + stateSection + "\n"
}

func trimExcessBlankLinesBeforeHeading(lines []string) []string {
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") && strings.TrimSpace(lines[i-1]) == "" {
			for i >= 2 && strings.TrimSpace(lines[i-2]) == "" {
				lines = append(lines[:i-2], lines[i-1:]...)
				i--
			}
		}
	}
	return lines
}

func (r Runner) runSessionContext(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		writeSessionContextHelp(out)
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		writeSessionContextHelp(out)
		return nil
	}
	switch args[0] {
	case "for-prompt", "--for-prompt":
		return r.runSessionContextForPrompt(args[1:], out)
	case "for-resumption", "--for-resumption":
		return r.runSessionContextForResumption(args[1:], out, runtime)
	case "for-compact", "--for-compact":
		return r.runSessionContextForCompact(args[1:], out, runtime)
	default:
		return unknownSubcommandError("session context", args[0])
	}
}

func writeSessionContextHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf session context <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Render session lifecycle context for hooks and compaction.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  for-prompt      Render implementation prompt context")
	fmt.Fprintln(out, "  for-resumption  Render post-compaction resumption context")
	fmt.Fprintln(out, "  for-compact     Render pre-compaction instructions")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help      Show help")
}

func (r Runner) runSessionContextForPrompt(args []string, out io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("session context for-prompt accepts no arguments")
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" {
		return nil
	}
	fmt.Fprintln(out, sessionContextPrompt())
	return nil
}

func (r Runner) runSessionContextForCompact(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) > 0 {
		return fmt.Errorf("session context for-compact accepts no arguments")
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runMarkdownSessionContextForCompact(out, runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" {
		return nil
	}
	_, err = state.LogJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalLogOptions{
		Entry:            "compact(session): context compaction triggered",
		ObservedBranch:   state.ObservedGitBranch(runtime.RootPath()),
		ObservedWorktree: runtime.RootPath(),
		HarnessSessionID: hookInput.SessionID,
		LinkSession:      true,
		IfSessionActive:  true,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, sessionContextCompactInstructions())
	return nil
}

func (r Runner) runMarkdownSessionContextForCompact(out io.Writer, runtime state.Runtime, root project.Root) error {
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" {
		return nil
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	sessionPath, found := findMarkdownSessionForContext(root.Path(), hookInput.SessionID, branch)
	if found {
		if err := appendMarkdownSessionJournalEntry(sessionPath, "compact(session): context compaction triggered"); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, sessionContextCompactInstructions())
	return nil
}

func findMarkdownSessionForContext(rootPath string, harnessSessionID string, branch string) (string, bool) {
	sessionsDir := filepath.Join(rootPath, ".agents", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", false
	}
	type candidate struct {
		path string
		when string
	}
	var branchCandidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(sessionsDir, entry.Name())
		fields, ok := markdownSessionFrontmatter(path)
		if !ok {
			continue
		}
		if harnessSessionID != "" && firstNonEmpty(fields["claude_session_id"], fields["harness_session_id"], fields["session_id"]) == harnessSessionID {
			return path, true
		}
		if branch != "" && fields["branch"] == branch && fields["status"] != "archived" {
			branchCandidates = append(branchCandidates, candidate{
				path: path,
				when: firstNonEmpty(fields["last_updated"], fields["last_entry"], fields["created"]),
			})
		}
	}
	if len(branchCandidates) == 0 {
		return "", false
	}
	sort.Slice(branchCandidates, func(i, j int) bool {
		return branchCandidates[i].when > branchCandidates[j].when
	})
	return branchCandidates[0].path, true
}

func markdownSessionFrontmatter(path string) (map[string]string, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	fields, ok := parseKnowledgeFrontmatter(body)
	if !ok {
		return nil, false
	}
	values := map[string]string{}
	for key, field := range fields {
		values[key] = firstFieldValue(field)
	}
	return values, true
}

func appendMarkdownSessionJournalEntry(path string, entry string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	updated, err := setFrontmatterScalar(body, "last_updated", now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	updated, err = setFrontmatterScalar(updated, "last_entry", now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	text := string(updated)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if !strings.Contains(text, "\n## Journal") {
		text += "\n## Journal\n\n"
	}
	text += fmt.Sprintf("[%s] %s\n", now.Format("2006-01-02 15:04"), entry)
	return os.WriteFile(path, []byte(text), 0o600)
}

func (r Runner) runSessionContextForResumption(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) > 0 {
		return fmt.Errorf("session context for-resumption accepts no arguments")
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return r.runMarkdownSessionContextForResumption(out, runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	session, found, err := r.sqliteSessionForContext(projectRoot, hookInput.SessionID, branch)
	if err != nil {
		return err
	}
	writeSessionResumptionContext(out, session, found, branch)
	return nil
}

func (r Runner) runMarkdownSessionContextForResumption(out io.Writer, runtime state.Runtime, root project.Root) error {
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	sessionPath, found := findMarkdownSessionForContext(root.Path(), hookInput.SessionID, branch)
	writeMarkdownSessionResumptionContext(out, root.Path(), sessionPath, found, branch)
	return nil
}

func writeMarkdownSessionResumptionContext(out io.Writer, rootPath string, sessionPath string, found bool, observedBranch string) {
	fmt.Fprintln(out, "=== POST-COMPACTION RESUMPTION ===")
	fmt.Fprintln(out)
	if !found {
		fmt.Fprintln(out, "WARNING: No active session found. Read .agents/sessions/ manually.")
		return
	}
	body, err := os.ReadFile(sessionPath)
	if err != nil {
		fmt.Fprintf(out, "WARNING: Could not read session file: %s\n", sessionPath)
		return
	}
	fields, _ := markdownSessionFrontmatter(sessionPath)
	rel := markdownSessionRelativePath(rootPath, sessionPath)
	fmt.Fprintf(out, "Session: %s\n", rel)
	if branch := firstNonEmpty(fields["branch"], observedBranch); branch != "" {
		fmt.Fprintf(out, "Branch: %s\n", branch)
	}
	if spec := fields["spec"]; spec != "" {
		fmt.Fprintf(out, "Spec: %s\n", spec)
	}
	fmt.Fprintln(out)
	content := markdownContentWithoutFrontmatter(string(body))
	if currentState := extractMarkdownCurrentState(content); currentState != "" {
		fmt.Fprintln(out, currentState)
	} else {
		fmt.Fprintln(out, "WARNING: No ## Current State was written before compaction.")
		fmt.Fprintln(out, "Read the session file's ## Journal section for context.")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## Recent Journal")
	fmt.Fprintln(out)
	entries := extractRecentMarkdownJournalEntries(content, 20)
	if len(entries) == 0 {
		fmt.Fprintln(out, "(no journal entries)")
	} else {
		for _, entry := range entries {
			fmt.Fprintln(out, entry)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "---")
	fmt.Fprintln(out, "Resume work from where the state summary left off.")
	fmt.Fprintln(out, "Do not ask 'where were we?' - the context above tells you.")
	fmt.Fprintf(out, "If you need more detail, read the full session file: %s\n", rel)
}

func markdownSessionRelativePath(rootPath string, sessionPath string) string {
	if rel, err := filepath.Rel(filepath.Join(rootPath, ".agents"), sessionPath); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Join(".agents", rel))
	}
	return sessionPath
}

type markdownSessionRecord struct {
	Alias          string
	Path           string
	RelativePath   string
	Fields         map[string]string
	Body           []byte
	Archived       bool
	JournalEntries []state.SessionJournalEntry
}

func markdownSessionList(rootPath string, options state.SessionListOptions) (state.SessionList, error) {
	records, err := collectMarkdownSessionRecords(rootPath, options.All)
	if err != nil {
		return state.SessionList{}, err
	}
	result := state.SessionList{Version: 1, Sessions: map[string]state.SessionItem{}}
	for _, record := range records {
		status := markdownSessionStatus(record)
		if !options.All && status == "archived" {
			continue
		}
		result.Sessions[record.Alias] = state.SessionItem{
			Branch:           record.Fields["branch"],
			Status:           status,
			HarnessSessionID: markdownSessionHarnessID(record.Fields),
			SourcePath:       record.RelativePath,
			JournalEntries:   len(record.JournalEntries),
		}
	}
	return result, nil
}

func markdownSessionShow(rootPath string, ref string) (state.SessionShow, error) {
	records, err := collectMarkdownSessionRecords(rootPath, true)
	if err != nil {
		return state.SessionShow{}, err
	}
	record, ok := resolveMarkdownSessionRecord(records, ref)
	if !ok {
		return state.SessionShow{}, fmt.Errorf("session %q not found in markdown sessions", ref)
	}
	hash := sha256.Sum256(record.Body)
	return state.SessionShow{
		Query: ref,
		Session: state.SessionDetail{
			ID:               record.Alias,
			Alias:            record.Alias,
			Branch:           record.Fields["branch"],
			Status:           markdownSessionStatus(record),
			HarnessSessionID: markdownSessionHarnessID(record.Fields),
			Sources: []state.TraceSource{{
				Path: record.RelativePath,
				Hash: hex.EncodeToString(hash[:]),
			}},
			JournalEntries: record.JournalEntries,
			Relationships:  []state.TraceRelationship{},
			CreatedAt:      firstNonEmpty(record.Fields["created"], record.Fields["last_entry"], record.Fields["last_updated"]),
			UpdatedAt:      firstNonEmpty(record.Fields["last_updated"], record.Fields["last_entry"], record.Fields["created"]),
		},
	}, nil
}

func markdownCompatibilitySummary(rootPath string, command string, mode string, action string, reason string) (compatibilityCommandSummary, error) {
	records, err := collectMarkdownSessionRecords(rootPath, true)
	if err != nil {
		return compatibilityCommandSummary{}, err
	}
	counts := map[string]int{"sessions": len(records)}
	for _, record := range records {
		status := markdownSessionStatus(record)
		if status == "" {
			status = "unknown"
		}
		counts[status]++
	}
	return compatibilityCommandSummary{
		ContractVersion: state.StateJSONContractVersion,
		Version:         1,
		Command:         command,
		Mode:            mode,
		Action:          action,
		Reason:          reason,
		Counts:          counts,
	}, nil
}

func (r Runner) runMarkdownSessionStart(options sessionStartOptions, out io.Writer, runtime state.Runtime, root project.Root) error {
	if os.Getenv("LOAF_ENRICHMENT") == "1" {
		return nil
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	if hookInput.AgentID != "" && !options.force {
		return nil
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	if branch == "" {
		return fmt.Errorf("session start requires a git branch")
	}
	harnessSessionID := firstNonEmpty(options.harnessSessionID, hookInput.SessionID)
	result, err := startMarkdownSession(root.Path(), branch, harnessSessionID)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSessionStart(out, branch, result)
	return nil
}

func (r Runner) runMarkdownSessionEnd(options sessionEndOptions, out io.Writer, runtime state.Runtime, root project.Root) error {
	if os.Getenv("LOAF_ENRICHMENT") == "1" {
		return nil
	}
	hookInput, err := r.readSessionHookInput()
	if err != nil {
		return err
	}
	branch := state.ObservedGitBranch(runtime.RootPath())
	harnessSessionID := firstNonEmpty(options.harnessSessionID, hookInput.SessionID)
	if branch == "" && harnessSessionID == "" {
		return fmt.Errorf("session end requires a git branch or harness session id")
	}
	result, err := endMarkdownSession(root.Path(), branch, harnessSessionID, markdownSessionEndOptions{
		ifActive: options.ifActive || options.fromHook,
		wrap:     options.wrap,
		clear:    hookInput.Reason == "clear",
	})
	if err != nil {
		if options.fromHook && strings.Contains(err.Error(), "no active session found") {
			return nil
		}
		return err
	}
	if options.fromHook && result.Action == state.SessionEndActionNoop && !options.jsonOutput {
		return nil
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSessionEnd(out, result)
	return nil
}

func startMarkdownSession(rootPath string, branch string, harnessSessionID string) (state.SessionStartResult, error) {
	if err := os.MkdirAll(filepath.Join(rootPath, ".agents", "sessions"), 0o755); err != nil {
		return state.SessionStartResult{}, fmt.Errorf("create markdown sessions directory: %w", err)
	}
	records, err := collectMarkdownSessionRecords(rootPath, false)
	if err != nil {
		return state.SessionStartResult{}, err
	}
	var existingByHarness *markdownSessionRecord
	var activeByBranch *markdownSessionRecord
	for i := range records {
		record := &records[i]
		if harnessSessionID != "" && markdownSessionHarnessID(record.Fields) == harnessSessionID {
			existingByHarness = record
			break
		}
		if activeByBranch == nil && record.Fields["branch"] == branch && markdownSessionStatus(*record) == "active" {
			activeByBranch = record
		}
	}
	existing := existingByHarness
	if existing == nil {
		existing = activeByBranch
	}

	now := time.Now().UTC()
	nowStamp := now.Format(time.RFC3339)
	journalTime := now.Format("2006-01-02 15:04")
	previous := (*state.TraceEntity)(nil)
	previousJournalIDs := []string{}
	action := state.SessionStartActionCreated
	if existing != nil && harnessSessionID != "" && markdownSessionHarnessID(existing.Fields) != "" && markdownSessionHarnessID(existing.Fields) != harnessSessionID {
		if err := appendMarkdownSessionLifecycleEntries(existing.Path, map[string]string{
			"status":       "stopped",
			"last_updated": nowStamp,
			"last_entry":   nowStamp,
		}, []string{
			fmt.Sprintf("[%s] session(end): closed by new conversation", journalTime),
			fmt.Sprintf("[%s] session(stop):   === SESSION STOPPED ===", journalTime),
		}, true); err != nil {
			return state.SessionStartResult{}, err
		}
		previous = &state.TraceEntity{Kind: "session", ID: existing.Alias, Alias: existing.Alias, Status: "stopped"}
		previousJournalIDs = []string{existing.Alias + ":end", existing.Alias + ":stop"}
		existing = nil
		action = state.SessionStartActionRotated
	}

	var active markdownSessionRecord
	journalIDs := []string{}
	if existing != nil {
		active = *existing
		action = state.SessionStartActionAlreadyActive
		updates := map[string]string{
			"branch":       branch,
			"status":       "active",
			"last_updated": nowStamp,
			"last_entry":   nowStamp,
		}
		if harnessSessionID != "" {
			updates["claude_session_id"] = harnessSessionID
		}
		var entries []string
		if markdownSessionStatus(active) != "active" {
			action = state.SessionStartActionResumed
			entries = append(entries, fmt.Sprintf("[%s] session(resume): %s", journalTime, markdownResumeMessage(harnessSessionID)))
			journalIDs = append(journalIDs, active.Alias+":resume")
		}
		if err := appendMarkdownSessionLifecycleEntries(active.Path, updates, entries, false); err != nil {
			return state.SessionStartResult{}, err
		}
		active.Fields["branch"] = branch
		active.Fields["status"] = "active"
		if harnessSessionID != "" {
			active.Fields["claude_session_id"] = harnessSessionID
		}
	} else {
		alias, path, err := nextMarkdownSessionPath(rootPath, now)
		if err != nil {
			return state.SessionStartResult{}, err
		}
		if err := writeNewMarkdownSession(path, branch, harnessSessionID, now); err != nil {
			return state.SessionStartResult{}, err
		}
		active = markdownSessionRecord{
			Alias: alias,
			Path:  path,
			Fields: map[string]string{
				"branch":            branch,
				"status":            "active",
				"claude_session_id": harnessSessionID,
			},
		}
		journalIDs = append(journalIDs, alias+":start")
	}
	return state.SessionStartResult{
		Version:             1,
		Action:              action,
		Session:             state.TraceEntity{Kind: "session", ID: active.Alias, Alias: active.Alias, Status: "active"},
		HarnessSessionID:    firstNonEmpty(harnessSessionID, markdownSessionHarnessID(active.Fields)),
		JournalEntryIDs:     journalIDs,
		PreviousSession:     previous,
		PreviousJournalIDs:  previousJournalIDs,
		PreviousSessionNote: markdownPreviousSessionNote(previous),
	}, nil
}

type markdownSessionEndOptions struct {
	ifActive bool
	wrap     bool
	clear    bool
}

func endMarkdownSession(rootPath string, branch string, harnessSessionID string, options markdownSessionEndOptions) (state.SessionEndResult, error) {
	records, err := collectMarkdownSessionRecords(rootPath, false)
	if err != nil {
		return state.SessionEndResult{}, err
	}
	var target *markdownSessionRecord
	for i := range records {
		record := &records[i]
		if harnessSessionID != "" && markdownSessionHarnessID(record.Fields) == harnessSessionID {
			target = record
			break
		}
		if target == nil && branch != "" && record.Fields["branch"] == branch && markdownSessionStatus(*record) == "active" {
			target = record
		}
	}
	if target == nil {
		if options.ifActive {
			return state.SessionEndResult{Version: 1, Action: state.SessionEndActionNoop, NoopReason: "no active session found"}, nil
		}
		return state.SessionEndResult{}, fmt.Errorf("no active session found")
	}
	status := markdownSessionStatus(*target)
	if status != "active" {
		if options.ifActive {
			return state.SessionEndResult{
				Version:          1,
				Action:           state.SessionEndActionNoop,
				Session:          state.TraceEntity{Kind: "session", ID: target.Alias, Alias: target.Alias, Status: status},
				HarnessSessionID: markdownSessionHarnessID(target.Fields),
				NoopReason:       fmt.Sprintf("session is %s", status),
			}, nil
		}
		return state.SessionEndResult{
			Version:          1,
			Action:           state.SessionEndActionAlreadyClosed,
			Session:          state.TraceEntity{Kind: "session", ID: target.Alias, Alias: target.Alias, Status: status},
			HarnessSessionID: markdownSessionHarnessID(target.Fields),
			NoopReason:       fmt.Sprintf("session is %s", status),
		}, nil
	}
	now := time.Now().UTC()
	nowStamp := now.Format(time.RFC3339)
	journalTime := now.Format("2006-01-02 15:04")
	action := state.SessionEndActionStopped
	nextStatus := "stopped"
	entries := []string{}
	journalIDs := []string{}
	switch {
	case options.clear:
		action = state.SessionEndActionCleared
		nextStatus = "active"
		entries = append(entries, fmt.Sprintf("[%s] session(clear):  === CONTEXT CLEARED ===", journalTime))
		journalIDs = append(journalIDs, target.Alias+":clear")
	case options.wrap:
		action = state.SessionEndActionDone
		nextStatus = "done"
		entries = append(entries, fmt.Sprintf("[%s] session(wrap): session ended", journalTime))
		journalIDs = append(journalIDs, target.Alias+":wrap")
	default:
		entries = append(entries,
			fmt.Sprintf("[%s] session(end): session ended", journalTime),
			fmt.Sprintf("[%s] session(stop):   === SESSION STOPPED ===", journalTime),
		)
		journalIDs = append(journalIDs, target.Alias+":end", target.Alias+":stop")
	}
	if err := appendMarkdownSessionLifecycleEntries(target.Path, map[string]string{
		"status":       nextStatus,
		"last_updated": nowStamp,
		"last_entry":   nowStamp,
	}, entries, true); err != nil {
		return state.SessionEndResult{}, err
	}
	return state.SessionEndResult{
		Version:          1,
		Action:           action,
		Session:          state.TraceEntity{Kind: "session", ID: target.Alias, Alias: target.Alias, Status: nextStatus},
		HarnessSessionID: firstNonEmpty(harnessSessionID, markdownSessionHarnessID(target.Fields)),
		JournalEntryIDs:  journalIDs,
	}, nil
}

func collectMarkdownSessionRecords(rootPath string, includeArchived bool) ([]markdownSessionRecord, error) {
	sessionsDir := filepath.Join(rootPath, ".agents", "sessions")
	records, err := readMarkdownSessionRecords(rootPath, sessionsDir, false)
	if err != nil {
		return nil, err
	}
	if includeArchived {
		archived, err := readMarkdownSessionRecords(rootPath, filepath.Join(sessionsDir, "archive"), true)
		if err != nil {
			return nil, err
		}
		records = append(records, archived...)
	}
	sort.Slice(records, func(i, j int) bool {
		left := firstNonEmpty(records[i].Fields["last_updated"], records[i].Fields["last_entry"], records[i].Fields["created"], records[i].Alias)
		right := firstNonEmpty(records[j].Fields["last_updated"], records[j].Fields["last_entry"], records[j].Fields["created"], records[j].Alias)
		if left == right {
			return records[i].Alias < records[j].Alias
		}
		return left > right
	})
	return records, nil
}

func readMarkdownSessionRecords(rootPath string, dir string, archived bool) ([]markdownSessionRecord, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read markdown sessions: %w", err)
	}
	records := []markdownSessionRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		record, ok, err := readMarkdownSessionRecord(rootPath, path, archived)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if !archived && markdownSessionStatus(record) == "archived" {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func readMarkdownSessionRecord(rootPath string, path string, archived bool) (markdownSessionRecord, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return markdownSessionRecord{}, false, fmt.Errorf("read markdown session %s: %w", path, err)
	}
	fields, ok := parseKnowledgeFrontmatter(body)
	if !ok {
		return markdownSessionRecord{}, false, nil
	}
	values := map[string]string{}
	for key, field := range fields {
		values[key] = firstFieldValue(field)
	}
	alias := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	content := markdownContentWithoutFrontmatter(string(body))
	return markdownSessionRecord{
		Alias:          alias,
		Path:           path,
		RelativePath:   markdownSessionRelativePath(rootPath, path),
		Fields:         values,
		Body:           body,
		Archived:       archived,
		JournalEntries: parseMarkdownSessionJournalEntries(alias, content),
	}, true, nil
}

func resolveMarkdownSessionRecord(records []markdownSessionRecord, ref string) (markdownSessionRecord, bool) {
	trimmed := strings.TrimSpace(ref)
	for _, record := range records {
		relativeWithoutExt := strings.TrimSuffix(record.RelativePath, filepath.Ext(record.RelativePath))
		switch {
		case record.Alias == trimmed:
			return record, true
		case record.RelativePath == trimmed:
			return record, true
		case relativeWithoutExt == trimmed:
			return record, true
		case record.Path == trimmed:
			return record, true
		case markdownSessionHarnessID(record.Fields) == trimmed:
			return record, true
		}
	}
	for _, record := range records {
		if record.Fields["branch"] == trimmed && markdownSessionStatus(record) != "archived" {
			return record, true
		}
	}
	return markdownSessionRecord{}, false
}

func markdownSessionStatus(record markdownSessionRecord) string {
	if record.Archived {
		return "archived"
	}
	return firstNonEmpty(record.Fields["status"], "active")
}

func markdownSessionHarnessID(fields map[string]string) string {
	return firstNonEmpty(fields["claude_session_id"], fields["harness_session_id"], fields["session_id"])
}

func parseMarkdownSessionJournalEntries(alias string, content string) []state.SessionJournalEntry {
	entryRE := regexp.MustCompile(`(?m)^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2})\]\s+(.+)$`)
	typedRE := regexp.MustCompile(`^([A-Za-z0-9_-]+)(?:\(([^)]*)\))?:\s*(.+)$`)
	matches := entryRE.FindAllStringSubmatch(content, -1)
	entries := make([]state.SessionJournalEntry, 0, len(matches))
	for i, match := range matches {
		entryType := "entry"
		scope := ""
		message := strings.TrimSpace(match[2])
		if typed := typedRE.FindStringSubmatch(message); typed != nil {
			entryType = typed[1]
			scope = strings.TrimSpace(typed[2])
			message = strings.TrimSpace(typed[3])
		}
		entries = append(entries, state.SessionJournalEntry{
			ID:        fmt.Sprintf("%s:%03d", alias, i+1),
			EntryType: entryType,
			Scope:     scope,
			Message:   message,
			CreatedAt: match[1],
		})
	}
	return entries
}

func nextMarkdownSessionPath(rootPath string, now time.Time) (string, string, error) {
	sessionsDir := filepath.Join(rootPath, ".agents", "sessions")
	stem := now.UTC().Format("20060102-150405") + "-session"
	for suffix := 0; ; suffix++ {
		alias := stem
		if suffix > 0 {
			alias = fmt.Sprintf("%s-%d", stem, suffix+1)
		}
		path := filepath.Join(sessionsDir, alias+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return alias, path, nil
		} else if err != nil {
			return "", "", fmt.Errorf("check markdown session path: %w", err)
		}
	}
}

func writeNewMarkdownSession(path string, branch string, harnessSessionID string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create markdown session directory: %w", err)
	}
	nowStamp := now.UTC().Format(time.RFC3339)
	journalTime := now.UTC().Format("2006-01-02 15:04")
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("status: active\n")
	fmt.Fprintf(&b, "branch: %s\n", branch)
	if harnessSessionID != "" {
		fmt.Fprintf(&b, "claude_session_id: %s\n", harnessSessionID)
	}
	fmt.Fprintf(&b, "created: %s\n", nowStamp)
	fmt.Fprintf(&b, "last_updated: %s\n", nowStamp)
	fmt.Fprintf(&b, "last_entry: %s\n", nowStamp)
	b.WriteString("---\n")
	b.WriteString("# Session\n\n")
	b.WriteString("## Journal\n\n")
	fmt.Fprintf(&b, "[%s] session(start):  %s\n", journalTime, markdownStartMessage(harnessSessionID))
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func appendMarkdownSessionLifecycleEntries(path string, fields map[string]string, entries []string, blankBefore bool) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := body
	for key, value := range fields {
		updated, err = setFrontmatterScalar(updated, key, value)
		if err != nil {
			return err
		}
	}
	text := string(updated)
	if len(entries) == 0 {
		return os.WriteFile(path, []byte(text), 0o600)
	}
	if !strings.Contains(text, "\n## Journal") {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n## Journal\n"
	}
	text = strings.TrimRight(text, "\n")
	separator := "\n"
	if blankBefore || strings.Contains(lastNonEmptyMarkdownLine(text), "session(stop):") {
		separator = "\n\n"
	}
	text += separator + strings.Join(entries, "\n") + "\n"
	return os.WriteFile(path, []byte(text), 0o600)
}

func markdownStartMessage(harnessSessionID string) string {
	if harnessSessionID == "" {
		return "=== SESSION STARTED ==="
	}
	return fmt.Sprintf("=== SESSION STARTED === (session %s)", shortMarkdownHarnessSessionID(harnessSessionID))
}

func markdownResumeMessage(harnessSessionID string) string {
	if harnessSessionID == "" {
		return "=== SESSION RESUMED ==="
	}
	return fmt.Sprintf("=== SESSION RESUMED === (session %s)", shortMarkdownHarnessSessionID(harnessSessionID))
}

func shortMarkdownHarnessSessionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func markdownPreviousSessionNote(previous *state.TraceEntity) string {
	if previous == nil {
		return ""
	}
	return "previous active session stopped because a different harness session started on the same branch"
}

func markdownContentWithoutFrontmatter(text string) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	rest := text[len("---\n"):]
	if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
		return rest[idx+len("\n---\n"):]
	}
	return text
}

func extractMarkdownCurrentState(content string) string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## Current State") {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

func extractRecentMarkdownJournalEntries(content string, limit int) []string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## Journal") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	entryRE := regexp.MustCompile(`(?m)^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}\] .+$`)
	entries := entryRE.FindAllString(strings.Join(lines[start:end], "\n"), -1)
	if limit > 0 && len(entries) > limit {
		return entries[len(entries)-limit:]
	}
	return entries
}

func (r Runner) sqliteSessionForContext(root project.Root, harnessSessionID string, branch string) (state.SessionShow, bool, error) {
	sessions, err := state.ListSessions(context.Background(), root, state.PathResolver{StateHome: r.StateHome}, state.SessionListOptions{All: true})
	if err != nil {
		return state.SessionShow{}, false, err
	}
	if harnessSessionID != "" {
		for alias, session := range sessions.Sessions {
			if session.HarnessSessionID == harnessSessionID {
				show, err := state.ShowSession(context.Background(), root, state.PathResolver{StateHome: r.StateHome}, alias)
				return show, true, err
			}
		}
	}
	if branch != "" {
		for alias, session := range sessions.Sessions {
			if session.Branch == branch && session.Status == "active" {
				show, err := state.ShowSession(context.Background(), root, state.PathResolver{StateHome: r.StateHome}, alias)
				return show, true, err
			}
		}
	}
	return state.SessionShow{}, false, nil
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
		result, err := markdownSessionShow(projectRoot.Path(), ref)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeSessionShow(out, result)
		return nil
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
		sessions, err := markdownSessionList(projectRoot.Path(), options.filters)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, sessions)
		}
		writeSessionList(out, sessions, options.filters)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	sessions, err := state.ListSessions(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	sessions.Diagnostics = stateListWarnings(status.Diagnostics)
	if options.jsonOutput {
		return writeJSON(out, sessions)
	}
	writeSessionList(out, sessions, options.filters)
	return nil
}

func (r Runner) runSessionLog(args []string, out io.Writer, runtime state.Runtime) error {
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
		return r.runMarkdownSessionLog(options, out, runtime, projectRoot)
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	if options.fromHook {
		hookInput, err := r.readSessionHookInput()
		if err != nil {
			return err
		}
		if hookInput.Raw == nil && options.harnessSessionID == "" {
			return nil
		}
		options.harnessSessionID = firstNonEmpty(options.harnessSessionID, hookInput.SessionID)
		if options.entry == "" && hookInput.Raw != nil {
			entry, ok := deriveSessionHookLogEntry(hookInput)
			if !ok {
				return nil
			}
			options.entry = entry
		}
	}
	linkSession := options.fromHook
	if options.detectLinear {
		entry, ok, disabled, err := detectLinearJournalEntry(projectRoot.Path(), runtime.RootPath())
		if err != nil {
			return err
		}
		if disabled {
			return nil
		}
		if !ok {
			fmt.Fprintln(out, "No Linear magic words detected in recent commits")
			return nil
		}
		options.entry = entry
		linkSession = true
	}
	result, err := state.LogJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalLogOptions{
		Entry:            options.entry,
		ObservedBranch:   state.ObservedGitBranch(runtime.RootPath()),
		ObservedWorktree: runtime.RootPath(),
		HarnessSessionID: options.harnessSessionID,
		LinkSession:      linkSession,
		IfSessionActive:  options.fromHook,
	})
	if err != nil {
		return err
	}
	if options.fromHook && result.ID == "" && !options.jsonOutput {
		return nil
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "logged journal entry: %s\n", result.ID)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runMarkdownSessionLog(options sessionLogOptions, out io.Writer, runtime state.Runtime, root project.Root) error {
	branch := state.ObservedGitBranch(runtime.RootPath())
	if branch == "" {
		return fmt.Errorf("not in a git repository")
	}
	hookSessionID := ""
	if options.fromHook {
		hookInput, err := r.readSessionHookInput()
		if err != nil {
			return err
		}
		if hookInput.Raw == nil && options.harnessSessionID == "" {
			return nil
		}
		hookSessionID = hookInput.SessionID
		if options.entry == "" && hookInput.Raw != nil {
			entry, ok := deriveSessionHookLogEntry(hookInput)
			if !ok {
				return nil
			}
			options.entry = entry
		}
	}
	if options.detectLinear {
		entry, ok, disabled, err := detectLinearJournalEntry(root.Path(), runtime.RootPath())
		if err != nil {
			return err
		}
		if disabled {
			return nil
		}
		if !ok {
			fmt.Fprintln(out, "No Linear magic words detected in recent commits")
			return nil
		}
		options.entry = entry
	}
	if !validMarkdownSessionLogEntry(options.entry) {
		return fmt.Errorf("invalid entry format; use type(scope): description")
	}
	resolution := resolveMarkdownSessionForLog(root.Path(), branch, options.harnessSessionID, hookSessionID)
	sessionPath, found := resolution.Path, resolution.Found
	if !found {
		if !options.fromHook {
			fmt.Fprintf(r.sessionLogErrWriter(), "WARN: no session for branch '%s'; no active sessions to fall back to. Pass --session-id <id> to silence.\n", branch)
		}
		if options.fromHook {
			return nil
		}
		return fmt.Errorf("no active session found for branch %s; run `loaf session start` first", branch)
	}
	if resolution.Adoption == "most-recent-active" && !options.fromHook {
		fmt.Fprintf(r.sessionLogErrWriter(), "WARN: no session for branch '%s'; logging to most-recent active session '%s' (origin branch '%s'). Pass --session-id <id> to silence.\n", branch, filepath.Base(sessionPath), resolution.OriginBranch)
	}
	result, err := appendMarkdownSessionLogEntry(sessionPath, options.entry)
	if err != nil {
		return err
	}
	if result.DidResume {
		fmt.Fprintln(out, "Auto-resumed stopped session")
	}
	if options.jsonOutput {
		entryType, scope, message := splitMarkdownSessionLogEntry(options.entry)
		return writeJSON(out, state.JournalLogResult{
			ID:               markdownSessionRelativePath(root.Path(), sessionPath),
			EntryType:        entryType,
			Scope:            scope,
			Message:          message,
			ObservedBranch:   branch,
			ObservedWorktree: runtime.RootPath(),
			HarnessSessionID: firstNonEmpty(options.harnessSessionID, hookSessionID),
		})
	}
	fmt.Fprintf(out, "Logged: %s\n", options.entry)
	return nil
}

func (r Runner) sessionLogErrWriter() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

type markdownSessionResolution struct {
	Path         string
	Found        bool
	OriginBranch string
	Adoption     string
}

func resolveMarkdownSessionForLog(rootPath string, branch string, explicitSessionID string, hookSessionID string) markdownSessionResolution {
	if explicitSessionID != "" {
		if path, found := findMarkdownSessionForContext(rootPath, explicitSessionID, ""); found {
			return markdownSessionResolution{Path: path, Found: true}
		}
	}
	if hookSessionID != "" && hookSessionID != explicitSessionID {
		if path, found := findMarkdownSessionForContext(rootPath, hookSessionID, ""); found {
			return markdownSessionResolution{Path: path, Found: true}
		}
	}
	if path, found := findMarkdownSessionForContext(rootPath, "", branch); found {
		return markdownSessionResolution{Path: path, Found: true, OriginBranch: branch}
	}
	if path, originBranch, found := findMostRecentActiveMarkdownSession(rootPath); found {
		return markdownSessionResolution{Path: path, Found: true, OriginBranch: originBranch, Adoption: "most-recent-active"}
	}
	return markdownSessionResolution{}
}

func findMostRecentActiveMarkdownSession(rootPath string) (string, string, bool) {
	sessionsDir := filepath.Join(rootPath, ".agents", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", "", false
	}
	type candidate struct {
		path   string
		branch string
		when   string
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(sessionsDir, entry.Name())
		fields, ok := markdownSessionFrontmatter(path)
		if !ok || fields["status"] != "active" {
			continue
		}
		candidates = append(candidates, candidate{
			path:   path,
			branch: fields["branch"],
			when:   firstNonEmpty(fields["last_updated"], fields["last_entry"], fields["created"]),
		})
	}
	if len(candidates) == 0 {
		return "", "", false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].when == candidates[j].when {
			return candidates[i].path < candidates[j].path
		}
		return candidates[i].when > candidates[j].when
	})
	return candidates[0].path, candidates[0].branch, true
}

func validMarkdownSessionLogEntry(entry string) bool {
	entryType, _, message := splitMarkdownSessionLogEntry(entry)
	if entryType == "" || message == "" {
		return false
	}
	validTypes := map[string]bool{
		"session": true,
		"start":   true, "resume": true, "pause": true, "clear": true, "progress": true, "commit": true, "pr": true, "merge": true,
		"decision": true, "discover": true, "finding": true, "block": true, "unblock": true,
		"spark": true, "todo": true, "assume": true,
		"branch": true, "task": true, "linear": true, "hypothesis": true, "try": true, "reject": true, "compact": true,
		"skill": true, "wrap": true,
		"idea": true, "spec": true, "report": true, "council": true, "brainstorm": true, "plan": true, "draft": true,
	}
	return validTypes[entryType]
}

func splitMarkdownSessionLogEntry(entry string) (string, string, string) {
	re := regexp.MustCompile(`^([a-z]+)(?:\(([^)]+)\))?:\s*(.+)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(entry))
	if matches == nil {
		return "", "", ""
	}
	return matches[1], strings.TrimSpace(matches[2]), strings.TrimSpace(matches[3])
}

type markdownSessionLogResult struct {
	DidResume bool
}

func appendMarkdownSessionLogEntry(path string, entry string) (markdownSessionLogResult, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return markdownSessionLogResult{}, err
	}
	now := time.Now().UTC()
	fields, ok := markdownSessionFrontmatter(path)
	if !ok {
		return markdownSessionLogResult{}, fmt.Errorf("frontmatter not found")
	}
	updated, err := setFrontmatterScalar(body, "last_updated", now.Format(time.RFC3339))
	if err != nil {
		return markdownSessionLogResult{}, err
	}
	updated, err = setFrontmatterScalar(updated, "last_entry", now.Format(time.RFC3339))
	if err != nil {
		return markdownSessionLogResult{}, err
	}
	didResume := fields["status"] == "stopped"
	if didResume {
		updated, err = setFrontmatterScalar(updated, "status", "active")
		if err != nil {
			return markdownSessionLogResult{}, err
		}
	}
	text := string(updated)
	if !strings.Contains(text, "\n## Journal") {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n## Journal\n"
	}
	text = strings.TrimRight(text, "\n")
	var entries []string
	if didResume {
		entries = append(entries, fmt.Sprintf("[%s] session(resume): === SESSION RESUMED ===", now.Format("2006-01-02 15:04")))
	}
	entries = append(entries, fmt.Sprintf("[%s] %s", now.Format("2006-01-02 15:04"), entry))
	separator := "\n"
	if didResume || strings.Contains(lastNonEmptyMarkdownLine(text), "session(stop):") {
		separator = "\n\n"
	}
	text += separator + strings.Join(entries, "\n") + "\n"
	return markdownSessionLogResult{DidResume: didResume}, os.WriteFile(path, []byte(text), 0o600)
}

func lastNonEmptyMarkdownLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

func detectLinearJournalEntry(projectRoot string, worktree string) (string, bool, bool, error) {
	if linearIntegrationDisabled(projectRoot) {
		return "", false, true, nil
	}
	commits, err := recentGitCommitSubjects(worktree, 3)
	if err != nil {
		return "", false, false, fmt.Errorf("scan recent commits: %w", err)
	}
	magicPattern := regexp.MustCompile(`(?i)\b(fixe?s?d?|close?s?d?|resolve?s?d?)\s+([A-Z]+-\d+)`)
	seen := map[string]bool{}
	var issues []string
	for _, commit := range commits {
		matches := magicPattern.FindAllStringSubmatch(commit.Subject, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			issueID := match[2]
			if seen[issueID] {
				continue
			}
			seen[issueID] = true
			issues = append(issues, issueID)
		}
	}
	if len(issues) == 0 {
		return "", false, false, nil
	}
	return "discover(linear): found magic words for " + strings.Join(issues, ", "), true, false, nil
}

type recentGitCommit struct {
	Hash    string
	Subject string
}

func recentGitCommitSubjects(worktree string, count int) ([]recentGitCommit, error) {
	if count <= 0 {
		return nil, nil
	}
	cmd := exec.Command("git", "log", fmt.Sprintf("-%d", count), "--pretty=format:%h%x00%s")
	cmd.Dir = worktree
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}
	var commits []recentGitCommit
	for _, line := range strings.Split(trimmed, "\n") {
		parts := strings.SplitN(line, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, recentGitCommit{Hash: parts[0], Subject: parts[1]})
	}
	return commits, nil
}

func linearIntegrationDisabled(projectRoot string) bool {
	path := filepath.Join(projectRoot, ".agents", "loaf.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var config struct {
		Integrations struct {
			Linear struct {
				Enabled *bool `json:"enabled"`
			} `json:"linear"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}
	return config.Integrations.Linear.Enabled != nil && !*config.Integrations.Linear.Enabled
}

func (r Runner) runSessionReport(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("session report", args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := state.ExportSessionMarkdown(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		result.Command = "session report"
		return writeJSON(out, result)
	}
	fmt.Fprint(out, result.Content)
	return nil
}

type sessionHookInput struct {
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id"`
	Source    string         `json:"source"`
	Reason    string         `json:"reason"`
	Raw       map[string]any `json:"-"`
}

func (r Runner) readSessionHookInput() (sessionHookInput, error) {
	reader := r.Stdin
	if reader == nil {
		info, err := os.Stdin.Stat()
		if err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
			reader = os.Stdin
		}
	}
	if reader == nil {
		return sessionHookInput{}, nil
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return sessionHookInput{}, fmt.Errorf("read session hook input: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return sessionHookInput{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return sessionHookInput{}, fmt.Errorf("parse session hook input: %w", err)
	}
	input := sessionHookInput{
		SessionID: stringMapValue(raw, "session_id"),
		AgentID:   stringMapValue(raw, "agent_id"),
		Source:    stringMapValue(raw, "source"),
		Reason:    stringMapValue(raw, "reason"),
		Raw:       raw,
	}
	return input, nil
}

func deriveSessionHookLogEntry(input sessionHookInput) (string, bool) {
	raw := input.Raw
	if raw == nil {
		return "", false
	}
	hookEventName := stringMapValue(raw, "hook_event_name")
	toolName := firstNonEmpty(stringMapValue(raw, "tool_name"), nestedStringMapValue(raw, "tool", "name"))
	if hookEventName == "TaskCompleted" || toolName == "TaskCompleted" {
		description := firstNonEmpty(stringMapValue(raw, "task_description"), stringMapValue(raw, "task_subject"), "task")
		return "task(completed): " + description, true
	}
	command := firstNonEmpty(
		nestedStringMapValue(raw, "tool_input", "command"),
		nestedStringMapValue(raw, "input", "command"),
		nestedStringMapValue(raw, "tool", "input", "command"),
	)
	if command == "" {
		if commit := stringMapValue(raw, "commit"); commit != "" {
			return fmt.Sprintf("commit(%s): %s", commit, firstNonEmpty(stringMapValue(raw, "message"), "commit")), true
		}
		if pr := stringMapValue(raw, "pr"); pr != "" {
			return fmt.Sprintf("pr(create): %s (#%s)", firstNonEmpty(stringMapValue(raw, "title"), "PR created"), pr), true
		}
		if merge := stringMapValue(raw, "merge"); merge != "" {
			return fmt.Sprintf("pr(merge): #%s", merge), true
		}
		return "", false
	}
	switch {
	case strings.Contains(command, "git commit"):
		message := commandFlagValue(command, "-m")
		if message == "" {
			message = "commit"
		}
		return fmt.Sprintf("commit(unknown): %s", message), true
	case strings.Contains(command, "gh pr create"):
		title := commandFlagValue(command, "--title")
		if title == "" {
			title = "PR created"
		}
		return "pr(create): " + title, true
	case strings.Contains(command, "gh pr merge"):
		return "pr(merge): #unknown", true
	default:
		return "", false
	}
}

func commandFlagValue(command string, flag string) string {
	prefixes := []string{flag + " \"", flag + " '"}
	for _, prefix := range prefixes {
		_, after, ok := strings.Cut(command, prefix)
		if !ok {
			continue
		}
		quote := strings.TrimPrefix(prefix, flag+" ")
		value, _, ok := strings.Cut(after, quote)
		if ok {
			return value
		}
	}
	return ""
}

func stringMapValue(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func nestedStringMapValue(values map[string]any, keys ...string) string {
	current := values
	for i, key := range keys {
		value, ok := current[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			if text, ok := value.(string); ok {
				return text
			}
			return ""
		}
		next, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func writeSessionStart(out io.Writer, branch string, result state.SessionStartResult) {
	fmt.Fprint(out, "\n  loaf session start\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  Branch: %s\n", branch)
	fmt.Fprintf(out, "  Action: %s\n", result.Action)
	fmt.Fprintf(out, "  Session: %s\n", firstNonEmpty(result.Session.Alias, result.Session.ID))
	if result.HarnessSessionID != "" {
		fmt.Fprintf(out, "  Harness session: %s\n", result.HarnessSessionID)
	}
	if result.PreviousSession != nil {
		fmt.Fprintf(out, "  Previous session: %s stopped\n", firstNonEmpty(result.PreviousSession.Alias, result.PreviousSession.ID))
	}
	for _, warning := range []string{result.PreviousSessionNote} {
		if warning != "" {
			fmt.Fprintf(out, "  Note: %s\n", warning)
		}
	}
	fmt.Fprintln(out)
}

func writeSessionEnd(out io.Writer, result state.SessionEndResult) {
	fmt.Fprint(out, "\n  loaf session end\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  Action: %s\n", result.Action)
	if result.NoopReason != "" {
		fmt.Fprintf(out, "  Reason: %s\n", result.NoopReason)
	}
	if result.Session.ID != "" {
		fmt.Fprintf(out, "  Session: %s\n", firstNonEmpty(result.Session.Alias, result.Session.ID))
		fmt.Fprintf(out, "  Status: %s\n", result.Session.Status)
	}
	if result.HarnessSessionID != "" {
		fmt.Fprintf(out, "  Harness session: %s\n", result.HarnessSessionID)
	}
	fmt.Fprintln(out)
}

func writeSessionArchive(out io.Writer, result state.SessionArchiveResult) {
	fmt.Fprint(out, "\n  loaf session archive\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  Action: %s\n", result.Action)
	fmt.Fprintf(out, "  Session: %s\n", firstNonEmpty(result.Session.Alias, result.Session.ID))
	fmt.Fprintf(out, "  Status: %s\n", result.Session.Status)
	if result.HarnessSessionID != "" {
		fmt.Fprintf(out, "  Harness session: %s\n", result.HarnessSessionID)
	}
	fmt.Fprintln(out)
}

func writeSessionList(out io.Writer, sessions state.SessionList, filters state.SessionListOptions) {
	active := sortedSessionsByArchivedState(sessions, false)
	archived := sortedSessionsByArchivedState(sessions, true)

	fmt.Fprint(out, "\n  loaf session list\n\n")
	writeProjectMutationContext(out, "  ", sessions.DatabaseScope, sessions.DatabasePath, sessions.ProjectID, sessions.ProjectName, sessions.ProjectCurrentPath)
	writeStateDiagnostics(out, "  ", sessions.Diagnostics)
	if sessionListHasContext(sessions) {
		fmt.Fprintln(out)
	}
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
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
	if len(args) == 0 || isHelpArg(args) {
		writeReportHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":     writeReportListHelp,
		"generate": writeReportGenerateHelp,
		"create":   writeReportCreateHelp,
		"finalize": writeReportFinalizeHelp,
		"archive":  writeReportArchiveHelp,
	}) {
		return nil
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
		return unknownSubcommandError("report", args[0])
	}
}

func sessionListHasContext(sessions state.SessionList) bool {
	return sessions.DatabaseScope != "" ||
		sessions.DatabasePath != "" ||
		sessions.ProjectID != "" ||
		sessions.ProjectName != "" ||
		sessions.ProjectCurrentPath != "" ||
		len(sessions.Diagnostics) > 0
}

func writeReportHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf report <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage durable reports in native SQLite state or markdown compatibility mode.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  list      List reports")
	fmt.Fprintln(out, "  generate  Generate read-only markdown exports")
	fmt.Fprintln(out, "  create    Create a report")
	fmt.Fprintln(out, "  finalize  Finalize a report")
	fmt.Fprintln(out, "  archive   Archive a report")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeReportListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report list [--type <type>|--status <status>] [--json]", "List reports.", "--type       Filter by report type", "--status     Filter by status; Loaf lifecycle statuses: draft, final, archived", "--json       Output reports, diagnostics, global database scope, and project identity as JSON")
}

func writeReportGenerateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report generate <kind> [ref] [--format markdown] [--json]", "Generate a read-only markdown report.", "--format     Output format: markdown", "--json       Output contract, command, project context, and markdown content as JSON")
}

func writeReportCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report create <slug> [--type <type>] [--source <source>] [--json]", "Create a report.", "--type       Report type", "--source     Report source", "--json       Output created report, event, global database scope, and project identity as JSON")
}

func writeReportFinalizeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report finalize <report> [--json]", "Finalize a report.", "--json       Output report status transition, event, global database scope, and project identity as JSON")
}

func writeReportArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report archive <report> [--json]", "Archive a report.", "--json       Output report status transition, event, global database scope, and project identity as JSON")
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
		reports, err := markdownReportList(projectRoot.Path(), options.filters)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, reports)
		}
		writeReportList(out, reports)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	reports, err := state.ListReports(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	reports.Diagnostics = stateListWarnings(status.Diagnostics)
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
		if options.jsonOutput {
			return err
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if options.jsonOutput {
		result.Command = "report generate " + options.kind
		return writeJSON(out, result)
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
		options, err := parseReportCreateArgs(args)
		if err != nil {
			return err
		}
		result, err := markdownReportCreate(projectRoot.Path(), options.create)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeReportCreate(out, result)
		return nil
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
		result, err := markdownReportFinalize(projectRoot.Path(), ref)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeReportStatus(out, "finalized", result)
		return nil
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
		result, err := markdownReportArchive(projectRoot.Path(), ref)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeReportStatus(out, "archived", result)
		return nil
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
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "status: %s\n", result.Report.Status)
	fmt.Fprintf(out, "type: %s\n", result.Kind)
	fmt.Fprintf(out, "source: %s\n", result.Source)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeReportStatus(out io.Writer, action string, result state.ReportStatusResult) {
	fmt.Fprintf(out, "%s report %s: %s\n", action, firstNonEmpty(result.Report.Alias, result.Report.ID), result.Report.Title)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "previous: %s\n", result.Previous)
	fmt.Fprintf(out, "status: %s\n", result.Status)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeReportList(out io.Writer, reports state.ReportList) {
	fmt.Fprint(out, "\n  loaf report list\n\n")
	writeProjectMutationContext(out, "  ", reports.DatabaseScope, reports.DatabasePath, reports.ProjectID, reports.ProjectName, reports.ProjectCurrentPath)
	writeStateDiagnostics(out, "  ", reports.Diagnostics)

	if len(reports.Reports) == 0 {
		if reportListHasContext(reports) {
			fmt.Fprintln(out)
		}
		fmt.Fprint(out, "  No reports found.\n\n")
		return
	}

	if reportListHasContext(reports) {
		fmt.Fprintln(out)
	}
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

func reportListHasContext(reports state.ReportList) bool {
	return reports.DatabaseScope != "" ||
		reports.DatabasePath != "" ||
		reports.ProjectID != "" ||
		reports.ProjectName != "" ||
		reports.ProjectCurrentPath != "" ||
		len(reports.Diagnostics) > 0
}

func markdownReportList(rootPath string, options state.ReportListOptions) (state.ReportList, error) {
	agentsDir := filepath.Join(rootPath, ".agents")
	files, err := filepath.Glob(filepath.Join(agentsDir, "reports", "*.md"))
	if err != nil {
		return state.ReportList{}, fmt.Errorf("find markdown reports: %w", err)
	}
	archivedFiles, err := filepath.Glob(filepath.Join(agentsDir, "reports", "archive", "*.md"))
	if err != nil {
		return state.ReportList{}, fmt.Errorf("find archived markdown reports: %w", err)
	}
	files = append(files, archivedFiles...)
	sort.Strings(files)

	reports := state.ReportList{Version: 1, Reports: map[string]state.ReportItem{}}
	for _, path := range files {
		item, alias, err := readMarkdownReport(rootPath, path)
		if err != nil {
			return state.ReportList{}, err
		}
		if !reportMatchesFilters(item, options) {
			continue
		}
		reports.Reports[alias] = item
	}
	return reports, nil
}

func readMarkdownReport(rootPath string, path string) (state.ReportItem, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return state.ReportItem{}, "", fmt.Errorf("read markdown report %s: %w", path, err)
	}
	frontmatter, _ := parseKnowledgeFrontmatter(body)
	field := func(keys ...string) string {
		for _, key := range keys {
			if value := firstFieldValue(frontmatter[key]); value != "" {
				return value
			}
		}
		return ""
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	alias := firstNonEmpty(field("id"), stem)
	status := firstNonEmpty(field("status"), "unknown")
	sourcePath := markdownReportSourcePath(rootPath, path)
	if strings.HasPrefix(sourcePath, ".agents/reports/archive/") {
		status = "archived"
	}
	title := firstNonEmpty(field("title"), firstMarkdownHeading(markdownContentWithoutFrontmatter(string(body))), alias)
	kind := firstNonEmpty(field("type", "report_kind", "kind"), "markdown")
	return state.ReportItem{
		Title:      title,
		Kind:       kind,
		Status:     status,
		SourcePath: sourcePath,
	}, alias, nil
}

func markdownReportSourcePath(rootPath string, path string) string {
	rel, err := filepath.Rel(rootPath, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func firstMarkdownHeading(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func reportMatchesFilters(item state.ReportItem, options state.ReportListOptions) bool {
	if options.Type != "" && item.Kind != options.Type {
		return false
	}
	if options.Status != "" && item.Status != options.Status {
		return false
	}
	return true
}

func markdownReportCreate(rootPath string, options state.ReportCreateOptions) (state.ReportCreateResult, error) {
	slug := strings.TrimSpace(options.Slug)
	if slug == "" {
		return state.ReportCreateResult{}, fmt.Errorf("report create requires a slug")
	}
	kind := strings.TrimSpace(options.Kind)
	if kind == "" {
		kind = "research"
	}
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = "ad-hoc"
	}
	reportsDir := filepath.Join(rootPath, ".agents", "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return state.ReportCreateResult{}, fmt.Errorf("create reports directory: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	safeType := sanitizeMarkdownReportPathSegment(kind)
	safeSlug := sanitizeMarkdownReportPathSegment(slug)
	if safeSlug == "" {
		return state.ReportCreateResult{}, fmt.Errorf("report create requires a slug")
	}
	filename := fmt.Sprintf("%s-%s-%s.md", time.Now().Format("20060102-150405"), safeType, safeSlug)
	path := filepath.Join(reportsDir, filename)
	if _, err := os.Stat(path); err == nil {
		return state.ReportCreateResult{}, fmt.Errorf("report file %s already exists", filename)
	} else if err != nil && !os.IsNotExist(err) {
		return state.ReportCreateResult{}, fmt.Errorf("inspect report file %s: %w", filename, err)
	}
	title := markdownReportTitleFromSlug(slug)
	frontmatter := map[string]frontmatterField{
		"title":   markdownReportFrontmatterScalar(title),
		"type":    markdownReportFrontmatterScalar(kind),
		"created": markdownReportFrontmatterScalar(now),
		"status":  markdownReportFrontmatterScalar("draft"),
		"source":  markdownReportFrontmatterScalar(source),
		"tags":    {Array: true, Set: true},
	}
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, markdownReportBody(title))), 0o600); err != nil {
		return state.ReportCreateResult{}, fmt.Errorf("write report %s: %w", filename, err)
	}
	alias := strings.TrimSuffix(filename, filepath.Ext(filename))
	return state.ReportCreateResult{
		Report: state.TraceEntity{
			Kind:   "report",
			ID:     alias,
			Alias:  alias,
			Title:  title,
			Status: "draft",
		},
		Kind:   kind,
		Source: source,
	}, nil
}

func markdownReportFinalize(rootPath string, ref string) (state.ReportStatusResult, error) {
	path, err := resolveMarkdownReportFile(rootPath, ref)
	if err != nil {
		return state.ReportStatusResult{}, err
	}
	frontmatter, body, alias, err := readMarkdownReportDocument(path)
	if err != nil {
		return state.ReportStatusResult{}, err
	}
	previous := firstNonEmpty(firstFieldValue(frontmatter["status"]), "unknown")
	if previous != "draft" {
		return state.ReportStatusResult{}, fmt.Errorf("report %q is not draft (status: %s)", ref, previous)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	frontmatter["status"] = markdownReportFrontmatterScalar("final")
	frontmatter["finalized_at"] = markdownReportFrontmatterScalar(now)
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, body)), 0o600); err != nil {
		return state.ReportStatusResult{}, fmt.Errorf("write finalized report %s: %w", ref, err)
	}
	title := firstNonEmpty(firstFieldValue(frontmatter["title"]), firstMarkdownHeading(body), alias)
	return state.ReportStatusResult{
		Report:   state.TraceEntity{Kind: "report", ID: alias, Alias: alias, Title: title, Status: "final"},
		Previous: previous,
		Status:   "final",
	}, nil
}

func markdownReportArchive(rootPath string, ref string) (state.ReportStatusResult, error) {
	path, err := resolveMarkdownReportFile(rootPath, ref)
	if err != nil {
		return state.ReportStatusResult{}, err
	}
	frontmatter, body, alias, err := readMarkdownReportDocument(path)
	if err != nil {
		return state.ReportStatusResult{}, err
	}
	previous := firstNonEmpty(firstFieldValue(frontmatter["status"]), "unknown")
	if previous != "final" {
		return state.ReportStatusResult{}, fmt.Errorf("report %q is not final (status: %s)", ref, previous)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	frontmatter["status"] = markdownReportFrontmatterScalar("archived")
	frontmatter["archived_at"] = markdownReportFrontmatterScalar(now)
	frontmatter["archived_by"] = markdownReportFrontmatterScalar("cli")
	reportsDir := filepath.Join(rootPath, ".agents", "reports")
	archiveDir := filepath.Join(reportsDir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return state.ReportStatusResult{}, fmt.Errorf("create report archive directory: %w", err)
	}
	destPath := filepath.Join(archiveDir, filepath.Base(path))
	if _, err := os.Stat(destPath); err == nil {
		return state.ReportStatusResult{}, fmt.Errorf("archived report %s already exists", filepath.Base(path))
	} else if err != nil && !os.IsNotExist(err) {
		return state.ReportStatusResult{}, fmt.Errorf("inspect archived report %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(destPath, []byte(renderMarkdownReport(frontmatter, body)), 0o600); err != nil {
		return state.ReportStatusResult{}, fmt.Errorf("write archived report %s: %w", ref, err)
	}
	if err := os.Remove(path); err != nil {
		return state.ReportStatusResult{}, fmt.Errorf("remove active report %s: %w", ref, err)
	}
	title := firstNonEmpty(firstFieldValue(frontmatter["title"]), firstMarkdownHeading(body), alias)
	return state.ReportStatusResult{
		Report:   state.TraceEntity{Kind: "report", ID: alias, Alias: alias, Title: title, Status: "archived"},
		Previous: previous,
		Status:   "archived",
	}, nil
}

func readMarkdownReportDocument(path string) (map[string]frontmatterField, string, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("read markdown report %s: %w", path, err)
	}
	parsed, _ := parseKnowledgeFrontmatter(content)
	body := markdownContentWithoutFrontmatter(string(content))
	alias := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return parsed, body, alias, nil
}

func resolveMarkdownReportFile(rootPath string, ref string) (string, error) {
	reportsDir := filepath.Join(rootPath, ".agents", "reports")
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("report ref is required")
	}
	if filepath.IsAbs(ref) {
		clean := filepath.Clean(ref)
		if pathInsideDir(clean, reportsDir) {
			if _, err := os.Stat(clean); err == nil {
				return clean, nil
			} else if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("inspect report %s: %w", ref, err)
			}
		}
	}
	if strings.Contains(ref, string(filepath.Separator)) || strings.Contains(ref, "/") || strings.Contains(ref, "\\") {
		clean := filepath.Clean(filepath.Join(reportsDir, filepath.FromSlash(ref)))
		if !pathInsideDir(clean, reportsDir) {
			return "", fmt.Errorf("report %q is outside .agents/reports", ref)
		}
		if _, err := os.Stat(clean); err == nil {
			return clean, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect report %s: %w", ref, err)
		}
	}
	direct := filepath.Join(reportsDir, ref)
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect report %s: %w", ref, err)
	}
	files, err := filepath.Glob(filepath.Join(reportsDir, "*.md"))
	if err != nil {
		return "", fmt.Errorf("find markdown reports: %w", err)
	}
	sort.Strings(files)
	var matches []string
	for _, path := range files {
		if strings.Contains(filepath.Base(path), ref) {
			matches = append(matches, path)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("report not found: %s", ref)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, match := range matches {
			names = append(names, filepath.Base(match))
		}
		return "", fmt.Errorf("ambiguous report %q: %s", ref, strings.Join(names, ", "))
	}
}

func pathInsideDir(path string, dir string) bool {
	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	resolvedDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(resolvedDir, resolvedPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func renderMarkdownReport(frontmatter map[string]frontmatterField, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range []string{"title", "type", "created", "status", "source", "finalized_at", "archived_at", "archived_by"} {
		if field := frontmatter[key]; field.Set {
			writeMarkdownReportFrontmatterField(&b, key, field)
		}
	}
	if field := frontmatter["tags"]; field.Set {
		writeMarkdownReportFrontmatterField(&b, "tags", field)
	} else {
		b.WriteString("tags: []\n")
	}
	var extraKeys []string
	for key, field := range frontmatter {
		if !field.Set || key == "title" || key == "type" || key == "created" || key == "status" || key == "source" || key == "finalized_at" || key == "archived_at" || key == "archived_by" || key == "tags" {
			continue
		}
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		writeMarkdownReportFrontmatterField(&b, key, frontmatter[key])
	}
	b.WriteString("---\n")
	if !strings.HasPrefix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func markdownReportFrontmatterScalar(value string) frontmatterField {
	return frontmatterField{Values: []string{value}, Set: true}
}

func writeMarkdownReportFrontmatterField(b *strings.Builder, key string, field frontmatterField) {
	if field.Array {
		if len(field.Values) == 0 {
			fmt.Fprintf(b, "%s: []\n", key)
			return
		}
		values := make([]string, 0, len(field.Values))
		for _, value := range field.Values {
			values = append(values, quoteYAMLString(value))
		}
		fmt.Fprintf(b, "%s: [%s]\n", key, strings.Join(values, ", "))
		return
	}
	fmt.Fprintf(b, "%s: %s\n", key, quoteYAMLString(firstFieldValue(field)))
}

func quoteYAMLString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func markdownReportBody(title string) string {
	return fmt.Sprintf(`# %s

## Question

_What question does this report answer?_

## Summary

_Executive summary of findings._

## Key Findings

- _Finding 1_
- _Finding 2_
- _Finding 3_

## Methodology

_How was this research conducted?_

## Detailed Analysis

_Full analysis goes here._

## Recommendations

- _Recommendation 1_
- _Recommendation 2_

## Sources

- _Source 1_

## Open Questions

- _Question 1_
`, title)
}

func sanitizeMarkdownReportPathSegment(input string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", `"`, "-", "<", "-", ">", "-", "|", "-")
	clean := replacer.Replace(input)
	clean = strings.TrimLeft(clean, ".")
	return clean
}

func markdownReportTitleFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
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

func parseStatePathArgs(args []string) (statePathOptions, error) {
	var options statePathOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--verbose":
			options.verboseOutput = true
		default:
			return statePathOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.jsonOutput && options.verboseOutput {
		return statePathOptions{}, fmt.Errorf("state path cannot combine --json and --verbose")
	}
	return options, nil
}

func parseStateBackupVerifyArgs(args []string) (backupVerifyOptions, error) {
	var options backupVerifyOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return backupVerifyOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			if options.path != "" {
				return backupVerifyOptions{}, fmt.Errorf("state backup verify accepts exactly one backup path")
			}
			options.path = arg
		}
	}
	if options.path == "" {
		return backupVerifyOptions{}, fmt.Errorf("state backup verify requires a backup path")
	}
	return options, nil
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
	jsonAliasRequested := false
	var positional []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--format":
			value, err := consumeFlagValue(args, &i, "state export --format")
			if err != nil {
				return stateExportOptions{}, err
			}
			if jsonAliasRequested && value != state.ExportFormatJSON {
				return stateExportOptions{}, fmt.Errorf("state export all cannot combine --json with --format %s", value)
			}
			options.format = value
		case strings.HasPrefix(arg, "--format="):
			value := strings.TrimPrefix(arg, "--format=")
			if jsonAliasRequested && value != state.ExportFormatJSON {
				return stateExportOptions{}, fmt.Errorf("state export all cannot combine --json with --format %s", value)
			}
			options.format = value
		case arg == "--json":
			jsonAliasRequested = true
			if options.kind != state.ExportKindAll {
				return stateExportOptions{}, fmt.Errorf("state export %s uses --format markdown; --json is only supported for state export all", options.kind)
			}
			if options.format != "" && options.format != state.ExportFormatJSON {
				return stateExportOptions{}, fmt.Errorf("state export all cannot combine --json with --format %s", options.format)
			}
			options.format = state.ExportFormatJSON
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

func stateExportJSONRequested(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--format":
			if i+1 < len(args) && args[i+1] == state.ExportFormatJSON {
				return true
			}
			i++
		case strings.HasPrefix(arg, "--format="):
			if strings.TrimPrefix(arg, "--format=") == state.ExportFormatJSON {
				return true
			}
		case arg == "--json":
			return true
		}
	}
	return false
}

func parseDoctorArgs(args []string) (bool, bool, bool, error) {
	jsonOutput := false
	fix := false
	dryRun := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--fix":
			fix = true
		case "--dry-run":
			dryRun = true
		default:
			return false, false, false, fmt.Errorf("unknown option %q", arg)
		}
	}
	return jsonOutput, fix, dryRun, nil
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

type sessionStartOptions struct {
	jsonOutput       bool
	force            bool
	harnessSessionID string
}

type sessionEndOptions struct {
	jsonOutput       bool
	ifActive         bool
	wrap             bool
	fromHook         bool
	harnessSessionID string
}

type sessionArchiveOptions struct {
	jsonOutput       bool
	branch           string
	harnessSessionID string
}

type sessionLogOptions struct {
	jsonOutput       bool
	fromHook         bool
	detectLinear     bool
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
	kind       string
	ref        string
	format     string
	jsonOutput bool
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
				return taskListOptions{}, fmt.Errorf("invalid status %q (valid: %s)", value, validTaskListStatusText())
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
				return taskCreateOptions{}, fmt.Errorf("invalid priority %q (valid: %s)", value, validTaskPriorityText())
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
				return taskUpdateOptions{}, fmt.Errorf("invalid status %q (valid: %s)", value, validTaskStatusText())
			}
			options.update.Status = value
			options.update.SetStatus = true
		case "--priority":
			value, err := consumeFlagValue(args, &i, "--priority")
			if err != nil {
				return taskUpdateOptions{}, err
			}
			if !state.ValidTaskPriority(value) {
				return taskUpdateOptions{}, fmt.Errorf("invalid priority %q (valid: %s)", value, validTaskPriorityText())
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
		case "--from":
			value, err := consumeFlagValue(args, &i, "--from")
			if err != nil {
				return linkMutationOptions{}, err
			}
			options.from = value
		case "--to":
			value, err := consumeFlagValue(args, &i, "--to")
			if err != nil {
				return linkMutationOptions{}, err
			}
			options.to = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return linkMutationOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) > 0 {
		if options.from != "" || options.to != "" {
			return linkMutationOptions{}, fmt.Errorf("%s cannot mix positional entities with --from or --to", command)
		}
		if len(positional) != 2 {
			return linkMutationOptions{}, fmt.Errorf("%s requires a source entity and target entity", command)
		}
		options.from = positional[0]
		options.to = positional[1]
	}
	if options.from == "" || options.to == "" {
		return linkMutationOptions{}, fmt.Errorf("%s requires a source entity and target entity", command)
	}
	if options.relationshipType == "" {
		return linkMutationOptions{}, fmt.Errorf("%s requires --type", command)
	}
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

func parseSessionStartArgs(args []string) (sessionStartOptions, error) {
	var options sessionStartOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--force":
			options.force = true
		case "--resume":
		case "--session-id", "--harness-session-id":
			value, err := consumeFlagValue(args, &i, args[i])
			if err != nil {
				return sessionStartOptions{}, err
			}
			options.harnessSessionID = value
		default:
			return sessionStartOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseSessionEndArgs(args []string) (sessionEndOptions, error) {
	var options sessionEndOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--if-active":
			options.ifActive = true
		case "--wrap":
			options.wrap = true
		case "--from-hook":
			options.fromHook = true
		case "--session-id", "--harness-session-id":
			value, err := consumeFlagValue(args, &i, args[i])
			if err != nil {
				return sessionEndOptions{}, err
			}
			options.harnessSessionID = value
		default:
			return sessionEndOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseSessionArchiveArgs(args []string) (sessionArchiveOptions, error) {
	var options sessionArchiveOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--branch":
			value, err := consumeFlagValue(args, &i, "--branch")
			if err != nil {
				return sessionArchiveOptions{}, err
			}
			options.branch = value
		case "--session-id", "--harness-session-id":
			value, err := consumeFlagValue(args, &i, args[i])
			if err != nil {
				return sessionArchiveOptions{}, err
			}
			options.harnessSessionID = value
		default:
			return sessionArchiveOptions{}, fmt.Errorf("unknown option %q", args[i])
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
		case "--from-hook":
			options.fromHook = true
		case "--detect-linear":
			options.detectLinear = true
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
	if len(entryParts) > 1 || (len(entryParts) == 0 && !options.fromHook && !options.detectLinear) {
		return sessionLogOptions{}, fmt.Errorf("session log accepts exactly one entry")
	}
	if len(entryParts) == 1 {
		options.entry = entryParts[0]
	}
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
	options := reportGenerateOptions{format: state.ExportFormatMarkdown}
	positional := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			options.jsonOutput = true
		case arg == "--format":
			value, err := consumeFlagValue(args, &i, "--format")
			if err != nil {
				return reportGenerateOptions{}, err
			}
			options.format = value
		case strings.HasPrefix(arg, "--format="):
			options.format = strings.TrimPrefix(arg, "--format=")
		case strings.HasPrefix(arg, "-"):
			return reportGenerateOptions{}, fmt.Errorf("unknown option %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return reportGenerateOptions{}, fmt.Errorf("report generate requires a kind")
	}
	if options.format != state.ExportFormatMarkdown {
		return reportGenerateOptions{}, fmt.Errorf("report generate supports only --format markdown")
	}
	options.kind = positional[0]
	refs := positional[1:]
	switch options.kind {
	case state.ExportKindSession:
		if len(refs) != 1 {
			return reportGenerateOptions{}, fmt.Errorf("report generate session requires exactly one session")
		}
		options.ref = refs[0]
	case state.ExportKindTriage, state.ExportKindReleaseReadiness:
		if len(refs) != 0 {
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
	statuses := state.TaskListStatuses()
	if !filters.Active {
		return statuses
	}
	return statuses[:len(statuses)-2]
}

func validTaskStatusText() string {
	return strings.Join(state.TaskStatuses(), ", ")
}

func validTaskListStatusText() string {
	return strings.Join(state.TaskListStatuses(), ", ")
}

func validTaskPriorityText() string {
	return strings.Join(state.TaskPriorities(), ", ")
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

func sessionContextPrompt() string {
	return strings.Join([]string{
		"[Implementation Principles]",
		"- When the user's message is a QUESTION, answer it and STOP. Do not implement anything.",
		"  Wait for explicit instructions before taking action.",
		"- Create a Task BEFORE any tool use that changes something (Edit, Write, Bash, etc.).",
		"  No threshold - if it mutates, track it. TaskCompleted events auto-log to the session journal.",
		"  Create tasks before starting work, update status as you go, mark complete when done.",
		"- Delegate code changes to agents - orchestrator coordinates, doesn't implement",
		"- Log decisions: loaf session log \"decision(scope): description\"",
		"- One concern per agent, parallel when independent",
		"- Keep session handoff-ready",
	}, "\n")
}

func sessionContextCompactInstructions() string {
	return strings.Join([]string{
		"CONTEXT COMPACTION IMMINENT: Your conversation context will be compacted soon.",
		"",
		"REQUIRED - two actions before the model responds:",
		"",
		"1. **Flush journal entries.** Log all unrecorded decisions, discoveries, and progress:",
		"   - `decision(scope): key decisions made this session`",
		"   - `discover(scope): important findings`",
		"   - `finding(scope): analysis result`",
		"   Run `loaf session log \"type(scope): description\"` for each.",
		"",
		"2. **Write state summary.** SQLite resumption uses session journal entries as the durable source of truth.",
		"   If a markdown Current State section exists for compatibility, refresh it as handoff context.",
		"   Use this shape:",
		"",
		"   ```",
		"   ## Current State (YYYY-MM-DD HH:MM)",
		"",
		"   **Working on:** spec/task - brief description",
		"   **Status:** one-line build/test/progress status",
		"",
		"   **Done this session:**",
		"   - bullet per significant change",
		"",
		"   **Blocked:** (omit if none)",
		"   - blockers with context",
		"",
		"   **Next:**",
		"   - immediate follow-ups",
		"   ```",
		"",
		"   Write it as if briefing a colleague who just walked in. Be specific (file names, commit hashes,",
		"   flag names), not vague.",
		"",
		"The journal IS your external memory. Entries not flushed now are lost forever.",
	}, "\n")
}

func writeSessionResumptionContext(out io.Writer, result state.SessionShow, found bool, observedBranch string) {
	fmt.Fprintln(out, "=== POST-COMPACTION RESUMPTION ===")
	fmt.Fprintln(out)
	if !found {
		fmt.Fprintln(out, "WARNING: No active session found. Run `loaf session list --all` for available SQLite sessions.")
		return
	}
	session := result.Session
	fmt.Fprintf(out, "Session: %s\n", firstNonEmpty(session.Alias, session.ID))
	if branch := firstNonEmpty(session.Branch, observedBranch); branch != "" {
		fmt.Fprintf(out, "Branch: %s\n", branch)
	}
	for _, source := range session.Sources {
		fmt.Fprintf(out, "Source: %s\n", source.Path)
	}
	fmt.Fprintln(out)
	if session.StateSnapshot != nil && strings.TrimSpace(session.StateSnapshot.Content) != "" {
		fmt.Fprintln(out, session.StateSnapshot.Content)
	} else {
		fmt.Fprintln(out, "WARNING: No SQLite session state snapshot was written before compaction.")
		fmt.Fprintln(out, "Use the recent journal below as the durable resumption context.")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## Recent Journal")
	fmt.Fprintln(out)
	entries := lastSessionJournalEntries(session.JournalEntries, 20)
	if len(entries) == 0 {
		fmt.Fprintln(out, "(no journal entries)")
	} else {
		for _, entry := range entries {
			fmt.Fprintf(out, "%s\n", formatSessionJournalEntry(entry))
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "---")
	fmt.Fprintln(out, "Resume work from where the journal left off.")
	fmt.Fprintln(out, "Do not ask 'where were we?' - the context above tells you.")
	fmt.Fprintf(out, "If you need more detail, run `loaf session show %s`.\n", firstNonEmpty(session.Alias, session.ID))
}

func lastSessionJournalEntries(entries []state.SessionJournalEntry, limit int) []state.SessionJournalEntry {
	if limit <= 0 || len(entries) <= limit {
		return entries
	}
	return entries[len(entries)-limit:]
}

func formatSessionJournalEntry(entry state.SessionJournalEntry) string {
	timestamp := formatSessionJournalTimestamp(entry.CreatedAt)
	if entry.Scope != "" {
		return fmt.Sprintf("[%s] %s(%s): %s", timestamp, entry.EntryType, entry.Scope, entry.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", timestamp, entry.EntryType, entry.Message)
}

func formatSessionJournalTimestamp(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format("2006-01-02 15:04")
	}
	if len(value) >= 16 {
		return strings.ReplaceAll(value[:16], "T", " ")
	}
	return value
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

type storageHomeMigrationOptions struct {
	jsonOutput bool
	apply      bool
	dryRun     bool
}

type relationshipOriginRepairOptions struct {
	jsonOutput bool
	apply      bool
	dryRun     bool
	origin     string
}

type legacyProjectDatabaseRepairOptions struct {
	jsonOutput bool
	apply      bool
	dryRun     bool
}

func parseMarkdownMigrationArgs(args []string, command string) (markdownMigrationOptions, error) {
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
		return markdownMigrationOptions{}, fmt.Errorf("%s cannot combine --apply and --dry-run", command)
	}
	if options.resume && options.dryRun {
		return markdownMigrationOptions{}, fmt.Errorf("%s cannot combine --resume and --dry-run", command)
	}
	if options.resume && options.apply {
		return markdownMigrationOptions{}, fmt.Errorf("%s cannot combine --resume and --apply", command)
	}
	return options, nil
}

func parseStorageHomeMigrationArgs(args []string, command string) (storageHomeMigrationOptions, error) {
	var options storageHomeMigrationOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		default:
			return storageHomeMigrationOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return storageHomeMigrationOptions{}, fmt.Errorf("%s cannot combine --apply and --dry-run", command)
	}
	return options, nil
}

func parseLegacyProjectDatabaseRepairArgs(args []string) (legacyProjectDatabaseRepairOptions, error) {
	var options legacyProjectDatabaseRepairOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		default:
			return legacyProjectDatabaseRepairOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return legacyProjectDatabaseRepairOptions{}, fmt.Errorf("state repair legacy-project-database cannot combine --apply and --dry-run")
	}
	return options, nil
}

func parseRelationshipOriginRepairArgs(args []string) (relationshipOriginRepairOptions, error) {
	var options relationshipOriginRepairOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		case "--origin":
			value, err := consumeFlagValue(args, &i, arg)
			if err != nil {
				return relationshipOriginRepairOptions{}, err
			}
			options.origin = value
		default:
			return relationshipOriginRepairOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return relationshipOriginRepairOptions{}, fmt.Errorf("state repair relationship-origin cannot combine --apply and --dry-run")
	}
	if options.origin == "" {
		return relationshipOriginRepairOptions{}, fmt.Errorf("state repair relationship-origin requires --origin imported|manual")
	}
	if options.origin != "imported" && options.origin != "manual" {
		return relationshipOriginRepairOptions{}, fmt.Errorf("relationship origin must be imported or manual")
	}
	return options, nil
}

func parseProjectRenameArgs(args []string) (projectRenameOptions, error) {
	options := projectRenameOptions{}
	values := []string{}
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--dry-run":
			options.dryRun = true
		default:
			values = append(values, arg)
		}
	}
	if len(values) == 0 {
		return projectRenameOptions{}, fmt.Errorf("project rename requires a name")
	}
	if len(values) > 1 {
		return projectRenameOptions{}, fmt.Errorf("project rename accepts exactly one name")
	}
	options.name = values[0]
	return options, nil
}

func parseProjectMoveArgs(args []string, currentPath string) (projectMoveOptions, error) {
	options := projectMoveOptions{toPath: currentPath}
	positionals := []string{}
	fromFlagSet := false
	toFlagSet := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--dry-run":
			options.dryRun = true
		case "--from":
			i++
			if i >= len(args) {
				return projectMoveOptions{}, fmt.Errorf("--from requires a value")
			}
			options.fromPath = args[i]
			fromFlagSet = true
		case "--to":
			i++
			if i >= len(args) {
				return projectMoveOptions{}, fmt.Errorf("--to requires a value")
			}
			options.toPath = args[i]
			toFlagSet = true
		default:
			if strings.HasPrefix(arg, "-") {
				return projectMoveOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) > 0 {
		if fromFlagSet || toFlagSet {
			return projectMoveOptions{}, fmt.Errorf("project move cannot mix positional paths with --from or --to")
		}
		if len(positionals) > 2 {
			return projectMoveOptions{}, fmt.Errorf("project move accepts at most <from> and [to] paths")
		}
		options.fromPath = positionals[0]
		if len(positionals) == 2 {
			options.toPath = positionals[1]
		}
	}
	if options.fromPath == "" {
		return projectMoveOptions{}, fmt.Errorf("project move requires <from> or --from")
	}
	if options.toPath == "" {
		return projectMoveOptions{}, fmt.Errorf("project move requires [to], --to, or a current project root")
	}
	if !filepath.IsAbs(options.fromPath) || !filepath.IsAbs(options.toPath) {
		return projectMoveOptions{}, fmt.Errorf("project move requires absolute from and to paths")
	}
	options.fromPath = filepath.Clean(options.fromPath)
	options.toPath = filepath.Clean(options.toPath)
	return options, nil
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeJSONCommandError(out io.Writer, command string, err error) error {
	if writeErr := writeJSON(out, commandErrorJSON{
		ContractVersion: state.StateJSONContractVersion,
		Command:         command,
		Error:           err.Error(),
	}); writeErr != nil {
		return writeErr
	}
	return ExitError{Code: 1}
}

func writeStateBackupVerifyJSONError(out io.Writer, backupPath string, err error) error {
	if writeErr := writeJSON(out, commandErrorJSON{
		ContractVersion: state.StateJSONContractVersion,
		Command:         "state backup verify",
		Error:           err.Error(),
		BackupPath:      backupPath,
	}); writeErr != nil {
		return writeErr
	}
	return ExitError{Code: 1}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
