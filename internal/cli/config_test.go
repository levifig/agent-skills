package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestRunnerConfigCheckFixCreatesProjectConfig(t *testing.T) {
	root, _ := setupInstallCommandFixture(t)

	var checkOut bytes.Buffer
	err := Runner{Stdout: &checkOut, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"config", "check", "--json"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("config check error = %v, want exit code 2", err)
	}
	var before configCheckResult
	if err := json.Unmarshal(checkOut.Bytes(), &before); err != nil {
		t.Fatalf("Unmarshal(check output) error = %v\n%s", err, checkOut.String())
	}
	if before.OK || before.Config.Status != "missing" {
		t.Fatalf("before = %#v, want missing config failure", before)
	}

	var fixOut bytes.Buffer
	err = Runner{Stdout: &fixOut, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"config", "check", "--fix", "--json"})
	if err != nil {
		t.Fatalf("config check --fix error = %v\n%s", err, fixOut.String())
	}
	var after configCheckResult
	if err := json.Unmarshal(fixOut.Bytes(), &after); err != nil {
		t.Fatalf("Unmarshal(fix output) error = %v\n%s", err, fixOut.String())
	}
	if !after.OK || !after.Fixed || after.Config.Status != "created" {
		t.Fatalf("after = %#v, want created valid config", after)
	}

	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	if config["version"] != loafConfigSchemaVersion || strings.TrimSpace(config["initialized"].(string)) == "" {
		t.Fatalf("config = %#v, want schema version and initialized timestamp", config)
	}
	knowledge := config["knowledge"].(map[string]any)
	if strings.Join(jsonStrings(t, knowledge["local"]), ",") != "docs/knowledge,docs/decisions" {
		t.Fatalf("knowledge = %#v, want default local dirs", knowledge)
	}
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want safe disabled defaults", integrations)
	}
	issue := config["issue"].(map[string]any)
	if issue["authority"] != "local" || strings.TrimSpace(fmt.Sprint(issue["prefix"])) == "" {
		t.Fatalf("issue = %#v, want local bootstrap prefix", issue)
	}
}

func TestConfigCheckValidatesOptionalLinearMCPServerName(t *testing.T) {
	now := time.Date(2026, time.August, 18, 0, 0, 0, 0, time.UTC)
	body, err := json.Marshal(defaultLoafConfig(now, t.TempDir()))
	if err != nil {
		t.Fatalf("Marshal(default config) error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("Unmarshal(default config) error = %v", err)
	}
	linear := config["integrations"].(map[string]any)["linear"].(map[string]any)
	linear["mcp_server_name"] = "linear-work"
	_, _, validationErrors := ensureLoafConfigDefaults(config, now)
	if len(validationErrors) != 0 {
		t.Fatalf("valid MCP server name errors = %#v, want none", validationErrors)
	}

	linear["mcp_server_name"] = "  "
	_, _, validationErrors = ensureLoafConfigDefaults(config, now)
	if !strings.Contains(strings.Join(validationErrors, "\n"), "integrations.linear.mcp_server_name must be a nonempty string") {
		t.Fatalf("blank MCP server name errors = %#v, want nonempty-string error", validationErrors)
	}
}

// A Codex file carrying only the operator's own group: the hook this version
// ships is enabled and not there, which is a reconcile rather than a refusal,
// and `--fix` adds Loaf's entry beside theirs without touching it.
func TestRunnerConfigCheckFixAcceptsCurrentCodexHooksSchema(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), strings.Join([]string{
		`{`,
		`  "version": "1.0.0",`,
		`  "initialized": "2026-07-06T00:00:00Z",`,
		`  "knowledge": {`,
		`    "local": ["docs/knowledge", "docs/decisions"],`,
		`    "staleness_threshold_days": 30,`,
		`    "imports": []`,
		`  },`,
		`  "integrations": {`,
		`    "linear": {"enabled": false},`,
		`    "serena": {"enabled": false},`,
		`    "github": {"account": "levifig"}`,
		`  }`,
		`}`,
	}, "\n")+"\n")
	installTestHookDistribution(t, root, "codex")
	writeInstallFile(t, filepath.Join(home, ".codex", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(home, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"user codex hook"}]}]}}`+"\n")

	before := runConfigCheckJSON(t, root, false)
	codexBefore := findConfigTargetStatus(before.Targets, "codex")
	if codexBefore.Status != "stale" || len(codexBefore.Hooks) != 1 || codexBefore.Hooks[0].State != hookStateEnabledMissing {
		t.Fatalf("codexBefore = %#v, want the shipped hook diagnosed as enabled and missing", codexBefore)
	}

	after := runConfigCheckJSON(t, root, true)
	codexAfter := findConfigTargetStatus(after.Targets, "codex")
	if !after.OK || codexAfter.Status != "updated" {
		t.Fatalf("after = %#v codexAfter = %#v, want the Codex hooks converged", after, codexAfter)
	}
	body, err := os.ReadFile(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.json) error = %v", err)
	}
	if !strings.Contains(string(body), "user codex hook") {
		t.Fatalf("hooks.json = %s, want the operator's group preserved", body)
	}
	if !strings.Contains(string(body), codexJournalHookCommandSuffix) {
		t.Fatalf("hooks.json = %s, want Loaf's session-start entry projected", body)
	}
}

func TestFixConfigTargetHooksRetainsProjectRootFromNestedWorkingDirectory(t *testing.T) {
	root := realpath(t, t.TempDir())
	projectRoot := filepath.Join(root, "project")
	nestedWorkingDir := filepath.Join(projectRoot, "nested", "worktree")
	loafRoot := filepath.Join(root, "loaf")
	configDir := filepath.Join(root, "codex-home")
	t.Setenv("CODEX_HOME", configDir)
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	installTestHookDistribution(t, loafRoot, "codex")
	writeInstallFile(t, filepath.Join(configDir, "hooks.json"), `{"hooks":{}}`+"\n")
	if err := os.MkdirAll(nestedWorkingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested working directory) error = %v", err)
	}
	if output, err := exec.Command("git", "init", "-q", projectRoot).CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v\n%s", err, output)
	}
	resolvedRoot, err := project.ResolveRoot(nestedWorkingDir)
	if err != nil {
		t.Fatalf("ResolveRoot(nested working directory) error = %v", err)
	}
	if resolvedRoot.Path() != projectRoot {
		t.Fatalf("ResolveRoot(nested working directory) = %q, want %q", resolvedRoot.Path(), projectRoot)
	}

	target := detectedInstallTool{key: "codex", configDir: configDir}
	var captured targetInstallOptions
	hookState, releaseHookState := (Runner{}).hookStateForApply(resolvedRoot.Path())
	defer releaseHookState()
	status := fixConfigTargetHooksWithInstaller(resolvedRoot.Path(), loafRoot, target, configTargetStatus{Status: "stale"}, hookState, func(options targetInstallOptions) error {
		captured = options
		return nil
	})
	if status.Error != "" {
		t.Fatalf("fixConfigTargetHooks error = %q", status.Error)
	}
	if captured.ProjectRoot != projectRoot {
		t.Fatalf("captured ProjectRoot = %q, want registered project root %q from nested working directory %q", captured.ProjectRoot, projectRoot, nestedWorkingDir)
	}
}

func TestRunnerConfigCheckFixFromNestedDirectoryRefusesProjectRootExecutable(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) error = %v", err)
	}
	root, home := setupInstallCommandFixture(t)
	projectRoot := root
	nestedWorkingDir := filepath.Join(projectRoot, "nested", "worktree")
	if err := os.MkdirAll(nestedWorkingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested working directory) error = %v", err)
	}
	if output, err := exec.Command(gitPath, "init", "-q", projectRoot).CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v\n%s", err, output)
	}
	writeInstallFile(t, filepath.Join(projectRoot, ".agents", "loaf.json"), `{"version":"1.0.0","initialized":"2026-07-13T00:00:00Z","knowledge":{"local":["docs/knowledge","docs/decisions"],"staleness_threshold_days":30,"imports":[]},"integrations":{"linear":{"enabled":false},"serena":{"enabled":false}}}`+"\n")
	writeInstallFile(t, filepath.Join(projectRoot, "dist", "codex", ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook","commandWindows":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`+"\n")
	// The shipped Codex identity, placeholder and all: the refusal under test is
	// what happens when reconciliation tries to render it.
	installTestHookDistribution(t, projectRoot, "codex", testCodexHookCatalogSource())
	writeInstallFile(t, filepath.Join(home, ".codex", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(home, ".codex", "hooks.json"), `{"hooks":{}}`+"\n")
	fakeLoaf := filepath.Join(projectRoot, "bin", "loaf")
	writeInstallFile(t, fakeLoaf, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(fakeLoaf, 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}

	var output bytes.Buffer
	runErr := (Runner{Stdout: &output, WorkingDir: nestedWorkingDir, Executable: distributionFixtureExecutable(projectRoot)}).Run([]string{"config", "check", "--fix", "--json"})
	var exitErr ExitError
	if !errors.As(runErr, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("nested config check --fix error = %v, want trust refusal exit 2", runErr)
	}
	var result configCheckResult
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
		t.Fatalf("Unmarshal(nested config check output) error = %v\n%s", decodeErr, output.String())
	}
	joinedErrors := strings.Join(result.Errors, "\n")
	if !strings.Contains(joinedErrors, "inside forbidden path") || !strings.Contains(joinedErrors, projectRoot) {
		t.Fatalf("nested config check errors = %q, want project-root executable trust refusal", joinedErrors)
	}
	assertInstallFile(t, filepath.Join(home, ".codex", "hooks.json"), `{"hooks":{}}`+"\n")
}

// The five states, all at once, on one installed target. Nothing about the two
// foreign entries this fixture also carries appears anywhere in the answer.
func TestRunnerConfigCheckDiagnosesEveryHookState(t *testing.T) {
	root, home := setupConfigCheckHookFixture(t)
	path := hooksFixtureFilePath(home)
	seedHooksFixtureForeignEntries(t, path)

	// disabled-and-correctly-absent: the verb recorded it and reprojected.
	runInstallFixture(t, root, "hooks", "disable", "render-drift", "--target", "cursor")
	// disabled-but-present: recorded, then the entry put back by hand.
	runInstallFixture(t, root, "hooks", "disable", "validate-push", "--target", "cursor")
	restoreHooksFixtureEntry(t, path, "beforeShellExecution", map[string]any{
		"command": "loaf check --hook validate-push", "matcher": "Bash", "loaf-managed": true,
	})
	// enabled-but-stale: fail-closed enforcement weakened by hand.
	weakenHooksFixtureEntry(t, path, "beforeShellExecution", "loaf check --hook security-audit")
	// enabled-and-missing: deleted by hand, never recorded.
	deleteHooksFixtureEntry(t, path, "beforeSubmitPrompt", "loaf check --hook artifact-names")
	// enabled-and-in-sync: validate-commit, untouched since the install.

	result := runConfigCheckJSON(t, root, false)
	cursor := findConfigTargetStatus(result.Targets, "cursor")
	if result.OK || cursor.Status != "stale" {
		t.Fatalf("result.OK = %v cursor = %#v, want the target reported as needing work", result.OK, cursor)
	}
	want := map[string]string{
		"render-drift":    hookStateDisabledAbsent,
		"validate-push":   hookStateDisabledPresent,
		"security-audit":  hookStateEnabledStale,
		"artifact-names":  hookStateEnabledMissing,
		"validate-commit": hookStateEnabledInSync,
	}
	got := map[string]string{}
	for _, hook := range cursor.Hooks {
		got[hook.HookID] = hook.State
	}
	if len(got) != len(want) {
		t.Fatalf("diagnosed hooks = %#v, want exactly the catalog identities %#v", got, want)
	}
	for hookID, state := range want {
		if got[hookID] != state {
			t.Fatalf("%s = %q, want %q", hookID, got[hookID], state)
		}
	}
	// The file also carries entries and a section Loaf does not own. Nothing in
	// the answer knows they exist.
	for _, foreign := range []string{"herdr", "afterFileEdit", "operator"} {
		if body, _ := json.Marshal(result); strings.Contains(string(body), foreign) {
			t.Fatalf("config check output names the foreign content %q:\n%s", foreign, body)
		}
	}

	// The operator's view of the same run.
	text := stripANSI(runConfigCheckText(t, root))
	for _, line := range []string{
		"  ! Cursor hooks need reconcile: security-audit (enabled but stale), artifact-names (enabled and missing)\n",
		"  ! Cursor hooks need reprojection: validate-push (disabled but present)\n",
		"  x cursor: hooks need reconcile: security-audit (enabled but stale), artifact-names (enabled and missing); hooks need reprojection: validate-push (disabled but present)\n",
	} {
		if !strings.Contains(text, line) {
			t.Fatalf("config check text = %q, want the line %q", text, line)
		}
	}
}

// A hook the operator disabled is healthy when it is absent — the state the old
// whole-file diagnosis could only read as a missing hook to be reinstated.
func TestRunnerConfigCheckReportsADisabledHookAsHealthilyAbsent(t *testing.T) {
	root, _ := setupConfigCheckHookFixture(t)
	runInstallFixture(t, root, "hooks", "disable", "render-drift", "--target", "cursor")

	result := runConfigCheckJSON(t, root, false)
	cursor := findConfigTargetStatus(result.Targets, "cursor")
	if !result.OK || cursor.Status != "ok" {
		t.Fatalf("result.OK = %v cursor = %#v, want a disabled-and-absent hook to read healthy", result.OK, cursor)
	}
	for _, hook := range cursor.Hooks {
		if hook.HookID == "render-drift" && hook.State != hookStateDisabledAbsent {
			t.Fatalf("render-drift = %q, want %q", hook.State, hookStateDisabledAbsent)
		}
	}
	if text := stripANSI(runConfigCheckText(t, root)); !strings.Contains(text, "✓ Cursor hooks current\n") {
		t.Fatalf("config check text = %q, want the target reported current", text)
	}
}

// `--fix` converges through the reconciler and nothing else: the hook the
// operator deleted comes back, the one they disabled stays out, and their own
// entries are where they were.
func TestRunnerConfigCheckFixConvergesHooksThroughTheReconciler(t *testing.T) {
	root, home := setupConfigCheckHookFixture(t)
	path := hooksFixtureFilePath(home)
	seedHooksFixtureForeignEntries(t, path)
	runInstallFixture(t, root, "hooks", "disable", "render-drift", "--target", "cursor")
	deleteHooksFixtureEntry(t, path, "beforeSubmitPrompt", "loaf check --hook artifact-names")

	before := runConfigCheckJSON(t, root, false)
	if before.OK {
		t.Fatalf("before = %#v, want the deleted hook reported", before)
	}

	after := runConfigCheckJSON(t, root, true)
	cursor := findConfigTargetStatus(after.Targets, "cursor")
	if !after.OK || !after.Fixed || cursor.Status != "updated" {
		t.Fatalf("after = %#v cursor = %#v, want the target converged by --fix", after, cursor)
	}
	body := string(readFileBytes(t, path))
	if !strings.Contains(body, "loaf check --hook artifact-names") {
		t.Fatalf("hooks.json = %s, want the deleted hook restored", body)
	}
	if strings.Contains(body, "loaf check --hook render-drift") {
		t.Fatalf("hooks.json = %s, want the disabled hook to stay out", body)
	}
	if !strings.Contains(body, "bash /opt/herdr/agent-state.sh beforeSubmitPrompt") {
		t.Fatalf("hooks.json = %s, want the foreign entries preserved", body)
	}
}

// A plain check reads. The state database is where enablement lives, and a
// diagnosis that created one would be a write this command does not promise.
func TestRunnerConfigCheckLeavesTheStateDatabaseUntouched(t *testing.T) {
	root, home := setupConfigCheckHookFixture(t)
	database := filepath.Join(t.TempDir(), "absent", "loaf.sqlite")
	t.Setenv("LOAF_DB", database)
	deleteHooksFixtureEntry(t, hooksFixtureFilePath(home), "beforeSubmitPrompt", "loaf check --hook artifact-names")

	result := runConfigCheckJSON(t, root, false)

	if result.OK {
		t.Fatalf("result = %#v, want the missing hook reported without a database", result)
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) = %v, want a read-only check to have created nothing", database, err)
	}
	// With no records at all every catalog hook reads enabled, so the file is
	// the only thing that decides.
	cursor := findConfigTargetStatus(result.Targets, "cursor")
	for _, hook := range cursor.Hooks {
		if hook.HookID == "artifact-names" && hook.State != hookStateEnabledMissing {
			t.Fatalf("artifact-names = %q, want %q", hook.State, hookStateEnabledMissing)
		}
	}
}

// setupConfigCheckHookFixture is the hooks fixture plus the project config file
// `loaf config check` validates alongside the harnesses, so a target verdict is
// never confused with a config-file one.
func setupConfigCheckHookFixture(t *testing.T) (string, string) {
	t.Helper()
	sources := append(hooksFixtureCursorSources(), hooksFixtureCursorSource("beforeShellExecution", "security-audit"))
	root, home := setupHooksFixture(t, sources...)
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), `{"version":"1.0.0","initialized":"2026-08-08T00:00:00Z","knowledge":{"local":["docs/knowledge","docs/decisions"],"staleness_threshold_days":30,"imports":[]},"integrations":{"linear":{"enabled":false},"serena":{"enabled":false},"github":{"account":"canary"}}}`+"\n")
	return root, home
}

func runConfigCheckJSON(t *testing.T, root string, fix bool) configCheckResult {
	t.Helper()
	args := []string{"config", "check", "--json"}
	if fix {
		args = []string{"config", "check", "--fix", "--json"}
	}
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run(args)
	var exitErr ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.Code != 2) {
		t.Fatalf("config check error = %v\n%s", err, stdout.String())
	}
	var result configCheckResult
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("Unmarshal(config check output) error = %v\n%s", decodeErr, stdout.String())
	}
	return result
}

func runConfigCheckText(t *testing.T, root string) string {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"config", "check"})
	var exitErr ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.Code != 2) {
		t.Fatalf("config check error = %v\n%s", err, stdout.String())
	}
	return stdout.String()
}

func TestRunnerConfigHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: t.TempDir()}).Run([]string{"config", "--help"}); err != nil {
		t.Fatalf("config --help error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: loaf config") || !strings.Contains(stdout.String(), "check") {
		t.Fatalf("stdout = %q, want config help", stdout.String())
	}
	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: t.TempDir()}).Run([]string{"config", "check", "--help"}); err != nil {
		t.Fatalf("config check --help error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: loaf config check") || !strings.Contains(stdout.String(), "--fix") {
		t.Fatalf("stdout = %q, want config check help", stdout.String())
	}
}

func findConfigTargetStatus(targets []configTargetStatus, target string) configTargetStatus {
	for _, status := range targets {
		if status.Target == target {
			return status
		}
	}
	return configTargetStatus{}
}

func jsonStrings(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want []any", value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("item = %#v, want string", item)
		}
		result = append(result, value)
	}
	return result
}

func TestRunnerConfigCheckWarnsMissingIssuePrefixWithoutInventing(t *testing.T) {
	root, _ := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), strings.Join([]string{
		`{`,
		`  "version": "1.0.0",`,
		`  "initialized": "2026-07-06T00:00:00Z",`,
		`  "knowledge": {`,
		`    "local": ["docs/knowledge", "docs/decisions"],`,
		`    "staleness_threshold_days": 30,`,
		`    "imports": []`,
		`  },`,
		`  "integrations": {`,
		`    "linear": {"enabled": false},`,
		`    "serena": {"enabled": false},`,
		`    "github": {"account": "levifig"}`,
		`  }`,
		`}`,
	}, "\n")+"\n")

	before := runConfigCheckJSON(t, root, false)
	if !before.OK {
		t.Fatalf("before = %#v, want valid config with issue.prefix warning", before)
	}
	joined := strings.Join(before.Warnings, "\n")
	if !strings.Contains(joined, "issue.prefix is not configured") {
		t.Fatalf("warnings = %q, want missing issue.prefix", joined)
	}

	after := runConfigCheckJSON(t, root, true)
	if !after.OK {
		t.Fatalf("after = %#v, want --fix to leave project-owned issue.prefix unset", after)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	if _, ok := config["issue"]; ok {
		t.Fatalf("config = %#v, want --fix not to invent issue", config)
	}
}
