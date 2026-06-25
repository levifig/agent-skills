package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ArtifactBodyKindMarkdown = "markdown"

// ArtifactBody is the SQLite-resident body for a Loaf artifact.
type ArtifactBody struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	EntityKind  string `json:"entity_kind"`
	EntityID    string `json:"entity_id"`
	BodyKind    string `json:"body_kind"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	SourceID    string `json:"source_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// UpsertArtifactBody writes a body and its FTS row in one transaction.
func (s *Store) UpsertArtifactBody(ctx context.Context, projectID string, entityKind string, entityID string, bodyKind string, content string, sourceID string) (ArtifactBody, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ArtifactBody{}, fmt.Errorf("begin artifact body transaction: %w", err)
	}
	defer tx.Rollback()

	body, err := upsertArtifactBodyTx(ctx, tx, projectID, entityKind, entityID, bodyKind, content, emptyToNil(sourceID), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return ArtifactBody{}, err
	}
	if err := tx.Commit(); err != nil {
		return ArtifactBody{}, fmt.Errorf("commit artifact body transaction: %w", err)
	}
	return body, nil
}

// ReadArtifactBody returns a body row by logical artifact address.
func (s *Store) ReadArtifactBody(ctx context.Context, projectID string, entityKind string, entityID string, bodyKind string) (ArtifactBody, bool, error) {
	bodyKind = firstNonEmpty(strings.TrimSpace(bodyKind), ArtifactBodyKindMarkdown)
	row := s.db.QueryRowContext(ctx, `
SELECT id, project_id, entity_kind, entity_id, body_kind, content, content_hash, COALESCE(source_id, ''), created_at, updated_at
FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND body_kind = ?
`, projectID, entityKind, entityID, bodyKind)
	body, err := scanArtifactBody(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtifactBody{}, false, nil
	}
	if err != nil {
		return ArtifactBody{}, false, fmt.Errorf("read artifact body %s/%s/%s: %w", entityKind, entityID, bodyKind, err)
	}
	return body, true, nil
}

// DeleteArtifactBody deletes a body and its FTS row in one transaction.
func (s *Store) DeleteArtifactBody(ctx context.Context, projectID string, entityKind string, entityID string, bodyKind string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin artifact body delete transaction: %w", err)
	}
	defer tx.Rollback()

	if err := deleteArtifactBodyTx(ctx, tx, projectID, entityKind, entityID, bodyKind); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit artifact body delete transaction: %w", err)
	}
	return nil
}

func (s *Store) artifactBodyOrSourceBody(ctx context.Context, rootPath string, projectID string, entityKind string, entityID string, sourcePath sql.NullString) (string, error) {
	body, ok, err := s.ReadArtifactBody(ctx, projectID, entityKind, entityID, ArtifactBodyKindMarkdown)
	if err != nil {
		return "", err
	}
	if ok {
		return body.Content, nil
	}
	if sourcePath.Valid && sourcePath.String != "" {
		content, err := readImportedSourceBody(rootPath, filepath.ToSlash(sourcePath.String))
		if err == nil {
			return content, nil
		}
	}
	return "", nil
}

func upsertArtifactBodyTx(ctx context.Context, tx *sql.Tx, projectID string, entityKind string, entityID string, bodyKind string, content string, sourceID any, now string) (ArtifactBody, error) {
	entityKind = strings.TrimSpace(entityKind)
	entityID = strings.TrimSpace(entityID)
	bodyKind = firstNonEmpty(strings.TrimSpace(bodyKind), ArtifactBodyKindMarkdown)
	if projectID == "" || entityKind == "" || entityID == "" {
		return ArtifactBody{}, fmt.Errorf("artifact body requires project_id, entity_kind, and entity_id")
	}

	oldIndexed, oldOK, err := readArtifactSearchRowTx(ctx, tx, projectID, entityKind, entityID, bodyKind)
	if err != nil {
		return ArtifactBody{}, err
	}

	id := stableMigrationID("artifact_body", projectID, entityKind, entityID, bodyKind)
	hash := artifactBodyHash(content)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO artifact_bodies (id, project_id, entity_kind, entity_id, body_kind, content, content_hash, source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, entity_kind, entity_id, body_kind) DO UPDATE SET
  content = excluded.content,
  content_hash = excluded.content_hash,
  source_id = excluded.source_id,
  updated_at = excluded.updated_at
`, id, projectID, entityKind, entityID, bodyKind, content, hash, sourceID, now, now); err != nil {
		return ArtifactBody{}, fmt.Errorf("upsert artifact body %s/%s/%s: %w", entityKind, entityID, bodyKind, err)
	}

	rowID, err := artifactBodyRowID(ctx, tx, projectID, entityKind, entityID, bodyKind)
	if err != nil {
		return ArtifactBody{}, err
	}
	if err := upsertArtifactSearchTx(ctx, tx, oldIndexed, oldOK, rowID, projectID, entityKind, entityID, bodyKind, content); err != nil {
		return ArtifactBody{}, err
	}
	return ArtifactBody{
		ID:          id,
		ProjectID:   projectID,
		EntityKind:  entityKind,
		EntityID:    entityID,
		BodyKind:    bodyKind,
		Content:     content,
		ContentHash: hash,
		SourceID:    stringFromAny(sourceID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func deleteArtifactBodyTx(ctx context.Context, tx *sql.Tx, projectID string, entityKind string, entityID string, bodyKind string) error {
	bodyKind = firstNonEmpty(strings.TrimSpace(bodyKind), ArtifactBodyKindMarkdown)
	oldIndexed, ok, err := readArtifactSearchRowTx(ctx, tx, projectID, entityKind, entityID, bodyKind)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := deleteArtifactSearchTx(ctx, tx, oldIndexed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND body_kind = ?
`, projectID, entityKind, entityID, bodyKind); err != nil {
		return fmt.Errorf("delete artifact body %s/%s/%s: %w", entityKind, entityID, bodyKind, err)
	}
	return nil
}

type artifactSearchRow struct {
	RowID      int64
	ProjectID  string
	EntityKind string
	EntityID   string
	BodyKind   string
	Content    string
}

func readArtifactSearchRowTx(ctx context.Context, tx *sql.Tx, projectID string, entityKind string, entityID string, bodyKind string) (artifactSearchRow, bool, error) {
	var row artifactSearchRow
	err := tx.QueryRowContext(ctx, `
SELECT rowid, project_id, entity_kind, entity_id, body_kind, content
FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND body_kind = ?
`, projectID, entityKind, entityID, bodyKind).Scan(&row.RowID, &row.ProjectID, &row.EntityKind, &row.EntityID, &row.BodyKind, &row.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return artifactSearchRow{}, false, nil
	}
	if err != nil {
		return artifactSearchRow{}, false, fmt.Errorf("read old artifact search row %s/%s/%s: %w", entityKind, entityID, bodyKind, err)
	}
	return row, true, nil
}

func artifactBodyRowID(ctx context.Context, tx *sql.Tx, projectID string, entityKind string, entityID string, bodyKind string) (int64, error) {
	var rowID int64
	err := tx.QueryRowContext(ctx, `
SELECT rowid FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND body_kind = ?
`, projectID, entityKind, entityID, bodyKind).Scan(&rowID)
	if err != nil {
		return 0, fmt.Errorf("read artifact body rowid %s/%s/%s: %w", entityKind, entityID, bodyKind, err)
	}
	return rowID, nil
}

func upsertArtifactSearchTx(ctx context.Context, tx *sql.Tx, oldRow artifactSearchRow, oldOK bool, rowID int64, projectID string, entityKind string, entityID string, bodyKind string, content string) error {
	if oldOK {
		if err := deleteArtifactSearchTx(ctx, tx, oldRow); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO artifact_search(rowid, project_id, entity_kind, entity_id, body_kind, content)
VALUES (?, ?, ?, ?, ?, ?)
`, rowID, projectID, entityKind, entityID, bodyKind, content); err != nil {
		return fmt.Errorf("upsert artifact search row %d: %w", rowID, err)
	}
	return nil
}

func deleteArtifactSearchTx(ctx context.Context, tx *sql.Tx, row artifactSearchRow) error {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO artifact_search(artifact_search, rowid, project_id, entity_kind, entity_id, body_kind, content)
VALUES ('delete', ?, ?, ?, ?, ?, ?)
`, row.RowID, row.ProjectID, row.EntityKind, row.EntityID, row.BodyKind, row.Content); err != nil {
		return fmt.Errorf("delete artifact search row %d: %w", row.RowID, err)
	}
	return nil
}

func scanArtifactBody(row interface {
	Scan(dest ...any) error
}) (ArtifactBody, error) {
	var body ArtifactBody
	err := row.Scan(&body.ID, &body.ProjectID, &body.EntityKind, &body.EntityID, &body.BodyKind, &body.Content, &body.ContentHash, &body.SourceID, &body.CreatedAt, &body.UpdatedAt)
	return body, err
}

func artifactBodyHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func markdownArtifactBodyContent(content []byte) string {
	return strings.TrimSpace(markdownBody(content))
}

func markdownBody(content []byte) string {
	text := string(content)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return text
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return text
}

func readImportedSourceBody(rootPath string, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("source path %q is absolute", relPath)
	}
	clean := filepath.Clean(filepath.FromSlash(relPath))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("source path %q escapes project root", relPath)
	}
	content, err := os.ReadFile(filepath.Join(rootPath, clean))
	if err != nil {
		return "", err
	}
	return markdownArtifactBodyContent(content), nil
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case sql.NullString:
		if v.Valid {
			return v.String
		}
	}
	return ""
}
