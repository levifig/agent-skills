package state

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestRehearseBackupRestoreCopiesAndVerifiesDisposableSnapshot(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	restorePath := filepath.Join(t.TempDir(), "restored.sqlite")
	sourceBytes := readRecoveryFile(t, backup.BackupPath)
	result, err := RehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath)
	if err != nil {
		t.Fatalf("RehearseBackupRestore() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion || result.DatabaseScope != "global" {
		t.Fatalf("contract metadata = %#v", result)
	}
	if result.BackupPath != backup.BackupPath || result.RestorePath != restorePath {
		t.Fatalf("paths = backup=%q restore=%q", result.BackupPath, result.RestorePath)
	}
	if result.SourceSHA256 != result.RestoredSHA256 || !result.ExactCopy || !result.DisposableRehearsal || result.LiveDatabaseMutated {
		t.Fatalf("copy metadata = %#v", result)
	}
	if !result.SQLiteValid || !result.JournalRetrievalReady || !result.RecoveryReady {
		t.Fatalf("recovery metadata = sqlite=%t retrieval=%t ready=%t", result.SQLiteValid, result.JournalRetrievalReady, result.RecoveryReady)
	}
	if result.WatermarkPresent != result.JournalWatermark.Present || !result.JournalWatermark.Present {
		t.Fatalf("watermark metadata = %#v/%t", result.JournalWatermark, result.WatermarkPresent)
	}
	info, err := os.Stat(restorePath)
	if err != nil {
		t.Fatalf("restored file stat error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode = %o, want 600", info.Mode().Perm())
	}
	if !bytes.Equal(sourceBytes, readRecoveryFile(t, restorePath)) {
		t.Fatal("restored bytes differ from source")
	}
	assertRecoverySidecarsAbsent(t, restorePath)
	if _, err := os.Stat(backup.BackupPath); err != nil {
		t.Fatalf("source backup disappeared: %v", err)
	}
	if err := os.Remove(restorePath); err != nil {
		t.Fatalf("remove rehearsal output error = %v", err)
	}
	if !bytes.Equal(sourceBytes, readRecoveryFile(t, backup.BackupPath)) {
		t.Fatal("source backup changed after rehearsal output deletion")
	}
}

func TestCompareRestoreRowsDetectsJournalProvenanceDifferences(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, resolver.StateHome)
	dirty := false
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(recovery): origin", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismHook, SourceEvent: "recovery"}}); err != nil {
		store.Close()
		t.Fatalf("LogJournal() error = %v", err)
	}
	if _, err := store.DeferJournal(ctx, root, JournalDeferOptions{Intent: "recover intent", Why: "recover why", Boundary: "recover boundary", Trigger: "recover trigger", OperationID: "recover-op", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismHook, ChangePath: "docs/recovery.md", ChangeSHA256: strings.Repeat("a", 64), Dirty: &dirty, Reconstructable: &dirty}}); err != nil {
		store.Close()
		t.Fatalf("DeferJournal() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	sourcePath := backup.BackupPath
	for _, tc := range []struct {
		name   string
		mutate func(*Store) error
		want   string
	}{
		{name: "origin", mutate: func(store *Store) error {
			_, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'corrupted'`)
			return err
		}, want: "journal_origins rows differ"},
		{name: "deferral", mutate: func(store *Store) error {
			_, err := store.db.ExecContext(ctx, `UPDATE journal_deferrals SET stored_digest = replace(stored_digest, 'a', 'b')`)
			return err
		}, want: "journal_deferrals rows differ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoredPath := filepath.Join(t.TempDir(), "restored.sqlite")
			if err := os.WriteFile(restoredPath, readRecoveryFile(t, sourcePath), 0o600); err != nil {
				t.Fatalf("copy backup: %v", err)
			}
			restoredStore, err := OpenStore(restoredPath)
			if err != nil {
				t.Fatalf("OpenStore(restored) error = %v", err)
			}
			if err := tc.mutate(restoredStore); err != nil {
				restoredStore.Close()
				t.Fatalf("mutate restored: %v", err)
			}
			if err := restoredStore.Close(); err != nil {
				t.Fatalf("Close(restored) error = %v", err)
			}
			if err := compareRestoreRows(ctx, sourcePath, restoredPath); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("compareRestoreRows() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRehearseBackupRestoreRejectsUnsafeTargetsBeforeMutation(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	parent := t.TempDir()
	livePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	aliasParent := filepath.Join(parent, "backup-alias")
	if err := os.Symlink(filepath.Dir(backup.BackupPath), aliasParent); err != nil {
		t.Fatalf("Symlink(backup alias) error = %v", err)
	}
	dangling := filepath.Join(parent, "dangling")
	if err := os.Symlink(filepath.Join(parent, "missing-target"), dangling); err != nil {
		t.Fatalf("Symlink(dangling) error = %v", err)
	}
	existingFile := filepath.Join(parent, "existing.sqlite")
	if err := os.WriteFile(existingFile, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}
	existingDir := filepath.Join(parent, "existing-dir")
	if err := os.Mkdir(existingDir, 0o700); err != nil {
		t.Fatalf("Mkdir(existing) error = %v", err)
	}
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "relative", path: "relative.sqlite", code: RestoreDestinationNotAbsoluteCode},
		{name: "existing-file", path: existingFile, code: RestoreDestinationExistsCode},
		{name: "existing-directory", path: existingDir, code: RestoreDestinationExistsCode},
		{name: "dangling-symlink", path: dangling, code: RestoreDestinationExistsCode},
		{name: "aliases-backup", path: filepath.Join(aliasParent, filepath.Base(backup.BackupPath)), code: RestoreDestinationAliasesBackupCode},
		{name: "live-database", path: livePath, code: RestoreDestinationIsLiveDatabaseCode},
		{name: "missing-parent", path: filepath.Join(parent, "missing", "restored.sqlite"), code: RestoreDestinationInvalidParentCode},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RehearseBackupRestore(ctx, root, resolver, backup.BackupPath, tc.path)
			assertRecoveryCode(t, err, tc.code)
		})
	}
}

func TestRehearseBackupRestoreRejectsUnreadySourcesWithoutCreatingTarget(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	invalid := filepath.Join(t.TempDir(), "invalid.sqlite")
	if err := os.WriteFile(invalid, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile(invalid) error = %v", err)
	}
	invalidTarget := filepath.Join(t.TempDir(), "invalid-restore.sqlite")
	partial, err := RehearseBackupRestore(ctx, root, resolver, invalid, invalidTarget)
	assertRecoveryCode(t, err, BackupNotRecoveryReadyCode)
	if partial.ContractVersion != StateJSONContractVersion || partial.DatabaseScope != "global" || partial.BackupPath != invalid || partial.RestorePath != invalidTarget || partial.SourceSHA256 == "" || partial.SQLiteValid || partial.JournalRetrievalReady || partial.ExactCopy || partial.RecoveryReady {
		t.Fatalf("unready source partial = %#v, want paths/source checksum and no readiness claims", partial)
	}
	assertRecoveryPathAbsent(t, invalidTarget)

	divergentStore, err := OpenStore(backup.BackupPath)
	if err != nil {
		t.Fatalf("OpenStore(divergent backup) error = %v", err)
	}
	if _, err := divergentStore.db.ExecContext(ctx, `DELETE FROM journal_search`); err != nil {
		divergentStore.Close()
		t.Fatalf("delete divergent index error = %v", err)
	}
	if err := divergentStore.Close(); err != nil {
		t.Fatalf("Close(divergent backup) error = %v", err)
	}
	divergentTarget := filepath.Join(t.TempDir(), "divergent-restore.sqlite")
	partial, err = RehearseBackupRestore(ctx, root, resolver, backup.BackupPath, divergentTarget)
	assertRecoveryCode(t, err, BackupNotRecoveryReadyCode)
	if partial.SourceSHA256 == "" || !partial.SQLiteValid || partial.JournalRetrievalReady || partial.ExactCopy || partial.RecoveryReady {
		t.Fatalf("divergent source partial = %#v, want SQLite evidence with retrieval/readiness false", partial)
	}
	assertRecoveryPathAbsent(t, divergentTarget)
}

func TestRehearseBackupRestoreLeavesLiveWALStateUntouched(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	livePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	liveStore, err := OpenStore(livePath)
	if err != nil {
		t.Fatalf("OpenStore(live) error = %v", err)
	}
	if _, err := liveStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(recovery): live-only"}); err != nil {
		liveStore.Close()
		t.Fatalf("live LogJournal() error = %v", err)
	}
	durableBefore := durableSQLiteFilesSnapshot(t, livePath)
	parityBefore, err := InspectJournalSearchParity(ctx, liveStore)
	if err != nil {
		liveStore.Close()
		t.Fatalf("live parity before error = %v", err)
	}
	var liveRowsBefore int
	if err := liveStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&liveRowsBefore); err != nil {
		liveStore.Close()
		t.Fatalf("live rows before error = %v", err)
	}
	restorePath := filepath.Join(t.TempDir(), "restored.sqlite")
	result, err := RehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath)
	if err != nil {
		liveStore.Close()
		t.Fatalf("RehearseBackupRestore() error = %v", err)
	}
	durableAfter := durableSQLiteFilesSnapshot(t, livePath)
	parityAfter, err := InspectJournalSearchParity(ctx, liveStore)
	if err != nil {
		liveStore.Close()
		t.Fatalf("live parity after error = %v", err)
	}
	var liveRowsAfter int
	if err := liveStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&liveRowsAfter); err != nil {
		liveStore.Close()
		t.Fatalf("live rows after error = %v", err)
	}
	if err := liveStore.Close(); err != nil {
		t.Fatalf("Close(live) error = %v", err)
	}
	if !reflect.DeepEqual(durableBefore, durableAfter) || liveRowsBefore != liveRowsAfter || parityBefore != parityAfter {
		t.Fatalf("live state changed across rehearsal: durable=%t rows=%d/%d parity=%#v/%#v", reflect.DeepEqual(durableBefore, durableAfter), liveRowsBefore, liveRowsAfter, parityBefore, parityAfter)
	}
	if result.LiveDatabaseMutated {
		t.Fatal("result claims live database mutated")
	}
}

func TestRehearseBackupRestoreCleansFailuresAndPreservesSource(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	sourceBefore := readRecoveryFile(t, backup.BackupPath)
	tests := []struct {
		name string
		ops  *recoveryOperations
		code string
	}{
		{name: "mid-copy", ops: &recoveryOperations{copy: func(_ context.Context, _ string, target *os.File) error {
			if _, err := target.Write([]byte("partial")); err != nil {
				return err
			}
			return errors.New("injected copy failure")
		}}, code: RestoreVerificationFailedCode},
		{name: "copy-mismatch", ops: &recoveryOperations{copy: func(_ context.Context, _ string, target *os.File) error {
			_, err := target.Write([]byte("different"))
			return err
		}}, code: RestoreCopyMismatchCode},
		{name: "verification", ops: &recoveryOperations{verify: func(context.Context, string) (BackupVerificationResult, error) {
			return BackupVerificationResult{}, errors.New("injected verification failure")
		}}, code: RestoreVerificationFailedCode},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
			partial, err := rehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath, tc.ops)
			assertRecoveryCode(t, err, tc.code)
			if partial.ContractVersion != StateJSONContractVersion || partial.BackupPath != backup.BackupPath || partial.RestorePath != restorePath || partial.SourceSHA256 == "" {
				t.Fatalf("partial result = %#v, want contract/paths/source evidence", partial)
			}
			if tc.name == "verification" && (partial.RestoredSHA256 == "" || !partial.ExactCopy || partial.RecoveryReady) {
				t.Fatalf("verification partial = %#v, want copied checksum/exact evidence without recovery readiness", partial)
			}
			assertRecoveryPathAbsent(t, restorePath)
			if !bytes.Equal(sourceBefore, readRecoveryFile(t, backup.BackupPath)) {
				t.Fatal("source backup changed after injected failure")
			}
		})
	}
}

func TestRehearseBackupRestoreTargetRacePreservesWinner(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	restorePath := filepath.Join(t.TempDir(), "race.sqlite")
	winner := []byte("winner")
	_, err := rehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath, &recoveryOperations{
		beforeCreate: func(path string) error { return os.WriteFile(path, winner, 0o600) },
	})
	assertRecoveryCode(t, err, RestoreDestinationExistsCode)
	if got := readRecoveryFile(t, restorePath); !bytes.Equal(got, winner) {
		t.Fatalf("race winner bytes = %q, want %q", got, winner)
	}
}

func TestRehearseBackupRestorePreservesAfterReserveReplacement(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	sourceBefore := readRecoveryFile(t, backup.BackupPath)
	tests := []struct {
		name        string
		replacement func(string) error
		want        []byte
	}{
		{name: "regular-winner", replacement: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.WriteFile(path, []byte("winner"), 0o600)
		}, want: []byte("winner")},
		{name: "hardlink-to-source", replacement: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Link(backup.BackupPath, path)
		}, want: sourceBefore},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
			var replacementPath string
			_, err := rehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath, &recoveryOperations{
				afterCreate: func(path string, _ *os.File) error {
					replacementPath = path
					return tc.replacement(path)
				},
			})
			assertRecoveryCode(t, err, RestoreDestinationReplacedCode)
			if got := readRecoveryFile(t, replacementPath); !bytes.Equal(got, tc.want) {
				t.Fatalf("replacement bytes = %d, want %d", len(got), len(tc.want))
			}
			if info, statErr := os.Stat(replacementPath); statErr != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("replacement mode/stat = %v, want 0600", statErr)
			}
			if !bytes.Equal(sourceBefore, readRecoveryFile(t, backup.BackupPath)) {
				t.Fatal("source backup changed after replacement")
			}
		})
	}
}

func TestRehearseBackupRestoreRejectsSameInodeAfterCopyMutation(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	sourceBefore := readRecoveryFile(t, backup.BackupPath)
	restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
	var targetPath string
	_, err := rehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath, &recoveryOperations{
		afterCopy: func(path string) error {
			targetPath = path
			return os.WriteFile(path, []byte("mutated-in-place"), 0o600)
		},
	})
	assertRecoveryCode(t, err, RestoreCopyMismatchCode)
	assertRecoveryPathAbsent(t, restorePath)
	assertRecoveryPathAbsent(t, targetPath)
	if !bytes.Equal(sourceBefore, readRecoveryFile(t, backup.BackupPath)) {
		t.Fatal("source backup changed after same-inode direct-target mutation")
	}
}

func TestRehearseBackupRestoreRejectsSameInodeBeforeFinalValidation(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	sourceBefore := readRecoveryFile(t, backup.BackupPath)
	livePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	liveBefore := durableSQLiteFilesSnapshot(t, livePath)
	restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
	var targetPath string
	_, err = rehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath, &recoveryOperations{
		beforeFinalValidation: func(path string) error {
			targetPath = path
			return os.WriteFile(path, []byte("mutated-after-logical-verification"), 0o600)
		},
	})
	assertRecoveryCode(t, err, RestoreCopyMismatchCode)
	assertRecoveryPathAbsent(t, restorePath)
	assertRecoveryPathAbsent(t, targetPath)
	if !bytes.Equal(sourceBefore, readRecoveryFile(t, backup.BackupPath)) {
		t.Fatal("source backup changed after final-validation mutation")
	}
	if liveAfter := durableSQLiteFilesSnapshot(t, livePath); !reflect.DeepEqual(liveBefore, liveAfter) {
		t.Fatal("live database changed after final-validation mutation")
	}
}

func TestRehearseBackupRestoreRejectsAndPreservesPreexistingSidecars(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	tests := []struct {
		name string
		seed func(string) error
	}{
		{name: "regular", seed: func(path string) error { return os.WriteFile(path, []byte("sidecar"), 0o600) }},
		{name: "directory", seed: func(path string) error { return os.Mkdir(path, 0o700) }},
		{name: "symlink", seed: func(path string) error { return os.Symlink(filepath.Join(filepath.Dir(path), "sidecar-target"), path) }},
		{name: "dangling-symlink", seed: func(path string) error { return os.Symlink(filepath.Join(filepath.Dir(path), "missing-sidecar"), path) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
			sidecar := restorePath + "-wal"
			if err := tc.seed(sidecar); err != nil {
				t.Fatalf("seed sidecar error = %v", err)
			}
			_, err := RehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath)
			assertRecoveryCode(t, err, RestoreDestinationSidecarsExistCode)
			assertRecoveryPathAbsent(t, restorePath)
			if _, err := os.Lstat(sidecar); err != nil {
				t.Fatalf("sidecar was removed: %v", err)
			}
		})
	}
}

func TestRehearseBackupRestorePreservesInjectedPostReservationSidecar(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, true)
	restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
	sidecar := restorePath + "-shm"
	_, err := rehearseBackupRestore(ctx, root, resolver, backup.BackupPath, restorePath, &recoveryOperations{
		afterCreate: func(_ string, _ *os.File) error { return os.WriteFile(sidecar, []byte("injected"), 0o600) },
	})
	assertRecoveryCode(t, err, RestoreDestinationSidecarsExistCode)
	assertRecoveryPathAbsent(t, restorePath)
	if _, err := os.Lstat(sidecar); err != nil {
		t.Fatalf("injected sidecar was removed: %v", err)
	}
}

func TestRehearseBackupRestoreWrapsLiveDatabasePathFailure(t *testing.T) {
	ctx, root, _, backup := readyRecoveryFixture(t, true)
	restorePath := filepath.Join(t.TempDir(), "restore.sqlite")
	_, err := RehearseBackupRestore(ctx, root, PathResolver{StateHome: "relative-state-home"}, backup.BackupPath, restorePath)
	assertRecoveryCode(t, err, RestoreLiveDatabasePathUnavailableCode)
}

func TestRehearseBackupRestoreAllowsEmptyWatermark(t *testing.T) {
	ctx, root, resolver, backup := readyRecoveryFixture(t, false)
	if backup.JournalWatermark.Present {
		t.Fatalf("empty source watermark = %#v, want absent", backup.JournalWatermark)
	}
	result, err := RehearseBackupRestore(ctx, root, resolver, backup.BackupPath, filepath.Join(t.TempDir(), "empty.sqlite"))
	if err != nil {
		t.Fatalf("RehearseBackupRestore(empty watermark) error = %v", err)
	}
	if result.WatermarkPresent || result.JournalWatermark.Present || !result.RecoveryReady {
		t.Fatalf("empty watermark result = %#v", result)
	}
}

func readyRecoveryFixture(t *testing.T, withJournal bool) (context.Context, project.Root, PathResolver, BackupResult) {
	t.Helper()
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if withJournal {
		if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(recovery): source"}); err != nil {
			t.Fatalf("LogJournal() error = %v", err)
		}
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	return ctx, root, resolver, backup
}

func assertRecoveryCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %q", code)
	}
	var recoveryErr *RecoveryError
	if !errors.As(err, &recoveryErr) || recoveryErr.Code != code {
		t.Fatalf("error = %v, want RecoveryError code %q", err, code)
	}
}

func assertRecoveryPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("path %q stat error = %v, want absent", path, err)
	}
}

func readRecoveryFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

func assertRecoverySidecarsAbsent(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("sidecar %s stat error = %v, want absent", suffix, err)
		}
	}
}
