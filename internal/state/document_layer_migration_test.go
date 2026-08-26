package state

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentLayerMigrationExportsThenDropsTables(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	migrations := SchemaMigrations()
	pre := migrations[:len(migrations)-1]
	if pre[len(pre)-1].Version != 20 || migrations[len(migrations)-1].Version != 21 {
		t.Fatalf("unexpected migration tail: last pre=%d final=%d", pre[len(pre)-1].Version, migrations[len(migrations)-1].Version)
	}
	if err := ApplyMigrations(ctx, db, pre); err != nil {
		t.Fatalf("ApplyMigrations(pre-0021) error = %v", err)
	}

	now := "2026-08-26T00:00:00Z"
	projectID := "proj_document_layer_export_01"
	if _, err := db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, created_at, updated_at, friendly_name, current_path)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "identity-document-layer", now, now, "Export Project", root.Path()); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, "path-document-layer", projectID, root.Path(), now, now, now, now); err != nil {
		t.Fatalf("insert project_paths: %v", err)
	}

	seed := []struct {
		kind, table, id, title, status, alias, body string
		extraCols                                   string
		extraVals                                   []any
	}{
		{"report", "reports", "report:export-1", "Exported Report", "final", "exported-report", "# Exported Report Body\n", ", report_kind", []any{"audit"}},
		{"council", "councils", "council:export-1", "Exported Council", "draft", "exported-council", "# Exported Council Body\n", "", nil},
		{"shaping_draft", "shaping_drafts", "shaping_draft:export-1", "Exported Draft", "finalized", "exported-draft", "# Exported Draft Body\n", "", nil},
	}
	for _, row := range seed {
		cols := "id, project_id, title, status, body_source_id, created_at, updated_at"
		placeholders := "?, ?, ?, ?, NULL, ?, ?"
		args := []any{row.id, projectID, row.title, row.status, now, now}
		if row.extraCols != "" {
			cols = "id, project_id, report_kind, title, status, body_source_id, created_at, updated_at"
			placeholders = "?, ?, ?, ?, ?, NULL, ?, ?"
			args = []any{row.id, projectID, row.extraVals[0], row.title, row.status, now, now}
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO "+row.table+" ("+cols+") VALUES ("+placeholders+")", args...); err != nil {
			t.Fatalf("insert %s: %v", row.table, err)
		}
		bodyID := "body-" + row.kind
		if _, err := db.ExecContext(ctx, `
INSERT INTO artifact_bodies (id, project_id, entity_kind, entity_id, body_kind, content, content_hash, source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, 'markdown', ?, 'hash', NULL, ?, ?)
`, bodyID, projectID, row.kind, row.id, row.body, now, now); err != nil {
			t.Fatalf("insert body %s: %v", row.kind, err)
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, "alias-"+row.kind, projectID, row.kind, row.id, row.kind, row.alias, now, now); err != nil {
			t.Fatalf("insert alias %s: %v", row.kind, err)
		}
	}

	if err := ApplyMigrations(ctx, db, migrations); err != nil {
		t.Fatalf("ApplyMigrations(0021) error = %v", err)
	}
	if err := validateGoneTablesAbsent(ctx, db); err != nil {
		t.Fatalf("tables still present after 0021: %v", err)
	}

	wantFiles := map[string]string{
		filepath.Join(root.Path(), ".agents", "reports", "exported-report.md"):   "Exported Report",
		filepath.Join(root.Path(), ".agents", "councils", "exported-council.md"): "Exported Council",
		filepath.Join(root.Path(), ".agents", "drafts", "exported-draft.md"):     "Exported Draft",
	}
	for path, needle := range wantFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if !strings.Contains(string(content), needle) {
			t.Fatalf("export %s missing %q; got %q", path, needle, string(content))
		}
	}
}

func TestDocumentLayerMigrationRefusesMissingProjectPath(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	migrations := SchemaMigrations()
	pre := migrations[:len(migrations)-1]
	if err := ApplyMigrations(ctx, db, pre); err != nil {
		t.Fatalf("ApplyMigrations(pre-0021) error = %v", err)
	}
	now := "2026-08-26T00:00:00Z"
	projectID := "proj_document_layer_nopath"
	if _, err := db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, created_at, updated_at, friendly_name, current_path)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "identity-nopath", now, now, "No Path", ""); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, 'audit', 'Orphan Report', 'draft', NULL, ?, ?)
`, "report:orphan", projectID, now, now); err != nil {
		t.Fatalf("insert report: %v", err)
	}
	err = ApplyMigrations(ctx, db, migrations)
	if err == nil {
		t.Fatal("ApplyMigrations(0021) error = nil, want missing-path refusal")
	}
	if !strings.Contains(err.Error(), "no current path") {
		t.Fatalf("ApplyMigrations(0021) error = %v, want no current path", err)
	}
}

func TestDocumentLayerMigrationRefusesExistingDest(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := sql.Open(sqliteDriverName, dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	migrations := SchemaMigrations()
	pre := migrations[:len(migrations)-1]
	if err := ApplyMigrations(ctx, db, pre); err != nil {
		t.Fatalf("ApplyMigrations(pre-0021) error = %v", err)
	}
	now := "2026-08-26T00:00:00Z"
	projectID := "proj_document_layer_conflict"
	if _, err := db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, created_at, updated_at, friendly_name, current_path)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "identity-conflict", now, now, "Conflict", root.Path()); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, "path-conflict", projectID, root.Path(), now, now, now, now); err != nil {
		t.Fatalf("insert project_paths: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, 'audit', 'Conflict Report', 'draft', NULL, ?, ?)
`, "report:conflict", projectID, now, now); err != nil {
		t.Fatalf("insert report: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'report', ?, 'report', 'conflict-report', ?, ?)
`, "alias-conflict", projectID, "report:conflict", now, now); err != nil {
		t.Fatalf("insert alias: %v", err)
	}
	dest := filepath.Join(root.Path(), ".agents", "reports")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "conflict-report.md"), []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}
	err = ApplyMigrations(ctx, db, migrations)
	if err == nil {
		t.Fatal("ApplyMigrations(0021) error = nil, want existing-dest refusal")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("ApplyMigrations(0021) error = %v, want already exists", err)
	}
}
