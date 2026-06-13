package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var regexpANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRunnerDoctorHealthyProjectRunsNatively(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	symlinkFile(t, "../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor error = %v", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "loaf doctor") || strings.Contains(output, "failed") {
		t.Fatalf("doctor output = %q, want native healthy report", output)
	}
	if strings.Contains(output, "args=doctor") {
		t.Fatalf("doctor output = %q, want no legacy bridge output", output)
	}
}

func TestRunnerDoctorReturnsExitOneForFailures(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorFile(t, filepath.Join(root, ".cursor", "rules", "loaf.mdc"), "legacy\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor error = %v, want exit code 1", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "stale-cursor-mdc") || !strings.Contains(output, "failed") {
		t.Fatalf("doctor failure output = %q, want failing stale-file check", output)
	}
}

func TestRunnerDoctorWarningsDoNotFail(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("0.0.1-test"))
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	symlinkFile(t, "../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor warning-only error = %v, want nil", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "Fenced section version drift") || strings.Contains(output, "failed") {
		t.Fatalf("doctor warning output = %q, want warning-only success", output)
	}
}

func TestRunnerDoctorFixMigratesLegacyFiles(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorFile(t, filepath.Join(root, "AGENTS.md"), "# Root Notes\n\nroot context\n")
	writeDoctorFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# Claude Notes\n\nclaude context\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor", "--fix"})
	if err != nil {
		t.Fatalf("doctor --fix legacy migration error = %v", err)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	for _, want := range []string{"root context", "claude context", "## Migrated from AGENTS.md", "## Migrated from .claude/CLAUDE.md"} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("canonical = %q, want %q", canonical, want)
		}
	}
	assertSymlinkTarget(t, filepath.Join(root, "AGENTS.md"), ".agents/AGENTS.md")
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../.agents/AGENTS.md")
	for _, path := range []string{"AGENTS.md.bak", ".claude/CLAUDE.md.bak"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("doctor backup %s missing: %v", path, err)
		}
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "fixed") || strings.Contains(output, "failed") {
		t.Fatalf("doctor --fix output = %q, want fixes and clean report", output)
	}
}

func TestRunnerDoctorFixStripsDuplicateFencedSection(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	writeDoctorFile(t, filepath.Join(root, "AGENTS.md"), "# root\n")
	writeDoctorFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# claude\n\n<!-- loaf:managed:start v0.1.0 -->\nold fence\n<!-- loaf:managed:end -->\n\ntrailing text\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor", "--fix"})
	if err != nil {
		t.Fatalf("doctor --fix duplicate fence error = %v", err)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	if strings.Contains(canonical, "old fence") {
		t.Fatalf("canonical = %q, want old managed fence stripped", canonical)
	}
	if !strings.Contains(canonical, "trailing text") {
		t.Fatalf("canonical = %q, want user text preserved", canonical)
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../.agents/AGENTS.md")
}

func TestRunnerDoctorVerboseAndHelpAreNative(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	symlinkFile(t, "../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "verbose", args: []string{"doctor", "--verbose"}, want: "agents-symlink"},
		{name: "help", args: []string{"doctor", "--help"}, want: "Usage: loaf doctor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: root,
			}.Run(tc.args)
			if err != nil {
				t.Fatalf("%v error = %v", tc.args, err)
			}
			if output := stripANSI(stdout.String()); !strings.Contains(output, tc.want) {
				t.Fatalf("%v output = %q, want %q", tc.args, output, tc.want)
			}
		})
	}
}

func TestRunnerDoctorRejectsUnknownOptionsNatively(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	err := Runner{
		WorkingDir: root,
	}.Run([]string{"doctor", "--wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown doctor option") {
		t.Fatalf("doctor unknown option error = %v, want native option error", err)
	}
}

func writeDoctorFixture(t *testing.T, version string) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"`+version+`"}`+"\n")
	return root
}

func writeDoctorAgents(t *testing.T, root string, body string) {
	t.Helper()
	writeDoctorFile(t, filepath.Join(root, ".agents", "AGENTS.md"), body)
}

func doctorFence(version string) string {
	return strings.Join([]string{
		"<!-- loaf:managed:start v" + version + " -->",
		"<!-- Maintained by loaf install/upgrade - do not edit manually -->",
		"## Loaf Framework",
		"",
		"Sample fenced content.",
		"<!-- loaf:managed:end -->",
		"",
	}, "\n")
}

func symlinkFile(t *testing.T, target string, linkPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("Symlink(%s, %s) error = %v", target, linkPath, err)
	}
}

func writeDoctorFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, body)
}

func assertSymlinkTarget(t *testing.T, linkPath string, want string) {
	t.Helper()
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat(%s) error = %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", linkPath)
	}
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s) error = %v", linkPath, err)
	}
	if got != want {
		t.Fatalf("Readlink(%s) = %q, want %q", linkPath, got, want)
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return body
}

func stripANSI(value string) string {
	return regexpANSI.ReplaceAllString(value, "")
}
