package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// ProjectDeleteResult describes a destructive project removal from the global database.
type ProjectDeleteResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Removed            []DeleteCount `json:"removed"`
}

// projectScopedDeleteTables lists every table that carries a project_id and must
// be cleared when a project is removed. artifact_bodies and docs_index are handled
// separately because they back FTS5 indexes.
var projectScopedDeleteTables = []string{
	"verdicts",
	"findings",
	"runs",
	"journal_entries",
	"session_state_snapshots",
	"handoffs",
	"councils",
	"plans",
	"bundle_members",
	"bundles",
	"entity_tags",
	"tags",
	"relationships",
	"events",
	"exports",
	"backend_mappings",
	"hook_events",
	"tasks",
	"specs",
	"ideas",
	"sparks",
	"brainstorms",
	"shaping_drafts",
	"reports",
	"sessions",
	"aliases",
	"sources",
	"project_paths",
}

// DeleteProject removes a project and every dependent row from the global database.
func DeleteProject(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ProjectDeleteResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ProjectDeleteResult{}, err
	}
	defer store.Close()
	return store.DeleteProject(ctx, ref)
}

// DeleteProject removes a project and every dependent row using an open store.
func (s *Store) DeleteProject(ctx context.Context, ref string) (ProjectDeleteResult, error) {
	projectID, err := s.resolveProjectRef(ctx, ref)
	if err != nil {
		return ProjectDeleteResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ProjectDeleteResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectDeleteResult{}, fmt.Errorf("begin project delete transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return ProjectDeleteResult{}, fmt.Errorf("defer foreign keys: %w", err)
	}

	removed := []DeleteCount{}

	artifactCount, err := deleteProjectArtifactBodiesTx(ctx, tx, projectID)
	if err != nil {
		return ProjectDeleteResult{}, err
	}
	removed = append(removed, DeleteCount{Table: "artifact_bodies", Rows: artifactCount})

	docsCount, err := deleteProjectDocsIndexTx(ctx, tx, projectID)
	if err != nil {
		return ProjectDeleteResult{}, err
	}
	removed = append(removed, DeleteCount{Table: "docs_index", Rows: docsCount})

	journalSearchCount, err := execCountTx(ctx, tx, `DELETE FROM journal_search WHERE project_id = ?`, projectID)
	if err != nil {
		return ProjectDeleteResult{}, fmt.Errorf("delete journal_search rows: %w", err)
	}
	removed = append(removed, DeleteCount{Table: "journal_search", Rows: journalSearchCount})

	for _, table := range projectScopedDeleteTables {
		count, err := execCountTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s WHERE project_id = ?`, quoteSQLiteIdentifier(table)), projectID)
		if err != nil {
			return ProjectDeleteResult{}, fmt.Errorf("delete %s rows for project: %w", table, err)
		}
		removed = append(removed, DeleteCount{Table: table, Rows: count})
	}

	projectCount, err := execCountTx(ctx, tx, `DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return ProjectDeleteResult{}, fmt.Errorf("delete project row: %w", err)
	}
	removed = append(removed, DeleteCount{Table: "projects", Rows: projectCount})

	if err := tx.Commit(); err != nil {
		return ProjectDeleteResult{}, fmt.Errorf("commit project delete transaction: %w", err)
	}

	return ProjectDeleteResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       s.path,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Removed:            removed,
	}, nil
}

// resolveProjectRef matches a project by exact ID, else by unique friendly name,
// else by current path.
func (s *Store) resolveProjectRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("project delete requires a project id")
	}

	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM projects WHERE id = ?`, ref).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("resolve project by id: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT projects.id
FROM projects
LEFT JOIN project_paths AS current_path
  ON current_path.project_id = projects.id AND current_path.is_current = 1
WHERE projects.friendly_name = ?
   OR COALESCE(current_path.path, projects.current_path, '') = ?
`, ref, ref)
	if err != nil {
		return "", fmt.Errorf("resolve project by name or path: %w", err)
	}
	defer rows.Close()
	var matches []string
	for rows.Next() {
		var match string
		if err := rows.Scan(&match); err != nil {
			return "", fmt.Errorf("scan project match: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate project matches: %w", err)
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("project %q not found; run `loaf project list` to see registered projects", ref)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("project %q is ambiguous; pass the exact project id from `loaf project list`", ref)
	}
}

func deleteProjectArtifactBodiesTx(ctx context.Context, tx *sql.Tx, projectID string) (int, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT rowid, entity_kind, entity_id, body_kind, content
FROM artifact_bodies
WHERE project_id = ?
`, projectID)
	if err != nil {
		return 0, fmt.Errorf("read artifact bodies for project: %w", err)
	}
	var searchRows []artifactSearchRow
	for rows.Next() {
		row := artifactSearchRow{ProjectID: projectID}
		if err := rows.Scan(&row.RowID, &row.EntityKind, &row.EntityID, &row.BodyKind, &row.Content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan artifact body for project: %w", err)
		}
		searchRows = append(searchRows, row)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close artifact body rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate artifact body rows: %w", err)
	}
	for _, row := range searchRows {
		if err := deleteArtifactSearchTx(ctx, tx, row); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM artifact_bodies WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("delete artifact bodies for project: %w", err)
	}
	return len(searchRows), nil
}

func deleteProjectDocsIndexTx(ctx context.Context, tx *sql.Tx, projectID string) (int, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT rowid, id, path, content
FROM docs_index
WHERE project_id = ?
`, projectID)
	if err != nil {
		return 0, fmt.Errorf("read docs index for project: %w", err)
	}
	var docRows []docsSearchRow
	for rows.Next() {
		row := docsSearchRow{ProjectID: projectID}
		if err := rows.Scan(&row.RowID, &row.DocID, &row.Path, &row.Content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan docs index for project: %w", err)
		}
		docRows = append(docRows, row)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close docs index rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate docs index rows: %w", err)
	}
	for _, row := range docRows {
		if err := deleteDocsSearchRow(ctx, tx, row); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM docs_index WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("delete docs index for project: %w", err)
	}
	return len(docRows), nil
}
