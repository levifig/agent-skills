package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestJournalDuplicatePairingAndAmbiguity(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID := seedJournalDuplicateFixtureBase(t)

	// Clean 1:1 pair across the two import windows.
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-pair-june13", "decision", "auth", "chose token rotation", "2026-06-13T01:39:42Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-pair-june24", "decision", "auth", "chose token rotation", "2026-06-24T13:03:15Z")

	// Ambiguous: two June-13 candidates share a key with one June-24.
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-amb-j13a", "discover", "scope", "ambiguous message", "2026-06-13T01:40:00Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-amb-j13b", "discover", "scope", "ambiguous message", "2026-06-13T01:41:00Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-amb-j24", "discover", "scope", "ambiguous message", "2026-06-24T13:03:30Z")

	// Legitimate same-day repeat outside the reimport window — not a pair.
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-legit-a", "todo", "work", "same text later", "2026-06-24T15:00:00Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-legit-b", "todo", "work", "same text later", "2026-06-24T16:00:00Z")

	// June-13 only — no twin.
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-solo-j13", "spark", "idea", "solo original", "2026-06-13T01:42:00Z")

	preview, err := PreviewJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("PreviewJournalDuplicateMigration() error = %v", err)
	}
	if !preview.CopyRun || preview.Applied {
		t.Fatalf("preview copy_run/applied = %t/%t, want true/false", preview.CopyRun, preview.Applied)
	}
	if preview.Totals.Pairs != 1 {
		t.Fatalf("pairs = %d, want 1", preview.Totals.Pairs)
	}
	if preview.Totals.Retire != 1 {
		t.Fatalf("retire = %d, want 1 (the clean pair only)", preview.Totals.Retire)
	}
	if preview.Totals.Unproven != 3 {
		t.Fatalf("unproven = %d, want 3", preview.Totals.Unproven)
	}
	if preview.Totals.June13Rows < 4 || preview.Totals.June24Rows < 2 {
		t.Fatalf("window totals = june13=%d june24=%d, want june13>=4 june24>=2", preview.Totals.June13Rows, preview.Totals.June24Rows)
	}

	byID := map[string]JournalDuplicateRowClassify{}
	for _, project := range preview.Projects {
		for _, c := range project.Classifications {
			byID[c.EntryID] = c
		}
	}
	if got := byID["je-pair-june13"]; got.Proof != journalDuplicateProofPair || got.Disposition != journalDuplicateDispositionRetire || got.TwinID != "je-pair-june24" {
		t.Fatalf("pair classify = %#v, want pair/retire twin=je-pair-june24", got)
	}
	if _, ok := byID["je-pair-june24"]; ok {
		t.Fatal("surviving June-24 twin must not be classified as a retire candidate")
	}
	for _, id := range []string{"je-amb-j13a", "je-amb-j13b", "je-amb-j24"} {
		if got := byID[id]; got.Proof != journalDuplicateProofUnproven || got.Disposition != "" {
			t.Fatalf("ambiguous %s classify = %#v, want unproven with empty disposition", id, got)
		}
	}
	for _, id := range []string{"je-legit-a", "je-legit-b", "je-solo-j13"} {
		if _, ok := byID[id]; ok {
			t.Fatalf("%s should not be classified", id)
		}
	}

	// Preview must not touch the live database.
	if !journalEntryExists(t, stateHome, root, "je-pair-june13") {
		t.Fatal("preview deleted pair June-13 row on live DB")
	}
}

func TestJournalDuplicateApplyRollbackIdempotencyAndFTS(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID := seedJournalDuplicateFixtureBase(t)

	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-pair-june13", "decision", "auth", "chose token rotation", "2026-06-13T01:39:42Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-pair-june24", "decision", "auth", "chose token rotation", "2026-06-24T13:03:15Z")
	seedJournalDuplicateOrigin(t, stateHome, root, projectID, "je-pair-june13", "2026-06-13T01:39:42Z")
	seedJournalDuplicateOrigin(t, stateHome, root, projectID, "je-pair-june24", "2026-06-24T13:03:15Z")

	// Soft-ref on the June-13 copy that should repoint to the twin.
	// Seed the spark first so journal-provenance integrity stays ModeSQLiteReady.
	seedJournalDuplicateSpark(t, stateHome, root, projectID, "spark-pair-j13", "fixture spark")
	seedJournalDuplicateDeferral(t, stateHome, root, projectID, "op-pair", "je-pair-june13", "spark-pair-j13")

	beforeCount := journalEntryCount(t, stateHome, root)
	beforeSearch := journalSearchCount(t, stateHome, root)

	applied, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyJournalDuplicateMigration() error = %v", err)
	}
	if !applied.Applied || applied.BackupPath == "" || applied.RollbackManifestPath == "" {
		t.Fatalf("apply result incomplete: %#v", applied)
	}
	if applied.Totals.EntriesRetired != 1 {
		t.Fatalf("entries_retired = %d, want 1", applied.Totals.EntriesRetired)
	}
	if _, err := os.Stat(applied.BackupPath); err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if _, err := os.Stat(applied.RollbackManifestPath); err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if journalEntryExists(t, stateHome, root, "je-pair-june13") {
		t.Fatal("June-13 pair still present after apply")
	}
	if !journalEntryExists(t, stateHome, root, "je-pair-june24") {
		t.Fatal("June-24 twin was retired; want preserved")
	}
	if got := journalEntryCount(t, stateHome, root); got != beforeCount-1 {
		t.Fatalf("journal_entries count = %d, want %d", got, beforeCount-1)
	}
	if got := journalSearchCount(t, stateHome, root); got != beforeSearch-1 {
		t.Fatalf("journal_search count = %d, want %d", got, beforeSearch-1)
	}
	assertJournalSearchParityReady(t, stateHome, root)

	// Soft-ref repointed to survivor.
	if got := journalDeferralEntryID(t, stateHome, root, projectID, "op-pair"); got != "je-pair-june24" {
		t.Fatalf("deferral journal_entry_id = %q, want je-pair-june24", got)
	}

	// Zero cross-window twins remain.
	secondPreview, err := PreviewJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("second preview error = %v", err)
	}
	if secondPreview.Totals.Pairs != 0 || secondPreview.Totals.Retire != 0 {
		t.Fatalf("second preview totals = %#v, want zero pairs/retire", secondPreview.Totals)
	}

	// Idempotent second apply.
	secondApply, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	if secondApply.Totals.EntriesRetired != 0 {
		t.Fatalf("second apply entries_retired = %d, want 0", secondApply.Totals.EntriesRetired)
	}

	// Rollback restores the June-13 row, its origin, FTS, and reverses the repoint.
	rolled, err := RollbackJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RollbackJournalDuplicateMigration() error = %v", err)
	}
	if !rolled.Applied || rolled.RowsRestored == 0 {
		t.Fatalf("rollback result = %#v, want restored rows", rolled)
	}
	if !journalEntryExists(t, stateHome, root, "je-pair-june13") {
		t.Fatal("June-13 pair not restored after rollback")
	}
	if got := journalEntryCount(t, stateHome, root); got != beforeCount {
		t.Fatalf("journal_entries after rollback = %d, want %d", got, beforeCount)
	}
	assertJournalSearchParityReady(t, stateHome, root)
	if got := journalDeferralEntryID(t, stateHome, root, projectID, "op-pair"); got != "je-pair-june13" {
		t.Fatalf("deferral after rollback = %q, want je-pair-june13", got)
	}
}

func TestJournalDuplicateOperatorRetireUnproven(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID := seedJournalDuplicateFixtureBase(t)

	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-amb-j13a", "discover", "scope", "ambiguous message", "2026-06-13T01:40:00Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-amb-j13b", "discover", "scope", "ambiguous message", "2026-06-13T01:41:00Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, "je-amb-j24", "discover", "scope", "ambiguous message", "2026-06-24T13:03:30Z")

	// Without disposition, nothing retires.
	preview, err := PreviewJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if preview.Totals.Retire != 0 || preview.Totals.Unproven != 3 {
		t.Fatalf("preview totals = %#v, want retire=0 unproven=3", preview.Totals)
	}

	applied, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{
		Retire: []string{"je-amb-j13a"},
		Flags:  []string{"--retire", "je-amb-j13a"},
	})
	if err != nil {
		t.Fatalf("apply with --retire error = %v", err)
	}
	// Fixed-point: retiring one multi-candidate leaves a clean 1:1 pair, which
	// the same apply pass then auto-retires (j13b). The June-24 survivor remains.
	if applied.Totals.EntriesRetired != 2 {
		t.Fatalf("entries_retired = %d, want 2 (operator force + follow-on pair)", applied.Totals.EntriesRetired)
	}
	if journalEntryExists(t, stateHome, root, "je-amb-j13a") {
		t.Fatal("operator-retired unproven row still present")
	}
	if journalEntryExists(t, stateHome, root, "je-amb-j13b") {
		t.Fatal("follow-on June-13 pair twin still present after fixed-point pass")
	}
	if !journalEntryExists(t, stateHome, root, "je-amb-j24") {
		t.Fatal("June-24 survivor was removed")
	}
	assertJournalSearchParityReady(t, stateHome, root)
}

func TestJournalDuplicateUnmatchedRetireRefused(t *testing.T) {
	ctx := context.Background()
	root, stateHome, _ := seedJournalDuplicateFixtureBase(t)

	_, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{
		Retire: []string{"je-does-not-exist"},
		Flags:  []string{"--retire", "je-does-not-exist"},
	})
	if err == nil {
		t.Fatal("apply with unmatched --retire error = nil, want refusal")
	}
}

// --- fixture helpers ---

func seedJournalDuplicateFixtureBase(t *testing.T) (project.Root, string, string) {
	t.Helper()
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	dbPath := filepath.Join(stateHome, "loaf", "loaf.sqlite")
	t.Setenv("LOAF_DB", dbPath)
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return root, stateHome, status.ProjectID
}

func seedJournalDuplicateEntry(t *testing.T, stateHome string, root project.Root, projectID, id, entryType, scope, message, createdAt string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, id, projectID, entryType, scope, message, createdAt, createdAt)
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if err := insertJournalSearchTx(context.Background(), tx, projectID, id, "", entryType, scope, message); err != nil {
		t.Fatalf("insert journal_search for %s: %v", id, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit journal_search for %s: %v", id, err)
	}
}

func seedJournalDuplicateOrigin(t *testing.T, stateHome string, root project.Root, projectID, entryID, createdAt string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO journal_origins (
  journal_entry_id, project_id, envelope_version, capture_mechanism, created_at
) VALUES (?, ?, 1, 'test-fixture', ?)
`, entryID, projectID, createdAt)
}

func seedJournalDuplicateSpark(t *testing.T, stateHome string, root project.Root, projectID, sparkID, text string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExecOpen(t, stateHome, root, `
INSERT INTO sparks (id, project_id, text, status, source_id, created_at, updated_at)
VALUES (?, ?, ?, 'captured', NULL, ?, ?)
`, sparkID, projectID, text, now, now)
}

func seedJournalDuplicateDeferral(t *testing.T, stateHome string, root project.Root, projectID, operationKey, entryID, sparkID string) {
	t.Helper()
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExecOpen(t, stateHome, root, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, operationKey, entryID, sparkID, digest, now)
}

func journalEntryExists(t *testing.T, stateHome string, root project.Root, id string) bool {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count journal_entries %s: %v", id, err)
	}
	return count > 0
}

func journalEntryCount(t *testing.T, stateHome string, root project.Root) int {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&count); err != nil {
		t.Fatalf("count journal_entries: %v", err)
	}
	return count
}

func journalSearchCount(t *testing.T, stateHome string, root project.Root) int {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM journal_search`).Scan(&count); err != nil {
		t.Fatalf("count journal_search: %v", err)
	}
	return count
}

func journalDeferralEntryID(t *testing.T, stateHome string, root project.Root, projectID, operationKey string) string {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var id string
	if err := store.db.QueryRow(`SELECT journal_entry_id FROM journal_deferrals WHERE project_id = ? AND operation_key = ?`, projectID, operationKey).Scan(&id); err != nil {
		t.Fatalf("read journal_deferrals: %v", err)
	}
	return id
}

func assertJournalSearchParityReady(t *testing.T, stateHome string, root project.Root) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	parity, err := InspectJournalSearchParity(context.Background(), store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("journal search parity not ready: %#v", parity)
	}
}
