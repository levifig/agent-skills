package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLauncherRequiresNativeRuntimeEvenWhenLegacyBundleExists(t *testing.T) {
	node := requireNode(t)
	root := createLauncherFixture(t)
	writeLegacyFallback(t, root)

	output, err := runCommand(root, node, filepath.Join(root, "bin", "loaf"), "release", "--help")
	if err == nil {
		t.Fatalf("launcher unexpectedly succeeded\n%s", output)
	}
	if strings.Contains(output, "fallback release --help") {
		t.Fatalf("launcher output = %q, want no TypeScript fallback execution", output)
	}
	for _, want := range []string{"native Loaf runtime not found for " + nodeRuntimeID(), "npm run build:go"} {
		if !strings.Contains(output, want) {
			t.Fatalf("launcher output = %q, want %q", output, want)
		}
	}
}

func TestLauncherRunsNativeRuntimeWithoutLegacyFallbackEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test writes a POSIX shell stub")
	}
	node := requireNode(t)
	root := createLauncherFixture(t)
	writeLegacyFallback(t, root)
	writeNativeRuntimeStub(t, root)

	output, err := runCommand(root, node, filepath.Join(root, "bin", "loaf"), "task", "list")
	if err != nil {
		t.Fatalf("launcher error = %v\n%s", err, output)
	}
	for _, want := range []string{"native task list", "legacy=\n"} {
		if !strings.Contains(output, want) {
			t.Fatalf("launcher output = %q, want %q", output, want)
		}
	}
}

func requireNode(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node executable not found")
	}
	return node
}

func createLauncherFixture(t *testing.T) string {
	t.Helper()
	source := filepath.Join(repoRoot(t), "cli", "runtime", "loaf-launcher.cjs")
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", binDir, err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", source, err)
	}
	launcher := filepath.Join(binDir, "loaf")
	if err := os.WriteFile(launcher, data, 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", launcher, err)
	}
	return root
}

func writeLegacyFallback(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "dist-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	path := filepath.Join(dir, "index.js")
	body := "#!/usr/bin/env node\nconsole.log('fallback ' + process.argv.slice(2).join(' '));\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func writeNativeRuntimeStub(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "bin", "native", nodeRuntimeID())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	name := "loaf"
	if runtime.GOOS == "windows" {
		name = "loaf.exe"
	}
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\nprintf 'native %s\\n' \"$*\"\nprintf 'legacy=%s\\n' \"${LOAF_LEGACY_CLI:-}\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func nodeRuntimeID() string {
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "ia32"
	}
	return platform + "-" + arch
}
