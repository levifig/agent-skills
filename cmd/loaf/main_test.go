package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicBinaryDispatchesStateVersionAndReleasePostMergeNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(repoRoot, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}

	workingDir := realpath(t, t.TempDir())
	dataHome := t.TempDir()
	legacyStateHome := t.TempDir()
	output, err := runBinary(binary, workingDir, envWith(
		"XDG_DATA_HOME="+dataHome,
		"XDG_STATE_HOME="+legacyStateHome,
	), "state", "path")
	if err != nil {
		t.Fatalf("loaf state path error = %v\n%s", err, output)
	}
	statePath := strings.TrimSpace(output)
	if !strings.HasPrefix(statePath, filepath.Join(dataHome, "loaf", "projects")+string(filepath.Separator)) {
		t.Fatalf("state path = %q, want under data home %q", statePath, dataHome)
	}
	if strings.HasPrefix(statePath, workingDir+string(filepath.Separator)) {
		t.Fatalf("state path = %q, want outside working dir %q", statePath, workingDir)
	}

	output, err = runBinary(binary, repoRoot, envWith(), "version")
	if err != nil {
		t.Fatalf("loaf version error = %v\n%s", err, output)
	}
	for _, want := range []string{"loaf", "Content:", "Skills:", "Agents:", "Hooks:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output = %q, want %q", output, want)
		}
	}

	output, err = runBinary(binary, workingDir, envWith(), "release", "--post-merge")
	if err == nil {
		t.Fatalf("loaf release --post-merge error = nil, want native guardrail failure\n%s", output)
	}
	for _, want := range []string{"loaf release", "Verifying post-merge state", "guardrail 1 failed"} {
		if !strings.Contains(output, want) {
			t.Fatalf("release output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "TypeScript fallback") {
		t.Fatalf("release output = %q, want native post-merge path without fallback lookup", output)
	}
}

func TestPublicBinaryDispatchesVersionFlagNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(repoRoot, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}

	output, err := runBinary(binary, repoRoot, envWith(), "--version")
	if err != nil {
		t.Fatalf("loaf --version error = %v\n%s", err, output)
	}
	for _, want := range []string{"loaf", "Content:", "Skills:", "Agents:", "Hooks:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("--version output = %q, want %q", output, want)
		}
	}
}

func TestPublicBinaryMigrateWorktreeStorageHelpAndDebugNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	main := createMainRepo(t, "help-main")

	output, err := runBinary(binary, main, envWith(), "migrate", "worktree-storage", "--help")
	if err != nil {
		t.Fatalf("loaf migrate worktree-storage --help error = %v\n%s", err, output)
	}
	for _, want := range []string{"--apply", "dry-run", "LOAF_DEBUG_RESOLVE"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output = %q, want %q", output, want)
		}
	}

	nonGit := realpath(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(nonGit, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonGit, ".agents", "AGENTS.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents/AGENTS.md) error = %v", err)
	}
	output, err = runBinary(binary, nonGit, envWith("LOAF_DEBUG_RESOLVE=1"), "migrate", "worktree-storage")
	if err == nil {
		t.Fatalf("loaf migrate worktree-storage in non-git dir error = nil, want git-context rejection\n%s", output)
	}
	for _, want := range []string{"LOAF_DEBUG_RESOLVE", "findMainWorktreeRoot fell back to parent-walk"} {
		if !strings.Contains(output, want) {
			t.Fatalf("debug output = %q, want %q", output, want)
		}
	}
}

func TestPublicBinaryPreA3WorktreeRefusalNudgeNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)

	main := createMainRepo(t, "nudge-refuse")
	linked := addLinkedWorktree(t, main, "nudge-refuse")
	seedPreA3WorktreeLayout(t, linked)
	output, err := runBinary(binary, linked, envWith(), "session", "list")
	if exitCode(err) != 2 {
		t.Fatalf("loaf session list exit = %d, want 2\n%s", exitCode(err), output)
	}
	for _, want := range []string{"SPEC-036", "loaf migrate worktree-storage", "LOAF_DEBUG_RESOLVE"} {
		if !strings.Contains(output, want) {
			t.Fatalf("refusal output = %q, want %q", output, want)
		}
	}

	main = createMainRepo(t, "nudge-unknown")
	linked = addLinkedWorktree(t, main, "nudge-unknown")
	seedPreA3WorktreeLayout(t, linked)
	output, err = runBinary(binary, linked, envWith(), "not-a-command")
	if exitCode(err) != 2 {
		t.Fatalf("loaf not-a-command exit = %d, want 2\n%s", exitCode(err), output)
	}
	for _, want := range []string{"unknown command 'not-a-command'", "SPEC-036", "loaf migrate worktree-storage"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unknown-command refusal output = %q, want %q", output, want)
		}
	}

	main = createMainRepo(t, "nudge-allow")
	linked = addLinkedWorktree(t, main, "nudge-allow")
	seedPreA3WorktreeLayout(t, linked)
	output, err = runBinary(binary, linked, envWith(), "migrate", "worktree-storage")
	if err != nil {
		t.Fatalf("loaf migrate worktree-storage error = %v\n%s", err, output)
	}
	if !strings.Contains(output, "Dry run") || strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("migrate allowlist output = %q, want dry-run without refusal", output)
	}

	for _, allowed := range [][]string{{"--help"}, {"--version"}} {
		main = createMainRepo(t, "nudge-allow-"+strings.TrimPrefix(allowed[0], "--"))
		linked = addLinkedWorktree(t, main, "nudge-allow-"+strings.TrimPrefix(allowed[0], "--"))
		seedPreA3WorktreeLayout(t, linked)
		output, err = runBinary(binary, linked, envWith(), allowed...)
		if err != nil {
			t.Fatalf("loaf %v error = %v\n%s", allowed, err, output)
		}
		if strings.Contains(output, "SPEC-036 centralizes") {
			t.Fatalf("allowlisted %v output = %q, want no refusal", allowed, output)
		}
	}

	main = createMainRepo(t, "nudge-main")
	output, err = runBinary(binary, main, envWith(), "version")
	if exitCode(err) == 2 || strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("main checkout output = %q, error = %v, want no pre-A3 refusal", output, err)
	}

	main = createMainRepo(t, "nudge-migrated")
	linked = addLinkedWorktree(t, main, "nudge-migrated")
	if err := os.MkdirAll(filepath.Join(linked, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(linked .agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(linked, ".agents", ".moved-to"), []byte(main+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.moved-to) error = %v", err)
	}
	output, err = runBinary(binary, linked, envWith(), "version")
	if exitCode(err) == 2 || strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("migrated linked worktree output = %q, error = %v, want no pre-A3 refusal", output, err)
	}

	main = createMainRepo(t, "nudge-stale-pointer")
	linked = addLinkedWorktree(t, main, "nudge-stale-pointer")
	seedPreA3WorktreeLayout(t, linked)
	if err := os.WriteFile(filepath.Join(linked, ".agents", ".moved-to"), []byte("/this/does/not/exist\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale .moved-to) error = %v", err)
	}
	output, err = runBinary(binary, linked, envWith(), "session", "list")
	if exitCode(err) != 2 || !strings.Contains(output, "SPEC-036") {
		t.Fatalf("stale pointer output = %q, error = %v, want pre-A3 refusal", output, err)
	}
}

func TestPublicBinaryMigrateWorktreeStorageFlagValidationNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	main := createMainRepo(t, "flag-mutex")
	linked := addLinkedWorktree(t, main, "flag-mutex")
	seedPreA3WorktreeLayout(t, linked)

	output, err := runBinary(binary, linked, envWith(), "migrate", "worktree-storage", "--force-from-worktree", "--force-from-main")
	if err == nil {
		t.Fatalf("loaf migrate worktree-storage flag conflict error = nil, want rejection\n%s", output)
	}
	for _, want := range []string{"--force-from-worktree", "--force-from-main"} {
		if !strings.Contains(output, want) {
			t.Fatalf("flag conflict output = %q, want %q", output, want)
		}
	}
}

func TestPublicBinaryRootHelpAndUnknownCommandAreNative(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(repoRoot, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}

	output, err := runBinary(binary, repoRoot, envWith())
	if err != nil {
		t.Fatalf("loaf root help error = %v\n%s", err, output)
	}
	for _, want := range []string{"Usage: loaf <command> [options]", "Commands:", "session", "task", "release"} {
		if !strings.Contains(output, want) {
			t.Fatalf("root help output = %q, want %q", output, want)
		}
	}

	output, err = runBinary(binary, repoRoot, envWith(), "not-a-command")
	if err == nil {
		t.Fatalf("loaf not-a-command error = nil, want exit error\n%s", output)
	}
	for _, want := range []string{"error: unknown command 'not-a-command'", "Usage: loaf <command> [options]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unknown-command output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "TypeScript fallback") {
		t.Fatalf("unknown-command output = %q, want native error without fallback lookup", output)
	}

	output, err = runBinary(binary, repoRoot, envWith(), "--agent-help")
	if err != nil {
		t.Fatalf("loaf --agent-help error = %v\n%s", err, output)
	}
	var doc struct {
		Name     string `json:"name"`
		Commands []struct {
			Name string `json:"name"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(output), &doc); err != nil {
		t.Fatalf("agent help JSON parse error = %v\n%s", err, output)
	}
	if doc.Name != "loaf" || len(doc.Commands) < 15 {
		t.Fatalf("agent help = %#v, want full native command catalog", doc)
	}
}

func buildLoafBinary(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(root, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}
	return binary
}

func createMainRepo(t *testing.T, name string) string {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", repoPath, err)
	}
	repoPath = realpath(t, repoPath)
	git(t, repoPath, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	git(t, repoPath, "add", ".")
	git(t, repoPath, "-c", "user.name=Test User", "-c", "user.email=test@test.com", "-c", "commit.gpgsign=false", "commit", "-m", "Initial commit")
	if err := os.MkdirAll(filepath.Join(repoPath, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".agents", "AGENTS.md"), []byte("# Project Instructions\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents/AGENTS.md) error = %v", err)
	}
	return repoPath
}

func addLinkedWorktree(t *testing.T, repoPath string, branch string) string {
	t.Helper()
	worktreePath := filepath.Join(t.TempDir(), branch)
	git(t, repoPath, "worktree", "add", "-b", branch, worktreePath)
	return realpath(t, worktreePath)
}

func seedPreA3WorktreeLayout(t *testing.T, worktreePath string) {
	t.Helper()
	agents := filepath.Join(worktreePath, ".agents")
	if err := os.MkdirAll(filepath.Join(agents, "sessions"), 0o755); err != nil {
		t.Fatalf("MkdirAll(sessions) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(agents, "kb"), 0o755); err != nil {
		t.Fatalf("MkdirAll(kb) error = %v", err)
	}
	for rel, body := range map[string]string{
		"AGENTS.md":                           "# Worktree AGENTS\n",
		"sessions/20260519-120000-session.md": "# Session\n",
		"kb/note.md":                          "# Note\n",
	} {
		if err := os.WriteFile(filepath.Join(agents, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s error = %v\n%s", strings.Join(args, " "), err, output)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %q missing go.mod: %v", root, err)
	}
	return root
}

func runCommand(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runBinary(binary string, dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func envWith(overrides ...string) []string {
	blocked := make(map[string]bool, len(overrides))
	for _, override := range overrides {
		if key, _, ok := strings.Cut(override, "="); ok {
			blocked[key] = true
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if ok && blocked[key] {
			continue
		}
		env = append(env, value)
	}
	return append(env, overrides...)
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return realpath
}
