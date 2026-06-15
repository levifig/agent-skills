package state

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestDatabasePathUsesStateHomeAndStaysOutsideRepository(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	stateHome := t.TempDir()

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	got, err := PathResolver{StateHome: stateHome}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	if got != filepath.Join(stateHome, "loaf", databaseFileName) {
		t.Fatalf("DatabasePath() = %q, want under state home %q", got, stateHome)
	}
	if strings.HasPrefix(got, repo+string(filepath.Separator)) {
		t.Fatalf("DatabasePath() = %q, want outside repository %q", got, repo)
	}
	if filepath.Base(got) != databaseFileName {
		t.Fatalf("DatabasePath() filename = %q, want %q", filepath.Base(got), databaseFileName)
	}
}

func TestDatabasePathMatchesForMainAndLinkedWorktree(t *testing.T) {
	requireGit(t)
	main := initGitRepo(t)
	linked := addLinkedWorktree(t, main, "state-path")
	stateHome := t.TempDir()

	mainRoot, err := project.ResolveRoot(main)
	if err != nil {
		t.Fatalf("ResolveRoot(main) error = %v", err)
	}
	linkedRoot, err := project.ResolveRoot(linked)
	if err != nil {
		t.Fatalf("ResolveRoot(linked) error = %v", err)
	}

	resolver := PathResolver{StateHome: stateHome}
	fromMain, err := resolver.DatabasePath(mainRoot)
	if err != nil {
		t.Fatalf("DatabasePath(main) error = %v", err)
	}
	fromLinked, err := resolver.DatabasePath(linkedRoot)
	if err != nil {
		t.Fatalf("DatabasePath(linked) error = %v", err)
	}

	if fromLinked != fromMain {
		t.Fatalf("linked DatabasePath() = %q, want main path %q", fromLinked, fromMain)
	}
}

func TestDatabasePathNonGitFallbackIsDeterministicForCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	stateHome := t.TempDir()

	firstRoot, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot(first) error = %v", err)
	}
	secondRoot, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot(second) error = %v", err)
	}

	resolver := PathResolver{StateHome: stateHome}
	first, err := resolver.DatabasePath(firstRoot)
	if err != nil {
		t.Fatalf("DatabasePath(first) error = %v", err)
	}
	second, err := resolver.DatabasePath(secondRoot)
	if err != nil {
		t.Fatalf("DatabasePath(second) error = %v", err)
	}

	if second != first {
		t.Fatalf("DatabasePath() = %q, want deterministic fallback %q", second, first)
	}
}

func TestDatabasePathIsSingleGlobalDatabaseAcrossProjects(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	stateHome := t.TempDir()

	firstRoot, err := project.ResolveRoot(firstDir)
	if err != nil {
		t.Fatalf("ResolveRoot(first) error = %v", err)
	}
	secondRoot, err := project.ResolveRoot(secondDir)
	if err != nil {
		t.Fatalf("ResolveRoot(second) error = %v", err)
	}

	resolver := PathResolver{StateHome: stateHome}
	first, err := resolver.DatabasePath(firstRoot)
	if err != nil {
		t.Fatalf("DatabasePath(first) error = %v", err)
	}
	second, err := resolver.DatabasePath(secondRoot)
	if err != nil {
		t.Fatalf("DatabasePath(second) error = %v", err)
	}

	if first != second {
		t.Fatalf("DatabasePath() = %q and %q, want one global database", first, second)
	}
}

func TestPathResolverUsesXDGDataHome(t *testing.T) {
	dir := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	got, err := PathResolver{}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	if !strings.HasPrefix(got, dataHome+string(filepath.Separator)) {
		t.Fatalf("DatabasePath() = %q, want under XDG_DATA_HOME %q", got, dataHome)
	}
}

func TestPathResolverPrefersXDGDataHomeOverLegacyStateHome(t *testing.T) {
	dir := t.TempDir()
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	got, err := PathResolver{}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	legacy, err := PathResolver{}.LegacyDatabasePath(root)
	if err != nil {
		t.Fatalf("LegacyDatabasePath() error = %v", err)
	}

	if !strings.HasPrefix(got, dataHome+string(filepath.Separator)) {
		t.Fatalf("DatabasePath() = %q, want under XDG_DATA_HOME %q", got, dataHome)
	}
	if !strings.HasPrefix(legacy, stateHome+string(filepath.Separator)) {
		t.Fatalf("LegacyDatabasePath() = %q, want under XDG_STATE_HOME %q", legacy, stateHome)
	}
	if got == legacy {
		t.Fatalf("DatabasePath() = LegacyDatabasePath() = %q, want distinct migration endpoints", got)
	}
}

func TestPathResolverRejectsRelativeStateHomeOverride(t *testing.T) {
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}

	_, err = PathResolver{StateHome: "relative-state"}.DatabasePath(root)
	if err == nil {
		t.Fatal("DatabasePath() error = nil, want relative state home rejection")
	}
}

func TestPathResolverRejectsStateHomeInsideProjectRoot(t *testing.T) {
	repo := initGitRepo(t)
	stateHome := filepath.Join(repo, ".loaf-state")

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	_, err = PathResolver{StateHome: stateHome}.DatabasePath(root)
	if err == nil {
		t.Fatal("DatabasePath() error = nil, want state home inside project root rejection")
	}
}

func TestPathResolverIgnoresRelativeXDGDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "relative-data")

	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	got, err := PathResolver{}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	if !filepath.IsAbs(got) {
		t.Fatalf("DatabasePath() = %q, want absolute fallback when XDG_DATA_HOME is relative", got)
	}
	if strings.Contains(got, "relative-data") {
		t.Fatalf("DatabasePath() = %q, want relative XDG_DATA_HOME ignored", got)
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
