package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// claudeCodeInstallTarget is the install target that registers the installed
// Loaf distribution with Claude Code's plugin system. Claude Code plugins ship
// content and hooks only, so this target writes nothing itself: it asks the
// `claude` CLI to add the distribution as a marketplace and install the plugin
// from it. The plugin's bin/loaf shim then runs whichever loaf is installed.
const claudeCodeInstallTarget = "claude-code"

// claudePluginCLI is the seam to `claude plugin ...`. Production execs the
// binary on PATH; tests substitute a recorder.
type claudePluginCLI interface {
	run(args ...string) (string, error)
}

type execClaudePluginCLI struct{}

func (execClaudePluginCLI) run(args ...string) (string, error) {
	cmd := exec.Command("claude", append([]string{"plugin"}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return "", fmt.Errorf("claude plugin %s: %w: %s", strings.Join(args, " "), err, detail)
		}
		return "", fmt.Errorf("claude plugin %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

func (r Runner) claudePluginCLI() claudePluginCLI {
	if r.ClaudePluginCLI != nil {
		return r.ClaudePluginCLI
	}
	return execClaudePluginCLI{}
}

// claudeMarketplaceIdentity is what the distribution declares about itself in
// .claude-plugin/marketplace.json: the marketplace name Claude Code will know
// it by and the plugin it offers.
type claudeMarketplaceIdentity struct {
	Name       string
	Path       string
	PluginName string
	PluginID   string
}

func readClaudeMarketplaceIdentity(loafRoot string) (claudeMarketplaceIdentity, error) {
	manifestPath := filepath.Join(loafRoot, ".claude-plugin", "marketplace.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return claudeMarketplaceIdentity{}, fmt.Errorf("this Loaf distribution has no Claude Code marketplace manifest at %s", manifestPath)
	}
	var manifest struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name string `json:"name"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return claudeMarketplaceIdentity{}, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	if strings.TrimSpace(manifest.Name) == "" || len(manifest.Plugins) == 0 || strings.TrimSpace(manifest.Plugins[0].Name) == "" {
		return claudeMarketplaceIdentity{}, fmt.Errorf("%s must declare a marketplace name and at least one plugin", manifestPath)
	}
	return claudeMarketplaceIdentity{
		Name:       manifest.Name,
		Path:       loafRoot,
		PluginName: manifest.Plugins[0].Name,
		PluginID:   manifest.Plugins[0].Name + "@" + manifest.Name,
	}, nil
}

type claudeMarketplaceEntry struct {
	Name            string `json:"name"`
	Source          string `json:"source"`
	Path            string `json:"path"`
	InstallLocation string `json:"installLocation"`
}

type claudeInstalledPlugin struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	InstallPath string `json:"installPath"`
}

func readClaudeMarketplaces(cli claudePluginCLI) ([]claudeMarketplaceEntry, error) {
	out, err := cli.run("marketplace", "list", "--json")
	if err != nil {
		return nil, err
	}
	var entries []claudeMarketplaceEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, fmt.Errorf("parse claude plugin marketplace list --json: %w", err)
	}
	return entries, nil
}

func readClaudeInstalledPlugins(cli claudePluginCLI) ([]claudeInstalledPlugin, error) {
	out, err := cli.run("list", "--json")
	if err != nil {
		return nil, err
	}
	var plugins []claudeInstalledPlugin
	if err := json.Unmarshal([]byte(out), &plugins); err != nil {
		return nil, fmt.Errorf("parse claude plugin list --json: %w", err)
	}
	return plugins, nil
}

// claudeCodePluginPlan is the read-only decision for the Claude Code target:
// what the marketplace and plugin actions will be, in the same vocabulary the
// dry-run plan uses for every other target.
type claudeCodePluginPlan struct {
	Identity    claudeMarketplaceIdentity
	Marketplace artifactPlanDecision
	Plugin      artifactPlanDecision
	Installed   bool
	Blocked     bool
}

// planClaudeCodePlugin reads Claude Code's marketplace and plugin state and
// decides. A marketplace of the same name that points somewhere else is a
// conflict, never something to replace: install only ever adds.
func planClaudeCodePlugin(cli claudePluginCLI, loafRoot string, upgrade bool) (claudeCodePluginPlan, error) {
	identity, err := readClaudeMarketplaceIdentity(loafRoot)
	if err != nil {
		return claudeCodePluginPlan{}, err
	}
	marketplaces, err := readClaudeMarketplaces(cli)
	if err != nil {
		return claudeCodePluginPlan{}, err
	}
	plugins, err := readClaudeInstalledPlugins(cli)
	if err != nil {
		return claudeCodePluginPlan{}, err
	}

	plan := claudeCodePluginPlan{Identity: identity}
	plan.Marketplace = artifactPlanDecision{
		ID:          "marketplace",
		Kind:        "claude-marketplace",
		Destination: identity.Name,
		Action:      planActionCreate,
		Detail:      "claude plugin marketplace add " + identity.Path,
	}
	for _, entry := range marketplaces {
		if entry.Name != identity.Name {
			continue
		}
		registered := entry.Path
		if registered == "" {
			registered = entry.InstallLocation
		}
		if entry.Source == "directory" && sameInstallPath(registered, identity.Path) {
			plan.Marketplace.Action = planActionPreserve
			plan.Marketplace.Detail = "registered at " + identity.Path
		} else {
			if registered == "" {
				registered = entry.Source
			}
			plan.Marketplace.Action = planActionConflict
			plan.Marketplace.Detail = fmt.Sprintf("marketplace %s already points at %s; remove it with `claude plugin marketplace remove %s` to let Loaf register %s", identity.Name, registered, identity.Name, identity.Path)
			plan.Blocked = true
		}
		break
	}

	plan.Plugin = artifactPlanDecision{ID: "plugin", Kind: "claude-plugin", Destination: identity.PluginID}
	installedVersion := ""
	for _, plugin := range plugins {
		if plugin.ID == identity.PluginID {
			plan.Installed = true
			installedVersion = plugin.Version
			if plugin.Scope == "user" {
				break
			}
		}
	}
	switch {
	case plan.Blocked:
		plan.Plugin.Action = planActionNone
		plan.Plugin.Detail = "blocked by the marketplace conflict"
	case !plan.Installed:
		plan.Plugin.Action = planActionCreate
		plan.Plugin.Detail = "claude plugin install " + identity.PluginID + " --scope user"
	case upgrade:
		plan.Plugin.Action = planActionUpdate
		plan.Plugin.Detail = "claude plugin marketplace update " + identity.Name + " && claude plugin update " + identity.PluginID
	default:
		plan.Plugin.Action = planActionPreserve
		plan.Plugin.Detail = "installed"
		if installedVersion != "" {
			plan.Plugin.Detail += " (v" + installedVersion + ")"
		}
	}
	return plan, nil
}

// sameInstallPath compares two directory paths as the filesystem sees them, so
// a marketplace registered through a symlinked prefix still matches.
func sameInstallPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	normalize := func(path string) string {
		cleaned := filepath.Clean(path)
		if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
			return resolved
		}
		return cleaned
	}
	return normalize(left) == normalize(right)
}

// applyClaudeCodePlugin performs the planned actions through the CLI.
func applyClaudeCodePlugin(cli claudePluginCLI, plan claudeCodePluginPlan) error {
	if plan.Blocked {
		return errors.New(plan.Marketplace.Detail)
	}
	if plan.Marketplace.Action == planActionCreate {
		if _, err := cli.run("marketplace", "add", plan.Identity.Path, "--scope", "user"); err != nil {
			return err
		}
	}
	switch plan.Plugin.Action {
	case planActionCreate:
		if _, err := cli.run("install", plan.Identity.PluginID, "--scope", "user"); err != nil {
			return err
		}
	case planActionUpdate:
		if _, err := cli.run("marketplace", "update", plan.Identity.Name); err != nil {
			return err
		}
		if _, err := cli.run("update", plan.Identity.PluginID); err != nil {
			return err
		}
	}
	return nil
}

// installClaudeCodePlugin is install's Claude Code half. It reports what it
// did in the same shape as the other targets and returns whether the plugin
// is now installed from this distribution.
func (r Runner) installClaudeCodePlugin(out io.Writer, loafRoot string) (bool, error) {
	cli := r.claudePluginCLI()
	plan, err := planClaudeCodePlugin(cli, loafRoot, false)
	if err != nil {
		fmt.Fprintf(out, "  %s %s - %v\n", ansiRed("✗"), installDisplayName(claudeCodeInstallTarget), err)
		return false, err
	}
	if err := applyClaudeCodePlugin(cli, plan); err != nil {
		fmt.Fprintf(out, "  %s %s - %v\n", ansiRed("✗"), installDisplayName(claudeCodeInstallTarget), err)
		return false, err
	}
	switch plan.Marketplace.Action {
	case planActionCreate:
		fmt.Fprintf(out, "  %s %s marketplace %s registered from %s\n", ansiGreen("✓"), installDisplayName(claudeCodeInstallTarget), plan.Identity.Name, ansiGray(plan.Identity.Path))
	case planActionPreserve:
		fmt.Fprintf(out, "  %s %s marketplace %s already registered at %s\n", ansiGray("○"), installDisplayName(claudeCodeInstallTarget), plan.Identity.Name, ansiGray(plan.Identity.Path))
	}
	switch plan.Plugin.Action {
	case planActionCreate:
		fmt.Fprintf(out, "  %s %s plugin %s installed\n", ansiGreen("✓"), installDisplayName(claudeCodeInstallTarget), plan.Identity.PluginID)
	case planActionPreserve:
		fmt.Fprintf(out, "  %s %s plugin %s already %s\n", ansiGray("○"), installDisplayName(claudeCodeInstallTarget), plan.Identity.PluginID, plan.Plugin.Detail)
	}
	writeClaudeCodeRuntimeAdvice(out)
	return true, nil
}

// upgradeClaudeCodePlugin is upgrade's Claude Code half: refresh a plugin this
// distribution installed. A plugin that came from somewhere else, or none at
// all, is left alone and named, because upgrade never onboards.
func (r Runner) upgradeClaudeCodePlugin(out io.Writer, loafRoot string) error {
	cli := r.claudePluginCLI()
	if _, err := readClaudeMarketplaceIdentity(loafRoot); err != nil {
		// A distribution that carries no marketplace manifest never installed a
		// plugin, so there is nothing for upgrade to refresh.
		fmt.Fprintf(out, "  %s %s plugin is not served by this distribution (no marketplace manifest)\n", ansiGray("○"), installDisplayName(claudeCodeInstallTarget))
		return nil
	}
	plan, err := planClaudeCodePlugin(cli, loafRoot, true)
	if err != nil {
		// Claude Code's state could not be read. Upgrade never onboards, so this
		// is advice rather than a failed target: the content sync stands.
		fmt.Fprintf(out, "  %s %s plugin state could not be read; run %s to check it (%v)\n", ansiYellow("⚠"), installDisplayName(claudeCodeInstallTarget), ansiBold("loaf install --to claude-code"), err)
		return nil
	}
	if plan.Blocked || !plan.Installed || plan.Marketplace.Action != planActionPreserve {
		fmt.Fprintf(out, "  %s %s plugin is not installed from this distribution; run %s to onboard it\n", ansiGray("○"), installDisplayName(claudeCodeInstallTarget), ansiBold("loaf install --to claude-code"))
		return nil
	}
	if err := applyClaudeCodePlugin(cli, plan); err != nil {
		fmt.Fprintf(out, "  %s %s - %v\n", ansiRed("✗"), installDisplayName(claudeCodeInstallTarget), err)
		return err
	}
	fmt.Fprintf(out, "  %s %s plugin %s refreshed from %s\n", ansiGreen("✓"), installDisplayName(claudeCodeInstallTarget), plan.Identity.PluginID, ansiGray(plan.Identity.Path))
	return nil
}

// planClaudeCodeTarget renders the Claude Code half into the dry-run plan.
// include is false when an upgrade plan has nothing to refresh: the plugin is
// absent or was installed from another marketplace source.
func (r Runner) planClaudeCodeTarget(loafRoot string, upgrade bool, hasClaudeCode bool) (targetDistributionPlan, bool) {
	entry := targetDistributionPlan{Target: claudeCodeInstallTarget, ConfigDir: loafRoot, Artifacts: []artifactPlanDecision{}}
	if !hasClaudeCode {
		entry.Note = "claude CLI not on PATH; install Claude Code first"
		return entry, !upgrade
	}
	plan, err := planClaudeCodePlugin(r.claudePluginCLI(), loafRoot, upgrade)
	if err != nil {
		entry.Note = err.Error()
		return entry, !upgrade
	}
	entry.Installed = plan.Installed
	entry.Blocked = plan.Blocked
	entry.Artifacts = []artifactPlanDecision{plan.Marketplace, plan.Plugin}
	if upgrade && (plan.Blocked || !plan.Installed || plan.Marketplace.Action != planActionPreserve) {
		return entry, false
	}
	return entry, true
}

// writeClaudeCodeRuntimeAdvice reminds the operator that the plugin's shim
// needs a loaf on PATH: the plugin carries no binary of its own.
func writeClaudeCodeRuntimeAdvice(out io.Writer) {
	if installCommandExists("loaf") {
		return
	}
	fmt.Fprintf(out, "  %s loaf is not on PATH; the plugin's hooks run the installed loaf, so add it to PATH or set LOAF_BIN\n", ansiYellow("⚠"))
}
