package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// install.go owns `loaf install`, and install is onboarding: it adds Loaf to a
// harness that does not have it, and it deploys Loaf's surfaces into a project
// that is not yet a Loaf repo. Refreshing either of those in place is
// `loaf upgrade`'s job, and the two never overlap.
//
// The project half — AGENTS.md and its managed section, the instruction
// symlinks, and the MCP-recommendation record in .agents/loaf.json — sits
// behind one gate. Outside a Loaf repo the gate asks for consent before the
// first write; inside one it stays shut and points at `loaf upgrade`. The
// harness half runs either way, so `--to <target>` still onboards a net-new
// tool from inside a Loaf repo.

type installOptions struct {
	target             string
	codexBasicCommands bool
	yes                *bool
	help               bool
	// projectDeployGranted settles the onboarding consent gate before it is
	// reached. Only `loaf setup` sets it: setup scaffolds the project itself
	// moments earlier, so the decision is already made and the marker it just
	// wrote must not read back as somebody else's deployment.
	projectDeployGranted bool
	// upgrade, dryRun, and json are inputs to the shared plan builder that
	// `loaf upgrade` fills in through installPlanOptions. `loaf install` never
	// parses them; the flags that used to set them are tombstoned.
	upgrade bool
	dryRun  bool
	json    bool
	// command names the CLI entry point these options were parsed for. It is
	// the seam the shared plan builder reads to stay command-aware; see
	// planCommandName for how an unset value resolves.
	command string
}

type detectedInstallTool struct {
	key        string
	name       string
	configDir  string
	installed  bool
	detectedBy string
}

var installDisplayNames = map[string]string{
	"claude-code": "Claude Code",
	"opencode":    "OpenCode",
	"cursor":      "Cursor",
	"codex":       "Codex",
	"amp":         "Amp",
}

var installValidTargets = []string{"opencode", "cursor", "codex", "amp"}

func (r Runner) runInstall(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseInstallArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeInstallHelp(out)
		return nil
	}
	return r.runInstallWithOptions(options, out, runtimeRoot)
}

// runInstallWithOptions is the install flow proper, reachable with options no
// command line can produce. `loaf setup` is the one caller that needs that: it
// composes init + build + install and already owns the deploy decision.
func (r Runner) runInstallWithOptions(options installOptions, out io.Writer, runtimeRoot string) error {
	loafRoot, err := r.resolveInstalledDistributionRoot()
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtimeRoot)
	if err != nil {
		return err
	}
	version := packageVersion(loafRoot)
	distRoot := filepath.Join(loafRoot, "dist")
	tools := detectInstallTools()
	hasClaudeCode := installCommandExists("claude")
	assumeYes := installAssumeYes(options)
	detection := detectLoafRepo(projectRoot, r.StateHome)

	fmt.Fprintln(out)
	fmt.Fprintln(out, ansiBold("loaf install"))
	fmt.Fprintln(out)
	writeInstallDetection(out, tools, hasClaudeCode, loafRoot)

	selectedTargets, err := r.selectedInstallTargets(options, tools, out)
	if err != nil {
		return err
	}
	if options.codexBasicCommands && !containsString(selectedTargets, "codex") {
		return fmt.Errorf("Codex basic command policy requested, but Codex is not selected or detected")
	}

	if len(selectedTargets) == 0 {
		// Consent is only worth asking for when there is something to deploy.
		// With no target and no Claude Code the project half has no surface to
		// write — every fenced section and symlink it manages belongs to a
		// harness — so onboarding asks nothing and writes nothing.
		if hasClaudeCode {
			if err := r.deployInstallProjectSurfaces(out, options, projectRoot.Path(), detection, nil, hasClaudeCode, assumeYes, version); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "  %s\n\n", ansiGray("No targets selected"))
		return nil
	}

	if needsInstallPathBinary(selectedTargets) {
		r.ensureInstallPathBinary(out, loafRoot, selectedTargets)
	}

	var installedTargets []string
	var codexBasicCommandsErr error
	defaults, layoutHome := resolveInstallLayout(projectRoot.Path())
	toolByKey := installToolsByKey(tools)
	hookState, releaseHookState := r.hookStateForApply(projectRoot.Path())
	defer releaseHookState()
	var installOptions []targetInstallOptions
	for _, target := range selectedTargets {
		distDir := filepath.Join(distRoot, target)
		if !dirExistsForInstall(distDir) {
			fmt.Fprintf(out, "  %s %s - no build output found. Run %s first.\n", ansiRed("✗"), installDisplayName(target), ansiBold("loaf build"))
			if target == "codex" && options.codexBasicCommands {
				codexBasicCommandsErr = fmt.Errorf("Codex basic command policy requires built Codex output")
			}
			continue
		}
		configDir := defaults[target]
		if tool, ok := toolByKey[target]; ok && tool.configDir != "" {
			configDir = tool.configDir
		}
		installOptions = append(installOptions, targetInstallOptions{
			Target:             target,
			DistDir:            distDir,
			ConfigDir:          configDir,
			CodexBasicCommands: options.codexBasicCommands,
			Version:            version,
			HomeDir:            layoutHome,
			CodexHome:          resolveInstallCodexHome(configDir),
			ProjectRoot:        projectRoot.Path(),
			SkipSkillsSync:     true,
			HookState:          hookState,
		})
	}
	managedContentLock, err := acquireHarnessReconcileLock(managedContentLockDir(layoutHome), managedContentLockWait)
	if err != nil {
		return err
	}
	defer managedContentLock.release()
	skillsErr := syncCanonicalManagedSkills(installOptions)
	var skillConflicts *skillSyncConflictsError
	hardSkillsErr := skillsErr
	if errors.As(skillsErr, &skillConflicts) {
		// A current marker means the complete managed-content cohort is current.
		// Never stamp target adapters after a partial skills sync; leave every
		// marker stale so startup reconciliation can retry after conflicts clear.
		fmt.Fprintf(out, "  %s skills - %v\n", ansiRed("✗"), skillConflicts)
		return ExitError{Code: 1}
	}
	skillsErrReported := false
	for _, opts := range installOptions {
		if hardSkillsErr != nil {
			if !skillsErrReported {
				fmt.Fprintf(out, "  %s skills - %v\n", ansiRed("✗"), hardSkillsErr)
				skillsErrReported = true
			}
			if opts.Target == "codex" && options.codexBasicCommands {
				codexBasicCommandsErr = fmt.Errorf("Codex basic command policy installation failed: %w", hardSkillsErr)
			}
			continue
		}
		var hookActions []hookAction
		opts.HookActions = func(actions []hookAction) { hookActions = append(hookActions, actions...) }
		err := installTargetDistribution(opts)
		if err != nil {
			fmt.Fprintf(out, "  %s %s - %v\n", ansiRed("✗"), installDisplayName(opts.Target), err)
			if opts.Target == "codex" && options.codexBasicCommands {
				codexBasicCommandsErr = fmt.Errorf("Codex basic command policy installation failed: %w", err)
			}
			continue
		}
		fmt.Fprintf(out, "  %s %s installed to %s\n", ansiGreen("✓"), installDisplayName(opts.Target), ansiGray(opts.ConfigDir))
		writeHookActionLines(out, hookActions)
		if opts.Target == "codex" {
			if options.codexBasicCommands {
				fmt.Fprintf(out, "  %s Codex basic command policy explicitly enabled (Loaf-owned exact-prefix rules)\n", ansiGreen("✓"))
			} else {
				fmt.Fprintf(out, "  %s Codex basic command policy not installed; opt in with --codex-basic-commands\n", ansiGray("○"))
			}
		}
		installedTargets = append(installedTargets, opts.Target)
	}
	fmt.Fprintln(out)

	targetsInScope := append([]string{}, selectedTargets...)
	targetsInScope = append(targetsInScope, installedTargets...)
	if err := r.deployInstallProjectSurfaces(out, options, projectRoot.Path(), detection, targetsInScope, hasClaudeCode, assumeYes, version); err != nil {
		return err
	}
	if codexBasicCommandsErr != nil {
		return codexBasicCommandsErr
	}
	if hardSkillsErr != nil {
		return hardSkillsErr
	}
	return nil
}

// deployInstallProjectSurfaces is install's project half. Every write it can
// make lands inside the project, so all of it — files and the
// MCP-recommendation record alike — is one decision, taken before the first
// byte is written.
func (r Runner) deployInstallProjectSurfaces(out io.Writer, options installOptions, projectRoot string, detection loafRepoDetection, targets []string, hasClaudeCode bool, assumeYes bool, version string) error {
	deploy, err := r.installProjectDeployApproved(out, options, detection)
	if err != nil || !deploy {
		return err
	}
	outcome := r.enforceInstallProjectFiles(out, projectRoot, targets, hasClaudeCode, assumeYes, version, false)
	if outcome.wrote {
		fmt.Fprintln(out)
	}
	// A fenced-section write that failed means this project file is not Loaf's
	// to manage right now. The MCP interview would write into the same project
	// on that reading, so onboarding stops rather than half-deploying.
	if outcome.fenceFailed {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("Managed project file could not be written — skipping the remaining project surfaces. Resolve the reported conflict and rerun loaf install."))
		return nil
	}
	mcpTargets := append([]string{}, targets...)
	if hasClaudeCode {
		mcpTargets = append(mcpTargets, "claude-code")
	}
	return r.runInstallMcpRecommendations(out, projectRoot, false, mcpTargets)
}

// installProjectDeployApproved answers the only question onboarding asks about
// this directory: may install write into it? A Loaf repo answers no — its
// surfaces exist already and refreshing them is `loaf upgrade`'s job. Anything
// weaker than that, legacy artifacts included, is a folder Loaf has never been
// deployed to, so it takes consent: an explicit --yes, or a yes to the prompt.
// Without a terminal and without --yes, install reports the consent it needs
// and writes nothing rather than reading silence as agreement.
func (r Runner) installProjectDeployApproved(out io.Writer, options installOptions, detection loafRepoDetection) (bool, error) {
	if options.projectDeployGranted {
		return true, nil
	}
	if detection.Tier >= loafRepoTierStrong {
		fmt.Fprintf(out, "  %s %s\n\n", ansiGray("○"), ansiGray("Loaf is already deployed here ("+detection.Bases[0]+") — run loaf upgrade to refresh project surfaces"))
		return false, nil
	}
	if options.yes != nil && *options.yes {
		return true, nil
	}
	if !r.installPromptInteractive() {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("Deploy consent required: rerun with --yes, or answer \"Deploy Loaf to this folder?\" in a terminal. No project files written."))
		return false, nil
	}
	if detection.Tier == loafRepoTierLegacy {
		fmt.Fprintf(out, "  %s %s\n", ansiYellow("⚠"), ansiGray(detection.Bases[0]))
	}
	yes, err := askInstallYesNo(r.installPromptReader(), out, "  No Loaf project detected here. Deploy Loaf to this folder? [y/N] ", false)
	if err != nil {
		return false, err
	}
	if !yes {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("Skipping project files. Harness content only."))
		return false, nil
	}
	return true, nil
}

func parseInstallArgs(args []string) (installOptions, error) {
	var options installOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--to":
			if i+1 >= len(args) {
				return installOptions{}, fmt.Errorf("--to requires a value")
			}
			i++
			options.target = args[i]
		case "--upgrade":
			return installOptions{}, installFlagRemovedError("--upgrade", "loaf upgrade")
		case "--dry-run":
			return installOptions{}, installFlagRemovedError("--dry-run", "loaf upgrade --dry-run")
		case "--json":
			return installOptions{}, installFlagRemovedError("--json", "loaf upgrade --dry-run --json")
		case "--codex-basic-commands":
			options.codexBasicCommands = true
		case "-y", "--yes":
			value := true
			options.yes = &value
		case "--no-yes":
			value := false
			options.yes = &value
		case "--help", "-h":
			options.help = true
		default:
			return installOptions{}, fmt.Errorf("unknown install option %q", arg)
		}
	}
	if options.codexBasicCommands && options.target != "codex" && options.target != "all" {
		return installOptions{}, fmt.Errorf("--codex-basic-commands requires --to codex or --to all")
	}
	return options, nil
}

// installFlagRemovedError tombstones a flag that left install for
// `loaf upgrade`. It names the replacement without ever spelling the removed
// invocation, so the sweep proving no live surface still advertises that
// invocation needs no exception for the tombstone itself.
func installFlagRemovedError(flag string, replacement string) error {
	return fmt.Errorf("the %q flag was removed from install; run %q instead", flag, replacement)
}

func writeInstallHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf install [options]",
		"",
		"Onboard Loaf. Install does two things, and only ever adds:",
		"",
		"  Harness  Installs the Loaf distribution into an agent tool's global config",
		"           directory. Use --to <target> to onboard a tool that does not have",
		"           it yet, from anywhere.",
		"  Project  Deploys Loaf's project surfaces here: AGENTS.md and its managed",
		"           section, the instruction symlinks, and the MCP recommendation",
		"           record in .agents/loaf.json. Outside a Loaf repo install asks",
		"           before writing anything; inside one it leaves the project alone.",
		"",
		"Options:",
		"  --to <target>  Target to install to (or \"all\")",
		"  --codex-basic-commands  Explicitly install the least-privilege Codex basic command policy (requires --to codex or --to all)",
		"  -y, --yes      Assume yes: grants project deploy consent and safe project-file symlink migrations",
		"  --no-yes       Force prompt-style declines in non-interactive mode",
		"  -h, --help     Show help",
		"",
		"Refreshing an install that already exists is loaf upgrade's job.",
	}, "\n"))
}

func (r Runner) selectedInstallTargets(options installOptions, tools []detectedInstallTool, out io.Writer) ([]string, error) {
	switch {
	case options.target == "all":
		targets := make([]string, 0, len(tools))
		for _, tool := range tools {
			targets = append(targets, tool.key)
		}
		return targets, nil
	case options.target != "":
		if !isValidInstallTarget(options.target) {
			return nil, fmt.Errorf("unknown install target %q (valid targets: %s, all)", options.target, strings.Join(installValidTargets, ", "))
		}
		if !containsInstallTool(tools, options.target) {
			fmt.Fprintf(out, "  %s %s was not auto-detected; installing to %s\n", ansiYellow("⚡"), options.target, installLayoutConfigDirs("")[options.target])
		}
		return []string{options.target}, nil
	case options.upgrade:
		var targets []string
		for _, tool := range tools {
			if tool.installed {
				targets = append(targets, tool.key)
			}
		}
		return targets, nil
	default:
		return r.promptInstallTargets(tools, out)
	}
}

func (r Runner) promptInstallTargets(tools []detectedInstallTool, out io.Writer) ([]string, error) {
	input := r.Stdin
	if input == nil {
		input = os.Stdin
	}
	reader := bufio.NewReader(input)
	var selected []string
	for _, tool := range tools {
		status := ""
		if tool.installed {
			status = " " + ansiYellow("(installed)")
		}
		yes, err := askInstallYesNo(reader, out, fmt.Sprintf("  Install to %s%s? [Y/n] ", ansiBold(tool.name), status), true)
		if err != nil {
			return nil, err
		}
		if yes {
			selected = append(selected, tool.key)
		}
	}
	return selected, nil
}

func askInstallYesNo(reader *bufio.Reader, out io.Writer, question string, defaultYes bool) (bool, error) {
	fmt.Fprint(out, question)
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	trimmed := strings.TrimSpace(strings.ToLower(answer))
	if trimmed == "" {
		return defaultYes, nil
	}
	return strings.HasPrefix(trimmed, "y"), nil
}

func (r Runner) ensureInstallPathBinary(out io.Writer, loafRoot string, selectedTargets []string) {
	if installCommandExists("loaf") {
		fmt.Fprintf(out, "  %s loaf binary available on PATH for target hooks\n\n", ansiGreen("✓"))
		return
	}

	fmt.Fprintf(out, "  %s Selected targets need 'loaf' on PATH but it is not available\n", ansiYellow("⚠"))
	fmt.Fprintf(out, "  %s %s\n", ansiGray("Targets:"), strings.Join(pathDependentInstallTargets(selectedTargets), ", "))

	if !r.installPromptInteractive() {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("Skipping binary installation. Some hooks may not work for PATH-dependent targets."))
		return
	}

	yes, err := askInstallYesNo(r.installPromptReader(), out, "  Install 'loaf' binary to ~/.local/bin? [y/N] ", false)
	if err != nil {
		fmt.Fprintf(out, "  %s Could not read binary install prompt: %v\n\n", ansiYellow("⚠"), err)
		return
	}
	if !yes {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("Skipping binary installation. Some hooks may not work for PATH-dependent targets."))
		return
	}
	installLoafBinary(out, loafRoot)
	fmt.Fprintln(out)
}

func (r Runner) installPromptInteractive() bool {
	if r.Stdin != nil {
		return true
	}
	stat, err := os.Stdin.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}

func (r Runner) installPromptReader() *bufio.Reader {
	input := r.Stdin
	if input == nil {
		input = os.Stdin
	}
	return bufio.NewReader(input)
}

func installLoafBinary(out io.Writer, loafRoot string) bool {
	home := installHome()
	if home == "" {
		fmt.Fprintf(out, "  %s Could not determine home directory for ~/.local/bin\n", ansiRed("✗"))
		return false
	}

	localBinDir := filepath.Join(home, ".local", "bin")
	sourceBinary := filepath.Join(loafRoot, "bin", "loaf")
	sourceNative := filepath.Join(loafRoot, "bin", "native")
	targetBinary := filepath.Join(localBinDir, "loaf")
	targetNative := filepath.Join(localBinDir, "native")

	if !fileExistsForInstall(sourceBinary) {
		fmt.Fprintf(out, "  %s CLI binary not found at %s\n", ansiRed("✗"), sourceBinary)
		fmt.Fprintf(out, "  %s\n", ansiGray("Run 'npm run build' first."))
		return false
	}
	if !dirExistsForInstall(sourceNative) {
		fmt.Fprintf(out, "  %s Native runtime artifacts not found at %s\n", ansiRed("✗"), sourceNative)
		fmt.Fprintf(out, "  %s\n", ansiGray("Run 'npm run build' first."))
		return false
	}
	if err := os.MkdirAll(localBinDir, 0o755); err != nil {
		fmt.Fprintf(out, "  %s Could not create %s: %v\n", ansiRed("✗"), localBinDir, err)
		return false
	}
	if err := os.Remove(targetBinary); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(out, "  %s Failed to replace binary: %v\n", ansiRed("✗"), err)
		return false
	}
	if err := copyFileWithModeForInstall(sourceBinary, targetBinary, 0o755); err != nil {
		fmt.Fprintf(out, "  %s Failed to install binary: %v\n", ansiRed("✗"), err)
		return false
	}
	if err := os.RemoveAll(targetNative); err != nil {
		fmt.Fprintf(out, "  %s Failed to replace native artifacts: %v\n", ansiRed("✗"), err)
		return false
	}
	if err := copyDirContentsForInstall(sourceNative, targetNative); err != nil {
		fmt.Fprintf(out, "  %s Failed to install native artifacts: %v\n", ansiRed("✗"), err)
		return false
	}

	fmt.Fprintf(out, "  %s Installed loaf binary to %s\n", ansiGreen("✓"), targetBinary)
	if !pathContainsInstallDir(os.Getenv("PATH"), localBinDir) {
		fmt.Fprintf(out, "  %s %s is not on your PATH\n", ansiYellow("⚠"), localBinDir)
		fmt.Fprintf(out, "  %s\n", ansiGray("Add this to your shell profile:"))
		fmt.Fprintf(out, "  %s\n", ansiGray("  export PATH=\""+localBinDir+":$PATH\""))
	}
	return true
}

// installProjectFileOutcome reports what the managed project-file pass did:
// whether it printed anything, and whether the pass refused to finish. The
// second field is what stops the writes that come after it.
//
// Both halves of the pass can raise it. A fenced write fails on a section that
// is not Loaf's to manage; the symlink pass fails on a path it was about to
// read whole that is not a file it can read whole. They are the same statement
// about the project, so they get the same answer.
type installProjectFileOutcome struct {
	wrote       bool
	fenceFailed bool
}

// enforceInstallProjectFiles writes the managed project files: the instruction
// symlinks first, then the fenced sections. That order is load-bearing rather
// than incidental — the fence write resolves symlinks before choosing a path,
// so `.claude/CLAUDE.md` must already point at `AGENTS.md` when it runs, or the
// section lands in a real file the symlink pass would then offer to replace.
func (r Runner) enforceInstallProjectFiles(out io.Writer, projectRoot string, selectedTargets []string, hasClaudeCode bool, assumeYes bool, version string, upgrade bool) installProjectFileOutcome {
	symlinkResults := ensureProjectInstallSymlinks(projectRoot, selectedTargets, hasClaudeCode, installSymlinkOptions{
		AssumeYes:      assumeYes,
		NonInteractive: !assumeYes,
	})
	outcome := installProjectFileOutcome{wrote: writeInstallSymlinkResults(out, symlinkResults)}
	if anyInstallSymlinkRefusal(symlinkResults) {
		outcome.fenceFailed = true
		return outcome
	}
	fencedTargets := append([]string{}, selectedTargets...)
	if hasClaudeCode {
		fencedTargets = append([]string{"claude-code"}, fencedTargets...)
	}
	fencedResults, fencedErr := installFencedSectionsForTargets(fencedTargets, projectRoot, version, upgrade)
	if writeInstallFencedResults(out, fencedResults) {
		outcome.wrote = true
	}
	outcome.fenceFailed = fencedErr != nil
	return outcome
}

// anyInstallSymlinkRefusal reports whether the symlink pass refused a path it
// could not read whole. The fenced pass runs after it and writes into the same
// files, so a refusal stops the pass here rather than letting the next writer
// meet the same path.
func anyInstallSymlinkRefusal(results map[string]installSymlinkResult) bool {
	for _, result := range results {
		if result.Refused {
			return true
		}
	}
	return false
}

func writeInstallDetection(out io.Writer, tools []detectedInstallTool, hasClaudeCode bool, loafRoot string) {
	if hasClaudeCode {
		fmt.Fprintf(out, "  %s Claude Code detected\n", ansiGreen("✓"))
		if isInstallDevMode(loafRoot) {
			fmt.Fprintf(out, "    %s %s\n", ansiGray("Test locally:"), ansiWhite("/plugin marketplace add "+loafRoot))
		} else {
			fmt.Fprintf(out, "    %s %s\n", ansiGray("Install via:"), ansiWhite("/plugin marketplace add levifig/loaf"))
		}
		fmt.Fprintln(out)
	}
	for _, tool := range tools {
		status := ""
		if tool.installed {
			status = " " + ansiYellow("(installed)")
		}
		fmt.Fprintf(out, "  %s %s detected%s\n", ansiGreen("✓"), tool.name, status)
	}
	if len(tools) == 0 && !hasClaudeCode {
		fmt.Fprintf(out, "  %s\n", ansiGray("No AI tools detected"))
	}
	fmt.Fprintln(out)
}

func writeInstallSymlinkResults(out io.Writer, results map[string]installSymlinkResult) bool {
	wrote := false
	anySkippedNoTTY := false
	for _, key := range sortedInstallSymlinkResultKeys(results) {
		result := results[key]
		switch result.Action {
		case "created", "relinked", "replaced-file":
			fmt.Fprintf(out, "  %s %s\n", ansiGreen("✓"), result.Message)
			wrote = true
		case "declined-relink", "declined-replace":
			fmt.Fprintf(out, "  %s %s\n", ansiYellow("⚠"), result.Message)
			wrote = true
		case "skipped-no-tty":
			anySkippedNoTTY = true
		case "error":
			fmt.Fprintf(out, "  %s %s\n", ansiRed("✗"), result.Message)
			wrote = true
		}
	}
	if anySkippedNoTTY {
		fmt.Fprintf(out, "  %s\n", ansiGray("Note: symlinks not enforced (non-interactive); run loaf doctor to check."))
		wrote = true
	}
	return wrote
}

func writeInstallFencedResults(out io.Writer, results map[string]fencedInstallResult) bool {
	wrote := false
	for _, target := range sortedFencedInstallTargets(results) {
		result := results[target]
		name := installDisplayName(target)
		switch result.Action {
		case "created":
			fmt.Fprintf(out, "  %s %s created project file with Loaf framework section\n", ansiGreen("✓"), name)
			wrote = true
		case "appended":
			fmt.Fprintf(out, "  %s %s added Loaf framework section to project file\n", ansiGreen("✓"), name)
			wrote = true
		case "updated":
			fmt.Fprintf(out, "  %s %s updated Loaf framework section in project file (v%s)\n", ansiGreen("✓"), name, result.Version)
			wrote = true
		case "skipped":
			fmt.Fprintf(out, "  %s %s Loaf framework section already current (v%s)\n", ansiGray("○"), name, result.Version)
			wrote = true
		case "error":
			fmt.Fprintf(out, "  %s %s project file - %s\n", ansiRed("✗"), name, result.Error)
			wrote = true
		}
	}
	return wrote
}

func detectInstallTools() []detectedInstallTool {
	return detectInstallToolsForProject("")
}

func detectInstallToolsForProject(projectRoot string) []detectedInstallTool {
	home := installLayoutHome(projectRoot)
	defaults := installLayoutConfigDirs(projectRoot)
	var tools []detectedInstallTool
	if dirExistsForInstall(defaults["opencode"]) || installRecordExists(home, "opencode") {
		tools = append(tools, newDetectedInstallTool("opencode", defaults["opencode"], "config", home))
	}
	cursorConfig := defaults["cursor"]
	switch {
	case installCommandExists("cursor"):
		tools = append(tools, newDetectedInstallTool("cursor", cursorConfig, "cli", home))
	case runtime.GOOS == "darwin" && (dirExistsForInstall("/Applications/Cursor.app") || dirExistsForInstall(filepath.Join(home, "Applications", "Cursor.app"))):
		tools = append(tools, newDetectedInstallTool("cursor", cursorConfig, "app", home))
	case dirExistsForInstall(cursorConfig) || installRecordExists(home, "cursor"):
		tools = append(tools, newDetectedInstallTool("cursor", cursorConfig, "config", home))
	}
	codexConfig := defaults["codex"]
	if installCommandExists("codex") || dirExistsForInstall(codexConfig) || dirExistsForInstall(filepath.Join(home, ".codex")) || installRecordExists(home, "codex") {
		tools = append(tools, newDetectedInstallTool("codex", codexConfig, "config", home))
	}
	ampConfig := defaults["amp"]
	legacyAmpConfig := filepath.Join(home, ".amp")
	if installCommandExists("amp") || dirExistsForInstall(ampConfig) || dirExistsForInstall(legacyAmpConfig) || installRecordExists(home, "amp") {
		tools = append(tools, newDetectedInstallTool("amp", ampConfig, "config", home))
	}
	return tools
}

func newDetectedInstallTool(key string, configDir string, detectedBy string, recordHome string) detectedInstallTool {
	return detectedInstallTool{
		key:        key,
		name:       installDisplayName(key),
		configDir:  configDir,
		installed:  isLoafInstalledForTargetInstall(key, configDir, recordHome),
		detectedBy: detectedBy,
	}
}

func defaultInstallConfigDirs() map[string]string {
	home := installHome()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	codexHome := strings.TrimRight(os.Getenv("CODEX_HOME"), string(filepath.Separator))
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	return map[string]string{
		"opencode": filepath.Join(xdgConfig, "opencode"),
		"cursor":   filepath.Join(home, ".cursor"),
		"codex":    codexHome,
		"amp":      filepath.Join(xdgConfig, "amp"),
	}
}

func isLoafInstalledForInstall(configDir string) bool {
	if fileExistsForInstall(filepath.Join(configDir, loafInstallMarkerFile)) {
		return true
	}
	for _, skill := range []string{"foundations", "python-development", "python"} {
		if dirExistsForInstall(filepath.Join(configDir, "skills", skill)) {
			return true
		}
	}
	return false
}

func isLoafInstalledForTargetInstall(target string, configDir string, recordHome string) bool {
	if recordHome == "" {
		recordHome = installHome()
	}
	if isLoafInstalledForInstall(configDir) || installRecordExists(recordHome, target) {
		return true
	}
	if target == "amp" {
		return isLoafInstalledForInstall(filepath.Join(recordHome, ".amp"))
	}
	return false
}

func installRecordExists(homeDir string, target string) bool {
	if homeDir == "" || target == "" {
		return false
	}
	return fileExistsForInstall(installRecordPath(homeDir, target))
}

func installAssumeYes(options installOptions) bool {
	if options.yes != nil {
		return *options.yes
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return stat.Mode()&os.ModeCharDevice == 0
}

func needsInstallPathBinary(targets []string) bool {
	return len(pathDependentInstallTargets(targets)) > 0
}

func pathDependentInstallTargets(targets []string) []string {
	pathDependent := map[string]bool{"opencode": true, "cursor": true, "codex": true, "amp": true}
	var result []string
	for _, target := range targets {
		if pathDependent[target] {
			result = append(result, target)
		}
	}
	return result
}

func pathContainsInstallDir(pathEnv string, dir string) bool {
	if pathEnv == "" || dir == "" {
		return false
	}
	for _, entry := range filepath.SplitList(pathEnv) {
		if entry == dir {
			return true
		}
	}
	return false
}

func installToolsByKey(tools []detectedInstallTool) map[string]detectedInstallTool {
	result := map[string]detectedInstallTool{}
	for _, tool := range tools {
		result[tool.key] = tool
	}
	return result
}

func containsInstallTool(tools []detectedInstallTool, target string) bool {
	for _, tool := range tools {
		if tool.key == target {
			return true
		}
	}
	return false
}

func isValidInstallTarget(target string) bool {
	for _, valid := range installValidTargets {
		if target == valid {
			return true
		}
	}
	return false
}

func installDisplayName(target string) string {
	if name, ok := installDisplayNames[target]; ok {
		return name
	}
	return target
}

func sortedInstallSymlinkResultKeys(results map[string]installSymlinkResult) []string {
	keys := make([]string, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFencedInstallTargets(results map[string]fencedInstallResult) []string {
	keys := make([]string, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func installHome() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

func installCommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func isInstallDevMode(root string) bool {
	return dirExistsForInstall(filepath.Join(root, ".git")) &&
		fileExistsForInstall(filepath.Join(root, "package.json")) &&
		dirExistsForInstall(filepath.Join(root, "content", "skills"))
}

func ansiWhite(value string) string {
	return "\x1b[97m" + value + "\x1b[0m"
}
