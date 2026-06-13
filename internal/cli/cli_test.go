package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRunnerDispatchesStatePathNatively(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "path"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	want, err := state.PathResolver{StateHome: stateHome}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, filepath.Join(stateHome, "loaf", "projects")+string(filepath.Separator)) {
		t.Fatalf("stdout = %q, want state path under %q", got, stateHome)
	}
}

func TestRunnerHousekeepingUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-housekeeping.md", `---
title: Housekeeping Spec
status: complete
---
# Housekeeping Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-housekeeping.md", "# Housekeeping Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Housekeeping Task","spec":"SPEC-001","status":"done"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"housekeeping", "--json"})
	if err != nil {
		t.Fatalf("housekeeping --json error = %v", err)
	}
	summary := decodeHousekeepingSummary(t, jsonOut.Bytes())
	if summary.Sections["specs"].ByStatus["complete"] != 1 || summary.Sections["tasks"].ByStatus["done"] != 1 {
		t.Fatalf("summary = %#v, want SQLite spec/task lifecycle counts", summary)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"housekeeping", "--dry-run"})
	if err != nil {
		t.Fatalf("housekeeping --dry-run error = %v", err)
	}
	for _, want := range []string{"loaf housekeeping (SQLite state, dry run)", "database:", "specs", "tasks", "cleanup candidate"} {
		if !strings.Contains(humanOut.String(), want) {
			t.Fatalf("stdout = %q, want %q", humanOut.String(), want)
		}
	}
}

func TestRunnerHousekeepingUsesMarkdownArtifactsWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-done.md", `---
status: complete
---
# Done Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-done.md", `---
status: done
---
# Done Task
`)
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
status: active
---
# Active Session
`)
	writeCLIAgentsFile(t, workingDir, "sessions/archive/20260527-stopped.md", `---
status: stopped
---
# Archived Session
`)
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-absorbed.md", `---
status: absorbed
---
# Absorbed Draft
`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"housekeeping", "--json"})
	if err != nil {
		t.Fatalf("housekeeping markdown summary error = %v", err)
	}
	summary := decodeHousekeepingSummary(t, jsonOut.Bytes())
	if summary.DatabasePath != filepath.Join(workingDir, ".agents") {
		t.Fatalf("database path = %q, want markdown artifacts path", summary.DatabasePath)
	}
	if summary.Sections["specs"].ByStatus["complete"] != 1 || summary.Sections["tasks"].ByStatus["done"] != 1 || summary.Sections["sessions"].ByStatus["active"] != 1 || summary.Sections["sessions"].ByStatus["archived"] != 1 || summary.Sections["shaping_drafts"].ByStatus["absorbed"] != 1 {
		t.Fatalf("summary = %#v, want markdown artifact lifecycle counts", summary)
	}
	if summary.Sections["specs"].CleanupCandidate != 1 || summary.Sections["tasks"].CleanupCandidate != 1 || summary.Sections["sessions"].CleanupCandidate != 1 || summary.Sections["shaping_drafts"].CleanupCandidate != 1 {
		t.Fatalf("summary = %#v, want markdown cleanup candidates", summary)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"housekeeping", "--dry-run", "--sessions"})
	if err != nil {
		t.Fatalf("housekeeping markdown human summary error = %v", err)
	}
	for _, want := range []string{"loaf housekeeping (markdown, dry run)", "artifacts:", "sessions", "cleanup candidate"} {
		if !strings.Contains(humanOut.String(), want) {
			t.Fatalf("stdout = %q, want %q", humanOut.String(), want)
		}
	}
	if strings.Contains(humanOut.String(), "specs") {
		t.Fatalf("stdout = %q, want --sessions filter to hide specs", humanOut.String())
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerStateMigrateStorageHomeCopiesLegacyDatabase(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	legacyStatus, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}

	var dryRun bytes.Buffer
	err = Runner{
		Stdout:     &dryRun,
		WorkingDir: workingDir,
	}.Run([]string{"state", "migrate", "storage-home", "--json"})
	if err != nil {
		t.Fatalf("state migrate storage-home --json error = %v", err)
	}
	var preview state.StorageHomeMigrationPlan
	if err := json.Unmarshal(dryRun.Bytes(), &preview); err != nil {
		t.Fatalf("Unmarshal(preview) error = %v\n%s", err, dryRun.String())
	}
	if preview.Action != state.StorageHomeActionCopy || preview.Applied {
		t.Fatalf("preview = %#v, want copy dry-run", preview)
	}

	var applyOut bytes.Buffer
	err = Runner{
		Stdout:     &applyOut,
		WorkingDir: workingDir,
	}.Run([]string{"state", "migrate", "storage-home", "--apply"})
	if err != nil {
		t.Fatalf("state migrate storage-home --apply error = %v", err)
	}
	for _, want := range []string{"loaf state migrate storage-home --apply", "action: already-migrated", "applied: true"} {
		if !strings.Contains(applyOut.String(), want) {
			t.Fatalf("stdout = %q, want %q", applyOut.String(), want)
		}
	}
	if _, err := os.Stat(legacyStatus.DatabasePath); err != nil {
		t.Fatalf("legacy database stat error = %v, want legacy preserved", err)
	}

	var statusOut bytes.Buffer
	err = Runner{
		Stdout:     &statusOut,
		WorkingDir: workingDir,
	}.Run([]string{"state", "status", "--json"})
	if err != nil {
		t.Fatalf("state status --json error = %v", err)
	}
	status := decodeStateStatus(t, statusOut.Bytes())
	if status.Mode != state.ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, state.ModeSQLiteReady)
	}
	if !strings.HasPrefix(status.DatabasePath, dataHome+string(filepath.Separator)) {
		t.Fatalf("DatabasePath = %q, want under data home %q", status.DatabasePath, dataHome)
	}
}

func TestRunnerMigrateStorageHomeUsesNativeAlias(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"migrate", "storage-home"})
	if err != nil {
		t.Fatalf("migrate storage-home error = %v", err)
	}
	for _, want := range []string{"loaf migrate storage-home --dry-run", "action: copy"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerMigrateMarkdownUsesNativeAlias(t *testing.T) {
	repo := initCLIGitRepo(t)
	mkdirAll(t, filepath.Join(repo, ".agents", "tasks"))
	writeFile(t, filepath.Join(repo, ".agents", "tasks", "TASK-001-demo.md"), strings.Join([]string{
		"---",
		"id: TASK-001",
		"title: Demo task",
		"status: todo",
		"---",
		"# Demo task",
	}, "\n"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"migrate", "markdown"})
	if err != nil {
		t.Fatalf("migrate markdown error = %v", err)
	}
	for _, want := range []string{"loaf migrate markdown --dry-run", "tasks: 1"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := state.PathResolver{}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("database stat error = %v, want dry-run to avoid creating SQLite database", err)
	}
}

func TestRunnerMigrateWorktreeStorageMainCheckoutNoopsNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"migrate", "worktree-storage"})
	if err != nil {
		t.Fatalf("migrate worktree-storage main checkout error = %v", err)
	}
	if !strings.Contains(stdout.String(), "already in the main worktree") {
		t.Fatalf("stdout = %q, want main checkout no-op", stdout.String())
	}
}

func TestRunnerMigrateWorktreeStorageRejectsNonGitNatively(t *testing.T) {
	repo := realpath(t, t.TempDir())
	err := Runner{
		WorkingDir: repo,
	}.Run([]string{"migrate", "worktree-storage"})
	if err == nil || !strings.Contains(err.Error(), "not in a git repository") {
		t.Fatalf("migrate worktree-storage non-git error = %v, want git-context rejection", err)
	}
}

func TestRunnerMigrateWorktreeStorageDryRunAndApplyNatively(t *testing.T) {
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "worktree-storage")
	files := seedCLIWorktreeAgents(t, linked)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: linked,
	}.Run([]string{"migrate", "worktree-storage"})
	if err != nil {
		t.Fatalf("migrate worktree-storage dry-run error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "loaf migrate worktree-storage (dry-run)") || !strings.Contains(output, "Dry run") {
		t.Fatalf("stdout = %q, want dry-run plan", output)
	}
	for _, rel := range files {
		if _, err := os.Stat(filepath.Join(linked, ".agents", filepath.FromSlash(rel))); err != nil {
			t.Fatalf("dry-run removed %s: %v", rel, err)
		}
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: linked,
	}.Run([]string{"migrate", "worktree-storage", "--apply"})
	if err != nil {
		t.Fatalf("migrate worktree-storage apply error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Migrated") {
		t.Fatalf("stdout = %q, want applied summary", stdout.String())
	}
	for _, rel := range files {
		if _, err := os.Stat(filepath.Join(main, ".agents", filepath.FromSlash(rel))); err != nil {
			t.Fatalf("main .agents missing migrated %s: %v", rel, err)
		}
		if _, err := os.Stat(filepath.Join(linked, ".agents", filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("worktree .agents %s stat = %v, want moved away", rel, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(linked, ".agents", ".moved-to"))
	if err != nil {
		t.Fatalf("ReadFile(.moved-to) error = %v", err)
	}
	if string(raw) != main+"\n" {
		t.Fatalf(".moved-to = %q, want %q", raw, main+"\n")
	}
}

func TestRunnerMigrateWorktreeStorageFlagConflictNatively(t *testing.T) {
	repo := initCLIGitRepo(t)
	err := Runner{
		WorkingDir: repo,
	}.Run([]string{"migrate", "worktree-storage", "--force-from-worktree", "--force-from-main"})
	if err == nil || !strings.Contains(err.Error(), "--force-from-worktree") || !strings.Contains(err.Error(), "--force-from-main") {
		t.Fatalf("migrate worktree-storage flag conflict error = %v, want both flags named", err)
	}
}

func TestRunnerMigrateWorktreeStorageConflictPoliciesNatively(t *testing.T) {
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "worktree-storage-conflict")
	mkdirAll(t, filepath.Join(main, ".agents"))
	mkdirAll(t, filepath.Join(linked, ".agents"))
	writeFile(t, filepath.Join(main, ".agents", "AGENTS.md"), "# from main\n")
	writeFile(t, filepath.Join(linked, ".agents", "AGENTS.md"), "# from worktree\n")

	err := Runner{
		WorkingDir: linked,
	}.Run([]string{"migrate", "worktree-storage", "--apply", "--force-from-main"})
	if err != nil {
		t.Fatalf("migrate worktree-storage force-main error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(main, ".agents", "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(main AGENTS.md) error = %v", err)
	}
	if string(body) != "# from main\n" {
		t.Fatalf("main AGENTS.md = %q, want main copy kept", body)
	}
	if _, err := os.Stat(filepath.Join(linked, ".agents", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("worktree conflict loser stat = %v, want removed", err)
	}
}

func TestRunnerMigrateWorktreeStorageRefusesSymlinksNatively(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture is platform-sensitive on Windows")
	}
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "worktree-storage-symlink")
	mkdirAll(t, filepath.Join(linked, ".agents"))
	writeFile(t, filepath.Join(linked, "target.md"), "target\n")
	if err := os.Symlink("../target.md", filepath.Join(linked, ".agents", "linked.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := Runner{
		WorkingDir: linked,
	}.Run([]string{"migrate", "worktree-storage"})
	if err == nil || !strings.Contains(err.Error(), "symlinks") || !strings.Contains(err.Error(), "linked.md") {
		t.Fatalf("migrate worktree-storage symlink error = %v, want symlink refusal", err)
	}
}

func TestRunnerMigrateWorktreeStorageRefusesPartialLeftoversNatively(t *testing.T) {
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "worktree-storage-partial")
	seedCLIWorktreeAgents(t, linked)
	partial := filepath.Join(main, ".agents", "kb"+worktreePartialSuffix)
	mkdirAll(t, filepath.Dir(partial))
	writeFile(t, partial, "partial\n")

	err := Runner{
		WorkingDir: linked,
	}.Run([]string{"migrate", "worktree-storage", "--apply"})
	if err == nil || !strings.Contains(err.Error(), worktreePartialSuffix) || !strings.Contains(err.Error(), partial) {
		t.Fatalf("migrate worktree-storage partial error = %v, want partial refusal", err)
	}
}

func TestRunnerRefusesPreA3LinkedWorktreeBeforeDispatch(t *testing.T) {
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "pre-a3-refusal")
	seedCLIWorktreeAgents(t, linked)
	var stderr bytes.Buffer

	err := Runner{
		Stderr:     &stderr,
		WorkingDir: linked,
	}.Run([]string{"session", "list"})

	exitErr, ok := err.(interface {
		ExitCode() int
		Silent() bool
	})
	if !ok || exitErr.ExitCode() != 2 || !exitErr.Silent() {
		t.Fatalf("Run() error = %#v, want silent exit code 2", err)
	}
	for _, want := range []string{"SPEC-036", "loaf migrate worktree-storage", "LOAF_DEBUG_RESOLVE"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunnerRefusesPreA3LinkedWorktreeWithUnknownCommandFeedback(t *testing.T) {
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "pre-a3-unknown")
	seedCLIWorktreeAgents(t, linked)
	var stderr bytes.Buffer

	err := Runner{
		Stderr:     &stderr,
		WorkingDir: linked,
	}.Run([]string{"not-a-command"})

	exitErr, ok := err.(interface {
		ExitCode() int
		Silent() bool
	})
	if !ok || exitErr.ExitCode() != 2 || !exitErr.Silent() {
		t.Fatalf("Run() error = %#v, want silent exit code 2", err)
	}
	for _, want := range []string{"unknown command 'not-a-command'", "SPEC-036", "loaf migrate worktree-storage"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunnerInitScaffoldsProjectNatively(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(workingDir, "go.mod"), "module example.test/init\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		Stdin:      bytes.NewBuffer(nil),
	}.Run([]string{"init", "--no-symlinks"})
	if err != nil {
		t.Fatalf("init --no-symlinks error = %v", err)
	}
	for _, path := range []string{
		".agents/AGENTS.md",
		".agents/loaf.json",
		".agents/sessions",
		".agents/ideas",
		".agents/handoffs",
		".agents/specs",
		".agents/tasks",
		"docs/VISION.md",
		"docs/STRATEGY.md",
		"docs/ARCHITECTURE.md",
		"docs/knowledge",
		"docs/decisions",
		"CHANGELOG.md",
	} {
		if _, err := os.Stat(filepath.Join(workingDir, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected init path %s: %v", path, err)
		}
	}
	var config struct {
		Version     string `json:"version"`
		Initialized string `json:"initialized"`
	}
	body, err := os.ReadFile(filepath.Join(workingDir, ".agents", "loaf.json"))
	if err != nil {
		t.Fatalf("ReadFile(loaf.json) error = %v", err)
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("loaf.json is not valid JSON: %v\n%s", err, body)
	}
	if config.Version != "1.0.0" || config.Initialized == "" {
		t.Fatalf("loaf.json = %#v, want version and initialized timestamp", config)
	}
	output := stdout.String()
	for _, want := range []string{"loaf init", "Go (go.mod)", "go-development", "Project initialized"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(workingDir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md symlink stat = %v, want absent with --no-symlinks", err)
	}
}

func TestRunnerInitIsIdempotentAndPreservesExistingFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(workingDir, ".agents"))
	writeFile(t, filepath.Join(workingDir, ".agents", "AGENTS.md"), "# Custom Instructions\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		Stdin:      bytes.NewBuffer(nil),
	}.Run([]string{"init", "--no-symlinks"})
	if err != nil {
		t.Fatalf("first init error = %v", err)
	}
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		Stdin:      bytes.NewBuffer(nil),
	}.Run([]string{"init", "--no-symlinks"})
	if err != nil {
		t.Fatalf("second init error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(workingDir, ".agents", "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if string(body) != "# Custom Instructions\n" {
		t.Fatalf("AGENTS.md = %q, want custom content preserved", body)
	}
	if !strings.Contains(stdout.String(), "Nothing to create") {
		t.Fatalf("stdout = %q, want idempotent no-op message", stdout.String())
	}
}

func TestRunnerInitDetectsTypeScriptReactNatively(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(workingDir, "package.json"), `{"dependencies":{"react":"latest","typescript":"latest"}}`+"\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		Stdin:      bytes.NewBuffer(nil),
	}.Run([]string{"init", "--no-symlinks"})
	if err != nil {
		t.Fatalf("init TypeScript React error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"TypeScript", "React", "typescript-development", "interface-design"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunnerInitHelpIsNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"init", "--help"})
	if err != nil {
		t.Fatalf("init --help error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: loaf init") || !strings.Contains(stdout.String(), "--no-symlinks") {
		t.Fatalf("stdout = %q, want native init help", stdout.String())
	}
}

func TestRunnerInitRejectsUnknownOptionsNatively(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	err := Runner{
		WorkingDir: workingDir,
	}.Run([]string{"init", "--wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown init option") {
		t.Fatalf("init unknown option error = %v, want native option error", err)
	}
}

func TestRunnerHousekeepingReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"housekeeping"})
	if err == nil {
		t.Fatal("housekeeping invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func assertSQLiteRequired(t *testing.T, args ...string) {
	t.Helper()
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: realpath(t, t.TempDir()),
		StateHome:  t.TempDir(),
	}.Run(args)
	if err == nil {
		t.Fatalf("Run(%v) error = nil, want SQLite state required error", args)
	}
	if !strings.Contains(err.Error(), "requires initialized SQLite state") {
		t.Fatalf("Run(%v) error = %v, want SQLite state required error", args, err)
	}
}

func TestRunnerTaskRefreshUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-refresh.md", "# Refresh Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-refresh.md", "# Refresh Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Refresh Task","spec":"SPEC-001","status":"todo"}}}`)
	before := readCLIAgentsFile(t, workingDir, "TASKS.json")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "refresh", "--json"})
	if err != nil {
		t.Fatalf("task refresh --json error = %v", err)
	}
	summary := decodeCompatibilityCommandSummary(t, stdout.Bytes())
	if summary.Command != "task refresh" || summary.Action != "read" || summary.Counts["tasks"] != 1 || summary.Counts["specs"] != 1 {
		t.Fatalf("summary = %#v, want SQLite task/spec counts", summary)
	}
	if after := readCLIAgentsFile(t, workingDir, "TASKS.json"); after != before {
		t.Fatalf("TASKS.json changed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestRunnerTaskSyncUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-sync.md", "# Sync Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-sync.md", "# Sync Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Sync Task","spec":"SPEC-001","status":"todo"}}}`)
	before := readCLIAgentsFile(t, workingDir, "TASKS.json")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "sync", "--push", "--json"})
	if err != nil {
		t.Fatalf("task sync --push --json error = %v", err)
	}
	summary := decodeCompatibilityCommandSummary(t, stdout.Bytes())
	if summary.Command != "task sync" || summary.Action != "skipped" || !strings.Contains(summary.Reason, "compatibility repair") || summary.Counts["tasks"] != 1 {
		t.Fatalf("summary = %#v, want skipped SQLite compatibility summary", summary)
	}
	if after := readCLIAgentsFile(t, workingDir, "TASKS.json"); after != before {
		t.Fatalf("TASKS.json changed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestRunnerTaskRefreshUsesMarkdownFilesWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-refresh.md", `---
id: TASK-001
title: Refresh Task
status: in-progress
priority: p1
spec: SPEC-001
depends_on: [TASK-000]
files: [internal/cli/cli.go]
verify: go test ./...
done: Refresh works
session: 20260611-refresh
created: 2026-06-01
updated: 2026-06-02T12:00:00Z
---
# Refresh body
`)
	writeCLIAgentsFile(t, workingDir, "tasks/archive/2026-06/TASK-002-archived.md", `---
title: Archived Task
status: archived
created: 2026-06-03T10:00:00Z
---
# Archived body
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-refresh.md", `---
id: SPEC-001
title: Refresh Spec
status: in-progress
source: direct
requirement: Rebuild from files
created: 2026-06-01
---
# Spec body
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 99,
  "tasks": {
    "TASK-042": {"title": "Stale Task", "status": "todo", "priority": "P2"}
  },
  "specs": {},
  "custom_root": "drop on rebuild"
}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "refresh"})
	if err != nil {
		t.Fatalf("task refresh markdown error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "Rebuilt TASKS.json") || !strings.Contains(output, "Tasks: 2") || !strings.Contains(output, "Specs: 1") {
		t.Fatalf("stdout = %q, want refresh summary", output)
	}

	var index map[string]any
	rawIndex, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json) error = %v", err)
	}
	if _, ok := index["custom_root"]; ok {
		t.Fatalf("custom_root preserved on rebuild: %#v", index)
	}
	if int(index["next_id"].(float64)) != 99 {
		t.Fatalf("next_id = %#v, want monotonic 99", index["next_id"])
	}
	tasks := index["tasks"].(map[string]any)
	if _, ok := tasks["TASK-042"]; ok {
		t.Fatalf("tasks = %#v, want stale TASK-042 dropped", tasks)
	}
	task := tasks["TASK-001"].(map[string]any)
	if task["title"] != "Refresh Task" || task["status"] != "in_progress" || task["priority"] != "P1" || task["spec"] != "SPEC-001" || task["file"] != "TASK-001-refresh.md" || task["verify"] != "go test ./..." || task["done"] != "Refresh works" || task["session"] != "20260611-refresh" {
		t.Fatalf("TASK-001 = %#v, want normalized task metadata", task)
	}
	if task["created"] != "2026-06-01T00:00:00Z" || task["updated"] != "2026-06-02T12:00:00Z" {
		t.Fatalf("TASK-001 dates = %v/%v, want normalized dates", task["created"], task["updated"])
	}
	deps := task["depends_on"].([]any)
	if len(deps) != 1 || deps[0] != "TASK-000" {
		t.Fatalf("depends_on = %#v, want TASK-000", deps)
	}
	files := task["files"].([]any)
	if len(files) != 1 || files[0] != "internal/cli/cli.go" {
		t.Fatalf("files = %#v, want source file hint", files)
	}
	archived := tasks["TASK-002"].(map[string]any)
	if archived["status"] != "done" || archived["file"] != "archive/2026-06/TASK-002-archived.md" || archived["completed_at"] == nil {
		t.Fatalf("TASK-002 = %#v, want archived file normalized to done", archived)
	}
	spec := index["specs"].(map[string]any)["SPEC-001"].(map[string]any)
	if spec["title"] != "Refresh Spec" || spec["status"] != "implementing" || spec["source"] != "direct" || spec["requirement"] != "Rebuild from files" || spec["created"] != "2026-06-01T00:00:00Z" || spec["file"] != "SPEC-001-refresh.md" {
		t.Fatalf("SPEC-001 = %#v, want normalized spec metadata", spec)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestMarkdownTaskRefreshPreservesScanWindowEntries(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-seeded.md", `---
id: TASK-001
title: Seeded
status: todo
priority: P2
---
# Seeded
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 11,
  "tasks": {
    "TASK-001": {"title": "Seeded", "status": "todo", "priority": "P2", "file": "TASK-001-seeded.md"},
    "TASK-011": {"title": "Fresh In Window", "status": "todo", "priority": "P2", "file": "TASK-011-fresh.md"}
  },
  "specs": {}
}`)

	if _, err := markdownTaskRefresh(workingDir); err != nil {
		t.Fatalf("markdownTaskRefresh error = %v", err)
	}
	index := readMarkdownTaskIndexForTest(t, workingDir)
	tasks := index["tasks"].(map[string]any)
	if _, ok := tasks["TASK-011"]; !ok {
		t.Fatalf("tasks = %#v, want TASK-011 preserved as scan-window entry", tasks)
	}
	if int(index["next_id"].(float64)) < 12 {
		t.Fatalf("next_id = %#v, want at least 12", index["next_id"])
	}
}

func TestMarkdownTaskRefreshDropsPreScanMissingEntries(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-seeded.md", `---
id: TASK-001
title: Seeded
status: todo
priority: P2
---
# Seeded
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 12,
  "tasks": {
    "TASK-001": {"title": "Seeded", "status": "todo", "priority": "P2", "file": "TASK-001-seeded.md"},
    "TASK-011": {"title": "Pre Scan Missing", "status": "todo", "priority": "P2", "file": "TASK-011-missing.md"},
    "TASK-ABC": {"title": "Malformed", "status": "todo", "priority": "P2"}
  },
  "specs": {
    "SPEC-001": {"title": "Missing Spec", "status": "drafting"}
  }
}`)

	if _, err := markdownTaskRefresh(workingDir); err != nil {
		t.Fatalf("markdownTaskRefresh error = %v", err)
	}
	index := readMarkdownTaskIndexForTest(t, workingDir)
	tasks := index["tasks"].(map[string]any)
	if _, ok := tasks["TASK-011"]; ok {
		t.Fatalf("tasks = %#v, want pre-scan missing TASK-011 dropped", tasks)
	}
	if _, ok := tasks["TASK-ABC"]; ok {
		t.Fatalf("tasks = %#v, want malformed index-only task dropped", tasks)
	}
	specs := index["specs"].(map[string]any)
	if _, ok := specs["SPEC-001"]; ok {
		t.Fatalf("specs = %#v, want pre-scan missing SPEC-001 dropped", specs)
	}
}

func TestMarkdownTaskRefreshPreservesScanWindowSpecsAndMonotonicNextID(t *testing.T) {
	scanned := map[string]any{
		"version": float64(1),
		"next_id": float64(3),
		"tasks": map[string]any{
			"TASK-002": map[string]any{"title": "Scanned"},
		},
		"specs": map[string]any{},
	}
	now := map[string]any{
		"version": float64(1),
		"next_id": float64(200),
		"tasks": map[string]any{
			"TASK-150": map[string]any{"title": "Fresh"},
		},
		"specs": map[string]any{
			"SPEC-010": map[string]any{"title": "Fresh Spec"},
		},
	}
	merged := mergeMarkdownTaskRefreshIndex(scanned, now, 100, map[string]any{"SPEC-001": map[string]any{"title": "Old"}})

	tasks := merged["tasks"].(map[string]any)
	if _, ok := tasks["TASK-150"]; !ok {
		t.Fatalf("tasks = %#v, want fresh TASK-150 preserved", tasks)
	}
	specs := merged["specs"].(map[string]any)
	if _, ok := specs["SPEC-010"]; !ok {
		t.Fatalf("specs = %#v, want fresh SPEC-010 preserved", specs)
	}
	if int(merged["next_id"].(float64)) != 200 {
		t.Fatalf("next_id = %#v, want monotonic 200", merged["next_id"])
	}
}

func readMarkdownTaskIndexForTest(t *testing.T, workingDir string) map[string]any {
	t.Helper()
	rawIndex, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	var index map[string]any
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json) error = %v", err)
	}
	return index
}

func TestRunnerTaskSyncPushUsesMarkdownIndexWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-sync.md", `---
id: TASK-001
title: Old Title
status: todo
priority: P3
spec: SPEC-OLD
depends_on: [TASK-OLD]
custom_task: keep me
---
# Task Body

Preserve task body.
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-sync.md", `---
id: SPEC-001
title: Old Spec
status: drafting
custom_spec: keep me too
---
# Spec Body

Preserve spec body.
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 2,
  "tasks": {
    "TASK-001": {
      "title": "Synced Task",
      "slug": "sync",
      "spec": "SPEC-001",
      "status": "done",
      "priority": "P0",
      "depends_on": ["TASK-000"],
      "files": ["go.mod"],
      "verify": "go test ./...",
      "done": "All green",
      "session": "20260611-sync",
      "created": "2026-06-01T10:00:00Z",
      "updated": "2026-06-02T11:00:00Z",
      "completed_at": "2026-06-03T12:00:00Z",
      "file": "TASK-001-sync.md"
    }
  },
  "specs": {
    "SPEC-001": {
      "title": "Synced Spec",
      "status": "approved",
      "source": "direct",
      "requirement": "Push frontmatter",
      "created": "2026-06-01T09:00:00Z",
      "file": "SPEC-001-sync.md"
    }
  }
}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "sync", "--push"})
	if err != nil {
		t.Fatalf("task sync --push markdown error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "Pushed TASKS.json metadata") || !strings.Contains(output, "Tasks: 1, Specs: 1") {
		t.Fatalf("stdout = %q, want push summary", output)
	}

	taskBody, err := os.ReadFile(filepath.Join(workingDir, ".agents", "tasks", "TASK-001-sync.md"))
	if err != nil {
		t.Fatalf("ReadFile(task) error = %v", err)
	}
	taskFrontmatter, ok := parseKnowledgeFrontmatter(taskBody)
	if !ok {
		t.Fatal("task frontmatter missing")
	}
	for key, want := range map[string]string{
		"title":        "Synced Task",
		"status":       "done",
		"priority":     "P0",
		"spec":         "SPEC-001",
		"verify":       "go test ./...",
		"done":         "All green",
		"session":      "20260611-sync",
		"completed_at": "2026-06-03T12:00:00Z",
		"custom_task":  "keep me",
	} {
		if got := firstFieldValue(taskFrontmatter[key]); got != want {
			t.Fatalf("task frontmatter[%s] = %q, want %q; all=%#v", key, got, want, taskFrontmatter)
		}
	}
	if strings.Join(taskFrontmatter["depends_on"].Values, ",") != "TASK-000" || strings.Join(taskFrontmatter["files"].Values, ",") != "go.mod" {
		t.Fatalf("task frontmatter arrays = depends_on:%#v files:%#v", taskFrontmatter["depends_on"], taskFrontmatter["files"])
	}
	if !strings.Contains(markdownContentWithoutFrontmatter(string(taskBody)), "Preserve task body.") {
		t.Fatalf("task body = %q, want preserved body", markdownContentWithoutFrontmatter(string(taskBody)))
	}

	specBody, err := os.ReadFile(filepath.Join(workingDir, ".agents", "specs", "SPEC-001-sync.md"))
	if err != nil {
		t.Fatalf("ReadFile(spec) error = %v", err)
	}
	specFrontmatter, ok := parseKnowledgeFrontmatter(specBody)
	if !ok {
		t.Fatal("spec frontmatter missing")
	}
	for key, want := range map[string]string{
		"title":       "Synced Spec",
		"status":      "approved",
		"source":      "direct",
		"requirement": "Push frontmatter",
		"custom_spec": "keep me too",
	} {
		if got := firstFieldValue(specFrontmatter[key]); got != want {
			t.Fatalf("spec frontmatter[%s] = %q, want %q; all=%#v", key, got, want, specFrontmatter)
		}
	}
	if !strings.Contains(markdownContentWithoutFrontmatter(string(specBody)), "Preserve spec body.") {
		t.Fatalf("spec body = %q, want preserved body", markdownContentWithoutFrontmatter(string(specBody)))
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskSyncImportUsesMarkdownFilesWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-existing.md", "# Existing\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-005-orphan.md", `---
title: Orphan Task
status: review
priority: P1
created: 2026-06-05T10:00:00Z
---
# Orphan
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-orphan.md", `---
title: Orphan Spec
status: approved
created: 2026-06-05T11:00:00Z
---
# Orphan Spec
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 2,
  "tasks": {
    "TASK-001": {
      "title": "Existing Task",
      "status": "todo",
      "priority": "P2",
      "file": "TASK-001-existing.md",
      "review_notes": "preserve me"
    }
  },
  "specs": {},
  "custom_root": "preserve me"
}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "sync", "--import"})
	if err != nil {
		t.Fatalf("task sync --import markdown error = %v", err)
	}
	if output := stdout.String(); !strings.Contains(output, "Imported 1 task(s) and 1 spec(s)") {
		t.Fatalf("stdout = %q, want import summary", output)
	}

	var index map[string]any
	rawIndex, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json) error = %v", err)
	}
	if index["custom_root"] != "preserve me" {
		t.Fatalf("custom_root = %#v, want preserved", index["custom_root"])
	}
	if int(index["next_id"].(float64)) != 6 {
		t.Fatalf("next_id = %#v, want 6", index["next_id"])
	}
	tasks := index["tasks"].(map[string]any)
	existing := tasks["TASK-001"].(map[string]any)
	if existing["review_notes"] != "preserve me" {
		t.Fatalf("existing task = %#v, want preserved unknown field", existing)
	}
	orphan := tasks["TASK-005"].(map[string]any)
	if orphan["title"] != "Orphan Task" || orphan["status"] != "review" || orphan["priority"] != "P1" || orphan["file"] != "TASK-005-orphan.md" {
		t.Fatalf("TASK-005 = %#v, want imported orphan task", orphan)
	}
	spec := index["specs"].(map[string]any)["SPEC-002"].(map[string]any)
	if spec["title"] != "Orphan Spec" || spec["status"] != "approved" || spec["file"] != "SPEC-002-orphan.md" {
		t.Fatalf("SPEC-002 = %#v, want imported orphan spec", spec)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskRefreshAndSyncReportInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeInvalidStateDatabase(t, workingDir, stateHome)

	for _, args := range [][]string{
		{"task", "refresh"},
		{"task", "sync"},
	} {
		err := Runner{
			Stdout:     &bytes.Buffer{},
			WorkingDir: workingDir,
			StateHome:  stateHome,
		}.Run(args)
		if err == nil {
			t.Fatalf("%v invalid state error = nil, want error", args)
		}
		if !strings.Contains(err.Error(), "state database is invalid") {
			t.Fatalf("%v error = %v, want invalid state error", args, err)
		}
	}
}

func TestRunnerSessionEnrichUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
status: done
branch: main
claude_session_id: abc123
---
# Session
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "enrich", "20260528-session.md", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("session enrich --json error = %v", err)
	}
	summary := decodeCompatibilityCommandSummary(t, stdout.Bytes())
	if summary.Command != "session enrich" || summary.Action != "skipped" || summary.Counts["sessions"] != 1 {
		t.Fatalf("summary = %#v, want skipped SQLite session enrich summary", summary)
	}
}

func TestRunnerSessionEnrichSummarizesMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
status: done
branch: main
claude_session_id: markdown-enrich-session
---
# Session
`)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "enrich", "20260528-session.md", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("session enrich markdown summary error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=session enrich") {
		t.Fatalf("stdout = %q, want native markdown enrich without legacy delegation", stdout.String())
	}
	summary := decodeCompatibilityCommandSummary(t, stdout.Bytes())
	if summary.Command != "session enrich" || summary.Mode != "markdown" || summary.Action != "skipped" || summary.Counts["sessions"] != 1 || summary.Counts["done"] != 1 {
		t.Fatalf("summary = %#v, want markdown enrich compatibility counts", summary)
	}
	if !strings.Contains(summary.Reason, "TypeScript bridge") {
		t.Fatalf("reason = %q, want explicit bridge-removal explanation", summary.Reason)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSessionEnrichReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeInvalidStateDatabase(t, workingDir, stateHome)

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "enrich"})
	if err == nil {
		t.Fatal("session enrich invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSessionHousekeepingUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
status: active
branch: main
claude_session_id: abc123
---
# Session
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	before := readCLIAgentsFile(t, workingDir, "sessions/20260528-session.md")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "housekeeping", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("session housekeeping --dry-run --json error = %v", err)
	}
	summary := decodeCompatibilityCommandSummary(t, stdout.Bytes())
	if summary.Command != "session housekeeping" || summary.Action != "skipped" || summary.Mode != "sqlite" || summary.Counts["sessions"] != 1 {
		t.Fatalf("summary = %#v, want skipped SQLite session housekeeping summary", summary)
	}
	if !strings.Contains(summary.Reason, "markdown session housekeeping") {
		t.Fatalf("reason = %q, want markdown compatibility explanation", summary.Reason)
	}
	if after := readCLIAgentsFile(t, workingDir, "sessions/20260528-session.md"); after != before {
		t.Fatalf("session markdown changed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestRunnerSessionHousekeepingSummarizesMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
status: active
branch: main
claude_session_id: markdown-housekeeping-active
---
# Session
`)
	writeCLIAgentsFile(t, workingDir, "sessions/archive/20260527-archived.md", `---
status: active
branch: old/session
claude_session_id: markdown-housekeeping-archived
---
# Session
`)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "housekeeping", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("session housekeeping markdown summary error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=session housekeeping") {
		t.Fatalf("stdout = %q, want native markdown housekeeping without legacy delegation", stdout.String())
	}
	summary := decodeCompatibilityCommandSummary(t, stdout.Bytes())
	if summary.Command != "session housekeeping" || summary.Mode != "markdown" || summary.Action != "skipped" || summary.Counts["sessions"] != 2 || summary.Counts["active"] != 1 || summary.Counts["archived"] != 1 {
		t.Fatalf("summary = %#v, want markdown housekeeping compatibility counts", summary)
	}
	if !strings.Contains(summary.Reason, "Native markdown session lifecycle") {
		t.Fatalf("reason = %q, want native lifecycle explanation", summary.Reason)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSessionHousekeepingReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeInvalidStateDatabase(t, workingDir, stateHome)

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "housekeeping"})
	if err == nil {
		t.Fatal("session housekeeping invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSessionStateUpdateWritesSQLiteSnapshot(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
status: active
branch: main
claude_session_id: abc123
---
# Session

## Journal

[2026-05-28 10:00] session(start):  === SESSION STARTED ===
`)
	markdownBefore := readCLIAgentsFile(t, workingDir, "sessions/20260528-session.md")
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "sqlite-state-session",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDir, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(dirty.txt) error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"sqlite-state-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "state", "update"})
	if err != nil {
		t.Fatalf("session state update error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want silent SQLite hook no-op", stdout.String())
	}
	if markdownAfter := readCLIAgentsFile(t, workingDir, "sessions/20260528-session.md"); markdownAfter != markdownBefore {
		t.Fatalf("session markdown changed:\nbefore=%s\nafter=%s", markdownBefore, markdownAfter)
	}
	after, err := state.ShowSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, "20260528-session")
	if err == nil {
		t.Fatalf("ShowSession(legacy markdown alias) error = nil, want SQLite state not to mutate markdown session")
	}
	after, err = state.ShowSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession(SQLite) error = %v", err)
	}
	if after.Session.StateSnapshot == nil {
		t.Fatalf("StateSnapshot = nil, want SQLite state update snapshot")
	}
	for _, want := range []string{"## Current State (", "Branch: main", "Last commit:", "Uncommitted:"} {
		if !strings.Contains(after.Session.StateSnapshot.Content, want) {
			t.Fatalf("snapshot content = %q, want %q", after.Session.StateSnapshot.Content, want)
		}
	}
}

func TestRunnerSessionStateUpdateUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-state-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
---
# Session

## Current State (2026-06-10 10:00)

old state text

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)
	if err := os.WriteFile(filepath.Join(workingDir, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(dirty.txt) error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"markdown-state-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "state", "update"})
	if err != nil {
		t.Fatalf("session state update markdown error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want silent hook update", stdout.String())
	}
	after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")
	for _, want := range []string{
		"## Current State (",
		"Branch: main",
		"Last commit:",
		"Uncommitted:",
		"## Journal",
		"session(start):  === SESSION STARTED ===",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("session markdown = %q, want %q", after, want)
		}
	}
	for _, notWant := range []string{
		"old state text",
		"last_updated: 2026-06-10T10:00:00Z",
	} {
		if strings.Contains(after, notWant) {
			t.Fatalf("session markdown = %q, did not want %q", after, notWant)
		}
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown state update not to create SQLite database", err)
	}
}

func TestRunnerSessionStateUpdateSkipsMarkdownSubagents(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-state-session
---
# Session

## Journal
`)
	before := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"agent_id":"subagent","session_id":"markdown-state-session"}`),
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
	}.Run([]string{"session", "state", "update"})
	if err != nil {
		t.Fatalf("session state update markdown subagent error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want subagent skip", stdout.String())
	}
	if after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md"); after != before {
		t.Fatalf("session markdown changed for subagent:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestRunnerSessionStateUpdateReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeInvalidStateDatabase(t, workingDir, stateHome)

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "state", "update"})
	if err == nil {
		t.Fatal("session state update invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSessionContextForPromptIsNativeAndSkipsSubagents(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-prompt"})
	if err != nil {
		t.Fatalf("session context for-prompt error = %v", err)
	}
	for _, want := range []string{"[Implementation Principles]", "When the user's message is a QUESTION", "loaf session log"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}

	stdout.Reset()
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"agent_id":"agent-123"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "--for-prompt"})
	if err != nil {
		t.Fatalf("session context --for-prompt subagent error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want subagent prompt skip", stdout.String())
	}
}

func TestRunnerSessionContextForCompactLogsSQLiteSessionMarker(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "compact-session",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"compact-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-compact"})
	if err != nil {
		t.Fatalf("session context for-compact error = %v", err)
	}
	if !strings.Contains(stdout.String(), "CONTEXT COMPACTION IMMINENT") || !strings.Contains(stdout.String(), "loaf session log") {
		t.Fatalf("stdout = %q, want compact instructions", stdout.String())
	}

	show, err := state.ShowSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if !hasSessionJournalEntry(show.Session.JournalEntries, "compact", "session", "context compaction triggered") {
		t.Fatalf("journal entries = %#v, want compact marker", show.Session.JournalEntries)
	}
}

func TestRunnerSessionContextForResumptionUsesSQLiteState(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "resume-session",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := state.LogJournal(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.JournalLogOptions{
		Entry:            "decision(sqlite): render resumption from state",
		ObservedBranch:   "main",
		ObservedWorktree: workingDir,
		HarnessSessionID: "resume-session",
		LinkSession:      true,
	}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"resume-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("session context for-resumption error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"=== POST-COMPACTION RESUMPTION ===", "Session: " + start.Session.Alias, "Branch: main", "WARNING: No SQLite session state snapshot was written before compaction.", "## Recent Journal", "decision(sqlite): render resumption from state"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunnerSessionContextForResumptionUsesSQLiteStateSnapshot(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "snapshot-resume-session",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := state.RecordSessionStateSnapshot(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStateSnapshotOptions{
		SessionRef: start.Session.Alias,
		Content:    "## Current State (2026-06-11 12:55)\n\n**Working on:** SQLite resumption snapshots\n**Status:** stored in native state",
	}); err != nil {
		t.Fatalf("RecordSessionStateSnapshot() error = %v", err)
	}
	if _, err := state.LogJournal(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.JournalLogOptions{
		Entry:            "decision(sqlite): render snapshot before journal",
		ObservedBranch:   "main",
		ObservedWorktree: workingDir,
		HarnessSessionID: "snapshot-resume-session",
		LinkSession:      true,
	}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"snapshot-resume-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("session context for-resumption error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"=== POST-COMPACTION RESUMPTION ===",
		"Session: " + start.Session.Alias,
		"## Current State (2026-06-11 12:55)",
		"**Working on:** SQLite resumption snapshots",
		"## Recent Journal",
		"decision(sqlite): render snapshot before journal",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "No SQLite session state snapshot") {
		t.Fatalf("stdout = %q, did not want missing-snapshot warning", output)
	}
}

func TestRunnerSessionContextForResumptionWarnsWhenNoSQLiteSession(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("session context for-resumption error = %v", err)
	}
	for _, want := range []string{"=== POST-COMPACTION RESUMPTION ===", "WARNING: No active session found. Run `loaf session list --all`"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerSessionContextForCompactUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-compact-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
last_entry: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"markdown-compact-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-compact"})
	if err != nil {
		t.Fatalf("session context for-compact markdown error = %v", err)
	}
	if !strings.Contains(stdout.String(), "CONTEXT COMPACTION IMMINENT") || !strings.Contains(stdout.String(), "loaf session log") {
		t.Fatalf("stdout = %q, want compact instructions", stdout.String())
	}
	after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")
	if !strings.Contains(after, "compact(session): context compaction triggered") {
		t.Fatalf("session markdown = %q, want compact marker", after)
	}
	if strings.Contains(after, "last_updated: 2026-06-10T10:00:00Z") || strings.Contains(after, "last_entry: 2026-06-10T10:00:00Z") {
		t.Fatalf("session markdown = %q, want updated frontmatter timestamps", after)
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown compact path not to create SQLite database", err)
	}
}

func TestRunnerSessionContextForCompactSkipsMarkdownSubagents(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-compact-session
---
# Session

## Journal
`)
	before := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"agent_id":"subagent","session_id":"markdown-compact-session"}`),
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
	}.Run([]string{"session", "context", "for-compact"})
	if err != nil {
		t.Fatalf("session context for-compact markdown subagent error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want subagent skip", stdout.String())
	}
	if after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md"); after != before {
		t.Fatalf("session markdown changed for subagent:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestRunnerSessionContextForResumptionUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
spec: SPEC-123
claude_session_id: markdown-resume-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:05:00Z
last_entry: 2026-06-10T10:05:00Z
---
# Session

## Current State (2026-06-10 10:05)

**Working on:** native markdown resumption
**Status:** focused test fixture

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
[2026-06-10 10:04] decision(session): port markdown resumption
[2026-06-10 10:05] discover(session): recent journal is rendered
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"markdown-resume-session"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("session context for-resumption markdown error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"=== POST-COMPACTION RESUMPTION ===",
		"Session: .agents/sessions/20260610-session.md",
		"Branch: main",
		"Spec: SPEC-123",
		"## Current State (2026-06-10 10:05)",
		"**Working on:** native markdown resumption",
		"## Recent Journal",
		"decision(session): port markdown resumption",
		"discover(session): recent journal is rendered",
		"read the full session file: .agents/sessions/20260610-session.md",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown resumption path not to create SQLite database", err)
	}
}

func TestRunnerSessionContextForResumptionWarnsWithoutMarkdownSession(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"missing-session"}`),
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
	}.Run([]string{"session", "context", "for-resumption"})
	if err != nil {
		t.Fatalf("session context for-resumption missing markdown error = %v", err)
	}
	for _, want := range []string{"=== POST-COMPACTION RESUMPTION ===", "WARNING: No active session found. Read .agents/sessions/ manually."} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerLinkedWorktreesShareSQLiteState(t *testing.T) {
	requireCLIGit(t)
	main := initCLIGitRepo(t)
	linked := addCLILinkedWorktree(t, main, "sqlite-state")
	stateHome := t.TempDir()

	var mainPathOut bytes.Buffer
	if err := (Runner{Stdout: &mainPathOut, WorkingDir: main, StateHome: stateHome}).Run([]string{"state", "path"}); err != nil {
		t.Fatalf("main state path error = %v", err)
	}
	var linkedPathOut bytes.Buffer
	if err := (Runner{Stdout: &linkedPathOut, WorkingDir: linked, StateHome: stateHome}).Run([]string{"state", "path"}); err != nil {
		t.Fatalf("linked state path error = %v", err)
	}

	mainPath := strings.TrimSpace(mainPathOut.String())
	linkedPath := strings.TrimSpace(linkedPathOut.String())
	if mainPath != linkedPath {
		t.Fatalf("linked state path = %q, want main path %q", linkedPath, mainPath)
	}
	if !strings.HasPrefix(mainPath, filepath.Join(stateHome, "loaf", "projects")+string(filepath.Separator)) {
		t.Fatalf("state path = %q, want under state home %q", mainPath, stateHome)
	}

	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: main, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("main state init error = %v", err)
	}

	var linkedStatusOut bytes.Buffer
	if err := (Runner{Stdout: &linkedStatusOut, WorkingDir: linked, StateHome: stateHome}).Run([]string{"state", "status", "--json"}); err != nil {
		t.Fatalf("linked state status error = %v", err)
	}
	linkedStatus := decodeStateStatus(t, linkedStatusOut.Bytes())
	if linkedStatus.Mode != state.ModeSQLiteReady || linkedStatus.DatabasePath != mainPath {
		t.Fatalf("linked status = %#v, want sqlite-ready at %q", linkedStatus, mainPath)
	}

	var captureOut bytes.Buffer
	if err := (Runner{Stdout: &captureOut, WorkingDir: linked, StateHome: stateHome}).Run([]string{"idea", "capture", "--title", "Cross Worktree Idea", "--json"}); err != nil {
		t.Fatalf("linked idea capture error = %v", err)
	}
	captured := decodeIdeaCaptureResult(t, captureOut.Bytes())
	if captured.Idea.Alias == "" {
		t.Fatalf("captured idea = %#v, want alias", captured.Idea)
	}

	var listOut bytes.Buffer
	if err := (Runner{Stdout: &listOut, WorkingDir: main, StateHome: stateHome}).Run([]string{"idea", "list", "--json"}); err != nil {
		t.Fatalf("main idea list error = %v", err)
	}
	ideas := decodeIdeaList(t, listOut.Bytes())
	if ideas.Ideas[captured.Idea.Alias].Title != "Cross Worktree Idea" {
		t.Fatalf("ideas = %#v, want captured linked-worktree idea %q", ideas.Ideas, captured.Idea.Alias)
	}

	for _, dir := range []string{main, linked} {
		if _, err := os.Stat(filepath.Join(dir, ".agents")); !os.IsNotExist(err) {
			t.Fatalf("state commands created repository .agents directory in %q; err = %v", dir, err)
		}
	}
}

func TestRunnerStateInitStatusAndDoctor(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()

	var statusBefore bytes.Buffer
	err := Runner{
		Stdout:     &statusBefore,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "status", "--json"})
	if err != nil {
		t.Fatalf("state status before init error = %v", err)
	}
	before := decodeStateStatus(t, statusBefore.Bytes())
	if before.Mode != state.ModeMarkdownOnly {
		t.Fatalf("before.Mode = %q, want %q", before.Mode, state.ModeMarkdownOnly)
	}
	if before.DatabaseExists {
		t.Fatal("before.DatabaseExists = true, want false")
	}

	var initOut bytes.Buffer
	err = Runner{
		Stdout:     &initOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "init", "--json"})
	if err != nil {
		t.Fatalf("state init error = %v", err)
	}
	initialized := decodeStateStatus(t, initOut.Bytes())
	if initialized.Mode != state.ModeSQLiteReady {
		t.Fatalf("initialized.Mode = %q, want %q", initialized.Mode, state.ModeSQLiteReady)
	}
	if initialized.SchemaVersion != state.CurrentSchemaVersion() {
		t.Fatalf("initialized.SchemaVersion = %d, want %d", initialized.SchemaVersion, state.CurrentSchemaVersion())
	}
	if _, err := os.Stat(initialized.DatabasePath); err != nil {
		t.Fatalf("state init did not create database: %v", err)
	}

	var doctorOut bytes.Buffer
	err = Runner{
		Stdout:     &doctorOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "doctor"})
	if err != nil {
		t.Fatalf("state doctor error = %v", err)
	}
	if !strings.Contains(doctorOut.String(), "mode: "+state.ModeSQLiteReady) {
		t.Fatalf("doctor output = %q, want sqlite-ready mode", doctorOut.String())
	}
}

func TestRunnerStateInitHumanOutputPrintsRepositoryExternalDatabaseWithoutSecrets(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "init"})
	if err != nil {
		t.Fatalf("state init error = %v", err)
	}

	output := stdout.String()
	databasePath := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "database: ") {
			databasePath = strings.TrimSpace(strings.TrimPrefix(line, "database: "))
			break
		}
	}
	if databasePath == "" {
		t.Fatalf("output = %q, want database path line", output)
	}
	if !filepath.IsAbs(databasePath) {
		t.Fatalf("database path = %q, want absolute path", databasePath)
	}
	if !strings.HasPrefix(databasePath, filepath.Join(stateHome, "loaf", "projects")+string(filepath.Separator)) {
		t.Fatalf("database path = %q, want under state home %q", databasePath, stateHome)
	}
	if strings.HasPrefix(databasePath, workingDir+string(filepath.Separator)) {
		t.Fatalf("database path = %q, want outside working dir %q", databasePath, workingDir)
	}
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("database was not created at printed path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("state init created repository .agents directory; err = %v", err)
	}
	lowerOutput := strings.ToLower(output)
	for _, forbidden := range []string{"token", "password", "secret", "api_key", "api key", "credential"} {
		if strings.Contains(lowerOutput, forbidden) {
			t.Fatalf("state init output contains forbidden secret-storage term %q:\n%s", forbidden, output)
		}
	}
}

func TestRunnerStateDoctorFixInitializesMissingDatabase(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "doctor", "--fix", "--json"})
	if err != nil {
		t.Fatalf("state doctor --fix error = %v", err)
	}
	status := decodeStateStatus(t, stdout.Bytes())
	if status.Mode != state.ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, state.ModeSQLiteReady)
	}
	if _, err := os.Stat(status.DatabasePath); err != nil {
		t.Fatalf("doctor --fix did not create database: %v", err)
	}
	if !hasDiagnostic(status.Diagnostics, "database-initialized") {
		t.Fatalf("diagnostics = %#v, want database-initialized", status.Diagnostics)
	}
}

func TestRunnerStateDoctorReportsSchemaMismatch(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (99, 'future_schema', 'future', '2026-05-28T10:00:00Z')`); err != nil {
		t.Fatalf("insert future schema migration error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "doctor"})
	if err == nil {
		t.Fatal("state doctor schema mismatch error = nil, want error")
	}
	if !strings.Contains(stdout.String(), fmt.Sprintf("schema version 99 does not match expected version %d", state.CurrentSchemaVersion())) {
		t.Fatalf("stdout = %q, want schema mismatch diagnostic", stdout.String())
	}
}

func TestRunnerStateBackupCreatesSQLiteCopy(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "backup", "--json"})
	if err != nil {
		t.Fatalf("state backup --json error = %v", err)
	}

	result := decodeStateBackupResult(t, stdout.Bytes())
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty")
	}
	if result.Bytes <= 0 {
		t.Fatalf("Bytes = %d, want > 0", result.Bytes)
	}
	if result.CreatedAt == "" {
		t.Fatal("CreatedAt is empty")
	}
	if strings.HasPrefix(result.BackupPath, workingDir+string(filepath.Separator)) {
		t.Fatalf("BackupPath = %q, want outside working dir %q", result.BackupPath, workingDir)
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	store, err := state.OpenStore(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStore(backup) error = %v", err)
	}
	defer store.Close()
	version, err := store.SchemaVersion(t.Context())
	if err != nil {
		t.Fatalf("backup SchemaVersion() error = %v", err)
	}
	if version != state.CurrentSchemaVersion() {
		t.Fatalf("backup schema version = %d, want %d", version, state.CurrentSchemaVersion())
	}
}

func TestRunnerStateBackupHumanOutput(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "backup"})
	if err != nil {
		t.Fatalf("state backup error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"loaf state backup", "database:", "backup:", "bytes:", "created at:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerStateBackupRejectsMissingAndInvalidState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "backup"})
	if err == nil {
		t.Fatal("state backup missing-state error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialization message", err)
	}

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "backup"})
	if err == nil {
		t.Fatal("state backup invalid-state error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestRunnerStateExportAllJSON(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","spec":"SPEC-001","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err != nil {
		t.Fatalf("state export all --format json error = %v", err)
	}

	snapshot := decodeStateExportSnapshot(t, stdout.Bytes())
	if snapshot.ExportKind != state.ExportKindAll {
		t.Fatalf("ExportKind = %q, want %q", snapshot.ExportKind, state.ExportKindAll)
	}
	if snapshot.Format != state.ExportFormatJSON {
		t.Fatalf("Format = %q, want %q", snapshot.Format, state.ExportFormatJSON)
	}
	if snapshot.Audience != state.ExportAudienceLocal {
		t.Fatalf("Audience = %q, want internal marker", snapshot.Audience)
	}
	if snapshot.SchemaVersion != state.CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", snapshot.SchemaVersion, state.CurrentSchemaVersion())
	}
	if len(snapshot.Tables["specs"]) != 1 || len(snapshot.Tables["tasks"]) != 1 {
		t.Fatalf("tables = %#v, want exported spec and task rows", snapshot.Tables)
	}
	if snapshot.Tables["tasks"][0]["title"] != "Example Task" {
		t.Fatalf("task title = %#v, want imported task", snapshot.Tables["tasks"][0]["title"])
	}
}

func TestRunnerStateExportTriageMarkdown(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"idea", "capture", "--title", "Ship SPEC-001 Track A follow-up"}); err != nil {
		t.Fatalf("idea capture error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"spark", "capture", "--scope", "Phase 2", "--text", "TASK-002 from .agents/tasks/TASK-002.md"}); err != nil {
		t.Fatalf("spark capture error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "triage", "--format", "markdown"})
	if err != nil {
		t.Fatalf("state export triage --format markdown error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"# Triage Export", "Audience: external", "## Ideas", "## Sparks", "## Brainstorms", "internal reference"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	for _, banned := range []string{"SPEC-001", "TASK-002", ".agents/", "Track A", "Phase 2"} {
		if strings.Contains(output, banned) {
			t.Fatalf("output leaked %q:\n%s", banned, output)
		}
	}
}

func TestRunnerStateExportReleaseReadinessMarkdown(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Example Task\n")
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
branch: feature/SPEC-001-Phase-2
status: active
---
[2026-05-28 10:00] decision(sqlite): release readiness
`)
	writeCLIAgentsFile(t, workingDir, "reports/release.md", `---
kind: session
title: Release SPEC-001 Track A report
status: final
---
# Release Report
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Example Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"}
  }
}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "release-readiness", "--format", "markdown"})
	if err != nil {
		t.Fatalf("state export release-readiness --format markdown error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Release Readiness Export",
		"Audience: external",
		"Release readiness: not ready",
		"Specs: 1 active, 0 complete, 0 archived",
		"Tasks: 1 unresolved, 0 done, 0 archived",
		"Sessions: 1 active, 1 total",
		"Reports: 0 draft, 1 total",
		"No generated exports recorded.",
		"session/final: Release internal reference internal reference report",
		"active session on feature/internal reference-internal reference with 1 journal entry",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	for _, banned := range []string{"SPEC-001", "TASK-001", ".agents/", "Track A", "Phase 2"} {
		if strings.Contains(output, banned) {
			t.Fatalf("output leaked %q:\n%s", banned, output)
		}
	}
}

func TestRunnerStateExportSpecMarkdown(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec

Imported spec prose.
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-todo.md", "# Todo task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-progress.md", "# Progress task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-003-done.md", "# Done task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "spec", "SPEC-001", "--format", "markdown"})
	if err != nil {
		t.Fatalf("state export spec --format markdown error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Spec Export",
		"Audience: internal",
		"Spec: `SPEC-001`",
		"Title: Example Spec",
		"Status: implementing",
		"Tasks: 1 todo, 1 in progress, 1 done",
		"`.agents/specs/SPEC-001-example.md`",
		"inbound `implements` task `TASK-001`",
		"# Example Spec",
		"Imported spec prose.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "status: implementing") || strings.Contains(output, "---") {
		t.Fatalf("output = %q, want stripped frontmatter", output)
	}
}

func TestRunnerStateExportSessionMarkdown(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
branch: feature/session-export
status: active
claude_session_id: harness-export
---
[2026-05-28 10:00] decision(sqlite): render this session
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-session.md", "# Session Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Session Task","status":"todo","priority":"P2"}
}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"task", "update", "TASK-001", "--session", "20260528-session"}); err != nil {
		t.Fatalf("task update --session error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "session", "20260528-session", "--format", "markdown"})
	if err != nil {
		t.Fatalf("state export session --format markdown error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Session Export",
		"Audience: internal",
		"Session: `20260528-session`",
		"Branch: `feature/session-export`",
		"Harness session: `harness-export`",
		"`.agents/sessions/20260528-session.md`",
		"`decision(sqlite)`: render this session",
		"inbound `associated_with` task `TASK-001`",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerReportGenerateSessionAndSessionReportMatchStateExport(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
branch: feature/session-report
status: active
---
[2026-05-28 10:00] decision(sqlite): render this session report
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var exportOut bytes.Buffer
	if err := (Runner{Stdout: &exportOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "export", "session", "20260528-session", "--format", "markdown"}); err != nil {
		t.Fatalf("state export session error = %v", err)
	}
	var reportOut bytes.Buffer
	if err := (Runner{Stdout: &reportOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "generate", "session", "20260528-session"}); err != nil {
		t.Fatalf("report generate session error = %v", err)
	}
	var sessionReportOut bytes.Buffer
	if err := (Runner{Stdout: &sessionReportOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "report", "20260528-session"}); err != nil {
		t.Fatalf("session report error = %v", err)
	}

	if reportOut.String() != exportOut.String() {
		t.Fatalf("report generate output differs from state export:\nreport=%s\nexport=%s", reportOut.String(), exportOut.String())
	}
	if sessionReportOut.String() != exportOut.String() {
		t.Fatalf("session report output differs from state export:\nsession=%s\nexport=%s", sessionReportOut.String(), exportOut.String())
	}
	if !strings.Contains(reportOut.String(), "# Session Export") {
		t.Fatalf("report output = %q, want session export markdown", reportOut.String())
	}
}

func TestRunnerReportGenerateTriageAndReleaseReadinessMatchStateExports(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"idea", "capture", "--title", "Release follow-up"}); err != nil {
		t.Fatalf("idea capture error = %v", err)
	}

	var triageExport bytes.Buffer
	if err := (Runner{Stdout: &triageExport, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "export", "triage", "--format", "markdown"}); err != nil {
		t.Fatalf("state export triage error = %v", err)
	}
	var triageReport bytes.Buffer
	if err := (Runner{Stdout: &triageReport, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "generate", "triage"}); err != nil {
		t.Fatalf("report generate triage error = %v", err)
	}
	if triageReport.String() != triageExport.String() {
		t.Fatalf("triage report output differs from state export:\nreport=%s\nexport=%s", triageReport.String(), triageExport.String())
	}

	var releaseExport bytes.Buffer
	if err := (Runner{Stdout: &releaseExport, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "export", "release-readiness", "--format", "markdown"}); err != nil {
		t.Fatalf("state export release-readiness error = %v", err)
	}
	var releaseReport bytes.Buffer
	if err := (Runner{Stdout: &releaseReport, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "generate", "release-readiness"}); err != nil {
		t.Fatalf("report generate release-readiness error = %v", err)
	}
	if releaseReport.String() != releaseExport.String() {
		t.Fatalf("release report output differs from state export:\nreport=%s\nexport=%s", releaseReport.String(), releaseExport.String())
	}
	if !strings.Contains(releaseReport.String(), "# Release Readiness Export") {
		t.Fatalf("release report output = %q, want release readiness markdown", releaseReport.String())
	}
}

func TestRunnerReportGenerateDoesNotMutateStateOrCreateRepoFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var firstOut bytes.Buffer
	if err := (Runner{Stdout: &firstOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "generate", "release-readiness"}); err != nil {
		t.Fatalf("first report generate release-readiness error = %v", err)
	}
	var secondOut bytes.Buffer
	if err := (Runner{Stdout: &secondOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "generate", "release-readiness"}); err != nil {
		t.Fatalf("second report generate release-readiness error = %v", err)
	}
	if firstOut.String() != secondOut.String() {
		t.Fatalf("report output changed:\nfirst=%s\nsecond=%s", firstOut.String(), secondOut.String())
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("report generate created repository .agents directory; err = %v", err)
	}
	var snapshotOut bytes.Buffer
	if err := (Runner{Stdout: &snapshotOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "export", "all", "--format", "json"}); err != nil {
		t.Fatalf("state export all error = %v", err)
	}
	snapshot := decodeStateExportSnapshot(t, snapshotOut.Bytes())
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestRunnerReportGenerateRejectsMissingInvalidUnsupportedState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "generate", "triage"})
	if err == nil {
		t.Fatal("report generate missing-state error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialization message", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "generate", "bundle"})
	if err == nil {
		t.Fatal("report generate unsupported kind error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "report generate kind \"bundle\" is not implemented yet") {
		t.Fatalf("error = %v, want unsupported kind message", err)
	}

	path := stateDBPathForWorkingDir(t, workingDir, stateHome)
	writeInvalidDatabaseFileForCLI(t, path)
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "generate", "release-readiness"})
	if err == nil {
		t.Fatal("report generate invalid-state error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want invalid-state message", err)
	}
}

func TestRunnerReportLifecycleUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	beforeFiles := repoFileList(t, workingDir)

	var createOut bytes.Buffer
	if err := (Runner{Stdout: &createOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "create", "release-readiness", "--type", "audit", "--source", "manual", "--json"}); err != nil {
		t.Fatalf("report create error = %v", err)
	}
	created := decodeReportCreateResult(t, createOut.Bytes())
	if created.Report.Alias != "report-release-readiness" || created.Report.Status != "draft" || created.Kind != "audit" || created.Source != "manual" {
		t.Fatalf("created = %#v, want draft report", created)
	}

	var draftListOut bytes.Buffer
	if err := (Runner{Stdout: &draftListOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "list", "--json"}); err != nil {
		t.Fatalf("report list after create error = %v", err)
	}
	draftReports := decodeReportList(t, draftListOut.Bytes())
	if draftReports.Reports["report-release-readiness"].Status != "draft" {
		t.Fatalf("draft reports = %#v, want draft report", draftReports.Reports)
	}

	var finalizeOut bytes.Buffer
	if err := (Runner{Stdout: &finalizeOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "finalize", "report-release-readiness", "--json"}); err != nil {
		t.Fatalf("report finalize error = %v", err)
	}
	finalized := decodeReportStatusResult(t, finalizeOut.Bytes())
	if finalized.Previous != "draft" || finalized.Status != "final" {
		t.Fatalf("finalized = %#v, want final transition", finalized)
	}

	var archiveOut bytes.Buffer
	if err := (Runner{Stdout: &archiveOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "archive", "report-release-readiness", "--json"}); err != nil {
		t.Fatalf("report archive error = %v", err)
	}
	archived := decodeReportStatusResult(t, archiveOut.Bytes())
	if archived.Previous != "final" || archived.Status != "archived" {
		t.Fatalf("archived = %#v, want archived transition", archived)
	}

	var archivedListOut bytes.Buffer
	if err := (Runner{Stdout: &archivedListOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"report", "list", "--json", "--status", "archived"}); err != nil {
		t.Fatalf("report list archived error = %v", err)
	}
	archivedReports := decodeReportList(t, archivedListOut.Bytes())
	if archivedReports.Reports["report-release-readiness"].Status != "archived" {
		t.Fatalf("archived reports = %#v, want archived report", archivedReports.Reports)
	}

	afterFiles := repoFileList(t, workingDir)
	if strings.Join(afterFiles, "\n") != strings.Join(beforeFiles, "\n") {
		t.Fatalf("report lifecycle created repository files:\nbefore=%v\nafter=%v", beforeFiles, afterFiles)
	}
}

func TestRunnerReportLifecycleUsesMarkdownFilesWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()

	var createOut bytes.Buffer
	err := Runner{
		Stdout:     &createOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "create", "release-readiness", "--type", "audit", "--source", "manual", "--json"})
	if err != nil {
		t.Fatalf("report create markdown error = %v", err)
	}
	created := decodeReportCreateResult(t, createOut.Bytes())
	if created.Report.Status != "draft" || created.Kind != "audit" || created.Source != "manual" || !strings.HasSuffix(created.Report.Alias, "-audit-release-readiness") {
		t.Fatalf("created = %#v, want markdown draft report", created)
	}
	reportFile := filepath.Join(workingDir, ".agents", "reports", created.Report.Alias+".md")
	reportRaw, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("ReadFile(created report) error = %v", err)
	}
	reportFrontmatter, ok := parseKnowledgeFrontmatter(reportRaw)
	if !ok {
		t.Fatal("created report frontmatter missing")
	}
	if firstFieldValue(reportFrontmatter["title"]) != "Release Readiness" || firstFieldValue(reportFrontmatter["type"]) != "audit" || firstFieldValue(reportFrontmatter["status"]) != "draft" || firstFieldValue(reportFrontmatter["source"]) != "manual" {
		t.Fatalf("frontmatter = %#v, want created report metadata", reportFrontmatter)
	}
	if body := markdownContentWithoutFrontmatter(string(reportRaw)); !strings.Contains(body, "## Key Findings") || !strings.Contains(body, "# Release Readiness") {
		t.Fatalf("body = %q, want report scaffold sections", body)
	}
	reportRaw = []byte(strings.Replace(string(reportRaw), "tags: []", "tags: [alpha, beta]\naudience: engineering", 1))
	if err := os.WriteFile(reportFile, reportRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(report with tags) error = %v", err)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "list", "--json", "--type", "audit"})
	if err != nil {
		t.Fatalf("report list markdown error = %v", err)
	}
	listed := decodeReportList(t, listOut.Bytes())
	if listed.Reports[created.Report.Alias].Status != "draft" || listed.Reports[created.Report.Alias].Kind != "audit" {
		t.Fatalf("reports = %#v, want created markdown report", listed.Reports)
	}

	var finalizeOut bytes.Buffer
	err = Runner{
		Stdout:     &finalizeOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "finalize", "release-readiness", "--json"})
	if err != nil {
		t.Fatalf("report finalize markdown error = %v", err)
	}
	finalized := decodeReportStatusResult(t, finalizeOut.Bytes())
	if finalized.Previous != "draft" || finalized.Status != "final" || finalized.Report.Alias != created.Report.Alias {
		t.Fatalf("finalized = %#v, want draft to final", finalized)
	}
	reportRaw, err = os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("ReadFile(finalized report) error = %v", err)
	}
	reportFrontmatter, ok = parseKnowledgeFrontmatter(reportRaw)
	if !ok {
		t.Fatal("finalized report frontmatter missing")
	}
	if firstFieldValue(reportFrontmatter["status"]) != "final" || firstFieldValue(reportFrontmatter["finalized_at"]) == "" {
		t.Fatalf("frontmatter = %#v, want finalized metadata", reportFrontmatter)
	}

	var archiveOut bytes.Buffer
	err = Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "archive", created.Report.Alias + ".md", "--json"})
	if err != nil {
		t.Fatalf("report archive markdown error = %v", err)
	}
	archived := decodeReportStatusResult(t, archiveOut.Bytes())
	if archived.Previous != "final" || archived.Status != "archived" || archived.Report.Alias != created.Report.Alias {
		t.Fatalf("archived = %#v, want final to archived", archived)
	}
	if _, err := os.Stat(reportFile); !os.IsNotExist(err) {
		t.Fatalf("active report stat error = %v, want removed", err)
	}
	archivedFile := filepath.Join(workingDir, ".agents", "reports", "archive", created.Report.Alias+".md")
	reportRaw, err = os.ReadFile(archivedFile)
	if err != nil {
		t.Fatalf("ReadFile(archived report) error = %v", err)
	}
	reportFrontmatter, ok = parseKnowledgeFrontmatter(reportRaw)
	if !ok {
		t.Fatal("archived report frontmatter missing")
	}
	if firstFieldValue(reportFrontmatter["status"]) != "archived" || firstFieldValue(reportFrontmatter["archived_at"]) == "" || firstFieldValue(reportFrontmatter["archived_by"]) != "cli" {
		t.Fatalf("frontmatter = %#v, want archived metadata", reportFrontmatter)
	}
	if !reportFrontmatter["tags"].Array || strings.Join(reportFrontmatter["tags"].Values, ",") != "alpha,beta" || firstFieldValue(reportFrontmatter["audience"]) != "engineering" {
		t.Fatalf("frontmatter = %#v, want tags and unknown fields preserved", reportFrontmatter)
	}

	writeCLIAgentsFile(t, workingDir, "reports/20260101-010101-research-alpha-one.md", "---\ntitle: Alpha One\nstatus: draft\ntype: research\nsource: manual\n---\n# Alpha One\n")
	writeCLIAgentsFile(t, workingDir, "reports/20260101-010102-research-alpha-two.md", "---\ntitle: Alpha Two\nstatus: draft\ntype: research\nsource: manual\n---\n# Alpha Two\n")
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "finalize", "alpha"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "ambiguous") {
		t.Fatalf("ambiguous finalize error = %v, want ambiguity", err)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerReportLifecycleReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	path := stateDBPathForWorkingDir(t, workingDir, stateHome)
	writeInvalidDatabaseFileForCLI(t, path)

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "create", "release-readiness"})
	if err == nil {
		t.Fatal("report create error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want invalid state message", err)
	}
}

func TestRunnerStateExportAllJSONDoesNotMutateStateOrCreateRepoFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var firstOut bytes.Buffer
	err := Runner{
		Stdout:     &firstOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format=json"})
	if err != nil {
		t.Fatalf("first state export error = %v", err)
	}
	var secondOut bytes.Buffer
	err = Runner{
		Stdout:     &secondOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err != nil {
		t.Fatalf("second state export error = %v", err)
	}

	first := decodeStateExportSnapshot(t, firstOut.Bytes())
	second := decodeStateExportSnapshot(t, secondOut.Bytes())
	if !reflect.DeepEqual(first.Tables, second.Tables) {
		t.Fatalf("export tables changed:\nfirst=%#v\nsecond=%#v", first.Tables, second.Tables)
	}
	if len(first.Tables["exports"]) != 0 || len(second.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: first=%#v second=%#v", first.Tables["exports"], second.Tables["exports"])
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("state export created repository .agents directory; err = %v", err)
	}
}

func TestRunnerStateExportReleaseReadinessMarkdownDoesNotMutateStateOrCreateRepoFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var firstOut bytes.Buffer
	err := Runner{
		Stdout:     &firstOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "release-readiness", "--format=markdown"})
	if err != nil {
		t.Fatalf("first state export release-readiness error = %v", err)
	}
	var secondOut bytes.Buffer
	err = Runner{
		Stdout:     &secondOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "release-readiness", "--format", "markdown"})
	if err != nil {
		t.Fatalf("second state export release-readiness error = %v", err)
	}
	if firstOut.String() != secondOut.String() {
		t.Fatalf("export output changed:\nfirst=%s\nsecond=%s", firstOut.String(), secondOut.String())
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("state export created repository .agents directory; err = %v", err)
	}
	var snapshotOut bytes.Buffer
	err = Runner{
		Stdout:     &snapshotOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err != nil {
		t.Fatalf("state export all error = %v", err)
	}
	snapshot := decodeStateExportSnapshot(t, snapshotOut.Bytes())
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestRunnerStateExportSpecMarkdownDoesNotMutateStateOrCreateRepoFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	beforeFiles := repoFileList(t, workingDir)

	var firstOut bytes.Buffer
	err := Runner{
		Stdout:     &firstOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "spec", "SPEC-001", "--format=markdown"})
	if err != nil {
		t.Fatalf("first state export spec error = %v", err)
	}
	var secondOut bytes.Buffer
	err = Runner{
		Stdout:     &secondOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "spec", "SPEC-001", "--format", "markdown"})
	if err != nil {
		t.Fatalf("second state export spec error = %v", err)
	}
	if firstOut.String() != secondOut.String() {
		t.Fatalf("export output changed:\nfirst=%s\nsecond=%s", firstOut.String(), secondOut.String())
	}
	afterFiles := repoFileList(t, workingDir)
	if !reflect.DeepEqual(beforeFiles, afterFiles) {
		t.Fatalf("repository files changed:\nbefore=%#v\nafter=%#v", beforeFiles, afterFiles)
	}
	var snapshotOut bytes.Buffer
	err = Runner{
		Stdout:     &snapshotOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err != nil {
		t.Fatalf("state export all error = %v", err)
	}
	snapshot := decodeStateExportSnapshot(t, snapshotOut.Bytes())
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestRunnerStateExportSessionMarkdownDoesNotMutateStateOrCreateRepoFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", `---
branch: feature/session-export
status: active
---
[2026-05-28 10:00] decision(sqlite): render this session
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	beforeFiles := repoFileList(t, workingDir)

	var firstOut bytes.Buffer
	err := Runner{
		Stdout:     &firstOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "session", "20260528-session", "--format=markdown"})
	if err != nil {
		t.Fatalf("first state export session error = %v", err)
	}
	var secondOut bytes.Buffer
	err = Runner{
		Stdout:     &secondOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "session", "20260528-session", "--format", "markdown"})
	if err != nil {
		t.Fatalf("second state export session error = %v", err)
	}
	if firstOut.String() != secondOut.String() {
		t.Fatalf("export output changed:\nfirst=%s\nsecond=%s", firstOut.String(), secondOut.String())
	}
	afterFiles := repoFileList(t, workingDir)
	if !reflect.DeepEqual(beforeFiles, afterFiles) {
		t.Fatalf("repository files changed:\nbefore=%#v\nafter=%#v", beforeFiles, afterFiles)
	}
	var snapshotOut bytes.Buffer
	err = Runner{
		Stdout:     &snapshotOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err != nil {
		t.Fatalf("state export all error = %v", err)
	}
	snapshot := decodeStateExportSnapshot(t, snapshotOut.Bytes())
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestRunnerStateExportTriageMarkdownDoesNotCreateRepoFiles(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "triage", "--format=markdown"})
	if err != nil {
		t.Fatalf("state export triage error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("state export created repository .agents directory; err = %v", err)
	}
}

func TestRunnerStateExportRejectsMissingInvalidUnsupportedState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err == nil {
		t.Fatal("state export missing-state error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialization message", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "bundle", "--format", "markdown"})
	if err == nil {
		t.Fatal("state export unsupported kind error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state export kind \"bundle\" is not implemented yet") {
		t.Fatalf("error = %v, want unsupported kind message", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "spec", "--format", "markdown"})
	if err == nil {
		t.Fatal("state export spec missing ref error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state export spec requires exactly one spec") {
		t.Fatalf("error = %v, want missing spec message", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "session", "--format", "markdown"})
	if err == nil {
		t.Fatal("state export session missing ref error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state export session requires exactly one session") {
		t.Fatalf("error = %v, want missing session message", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "markdown"})
	if err == nil {
		t.Fatal("state export unsupported format error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state export format \"markdown\" is not implemented yet") {
		t.Fatalf("error = %v, want unsupported format message", err)
	}

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "export", "all", "--format", "json"})
	if err == nil {
		t.Fatal("state export invalid-state error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestRunnerSessionHelpAndUnknownSubcommandAreNative(t *testing.T) {
	for _, args := range [][]string{{"session"}, {"session", "--help"}, {"session", "help"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run(args)
			if err != nil {
				t.Fatalf("%v error = %v", args, err)
			}
			if !strings.Contains(stdout.String(), "Usage: loaf session <subcommand>") || !strings.Contains(stdout.String(), "start") {
				t.Fatalf("stdout = %q, want native session help", stdout.String())
			}
		})
	}

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: t.TempDir(),
	}.Run([]string{"session", "legacy-tail"})
	if err == nil || !strings.Contains(err.Error(), `unknown loaf session subcommand "legacy-tail"`) {
		t.Fatalf("session unknown error = %v, want native unknown subcommand", err)
	}
}

func TestRunnerSessionNestedHelpAndUnknownSubcommandsAreNative(t *testing.T) {
	cases := []struct {
		command        string
		args           []string
		wantHelp       string
		wantSubcommand string
	}{
		{command: "session state", args: []string{"session", "state"}, wantHelp: "Usage: loaf session state <subcommand>", wantSubcommand: "update"},
		{command: "session context", args: []string{"session", "context"}, wantHelp: "Usage: loaf session context <subcommand>", wantSubcommand: "for-resumption"},
	}

	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			for _, args := range [][]string{tc.args, append(append([]string{}, tc.args...), "--help"), append(append([]string{}, tc.args...), "help")} {
				var stdout bytes.Buffer
				err := Runner{
					Stdout:     &stdout,
					WorkingDir: t.TempDir(),
				}.Run(args)
				if err != nil {
					t.Fatalf("%v error = %v", args, err)
				}
				if !strings.Contains(stdout.String(), tc.wantHelp) || !strings.Contains(stdout.String(), tc.wantSubcommand) {
					t.Fatalf("stdout = %q, want %q and %q", stdout.String(), tc.wantHelp, tc.wantSubcommand)
				}
			}

			err := Runner{
				Stdout:     &bytes.Buffer{},
				WorkingDir: t.TempDir(),
			}.Run(append(append([]string{}, tc.args...), "legacy-tail"))
			wantErr := fmt.Sprintf("unknown loaf %s subcommand \"legacy-tail\"", tc.command)
			if err == nil || !strings.Contains(err.Error(), wantErr) {
				t.Fatalf("%s unknown error = %v, want %q", tc.command, err, wantErr)
			}
		})
	}
}

func TestRunnerHybridCommandHelpAndUnknownSubcommandsAreNative(t *testing.T) {
	cases := []struct {
		command        string
		wantHelp       string
		wantSubcommand string
	}{
		{command: "task", wantHelp: "Usage: loaf task <subcommand>", wantSubcommand: "create"},
		{command: "spec", wantHelp: "Usage: loaf spec <subcommand>", wantSubcommand: "list"},
		{command: "report", wantHelp: "Usage: loaf report <subcommand>", wantSubcommand: "generate"},
	}

	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			for _, args := range [][]string{{tc.command}, {tc.command, "--help"}, {tc.command, "help"}} {
				var stdout bytes.Buffer
				err := Runner{
					Stdout:     &stdout,
					WorkingDir: t.TempDir(),
				}.Run(args)
				if err != nil {
					t.Fatalf("%v error = %v", args, err)
				}
				if !strings.Contains(stdout.String(), tc.wantHelp) || !strings.Contains(stdout.String(), tc.wantSubcommand) {
					t.Fatalf("stdout = %q, want %q and %q", stdout.String(), tc.wantHelp, tc.wantSubcommand)
				}
			}

			err := Runner{
				Stdout:     &bytes.Buffer{},
				WorkingDir: t.TempDir(),
			}.Run([]string{tc.command, "legacy-tail"})
			wantErr := fmt.Sprintf("unknown loaf %s subcommand \"legacy-tail\"", tc.command)
			if err == nil || !strings.Contains(err.Error(), wantErr) {
				t.Fatalf("%s unknown error = %v, want %q", tc.command, err, wantErr)
			}
		})
	}
}

func TestRunnerHousekeepingHelpIsNative(t *testing.T) {
	for _, args := range [][]string{{"housekeeping", "--help"}, {"housekeeping", "-h"}, {"housekeeping", "help"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: t.TempDir(),
			}.Run(args)
			if err != nil {
				t.Fatalf("%v error = %v", args, err)
			}
			if !strings.Contains(stdout.String(), "Usage: loaf housekeeping [options]") || !strings.Contains(stdout.String(), "--sessions") {
				t.Fatalf("stdout = %q, want native housekeeping help", stdout.String())
			}
		})
	}
}

func TestRunnerSessionStartUsesSQLiteStateWhenInitialized(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"harness-cli-123456"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "start", "--json"})
	if err != nil {
		t.Fatalf("session start --json error = %v", err)
	}
	start := decodeSessionStart(t, stdout.Bytes())
	if start.Action != state.SessionStartActionCreated || start.Session.Alias == "" || start.HarnessSessionID != "harness-cli-123456" {
		t.Fatalf("start = %#v, want created harness-backed session", start)
	}

	var showOut bytes.Buffer
	if err := (Runner{Stdout: &showOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "show", start.Session.Alias, "--json"}); err != nil {
		t.Fatalf("session show --json error = %v", err)
	}
	show := decodeSessionShow(t, showOut.Bytes())
	if show.Session.Branch != "main" || show.Session.Status != "active" || show.Session.HarnessSessionID != "harness-cli-123456" {
		t.Fatalf("session = %#v, want native active session", show.Session)
	}
	if len(show.Session.JournalEntries) != 1 || show.Session.JournalEntries[0].EntryType != "session" || show.Session.JournalEntries[0].Scope != "start" {
		t.Fatalf("journal entries = %#v, want linked session(start)", show.Session.JournalEntries)
	}
}

func TestRunnerSessionStartUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"markdown-start-111111"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "start", "--json"})
	if err != nil {
		t.Fatalf("session start markdown --json error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=session start") {
		t.Fatalf("stdout = %q, want native markdown start without legacy delegation", stdout.String())
	}
	start := decodeSessionStart(t, stdout.Bytes())
	if start.Action != state.SessionStartActionCreated || start.Session.Alias == "" || start.HarnessSessionID != "markdown-start-111111" {
		t.Fatalf("start = %#v, want created markdown session", start)
	}
	sessionRel := filepath.ToSlash(filepath.Join("sessions", start.Session.Alias+".md"))
	created := readCLIAgentsFile(t, workingDir, sessionRel)
	for _, want := range []string{"status: active", "branch: main", "claude_session_id: markdown-start-111111", "session(start):  === SESSION STARTED === (session markdown)"} {
		if !strings.Contains(created, want) {
			t.Fatalf("created markdown = %q, want %q", created, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)

	var resumeOut bytes.Buffer
	err = Runner{
		Stdout:     &resumeOut,
		Stdin:      strings.NewReader(`{"session_id":"markdown-start-111111"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "start", "--json"})
	if err != nil {
		t.Fatalf("session start existing markdown error = %v", err)
	}
	resumed := decodeSessionStart(t, resumeOut.Bytes())
	if resumed.Action != state.SessionStartActionAlreadyActive || resumed.Session.Alias != start.Session.Alias {
		t.Fatalf("resumed = %#v, want same already-active session", resumed)
	}

	var rotateOut bytes.Buffer
	err = Runner{
		Stdout:     &rotateOut,
		Stdin:      strings.NewReader(`{"session_id":"markdown-start-222222"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "start", "--json"})
	if err != nil {
		t.Fatalf("session start rotate markdown error = %v", err)
	}
	rotated := decodeSessionStart(t, rotateOut.Bytes())
	if rotated.Action != state.SessionStartActionRotated || rotated.Session.Alias == start.Session.Alias || rotated.PreviousSession == nil {
		t.Fatalf("rotated = %#v, want new session and stopped previous", rotated)
	}
	previous := readCLIAgentsFile(t, workingDir, sessionRel)
	for _, want := range []string{"status: stopped", "session(end): closed by new conversation", "session(stop):   === SESSION STOPPED ==="} {
		if !strings.Contains(previous, want) {
			t.Fatalf("previous markdown = %q, want %q", previous, want)
		}
	}

	rotatedRel := filepath.ToSlash(filepath.Join("sessions", rotated.Session.Alias+".md"))
	beforeSubagent := readCLIAgentsFile(t, workingDir, rotatedRel)
	var subagentOut bytes.Buffer
	err = Runner{
		Stdout:     &subagentOut,
		Stdin:      strings.NewReader(`{"session_id":"markdown-start-333333","agent_id":"subagent"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "start", "--json"})
	if err != nil {
		t.Fatalf("session start markdown subagent error = %v", err)
	}
	if subagentOut.String() != "" {
		t.Fatalf("subagent stdout = %q, want silent skip", subagentOut.String())
	}
	afterSubagent := readCLIAgentsFile(t, workingDir, rotatedRel)
	if afterSubagent != beforeSubagent {
		t.Fatalf("subagent changed markdown session:\nbefore=%s\nafter=%s", beforeSubagent, afterSubagent)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSessionEndTargetsHarnessSessionInSQLiteState(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	target, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "feature/target",
		HarnessSessionID: "harness-target",
	})
	if err != nil {
		t.Fatalf("target StartSession() error = %v", err)
	}
	other, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-main",
	})
	if err != nil {
		t.Fatalf("main StartSession() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "end", "--json", "--session-id", "harness-target"})
	if err != nil {
		t.Fatalf("session end --json --session-id error = %v", err)
	}
	ended := decodeSessionEnd(t, stdout.Bytes())
	if ended.Action != state.SessionEndActionStopped || ended.Session.ID != target.Session.ID || len(ended.JournalEntryIDs) != 2 {
		t.Fatalf("ended = %#v, want stopped target session", ended)
	}

	var targetShowOut bytes.Buffer
	if err := (Runner{Stdout: &targetShowOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "show", target.Session.Alias, "--json"}); err != nil {
		t.Fatalf("session show target error = %v", err)
	}
	targetShow := decodeSessionShow(t, targetShowOut.Bytes())
	if targetShow.Session.Status != "stopped" {
		t.Fatalf("target status = %q, want stopped", targetShow.Session.Status)
	}

	var otherShowOut bytes.Buffer
	if err := (Runner{Stdout: &otherShowOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "show", other.Session.Alias, "--json"}); err != nil {
		t.Fatalf("session show other error = %v", err)
	}
	otherShow := decodeSessionShow(t, otherShowOut.Bytes())
	if otherShow.Session.Status != "active" {
		t.Fatalf("other status = %q, want active", otherShow.Session.Status)
	}

	var listOut bytes.Buffer
	if err := (Runner{Stdout: &listOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "list", "--json"}); err != nil {
		t.Fatalf("session list --json error = %v", err)
	}
	list := decodeSessionList(t, listOut.Bytes())
	if _, ok := list.Sessions[target.Session.Alias]; ok {
		t.Fatalf("active session list includes stopped target %#v", list.Sessions[target.Session.Alias])
	}
	if _, ok := list.Sessions[other.Session.Alias]; !ok {
		t.Fatalf("active session list missing active session %s", other.Session.Alias)
	}
}

func TestRunnerSessionEndIfActiveNoopsInSQLiteState(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "end", "--if-active", "--json"})
	if err != nil {
		t.Fatalf("session end --if-active --json error = %v", err)
	}
	ended := decodeSessionEnd(t, stdout.Bytes())
	if ended.Action != state.SessionEndActionNoop || ended.NoopReason == "" {
		t.Fatalf("ended = %#v, want noop with reason", ended)
	}
}

func TestRunnerSessionEndUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-active.md", `---
status: active
branch: main
claude_session_id: markdown-end-111111
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "end", "--json", "--session-id", "markdown-end-111111"})
	if err != nil {
		t.Fatalf("session end markdown --json error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=session end") {
		t.Fatalf("stdout = %q, want native markdown end without legacy delegation", stdout.String())
	}
	ended := decodeSessionEnd(t, stdout.Bytes())
	if ended.Action != state.SessionEndActionStopped || ended.Session.Alias != "20260610-active" || ended.Session.Status != "stopped" || len(ended.JournalEntryIDs) != 2 {
		t.Fatalf("ended = %#v, want stopped markdown session", ended)
	}
	stopped := readCLIAgentsFile(t, workingDir, "sessions/20260610-active.md")
	for _, want := range []string{"status: stopped", "session(end): session ended", "session(stop):   === SESSION STOPPED ==="} {
		if !strings.Contains(stopped, want) {
			t.Fatalf("stopped markdown = %q, want %q", stopped, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)

	var noopOut bytes.Buffer
	err = Runner{
		Stdout:     &noopOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "end", "--if-active", "--json"})
	if err != nil {
		t.Fatalf("session end markdown --if-active --json error = %v", err)
	}
	noop := decodeSessionEnd(t, noopOut.Bytes())
	if noop.Action != state.SessionEndActionNoop || noop.NoopReason == "" {
		t.Fatalf("noop = %#v, want noop when no active markdown session exists", noop)
	}
}

func TestRunnerSessionEndMarkdownClearAndWrap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-clear.md", `---
status: active
branch: main
claude_session_id: markdown-clear
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)

	var clearOut bytes.Buffer
	err := Runner{
		Stdout:     &clearOut,
		Stdin:      strings.NewReader(`{"session_id":"markdown-clear","reason":"clear"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "end", "--json"})
	if err != nil {
		t.Fatalf("session end markdown clear error = %v", err)
	}
	cleared := decodeSessionEnd(t, clearOut.Bytes())
	if cleared.Action != state.SessionEndActionCleared || cleared.Session.Status != "active" {
		t.Fatalf("cleared = %#v, want active clear marker", cleared)
	}
	clearMarkdown := readCLIAgentsFile(t, workingDir, "sessions/20260610-clear.md")
	for _, want := range []string{"status: active", "session(clear):  === CONTEXT CLEARED ==="} {
		if !strings.Contains(clearMarkdown, want) {
			t.Fatalf("clear markdown = %q, want %q", clearMarkdown, want)
		}
	}

	var wrapOut bytes.Buffer
	err = Runner{
		Stdout:     &wrapOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "end", "--wrap", "--json", "--session-id", "markdown-clear"})
	if err != nil {
		t.Fatalf("session end markdown wrap error = %v", err)
	}
	wrapped := decodeSessionEnd(t, wrapOut.Bytes())
	if wrapped.Action != state.SessionEndActionDone || wrapped.Session.Status != "done" {
		t.Fatalf("wrapped = %#v, want done session", wrapped)
	}
	wrapMarkdown := readCLIAgentsFile(t, workingDir, "sessions/20260610-clear.md")
	for _, want := range []string{"status: done", "session(wrap): session ended"} {
		if !strings.Contains(wrapMarkdown, want) {
			t.Fatalf("wrap markdown = %q, want %q", wrapMarkdown, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSessionArchiveTargetsHarnessSessionInSQLiteState(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	target, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "feature/archive-target",
		HarnessSessionID: "archive-target",
	})
	if err != nil {
		t.Fatalf("target StartSession() error = %v", err)
	}
	other, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "archive-main",
	})
	if err != nil {
		t.Fatalf("main StartSession() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "archive", "--json", "--session-id", "archive-target"})
	if err != nil {
		t.Fatalf("session archive --json --session-id error = %v", err)
	}
	archived := decodeSessionArchive(t, stdout.Bytes())
	if archived.Action != state.SessionArchiveActionArchived || archived.Session.ID != target.Session.ID || archived.Session.Status != "archived" {
		t.Fatalf("archived = %#v, want archived target session", archived)
	}

	var listOut bytes.Buffer
	if err := (Runner{Stdout: &listOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "list", "--json"}); err != nil {
		t.Fatalf("session list --json error = %v", err)
	}
	activeOnly := decodeSessionList(t, listOut.Bytes())
	if _, ok := activeOnly.Sessions[target.Session.Alias]; ok {
		t.Fatalf("active session list includes archived target %#v", activeOnly.Sessions[target.Session.Alias])
	}
	if _, ok := activeOnly.Sessions[other.Session.Alias]; !ok {
		t.Fatalf("active session list missing active session %s", other.Session.Alias)
	}

	var allOut bytes.Buffer
	if err := (Runner{Stdout: &allOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "list", "--json", "--all"}); err != nil {
		t.Fatalf("session list --json --all error = %v", err)
	}
	all := decodeSessionList(t, allOut.Bytes())
	if all.Sessions[target.Session.Alias].Status != "archived" {
		t.Fatalf("archived session = %#v, want archived status", all.Sessions[target.Session.Alias])
	}
}

func TestRunnerSessionArchiveUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
spec: SPEC-123
claude_session_id: markdown-archive-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
[2026-06-10 10:01] decision(archive): keep this decision visible
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-123-archive.md", `---
status: implementing
---
# Archive Spec

## Changelog

- existing entry
`)
	writeCLIAgentsFile(t, workingDir, "tmp/markdown-archive-session-enrichment.txt", "temporary\n")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "archive", "--session-id", "markdown-archive-session"})
	if err != nil {
		t.Fatalf("session archive markdown error = %v", err)
	}
	for _, want := range []string{
		"loaf session archive",
		"decision: [2026-06-10 10:01] decision(archive): keep this decision visible",
		"Appended decisions to SPEC-123-archive.md",
		"Archived: .agents/sessions/archive/20260610-session.md",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "sessions", "20260610-session.md")); !os.IsNotExist(err) {
		t.Fatalf("active session stat = %v, want moved from active sessions dir", err)
	}
	archived := readCLIAgentsFile(t, workingDir, "sessions/archive/20260610-session.md")
	for _, want := range []string{
		"status: archived",
		"archived_at:",
		"[2026-06-10 10:01] decision(archive): keep this decision visible",
	} {
		if !strings.Contains(archived, want) {
			t.Fatalf("archived markdown = %q, want %q", archived, want)
		}
	}
	if strings.Contains(archived, "last_updated: 2026-06-10T10:00:00Z") {
		t.Fatalf("archived markdown = %q, want updated last_updated", archived)
	}
	spec := readCLIAgentsFile(t, workingDir, "specs/SPEC-123-archive.md")
	for _, want := range []string{
		"## Changelog",
		"Session main archived: 1 decision(s) extracted",
		"[2026-06-10 10:01] decision(archive): keep this decision visible",
		"- existing entry",
	} {
		if !strings.Contains(spec, want) {
			t.Fatalf("spec markdown = %q, want %q", spec, want)
		}
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "tmp", "markdown-archive-session-enrichment.txt")); !os.IsNotExist(err) {
		t.Fatalf("enrichment temp stat = %v, want removed", err)
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown archive not to create SQLite database", err)
	}
}

func TestRunnerSessionArchiveAdoptsMostRecentMarkdownSession(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-older-session.md", `---
status: active
branch: older-branch
created: 2026-06-10T09:00:00Z
last_updated: 2026-06-10T09:00:00Z
---
# Session

## Journal
`)
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-newer-session.md", `---
status: active
branch: newer-branch
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:30:00Z
---
# Session

## Journal
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stderr:     &stderr,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "archive"})
	if err != nil {
		t.Fatalf("session archive markdown adoption error = %v", err)
	}
	if !strings.Contains(stderr.String(), "WARN: no session for branch 'main'; logging to most-recent active session '20260610-newer-session.md' (origin branch 'newer-branch')") {
		t.Fatalf("stderr = %q, want most-recent active adoption warning", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "sessions", "20260610-newer-session.md")); !os.IsNotExist(err) {
		t.Fatalf("newer active session stat = %v, want archived", err)
	}
	newer := readCLIAgentsFile(t, workingDir, "sessions/archive/20260610-newer-session.md")
	if !strings.Contains(newer, "status: archived") {
		t.Fatalf("newer archived markdown = %q, want archived status", newer)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "sessions", "20260610-older-session.md")); err != nil {
		t.Fatalf("older active session stat = %v, want still active", err)
	}
}

func TestRunnerStateMigrateMarkdownJSONDryRunDoesNotCreateDatabase(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-idea.md", "# Idea\n")
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", "[2026-05-28 10:00] spark(scope): capture this\n")
	writeCLIAgentsFile(t, workingDir, "reports/report.md", "# Report\n")
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-topic.md", "# Brainstorm\n")
	writeCLIAgentsFile(t, workingDir, "tmp/unknown.txt", "skip me\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"spec":"SPEC-001","depends_on":["TASK-000"]}}}`)

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := state.PathResolver{StateHome: stateHome}.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "migrate", "markdown", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("state migrate markdown --dry-run --json error = %v", err)
	}

	plan := decodeMarkdownMigrationPlan(t, stdout.Bytes())
	if plan.Specs != 1 ||
		plan.Tasks != 1 ||
		plan.Ideas != 1 ||
		plan.Sparks != 1 ||
		plan.Brainstorms != 1 ||
		plan.Sessions != 1 ||
		plan.Reports != 1 {
		t.Fatalf("plan = %#v, want one of every dry-run artifact class", plan)
	}
	if plan.Relationships != 2 {
		t.Fatalf("Relationships = %d, want 2", plan.Relationships)
	}
	if len(plan.SkippedFiles) != 1 || plan.SkippedFiles[0] != ".agents/tmp/unknown.txt" {
		t.Fatalf("SkippedFiles = %#v, want unknown file", plan.SkippedFiles)
	}
	if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
		t.Fatalf("database parent exists after dry-run; err = %v", err)
	}
}

func TestRunnerStateMigrateMarkdownHumanDryRun(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-idea.md", "# Idea\n")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
	}.Run([]string{"state", "migrate", "markdown", "--dry-run"})
	if err != nil {
		t.Fatalf("state migrate markdown --dry-run error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "loaf state migrate markdown --dry-run") {
		t.Fatalf("output = %q, want dry-run heading", output)
	}
	if !strings.Contains(output, "ideas: 1") {
		t.Fatalf("output = %q, want idea count", output)
	}
}

func TestRunnerInitializedMutationCommandsWriteThroughSQLite(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-active.md", `---
id: SPEC-001
title: Active Spec
status: implementing
---
# Active Spec
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-complete.md", `---
id: SPEC-002
title: Complete Spec
status: complete
---
# Complete Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-base.md", "# Base Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-done.md", "# Done Task\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-source-idea.md", `---
title: Source Idea
status: open
---
# Source Idea
`)
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", `---
title: Target Idea
status: open
---
# Target Idea
`)
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-matrix.md", `---
title: Matrix Brainstorm
status: open
---
# Matrix Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): matrix spark\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 3,
  "tasks": {
    "TASK-001": {"title": "Base Task", "spec": "SPEC-001", "status": "todo", "priority": "P2", "file": "TASK-001-base.md"},
    "TASK-002": {"title": "Done Task", "spec": "SPEC-002", "status": "done", "priority": "P2", "file": "TASK-002-done.md"}
  },
  "specs": {}
}`)

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}

	run := func(args ...string) {
		t.Helper()
		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run(args); err != nil {
			t.Fatalf("loaf %s error = %v\nstdout:\n%s", strings.Join(args, " "), err, stdout.String())
		}
	}

	run("state", "migrate", "markdown", "--apply")
	run("task", "create", "--title", "Matrix Task", "--spec", "SPEC-001", "--json")
	run("task", "update", "TASK-001", "--status", "in_progress", "--json")
	run("task", "archive", "TASK-002", "--json")
	run("idea", "capture", "--title", "Matrix Idea", "--json")
	run("idea", "promote", "20260528-source-idea", "--to-spec", "SPEC-001", "--json")
	run("idea", "resolve", "20260528-source-idea", "--by", "SPEC-001", "--json")
	run("spark", "capture", "--scope", "matrix", "--text", "Matrix Spark", "--json")
	run("spark", "promote", "SPARK-matrix", "--to-idea", "20260528-target-idea", "--json")
	run("spark", "resolve", "SPARK-matrix", "--by", "20260528-target-idea", "--reason", "matrix resolved", "--json")
	run("brainstorm", "promote", "20260528-brainstorm-matrix", "--to-idea", "20260528-target-idea", "--json")
	run("brainstorm", "archive", "20260528-brainstorm-matrix", "--reason", "matrix archived", "--json")
	run("spec", "archive", "SPEC-002", "--json")
	run("session", "log", "--json", "--session-id", "matrix-harness", "decision(sqlite): matrix write")
	run("tag", "add", "SPEC-001", "matrix", "--json")
	run("tag", "remove", "SPEC-001", "matrix", "--json")
	run("bundle", "create", "matrix-bundle", "--tag", "matrix", "--json")
	run("bundle", "add", "matrix-bundle", "TASK-001", "--json")
	run("bundle", "remove", "matrix-bundle", "TASK-001", "--json")
	run("link", "create", "20260528-target-idea", "SPEC-001", "--type", "resolved_by", "--reason", "matrix link", "--json")
	run("link", "remove", "20260528-target-idea", "SPEC-001", "--type", "resolved_by", "--json")

	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM tasks WHERE title = ?`, "Matrix Task"); got != 1 {
		t.Fatalf("Matrix Task count = %d, want 1", got)
	}
	if got := sqliteEntityStatus(t, db, "tasks", "task", "TASK-001"); got != "in_progress" {
		t.Fatalf("TASK-001 status = %q, want in_progress", got)
	}
	if got := sqliteEntityStatus(t, db, "tasks", "task", "TASK-002"); got != "archived" {
		t.Fatalf("TASK-002 status = %q, want archived", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM ideas WHERE title = ?`, "Matrix Idea"); got != 1 {
		t.Fatalf("Matrix Idea count = %d, want 1", got)
	}
	if got := sqliteEntityStatus(t, db, "ideas", "idea", "20260528-source-idea"); got != "resolved" {
		t.Fatalf("20260528-source-idea status = %q, want resolved", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM sparks WHERE text = ? AND scope = ?`, "Matrix Spark", "matrix"); got != 1 {
		t.Fatalf("captured Matrix Spark count = %d, want 1", got)
	}
	if got := sqliteEntityStatus(t, db, "sparks", "spark", "SPARK-matrix"); got != "resolved" {
		t.Fatalf("SPARK-matrix status = %q, want resolved", got)
	}
	if got := sqliteEntityStatus(t, db, "brainstorms", "brainstorm", "20260528-brainstorm-matrix"); got != "archived" {
		t.Fatalf("20260528-brainstorm-matrix status = %q, want archived", got)
	}
	if got := sqliteEntityStatus(t, db, "specs", "spec", "SPEC-002"); got != "archived" {
		t.Fatalf("SPEC-002 status = %q, want archived", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM journal_entries WHERE entry_type = ? AND scope = ? AND message = ? AND harness_session_id = ?`, "decision", "sqlite", "matrix write", "matrix-harness"); got != 1 {
		t.Fatalf("journal entry count = %d, want 1", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM tags WHERE name = ?`, "matrix"); got != 1 {
		t.Fatalf("matrix tag count = %d, want 1", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM entity_tags`); got != 0 {
		t.Fatalf("entity_tags count = %d, want 0 after tag remove", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM bundles WHERE slug = ? AND tag_query = ?`, "matrix-bundle", "matrix"); got != 1 {
		t.Fatalf("matrix-bundle count = %d, want 1", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM bundle_members`); got != 0 {
		t.Fatalf("bundle_members count = %d, want 0 after bundle remove", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM relationships WHERE relationship_type = ?`, "promoted_to"); got != 3 {
		t.Fatalf("promoted_to relationship count = %d, want 3", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM relationships WHERE relationship_type = ? AND reason = ?`, "resolved_by", "matrix link"); got != 0 {
		t.Fatalf("matrix resolved_by link count = %d, want 0 after link remove", got)
	}
}

func TestRunnerStateMigrateMarkdownApplyJSON(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"spec":"SPEC-001"}}}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "migrate", "markdown", "--apply", "--json"})
	if err != nil {
		t.Fatalf("state migrate markdown --apply --json error = %v", err)
	}

	result := decodeMarkdownMigrationResult(t, stdout.Bytes())
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if _, err := os.Stat(result.DatabasePath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
	if result.Specs != 1 || result.Tasks != 1 || result.Relationships != 1 {
		t.Fatalf("result = %#v, want imported spec, task, and relationship counts", result)
	}
}

func TestRunnerStateMigrateMarkdownResumeJSON(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-resume.md", "# Resume Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-resume.md", "# Resume Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"spec":"SPEC-001"}}}`)

	var firstStdout bytes.Buffer
	err := Runner{
		Stdout:     &firstStdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "migrate", "markdown", "--resume", "--json"})
	if err != nil {
		t.Fatalf("state migrate markdown --resume --json error = %v", err)
	}

	firstResult := decodeMarkdownMigrationResult(t, firstStdout.Bytes())
	if !firstResult.Applied {
		t.Fatal("Applied = false, want true")
	}
	if firstResult.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if firstResult.Specs != 1 || firstResult.Tasks != 1 || firstResult.Relationships != 1 {
		t.Fatalf("first result = %#v, want imported spec, task, and relationship counts", firstResult)
	}

	var secondStdout bytes.Buffer
	err = Runner{
		Stdout:     &secondStdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "migrate", "markdown", "--resume", "--json"})
	if err != nil {
		t.Fatalf("second state migrate markdown --resume --json error = %v", err)
	}

	secondResult := decodeMarkdownMigrationResult(t, secondStdout.Bytes())
	if secondResult.DatabasePath != firstResult.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", secondResult.DatabasePath, firstResult.DatabasePath)
	}
	if secondResult.Specs != 1 || secondResult.Tasks != 1 || secondResult.Relationships != 1 {
		t.Fatalf("second result = %#v, want idempotent imported counts", secondResult)
	}
}

func TestRunnerStateMigrateMarkdownResumeHuman(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-resume-idea.md", "# Resume Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"state", "migrate", "markdown", "--resume"})
	if err != nil {
		t.Fatalf("state migrate markdown --resume error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "loaf state migrate markdown --resume") {
		t.Fatalf("output = %q, want resume command label", output)
	}
	if !strings.Contains(output, "database: ") {
		t.Fatalf("output = %q, want database path", output)
	}
	if !strings.Contains(output, "ideas: 1") {
		t.Fatalf("output = %q, want idea count", output)
	}
}

func TestRunnerStateMigrateMarkdownRejectsApplyWithDryRun(t *testing.T) {
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: realpath(t, t.TempDir()),
		StateHome:  t.TempDir(),
	}.Run([]string{"state", "migrate", "markdown", "--apply", "--dry-run"})
	if err == nil {
		t.Fatal("state migrate markdown --apply --dry-run error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "cannot combine --apply and --dry-run") {
		t.Fatalf("error = %v, want apply/dry-run rejection", err)
	}
}

func TestRunnerStateMigrateMarkdownResumeRejectsFlagCombinations(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "dry-run",
			args:    []string{"state", "migrate", "markdown", "--resume", "--dry-run"},
			wantErr: "cannot combine --resume and --dry-run",
		},
		{
			name:    "apply",
			args:    []string{"state", "migrate", "markdown", "--resume", "--apply"},
			wantErr: "cannot combine --resume and --apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Runner{
				Stdout:     &bytes.Buffer{},
				WorkingDir: realpath(t, t.TempDir()),
				StateHome:  t.TempDir(),
			}.Run(tt.args)
			if err == nil {
				t.Fatal("state migrate markdown --resume error = nil, want rejection")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunnerTraceJSONUsesSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","spec":"SPEC-001","status":"todo","depends_on":["TASK-000"]}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("trace TASK-001 --json error = %v", err)
	}

	trace := decodeTraceResult(t, stdout.Bytes())
	if trace.Entity.Kind != "task" || trace.Entity.Alias != "TASK-001" || trace.Entity.Title != "Example Task" {
		t.Fatalf("Entity = %#v, want imported task", trace.Entity)
	}
	if !hasTraceRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-001") {
		t.Fatalf("Relationships = %#v, want task implements spec", trace.Relationships)
	}
	if !hasTraceRelationship(trace.Relationships, "outbound", "blocked_by", "task", "TASK-000") {
		t.Fatalf("Relationships = %#v, want task dependency alias", trace.Relationships)
	}
}

func TestRunnerTraceHumanMissingDatabase(t *testing.T) {
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: realpath(t, t.TempDir()),
		StateHome:  t.TempDir(),
	}.Run([]string{"trace", "TASK-001"})
	if err == nil {
		t.Fatal("trace error = nil, want missing DB error")
	}
	if !strings.Contains(err.Error(), "loaf state migrate markdown --apply") {
		t.Fatalf("error = %v, want migration hint", err)
	}
}

func TestRunnerTaskListJSONUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-done.md", "# Done\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Example Task", "spec": "SPEC-001", "status": "todo", "priority": "P1", "depends_on": ["TASK-000"]},
    "TASK-002": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2", "depends_on": []}
  }
}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--json", "--active"})
	if err != nil {
		t.Fatalf("task list --json --active error = %v", err)
	}

	tasks := decodeTaskList(t, stdout.Bytes())
	if _, ok := tasks.Tasks["TASK-002"]; ok {
		t.Fatal("active task list includes done task")
	}
	task := tasks.Tasks["TASK-001"]
	if task.Title != "Example Task" || task.Spec != "SPEC-001" || task.Priority != "P1" || task.SourcePath != ".agents/tasks/TASK-001-example.md" {
		t.Fatalf("TASK-001 = %#v, want imported task metadata", task)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "TASK-000" {
		t.Fatalf("TASK-001 DependsOn = %#v, want TASK-000", task.DependsOn)
	}
}

func TestRunnerTaskListHumanUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","spec":"SPEC-001","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--status", "todo"})
	if err != nil {
		t.Fatalf("task list --status todo error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "loaf task list") || !strings.Contains(output, "TASK-001") || !strings.Contains(output, "Example Task") {
		t.Fatalf("output = %q, want state-backed task list", output)
	}
}

func TestRunnerTaskStatusUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-implementing.md", `---
status: implementing
---
# Implementing
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-complete.md", `---
status: complete
---
# Complete
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-todo.md", "# Todo\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-progress.md", "# Progress\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-003-done.md", "# Done\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Todo Task","spec":"SPEC-001","status":"todo","priority":"P1"},
  "TASK-002":{"title":"Progress Task","spec":"SPEC-001","status":"in_progress","priority":"P2"},
  "TASK-003":{"title":"Done Task","spec":"SPEC-002","status":"done","priority":"P3"}
}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "status"})
	if err != nil {
		t.Fatalf("task status error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"loaf task status",
		"Tasks:",
		"1 in_progress",
		"0 blocked",
		"1 todo",
		"0 review",
		"1 done",
		"(3 total)",
		"Specs:",
		"1 implementing",
		"0 approved",
		"0 drafting",
		"1 complete",
		"(2 total)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerTaskCreateUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-existing.md", "# Existing\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"version":1,"next_id":2,"tasks":{"TASK-001":{"title":"Existing Task","spec":"SPEC-001","status":"todo","priority":"P2","file":"TASK-001-existing.md"}},"specs":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var createOut bytes.Buffer
	err := Runner{
		Stdout:     &createOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Created Task", "--spec", "SPEC-001", "--priority", "P1", "--depends-on", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task create --json error = %v", err)
	}
	created := decodeTaskCreateResult(t, createOut.Bytes())
	if created.Task.Alias != "TASK-002" || created.Task.Title != "Created Task" || created.Task.Status != "todo" || created.Priority != "P1" || created.Spec.Alias != "SPEC-001" || created.EventID == "" {
		t.Fatalf("created = %#v, want TASK-002 under SPEC-001", created)
	}
	if len(created.Depends) != 1 || created.Depends[0].Alias != "TASK-001" {
		t.Fatalf("created.Depends = %#v, want TASK-001", created.Depends)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-002", "--json"})
	if err != nil {
		t.Fatalf("task show created error = %v", err)
	}
	show := decodeTaskShow(t, showOut.Bytes())
	if show.Task.Title != "Created Task" || show.Task.Priority != "P1" || show.Task.Spec != "SPEC-001" || len(show.Task.DependsOn) != 1 || show.Task.DependsOn[0] != "TASK-001" {
		t.Fatalf("show = %#v, want created task details", show)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "TASK-002", "--json"})
	if err != nil {
		t.Fatalf("trace created task error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if !hasTraceRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-001") {
		t.Fatalf("trace relationships = %#v, want implements SPEC-001", trace.Relationships)
	}
	if !hasTraceRelationship(trace.Relationships, "outbound", "blocked_by", "task", "TASK-001") {
		t.Fatalf("trace relationships = %#v, want blocked_by TASK-001", trace.Relationships)
	}
}

func TestRunnerTaskCreateHumanUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Human Task"})
	if err != nil {
		t.Fatalf("task create human error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"created task TASK-001: Human Task", "status: todo", "priority: P2", "event:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerTaskShowJSONUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", `---
id: TASK-001
---
# Task Body

Imported body.
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-dependency.md", "# Dependency\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Example Task", "spec": "SPEC-001", "status": "todo", "priority": "P1", "depends_on": ["TASK-002"]},
    "TASK-002": {"title": "Dependency Task", "status": "todo"}
  }
}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task show --json error = %v", err)
	}

	show := decodeTaskShow(t, stdout.Bytes())
	task := show.Task
	if show.Query != "TASK-001" || task.Alias != "TASK-001" || task.Title != "Example Task" || task.Status != "todo" || task.Priority != "P1" || task.Spec != "SPEC-001" {
		t.Fatalf("show = %#v, want imported TASK-001 details", show)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "TASK-002" {
		t.Fatalf("DependsOn = %#v, want TASK-002", task.DependsOn)
	}
	if len(task.Sources) != 1 || task.Sources[0].Path != ".agents/tasks/TASK-001-example.md" || task.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want task source", task.Sources)
	}
	if !strings.Contains(task.Body, "Imported body.") || strings.Contains(task.Body, "---") {
		t.Fatalf("Body = %q, want imported body without frontmatter", task.Body)
	}
}

func TestRunnerTaskShowHumanUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task Body\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","spec":"SPEC-001","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001"})
	if err != nil {
		t.Fatalf("task show error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"task TASK-001", "title: Example Task", "status: todo", "priority: P1", "spec: SPEC-001", "source: .agents/tasks/TASK-001-example.md", "# Task Body"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerTaskListUsesMarkdownTasksWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", `---
id: TASK-001
title: Frontmatter Title
status: blocked
priority: P9
spec: SPEC-X
depends_on: TASK-999
---
# Task Body
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-done.md", "# Done task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "Example Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1",
      "depends_on": ["TASK-000"]
    },
    "TASK-002": {
      "title": "Done Task",
      "spec": "SPEC-001",
      "status": "done",
      "priority": "P2",
      "depends_on": []
    }
  }
}`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--json", "--active"})
	if err != nil {
		t.Fatalf("task list markdown --json --active error = %v", err)
	}
	tasks := decodeTaskList(t, jsonOut.Bytes())
	if _, ok := tasks.Tasks["TASK-002"]; ok {
		t.Fatalf("active tasks = %#v, want done task filtered out", tasks.Tasks)
	}
	task := tasks.Tasks["TASK-001"]
	if task.Title != "Example Task" || task.Status != "todo" || task.Priority != "P1" || task.Spec != "SPEC-001" || task.SourcePath != ".agents/tasks/TASK-001-example.md" {
		t.Fatalf("TASK-001 = %#v, want TASKS.json metadata over frontmatter", task)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "TASK-000" {
		t.Fatalf("TASK-001 DependsOn = %#v, want TASK-000", task.DependsOn)
	}

	var doneOut bytes.Buffer
	err = Runner{
		Stdout:     &doneOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--json", "--status", "done"})
	if err != nil {
		t.Fatalf("task list markdown --status done error = %v", err)
	}
	doneTasks := decodeTaskList(t, doneOut.Bytes())
	if len(doneTasks.Tasks) != 1 || doneTasks.Tasks["TASK-002"].Status != "done" {
		t.Fatalf("done tasks = %#v, want only TASK-002", doneTasks.Tasks)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--active"})
	if err != nil {
		t.Fatalf("task list markdown human error = %v", err)
	}
	output := humanOut.String()
	for _, want := range []string{"loaf task list", "Todo (1)", "TASK-001", "P1", "Example Task", "SPEC-001", "Total: 1 active tasks across 1 specs"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskStatusUsesMarkdownStateWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-done.md", `---
id: SPEC-002
title: Done Spec
status: complete
---
# Done Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-todo.md", "# Todo task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-progress.md", "# Progress task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-003-done.md", "# Done task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "status"})
	if err != nil {
		t.Fatalf("task status markdown error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"loaf task status", "Tasks:", "1 in_progress", "1 todo", "1 done", "(3 total)", "Specs:", "1 implementing", "1 complete", "(2 total)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskCreateUsesMarkdownIndexWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-existing.md", "# Existing\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 2,
  "tasks": {
    "TASK-001": {
      "title": "Existing Task",
      "slug": "existing-task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P2",
      "depends_on": [],
      "files": ["keep.go"],
      "verify": null,
      "done": null,
      "session": null,
      "created": "2026-05-01T10:00:00Z",
      "updated": "2026-05-01T10:00:00Z",
      "completed_at": null,
      "file": "TASK-001-existing.md"
    }
  },
  "specs": {
    "SPEC-001": {
      "title": "Example Spec",
      "status": "implementing",
      "file": "SPEC-001-example.md",
      "requirement": "preserve spec field"
    }
  },
  "custom_root": "preserve me"
}`)
	var createOut bytes.Buffer
	err := Runner{
		Stdout:     &createOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Created Task!", "--spec", "SPEC-001", "--priority", "P1", "--depends-on", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task create markdown --json error = %v", err)
	}
	created := decodeTaskCreateResult(t, createOut.Bytes())
	if created.Task.Alias != "TASK-002" || created.Task.Title != "Created Task!" || created.Task.Status != "todo" || created.Priority != "P1" || created.Spec.Alias != "SPEC-001" {
		t.Fatalf("created = %#v, want TASK-002 under SPEC-001", created)
	}
	if len(created.Depends) != 1 || created.Depends[0].Alias != "TASK-001" {
		t.Fatalf("created.Depends = %#v, want TASK-001", created.Depends)
	}

	var index map[string]any
	rawIndex, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json) error = %v", err)
	}
	if index["custom_root"] != "preserve me" {
		t.Fatalf("index custom_root = %#v, want preserved", index["custom_root"])
	}
	if int(index["next_id"].(float64)) != 3 {
		t.Fatalf("next_id = %#v, want 3", index["next_id"])
	}
	tasks := index["tasks"].(map[string]any)
	task := tasks["TASK-002"].(map[string]any)
	if task["title"] != "Created Task!" || task["slug"] != "created-task" || task["status"] != "todo" || task["priority"] != "P1" || task["spec"] != "SPEC-001" || task["file"] != "TASK-002-created-task.md" {
		t.Fatalf("TASK-002 index = %#v, want created metadata", task)
	}
	deps := task["depends_on"].([]any)
	if len(deps) != 1 || deps[0] != "TASK-001" {
		t.Fatalf("depends_on = %#v, want TASK-001", deps)
	}
	existing := tasks["TASK-001"].(map[string]any)
	files := existing["files"].([]any)
	if len(files) != 1 || files[0] != "keep.go" {
		t.Fatalf("existing task = %#v, want unknown fields preserved", existing)
	}
	spec := index["specs"].(map[string]any)["SPEC-001"].(map[string]any)
	if spec["requirement"] != "preserve spec field" {
		t.Fatalf("spec = %#v, want unknown spec fields preserved", spec)
	}

	taskFile := filepath.Join(workingDir, ".agents", "tasks", "TASK-002-created-task.md")
	body, err := os.ReadFile(taskFile)
	if err != nil {
		t.Fatalf("ReadFile(created task) error = %v", err)
	}
	frontmatter, ok := parseKnowledgeFrontmatter(body)
	if !ok {
		t.Fatal("created task frontmatter missing")
	}
	if firstFieldValue(frontmatter["id"]) != "TASK-002" || firstFieldValue(frontmatter["title"]) != "Created Task!" || firstFieldValue(frontmatter["spec"]) != "SPEC-001" || !frontmatter["depends_on"].Array || strings.Join(frontmatter["depends_on"].Values, ",") != "TASK-001" {
		t.Fatalf("frontmatter = %#v, want created task metadata", frontmatter)
	}
	content := markdownContentWithoutFrontmatter(string(body))
	if !strings.Contains(content, "# TASK-002: Created Task!") || !strings.Contains(content, "## Acceptance Criteria") || !strings.Contains(content, "## Verification") {
		t.Fatalf("content = %q, want task scaffold body", content)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Human Task"})
	if err != nil {
		t.Fatalf("task create markdown human error = %v", err)
	}
	if !strings.Contains(humanOut.String(), "created task TASK-003: Human Task") || !strings.Contains(humanOut.String(), "priority: P2") {
		t.Fatalf("human output = %q, want created task summary", humanOut.String())
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Bad", "--spec", "SPEC-999"})
	if err == nil || !strings.Contains(err.Error(), "Spec \"SPEC-999\" not found in index") {
		t.Fatalf("missing spec error = %v, want index validation", err)
	}
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Bad", "--depends-on", "TASK-999"})
	if err == nil || !strings.Contains(err.Error(), "Dependency \"TASK-999\" not found in index") {
		t.Fatalf("missing dependency error = %v, want index validation", err)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskShowUsesMarkdownTaskWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", `---
id: TASK-001
title: Frontmatter Title
status: blocked
priority: P9
spec: SPEC-X
depends_on: TASK-999
session: old-session
---
# Task Body

Markdown details.
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "Example Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1",
      "depends_on": ["TASK-000"],
      "session": "20260528-session",
      "created": "2026-05-28T10:00:00Z",
      "updated": "2026-05-29T11:00:00Z"
    }
  }
}`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task show markdown --json error = %v", err)
	}
	show := decodeTaskShow(t, jsonOut.Bytes())
	task := show.Task
	if show.Query != "TASK-001" || task.Alias != "TASK-001" || task.Title != "Example Task" || task.Status != "todo" || task.Priority != "P1" || task.Spec != "SPEC-001" {
		t.Fatalf("show = %#v, want TASKS.json metadata over frontmatter", show)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "TASK-000" {
		t.Fatalf("DependsOn = %#v, want TASK-000", task.DependsOn)
	}
	if len(task.Sessions) != 1 || task.Sessions[0] != "20260528-session" {
		t.Fatalf("Sessions = %#v, want 20260528-session", task.Sessions)
	}
	if len(task.Sources) != 1 || task.Sources[0].Path != ".agents/tasks/TASK-001-example.md" || task.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want task source with hash", task.Sources)
	}
	if !strings.Contains(task.Body, "Markdown details.") || strings.Contains(task.Body, "---") {
		t.Fatalf("Body = %q, want markdown body without frontmatter", task.Body)
	}
	if task.CreatedAt != "2026-05-28T10:00:00Z" || task.UpdatedAt != "2026-05-29T11:00:00Z" {
		t.Fatalf("timestamps = %q/%q, want index timestamps", task.CreatedAt, task.UpdatedAt)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001"})
	if err != nil {
		t.Fatalf("task show markdown human error = %v", err)
	}
	output := humanOut.String()
	for _, want := range []string{"task TASK-001", "title: Example Task", "status: todo", "priority: P1", "spec: SPEC-001", "depends on: TASK-000", "sessions: 20260528-session", "source: .agents/tasks/TASK-001-example.md", "source hash:", "# Task Body", "Markdown details."} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskListReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list"})
	if err == nil {
		t.Fatal("task list error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001"})
	if err == nil {
		t.Fatal("task status error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerTaskCreateReportsValidationAndInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Bad", "--priority", "PX"})
	if err == nil {
		t.Fatal("task create invalid priority error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid priority") {
		t.Fatalf("error = %v, want invalid priority", err)
	}

	if _, err := parseTaskCreateArgs([]string{"--title", "--json"}); err == nil || !strings.Contains(err.Error(), "--title requires a value") {
		t.Fatalf("parseTaskCreateArgs flag value error = %v, want --title requires a value", err)
	}

	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "create", "--title", "Created"})
	if err == nil {
		t.Fatal("task create invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerTaskShowReportsInvalidSQLiteStateAndMissingTargets(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001"})
	if err == nil {
		t.Fatal("task show invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	stateHome = t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", "# Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","status":"todo"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "SPEC-001"})
	if err == nil {
		t.Fatal("task show non-task error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not task") {
		t.Fatalf("error = %v, want non-task rejection", err)
	}
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-999"})
	if err == nil {
		t.Fatal("task show missing error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestRunnerTaskUpdateStatusUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-status.md", "# Status Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Status Task","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var updateOut bytes.Buffer
	err := Runner{
		Stdout:     &updateOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--status", "in_progress", "--json"})
	if err != nil {
		t.Fatalf("task update --status error = %v", err)
	}
	updated := decodeTaskStatusUpdateResult(t, updateOut.Bytes())
	if updated.Task.Alias != "TASK-001" || updated.Previous != "todo" || updated.Status != "in_progress" || updated.EventID == "" {
		t.Fatalf("updated = %#v, want TASK-001 todo -> in_progress", updated)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--json", "--status", "in_progress"})
	if err != nil {
		t.Fatalf("task list after update error = %v", err)
	}
	tasks := decodeTaskList(t, listOut.Bytes())
	if tasks.Tasks["TASK-001"].Status != "in_progress" {
		t.Fatalf("TASK-001 = %#v, want in_progress", tasks.Tasks["TASK-001"])
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("trace after update error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "in_progress" {
		t.Fatalf("trace entity = %#v, want in_progress", trace.Entity)
	}
}

func TestRunnerTaskUpdateMetadataUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-original.md", "# Original Spec\n")
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-new.md", "# New Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-update.md", "# Updated Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-dependency.md", "# Dependency Task\n")
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", "# Session\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Updated Task","spec":"SPEC-001","status":"todo","priority":"P2"},
  "TASK-002":{"title":"Dependency Task","status":"todo","priority":"P3"}
}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var updateOut bytes.Buffer
	err := Runner{
		Stdout:     &updateOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--priority", "P0", "--spec", "SPEC-002", "--depends-on", "TASK-002", "--session", "20260528-session", "--json"})
	if err != nil {
		t.Fatalf("task update metadata error = %v", err)
	}
	updated := decodeTaskStatusUpdateResult(t, updateOut.Bytes())
	if updated.Priority != "P0" || updated.Spec == nil || updated.Spec.Alias != "SPEC-002" || updated.Session == nil || updated.Session.Alias != "20260528-session" {
		t.Fatalf("updated = %#v, want priority/spec/session update", updated)
	}
	if len(updated.Depends) != 1 || updated.Depends[0].Alias != "TASK-002" {
		t.Fatalf("updated.Depends = %#v, want TASK-002", updated.Depends)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task show after metadata update error = %v", err)
	}
	show := decodeTaskShow(t, showOut.Bytes())
	if show.Task.Priority != "P0" || show.Task.Spec != "SPEC-002" || len(show.Task.DependsOn) != 1 || show.Task.DependsOn[0] != "TASK-002" || len(show.Task.Sessions) != 1 || show.Task.Sessions[0] != "20260528-session" {
		t.Fatalf("show = %#v, want updated metadata", show)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("trace after metadata update error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if hasTraceRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-001") {
		t.Fatalf("trace relationships = %#v, still has old SPEC-001", trace.Relationships)
	}
	if !hasTraceRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-002") || !hasTraceRelationship(trace.Relationships, "outbound", "blocked_by", "task", "TASK-002") || !hasTraceRelationship(trace.Relationships, "outbound", "associated_with", "session", "20260528-session") {
		t.Fatalf("trace relationships = %#v, want spec, dependency, and session relationships", trace.Relationships)
	}

	var clearOut bytes.Buffer
	err = Runner{
		Stdout:     &clearOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--spec", "none", "--depends-on", "none", "--session", "none"})
	if err != nil {
		t.Fatalf("task update clear metadata error = %v", err)
	}
	output := clearOut.String()
	if !strings.Contains(output, "updated task TASK-001") || !strings.Contains(output, "priority: P0") {
		t.Fatalf("output = %q, want human update summary", output)
	}
	showOut.Reset()
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task show after clear error = %v", err)
	}
	show = decodeTaskShow(t, showOut.Bytes())
	if show.Task.Spec != "" || len(show.Task.DependsOn) != 0 || len(show.Task.Sessions) != 0 {
		t.Fatalf("show after clear = %#v, want cleared metadata", show)
	}
}

func TestRunnerTaskUpdateUsesMarkdownIndexWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-update.md", `---
id: TASK-001
title: Old Title
status: todo
priority: P3
spec: SPEC-001
depends_on: [TASK-002]
session: old-session
created: 2026-05-01T10:00:00Z
updated: 2026-05-01T10:00:00Z
---
# Task Body

Preserve this body.
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-dependency.md", "# Dependency\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-003-new-dependency.md", "# New Dependency\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 4,
  "tasks": {
    "TASK-001": {
      "title": "Updated Task",
      "slug": "updated-task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P2",
      "depends_on": ["TASK-002"],
      "files": ["keep.go"],
      "verify": "go test ./...",
      "done": null,
      "session": "old-session",
      "created": "2026-05-01T10:00:00Z",
      "updated": "2026-05-01T10:00:00Z",
      "completed_at": null,
      "file": "TASK-001-update.md"
    },
    "TASK-002": {"title": "Dependency Task", "status": "todo", "priority": "P2", "depends_on": [], "file": "TASK-002-dependency.md"},
    "TASK-003": {"title": "New Dependency", "status": "todo", "priority": "P2", "depends_on": [], "file": "TASK-003-new-dependency.md"}
  },
  "specs": {
    "SPEC-001": {"title": "Original Spec", "status": "implementing", "file": "SPEC-001-original.md"},
    "SPEC-002": {"title": "New Spec", "status": "approved", "file": "SPEC-002-new.md"}
  },
  "custom_root": "preserve me"
}`)

	var updateOut bytes.Buffer
	err := Runner{
		Stdout:     &updateOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--status", "done", "--priority", "P0", "--spec", "SPEC-002", "--depends-on", "TASK-003", "--session", "new-session", "--json"})
	if err != nil {
		t.Fatalf("task update markdown --json error = %v", err)
	}
	updated := decodeTaskStatusUpdateResult(t, updateOut.Bytes())
	if updated.Task.Alias != "TASK-001" || updated.Previous != "todo" || updated.Status != "done" || updated.Priority != "P0" || updated.Spec == nil || updated.Spec.Alias != "SPEC-002" || updated.Session == nil || updated.Session.Alias != "new-session" {
		t.Fatalf("updated = %#v, want markdown metadata update", updated)
	}
	if len(updated.Depends) != 1 || updated.Depends[0].Alias != "TASK-003" {
		t.Fatalf("updated.Depends = %#v, want TASK-003", updated.Depends)
	}

	rawIndex, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	var index map[string]any
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json) error = %v", err)
	}
	if index["custom_root"] != "preserve me" {
		t.Fatalf("index custom_root = %#v, want preserved", index["custom_root"])
	}
	task := index["tasks"].(map[string]any)["TASK-001"].(map[string]any)
	if task["status"] != "done" || task["priority"] != "P0" || task["spec"] != "SPEC-002" || task["session"] != "new-session" || task["completed_at"] == nil || task["verify"] != "go test ./..." {
		t.Fatalf("TASK-001 index = %#v, want updated metadata with unknown fields preserved", task)
	}
	deps := task["depends_on"].([]any)
	if len(deps) != 1 || deps[0] != "TASK-003" {
		t.Fatalf("depends_on = %#v, want TASK-003", deps)
	}
	body, err := os.ReadFile(filepath.Join(workingDir, ".agents", "tasks", "TASK-001-update.md"))
	if err != nil {
		t.Fatalf("ReadFile(updated task) error = %v", err)
	}
	frontmatter, ok := parseKnowledgeFrontmatter(body)
	if !ok {
		t.Fatal("updated task frontmatter missing")
	}
	if firstFieldValue(frontmatter["status"]) != "done" || firstFieldValue(frontmatter["priority"]) != "P0" || firstFieldValue(frontmatter["spec"]) != "SPEC-002" || firstFieldValue(frontmatter["session"]) != "new-session" || strings.Join(frontmatter["depends_on"].Values, ",") != "TASK-003" {
		t.Fatalf("frontmatter = %#v, want synced updated metadata", frontmatter)
	}
	if !strings.Contains(markdownContentWithoutFrontmatter(string(body)), "Preserve this body.") {
		t.Fatalf("body = %q, want preserved task body", markdownContentWithoutFrontmatter(string(body)))
	}

	var clearOut bytes.Buffer
	err = Runner{
		Stdout:     &clearOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--status", "todo", "--spec", "none", "--depends-on", "none", "--session", "none", "--json"})
	if err != nil {
		t.Fatalf("task update markdown clear error = %v", err)
	}
	cleared := decodeTaskStatusUpdateResult(t, clearOut.Bytes())
	if cleared.Previous != "done" || cleared.Status != "todo" || cleared.Spec != nil || cleared.Session != nil || len(cleared.Depends) != 0 {
		t.Fatalf("cleared = %#v, want cleared metadata", cleared)
	}
	rawIndex, err = os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json after clear) error = %v", err)
	}
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json after clear) error = %v", err)
	}
	task = index["tasks"].(map[string]any)["TASK-001"].(map[string]any)
	if task["status"] != "todo" || task["completed_at"] != nil || task["spec"] != nil || task["session"] != nil || len(task["depends_on"].([]any)) != 0 {
		t.Fatalf("TASK-001 after clear = %#v, want cleared index metadata", task)
	}
	body, err = os.ReadFile(filepath.Join(workingDir, ".agents", "tasks", "TASK-001-update.md"))
	if err != nil {
		t.Fatalf("ReadFile(cleared task) error = %v", err)
	}
	frontmatter, ok = parseKnowledgeFrontmatter(body)
	if !ok {
		t.Fatal("cleared task frontmatter missing")
	}
	if firstFieldValue(frontmatter["spec"]) != "" || firstFieldValue(frontmatter["session"]) != "" || len(frontmatter["depends_on"].Values) != 0 || firstFieldValue(frontmatter["status"]) != "todo" {
		t.Fatalf("frontmatter after clear = %#v, want cleared frontmatter metadata", frontmatter)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--spec", "SPEC-999"})
	if err == nil || !strings.Contains(err.Error(), "Unknown spec") {
		t.Fatalf("missing spec error = %v, want unknown spec", err)
	}
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--depends-on", "TASK-999"})
	if err == nil || !strings.Contains(err.Error(), "Unknown task ID") {
		t.Fatalf("missing dependency error = %v, want unknown dependency", err)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskUpdateReportsValidationAndInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-status.md", "# Status Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Status Task","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001"})
	if err == nil {
		t.Fatal("task update empty update error = nil, want error")
	}
	if !strings.Contains(err.Error(), "at least one update") {
		t.Fatalf("error = %v, want empty update error", err)
	}
	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--priority", "P9"})
	if err == nil {
		t.Fatal("task update invalid priority error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid priority") {
		t.Fatalf("error = %v, want invalid priority", err)
	}

	stateHome = t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "update", "TASK-001", "--status", "done"})
	if err == nil {
		t.Fatal("task update invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerTaskArchiveUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-archive.md", "# Archive Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-done.md", "# Done Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-todo.md", "# Todo Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Done Task","spec":"SPEC-001","status":"done","priority":"P1"},
  "TASK-002":{"title":"Todo Task","spec":"SPEC-001","status":"todo","priority":"P2"}
}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var archiveOut bytes.Buffer
	err := Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "TASK-001", "TASK-002", "SPEC-001", "TASK-999", "--json"})
	if err != nil {
		t.Fatalf("task archive --json error = %v", err)
	}
	archive := decodeTaskArchiveResult(t, archiveOut.Bytes())
	if len(archive.Archived) != 1 || archive.Archived[0].Task == nil || archive.Archived[0].Task.Alias != "TASK-001" || archive.Archived[0].EventID == "" {
		t.Fatalf("Archived = %#v, want TASK-001 archived with event", archive.Archived)
	}
	if len(archive.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want three skipped refs", archive.Skipped)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--json", "--status", "archived"})
	if err != nil {
		t.Fatalf("task list --status archived error = %v", err)
	}
	tasks := decodeTaskList(t, listOut.Bytes())
	if len(tasks.Tasks) != 1 || tasks.Tasks["TASK-001"].Status != "archived" {
		t.Fatalf("archived tasks = %#v, want TASK-001", tasks.Tasks)
	}
	listOut.Reset()
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "list", "--json", "--active"})
	if err != nil {
		t.Fatalf("task list --active after archive error = %v", err)
	}
	active := decodeTaskList(t, listOut.Bytes())
	if _, ok := active.Tasks["TASK-001"]; ok {
		t.Fatalf("active tasks = %#v, want archived task hidden", active.Tasks)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "show", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task show after archive error = %v", err)
	}
	show := decodeTaskShow(t, showOut.Bytes())
	if show.Task.Status != "archived" {
		t.Fatalf("show status = %q, want archived", show.Task.Status)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("trace after archive error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "TASK-001"})
	if err != nil {
		t.Fatalf("task archive human error = %v", err)
	}
	output := humanOut.String()
	if !strings.Contains(output, "loaf task archive") || !strings.Contains(output, "skipped TASK-001: already archived") || !strings.Contains(output, "Skipped 1 task(s)") {
		t.Fatalf("output = %q, want already-archived human summary", output)
	}
}

func TestRunnerTaskArchiveBySpecUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-archive.md", "# Archive Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-done.md", "# Done Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-todo.md", "# Todo Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Done Task","spec":"SPEC-001","status":"done","priority":"P1"},
  "TASK-002":{"title":"Todo Task","spec":"SPEC-001","status":"todo","priority":"P2"}
}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var archiveOut bytes.Buffer
	err := Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "--spec", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("task archive --spec --json error = %v", err)
	}
	archive := decodeTaskArchiveResult(t, archiveOut.Bytes())
	if archive.Spec == nil || archive.Spec.Alias != "SPEC-001" || len(archive.Archived) != 1 || archive.Archived[0].Task == nil || archive.Archived[0].Task.Alias != "TASK-001" || len(archive.Skipped) != 0 {
		t.Fatalf("archive = %#v, want only done task archived by spec", archive)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "--spec", "SPEC-001"})
	if err != nil {
		t.Fatalf("task archive --spec human empty error = %v", err)
	}
	if !strings.Contains(humanOut.String(), "No completed tasks found for SPEC-001") {
		t.Fatalf("output = %q, want no completed tasks message", humanOut.String())
	}
}

func TestRunnerTaskArchiveUsesMarkdownIndexWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-done.md", "# Done Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-todo.md", "# Todo Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 3,
  "tasks": {
    "TASK-001": {
      "title": "Done Task",
      "spec": "SPEC-001",
      "status": "done",
      "priority": "P1",
      "file": "TASK-001-done.md",
      "review_notes": "preserve me"
    },
    "TASK-002": {
      "title": "Todo Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P2",
      "file": "TASK-002-todo.md"
    }
  },
  "specs": {
    "SPEC-001": {
      "title": "Archive Spec",
      "status": "drafting",
      "file": "SPEC-001-archive.md"
    }
  }
}`)

	var archiveOut bytes.Buffer
	err := Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "TASK-001", "TASK-002", "TASK-999", "--json"})
	if err != nil {
		t.Fatalf("task archive markdown --json error = %v", err)
	}
	archive := decodeTaskArchiveResult(t, archiveOut.Bytes())
	if len(archive.Archived) != 1 || archive.Archived[0].Task == nil || archive.Archived[0].Task.Alias != "TASK-001" || archive.Archived[0].Previous != "done" || archive.Archived[0].Status != "archived" {
		t.Fatalf("Archived = %#v, want TASK-001 archived", archive.Archived)
	}
	if len(archive.Skipped) != 2 {
		t.Fatalf("Skipped = %#v, want two skipped refs", archive.Skipped)
	}
	if archive.Skipped[0].Ref != "TASK-002" || !strings.Contains(archive.Skipped[0].Reason, "must be done") {
		t.Fatalf("Skipped[0] = %#v, want todo skip", archive.Skipped[0])
	}
	if archive.Skipped[1].Ref != "TASK-999" || archive.Skipped[1].Reason != "not found in index" {
		t.Fatalf("Skipped[1] = %#v, want not-found skip", archive.Skipped[1])
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "tasks", "TASK-001-done.md")); !os.IsNotExist(err) {
		t.Fatalf("active task file stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "tasks", "archive", "TASK-001-done.md")); err != nil {
		t.Fatalf("archived task file stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "tasks", "TASK-002-todo.md")); err != nil {
		t.Fatalf("todo task file stat error = %v", err)
	}

	var index map[string]any
	rawIndex, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	if err := json.Unmarshal(rawIndex, &index); err != nil {
		t.Fatalf("Unmarshal(TASKS.json) error = %v", err)
	}
	tasks := index["tasks"].(map[string]any)
	task := tasks["TASK-001"].(map[string]any)
	if task["file"] != "archive/TASK-001-done.md" || task["status"] != "done" || task["review_notes"] != "preserve me" {
		t.Fatalf("TASK-001 index = %#v, want archived file with legacy status and unknown fields preserved", task)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "TASK-001"})
	if err != nil {
		t.Fatalf("task archive already archived error = %v", err)
	}
	if !strings.Contains(humanOut.String(), "skipped TASK-001: already archived") {
		t.Fatalf("human output = %q, want already archived skip", humanOut.String())
	}

	var specOut bytes.Buffer
	err = Runner{
		Stdout:     &specOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "--spec", "SPEC-001"})
	if err != nil {
		t.Fatalf("task archive --spec markdown error = %v", err)
	}
	if !strings.Contains(specOut.String(), "skipped TASK-001: already archived") {
		t.Fatalf("spec output = %q, want already archived skip", specOut.String())
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerTaskArchiveReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"task", "archive", "TASK-001"})
	if err == nil {
		t.Fatal("task archive invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerBrainstormListUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-open.md", `---
title: Open Brainstorm
status: open
---
# Open Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-resolved.md", `---
title: Resolved Brainstorm
status: resolved
---
# Resolved Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-archived.md", `---
title: Archived Brainstorm
status: archived
---
# Archived Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var defaultOut bytes.Buffer
	err := Runner{
		Stdout:     &defaultOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list", "--json"})
	if err != nil {
		t.Fatalf("brainstorm list --json error = %v", err)
	}
	defaultList := decodeBrainstormList(t, defaultOut.Bytes())
	if defaultList.Brainstorms["20260528-brainstorm-open"].Status != "open" {
		t.Fatalf("defaultList = %#v, want open brainstorm", defaultList.Brainstorms)
	}
	if _, ok := defaultList.Brainstorms["20260528-brainstorm-resolved"]; ok {
		t.Fatalf("defaultList = %#v, want resolved brainstorm hidden", defaultList.Brainstorms)
	}
	if _, ok := defaultList.Brainstorms["20260528-brainstorm-archived"]; ok {
		t.Fatalf("defaultList = %#v, want archived brainstorm hidden", defaultList.Brainstorms)
	}
	open := defaultList.Brainstorms["20260528-brainstorm-open"]
	if open.Title != "Open Brainstorm" || open.SourcePath != ".agents/drafts/20260528-brainstorm-open.md" {
		t.Fatalf("open = %#v, want imported title and source", open)
	}

	var allOut bytes.Buffer
	err = Runner{
		Stdout:     &allOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list", "--all", "--json"})
	if err != nil {
		t.Fatalf("brainstorm list --all --json error = %v", err)
	}
	all := decodeBrainstormList(t, allOut.Bytes())
	if len(all.Brainstorms) != 3 || all.Brainstorms["20260528-brainstorm-resolved"].Status != "resolved" {
		t.Fatalf("all = %#v, want all brainstorms", all.Brainstorms)
	}

	var archivedOut bytes.Buffer
	err = Runner{
		Stdout:     &archivedOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list", "--status", "archived", "--json"})
	if err != nil {
		t.Fatalf("brainstorm list --status archived --json error = %v", err)
	}
	archived := decodeBrainstormList(t, archivedOut.Bytes())
	if len(archived.Brainstorms) != 1 || archived.Brainstorms["20260528-brainstorm-archived"].Status != "archived" {
		t.Fatalf("archived = %#v, want archived brainstorm only", archived.Brainstorms)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list", "--all"})
	if err != nil {
		t.Fatalf("brainstorm list human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"loaf brainstorm list", "20260528-brainstorm-open", "Open Brainstorm", "[resolved]", ".agents/drafts/20260528-brainstorm-open.md"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerBrainstormListRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "brainstorm", "list", "--all", "--json")
}

func TestRunnerBrainstormListReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list"})
	if err == nil {
		t.Fatal("brainstorm list invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerBrainstormShowUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-sqlite.md", `---
title: SQLite Brainstorm
status: open
promoted_to: .agents/ideas/20260528-target-idea.md
---
# SQLite Brainstorm

Imported brainstorm prose.
`)
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", `---
title: Target Idea
status: open
---
# Target Idea
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var showOut bytes.Buffer
	err := Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "show", "20260528-brainstorm-sqlite", "--json"})
	if err != nil {
		t.Fatalf("brainstorm show --json error = %v", err)
	}
	show := decodeBrainstormShow(t, showOut.Bytes())
	if show.Brainstorm.Alias != "20260528-brainstorm-sqlite" || show.Brainstorm.Title != "SQLite Brainstorm" || show.Brainstorm.Status != "open" {
		t.Fatalf("show = %#v, want imported brainstorm metadata", show)
	}
	if len(show.Brainstorm.Sources) != 1 || show.Brainstorm.Sources[0].Path != ".agents/drafts/20260528-brainstorm-sqlite.md" || show.Brainstorm.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want imported brainstorm source", show.Brainstorm.Sources)
	}
	if !strings.Contains(show.Brainstorm.Body, "Imported brainstorm prose.") || strings.Contains(show.Brainstorm.Body, "promoted_to") {
		t.Fatalf("Body = %q, want frontmatter-stripped imported body", show.Brainstorm.Body)
	}
	if !hasTraceRelationship(show.Brainstorm.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("Relationships = %#v, want promoted_to target idea", show.Brainstorm.Relationships)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "show", "20260528-brainstorm-sqlite"})
	if err != nil {
		t.Fatalf("brainstorm show human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"brainstorm 20260528-brainstorm-sqlite", "title: SQLite Brainstorm", "status: open", "source: .agents/drafts/20260528-brainstorm-sqlite.md", "outbound promoted_to idea 20260528-target-idea", "Imported brainstorm prose."} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerBrainstormShowRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "brainstorm", "show", "20260528-brainstorm-sqlite", "--json")
}

func TestRunnerBrainstormShowReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "show", "20260528-brainstorm-sqlite"})
	if err == nil {
		t.Fatal("brainstorm show invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerBrainstormPromoteUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-sqlite.md", `---
title: SQLite Brainstorm
status: open
---
# SQLite Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", `---
title: Target Idea
status: open
---
# Target Idea
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var promoteOut bytes.Buffer
	err := Runner{
		Stdout:     &promoteOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "promote", "20260528-brainstorm-sqlite", "--to-idea", "20260528-target-idea", "--json"})
	if err != nil {
		t.Fatalf("brainstorm promote --json error = %v", err)
	}
	result := decodeBrainstormPromoteResult(t, promoteOut.Bytes())
	if result.Brainstorm.Alias != "20260528-brainstorm-sqlite" || result.Idea.Alias != "20260528-target-idea" || result.Relationship == "" {
		t.Fatalf("result = %#v, want brainstorm promoted to target idea with relationship", result)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "20260528-brainstorm-sqlite", "--json"})
	if err != nil {
		t.Fatalf("trace promoted brainstorm error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if !hasTraceRelationship(trace.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("trace = %#v, want promoted_to target idea", trace)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "show", "20260528-brainstorm-sqlite", "--json"})
	if err != nil {
		t.Fatalf("brainstorm show promoted brainstorm error = %v", err)
	}
	show := decodeBrainstormShow(t, showOut.Bytes())
	if !hasTraceRelationship(show.Brainstorm.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("show relationships = %#v, want promoted_to target idea", show.Brainstorm.Relationships)
	}

	var linkOut bytes.Buffer
	err = Runner{
		Stdout:     &linkOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "list", "20260528-brainstorm-sqlite", "--json"})
	if err != nil {
		t.Fatalf("link list promoted brainstorm error = %v", err)
	}
	links := decodeLinkListResult(t, linkOut.Bytes())
	if !hasTraceRelationship(links.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("links = %#v, want promoted_to target idea", links)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "promote", "20260528-brainstorm-sqlite", "--to-idea", "20260528-target-idea"})
	if err != nil {
		t.Fatalf("brainstorm promote human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"promoted brainstorm 20260528-brainstorm-sqlite to idea 20260528-target-idea", "relationship:"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerBrainstormPromoteRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "brainstorm", "promote", "20260528-brainstorm-sqlite", "--to-idea", "20260528-target-idea", "--json")
}

func TestRunnerBrainstormPromoteReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "promote", "20260528-brainstorm-sqlite", "--to-idea", "20260528-target-idea"})
	if err == nil {
		t.Fatal("brainstorm promote invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerBrainstormArchiveUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-open.md", `---
title: Open Brainstorm
status: open
---
# Open Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "drafts/20260528-brainstorm-archived.md", `---
title: Archived Brainstorm
status: archived
---
# Archived Brainstorm
`)
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var archiveOut bytes.Buffer
	err := Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "archive", "20260528-brainstorm-open", "20260528-brainstorm-archived", "20260528-target-idea", "20260528-missing", "--reason", "promoted to idea", "--json"})
	if err != nil {
		t.Fatalf("brainstorm archive --json error = %v", err)
	}
	archive := decodeBrainstormArchiveResult(t, archiveOut.Bytes())
	if len(archive.Archived) != 1 || archive.Archived[0].Brainstorm == nil || archive.Archived[0].Brainstorm.Alias != "20260528-brainstorm-open" || archive.Archived[0].EventID == "" || archive.Archived[0].Note != "promoted to idea" {
		t.Fatalf("Archived = %#v, want open brainstorm archived with event", archive.Archived)
	}
	if len(archive.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want three skipped refs", archive.Skipped)
	}

	var defaultOut bytes.Buffer
	err = Runner{
		Stdout:     &defaultOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list", "--json"})
	if err != nil {
		t.Fatalf("brainstorm list default after archive error = %v", err)
	}
	defaultList := decodeBrainstormList(t, defaultOut.Bytes())
	if _, ok := defaultList.Brainstorms["20260528-brainstorm-open"]; ok {
		t.Fatalf("defaultList.Brainstorms = %#v, want archived brainstorm hidden", defaultList.Brainstorms)
	}

	var archivedOut bytes.Buffer
	err = Runner{
		Stdout:     &archivedOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "list", "--json", "--status", "archived"})
	if err != nil {
		t.Fatalf("brainstorm list --status archived error = %v", err)
	}
	archived := decodeBrainstormList(t, archivedOut.Bytes())
	if archived.Brainstorms["20260528-brainstorm-open"].Status != "archived" || archived.Brainstorms["20260528-brainstorm-archived"].Status != "archived" {
		t.Fatalf("archived.Brainstorms = %#v, want both archived brainstorms", archived.Brainstorms)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "20260528-brainstorm-open", "--json"})
	if err != nil {
		t.Fatalf("trace after brainstorm archive error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "show", "20260528-brainstorm-open", "--json"})
	if err != nil {
		t.Fatalf("brainstorm show after archive error = %v", err)
	}
	show := decodeBrainstormShow(t, showOut.Bytes())
	if show.Brainstorm.Status != "archived" {
		t.Fatalf("show status = %q, want archived", show.Brainstorm.Status)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "archive", "20260528-brainstorm-open"})
	if err != nil {
		t.Fatalf("brainstorm archive human error = %v", err)
	}
	output := humanOut.String()
	if !strings.Contains(output, "loaf brainstorm archive") || !strings.Contains(output, "skipped 20260528-brainstorm-open: already archived") || !strings.Contains(output, "Skipped 1 brainstorm(s)") {
		t.Fatalf("output = %q, want already-archived human summary", output)
	}
}

func TestRunnerBrainstormArchiveRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "brainstorm", "archive", "20260528-brainstorm-open", "--reason", "done", "--json")
}

func TestRunnerBrainstormArchiveReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"brainstorm", "archive", "20260528-brainstorm-open"})
	if err == nil {
		t.Fatal("brainstorm archive invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerIdeaListAndResolveUseSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-sqlite-state.md", `---
title: SQLite State
status: open
---
# SQLite State
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var beforeOut bytes.Buffer
	err := Runner{
		Stdout:     &beforeOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list", "--json"})
	if err != nil {
		t.Fatalf("idea list --json before resolve error = %v", err)
	}
	before := decodeIdeaList(t, beforeOut.Bytes())
	if before.Ideas["20260528-sqlite-state"].Status != "open" {
		t.Fatalf("before.Ideas = %#v, want open imported idea", before.Ideas)
	}

	var resolveOut bytes.Buffer
	err = Runner{
		Stdout:     &resolveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "resolve", "20260528-sqlite-state", "--by", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("idea resolve error = %v", err)
	}
	result := decodeIdeaResolveResult(t, resolveOut.Bytes())
	if result.Idea.Status != "resolved" || result.ResolvedBy.Alias != "SPEC-001" || result.EventID == "" {
		t.Fatalf("result = %#v, want resolved idea by SPEC-001 with event", result)
	}

	var afterOut bytes.Buffer
	err = Runner{
		Stdout:     &afterOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list", "--json"})
	if err != nil {
		t.Fatalf("idea list --json after resolve error = %v", err)
	}
	after := decodeIdeaList(t, afterOut.Bytes())
	if _, ok := after.Ideas["20260528-sqlite-state"]; ok {
		t.Fatalf("after.Ideas = %#v, want resolved idea omitted by default", after.Ideas)
	}

	var allOut bytes.Buffer
	err = Runner{
		Stdout:     &allOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list", "--all", "--json"})
	if err != nil {
		t.Fatalf("idea list --all --json error = %v", err)
	}
	all := decodeIdeaList(t, allOut.Bytes())
	if all.Ideas["20260528-sqlite-state"].Status != "resolved" {
		t.Fatalf("all.Ideas = %#v, want resolved idea included with --all", all.Ideas)
	}
}

func TestRunnerIdeaShowUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-sqlite-state.md", `---
title: SQLite State
status: open
resolved_by:
  - SPEC-001
---
# SQLite State

Imported idea prose.
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var showOut bytes.Buffer
	err := Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "show", "20260528-sqlite-state", "--json"})
	if err != nil {
		t.Fatalf("idea show --json error = %v", err)
	}
	show := decodeIdeaShow(t, showOut.Bytes())
	if show.Idea.Alias != "20260528-sqlite-state" || show.Idea.Title != "SQLite State" || show.Idea.Status != "open" {
		t.Fatalf("show = %#v, want imported idea metadata", show)
	}
	if len(show.Idea.Sources) != 1 || show.Idea.Sources[0].Path != ".agents/ideas/20260528-sqlite-state.md" || show.Idea.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want imported idea source", show.Idea.Sources)
	}
	if !strings.Contains(show.Idea.Body, "Imported idea prose.") || strings.Contains(show.Idea.Body, "resolved_by") {
		t.Fatalf("Body = %q, want frontmatter-stripped imported body", show.Idea.Body)
	}
	if !hasTraceRelationship(show.Idea.Relationships, "outbound", "resolved_by", "spec", "SPEC-001") {
		t.Fatalf("Relationships = %#v, want resolved_by SPEC-001", show.Idea.Relationships)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "show", "20260528-sqlite-state"})
	if err != nil {
		t.Fatalf("idea show human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"idea 20260528-sqlite-state", "title: SQLite State", "status: open", "source: .agents/ideas/20260528-sqlite-state.md", "outbound resolved_by spec SPEC-001", "Imported idea prose."} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}

	var captureOut bytes.Buffer
	err = Runner{
		Stdout:     &captureOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "capture", "--title", "Captured Idea", "--json"})
	if err != nil {
		t.Fatalf("idea capture --json error = %v", err)
	}
	captured := decodeIdeaCaptureResult(t, captureOut.Bytes())
	var capturedShowOut bytes.Buffer
	err = Runner{
		Stdout:     &capturedShowOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "show", captured.Idea.Alias, "--json"})
	if err != nil {
		t.Fatalf("idea show captured error = %v", err)
	}
	capturedShow := decodeIdeaShow(t, capturedShowOut.Bytes())
	if capturedShow.Idea.Alias != captured.Idea.Alias || capturedShow.Idea.Title != "Captured Idea" || len(capturedShow.Idea.Sources) != 0 || capturedShow.Idea.Body != "" {
		t.Fatalf("capturedShow = %#v, want captured idea without source/body", capturedShow)
	}
}

func TestRunnerIdeaPromoteUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-sqlite-state.md", `---
title: SQLite State
status: open
---
# SQLite State
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var promoteOut bytes.Buffer
	err := Runner{
		Stdout:     &promoteOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "promote", "20260528-sqlite-state", "--to-spec", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("idea promote --json error = %v", err)
	}
	result := decodeIdeaPromoteResult(t, promoteOut.Bytes())
	if result.Idea.Alias != "20260528-sqlite-state" || result.Idea.Status != "open" || result.Spec.Alias != "SPEC-001" || result.Relationship == "" {
		t.Fatalf("result = %#v, want open idea promoted to target spec with relationship", result)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "20260528-sqlite-state", "--json"})
	if err != nil {
		t.Fatalf("trace promoted idea error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "open" || !hasTraceRelationship(trace.Relationships, "outbound", "promoted_to", "spec", "SPEC-001") {
		t.Fatalf("trace = %#v, want open idea with promoted_to target spec", trace)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "show", "20260528-sqlite-state", "--json"})
	if err != nil {
		t.Fatalf("idea show promoted idea error = %v", err)
	}
	show := decodeIdeaShow(t, showOut.Bytes())
	if !hasTraceRelationship(show.Idea.Relationships, "outbound", "promoted_to", "spec", "SPEC-001") {
		t.Fatalf("show relationships = %#v, want promoted_to target spec", show.Idea.Relationships)
	}

	var linkOut bytes.Buffer
	err = Runner{
		Stdout:     &linkOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "list", "20260528-sqlite-state", "--json"})
	if err != nil {
		t.Fatalf("link list promoted idea error = %v", err)
	}
	links := decodeLinkListResult(t, linkOut.Bytes())
	if !hasTraceRelationship(links.Relationships, "outbound", "promoted_to", "spec", "SPEC-001") {
		t.Fatalf("links = %#v, want promoted_to target spec", links)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "promote", "20260528-sqlite-state", "--to-spec", "SPEC-001"})
	if err != nil {
		t.Fatalf("idea promote human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"promoted idea 20260528-sqlite-state to spec SPEC-001", "relationship:"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerIdeaCaptureUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var captureOut bytes.Buffer
	err := Runner{
		Stdout:     &captureOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "capture", "--title", "Repeat Idea", "--json"})
	if err != nil {
		t.Fatalf("idea capture --json error = %v", err)
	}
	result := decodeIdeaCaptureResult(t, captureOut.Bytes())
	if result.Idea.Status != "open" || result.Idea.Title != "Repeat Idea" || !strings.HasPrefix(result.Idea.Alias, "IDEA-") || !strings.Contains(result.Idea.Alias, "repeat-idea") || result.EventID == "" {
		t.Fatalf("result = %#v, want captured idea with alias and event", result)
	}

	var secondOut bytes.Buffer
	err = Runner{
		Stdout:     &secondOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "capture", "--title", "Repeat Idea", "--json"})
	if err != nil {
		t.Fatalf("idea capture collision error = %v", err)
	}
	second := decodeIdeaCaptureResult(t, secondOut.Bytes())
	if second.Idea.Alias != result.Idea.Alias+"-2" {
		t.Fatalf("second alias = %q, want collision suffix after %q", second.Idea.Alias, result.Idea.Alias)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list", "--json"})
	if err != nil {
		t.Fatalf("idea list after capture error = %v", err)
	}
	ideas := decodeIdeaList(t, listOut.Bytes())
	if ideas.Ideas[result.Idea.Alias].Status != "open" || ideas.Ideas[result.Idea.Alias].Title != "Repeat Idea" {
		t.Fatalf("ideas = %#v, want captured idea in list", ideas.Ideas)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", result.Idea.Alias, "--json"})
	if err != nil {
		t.Fatalf("trace captured idea error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "open" || trace.Entity.Alias != result.Idea.Alias {
		t.Fatalf("trace entity = %#v, want captured idea", trace.Entity)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "capture", "--title", "Human Idea"})
	if err != nil {
		t.Fatalf("idea capture human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"captured idea IDEA-", "human-idea", "title: Human Idea", "event:"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerIdeaArchiveUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-open-idea.md", `---
title: Open Idea
status: open
---
# Open Idea
`)
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-archived-idea.md", `---
title: Archived Idea
status: archived
---
# Archived Idea
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var archiveOut bytes.Buffer
	err := Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "archive", "20260528-open-idea", "20260528-archived-idea", "SPEC-001", "20260528-missing", "--reason", "covered by SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("idea archive --json error = %v", err)
	}
	archive := decodeIdeaArchiveResult(t, archiveOut.Bytes())
	if len(archive.Archived) != 1 || archive.Archived[0].Idea == nil || archive.Archived[0].Idea.Alias != "20260528-open-idea" || archive.Archived[0].EventID == "" || archive.Archived[0].Note != "covered by SPEC-001" {
		t.Fatalf("Archived = %#v, want open idea archived with event", archive.Archived)
	}
	if len(archive.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want three skipped refs", archive.Skipped)
	}

	var defaultOut bytes.Buffer
	err = Runner{
		Stdout:     &defaultOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list", "--json"})
	if err != nil {
		t.Fatalf("idea list default after archive error = %v", err)
	}
	defaultList := decodeIdeaList(t, defaultOut.Bytes())
	if _, ok := defaultList.Ideas["20260528-open-idea"]; ok {
		t.Fatalf("defaultList.Ideas = %#v, want archived idea hidden", defaultList.Ideas)
	}

	var archivedOut bytes.Buffer
	err = Runner{
		Stdout:     &archivedOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list", "--json", "--status", "archived"})
	if err != nil {
		t.Fatalf("idea list --status archived error = %v", err)
	}
	archived := decodeIdeaList(t, archivedOut.Bytes())
	if archived.Ideas["20260528-open-idea"].Status != "archived" || archived.Ideas["20260528-archived-idea"].Status != "archived" {
		t.Fatalf("archived.Ideas = %#v, want both archived ideas", archived.Ideas)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "20260528-open-idea", "--json"})
	if err != nil {
		t.Fatalf("trace after idea archive error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "archive", "20260528-open-idea"})
	if err != nil {
		t.Fatalf("idea archive human error = %v", err)
	}
	output := humanOut.String()
	if !strings.Contains(output, "loaf idea archive") || !strings.Contains(output, "skipped 20260528-open-idea: already archived") || !strings.Contains(output, "Skipped 1 idea(s)") {
		t.Fatalf("output = %q, want already-archived human summary", output)
	}
}

func TestRunnerIdeaCommandRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "idea", "resolve", "20260528-sqlite-state", "--by", "SPEC-001")
	assertSQLiteRequired(t, "idea", "show", "20260528-sqlite-state", "--json")
	assertSQLiteRequired(t, "idea", "promote", "20260528-sqlite-state", "--to-spec", "SPEC-001")
	assertSQLiteRequired(t, "idea", "capture", "--title", "Smoke Idea")
	assertSQLiteRequired(t, "idea", "archive", "20260528-sqlite-state", "--reason", "covered")
}

func TestRunnerIdeaCommandReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "list"})
	if err == nil {
		t.Fatal("idea list invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "promote", "20260528-sqlite-state", "--to-spec", "SPEC-001"})
	if err == nil {
		t.Fatal("idea promote invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "show", "20260528-sqlite-state"})
	if err == nil {
		t.Fatal("idea show invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "capture", "--title", "Smoke Idea"})
	if err == nil {
		t.Fatal("idea capture invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"idea", "archive", "20260528-sqlite-state"})
	if err == nil {
		t.Fatal("idea archive invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSparkListAndResolveUseSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var beforeOut bytes.Buffer
	err := Runner{
		Stdout:     &beforeOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "list", "--json"})
	if err != nil {
		t.Fatalf("spark list --json before resolve error = %v", err)
	}
	before := decodeSparkList(t, beforeOut.Bytes())
	if before.Sparks["SPARK-smoke"].Status != "open" {
		t.Fatalf("before.Sparks = %#v, want open imported spark", before.Sparks)
	}

	var resolveOut bytes.Buffer
	err = Runner{
		Stdout:     &resolveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "resolve", "SPARK-smoke", "--by", "20260528-target-idea", "--reason", "triaged into target idea", "--json"})
	if err != nil {
		t.Fatalf("spark resolve error = %v", err)
	}
	result := decodeSparkResolveResult(t, resolveOut.Bytes())
	if result.Spark.Status != "resolved" || result.ResolvedBy.Alias != "20260528-target-idea" || result.EventID == "" || result.Reason != "triaged into target idea" {
		t.Fatalf("result = %#v, want resolved spark by target idea with event", result)
	}

	var afterOut bytes.Buffer
	err = Runner{
		Stdout:     &afterOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "list", "--json"})
	if err != nil {
		t.Fatalf("spark list --json after resolve error = %v", err)
	}
	after := decodeSparkList(t, afterOut.Bytes())
	if _, ok := after.Sparks["SPARK-smoke"]; ok {
		t.Fatalf("after.Sparks = %#v, want resolved spark omitted by default", after.Sparks)
	}

	var allOut bytes.Buffer
	err = Runner{
		Stdout:     &allOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "list", "--all", "--json"})
	if err != nil {
		t.Fatalf("spark list --all --json error = %v", err)
	}
	all := decodeSparkList(t, allOut.Bytes())
	if all.Sparks["SPARK-smoke"].Status != "resolved" {
		t.Fatalf("all.Sparks = %#v, want resolved spark included with --all", all.Sparks)
	}
}

func TestRunnerSparkPromoteUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var promoteOut bytes.Buffer
	err := Runner{
		Stdout:     &promoteOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "promote", "SPARK-smoke", "--to-idea", "20260528-target-idea", "--json"})
	if err != nil {
		t.Fatalf("spark promote --json error = %v", err)
	}
	result := decodeSparkPromoteResult(t, promoteOut.Bytes())
	if result.Spark.Alias != "SPARK-smoke" || result.Spark.Status != "open" || result.Idea.Alias != "20260528-target-idea" || result.Relationship == "" {
		t.Fatalf("result = %#v, want open spark promoted to target idea with relationship", result)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "SPARK-smoke", "--json"})
	if err != nil {
		t.Fatalf("trace promoted spark error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "open" || !hasTraceRelationship(trace.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("trace = %#v, want open spark with promoted_to target idea", trace)
	}

	var linkOut bytes.Buffer
	err = Runner{
		Stdout:     &linkOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "list", "SPARK-smoke", "--json"})
	if err != nil {
		t.Fatalf("link list promoted spark error = %v", err)
	}
	links := decodeLinkListResult(t, linkOut.Bytes())
	if !hasTraceRelationship(links.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("links = %#v, want promoted_to target idea", links)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "promote", "SPARK-smoke", "--to-idea", "20260528-target-idea"})
	if err != nil {
		t.Fatalf("spark promote human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"promoted spark SPARK-smoke to idea 20260528-target-idea", "relationship:"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerSparkShowUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"spark", "promote", "SPARK-smoke", "--to-idea", "20260528-target-idea"}); err != nil {
		t.Fatalf("spark promote setup error = %v", err)
	}

	var showOut bytes.Buffer
	err := Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "show", "SPARK-smoke", "--json"})
	if err != nil {
		t.Fatalf("spark show --json error = %v", err)
	}
	show := decodeSparkShow(t, showOut.Bytes())
	if show.Spark.Alias != "SPARK-smoke" || show.Spark.Text != "smoke spark" || show.Spark.Scope != "sqlite" || show.Spark.Status != "open" {
		t.Fatalf("show = %#v, want imported spark metadata", show)
	}
	if len(show.Spark.Sources) != 1 || show.Spark.Sources[0].Path != ".agents/sessions/20260528-session.md" || show.Spark.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want session source with hash", show.Spark.Sources)
	}
	if !hasTraceRelationship(show.Spark.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("Relationships = %#v, want promoted_to target idea", show.Spark.Relationships)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "show", "SPARK-smoke"})
	if err != nil {
		t.Fatalf("spark show human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"spark SPARK-smoke", "scope: sqlite", "status: open", "text: smoke spark", "source: .agents/sessions/20260528-session.md", "outbound promoted_to idea 20260528-target-idea"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerSparkCaptureUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var captureOut bytes.Buffer
	err := Runner{
		Stdout:     &captureOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "capture", "--scope", "architecture", "--text", "Repeat Spark", "--json"})
	if err != nil {
		t.Fatalf("spark capture --json error = %v", err)
	}
	result := decodeSparkCaptureResult(t, captureOut.Bytes())
	if result.Spark.Alias != "SPARK-repeat-spark" || result.Spark.Status != "open" || result.Scope != "architecture" || result.EventID == "" {
		t.Fatalf("result = %#v, want captured spark with alias, scope, and event", result)
	}

	var secondOut bytes.Buffer
	err = Runner{
		Stdout:     &secondOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "capture", "--text", "Repeat Spark", "--json"})
	if err != nil {
		t.Fatalf("spark capture collision error = %v", err)
	}
	second := decodeSparkCaptureResult(t, secondOut.Bytes())
	if second.Spark.Alias != "SPARK-repeat-spark-2" {
		t.Fatalf("second alias = %q, want collision suffix", second.Spark.Alias)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "list", "--json"})
	if err != nil {
		t.Fatalf("spark list after capture error = %v", err)
	}
	sparks := decodeSparkList(t, listOut.Bytes())
	if sparks.Sparks["SPARK-repeat-spark"].Status != "open" || sparks.Sparks["SPARK-repeat-spark"].Scope != "architecture" {
		t.Fatalf("sparks = %#v, want captured spark in list", sparks.Sparks)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "SPARK-repeat-spark", "--json"})
	if err != nil {
		t.Fatalf("trace captured spark error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "open" || trace.Entity.Alias != "SPARK-repeat-spark" {
		t.Fatalf("trace entity = %#v, want captured spark", trace.Entity)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "capture", "--scope", "ops", "--text", "Human Spark"})
	if err != nil {
		t.Fatalf("spark capture human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"captured spark SPARK-human-spark", "scope: ops", "text: Human Spark", "event:"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerSparkCommandRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "spark", "resolve", "SPARK-smoke", "--by", "20260528-target-idea", "--reason", "covered")
	assertSQLiteRequired(t, "spark", "show", "SPARK-smoke", "--json")
	assertSQLiteRequired(t, "spark", "capture", "--scope", "architecture", "--text", "Smoke Spark")
	assertSQLiteRequired(t, "spark", "promote", "SPARK-smoke", "--to-idea", "20260528-target-idea")
}

func TestRunnerSparkCommandReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "list"})
	if err == nil {
		t.Fatal("spark list invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "capture", "--text", "Smoke Spark"})
	if err == nil {
		t.Fatal("spark capture invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "resolve", "SPARK-smoke", "--by", "20260528-target-idea", "--reason", "covered"})
	if err == nil {
		t.Fatal("spark resolve invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "promote", "SPARK-smoke", "--to-idea", "20260528-target-idea"})
	if err == nil {
		t.Fatal("spark promote invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spark", "show", "SPARK-smoke"})
	if err == nil {
		t.Fatal("spark show invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerTagCommandsUseSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-tags.md", "# Tag Spec\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-tag-idea.md", "# Tag Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var addOut bytes.Buffer
	err := Runner{
		Stdout:     &addOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"tag", "add", "SPEC-001", "SQLite", "--json"})
	if err != nil {
		t.Fatalf("tag add spec error = %v", err)
	}
	added := decodeTagMutationResult(t, addOut.Bytes())
	if added.Name != "sqlite" || added.Entity.Kind != "spec" || added.Entity.Alias != "SPEC-001" {
		t.Fatalf("added = %#v, want sqlite tag on SPEC-001", added)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"tag", "add", "20260528-tag-idea", "sqlite"}); err != nil {
		t.Fatalf("tag add idea error = %v", err)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"tag", "list", "--json"})
	if err != nil {
		t.Fatalf("tag list error = %v", err)
	}
	tags := decodeTagList(t, listOut.Bytes())
	if tags.Tags["sqlite"].Count != 2 {
		t.Fatalf("tags = %#v, want sqlite count 2", tags.Tags)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"tag", "show", "sqlite", "--json"})
	if err != nil {
		t.Fatalf("tag show error = %v", err)
	}
	show := decodeTagShowResult(t, showOut.Bytes())
	if len(show.Members) != 2 {
		t.Fatalf("show.Members = %#v, want 2 members", show.Members)
	}

	var removeOut bytes.Buffer
	err = Runner{
		Stdout:     &removeOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"tag", "remove", "SPEC-001", "sqlite", "--json"})
	if err != nil {
		t.Fatalf("tag remove error = %v", err)
	}
	removed := decodeTagMutationResult(t, removeOut.Bytes())
	if removed.Entity.Alias != "SPEC-001" {
		t.Fatalf("removed = %#v, want SPEC-001 removed", removed)
	}
}

func TestRunnerTagCommandRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "tag", "add", "SPEC-001", "sqlite")
}

func TestRunnerTagCommandReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"tag", "list"})
	if err == nil {
		t.Fatal("tag list invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerBundleCommandsUseSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-bundle.md", "# Bundle Spec\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-bundle.md", "# Bundle Task\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-bundle-idea.md", "# Bundle Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Bundle Task","spec":"SPEC-001","status":"todo"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"tag", "add", "SPEC-001", "sqlite"}); err != nil {
		t.Fatalf("tag add spec error = %v", err)
	}

	var createOut bytes.Buffer
	err := Runner{
		Stdout:     &createOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "create", "sqlite-backend", "--tag", "sqlite", "--title", "SQLite Backend", "--json"})
	if err != nil {
		t.Fatalf("bundle create error = %v", err)
	}
	created := decodeBundleMutationResult(t, createOut.Bytes())
	if created.Slug != "sqlite-backend" || created.Title != "SQLite Backend" || len(created.Tags) != 1 || created.Tags[0] != "sqlite" {
		t.Fatalf("created = %#v, want sqlite-backend bundle", created)
	}
	if created.Entity != nil {
		t.Fatalf("created.Entity = %#v, want nil for bundle create", created.Entity)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "list", "--json"})
	if err != nil {
		t.Fatalf("bundle list error = %v", err)
	}
	list := decodeBundleList(t, listOut.Bytes())
	listed := list.Bundles["sqlite-backend"]
	if listed.Title != "SQLite Backend" || listed.TagMatchedCount != 1 || listed.MemberCount != 1 {
		t.Fatalf("list = %#v, want sqlite-backend bundle with tag-matched spec", list)
	}

	var updateOut bytes.Buffer
	err = Runner{
		Stdout:     &updateOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "update", "sqlite-backend", "--title", "SQLite Runtime", "--tag", "sqlite", "--tag", "state", "--json"})
	if err != nil {
		t.Fatalf("bundle update error = %v", err)
	}
	updated := decodeBundleMutationResult(t, updateOut.Bytes())
	if updated.Title != "SQLite Runtime" || len(updated.Tags) != 2 || updated.Tags[0] != "sqlite" || updated.Tags[1] != "state" {
		t.Fatalf("updated = %#v, want replaced title and tags", updated)
	}

	var addOut bytes.Buffer
	err = Runner{
		Stdout:     &addOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "add", "sqlite-backend", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("bundle add error = %v", err)
	}
	added := decodeBundleMutationResult(t, addOut.Bytes())
	if added.Entity == nil || added.Entity.Kind != "task" || added.Entity.Alias != "TASK-001" {
		t.Fatalf("added = %#v, want TASK-001 explicit member", added)
	}

	var showOut bytes.Buffer
	err = Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "show", "sqlite-backend", "--json"})
	if err != nil {
		t.Fatalf("bundle show error = %v", err)
	}
	show := decodeBundleShowResult(t, showOut.Bytes())
	if show.Title != "SQLite Runtime" || len(show.TagQuery) != 2 || len(show.TagMatched) != 1 || len(show.Explicit) != 1 || len(show.Members) != 2 {
		t.Fatalf("show = %#v, want updated bundle with tag-matched spec and explicit task", show)
	}

	var removeOut bytes.Buffer
	err = Runner{
		Stdout:     &removeOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "remove", "sqlite-backend", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("bundle remove error = %v", err)
	}
	removed := decodeBundleMutationResult(t, removeOut.Bytes())
	if removed.Entity == nil || removed.Entity.Alias != "TASK-001" {
		t.Fatalf("removed = %#v, want TASK-001 removed", removed)
	}
}

func TestRunnerBundleCommandRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "bundle", "list", "--json")
	assertSQLiteRequired(t, "bundle", "update", "sqlite-backend", "--title", "SQLite Backend")
}

func TestRunnerBundleCommandReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "list"})
	if err == nil {
		t.Fatal("bundle list invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"bundle", "update", "sqlite-backend", "--title", "SQLite Backend"})
	if err == nil {
		t.Fatal("bundle update invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerLinkCommandsUseSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-link.md", "# Link Spec\n")
	writeCLIAgentsFile(t, workingDir, "ideas/20260528-link-idea.md", "# Link Idea\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var createOut bytes.Buffer
	err := Runner{
		Stdout:     &createOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "create", "20260528-link-idea", "SPEC-001", "--type", "resolved_by", "--reason", "from cli test", "--json"})
	if err != nil {
		t.Fatalf("link create error = %v", err)
	}
	created := decodeLinkMutationResult(t, createOut.Bytes())
	if created.Type != "resolved_by" || created.From.Alias != "20260528-link-idea" || created.To.Alias != "SPEC-001" || created.Reason != "from cli test" {
		t.Fatalf("created = %#v, want idea resolved_by SPEC-001", created)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "list", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("link list error = %v", err)
	}
	list := decodeLinkListResult(t, listOut.Bytes())
	if len(list.Relationships) != 1 || !hasTraceRelationship(list.Relationships, "inbound", "resolved_by", "idea", "20260528-link-idea") {
		t.Fatalf("list = %#v, want inbound idea relationship", list)
	}

	var removeOut bytes.Buffer
	err = Runner{
		Stdout:     &removeOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "remove", "20260528-link-idea", "SPEC-001", "--type", "resolved_by", "--json"})
	if err != nil {
		t.Fatalf("link remove error = %v", err)
	}
	removed := decodeLinkMutationResult(t, removeOut.Bytes())
	if removed.Type != "resolved_by" || removed.From.Alias != "20260528-link-idea" || removed.To.Alias != "SPEC-001" || removed.Reason != "from cli test" {
		t.Fatalf("removed = %#v, want removed relationship", removed)
	}

	var listAfterRemove bytes.Buffer
	err = Runner{
		Stdout:     &listAfterRemove,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "list", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("link list after remove error = %v", err)
	}
	after := decodeLinkListResult(t, listAfterRemove.Bytes())
	if len(after.Relationships) != 0 {
		t.Fatalf("relationships after remove = %#v, want none", after.Relationships)
	}
}

func TestRunnerLinkCommandRequiresSQLiteWhenMarkdownOnly(t *testing.T) {
	assertSQLiteRequired(t, "link", "create", "20260528-link-idea", "SPEC-001", "--type", "resolved_by")
}

func TestRunnerLinkCommandReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"link", "list", "SPEC-001"})
	if err == nil {
		t.Fatal("link list invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSpecListJSONUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-002-done.md", "# Done\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Example Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "list", "--json"})
	if err != nil {
		t.Fatalf("spec list --json error = %v", err)
	}

	specs := decodeSpecList(t, stdout.Bytes())
	spec := specs.Specs["SPEC-001"]
	if spec.Title != "Example Spec" || spec.Status != "implementing" || spec.SourcePath != ".agents/specs/SPEC-001-example.md" {
		t.Fatalf("SPEC-001 = %#v, want imported spec metadata", spec)
	}
	if spec.Tasks.Todo != 1 || spec.Tasks.InProgress != 0 || spec.Tasks.Done != 1 {
		t.Fatalf("SPEC-001 task counts = %#v, want todo=1 in_progress=0 done=1", spec.Tasks)
	}
}

func TestRunnerSpecListHumanUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","spec":"SPEC-001","status":"in_progress","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "list"})
	if err != nil {
		t.Fatalf("spec list error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"loaf spec list", "Implementing (1)", "SPEC-001", "Example Spec", "0 todo / 1 in_progress / 0 done"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerSpecListUsesMarkdownSpecsWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-draft.md", `---
id: SPEC-002
title: Draft Spec
status: drafting
---
# Draft Spec
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"},
    "TASK-004": {"title": "Review Task", "spec": "SPEC-001", "status": "review", "priority": "P2"}
  }
}`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "list", "--json"})
	if err != nil {
		t.Fatalf("spec list markdown --json error = %v", err)
	}
	specs := decodeSpecList(t, jsonOut.Bytes())
	spec := specs.Specs["SPEC-001"]
	if spec.Title != "Example Spec" || spec.Status != "implementing" || spec.SourcePath != ".agents/specs/SPEC-001-example.md" {
		t.Fatalf("SPEC-001 = %#v, want markdown spec metadata", spec)
	}
	if spec.Tasks.Todo != 2 || spec.Tasks.InProgress != 1 || spec.Tasks.Done != 1 {
		t.Fatalf("SPEC-001 task counts = %#v, want todo=2 in_progress=1 done=1", spec.Tasks)
	}
	if specs.Specs["SPEC-002"].Tasks != (state.SpecTaskCounts{}) {
		t.Fatalf("SPEC-002 task counts = %#v, want zero counts", specs.Specs["SPEC-002"].Tasks)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "list"})
	if err != nil {
		t.Fatalf("spec list markdown human error = %v", err)
	}
	output := humanOut.String()
	for _, want := range []string{"loaf spec list", "Implementing (1)", "SPEC-001", "Example Spec", "2 todo / 1 in_progress / 1 done", "Drafting (1)", "SPEC-002"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSpecListReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "list"})
	if err == nil {
		t.Fatal("spec list error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSpecShowUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec

Imported spec prose.
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-example.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","spec":"SPEC-001","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var showOut bytes.Buffer
	err := Runner{
		Stdout:     &showOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "show", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("spec show --json error = %v", err)
	}
	show := decodeSpecShow(t, showOut.Bytes())
	if show.Spec.Alias != "SPEC-001" || show.Spec.Title != "Example Spec" || show.Spec.Status != "implementing" {
		t.Fatalf("show = %#v, want imported spec metadata", show)
	}
	if show.Spec.Tasks.Todo != 1 || show.Spec.Tasks.InProgress != 0 || show.Spec.Tasks.Done != 0 {
		t.Fatalf("show.Spec.Tasks = %#v, want one todo task", show.Spec.Tasks)
	}
	if len(show.Spec.Sources) != 1 || show.Spec.Sources[0].Path != ".agents/specs/SPEC-001-example.md" || show.Spec.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want spec source with hash", show.Spec.Sources)
	}
	if !strings.Contains(show.Spec.Body, "Imported spec prose.") || strings.Contains(show.Spec.Body, "status: implementing") {
		t.Fatalf("Body = %q, want frontmatter-stripped imported body", show.Spec.Body)
	}
	if !hasTraceRelationship(show.Spec.Relationships, "inbound", "implements", "task", "TASK-001") {
		t.Fatalf("Relationships = %#v, want inbound task implements relationship", show.Spec.Relationships)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "show", "SPEC-001"})
	if err != nil {
		t.Fatalf("spec show human error = %v", err)
	}
	human := humanOut.String()
	for _, want := range []string{"spec SPEC-001", "title: Example Spec", "status: implementing", "tasks: 1 todo / 0 in_progress / 0 done", "source: .agents/specs/SPEC-001-example.md", "inbound implements task TASK-001", "Imported spec prose."} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output = %q, want %q", human, want)
		}
	}
}

func TestRunnerSpecShowUsesMarkdownSpecWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Frontmatter Spec
status: drafting
created: 2026-05-27T09:00:00Z
---
# Spec Body

Markdown spec prose.
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "specs": {
    "SPEC-001": {
      "title": "Example Spec",
      "status": "implementing",
      "created": "2026-05-28T10:00:00Z",
      "updated": "2026-05-29T11:00:00Z"
    }
  },
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "show", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("spec show markdown --json error = %v", err)
	}
	show := decodeSpecShow(t, jsonOut.Bytes())
	spec := show.Spec
	if show.Query != "SPEC-001" || spec.Alias != "SPEC-001" || spec.Title != "Example Spec" || spec.Status != "implementing" {
		t.Fatalf("show = %#v, want TASKS.json spec metadata over frontmatter", show)
	}
	if spec.Tasks.Todo != 1 || spec.Tasks.InProgress != 1 || spec.Tasks.Done != 1 {
		t.Fatalf("spec.Tasks = %#v, want one todo/in_progress/done", spec.Tasks)
	}
	if len(spec.Sources) != 1 || spec.Sources[0].Path != ".agents/specs/SPEC-001-example.md" || spec.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want markdown spec source with hash", spec.Sources)
	}
	if !strings.Contains(spec.Body, "Markdown spec prose.") || strings.Contains(spec.Body, "---") {
		t.Fatalf("Body = %q, want markdown body without frontmatter", spec.Body)
	}
	if spec.CreatedAt != "2026-05-28T10:00:00Z" || spec.UpdatedAt != "2026-05-29T11:00:00Z" {
		t.Fatalf("timestamps = %q/%q, want index timestamps", spec.CreatedAt, spec.UpdatedAt)
	}
	if !hasTraceRelationship(spec.Relationships, "inbound", "implements", "task", "TASK-001") || !hasTraceRelationship(spec.Relationships, "inbound", "implements", "task", "TASK-002") || !hasTraceRelationship(spec.Relationships, "inbound", "implements", "task", "TASK-003") {
		t.Fatalf("Relationships = %#v, want inbound task relationships", spec.Relationships)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "show", "SPEC-001"})
	if err != nil {
		t.Fatalf("spec show markdown human error = %v", err)
	}
	output := humanOut.String()
	for _, want := range []string{"spec SPEC-001", "title: Example Spec", "status: implementing", "tasks: 1 todo / 1 in_progress / 1 done", "source: .agents/specs/SPEC-001-example.md", "source hash:", "inbound implements task TASK-001", "# Spec Body", "Markdown spec prose."} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSpecShowReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "show", "SPEC-001"})
	if err == nil {
		t.Fatal("spec show invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSpecArchiveUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-complete.md", `---
id: SPEC-001
title: Complete Spec
status: complete
---
# Complete Spec
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-draft.md", `---
id: SPEC-002
title: Draft Spec
status: drafting
---
# Draft Spec
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-task.md", "# Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Task","status":"todo","priority":"P1"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var archiveOut bytes.Buffer
	err := Runner{
		Stdout:     &archiveOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "archive", "SPEC-001", "SPEC-002", "TASK-001", "SPEC-999", "--json"})
	if err != nil {
		t.Fatalf("spec archive --json error = %v", err)
	}
	archive := decodeSpecArchiveResult(t, archiveOut.Bytes())
	if len(archive.Archived) != 1 || archive.Archived[0].Spec == nil || archive.Archived[0].Spec.Alias != "SPEC-001" || archive.Archived[0].EventID == "" {
		t.Fatalf("Archived = %#v, want SPEC-001 archived with event", archive.Archived)
	}
	if len(archive.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want three skipped specs", archive.Skipped)
	}

	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "list", "--json"})
	if err != nil {
		t.Fatalf("spec list after archive error = %v", err)
	}
	specs := decodeSpecList(t, listOut.Bytes())
	if specs.Specs["SPEC-001"].Status != "archived" || specs.Specs["SPEC-002"].Status != "drafting" {
		t.Fatalf("specs = %#v, want SPEC-001 archived and SPEC-002 unchanged", specs.Specs)
	}

	var traceOut bytes.Buffer
	err = Runner{
		Stdout:     &traceOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"trace", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("trace after archive error = %v", err)
	}
	trace := decodeTraceResult(t, traceOut.Bytes())
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "archive", "SPEC-001"})
	if err != nil {
		t.Fatalf("spec archive human error = %v", err)
	}
	output := humanOut.String()
	if !strings.Contains(output, "loaf spec archive") || !strings.Contains(output, "skipped SPEC-001: already archived") || !strings.Contains(output, "Skipped 1 spec(s)") {
		t.Fatalf("output = %q, want already-archived human summary", output)
	}
}

func TestRunnerSpecArchiveUsesMarkdownIndexWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-complete.md", `---
id: SPEC-001
title: Complete Spec
status: complete
---
# Complete Spec
`)
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-002-draft.md", `---
id: SPEC-002
title: Draft Spec
status: drafting
---
# Draft Spec
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{
  "version": 1,
  "next_id": 7,
  "tasks": {
    "TASK-001": {
      "title": "Preserved Task",
      "status": "todo",
      "priority": "P1",
      "files": ["keep.go"]
    }
  },
  "specs": {
    "SPEC-001": {
      "title": "Complete Spec",
      "status": "complete",
      "requirement": "preserve me",
      "file": "SPEC-001-complete.md"
    },
    "SPEC-002": {
      "title": "Draft Spec",
      "status": "drafting",
      "file": "SPEC-002-draft.md"
    }
  }
}`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "archive", "SPEC-001", "SPEC-002", "SPEC-999", "--json"})
	if err != nil {
		t.Fatalf("spec archive markdown --json error = %v", err)
	}
	archive := decodeSpecArchiveResult(t, jsonOut.Bytes())
	if len(archive.Archived) != 1 || archive.Archived[0].Spec == nil || archive.Archived[0].Spec.Alias != "SPEC-001" || archive.Archived[0].Previous != "complete" || archive.Archived[0].Status != "archived" {
		t.Fatalf("Archived = %#v, want SPEC-001 archived", archive.Archived)
	}
	if len(archive.Skipped) != 2 || archive.Skipped[0].Ref != "SPEC-002" || !strings.Contains(archive.Skipped[0].Reason, "status is drafting") || archive.Skipped[1].Ref != "SPEC-999" || archive.Skipped[1].Reason != "not found in index" {
		t.Fatalf("Skipped = %#v, want draft and missing skips", archive.Skipped)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "specs", "SPEC-001-complete.md")); !os.IsNotExist(err) {
		t.Fatalf("active spec still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".agents", "specs", "archive", "SPEC-001-complete.md")); err != nil {
		t.Fatalf("archived spec missing: %v", err)
	}
	var index struct {
		Tasks map[string]map[string]any `json:"tasks"`
		Specs map[string]map[string]any `json:"specs"`
	}
	content, err := os.ReadFile(filepath.Join(workingDir, ".agents", "TASKS.json"))
	if err != nil {
		t.Fatalf("ReadFile(TASKS.json) error = %v", err)
	}
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatalf("json.Unmarshal(TASKS.json) error = %v", err)
	}
	if got := index.Specs["SPEC-001"]["file"]; got != "archive/SPEC-001-complete.md" {
		t.Fatalf("SPEC-001 file = %#v, want archive path", got)
	}
	if got := index.Specs["SPEC-001"]["status"]; got != "complete" {
		t.Fatalf("SPEC-001 status = %#v, want legacy markdown status preserved", got)
	}
	if got := index.Specs["SPEC-001"]["requirement"]; got != "preserve me" {
		t.Fatalf("SPEC-001 requirement = %#v, want unknown spec fields preserved", got)
	}
	files, ok := index.Tasks["TASK-001"]["files"].([]any)
	if !ok || len(files) != 1 || files[0] != "keep.go" {
		t.Fatalf("TASK-001 files = %#v, want task fields preserved", index.Tasks["TASK-001"]["files"])
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "archive", "SPEC-001"})
	if err != nil {
		t.Fatalf("spec archive markdown human error = %v", err)
	}
	output := humanOut.String()
	if !strings.Contains(output, "loaf spec archive") || !strings.Contains(output, "skipped SPEC-001: already archived") || !strings.Contains(output, "Skipped 1 spec(s)") {
		t.Fatalf("output = %q, want already-archived human summary", output)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerSpecArchiveReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"spec", "archive", "SPEC-001"})
	if err == nil {
		t.Fatal("spec archive invalid state error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSessionListJSONUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
branch: feature/session-list
status: active
claude_session_id: session-active
---
[2026-05-28 10:00] decision(scope): active entry
`)
	writeCLIAgentsFile(t, workingDir, "sessions/archive/20260527-archived.md", `---
branch: old/session
status: active
claude_session_id: session-archived
---
[2026-05-27 10:00] discover(scope): archived entry
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list", "--json", "--all"})
	if err != nil {
		t.Fatalf("session list --json --all error = %v", err)
	}

	sessions := decodeSessionList(t, stdout.Bytes())
	active := sessions.Sessions["20260528-active"]
	if active.Branch != "feature/session-list" || active.Status != "active" || active.HarnessSessionID != "session-active" {
		t.Fatalf("active session = %#v, want imported metadata", active)
	}
	if active.SourcePath != ".agents/sessions/20260528-active.md" || active.JournalEntries != 1 {
		t.Fatalf("active session provenance = %#v, want source path and journal count", active)
	}
	archived := sessions.Sessions["20260527-archived"]
	if archived.Status != "archived" || archived.SourcePath != ".agents/sessions/archive/20260527-archived.md" {
		t.Fatalf("archived session = %#v, want archived imported session", archived)
	}
}

func TestRunnerSessionListHumanUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
branch: feature/session-list
status: active
claude_session_id: session-active
---
[2026-05-28 10:00] decision(scope): active entry
`)
	writeCLIAgentsFile(t, workingDir, "sessions/archive/20260527-archived.md", `---
branch: old/session
status: active
claude_session_id: session-archived
---
[2026-05-27 10:00] discover(scope): archived entry
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var activeOnly bytes.Buffer
	err := Runner{
		Stdout:     &activeOnly,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list"})
	if err != nil {
		t.Fatalf("session list error = %v", err)
	}
	output := activeOnly.String()
	for _, want := range []string{"loaf session list", "Active Sessions", "feature/session-list", ".agents/sessions/20260528-active.md", "1 active"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "old/session") {
		t.Fatalf("output = %q, want archived session hidden without --all", output)
	}

	var all bytes.Buffer
	err = Runner{
		Stdout:     &all,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list", "--all"})
	if err != nil {
		t.Fatalf("session list --all error = %v", err)
	}
	allOutput := all.String()
	for _, want := range []string{"Archived Sessions", "old/session", ".agents/sessions/archive/20260527-archived.md", "1 active, 1 archived"} {
		if !strings.Contains(allOutput, want) {
			t.Fatalf("output = %q, want %q", allOutput, want)
		}
	}
}

func TestRunnerSessionListUsesMarkdownSessionsWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
branch: feature/session-list
status: active
claude_session_id: session-active
last_updated: 2026-05-28T10:05:00Z
---
[2026-05-28 10:00] decision(scope): active entry
`)
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-stopped.md", `---
branch: feature/stopped-session
status: stopped
claude_session_id: session-stopped
---
[2026-05-28 10:10] session(stop): stopped for handoff
`)
	writeCLIAgentsFile(t, workingDir, "sessions/archive/20260527-archived.md", `---
branch: old/session
status: active
claude_session_id: session-archived
---
[2026-05-27 10:00] discover(scope): archived entry
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list", "--json", "--all"})
	if err != nil {
		t.Fatalf("session list markdown-only error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=session list") {
		t.Fatalf("stdout = %q, want native markdown session list without legacy delegation", stdout.String())
	}
	sessions := decodeSessionList(t, stdout.Bytes())
	active := sessions.Sessions["20260528-active"]
	if active.Branch != "feature/session-list" || active.Status != "active" || active.HarnessSessionID != "session-active" {
		t.Fatalf("active session = %#v, want markdown frontmatter metadata", active)
	}
	if active.SourcePath != ".agents/sessions/20260528-active.md" || active.JournalEntries != 1 {
		t.Fatalf("active session provenance = %#v, want markdown source and journal count", active)
	}
	stopped := sessions.Sessions["20260528-stopped"]
	if stopped.Status != "stopped" || stopped.Branch != "feature/stopped-session" {
		t.Fatalf("stopped session = %#v, want non-archived markdown session included", stopped)
	}
	archived := sessions.Sessions["20260527-archived"]
	if archived.Status != "archived" || archived.SourcePath != ".agents/sessions/archive/20260527-archived.md" {
		t.Fatalf("archived session = %#v, want archive directory to force archived status", archived)
	}
	assertNoStateDatabase(t, workingDir, stateHome)

	var activeOnly bytes.Buffer
	err = Runner{
		Stdout:     &activeOnly,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list"})
	if err != nil {
		t.Fatalf("session list markdown-only human error = %v", err)
	}
	output := activeOnly.String()
	for _, want := range []string{"loaf session list", "Active Sessions", "feature/session-list", ".agents/sessions/20260528-active.md", "feature/stopped-session", "2 active"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "old/session") || strings.Contains(output, "args=session list") {
		t.Fatalf("output = %q, want archive hidden and no legacy delegation", output)
	}
}

func TestRunnerSessionListReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list"})
	if err == nil {
		t.Fatal("session list error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSessionShowUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
branch: feature/session-show
status: active
claude_session_id: session-active
---
[2026-05-28 10:00] decision(sqlite): keep session state queryable
[2026-05-28 10:05] discover(sqlite): imported journal entries
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-session.md", "# Session Task\n")
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Session Task","status":"todo","priority":"P2"}
}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"task", "update", "TASK-001", "--session", "20260528-active"}); err != nil {
		t.Fatalf("task update --session error = %v", err)
	}

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "show", "20260528-active", "--json"})
	if err != nil {
		t.Fatalf("session show --json error = %v", err)
	}
	show := decodeSessionShow(t, jsonOut.Bytes())
	session := show.Session
	if show.Query != "20260528-active" || session.Alias != "20260528-active" {
		t.Fatalf("show = %#v, want query and alias", show)
	}
	if session.Branch != "feature/session-show" || session.Status != "active" || session.HarnessSessionID != "session-active" {
		t.Fatalf("session metadata = %#v, want imported frontmatter", session)
	}
	if len(session.Sources) != 1 || session.Sources[0].Path != ".agents/sessions/20260528-active.md" || session.Sources[0].Hash == "" {
		t.Fatalf("sources = %#v, want imported source provenance", session.Sources)
	}
	if len(session.JournalEntries) != 2 {
		t.Fatalf("journal entries = %#v, want two imported entries", session.JournalEntries)
	}
	if !hasTraceRelationship(session.Relationships, "inbound", "associated_with", "task", "TASK-001") {
		t.Fatalf("relationships = %#v, want associated task", session.Relationships)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "show", "20260528-active"})
	if err != nil {
		t.Fatalf("session show error = %v", err)
	}
	output := humanOut.String()
	for _, want := range []string{"session 20260528-active", "branch: feature/session-show", "status: active", "harness session: session-active", ".agents/sessions/20260528-active.md", "decision(sqlite): keep session state queryable", "inbound associated_with task TASK-001"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
}

func TestRunnerSessionShowUsesMarkdownSessionsWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260528-active.md", `---
branch: feature/session-show
status: active
claude_session_id: session-active
created: 2026-05-28T10:00:00Z
last_updated: 2026-05-28T10:05:00Z
---
[2026-05-28 10:00] decision(markdown): keep session readable before SQLite import
[2026-05-28 10:05] discover(markdown): parsed compact journal entries
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "show", "20260528-active", "--json"})
	if err != nil {
		t.Fatalf("session show markdown-only error = %v", err)
	}
	if strings.Contains(stdout.String(), "args=session show") {
		t.Fatalf("stdout = %q, want native markdown session show without legacy delegation", stdout.String())
	}
	show := decodeSessionShow(t, stdout.Bytes())
	session := show.Session
	if show.Query != "20260528-active" || session.Alias != "20260528-active" {
		t.Fatalf("show = %#v, want query and alias", show)
	}
	if session.Branch != "feature/session-show" || session.Status != "active" || session.HarnessSessionID != "session-active" {
		t.Fatalf("session metadata = %#v, want markdown frontmatter metadata", session)
	}
	if len(session.Sources) != 1 || session.Sources[0].Path != ".agents/sessions/20260528-active.md" || session.Sources[0].Hash == "" {
		t.Fatalf("sources = %#v, want markdown source provenance", session.Sources)
	}
	if len(session.JournalEntries) != 2 || session.JournalEntries[0].EntryType != "decision" || session.JournalEntries[0].Scope != "markdown" {
		t.Fatalf("journal entries = %#v, want parsed compact journal entries", session.JournalEntries)
	}
	if session.CreatedAt != "2026-05-28T10:00:00Z" || session.UpdatedAt != "2026-05-28T10:05:00Z" {
		t.Fatalf("session timestamps = %#v, want frontmatter timestamps", session)
	}
	assertNoStateDatabase(t, workingDir, stateHome)

	var branchOut bytes.Buffer
	err = Runner{
		Stdout:     &branchOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "show", "feature/session-show"})
	if err != nil {
		t.Fatalf("session show markdown-only branch error = %v", err)
	}
	output := branchOut.String()
	for _, want := range []string{"session 20260528-active", "branch: feature/session-show", "status: active", "harness session: session-active", ".agents/sessions/20260528-active.md", "decision(markdown): keep session readable before SQLite import"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "args=session show") {
		t.Fatalf("output = %q, want no legacy delegation", output)
	}
}

func TestRunnerSessionShowReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "show", "20260528-active"})
	if err == nil {
		t.Fatal("session show error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerSessionLogJSONUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--json", "--session-id", "harness-123", "decision(sqlite): write to state"})
	if err != nil {
		t.Fatalf("session log --json error = %v", err)
	}

	result := decodeJournalLogResult(t, stdout.Bytes())
	if result.EntryType != "decision" || result.Scope != "sqlite" || result.Message != "write to state" {
		t.Fatalf("result = %#v, want parsed journal entry", result)
	}
	if result.ObservedWorktree != workingDir || result.HarnessSessionID != "harness-123" {
		t.Fatalf("result context = %#v, want observed worktree and harness id", result)
	}

	var sessions state.SessionList
	var listOut bytes.Buffer
	err = Runner{
		Stdout:     &listOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "list", "--json", "--all"})
	if err != nil {
		t.Fatalf("session list --json --all error = %v", err)
	}
	sessions = decodeSessionList(t, listOut.Bytes())
	if len(sessions.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want no synthetic session row from unresolved log", sessions.Sessions)
	}
}

func TestRunnerSessionLogUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-log-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
last_entry: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--session-id", "markdown-log-session", "decision(markdown): native append"})
	if err != nil {
		t.Fatalf("session log markdown error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Logged: decision(markdown): native append") {
		t.Fatalf("stdout = %q, want native logged message", stdout.String())
	}
	after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")
	for _, want := range []string{
		"[2026-06-10 10:00] session(start):  === SESSION STARTED ===",
		"decision(markdown): native append",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("session markdown = %q, want %q", after, want)
		}
	}
	for _, notWant := range []string{
		"last_updated: 2026-06-10T10:00:00Z",
		"last_entry: 2026-06-10T10:00:00Z",
	} {
		if strings.Contains(after, notWant) {
			t.Fatalf("session markdown = %q, did not want stale %q", after, notWant)
		}
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown log not to create SQLite database", err)
	}
}

func TestRunnerSessionLogFromHookUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-hook-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
last_entry: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"markdown-hook-session","hook_event_name":"TaskCompleted","task_description":"write native markdown hook log"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--from-hook", "--session-id", "missing-session"})
	if err != nil {
		t.Fatalf("session log markdown hook error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Logged: task(completed): write native markdown hook log") {
		t.Fatalf("stdout = %q, want native hook logged message", stdout.String())
	}
	after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")
	if !strings.Contains(after, "task(completed): write native markdown hook log") {
		t.Fatalf("session markdown = %q, want hook entry", after)
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown hook log not to create SQLite database", err)
	}
}

func TestRunnerSessionLogAutoResumesStoppedMarkdownSession(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: stopped
branch: main
claude_session_id: markdown-stopped-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
last_entry: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
[2026-06-10 10:05] session(stop):   === SESSION STOPPED ===
`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "discover(markdown): resumed by native log"})
	if err != nil {
		t.Fatalf("session log stopped markdown error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Auto-resumed stopped session") || !strings.Contains(stdout.String(), "Logged: discover(markdown): resumed by native log") {
		t.Fatalf("stdout = %q, want auto-resume and logged messages", stdout.String())
	}
	after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")
	for _, want := range []string{
		"status: active",
		"session(resume): === SESSION RESUMED ===",
		"discover(markdown): resumed by native log",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("session markdown = %q, want %q", after, want)
		}
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown stopped log not to create SQLite database", err)
	}
}

func TestRunnerSessionLogDetectLinearUsesMarkdownSessionWhenMarkdownOnly(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-session.md", `---
status: active
branch: main
claude_session_id: markdown-linear-session
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:00:00Z
last_entry: 2026-06-10T10:00:00Z
---
# Session

## Journal

[2026-06-10 10:00] session(start):  === SESSION STARTED ===
`)
	if err := os.WriteFile(filepath.Join(workingDir, "linear-markdown.txt"), []byte("linear\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(linear-markdown.txt) error = %v", err)
	}
	gitCLI(t, workingDir, "add", "linear-markdown.txt")
	gitCLI(t, workingDir, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "Resolves ENG-777")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--detect-linear"})
	if err != nil {
		t.Fatalf("session log markdown detect-linear error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Logged: discover(linear): found magic words for ENG-777") {
		t.Fatalf("stdout = %q, want native Linear detection logged message", stdout.String())
	}
	after := readCLIAgentsFile(t, workingDir, "sessions/20260610-session.md")
	if !strings.Contains(after, "discover(linear): found magic words for ENG-777") {
		t.Fatalf("session markdown = %q, want Linear detection entry", after)
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown detect-linear log not to create SQLite database", err)
	}
}

func TestRunnerSessionLogAdoptsMostRecentActiveMarkdownSession(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-older-session.md", `---
status: active
branch: older-branch
created: 2026-06-10T09:00:00Z
last_updated: 2026-06-10T09:00:00Z
---
# Session

## Journal
`)
	writeCLIAgentsFile(t, workingDir, "sessions/20260610-newer-session.md", `---
status: active
branch: newer-branch
created: 2026-06-10T10:00:00Z
last_updated: 2026-06-10T10:30:00Z
---
# Session

## Journal
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stderr:     &stderr,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "decision(markdown): adopted active session"})
	if err != nil {
		t.Fatalf("session log markdown adoption error = %v", err)
	}
	if !strings.Contains(stderr.String(), "WARN: no session for branch 'main'; logging to most-recent active session '20260610-newer-session.md' (origin branch 'newer-branch')") {
		t.Fatalf("stderr = %q, want most-recent active adoption warning", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Logged: decision(markdown): adopted active session") {
		t.Fatalf("stdout = %q, want native logged message", stdout.String())
	}
	newer := readCLIAgentsFile(t, workingDir, "sessions/20260610-newer-session.md")
	if !strings.Contains(newer, "decision(markdown): adopted active session") {
		t.Fatalf("newer session markdown = %q, want adopted entry", newer)
	}
	older := readCLIAgentsFile(t, workingDir, "sessions/20260610-older-session.md")
	if strings.Contains(older, "decision(markdown): adopted active session") {
		t.Fatalf("older session markdown = %q, did not want adopted entry", older)
	}
	if _, err := os.Stat(stateDBPathForWorkingDir(t, workingDir, stateHome)); !os.IsNotExist(err) {
		t.Fatalf("state db stat = %v, want markdown adoption log not to create SQLite database", err)
	}
}

func TestRunnerSessionLogFromHookUsesSQLiteStateWhenInitialized(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "hook-session",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	hookPayload := `{"session_id":"hook-session","hook_event_name":"TaskCompleted","task_description":"port hook logging"}`
	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(hookPayload),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--from-hook", "--json"})
	if err != nil {
		t.Fatalf("session log --from-hook --json error = %v", err)
	}
	result := decodeJournalLogResult(t, stdout.Bytes())
	if result.EntryType != "task" || result.Scope != "completed" || result.Message != "port hook logging" {
		t.Fatalf("result = %#v, want TaskCompleted entry", result)
	}
	if result.Session == nil || result.Session.ID != start.Session.ID {
		t.Fatalf("result session = %#v, want linked started session", result.Session)
	}

	var showOut bytes.Buffer
	if err := (Runner{Stdout: &showOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"session", "show", start.Session.Alias, "--json"}); err != nil {
		t.Fatalf("session show error = %v", err)
	}
	show := decodeSessionShow(t, showOut.Bytes())
	if !hasSessionJournalEntry(show.Session.JournalEntries, "task", "completed", "port hook logging") {
		t.Fatalf("journal entries = %#v, want linked hook entry", show.Session.JournalEntries)
	}
}

func TestRunnerSessionLogFromHookNoopsWithoutPayloadOrSession(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader(`{"session_id":"missing","hook_event_name":"TaskCompleted","task_description":"nothing"}`),
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--from-hook"})
	if err != nil {
		t.Fatalf("session log --from-hook missing session error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want silent noop", stdout.String())
	}
}

func TestRunnerSessionLogDetectLinearUsesSQLiteStateWhenInitialized(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch: "main",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDir, "linear.txt"), []byte("linear\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(linear.txt) error = %v", err)
	}
	gitCLI(t, workingDir, "add", "linear.txt")
	gitCLI(t, workingDir, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "Fixes ENG-123 and closes PLT-456")

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--detect-linear"})
	if err != nil {
		t.Fatalf("session log --detect-linear error = %v", err)
	}
	if !strings.Contains(stdout.String(), "logged journal entry:") {
		t.Fatalf("stdout = %q, want logged journal entry", stdout.String())
	}

	show, err := state.ShowSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if !hasSessionJournalEntry(show.Session.JournalEntries, "discover", "linear", "found magic words for ENG-123, PLT-456") {
		t.Fatalf("journal entries = %#v, want Linear detection entry", show.Session.JournalEntries)
	}
}

func TestRunnerSessionLogDetectLinearNoopsWithoutMagicWords(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch: "main",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--detect-linear"})
	if err != nil {
		t.Fatalf("session log --detect-linear error = %v", err)
	}
	if !strings.Contains(stdout.String(), "No Linear magic words detected") {
		t.Fatalf("stdout = %q, want no-detection message", stdout.String())
	}
	show, err := state.ShowSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if hasSessionJournalEntry(show.Session.JournalEntries, "discover", "linear", "found magic words for") {
		t.Fatalf("journal entries = %#v, want no Linear detection entry", show.Session.JournalEntries)
	}
}

func TestRunnerSessionLogDetectLinearNoopsWhenIntegrationDisabled(t *testing.T) {
	requireCLIGit(t)
	workingDir := initCLIGitRepo(t)
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.Initialize(t.Context(), root, state.PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := state.StartSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.SessionStartOptions{
		Branch: "main",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	writeCLIAgentsFile(t, workingDir, "loaf.json", `{"integrations":{"linear":{"enabled":false}}}`)
	if err := os.WriteFile(filepath.Join(workingDir, "disabled-linear.txt"), []byte("linear\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(disabled-linear.txt) error = %v", err)
	}
	gitCLI(t, workingDir, "add", "disabled-linear.txt", ".agents/loaf.json")
	gitCLI(t, workingDir, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "Resolves ENG-999")

	var stdout bytes.Buffer
	err = Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "--detect-linear"})
	if err != nil {
		t.Fatalf("session log --detect-linear error = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want silent disabled noop", stdout.String())
	}
	show, err := state.ShowSession(t.Context(), root, state.PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if hasSessionJournalEntry(show.Session.JournalEntries, "discover", "linear", "found magic words for ENG-999") {
		t.Fatalf("journal entries = %#v, want no Linear detection entry when disabled", show.Session.JournalEntries)
	}
}

func TestRunnerSessionLogReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"session", "log", "decision(sqlite): invalid"})
	if err == nil {
		t.Fatal("session log error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func TestRunnerReportListJSONUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "reports/draft.md", `---
title: Draft Report
type: research
status: draft
source: ad-hoc
---
# Draft Report
`)
	writeCLIAgentsFile(t, workingDir, "reports/final.md", `---
title: Final Report
kind: audit
status: final
source: SPEC-001
---
# Final Report
`)
	writeCLIAgentsFile(t, workingDir, "reports/archive/old.md", `---
title: Old Report
type: research
status: final
source: old
---
# Old Report
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "list", "--json", "--type", "research"})
	if err != nil {
		t.Fatalf("report list --json --type research error = %v", err)
	}

	reports := decodeReportList(t, stdout.Bytes())
	if len(reports.Reports) != 2 {
		t.Fatalf("reports = %#v, want two research reports", reports.Reports)
	}
	draft := reports.Reports["draft"]
	if draft.Title != "Draft Report" || draft.Kind != "research" || draft.Status != "draft" || draft.SourcePath != ".agents/reports/draft.md" {
		t.Fatalf("draft report = %#v, want imported metadata", draft)
	}
	archived := reports.Reports["old"]
	if archived.Status != "archived" || archived.SourcePath != ".agents/reports/archive/old.md" {
		t.Fatalf("archived report = %#v, want archive-location status", archived)
	}
}

func TestRunnerReportListHumanUsesSQLiteStateWhenInitialized(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "reports/draft.md", `---
title: Draft Report
type: research
status: draft
source: ad-hoc
---
# Draft Report
`)
	writeCLIAgentsFile(t, workingDir, "reports/final.md", `---
title: Final Report
type: audit
status: final
source: SPEC-001
---
# Final Report
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{}}
`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "list", "--status", "final"})
	if err != nil {
		t.Fatalf("report list --status final error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"loaf report list", "Final:", "Final Report", "[audit]", ".agents/reports/final.md", "1 report(s) total"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "Draft Report") {
		t.Fatalf("output = %q, want draft report filtered out", output)
	}
}

func TestRunnerReportListUsesMarkdownReportsWhenMarkdownOnly(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	writeCLIAgentsFile(t, workingDir, "reports/draft.md", `---
title: Draft Report
type: research
status: draft
source: ad-hoc
---
# Draft Report
`)
	writeCLIAgentsFile(t, workingDir, "reports/final.md", `---
title: Final Report
kind: audit
status: final
source: SPEC-001
---
# Final Report
`)
	writeCLIAgentsFile(t, workingDir, "reports/archive/old.md", `---
title: Old Report
type: research
status: final
source: old
---
# Old Report
`)

	var jsonOut bytes.Buffer
	err := Runner{
		Stdout:     &jsonOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "list", "--json", "--type", "research"})
	if err != nil {
		t.Fatalf("report list markdown --json --type research error = %v", err)
	}
	reports := decodeReportList(t, jsonOut.Bytes())
	if len(reports.Reports) != 2 {
		t.Fatalf("reports = %#v, want two markdown research reports", reports.Reports)
	}
	draft := reports.Reports["draft"]
	if draft.Title != "Draft Report" || draft.Kind != "research" || draft.Status != "draft" || draft.SourcePath != ".agents/reports/draft.md" {
		t.Fatalf("draft report = %#v, want markdown metadata", draft)
	}
	archived := reports.Reports["old"]
	if archived.Title != "Old Report" || archived.Kind != "research" || archived.Status != "archived" || archived.SourcePath != ".agents/reports/archive/old.md" {
		t.Fatalf("archived report = %#v, want archive-location status", archived)
	}

	var humanOut bytes.Buffer
	err = Runner{
		Stdout:     &humanOut,
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "list", "--status", "final"})
	if err != nil {
		t.Fatalf("report list markdown --status final error = %v", err)
	}
	output := humanOut.String()
	for _, want := range []string{"loaf report list", "Final:", "Final Report", "[audit]", ".agents/reports/final.md", "1 report(s) total"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "Draft Report") || strings.Contains(output, "Old Report") {
		t.Fatalf("output = %q, want status filter to hide non-final reports", output)
	}
	assertNoStateDatabase(t, workingDir, stateHome)
}

func TestRunnerReportListReportsInvalidSQLiteState(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
		StateHome:  stateHome,
	}.Run([]string{"report", "list"})
	if err == nil {
		t.Fatal("report list error = nil, want invalid state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid") {
		t.Fatalf("error = %v, want invalid state error", err)
	}
}

func decodeStateStatus(t *testing.T, data []byte) state.Status {
	t.Helper()
	var status state.Status
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return status
}

func decodeStateBackupResult(t *testing.T, data []byte) state.BackupResult {
	t.Helper()
	var result state.BackupResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeStateExportSnapshot(t *testing.T, data []byte) state.ExportSnapshot {
	t.Helper()
	var snapshot state.ExportSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return snapshot
}

func decodeMarkdownMigrationPlan(t *testing.T, data []byte) state.MarkdownMigrationPlan {
	t.Helper()
	var plan state.MarkdownMigrationPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return plan
}

func decodeMarkdownMigrationResult(t *testing.T, data []byte) state.MarkdownMigrationResult {
	t.Helper()
	var result state.MarkdownMigrationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTraceResult(t *testing.T, data []byte) state.TraceResult {
	t.Helper()
	var result state.TraceResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTaskList(t *testing.T, data []byte) state.TaskList {
	t.Helper()
	var result state.TaskList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTaskShow(t *testing.T, data []byte) state.TaskShow {
	t.Helper()
	var result state.TaskShow
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTaskCreateResult(t *testing.T, data []byte) state.TaskCreateResult {
	t.Helper()
	var result state.TaskCreateResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTaskStatusUpdateResult(t *testing.T, data []byte) state.TaskStatusUpdateResult {
	t.Helper()
	var result state.TaskStatusUpdateResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTaskArchiveResult(t *testing.T, data []byte) state.TaskArchiveResult {
	t.Helper()
	var result state.TaskArchiveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeIdeaList(t *testing.T, data []byte) state.IdeaList {
	t.Helper()
	var result state.IdeaList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBrainstormList(t *testing.T, data []byte) state.BrainstormList {
	t.Helper()
	var result state.BrainstormList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBrainstormShow(t *testing.T, data []byte) state.BrainstormShow {
	t.Helper()
	var result state.BrainstormShow
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBrainstormPromoteResult(t *testing.T, data []byte) state.BrainstormPromoteResult {
	t.Helper()
	var result state.BrainstormPromoteResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBrainstormArchiveResult(t *testing.T, data []byte) state.BrainstormArchiveResult {
	t.Helper()
	var result state.BrainstormArchiveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeIdeaShow(t *testing.T, data []byte) state.IdeaShow {
	t.Helper()
	var result state.IdeaShow
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeIdeaCaptureResult(t *testing.T, data []byte) state.IdeaCaptureResult {
	t.Helper()
	var result state.IdeaCaptureResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeIdeaResolveResult(t *testing.T, data []byte) state.IdeaResolveResult {
	t.Helper()
	var result state.IdeaResolveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeIdeaPromoteResult(t *testing.T, data []byte) state.IdeaPromoteResult {
	t.Helper()
	var result state.IdeaPromoteResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeIdeaArchiveResult(t *testing.T, data []byte) state.IdeaArchiveResult {
	t.Helper()
	var result state.IdeaArchiveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSparkList(t *testing.T, data []byte) state.SparkList {
	t.Helper()
	var result state.SparkList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSparkShow(t *testing.T, data []byte) state.SparkShow {
	t.Helper()
	var result state.SparkShow
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSparkCaptureResult(t *testing.T, data []byte) state.SparkCaptureResult {
	t.Helper()
	var result state.SparkCaptureResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSparkResolveResult(t *testing.T, data []byte) state.SparkResolveResult {
	t.Helper()
	var result state.SparkResolveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSparkPromoteResult(t *testing.T, data []byte) state.SparkPromoteResult {
	t.Helper()
	var result state.SparkPromoteResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTagList(t *testing.T, data []byte) state.TagList {
	t.Helper()
	var result state.TagList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTagShowResult(t *testing.T, data []byte) state.TagShowResult {
	t.Helper()
	var result state.TagShowResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeTagMutationResult(t *testing.T, data []byte) state.TagMutationResult {
	t.Helper()
	var result state.TagMutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBundleShowResult(t *testing.T, data []byte) state.BundleShowResult {
	t.Helper()
	var result state.BundleShowResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBundleList(t *testing.T, data []byte) state.BundleList {
	t.Helper()
	var result state.BundleList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeBundleMutationResult(t *testing.T, data []byte) state.BundleMutationResult {
	t.Helper()
	var result state.BundleMutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeHousekeepingSummary(t *testing.T, data []byte) state.HousekeepingSummary {
	t.Helper()
	var result state.HousekeepingSummary
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeCompatibilityCommandSummary(t *testing.T, data []byte) compatibilityCommandSummary {
	t.Helper()
	var result compatibilityCommandSummary
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeLinkMutationResult(t *testing.T, data []byte) state.LinkMutationResult {
	t.Helper()
	var result state.LinkMutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeLinkListResult(t *testing.T, data []byte) state.LinkListResult {
	t.Helper()
	var result state.LinkListResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSpecList(t *testing.T, data []byte) state.SpecList {
	t.Helper()
	var result state.SpecList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSpecShow(t *testing.T, data []byte) state.SpecShow {
	t.Helper()
	var result state.SpecShow
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSpecArchiveResult(t *testing.T, data []byte) state.SpecArchiveResult {
	t.Helper()
	var result state.SpecArchiveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSessionList(t *testing.T, data []byte) state.SessionList {
	t.Helper()
	var result state.SessionList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSessionShow(t *testing.T, data []byte) state.SessionShow {
	t.Helper()
	var result state.SessionShow
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSessionStart(t *testing.T, data []byte) state.SessionStartResult {
	t.Helper()
	var result state.SessionStartResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSessionEnd(t *testing.T, data []byte) state.SessionEndResult {
	t.Helper()
	var result state.SessionEndResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeSessionArchive(t *testing.T, data []byte) state.SessionArchiveResult {
	t.Helper()
	var result state.SessionArchiveResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeJournalLogResult(t *testing.T, data []byte) state.JournalLogResult {
	t.Helper()
	var result state.JournalLogResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func hasSessionJournalEntry(entries []state.SessionJournalEntry, entryType string, scope string, message string) bool {
	for _, entry := range entries {
		if entry.EntryType == entryType && entry.Scope == scope && entry.Message == message {
			return true
		}
	}
	return false
}

func decodeReportList(t *testing.T, data []byte) state.ReportList {
	t.Helper()
	var result state.ReportList
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeReportCreateResult(t *testing.T, data []byte) state.ReportCreateResult {
	t.Helper()
	var result state.ReportCreateResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func decodeReportStatusResult(t *testing.T, data []byte) state.ReportStatusResult {
	t.Helper()
	var result state.ReportStatusResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
}

func hasTraceRelationship(relationships []state.TraceRelationship, direction string, relationshipType string, kind string, alias string) bool {
	for _, relationship := range relationships {
		if relationship.Direction == direction && relationship.Type == relationshipType && relationship.Entity.Kind == kind && relationship.Entity.Alias == alias {
			return true
		}
	}
	return false
}

func repoFileList(t *testing.T, root string) []string {
	t.Helper()
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", root, err)
	}
	return files
}

func stateDBPathForWorkingDir(t *testing.T, workingDir string, stateHome string) string {
	t.Helper()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	return databasePath
}

func writeInvalidDatabaseFileForCLI(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func hasDiagnostic(diagnostics []state.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func sqliteCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("sqlite count query %q error = %v", query, err)
	}
	return count
}

func sqliteEntityStatus(t *testing.T, db *sql.DB, table string, kind string, alias string) string {
	t.Helper()
	query := "SELECT " + table + ".status FROM " + table + " JOIN aliases ON aliases.project_id = " + table + ".project_id AND aliases.entity_kind = ? AND aliases.entity_id = " + table + ".id WHERE aliases.alias = ?"
	var status string
	if err := db.QueryRow(query, kind, alias).Scan(&status); err != nil {
		t.Fatalf("sqlite status query for %s %s error = %v", kind, alias, err)
	}
	return status
}

func writeCLIAgentsFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, ".agents", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func readCLIAgentsFile(t *testing.T, root string, rel string) string {
	t.Helper()
	path := filepath.Join(root, ".agents", filepath.FromSlash(rel))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}

func writeInvalidStateDatabase(t *testing.T, workingDir string, stateHome string) {
	t.Helper()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func requireCLIGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func initCLIGitRepo(t *testing.T) string {
	t.Helper()
	repo := realpath(t, t.TempDir())
	gitCLI(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	gitCLI(t, repo, "add", "README.md")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	return repo
}

func addCLILinkedWorktree(t *testing.T, repo string, branch string) string {
	t.Helper()
	linked := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-"+branch)
	gitCLI(t, repo, "worktree", "add", "-b", branch, linked)
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", linked).Run()
	})
	return realpath(t, linked)
}

func seedCLIWorktreeAgents(t *testing.T, worktreePath string) []string {
	t.Helper()
	files := []struct {
		rel  string
		body string
	}{
		{"AGENTS.md", "# Worktree AGENTS\n"},
		{"loaf.json", "{\"foo\":\"bar\"}\n"},
		{"sessions/20260519-120000-session.md", "# Session\n"},
		{"kb/some-note.md", "# KB Note\n"},
		{"tasks/TASK-200-example.md", "# Task 200\n"},
		{"specs/SPEC-040-example.md", "# Spec 040\n"},
	}
	var rels []string
	for _, file := range files {
		target := filepath.Join(worktreePath, ".agents", filepath.FromSlash(file.rel))
		mkdirAll(t, filepath.Dir(target))
		writeFile(t, target, file.body)
		rels = append(rels, file.rel)
	}
	return rels
}

func gitCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestRunnerRootHelpIsNative(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"-h"}, {"help"}} {
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			WorkingDir: t.TempDir(),
		}.Run(args)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		for _, want := range []string{"Usage: loaf <command> [options]", "Commands:", "session", "task", "release"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("Run(%v) stdout = %q, want %q", args, stdout.String(), want)
			}
		}
	}
}

func TestRunnerUnknownTopLevelCommandIsNative(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stderr:     &stderr,
		WorkingDir: t.TempDir(),
	}.Run([]string{"not-a-command"})
	if err == nil {
		t.Fatal("Run(not-a-command) error = nil, want native unknown-command exit")
	}
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("Run(not-a-command) error = %T %[1]v, want ExitError{Code:1}", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"error: unknown command 'not-a-command'", "Usage: loaf <command> [options]"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunnerAgentHelpIsNative(t *testing.T) {
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: t.TempDir(),
	}.Run([]string{"--agent-help"})
	if err != nil {
		t.Fatalf("Run(--agent-help) error = %v", err)
	}
	var doc struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Commands    []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Subcommands []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Options     []struct {
					Flags       string `json:"flags"`
					Description string `json:"description"`
				} `json:"options"`
			} `json:"subcommands"`
			Options []struct {
				Flags       string `json:"flags"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("Unmarshal(agent help) error = %v\n%s", err, stdout.String())
	}
	if doc.Name != "loaf" || !strings.Contains(doc.Description, "Opinionated Agentic Framework") {
		t.Fatalf("agent help root = %#v, want loaf metadata", doc)
	}
	commands := map[string]struct {
		subcommands []string
		options     []string
	}{}
	for _, command := range doc.Commands {
		entry := commands[command.Name]
		for _, subcommand := range command.Subcommands {
			entry.subcommands = append(entry.subcommands, subcommand.Name)
			for _, option := range subcommand.Options {
				entry.options = append(entry.options, command.Name+" "+subcommand.Name+" "+option.Flags)
			}
		}
		for _, option := range command.Options {
			entry.options = append(entry.options, command.Name+" "+option.Flags)
		}
		commands[command.Name] = entry
	}
	for _, want := range []string{"build", "session", "task", "spec", "report", "kb", "release", "version"} {
		if _, ok := commands[want]; !ok {
			t.Fatalf("agent help commands missing %q: %#v", want, commands)
		}
	}
	if len(doc.Commands) < 15 {
		t.Fatalf("agent help commands = %d, want full native surface rather than stale release-only JSON", len(doc.Commands))
	}
	for _, want := range []string{"refresh", "sync"} {
		if !stringSliceContains(commands["task"].subcommands, want) {
			t.Fatalf("task subcommands = %#v, want %q", commands["task"].subcommands, want)
		}
	}
	if !stringSliceContains(commands["task"].options, "task sync --import") || !stringSliceContains(commands["task"].options, "task sync --push") {
		t.Fatalf("task options = %#v, want sync import/push options", commands["task"].options)
	}
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return realpath
}

func assertNoStateDatabase(t *testing.T, workingDir string, stateHome string) {
	t.Helper()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("state database stat error = %v, want missing database at %s", err, databasePath)
	}
}
