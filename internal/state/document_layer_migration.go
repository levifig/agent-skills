package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const documentLayerMigrationVersion = 21

// exportDocumentLayerBeforeMigration writes report, council, and shaping_draft
// bodies to .agents/ before migration 0021 drops their SQLite tables.
func exportDocumentLayerBeforeMigration(ctx context.Context, db *sql.Tx) error {
	exists, err := documentLayerTableExists(ctx, db, "reports")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var count int
	if err := db.QueryRowContext(ctx, `
SELECT (
  (SELECT COUNT(*) FROM reports) +
  (SELECT COUNT(*) FROM councils) +
  (SELECT COUNT(*) FROM shaping_drafts)
)`).Scan(&count); err != nil {
		return fmt.Errorf("count document-layer rows: %w", err)
	}
	if count == 0 {
		return nil
	}

	rows, err := db.QueryContext(ctx, `
SELECT project_id, COUNT(*) AS n FROM (
  SELECT project_id FROM reports
  UNION ALL
  SELECT project_id FROM councils
  UNION ALL
  SELECT project_id FROM shaping_drafts
) GROUP BY project_id
`)
	if err != nil {
		return fmt.Errorf("list projects with document-layer rows: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var projectID string
		var n int
		if err := rows.Scan(&projectID, &n); err != nil {
			return fmt.Errorf("scan document-layer project count: %w", err)
		}
		rootPath, err := documentLayerProjectPath(ctx, db, projectID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(rootPath) == "" {
			return fmt.Errorf("document-layer demotion refused: project %s has %d row(s) but no current path; cannot export before DROP", projectID, n)
		}
		if _, err := os.Stat(rootPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("document-layer demotion refused: project %s path %q does not exist on disk (%d row(s) would be dropped without export)", projectID, rootPath, n)
			}
			return fmt.Errorf("document-layer demotion refused: stat project %s path %q: %w", projectID, rootPath, err)
		}
		if err := exportProjectDocumentLayer(ctx, db, projectID, rootPath); err != nil {
			return err
		}
	}
	return rows.Err()
}

func documentLayerProjectPath(ctx context.Context, db *sql.Tx, projectID string) (string, error) {
	var rootPath string
	err := db.QueryRowContext(ctx, `
SELECT COALESCE(pp.path, p.current_path, '')
FROM projects AS p
LEFT JOIN project_paths AS pp ON pp.project_id = p.id AND pp.is_current = 1
WHERE p.id = ?
`, projectID).Scan(&rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve project path for %s: %w", projectID, err)
	}
	return rootPath, nil
}

func exportProjectDocumentLayer(ctx context.Context, db *sql.Tx, projectID, rootPath string) error {
	if strings.TrimSpace(rootPath) == "" {
		return fmt.Errorf("document-layer demotion refused: empty project path for %s", projectID)
	}
	agents := filepath.Join(rootPath, ".agents")
	for _, sub := range []string{"reports", "councils", "drafts"} {
		if err := os.MkdirAll(filepath.Join(agents, sub), 0o755); err != nil {
			return fmt.Errorf("ensure %s dir: %w", sub, err)
		}
	}
	if err := exportDocumentLayerKind(ctx, db, projectID, agents, "report", "reports", "reports"); err != nil {
		return err
	}
	if err := exportDocumentLayerKind(ctx, db, projectID, agents, "council", "councils", "councils"); err != nil {
		return err
	}
	return exportDocumentLayerKind(ctx, db, projectID, agents, "shaping_draft", "shaping_drafts", "drafts")
}

func exportDocumentLayerKind(ctx context.Context, db *sql.Tx, projectID, agentsDir, entityKind, table, subdir string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
SELECT e.id, e.title, e.status, COALESCE(ab.content, ''), COALESCE(a.alias, e.id)
FROM %s AS e
LEFT JOIN artifact_bodies AS ab ON ab.project_id = e.project_id AND ab.entity_kind = ? AND ab.entity_id = e.id
LEFT JOIN aliases AS a ON a.project_id = e.project_id AND a.entity_kind = ? AND a.entity_id = e.id
WHERE e.project_id = ?
`, table), entityKind, entityKind, projectID)
	if err != nil {
		return fmt.Errorf("list %s for export: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, status, body, alias string
		if err := rows.Scan(&id, &title, &status, &body, &alias); err != nil {
			return fmt.Errorf("scan %s row: %w", table, err)
		}
		safeName := sanitizeDocumentLayerFilename(alias)
		if safeName == "" {
			safeName = sanitizeDocumentLayerFilename(id)
		}
		dest := filepath.Join(agentsDir, subdir, safeName+".md")
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("document-layer demotion refused: export target %s already exists for %s %s; resolve the conflict before applying migration 0021", dest, entityKind, id)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat export target %s: %w", dest, err)
		}
		content := renderExportedDocumentLayerMarkdown(title, status, alias, body)
		if err := os.WriteFile(dest, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return rows.Err()
}

func sanitizeDocumentLayerFilename(name string) string {
	name = strings.TrimSpace(name)
	for _, prefix := range []string{"report:", "council:", "shaping_draft:"} {
		name = strings.TrimPrefix(name, prefix)
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
}

func renderExportedDocumentLayerMarkdown(title, status, alias, body string) string {
	if strings.TrimSpace(body) == "" {
		body = "# " + title + "\n"
	}
	return fmt.Sprintf("---\nid: %s\nstatus: %s\ntitle: %q\n---\n\n%s", alias, status, title, strings.TrimSpace(body)+"\n")
}

func documentLayerTableExists(ctx context.Context, db *sql.Tx, table string) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	switch {
	case err == nil:
		return true, nil
	case err == sql.ErrNoRows:
		return false, nil
	default:
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
}
