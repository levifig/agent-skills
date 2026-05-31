package state

import (
	"context"
	"fmt"
	"os"

	"github.com/levifig/loaf/internal/project"
)

// ReportList is the state-backed report-list read model.
type ReportList struct {
	Version int                   `json:"version"`
	Reports map[string]ReportItem `json:"reports"`
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
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return ReportList{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return ReportList{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return ReportList{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return ReportList{}, err
	}
	defer store.Close()
	return store.ListReports(ctx, root, options)
}

// ListReports returns imported reports from an open store.
func (s *Store) ListReports(ctx context.Context, root project.Root, options ReportListOptions) (ReportList, error) {
	projectID := ProjectID(root)
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

	reportList := ReportList{Version: 1, Reports: map[string]ReportItem{}}
	for rows.Next() {
		var alias, title, kind, status, sourcePath string
		if err := rows.Scan(&alias, &title, &kind, &status, &sourcePath); err != nil {
			rows.Close()
			return ReportList{}, fmt.Errorf("scan report: %w", err)
		}
		if !includeReport(kind, status, options) {
			continue
		}
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
	if options.Status != "" && status != options.Status {
		return false
	}
	return true
}
