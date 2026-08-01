package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// upgrade.go owns `loaf upgrade`, the canonical in-place refresh. It has two
// parts with different scopes and they never blur: the global part syncs every
// installed harness config dir from the installed distribution and runs
// deprecation cleanup, and runs anywhere; the project part refreshes this
// repo's Loaf surfaces and runs only when the tiered detector says this is a
// Loaf repo. Every project write — fenced sections, instruction symlinks and
// their migrations, and the MCP-recommendation record in .agents/loaf.json —
// sits behind that gate, so `loaf upgrade` in a stranger's directory leaves it
// byte-identical.

// upgradeCommandName is the one name this operation answers to. Plans, apply
// commands, and remediation strings are all built from it.
const upgradeCommandName = "upgrade"

// upgradeAllTargets is the explicit spelling of the unfiltered default.
const upgradeAllTargets = "all"

type upgradeOptions struct {
	target string
	yes    *bool
	help   bool
	dryRun bool
	json   bool
}

func (r Runner) runUpgrade(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseUpgradeArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeUpgradeHelp(out)
		return nil
	}

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
	planOptions := options.installPlanOptions()
	assumeYes := installAssumeYes(planOptions)
	detection := detectLoafRepo(projectRoot, r.StateHome)

	targets, err := selectUpgradeTargets(options, tools)
	if err != nil {
		return err
	}

	if options.dryRun {
		return r.runInstallDryRun(planOptions, out, loafRoot, projectRoot.Path(), version, distRoot, tools, hasClaudeCode, assumeYes, planUpgradeProjectPart(detection))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, ansiBold("loaf upgrade"))
	fmt.Fprintln(out)

	failedTargets, err := r.upgradeInstalledTargets(out, options, targets, tools, loafRoot, distRoot, version, projectRoot.Path())
	if err != nil {
		return err
	}
	// `--to` filters the global sync only. The project surfaces describe every
	// harness this repo is set up for, so narrowing them to the synced target
	// would silently retire the others' fenced sections and symlinks.
	projectFailed, err := r.refreshUpgradeProjectSurfaces(out, projectRoot.Path(), detection, installedUpgradeTargets(tools), hasClaudeCode, assumeYes, version)
	if err != nil {
		return err
	}
	// Every part has had its turn by now, which is the point: a target that
	// could not be synced does not stop the ones after it, and the summary that
	// names the failures is written once, at the end, over the whole run.
	failure := upgradeFailureSummary(failedTargets, projectFailed)
	if failure != "" {
		fmt.Fprintf(out, "  %s %s\n\n", ansiRed("✗"), failure)
	}
	// The epilogue: content is now current, but the binary that synced it may
	// not be. The advisory is best-effort and never affects what came before it
	// (see upgrade_advisory.go).
	writeUpgradeCurrencyAdvisory(out, loafRoot, version)
	if failure != "" {
		return ExitError{Code: 1}
	}
	return nil
}

// upgradeFailureSummary names what did not finish, or "" when everything did.
// Reporting failures on stdout and carrying them out as an exit code is what
// makes `loaf upgrade` usable from a script: the per-target lines scroll past,
// the exit status does not.
func upgradeFailureSummary(failedTargets []string, projectFailed bool) string {
	var parts []string
	if len(failedTargets) > 0 {
		names := make([]string, 0, len(failedTargets))
		for _, target := range failedTargets {
			names = append(names, installDisplayName(target))
		}
		parts = append(parts, "harness content not synced for "+strings.Join(names, ", "))
	}
	if projectFailed {
		parts = append(parts, "project surfaces incomplete")
	}
	if len(parts) == 0 {
		return ""
	}
	return "Upgrade finished with errors: " + strings.Join(parts, "; ")
}

func parseUpgradeArgs(args []string) (upgradeOptions, error) {
	var options upgradeOptions
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--to":
			if i+1 >= len(args) {
				return upgradeOptions{}, fmt.Errorf("--to requires a value")
			}
			i++
			options.target = args[i]
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.json = true
		case "-y", "--yes":
			value := true
			options.yes = &value
		case "--no-yes":
			value := false
			options.yes = &value
		case "--help", "-h":
			options.help = true
		default:
			return upgradeOptions{}, fmt.Errorf("unknown upgrade option %q", arg)
		}
	}
	if options.json && !options.dryRun {
		return upgradeOptions{}, fmt.Errorf("--json requires --dry-run")
	}
	return options, nil
}

// installPlanOptions projects the upgrade flags onto the option struct the
// shared install machinery speaks. The unfiltered default maps to the empty
// target the plan builder already reads as "every installed target".
func (o upgradeOptions) installPlanOptions() installOptions {
	target := o.target
	if target == upgradeAllTargets {
		target = ""
	}
	return installOptions{
		target:  target,
		upgrade: true,
		yes:     o.yes,
		dryRun:  o.dryRun,
		json:    o.json,
		command: upgradeCommandName,
	}
}

// selectUpgradeTargets narrows the sync to already-installed targets. `--to`
// filters; it never onboards. Naming a target that is not installed is an
// error that points at install, which owns onboarding.
func selectUpgradeTargets(options upgradeOptions, tools []detectedInstallTool) ([]string, error) {
	installed := installedUpgradeTargets(tools)
	if options.target == "" || options.target == upgradeAllTargets {
		return installed, nil
	}
	if !isValidInstallTarget(options.target) {
		return nil, fmt.Errorf("unknown upgrade target %q (valid targets: %s, %s)", options.target, strings.Join(installValidTargets, ", "), upgradeAllTargets)
	}
	if !containsString(installed, options.target) {
		return nil, fmt.Errorf("%s is not installed here, so there is nothing to upgrade; run `loaf install --to %s` to add it", installDisplayName(options.target), options.target)
	}
	return []string{options.target}, nil
}

// installedUpgradeTargets is the unfiltered set the project part always works
// from: every harness that actually carries Loaf content here.
func installedUpgradeTargets(tools []detectedInstallTool) []string {
	var installed []string
	for _, tool := range tools {
		if tool.installed {
			installed = append(installed, tool.key)
		}
	}
	return installed
}

// upgradeInstalledTargets is the global part: deprecation cleanup followed by a
// content sync of each installed harness from the installed distribution.
// installTargetDistribution stamps every .loaf-version marker as it goes. It
// returns the targets that could not be synced — one broken harness must not
// cost the others their refresh, so the failures are collected rather than
// raised, and the caller decides the exit code once.
func (r Runner) upgradeInstalledTargets(out io.Writer, options upgradeOptions, targets []string, tools []detectedInstallTool, loafRoot string, distRoot string, version string, projectRoot string) ([]string, error) {
	if len(targets) == 0 {
		fmt.Fprintf(out, "  %s\n", ansiGray("No installed targets to upgrade"))
	} else {
		fmt.Fprintf(out, "  %s %s\n", ansiGray("Upgrading:"), strings.Join(targets, ", "))
	}

	// Destructive deprecation cleanup requires an explicit --yes; a
	// non-interactive run without it reports the requirement and applies
	// nothing, never assuming consent from the absence of a terminal.
	allowDestructiveCleanup := options.yes != nil && *options.yes
	if err := runInstallDeprecationCleanup(loafRoot, out, allowDestructiveCleanup); err != nil {
		return nil, err
	}

	var failed []string
	defaults := defaultInstallConfigDirs()
	toolByKey := installToolsByKey(tools)
	for _, target := range targets {
		distDir := filepath.Join(distRoot, target)
		if !dirExistsForInstall(distDir) {
			fmt.Fprintf(out, "  %s %s - no build output found. Run %s first.\n", ansiRed("✗"), installDisplayName(target), ansiBold("loaf build"))
			failed = append(failed, target)
			continue
		}
		configDir := defaults[target]
		if tool, ok := toolByKey[target]; ok && tool.configDir != "" {
			configDir = tool.configDir
		}
		err := installTargetDistribution(targetInstallOptions{
			Target:      target,
			DistDir:     distDir,
			ConfigDir:   configDir,
			Upgrade:     true,
			Version:     version,
			HomeDir:     installHome(),
			CodexHome:   os.Getenv("CODEX_HOME"),
			ProjectRoot: projectRoot,
		})
		if err != nil {
			fmt.Fprintf(out, "  %s %s - %v\n", ansiRed("✗"), installDisplayName(target), err)
			failed = append(failed, target)
			continue
		}
		fmt.Fprintf(out, "  %s %s refreshed at %s (v%s)\n", ansiGreen("✓"), installDisplayName(target), ansiGray(configDir), version)
	}
	fmt.Fprintln(out)
	return failed, nil
}

// refreshUpgradeProjectSurfaces is the project part. It runs only behind the
// detector gate; when the gate is closed it writes nothing at all. It reports
// whether the part completed, which is the second half of the run's exit code.
func (r Runner) refreshUpgradeProjectSurfaces(out io.Writer, projectRoot string, detection loafRepoDetection, targets []string, hasClaudeCode bool, assumeYes bool, version string) (bool, error) {
	proceed, err := r.upgradeProjectPartInScope(out, detection)
	if err != nil || !proceed {
		return false, err
	}

	outcome := r.enforceInstallProjectFiles(out, projectRoot, targets, hasClaudeCode, assumeYes, version, true)
	if outcome.wrote {
		fmt.Fprintln(out)
	}
	// A fenced-section write fails when the managed section was tampered with,
	// its header is malformed, or the fences are unbalanced — all of which say
	// this project file is not currently Loaf's to manage. Continuing to the
	// recommendation record would leave the repo half-refreshed on that reading,
	// so the part stops here and reports itself failed.
	if outcome.fenceFailed {
		fmt.Fprintf(out, "  %s %s\n\n", ansiYellow("⚠"), ansiGray("Managed project file could not be written — skipping the remaining project surfaces. Resolve the reported conflict and rerun loaf upgrade."))
		return true, nil
	}

	// A project config Loaf cannot read is preserved and reported, not repaired
	// and not treated as a failed run: declining to overwrite somebody's file is
	// the correct outcome, and the line says what was left alone and why.
	config, err := readInstallLoafConfigDocument(projectRoot)
	if err != nil {
		if writeMalformedLoafConfigReport(out, err) {
			return false, nil
		}
		return true, err
	}
	pending := pendingMcpRecommendationsIn(config)
	if len(pending) == 0 {
		return false, nil
	}
	if err := recordDefaultInstallMcpChoices(projectRoot); err != nil {
		return true, err
	}
	fmt.Fprintf(out, "  %s Recorded new MCP recommendations (%s) in %s\n\n", ansiGreen("✓"), strings.Join(pending, ", "), ansiGray(".agents/loaf.json"))
	return false, nil
}

// upgradeProjectPartInScope applies the tiered detector's confirmation floor:
// authoritative and strong signals proceed and print their basis, legacy
// signals alone ask, and no signal skips with a single line.
func (r Runner) upgradeProjectPartInScope(out io.Writer, detection loafRepoDetection) (bool, error) {
	switch {
	case detection.Tier >= loafRepoTierStrong:
		fmt.Fprintf(out, "  %s Loaf project detected — %s\n", ansiGreen("✓"), ansiGray(detection.Bases[0]))
		return true, nil
	case detection.Tier == loafRepoTierLegacy:
		return r.confirmLegacyUpgradeProject(out, detection)
	default:
		fmt.Fprintf(out, "  %s\n", ansiGray("No Loaf project here — harness content only. Run loaf install to deploy Loaf to this folder."))
		return false, nil
	}
}

func (r Runner) confirmLegacyUpgradeProject(out io.Writer, detection loafRepoDetection) (bool, error) {
	fmt.Fprintf(out, "  %s %s\n", ansiYellow("⚠"), ansiGray(detection.Bases[0]))
	if !r.installPromptInteractive() {
		fmt.Fprintf(out, "  %s\n", ansiGray("Confirmation required: rerun in a terminal to answer \"Is this a Loaf project?\", or run loaf install to deploy Loaf here. Skipping project surfaces."))
		return false, nil
	}
	yes, err := askInstallYesNo(r.installPromptReader(), out, "  Legacy Loaf artifacts found. Is this a Loaf project? [y/N] ", false)
	if err != nil {
		return false, err
	}
	if !yes {
		fmt.Fprintf(out, "  %s\n", ansiGray("Skipping project surfaces. Run loaf install to deploy Loaf to this folder."))
		return false, nil
	}
	return true, nil
}

// pendingUpgradeMcpRecommendations lists the recommendations this release ships
// that the project has never answered. Upgrade refreshes the record so a newly
// shipped recommendation is visible in .agents/loaf.json; the interview that
// turns an answer into configuration belongs to onboarding.
func pendingUpgradeMcpRecommendations(projectRoot string) []string {
	return pendingMcpRecommendationsIn(readInstallLoafConfig(projectRoot))
}

func pendingMcpRecommendationsIn(config map[string]any) []string {
	integrations, _ := config["integrations"].(map[string]any)
	var pending []string
	for _, def := range installMcpDefinitions {
		if integrations == nil || integrations[def.id] == nil {
			pending = append(pending, def.id)
		}
	}
	return pending
}

// planUpgradeProjectPart reports the detector gate on the dry-run plan.
func planUpgradeProjectPart(detection loafRepoDetection) *projectPartPlan {
	return &projectPartPlan{
		InScope:              detection.Tier >= loafRepoTierStrong,
		Tier:                 detection.Tier.String(),
		ConfirmationRequired: detection.Tier == loafRepoTierLegacy,
		Bases:                detection.Bases,
	}
}

// planUpgradeMcpRecord mirrors the recommendation-record refresh read-only.
func planUpgradeMcpRecord(projectRoot string) projectFilePlanEntry {
	entry := projectFilePlanEntry{Path: ".agents/loaf.json"}
	config, err := readInstallLoafConfigDocument(projectRoot)
	if err != nil {
		// The plan may never promise a write the apply path refuses to make.
		entry.Action = "skipped"
		entry.Detail = err.Error()
		return entry
	}
	pending := pendingMcpRecommendationsIn(config)
	if len(pending) == 0 {
		entry.Action = "already-correct"
		entry.Detail = "MCP recommendations already recorded"
		return entry
	}
	entry.Action = "updated"
	entry.Detail = "Record MCP recommendations: " + strings.Join(pending, ", ")
	if !installFileExists(filepath.Join(projectRoot, ".agents", "loaf.json")) {
		entry.Action = "created"
	}
	return entry
}

func writeUpgradeHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf upgrade [options]",
		"",
		"Refresh Loaf in place. The work has two parts:",
		"",
		"  Global   Runs anywhere: syncs every installed harness config directory from",
		"           the installed distribution, applies deprecation-manifest cleanup,",
		"           and stamps each .loaf-version marker.",
		"  Project  Runs only in a Loaf repo: refreshes the managed fenced sections,",
		"           instruction symlinks and their migrations, and the MCP",
		"           recommendation record in .agents/loaf.json. Legacy-only signals",
		"           are confirmed first; outside a Loaf repo nothing is written.",
		"",
		"A harness that cannot be synced does not stop the others: the run finishes,",
		"names what failed, and exits non-zero.",
		"",
		"Options:",
		"  --to <target>  Filter the global part to one already-installed target (or \"all\")",
		"  --dry-run      Report the plan without writing anything",
		"  --json         Emit the dry-run plan as a single JSON document (requires --dry-run)",
		"  -y, --yes      Assume yes to safe project-file symlink migrations and destructive deprecation cleanup",
		"  --no-yes       Force prompt-style declines in non-interactive mode",
		"  -h, --help     Show help",
		"",
		"Onboarding a new harness or a new project is loaf install's job.",
	}, "\n"))
}
