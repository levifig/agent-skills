package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	// BuildCommit and BuildDate carry optional build metadata injected at link
	// time (see cmd/loaf/main.go). They are empty for plain builds, `go run`,
	// and `go test`, and surface in `loaf --version` / `loaf version` when set.
	BuildCommit string
	BuildDate   string
	// Executable optionally overrides os.Executable as the source of
	// installed-distribution provenance (see distribution.go). Tests inject
	// fixture layouts through it; nil means the real executable.
	Executable func() (string, error)
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
	dryRun     bool
	sessionRef string
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
	jsonSet    bool
}

type stateBackupOptions struct {
	destination    string
	destinationSet bool
	jsonOutput     bool
	jsonSet        bool
}

type stateBackupRestoreOptions struct {
	backupPath     string
	restorePath    string
	restorePathSet bool
	jsonOutput     bool
	jsonSet        bool
}

type restoreEphemeralsOptions struct {
	target     string
	jsonOutput bool
}

type verifyEphemeralsOptions struct {
	target     string
	jsonOutput bool
}

type commandErrorJSON struct {
	ContractVersion               int                        `json:"contract_version"`
	Command                       string                     `json:"command"`
	Error                         string                     `json:"error"`
	BackupPath                    string                     `json:"backup_path,omitempty"`
	RestorePath                   string                     `json:"restore_path,omitempty"`
	RequestedDestinationDirectory string                     `json:"requested_destination_directory,omitempty"`
	ResolvedDestinationDirectory  string                     `json:"resolved_destination_directory,omitempty"`
	RecoveryTier                  string                     `json:"recovery_tier,omitempty"`
	SHA256                        string                     `json:"sha256,omitempty"`
	SourceSHA256                  string                     `json:"source_sha256,omitempty"`
	RestoredSHA256                string                     `json:"restored_sha256,omitempty"`
	BackupResult                  *state.BackupResult        `json:"backup_result,omitempty"`
	RestoreResult                 *state.BackupRestoreResult `json:"restore_result,omitempty"`
	BackupVerified                bool                       `json:"backup_verified"`
	Code                          string                     `json:"code,omitempty"`
	Field                         string                     `json:"field,omitempty"`
	Ref                           string                     `json:"ref,omitempty"`
	Path                          string                     `json:"path,omitempty"`
	DatabasePath                  string                     `json:"database_path,omitempty"`
	CurrentVersion                int                        `json:"current_version,omitempty"`
	RequiredVersion               int                        `json:"required_version,omitempty"`
	PendingVersions               []int                      `json:"pending_versions,omitempty"`
	Suggestion                    string                     `json:"suggestion,omitempty"`
	JournalSearchParity           *state.JournalSearchParity `json:"journal_search_parity,omitempty"`
	CurrentPath                   string                     `json:"current_path,omitempty"`
	KnownCurrentPaths             []string                   `json:"known_current_paths,omitempty"`
	Suggestions                   []commandErrorSuggestion   `json:"suggestions,omitempty"`
}

type commandErrorSuggestion struct {
	Action  string `json:"action"`
	Command string `json:"command"`
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

type searchOptions struct {
	query       string
	allProjects bool
	limit       int
	jsonOutput  bool
}

type docsIndexCLIOptions struct {
	rebuild    bool
	jsonOutput bool
}

type findingCreateCLIOptions struct {
	jsonOutput bool
	create     state.FindingCreateOptions
	body       bodyInputOptions
}

type findingListCLIOptions struct {
	jsonOutput bool
	format     string
	filters    state.FindingListOptions
}

type findingShowCLIOptions struct {
	ref        string
	jsonOutput bool
	format     string
}

type findingVerdictCLIOptions struct {
	jsonOutput bool
	verdict    state.FindingVerdictOptions
}

type findingImportJSONCLIOptions struct {
	jsonOutput bool
	importJSON state.FindingImportJSONOptions
}

type runCreateCLIOptions struct {
	jsonOutput bool
	create     state.RunCreateOptions
}

type runListCLIOptions struct {
	jsonOutput bool
	filters    state.RunListOptions
}

type runCompleteCLIOptions struct {
	jsonOutput bool
	complete   state.RunCompleteOptions
}

type renderSweepOptions struct {
	jsonOutput bool
	dryRun     bool
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
		dispatchErr = r.runVersion(out)
	case "build":
		dispatchErr = r.runBuild(args[1:], out, runtime.RootPath())
	case "init":
		dispatchErr = r.runInit(args[1:], out, runtime.RootPath())
	case "install":
		dispatchErr = r.runInstall(args[1:], out, runtime.RootPath())
	case "config":
		dispatchErr = r.runConfig(args[1:], out, runtime.RootPath())
	case "migrate":
		dispatchErr = r.runMigrate(args[1:], out, runtime)
	case "release":
		dispatchErr = r.runRelease(args[1:], out, runtime.RootPath())
	case "render":
		dispatchErr = r.runRender(args[1:], out, runtime)
	case "setup":
		dispatchErr = r.runSetup(args[1:], out, runtime.RootPath())
	case "state":
		dispatchErr = r.runState(args[1:], out, runtime)
	case "project":
		dispatchErr = r.runProject(args[1:], out, runtime)
	case "search":
		dispatchErr = r.runSearch(args[1:], out, runtime)
	case "docs":
		dispatchErr = r.runDocs(args[1:], out, runtime)
	case "finding":
		dispatchErr = r.runFinding(args[1:], out, runtime)
	case "run":
		dispatchErr = r.runRun(args[1:], out, runtime)
	case "trace":
		dispatchErr = r.runTrace(args[1:], out, runtime)
	case "brainstorm":
		dispatchErr = r.runBrainstorm(args[1:], out, runtime)
	case "plan":
		dispatchErr = r.runArtifactEntityCommand("plan", args[1:], out, runtime)
	case "handoff":
		dispatchErr = r.runArtifactEntityCommand("handoff", args[1:], out, runtime)
	case "council":
		dispatchErr = r.runArtifactEntityCommand("council", args[1:], out, runtime)
	case "idea":
		dispatchErr = r.runIdea(args[1:], out, runtime)
	case "intent":
		dispatchErr = r.runIntent(args[1:], out, runtime)
	case "intake":
		dispatchErr = r.runIntake(args[1:], out, runtime)
	case "exploration":
		dispatchErr = r.runExploration(args[1:], out, runtime)
	case "conversation":
		dispatchErr = r.runConversation(args[1:], out, runtime)
	case "spark":
		dispatchErr = r.runSpark(args[1:], out, runtime)
	case "tag":
		dispatchErr = r.runTag(args[1:], out, runtime)
	case "bundle":
		dispatchErr = r.runBundle(args[1:], out, runtime)
	case "change":
		dispatchErr = r.runChange(args[1:], out, runtime)
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
	case "journal":
		dispatchErr = r.runJournal(args[1:], out, runtime)
	case "task":
		dispatchErr = r.runTask(args[1:], out, runtime)
	case "housekeeping":
		dispatchErr = r.runHousekeeping(args[1:], out, runtime)
	case "kb":
		dispatchErr = r.runKb(args[1:], out, runtime.RootPath())
	case "version":
		dispatchErr = r.runVersion(out)
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
	fmt.Fprintln(out, "  config        Validate and refresh project Loaf config")
	fmt.Fprintln(out, "  setup         Initialize, build, and install")
	fmt.Fprintln(out, "  state         Manage native SQLite state")
	fmt.Fprintln(out, "  project       Manage project identity")
	fmt.Fprintln(out, "  search        Search SQLite-resident artifacts and journals")
	fmt.Fprintln(out, "  docs          Manage docs/ indexing")
	fmt.Fprintln(out, "  finding       Manage report findings and verdicts")
	fmt.Fprintln(out, "  run           Manage provenance runs")
	fmt.Fprintln(out, "  migrate       Run migration workflows")
	fmt.Fprintln(out, "  render        Maintain durable markdown renders")
	fmt.Fprintln(out, "  journal       Record and read the project journal")
	fmt.Fprintln(out, "  intent        Manage tracked Intent")
	fmt.Fprintln(out, "  exploration   Manage Exploration continuity")
	fmt.Fprintln(out, "  conversation  Manage conversation provenance")
	fmt.Fprintln(out, "  intake        Read the local intake projection")
	fmt.Fprintln(out, "  task          Manage tasks")
	fmt.Fprintln(out, "  spec          Manage specs")
	fmt.Fprintln(out, "  report        Manage reports")
	fmt.Fprintln(out, "  plan          Manage plans")
	fmt.Fprintln(out, "  handoff       Manage handoffs")
	fmt.Fprintln(out, "  council       Manage councils")
	fmt.Fprintln(out, "  kb            Manage knowledge base")
	fmt.Fprintln(out, "  change        Manage shape-first Change artifacts")
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

func (r Runner) runRender(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeRenderHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"sweep": writeRenderSweepHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "sweep":
		return r.runRenderSweep(args[1:], out, runtime)
	default:
		return unknownSubcommandError("render", args[0])
	}
}

func writeRenderHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf render <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Maintain committed durable Markdown renders.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  sweep  Upgrade committed durable renders to the current renderer contract")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeRenderSweepHelp(out io.Writer) {
	writeUsageHelp(out, "loaf render sweep [--dry-run] [--json]", "Upgrade committed durable renders to the current renderer contract.", "--dry-run    Report upgrade-needed files without rewriting them", "--json       Output scanned files, upgrade counts, drift counts, and target contract as JSON")
}

func (r Runner) runRenderSweep(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseRenderSweepArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := state.SweepDurableRenderContracts(projectRoot, state.DurableRenderSweepOptions{DryRun: options.dryRun})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		if err := writeJSON(out, result); err != nil {
			return err
		}
	} else {
		writeRenderSweepResult(out, result)
	}
	if result.HasBlockingFindings() {
		return ExitError{Code: 2}
	}
	return nil
}

func parseRenderSweepArgs(args []string) (renderSweepOptions, error) {
	var options renderSweepOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--dry-run":
			options.dryRun = true
		default:
			return renderSweepOptions{}, fmt.Errorf("unknown render sweep option %q", arg)
		}
	}
	return options, nil
}

func writeHousekeepingHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf housekeeping [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Scan agent artifacts and summarize housekeeping recommendations.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --json       Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON")
	fmt.Fprintln(out, "  --dry-run    Show recommendations without applying actions")
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
	{name: "brainstorms", relativeDir: filepath.Join("drafts", "brainstorms"), cleanupStatuses: cleanupStatusSet("done", "resolved", "archived")},
	{name: "ideas", relativeDir: "ideas", cleanupStatuses: cleanupStatusSet("done", "resolved", "archived")},
	{name: "reports", relativeDir: "reports", cleanupStatuses: cleanupStatusSet("done", "final", "archived")},
	{name: "shaping_drafts", relativeDir: "drafts", cleanupStatuses: cleanupStatusSet("absorbed", "archived")},
	{name: "sparks", relativeDir: "sparks", cleanupStatuses: cleanupStatusSet("done", "resolved", "archived")},
	{name: "specs", relativeDir: "specs", cleanupStatuses: cleanupStatusSet("done", "complete", "archived")},
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
		"capture": writeBrainstormCaptureHelp,
		"list":    writeBrainstormListHelp,
		"show":    writeBrainstormShowHelp,
		"promote": writeBrainstormPromoteHelp,
		"archive": writeBrainstormArchiveHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "capture":
		return r.runBrainstormCapture(args[1:], out, runtime)
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
		{Name: "capture", Summary: "Capture a brainstorm"},
		{Name: "list", Summary: "List brainstorms"},
		{Name: "show", Summary: "Show one brainstorm"},
		{Name: "promote", Summary: "Promote a brainstorm to an idea"},
		{Name: "archive", Summary: "Archive brainstorms"},
	})
}

func writeBrainstormCaptureHelp(out io.Writer) {
	writeUsageHelp(out, "loaf brainstorm capture --title <title> [--body-file <path>|--body -|--message <text>] [--json]", "Capture a brainstorm in SQLite state.", "--title      Brainstorm title", "--body-file  Read Markdown body from a UTF-8 file", "--body       Use '-' to read Markdown body from stdin", "--message    Inline Markdown body; lower precedence than --body-file and --body -", "--json       Output created brainstorm, event, global database scope, and project identity as JSON")
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

func (r Runner) runBrainstormCapture(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		err := sqliteStateRequiredError("brainstorm capture")
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}

	options, err := parseBrainstormCaptureArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}
	body, ok, err := r.resolveBodyInput("brainstorm capture", options.body, true)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}
	if !ok {
		err := fmt.Errorf("brainstorm capture requires body content")
		if jsonRequested {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}
	options.capture.Body = body
	result, err := state.CaptureBrainstorm(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.capture)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "brainstorm capture", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "captured brainstorm %s\n", firstNonEmpty(result.Brainstorm.Alias, result.Brainstorm.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "title: %s\n", result.Brainstorm.Title)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
	return nil
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

func (r Runner) runArtifactEntityCommand(kind string, args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeArtifactEntityHelp(out, kind)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"new":  func(out io.Writer) { writeArtifactEntityNewHelp(out, kind) },
		"show": func(out io.Writer) { writeArtifactEntityShowHelp(out, kind) },
		"list": func(out io.Writer) { writeArtifactEntityListHelp(out, kind) },
		"link": func(out io.Writer) { writeArtifactEntityLinkHelp(out, kind) },
	}) {
		return nil
	}
	switch args[0] {
	case "new":
		return r.runArtifactEntityNew(kind, args[1:], out, runtime)
	case "show":
		return r.runArtifactEntityShow(kind, args[1:], out, runtime)
	case "list":
		return r.runArtifactEntityList(kind, args[1:], out, runtime)
	case "link":
		return r.runArtifactEntityLink(kind, args[1:], out, runtime)
	default:
		return unknownSubcommandError(kind, args[0])
	}
}

func writeArtifactEntityHelp(out io.Writer, kind string) {
	writeCommandGroupHelp(out, fmt.Sprintf("loaf %s <subcommand> [options]", kind), fmt.Sprintf("Manage %ss in native SQLite state.", kind), []subcommandHelpItem{
		{Name: "new", Summary: "Create an artifact"},
		{Name: "show", Summary: "Show one artifact"},
		{Name: "list", Summary: "List artifacts"},
		{Name: "link", Summary: "Link this artifact to another entity"},
	})
}

func writeArtifactEntityNewHelp(out io.Writer, kind string) {
	options := []string{
		"--title      Artifact title",
		"--body-file  Read Markdown body from a UTF-8 file",
		"--body       Use '-' to read Markdown body from stdin",
		"--message    Inline Markdown body; lower precedence than --body-file and --body -",
	}
	switch kind {
	case "plan", "council":
		options = append(options, "--spec       Optional related spec")
	case "handoff":
		options = append(options, "--harness-session-id  Optional conversation correlation tag", "--task       Optional related task")
	}
	options = append(options, "--json       Output created artifact, event, global database scope, and project identity as JSON")
	writeUsageHelp(out, fmt.Sprintf("loaf %s new --title <title> [--body-file <path>|--body -|--message <text>] [--json]", kind), fmt.Sprintf("Create a %s in SQLite state.", kind), options...)
}

func writeArtifactEntityShowHelp(out io.Writer, kind string) {
	writeUsageHelp(out, fmt.Sprintf("loaf %s show <%s> [--json]", kind, kind), fmt.Sprintf("Show one %s from SQLite state.", kind), "--json       Output artifact details, relationships, global database scope, and project identity as JSON")
}

func writeArtifactEntityListHelp(out io.Writer, kind string) {
	writeUsageHelp(out, fmt.Sprintf("loaf %s list [--all|--status <status>] [--json]", kind), fmt.Sprintf("List %ss from SQLite state.", kind), "--all        Include archived artifacts", "--status     Filter by status", "--json       Output artifacts, global database scope, and project identity as JSON")
}

func writeArtifactEntityLinkHelp(out io.Writer, kind string) {
	writeUsageHelp(out, fmt.Sprintf("loaf %s link <%s> <entity> [--type <type>] [--reason <text>] [--json]", kind, kind), fmt.Sprintf("Link a %s to another entity.", kind), "--type       Relationship type; defaults to related_to", "--reason     Relationship reason", "--json       Output relationship ID, source/target, global database scope, and project identity as JSON")
}

func (r Runner) runArtifactEntityNew(kind string, args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	if mode == state.ModeMarkdownOnly {
		err := sqliteStateRequiredError(kind + " new")
		if jsonRequested {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	if mode == state.ModeInvalid {
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonRequested {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	options, err := parseArtifactEntityNewArgs(kind, args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	body, ok, err := r.resolveBodyInput(kind+" new", options.body, true)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	if !ok {
		err := fmt.Errorf("%s new requires body content", kind)
		if jsonRequested {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	options.create.Body = body
	result, err := state.CreateArtifactEntity(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, kind+" new", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "created %s %s\n", kind, firstNonEmpty(result.Entity.Alias, result.Entity.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "title: %s\n", result.Entity.Title)
	fmt.Fprintf(out, "status: %s\n", result.Entity.Status)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
	return nil
}

func (r Runner) runArtifactEntityShow(kind string, args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs(kind+" show", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	if mode == state.ModeMarkdownOnly {
		return sqliteStateRequiredError(kind + " show")
	}
	if mode == state.ModeInvalid {
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ShowArtifactEntity(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, kind, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeArtifactEntityShow(out, result)
	return nil
}

func (r Runner) runArtifactEntityList(kind string, args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseArtifactEntityListArgs(kind, args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		return err
	}
	if mode == state.ModeMarkdownOnly {
		return sqliteStateRequiredError(kind + " list")
	}
	if mode == state.ModeInvalid {
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ListArtifactEntities(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.list)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeArtifactEntityList(out, result, options.list)
	return nil
}

func (r Runner) runArtifactEntityLink(kind string, args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseArtifactEntityLinkArgs(kind, args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, kind+" link", err)
		}
		return err
	}
	projectRoot, mode, err := r.stateMode(runtime)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, kind+" link", err)
		}
		return err
	}
	if mode == state.ModeMarkdownOnly {
		err := sqliteStateRequiredError(kind + " link")
		if jsonRequested {
			return writeJSONCommandError(out, kind+" link", err)
		}
		return err
	}
	if mode == state.ModeInvalid {
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonRequested {
			return writeJSONCommandError(out, kind+" link", err)
		}
		return err
	}
	result, err := state.CreateLink(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.LinkMutationOptions{
		From:   options.from,
		To:     options.to,
		Type:   firstNonEmpty(options.relationshipType, "related_to"),
		Reason: options.reason,
	})
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, kind+" link", err)
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

func writeArtifactEntityShow(out io.Writer, result state.ArtifactEntityShow) {
	entity := result.Entity
	fmt.Fprintf(out, "%s %s\n", entity.Kind, firstNonEmpty(entity.Alias, entity.ID))
	fmt.Fprintf(out, "title: %s\n", entity.Title)
	fmt.Fprintf(out, "status: %s\n", entity.Status)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	for _, source := range entity.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(entity.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range entity.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintf(out, "created: %s\n", entity.CreatedAt)
	fmt.Fprintf(out, "updated: %s\n", entity.UpdatedAt)
	if entity.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, entity.Body)
	}
}

func writeArtifactEntityList(out io.Writer, result state.ArtifactEntityList, filters state.ArtifactEntityListOptions) {
	fmt.Fprintf(out, "\n  loaf %s list\n\n", result.Kind)
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Entities) == 0 {
		if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "  No %ss found.\n\n", result.Kind)
		return
	}
	if result.DatabaseScope != "" || result.DatabasePath != "" || result.ProjectID != "" || result.ProjectName != "" || result.ProjectCurrentPath != "" {
		fmt.Fprintln(out)
	}
	aliases := make([]string, 0, len(result.Entities))
	for alias := range result.Entities {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		entity := result.Entities[alias]
		fmt.Fprintf(out, "    %-32s%s", alias, entity.Title)
		if filters.All || filters.Status != "" {
			fmt.Fprintf(out, "  [%s]", entity.Status)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "\n  %d %s(s)\n\n", len(result.Entities), result.Kind)
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
			"verify":  writeStateBackupVerifyHelp,
			"restore": writeStateBackupRestoreHelp,
		}) {
			return nil
		}
		return r.runStateBackup(args[1:], out, runtime)
	case "restore-ephemerals":
		if isHelpArg(args[1:]) {
			writeStateRestoreEphemeralsHelp(out)
			return nil
		}
		return r.runStateRestoreEphemerals(args[1:], out, runtime)
	case "verify-ephemerals":
		if isHelpArg(args[1:]) {
			writeStateVerifyEphemeralsHelp(out)
			return nil
		}
		return r.runStateVerifyEphemerals(args[1:], out, runtime)
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
	fmt.Fprintln(out, "  backup restore Rehearse an isolated disposable restore")
	fmt.Fprintln(out, "  restore-ephemerals Restore .agents ephemeral Markdown from a rollback backup")
	fmt.Fprintln(out, "  verify-ephemerals Verify .agents ephemeral Markdown before cutover")
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
	fmt.Fprintln(out, "  relationship-origin  Backfill missing relationship provenance and reclassify retired legacy origins")
	fmt.Fprintln(out, "  journal-search  Rebuild derived journal search from canonical entries")
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
	fmt.Fprintln(out, "  markdown            Import .agents Markdown artifacts into SQLite")
	fmt.Fprintln(out, "  storage-home        Copy legacy per-project state into the global XDG data-home database")
	fmt.Fprintln(out, "  schema              Preview or apply pending SQLite schema upgrades")
	fmt.Fprintln(out, "  lifecycle-statuses  Normalize legacy lifecycle statuses in SQLite")
	fmt.Fprintln(out, "  journal-first       Transform the global database to the journal-first model")
	fmt.Fprintln(out, "  deferrals           Convert historical journal deferrals into canonical deferred Intents")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help    Show help")
}

func writeStateBackupHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf state backup [--to <DIRECTORY>] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Create a SQLite database backup.")
	fmt.Fprintln(out, "Without --to, create a local_rollback snapshot under the global data-home backups directory.")
	fmt.Fprintln(out, "With --to, create an operator-selected non-temporary external destination copy; this is not proof of off-device protection.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  verify <backup>  Verify an existing SQLite backup")
	fmt.Fprintln(out, "  restore <backup> --to <absolute-empty-database-path>  Run an isolated disposable restore rehearsal; never activates or replaces the live database")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --to <DIRECTORY>  Operator-selected non-temporary external destination directory")
	fmt.Fprintln(out, "  --json            Output backup verification, classification, readiness, checksum, watermark, and current project identity as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func writeStateBackupVerifyHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state backup verify <backup> [--json]", "Verify an existing SQLite database backup without reading or mutating live state.", "--json       Output schema version, SQLite validity, journal retrieval readiness, recovery readiness, watermark, and captured project identities as JSON")
}

func writeStateBackupRestoreHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state backup restore <backup> --to <absolute-empty-database-path> [--json]", "Run an isolated disposable restore rehearsal. It never activates or replaces the live database.", "--to <absolute-empty-database-path>  Required empty disposable restore target; the target must not exist", "--json                              Output exact-copy, integrity, retrieval, watermark, and live-database safety evidence as JSON")
}

func writeStateRestoreEphemeralsHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state restore-ephemerals <manifest|backup-dir|backup-id> [--json]", "Restore .agents ephemeral Markdown files from a checksum-verified rollback manifest and stage them when inside a Git worktree.", "--json       Output rollback contract, project path, manifest path, restored file list, and restored status as JSON")
}

func writeStateVerifyEphemeralsHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state verify-ephemerals <manifest|backup-dir|backup-id> [--json]", "Verify .agents ephemeral Markdown files still match checksum-verified rollback backup bytes before cutover.", "--json       Output verification contract, project context, manifest path, per-file checks, and failures as JSON")
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
	case "delete":
		if isHelpArg(args[1:]) {
			writeProjectDeleteHelp(out)
			return nil
		}
		return r.runProjectDelete(args[1:], out, runtime)
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
	fmt.Fprintln(out, "  delete    Permanently delete a project and all its rows")
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
	_, store, err := r.openProjectStoreDiagnostic(runtime)
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
	projectRoot, store, err := r.openProjectStoreDiagnostic(runtime)
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
	projectRoot, store, err := r.openProjectStoreDiagnostic(runtime)
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
	projectRoot, store, err := r.openProjectStoreDiagnostic(runtime)
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

func writeProjectDeleteHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf project delete <project-id> [--yes] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Permanently delete a project and every dependent row across all entity tables.")
	fmt.Fprintln(out, "Destructive: requires --yes. Accepts a project id, friendly name, or current path.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --yes        Confirm the destructive delete")
	fmt.Fprintln(out, "  --json       Output removed-row counts and global database scope as JSON")
	fmt.Fprintln(out, "  -h, --help   Show help")
}

func (r Runner) runProjectDelete(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	ref, confirm, jsonOutput, err := parseDeleteRefArgs("project delete", args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "project delete", err)
		}
		return err
	}
	if !confirm {
		err := fmt.Errorf("confirmation-required: refusing to delete project %q without --yes; this permanently removes the project and every dependent row", ref)
		if jsonOutput {
			return writeJSONCommandError(out, "project delete", err)
		}
		return err
	}
	_, store, err := r.openProjectStore(runtime)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "project delete", err)
		}
		return err
	}
	defer store.Close()
	result, err := store.DeleteProject(context.Background(), ref)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "project delete", err)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeProjectDelete(out, result)
	return nil
}

func writeProjectDelete(out io.Writer, result state.ProjectDeleteResult) {
	name := firstNonEmpty(result.ProjectName, result.ProjectID)
	fmt.Fprintf(out, "deleted project %s\n", name)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	writeDeleteCounts(out, "removed", result.Removed)
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
	err = store.RequireCurrentSchema(context.Background())
	if err != nil {
		store.Close()
		return project.Root{}, nil, fmt.Errorf("project state database is invalid at %s (scope: global database): %w; run `loaf state doctor`", databasePath, err)
	}
	return projectRoot, store, nil
}

// openProjectStoreDiagnostic opens schema-current state for a read-only
// diagnostic surface. It intentionally does not require operational invariants:
// callers such as `loaf project list` must be able to inspect a divergent path
// record in order to make the corresponding doctor repair plan executable.
func (r Runner) openProjectStoreDiagnostic(runtime state.Runtime) (project.Root, *state.Store, error) {
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
	version, err := store.SchemaVersion(context.Background())
	if err != nil {
		store.Close()
		return project.Root{}, nil, fmt.Errorf("project state database is invalid at %s (scope: global database): %w; run `loaf state doctor`", databasePath, err)
	}
	if version != state.CurrentSchemaVersion() {
		err := store.RequireCurrentSchema(context.Background())
		store.Close()
		if err != nil {
			return project.Root{}, nil, fmt.Errorf("project state database is invalid at %s (scope: global database): %w; run `loaf state doctor`", databasePath, err)
		}
		return project.Root{}, nil, fmt.Errorf("project state database is invalid at %s (scope: global database): schema version changed while opening; run `loaf state doctor`", databasePath)
	}
	if _, err := store.ValidateCurrentSchema(context.Background()); err != nil {
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
	if err := store.RequireCurrentSchema(context.Background()); err != nil {
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
		if status.Mode == state.ModeInvalid || hasErrorStateDiagnostic(status.Diagnostics) {
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
	if status.Mode == state.ModeInvalid || hasErrorStateDiagnostic(status.Diagnostics) {
		return fmt.Errorf("state doctor found errors")
	}
	return nil
}

func hasErrorStateDiagnostic(diagnostics []state.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			return true
		}
	}
	return false
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
		"journal-search":          writeStateRepairJournalSearchHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "legacy-project-database":
		return r.runStateRepairLegacyProjectDatabase(args[1:], out, runtime)
	case "relationship-origin":
		return r.runStateRepairRelationshipOrigin(args[1:], out, runtime)
	case "journal-search":
		return r.runStateRepairJournalSearch(args[1:], out, runtime)
	default:
		return fmt.Errorf("state repair target %q is not implemented yet", args[0])
	}
}

func writeStateRepairLegacyProjectDatabaseHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state repair legacy-project-database [--dry-run|--apply] [--json]", "Archive migrated legacy per-project SQLite files without deleting them.", "--dry-run    Preview archive paths without writing", "--apply      Move legacy SQLite files into the archive directory", "--json       Output archive plan/result, global database scope, and project identity as JSON")
}

func writeStateRepairRelationshipOriginHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state repair relationship-origin [--origin <imported|manual>] [--dry-run|--apply] [--json]", "Reclassify the retired legacy origins (intent-create, legacy-conversion, exploration-create, system) to 'command' for the current project; foreign origins are reported, never rewritten. Without --origin this reclassification is all that runs and rows with no origin are left untouched; --origin additionally backfills them.", "--origin     Enable the missing-origin backfill with this provenance value: imported or manual (omit for reclassify-only)", "--dry-run    Preview affected rows without writing", "--apply      Apply the reclassification, and the backfill when --origin is given, after a backup", "--json       Output repair mode, plan/result, global database scope, and project identity as JSON")
}

func writeStateRepairJournalSearchHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state repair journal-search [--dry-run|--apply] [--json]", "Rebuild the derived journal search index from canonical journal entries.", "--dry-run    Preview canonical/index parity counts without writing", "--apply      Create a verified backup, rebuild the index, and verify exact parity", "--json       Output parity counts, backup verification, and repair result as JSON")
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
	result, err := state.RepairRelationshipOrigins(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.RelationshipOriginRepairOptions{
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
	fmt.Fprintf(out, "mode: %s\n", result.Mode)
	backfilling := result.Mode == state.RelationshipOriginRepairModeBackfillAndReclassify
	if backfilling {
		fmt.Fprintf(out, "origin: %s\n", result.Origin)
		fmt.Fprintf(out, "matched: %d\n", result.Matched)
		fmt.Fprintf(out, "updated: %d\n", result.Updated)
	} else if result.Matched > 0 {
		// Reclassify-only never writes these rows, so they are reported as
		// untouched state rather than as a backfill that matched nothing.
		fmt.Fprintf(out, "missing origin: %d row(s) left untouched; pass --origin imported|manual to backfill them\n", result.Matched)
	}
	reclassifiable := 0
	reclassifyTarget := ""
	for _, reclassification := range result.Reclassified {
		reclassifiable += reclassification.Matched
		// Every legacy origin reclassifies to the same registry target, so any
		// group's target names the dry-run summary value.
		reclassifyTarget = reclassification.Target
		fmt.Fprintf(out, "reclassify %s -> %s: matched %d, updated %d\n", reclassification.Origin, reclassification.Target, reclassification.Matched, reclassification.Updated)
	}
	for _, foreign := range result.ForeignOrigins {
		fmt.Fprintf(out, "warn: foreign origin %q on %d relationship row(s); left for doctor, never rewritten\n", foreign.Origin, foreign.Count)
	}
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	if !result.Applied && reclassifiable > 0 {
		fmt.Fprintf(out, "%d relationship row(s) with unknown origin would be reclassified to '%s'\n", reclassifiable, reclassifyTarget)
	}
	pendingBackfill := 0
	if backfilling {
		pendingBackfill = result.Matched
	}
	if !result.Applied && (pendingBackfill > 0 || reclassifiable > 0) {
		if backfilling {
			fmt.Fprintln(out, "next: rerun with --apply after reviewing the selected origin")
		} else {
			fmt.Fprintln(out, "next: rerun with --apply after reviewing the reclassification plan")
		}
	}
	return nil
}

func (r Runner) runStateRepairJournalSearch(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalSearchRepairArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state repair journal-search", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state repair journal-search", err)
		}
		return err
	}
	result, err := state.RepairJournalSearch(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalSearchRepairOptions{Apply: options.apply})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state repair journal-search", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}

	fmode := "--dry-run"
	if options.apply {
		fmode = "--apply"
	}
	fmt.Fprintf(out, "loaf state repair journal-search %s\n", fmode)
	fmt.Fprintf(out, "scope: %s database\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	if result.ProjectID != "" {
		fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	}
	if result.ProjectName != "" {
		fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	}
	if result.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	}
	fmt.Fprintf(out, "canonical rows: %d\n", result.CanonicalRows)
	fmt.Fprintf(out, "index rows: %d\n", result.IndexRows)
	fmt.Fprintf(out, "missing: %d\n", result.Missing)
	fmt.Fprintf(out, "extra: %d\n", result.Extra)
	fmt.Fprintf(out, "changed: %d\n", result.Changed)
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	if result.BackupPath != "" {
		fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	}
	fmt.Fprintf(out, "backup verified: %t\n", result.BackupVerified)
	fmt.Fprintf(out, "rebuilt: %d\n", result.Rebuilt)
	fmt.Fprintf(out, "parity verified: %t\n", result.ParityVerified)
	if !options.apply && (result.Missing > 0 || result.Extra > 0 || result.Changed > 0 || result.CanonicalRows != result.IndexRows) {
		fmt.Fprintln(out, "next: loaf state repair journal-search --apply")
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
	if len(args) > 0 && args[0] == "restore" {
		return r.runStateBackupRestore(args[1:], out, runtime)
	}
	jsonRequested := hasFlag(args, "--json")
	options, err := parseStateBackupArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state backup", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state backup", err)
		}
		return err
	}
	var result state.BackupResult
	if options.destination == "" {
		result, err = state.Backup(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	} else {
		result, err = state.BackupWithDestination(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.destination)
	}
	if err != nil {
		if options.jsonOutput {
			return writeStateBackupJSONError(out, "state backup", result, err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintln(out, "loaf state backup")
	fmt.Fprintf(out, "recovery tier: %s\n", result.RecoveryTier)
	if result.RequestedDestinationDirectory == "" {
		fmt.Fprintln(out, "requested destination directory: local data-home backup directory")
	} else {
		fmt.Fprintf(out, "requested destination directory: %s\n", result.RequestedDestinationDirectory)
	}
	fmt.Fprintf(out, "resolved destination directory: %s\n", result.ResolvedDestinationDirectory)
	if result.DeviceLossProtectionBasis == state.ProtectionBasisOperatorSelected {
		fmt.Fprintln(out, "destination classification: operator-selected non-temporary external destination")
	} else {
		fmt.Fprintln(out, "destination classification: local rollback destination")
	}
	fmt.Fprintf(out, "device loss protected: %t\n", result.DeviceLossProtected)
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
	fmt.Fprintf(out, "SQLite valid: %t\n", result.SQLiteValid)
	fmt.Fprintf(out, "journal retrieval ready: %t\n", result.JournalRetrievalReady)
	fmt.Fprintf(out, "recovery ready: %t\n", result.RecoveryReady)
	writeJournalWatermarkHuman(out, result.JournalWatermark)
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
	fmt.Fprintf(out, "SQLite valid: %t\n", result.SQLiteValid)
	fmt.Fprintf(out, "journal retrieval ready: %t\n", result.JournalRetrievalReady)
	fmt.Fprintf(out, "recovery ready: %t\n", result.RecoveryReady)
	writeJournalWatermarkHuman(out, result.JournalWatermark)
	fmt.Fprintf(out, "next: run `loaf state backup restore %s --to /absolute/empty-disposable-path` for an isolated rehearsal; live activation remains manual after documented universal quiescence\n", result.BackupPath)
	return nil
}

func (r Runner) runStateBackupRestore(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseStateBackupRestoreArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state backup restore", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state backup restore", err)
		}
		return err
	}
	result, err := state.RehearseBackupRestore(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.backupPath, options.restorePath)
	if err != nil {
		if options.jsonOutput {
			return writeStateRestoreJSONError(out, result, err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeStateBackupRestoreHuman(out, result)
	return nil
}

func writeStateBackupRestoreHuman(out io.Writer, result state.BackupRestoreResult) {
	fmt.Fprintln(out, "loaf state backup restore")
	fmt.Fprintln(out, "isolated restore rehearsal: true")
	fmt.Fprintln(out, "disposable: true")
	fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	fmt.Fprintf(out, "restore: %s\n", result.RestorePath)
	fmt.Fprintf(out, "source sha256: %s\n", result.SourceSHA256)
	fmt.Fprintf(out, "restored sha256: %s\n", result.RestoredSHA256)
	fmt.Fprintf(out, "exact copy: %t\n", result.ExactCopy)
	fmt.Fprintf(out, "live database: %s\n", result.LiveDatabasePath)
	fmt.Fprintf(out, "live database mutated: %t\n", result.LiveDatabaseMutated)
	fmt.Fprintf(out, "SQLite valid: %t\n", result.SQLiteValid)
	fmt.Fprintf(out, "integrity: %s\n", result.IntegrityCheck)
	fmt.Fprintf(out, "foreign keys: %s\n", result.ForeignKeyCheck)
	fmt.Fprintf(out, "schema version: %d\n", result.SchemaVersion)
	fmt.Fprintf(out, "projects: %d\n", result.ProjectCount)
	fmt.Fprintf(out, "journal retrieval ready: %t\n", result.JournalRetrievalReady)
	fmt.Fprintf(out, "journal search parity: %t\n", result.JournalSearchParity.Ready)
	writeJournalWatermarkHuman(out, result.JournalWatermark)
	fmt.Fprintf(out, "recovery ready: %t\n", result.RecoveryReady)
	fmt.Fprintln(out, "activation: manual only after documented universal quiescence; no concurrent live restore")
}

func writeJournalWatermarkHuman(out io.Writer, watermark state.JournalWatermark) {
	if !watermark.Present {
		fmt.Fprintln(out, "watermark present: false")
		fmt.Fprintln(out, "covered through: none (no journal watermark)")
		return
	}
	fmt.Fprintln(out, "watermark present: true")
	fmt.Fprintf(out, "covered through: journal:%s at %s\n", watermark.JournalEntryID, watermark.CreatedAt)
}

func (r Runner) runStateRestoreEphemerals(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseStateRestoreEphemeralsArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state restore-ephemerals", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state restore-ephemerals", err)
		}
		return err
	}
	manifestPath, err := r.resolveEphemeralRestoreManifest(projectRoot, options.target)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state restore-ephemerals", err)
		}
		return err
	}
	result, err := state.RestoreEphemeralMarkdownBackup(context.Background(), projectRoot, manifestPath)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state restore-ephemerals", err)
		}
		return err
	}
	if err := stageRestoredEphemeralFiles(projectRoot, result.RestoredFiles); err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state restore-ephemerals", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeMarkdownRollbackResultHuman(out, "loaf state restore-ephemerals", result)
	return nil
}

func (r Runner) runStateVerifyEphemerals(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseStateVerifyEphemeralsArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "state verify-ephemerals", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state verify-ephemerals", err)
		}
		return err
	}
	manifestPath, err := r.resolveEphemeralRestoreManifest(projectRoot, options.target)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state verify-ephemerals", err)
		}
		return err
	}
	result, err := state.VerifyEphemeralMarkdownBackup(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, manifestPath)
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "state verify-ephemerals", err)
		}
		return err
	}
	if options.jsonOutput {
		if err := writeJSON(out, result); err != nil {
			return err
		}
		if !result.Verified {
			return ExitError{Code: 1}
		}
		return nil
	}
	writeEphemeralVerificationHuman(out, result)
	if !result.Verified {
		return ExitError{Code: 1}
	}
	return nil
}

func (r Runner) resolveEphemeralRestoreManifest(projectRoot project.Root, target string) (string, error) {
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			return filepath.Join(target, "manifest.json"), nil
		}
		return target, nil
	} else if filepath.IsAbs(target) || strings.Contains(target, string(filepath.Separator)) || strings.Contains(target, "/") || strings.Contains(target, "\\") {
		return target, nil
	}

	databasePath, err := (state.PathResolver{StateHome: r.StateHome}).DatabasePath(projectRoot)
	if err != nil {
		return "", err
	}
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	candidates := []string{
		filepath.Join(backupDir, target, "manifest.json"),
		filepath.Join(backupDir, target+"-markdown", "manifest.json"),
	}
	if ext := filepath.Ext(target); ext != "" {
		base := strings.TrimSuffix(target, ext)
		candidates = append(candidates, filepath.Join(backupDir, base+"-markdown", "manifest.json"))
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ephemeral backup target %q not found; pass a manifest path, backup directory, or backup id under %s", target, backupDir)
}

func writeEphemeralVerificationHuman(out io.Writer, result state.EphemeralVerificationResult) {
	fmt.Fprintln(out, "loaf state verify-ephemerals")
	fmt.Fprintf(out, "scope: %s database\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	if result.ProjectID != "" {
		fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	}
	if result.ProjectName != "" {
		fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	}
	if result.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	}
	fmt.Fprintf(out, "rollback manifest: %s\n", result.RollbackManifestPath)
	fmt.Fprintf(out, "verified: %t\n", result.Verified)
	fmt.Fprintf(out, "verified files: %d/%d\n", result.VerifiedFiles, result.TotalFiles)
	if len(result.Failures) > 0 {
		fmt.Fprintf(out, "failures: %d\n", len(result.Failures))
		for _, failure := range result.Failures {
			fmt.Fprintf(out, "  - %s: %s\n", failure.Path, failure.Reason)
		}
	}
}

func stageRestoredEphemeralFiles(projectRoot project.Root, restoredFiles []string) error {
	if len(restoredFiles) == 0 {
		return nil
	}
	check := exec.Command("git", "-C", projectRoot.Path(), "rev-parse", "--is-inside-work-tree")
	if err := check.Run(); err != nil {
		return nil
	}
	args := []string{"-C", projectRoot.Path(), "add", "--"}
	args = append(args, restoredFiles...)
	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stage restored ephemeral files: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (r Runner) runStateExport(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := stateExportJSONRequested(args)
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"all":               writeStateExportAllHelp,
		"release-readiness": writeStateExportReleaseReadinessHelp,
		"spec":              writeStateExportSpecHelp,
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

// stateMigrateSource binds one dispatchable `loaf state migrate` source to its
// runner and its --help writer, so a source cannot gain dispatch without help
// routing or vice versa.
type stateMigrateSource struct {
	run  func(Runner, []string, io.Writer, state.Runtime) error
	help func(io.Writer)
}

// stateMigrateSources is the single registry runStateMigrate dispatches
// through; nested `<source> --help` routing is derived from it. The source
// list in writeStateMigrateHelp must name every key;
// TestStateMigrateHelpListsEveryDispatchableSource enforces that parity.
var stateMigrateSources = map[string]stateMigrateSource{
	"lifecycle-statuses": {run: Runner.runStateMigrateLifecycleStatuses, help: writeStateMigrateLifecycleStatusesHelp},
	"journal-first": {
		run: func(r Runner, args []string, out io.Writer, runtime state.Runtime) error {
			return r.runJournalFirstMigration(args, out, runtime, "loaf state migrate journal-first")
		},
		help: writeStateMigrateJournalFirstHelp,
	},
	"schema": {
		run: func(r Runner, args []string, out io.Writer, runtime state.Runtime) error {
			return r.runSchemaUpgrade(args, out, runtime, "loaf state migrate schema")
		},
		help: writeStateMigrateSchemaHelp,
	},
	"markdown":     {run: Runner.runStateMigrateMarkdown, help: writeStateMigrateMarkdownHelp},
	"storage-home": {run: Runner.runStateMigrateStorageHome, help: writeStateMigrateStorageHomeHelp},
	"deferrals":    {run: Runner.runStateMigrateDeferrals, help: writeStateMigrateDeferralsHelp},
}

// stateMigrateSourceHelp derives the `loaf state migrate <source> --help`
// routing map from the dispatch registry.
func stateMigrateSourceHelp() map[string]func(io.Writer) {
	writers := make(map[string]func(io.Writer), len(stateMigrateSources))
	for name, source := range stateMigrateSources {
		writers[name] = source.help
	}
	return writers
}

func (r Runner) runStateMigrate(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		return fmt.Errorf("state migrate requires a source")
	}
	if writeNestedHelp(out, args, stateMigrateSourceHelp()) {
		return nil
	}
	source, ok := stateMigrateSources[args[0]]
	if !ok {
		return fmt.Errorf("state migrate source %q is not implemented yet", args[0])
	}
	return source.run(r, args[1:], out, runtime)
}

func writeStateMigrateMarkdownHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate markdown [--dry-run|--apply|--resume] [--backup] [--remove-source] [--rollback <manifest>] [--json]", "Import .agents Markdown artifacts into SQLite without mutating Markdown by default.", "--dry-run       Preview import work", "--apply         Apply the import", "--resume        Resume an interrupted import", "--backup        Create SQLite and .agents rollback backups during apply/resume", "--remove-source Remove ephemeral Markdown sources; requires --backup", "--rollback      Restore .agents files from a rollback manifest", "--json          Output migration contract, scope, project context, counts, and rollback fields as JSON")
}

func writeStateMigrateStorageHomeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate storage-home [--dry-run|--apply] [--json]", "Copy legacy per-project state into the global XDG data-home SQLite database.", "--dry-run     Preview migration work", "--apply       Apply the migration", "--json        Output migration contract, global database paths, action, and project identity when available")
}

func writeStateMigrateLifecycleStatusesHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate lifecycle-statuses [--dry-run|--apply|--rollback <manifest>] [--json]", "Normalize legacy lifecycle statuses in SQLite with a backup and rollback manifest.", "--dry-run  Preview on a temporary database copy", "--apply    Normalize live SQLite statuses after creating a backup", "--rollback Restore statuses from a lifecycle-statuses rollback manifest", "--json     Output migration contract, project context, counts, backup, and rollback fields as JSON")
}

func writeStateMigrateSchemaHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate schema [--dry-run|--apply] [--json]", "Preview or apply pending SQLite schema upgrades with a verified backup before mutation.", "--dry-run  Preview pending schema upgrades without writing", "--apply    Apply pending schema upgrades after creating and verifying a backup", "--json     Output schema upgrade action, versions, pending migrations, backup, and verification as JSON")
}

func writeMigrateSchemaHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate schema [--dry-run|--apply] [--json]", "Preview or apply pending SQLite schema upgrades with a verified backup before mutation.", "--dry-run  Preview pending schema upgrades without writing", "--apply    Apply pending schema upgrades after creating and verifying a backup", "--json     Output schema upgrade action, versions, pending migrations, backup, and verification as JSON")
}

func writeMigrateMarkdownHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate markdown [--dry-run|--apply|--resume] [--backup] [--remove-source] [--rollback <manifest>] [--json]", "Import .agents Markdown artifacts into SQLite without mutating Markdown by default.", "--dry-run       Preview import work", "--apply         Apply the import", "--resume        Resume an interrupted import", "--backup        Create SQLite and .agents rollback backups during apply/resume", "--remove-source Remove ephemeral Markdown sources; requires --backup", "--rollback      Restore .agents files from a rollback manifest", "--json          Output migration contract, scope, project context, counts, and rollback fields as JSON")
}

func writeMigrateStorageHomeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate storage-home [--dry-run|--apply] [--json]", "Copy legacy per-project state into the global XDG data-home SQLite database.", "--dry-run     Preview migration work", "--apply       Apply the migration", "--json        Output migration contract, global database paths, action, and project identity when available")
}

func writeMigrateLifecycleStatusesHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate lifecycle-statuses [--dry-run|--apply|--rollback <manifest>] [--json]", "Normalize legacy lifecycle statuses in SQLite with a backup and rollback manifest.", "--dry-run  Preview on a temporary database copy", "--apply    Normalize live SQLite statuses after creating a backup", "--rollback Restore statuses from a lifecycle-statuses rollback manifest", "--json     Output migration contract, project context, counts, backup, and rollback fields as JSON")
}

type schemaUpgradeOptions struct {
	dryRun     bool
	apply      bool
	jsonOutput bool
}

func parseSchemaUpgradeArgs(args []string, command string) (schemaUpgradeOptions, error) {
	var options schemaUpgradeOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			if options.dryRun {
				return schemaUpgradeOptions{}, fmt.Errorf("%s cannot repeat --dry-run", command)
			}
			options.dryRun = true
		case "--apply":
			if options.apply {
				return schemaUpgradeOptions{}, fmt.Errorf("%s cannot repeat --apply", command)
			}
			options.apply = true
		case "--json":
			if options.jsonOutput {
				return schemaUpgradeOptions{}, fmt.Errorf("%s cannot repeat --json", command)
			}
			options.jsonOutput = true
		default:
			return schemaUpgradeOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.dryRun && options.apply {
		return schemaUpgradeOptions{}, fmt.Errorf("%s cannot combine --apply and --dry-run", command)
	}
	if !options.dryRun && !options.apply {
		return schemaUpgradeOptions{}, fmt.Errorf("%s requires --dry-run or --apply", command)
	}
	return options, nil
}

func (r Runner) runSchemaUpgrade(args []string, out io.Writer, runtime state.Runtime, displayCommand string) error {
	command := strings.TrimPrefix(displayCommand, "loaf ")
	jsonRequested := hasFlag(args, "--json")
	options, err := parseSchemaUpgradeArgs(args, displayCommand)
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
	var result state.SchemaUpgradeResult
	if options.apply {
		result, err = state.ApplySchemaUpgrade(context.Background(), projectRoot, resolver)
	} else {
		result, err = state.PreviewSchemaUpgrade(context.Background(), projectRoot, resolver)
	}
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSchemaUpgradeHuman(out, displayCommand, result, options.apply)
	return nil
}

// writeMigrateHelp lists the sources the runMigrate switch dispatches. It is
// the top-level sibling of writeStateMigrateHelp and covers a different source
// set: worktree-storage is top-level only, and deferrals is state-level only.
// TestRunnerMigrateHelpListsEveryDispatchableSource enforces that parity.
func writeMigrateHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf migrate <source> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run native migration workflows.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sources:")
	fmt.Fprintln(out, "  markdown            Import .agents Markdown artifacts into SQLite")
	fmt.Fprintln(out, "  storage-home        Copy legacy per-project state into the global XDG data-home database")
	fmt.Fprintln(out, "  schema              Preview or apply pending SQLite schema upgrades")
	fmt.Fprintln(out, "  lifecycle-statuses  Normalize legacy lifecycle statuses in SQLite")
	fmt.Fprintln(out, "  journal-first       Transform the global database to the journal-first model")
	fmt.Fprintln(out, "  worktree-storage    Move linked-worktree .agents content to the main checkout")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help    Show help")
}

func (r Runner) runMigrate(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 {
		// Bare invocation stays an error, but the source list goes to stderr
		// so the failure is self-correcting instead of a dead end.
		writeMigrateHelp(firstWriter(r.Stderr, os.Stderr))
		return fmt.Errorf("migrate requires a source")
	}
	if isHelpArg(args) {
		writeMigrateHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"lifecycle-statuses": writeMigrateLifecycleStatusesHelp,
		"journal-first":      writeMigrateJournalFirstHelp,
		"schema":             writeMigrateSchemaHelp,
		"markdown":           writeMigrateMarkdownHelp,
		"storage-home":       writeMigrateStorageHomeHelp,
		"worktree-storage":   writeWorktreeMigrationHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "lifecycle-statuses":
		return r.runLifecycleStatusMigration(args[1:], out, runtime, "loaf migrate lifecycle-statuses")
	case "journal-first":
		return r.runJournalFirstMigration(args[1:], out, runtime, "loaf migrate journal-first")
	case "schema":
		return r.runSchemaUpgrade(args[1:], out, runtime, "loaf migrate schema")
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

func (r Runner) runStateMigrateLifecycleStatuses(args []string, out io.Writer, runtime state.Runtime) error {
	return r.runLifecycleStatusMigration(args, out, runtime, "loaf state migrate lifecycle-statuses")
}

func (r Runner) runLifecycleStatusMigration(args []string, out io.Writer, runtime state.Runtime, displayCommand string) error {
	command := strings.TrimPrefix(displayCommand, "loaf ")
	jsonRequested := hasFlag(args, "--json")
	options, err := parseLifecycleStatusMigrationArgs(args, command)
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
	var result state.LifecycleStatusMigrationResult
	switch {
	case options.rollbackPath != "":
		result, err = state.RollbackLifecycleStatusMigration(context.Background(), projectRoot, resolver, options.rollbackPath)
	case options.apply:
		result, err = state.ApplyLifecycleStatusMigration(context.Background(), projectRoot, resolver)
	default:
		result, err = state.PreviewLifecycleStatusMigration(context.Background(), projectRoot, resolver)
	}
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeLifecycleStatusMigrationHuman(out, displayCommand, result)
	return nil
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
		if options.backup {
			backup, err := state.Backup(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
			if err != nil {
				if options.jsonOutput {
					return writeJSONCommandError(out, command, err)
				}
				return err
			}
			rollbackBackup, err := state.CreateMarkdownRollbackBackup(context.Background(), projectRoot, backup.BackupPath)
			if err != nil {
				if options.jsonOutput {
					return writeJSONCommandError(out, command, err)
				}
				return err
			}
			result.BackupPath = backup.BackupPath
			result.RollbackManifestPath = rollbackBackup.RollbackManifestPath
			result.AgentsBackupPath = rollbackBackup.AgentsBackupPath
			if options.removeSource {
				removed, err := state.RemoveMarkdownMigrationSources(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, rollbackBackup.RollbackManifestPath)
				if err != nil {
					if options.jsonOutput {
						return writeJSONCommandError(out, command, err)
					}
					return err
				}
				result.RemovedSourceFiles = removed
			}
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
	if options.rollbackPath != "" {
		result, err := state.RollbackMarkdownMigration(context.Background(), projectRoot, options.rollbackPath)
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, command, err)
			}
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeMarkdownRollbackResultHuman(out, displayCommand+" --rollback", result)
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
	if result.BackupPath != "" {
		fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	}
	if result.RollbackManifestPath != "" {
		fmt.Fprintf(out, "rollback manifest: %s\n", result.RollbackManifestPath)
	}
	if result.AgentsBackupPath != "" {
		fmt.Fprintf(out, "agents backup: %s\n", result.AgentsBackupPath)
	}
	if len(result.RemovedSourceFiles) > 0 {
		fmt.Fprintf(out, "removed source files: %d\n", len(result.RemovedSourceFiles))
		for _, path := range result.RemovedSourceFiles {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	writeMarkdownMigrationPlan(out, result.MarkdownMigrationPlan)
}

func writeMarkdownRollbackResultHuman(out io.Writer, command string, result state.MarkdownRollbackResult) {
	fmt.Fprintln(out, command)
	fmt.Fprintf(out, "action: %s\n", result.Action)
	fmt.Fprintf(out, "project path: %s\n", result.ProjectPath)
	fmt.Fprintf(out, "rollback manifest: %s\n", result.RollbackManifestPath)
	fmt.Fprintf(out, "state backup: %s\n", result.StateBackupPath)
	fmt.Fprintf(out, "restored: %t\n", result.Restored)
	fmt.Fprintf(out, "restored files: %d\n", len(result.RestoredFiles))
	for _, path := range result.RestoredFiles {
		fmt.Fprintf(out, "  - %s\n", path)
	}
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
	fmt.Fprintf(out, "unimported files: %d\n", len(plan.UnimportedFiles))
	for _, file := range plan.UnimportedFiles {
		fmt.Fprintf(out, "  - %s (%s)\n", file.Path, file.Reason)
	}
	fmt.Fprintf(out, "ignored files: %d\n", len(plan.IgnoredFiles))
	for _, file := range plan.IgnoredFiles {
		fmt.Fprintf(out, "  - %s (%s)\n", file.Path, file.Reason)
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

func writeLifecycleStatusMigrationHuman(out io.Writer, displayCommand string, result state.LifecycleStatusMigrationResult) {
	switch result.Action {
	case state.LifecycleStatusMigrationActionApply:
		fmt.Fprintf(out, "%s --apply\n", displayCommand)
	case state.LifecycleStatusMigrationActionRollback:
		fmt.Fprintf(out, "%s --rollback\n", displayCommand)
	default:
		fmt.Fprintf(out, "%s --dry-run\n", displayCommand)
	}
	fmt.Fprintf(out, "scope: %s database, lifecycle status migration\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	fmt.Fprintf(out, "project name: %s\n", result.ProjectName)
	fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	fmt.Fprintf(out, "action: %s\n", result.Action)
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	fmt.Fprintf(out, "copy run: %t\n", result.CopyRun)
	if result.BackupPath != "" {
		fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	}
	if result.RollbackManifestPath != "" {
		fmt.Fprintf(out, "rollback manifest: %s\n", result.RollbackManifestPath)
	}
	fmt.Fprintf(out, "entities scanned: %d\n", result.EntitiesScanned)
	fmt.Fprintf(out, "entities rewritten: %d\n", result.EntitiesRewritten)
	fmt.Fprintf(out, "events scanned: %d\n", result.EventsScanned)
	fmt.Fprintf(out, "events rewritten: %d\n", result.EventsRewritten)
	fmt.Fprintf(out, "normalization events: %d\n", result.NormalizationEvents)
	fmt.Fprintf(out, "legacy statuses remaining: %d\n", result.LegacyStatusesRemaining)
	if result.RollbackEntitiesRestored > 0 || result.RollbackEventsRestored > 0 {
		fmt.Fprintf(out, "rollback entities restored: %d\n", result.RollbackEntitiesRestored)
		fmt.Fprintf(out, "rollback events restored: %d\n", result.RollbackEventsRestored)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "warning: %s\n", warning)
	}
	switch result.Action {
	case state.LifecycleStatusMigrationActionDryRun:
		if result.EntitiesRewritten > 0 || result.EventsRewritten > 0 {
			fmt.Fprintln(out, "next: rerun with --apply to normalize live SQLite statuses after a backup")
		} else {
			fmt.Fprintln(out, "next: no lifecycle status migration is needed")
		}
	case state.LifecycleStatusMigrationActionApply:
		if result.RollbackManifestPath != "" {
			fmt.Fprintln(out, "next: keep the rollback manifest until the migration is verified")
		}
	case state.LifecycleStatusMigrationActionRollback:
		fmt.Fprintln(out, "next: inspect state before rerunning lifecycle status migration")
	}
}

func writeSchemaUpgradeHuman(out io.Writer, displayCommand string, result state.SchemaUpgradeResult, applyRequested bool) {
	flag := "--dry-run"
	if applyRequested {
		flag = "--apply"
	}
	fmt.Fprintf(out, "%s %s\n", displayCommand, flag)
	fmt.Fprintf(out, "scope: %s database, schema migration\n", result.DatabaseScope)
	fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "current schema version: %d\n", result.CurrentVersion)
	fmt.Fprintf(out, "required schema version: %d\n", result.RequiredVersion)
	if len(result.PendingVersions) == 0 {
		fmt.Fprintln(out, "pending versions: none")
	} else {
		fmt.Fprintf(out, "pending versions: %v\n", result.PendingVersions)
	}
	fmt.Fprintf(out, "action: %s\n", result.Action)
	fmt.Fprintf(out, "applied: %t\n", result.Applied)
	fmt.Fprintf(out, "verified: %t\n", result.Verified)
	fmt.Fprintf(out, "backup verified: %t\n", result.BackupVerified)
	if result.BackupPath != "" {
		fmt.Fprintf(out, "backup: %s\n", result.BackupPath)
	}
	fmt.Fprintf(out, "final schema version: %d\n", result.SchemaVersion)
	if result.Action == state.SchemaUpgradeActionDryRun && len(result.PendingVersions) > 0 {
		fmt.Fprintln(out, "next: rerun with --apply to create a verified backup and apply pending schema upgrades")
	} else if result.Action == state.SchemaUpgradeActionAlreadyReady {
		fmt.Fprintln(out, "next: no schema upgrade is needed")
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

func (r Runner) runSearch(args []string, out io.Writer, runtime state.Runtime) error {
	if isHelpArg(args) {
		writeSearchHelp(out)
		return nil
	}
	jsonRequested := hasFlag(args, "--json")
	options, err := parseSearchArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "search", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "search", err)
		}
		return err
	}
	result, err := state.Search(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SearchOptions{
		Query:        options.query,
		AllProjects:  options.allProjects,
		Limit:        options.limit,
		WorktreePath: runtime.RootPath(),
	})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "search", err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "search %q\n", result.Query)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.AllProjects {
		fmt.Fprintln(out, "scope: all projects")
	} else {
		fmt.Fprintln(out, "scope: current project")
	}
	fmt.Fprintf(out, "results: %d\n", len(result.Results))
	for _, hit := range result.Results {
		fmt.Fprintf(out, "- [%s] %s %s", hit.Tier, hit.Source, searchHitAddress(hit))
		if hit.Snippet != "" {
			fmt.Fprintf(out, " — %s", hit.Snippet)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func writeSearchHelp(out io.Writer) {
	writeUsageHelp(out, "loaf search <query> [--all-projects] [--limit <n>] [--json]", "Search SQLite artifact bodies, journal entries, and indexed docs.", "--all-projects  Search every registered project instead of only the current project", "--limit         Maximum results to return (default: 20)", "--json          Output tiered hits, stable entity addresses, snippets, global database scope, and project identity as JSON")
}

func parseSearchArgs(args []string) (searchOptions, error) {
	var options searchOptions
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--all-projects":
			options.allProjects = true
		case "--limit":
			value, err := consumeFlagValue(args, &i, "--limit")
			if err != nil {
				return searchOptions{}, err
			}
			limit, err := strconv.Atoi(value)
			if err != nil || limit <= 0 {
				return searchOptions{}, fmt.Errorf("--limit must be a positive integer")
			}
			options.limit = limit
		default:
			if strings.HasPrefix(args[i], "-") {
				return searchOptions{}, fmt.Errorf("unknown search option %q", args[i])
			}
			queryParts = append(queryParts, args[i])
		}
	}
	options.query = strings.TrimSpace(strings.Join(queryParts, " "))
	if options.query == "" {
		return searchOptions{}, fmt.Errorf("search requires a query")
	}
	return options, nil
}

func searchHitAddress(hit state.SearchHit) string {
	if hit.Source == "docs_index" {
		locator := hit.Locator
		if locator == "" {
			locator = hit.Path
		}
		if hit.ProjectName != "" {
			return hit.ProjectName + ":" + locator
		}
		return locator
	}
	if hit.Locator != "" {
		return hit.Locator
	}
	if hit.Source == "journal_entry" {
		address := hit.JournalEntryID
		if hit.EntryType != "" {
			address = hit.EntryType + ":" + address
		}
		return address
	}
	address := hit.EntityKind + "/" + hit.EntityID
	if hit.BodyKind != "" {
		address += "#" + hit.BodyKind
	}
	return address
}

func (r Runner) runDocs(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeDocsHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"index": writeDocsIndexHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "index":
		return r.runDocsIndex(args[1:], out, runtime)
	default:
		return unknownSubcommandError("docs", args[0])
	}
}

func writeDocsHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf docs <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage docs/ indexing.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  index  Index docs/ Markdown into SQLite FTS")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeDocsIndexHelp(out io.Writer) {
	writeUsageHelp(out, "loaf docs index [--rebuild] [--json]", "Index docs/ Markdown into SQLite FTS.", "--rebuild    Rebuild current worktree docs index before scanning", "--json       Output indexed docs, counts, global database scope, and project identity as JSON")
}

func (r Runner) runDocsIndex(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseDocsIndexArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := state.IndexDocs(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.DocsIndexOptions{Rebuild: options.rebuild, WorktreePath: runtime.RootPath()})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeDocsIndexResult(out, result)
	return nil
}

func parseDocsIndexArgs(args []string) (docsIndexCLIOptions, error) {
	var options docsIndexCLIOptions
	for _, arg := range args {
		switch arg {
		case "--rebuild":
			options.rebuild = true
		case "--json":
			options.jsonOutput = true
		default:
			return docsIndexCLIOptions{}, fmt.Errorf("unknown docs index option %q", arg)
		}
	}
	return options, nil
}

func writeDocsIndexResult(out io.Writer, result state.DocsIndexResult) {
	fmt.Fprintln(out, "loaf docs index")
	if result.Rebuild {
		fmt.Fprintln(out, "mode: rebuild")
	}
	fmt.Fprintln(out, "scope: global database")
	if result.DatabasePath != "" {
		fmt.Fprintf(out, "database: %s\n", result.DatabasePath)
	}
	if result.ProjectName != "" {
		fmt.Fprintf(out, "project: %s\n", result.ProjectName)
	}
	if result.ProjectCurrentPath != "" {
		fmt.Fprintf(out, "project path: %s\n", result.ProjectCurrentPath)
	}
	if result.IndexedWorktree != "" {
		fmt.Fprintf(out, "indexed worktree: %s\n", result.IndexedWorktree)
	}
	if result.IndexedRef != "" {
		fmt.Fprintf(out, "indexed ref: %s\n", result.IndexedRef)
	}
	fmt.Fprintf(out, "scanned: %d\n", result.Scanned)
	fmt.Fprintf(out, "indexed: %d\n", result.Indexed)
	fmt.Fprintf(out, "removed: %d\n", result.Removed)
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
	writeUsageHelp(out, "loaf task update <task> [options]", "Update task metadata.", "--status     New task status: "+validTaskStatusText(), "--priority   New task priority: "+validTaskPriorityText(), "--spec       Associated spec", "--depends-on Comma-separated task refs or none", "--json       Output updated task, event, global database scope, and project identity as JSON")
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
	if !options.SetStatus && !options.SetPriority && !options.SetSpec && !options.SetDependsOn {
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
	return []string{"id", "title", "spec", "status", "priority", "created", "updated", "depends_on", "files", "verify", "done", "completed_at"}
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
	for _, key := range []string{"id", "title", "status", "priority", "spec", "depends_on", "created", "updated"} {
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
	fmt.Fprintf(out, "  Specs:  %s  (%d total)\n\n", formatStatusCounts(specCounts, []string{state.LifecycleStatusInProgress, state.LifecycleStatusTodo, state.LifecycleStatusDraft, state.LifecycleStatusDone}), len(specs.Specs))
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
	writeUsageHelp(out, "loaf idea list [--all|--status <status>] [--json]", "List ideas from SQLite state.", "--all        Include done and archived ideas", "--status     Filter by status", "--json       Output ideas, global database scope, and project identity as JSON")
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
	writeUsageHelp(out, "loaf spark list [--all|--status <status>] [--json]", "List sparks from SQLite state.", "--all        Include done sparks", "--status     Filter by status", "--json       Output sparks, global database scope, and project identity as JSON")
}

func writeSparkShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark show <spark> [--json]", "Show one spark from SQLite state.", "--json       Output spark details, relationships, global database scope, and project identity as JSON")
}

func writeSparkCaptureHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark capture --scope <scope> --text <text> [--json]", "Capture a spark in SQLite state.", "--scope      Spark scope", "--text       Spark text", "--json       Output created spark, event, global database scope, and project identity as JSON")
}

func writeSparkResolveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spark resolve <spark> --by <entity> [--reason <text>] [--json]", "Resolve a spark by linking it to the entity that resolves it.", "--by         Resolving entity reference (required)", "--reason     Resolution reason", "--json       Output resolution relationship, event, global database scope, and project identity as JSON")
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
		"new":      writeSpecNewHelp,
		"list":     writeSpecListHelp,
		"show":     writeSpecShowHelp,
		"status":   writeSpecStatusHelp,
		"render":   writeSpecRenderHelp,
		"finalize": writeSpecFinalizeHelp,
		"archive":  writeSpecArchiveHelp,
		"delete":   writeSpecDeleteHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "new":
		return r.runSpecNew(args[1:], out, runtime)
	case "list":
		return r.runSpecList(args[1:], out, runtime)
	case "show":
		return r.runSpecShow(args[1:], out, runtime)
	case "status":
		return r.runSpecStatus(args[1:], out, runtime)
	case "render":
		return r.runSpecRender(args[1:], out, runtime)
	case "finalize":
		return r.runSpecFinalize(args[1:], out, runtime)
	case "archive":
		return r.runSpecArchive(args[1:], out, runtime)
	case "delete":
		return r.runSpecDelete(args[1:], out, runtime)
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
	fmt.Fprintln(out, "  new      Create a spec in SQLite state")
	fmt.Fprintln(out, "  list     List specs")
	fmt.Fprintln(out, "  show     Show one spec")
	fmt.Fprintln(out, "  status   Set a spec's lifecycle status")
	fmt.Fprintln(out, "  render   Render a spec to the XDG cache")
	fmt.Fprintln(out, "  finalize Write a deterministic spec render to git")
	fmt.Fprintln(out, "  archive  Archive completed specs")
	fmt.Fprintln(out, "  delete   Permanently delete a spec and its dependent rows")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeSpecNewHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec new <slug> --title <title> [options]", "Create a spec in SQLite state.",
		"--title      Spec title (defaults to a title derived from the slug)",
		"--id         Explicit spec id (SPEC-NNN); auto-allocated when omitted",
		"--source     Provenance label recorded on the spec and creation event (default: ad-hoc)",
		"--branch     Implementation branch recorded on the spec for breakdown/implement handoff",
		"--related    Comma-separated spec refs to link as related (SPEC-A,SPEC-B)",
		"--body-file  Read the spec body from a file",
		"--body -     Read the spec body from stdin",
		"--message    Use the given text as the spec body",
		"--json       Output the created spec, global database scope, and project identity as JSON")
}

func writeSpecListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec list [--json]", "List specs.", "--json       Output specs, diagnostics, task counts, global database scope, and project identity as JSON")
}

func writeSpecShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec show <spec> [--json]", "Show one spec.", "--json       Output spec details, task counts, relationships, global database scope, and project identity as JSON")
}

func writeSpecStatusHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec status <spec> <status> [--json]", "Set a spec's lifecycle status: "+validSpecStatusText()+".", "--json       Output spec status transition, event, global database scope, and project identity as JSON")
}

func validSpecStatusText() string {
	return strings.Join(state.LifecycleStatusesForEntity(state.LifecycleEntitySpec), ", ")
}

func writeSpecRenderHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec render <spec> [--json]", "Render a deterministic spec Markdown file to the XDG cache.", "--json       Output render path, content hash, contract, global database scope, and project identity as JSON")
}

func writeSpecFinalizeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec finalize <spec> [--json]", "Write a deterministic spec Markdown render to its tracked git location.", "--json       Output render path, content hash, contract, global database scope, and project identity as JSON")
}

func writeSpecArchiveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec archive <spec...> [--json]", "Archive completed specs.", "--json       Output archive result, archived specs, global database scope, and project identity as JSON")
}

func writeSpecDeleteHelp(out io.Writer) {
	writeUsageHelp(out, "loaf spec delete <spec> [--yes] [--json]", "Permanently delete a spec and every dependent row (aliases, bodies, search index, events, sources). Destructive: requires --yes. The on-disk render file, if any, is left in place.", "--yes        Confirm the destructive delete", "--json       Output removed-row counts, global database scope, and project identity as JSON")
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

func (r Runner) runSpecNew(args []string, out io.Writer, runtime state.Runtime) error {
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
		return sqliteStateRequiredError("spec new")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	options, err := parseSpecNewArgs(args)
	if err != nil {
		return err
	}
	body, ok, err := r.resolveBodyInput("spec new", options.body, false)
	if err != nil {
		return err
	}
	if ok {
		options.create.Body = body
		options.create.SetBody = true
	}
	result, err := state.CreateSpec(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeSpecCreate(out, result)
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

func (r Runner) runSpecStatus(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	ref, newStatus, jsonOutput, err := parseSpecStatusArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "spec status", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "spec status", err)
		}
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "spec status", err)
		}
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		err := sqliteStateRequiredError("spec status")
		if jsonOutput {
			return writeJSONCommandError(out, "spec status", err)
		}
		return err
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonOutput {
			return writeJSONCommandError(out, "spec status", err)
		}
		return err
	}

	result, err := state.SetSpecStatus(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, newStatus)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "spec status", err)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeSpecStatus(out, result)
	return nil
}

func parseSpecStatusArgs(args []string) (string, string, bool, error) {
	var positional []string
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", "", false, fmt.Errorf("spec status requires a spec and a status")
	}
	return positional[0], positional[1], jsonOutput, nil
}

func (r Runner) runSpecRender(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("spec render", args)
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
		return sqliteStateRequiredError("spec render")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RenderDurableArtifact(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.DurableRenderOptions{
		Kind: "spec",
		Ref:  ref,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeDurableRenderResult(out, result)
	return nil
}

func (r Runner) runSpecFinalize(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("spec finalize", args)
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
		return sqliteStateRequiredError("spec finalize")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.FinalizeDurableArtifact(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.DurableFinalizeOptions{
		Kind: "spec",
		Ref:  ref,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeDurableFinalizeResult(out, result)
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

func (r Runner) runSpecDelete(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	ref, confirm, jsonOutput, err := parseDeleteRefArgs("spec delete", args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	}
	if !confirm {
		err := fmt.Errorf("confirmation-required: refusing to delete spec %q without --yes; this permanently removes the spec and every dependent row", ref)
		if jsonOutput {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		err := sqliteStateRequiredError("spec delete")
		if jsonOutput {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	case state.ModeInvalid:
		err := fmt.Errorf("state database is invalid; run `loaf state doctor`")
		if jsonOutput {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	}
	result, err := state.DeleteSpec(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "spec delete", err)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeSpecDelete(out, result)
	return nil
}

func writeSpecDelete(out io.Writer, result state.SpecDeleteResult) {
	ref := result.Ref
	if result.Spec != nil {
		ref = firstNonEmpty(result.Spec.Alias, result.Ref, result.Spec.ID)
	}
	fmt.Fprintf(out, "deleted spec %s\n", ref)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	writeDeleteCounts(out, "removed", result.Removed)
	writeDeleteCounts(out, "unlinked", result.Unlinked)
	if result.RenderRetained {
		fmt.Fprintf(out, "render left in place: %s\n", result.RenderPath)
	}
}

func writeDeleteCounts(out io.Writer, label string, counts []state.DeleteCount) {
	total := 0
	for _, count := range counts {
		total += count.Rows
	}
	if total == 0 {
		return
	}
	fmt.Fprintf(out, "%s:\n", label)
	for _, count := range counts {
		if count.Rows == 0 {
			continue
		}
		fmt.Fprintf(out, "  %-18s %d\n", count.Table, count.Rows)
	}
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
				HasBody:       strings.TrimSpace(markdownContentWithoutFrontmatter(string(body))) != "",
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
	status := state.LifecycleStatusForDisplay(state.LifecycleEntitySpec, firstNonEmpty(meta.Status, field("status"), "drafting"))
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
		canonical := state.LifecycleStatusForDisplay(state.LifecycleEntitySpec, status)
		spec := state.TraceEntity{Kind: "spec", ID: ref, Alias: ref, Title: title, Status: canonical}
		if !state.LifecycleStatusMatches(state.LifecycleEntitySpec, status, state.LifecycleStatusDone) {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: canonical, Status: canonical, Reason: fmt.Sprintf("status is %s, must be done", canonical)})
			continue
		}
		file := jsonObjectString(entry, "file")
		if strings.HasPrefix(file, "archive/") {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: canonical, Status: canonical, Reason: "already archived"})
			continue
		}
		if file == "" {
			file = markdownSpecFileForAlias(rootPath, ref)
		}
		if file == "" {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: canonical, Status: canonical, Reason: "file not found in index"})
			continue
		}
		srcPath := filepath.Join(rootPath, ".agents", "specs", filepath.FromSlash(file))
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: canonical, Status: canonical, Reason: fmt.Sprintf("file not found at %s", file)})
			continue
		} else if err != nil {
			return state.SpecArchiveResult{}, fmt.Errorf("inspect markdown spec %s: %w", file, err)
		}
		archivedFile := filepath.ToSlash(filepath.Join("archive", filepath.FromSlash(file)))
		destPath := filepath.Join(rootPath, ".agents", "specs", filepath.FromSlash(archivedFile))
		if _, err := os.Stat(destPath); err == nil {
			result.Skipped = append(result.Skipped, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: canonical, Status: canonical, Reason: fmt.Sprintf("%s already exists", archivedFile)})
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
		result.Archived = append(result.Archived, state.SpecArchiveItem{Spec: &spec, Ref: ref, Previous: canonical, Status: "archived"})
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
	if spec.Branch != "" {
		fmt.Fprintf(out, "branch: %s\n", spec.Branch)
	}
	if spec.Source != "" {
		fmt.Fprintf(out, "source: %s\n", spec.Source)
	}
	fmt.Fprintf(out, "tasks: %d todo / %d in_progress / %d done\n", spec.Tasks.Todo, spec.Tasks.InProgress, spec.Tasks.Done)
	for _, source := range spec.Sources {
		fmt.Fprintf(out, "render: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "render hash: %s\n", source.Hash)
		}
	}
	if len(spec.Related) > 0 {
		labels := make([]string, 0, len(spec.Related))
		for _, related := range spec.Related {
			labels = append(labels, firstNonEmpty(related.Alias, related.ID))
		}
		fmt.Fprintf(out, "related: %s\n", strings.Join(labels, ", "))
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

func writeSpecStatus(out io.Writer, result state.SpecStatusResult) {
	spec := result.Ref
	if result.Spec != nil {
		spec = firstNonEmpty(result.Spec.Alias, result.Ref, result.Spec.ID)
	}
	fmt.Fprintf(out, "spec %s\n", spec)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.Previous != result.Status {
		fmt.Fprintf(out, "status: %s -> %s\n", result.Previous, result.Status)
	} else {
		fmt.Fprintf(out, "status: %s\n", result.Status)
	}
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
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

func (r Runner) runReport(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeReportHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":     writeReportListHelp,
		"show":     writeReportShowHelp,
		"render":   writeReportRenderHelp,
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
	case "show":
		return r.runReportShow(args[1:], out, runtime)
	case "render":
		return r.runReportRender(args[1:], out, runtime)
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

func (r Runner) runFinding(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeFindingHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":        writeFindingListHelp,
		"show":        writeFindingShowHelp,
		"create":      writeFindingCreateHelp,
		"verdict":     writeFindingVerdictHelp,
		"import-json": writeFindingImportJSONHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "list":
		return r.runFindingList(args[1:], out, runtime)
	case "show":
		return r.runFindingShow(args[1:], out, runtime)
	case "create":
		return r.runFindingCreate(args[1:], out, runtime)
	case "verdict":
		return r.runFindingVerdict(args[1:], out, runtime)
	case "import-json":
		return r.runFindingImportJSON(args[1:], out, runtime)
	default:
		return unknownSubcommandError("finding", args[0])
	}
}

func writeFindingHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf finding <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage report findings and verdicts in native SQLite state.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  list     List findings")
	fmt.Fprintln(out, "  show     Show one finding")
	fmt.Fprintln(out, "  create   Create a finding")
	fmt.Fprintln(out, "  verdict  Record a finding verdict")
	fmt.Fprintln(out, "  import-json")
	fmt.Fprintln(out, "           Import row-shaped findings and verdicts")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeFindingListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf finding list [--report <report>] [--status <status>] [--severity <severity>] [--confidence <confidence>] [--dimension <dimension>] [--format json|csv|markdown|html] [--json]", "List findings.", "--report     Filter by parent report", "--status     Filter by finding status: open, confirmed, refuted, partial, archived", "--severity   Filter by severity: critical, high, medium, low, info", "--confidence Filter by confidence: high, medium, low", "--dimension  Filter by freeform dimension", "--format     Output format: json, csv, markdown, html", "--json       Alias for --format json")
}

func writeFindingShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf finding show <finding> [--format json|csv|markdown|html] [--json]", "Show one finding.", "--format     Output format: json, csv, markdown, html", "--json       Alias for --format json")
}

func writeFindingCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf finding create --report <report> --title <title> [options]", "Create a finding.", "--report     Parent report", "--title      Finding title", "--severity   Severity: critical, high, medium, low, info", "--confidence Confidence: high, medium, low", "--status     Initial status: open, confirmed, refuted, partial, archived", "--dimension  Freeform finding dimension", "--path       File path location", "--line-start Starting line", "--line-end   Ending line", "--symbol     Symbol or object location", "--metadata   JSON metadata", "--body-file  Read finding narrative from a UTF-8 file", "--body       Use '-' to read narrative from stdin", "--message    Inline finding narrative", "--json       Output created finding, event, global database scope, and project identity as JSON")
}

func writeFindingVerdictHelp(out io.Writer) {
	writeUsageHelp(out, "loaf finding verdict <finding> --outcome <outcome> --rationale <text> [--json]", "Record a finding verdict.", "--outcome    Verdict outcome: confirmed, refuted, partial", "--rationale  Verdict rationale", "--run        Optional run provenance row", "--notes      Reproduction notes", "--metadata   JSON metadata", "--json       Output verdict, updated finding, event, global database scope, and project identity as JSON")
}

func writeFindingImportJSONHelp(out io.Writer) {
	writeUsageHelp(out, "loaf finding import-json --report <report> [--findings <path>] [--verdicts <path>] [--json]", "Import row-shaped finding and verdict JSON.", "--report      Existing report ref, or slug for a new import report", "--report-type Type used when creating a missing report", "--source      Source label used when creating a missing report", "--run         Optional run provenance row for imported rows", "--findings    Row-shaped findings JSON; may be repeated", "--verdicts    Row-shaped verdicts JSON; may be repeated", "--json        Output import counts, files, global database scope, and project identity as JSON")
}

func (r Runner) runFindingList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseFindingListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("finding list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ListFindings(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	switch options.format {
	case state.ExportFormatJSON:
		return writeJSON(out, result)
	case "csv":
		return writeFindingListCSV(out, result)
	case state.ExportFormatMarkdown:
		return writeFindingListMarkdown(out, result)
	case "html":
		writeFindingListHTML(out, result)
		return nil
	}
	writeFindingList(out, result)
	return nil
}

func (r Runner) runFindingShow(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseFindingShowArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("finding show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ShowFinding(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref)
	if err != nil {
		return err
	}
	switch options.format {
	case state.ExportFormatJSON:
		return writeJSON(out, result)
	case "csv":
		return writeFindingShowCSV(out, result)
	case state.ExportFormatMarkdown:
		return writeFindingShowMarkdown(out, result)
	case "html":
		writeFindingShowHTML(out, result)
		return nil
	}
	writeFindingShow(out, result)
	return nil
}

func (r Runner) runFindingCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseFindingCreateArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("finding create")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	body, ok, err := r.resolveBodyInput("finding create", options.body, false)
	if err != nil {
		return err
	}
	if ok {
		options.create.Body = body
		options.create.SetBody = true
	}
	result, err := state.CreateFinding(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeFindingCreate(out, result)
	return nil
}

func (r Runner) runFindingVerdict(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseFindingVerdictArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("finding verdict")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RecordFindingVerdict(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.verdict)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeFindingVerdict(out, result)
	return nil
}

func (r Runner) runFindingImportJSON(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseFindingImportJSONArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("finding import-json")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	options.importJSON.FindingFiles = resolveFindingImportPaths(projectRoot.Path(), options.importJSON.FindingFiles)
	options.importJSON.VerdictFiles = resolveFindingImportPaths(projectRoot.Path(), options.importJSON.VerdictFiles)
	result, err := state.ImportFindingJSON(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.importJSON)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeFindingImportJSON(out, result)
	return nil
}

func (r Runner) findingStateMode(runtime state.Runtime) (project.Root, string, error) {
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

func (r Runner) runRun(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeRunHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list":     writeRunListHelp,
		"show":     writeRunShowHelp,
		"create":   writeRunCreateHelp,
		"complete": writeRunCompleteHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "list":
		return r.runRunList(args[1:], out, runtime)
	case "show":
		return r.runRunShow(args[1:], out, runtime)
	case "create":
		return r.runRunCreate(args[1:], out, runtime)
	case "complete":
		return r.runRunComplete(args[1:], out, runtime)
	default:
		return unknownSubcommandError("run", args[0])
	}
}

func writeRunHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf run <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage provenance runs in native SQLite state.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  list      List runs")
	fmt.Fprintln(out, "  show      Show one run")
	fmt.Fprintln(out, "  create    Create a provenance run")
	fmt.Fprintln(out, "  complete  Complete a provenance run")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeRunListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf run list [--status <status>] [--generator <ref>] [--json]", "List provenance runs.", "--status     Filter by status: pending, running, completed, failed, archived", "--generator  Filter by generator ref", "--json       Output runs, filters, global database scope, and project identity as JSON")
}

func writeRunShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf run show <run> [--json]", "Show one provenance run.", "--json       Output run details, relationships, global database scope, and project identity as JSON")
}

func writeRunCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf run create --generator <ref> [options]", "Create a provenance run.", "--generator  Generator reference; code is never accepted or stored", "--version    Generator version", "--hash       Generator hash", "--status     Initial status: pending, running, completed, failed, archived", "--metadata   JSON metadata", "--report     Optional report relationship", "--json       Output created run, event, global database scope, and project identity as JSON")
}

func writeRunCompleteHelp(out io.Writer) {
	writeUsageHelp(out, "loaf run complete <run> [--status completed|failed|archived] [--json]", "Complete a provenance run.", "--status     Completion status: completed, failed, archived", "--metadata   JSON metadata to replace run metadata", "--json       Output run status transition, event, global database scope, and project identity as JSON")
}

func (r Runner) runRunList(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseRunListArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("run list")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ListRuns(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.filters)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeRunList(out, result)
	return nil
}

func (r Runner) runRunShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("run show", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("run show")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ShowRun(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeRunShow(out, result)
	return nil
}

func (r Runner) runRunCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseRunCreateArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("run create")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.CreateRun(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeRunCreate(out, result)
	return nil
}

func (r Runner) runRunComplete(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseRunCompleteArgs(args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.findingStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("run complete")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.CompleteRun(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.complete)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeRunComplete(out, result)
	return nil
}

func writeReportHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf report <subcommand> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Manage durable reports in native SQLite state or markdown compatibility mode.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  list      List reports")
	fmt.Fprintln(out, "  show      Show one report")
	fmt.Fprintln(out, "  render    Render a report to the XDG cache")
	fmt.Fprintln(out, "  generate  Generate read-only markdown exports")
	fmt.Fprintln(out, "  create    Create a report")
	fmt.Fprintln(out, "  finalize  Finalize a report")
	fmt.Fprintln(out, "  archive   Archive a report")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  -h, --help  Show help")
}

func writeReportListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report list [--type <type>|--status <status>] [--json]", "List reports.", "--type       Filter by report type", "--status     Filter by status; Loaf lifecycle statuses: draft, done, archived", "--json       Output reports, diagnostics, global database scope, and project identity as JSON")
}

func writeReportShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report show <report> [--json]", "Show one report.", "--json       Output report details, relationships, global database scope, and project identity as JSON")
}

func writeReportRenderHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report render <report> [--json]", "Render a deterministic report Markdown file to the XDG cache.", "--json       Output render path, content hash, contract, global database scope, and project identity as JSON")
}

func writeReportGenerateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report generate <kind> [ref] [--format markdown] [--json]", "Generate a read-only markdown report.", "--format     Output format: markdown", "--json       Output contract, command, project context, and markdown content as JSON")
}

func writeReportCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf report create <slug> [--type <type>] [--source <source>] [--body-file <path>|--body -|--message <text>] [--json]", "Create a report.", "--type       Report type", "--source     Report source", "--body-file  Read Markdown body from a UTF-8 file", "--body       Use '-' to read Markdown body from stdin", "--message    Inline Markdown body; lower precedence than --body-file and --body -", "--json       Output created report, event, global database scope, and project identity as JSON")
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

func (r Runner) runReportShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("report show", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.reportStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		result, err := markdownReportShow(projectRoot.Path(), ref)
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		writeReportShow(out, result)
		return nil
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.ShowReport(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeReportShow(out, result)
	return nil
}

func (r Runner) runReportRender(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("report render", args)
	if err != nil {
		return err
	}
	projectRoot, mode, err := r.reportStateMode(runtime)
	if err != nil {
		return err
	}
	switch mode {
	case state.ModeMarkdownOnly:
		return sqliteStateRequiredError("report render")
	case state.ModeInvalid:
		return fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	result, err := state.RenderDurableArtifact(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.DurableRenderOptions{
		Kind: "report",
		Ref:  ref,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeDurableRenderResult(out, result)
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
		if bodyInputProvided(options.body) {
			return sqliteStateRequiredError("report create with body input")
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
	body, ok, err := r.resolveBodyInput("report create", options.body, false)
	if err != nil {
		return err
	}
	if ok {
		options.create.Body = body
		options.create.SetBody = true
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
	render, err := state.FinalizeDurableArtifact(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.DurableFinalizeOptions{
		Kind: "report",
		Ref:  ref,
	})
	if err != nil {
		return err
	}
	result.Render = &render
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

func writeSpecCreate(out io.Writer, result state.SpecCreateResult) {
	fmt.Fprintf(out, "created spec %s: %s\n", firstNonEmpty(result.Spec.Alias, result.Spec.ID), result.Spec.Title)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "status: %s\n", result.Spec.Status)
	fmt.Fprintf(out, "source: %s\n", result.Source)
	if result.Branch != "" {
		fmt.Fprintf(out, "branch: %s\n", result.Branch)
	}
	if len(result.Related) > 0 {
		labels := make([]string, 0, len(result.Related))
		for _, related := range result.Related {
			labels = append(labels, firstNonEmpty(related.Alias, related.ID))
		}
		fmt.Fprintf(out, "related: %s\n", strings.Join(labels, ", "))
	}
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
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

func writeReportShow(out io.Writer, result state.ReportShow) {
	report := result.Report
	fmt.Fprintf(out, "report %s\n", firstNonEmpty(report.Alias, report.ID))
	fmt.Fprintf(out, "title: %s\n", report.Title)
	fmt.Fprintf(out, "status: %s\n", report.Status)
	fmt.Fprintf(out, "type: %s\n", report.Kind)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	for _, source := range report.Sources {
		fmt.Fprintf(out, "source: %s\n", source.Path)
		if source.Hash != "" {
			fmt.Fprintf(out, "source hash: %s\n", source.Hash)
		}
	}
	if len(report.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range report.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	if len(report.Findings) == 0 {
		fmt.Fprintln(out, "findings: none")
	} else {
		fmt.Fprintln(out, "findings:")
		for _, finding := range report.Findings {
			ref := firstNonEmpty(finding.Alias, finding.ID)
			location := finding.Path
			if finding.LineStart > 0 {
				location = fmt.Sprintf("%s:%d", location, finding.LineStart)
			}
			if location != "" {
				fmt.Fprintf(out, "  - %s [%s/%s/%s] %s (%s)\n", ref, finding.Severity, finding.Confidence, finding.Status, finding.Title, location)
			} else {
				fmt.Fprintf(out, "  - %s [%s/%s/%s] %s\n", ref, finding.Severity, finding.Confidence, finding.Status, finding.Title)
			}
		}
	}
	if report.CreatedAt != "" {
		fmt.Fprintf(out, "created: %s\n", report.CreatedAt)
	}
	if report.UpdatedAt != "" {
		fmt.Fprintf(out, "updated: %s\n", report.UpdatedAt)
	}
	if report.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, report.Body)
	}
}

func writeFindingCreate(out io.Writer, result state.FindingCreateResult) {
	finding := result.Finding
	fmt.Fprintf(out, "created finding %s: %s\n", firstNonEmpty(finding.Alias, finding.ID), finding.Title)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "report: %s\n", firstNonEmpty(finding.Report.Alias, finding.Report.ID))
	fmt.Fprintf(out, "status: %s\n", finding.Status)
	fmt.Fprintf(out, "severity: %s\n", finding.Severity)
	fmt.Fprintf(out, "confidence: %s\n", finding.Confidence)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeFindingList(out io.Writer, result state.FindingList) {
	fmt.Fprint(out, "\n  loaf finding list\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Findings) == 0 {
		fmt.Fprint(out, "  No findings found.\n\n")
		return
	}
	keys := make([]string, 0, len(result.Findings))
	for key := range result.Findings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		finding := result.Findings[key]
		fmt.Fprintf(out, "  %-28s%s\n", key, finding.Title)
		fmt.Fprintf(out, "                              %s / %s / %s", finding.Severity, finding.Confidence, finding.Status)
		if finding.Dimension != "" {
			fmt.Fprintf(out, " / %s", finding.Dimension)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "\n  Total: %d finding(s)\n\n", len(result.Findings))
}

func writeFindingListCSV(out io.Writer, result state.FindingList) error {
	writer := csv.NewWriter(out)
	if err := writer.Write([]string{"alias", "title", "status", "severity", "confidence", "dimension", "report", "run", "path", "line_start", "line_end", "symbol"}); err != nil {
		return err
	}
	for _, finding := range sortedFindingItems(result.Findings) {
		if err := writer.Write([]string{
			firstNonEmpty(finding.Alias, finding.ID),
			finding.Title,
			finding.Status,
			finding.Severity,
			finding.Confidence,
			finding.Dimension,
			finding.Report,
			finding.Run,
			finding.Path,
			intString(finding.LineStart),
			intString(finding.LineEnd),
			finding.Symbol,
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeFindingListMarkdown(out io.Writer, result state.FindingList) error {
	var builder strings.Builder
	builder.WriteString("# Findings\n\n")
	builder.WriteString("| Finding | Status | Severity | Confidence | Dimension | Location |\n")
	builder.WriteString("|---|---|---|---|---|---|\n")
	for _, finding := range sortedFindingItems(result.Findings) {
		builder.WriteString("| ")
		builder.WriteString(markdownCell(firstNonEmpty(finding.Alias, finding.ID) + " - " + finding.Title))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(finding.Status))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(finding.Severity))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(finding.Confidence))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(finding.Dimension))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(findingItemLocation(finding)))
		builder.WriteString(" |\n")
	}
	content := builder.String()
	if err := state.ValidateExternalMarkdownExport(content); err != nil {
		return err
	}
	fmt.Fprint(out, content)
	return nil
}

func writeFindingListHTML(out io.Writer, result state.FindingList) {
	fmt.Fprintln(out, "<!doctype html>")
	fmt.Fprintln(out, "<html><head><meta charset=\"utf-8\"><title>Findings</title></head><body>")
	fmt.Fprintln(out, "<h1>Findings</h1>")
	fmt.Fprintln(out, "<table>")
	fmt.Fprintln(out, "<thead><tr><th>Finding</th><th>Status</th><th>Severity</th><th>Confidence</th><th>Dimension</th><th>Location</th></tr></thead>")
	fmt.Fprintln(out, "<tbody>")
	for _, finding := range sortedFindingItems(result.Findings) {
		fmt.Fprintf(out, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(firstNonEmpty(finding.Alias, finding.ID)+" - "+finding.Title),
			html.EscapeString(finding.Status),
			html.EscapeString(finding.Severity),
			html.EscapeString(finding.Confidence),
			html.EscapeString(finding.Dimension),
			html.EscapeString(findingItemLocation(finding)))
	}
	fmt.Fprintln(out, "</tbody></table>")
	fmt.Fprintln(out, "</body></html>")
}

func writeFindingShow(out io.Writer, result state.FindingShow) {
	finding := result.Finding
	fmt.Fprintf(out, "finding %s\n", firstNonEmpty(finding.Alias, finding.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "title: %s\n", finding.Title)
	fmt.Fprintf(out, "report: %s\n", firstNonEmpty(finding.Report.Alias, finding.Report.ID))
	if finding.Run != nil {
		fmt.Fprintf(out, "run: %s\n", firstNonEmpty(finding.Run.Alias, finding.Run.ID))
	}
	fmt.Fprintf(out, "status: %s\n", finding.Status)
	fmt.Fprintf(out, "severity: %s\n", finding.Severity)
	fmt.Fprintf(out, "confidence: %s\n", finding.Confidence)
	if finding.Dimension != "" {
		fmt.Fprintf(out, "dimension: %s\n", finding.Dimension)
	}
	if finding.Path != "" {
		if finding.LineStart > 0 {
			fmt.Fprintf(out, "location: %s:%d\n", finding.Path, finding.LineStart)
		} else {
			fmt.Fprintf(out, "location: %s\n", finding.Path)
		}
	}
	if len(finding.Verdicts) == 0 {
		fmt.Fprintln(out, "verdicts: none")
	} else {
		fmt.Fprintln(out, "verdicts:")
		for _, verdict := range finding.Verdicts {
			fmt.Fprintf(out, "  - %s: %s\n", verdict.Outcome, verdict.Rationale)
		}
	}
	if len(finding.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range finding.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	if finding.Body != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, finding.Body)
	}
}

func writeFindingShowCSV(out io.Writer, result state.FindingShow) error {
	writer := csv.NewWriter(out)
	if err := writer.Write([]string{"alias", "title", "status", "severity", "confidence", "dimension", "report", "run", "path", "line_start", "line_end", "symbol", "verdict_count"}); err != nil {
		return err
	}
	finding := result.Finding
	run := ""
	if finding.Run != nil {
		run = firstNonEmpty(finding.Run.Alias, finding.Run.ID)
	}
	if err := writer.Write([]string{
		firstNonEmpty(finding.Alias, finding.ID),
		finding.Title,
		finding.Status,
		finding.Severity,
		finding.Confidence,
		finding.Dimension,
		firstNonEmpty(finding.Report.Alias, finding.Report.ID),
		run,
		finding.Path,
		intString(finding.LineStart),
		intString(finding.LineEnd),
		finding.Symbol,
		strconv.Itoa(len(finding.Verdicts)),
	}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func writeFindingShowMarkdown(out io.Writer, result state.FindingShow) error {
	finding := result.Finding
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(markdownText(finding.Title))
	builder.WriteString("\n\n")
	builder.WriteString("- Finding: ")
	builder.WriteString(markdownText(firstNonEmpty(finding.Alias, finding.ID)))
	builder.WriteString("\n- Status: ")
	builder.WriteString(markdownText(finding.Status))
	builder.WriteString("\n- Severity: ")
	builder.WriteString(markdownText(finding.Severity))
	builder.WriteString("\n- Confidence: ")
	builder.WriteString(markdownText(finding.Confidence))
	if finding.Dimension != "" {
		builder.WriteString("\n- Dimension: ")
		builder.WriteString(markdownText(finding.Dimension))
	}
	if location := findingDetailLocation(finding); location != "" {
		builder.WriteString("\n- Location: ")
		builder.WriteString(markdownText(location))
	}
	builder.WriteString("\n\n")
	if finding.Body != "" {
		builder.WriteString("## Narrative\n\n")
		builder.WriteString(markdownText(finding.Body))
		builder.WriteString("\n\n")
	}
	builder.WriteString("## Verdicts\n\n")
	if len(finding.Verdicts) == 0 {
		builder.WriteString("No verdicts recorded.\n")
	} else {
		for _, verdict := range finding.Verdicts {
			builder.WriteString("- ")
			builder.WriteString(markdownText(verdict.Outcome))
			builder.WriteString(": ")
			builder.WriteString(markdownText(verdict.Rationale))
			builder.WriteString("\n")
		}
	}
	content := builder.String()
	if err := state.ValidateExternalMarkdownExport(content); err != nil {
		return err
	}
	fmt.Fprint(out, content)
	return nil
}

func writeFindingShowHTML(out io.Writer, result state.FindingShow) {
	finding := result.Finding
	fmt.Fprintln(out, "<!doctype html>")
	fmt.Fprintf(out, "<html><head><meta charset=\"utf-8\"><title>%s</title></head><body>\n", html.EscapeString(finding.Title))
	fmt.Fprintf(out, "<h1>%s</h1>\n", html.EscapeString(finding.Title))
	fmt.Fprintln(out, "<dl>")
	fmt.Fprintf(out, "<dt>Finding</dt><dd>%s</dd>\n", html.EscapeString(firstNonEmpty(finding.Alias, finding.ID)))
	fmt.Fprintf(out, "<dt>Status</dt><dd>%s</dd>\n", html.EscapeString(finding.Status))
	fmt.Fprintf(out, "<dt>Severity</dt><dd>%s</dd>\n", html.EscapeString(finding.Severity))
	fmt.Fprintf(out, "<dt>Confidence</dt><dd>%s</dd>\n", html.EscapeString(finding.Confidence))
	if finding.Dimension != "" {
		fmt.Fprintf(out, "<dt>Dimension</dt><dd>%s</dd>\n", html.EscapeString(finding.Dimension))
	}
	if location := findingDetailLocation(finding); location != "" {
		fmt.Fprintf(out, "<dt>Location</dt><dd>%s</dd>\n", html.EscapeString(location))
	}
	fmt.Fprintln(out, "</dl>")
	if finding.Body != "" {
		fmt.Fprintf(out, "<h2>Narrative</h2><p>%s</p>\n", html.EscapeString(finding.Body))
	}
	fmt.Fprintln(out, "<h2>Verdicts</h2>")
	if len(finding.Verdicts) == 0 {
		fmt.Fprintln(out, "<p>No verdicts recorded.</p>")
	} else {
		fmt.Fprintln(out, "<ul>")
		for _, verdict := range finding.Verdicts {
			fmt.Fprintf(out, "<li><strong>%s</strong>: %s</li>\n", html.EscapeString(verdict.Outcome), html.EscapeString(verdict.Rationale))
		}
		fmt.Fprintln(out, "</ul>")
	}
	fmt.Fprintln(out, "</body></html>")
}

func writeFindingVerdict(out io.Writer, result state.FindingVerdictResult) {
	fmt.Fprintf(out, "recorded verdict for %s\n", firstNonEmpty(result.Finding.Alias, result.Finding.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "outcome: %s\n", result.Verdict.Outcome)
	fmt.Fprintf(out, "status: %s\n", result.Finding.Status)
	fmt.Fprintf(out, "rationale: %s\n", result.Verdict.Rationale)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func sortedFindingItems(findings map[string]state.FindingItem) []state.FindingItem {
	keys := make([]string, 0, len(findings))
	for key := range findings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]state.FindingItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, findings[key])
	}
	return items
}

func findingItemLocation(finding state.FindingItem) string {
	location := finding.Path
	if finding.LineStart > 0 {
		location = fmt.Sprintf("%s:%d", location, finding.LineStart)
	}
	if finding.LineEnd > 0 && finding.LineEnd != finding.LineStart {
		location = fmt.Sprintf("%s-%d", location, finding.LineEnd)
	}
	return location
}

func findingDetailLocation(finding state.FindingDetail) string {
	location := finding.Path
	if finding.LineStart > 0 {
		location = fmt.Sprintf("%s:%d", location, finding.LineStart)
	}
	if finding.LineEnd > 0 && finding.LineEnd != finding.LineStart {
		location = fmt.Sprintf("%s-%d", location, finding.LineEnd)
	}
	return location
}

func intString(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func markdownCell(value string) string {
	return strings.ReplaceAll(markdownText(value), "|", "\\|")
}

func markdownText(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func writeRunCreate(out io.Writer, result state.RunCreateResult) {
	fmt.Fprintf(out, "created run %s\n", firstNonEmpty(result.Run.Alias, result.Run.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "generator: %s\n", result.Run.GeneratorRef)
	fmt.Fprintf(out, "status: %s\n", result.Run.Status)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeRunList(out io.Writer, result state.RunList) {
	fmt.Fprint(out, "\n  loaf run list\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Runs) == 0 {
		fmt.Fprint(out, "  No runs found.\n\n")
		return
	}
	keys := make([]string, 0, len(result.Runs))
	for key := range result.Runs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		run := result.Runs[key]
		fmt.Fprintf(out, "  %-28s%s\n", key, run.GeneratorRef)
		fmt.Fprintf(out, "                              %s", run.Status)
		if run.GeneratorVersion != "" {
			fmt.Fprintf(out, " / %s", run.GeneratorVersion)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "\n  Total: %d run(s)\n\n", len(result.Runs))
}

func writeRunShow(out io.Writer, result state.RunShow) {
	run := result.Run
	fmt.Fprintf(out, "run %s\n", firstNonEmpty(run.Alias, run.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "generator: %s\n", run.GeneratorRef)
	if run.GeneratorVersion != "" {
		fmt.Fprintf(out, "version: %s\n", run.GeneratorVersion)
	}
	if run.GeneratorHash != "" {
		fmt.Fprintf(out, "hash: %s\n", run.GeneratorHash)
	}
	fmt.Fprintf(out, "status: %s\n", run.Status)
	if run.StartedAt != "" {
		fmt.Fprintf(out, "started: %s\n", run.StartedAt)
	}
	if run.CompletedAt != "" {
		fmt.Fprintf(out, "completed: %s\n", run.CompletedAt)
	}
	if len(run.Relationships) == 0 {
		fmt.Fprintln(out, "relationships: none")
	} else {
		fmt.Fprintln(out, "relationships:")
		for _, relationship := range run.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(out, "  - %s %s %s %s", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(out, " (%s)", relationship.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	if run.Metadata != "" {
		fmt.Fprintf(out, "metadata: %s\n", run.Metadata)
	}
}

func writeRunComplete(out io.Writer, result state.RunCompleteResult) {
	fmt.Fprintf(out, "completed run %s\n", firstNonEmpty(result.Run.Alias, result.Run.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "previous: %s\n", result.Previous)
	fmt.Fprintf(out, "status: %s\n", result.Status)
	if result.EventID != "" {
		fmt.Fprintf(out, "event: %s\n", result.EventID)
	}
}

func writeFindingImportJSON(out io.Writer, result state.FindingImportJSONResult) {
	fmt.Fprintf(out, "imported findings into %s\n", firstNonEmpty(result.Report.Alias, result.Report.ID))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "findings: %d imported / %d skipped\n", result.FindingsImported, result.FindingsSkipped)
	fmt.Fprintf(out, "verdicts: %d imported / %d skipped\n", result.VerdictsImported, result.VerdictsSkipped)
	if len(result.Files) == 0 {
		fmt.Fprintln(out, "files: none")
		return
	}
	fmt.Fprintln(out, "files:")
	for _, file := range result.Files {
		fmt.Fprintf(out, "  - %s (%s): %d row(s)", file.Path, file.Kind, file.Rows)
		if file.FindingsImported > 0 || file.FindingsSkipped > 0 {
			fmt.Fprintf(out, ", findings %d imported / %d skipped", file.FindingsImported, file.FindingsSkipped)
		}
		if file.VerdictsImported > 0 || file.VerdictsSkipped > 0 {
			fmt.Fprintf(out, ", verdicts %d imported / %d skipped", file.VerdictsImported, file.VerdictsSkipped)
		}
		fmt.Fprintln(out)
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
	if result.Render != nil {
		fmt.Fprintf(out, "render: %s\n", result.Render.RelativePath)
		fmt.Fprintf(out, "render sha256: %s\n", result.Render.ContentHash)
	}
}

func writeDurableRenderResult(out io.Writer, result state.DurableRenderResult) {
	fmt.Fprintf(out, "rendered %s %s\n", result.Kind, result.Ref)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.Title != "" {
		fmt.Fprintf(out, "title: %s\n", result.Title)
	}
	fmt.Fprintf(out, "branch: %s\n", result.Branch)
	fmt.Fprintf(out, "contract: %s\n", result.Contract)
	fmt.Fprintf(out, "path: %s\n", result.Path)
	fmt.Fprintf(out, "sha256: %s\n", result.ContentHash)
}

func writeDurableFinalizeResult(out io.Writer, result state.DurableFinalizeResult) {
	fmt.Fprintf(out, "finalized %s %s\n", result.Kind, result.Ref)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.Title != "" {
		fmt.Fprintf(out, "title: %s\n", result.Title)
	}
	fmt.Fprintf(out, "contract: %s\n", result.Contract)
	fmt.Fprintf(out, "path: %s\n", result.RelativePath)
	fmt.Fprintf(out, "sha256: %s\n", result.ContentHash)
}

func writeRenderSweepResult(out io.Writer, result state.DurableRenderSweepResult) {
	fmt.Fprintln(out, "render sweep")
	fmt.Fprintf(out, "contract: %s\n", result.Contract)
	fmt.Fprintf(out, "scanned: %d\n", result.Scanned)
	fmt.Fprintf(out, "current: %d\n", result.Current)
	fmt.Fprintf(out, "upgrade needed: %d\n", result.UpgradeNeeded)
	fmt.Fprintf(out, "upgraded: %d\n", result.Upgraded)
	fmt.Fprintf(out, "drift: %d\n", result.Drift)
	fmt.Fprintf(out, "invalid: %d\n", result.Invalid)
	if result.Skipped > 0 {
		fmt.Fprintf(out, "skipped: %d\n", result.Skipped)
	}
	if result.DryRun {
		fmt.Fprintln(out, "dry run: true")
	}
	for _, file := range result.Files {
		switch file.Status {
		case "upgraded":
			fmt.Fprintf(out, "upgraded %s: %s -> %s\n", file.RelativePath, file.FromContract, file.ToContract)
		case "upgrade-needed":
			fmt.Fprintf(out, "upgrade-needed %s: %s -> %s\n", file.RelativePath, file.FromContract, file.ToContract)
		case "drift", "invalid":
			fmt.Fprintf(out, "%s %s: %s\n", file.Status, file.RelativePath, file.Error)
		}
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

func markdownReportShow(rootPath string, ref string) (state.ReportShow, error) {
	path, item, alias, err := findMarkdownReport(rootPath, ref)
	if err != nil {
		return state.ReportShow{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return state.ReportShow{}, fmt.Errorf("read markdown report %s: %w", path, err)
	}
	info, _ := os.Stat(path)
	updatedAt := ""
	if info != nil {
		updatedAt = info.ModTime().UTC().Format(time.RFC3339)
	}
	body := markdownContentWithoutFrontmatter(string(content))
	return state.ReportShow{
		Query: ref,
		Report: state.ReportDetail{
			ID:        alias,
			Alias:     alias,
			Title:     item.Title,
			Kind:      item.Kind,
			Status:    item.Status,
			Sources:   []state.TraceSource{{Path: item.SourcePath}},
			Body:      body,
			CreatedAt: updatedAt,
			UpdatedAt: updatedAt,
		},
	}, nil
}

func findMarkdownReport(rootPath string, ref string) (string, state.ReportItem, string, error) {
	agentsDir := filepath.Join(rootPath, ".agents")
	patterns := []string{
		filepath.Join(agentsDir, "reports", "*.md"),
		filepath.Join(agentsDir, "reports", "archive", "*.md"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return "", state.ReportItem{}, "", fmt.Errorf("find markdown reports: %w", err)
		}
		sort.Strings(files)
		for _, path := range files {
			item, alias, err := readMarkdownReport(rootPath, path)
			if err != nil {
				return "", state.ReportItem{}, "", err
			}
			if ref == alias || ref == item.SourcePath || ref == strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) {
				return path, item, alias, nil
			}
		}
	}
	return "", state.ReportItem{}, "", fmt.Errorf("report %q not found", ref)
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
	status := firstNonEmpty(field("status"), "draft")
	sourcePath := markdownReportSourcePath(rootPath, path)
	if strings.HasPrefix(sourcePath, ".agents/reports/archive/") {
		status = "archived"
	}
	status = state.LifecycleStatusForDisplay(state.LifecycleEntityReport, status)
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
	if !state.LifecycleStatusFilterMatches(state.LifecycleEntityReport, item.Status, options.Status) {
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
	previousRaw := firstNonEmpty(firstFieldValue(frontmatter["status"]), "draft")
	previous := state.LifecycleStatusForDisplay(state.LifecycleEntityReport, previousRaw)
	if !state.LifecycleStatusMatches(state.LifecycleEntityReport, previousRaw, state.LifecycleStatusDraft) {
		return state.ReportStatusResult{}, fmt.Errorf("report %q is not draft (status: %s)", ref, previous)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	frontmatter["status"] = markdownReportFrontmatterScalar(state.LifecycleStatusDone)
	frontmatter["finalized_at"] = markdownReportFrontmatterScalar(now)
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, body)), 0o600); err != nil {
		return state.ReportStatusResult{}, fmt.Errorf("write finalized report %s: %w", ref, err)
	}
	title := firstNonEmpty(firstFieldValue(frontmatter["title"]), firstMarkdownHeading(body), alias)
	return state.ReportStatusResult{
		Report:   state.TraceEntity{Kind: "report", ID: alias, Alias: alias, Title: title, Status: state.LifecycleStatusDone},
		Previous: previous,
		Status:   state.LifecycleStatusDone,
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
	previousRaw := firstNonEmpty(firstFieldValue(frontmatter["status"]), "draft")
	previous := state.LifecycleStatusForDisplay(state.LifecycleEntityReport, previousRaw)
	if !state.LifecycleStatusMatches(state.LifecycleEntityReport, previousRaw, state.LifecycleStatusDone) {
		return state.ReportStatusResult{}, fmt.Errorf("report %q is not done (status: %s)", ref, previous)
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
			if options.jsonSet {
				return backupVerifyOptions{}, fmt.Errorf("state backup verify accepts --json at most once")
			}
			options.jsonOutput = true
			options.jsonSet = true
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

func parseStateBackupArgs(args []string) (stateBackupOptions, error) {
	var options stateBackupOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			if options.jsonSet {
				return stateBackupOptions{}, fmt.Errorf("state backup accepts --json at most once")
			}
			options.jsonOutput = true
			options.jsonSet = true
		case "--to":
			if options.destinationSet {
				return stateBackupOptions{}, fmt.Errorf("state backup accepts --to at most once")
			}
			value, err := consumeFlagValue(args, &i, "--to")
			if err != nil {
				return stateBackupOptions{}, err
			}
			if strings.TrimSpace(value) == "" {
				return stateBackupOptions{}, fmt.Errorf("--to requires a non-empty directory")
			}
			options.destination = value
			options.destinationSet = true
		default:
			return stateBackupOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseStateBackupRestoreArgs(args []string) (stateBackupRestoreOptions, error) {
	var options stateBackupRestoreOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			if options.jsonSet {
				return stateBackupRestoreOptions{}, fmt.Errorf("state backup restore accepts --json at most once")
			}
			options.jsonOutput = true
			options.jsonSet = true
		case "--to":
			if options.restorePathSet {
				return stateBackupRestoreOptions{}, fmt.Errorf("state backup restore requires --to exactly once")
			}
			value, err := consumeFlagValue(args, &i, "--to")
			if err != nil {
				return stateBackupRestoreOptions{}, err
			}
			if strings.TrimSpace(value) == "" {
				return stateBackupRestoreOptions{}, fmt.Errorf("--to requires a non-empty absolute database path")
			}
			options.restorePath = value
			options.restorePathSet = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return stateBackupRestoreOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.backupPath != "" {
				return stateBackupRestoreOptions{}, fmt.Errorf("state backup restore accepts exactly one backup path")
			}
			options.backupPath = args[i]
		}
	}
	if options.backupPath == "" {
		return stateBackupRestoreOptions{}, fmt.Errorf("state backup restore requires a backup path")
	}
	if options.restorePath == "" {
		return stateBackupRestoreOptions{}, fmt.Errorf("state backup restore requires --to exactly once")
	}
	return options, nil
}

func parseStateRestoreEphemeralsArgs(args []string) (restoreEphemeralsOptions, error) {
	var options restoreEphemeralsOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return restoreEphemeralsOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			if options.target != "" {
				return restoreEphemeralsOptions{}, fmt.Errorf("state restore-ephemerals accepts exactly one manifest path, backup directory, or backup id")
			}
			options.target = arg
		}
	}
	if options.target == "" {
		return restoreEphemeralsOptions{}, fmt.Errorf("state restore-ephemerals requires a manifest path, backup directory, or backup id")
	}
	return options, nil
}

func parseStateVerifyEphemeralsArgs(args []string) (verifyEphemeralsOptions, error) {
	var options verifyEphemeralsOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return verifyEphemeralsOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			if options.target != "" {
				return verifyEphemeralsOptions{}, fmt.Errorf("state verify-ephemerals accepts exactly one manifest path, backup directory, or backup id")
			}
			options.target = arg
		}
	}
	if options.target == "" {
		return verifyEphemeralsOptions{}, fmt.Errorf("state verify-ephemerals requires a manifest path, backup directory, or backup id")
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
		case "--dry-run":
			if allowedFlags != nil && allowedFlags[arg] {
				options.dryRun = true
				continue
			}
			return compatibilityCommandOptions{}, fmt.Errorf("unknown %s option %q", command, arg)
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
			options.sessionRef = arg
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
	case state.ExportKindAll, state.ExportKindReleaseReadiness, state.ExportKindSpec, state.ExportKindTriage:
	default:
		return stateExportOptions{}, fmt.Errorf("state export kind %q is not implemented yet", options.kind)
	}
	switch options.kind {
	case state.ExportKindSpec:
		if len(positional) != 1 {
			return stateExportOptions{}, fmt.Errorf("state export spec requires exactly one spec")
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

type bodyInputOptions struct {
	bodyFile string
	body     string
	message  string
}

type brainstormCaptureOptions struct {
	jsonOutput bool
	capture    state.BrainstormCaptureOptions
	body       bodyInputOptions
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

type artifactEntityNewOptions struct {
	jsonOutput bool
	create     state.ArtifactEntityCreateOptions
	body       bodyInputOptions
}

type artifactEntityListOptions struct {
	jsonOutput bool
	list       state.ArtifactEntityListOptions
}

type reportListOptions struct {
	jsonOutput bool
	filters    state.ReportListOptions
}

type reportCreateOptions struct {
	jsonOutput bool
	create     state.ReportCreateOptions
	body       bodyInputOptions
}

type specNewOptions struct {
	jsonOutput bool
	create     state.SpecCreateOptions
	body       bodyInputOptions
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
	if !options.update.SetStatus && !options.update.SetPriority && !options.update.SetSpec && !options.update.SetDependsOn {
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

func parseBodyInputFlag(args []string, index *int, options *bodyInputOptions) (bool, error) {
	switch args[*index] {
	case "--body-file":
		value, err := consumeFlagValue(args, index, "--body-file")
		if err != nil {
			return true, err
		}
		options.bodyFile = value
		return true, nil
	case "--body":
		value, err := consumeFlagValue(args, index, "--body")
		if err != nil {
			return true, err
		}
		if value != "-" {
			return true, fmt.Errorf("--body only supports '-' for stdin input")
		}
		options.body = value
		return true, nil
	case "--message":
		value, err := consumeFlagValue(args, index, "--message")
		if err != nil {
			return true, err
		}
		options.message = value
		return true, nil
	default:
		return false, nil
	}
}

func bodyInputProvided(options bodyInputOptions) bool {
	return strings.TrimSpace(options.bodyFile) != "" || options.body == "-" || options.message != ""
}

func (r Runner) resolveBodyInput(command string, options bodyInputOptions, requireBody bool) (string, bool, error) {
	switch {
	case strings.TrimSpace(options.bodyFile) != "":
		content, err := os.ReadFile(options.bodyFile)
		if err != nil {
			return "", false, fmt.Errorf("read %s --body-file: %w", command, err)
		}
		if !utf8.Valid(content) {
			return "", false, fmt.Errorf("%s --body-file must be valid UTF-8", command)
		}
		return string(content), true, nil
	case options.body == "-":
		reader := r.Stdin
		if reader == nil {
			reader = os.Stdin
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			return "", false, fmt.Errorf("read %s --body -: %w", command, err)
		}
		if !utf8.Valid(content) {
			return "", false, fmt.Errorf("%s --body - input must be valid UTF-8", command)
		}
		return string(content), true, nil
	case options.message != "":
		content := []byte(options.message)
		if !utf8.Valid(content) {
			return "", false, fmt.Errorf("%s --message must be valid UTF-8", command)
		}
		return options.message, true, nil
	}

	if !requireBody {
		return "", false, nil
	}
	if body, ok, err := r.readBodyFromEditor(command); ok || err != nil {
		return body, ok, err
	}
	if requireBody {
		return "", false, fmt.Errorf("%s requires body content via --body-file, --body -, --message, or $EDITOR", command)
	}
	return "", false, nil
}

func (r Runner) readBodyFromEditor(command string) (string, bool, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" || !stdinIsTerminal(r.Stdin) {
		return "", false, nil
	}
	file, err := os.CreateTemp("", "loaf-body-*.md")
	if err != nil {
		return "", false, fmt.Errorf("create %s editor buffer: %w", command, err)
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return "", false, nil
	}
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("run $EDITOR for %s: %w", command, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read %s editor buffer: %w", command, err)
	}
	if !utf8.Valid(content) {
		return "", false, fmt.Errorf("%s editor buffer must be valid UTF-8", command)
	}
	if strings.TrimSpace(string(content)) == "" {
		return "", false, nil
	}
	return string(content), true, nil
}

func stdinIsTerminal(reader io.Reader) bool {
	if reader != nil && reader != os.Stdin {
		return false
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
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

func parseBrainstormCaptureArgs(args []string) (brainstormCaptureOptions, error) {
	var options brainstormCaptureOptions
	for i := 0; i < len(args); i++ {
		if ok, err := parseBodyInputFlag(args, &i, &options.body); ok || err != nil {
			if err != nil {
				return brainstormCaptureOptions{}, err
			}
			continue
		}
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return brainstormCaptureOptions{}, err
			}
			options.capture.Title = value
		default:
			return brainstormCaptureOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.capture.Title) == "" {
		return brainstormCaptureOptions{}, fmt.Errorf("brainstorm capture requires --title")
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

func parseDeleteRefArgs(command string, args []string) (ref string, confirm bool, jsonOutput bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--yes", "-y":
			confirm = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, false, fmt.Errorf("unknown option %q", arg)
			}
			if ref != "" {
				return "", false, false, fmt.Errorf("%s accepts exactly one argument", command)
			}
			ref = arg
		}
	}
	if ref == "" {
		return "", false, false, fmt.Errorf("%s requires an argument", command)
	}
	return ref, confirm, jsonOutput, nil
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

func parseArtifactEntityNewArgs(kind string, args []string) (artifactEntityNewOptions, error) {
	options := artifactEntityNewOptions{create: state.ArtifactEntityCreateOptions{Kind: kind}}
	for i := 0; i < len(args); i++ {
		if ok, err := parseBodyInputFlag(args, &i, &options.body); ok || err != nil {
			if err != nil {
				return artifactEntityNewOptions{}, err
			}
			continue
		}
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return artifactEntityNewOptions{}, err
			}
			options.create.Title = value
		case "--spec":
			value, err := consumeFlagValue(args, &i, "--spec")
			if err != nil {
				return artifactEntityNewOptions{}, err
			}
			options.create.Spec = value
		case "--harness-session-id", "--session":
			value, err := consumeFlagValue(args, &i, "--harness-session-id")
			if err != nil {
				return artifactEntityNewOptions{}, err
			}
			options.create.HarnessSessionID = value
		case "--task":
			value, err := consumeFlagValue(args, &i, "--task")
			if err != nil {
				return artifactEntityNewOptions{}, err
			}
			options.create.Task = value
		default:
			return artifactEntityNewOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.create.Title) == "" {
		return artifactEntityNewOptions{}, fmt.Errorf("%s new requires --title", kind)
	}
	return options, nil
}

func parseArtifactEntityListArgs(kind string, args []string) (artifactEntityListOptions, error) {
	options := artifactEntityListOptions{list: state.ArtifactEntityListOptions{Kind: kind}}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--all":
			options.list.All = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return artifactEntityListOptions{}, err
			}
			options.list.Status = value
		default:
			return artifactEntityListOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseArtifactEntityLinkArgs(kind string, args []string) (linkMutationOptions, error) {
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
		return linkMutationOptions{}, fmt.Errorf("%s link requires a %s and target entity", kind, kind)
	}
	options.from = positional[0]
	options.to = positional[1]
	return options, nil
}

func parseRunListArgs(args []string) (runListCLIOptions, error) {
	var options runListCLIOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return runListCLIOptions{}, err
			}
			options.filters.Status = value
		case "--generator":
			value, err := consumeFlagValue(args, &i, "--generator")
			if err != nil {
				return runListCLIOptions{}, err
			}
			options.filters.Generator = value
		default:
			return runListCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func parseRunCreateArgs(args []string) (runCreateCLIOptions, error) {
	var options runCreateCLIOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--generator":
			value, err := consumeFlagValue(args, &i, "--generator")
			if err != nil {
				return runCreateCLIOptions{}, err
			}
			options.create.GeneratorRef = value
		case "--version":
			value, err := consumeFlagValue(args, &i, "--version")
			if err != nil {
				return runCreateCLIOptions{}, err
			}
			options.create.GeneratorVersion = value
		case "--hash":
			value, err := consumeFlagValue(args, &i, "--hash")
			if err != nil {
				return runCreateCLIOptions{}, err
			}
			options.create.GeneratorHash = value
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return runCreateCLIOptions{}, err
			}
			options.create.Status = value
		case "--metadata":
			value, err := consumeFlagValue(args, &i, "--metadata")
			if err != nil {
				return runCreateCLIOptions{}, err
			}
			options.create.Metadata = value
		case "--report":
			value, err := consumeFlagValue(args, &i, "--report")
			if err != nil {
				return runCreateCLIOptions{}, err
			}
			options.create.Report = value
		default:
			return runCreateCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.create.GeneratorRef) == "" {
		return runCreateCLIOptions{}, fmt.Errorf("run create requires --generator")
	}
	return options, nil
}

func parseRunCompleteArgs(args []string) (runCompleteCLIOptions, error) {
	var options runCompleteCLIOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return runCompleteCLIOptions{}, err
			}
			options.complete.Status = value
		case "--metadata":
			value, err := consumeFlagValue(args, &i, "--metadata")
			if err != nil {
				return runCompleteCLIOptions{}, err
			}
			options.complete.Metadata = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return runCompleteCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return runCompleteCLIOptions{}, fmt.Errorf("run complete requires exactly one run")
	}
	options.complete.Run = positional[0]
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

func parseSpecNewArgs(args []string) (specNewOptions, error) {
	options := specNewOptions{create: state.SpecCreateOptions{Source: "ad-hoc"}}
	var positional []string
	for i := 0; i < len(args); i++ {
		if ok, err := parseBodyInputFlag(args, &i, &options.body); ok || err != nil {
			if err != nil {
				return specNewOptions{}, err
			}
			continue
		}
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return specNewOptions{}, err
			}
			options.create.Title = value
		case "--id":
			value, err := consumeFlagValue(args, &i, "--id")
			if err != nil {
				return specNewOptions{}, err
			}
			options.create.ID = value
		case "--source":
			value, err := consumeFlagValue(args, &i, "--source")
			if err != nil {
				return specNewOptions{}, err
			}
			options.create.Source = value
		case "--branch":
			value, err := consumeFlagValue(args, &i, "--branch")
			if err != nil {
				return specNewOptions{}, err
			}
			options.create.Branch = value
		case "--related":
			value, err := consumeFlagValue(args, &i, "--related")
			if err != nil {
				return specNewOptions{}, err
			}
			options.create.Related = append(options.create.Related, splitCommaList(value)...)
		default:
			if strings.HasPrefix(args[i], "-") {
				return specNewOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return specNewOptions{}, fmt.Errorf("spec new requires exactly one slug")
	}
	options.create.Slug = positional[0]
	return options, nil
}

func parseReportCreateArgs(args []string) (reportCreateOptions, error) {
	options := reportCreateOptions{create: state.ReportCreateOptions{Kind: "research", Source: "ad-hoc"}}
	var positional []string
	for i := 0; i < len(args); i++ {
		if ok, err := parseBodyInputFlag(args, &i, &options.body); ok || err != nil {
			if err != nil {
				return reportCreateOptions{}, err
			}
			continue
		}
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

func parseFindingListArgs(args []string) (findingListCLIOptions, error) {
	var options findingListCLIOptions
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			options.jsonOutput = true
			options.format = state.ExportFormatJSON
		case args[i] == "--format":
			value, err := consumeFlagValue(args, &i, "--format")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.format = normalizeFindingOutputFormat(value)
		case strings.HasPrefix(args[i], "--format="):
			options.format = normalizeFindingOutputFormat(strings.TrimPrefix(args[i], "--format="))
		case args[i] == "--report":
			value, err := consumeFlagValue(args, &i, "--report")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.filters.Report = value
		case args[i] == "--run":
			value, err := consumeFlagValue(args, &i, "--run")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.filters.Run = value
		case args[i] == "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.filters.Status = value
		case args[i] == "--severity":
			value, err := consumeFlagValue(args, &i, "--severity")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.filters.Severity = value
		case args[i] == "--confidence":
			value, err := consumeFlagValue(args, &i, "--confidence")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.filters.Confidence = value
		case args[i] == "--dimension":
			value, err := consumeFlagValue(args, &i, "--dimension")
			if err != nil {
				return findingListCLIOptions{}, err
			}
			options.filters.Dimension = value
		default:
			return findingListCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if !validFindingOutputFormat(options.format) {
		return findingListCLIOptions{}, fmt.Errorf("finding list supports --format json, csv, markdown, or html")
	}
	return options, nil
}

func parseFindingShowArgs(args []string) (findingShowCLIOptions, error) {
	var options findingShowCLIOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			options.jsonOutput = true
			options.format = state.ExportFormatJSON
		case args[i] == "--format":
			value, err := consumeFlagValue(args, &i, "--format")
			if err != nil {
				return findingShowCLIOptions{}, err
			}
			options.format = normalizeFindingOutputFormat(value)
		case strings.HasPrefix(args[i], "--format="):
			options.format = normalizeFindingOutputFormat(strings.TrimPrefix(args[i], "--format="))
		case strings.HasPrefix(args[i], "-"):
			return findingShowCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return findingShowCLIOptions{}, fmt.Errorf("finding show requires exactly one finding")
	}
	if !validFindingOutputFormat(options.format) {
		return findingShowCLIOptions{}, fmt.Errorf("finding show supports --format json, csv, markdown, or html")
	}
	options.ref = positional[0]
	return options, nil
}

func normalizeFindingOutputFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "md":
		return state.ExportFormatMarkdown
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}

func validFindingOutputFormat(format string) bool {
	switch format {
	case "", state.ExportFormatJSON, "csv", state.ExportFormatMarkdown, "html":
		return true
	default:
		return false
	}
}

func parseFindingCreateArgs(args []string) (findingCreateCLIOptions, error) {
	options := findingCreateCLIOptions{create: state.FindingCreateOptions{Severity: "medium", Confidence: "medium"}}
	for i := 0; i < len(args); i++ {
		if ok, err := parseBodyInputFlag(args, &i, &options.body); ok || err != nil {
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			continue
		}
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--report":
			value, err := consumeFlagValue(args, &i, "--report")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Report = value
		case "--run":
			value, err := consumeFlagValue(args, &i, "--run")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Run = value
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Title = value
		case "--status":
			value, err := consumeFlagValue(args, &i, "--status")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Status = value
		case "--severity":
			value, err := consumeFlagValue(args, &i, "--severity")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Severity = value
		case "--confidence":
			value, err := consumeFlagValue(args, &i, "--confidence")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Confidence = value
		case "--dimension":
			value, err := consumeFlagValue(args, &i, "--dimension")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Dimension = value
		case "--path":
			value, err := consumeFlagValue(args, &i, "--path")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Path = value
		case "--line-start":
			value, err := consumeFlagValue(args, &i, "--line-start")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			line, err := strconv.Atoi(value)
			if err != nil || line < 0 {
				return findingCreateCLIOptions{}, fmt.Errorf("--line-start must be a non-negative integer")
			}
			options.create.LineStart = line
		case "--line-end":
			value, err := consumeFlagValue(args, &i, "--line-end")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			line, err := strconv.Atoi(value)
			if err != nil || line < 0 {
				return findingCreateCLIOptions{}, fmt.Errorf("--line-end must be a non-negative integer")
			}
			options.create.LineEnd = line
		case "--symbol":
			value, err := consumeFlagValue(args, &i, "--symbol")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Symbol = value
		case "--metadata":
			value, err := consumeFlagValue(args, &i, "--metadata")
			if err != nil {
				return findingCreateCLIOptions{}, err
			}
			options.create.Metadata = value
		default:
			return findingCreateCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.create.Report) == "" {
		return findingCreateCLIOptions{}, fmt.Errorf("finding create requires --report")
	}
	if strings.TrimSpace(options.create.Title) == "" {
		return findingCreateCLIOptions{}, fmt.Errorf("finding create requires --title")
	}
	return options, nil
}

func parseFindingVerdictArgs(args []string) (findingVerdictCLIOptions, error) {
	var options findingVerdictCLIOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--outcome":
			value, err := consumeFlagValue(args, &i, "--outcome")
			if err != nil {
				return findingVerdictCLIOptions{}, err
			}
			options.verdict.Outcome = value
		case "--rationale":
			value, err := consumeFlagValue(args, &i, "--rationale")
			if err != nil {
				return findingVerdictCLIOptions{}, err
			}
			options.verdict.Rationale = value
		case "--run":
			value, err := consumeFlagValue(args, &i, "--run")
			if err != nil {
				return findingVerdictCLIOptions{}, err
			}
			options.verdict.Run = value
		case "--notes":
			value, err := consumeFlagValue(args, &i, "--notes")
			if err != nil {
				return findingVerdictCLIOptions{}, err
			}
			options.verdict.ReproductionNotes = value
		case "--metadata":
			value, err := consumeFlagValue(args, &i, "--metadata")
			if err != nil {
				return findingVerdictCLIOptions{}, err
			}
			options.verdict.Metadata = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return findingVerdictCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return findingVerdictCLIOptions{}, fmt.Errorf("finding verdict requires exactly one finding")
	}
	options.verdict.Finding = positional[0]
	if strings.TrimSpace(options.verdict.Outcome) == "" {
		return findingVerdictCLIOptions{}, fmt.Errorf("finding verdict requires --outcome")
	}
	if strings.TrimSpace(options.verdict.Rationale) == "" {
		return findingVerdictCLIOptions{}, fmt.Errorf("finding verdict requires --rationale")
	}
	return options, nil
}

func parseFindingImportJSONArgs(args []string) (findingImportJSONCLIOptions, error) {
	var options findingImportJSONCLIOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--report":
			value, err := consumeFlagValue(args, &i, "--report")
			if err != nil {
				return findingImportJSONCLIOptions{}, err
			}
			options.importJSON.Report = value
		case "--report-type":
			value, err := consumeFlagValue(args, &i, "--report-type")
			if err != nil {
				return findingImportJSONCLIOptions{}, err
			}
			options.importJSON.ReportKind = value
		case "--source":
			value, err := consumeFlagValue(args, &i, "--source")
			if err != nil {
				return findingImportJSONCLIOptions{}, err
			}
			options.importJSON.Source = value
		case "--run":
			value, err := consumeFlagValue(args, &i, "--run")
			if err != nil {
				return findingImportJSONCLIOptions{}, err
			}
			options.importJSON.Run = value
		case "--findings":
			value, err := consumeFlagValue(args, &i, "--findings")
			if err != nil {
				return findingImportJSONCLIOptions{}, err
			}
			options.importJSON.FindingFiles = append(options.importJSON.FindingFiles, value)
		case "--verdicts":
			value, err := consumeFlagValue(args, &i, "--verdicts")
			if err != nil {
				return findingImportJSONCLIOptions{}, err
			}
			options.importJSON.VerdictFiles = append(options.importJSON.VerdictFiles, value)
		default:
			return findingImportJSONCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.importJSON.Report) == "" {
		return findingImportJSONCLIOptions{}, fmt.Errorf("finding import-json requires --report")
	}
	if len(options.importJSON.FindingFiles) == 0 && len(options.importJSON.VerdictFiles) == 0 {
		return findingImportJSONCLIOptions{}, fmt.Errorf("finding import-json requires --findings or --verdicts")
	}
	return options, nil
}

func resolveFindingImportPaths(root string, paths []string) []string {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		if filepath.IsAbs(path) {
			resolved = append(resolved, path)
			continue
		}
		resolved = append(resolved, filepath.Join(root, path))
	}
	return resolved
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
	case state.LifecycleStatusInProgress:
		return "In Progress"
	case state.LifecycleStatusTodo:
		return "Todo"
	case state.LifecycleStatusDraft:
		return "Draft"
	case state.LifecycleStatusDone:
		return "Done"
	case state.LifecycleStatusArchived:
		return "Archived"
	default:
		return status
	}
}

func reportStatusDisplayOrder(reports state.ReportList) []string {
	statuses := []string{state.LifecycleStatusDraft, state.LifecycleStatusDone, state.LifecycleStatusArchived}
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
	case "done":
		return "Done"
	case "archived":
		return "Archived"
	default:
		return status
	}
}

type markdownMigrationOptions struct {
	jsonOutput   bool
	apply        bool
	dryRun       bool
	resume       bool
	backup       bool
	removeSource bool
	rollbackPath string
}

type storageHomeMigrationOptions struct {
	jsonOutput bool
	apply      bool
	dryRun     bool
}

type lifecycleStatusMigrationOptions struct {
	jsonOutput   bool
	apply        bool
	dryRun       bool
	rollbackPath string
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

type journalSearchRepairOptions struct {
	jsonOutput bool
	apply      bool
	dryRun     bool
}

func parseMarkdownMigrationArgs(args []string, command string) (markdownMigrationOptions, error) {
	var options markdownMigrationOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		case "--resume":
			options.resume = true
		case "--backup":
			options.backup = true
		case "--remove-source":
			options.removeSource = true
		case "--rollback":
			if i+1 >= len(args) {
				return markdownMigrationOptions{}, fmt.Errorf("%s requires --rollback <manifest>", command)
			}
			i++
			options.rollbackPath = args[i]
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
	if options.rollbackPath != "" {
		if options.apply || options.resume || options.dryRun || options.backup || options.removeSource {
			return markdownMigrationOptions{}, fmt.Errorf("%s cannot combine --rollback with --apply, --resume, --dry-run, --backup, or --remove-source", command)
		}
		return options, nil
	}
	if options.backup && !options.apply && !options.resume {
		return markdownMigrationOptions{}, fmt.Errorf("%s requires --apply or --resume with --backup", command)
	}
	if options.removeSource && !options.backup {
		return markdownMigrationOptions{}, fmt.Errorf("%s requires --backup with --remove-source", command)
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

func parseLifecycleStatusMigrationArgs(args []string, command string) (lifecycleStatusMigrationOptions, error) {
	var options lifecycleStatusMigrationOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		case "--rollback":
			if i+1 >= len(args) {
				return lifecycleStatusMigrationOptions{}, fmt.Errorf("%s requires --rollback <manifest>", command)
			}
			i++
			options.rollbackPath = args[i]
		default:
			return lifecycleStatusMigrationOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return lifecycleStatusMigrationOptions{}, fmt.Errorf("%s cannot combine --apply and --dry-run", command)
	}
	if options.rollbackPath != "" && (options.apply || options.dryRun) {
		return lifecycleStatusMigrationOptions{}, fmt.Errorf("%s cannot combine --rollback with --apply or --dry-run", command)
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

func parseJournalSearchRepairArgs(args []string) (journalSearchRepairOptions, error) {
	var options journalSearchRepairOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		case "--apply":
			options.apply = true
		default:
			return journalSearchRepairOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return journalSearchRepairOptions{}, fmt.Errorf("state repair journal-search cannot combine --apply and --dry-run")
	}
	return options, nil
}

func parseRelationshipOriginRepairArgs(args []string) (relationshipOriginRepairOptions, error) {
	var options relationshipOriginRepairOptions
	originSet := false
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
			originSet = true
		default:
			return relationshipOriginRepairOptions{}, fmt.Errorf("unknown option %q", arg)
		}
	}
	if options.apply && options.dryRun {
		return relationshipOriginRepairOptions{}, fmt.Errorf("state repair relationship-origin cannot combine --apply and --dry-run")
	}
	// Omitting --origin is valid: it selects reclassify-only, the mechanical
	// registry-derived repair. Supplying it opts into the missing-origin
	// backfill, which needs a real mechanism value — an explicitly empty one is
	// an operator mistake, not a mode selection.
	if originSet && options.origin != "imported" && options.origin != "manual" {
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
	output := commandErrorJSON{
		ContractVersion: state.StateJSONContractVersion,
		Command:         command,
		Error:           err.Error(),
	}
	var identityErr *state.UnregisteredProjectIdentityError
	if errors.As(err, &identityErr) {
		output.Code = identityErr.Code
		output.CurrentPath = identityErr.CurrentPath
		output.KnownCurrentPaths = identityErr.KnownCurrentPaths
		for _, moveCommand := range identityErr.MoveCommands {
			output.Suggestions = append(output.Suggestions, commandErrorSuggestion{Action: "move-project", Command: moveCommand})
		}
		initCommand := identityErr.InitCommand
		if initCommand == "" {
			initCommand = "loaf state init"
		}
		output.Suggestions = append(output.Suggestions, commandErrorSuggestion{Action: "initialize-project", Command: initCommand})
	}
	var journalSearchErr *state.JournalSearchDivergenceError
	if errors.As(err, &journalSearchErr) {
		output.Code = journalSearchErr.Code
		parity := journalSearchErr.Parity
		output.JournalSearchParity = &parity
	}
	var repairErr *state.JournalSearchRepairError
	if errors.As(err, &repairErr) {
		output.BackupPath = repairErr.Result.BackupPath
		output.BackupVerified = repairErr.Result.BackupVerified
		parity := state.JournalSearchParity{
			CanonicalRows: repairErr.Result.CanonicalRows,
			IndexRows:     repairErr.Result.IndexRows,
			Missing:       repairErr.Result.Missing,
			Extra:         repairErr.Result.Extra,
			Changed:       repairErr.Result.Changed,
			Ready:         repairErr.Result.Missing == 0 && repairErr.Result.Extra == 0 && repairErr.Result.Changed == 0 && repairErr.Result.CanonicalRows == repairErr.Result.IndexRows,
		}
		output.JournalSearchParity = &parity
	}
	var destinationErr *state.BackupDestinationError
	if errors.As(err, &destinationErr) {
		output.Code = destinationErr.Code
		output.RequestedDestinationDirectory = destinationErr.Requested
		output.ResolvedDestinationDirectory = destinationErr.Resolved
	}
	var recoveryErr *state.RecoveryError
	if errors.As(err, &recoveryErr) {
		output.Code = recoveryErr.Code
		output.BackupPath = recoveryErr.BackupPath
		output.RestorePath = recoveryErr.RestorePath
	}
	var changeOriginErr *ChangeOriginError
	if errors.As(err, &changeOriginErr) {
		output.Code = changeOriginErr.Code
		output.Ref = changeOriginErr.Ref
		output.Path = changeOriginErr.Path
	}
	var journalDeferValidationErr *state.JournalDeferValidationError
	if errors.As(err, &journalDeferValidationErr) {
		output.Code = "journal-defer-validation"
		output.Field = journalDeferValidationErr.Field
	}
	var journalDeferTransactionErr *state.JournalDeferTransactionError
	if errors.As(err, &journalDeferTransactionErr) {
		output.Code = "journal-defer-transaction"
		output.Field = journalDeferTransactionErr.Stage
	}
	var cursorInvalid *state.JournalContextCursorInvalidError
	if errors.As(err, &cursorInvalid) {
		output.Code = cursorInvalid.Code
	}
	var journalExecpolicySafeErr *journalExecpolicySafeError
	if errors.As(err, &journalExecpolicySafeErr) {
		output.Code = journalExecpolicySafeCode
	}
	var cursorStale *state.JournalContextCursorStaleError
	if errors.As(err, &cursorStale) {
		output.Code = cursorStale.Code
		output.Suggestion = cursorStale.RestartCommand
	}
	var activeCursorErr *journalContextCursorError
	if errors.As(err, &activeCursorErr) {
		output.Code = activeCursorErr.Code
		output.Suggestion = activeCursorErr.RestartCommand
	}
	var schemaUpgradeErr *state.SchemaUpgradeRequiredError
	if errors.As(err, &schemaUpgradeErr) {
		output.Code = schemaUpgradeErr.Code
		output.DatabasePath = schemaUpgradeErr.DatabasePath
		output.CurrentVersion = schemaUpgradeErr.CurrentVersion
		output.RequiredVersion = schemaUpgradeErr.RequiredVersion
		output.PendingVersions = schemaUpgradeErr.PendingVersions
		output.Suggestion = schemaUpgradeErr.Command
	}
	if command == "journal defer" && output.Code == "" {
		output.Code = "journal-defer-error"
	}
	if writeErr := writeJSON(out, output); writeErr != nil {
		return writeErr
	}
	return ExitError{Code: 1}
}

func writeStateBackupJSONError(out io.Writer, command string, result state.BackupResult, err error) error {
	output := commandErrorJSON{
		ContractVersion:               state.StateJSONContractVersion,
		Command:                       command,
		Error:                         err.Error(),
		BackupPath:                    result.BackupPath,
		RequestedDestinationDirectory: result.RequestedDestinationDirectory,
		ResolvedDestinationDirectory:  result.ResolvedDestinationDirectory,
		RecoveryTier:                  result.RecoveryTier,
		SHA256:                        result.SHA256,
		BackupResult:                  &result,
	}
	var destinationErr *state.BackupDestinationError
	if errors.As(err, &destinationErr) {
		output.Code = destinationErr.Code
		output.RequestedDestinationDirectory = destinationErr.Requested
		output.ResolvedDestinationDirectory = destinationErr.Resolved
	}
	if writeErr := writeJSON(out, output); writeErr != nil {
		return writeErr
	}
	return ExitError{Code: 1}
}

func writeStateRestoreJSONError(out io.Writer, result state.BackupRestoreResult, err error) error {
	output := commandErrorJSON{
		ContractVersion: state.StateJSONContractVersion,
		Command:         "state backup restore",
		Error:           err.Error(),
		BackupPath:      result.BackupPath,
		RestorePath:     result.RestorePath,
		SourceSHA256:    result.SourceSHA256,
		RestoredSHA256:  result.RestoredSHA256,
	}
	if result.ContractVersion != 0 || result.BackupPath != "" || result.RestorePath != "" || result.SourceSHA256 != "" || result.RestoredSHA256 != "" {
		output.RestoreResult = &result
	}
	var recoveryErr *state.RecoveryError
	if errors.As(err, &recoveryErr) {
		output.Code = recoveryErr.Code
		output.BackupPath = recoveryErr.BackupPath
		output.RestorePath = recoveryErr.RestorePath
	}
	if writeErr := writeJSON(out, output); writeErr != nil {
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
