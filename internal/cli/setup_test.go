package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerSetupHelpIsNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"setup", "--help"})
	if err != nil {
		t.Fatalf("setup --help error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "Usage: loaf setup [path]") || !strings.Contains(output, "One-step bootstrap") {
		t.Fatalf("output = %q, want native setup help", output)
	}
}

func TestRunnerSetupRunsInitBuildAndInstallNatively(t *testing.T) {
	root := setupCommandLoafRoot(t)
	stateHome := t.TempDir()
	npmLog := setupFakeNPM(t, 0)
	target := filepath.Join(root, "fixture-project")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
		StateHome:  stateHome,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"setup", target})
	if err != nil {
		t.Fatalf("setup error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "docs", "ARCHITECTURE.md")); err != nil {
		t.Fatalf("ARCHITECTURE.md stat error = %v", err)
	}
	log := readSetupLog(t, npmLog)
	if !strings.Contains(log, "cwd="+root) || !strings.Contains(log, "args=run build") {
		t.Fatalf("npm log = %q, want build run at loaf package root", log)
	}
	output := stdout.String()
	for _, want := range []string{"loaf setup", "loaf init", "loaf install", "Setup complete"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	assertNoStateDatabase(t, target, stateHome)
}

func TestRunnerSetupRejectsExistingFilePath(t *testing.T) {
	root := setupCommandLoafRoot(t)
	target := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(target, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: root,
	}.Run([]string{"setup", target})
	if err == nil {
		t.Fatal("setup error = nil, want existing-file error")
	}
	if !strings.Contains(err.Error(), "path exists but is not a directory") {
		t.Fatalf("error = %v, want existing-file error", err)
	}
}

func TestRunnerSetupReportsBuildFailure(t *testing.T) {
	root := setupCommandLoafRoot(t)
	setupFakeNPM(t, 42)
	target := filepath.Join(root, "fixture-project")

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: root,
	}.Run([]string{"setup", target})
	if err == nil {
		t.Fatal("setup error = nil, want build failure")
	}
	if !strings.Contains(err.Error(), "setup build failed") {
		t.Fatalf("error = %v, want build failure", err)
	}
}

func setupCommandLoafRoot(t *testing.T) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll(home) error = %v", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PATH", bin)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"loaf","version":"9.8.7-test.1"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}
	return root
}

func setupFakeNPM(t *testing.T, exitCode int) string {
	t.Helper()
	bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	log := filepath.Join(t.TempDir(), "npm.log")
	t.Setenv("LOAF_TEST_NPM_LOG", log)
	script := strings.Join([]string{
		"#!/bin/sh",
		`printf 'cwd=%s\n' "$PWD" >> "$LOAF_TEST_NPM_LOG"`,
		`printf 'args=%s\n' "$*" >> "$LOAF_TEST_NPM_LOG"`,
		"exit " + fmt.Sprint(exitCode),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(npm) error = %v", err)
	}
	return log
}

func readSetupLog(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}
