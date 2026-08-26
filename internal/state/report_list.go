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
	_ = ctx
	_ = root
	_ = resolver
	_ = options
	return ReportList{}, fmt.Errorf("reports are file-backed under .agents/reports/; SQLite rows were removed in migration 0021")
}

// ListReports refuses SQLite authority; reports are file-backed after migration 0021.
func (s *Store) ListReports(ctx context.Context, root project.Root, options ReportListOptions) (ReportList, error) {
	_ = s
	_ = ctx
	_ = root
	_ = options
	return ReportList{}, errReportFileBacked
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
