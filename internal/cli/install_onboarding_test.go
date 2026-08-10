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

// install_onboarding_test.go covers the boundary that makes install onboarding
// only: what it asks before touching a project, what it refuses to touch in a
// project that already has Loaf, and the flags that now belong to
// `loaf upgrade`. The literal removed invocation appears here on purpose — the
// sweep that forbids it everywhere else excludes tests, and these assertions
// are what keep it out of the tombstone's own wording.

func TestParseInstallArgsTombstonesFlagsThatMovedToUpgrade(t *testing.T) {
	for _, flag := range []string{"--upgrade", "--dry-run", "--json"} {
		_, err := parseInstallArgs([]string{flag})
		if err == nil {
			t.Fatalf("parseInstallArgs(%s) error = nil, want the removed-flag tombstone", flag)
		}
		if !strings.Contains(err.Error(), "loaf upgrade") {
			t.Fatalf("parseInstallArgs(%s) error = %q, want it to name loaf upgrade", flag, err)
		}
		if strings.Contains(err.Error(), "install --upgrade") {
			t.Fatalf("parseInstallArgs(%s) error = %q, must not spell the removed invocation", flag, err)
		}
	}
}

func TestRunnerInstallUpgradeFlagIsAHardError(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--upgrade", "--dry-run"})
	if err == nil {
		t.Fatalf("install with the removed flag error = nil, want a hard error\n%s", stdout.String())
	}
	if !strings.Contains(err.Error(), "loaf upgrade") || strings.Contains(err.Error(), "install --upgrade") {
		t.Fatalf("error = %q, want a pointer at loaf upgrade that never spells the removed invocation", err)
	}
	var exitErr ExitError
	if errors.As(err, &exitErr) && exitErr.Code != 1 {
		t.Fatalf("exit code = %d, want the default failure code 1", exitErr.Code)
	}
	// The error is raised at parse time, so nothing at all ran.
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	assertInstallPathMissing(t, filepath.Join(root, "AGENTS.md"))
}

// TestRunnerInstallOutsideALoafProjectAsksBeforeDeploying is the onboarding
// consent gate: the harness half runs either way, the project half only after a
// yes.
func TestRunnerInstallOutsideALoafProjectAsksBeforeDeploying(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		root, home := setupInstallOnboardingFixture(t)

		output := runInstallWithStdin(t, root, "y\n", "install", "--to", "cursor")

		if !strings.Contains(output, "Deploy Loaf to this folder? [y/N]") {
			t.Fatalf("install output = %q, want the deploy consent prompt", output)
		}
		assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
		body := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
		if !strings.Contains(body, "<!-- loaf:managed:start sha256=") {
			t.Fatalf("AGENTS.md = %q, want the managed section deployed after consent", body)
		}
		config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
		if _, ok := config["integrations"].(map[string]any); !ok {
			t.Fatalf("loaf.json = %#v, want the MCP recommendation record written after consent", config)
		}
	})

	t.Run("declined", func(t *testing.T) {
		root, home := setupInstallOnboardingFixture(t)

		output := runInstallWithStdin(t, root, "n\n", "install", "--to", "cursor")

		if !strings.Contains(output, "Deploy Loaf to this folder? [y/N]") {
			t.Fatalf("install output = %q, want the deploy consent prompt", output)
		}
		// Declining costs the project nothing and the harness nothing either.
		assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
		assertNoInstallProjectFiles(t, root)
	})
}

// TestRunnerInstallInsideALoafProjectOnboardsTheHarnessOnly is the other half of
// the matrix: --to still onboards a net-new harness from inside a Loaf repo,
// and the project surfaces it finds there belong to `loaf upgrade`, even under
// --yes.
func TestRunnerInstallInsideALoafProjectOnboardsTheHarnessOnly(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")
	writeInstallFile(t, filepath.Join(root, "AGENTS.md"), "# Project Instructions\n")

	output := runInstallCapture(t, root, "install", "--to", "cursor", "--yes")

	if !strings.Contains(output, "already deployed here") || !strings.Contains(output, "loaf upgrade") {
		t.Fatalf("install output = %q, want the already-deployed note pointing at loaf upgrade", output)
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(root, "AGENTS.md"), "# Project Instructions\n")
	assertInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")
	assertInstallPathMissing(t, filepath.Join(root, ".claude"))
}

// TestRunnerInstallWithoutATerminalReportsRequiredDeployConsent pins the
// automation contract: no prompt, no hang, no assumption, no files.
func TestRunnerInstallWithoutATerminalReportsRequiredDeployConsent(t *testing.T) {
	root, home := setupInstallOnboardingFixture(t)
	withoutTerminalStdin(t)

	output := runInstallCapture(t, root, "install", "--to", "cursor")

	if !strings.Contains(output, "Deploy consent required") {
		t.Fatalf("install output = %q, want the required-consent report", output)
	}
	if strings.Contains(output, "[y/N]") {
		t.Fatalf("install output = %q, want no prompt without a terminal", output)
	}
	assertNoInstallProjectFiles(t, root)
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
}

// TestRunnerInstallWithNothingToDeployNeitherAsksNorWrites covers the machine
// with no harness and no Claude Code: every project surface install manages
// belongs to some harness, so with none selected there is nothing to deploy —
// and consent must not be requested for it, let alone spent.
func TestRunnerInstallWithNothingToDeployNeitherAsksNorWrites(t *testing.T) {
	root, _ := setupInstallCommandFixture(t)

	// Decline every harness the host happens to expose, then leave a yes on
	// stdin: if a deploy prompt is asked anyway, it is answered in the affirmative
	// and the project assertions below catch whatever it writes.
	output := runInstallWithStdin(t, root, strings.Repeat("n\n", len(detectInstallTools()))+"y\n", "install")

	if strings.Contains(output, "Deploy Loaf to this folder?") {
		t.Fatalf("install output = %q, want no consent prompt when there is nothing to deploy", output)
	}
	if !strings.Contains(output, "No targets selected") {
		t.Fatalf("install output = %q, want the no-targets note", output)
	}
	assertNoInstallProjectFiles(t, root)
}

// TestRunnerConfigCheckFixStillRefreshesTargetsThroughTheSharedInstaller is the
// regression the retargeting risked: `config check --fix` drives
// installTargetDistribution directly, and none of install's new gating may
// reach it.
func TestRunnerConfigCheckFixStillRefreshesTargetsThroughTheSharedInstaller(t *testing.T) {
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
		`    "serena": {"enabled": false}`,
		`  }`,
		`}`,
	}, "\n")+"\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf check --hook check-secrets","matcher":"Bash","loaf-managed":true}]}}`+"\n")
	// The distribution hooks.json is what `config check` diagnoses against; the
	// catalog is what the reconciler restores from, so the two name one hook.
	installTestHookDistribution(t, root, "cursor", hookCatalogSource{
		event:    "PostToolUse",
		hookID:   "check-secrets",
		typeName: "command",
		command:  "loaf check --hook check-secrets",
		template: map[string]any{"command": "loaf check --hook check-secrets", "matcher": "Bash", "loaf-managed": true},
	})
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(home, ".cursor", "hooks.json"), `{"version":1,"hooks":{}}`+"\n")

	var checkOut bytes.Buffer
	err := Runner{Stdout: &checkOut, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"config", "check", "--json"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("config check error = %v, want the stale-hook failure\n%s", err, checkOut.String())
	}
	var before configCheckResult
	if err := json.Unmarshal(checkOut.Bytes(), &before); err != nil {
		t.Fatalf("Unmarshal(check output) error = %v\n%s", err, checkOut.String())
	}
	if status := findConfigTargetStatus(before.Targets, "cursor"); status.Status != "stale" {
		t.Fatalf("cursor before = %#v, want stale hooks", status)
	}

	var fixOut bytes.Buffer
	if err := (Runner{Stdout: &fixOut, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"config", "check", "--fix", "--json"}); err != nil {
		t.Fatalf("config check --fix error = %v\n%s", err, fixOut.String())
	}
	var after configCheckResult
	if err := json.Unmarshal(fixOut.Bytes(), &after); err != nil {
		t.Fatalf("Unmarshal(fix output) error = %v\n%s", err, fixOut.String())
	}
	if status := findConfigTargetStatus(after.Targets, "cursor"); status.Status != "updated" {
		t.Fatalf("cursor after = %#v, want the shared installer to have refreshed the target", status)
	}
	hooks := string(readFileBytes(t, filepath.Join(home, ".cursor", "hooks.json")))
	if !strings.Contains(hooks, "loaf check --hook check-secrets") {
		t.Fatalf("installed hooks.json = %s, want the managed hook restored", hooks)
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
}

// --- helpers -------------------------------------------------------------

// setupInstallOnboardingFixture is the install fixture plus the two things that
// keep an onboarding run down to a single prompt: a target with build output,
// and a `loaf` already on PATH so the binary self-install never asks.
func setupInstallOnboardingFixture(t *testing.T) (string, string) {
	t.Helper()
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	writeInstallFile(t, filepath.Join(root, "bin", "loaf"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(root, "bin", "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}
	return root, home
}

func runInstallWithStdin(t *testing.T, root string, stdin string, args ...string) string {
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

func assertNoInstallProjectFiles(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{"AGENTS.md", ".agents", ".claude"} {
		assertInstallPathMissing(t, filepath.Join(root, path))
	}
}
