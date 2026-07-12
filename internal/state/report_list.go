package state

import (
	"context"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

// ReportList is the state-backed report-list read model.
type ReportList struct {
	ContractVersion    int                   `json:"contract_version,omitempty"`
	DatabaseScope      string                `json:"database_scope,omitempty"`
	DatabasePath       string                `json:"database_path,omitempty"`
	ProjectID          string                `json:"project_id,omitempty"`
	ProjectName        string                `json:"project_name,omitempty"`
	ProjectCurrentPath string                `json:"project_current_path,omitempty"`
	Diagnostics        []Diagnostic          `json:"diagnostics,omitempty"`
	Version            int                   `json:"version"`
	Reports            map[string]ReportItem `json:"reports"`
}

// ReportItem is a report entry returned by the state-backed report list.
type ReportItem struct {
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	SourcePath string `json:"source_path,omitempty"`
}

// ReportListOptions filter the state-backed report list.
type ReportListOptions struct {
	Type   string
	Status string
}

// ListReports returns imported reports from initialized SQLite state.
func ListReports(ctx context.Context, root project.Root, resolver PathResolver, options ReportListOptions) (ReportList, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return ReportList{}, err
	}
	defer store.Close()
	return store.ListReports(ctx, root, options)
}

// ListReports returns imported reports from an open store.
func (s *Store) ListReports(ctx context.Context, root project.Root, options ReportListOptions) (ReportList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ReportList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ReportList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  report_alias.alias,
  reports.title,
  reports.report_kind,
  reports.status,
  COALESCE(sources.path, '')
FROM reports
JOIN aliases report_alias
  ON report_alias.project_id = reports.project_id
 AND report_alias.entity_kind = 'report'
 AND report_alias.entity_id = reports.id
 AND report_alias.namespace = 'report'
LEFT JOIN sources ON sources.id = reports.body_source_id
WHERE reports.project_id = ?
ORDER BY report_alias.alias
`, projectID)
	if err != nil {
		return ReportList{}, fmt.Errorf("query reports: %w", err)
	}

	reportList := ReportList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Reports:            map[string]ReportItem{},
	}
	for rows.Next() {
		var alias, title, kind, status, sourcePath string
		if err := rows.Scan(&alias, &title, &kind, &status, &sourcePath); err != nil {
			rows.Close()
			return ReportList{}, fmt.Errorf("scan report: %w", err)
		}
		if !includeReport(kind, status, options) {
			continue
		}
		status = LifecycleStatusForDisplay(LifecycleEntityReport, status)
		reportList.Reports[alias] = ReportItem{
			Title:      title,
			Kind:       kind,
			Status:     status,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return ReportList{}, fmt.Errorf("close reports: %w", err)
	}
	if err := rows.Err(); err != nil {
		return ReportList{}, fmt.Errorf("iterate reports: %w", err)
	}
	return reportList, nil
}

func includeReport(kind string, status string, options ReportListOptions) bool {
	if options.Type != "" && kind != options.Type {
		return false
	}
	if !LifecycleStatusFilterMatches(LifecycleEntityReport, status, options.Status) {
		return false
	}
	return true
}
