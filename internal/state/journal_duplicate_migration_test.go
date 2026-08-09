package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func TestJournalDuplicatePolymorphicResidueSweep(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID := seedJournalDuplicateFixtureBase(t)

	const (
		june13ID = "je-poly-june13"
		june24ID = "je-poly-june24"
		bodyText = "journal poly residue body needlexyz"
	)
	seedJournalDuplicateEntry(t, stateHome, root, projectID, june13ID, "decision", "auth", "chose poly rotation", "2026-06-13T01:39:42Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, june24ID, "decision", "auth", "chose poly rotation", "2026-06-24T13:03:15Z")

	now := "2026-06-13T10:00:00Z"
	// Both relationship directions against the June-13 row.
	mustExecOpen(t, stateHome, root, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, 'journal_entry', ?, 'spark', 'spark-poly-target', 'promoted_to', 'from-j13', ?, ?)
`, "rel-from-j13", projectID, june13ID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, 'spark', 'spark-poly-source', 'journal_entry', ?, 'sourced_from', 'to-j13', ?, ?)
`, "rel-to-j13", projectID, june13ID, now, now)
	// Twin residue that must survive the sweep.
	mustExecOpen(t, stateHome, root, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, 'journal_entry', ?, 'spark', 'spark-poly-twin', 'promoted_to', 'from-j24', ?, ?)
`, "rel-from-j24", projectID, june24ID, now, now)

	mustExecOpen(t, stateHome, root, `
INSERT INTO tags (id, project_id, name, created_at, updated_at) VALUES (?, ?, 'journal-poly-tag', ?, ?)
`, "tag-journal-poly", projectID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO entity_tags (id, project_id, tag_id, entity_kind, entity_id, created_at, updated_at)
VALUES (?, ?, ?, 'journal_entry', ?, ?, ?)
`, "etag-journal-poly", projectID, "tag-journal-poly", june13ID, now, now)

	mustExecOpen(t, stateHome, root, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'journal_entry', ?, 'noted', NULL, NULL, 'j13 event', ?, ?)
`, "event-j13", projectID, june13ID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'journal_entry', ?, 'noted', NULL, NULL, 'j24 event', ?, ?)
`, "event-j24", projectID, june24ID, now, now)

	store := openTestStore(t, root, stateHome)
	if _, err := store.UpsertArtifactBody(ctx, projectID, "journal_entry", june13ID, ArtifactBodyKindMarkdown, bodyText, ""); err != nil {
		store.Close()
		t.Fatalf("UpsertArtifactBody for june13: %v", err)
	}
	store.Close()

	mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'journal_entry', ?, 'journal_entry', 'poly-j13-alias', ?, ?)
`, "alias-j13", projectID, june13ID, now, now)

	mustExecOpen(t, stateHome, root, `
INSERT INTO bundles (id, project_id, slug, title, created_at, updated_at)
VALUES (?, ?, 'journal-poly-bundle', 'Journal poly bundle', ?, ?)
`, "bundle-journal-poly", projectID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO bundle_members (id, project_id, bundle_id, entity_kind, entity_id, created_at, updated_at)
VALUES (?, ?, ?, 'journal_entry', ?, ?, ?)
`, "bm-journal-poly", projectID, "bundle-journal-poly", june13ID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, sync_status, created_at, updated_at)
VALUES (?, ?, 'linear', 'journal_entry', ?, 'issue', 'EXT-J13', 'synced', ?, ?)
`, "bmap-journal-poly", projectID, june13ID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO exports (id, project_id, export_kind, format, path, source_entity_kind, source_entity_id, generated_at, created_at, updated_at)
VALUES (?, ?, 'render', 'markdown', 'out-j13.md', 'journal_entry', ?, ?, ?, ?)
`, "export-j13", projectID, june13ID, now, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO exports (id, project_id, export_kind, format, path, source_entity_kind, source_entity_id, generated_at, created_at, updated_at)
VALUES (?, ?, 'render', 'markdown', 'out-j24.md', 'journal_entry', ?, ?, ?, ?)
`, "export-j24", projectID, june24ID, now, now, now)

	beforeJ13 := snapshotJournalPolymorphicResidue(t, stateHome, root, projectID, june13ID)
	if len(beforeJ13) == 0 {
		t.Fatal("expected polymorphic residue on June-13 row before apply")
	}
	beforeJ24 := snapshotJournalPolymorphicResidue(t, stateHome, root, projectID, june24ID)
	if len(beforeJ24) == 0 {
		t.Fatal("expected twin residue on June-24 row before apply")
	}
	beforeBodies := snapshotArtifactBodiesNullable(t, stateHome, root, projectID, "journal_entry", june13ID)
	if len(beforeBodies) == 0 {
		t.Fatal("expected artifact_bodies row for June-13 before apply")
	}
	beforeSearch := snapshotArtifactSearchIndex(t, stateHome, root, "needlexyz")
	if len(beforeSearch) == 0 {
		t.Fatal("expected artifact_search index membership for needlexyz before apply")
	}

	applied, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyJournalDuplicateMigration() error = %v", err)
	}
	if applied.Totals.EntriesRetired != 1 {
		t.Fatalf("entries_retired = %d, want 1", applied.Totals.EntriesRetired)
	}
	assertJournalSearchParityReady(t, stateHome, root)

	if got := countJournalPolymorphicResidue(t, stateHome, root, projectID, june13ID); got != 0 {
		t.Fatalf("polymorphic residue citing retired id after apply = %d, want 0", got)
	}
	afterJ24 := snapshotJournalPolymorphicResidue(t, stateHome, root, projectID, june24ID)
	if !equalRowSnapshots(beforeJ24, afterJ24) {
		t.Fatalf("twin polymorphic residue changed after apply\nbefore=%v\nafter=%v", beforeJ24, afterJ24)
	}

	manifest, err := readJournalDuplicateRollbackManifest(applied.RollbackManifestPath)
	if err != nil {
		t.Fatalf("read rollback manifest: %v", err)
	}
	for id, row := range beforeJ13 {
		found := false
		for _, deleted := range manifest.DeletedRows {
			if deleted.Table != row.table {
				continue
			}
			if journalDuplicateRowValueString(deleted, "id") == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("manifest missing deleted_rows entry for %s id=%s", row.table, id)
		}
	}

	// FTS body gone with the entity.
	if n := artifactSearchMatchCount(t, stateHome, root, "needlexyz"); n != 0 {
		t.Fatalf("artifact_search hits after apply = %d, want 0", n)
	}

	secondApply, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	if secondApply.Totals.EntriesRetired != 0 {
		t.Fatalf("second apply entries_retired = %d, want 0", secondApply.Totals.EntriesRetired)
	}

	rolled, err := RollbackJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RollbackJournalDuplicateMigration() error = %v", err)
	}
	if !rolled.Applied {
		t.Fatalf("rollback result = %#v, want applied", rolled)
	}
	assertJournalSearchParityReady(t, stateHome, root)

	afterRollbackJ13 := snapshotJournalPolymorphicResidue(t, stateHome, root, projectID, june13ID)
	if !equalRowSnapshots(beforeJ13, afterRollbackJ13) {
		t.Fatalf("June-13 polymorphic residue not restored byte-identically\nbefore=%v\nafter=%v", beforeJ13, afterRollbackJ13)
	}
	afterRollbackJ24 := snapshotJournalPolymorphicResidue(t, stateHome, root, projectID, june24ID)
	if !equalRowSnapshots(beforeJ24, afterRollbackJ24) {
		t.Fatalf("June-24 twin residue changed after rollback\nbefore=%v\nafter=%v", beforeJ24, afterRollbackJ24)
	}
	afterBodies := snapshotArtifactBodiesNullable(t, stateHome, root, projectID, "journal_entry", june13ID)
	if !equalArtifactBodySnaps(beforeBodies, afterBodies) {
		t.Fatalf("artifact_bodies not restored byte-identically\nbefore=%v\nafter=%v", beforeBodies, afterBodies)
	}
	afterSearch := snapshotArtifactSearchIndex(t, stateHome, root, "needlexyz")
	if !reflect.DeepEqual(beforeSearch, afterSearch) {
		t.Fatalf("artifact_search index state not restored\nbefore=%v\nafter=%v", beforeSearch, afterSearch)
	}
	if n := artifactSearchMatchCount(t, stateHome, root, "needlexyz"); n != 1 {
		t.Fatalf("artifact_search hits after rollback = %d, want 1", n)
	}
}

func TestJournalDuplicateApplyToleratesMissingArtifactSearchMirror(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID := seedJournalDuplicateFixtureBase(t)

	const (
		june13ID = "je-desync-june13"
		june24ID = "je-desync-june24"
		bodyText = "journal desync body needledesync"
	)
	seedJournalDuplicateEntry(t, stateHome, root, projectID, june13ID, "decision", "auth", "chose desync rotation", "2026-06-13T01:39:42Z")
	seedJournalDuplicateEntry(t, stateHome, root, projectID, june24ID, "decision", "auth", "chose desync rotation", "2026-06-24T13:03:15Z")

	store := openTestStore(t, root, stateHome)
	if _, err := store.UpsertArtifactBody(ctx, projectID, "journal_entry", june13ID, ArtifactBodyKindMarkdown, bodyText, ""); err != nil {
		store.Close()
		t.Fatalf("UpsertArtifactBody: %v", err)
	}
	var rowID int64
	var proj, kind, eid, bkind, content string
	if err := store.db.QueryRow(`
SELECT rowid, project_id, entity_kind, entity_id, body_kind, content
FROM artifact_bodies
WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ? AND body_kind = ?
`, projectID, june13ID, ArtifactBodyKindMarkdown).Scan(&rowID, &proj, &kind, &eid, &bkind, &content); err != nil {
		store.Close()
		t.Fatalf("read artifact body for desync setup: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO artifact_search(artifact_search, rowid, project_id, entity_kind, entity_id, body_kind, content)
VALUES('delete', ?, ?, ?, ?, ?, ?)
`, rowID, proj, kind, eid, bkind, content); err != nil {
		store.Close()
		t.Fatalf("remove artifact_search index entry: %v", err)
	}
	store.Close()

	if n := artifactSearchMatchCount(t, stateHome, root, "needledesync"); n != 0 {
		t.Fatalf("artifact_search hits after desync setup = %d, want 0", n)
	}

	applied, err := ApplyJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, JournalDuplicateApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyJournalDuplicateMigration() error = %v", err)
	}
	if applied.Totals.EntriesRetired != 1 {
		t.Fatalf("entries_retired = %d, want 1", applied.Totals.EntriesRetired)
	}
	if journalEntryExists(t, stateHome, root, june13ID) {
		t.Fatal("June-13 pair still present after apply")
	}

	rolled, err := RollbackJournalDuplicateMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RollbackJournalDuplicateMigration() error = %v", err)
	}
	if !rolled.Applied {
		t.Fatalf("rollback result = %#v, want applied", rolled)
	}
	if !journalEntryExists(t, stateHome, root, june13ID) {
		t.Fatal("June-13 pair not restored after rollback")
	}
	if n := artifactSearchMatchCount(t, stateHome, root, "needledesync"); n != 1 {
		t.Fatalf("artifact_search hits after rollback = %d, want 1", n)
	}
	assertArtifactSearchMatchReadable(t, stateHome, root, "needledesync", projectID, "journal_entry", june13ID)
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

type journalPolyRowSnap struct {
	table string
	cols  map[string]string
}

func snapshotJournalPolymorphicResidue(t *testing.T, stateHome string, root project.Root, projectID, entryID string) map[string]journalPolyRowSnap {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	out := map[string]journalPolyRowSnap{}
	queries := []struct {
		table string
		sql   string
		args  []any
	}{
		{"relationships", `SELECT * FROM relationships WHERE project_id = ? AND ((from_entity_kind = 'journal_entry' AND from_entity_id = ?) OR (to_entity_kind = 'journal_entry' AND to_entity_id = ?))`, []any{projectID, entryID, entryID}},
		{"entity_tags", `SELECT * FROM entity_tags WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ?`, []any{projectID, entryID}},
		{"events", `SELECT * FROM events WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ?`, []any{projectID, entryID}},
		{"bundle_members", `SELECT * FROM bundle_members WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ?`, []any{projectID, entryID}},
		{"backend_mappings", `SELECT * FROM backend_mappings WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ?`, []any{projectID, entryID}},
		{"exports", `SELECT * FROM exports WHERE project_id = ? AND source_entity_kind = 'journal_entry' AND source_entity_id = ?`, []any{projectID, entryID}},
		{"artifact_bodies", `SELECT * FROM artifact_bodies WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ?`, []any{projectID, entryID}},
		{"aliases", `SELECT * FROM aliases WHERE project_id = ? AND entity_kind = 'journal_entry' AND entity_id = ?`, []any{projectID, entryID}},
	}
	for _, q := range queries {
		rows, err := store.db.Query(q.sql, q.args...)
		if err != nil {
			t.Fatalf("query %s: %v", q.table, err)
		}
		scanned, err := scanRows(rows)
		rows.Close()
		if err != nil {
			t.Fatalf("scan %s: %v", q.table, err)
		}
		for _, row := range scanned {
			cols := map[string]string{}
			id := ""
			for col, val := range row {
				s := ""
				if val != nil {
					switch v := val.(type) {
					case string:
						s = v
					case []byte:
						s = string(v)
					default:
						s = fmt.Sprint(v)
					}
				}
				cols[col] = s
				if col == "id" {
					id = s
				}
			}
			if id == "" {
				t.Fatalf("%s row missing id: %#v", q.table, row)
			}
			out[id] = journalPolyRowSnap{table: q.table, cols: cols}
		}
	}
	return out
}

func countJournalPolymorphicResidue(t *testing.T, stateHome string, root project.Root, projectID, entryID string) int {
	t.Helper()
	return len(snapshotJournalPolymorphicResidue(t, stateHome, root, projectID, entryID))
}

func equalRowSnapshots(a, b map[string]journalPolyRowSnap) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || left.table != right.table || len(left.cols) != len(right.cols) {
			return false
		}
		for col, val := range left.cols {
			if right.cols[col] != val {
				return false
			}
		}
	}
	return true
}

func artifactSearchMatchCount(t *testing.T, stateHome string, root project.Root, term string) int {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM artifact_search WHERE artifact_search MATCH ?`, term).Scan(&n); err != nil {
		t.Fatalf("artifact_search MATCH %q: %v", term, err)
	}
	return n
}

func assertArtifactSearchMatchReadable(t *testing.T, stateHome string, root project.Root, term, projectID, entityKind, entityID string) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var gotProjectID, gotKind, gotEntityID, gotBodyKind, gotContent string
	err := store.db.QueryRow(`
SELECT project_id, entity_kind, entity_id, body_kind, content
FROM artifact_search
WHERE artifact_search MATCH ?
`, term).Scan(&gotProjectID, &gotKind, &gotEntityID, &gotBodyKind, &gotContent)
	if err != nil {
		t.Fatalf("read artifact_search MATCH columns for %q: %v", term, err)
	}
	if gotProjectID != projectID || gotKind != entityKind || gotEntityID != entityID {
		t.Fatalf("MATCH columns = project=%q kind=%q id=%q, want project=%q kind=%q id=%q",
			gotProjectID, gotKind, gotEntityID, projectID, entityKind, entityID)
	}
	if gotBodyKind == "" || gotContent == "" {
		t.Fatalf("MATCH body_kind/content empty: body_kind=%q content=%q", gotBodyKind, gotContent)
	}
}

type artifactBodyNullableSnap struct {
	ID          string
	ProjectID   string
	EntityKind  string
	EntityID    string
	BodyKind    string
	Content     string
	ContentHash string
	SourceID    *string
	CreatedAt   string
	UpdatedAt   string
}

// Restored rows get fresh rowids (artifact_bodies keys on a TEXT id), so index
// membership is compared by logical columns, never by rowid.
type artifactSearchMatchSnap struct {
	ProjectID  string
	EntityKind string
	EntityID   string
	BodyKind   string
	Content    string
}

func snapshotArtifactBodiesNullable(t *testing.T, stateHome string, root project.Root, projectID, entityKind, entityID string) []artifactBodyNullableSnap {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	rows, err := store.db.Query(`
SELECT id, project_id, entity_kind, entity_id, body_kind, content, content_hash, source_id, created_at, updated_at
FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
ORDER BY body_kind, id
`, projectID, entityKind, entityID)
	if err != nil {
		t.Fatalf("query artifact_bodies: %v", err)
	}
	defer rows.Close()
	var out []artifactBodyNullableSnap
	for rows.Next() {
		var snap artifactBodyNullableSnap
		var sourceID sql.NullString
		if err := rows.Scan(
			&snap.ID, &snap.ProjectID, &snap.EntityKind, &snap.EntityID, &snap.BodyKind,
			&snap.Content, &snap.ContentHash, &sourceID, &snap.CreatedAt, &snap.UpdatedAt,
		); err != nil {
			t.Fatalf("scan artifact_bodies: %v", err)
		}
		if sourceID.Valid {
			s := sourceID.String
			snap.SourceID = &s
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate artifact_bodies: %v", err)
	}
	return out
}

func equalArtifactBodySnaps(a, b []artifactBodyNullableSnap) bool {
	return reflect.DeepEqual(a, b)
}

func snapshotArtifactSearchIndex(t *testing.T, stateHome string, root project.Root, term string) []artifactSearchMatchSnap {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	rows, err := store.db.Query(`
SELECT project_id, entity_kind, entity_id, body_kind, content
FROM artifact_search
WHERE artifact_search MATCH ?
`, term)
	if err != nil {
		t.Fatalf("query artifact_search MATCH %q: %v", term, err)
	}
	defer rows.Close()
	var snaps []artifactSearchMatchSnap
	for rows.Next() {
		var m artifactSearchMatchSnap
		if err := rows.Scan(&m.ProjectID, &m.EntityKind, &m.EntityID, &m.BodyKind, &m.Content); err != nil {
			t.Fatalf("scan artifact_search MATCH: %v", err)
		}
		snaps = append(snaps, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate artifact_search MATCH: %v", err)
	}
	sort.Slice(snaps, func(i, j int) bool {
		if snaps[i].EntityID != snaps[j].EntityID {
			return snaps[i].EntityID < snaps[j].EntityID
		}
		return snaps[i].BodyKind < snaps[j].BodyKind
	})
	return snaps
}
