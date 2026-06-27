package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerVersionUsesNativePackageMetadata(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"loaf",
		"9.8.7-test.1",
		"go",
		"Targets:",
		"claude-code",
		"plugins/loaf/",
		"cursor",
		"dist/cursor/",
		"Content:",
		"Skills:  2",
		"Agents:  2",
		"Hooks:   4",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output = %q, want %q", output, want)
		}
	}
}

func TestRunnerVersionDoesNotRequireLegacyBridge(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=version") {
		t.Fatalf("version output = %q, want native output instead of legacy bridge", stdout.String())
	}
}

func TestRunnerVersionFlagsDoNotRequireLegacyBridge(t *testing.T) {
	root := writeVersionFixture(t)
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			var stdout bytes.Buffer

			err := Runner{
				Stdout:     &stdout,
				WorkingDir: root,
			}.Run([]string{flag})
			if err != nil {
				t.Fatalf("%s error = %v", flag, err)
			}
			output := stdout.String()
			if !strings.Contains(output, "9.8.7-test.1") || !strings.Contains(output, "Content:") {
				t.Fatalf("%s output = %q, want native version details", flag, output)
			}
		})
	}
}

func TestBuildInfoSuffixFormatsBothPartsDateThenCommit(t *testing.T) {
	got := buildInfoSuffix("abc1234", "2026-06-27T12:00:00Z")
	want := " (built 2026-06-27T12:00:00Z · git abc1234)"
	if got != want {
		t.Fatalf("buildInfoSuffix = %q, want %q", got, want)
	}
}

func TestBuildInfoSuffixEmptyWhenNeitherSet(t *testing.T) {
	if got := buildInfoSuffix("", ""); got != "" {
		t.Fatalf("buildInfoSuffix = %q, want empty string", got)
	}
}

func TestBuildInfoSuffixOnlyCommit(t *testing.T) {
	got := buildInfoSuffix("abc1234", "")
	want := " (git abc1234)"
	if got != want {
		t.Fatalf("buildInfoSuffix = %q, want %q", got, want)
	}
}

func TestBuildInfoSuffixOnlyDate(t *testing.T) {
	got := buildInfoSuffix("", "2026-06-27T12:00:00Z")
	want := " (built 2026-06-27T12:00:00Z)"
	if got != want {
		t.Fatalf("buildInfoSuffix = %q, want %q", got, want)
	}
}

func TestRunnerVersionIncludesBuildInfoWhenSet(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:      &stdout,
		WorkingDir:  root,
		BuildCommit: "abc1234",
		BuildDate:   "2026-06-27T12:00:00Z",
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	// ansiBold("loaf") renders as "\x1b[1mloaf\x1b[0m"; assert the exact rendered
	// first line so the build-info suffix sits immediately after the version.
	want := "loaf\x1b[0m 9.8.7-test.1 (built 2026-06-27T12:00:00Z · git abc1234)\n"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("version output = %q, want to contain %q", stdout.String(), want)
	}
}

func TestRunnerVersionCleanWithoutBuildInfo(t *testing.T) {
	root := writeVersionFixture(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}

	output := stdout.String()
	cleanLine := "loaf\x1b[0m 9.8.7-test.1\n"
	if !strings.Contains(output, cleanLine) {
		t.Fatalf("version output = %q, want clean version line %q (no regression)", output, cleanLine)
	}
	for _, forbidden := range []string{"(built", "· git", "(git "} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("version output = %q, must not contain %q without build info", output, forbidden)
		}
	}
}

func writeVersionFixture(t *testing.T) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "content", "skills", "go-development"))
	mkdirAll(t, filepath.Join(root, "content", "skills", "typescript-development"))
	mkdirAll(t, filepath.Join(root, "content", "agents"))
	mkdirAll(t, filepath.Join(root, "config"))
	mkdirAll(t, filepath.Join(root, "plugins", "loaf"))
	mkdirAll(t, filepath.Join(root, "dist", "cursor"))

	writeFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`)
	writeFile(t, filepath.Join(root, "content", "skills", "README.md"), "# not a skill\n")
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.md"), "# Implementer\n")
	writeFile(t, filepath.Join(root, "content", "agents", "reviewer.md"), "# Reviewer\n")
	writeFile(t, filepath.Join(root, "content", "agents", "reviewer.yaml"), "ignored: true\n")
	writeFile(t, filepath.Join(root, "config", "hooks.yaml"), strings.Join([]string{
		"hooks:",
		"  pre-tool:",
		"    - id: check-secrets",
		"    - id: session-nudge",
		"  post-tool:",
		"    - id: capture-result",
		"  session:",
		"    - id: pre-compact",
		"  pre-commit:",
		"    - id: ignored-by-version-command",
	}, "\n"))
	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
