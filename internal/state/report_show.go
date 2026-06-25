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
	Findings      []FindingItem       `json:"findings,omitempty"`
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
	findings, err := s.reportFindings(ctx, projectID, entity.ID)
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
		Findings:      findings,
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Store) reportFindings(ctx context.Context, projectID string, reportID string) ([]FindingItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  findings.id,
  COALESCE(finding_alias.alias, ''),
  COALESCE(report_alias.alias, findings.report_id),
  COALESCE(run_alias.alias, findings.run_id, ''),
  findings.title,
  findings.status,
  findings.severity,
  findings.confidence,
  COALESCE(findings.dimension, ''),
  COALESCE(findings.path, ''),
  COALESCE(findings.line_start, 0),
  COALESCE(findings.line_end, 0),
  COALESCE(findings.symbol, '')
FROM findings
LEFT JOIN aliases finding_alias
  ON finding_alias.project_id = findings.project_id
 AND finding_alias.entity_kind = 'finding'
 AND finding_alias.entity_id = findings.id
 AND finding_alias.namespace = 'finding'
LEFT JOIN aliases report_alias
  ON report_alias.project_id = findings.project_id
 AND report_alias.entity_kind = 'report'
 AND report_alias.entity_id = findings.report_id
 AND report_alias.namespace = 'report'
LEFT JOIN aliases run_alias
  ON run_alias.project_id = findings.project_id
 AND run_alias.entity_kind = 'run'
 AND run_alias.entity_id = findings.run_id
 AND run_alias.namespace = 'run'
WHERE findings.project_id = ? AND findings.report_id = ?
ORDER BY CASE findings.severity
    WHEN 'critical' THEN 1
    WHEN 'high' THEN 2
    WHEN 'medium' THEN 3
    WHEN 'low' THEN 4
    WHEN 'info' THEN 5
    ELSE 99
  END,
  finding_alias.alias,
  findings.id
`, projectID, reportID)
	if err != nil {
		return nil, fmt.Errorf("query report findings: %w", err)
	}
	var findings []FindingItem
	for rows.Next() {
		var item FindingItem
		if err := rows.Scan(&item.ID, &item.Alias, &item.Report, &item.Run, &item.Title, &item.Status, &item.Severity, &item.Confidence, &item.Dimension, &item.Path, &item.LineStart, &item.LineEnd, &item.Symbol); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan report finding: %w", err)
		}
		findings = append(findings, item)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close report findings: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report findings: %w", err)
	}
	return findings, nil
}
