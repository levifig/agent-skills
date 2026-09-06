package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaudePluginCLI stands in for `claude plugin ...`: it serves the state a
// test seeds, records every mutating call, and mutates its own state the way
// Claude Code would, so a second plan sees the first apply.
type fakeClaudePluginCLI struct {
	marketplaces []claudeMarketplaceEntry
	plugins      []claudeInstalledPlugin
	calls        [][]string
	failOn       string
}

func (f *fakeClaudePluginCLI) run(args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if f.failOn != "" && strings.HasPrefix(joined, f.failOn) {
		return "", errors.New("claude plugin " + joined + ": simulated failure")
	}
	switch {
	case joined == "marketplace list --json":
		body, _ := json.Marshal(f.marketplaces)
		return string(body), nil
	case joined == "list --json":
		body, _ := json.Marshal(f.plugins)
		return string(body), nil
	}
	f.calls = append(f.calls, args)
	switch {
	case len(args) >= 3 && args[0] == "marketplace" && args[1] == "add":
		f.marketplaces = append(f.marketplaces, claudeMarketplaceEntry{Name: "levifig-loaf", Source: "directory", Path: args[2], InstallLocation: args[2]})
	case len(args) >= 2 && args[0] == "install":
		f.plugins = append(f.plugins, claudeInstalledPlugin{ID: args[1], Version: "9.8.7-test.1", Scope: "user", Enabled: true})
	}
	return "", nil
}

func (f *fakeClaudePluginCLI) mutations() []string {
	var lines []string
	for _, call := range f.calls {
		lines = append(lines, strings.Join(call, " "))
	}
	return lines
}

func writeClaudeMarketplaceManifest(t *testing.T, root string) {
	t.Helper()
	writeInstallFile(t, filepath.Join(root, ".claude-plugin", "marketplace.json"), `{"name":"levifig-loaf","plugins":[{"name":"loaf","source":"./plugins/loaf"}]}`+"\n")
}

func TestPlanClaudeCodePluginRegistersAndInstallsOnAFreshMachine(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeClaudeMarketplaceManifest(t, root)
	cli := &fakeClaudePluginCLI{}

	plan, err := planClaudeCodePlugin(cli, root, false)
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	if plan.Identity.PluginID != "loaf@levifig-loaf" || plan.Identity.Path != root {
		t.Fatalf("identity = %#v, want loaf@levifig-loaf at %s", plan.Identity, root)
	}
	if plan.Marketplace.Action != planActionCreate || plan.Plugin.Action != planActionCreate || plan.Blocked || plan.Installed {
		t.Fatalf("plan = %#v, want create/create", plan)
	}
	if err := applyClaudeCodePlugin(cli, plan); err != nil {
		t.Fatalf("apply error = %v", err)
	}
	want := []string{"marketplace add " + root + " --scope user", "install loaf@levifig-loaf --scope user"}
	if got := cli.mutations(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("claude calls = %q, want %q", got, want)
	}

	// The same machine a moment later: nothing to add, nothing to install.
	again, err := planClaudeCodePlugin(cli, root, false)
	if err != nil {
		t.Fatalf("second plan error = %v", err)
	}
	if again.Marketplace.Action != planActionPreserve || again.Plugin.Action != planActionPreserve || !again.Installed {
		t.Fatalf("second plan = %#v, want preserve/preserve installed", again)
	}
	if err := applyClaudeCodePlugin(cli, again); err != nil || len(cli.calls) != 2 {
		t.Fatalf("second apply err=%v calls=%q, want no further mutations", err, cli.mutations())
	}
}

func TestPlanClaudeCodePluginMatchesAMarketplaceRegisteredThroughASymlink(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeClaudeMarketplaceManifest(t, root)
	link := filepath.Join(realpath(t, t.TempDir()), "loaf-current")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	cli := &fakeClaudePluginCLI{
		marketplaces: []claudeMarketplaceEntry{{Name: "levifig-loaf", Source: "directory", Path: link, InstallLocation: link}},
		plugins:      []claudeInstalledPlugin{{ID: "loaf@levifig-loaf", Version: "0.5.0", Scope: "user", Enabled: true}},
	}

	plan, err := planClaudeCodePlugin(cli, root, true)
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	if plan.Marketplace.Action != planActionPreserve || plan.Plugin.Action != planActionUpdate {
		t.Fatalf("upgrade plan = %#v, want preserve/update", plan)
	}
	if err := applyClaudeCodePlugin(cli, plan); err != nil {
		t.Fatalf("apply error = %v", err)
	}
	want := []string{"marketplace update levifig-loaf", "update loaf@levifig-loaf"}
	if got := cli.mutations(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("claude calls = %q, want %q", got, want)
	}
}

func TestPlanClaudeCodePluginNeverReplacesAForeignMarketplace(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeClaudeMarketplaceManifest(t, root)
	cli := &fakeClaudePluginCLI{
		marketplaces: []claudeMarketplaceEntry{{Name: "levifig-loaf", Source: "github", Path: "", InstallLocation: "/elsewhere/levifig-loaf"}},
	}

	plan, err := planClaudeCodePlugin(cli, root, false)
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	if !plan.Blocked || plan.Marketplace.Action != planActionConflict || plan.Plugin.Action != planActionNone {
		t.Fatalf("plan = %#v, want a blocked conflict", plan)
	}
	if !strings.Contains(plan.Marketplace.Detail, "claude plugin marketplace remove levifig-loaf") {
		t.Fatalf("conflict detail = %q, want the remove command", plan.Marketplace.Detail)
	}
	if err := applyClaudeCodePlugin(cli, plan); err == nil || len(cli.calls) != 0 {
		t.Fatalf("apply err=%v calls=%q, want refusal without mutations", err, cli.mutations())
	}
}

func TestPlanClaudeCodePluginRequiresTheDistributionManifest(t *testing.T) {
	root := realpath(t, t.TempDir())
	if _, err := planClaudeCodePlugin(&fakeClaudePluginCLI{}, root, false); err == nil || !strings.Contains(err.Error(), "marketplace manifest") {
		t.Fatalf("plan error = %v, want a missing-manifest refusal", err)
	}
}

// installClaudeCodeFixture seeds a distribution root whose Claude Code CLI is a
// stub on PATH (so detection sees it) while the Runner talks to the fake.
func installClaudeCodeFixture(t *testing.T) (string, *fakeClaudePluginCLI) {
	t.Helper()
	root, _ := setupInstallCommandFixture(t)
	writeClaudeMarketplaceManifest(t, root)
	writeInstallFile(t, filepath.Join(root, "bin", "claude"), "#!/bin/sh\nexit 1\n")
	if err := os.Chmod(filepath.Join(root, "bin", "claude"), 0o755); err != nil {
		t.Fatalf("Chmod(claude stub) error = %v", err)
	}
	return root, &fakeClaudePluginCLI{}
}

func TestRunnerInstallToClaudeCodeRegistersTheDistributionAndInstallsThePlugin(t *testing.T) {
	root, cli := installClaudeCodeFixture(t)
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root), ClaudePluginCLI: cli}.Run([]string{"install", "--to", claudeCodeInstallTarget, "--yes"})
	if err != nil {
		t.Fatalf("install --to claude-code error = %v\n%s", err, stdout.String())
	}
	want := []string{"marketplace add " + root + " --scope user", "install loaf@levifig-loaf --scope user"}
	if got := cli.mutations(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("claude calls = %q, want %q", got, want)
	}
	for _, expect := range []string{"marketplace levifig-loaf registered", "plugin loaf@levifig-loaf installed", "Plugin via:"} {
		if !strings.Contains(stdout.String(), expect) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), expect)
		}
	}
	if strings.Contains(stdout.String(), "/plugin marketplace add") {
		t.Fatalf("stdout = %q, must not advertise the manual marketplace path", stdout.String())
	}
}

func TestRunnerInstallToClaudeCodeRefusesWithoutTheClaudeCLI(t *testing.T) {
	root, _ := setupInstallCommandFixture(t)
	writeClaudeMarketplaceManifest(t, root)
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root), ClaudePluginCLI: &fakeClaudePluginCLI{}}.Run([]string{"install", "--to", claudeCodeInstallTarget, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("install without claude error = %v, want a PATH refusal\n%s", err, stdout.String())
	}
}

func TestRunnerUpgradeRefreshesAPluginThisDistributionInstalled(t *testing.T) {
	root, cli := installClaudeCodeFixture(t)
	cli.marketplaces = []claudeMarketplaceEntry{{Name: "levifig-loaf", Source: "directory", Path: root, InstallLocation: root}}
	cli.plugins = []claudeInstalledPlugin{{ID: "loaf@levifig-loaf", Version: "0.5.0", Scope: "user", Enabled: true}}
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root), ClaudePluginCLI: cli}.Run([]string{"upgrade"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	want := []string{"marketplace update levifig-loaf", "update loaf@levifig-loaf"}
	if got := cli.mutations(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("claude calls = %q, want %q", got, want)
	}
	if !strings.Contains(stdout.String(), "plugin loaf@levifig-loaf refreshed") {
		t.Fatalf("stdout = %q, want the refresh line", stdout.String())
	}
}

func TestRunnerUpgradeLeavesAPluginFromAnotherSourceAlone(t *testing.T) {
	root, cli := installClaudeCodeFixture(t)
	cli.marketplaces = []claudeMarketplaceEntry{{Name: "levifig-loaf", Source: "github", InstallLocation: "/elsewhere"}}
	cli.plugins = []claudeInstalledPlugin{{ID: "loaf@levifig-loaf", Version: "0.5.0", Scope: "user", Enabled: true}}
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root), ClaudePluginCLI: cli}.Run([]string{"upgrade"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	if len(cli.calls) != 0 {
		t.Fatalf("claude calls = %q, want none for a foreign marketplace", cli.mutations())
	}
	if !strings.Contains(stdout.String(), "not installed from this distribution") {
		t.Fatalf("stdout = %q, want the onboarding hint", stdout.String())
	}
}

func TestPlanClaudeCodeTargetRendersIntoTheDryRunPlan(t *testing.T) {
	root, cli := installClaudeCodeFixture(t)
	runner := Runner{ClaudePluginCLI: cli}

	entry, include := runner.planClaudeCodeTarget(root, false, true)
	if !include || entry.Target != claudeCodeInstallTarget || len(entry.Artifacts) != 2 || entry.Artifacts[0].Action != planActionCreate {
		t.Fatalf("install plan entry = %#v include=%t, want two create artifacts", entry, include)
	}
	if _, include := runner.planClaudeCodeTarget(root, true, true); include {
		t.Fatal("upgrade plan included a plugin this distribution never installed")
	}
	entry, include = runner.planClaudeCodeTarget(root, false, false)
	if !include || !strings.Contains(entry.Note, "not on PATH") {
		t.Fatalf("plan entry without claude = %#v include=%t, want a PATH note", entry, include)
	}
	if len(cli.calls) != 0 {
		t.Fatalf("planning mutated Claude Code: %q", cli.mutations())
	}
}
