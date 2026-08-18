package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUpgradeArgs(t *testing.T) {
	options, err := parseUpgradeArgs([]string{"--to", "cursor", "--dry-run", "--json", "--yes"})
	if err != nil {
		t.Fatalf("parseUpgradeArgs() error = %v", err)
	}
	if options.target != "cursor" || !options.dryRun || !options.json || options.yes == nil || !*options.yes {
		t.Fatalf("options = %#v, want cursor/dryRun/json/yes", options)
	}
	if _, err := parseUpgradeArgs([]string{"--json"}); err == nil || !strings.Contains(err.Error(), "requires --dry-run") {
		t.Fatalf("parseUpgradeArgs(--json) error = %v, want requires --dry-run", err)
	}
	if _, err := parseUpgradeArgs([]string{"--to"}); err == nil || !strings.Contains(err.Error(), "--to requires a value") {
		t.Fatalf("parseUpgradeArgs(--to) error = %v, want a missing-value error", err)
	}
	if _, err := parseUpgradeArgs([]string{"--upgrade"}); err == nil || !strings.Contains(err.Error(), "unknown upgrade option") {
		t.Fatalf("parseUpgradeArgs(--upgrade) error = %v, want unknown option", err)
	}
	declined, err := parseUpgradeArgs([]string{"--no-yes"})
	if err != nil || declined.yes == nil || *declined.yes {
		t.Fatalf("parseUpgradeArgs(--no-yes) = %#v, %v, want an explicit no", declined, err)
	}
}

func TestRunnerUpgradeHelpNamesBothParts(t *testing.T) {
	root, _ := setupUpgradeFixture(t)
	output := runInstallCapture(t, root, "upgrade", "--help")
	for _, want := range []string{"loaf upgrade", "Global", "Project", "--to <target>", "--dry-run"} {
		if !strings.Contains(output, want) {
			t.Fatalf("upgrade --help = %q, want it to contain %q", output, want)
		}
	}
}

// TestRunnerUpgradeOutsideALoafProjectWritesNoProjectFiles is the matrix row
// that motivated the whole split: the global part runs anywhere, and a
// directory that is not a Loaf repo must come out untouched.
func TestRunnerUpgradeOutsideALoafProjectWritesNoProjectFiles(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")

	output := runInstallCapture(t, root, "upgrade", "--yes")

	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Foundations v2\n")
	if !strings.Contains(output, "No Loaf project here") {
		t.Fatalf("upgrade output = %q, want the skipped-project note", output)
	}
	assertNoUpgradeProjectFiles(t, root)
}

func TestRunnerUpgradeInsideALoafProjectRefreshesProjectSurfaces(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")

	output := runInstallCapture(t, root, "upgrade", "--yes")

	if !strings.Contains(output, "Loaf project detected") || !strings.Contains(output, ".agents/loaf.json") {
		t.Fatalf("upgrade output = %q, want the detection basis printed", output)
	}
	body := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	if !strings.Contains(body, "<!-- loaf:managed:start sha256=") {
		t.Fatalf("AGENTS.md = %q, want the managed fenced section refreshed", body)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations, ok := config["integrations"].(map[string]any)
	if !ok || integrations["serena"] == nil || integrations["linear"] != nil {
		t.Fatalf("integrations = %#v, want only the shipped Serena recommendation recorded", config["integrations"])
	}
}

// TestRunnerUpgradeMcpRecordRefreshStaysBehindTheDetectorGate pins Decision 7:
// the .agents/loaf.json write is a project write like any other.
func TestRunnerUpgradeMcpRecordRefreshStaysBehindTheDetectorGate(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")

	runInstallCapture(t, root, "upgrade", "--yes")

	assertInstallPathMissing(t, filepath.Join(root, ".agents", "loaf.json"))
}

func TestRunnerUpgradeLegacyProjectPromptRoutes(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		root, home := setupUpgradeFixture(t)
		installUpgradeFixtureTarget(t, root, home, "cursor")
		mkdirAll(t, filepath.Join(root, ".agents", "specs"))

		output := runUpgradeWithStdin(t, root, "y\n", "upgrade", "--yes")

		if !strings.Contains(output, "Is this a Loaf project?") {
			t.Fatalf("upgrade output = %q, want the legacy confirmation prompt", output)
		}
		if !installFileExists(filepath.Join(root, "AGENTS.md")) {
			t.Fatalf("AGENTS.md missing; a confirmed legacy project takes the full project path\n%s", output)
		}
	})

	t.Run("declined", func(t *testing.T) {
		root, home := setupUpgradeFixture(t)
		installUpgradeFixtureTarget(t, root, home, "cursor")
		mkdirAll(t, filepath.Join(root, ".agents", "specs"))

		output := runUpgradeWithStdin(t, root, "n\n", "upgrade", "--yes")

		if !strings.Contains(output, "loaf install") {
			t.Fatalf("upgrade output = %q, want the install offer after declining", output)
		}
		assertInstallPathMissing(t, filepath.Join(root, "AGENTS.md"))
		assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	})

	t.Run("non-interactive reports the required confirmation", func(t *testing.T) {
		root, home := setupUpgradeFixture(t)
		installUpgradeFixtureTarget(t, root, home, "cursor")
		mkdirAll(t, filepath.Join(root, ".agents", "specs"))
		withoutTerminalStdin(t)

		output := runInstallCapture(t, root, "upgrade", "--yes")

		if !strings.Contains(output, "Confirmation required") {
			t.Fatalf("upgrade output = %q, want the required-confirmation report", output)
		}
		if strings.Contains(output, "[y/N]") {
			t.Fatalf("upgrade output = %q, want no prompt without a terminal", output)
		}
		assertInstallPathMissing(t, filepath.Join(root, "AGENTS.md"))
		assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	})
}

// TestRunnerUpgradePreservesAnUnparseableProjectConfig covers the file upgrade
// must never rewrite from scratch. Whatever `.agents/loaf.json` holds, if it is
// not a JSON object then Loaf cannot merge into it, and replacing it with
// defaults would destroy state somebody else owns. The run reports what it left
// alone and finishes normally.
func TestRunnerUpgradePreservesAnUnparseableProjectConfig(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "truncated object", body: "{\"integrations\": {\"linear\": {\"enabled\""},
		{name: "json array", body: "[{\"integrations\": {}}]\n"},
		{name: "not json at all", body: "# hand-written notes, not configuration\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root, home := setupUpgradeFixture(t)
			installUpgradeFixtureTarget(t, root, home, "cursor")
			configPath := filepath.Join(root, ".agents", "loaf.json")
			writeInstallFile(t, configPath, testCase.body)

			output := runInstallCapture(t, root, "upgrade", "--yes")

			if got := string(readFileBytes(t, configPath)); got != testCase.body {
				t.Fatalf("loaf.json = %q, want it preserved byte-for-byte as %q", got, testCase.body)
			}
			if !strings.Contains(output, "does not parse as a JSON object") || !strings.Contains(output, filepath.Join(".agents", "loaf.json")) {
				t.Fatalf("upgrade output = %q, want the parse failure and the path reported", output)
			}
			// The rest of the project part still ran.
			if !strings.Contains(string(readFileBytes(t, filepath.Join(root, "AGENTS.md"))), "<!-- loaf:managed:start sha256=") {
				t.Fatalf("AGENTS.md missing its managed section; the fenced write must still happen")
			}
		})
	}

	t.Run("a valid object is still refreshed", func(t *testing.T) {
		root, home := setupUpgradeFixture(t)
		installUpgradeFixtureTarget(t, root, home, "cursor")
		writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")

		output := runInstallCapture(t, root, "upgrade", "--yes")

		if strings.Contains(output, "does not parse") {
			t.Fatalf("upgrade output = %q, want no preservation notice for a well-formed object", output)
		}
		config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
		integrations, ok := config["integrations"].(map[string]any)
		if !ok || integrations["serena"] == nil || integrations["linear"] != nil {
			t.Fatalf("integrations = %#v, want only the Serena recommendation recorded", config["integrations"])
		}
	})
}

// TestRunnerUpgradeStopsTheProjectPartAfterAFenceError pins the abort: a
// managed section upgrade refuses to overwrite means this project is not
// currently Loaf's to refresh, so nothing after that write happens and the run
// carries the failure out as an exit code.
func TestRunnerUpgradeStopsTheProjectPartAfterAFenceError(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	tampered := tamperedFencedAgentsBody()
	agentsPath := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, agentsPath, tampered)

	output := runUpgradeExpectingExitError(t, root, "upgrade", "--yes")

	if !strings.Contains(output, "refusing to overwrite") {
		t.Fatalf("upgrade output = %q, want the fenced-section refusal reported", output)
	}
	if !strings.Contains(output, "skipping the remaining project surfaces") {
		t.Fatalf("upgrade output = %q, want the abort reported", output)
	}
	if !strings.Contains(output, "project surfaces incomplete") {
		t.Fatalf("upgrade output = %q, want the failure summary to name the project part", output)
	}
	assertInstallPathMissing(t, filepath.Join(root, ".agents", "loaf.json"))
	if got := string(readFileBytes(t, agentsPath)); got != tampered {
		t.Fatalf("AGENTS.md = %q, want it untouched after the refusal", got)
	}
}

// TestRunnerUpgradeExitsNonZeroAfterATargetFailure is the scripting contract: a
// partial upgrade finishes its remaining work, prints what failed, and does not
// report success.
func TestRunnerUpgradeExitsNonZeroAfterATargetFailure(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	installUpgradeFixtureTarget(t, root, home, "opencode")
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")
	// OpenCode still looks installed, but there is nothing to sync from.
	if err := os.RemoveAll(filepath.Join(root, "dist", "opencode")); err != nil {
		t.Fatalf("RemoveAll(dist/opencode) error = %v", err)
	}

	output := runUpgradeExpectingExitError(t, root, "upgrade", "--yes")

	if !strings.Contains(output, "Cursor refreshed") {
		t.Fatalf("upgrade output = %q, want the healthy target synced anyway", output)
	}
	if !strings.Contains(output, "Upgrade finished with errors: harness content not synced for OpenCode") {
		t.Fatalf("upgrade output = %q, want the failure summary naming OpenCode", output)
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	// The project part runs after the targets, so it must have completed too.
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	if integrations, ok := config["integrations"].(map[string]any); !ok || integrations["serena"] == nil || integrations["linear"] != nil {
		t.Fatalf("integrations = %#v, want the project part to record only Serena despite the target failure", config["integrations"])
	}
}

func TestRunnerUpgradeToFiltersInstalledTargetsOnly(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	installUpgradeFixtureTarget(t, root, home, "opencode")
	writeInstallFile(t, filepath.Join(home, ".config", "opencode", loafInstallMarkerFile), "old\n")

	output := runInstallCapture(t, root, "upgrade", "--to", "cursor", "--yes")

	if !strings.Contains(output, "Cursor refreshed") || strings.Contains(output, "OpenCode refreshed") {
		t.Fatalf("upgrade output = %q, want only cursor upgraded", output)
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".config", "opencode", loafInstallMarkerFile), "old\n")
}

// TestRunnerUpgradeToNarrowsTheGlobalSyncOnly pins Decision 8's scope: `--to`
// filters the harness content sync, and the project surfaces — which describe
// every harness this repo is set up for — are refreshed whole. Narrowing them
// with the sync would silently retire the unnamed harnesses' fenced sections.
func TestRunnerUpgradeToNarrowsTheGlobalSyncOnly(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	installUpgradeFixtureTarget(t, root, home, "opencode")
	writeInstallFile(t, filepath.Join(home, ".config", "opencode", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")

	output := runInstallCapture(t, root, "upgrade", "--to", "cursor", "--yes")

	if !strings.Contains(output, "Cursor refreshed") || strings.Contains(output, "OpenCode refreshed") {
		t.Fatalf("upgrade output = %q, want only cursor's content synced", output)
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".config", "opencode", loafInstallMarkerFile), "old\n")

	if !strings.Contains(output, "Loaf project detected") {
		t.Fatalf("upgrade output = %q, want the project part to run", output)
	}
	for _, name := range []string{"Cursor", "OpenCode"} {
		if !hasUpgradeProjectFileLine(output, name) {
			t.Fatalf("upgrade output = %q, want a project-file line for %s", output, name)
		}
	}
	body := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	if !strings.Contains(body, "<!-- loaf:managed:start sha256=") {
		t.Fatalf("AGENTS.md = %q, want the managed fenced section refreshed", body)
	}
}

func TestRunnerUpgradeToUninstalledTargetPointsAtInstall(t *testing.T) {
	root, _ := setupUpgradeFixture(t)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--to", "cursor"})
	if err == nil || !strings.Contains(err.Error(), "loaf install --to cursor") {
		t.Fatalf("upgrade --to cursor error = %v, want a pointer at loaf install --to cursor", err)
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--to", "nonesuch"})
	if err == nil || !strings.Contains(err.Error(), "unknown upgrade target") {
		t.Fatalf("upgrade --to nonesuch error = %v, want an unknown-target error", err)
	}
}

func TestRunnerUpgradeReportsDeprecationConsentWithoutAssumingIt(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	retired := filepath.Join(home, ".retired-tool")
	writeInstallFile(t, filepath.Join(retired, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(retired, "skills", "stale", "SKILL.md"), "stale\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "retired-tool",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "retired by test manifest",
      "paths": ["${HOME}/.retired-tool"]
    }
  ],
  "retired_skills": [],
  "relocations": [],
  "aliases": []
}`)

	output := runInstallCapture(t, root, "upgrade")

	if !strings.Contains(output, "rerun with --yes") {
		t.Fatalf("upgrade output = %q, want the destructive-cleanup consent requirement reported", output)
	}
	if _, err := os.Stat(retired); err != nil {
		t.Fatalf("retired path stat = %v, want it preserved without explicit consent", err)
	}
}

func TestRunnerUpgradeDryRunIsStableAndNonMutating(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")

	first := runInstallCapture(t, root, "upgrade", "--dry-run", "--json")
	plan := assertDryRunNonMutating(t, root, home, "upgrade", "--dry-run", "--json")
	if second := runInstallCapture(t, root, "upgrade", "--dry-run", "--json"); first != second {
		t.Fatalf("dry-run JSON is not byte-stable:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if plan.Command != upgradeCommandName || plan.ContractVersion != installPlanContractVersion || !plan.DryRun {
		t.Fatalf("plan envelope = %#v, want the upgrade command at contract %d", plan, installPlanContractVersion)
	}
	if plan.ProjectPart == nil || !plan.ProjectPart.InScope || plan.ProjectPart.Tier != loafRepoTierStrong.String() {
		t.Fatalf("project_part = %#v, want an in-scope strong tier", plan.ProjectPart)
	}
	if !hasUpgradePlanPath(plan, ".agents/loaf.json") {
		t.Fatalf("project_files = %#v, want the MCP recommendation record planned", plan.ProjectFiles)
	}
}

// TestRunnerUpgradeDryRunNeverEmitsTheRemovedFlag is V6's in-process twin.
func TestRunnerUpgradeDryRunNeverEmitsTheRemovedFlag(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")

	for _, args := range [][]string{
		{"upgrade", "--dry-run", "--json"},
		{"upgrade", "--dry-run"},
		{"upgrade", "--to", "cursor", "--dry-run", "--json"},
	} {
		output := runInstallCapture(t, root, args...)
		if strings.Contains(output, "install --upgrade") {
			t.Fatalf("%v output names the removed flag:\n%s", args, output)
		}
	}

	plan := parseInstallPlanJSON(t, runInstallCapture(t, root, "upgrade", "--dry-run", "--json"))
	for _, command := range plan.FollowUpCommands {
		if !strings.HasPrefix(command, "loaf upgrade") && command != "loaf build" {
			t.Fatalf("follow_up_commands = %#v, want every apply command to name loaf upgrade", plan.FollowUpCommands)
		}
	}
}

// TestRunnerUpgradeDryRunOutsideALoafProjectPlansNoProjectFiles keeps the plan
// honest: it may never promise writes the apply path refuses to make.
func TestRunnerUpgradeDryRunOutsideALoafProjectPlansNoProjectFiles(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")

	plan := assertDryRunNonMutating(t, root, home, "upgrade", "--dry-run", "--json")

	if plan.ProjectPart == nil || plan.ProjectPart.InScope || plan.ProjectPart.Tier != loafRepoTierNone.String() {
		t.Fatalf("project_part = %#v, want an out-of-scope none tier", plan.ProjectPart)
	}
	if len(plan.ProjectFiles) != 0 {
		t.Fatalf("project_files = %#v, want none outside a Loaf project", plan.ProjectFiles)
	}
	human := runInstallCapture(t, root, "upgrade", "--dry-run")
	if !strings.Contains(human, "no Loaf project detected here") {
		t.Fatalf("dry-run output = %q, want the skipped project-files reason", human)
	}
}

func TestRunnerUpgradeDryRunLegacyProjectReportsConfirmation(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	mkdirAll(t, filepath.Join(root, ".agents", "drafts"))

	plan := assertDryRunNonMutating(t, root, home, "upgrade", "--dry-run", "--json")

	if plan.ProjectPart == nil || plan.ProjectPart.InScope || !plan.ProjectPart.ConfirmationRequired {
		t.Fatalf("project_part = %#v, want confirmation required and nothing in scope", plan.ProjectPart)
	}
	if len(plan.ProjectFiles) != 0 {
		t.Fatalf("project_files = %#v, want none until the legacy project is confirmed", plan.ProjectFiles)
	}
}

// --- helpers -------------------------------------------------------------

// setupUpgradeFixture builds the install fixture with an isolated, nonexistent
// state database so the detector can never reach the real global DB.
func setupUpgradeFixture(t *testing.T) (string, string) {
	t.Helper()
	root, home := setupInstallCommandFixture(t)
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	return root, home
}

// installUpgradeFixtureTarget makes a target look installed and leaves the
// distribution one revision ahead, so the upgrade has real work to do.
func installUpgradeFixtureTarget(t *testing.T, root string, home string, target string) {
	t.Helper()
	writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, target)
	runInstallFixture(t, root, "install", "--to", target, "--yes")
	writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), "# Foundations v2\n")
	// install scaffolds project files; the upgrade matrix judges what upgrade
	// itself writes, so reset the project surfaces it left behind.
	for _, path := range []string{"AGENTS.md", ".agents", ".claude"} {
		if err := os.RemoveAll(filepath.Join(root, path)); err != nil {
			t.Fatalf("RemoveAll(%s) error = %v", path, err)
		}
	}
}

// withoutTerminalStdin replaces os.Stdin with a closed pipe for the duration of
// the test. `go test` hands the binary /dev/null, which is a character device
// and therefore reads as a terminal; a pipe is what real automation looks like.
func withoutTerminalStdin(t *testing.T) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	original := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = original
		reader.Close()
	})
}

// runUpgradeExpectingExitError runs a command that must fail through the CLI's
// exit convention and returns everything it printed on the way.
func runUpgradeExpectingExitError(t *testing.T, root string, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run(args)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("%v error = %v, want a non-zero ExitError\n%s", args, err, stdout.String())
	}
	return stdout.String()
}

// tamperedFencedAgentsBody is a complete managed section whose recorded
// fingerprint does not describe its body — the shape install and upgrade refuse
// to overwrite.
func tamperedFencedAgentsBody() string {
	_, body, _ := strings.Cut(generateFencedContent(), "\n")
	return "# Project\n\n" + fencedStartMarker + " sha256=" + strings.Repeat("b", 64) + " -->\n" + body + "\n"
}

func runUpgradeWithStdin(t *testing.T, root string, stdin string, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(stdin),
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run(args)
	if err != nil {
		t.Fatalf("%v error = %v\n%s", args, err, stdout.String())
	}
	return stdout.String()
}

func assertNoUpgradeProjectFiles(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{"AGENTS.md", ".agents", ".claude"} {
		assertInstallPathMissing(t, filepath.Join(root, path))
	}
}

// hasUpgradeProjectFileLine finds one harness's managed-project-file line in
// upgrade output, whichever of the create/append/update/skip verbs it took.
func hasUpgradeProjectFileLine(output string, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, name) && strings.Contains(line, "Loaf framework section") {
			return true
		}
	}
	return false
}

func hasUpgradePlanPath(plan installDryRunPlan, path string) bool {
	for _, entry := range plan.ProjectFiles {
		if entry.Path == path {
			return true
		}
	}
	return false
}
