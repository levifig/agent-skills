package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveWorkingDirectoryCleansAbsolutePath(t *testing.T) {
	base := t.TempDir()
	raw := filepath.Join(base, "nested", "..", ".")

	got, err := ResolveWorkingDirectory(raw)
	if err != nil {
		t.Fatalf("ResolveWorkingDirectory() error = %v", err)
	}

	want, err := filepath.Abs(base)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	want = filepath.Clean(want)

	if got.Path() != want {
		t.Fatalf("Path() = %q, want %q", got.Path(), want)
	}
}

func TestResolveRootUsesGitTopLevelFromSubdirectory(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	subdir := filepath.Join(repo, "nested", "deeper")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	got, err := ResolveRoot(subdir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}

	if got.Path() != repo {
		t.Fatalf("Path() = %q, want %q", got.Path(), repo)
	}
}

func TestResolveRootUsesMainWorktreeForLinkedWorktree(t *testing.T) {
	requireGit(t)
	main := initGitRepo(t)
	linked := addLinkedWorktree(t, main, "state-path")

	fromMain, err := ResolveRoot(main)
	if err != nil {
		t.Fatalf("ResolveRoot(main) error = %v", err)
	}
	fromLinked, err := ResolveRoot(linked)
	if err != nil {
		t.Fatalf("ResolveRoot(linked) error = %v", err)
	}

	if fromMain.Path() != main {
		t.Fatalf("main Path() = %q, want %q", fromMain.Path(), main)
	}
	if fromLinked.Path() != main {
		t.Fatalf("linked Path() = %q, want main %q", fromLinked.Path(), main)
	}
}

func TestResolveRootNonGitFallbackUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	first, err := ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot(first) error = %v", err)
	}
	second, err := ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot(second) error = %v", err)
	}

	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	want = filepath.Clean(want)

	if first.Path() != want {
		t.Fatalf("Path() = %q, want %q", first.Path(), want)
	}
	if second.Path() != first.Path() {
		t.Fatalf("second Path() = %q, want deterministic fallback %q", second.Path(), first.Path())
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}

	git(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	git(t, repo, "add", "README.md")
	git(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	return repo
}

func addLinkedWorktree(t *testing.T, repo string, branch string) string {
	t.Helper()
	parent := filepath.Dir(repo)
	linked := filepath.Join(parent, filepath.Base(repo)+"-"+branch)
	git(t, repo, "worktree", "add", "-b", branch, linked)
	realpath, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatalf("EvalSymlinks(linked) error = %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", linked).Run()
	})
	return realpath
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
