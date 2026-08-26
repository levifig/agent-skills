package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerReleaseHelpIsNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"release", "--help"})
	if err != nil {
		t.Fatalf("release --help error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage: loaf release <subcommand>") || !strings.Contains(output, "suggest") || !strings.Contains(output, "cut") {
		t.Fatalf("output = %q, want retroactive suggest/cut help", output)
	}
	if strings.Contains(output, "Create a new release with changelog") {
		t.Fatalf("output = %q, want legacy apply-path help removed", output)
	}
}

func TestReleaseLegacyFlagsFailWithGuidance(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	for _, args := range [][]string{
		{"release", "--bump", "patch"},
		{"release", "--pre-merge"},
		{"release", "--post-merge"},
		{"release", "--yes"},
		{"release"},
	} {
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			WorkingDir: workingDir,
		}.Run(args)
		if err == nil {
			t.Fatalf("%v error = nil, want guidance", args)
		}
		msg := err.Error() + stdout.String()
		if !strings.Contains(msg, "suggest") || !strings.Contains(msg, "cut") {
			t.Fatalf("%v error = %v\n%s, want suggest/cut guidance", args, err, stdout.String())
		}
	}
}


// commitBootstrapAgentsFiles records genesis .agents/ files (e.g. loaf.conf) so
// release cut's clean-tree gate passes after state init.
func commitBootstrapAgentsFiles(t *testing.T, repo string) {
	t.Helper()
	status := gitOutputReleaseTest(t, repo, "status", "--porcelain=v1", ".agents")
	if strings.TrimSpace(status) == "" {
		return
	}
	gitCLI(t, repo, "add", ".agents")
	gitCLI(t, repo, "commit", "-m", "chore: bootstrap loaf project files")
}

func seedReleaseTaggedRepo(t *testing.T) string {
	t.Helper()
	repo := realpath(t, t.TempDir())
	gitCLI(t, repo, "init", "-b", "main")
	gitCLI(t, repo, "config", "user.name", "Loaf Test")
	gitCLI(t, repo, "config", "user.email", "loaf@example.test")
	gitCLI(t, repo, "config", "commit.gpgsign", "false")
	gitCLI(t, repo, "config", "tag.gpgsign", "false")
	writeFile(t, filepath.Join(repo, "package.json"), "{\n  \"name\": \"release-fixture\",\n  \"version\": \"1.0.0\",\n  \"scripts\": {\n    \"build\": \"echo build\"\n  }\n}\n")
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- _No unreleased changes yet._",
		"",
	}, "\n"))
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "commit", "-m", "chore: initial release")
	gitCLI(t, repo, "tag", "v1.0.0")
	return repo
}

func gitOutputReleaseTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
