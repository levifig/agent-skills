package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestRunnerConfigCheckFixCreatesProjectConfig(t *testing.T) {
	root, _ := setupInstallCommandFixture(t)

	var checkOut bytes.Buffer
	err := Runner{Stdout: &checkOut, WorkingDir: root}.Run([]string{"config", "check", "--json"})
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
	err = Runner{Stdout: &fixOut, WorkingDir: root}.Run([]string{"config", "check", "--fix", "--json"})
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
}

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
	writeInstallFile(t, filepath.Join(root, "dist", "codex", ".codex", "hooks.json"), `{"hooks":{}}`+"\n")
	writeInstallFile(t, filepath.Join(home, ".codex", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(home, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"user codex hook"}]}]}}`+"\n")

	var checkOut bytes.Buffer
	err := Runner{Stdout: &checkOut, WorkingDir: root}.Run([]string{"config", "check", "--json"})
	if err != nil {
		t.Fatalf("config check error = %v, want current schema to pass", err)
	}
	var before configCheckResult
	if err := json.Unmarshal(checkOut.Bytes(), &before); err != nil {
		t.Fatalf("Unmarshal(check output) error = %v\n%s", err, checkOut.String())
	}
	codexBefore := findConfigTargetStatus(before.Targets, "codex")
	if codexBefore.Status != "ok" || len(codexBefore.MissingHooks) != 0 {
		t.Fatalf("codexBefore = %#v, want current Codex hook contract", codexBefore)
	}

	var fixOut bytes.Buffer
	err = Runner{Stdout: &fixOut, WorkingDir: root}.Run([]string{"config", "check", "--fix", "--json"})
	if err != nil {
		t.Fatalf("config check --fix error = %v\n%s", err, fixOut.String())
	}
	var after configCheckResult
	if err := json.Unmarshal(fixOut.Bytes(), &after); err != nil {
		t.Fatalf("Unmarshal(fix output) error = %v\n%s", err, fixOut.String())
	}
	codexAfter := findConfigTargetStatus(after.Targets, "codex")
	if !after.OK || codexAfter.Status != "ok" || len(codexAfter.MissingHooks) != 0 {
		t.Fatalf("after = %#v codexAfter = %#v, want current codex hooks", after, codexAfter)
	}
	body, err := os.ReadFile(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.json) error = %v", err)
	}
	if strings.Contains(string(body), "loaf check --hook") || !strings.Contains(string(body), "user codex hook") {
		t.Fatalf("hooks.json = %s, want user hook preserved without Loaf handlers", body)
	}
}

func TestFixConfigTargetHooksRetainsProjectRootFromNestedWorkingDirectory(t *testing.T) {
	root := realpath(t, t.TempDir())
	projectRoot := filepath.Join(root, "project")
	nestedWorkingDir := filepath.Join(projectRoot, "nested", "worktree")
	loafRoot := filepath.Join(root, "loaf")
	configDir := filepath.Join(root, "codex-home")
	writeInstallFile(t, filepath.Join(loafRoot, "dist", "codex", ".codex", "hooks.json"), `{"hooks":{}}`+"\n")
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
	status := fixConfigTargetHooksWithInstaller(resolvedRoot.Path(), loafRoot, target, configTargetStatus{Status: "stale"}, func(options targetInstallOptions) error {
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
	writeInstallFile(t, filepath.Join(home, ".codex", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(home, ".codex", "hooks.json"), `{"hooks":{}}`+"\n")
	fakeLoaf := filepath.Join(projectRoot, "bin", "loaf")
	writeInstallFile(t, fakeLoaf, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(fakeLoaf, 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}

	var output bytes.Buffer
	runErr := (Runner{Stdout: &output, WorkingDir: nestedWorkingDir}).Run([]string{"config", "check", "--fix", "--json"})
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
