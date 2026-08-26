package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestAliasOrphanClassificationProofs(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)

	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "TASK-190"
	twinID := stableMigrationID("task", projectID, alias)
	orphanDerivedID := stableMigrationID("task", legacyID, alias)
	if twinID == orphanDerivedID {
		t.Fatalf("fixture requires distinct twin and derived orphan ids; both %s", twinID)
	}

	seedTask(t, stateHome, root, projectID, twinID, "Archive duplicate work", "done", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, orphanDerivedID, "Archive duplicate work", "done", "2026-06-13T10:00:00Z", false, "")

	contentOrphanID := "task:contentidentity0000001"
	seedTask(t, stateHome, root, projectID, "task:content-twin00000000001", "Content Identity Twin", "todo", "2026-06-24T13:03:00Z", true, "TASK-CONTENT")
	seedTask(t, stateHome, root, projectID, contentOrphanID, "Content Identity Twin", "todo", "2026-06-13T01:39:42Z", false, "")

	unprovenID := "task:unproven00000000000001"
	seedTask(t, stateHome, root, projectID, unprovenID, "Unproven Orphan Title", "todo", "2026-06-13T12:00:00Z", false, "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	if !preview.CopyRun || preview.Applied {
		t.Fatalf("preview copy_run/applied = %t/%t, want true/false", preview.CopyRun, preview.Applied)
	}

	byID := map[string]AliasOrphanRowClassify{}
	for _, project := range preview.Projects {
		for _, table := range project.Tables {
			for _, c := range table.Classifications {
				byID[c.EntityID] = c
			}
		}
	}
	if got := byID[orphanDerivedID]; got.Proof != aliasOrphanProofDerivation || got.Disposition != aliasOrphanDispositionRetire {
		t.Fatalf("derived orphan classify = %#v, want derivation/retire", got)
	}
	if got := byID[contentOrphanID]; got.Proof != aliasOrphanProofContentIdentity || got.Disposition != aliasOrphanDispositionRetire {
		t.Fatalf("content-identity orphan classify = %#v, want content-identity/retire", got)
	}
	if got := byID[unprovenID]; got.Proof != aliasOrphanProofUnproven || got.Disposition != "" {
		t.Fatalf("unproven orphan classify = %#v, want unproven with empty disposition", got)
	}
	if preview.Totals.Retire < 2 || preview.Totals.Unproven < 1 {
		t.Fatalf("preview totals = %#v, want retire>=2 unproven>=1", preview.Totals)
	}

	// Live DB untouched by preview.
	if !entityExists(t, stateHome, root, "tasks", orphanDerivedID) {
		t.Fatal("preview deleted derived orphan on live DB")
	}
	if !entityExists(t, stateHome, root, "tasks", unprovenID) {
		t.Fatal("preview deleted unproven orphan on live DB")
	}
}

func TestAliasOrphanApplyRollbackAndIdempotency(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "SPEC-001"
	twinID := stableMigrationID("spec", projectID, alias)
	orphanID := stableMigrationID("spec", legacyID, alias)

	seedSpec(t, stateHome, root, projectID, twinID, "Canonical Spec", "active", "2026-06-24T13:03:00Z", true, alias)
	seedSpec(t, stateHome, root, projectID, orphanID, "Canonical Spec", "active", "2026-06-13T10:00:00Z", false, "")
	seedSpecResidue(t, stateHome, root, projectID, orphanID)

	danglingAliasID := "alias:dangling000000000001"
	mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'task', 'task:missing0000000000001', 'task', 'TASK-MISSING', ?, ?)
`, danglingAliasID, projectID, "2026-06-24T13:03:00Z", "2026-06-24T13:03:00Z")

	unprovenID := "spec:unproven0000000000001"
	seedSpec(t, stateHome, root, projectID, unprovenID, "Unproven Spec", "active", "2026-06-13T12:00:00Z", false, "")

	// reports table gone (migration 0021); broken-evidence disposition is a no-op when absent.

	// Apply without disposition must leave unproven alone and retire proven rows.
	applied, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	if !applied.Applied || applied.BackupPath == "" || applied.RollbackManifestPath == "" {
		t.Fatalf("apply result incomplete: %#v", applied)
	}
	if _, err := os.Stat(applied.BackupPath); err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if _, err := os.Stat(applied.RollbackManifestPath); err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if entityExists(t, stateHome, root, "specs", orphanID) {
		t.Fatal("derived orphan still present after apply")
	}
	if !entityExists(t, stateHome, root, "specs", twinID) {
		t.Fatal("twin was retired; want preserved")
	}
	if !entityExists(t, stateHome, root, "specs", unprovenID) {
		t.Fatal("unproven orphan was retired without disposition")
	}
	if entityExists(t, stateHome, root, "aliases", danglingAliasID) {
		t.Fatal("dangling alias still present after apply")
	}
	if residueCount(t, stateHome, root, projectID, "spec", orphanID) != 0 {
		t.Fatalf("reference residue remains for retired orphan")
	}
	// Idempotency: second preview classifies zero retire/dangling; second apply no-ops.
	secondPreview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("second PreviewAliasOrphanMigration() error = %v", err)
	}
	if secondPreview.Totals.Retire != 0 || secondPreview.Totals.DanglingAliases != 0 {
		t.Fatalf("second preview totals = %#v, want zero retire and dangling", secondPreview.Totals)
	}
	if secondPreview.Totals.Unproven < 1 {
		t.Fatalf("second preview unproven = %d, want the untouched unproven orphan", secondPreview.Totals.Unproven)
	}
	secondApply, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("second ApplyAliasOrphanMigration() error = %v", err)
	}
	if secondApply.Totals.EntitiesRetired != 0 || secondApply.Totals.AliasesDeleted != 0 {
		t.Fatalf("second apply should no-op retire/delete, got %#v", secondApply.Totals)
	}

	// Rollback restores the retired orphan and residue.
	rolled, err := RollbackAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RollbackAliasOrphanMigration() error = %v", err)
	}
	if !rolled.Applied || rolled.RowsRestored == 0 {
		t.Fatalf("rollback result = %#v, want restored rows", rolled)
	}
	if !entityExists(t, stateHome, root, "specs", orphanID) {
		t.Fatal("orphan not restored after rollback")
	}
	if residueCount(t, stateHome, root, projectID, "spec", orphanID) == 0 {
		t.Fatal("expected residue restored with orphan")
	}
}

func TestAliasOrphanOperatorDispositions(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)

	retireID := "task:operatorretire00000001"
	realiasID := "task:operatorrealias0000001"
	seedTask(t, stateHome, root, projectID, retireID, "Operator Retire Me", "todo", "2026-05-01T00:00:00Z", false, "")
	seedTask(t, stateHome, root, projectID, realiasID, "Operator Realias Me", "todo", "2026-05-01T00:00:00Z", false, "")

	// Without disposition, unproven remain.
	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if preview.Totals.Unproven < 2 || preview.Totals.Retire != 0 {
		t.Fatalf("preview totals = %#v, want unproven>=2 retire=0", preview.Totals)
	}

	applied, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{
		Retire:  []string{retireID},
		Realias: map[string]string{realiasID: "TASK-REALIAS"},
		Flags:   []string{"--retire " + retireID, "--realias " + realiasID + "=TASK-REALIAS"},
	})
	if err != nil {
		t.Fatalf("apply with dispositions error = %v", err)
	}
	if entityExists(t, stateHome, root, "tasks", retireID) {
		t.Fatal("operator --retire left the row in place")
	}
	if !entityExists(t, stateHome, root, "tasks", realiasID) {
		t.Fatal("operator --realias deleted the row")
	}
	if !aliasPointsTo(t, stateHome, root, projectID, "task", "TASK-REALIAS", realiasID) {
		t.Fatal("operator --realias did not attach alias")
	}
	if applied.RollbackManifestPath == "" {
		t.Fatal("expected rollback manifest for operator dispositions")
	}

	// After dispositions, no unproven remain for these IDs.
	after, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("post-disposition preview error = %v", err)
	}
	for _, project := range after.Projects {
		for _, table := range project.Tables {
			for _, c := range table.Classifications {
				if c.EntityID == retireID || c.EntityID == realiasID {
					t.Fatalf("classified %s after disposition: %#v", c.EntityID, c)
				}
			}
		}
	}
}

func TestAliasOrphanPreviewIsolatesAllProjects(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "TASK-MULTI"
	twinID := stableMigrationID("task", projectID, alias)
	orphanID := stableMigrationID("task", legacyID, alias)
	seedTask(t, stateHome, root, projectID, twinID, "Multi Project Task", "todo", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, orphanID, "Multi Project Task", "todo", "2026-06-13T10:00:00Z", false, "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}
	if len(preview.Projects) == 0 {
		t.Fatal("preview reported no projects")
	}
	found := false
	for _, project := range preview.Projects {
		if project.ProjectID == projectID {
			found = true
			if project.LegacyProjectID != legacyID {
				t.Fatalf("legacy project id = %q, want %q", project.LegacyProjectID, legacyID)
			}
			if project.Counts.Retire < 1 {
				t.Fatalf("project counts = %#v, want retire>=1", project.Counts)
			}
		}
	}
	if !found {
		t.Fatalf("preview missing project %s", projectID)
	}
	if !entityExists(t, stateHome, root, "tasks", orphanID) {
		t.Fatal("preview mutated live orphan")
	}
}

// --- fixture helpers ---

func seedAliasOrphanFixtureBase(t *testing.T) (project.Root, string, string, string) {
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
	return root, stateHome, status.ProjectID, status.ProjectCurrentPath
}

func seedTask(t *testing.T, stateHome string, root project.Root, projectID, id, title, status, createdAt string, withAlias bool, alias string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES (?, ?, NULL, ?, ?, NULL, NULL, ?, ?)
`, id, projectID, title, status, createdAt, createdAt)
	if withAlias {
		mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'task', ?, 'task', ?, ?, ?)
`, stableMigrationID("alias", projectID, "task", alias), projectID, id, alias, createdAt, createdAt)
	}
}

func seedSpec(t *testing.T, stateHome string, root project.Root, projectID, id, title, status, createdAt string, withAlias bool, alias string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO specs (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, NULL, ?, ?)
`, id, projectID, title, status, createdAt, createdAt)
	if withAlias {
		mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'spec', ?, 'spec', ?, ?, ?)
`, stableMigrationID("alias", projectID, "spec", alias), projectID, id, alias, createdAt, createdAt)
	}
}

func seedSpecResidue(t *testing.T, stateHome string, root project.Root, projectID, specID string) {
	t.Helper()
	now := "2026-06-13T10:00:00Z"
	sourceID := stableMigrationID("source", projectID, "specs/"+specID+".md")
	mustExecOpen(t, stateHome, root, `
INSERT INTO sources (id, project_id, source_kind, path, hash, line_start, line_end, imported_at, created_at, updated_at)
VALUES (?, ?, 'markdown', ?, 'hash', NULL, NULL, ?, ?, ?)
`, sourceID, projectID, ".agents/specs/"+specID+".md", now, now, now)
	mustExecOpen(t, stateHome, root, `UPDATE specs SET body_source_id = ? WHERE id = ?`, sourceID, specID)
	bodyID := stableMigrationID("artifact_body", projectID, "spec", specID, "markdown")
	mustExecOpen(t, stateHome, root, `
INSERT INTO artifact_bodies (id, project_id, entity_kind, entity_id, body_kind, content, content_hash, source_id, created_at, updated_at)
VALUES (?, ?, 'spec', ?, 'markdown', 'body', 'hash', ?, ?, ?)
`, bodyID, projectID, specID, sourceID, now, now)
	// Index FTS for the body.
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var rowID int64
	if err := store.db.QueryRow(`SELECT rowid FROM artifact_bodies WHERE id = ?`, bodyID).Scan(&rowID); err != nil {
		t.Fatalf("read body rowid: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO artifact_search(rowid, project_id, entity_kind, entity_id, body_kind, content) VALUES (?, ?, 'spec', ?, 'markdown', 'body')`, rowID, projectID, specID); err != nil {
		t.Fatalf("insert artifact_search: %v", err)
	}
	mustExecOpen(t, stateHome, root, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'spec', ?, 'status_changed', 'active', 'archived', 'housekeeping', ?, ?)
`, stableMigrationID("event", projectID, "spec", specID, "archived"), projectID, specID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO tags (id, project_id, name, created_at, updated_at) VALUES (?, ?, 'orphan-tag', ?, ?)
`, stableMigrationID("tag", projectID, "orphan-tag"), projectID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO entity_tags (id, project_id, tag_id, entity_kind, entity_id, created_at, updated_at)
VALUES (?, ?, ?, 'spec', ?, ?, ?)
`, stableMigrationID("entity_tag", projectID, "orphan-tag", "spec", specID), projectID, stableMigrationID("tag", projectID, "orphan-tag"), specID, now, now)
	mustExecOpen(t, stateHome, root, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, 'spec', ?, 'spec', ?, 'related_to', 'fixture', ?, ?)
`, stableMigrationID("relationship", projectID, "spec", specID, "related_to", "spec", specID), projectID, specID, specID, now, now)
}

func mustExecOpen(t *testing.T, stateHome string, root project.Root, query string, args ...any) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	if _, err := store.db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func entityExists(t *testing.T, stateHome string, root project.Root, table, id string) bool {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM `+quoteSQLiteIdentifier(table)+` WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count %s %s: %v", table, id, err)
	}
	return count > 0
}

func residueCount(t *testing.T, stateHome string, root project.Root, projectID, kind, entityID string) int {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	total := 0
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`SELECT COUNT(*) FROM artifact_bodies WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{`SELECT COUNT(*) FROM events WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{`SELECT COUNT(*) FROM entity_tags WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{`SELECT COUNT(*) FROM relationships WHERE project_id = ? AND ((from_entity_kind = ? AND from_entity_id = ?) OR (to_entity_kind = ? AND to_entity_id = ?))`, []any{projectID, kind, entityID, kind, entityID}},
	} {
		var n int
		if err := store.db.QueryRow(q.sql, q.args...).Scan(&n); err != nil {
			t.Fatalf("residue count: %v", err)
		}
		total += n
	}
	return total
}

func mootEventCount(t *testing.T, stateHome string, root project.Root, projectID, entityID string) int {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var n int
	if err := store.db.QueryRow(`
SELECT COUNT(*) FROM events
WHERE project_id = ? AND entity_id = ? AND event_type = ? AND note = ?
`, projectID, entityID, aliasOrphanArchiveMootEventType, aliasOrphanArchiveMootNote).Scan(&n); err != nil {
		t.Fatalf("moot event count: %v", err)
	}
	return n
}

func aliasPointsTo(t *testing.T, stateHome string, root project.Root, projectID, namespace, alias, entityID string) bool {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var got string
	err := store.db.QueryRow(`
SELECT entity_id FROM aliases WHERE project_id = ? AND namespace = ? AND alias = ?
`, projectID, namespace, alias).Scan(&got)
	if err != nil {
		return false
	}
	return got == entityID
}

func sha256Sum(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}
