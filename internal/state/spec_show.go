package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// SpecShow is the state-backed single-spec read model.
type SpecShow struct {
	ContractVersion    int        `json:"contract_version,omitempty"`
	DatabaseScope      string     `json:"database_scope,omitempty"`
	DatabasePath       string     `json:"database_path,omitempty"`
	ProjectID          string     `json:"project_id,omitempty"`
	ProjectName        string     `json:"project_name,omitempty"`
	ProjectCurrentPath string     `json:"project_current_path,omitempty"`
	Query              string     `json:"query"`
	Spec               SpecDetail `json:"spec"`
}

// SpecDetail contains operational spec metadata plus imported source context.
type SpecDetail struct {
	ID            string              `json:"id"`
	Alias         string              `json:"alias,omitempty"`
	Title         string              `json:"title"`
	Status        string              `json:"status"`
	Branch        string              `json:"branch,omitempty"`
	Source        string              `json:"source,omitempty"`
	Tasks         SpecTaskCounts      `json:"tasks"`
	Sources       []TraceSource       `json:"sources"`
	Related       []TraceEntity       `json:"related"`
	Body          string              `json:"body,omitempty"`
	HasBody       bool                `json:"has_body"`
	Relationships []TraceRelationship `json:"relationships"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// ShowSpec returns one spec from initialized SQLite state.
func ShowSpec(ctx context.Context, root project.Root, resolver PathResolver, ref string) (SpecShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SpecShow{}, err
	}
	defer store.Close()
	return store.ShowSpec(ctx, root, ref)
}

// ShowSpec returns one spec from an open store.
func (s *Store) ShowSpec(ctx context.Context, root project.Root, ref string) (SpecShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SpecShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SpecShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return SpecShow{}, err
	}
	if entity.Kind != "spec" {
		return SpecShow{}, fmt.Errorf("spec show target %q resolved to %s, not spec", ref, entity.Kind)
	}

	spec, err := s.specDetail(ctx, root, projectID, entity)
	if err != nil {
		return SpecShow{}, err
	}
	return SpecShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Spec:               spec,
	}, nil
}

func (s *Store) specDetail(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (SpecDetail, error) {
	var title, status, createdAt, updatedAt string
	var branch, source, sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  specs.title,
  specs.status,
  specs.branch,
  specs.source,
  specs.created_at,
  specs.updated_at,
  sources.path,
  sources.hash
FROM specs
LEFT JOIN sources ON sources.id = specs.body_source_id
WHERE specs.project_id = ? AND specs.id = ?
`, projectID, entity.ID).Scan(&title, &status, &branch, &source, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return SpecDetail{}, fmt.Errorf("spec %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return SpecDetail{}, fmt.Errorf("read spec %s: %w", entity.ID, err)
	}
	status = LifecycleStatusForDisplay(LifecycleEntitySpec, status)

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "spec", entity.ID); err == nil {
			alias = found
		}
	}
	tasks, err := s.specTaskCountsByID(ctx, projectID, entity.ID)
	if err != nil {
		return SpecDetail{}, err
	}

	sources := []TraceSource{}
	body := ""
	if sourcePath.Valid && sourcePath.String != "" {
		path := filepath.ToSlash(sourcePath.String)
		sources = append(sources, TraceSource{Path: path, Hash: sourceHash.String})
	}
	body, err = s.artifactBodyOrSourceBody(ctx, root.Path(), projectID, "spec", entity.ID, sourcePath)
	if err != nil {
		return SpecDetail{}, err
	}

	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{
		Kind:   "spec",
		ID:     entity.ID,
		Alias:  alias,
		Title:  title,
		Status: status,
	})
	if err != nil {
		return SpecDetail{}, err
	}

	return SpecDetail{
		ID:            entity.ID,
		Alias:         alias,
		Title:         title,
		Status:        status,
		Branch:        branch.String,
		Source:        source.String,
		Tasks:         tasks,
		Sources:       sources,
		Related:       relatedSpecsFromRelationships(relationships),
		Body:          body,
		HasBody:       strings.TrimSpace(body) != "",
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

// relatedSpecsFromRelationships resolves the specs connected to this spec via a
// related_to relationship in either direction, deduplicated and sorted by alias.
func relatedSpecsFromRelationships(relationships []TraceRelationship) []TraceEntity {
	related := []TraceEntity{}
	seen := map[string]bool{}
	for _, relationship := range relationships {
		if relationship.Type != "related_to" || relationship.Entity.Kind != "spec" {
			continue
		}
		if seen[relationship.Entity.ID] {
			continue
		}
		seen[relationship.Entity.ID] = true
		related = append(related, relationship.Entity)
	}
	sort.Slice(related, func(i, j int) bool {
		return firstNonEmpty(related[i].Alias, related[i].ID) < firstNonEmpty(related[j].Alias, related[j].ID)
	})
	return related
}

func (s *Store) specTaskCountsByID(ctx context.Context, projectID string, specID string) (SpecTaskCounts, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT status, COUNT(*)
FROM tasks
WHERE project_id = ? AND spec_id = ?
GROUP BY status
`, projectID, specID)
	if err != nil {
		return SpecTaskCounts{}, fmt.Errorf("query spec task counts: %w", err)
	}
	defer rows.Close()

	var counts SpecTaskCounts
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return SpecTaskCounts{}, fmt.Errorf("scan spec task count: %w", err)
		}
		switch status {
		case "done", "archived":
			counts.Done += count
		case "in_progress":
			counts.InProgress += count
		default:
			counts.Todo += count
		}
	}
	if err := rows.Err(); err != nil {
		return SpecTaskCounts{}, fmt.Errorf("iterate spec task counts: %w", err)
	}
	return counts, nil
}
