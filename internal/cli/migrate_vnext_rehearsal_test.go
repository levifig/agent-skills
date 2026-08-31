package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/archive"
)

type vnextRehearsalCLIFixture struct {
	workingDir  string
	stateHome   string
	projectRoot project.Root
	database    string
	backup      state.BackupResult
}

func TestRunnerMigrateVNextRehearsalJSONCreatesVerifiedDisposableDestination(t *testing.T) {
	fixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(fixture.workingDir), "isolated-json")
	liveBefore := rehearsalTestFileSHA256(t, fixture.database)
	backupBefore := rehearsalTestFileSHA256(t, fixture.backup.BackupPath)
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: fixture.workingDir, StateHome: fixture.stateHome}.Run([]string{
		"migrate", "vnext-rehearsal", "--backup", fixture.backup.BackupPath, "--to", destination, "--json",
	})
	if err != nil {
		t.Fatalf("Run(migrate vnext-rehearsal --json) error = %v\n%s", err, stdout.String())
	}
	var result migrateVNextRehearsalResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if result.ContractVersion != 1 || result.Command != "migrate vnext-rehearsal" || result.Mode != "isolated-rehearsal" {
		t.Fatalf("command evidence = %#v", result)
	}
	if result.ProjectID == "" || result.ProjectRoot != fixture.projectRoot.Path() {
		t.Fatalf("project evidence = %#v", result)
	}
	if result.Source.BackupPath != fixture.backup.BackupPath || result.Source.SHA256 != fixture.backup.SHA256 ||
		result.Source.Bytes == 0 || result.Source.SchemaVersion != state.CurrentSchemaVersion() || !result.Source.Verified {
		t.Fatalf("source evidence = %#v", result.Source)
	}
	if result.Destination.Root != destination || result.Destination.Database != filepath.Join(destination, "vnext", "continuity.sqlite") ||
		!strings.HasPrefix(string(result.Destination.EnvironmentID), "environment_") {
		t.Fatalf("destination evidence = %#v", result.Destination)
	}
	if result.Archive.Format != archive.Format || result.Archive.Version != archive.Version ||
		result.Archive.ContentSHA256 == "" || result.Archive.RecordCount != 4 {
		t.Fatalf("archive evidence = %#v", result.Archive)
	}
	if result.Archive.Source.NormalizedJournalCategoryRows != 1 ||
		result.Archive.Source.JournalCategoryMapping != archive.JournalCategoryMappingUnsupportedToNoteV1 {
		t.Fatalf("archive source normalization evidence = %#v", result.Archive.Source)
	}
	if !result.Archive.Families.Project || !result.Archive.Families.Journal || !result.Archive.Families.Wrap || !result.Archive.Families.Handoffs ||
		result.Archive.Families.Sparks || result.Archive.Families.Scratchpads || result.Archive.Families.CompleteForCutover {
		t.Fatalf("archive families = %#v", result.Archive.Families)
	}
	if result.Archive.Expected != result.Archive.Actual || !result.ProjectionVerified || !result.Disposable || result.ActivationPerformed || result.CutoverReady || !result.Succeeded {
		t.Fatalf("safety evidence = %#v", result)
	}
	if strings.Contains(stdout.String(), "do-not-render-this-body") {
		t.Fatalf("JSON output leaked continuity body: %s", stdout.String())
	}
	if got := rehearsalTestFileSHA256(t, fixture.database); got != liveBefore {
		t.Fatalf("live database changed: got %s, want %s", got, liveBefore)
	}
	if got := rehearsalTestFileSHA256(t, fixture.backup.BackupPath); got != backupBefore {
		t.Fatalf("backup changed: got %s, want %s", got, backupBefore)
	}
	assertRehearsalPrivateMode(t, destination, 0o700)
	assertRehearsalPrivateMode(t, filepath.Join(destination, "vnext"), 0o700)
	assertRehearsalPrivateMode(t, result.Destination.Database, 0o600)
}

func TestRunnerMigrateVNextRehearsalHumanLeadsWithIsolationAndNoActivation(t *testing.T) {
	fixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(fixture.workingDir), "isolated-human")
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: fixture.workingDir, StateHome: fixture.stateHome}.Run([]string{
		"migrate", "vnext-rehearsal", "--backup", fixture.backup.BackupPath, "--to", destination,
	})
	if err != nil {
		t.Fatalf("Run(migrate vnext-rehearsal) error = %v", err)
	}
	output := stdout.String()
	if !strings.HasPrefix(output, "Verified isolated vNext rehearsal completed; no activation or cutover was performed.\n") {
		t.Fatalf("human output lead = %q", output)
	}
	for _, want := range []string{
		"journal category normalization: unsupported_legacy_journal_category_to_note_v1 (1 rows)",
		"projection verified: true", "disposable: true", "activation performed: false", "cutover ready: false",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("human output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "do-not-render-this-body") {
		t.Fatalf("human output leaked continuity body: %s", output)
	}
}

func TestParseMigrateVNextRehearsalArgsRejectsAmbiguousAndUnsafeShapes(t *testing.T) {
	absoluteBackup := filepath.Join(t.TempDir(), "backup.sqlite")
	absoluteTarget := filepath.Join(t.TempDir(), "target")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing backup", args: []string{"--to", absoluteTarget}, want: "--backup is required"},
		{name: "missing target", args: []string{"--backup", absoluteBackup}, want: "--to is required"},
		{name: "duplicate backup", args: []string{"--backup", absoluteBackup, "--backup", absoluteBackup, "--to", absoluteTarget}, want: "only once"},
		{name: "duplicate target", args: []string{"--backup", absoluteBackup, "--to", absoluteTarget, "--to", absoluteTarget}, want: "only once"},
		{name: "duplicate json", args: []string{"--backup", absoluteBackup, "--to", absoluteTarget, "--json", "--json"}, want: "only once"},
		{name: "unknown", args: []string{"--backup", absoluteBackup, "--to", absoluteTarget, "--force"}, want: "unknown"},
		{name: "relative backup", args: []string{"--backup", "backup.sqlite", "--to", absoluteTarget}, want: "backup path must be absolute"},
		{name: "relative target", args: []string{"--backup", absoluteBackup, "--to", "target"}, want: "destination path must be absolute"},
		{name: "unclean backup", args: []string{"--backup", absoluteBackup + "/../backup.sqlite", "--to", absoluteTarget}, want: "backup path must be clean"},
		{name: "unclean target", args: []string{"--backup", absoluteBackup, "--to", absoluteTarget + "/../target"}, want: "destination path must be clean"},
		{name: "root target", args: []string{"--backup", absoluteBackup, "--to", string(filepath.Separator)}, want: "filesystem root"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseMigrateVNextRehearsalArgs(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseMigrateVNextRehearsalArgs() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateVNextRehearsalDestinationRejectsExistingSymlinkAndContainment(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "project")
	liveTree := filepath.Join(base, "state", "loaf")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(liveTree, 0o700); err != nil {
		t.Fatal(err)
	}
	liveDatabase := filepath.Join(liveTree, "loaf.sqlite")
	if err := os.WriteFile(liveDatabase, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(base, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkAncestor := filepath.Join(base, "symlink-parent")
	realParent := filepath.Join(base, "real-parent")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(realParent, "existing"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realParent, symlinkAncestor); err != nil {
		t.Fatal(err)
	}
	nonDirectory := filepath.Join(base, "plain-file")
	if err := os.WriteFile(nonDirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "existing", path: existing, want: "already exists"},
		{name: "missing parent", path: filepath.Join(base, "missing-parent", "new"), want: "parent must already exist"},
		{name: "symlink ancestor", path: filepath.Join(symlinkAncestor, "existing", "new"), want: "symlink"},
		{name: "non-directory parent", path: filepath.Join(nonDirectory, "new"), want: "non-directory parent"},
		{name: "project descendant", path: filepath.Join(projectRoot, "new"), want: "outside the authoritative project root"},
		{name: "live descendant", path: filepath.Join(liveTree, "new"), want: "live state tree"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateVNextRehearsalDestination(test.path, projectRoot, liveDatabase)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateVNextRehearsalDestination() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunnerMigrateVNextRehearsalJSONErrorIsMachineReadableAndDoesNotReserveDestination(t *testing.T) {
	fixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(fixture.workingDir), "never-created")
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: fixture.workingDir, StateHome: fixture.stateHome}.Run([]string{
		"migrate", "vnext-rehearsal", "--backup", fixture.backup.BackupPath, "--to", destination, "--unknown", "--json",
	})
	if err == nil {
		t.Fatal("Run(invalid --json) error = nil")
	}
	var result commandErrorJSON
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout.String(), decodeErr)
	}
	if result.Command != "migrate vnext-rehearsal" || !strings.Contains(result.Error, "unknown") {
		t.Fatalf("JSON error = %#v", result)
	}
	if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("destination stat error = %v, want not exist", statErr)
	}
}

func TestValidateVNextRehearsalBackupRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "backup.sqlite")
	if err := os.WriteFile(regular, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "backup-link.sqlite")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	if err := validateVNextRehearsalBackupPath(symlink, regular); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("validateVNextRehearsalBackupPath() error = %v, want symlink refusal", err)
	}
	if err := validateVNextRehearsalBackupPath(regular, regular); err == nil || !strings.Contains(err.Error(), "standalone backup") {
		t.Fatalf("validateVNextRehearsalBackupPath(live) error = %v, want live-database refusal", err)
	}
}

func TestRunnerMigrateVNextRehearsalForeignBackupDoesNotReserveDestination(t *testing.T) {
	liveFixture := prepareVNextRehearsalCLIFixture(t)
	foreignFixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(liveFixture.workingDir), "preserved-failure")
	backupBefore := rehearsalTestFileSHA256(t, foreignFixture.backup.BackupPath)
	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: liveFixture.workingDir, StateHome: liveFixture.stateHome}.Run([]string{
		"migrate", "vnext-rehearsal", "--backup", foreignFixture.backup.BackupPath, "--to", destination,
	})
	if err == nil || !strings.Contains(err.Error(), "is absent from the verified backup") {
		t.Fatalf("Run(foreign backup) error = %v, want selected-project refusal", err)
	}
	if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("foreign backup destination stat error = %v, want absent", statErr)
	}
	if got := rehearsalTestFileSHA256(t, foreignFixture.backup.BackupPath); got != backupBefore {
		t.Fatalf("failed rehearsal changed source backup: got %s, want %s", got, backupBefore)
	}
}

func TestRunnerMigrateVNextRehearsalJSONFailureAfterReservationEmitsStructuredEvidenceAndPreservesArtifacts(t *testing.T) {
	fixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(fixture.workingDir), "preserved-injected-failure")
	backupBefore := rehearsalTestFileSHA256(t, fixture.backup.BackupPath)
	workingDirectory, err := project.ResolveWorkingDirectory(fixture.workingDir)
	if err != nil {
		t.Fatalf("project.ResolveWorkingDirectory() error = %v", err)
	}
	var stdout bytes.Buffer
	err = (Runner{Stdout: &stdout, WorkingDir: fixture.workingDir, StateHome: fixture.stateHome}).runMigrateVNextRehearsalWithOperations(
		[]string{"--backup", fixture.backup.BackupPath, "--to", destination, "--json"},
		&stdout,
		state.NewRuntime(workingDirectory),
		migrateVNextRehearsalOperations{
			newEnvironmentID: func() (continuity.EnvironmentID, error) { return "environment_injected_failure", nil },
			run: func(context.Context, state.VNextRehearsalExportOptions, *continuitysqlite.Store) (state.VNextRehearsalResult, error) {
				return state.VNextRehearsalResult{}, errors.New("do-not-render-this-failure-detail")
			},
		},
	)
	var silent interface {
		ExitCode() int
		Silent() bool
	}
	if !errors.As(err, &silent) || silent.ExitCode() != 1 || !silent.Silent() {
		t.Fatalf("injected failure error = %#v, want silent exit 1", err)
	}
	var result migrateVNextRehearsalResult
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout.String(), decodeErr)
	}
	if result.Succeeded || result.ErrorCode != "rehearsal_failed" || result.Error == "" ||
		result.Source.BackupPath != fixture.backup.BackupPath || result.Source.SHA256 != fixture.backup.SHA256 ||
		result.Destination.Root != destination || result.Destination.Database != filepath.Join(destination, "vnext", "continuity.sqlite") ||
		result.Destination.EnvironmentID != "environment_injected_failure" || !result.Disposable || result.ActivationPerformed || result.CutoverReady {
		t.Fatalf("structured failure evidence = %#v", result)
	}
	if strings.Contains(stdout.String(), "do-not-render-this-failure-detail") || strings.Contains(stdout.String(), "do-not-render-this-body") {
		t.Fatalf("structured failure leaked private detail: %s", stdout.String())
	}
	if info, statErr := os.Stat(destination); statErr != nil || !info.IsDir() {
		t.Fatalf("reserved failure destination was not preserved: info=%v error=%v", info, statErr)
	}
	if got := rehearsalTestFileSHA256(t, fixture.backup.BackupPath); got != backupBefore {
		t.Fatalf("failed rehearsal changed source backup: got %s, want %s", got, backupBefore)
	}
}

func TestRunnerMigrateVNextRehearsalRefusesRelocatedDestinationBeforeRun(t *testing.T) {
	fixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(fixture.workingDir), "relocated-after-open")
	workingDirectory, err := project.ResolveWorkingDirectory(fixture.workingDir)
	if err != nil {
		t.Fatalf("project.ResolveWorkingDirectory() error = %v", err)
	}
	runCalled := false
	var stdout bytes.Buffer
	err = (Runner{WorkingDir: fixture.workingDir, StateHome: fixture.stateHome}).runMigrateVNextRehearsalWithOperations(
		[]string{"--backup", fixture.backup.BackupPath, "--to", destination, "--json"},
		&stdout,
		state.NewRuntime(workingDirectory),
		migrateVNextRehearsalOperations{
			open: func(path string, environmentID continuity.EnvironmentID) (*continuitysqlite.Store, error) {
				store, openErr := continuitysqlite.Open(path, environmentID)
				if openErr != nil {
					return nil, openErr
				}
				if renameErr := os.Rename(path, path+"-moved"); renameErr != nil {
					_ = store.Close()
					return nil, renameErr
				}
				if mkdirErr := os.Mkdir(path, 0o700); mkdirErr != nil {
					_ = store.Close()
					return nil, mkdirErr
				}
				return store, nil
			},
			run: func(context.Context, state.VNextRehearsalExportOptions, *continuitysqlite.Store) (state.VNextRehearsalResult, error) {
				runCalled = true
				return state.VNextRehearsalResult{}, nil
			},
		},
	)
	if runCalled {
		t.Fatal("rehearsal run was called after destination relocation")
	}
	var silent interface {
		ExitCode() int
		Silent() bool
	}
	if !errors.As(err, &silent) || silent.ExitCode() != 1 || !silent.Silent() {
		t.Fatalf("relocation error = %#v, want silent exit 1", err)
	}
	var result migrateVNextRehearsalResult
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout.String(), decodeErr)
	}
	if result.ErrorCode != "destination_validation_failed" || result.Succeeded {
		t.Fatalf("relocation failure evidence = %#v", result)
	}
}

func TestRunnerMigrateVNextRehearsalCloseFailureRetainsVerifiedProjectionEvidence(t *testing.T) {
	fixture := prepareVNextRehearsalCLIFixture(t)
	destination := filepath.Join(filepath.Dir(fixture.workingDir), "injected-close-failure")
	workingDirectory, err := project.ResolveWorkingDirectory(fixture.workingDir)
	if err != nil {
		t.Fatalf("project.ResolveWorkingDirectory() error = %v", err)
	}
	var stdout bytes.Buffer
	err = (Runner{WorkingDir: fixture.workingDir, StateHome: fixture.stateHome}).runMigrateVNextRehearsalWithOperations(
		[]string{"--backup", fixture.backup.BackupPath, "--to", destination, "--json"},
		&stdout,
		state.NewRuntime(workingDirectory),
		migrateVNextRehearsalOperations{
			close: func(store *continuitysqlite.Store) error {
				if closeErr := store.Close(); closeErr != nil {
					return closeErr
				}
				return errors.New("injected close failure")
			},
		},
	)
	var silent interface {
		ExitCode() int
		Silent() bool
	}
	if !errors.As(err, &silent) || silent.ExitCode() != 1 || !silent.Silent() {
		t.Fatalf("close failure error = %#v, want silent exit 1", err)
	}
	var result migrateVNextRehearsalResult
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", stdout.String(), decodeErr)
	}
	if result.ErrorCode != "destination_close_failed" || result.Succeeded || !result.ProjectionVerified ||
		result.Archive.ContentSHA256 == "" || result.Archive.Expected != result.Archive.Actual || result.Archive.RecordCount != 4 ||
		result.Archive.Source.NormalizedJournalCategoryRows != 1 ||
		result.Archive.Source.JournalCategoryMapping != archive.JournalCategoryMappingUnsupportedToNoteV1 {
		t.Fatalf("close failure evidence = %#v", result)
	}
}

func prepareVNextRehearsalCLIFixture(t *testing.T) vnextRehearsalCLIFixture {
	t.Helper()
	base := t.TempDir()
	workingDir := filepath.Join(base, "project")
	stateHome := filepath.Join(base, "state")
	if err := os.Mkdir(workingDir, 0o700); err != nil {
		t.Fatalf("os.Mkdir(project) error = %v", err)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("project.ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	status, err := state.Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("state.Initialize() error = %v", err)
	}
	discover, err := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: "discover(rehearsal): safe operator path", HarnessSessionID: "session-cli"})
	if err != nil {
		t.Fatalf("state.LogJournal(discover) error = %v", err)
	}
	if _, err := state.LogJournal(context.Background(), root, resolver, state.JournalLogOptions{Entry: "wrap(rehearsal): dogfood the isolated result", HarnessSessionID: "session-cli"}); err != nil {
		t.Fatalf("state.LogJournal(wrap) error = %v", err)
	}
	if _, err := state.CreateArtifactEntity(context.Background(), root, resolver, state.ArtifactEntityCreateOptions{
		Kind: "handoff", Title: "CLI rehearsal", Body: "do-not-render-this-body",
	}); err != nil {
		t.Fatalf("state.CreateArtifactEntity(handoff) error = %v", err)
	}
	store, err := state.OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("state.OpenStore(normalization seed) error = %v", err)
	}
	if _, err := store.DB().Exec(`UPDATE facts SET payload = replace(payload, '"entry_type":"discover"', '"entry_type":"legacy_session"') WHERE id = ?`, discover.ID); err != nil {
		t.Fatalf("update journal fact category: %v", err)
	}
	if _, err := store.DB().Exec(`UPDATE journal_entries SET entry_type = 'legacy_session' WHERE project_id = ? AND id = ?`, status.ProjectID, discover.ID); err != nil {
		t.Fatalf("update journal projection category: %v", err)
	}
	if _, err := store.DB().Exec(`UPDATE journal_search SET entry_type = 'legacy_session' WHERE project_id = ? AND journal_entry_id = ?`, status.ProjectID, discover.ID); err != nil {
		t.Fatalf("update journal search category: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("state.Close(normalization seed) error = %v", err)
	}
	backup, err := state.Backup(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("state.Backup() error = %v", err)
	}
	return vnextRehearsalCLIFixture{workingDir: workingDir, stateHome: stateHome, projectRoot: root, database: status.DatabasePath, backup: backup}
}

func rehearsalTestFileSHA256(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func assertRehearsalPrivateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode(%q) = %#o, want %#o", path, got, want)
	}
}
