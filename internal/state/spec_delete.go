package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

// DeleteCount records how many rows were removed (or unlinked) from a table.
type DeleteCount struct {
	Table string `json:"table"`
	Rows  int    `json:"rows"`
}

// SpecDeleteResult describes a destructive spec removal from SQLite state.
type SpecDeleteResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Spec               *TraceEntity  `json:"spec,omitempty"`
	Ref                string        `json:"ref,omitempty"`
	Removed            []DeleteCount `json:"removed"`
	Unlinked           []DeleteCount `json:"unlinked,omitempty"`
	RenderPath         string        `json:"render_path,omitempty"`
	RenderRetained     bool          `json:"render_retained,omitempty"`
}

// DeleteSpec removes a spec and every dependent row from initialized SQLite state.
func DeleteSpec(ctx context.Context, root project.Root, resolver PathResolver, ref string) (SpecDeleteResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SpecDeleteResult{}, err
	}
	defer store.Close()
	return store.DeleteSpec(ctx, root, ref)
}

// DeleteSpec removes a spec and every dependent row using an open store.
func (s *Store) DeleteSpec(ctx context.Context, root project.Root, ref string) (SpecDeleteResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SpecDeleteResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SpecDeleteResult{}, err
	}

	spec, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return SpecDeleteResult{}, err
	}
	if spec.Kind != "spec" {
		return SpecDeleteResult{}, fmt.Errorf("%q resolves to %s, not spec", ref, spec.Kind)
	}

	var bodySourceID sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT body_source_id FROM specs WHERE project_id = ? AND id = ?`, projectID, spec.ID).Scan(&bodySourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return SpecDeleteResult{}, fmt.Errorf("spec %q not found in SQLite state", ref)
	}
	if err != nil {
		return SpecDeleteResult{}, fmt.Errorf("read spec before delete: %w", err)
	}

	renderPath := s.specRenderPathOnDisk(ctx, root, projectID, spec)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SpecDeleteResult{}, fmt.Errorf("begin spec delete transaction: %w", err)
	}
	defer tx.Rollback()

	// Relax intra-transaction FK ordering; consistency is still enforced at COMMIT.
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return SpecDeleteResult{}, fmt.Errorf("defer foreign keys: %w", err)
	}

	removed := []DeleteCount{}
	unlinked := []DeleteCount{}

	bodyCount, bodySources, err := deleteArtifactBodiesForEntityTx(ctx, tx, projectID, "spec", spec.ID)
	if err != nil {
		return SpecDeleteResult{}, err
	}
	removed = append(removed, DeleteCount{Table: "artifact_bodies", Rows: bodyCount})

	polymorphic := []struct {
		table string
		query string
		args  []any
	}{
		{"aliases", `DELETE FROM aliases WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, spec.ID}},
		{"events", `DELETE FROM events WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, spec.ID}},
		{"entity_tags", `DELETE FROM entity_tags WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, spec.ID}},
		{"bundle_members", `DELETE FROM bundle_members WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, spec.ID}},
		{"backend_mappings", `DELETE FROM backend_mappings WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, spec.ID}},
		{"exports", `DELETE FROM exports WHERE project_id = ? AND source_entity_kind = 'spec' AND source_entity_id = ?`, []any{projectID, spec.ID}},
		{"relationships", `DELETE FROM relationships WHERE project_id = ? AND ((from_entity_kind = 'spec' AND from_entity_id = ?) OR (to_entity_kind = 'spec' AND to_entity_id = ?))`, []any{projectID, spec.ID, spec.ID}},
	}
	for _, op := range polymorphic {
		count, err := execCountTx(ctx, tx, op.query, op.args...)
		if err != nil {
			return SpecDeleteResult{}, fmt.Errorf("delete %s rows for spec: %w", op.table, err)
		}
		removed = append(removed, DeleteCount{Table: op.table, Rows: count})
	}

	// Preserve first-class entities that merely reference the spec by clearing the link.
	for _, table := range []string{"tasks", "journal_entries", "plans", "councils"} {
		count, err := execCountTx(ctx, tx, fmt.Sprintf(`UPDATE %s SET spec_id = NULL WHERE project_id = ? AND spec_id = ?`, quoteSQLiteIdentifier(table)), projectID, spec.ID)
		if err != nil {
			return SpecDeleteResult{}, fmt.Errorf("unlink %s from spec: %w", table, err)
		}
		if count > 0 {
			unlinked = append(unlinked, DeleteCount{Table: table, Rows: count})
		}
	}

	specCount, err := execCountTx(ctx, tx, `DELETE FROM specs WHERE project_id = ? AND id = ?`, projectID, spec.ID)
	if err != nil {
		return SpecDeleteResult{}, fmt.Errorf("delete spec row: %w", err)
	}
	removed = append(removed, DeleteCount{Table: "specs", Rows: specCount})

	candidateSources := map[string]struct{}{}
	if bodySourceID.Valid && bodySourceID.String != "" {
		candidateSources[bodySourceID.String] = struct{}{}
	}
	for _, sid := range bodySources {
		if sid != "" {
			candidateSources[sid] = struct{}{}
		}
	}
	sourcesRemoved := 0
	for sid := range candidateSources {
		referenced, err := sourceStillReferencedTx(ctx, tx, projectID, sid)
		if err != nil {
			return SpecDeleteResult{}, err
		}
		if referenced {
			continue
		}
		count, err := execCountTx(ctx, tx, `DELETE FROM sources WHERE project_id = ? AND id = ?`, projectID, sid)
		if err != nil {
			return SpecDeleteResult{}, fmt.Errorf("delete source row: %w", err)
		}
		sourcesRemoved += count
	}
	removed = append(removed, DeleteCount{Table: "sources", Rows: sourcesRemoved})

	if err := tx.Commit(); err != nil {
		return SpecDeleteResult{}, fmt.Errorf("commit spec delete transaction: %w", err)
	}

	return SpecDeleteResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Spec:               &spec,
		Ref:                ref,
		Removed:            removed,
		Unlinked:           unlinked,
		RenderPath:         renderPath,
		RenderRetained:     renderPath != "",
	}, nil
}

// specRenderPathOnDisk reports the on-disk finalized render for a spec, if one exists.
// Spec delete is DB-scoped and intentionally leaves this file in place for the user.
func (s *Store) specRenderPathOnDisk(ctx context.Context, root project.Root, projectID string, spec TraceEntity) string {
	candidates := []string{}
	var sourcePath sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT sources.path
FROM specs
JOIN sources ON sources.id = specs.body_source_id
WHERE specs.project_id = ? AND specs.id = ?
`, projectID, spec.ID).Scan(&sourcePath)
	if err == nil && sourcePath.Valid && sourcePath.String != "" {
		candidates = append(candidates, filepath.Join(root.Path(), filepath.FromSlash(sourcePath.String)))
	}
	alias := firstNonEmpty(spec.Alias, spec.ID)
	candidates = append(candidates, filepath.Join(root.Path(), ".agents", "specs", safeDurableRenderFilename(alias)+".md"))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// deleteArtifactBodiesForEntityTx removes all artifact bodies (and their FTS rows)
// for one entity, returning the count and the referenced source IDs.
func deleteArtifactBodiesForEntityTx(ctx context.Context, tx *sql.Tx, projectID string, entityKind string, entityID string) (int, []string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT body_kind, COALESCE(source_id, '')
FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
`, projectID, entityKind, entityID)
	if err != nil {
		return 0, nil, fmt.Errorf("read artifact bodies for %s %s: %w", entityKind, entityID, err)
	}
	type bodyRef struct {
		bodyKind string
		sourceID string
	}
	var bodies []bodyRef
	for rows.Next() {
		var b bodyRef
		if err := rows.Scan(&b.bodyKind, &b.sourceID); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("scan artifact body for %s %s: %w", entityKind, entityID, err)
		}
		bodies = append(bodies, b)
	}
	if err := rows.Close(); err != nil {
		return 0, nil, fmt.Errorf("close artifact body rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("iterate artifact body rows: %w", err)
	}

	sources := []string{}
	for _, b := range bodies {
		if err := deleteArtifactBodyTx(ctx, tx, projectID, entityKind, entityID, b.bodyKind); err != nil {
			return 0, nil, err
		}
		if b.sourceID != "" {
			sources = append(sources, b.sourceID)
		}
	}
	return len(bodies), sources, nil
}

// sourceStillReferencedTx reports whether any row still points at a source.
func sourceStillReferencedTx(ctx context.Context, tx *sql.Tx, projectID string, sourceID string) (bool, error) {
	refs := []struct {
		table  string
		column string
	}{
		{"specs", "body_source_id"},
		{"tasks", "body_source_id"},
		{"ideas", "body_source_id"},
		{"sparks", "source_id"},
		{"brainstorms", "body_source_id"},
		{"shaping_drafts", "body_source_id"},
		{"sessions", "body_source_id"},
		{"reports", "body_source_id"},
		{"plans", "body_source_id"},
		{"handoffs", "body_source_id"},
		{"councils", "body_source_id"},
		{"artifact_bodies", "source_id"},
		{"relationships", "source_id"},
	}
	for _, ref := range refs {
		var exists int
		query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE project_id = ? AND %s = ?)`, quoteSQLiteIdentifier(ref.table), quoteSQLiteIdentifier(ref.column))
		if err := tx.QueryRowContext(ctx, query, projectID, sourceID).Scan(&exists); err != nil {
			return false, fmt.Errorf("check %s.%s reference: %w", ref.table, ref.column, err)
		}
		if exists == 1 {
			return true, nil
		}
	}
	return false, nil
}

// execCountTx executes a write statement and returns the affected row count.
func execCountTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
