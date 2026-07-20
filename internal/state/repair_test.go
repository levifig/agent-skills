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

func TestRepairRelationshipOriginsDryRunDoesNotWrite(t *testing.T) {
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

	result, err := RepairRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "imported"})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins() error = %v", err)
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

func TestRepairRelationshipOriginsApplyBackfillsCurrentProject(t *testing.T) {
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

	result, err := RepairRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "imported", Apply: true})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins() error = %v", err)
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

func TestRepairRelationshipOriginsRejectsUnknownOrigin(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := RepairRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "guessed", Apply: true})
	if err == nil {
		t.Fatal("RepairRelationshipOrigins() error = nil, want invalid origin error")
	}
}

func TestRepairRelationshipOriginReclassifiesLegacyOriginsAndPreservesForeign(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	ctx := context.Background()
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithOrigin(t, store, projectID, "relationship-intent-create", "intent-create")
	insertRelationshipWithOrigin(t, store, projectID, "relationship-legacy-conversion", "legacy-conversion")
	insertRelationshipWithOrigin(t, store, projectID, "relationship-exploration-create", "exploration-create")
	// 'system' is retired writer provenance released alphas wrote onto run and
	// finding edges; it must reclassify like any other legacy origin rather
	// than linger as a foreign-origin doctor warning.
	insertRelationshipWithOrigin(t, store, projectID, "relationship-system", "system")
	insertRelationshipWithOrigin(t, store, projectID, "relationship-mystery-import", "mystery-import")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	wantPlan := []RelationshipOriginReclassification{
		{Origin: "exploration-create", Target: "command", Matched: 1},
		{Origin: "intent-create", Target: "command", Matched: 1},
		{Origin: "legacy-conversion", Target: "command", Matched: 1},
		{Origin: "system", Target: "command", Matched: 1},
	}
	wantForeign := []RelationshipOriginForeignGroup{{Origin: "mystery-import", Count: 1}}

	dryRun, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Origin: "imported"})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(dry-run) error = %v", err)
	}
	if dryRun.Applied || dryRun.BackupPath != "" {
		t.Fatalf("dry-run result = applied %t backup %q, want unapplied without backup", dryRun.Applied, dryRun.BackupPath)
	}
	if !reflect.DeepEqual(dryRun.Reclassified, wantPlan) {
		t.Fatalf("dry-run Reclassified = %#v, want %#v", dryRun.Reclassified, wantPlan)
	}
	if !reflect.DeepEqual(dryRun.ForeignOrigins, wantForeign) {
		t.Fatalf("dry-run ForeignOrigins = %#v, want %#v", dryRun.ForeignOrigins, wantForeign)
	}
	store = openTestStore(t, root, stateHome)
	assertRelationshipOrigin(t, store, "relationship-intent-create", "intent-create")
	assertRelationshipOrigin(t, store, "relationship-legacy-conversion", "legacy-conversion")
	assertRelationshipOrigin(t, store, "relationship-exploration-create", "exploration-create")
	assertRelationshipOrigin(t, store, "relationship-system", "system")
	assertRelationshipOrigin(t, store, "relationship-mystery-import", "mystery-import")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	applied, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Origin: "imported", Apply: true})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(apply) error = %v", err)
	}
	if !applied.Applied {
		t.Fatal("Applied = false, want true")
	}
	if applied.BackupPath == "" {
		t.Fatal("BackupPath is empty for applied reclassification")
	}
	if _, err := os.Stat(applied.BackupPath); err != nil {
		t.Fatalf("repair backup does not exist: %v", err)
	}
	wantApplied := []RelationshipOriginReclassification{
		{Origin: "exploration-create", Target: "command", Matched: 1, Updated: 1},
		{Origin: "intent-create", Target: "command", Matched: 1, Updated: 1},
		{Origin: "legacy-conversion", Target: "command", Matched: 1, Updated: 1},
		{Origin: "system", Target: "command", Matched: 1, Updated: 1},
	}
	if !reflect.DeepEqual(applied.Reclassified, wantApplied) {
		t.Fatalf("apply Reclassified = %#v, want %#v", applied.Reclassified, wantApplied)
	}
	if !reflect.DeepEqual(applied.ForeignOrigins, wantForeign) {
		t.Fatalf("apply ForeignOrigins = %#v, want %#v", applied.ForeignOrigins, wantForeign)
	}
	store = openTestStore(t, root, stateHome)
	assertRelationshipOrigin(t, store, "relationship-intent-create", "command")
	assertRelationshipOrigin(t, store, "relationship-legacy-conversion", "command")
	assertRelationshipOrigin(t, store, "relationship-exploration-create", "command")
	assertRelationshipOrigin(t, store, "relationship-system", "command")
	assertRelationshipOrigin(t, store, "relationship-mystery-import", "mystery-import")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	rerun, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Origin: "imported", Apply: true})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(rerun) error = %v", err)
	}
	if rerun.BackupPath != "" {
		t.Fatalf("rerun BackupPath = %q, want no backup for a no-op", rerun.BackupPath)
	}
	for _, reclassification := range rerun.Reclassified {
		if reclassification.Matched != 0 || reclassification.Updated != 0 {
			t.Fatalf("rerun reclassification = %#v, want zero matched and updated", reclassification)
		}
	}
	if !reflect.DeepEqual(rerun.ForeignOrigins, wantForeign) {
		t.Fatalf("rerun ForeignOrigins = %#v, want foreign origin still surfaced", rerun.ForeignOrigins)
	}

	store = openTestStore(t, root, stateHome)
	defer store.Close()
	assertRelationshipOrigin(t, store, "relationship-mystery-import", "mystery-import")
	diagnostics, err := inspectRelationshipOriginInvariants(ctx, store)
	if err != nil {
		t.Fatalf("inspectRelationshipOriginInvariants() error = %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one foreign-origin warning", diagnostics)
	}
	if diagnostics[0].Code != "relationship-origin-unknown" || !strings.Contains(diagnostics[0].Message, "mystery-import") {
		t.Fatalf("diagnostic = %#v, want relationship-origin-unknown naming mystery-import", diagnostics[0])
	}
}

// Bare invocation is the Change's Observable Workflow form: it reclassifies the
// retired legacy origins without touching rows that carry no origin at all,
// because choosing a provenance value for those needs operator judgement.
func TestRepairRelationshipOriginsReclassifyOnlyLeavesMissingOriginsUntouched(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	ctx := context.Background()
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithOrigin(t, store, projectID, "relationship-intent-create", "intent-create")
	insertRelationshipWithoutOrigin(t, store, projectID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	dryRun, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(bare dry-run) error = %v", err)
	}
	if dryRun.Mode != RelationshipOriginRepairModeReclassifyOnly {
		t.Fatalf("dry-run Mode = %q, want %q", dryRun.Mode, RelationshipOriginRepairModeReclassifyOnly)
	}
	if dryRun.Origin != "" {
		t.Fatalf("dry-run Origin = %q, want empty in reclassify-only mode", dryRun.Origin)
	}
	if dryRun.Matched != 1 {
		t.Fatalf("dry-run Matched = %d, want the missing-origin row reported", dryRun.Matched)
	}
	if dryRun.Updated != 0 {
		t.Fatalf("dry-run Updated = %d, want 0", dryRun.Updated)
	}
	wantPlan := []RelationshipOriginReclassification{
		{Origin: "exploration-create", Target: "command", Matched: 0},
		{Origin: "intent-create", Target: "command", Matched: 1},
		{Origin: "legacy-conversion", Target: "command", Matched: 0},
		{Origin: "system", Target: "command", Matched: 0},
	}
	if !reflect.DeepEqual(dryRun.Reclassified, wantPlan) {
		t.Fatalf("dry-run Reclassified = %#v, want %#v", dryRun.Reclassified, wantPlan)
	}

	applied, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Apply: true})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(bare apply) error = %v", err)
	}
	if applied.Mode != RelationshipOriginRepairModeReclassifyOnly {
		t.Fatalf("apply Mode = %q, want %q", applied.Mode, RelationshipOriginRepairModeReclassifyOnly)
	}
	if applied.BackupPath == "" {
		t.Fatal("BackupPath is empty for an applied reclassification")
	}
	if _, err := os.Stat(applied.BackupPath); err != nil {
		t.Fatalf("repair backup does not exist: %v", err)
	}
	if applied.Updated != 0 {
		t.Fatalf("apply Updated = %d, want 0 — reclassify-only must never backfill", applied.Updated)
	}

	store = openTestStore(t, root, stateHome)
	assertRelationshipOrigin(t, store, "relationship-intent-create", "command")
	// The missing-origin row is still missing: bare invocation reports it and
	// leaves it for an explicit --origin run.
	assertMissingRelationshipOrigins(t, store, projectID, 1)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	rerun, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Apply: true})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(bare rerun) error = %v", err)
	}
	if rerun.BackupPath != "" {
		t.Fatalf("rerun BackupPath = %q, want no backup for a no-op", rerun.BackupPath)
	}
	for _, reclassification := range rerun.Reclassified {
		if reclassification.Matched != 0 || reclassification.Updated != 0 {
			t.Fatalf("rerun reclassification = %#v, want zero matched and updated", reclassification)
		}
	}
	if rerun.Matched != 1 {
		t.Fatalf("rerun Matched = %d, want the missing-origin row still reported", rerun.Matched)
	}
}

// V3's doctor-clean requirement: a database whose only unknown origins are the
// retired legacy values is fully healed by one apply, with no residual warning.
// The mixed fixture above proves foreign origins survive; this one proves
// nothing else does.
func TestRepairRelationshipOriginsLegacyOnlyFixtureIsDoctorCleanAfterApply(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	ctx := context.Background()
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	for _, origin := range legacyRelationshipOrigins() {
		insertRelationshipWithOrigin(t, store, projectID, "relationship-"+origin, origin)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	applied, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Apply: true})
	if err != nil {
		t.Fatalf("RepairRelationshipOrigins(apply) error = %v", err)
	}
	if applied.BackupPath == "" {
		t.Fatal("BackupPath is empty for an applied reclassification")
	}
	if len(applied.ForeignOrigins) != 0 {
		t.Fatalf("ForeignOrigins = %#v, want none in a legacy-only fixture", applied.ForeignOrigins)
	}
	for _, reclassification := range applied.Reclassified {
		if reclassification.Matched != 1 || reclassification.Updated != 1 {
			t.Fatalf("reclassification = %#v, want exactly one row matched and updated", reclassification)
		}
	}

	store = openTestStore(t, root, stateHome)
	defer store.Close()
	for _, origin := range legacyRelationshipOrigins() {
		assertRelationshipOrigin(t, store, "relationship-"+origin, "command")
	}
	diagnostics, err := inspectRelationshipOriginInvariants(ctx, store)
	if err != nil {
		t.Fatalf("inspectRelationshipOriginInvariants() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want doctor clean after repairing a legacy-only database", diagnostics)
	}
}

func TestRepairRelationshipOriginsFailedWriteAfterBackupPreservesBackupPath(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	ctx := context.Background()
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithoutOrigin(t, store, projectID)
	// Abort every relationship update at the SQLite layer so the repair fails
	// after its pre-repair backup exists. That window is the only one in which
	// the backup path is the operator's sole recovery reference, so dropping it
	// from the surfaced result would strand them.
	if _, err := store.db.ExecContext(ctx, `
CREATE TRIGGER forced_relationship_update_failure
BEFORE UPDATE ON relationships
BEGIN
  SELECT RAISE(ABORT, 'forced post-backup failure');
END
`); err != nil {
		store.Close()
		t.Fatalf("create forced failure trigger error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := RepairRelationshipOrigins(ctx, root, resolver, RelationshipOriginRepairOptions{Origin: "imported", Apply: true})
	if err == nil {
		t.Fatal("RepairRelationshipOrigins() error = nil, want forced post-backup failure")
	}
	var repairErr *RelationshipOriginRepairError
	if !errors.As(err, &repairErr) {
		t.Fatalf("error = %T %v, want *RelationshipOriginRepairError", err, err)
	}
	if errors.Unwrap(repairErr) == nil {
		t.Fatal("Unwrap() = nil, want the underlying repair failure")
	}
	if !strings.Contains(err.Error(), "forced post-backup failure") {
		t.Fatalf("error = %v, want the underlying SQLite abort preserved", err)
	}
	if repairErr.Result.BackupPath == "" {
		t.Fatal("preserved result BackupPath is empty after a post-backup failure")
	}
	if !strings.Contains(err.Error(), repairErr.Result.BackupPath) {
		t.Fatalf("error = %v, want the preserved backup path named", err)
	}
	if result.BackupPath != repairErr.Result.BackupPath {
		t.Fatalf("returned result BackupPath = %q, want %q from the preserved error", result.BackupPath, repairErr.Result.BackupPath)
	}
	if _, err := os.Stat(repairErr.Result.BackupPath); err != nil {
		t.Fatalf("preserved backup does not exist: %v", err)
	}
	verification, err := VerifyBackup(ctx, repairErr.Result.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup(preserved) error = %v", err)
	}
	if !verification.Verified {
		t.Fatalf("preserved backup verification = %#v, want verified", verification)
	}

	// The live row is untouched, so a rerun after the operator clears the fault
	// still has the same work to do.
	store = openTestStore(t, root, stateHome)
	defer store.Close()
	assertMissingRelationshipOrigins(t, store, projectID, 1)
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

func insertRelationshipWithOrigin(t *testing.T, store *Store, projectID string, id string, origin string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'spark', 'spark-one', 'intent', ?, 'source-of', 'seeded provenance row', ?, '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')
`, id, projectID, "intent-for-"+id, origin); err != nil {
		t.Fatalf("insert relationship with origin %q error = %v", origin, err)
	}
}

func assertRelationshipOrigin(t *testing.T, store *Store, id string, want string) {
	t.Helper()
	var got string
	if err := store.db.QueryRowContext(context.Background(), `SELECT origin FROM relationships WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatalf("read relationship %s origin error = %v", id, err)
	}
	if got != want {
		t.Fatalf("relationship %s origin = %q, want %q", id, got, want)
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
