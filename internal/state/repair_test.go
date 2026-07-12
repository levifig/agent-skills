package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRepairJournalSearchDryRunApplyAndIdempotency(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(repair): canonical repair"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	var journalID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries LIMIT 1`).Scan(&journalID); err != nil {
		store.Close()
		t.Fatalf("read journal ID error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID); err != nil {
		store.Close()
		t.Fatalf("delete derived row error = %v", err)
	}
	before := durableSQLiteFilesSnapshot(t, status.DatabasePath)
	dryRun, err := RepairJournalSearch(ctx, root, resolver, JournalSearchRepairOptions{})
	if err != nil {
		t.Fatalf("RepairJournalSearch(dry-run) error = %v", err)
	}
	if dryRun.Applied || dryRun.BackupPath != "" || dryRun.BackupVerified || dryRun.Rebuilt != 0 || dryRun.ParityVerified {
		t.Fatalf("dry-run result = %#v, want no mutation", dryRun)
	}
	if dryRun.CanonicalRows != 1 || dryRun.IndexRows != 0 || dryRun.Missing != 1 || dryRun.Extra != 0 || dryRun.Changed != 0 {
		t.Fatalf("dry-run parity = %#v, want canonical=1/index=0/missing=1", dryRun)
	}
	after := durableSQLiteFilesSnapshot(t, status.DatabasePath)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("dry-run changed durable database/WAL files: before=%#v after=%#v", before, after)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	applied, err := RepairJournalSearch(ctx, root, resolver, JournalSearchRepairOptions{Apply: true})
	if err != nil {
		t.Fatalf("RepairJournalSearch(apply) error = %v", err)
	}
	if !applied.Applied || !applied.BackupVerified || applied.BackupPath == "" || applied.Rebuilt != 1 || !applied.ParityVerified {
		t.Fatalf("apply result = %#v, want verified backup and one rebuilt row", applied)
	}
	backupVerification, err := VerifyBackup(ctx, applied.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup(pre-repair) error = %v", err)
	}
	if !backupVerification.Verified || backupVerification.JournalRetrievalReady || backupVerification.JournalSearchParity.Ready {
		t.Fatalf("pre-repair backup verification = %#v, want structurally verified but retrieval-not-ready", backupVerification)
	}
	if backupVerification.JournalSearchParity.CanonicalRows != 1 || backupVerification.JournalSearchParity.IndexRows != 0 || backupVerification.JournalSearchParity.Missing != 1 {
		t.Fatalf("pre-repair backup parity = %#v, want canonical=1/index=0/missing=1", backupVerification.JournalSearchParity)
	}

	store = openTestStore(t, root, stateHome)
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		store.Close()
		t.Fatalf("InspectJournalSearchParity(final) error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 1 || parity.IndexRows != 1 {
		store.Close()
		t.Fatalf("final parity = %#v, want ready with one row", parity)
	}
	search, err := store.SearchJournal(ctx, root, SearchOptions{Query: "canonical"})
	if err != nil {
		store.Close()
		t.Fatalf("SearchJournal(final) error = %v", err)
	}
	if len(search.Results) != 1 {
		store.Close()
		t.Fatalf("final search hits = %d, want 1", len(search.Results))
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(final) error = %v", err)
	}

	second, err := RepairJournalSearch(ctx, root, resolver, JournalSearchRepairOptions{Apply: true})
	if err != nil {
		t.Fatalf("RepairJournalSearch(second apply) error = %v", err)
	}
	if !second.Applied || second.BackupPath != "" || second.BackupVerified || second.Rebuilt != 0 || !second.ParityVerified {
		t.Fatalf("second apply result = %#v, want idempotent no-op without backup", second)
	}
}

func TestRepairJournalSearchFailedRebuildRollsBackAndPreservesBackup(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	_, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(repair): forced failure"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	var journalID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries LIMIT 1`).Scan(&journalID); err != nil {
		store.Close()
		t.Fatalf("read journal ID error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID); err != nil {
		store.Close()
		t.Fatalf("delete derived row error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE journal_search`); err != nil {
		store.Close()
		t.Fatalf("drop journal_search error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE journal_search (
  rowid INTEGER PRIMARY KEY,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  harness_session_id TEXT,
  entry_type TEXT NOT NULL,
  scope TEXT NOT NULL,
  message TEXT NOT NULL,
  required TEXT NOT NULL
)`); err != nil {
		store.Close()
		t.Fatalf("create rebuild failure journal_search error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := RepairJournalSearch(ctx, root, resolver, JournalSearchRepairOptions{Apply: true})
	if err == nil || !strings.Contains(err.Error(), "rebuild journal search") {
		t.Fatalf("RepairJournalSearch() error = %v, want rebuild failure", err)
	}
	if result.BackupPath == "" || !result.BackupVerified {
		t.Fatalf("failed repair result = %#v, want preserved verified backup", result)
	}
	backupVerification, err := VerifyBackup(ctx, result.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup(preserved) error = %v", err)
	}
	if !backupVerification.Verified || backupVerification.JournalRetrievalReady {
		t.Fatalf("preserved backup verification = %#v, want structural true/retrieval false", backupVerification)
	}
	store = openTestStore(t, root, stateHome)
	defer store.Close()
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity(after rollback) error = %v", err)
	}
	if parity.Ready || parity.Missing != 1 || parity.CanonicalRows != 1 || parity.IndexRows != 0 {
		t.Fatalf("parity after rollback = %#v, want original divergence", parity)
	}
}

func TestRepairJournalSearchHookFailureRollsBackAfterRebuild(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(hook): rollback"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search`); err != nil {
		store.Close()
		t.Fatalf("delete journal search error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := repairJournalSearch(ctx, root, resolver, JournalSearchRepairOptions{Apply: true}, func(context.Context, *sql.Conn) error {
		return errors.New("forced post-rebuild failure")
	})
	if err == nil || !strings.Contains(err.Error(), "forced post-rebuild failure") {
		t.Fatalf("repair error = %v, want forced post-rebuild failure", err)
	}
	var repairErr *JournalSearchRepairError
	if !errors.As(err, &repairErr) {
		t.Fatalf("repair error = %T %v, want JournalSearchRepairError", err, err)
	}
	if result.BackupPath == "" || !result.BackupVerified || result.Rebuilt != 1 {
		t.Fatalf("partial result = %#v, want verified backup and rebuilt count", result)
	}
	verified, err := VerifyBackup(ctx, result.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	if !verified.Verified || verified.JournalRetrievalReady {
		t.Fatalf("backup verification = %#v, want structural true/retrieval false", verified)
	}
	store = openTestStore(t, root, stateHome)
	defer store.Close()
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if parity.Ready || parity.Missing != 1 || parity.CanonicalRows != 1 || parity.IndexRows != 0 {
		t.Fatalf("live parity = %#v, want original divergence", parity)
	}
}

func TestRepairJournalSearchHoldsImmediateLockThroughBackupAndCommit(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(lock): before"}); err != nil {
		t.Fatalf("LogJournal(before) error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search`); err != nil {
		store.Close()
		t.Fatalf("delete journal search error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	hookStarted := make(chan struct{})
	release := make(chan struct{})
	repairDone := make(chan struct{})
	var repairResult JournalSearchRepairResult
	var repairErr error
	go func() {
		repairResult, repairErr = repairJournalSearch(ctx, root, resolver, JournalSearchRepairOptions{Apply: true}, func(context.Context, *sql.Conn) error {
			close(hookStarted)
			<-release
			return nil
		})
		close(repairDone)
	}()
	select {
	case <-hookStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for immediate repair lock")
	}

	// The public writer correctly rejects known divergence before it attempts a
	// write. Use an already-open store to exercise the actual SQLite writer
	// against the repair's immediate transaction instead.
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	writerStore, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(writer) error = %v", err)
	}
	defer writerStore.Close()
	writerDone := make(chan error, 1)
	go func() {
		_, err := writerStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(lock): after"})
		writerDone <- err
	}()
	select {
	case err := <-writerDone:
		t.Fatalf("writer completed while repair lock held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-repairDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for repair commit")
	}
	if repairErr != nil {
		t.Fatalf("repair error = %v", repairErr)
	}
	if !repairResult.Applied || repairResult.BackupPath == "" || !repairResult.BackupVerified || !repairResult.ParityVerified {
		t.Fatalf("repair result = %#v, want applied/verified", repairResult)
	}
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("writer error after repair commit = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for writer after repair commit")
	}
	backup, err := VerifyBackup(ctx, repairResult.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	if backup.JournalSearchParity.CanonicalRows != 1 || backup.JournalSearchParity.IndexRows != 0 {
		t.Fatalf("backup parity = %#v, want pre-lock one canonical/zero index row", backup.JournalSearchParity)
	}
	store = openTestStore(t, root, stateHome)
	defer store.Close()
	var liveRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&liveRows); err != nil {
		t.Fatalf("count live journal rows error = %v", err)
	}
	if liveRows != 2 {
		t.Fatalf("live journal rows = %d, want 2 including post-lock writer", liveRows)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity(live) error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 2 || parity.IndexRows != 2 {
		t.Fatalf("live parity = %#v, want ready two rows", parity)
	}
}

func TestRepairMissingRelationshipOriginsDryRunDoesNotWrite(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithoutOrigin(t, store, projectID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := RepairMissingRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "imported"})
	if err != nil {
		t.Fatalf("RepairMissingRelationshipOrigins() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", result.ProjectID, projectID)
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if result.Applied {
		t.Fatal("Applied = true, want dry-run")
	}
	if result.Matched != 1 {
		t.Fatalf("Matched = %d, want 1", result.Matched)
	}
	if result.Updated != 0 {
		t.Fatalf("Updated = %d, want 0 for dry-run", result.Updated)
	}

	store = openTestStore(t, root, stateHome)
	defer store.Close()
	assertMissingRelationshipOrigins(t, store, projectID, 1)
}

func TestRepairMissingRelationshipOriginsApplyBackfillsCurrentProject(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithoutOrigin(t, store, projectID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := RepairMissingRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "imported", Apply: true})
	if err != nil {
		t.Fatalf("RepairMissingRelationshipOrigins() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", result.ProjectID, projectID)
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if result.Matched != 1 {
		t.Fatalf("Matched = %d, want 1", result.Matched)
	}
	if result.Updated != 1 {
		t.Fatalf("Updated = %d, want 1", result.Updated)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty for applied repair")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("repair backup does not exist: %v", err)
	}

	store = openTestStore(t, root, stateHome)
	defer store.Close()
	assertMissingRelationshipOrigins(t, store, projectID, 0)
	var origin string
	if err := store.db.QueryRowContext(context.Background(), `SELECT origin FROM relationships WHERE id = 'relationship-without-origin'`).Scan(&origin); err != nil {
		t.Fatalf("read repaired relationship origin error = %v", err)
	}
	if origin != "imported" {
		t.Fatalf("origin = %q, want imported", origin)
	}
}

func TestRepairMissingRelationshipOriginsRejectsUnknownOrigin(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := RepairMissingRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "guessed", Apply: true})
	if err == nil {
		t.Fatal("RepairMissingRelationshipOrigins() error = nil, want invalid origin error")
	}
}

func TestArchiveLegacyProjectDatabaseDryRunDoesNotMoveFiles(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	if _, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{}); err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	result, err := ArchiveLegacyProjectDatabase(root, PathResolver{}, false)
	if err != nil {
		t.Fatalf("ArchiveLegacyProjectDatabase() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if result.Applied {
		t.Fatal("Applied = true, want dry-run")
	}
	if result.Action != LegacyProjectDatabaseArchiveAction {
		t.Fatalf("Action = %q, want %q", result.Action, LegacyProjectDatabaseArchiveAction)
	}
	wantMatched := []string{legacyPath}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(legacyPath + suffix); err == nil {
			wantMatched = append(wantMatched, legacyPath+suffix)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat legacy sidecar %q: %v", suffix, err)
		}
	}
	if !reflect.DeepEqual(result.MatchedPaths, wantMatched) {
		t.Fatalf("MatchedPaths = %#v, want present database artifacts %#v", result.MatchedPaths, wantMatched)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy database moved during dry-run: %v", err)
	}
	if result.ArchivePath == "" {
		t.Fatal("ArchivePath is empty")
	}
	if _, err := os.Stat(result.ArchivePath); !os.IsNotExist(err) {
		t.Fatalf("archive path exists during dry-run; err = %v", err)
	}
}

func TestArchiveLegacyProjectDatabaseApplyMovesDatabaseAndSidecars(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	sidecarPath := legacyPath + "-wal"
	if _, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{}); err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte("sidecar"), 0o600); err != nil {
		t.Fatalf("write legacy sidecar error = %v", err)
	}
	wantArchived := []string{legacyPath}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(legacyPath + suffix); err == nil {
			wantArchived = append(wantArchived, legacyPath+suffix)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat legacy sidecar %q: %v", suffix, err)
		}
	}

	result, err := ArchiveLegacyProjectDatabase(root, PathResolver{}, true)
	if err != nil {
		t.Fatalf("ArchiveLegacyProjectDatabase() error = %v", err)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	// SQLite may omit an unused WAL or SHM sidecar. Every sidecar that is
	// present must be archived, without treating an ephemeral SHM file as a
	// required durable artifact.
	if len(result.ArchivedPaths) != len(wantArchived) {
		t.Fatalf("ArchivedPaths = %#v, want one archive per present source %#v", result.ArchivedPaths, wantArchived)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy database still exists after archive; err = %v", err)
	}
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Fatalf("legacy sidecar still exists after archive; err = %v", err)
	}
	for _, path := range result.ArchivedPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("archived path %q missing: %v", path, err)
		}
	}
}

func TestArchiveLegacyProjectDatabaseRejectsPendingMigration(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	initializeLegacyStateDatabase(t, root, PathResolver{})
	databasePath, err := (PathResolver{}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("create global database dir error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations(global) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}

	_, err = ArchiveLegacyProjectDatabase(root, PathResolver{}, true)
	if err == nil {
		t.Fatal("ArchiveLegacyProjectDatabase() error = nil, want pending migration error")
	}
	if !strings.Contains(err.Error(), "legacy project database still needs migration") {
		t.Fatalf("error = %v, want pending migration error", err)
	}
}

func insertRelationshipWithoutOrigin(t *testing.T, store *Store, projectID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES ('relationship-without-origin', ?, 'task', 'task-one', 'spec', 'spec-one', 'implements', 'legacy row', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert relationship without origin error = %v", err)
	}
}

func assertMissingRelationshipOrigins(t *testing.T, store *Store, projectID string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM relationships
WHERE project_id = ?
  AND (origin IS NULL OR TRIM(origin) = '')
`, projectID).Scan(&got); err != nil {
		t.Fatalf("count missing relationship origins error = %v", err)
	}
	if got != want {
		t.Fatalf("missing relationship origins = %d, want %d", got, want)
	}
}
