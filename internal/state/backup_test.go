package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestBackupCreatesSQLiteCopyOutsideRepository(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	result, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if result.DatabasePath != status.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", result.DatabasePath, status.DatabasePath)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty")
	}
	if isWithinRoot(result.BackupPath, root.Path()) {
		t.Fatalf("BackupPath = %q, want outside repository %q", result.BackupPath, root.Path())
	}
	if !strings.HasPrefix(result.BackupPath, filepath.Join(stateHome, "loaf", "backups")+string(filepath.Separator)) {
		t.Fatalf("BackupPath = %q, want under state home %q", result.BackupPath, stateHome)
	}
	if !strings.HasSuffix(result.BackupPath, ".sqlite") {
		t.Fatalf("BackupPath = %q, want sqlite suffix", result.BackupPath)
	}
	info, err := os.Stat(result.BackupPath)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if info.Size() <= 0 {
		t.Fatalf("backup file size = %d, want > 0", info.Size())
	}
	if result.Bytes != info.Size() {
		t.Fatalf("Bytes = %d, want %d", result.Bytes, info.Size())
	}
	if result.SHA256 != testFileSHA256(t, result.BackupPath) {
		t.Fatalf("SHA256 = %q, want actual backup digest", result.SHA256)
	}
	if result.CreatedAt == "" {
		t.Fatal("CreatedAt is empty")
	}
	if !result.Verified {
		t.Fatal("Verified = false, want true")
	}
	if !result.SQLiteValid || !result.RecoveryReady {
		t.Fatalf("SQLiteValid/RecoveryReady = %t/%t, want true/true", result.SQLiteValid, result.RecoveryReady)
	}
	if result.RecoveryTier != RecoveryTierLocalRollback || result.DeviceLossProtectionBasis != ProtectionBasisNone || result.DeviceLossProtected {
		t.Fatalf("recovery metadata = tier=%q basis=%q protected=%t, want local/none/false", result.RecoveryTier, result.DeviceLossProtectionBasis, result.DeviceLossProtected)
	}
	if result.JournalWatermark.Present {
		t.Fatalf("JournalWatermark = %#v, want empty for no journal entries", result.JournalWatermark)
	}
	if result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", result.SchemaVersion, CurrentSchemaVersion())
	}
	if result.ProjectCount != 1 {
		t.Fatalf("ProjectCount = %d, want 1", result.ProjectCount)
	}
	if result.ProjectID != status.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", result.ProjectID, status.ProjectID)
	}
	if result.ProjectName != status.ProjectName {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, status.ProjectName)
	}
	if result.ProjectCurrentPath != status.ProjectCurrentPath {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, status.ProjectCurrentPath)
	}
	if result.IntegrityCheck != "ok" {
		t.Fatalf("IntegrityCheck = %q, want ok", result.IntegrityCheck)
	}
	if result.ForeignKeyCheck != "ok" {
		t.Fatalf("ForeignKeyCheck = %q, want ok", result.ForeignKeyCheck)
	}
	if !result.JournalRetrievalReady || !result.JournalSearchParity.Ready {
		t.Fatalf("journal retrieval = %t parity = %#v, want ready", result.JournalRetrievalReady, result.JournalSearchParity)
	}
	if result.JournalSearchParity.CanonicalRows != 0 || result.JournalSearchParity.IndexRows != 0 {
		t.Fatalf("journal parity = %#v, want empty clean parity", result.JournalSearchParity)
	}
	assertNoSQLiteSidecars(t, result.BackupPath)

	backupStore, err := OpenStoreReadOnly(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(backup) error = %v", err)
	}
	defer backupStore.Close()
	version, err := backupStore.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("backup SchemaVersion() error = %v", err)
	}
	if version != CurrentSchemaVersion() {
		t.Fatalf("backup schema version = %d, want %d", version, CurrentSchemaVersion())
	}
	assertNoSQLiteSidecars(t, result.BackupPath)
}

func TestBackupReportsRecoveryMetadataAndJournalWatermark(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	entry, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(backup-watermark): included"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	result, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if result.RecoveryTier != RecoveryTierLocalRollback {
		t.Fatalf("RecoveryTier = %q, want %q", result.RecoveryTier, RecoveryTierLocalRollback)
	}
	if result.RequestedDestinationDirectory != "" {
		t.Fatalf("RequestedDestinationDirectory = %q, want empty local request", result.RequestedDestinationDirectory)
	}
	wantResolved, err := filepath.EvalSymlinks(filepath.Join(stateHome, "loaf", "backups"))
	if err != nil {
		t.Fatalf("EvalSymlinks(local backup directory) error = %v", err)
	}
	if result.ResolvedDestinationDirectory != wantResolved {
		t.Fatalf("ResolvedDestinationDirectory = %q, want %q", result.ResolvedDestinationDirectory, wantResolved)
	}
	if result.DeviceLossProtected || result.DeviceLossProtectionBasis != ProtectionBasisNone {
		t.Fatalf("device-loss metadata = %t/%q, want false/none", result.DeviceLossProtected, ProtectionBasisNone)
	}
	if !result.SQLiteValid || !result.Verified || !result.JournalRetrievalReady || !result.RecoveryReady {
		t.Fatalf("validity metadata = sqlite=%t verified=%t retrieval=%t recovery=%t, want all true", result.SQLiteValid, result.Verified, result.JournalRetrievalReady, result.RecoveryReady)
	}
	if !result.JournalWatermark.Present || result.JournalWatermark.JournalEntryID != entry.ID || result.JournalWatermark.CreatedAt == "" {
		t.Fatalf("JournalWatermark = %#v, want entry %q with timestamp", result.JournalWatermark, entry.ID)
	}
	verified, err := VerifyBackup(ctx, result.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	if !verified.SQLiteValid || !verified.RecoveryReady || verified.JournalWatermark != result.JournalWatermark {
		t.Fatalf("verification metadata = %#v, want snapshot metadata %#v", verified, result.JournalWatermark)
	}
}

func TestBackupWithDestinationValidatesExplicitDirectory(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "relative", path: "relative-backup", code: BackupDestinationNotAbsoluteCode},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BackupWithDestination(ctx, root, resolver, tc.path)
			var destinationErr *BackupDestinationError
			if !errors.As(err, &destinationErr) {
				t.Fatalf("BackupWithDestination() error = %v, want BackupDestinationError", err)
			}
			if destinationErr.Code != tc.code || destinationErr.Requested != tc.path {
				t.Fatalf("destination error = %#v, want code=%q requested=%q", destinationErr, tc.code, tc.path)
			}
		})
	}
	volatileRoot := t.TempDir()
	volatilePath := filepath.Join(volatileRoot, "backup-test-volatile")
	_, err := backupWithDestination(ctx, root, resolver, volatilePath, &backupOperations{volatileRoots: []string{volatileRoot}})
	var volatileErr *BackupDestinationError
	if !errors.As(err, &volatileErr) || volatileErr.Code != BackupDestinationVolatileCode {
		t.Fatalf("volatile error = %v, want code %q", err, BackupDestinationVolatileCode)
	}
	nonDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(nonDirectory, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err = BackupWithDestination(ctx, root, resolver, nonDirectory)
	var destinationErr *BackupDestinationError
	if !errors.As(err, &destinationErr) || destinationErr.Code != BackupDestinationNotDirectoryCode {
		t.Fatalf("non-directory error = %v, want code %q", err, BackupDestinationNotDirectoryCode)
	}
}

func TestVolatileRootInventoryUsesCurrentPlatformAndDurableControl(t *testing.T) {
	probe := filepath.Join(os.TempDir(), "backup-test-classification-probe")
	if !pathWithinVolatileRoot(probe) {
		t.Fatalf("default volatile roots do not classify %q", probe)
	}
	switch runtime.GOOS {
	case "linux":
		for _, root := range []string{"/dev/shm", "/run/user", "/run/lock", "/run/shm"} {
			if !pathWithinVolatileRoots(filepath.Join(root, "backup-test-probe"), []string{root}) {
				t.Fatalf("Linux volatile root %q was not classified", root)
			}
		}
		if !pathWithinVolatileRoots("/var/run/backup-test-probe", nil) {
			t.Fatal("Linux legacy /var/run runtime namespace was not classified")
		}
		if pathWithinVolatileRoots("/run/media/backup-test-probe", []string{"/run/user", "/run/lock", "/run/shm"}) {
			t.Fatal("Linux durable /run/media control was over-classified as volatile")
		}
	case "darwin":
		for _, root := range []string{"/var/folders", "/private/var/folders"} {
			if _, err := os.Stat(root); err == nil && !pathWithinVolatileRoot(filepath.Join(root, "backup-test-probe")) {
				t.Fatalf("Darwin volatile root %q was not classified", root)
			}
		}
	}
	volatile := t.TempDir()
	durable := t.TempDir()
	if pathWithinVolatileRoots(filepath.Join(durable, "candidate"), []string{volatile}) {
		t.Fatalf("durable control path classified as volatile")
	}
	if !pathWithinVolatileRoots(filepath.Join(volatile, "candidate"), []string{volatile}) {
		t.Fatalf("injected volatile root was not classified")
	}
}

func TestBackupWithDestinationResolvesMissingTailAndClassifiesExternalCopy(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	parent := t.TempDir()
	policyRoot := t.TempDir()
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	destination := filepath.Join(parent, "missing", "tail")
	result, err := backupWithDestination(ctx, root, resolver, destination, &backupOperations{volatileRoots: []string{policyRoot}})
	if err != nil {
		t.Fatalf("BackupWithDestination() error = %v", err)
	}
	if result.RecoveryTier != RecoveryTierExternalDisasterCopy || result.DeviceLossProtected || result.DeviceLossProtectionBasis != ProtectionBasisOperatorSelected {
		t.Fatalf("external metadata = tier=%q protected=%t basis=%q", result.RecoveryTier, result.DeviceLossProtected, result.DeviceLossProtectionBasis)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatalf("EvalSymlinks(parent) error = %v", err)
	}
	wantResolved := filepath.Join(resolvedParent, "missing", "tail")
	if result.RequestedDestinationDirectory != destination || result.ResolvedDestinationDirectory != wantResolved {
		t.Fatalf("destination metadata = requested=%q resolved=%q, want %q", result.RequestedDestinationDirectory, result.ResolvedDestinationDirectory, wantResolved)
	}
	if !strings.HasPrefix(result.BackupPath, wantResolved+string(filepath.Separator)) {
		t.Fatalf("BackupPath = %q, want under %q", result.BackupPath, wantResolved)
	}
}

func TestBackupWithDestinationRejectsVolatileSymlinkBeforeCreatingTail(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	parent := t.TempDir()
	volatileTarget := t.TempDir()
	link := filepath.Join(parent, "volatile-link")
	if err := os.Symlink(volatileTarget, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	tail := filepath.Join(link, "backup-test-must-not-create")
	_, err := backupWithDestination(ctx, root, resolver, tail, &backupOperations{volatileRoots: []string{volatileTarget}})
	var destinationErr *BackupDestinationError
	if !errors.As(err, &destinationErr) || destinationErr.Code != BackupDestinationVolatileCode {
		t.Fatalf("volatile symlink error = %v, want code %q", err, BackupDestinationVolatileCode)
	}
	if _, err := os.Stat(filepath.Join(volatileTarget, "backup-test-must-not-create")); !os.IsNotExist(err) {
		t.Fatalf("volatile tail stat error = %v, want no directory created", err)
	}
}

func TestBackupWithDestinationRejectsProjectContainedMissingTailBeforeMkdir(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	destination := filepath.Join(root.Path(), "new-backup", "missing-tail")
	_, err := backupWithDestination(ctx, root, resolver, destination, &backupOperations{volatileRoots: []string{t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), "backup directory must be outside project root") {
		t.Fatalf("project-contained destination error = %v, want outside-project rejection", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("rejected destination stat error = %v, want path absent after rejection", statErr)
	}
}

func TestBackupOperationSeamsCleanReservationAndRetainCompletedArtifact(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	reservationResult, err := backupWithDestination(ctx, root, resolver, "", &backupOperations{
		afterReserve: func(string) error { return errors.New("fail before vacuum") },
	})
	if err == nil || !strings.Contains(err.Error(), "fail before vacuum") {
		t.Fatalf("reservation seam error = %v, want injected failure", err)
	}
	if reservationResult.BackupPath == "" {
		t.Fatal("reservation seam result omitted reserved path")
	}
	if _, statErr := os.Stat(reservationResult.BackupPath); !os.IsNotExist(statErr) {
		t.Fatalf("reserved path stat error = %v, want reservation removed", statErr)
	}

	verificationResult, err := backupWithDestination(ctx, root, resolver, "", &backupOperations{
		verify: func(context.Context, string, project.Root) (backupVerification, error) {
			return backupVerification{}, errors.New("fail after vacuum")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "fail after vacuum") {
		t.Fatalf("verification seam error = %v, want injected failure", err)
	}
	if verificationResult.BackupPath == "" {
		t.Fatal("verification seam result omitted completed path")
	}
	info, statErr := os.Stat(verificationResult.BackupPath)
	if statErr != nil {
		t.Fatalf("completed artifact stat error = %v, want retained path", statErr)
	}
	if info.Size() == 0 {
		t.Fatal("completed artifact is empty, want retained SQLite snapshot")
	}
}

func TestBackupWithLiveWALWriterCapturesContiguousPrefixAndWatermark(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	writer, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(writer) error = %v", err)
	}
	defer writer.Close()
	baseline, err := writer.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(backup-prefix): baseline"})
	if err != nil {
		t.Fatalf("baseline LogJournal() error = %v", err)
	}
	mainBefore := testFileBytes(t, status.DatabasePath)
	walBefore := testOptionalFileBytes(t, status.DatabasePath+"-wal")
	var during, after JournalLogResult
	result, err := backupWithDestination(ctx, root, resolver, "", &backupOperations{
		beforeVacuum: func(string) error {
			var err error
			during, err = writer.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(backup-prefix): during"})
			return err
		},
		afterVacuum: func(string) error {
			var err error
			after, err = writer.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(backup-prefix): after"})
			return err
		},
	})
	if err != nil {
		t.Fatalf("backup with live writer error = %v", err)
	}
	mainAfter := testFileBytes(t, status.DatabasePath)
	walAfter := testOptionalFileBytes(t, status.DatabasePath+"-wal")
	if len(mainBefore) == 0 || len(mainAfter) == 0 || len(walBefore) == 0 || len(walAfter) == 0 {
		t.Fatalf("source durable files missing: main=%d/%d wal=%d/%d", len(mainBefore), len(mainAfter), len(walBefore), len(walAfter))
	}
	if bytes.Equal(mainBefore, mainAfter) && bytes.Equal(walBefore, walAfter) {
		t.Fatalf("live WAL writer produced no durable source change")
	}
	if during.ID == "" || after.ID == "" || baseline.ID == "" {
		t.Fatalf("writer IDs = baseline=%q during=%q after=%q, want all present", baseline.ID, during.ID, after.ID)
	}
	snapshot, err := OpenStoreReadOnly(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(snapshot) error = %v", err)
	}
	defer snapshot.Close()
	var canonical, derived int
	if err := snapshot.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&canonical); err != nil {
		t.Fatalf("snapshot canonical count error = %v", err)
	}
	if err := snapshot.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search`).Scan(&derived); err != nil {
		t.Fatalf("snapshot derived count error = %v", err)
	}
	if canonical != 2 || derived != 2 {
		t.Fatalf("snapshot rows canonical=%d derived=%d, want contiguous baseline/during prefix", canonical, derived)
	}
	for _, id := range []string{baseline.ID, during.ID} {
		var count int
		if err := snapshot.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = ?`, id).Scan(&count); err != nil {
			t.Fatalf("snapshot journal %q count error = %v", id, err)
		}
		if count != 1 {
			t.Fatalf("snapshot journal %q count = %d, want 1", id, count)
		}
	}
	var afterCount int
	if err := snapshot.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = ?`, after.ID).Scan(&afterCount); err != nil {
		t.Fatalf("snapshot post-snapshot row count error = %v", err)
	}
	if afterCount != 0 {
		t.Fatalf("snapshot contains post-snapshot row %q", after.ID)
	}
	parity, err := InspectJournalSearchParity(ctx, snapshot)
	if err != nil {
		t.Fatalf("snapshot parity error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 2 || parity.IndexRows != 2 {
		t.Fatalf("snapshot parity = %#v, want exact two-row parity", parity)
	}
	if !result.JournalWatermark.Present || result.JournalWatermark.JournalEntryID != during.ID {
		t.Fatalf("reported watermark = %#v, want during row %q", result.JournalWatermark, during.ID)
	}
}

func TestBackupPreservesLiveWALSourceBytesAndLogicalState(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	writer, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(writer) error = %v", err)
	}
	defer writer.Close()
	if _, err := writer.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(backup-wal): durable"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	// durableSQLiteFilesSnapshot intentionally captures only the main database
	// and WAL; SHM byte identity is excluded because SHM is ephemeral SQLite
	// coordination, not durable source state.
	durableBefore := durableSQLiteFilesSnapshot(t, status.DatabasePath)
	if len(durableBefore[status.DatabasePath+"-wal"]) == 0 {
		t.Fatal("source WAL is empty before backup; expected live WAL content")
	}
	var canonicalBefore, indexBefore int
	if err := writer.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&canonicalBefore); err != nil {
		t.Fatalf("source canonical count before backup error = %v", err)
	}
	if err := writer.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search`).Scan(&indexBefore); err != nil {
		t.Fatalf("source index count before backup error = %v", err)
	}
	parityBefore, err := InspectJournalSearchParity(ctx, writer)
	if err != nil {
		t.Fatalf("source parity before backup error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	durableAfter := durableSQLiteFilesSnapshot(t, status.DatabasePath)
	if !reflect.DeepEqual(durableBefore, durableAfter) {
		t.Fatal("Backup() mutated durable main database or WAL bytes")
	}
	var canonicalAfter, indexAfter int
	if err := writer.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&canonicalAfter); err != nil {
		t.Fatalf("source canonical count after backup error = %v", err)
	}
	if err := writer.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search`).Scan(&indexAfter); err != nil {
		t.Fatalf("source index count after backup error = %v", err)
	}
	parityAfter, err := InspectJournalSearchParity(ctx, writer)
	if err != nil {
		t.Fatalf("source parity after backup error = %v", err)
	}
	if canonicalAfter != canonicalBefore || indexAfter != indexBefore || parityAfter != parityBefore {
		t.Fatalf("source logical state changed: counts %d/%d -> %d/%d, parity %#v -> %#v", canonicalBefore, indexBefore, canonicalAfter, indexAfter, parityBefore, parityAfter)
	}
	if !backup.SQLiteValid || !backup.RecoveryReady {
		t.Fatalf("backup metadata = sqlite=%t recovery=%t, want true/true", backup.SQLiteValid, backup.RecoveryReady)
	}
}

func TestConcurrentBackupAllocationsAreDistinct(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	const count = 3
	results := make(chan BackupResult, count)
	errs := make(chan error, count)
	var wait sync.WaitGroup
	for range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := Backup(ctx, root, resolver)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Backup() error = %v", err)
	}
	paths := map[string]bool{}
	for result := range results {
		if !result.SQLiteValid || !result.RecoveryReady {
			t.Fatalf("concurrent result = %#v, want valid/recovery-ready", result)
		}
		if paths[result.BackupPath] {
			t.Fatalf("duplicate concurrent backup path %q", result.BackupPath)
		}
		paths[result.BackupPath] = true
	}
	if len(paths) != count {
		t.Fatalf("concurrent backup count = %d, want %d", len(paths), count)
	}
}

func TestBackupReportsGlobalProjectCount(t *testing.T) {
	firstRoot := projectRoot(t)
	secondRoot := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), firstRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(firstRoot) error = %v", err)
	}
	if _, err := Initialize(context.Background(), secondRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(secondRoot) error = %v", err)
	}

	result, err := Backup(context.Background(), firstRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectCount != 2 {
		t.Fatalf("ProjectCount = %d, want 2", result.ProjectCount)
	}
}

func TestVerifyBackupReportsAllProjectsWithoutLiveState(t *testing.T) {
	firstRoot := projectRoot(t)
	secondRoot := projectRoot(t)
	stateHome := t.TempDir()
	firstStatus, err := Initialize(context.Background(), firstRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(firstRoot) error = %v", err)
	}
	secondStatus, err := Initialize(context.Background(), secondRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(secondRoot) error = %v", err)
	}
	backup, err := Backup(context.Background(), firstRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if err := os.Remove(firstStatus.DatabasePath); err != nil {
		t.Fatalf("remove live database error = %v", err)
	}

	result, err := VerifyBackup(context.Background(), backup.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}

	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.BackupPath != backup.BackupPath {
		t.Fatalf("BackupPath = %q, want %q", result.BackupPath, backup.BackupPath)
	}
	if result.Bytes != backup.Bytes {
		t.Fatalf("Bytes = %d, want %d", result.Bytes, backup.Bytes)
	}
	if result.SHA256 != backup.SHA256 {
		t.Fatalf("SHA256 = %q, want %q", result.SHA256, backup.SHA256)
	}
	if !result.Verified {
		t.Fatal("Verified = false, want true")
	}
	if !result.SQLiteValid || !result.RecoveryReady {
		t.Fatalf("verification recovery metadata = sqlite=%t recovery=%t, want true/true", result.SQLiteValid, result.RecoveryReady)
	}
	if result.JournalWatermark.Present {
		t.Fatalf("JournalWatermark = %#v, want empty for no journal entries", result.JournalWatermark)
	}
	if result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", result.SchemaVersion, CurrentSchemaVersion())
	}
	if result.ProjectCount != 2 || len(result.Projects) != 2 {
		t.Fatalf("projects = %d/%d, want two projects", result.ProjectCount, len(result.Projects))
	}
	seen := map[string]bool{}
	for _, project := range result.Projects {
		seen[project.ID] = true
		if project.DatabasePath != backup.BackupPath {
			t.Fatalf("project DatabasePath = %q, want backup path %q", project.DatabasePath, backup.BackupPath)
		}
	}
	if !seen[firstStatus.ProjectID] || !seen[secondStatus.ProjectID] {
		t.Fatalf("verified projects = %#v, want %q and %q", seen, firstStatus.ProjectID, secondStatus.ProjectID)
	}
	if result.IntegrityCheck != "ok" {
		t.Fatalf("IntegrityCheck = %q, want ok", result.IntegrityCheck)
	}
	if result.ForeignKeyCheck != "ok" {
		t.Fatalf("ForeignKeyCheck = %q, want ok", result.ForeignKeyCheck)
	}
	if !result.JournalRetrievalReady || !result.JournalSearchParity.Ready {
		t.Fatalf("journal retrieval = %t parity = %#v, want ready", result.JournalRetrievalReady, result.JournalSearchParity)
	}
}

func TestVerifyBackupReportsDivergentJournalSearchWithoutStructuralFailure(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(backup): parity"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search`); err != nil {
		store.Close()
		t.Fatalf("delete journal search rows error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if !backup.Verified || !backup.SQLiteValid || backup.JournalRetrievalReady || backup.RecoveryReady || backup.JournalSearchParity.Ready {
		t.Fatalf("backup = %#v, want verified but retrieval-not-ready", backup)
	}
	if backup.JournalSearchParity.CanonicalRows != 1 || backup.JournalSearchParity.IndexRows != 0 || backup.JournalSearchParity.Missing != 1 {
		t.Fatalf("backup parity = %#v, want canonical=1/index=0/missing=1", backup.JournalSearchParity)
	}
	verified, err := VerifyBackup(ctx, backup.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	if !verified.Verified || !verified.SQLiteValid || verified.JournalRetrievalReady || verified.RecoveryReady || verified.JournalSearchParity != backup.JournalSearchParity {
		t.Fatalf("verified backup = %#v, want structural true/retrieval false and matching parity", verified)
	}
}

func TestBackupCreatesTimestampedFilesWithoutOverwriting(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first Backup() error = %v", err)
	}
	second, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second Backup() error = %v", err)
	}

	if first.BackupPath == second.BackupPath {
		t.Fatalf("backup path reused: %q", first.BackupPath)
	}
	if _, err := os.Stat(first.BackupPath); err != nil {
		t.Fatalf("first backup missing after second backup: %v", err)
	}
	if _, err := os.Stat(second.BackupPath); err != nil {
		t.Fatalf("second backup missing: %v", err)
	}
}

func TestNextBackupPathIncludesNanoseconds(t *testing.T) {
	path, err := nextBackupPath(t.TempDir(), time.Date(2026, 5, 28, 21, 15, 41, 193211000, time.UTC))
	if err != nil {
		t.Fatalf("nextBackupPath() error = %v", err)
	}
	if !strings.HasSuffix(path, "loaf-20260528-211541-193211000.sqlite") {
		t.Fatalf("path = %q, want nanosecond timestamp", path)
	}
}

func TestReserveBackupPathSkipsExistingEmptyCandidate(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 28, 21, 15, 41, 193211000, time.UTC)
	first := filepath.Join(dir, "loaf-20260528-211541-193211000.sqlite")
	if file, err := os.OpenFile(first, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); err != nil {
		t.Fatalf("reserve existing candidate error = %v", err)
	} else if err := file.Close(); err != nil {
		t.Fatalf("close existing candidate error = %v", err)
	}
	reserved, err := reserveBackupPath(dir, now)
	if err != nil {
		t.Fatalf("reserveBackupPath() error = %v", err)
	}
	want := filepath.Join(dir, "loaf-20260528-211541-193211000-001.sqlite")
	if reserved != want {
		t.Fatalf("reserved = %q, want %q", reserved, want)
	}
	if info, err := os.Stat(first); err != nil {
		t.Fatalf("existing candidate missing: %v", err)
	} else if info.Size() != 0 {
		t.Fatalf("existing candidate size = %d, want empty reservation", info.Size())
	}
}

func TestBackupRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := Backup(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err == nil {
		t.Fatal("Backup() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestBackupRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err == nil {
		t.Fatal("Backup() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
	backupDir := filepath.Join(filepath.Dir(path), "backups")
	entries, readErr := os.ReadDir(backupDir)
	if readErr == nil && len(entries) != 0 {
		t.Fatalf("backup directory entries = %d, want incomplete reservation removed", len(entries))
	} else if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("ReadDir(%s) error = %v", backupDir, readErr)
	}
}

func TestVerifyNoForeignKeyViolationsReportsDetails(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-orphaned-project', 'project-missing', 'task', 'task-missing', 'task', 'TASK-MISSING', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`); err != nil {
		t.Fatalf("insert orphaned alias fixture error = %v", err)
	}

	_, err := verifyNoForeignKeyViolations(context.Background(), store)
	if err == nil {
		t.Fatal("verifyNoForeignKeyViolations() error = nil, want detailed violation")
	}
	for _, want := range []string{"SQLite foreign key violation", "aliases", "projects", "constraint"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func assertNoSQLiteSidecars(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("backup sidecar %s exists or stat failed: %v", sidecar, err)
		}
	}
}

func testFileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func testFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

func testOptionalFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}
