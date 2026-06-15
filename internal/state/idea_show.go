package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

// IdeaShow is the state-backed single-idea read model.
type IdeaShow struct {
	ContractVersion    int        `json:"contract_version,omitempty"`
	DatabaseScope      string     `json:"database_scope,omitempty"`
	DatabasePath       string     `json:"database_path,omitempty"`
	ProjectID          string     `json:"project_id,omitempty"`
	ProjectName        string     `json:"project_name,omitempty"`
	ProjectCurrentPath string     `json:"project_current_path,omitempty"`
	Query              string     `json:"query"`
	Idea               IdeaDetail `json:"idea"`
}

// IdeaDetail contains operational idea metadata plus imported source context.
type IdeaDetail struct {
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

// ShowIdea returns one idea from initialized SQLite state.
func ShowIdea(ctx context.Context, root project.Root, resolver PathResolver, ref string) (IdeaShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return IdeaShow{}, err
	}
	defer store.Close()
	return store.ShowIdea(ctx, root, ref)
}

// ShowIdea returns one idea from an open store.
func (s *Store) ShowIdea(ctx context.Context, root project.Root, ref string) (IdeaShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IdeaShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IdeaShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return IdeaShow{}, err
	}
	if entity.Kind != "idea" {
		return IdeaShow{}, fmt.Errorf("idea show target %q resolved to %s, not idea", ref, entity.Kind)
	}

	idea, err := s.ideaDetail(ctx, root, projectID, entity)
	if err != nil {
		return IdeaShow{}, err
	}
	return IdeaShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Idea:               idea,
	}, nil
}

func (s *Store) ideaDetail(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (IdeaDetail, error) {
	var title, status, createdAt, updatedAt string
	var sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  ideas.title,
  ideas.status,
  ideas.created_at,
  ideas.updated_at,
  sources.path,
  sources.hash
FROM ideas
LEFT JOIN sources ON sources.id = ideas.body_source_id
WHERE ideas.project_id = ? AND ideas.id = ?
`, projectID, entity.ID).Scan(&title, &status, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return IdeaDetail{}, fmt.Errorf("idea %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return IdeaDetail{}, fmt.Errorf("read idea %s: %w", entity.ID, err)
	}

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "idea", entity.ID); err == nil {
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
		Kind:   "idea",
		ID:     entity.ID,
		Alias:  alias,
		Title:  title,
		Status: status,
	})
	if err != nil {
		return IdeaDetail{}, err
	}

	return IdeaDetail{
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
