package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func committedOriginFixture(t *testing.T, slug, date string) (string, string, []byte) {
	t.Helper()
	repo := t.TempDir()
	if err := originGitCLI(repo, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "config", "user.name", "Loaf Test"); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "config", "user.email", "loaf@example.test"); err != nil {
		t.Fatal(err)
	}
	changeFile := filepath.Join(repo, "docs", "changes", date+"-"+slug, "change.md")
	content := []byte("---\nslug: " + slug + "\n---\ncommitted bytes\n")
	if err := os.MkdirAll(filepath.Dir(changeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changeFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := originGitCLI(repo, "-c", "commit.gpgsign=false", "commit", "-m", "initial"); err != nil {
		t.Fatal(err)
	}
	return repo, changeFile, content
}

func originGitCLI(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(output)))
	}
	return nil
}

func mustOriginGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(output)
}
