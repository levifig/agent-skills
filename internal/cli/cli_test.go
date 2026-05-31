package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/legacy"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
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

func TestRunnerHousekeepingDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"housekeeping", "--dry-run"})
	if err != nil {
		t.Fatalf("housekeeping markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=housekeeping --dry-run") {
		t.Fatalf("stdout = %q, want delegated housekeeping", stdout.String())
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

func TestRunnerTaskRefreshAndSyncDelegateWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	runner := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}
	if err := runner.Run([]string{"task", "refresh"}); err != nil {
		t.Fatalf("task refresh markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=task refresh") {
		t.Fatalf("stdout = %q, want delegated task refresh", stdout.String())
	}

	stdout.Reset()
	if err := runner.Run([]string{"task", "sync", "--import"}); err != nil {
		t.Fatalf("task sync markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=task sync --import") {
		t.Fatalf("stdout = %q, want delegated task sync", stdout.String())
	}
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

func TestRunnerSessionEnrichDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"session", "enrich", "20260528-session.md", "--dry-run"})
	if err != nil {
		t.Fatalf("session enrich markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=session enrich 20260528-session.md --dry-run") {
		t.Fatalf("stdout = %q, want delegated session enrich", stdout.String())
	}
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
	if initialized.SchemaVersion != 1 {
		t.Fatalf("initialized.SchemaVersion = %d, want 1", initialized.SchemaVersion)
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
	if !strings.Contains(stdout.String(), "schema version 99 does not match expected version 1") {
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
	if version != 1 {
		t.Fatalf("backup schema version = %d, want 1", version)
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
	if snapshot.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", snapshot.SchemaVersion)
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

func TestRunnerReportLifecycleDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	commands := [][]string{
		{"report", "create", "release-readiness", "--type", "audit"},
		{"report", "finalize", "release-readiness"},
		{"report", "archive", "release-readiness"},
	}
	for _, command := range commands {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			workingDir := realpath(t, t.TempDir())
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: workingDir,
				StateHome:  t.TempDir(),
				Legacy: legacy.Runner{
					ScriptPath: writeLegacyScript(t),
					NodePath:   writeFakeNode(t),
				},
			}.Run(command)
			if err != nil {
				t.Fatalf("%s markdown fallback error = %v", strings.Join(command, " "), err)
			}
			want := "args=" + strings.Join(command, " ")
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout = %q, want delegated %q", stdout.String(), want)
			}
		})
	}
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

func TestRunnerTaskListDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"task", "list", "--json"})
	if err != nil {
		t.Fatalf("task list markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=task list --json") {
		t.Fatalf("stdout = %q, want delegated task list", stdout.String())
	}
}

func TestRunnerTaskCreateDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"task", "create", "--title", "Fallback Task", "--priority", "P1"})
	if err != nil {
		t.Fatalf("task create markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=task create --title Fallback Task --priority P1") {
		t.Fatalf("stdout = %q, want delegated task create", stdout.String())
	}
}

func TestRunnerTaskShowDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"task", "show", "TASK-001", "--json"})
	if err != nil {
		t.Fatalf("task show markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=task show TASK-001 --json") {
		t.Fatalf("stdout = %q, want delegated task show", stdout.String())
	}
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

func TestRunnerTaskUpdateDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var markdownOut bytes.Buffer
	err := Runner{
		Stdout:     &markdownOut,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"task", "update", "TASK-001", "--status", "done"})
	if err != nil {
		t.Fatalf("task update markdown fallback error = %v", err)
	}
	if !strings.Contains(markdownOut.String(), "args=task update TASK-001 --status done") {
		t.Fatalf("stdout = %q, want delegated task update", markdownOut.String())
	}
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

func TestRunnerTaskArchiveDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"task", "archive", "TASK-001"})
	if err != nil {
		t.Fatalf("task archive markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=task archive TASK-001") {
		t.Fatalf("stdout = %q, want delegated task archive", stdout.String())
	}
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

func TestRunnerSpecListDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"spec", "list", "--json"})
	if err != nil {
		t.Fatalf("spec list markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=spec list --json") {
		t.Fatalf("stdout = %q, want delegated spec list", stdout.String())
	}
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

func TestRunnerSpecShowDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"spec", "show", "SPEC-001", "--json"})
	if err != nil {
		t.Fatalf("spec show markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=spec show SPEC-001 --json") {
		t.Fatalf("stdout = %q, want delegated spec show", stdout.String())
	}
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

func TestRunnerSpecArchiveDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"spec", "archive", "SPEC-001"})
	if err != nil {
		t.Fatalf("spec archive markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=spec archive SPEC-001") {
		t.Fatalf("stdout = %q, want delegated spec archive", stdout.String())
	}
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

func TestRunnerSessionListDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"session", "list", "--all"})
	if err != nil {
		t.Fatalf("session list markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=session list --all") {
		t.Fatalf("stdout = %q, want delegated session list", stdout.String())
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

func TestRunnerSessionShowDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"session", "show", "20260528-active", "--json"})
	if err != nil {
		t.Fatalf("session show markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=session show 20260528-active --json") {
		t.Fatalf("stdout = %q, want delegated session show", stdout.String())
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

func TestRunnerSessionLogDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"session", "log", "decision(sqlite): fallback"})
	if err != nil {
		t.Fatalf("session log markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=session log decision(sqlite): fallback") {
		t.Fatalf("stdout = %q, want delegated session log", stdout.String())
	}
}

func TestRunnerSessionLogDelegatesHookModesWhenSQLiteReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	for _, args := range [][]string{{"session", "log", "--from-hook"}, {"session", "log", "--detect-linear"}} {
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			WorkingDir: workingDir,
			StateHome:  stateHome,
			Legacy: legacy.Runner{
				ScriptPath: writeLegacyScript(t),
				NodePath:   writeFakeNode(t),
			},
		}.Run(args)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		want := "args=" + strings.Join(args, " ")
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want delegated %q", stdout.String(), want)
		}
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

func TestRunnerReportListDelegatesWhenMarkdownOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  t.TempDir(),
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"report", "list", "--type", "research"})
	if err != nil {
		t.Fatalf("report list markdown fallback error = %v", err)
	}
	if !strings.Contains(stdout.String(), "args=report list --type research") {
		t.Fatalf("stdout = %q, want delegated report list", stdout.String())
	}
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

func TestRunnerDelegatesUnmigratedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	workingDir := t.TempDir()
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		Legacy: legacy.Runner{
			ScriptPath: writeLegacyScript(t),
			NodePath:   writeFakeNode(t),
		},
	}.Run([]string{"task", "list", "--json"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "args=task list --json") {
		t.Fatalf("stdout = %q, want delegated argv", out)
	}
	if !containsCwd(out, workingDir) {
		t.Fatalf("stdout = %q, want delegated cwd %q", out, workingDir)
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

func decodeJournalLogResult(t *testing.T, data []byte) state.JournalLogResult {
	t.Helper()
	var result state.JournalLogResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(data), err)
	}
	return result
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
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "commit", "-m", "initial")
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

func gitCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestRunnerDelegatesHelpAndVersionDuringBridgePeriod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	for _, args := range [][]string{{"--help"}, {"--version"}, {}} {
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			WorkingDir: t.TempDir(),
			Legacy: legacy.Runner{
				ScriptPath: writeLegacyScript(t),
				NodePath:   writeFakeNode(t),
			},
		}.Run(args)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		want := "args=" + strings.Join(args, " ")
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("Run(%v) stdout = %q, want %q", args, stdout.String(), want)
		}
	}
}

func writeFakeNode(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "node")
	body := "#!/bin/sh\nexec \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(node) error = %v", err)
	}
	return path
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return realpath
}

func containsCwd(output string, cwd string) bool {
	candidates := []string{cwd}
	if realpath, err := filepath.EvalSymlinks(cwd); err == nil && realpath != cwd {
		candidates = append(candidates, realpath)
	}
	if strings.HasPrefix(cwd, "/var/") || strings.HasPrefix(cwd, "/tmp/") {
		candidates = append(candidates, "/private"+cwd)
	}
	for _, candidate := range candidates {
		if strings.Contains(output, "cwd="+candidate) {
			return true
		}
	}
	return false
}

func writeLegacyScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy-cli")
	body := `#!/bin/sh
input="$(cat)"
printf 'cwd=%s\n' "$PWD"
printf 'args=%s\n' "$*"
printf 'input=%s\n' "$input"
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(legacy-cli) error = %v", err)
	}
	return path
}
