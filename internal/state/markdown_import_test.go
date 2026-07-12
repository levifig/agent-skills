package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestApplyMarkdownMigrationImportsArtifactsAndPreservesSources(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	taskBody := "# Task body\n\nFirst paragraph.\n\nSecond paragraph.\n"
	writeMarkdownImportFixture(t, root.Path(), taskBody)

	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ImportScope != "project" {
		t.Fatalf("ImportScope = %q, want project", result.ImportScope)
	}
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName == "" {
		t.Fatal("ProjectName is empty")
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if _, err := os.Stat(result.DatabasePath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
	if result.Specs != 1 || result.Tasks != 1 || result.Ideas != 1 || result.Brainstorms != 1 || result.Sessions != 1 || result.Reports != 1 || result.Sparks != 1 {
		t.Fatalf("result counts = %#v, want one imported artifact of each fixture kind", result.MarkdownMigrationPlan)
	}
	if result.Relationships != 2 {
		t.Fatalf("Relationships = %d, want dry-run relationship count 2", result.Relationships)
	}

	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	assertTableCount(t, store, "specs", 1)
	assertTableCount(t, store, "tasks", 1)
	assertTableCount(t, store, "ideas", 1)
	assertTableCount(t, store, "brainstorms", 1)
	assertTableCount(t, store, "reports", 1)
	assertTableCount(t, store, "sparks", 1)
	assertTableCount(t, store, "artifact_bodies", 5)
	assertTableCount(t, store, "journal_entries", 1)
	assertTableCount(t, store, "journal_origins", 1)
	var mechanism, sourceEvent, branch, worktree, harnessSessionID string
	if err := store.db.QueryRowContext(context.Background(), `SELECT capture_mechanism, source_event, COALESCE(branch, ''), COALESCE(worktree, ''), COALESCE(harness_session_id, '') FROM journal_origins`).Scan(&mechanism, &sourceEvent, &branch, &worktree, &harnessSessionID); err != nil {
		t.Fatalf("read imported journal origin: %v", err)
	}
	if mechanism != JournalOriginMechanismMigration || sourceEvent != "markdown_import" || branch != "feature/example" || worktree != "" || harnessSessionID != "" {
		t.Fatalf("imported origin = %q/%q/%q/%q/%q, want migration/markdown_import and observable legacy fields", mechanism, sourceEvent, branch, worktree, harnessSessionID)
	}
	assertTableCount(t, store, "relationships", 2)
	assertArtifactSearchHitCount(t, store, "Second", 1)

	var sourceHash string
	err = store.db.QueryRowContext(
		context.Background(),
		`SELECT hash FROM sources WHERE path = ?`,
		".agents/tasks/TASK-001-example.md",
	).Scan(&sourceHash)
	if err != nil {
		t.Fatalf("read source hash error = %v", err)
	}
	sum := sha256.Sum256([]byte(taskBody))
	if sourceHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("source hash = %q, want task body hash", sourceHash)
	}
	var importedTaskBody string
	err = store.db.QueryRowContext(
		context.Background(),
		`SELECT artifact_bodies.content
FROM artifact_bodies
JOIN tasks ON tasks.project_id = artifact_bodies.project_id
 AND tasks.id = artifact_bodies.entity_id
WHERE tasks.project_id = ?
  AND artifact_bodies.entity_kind = 'task'
  AND artifact_bodies.body_kind = 'markdown'`,
		result.ProjectID,
	).Scan(&importedTaskBody)
	if err != nil {
		t.Fatalf("read imported task body error = %v", err)
	}
	if importedTaskBody != strings.TrimSpace(taskBody) {
		t.Fatalf("artifact body = %q, want byte-exact frontmatter-stripped task body", importedTaskBody)
	}
	taskPath := filepath.Join(root.Path(), ".agents", "tasks", "TASK-001-example.md")
	contentAfterApply, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("ReadFile(task) error = %v", err)
	}
	if string(contentAfterApply) != taskBody {
		t.Fatalf("task source was mutated: %q", string(contentAfterApply))
	}
	if _, err := store.db.ExecContext(
		context.Background(),
		`UPDATE aliases SET id = ? WHERE project_id = ? AND namespace = ? AND alias = ?`,
		"legacy-spec-alias-id",
		result.ProjectID,
		"spec",
		"SPEC-001",
	); err != nil {
		t.Fatalf("seed legacy alias id error = %v", err)
	}

	second, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if second.DatabasePath != result.DatabasePath {
		t.Fatalf("DatabasePath changed: %q -> %q", result.DatabasePath, second.DatabasePath)
	}
	assertTableCount(t, store, "specs", 1)
	assertTableCount(t, store, "tasks", 1)
	assertTableCount(t, store, "relationships", 2)
	assertTableCount(t, store, "aliases", 7)
	assertTableCount(t, store, "journal_origins", 1)
}

func TestImportMarkdownBackfillsMissingOriginAndProtectsNonMigrationOrigin(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/import-origin.md", `---
branch: feature/origin
harness_session_id: hsid-origin
---
[2026-07-10 10:00] decision(import): preserve origin
`)
	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var journalID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries`).Scan(&journalID); err != nil {
		t.Fatalf("read imported journal ID: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, result.ProjectID, journalID); err != nil {
		t.Fatalf("delete origin fixture: %v", err)
	}
	if err := store.ImportMarkdown(ctx, root); err != nil {
		t.Fatalf("ImportMarkdown(backfill) error = %v", err)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, result.ProjectID, journalID); got != 1 {
		t.Fatalf("backfilled origin rows = %d, want 1", got)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'manual', source_event = 'manual' WHERE project_id = ? AND journal_entry_id = ?`, result.ProjectID, journalID); err != nil {
		t.Fatalf("mark non-migration origin: %v", err)
	}
	if err := store.ImportMarkdown(ctx, root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite non-migration journal origin") {
		t.Fatalf("ImportMarkdown(non-migration origin) error = %v, want deterministic refusal", err)
	}
	var mechanism string
	if err := store.db.QueryRowContext(ctx, `SELECT capture_mechanism FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, result.ProjectID, journalID).Scan(&mechanism); err != nil {
		t.Fatalf("read protected origin: %v", err)
	}
	if mechanism != "manual" {
		t.Fatalf("protected origin mechanism = %q, want manual", mechanism)
	}
}

func TestApplyMarkdownMigrationDoesNotRequireTasksJSON(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-markdown-only.md", `---
id: TASK-001
title: Markdown Only Task
status: todo
priority: P2
depends_on: []
---

# Markdown Only Task
`)

	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if !result.Applied || result.Tasks != 1 {
		t.Fatalf("result = %#v, want one applied markdown task", result)
	}

	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	assertTableCount(t, store, "tasks", 1)
	assertTableCount(t, store, "sources", 1)
	assertTableCount(t, store, "relationships", 0)
}

func TestImportMarkdownRebuildsChangedJournalContentWithoutStaleRows(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260710-journal.md", `---
harness_session_id: hsid-markdown
---
[2026-07-10 10:00] decision(import): original content
`)
	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	writeAgentsFile(t, root.Path(), "sessions/20260710-journal.md", `---
harness_session_id: hsid-markdown
---
[2026-07-10 10:00] decision(import): updated content
`)
	if err := store.ImportMarkdown(ctx, root); err != nil {
		t.Fatalf("ImportMarkdown(updated) error = %v", err)
	}

	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 1 || parity.IndexRows != 1 {
		t.Fatalf("journal parity = %#v, want one exact ready row", parity)
	}
	var journalID, message string
	if err := store.db.QueryRowContext(ctx, `SELECT id, message FROM journal_entries`).Scan(&journalID, &message); err != nil {
		t.Fatalf("read canonical journal row: %v", err)
	}
	if message != "updated content" {
		t.Fatalf("canonical message = %q, want updated content", message)
	}
	var matching, stale, derivedCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE journal_search MATCH 'updated'`).Scan(&matching); err != nil {
		t.Fatalf("query updated journal_search row: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE journal_search MATCH 'original'`).Scan(&stale); err != nil {
		t.Fatalf("query stale journal_search row: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = ?`, journalID).Scan(&derivedCount); err != nil {
		t.Fatalf("count derived journal row: %v", err)
	}
	if matching != 1 || stale != 0 || derivedCount != 1 {
		t.Fatalf("journal_search updated=%d stale=%d derived=%d, want 1/0/1", matching, stale, derivedCount)
	}
}

func TestImportMarkdownParityFailureRollsBackCanonicalAndDerivedState(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260710-journal.md", `---
harness_session_id: hsid-markdown
---
[2026-07-10 10:00] decision(import): capture this
`)
	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	var journalID, projectID, entryType, scope, message, harnessSessionID string
	var rowID int64
	if err := store.db.QueryRowContext(ctx, `
SELECT rowid, id, project_id, entry_type, COALESCE(scope, ''), message, COALESCE(harness_session_id, '')
FROM journal_entries`).Scan(&rowID, &journalID, &projectID, &entryType, &scope, &message, &harnessSessionID); err != nil {
		t.Fatalf("read initial journal row: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE journal_search`); err != nil {
		t.Fatalf("drop derived journal_search fixture: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
CREATE TABLE journal_search (
  rowid INTEGER PRIMARY KEY,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  harness_session_id TEXT,
  entry_type TEXT NOT NULL,
  scope TEXT NOT NULL,
  message TEXT NOT NULL CHECK(message = 'capture this')
)`); err != nil {
		t.Fatalf("create failing journal_search fixture: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, harness_session_id, entry_type, scope, message)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, rowID, projectID, journalID, harnessSessionID, entryType, scope, message); err != nil {
		t.Fatalf("seed derived journal_search fixture: %v", err)
	}

	writeAgentsFile(t, root.Path(), "sessions/20260710-journal.md", `---
harness_session_id: hsid-markdown
---
[2026-07-10 10:00] decision(import): changed and should fail
`)
	if err := store.ImportMarkdown(ctx, root); err == nil {
		t.Fatal("ImportMarkdown() error = nil, want parity/rebuild failure")
	}
	var gotMessage string
	if err := store.db.QueryRowContext(ctx, `SELECT message FROM journal_entries WHERE id = ?`, journalID).Scan(&gotMessage); err != nil {
		t.Fatalf("read canonical journal row after rollback: %v", err)
	}
	if gotMessage != message {
		t.Fatalf("canonical message after rollback = %q, want %q", gotMessage, message)
	}
	var derivedCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = ? AND message = ?`, journalID, message).Scan(&derivedCount); err != nil {
		t.Fatalf("read derived journal row after rollback: %v", err)
	}
	if derivedCount != 1 {
		t.Fatalf("derived rows after rollback = %d, want one preserved row", derivedCount)
	}
}

func TestMarkdownRollbackBackupRemovesAndRestoresEphemeralSources(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	taskBody := "# Task body\n\nFirst paragraph.\n"
	writeMarkdownImportFixture(t, root.Path(), taskBody)
	writeAgentsFile(t, root.Path(), "drafts/20260528-research-note.md", "# Research Note\n")
	writeAgentsFile(t, root.Path(), "tasks/archive/TASK-999-archived.md", "# Archived Task\n")
	writeAgentsFile(t, root.Path(), "ideas/archive/.gitkeep", "")

	migration, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	backup, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	rollbackBackup, err := CreateMarkdownRollbackBackup(context.Background(), root, backup.BackupPath)
	if err != nil {
		t.Fatalf("CreateMarkdownRollbackBackup() error = %v", err)
	}

	removed, err := RemoveMarkdownMigrationSources(context.Background(), root, PathResolver{StateHome: stateHome}, rollbackBackup.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RemoveMarkdownMigrationSources() error = %v", err)
	}
	wantRemoved := []string{
		".agents/TASKS.json",
		".agents/drafts/20260528-brainstorm-topic.md",
		".agents/drafts/20260528-research-note.md",
		".agents/ideas/20260528-idea.md",
		".agents/ideas/archive/.gitkeep",
		".agents/sessions/20260528-session.md",
		".agents/tasks/TASK-001-example.md",
		".agents/tasks/archive/TASK-999-archived.md",
	}
	if !reflect.DeepEqual(removed, wantRemoved) {
		t.Fatalf("removed = %#v, want %#v", removed, wantRemoved)
	}
	for _, rel := range removed {
		if _, err := os.Stat(filepath.Join(root.Path(), filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after remove, err=%v", rel, err)
		}
	}
	for _, rel := range []string{
		".agents/specs/SPEC-001-example.md",
		".agents/reports/report.md",
	} {
		if _, err := os.Stat(filepath.Join(root.Path(), filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s should remain after remove: %v", rel, err)
		}
	}

	rollback, err := RollbackMarkdownMigration(context.Background(), root, rollbackBackup.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RollbackMarkdownMigration() error = %v", err)
	}
	if !rollback.Restored {
		t.Fatal("Restored = false, want true")
	}
	if rollback.Action != MarkdownMigrationActionRollback {
		t.Fatalf("Action = %q, want %q", rollback.Action, MarkdownMigrationActionRollback)
	}
	taskPath := filepath.Join(root.Path(), ".agents", "tasks", "TASK-001-example.md")
	content, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("ReadFile(task) after rollback error = %v", err)
	}
	if string(content) != taskBody {
		t.Fatalf("restored task body = %q, want byte-exact original", string(content))
	}
	archivedTaskPath := filepath.Join(root.Path(), ".agents", "tasks", "archive", "TASK-999-archived.md")
	archivedContent, err := os.ReadFile(archivedTaskPath)
	if err != nil {
		t.Fatalf("ReadFile(archived task) after rollback error = %v", err)
	}
	if string(archivedContent) != "# Archived Task\n" {
		t.Fatalf("restored archived task body = %q, want byte-exact original", string(archivedContent))
	}
	if migration.DatabasePath != backup.DatabasePath {
		t.Fatalf("backup database path = %q, want migration database path %q", backup.DatabasePath, migration.DatabasePath)
	}
	if rollback.StateBackupPath != backup.BackupPath {
		t.Fatalf("rollback StateBackupPath = %q, want %q", rollback.StateBackupPath, backup.BackupPath)
	}
}

func TestMarkdownRollbackRemoveRequiresIntactBackup(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-example.md", "# Task\n")

	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	backup, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	rollbackBackup, err := CreateMarkdownRollbackBackup(context.Background(), root, backup.BackupPath)
	if err != nil {
		t.Fatalf("CreateMarkdownRollbackBackup() error = %v", err)
	}
	manifest, err := readMarkdownRollbackManifest(rollbackBackup.RollbackManifestPath)
	if err != nil {
		t.Fatalf("readMarkdownRollbackManifest() error = %v", err)
	}
	for _, file := range manifest.Files {
		if file.Path == ".agents/tasks/TASK-001-example.md" {
			if err := os.WriteFile(file.BackupPath, []byte("corrupt\n"), 0o600); err != nil {
				t.Fatalf("corrupt rollback backup file error = %v", err)
			}
			break
		}
	}

	_, err = RemoveMarkdownMigrationSources(context.Background(), root, PathResolver{StateHome: stateHome}, rollbackBackup.RollbackManifestPath)
	if err == nil {
		t.Fatal("RemoveMarkdownMigrationSources() error = nil, want checksum rejection")
	}
	if !strings.Contains(err.Error(), "checksum mismatch before removal") {
		t.Fatalf("error = %v, want checksum mismatch before removal", err)
	}
	if _, err := os.Stat(filepath.Join(root.Path(), ".agents", "tasks", "TASK-001-example.md")); err != nil {
		t.Fatalf("task source should remain after failed removal: %v", err)
	}
}

func TestMarkdownRollbackRemoveAbortsAtomicallyOnLaterMismatch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	taskBody := "# Task body\n\nFirst paragraph.\n"
	// Multiple ephemeral sources spanning several roots so the corrupted entry
	// is *not* the first one removed in manifest (sorted) order.
	writeMarkdownImportFixture(t, root.Path(), taskBody)
	writeAgentsFile(t, root.Path(), "drafts/20260528-research-note.md", "# Research Note\n")
	writeAgentsFile(t, root.Path(), "tasks/archive/TASK-999-archived.md", "# Archived Task\n")
	writeAgentsFile(t, root.Path(), "ideas/archive/.gitkeep", "")

	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	backup, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	rollbackBackup, err := CreateMarkdownRollbackBackup(context.Background(), root, backup.BackupPath)
	if err != nil {
		t.Fatalf("CreateMarkdownRollbackBackup() error = %v", err)
	}
	manifest, err := readMarkdownRollbackManifest(rollbackBackup.RollbackManifestPath)
	if err != nil {
		t.Fatalf("readMarkdownRollbackManifest() error = %v", err)
	}

	// Collect every ephemeral source in the order RemoveMarkdownMigrationSources
	// would delete them, then corrupt the backup of the *last* one. With a
	// non-atomic implementation the earlier files would already be deleted by
	// the time this mismatch is hit.
	var ephemeral []string
	for _, file := range manifest.Files {
		if isEphemeralMarkdownMigrationSource(file.Path) {
			ephemeral = append(ephemeral, file.Path)
		}
	}
	if len(ephemeral) < 2 {
		t.Fatalf("expected multiple ephemeral sources, got %#v", ephemeral)
	}
	lastPath := ephemeral[len(ephemeral)-1]
	corrupted := false
	for _, file := range manifest.Files {
		if file.Path == lastPath {
			if err := os.WriteFile(file.BackupPath, []byte("corrupt\n"), 0o600); err != nil {
				t.Fatalf("corrupt rollback backup file error = %v", err)
			}
			corrupted = true
			break
		}
	}
	if !corrupted {
		t.Fatalf("failed to locate backup for %q", lastPath)
	}

	_, err = RemoveMarkdownMigrationSources(context.Background(), root, PathResolver{StateHome: stateHome}, rollbackBackup.RollbackManifestPath)
	if err == nil {
		t.Fatal("RemoveMarkdownMigrationSources() error = nil, want atomic checksum rejection")
	}
	if !strings.Contains(err.Error(), "checksum mismatch before removal") {
		t.Fatalf("error = %v, want checksum mismatch before removal", err)
	}
	if !strings.Contains(err.Error(), lastPath) {
		t.Fatalf("error = %v, want reference to corrupted file %q", err, lastPath)
	}

	// Atomicity: no ephemeral source may be deleted when the abort fires,
	// including the ones that would have been processed before the mismatch.
	for _, rel := range ephemeral {
		if _, err := os.Stat(filepath.Join(root.Path(), filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s was deleted despite atomic abort: %v", rel, err)
		}
	}
}

func TestFrontmatterListItemsPreserveCommas(t *testing.T) {
	frontmatter := parseFrontmatterMap([]byte(`---
implements:
  - .agents/specs/SPEC-000-target, with comma.md
  - SPEC-001
---
# Source Spec
`))

	got := splitFrontmatterList(frontmatter["implements"])
	want := []string{".agents/specs/SPEC-000-target, with comma.md", "SPEC-001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitFrontmatterList() = %#v, want %#v", got, want)
	}
}

func writeMarkdownImportFixture(t *testing.T, root string, taskBody string) {
	t.Helper()
	writeAgentsFile(t, root, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeAgentsFile(t, root, "tasks/TASK-001-example.md", taskBody)
	writeAgentsFile(t, root, "ideas/20260528-idea.md", "# Example Idea\n")
	writeAgentsFile(t, root, "drafts/20260528-brainstorm-topic.md", "# Example Brainstorm\n")
	writeAgentsFile(t, root, "sessions/20260528-session.md", `---
branch: feature/example
status: active
---
[2026-05-28 10:00] spark(scope): capture this
`)
	writeAgentsFile(t, root, "reports/report.md", `---
kind: session
title: Example Report
status: final
---
# Example Report
`)
	writeAgentsFile(t, root, "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "Example Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1",
      "depends_on": ["TASK-000"]
    }
  }
}
`)
}

func assertTableCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRowContext(context.Background(), fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&got); err != nil {
		t.Fatalf("count %s error = %v", table, err)
	}
	if got != want {
		t.Fatalf("count %s = %d, want %d", table, got, want)
	}
}
