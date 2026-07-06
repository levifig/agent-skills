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

func TestRunnerConfigCheckFixRefreshesMissingManagedHooks(t *testing.T) {
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
	writeInstallFile(t, filepath.Join(root, "dist", "codex", ".codex", "hooks.json"), `{"version":1,"hooks":{"PreToolUse":[{"command":"loaf check --hook check-secrets","loaf-managed":true},{"command":"loaf check --hook github-account","loaf-managed":true}]}}`+"\n")
	writeInstallFile(t, filepath.Join(home, ".codex", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(home, ".codex", "hooks.json"), `{"version":1,"hooks":{"PreToolUse":[{"command":"loaf check --hook check-secrets","loaf-managed":true}]}}`+"\n")

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
	codexBefore := findConfigTargetStatus(before.Targets, "codex")
	if codexBefore.Status != "stale" || strings.Join(codexBefore.MissingHooks, ",") != "github-account" {
		t.Fatalf("codexBefore = %#v, want missing github-account", codexBefore)
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
	if !after.OK || codexAfter.Status != "updated" || len(codexAfter.MissingHooks) != 0 {
		t.Fatalf("after = %#v codexAfter = %#v, want refreshed codex hooks", after, codexAfter)
	}
	body, err := os.ReadFile(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.json) error = %v", err)
	}
	if !strings.Contains(string(body), "loaf check --hook github-account") {
		t.Fatalf("hooks.json = %s, want github-account hook", body)
	}
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
