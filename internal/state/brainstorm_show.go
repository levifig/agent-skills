package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

// BrainstormShow is the state-backed single-brainstorm read model.
type BrainstormShow struct {
	Query      string           `json:"query"`
	Brainstorm BrainstormDetail `json:"brainstorm"`
}

// BrainstormDetail contains operational brainstorm metadata plus imported source context.
type BrainstormDetail struct {
	ID            string              `json:"id"`
	Alias         string              `json:"alias,omitempty"`
	Title         string              `json:"title"`
	Status        string              `json:"status"`
	Sources       []TraceSource       `json:"sources"`
	Body          string              `json:"body,omitempty"`
	Relationships []TraceRelationship `json:"relationships"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// ShowBrainstorm returns one brainstorm from initialized SQLite state.
func ShowBrainstorm(ctx context.Context, root project.Root, resolver PathResolver, ref string) (BrainstormShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BrainstormShow{}, err
	}
	defer store.Close()
	return store.ShowBrainstorm(ctx, root, ref)
}

// ShowBrainstorm returns one brainstorm from an open store.
func (s *Store) ShowBrainstorm(ctx context.Context, root project.Root, ref string) (BrainstormShow, error) {
	projectID := ProjectID(root)
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return BrainstormShow{}, err
	}
	if entity.Kind != "brainstorm" {
		return BrainstormShow{}, fmt.Errorf("brainstorm show target %q resolved to %s, not brainstorm", ref, entity.Kind)
	}

	brainstorm, err := s.brainstormDetail(ctx, root, projectID, entity)
	if err != nil {
		return BrainstormShow{}, err
	}
	return BrainstormShow{Query: ref, Brainstorm: brainstorm}, nil
}

func (s *Store) brainstormDetail(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (BrainstormDetail, error) {
	var title, status, createdAt, updatedAt string
	var sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  brainstorms.title,
  brainstorms.status,
  brainstorms.created_at,
  brainstorms.updated_at,
  sources.path,
  sources.hash
FROM brainstorms
LEFT JOIN sources ON sources.id = brainstorms.body_source_id
WHERE brainstorms.project_id = ? AND brainstorms.id = ?
`, projectID, entity.ID).Scan(&title, &status, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return BrainstormDetail{}, fmt.Errorf("brainstorm %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return BrainstormDetail{}, fmt.Errorf("read brainstorm %s: %w", entity.ID, err)
	}

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "brainstorm", entity.ID); err == nil {
			alias = found
		}
	}

	sources := []TraceSource{}
	body := ""
	if sourcePath.Valid && sourcePath.String != "" {
		path := filepath.ToSlash(sourcePath.String)
		sources = append(sources, TraceSource{Path: path, Hash: sourceHash.String})
		if content, err := readImportedSourceBody(root.Path(), path); err == nil {
			body = content
		}
	}

	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{
		Kind:   "brainstorm",
		ID:     entity.ID,
		Alias:  alias,
		Title:  title,
		Status: status,
	})
	if err != nil {
		return BrainstormDetail{}, err
	}

	return BrainstormDetail{
		ID:            entity.ID,
		Alias:         alias,
		Title:         title,
		Status:        status,
		Sources:       sources,
		Body:          body,
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}
