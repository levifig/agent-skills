package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

// ReportShow is the state-backed single-report read model.
type ReportShow struct {
	ContractVersion    int          `json:"contract_version,omitempty"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Query              string       `json:"query"`
	Report             ReportDetail `json:"report"`
}

// ReportDetail contains operational report metadata plus body content.
type ReportDetail struct {
	ID            string              `json:"id"`
	Alias         string              `json:"alias,omitempty"`
	Title         string              `json:"title"`
	Kind          string              `json:"kind"`
	Status        string              `json:"status"`
	Sources       []TraceSource       `json:"sources"`
	Body          string              `json:"body,omitempty"`
	Relationships []TraceRelationship `json:"relationships"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// ShowReport returns one report from initialized SQLite state.
func ShowReport(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ReportShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ReportShow{}, err
	}
	defer store.Close()
	return store.ShowReport(ctx, root, ref)
}

// ShowReport returns one report from an open store.
func (s *Store) ShowReport(ctx context.Context, root project.Root, ref string) (ReportShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ReportShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ReportShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return ReportShow{}, err
	}
	if entity.Kind != "report" {
		return ReportShow{}, fmt.Errorf("report show target %q resolved to %s, not report", ref, entity.Kind)
	}
	report, err := s.reportDetail(ctx, root, projectID, entity)
	if err != nil {
		return ReportShow{}, err
	}
	return ReportShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Report:             report,
	}, nil
}

func (s *Store) reportDetail(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (ReportDetail, error) {
	var title, kind, status, createdAt, updatedAt string
	var sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  reports.title,
  reports.report_kind,
  reports.status,
  reports.created_at,
  reports.updated_at,
  sources.path,
  sources.hash
FROM reports
LEFT JOIN sources ON sources.id = reports.body_source_id
WHERE reports.project_id = ? AND reports.id = ?
`, projectID, entity.ID).Scan(&title, &kind, &status, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return ReportDetail{}, fmt.Errorf("report %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return ReportDetail{}, fmt.Errorf("read report %s: %w", entity.ID, err)
	}

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "report", entity.ID); err == nil {
			alias = found
		}
	}

	sources := []TraceSource{}
	if sourcePath.Valid && sourcePath.String != "" {
		sources = append(sources, TraceSource{Path: filepath.ToSlash(sourcePath.String), Hash: sourceHash.String})
	}
	body, err := s.artifactBodyOrSourceBody(ctx, root.Path(), projectID, "report", entity.ID, sourcePath)
	if err != nil {
		return ReportDetail{}, err
	}
	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{
		Kind:   "report",
		ID:     entity.ID,
		Alias:  alias,
		Title:  title,
		Status: status,
	})
	if err != nil {
		return ReportDetail{}, err
	}

	return ReportDetail{
		ID:            entity.ID,
		Alias:         alias,
		Title:         title,
		Kind:          kind,
		Status:        status,
		Sources:       sources,
		Body:          body,
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}
