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

// durableSourceBodyForm extracts the stored-body form of a legacy source file.
// A finalized durable render parses cleanly, so its frontmatter and render
// stamp (both machine-regenerated by finalize) are dropped losslessly. Anything
// else keeps everything after frontmatter — the conservative, lossless form.
// Accepted edge: a never-finalized file that happens to parse as a valid
// durable render has its stamp consumed on import.
func durableSourceBodyForm(content []byte) string {
	if doc, err := ParseDurableRender(string(content)); err == nil {
		return doc.Body
	}
	return markdownArtifactBodyContent(content)
}

// editBodyForm normalizes the incoming body of a sanctioned edit. Blocking-hook
// remediations name the committed render itself as the `--body-file` path, so
// when the whole input parses as a valid durable render the stored body is the
// parsed document's Body — frontmatter and render stamp are machine-regenerated
// by finalize, making the strip lossless. Anything else is stored verbatim:
// unlike durableSourceBodyForm, this never strips arbitrary leading frontmatter
// from input that is not a complete durable render (e.g. prose that starts with
// "---" but carries no render stamp).
func editBodyForm(body string) string {
	if doc, err := ParseDurableRender(body); err == nil {
		return doc.Body
	}
	return body
}

// sourceBodiesDiverge compares bodies in the renderer's normalized form: the
// renderer normalizes CRLF and trims edge whitespace, so a raw byte comparison
// would report false divergence for round-tripped content.
func sourceBodiesDiverge(fileForm string, stored string) bool {
	return strings.TrimSpace(normalizeLineEndings(fileForm)) != strings.TrimSpace(normalizeLineEndings(stored))
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
	path, err := validatedSourceFilePath(rootPath, relPath)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return markdownArtifactBodyContent(content), nil
}

func validatedSourceFilePath(rootPath string, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("source path %q is absolute", relPath)
	}
	clean := filepath.Clean(filepath.FromSlash(relPath))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("source path %q escapes project root", relPath)
	}
	return filepath.Join(rootPath, clean), nil
}

// artifactBodyEditOutcome reports what a sanctioned body edit did.
type artifactBodyEditOutcome struct {
	Imported    bool
	ContentHash string
	EventID     string
}

// editArtifactBody applies a sanctioned body edit: it normalizes the incoming
// body via editBodyForm (so passing a committed durable render stores only its
// prose), on first edit of a body that lives only in a legacy source file it
// imports that body into SQLite, it refuses when the legacy file and the
// stored body diverge (unless forced), then upserts the new body, touches the
// entity row, and records a body_edited event. Native artifacts carry no
// legacy source path, so renderPath names the canonical durable render as the
// divergence-guard fallback: a hand-edited or hand-mangled render refuses the
// same way a diverged legacy source does. It mutates SQLite only; finalize
// rewrites the durable render.
func (s *Store) editArtifactBody(ctx context.Context, rootPath string, projectID string, entityKind string, entityTable string, entityID string, display string, bodySourceID sql.NullString, sourcePath sql.NullString, renderPath string, body string, force bool) (artifactBodyEditOutcome, error) {
	body = editBodyForm(body)
	row, hasRow, err := s.ReadArtifactBody(ctx, projectID, entityKind, entityID, ArtifactBodyKindMarkdown)
	if err != nil {
		return artifactBodyEditOutcome{}, err
	}

	fileForm := ""
	fileExists := false
	relPath := ""
	if sourcePath.Valid && strings.TrimSpace(sourcePath.String) != "" {
		relPath = filepath.ToSlash(strings.TrimSpace(sourcePath.String))
		path, err := validatedSourceFilePath(rootPath, relPath)
		if err != nil {
			return artifactBodyEditOutcome{}, err
		}
		raw, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return artifactBodyEditOutcome{}, fmt.Errorf("read %s source %s: %w", entityKind, relPath, err)
		}
		if err == nil {
			fileExists = true
			fileForm = durableSourceBodyForm(raw)
		}
	}

	if hasRow && fileExists && sourceBodiesDiverge(fileForm, row.Content) && !force {
		return artifactBodyEditOutcome{}, fmt.Errorf("legacy source %s no longer matches the SQLite body for %s; refusing so neither side is silently lost: run `loaf %s finalize %s` to rewrite the file from SQLite, or re-run with --force to edit the SQLite body anyway (the next finalize overwrites the file)", relPath, display, entityKind, display)
	}

	if relPath == "" && strings.TrimSpace(renderPath) != "" {
		renderRel := filepath.ToSlash(strings.TrimSpace(renderPath))
		path, err := validatedSourceFilePath(rootPath, renderRel)
		if err != nil {
			return artifactBodyEditOutcome{}, err
		}
		raw, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return artifactBodyEditOutcome{}, fmt.Errorf("read %s render %s: %w", entityKind, renderRel, err)
		}
		if err == nil && !force {
			// A render that no longer parses was hand-mangled, so it must trip
			// the guard exactly like a parseable render with divergent prose.
			doc, parseErr := ParseDurableRender(string(raw))
			if parseErr != nil || sourceBodiesDiverge(doc.Body, row.Content) {
				return artifactBodyEditOutcome{}, fmt.Errorf("durable render %s no longer matches the SQLite body for %s; refusing so neither side is silently lost: run `loaf %s finalize %s` to rewrite the render from SQLite, or re-run with --force to edit the SQLite body anyway (the next finalize overwrites the render)", renderRel, display, entityKind, display)
			}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return artifactBodyEditOutcome{}, fmt.Errorf("begin %s edit transaction: %w", entityKind, err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	sourceRef := emptyToNil(stringFromAny(bodySourceID))
	outcome := artifactBodyEditOutcome{}
	if !hasRow && fileExists && fileForm != "" {
		if _, err := upsertArtifactBodyTx(ctx, tx, projectID, entityKind, entityID, ArtifactBodyKindMarkdown, fileForm, sourceRef, now); err != nil {
			return artifactBodyEditOutcome{}, err
		}
		importEventID := stableMigrationID("event", projectID, entityKind, entityID, "body_imported", artifactBodyHash(fileForm))
		if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, 'body_imported', NULL, NULL, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, importEventID, projectID, entityKind, entityID, "recorded by "+entityKind+" edit; imported legacy source", now, now); err != nil {
			return artifactBodyEditOutcome{}, fmt.Errorf("record %s body import event: %w", entityKind, err)
		}
		outcome.Imported = true
	}

	if _, err := upsertArtifactBodyTx(ctx, tx, projectID, entityKind, entityID, ArtifactBodyKindMarkdown, body, sourceRef, now); err != nil {
		return artifactBodyEditOutcome{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE `+entityTable+` SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, entityID); err != nil {
		return artifactBodyEditOutcome{}, fmt.Errorf("touch %s %s: %w", entityKind, entityID, err)
	}
	editEventID := stableMigrationID("event", projectID, entityKind, entityID, "body_edited", artifactBodyHash(body))
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, 'body_edited', NULL, NULL, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, editEventID, projectID, entityKind, entityID, "recorded by "+entityKind+" edit", now, now); err != nil {
		return artifactBodyEditOutcome{}, fmt.Errorf("record %s body edit event: %w", entityKind, err)
	}

	if err := tx.Commit(); err != nil {
		return artifactBodyEditOutcome{}, fmt.Errorf("commit %s edit transaction: %w", entityKind, err)
	}
	outcome.ContentHash = artifactBodyHash(body)
	outcome.EventID = editEventID
	return outcome, nil
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
