package cli

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitInstallTargetsAcceptsCommaLists(t *testing.T) {
	if got := splitInstallTargets("cursor, codex,,claude-code,cursor"); strings.Join(got, "|") != "cursor|codex|claude-code" {
		t.Fatalf("splitInstallTargets = %q, want deduplicated trimmed list", got)
	}
	if got := splitInstallTargets("all"); len(got) != 1 || got[0] != "all" {
		t.Fatalf("splitInstallTargets(all) = %q", got)
	}
}

func TestParseInstallArgsListsAndInteractive(t *testing.T) {
	options, err := parseInstallArgs([]string{"--to", "cursor,codex", "--codex-basic-commands"})
	if err != nil || strings.Join(options.targets, "|") != "cursor|codex" {
		t.Fatalf("options = %#v err = %v, want cursor and codex", options, err)
	}
	if _, err := parseInstallArgs([]string{"-i", "--to", "cursor"}); err == nil || !strings.Contains(err.Error(), "drop --to or drop -i") {
		t.Fatalf("-i with --to error = %v, want a refusal", err)
	}
	options, err = parseInstallArgs([]string{"--interactive"})
	if err != nil || !options.interactive {
		t.Fatalf("options = %#v err = %v, want interactive", options, err)
	}
}

func TestPromptInstallChecklistSelections(t *testing.T) {
	entries := []installChecklistEntry{
		{Key: "cursor", Name: "Cursor", Status: "installed"},
		{Key: "codex", Name: "Codex"},
		{Key: claudeCodeInstallTarget, Name: "Claude Code", Status: "plugin"},
	}
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "enter keeps all", input: "\n", want: "cursor|codex|claude-code"},
		{name: "eof keeps all", input: "", want: "cursor|codex|claude-code"},
		{name: "all keyword", input: "all\n", want: "cursor|codex|claude-code"},
		{name: "none", input: "none\n", want: ""},
		{name: "numbers", input: "1 3\n", want: "cursor|claude-code"},
		{name: "names and commas", input: "codex, claude code\n", want: "codex|claude-code"},
		{name: "duplicate collapses", input: "2 2 codex\n", want: "codex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := promptInstallChecklist(bufio.NewReader(strings.NewReader(tc.input)), &out, "Install", entries)
			if err != nil {
				t.Fatalf("checklist error = %v", err)
			}
			if strings.Join(got, "|") != tc.want {
				t.Fatalf("selection = %q, want %q", got, tc.want)
			}
			if !strings.Contains(out.String(), "Install to which harnesses?") || !strings.Contains(out.String(), "1. Cursor") {
				t.Fatalf("prompt output = %q, want the numbered checklist", out.String())
			}
		})
	}
	var out bytes.Buffer
	if _, err := promptInstallChecklist(bufio.NewReader(strings.NewReader("7\n")), &out, "Install", entries); err == nil || !strings.Contains(err.Error(), "unknown selection") {
		t.Fatalf("out-of-range selection error = %v, want refusal", err)
	}
}

func TestRunnerInstallDefaultsToEveryDetectedHarnessWithoutPrompting(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	// Stdin is empty on purpose: the default path must not read a selection.
	err := Runner{Stdout: &stdout, Stdin: strings.NewReader(""), WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--yes"})
	if err != nil {
		t.Fatalf("default install error = %v\n%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "which harnesses?") || strings.Contains(stdout.String(), "[Y/n]") {
		t.Fatalf("stdout = %q, must not prompt for targets by default", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Cursor installed") {
		t.Fatalf("stdout = %q, want Cursor installed by default", stdout.String())
	}
}

func TestRunnerInstallAcceptsACommaSeparatedTargetList(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	writeInstallFile(t, filepath.Join(root, "dist", "opencode", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "opencode")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "cursor,opencode", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor,opencode error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"Cursor installed", "OpenCode installed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "cursor,all", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("--to cursor,all error = %v, want refusal", err)
	}
}

func TestSelectUpgradeTargetsHandlesListsAndClaudeCode(t *testing.T) {
	tools := []detectedInstallTool{
		{key: "cursor", name: "Cursor", installed: true},
		{key: "codex", name: "Codex", installed: false},
	}
	runner := Runner{}
	options, err := parseUpgradeArgs([]string{"--to", "cursor,claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	targets, refresh, err := runner.selectUpgradeTargets(options, tools, true, &bytes.Buffer{})
	if err != nil || strings.Join(targets, "|") != "cursor" || !refresh {
		t.Fatalf("targets=%q refresh=%t err=%v, want cursor plus a Claude Code refresh", targets, refresh, err)
	}
	options, _ = parseUpgradeArgs([]string{"--to", "codex"})
	if _, _, err := runner.selectUpgradeTargets(options, tools, true, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not installed here") {
		t.Fatalf("uninstalled target error = %v, want onboarding hint", err)
	}
	options, _ = parseUpgradeArgs(nil)
	targets, refresh, err = runner.selectUpgradeTargets(options, tools, false, &bytes.Buffer{})
	if err != nil || strings.Join(targets, "|") != "cursor" || refresh {
		t.Fatalf("default targets=%q refresh=%t err=%v, want installed only and no Claude refresh without the CLI", targets, refresh, err)
	}
	options, _ = parseUpgradeArgs([]string{"-i"})
	var out bytes.Buffer
	targets, refresh, err = Runner{Stdin: strings.NewReader("claude code\n")}.selectUpgradeTargets(options, tools, true, &out)
	if err != nil || len(targets) != 0 || !refresh || !strings.Contains(out.String(), "Upgrade to which harnesses?") {
		t.Fatalf("interactive targets=%q refresh=%t err=%v out=%q, want only the Claude Code refresh", targets, refresh, err, out.String())
	}
}
