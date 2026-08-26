package state

import (
	"context"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

var errReportFileBacked = fmt.Errorf("reports are file-backed under .agents/reports/; SQLite rows were removed in migration 0021")

// ReportCreateOptions describes a SQLite-backed report creation request.
type ReportCreateOptions struct {
	Slug    string
	Kind    string
	Source  string
	Body    string
	SetBody bool
}

// ReportCreateResult describes a created SQLite-backed report.
type ReportCreateResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Report             TraceEntity `json:"report"`
	Kind               string      `json:"kind"`
	Source             string      `json:"source"`
	EventID            string      `json:"event_id"`
}

// ReportStatusResult describes a SQLite-backed report status transition.
type ReportStatusResult struct {
	ContractVersion    int                    `json:"contract_version,omitempty"`
	DatabaseScope      string                 `json:"database_scope,omitempty"`
	DatabasePath       string                 `json:"database_path,omitempty"`
	ProjectID          string                 `json:"project_id,omitempty"`
	ProjectName        string                 `json:"project_name,omitempty"`
	ProjectCurrentPath string                 `json:"project_current_path,omitempty"`
	Report             TraceEntity            `json:"report"`
	Previous           string                 `json:"previous"`
	Status             string                 `json:"status"`
	EventID            string                 `json:"event_id"`
	Render             *DurableFinalizeResult `json:"render,omitempty"`
}

// ReportEditOptions describes a SQLite-backed report body edit request.
type ReportEditOptions struct {
	Ref   string
	Body  string
	Force bool // proceed when the legacy source .md diverges from the stored body
}

// ReportEditResult describes an applied SQLite-backed report body edit.
type ReportEditResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Report             TraceEntity `json:"report"`
	Imported           bool        `json:"imported"`
	ContentHash        string      `json:"content_hash"`
	EventID            string      `json:"event_id"`
}

// CreateReport creates a draft report in initialized SQLite state.
func CreateReport(ctx context.Context, root project.Root, resolver PathResolver, options ReportCreateOptions) (ReportCreateResult, error) {
	_ = ctx
	_ = root
	_ = resolver
	return ReportCreateResult{}, errReportFileBacked
}

// EditReportBody replaces a report's SQLite body in initialized SQLite state.
func EditReportBody(ctx context.Context, root project.Root, resolver PathResolver, options ReportEditOptions) (ReportEditResult, error) {
	_ = ctx
	_ = root
	_ = resolver
	return ReportEditResult{}, errReportFileBacked
}

// EditReportBody refuses SQLite authority; reports are file-backed after migration 0021.
func (s *Store) EditReportBody(ctx context.Context, root project.Root, options ReportEditOptions) (ReportEditResult, error) {
	_ = s
	_ = ctx
	_ = root
	_ = options
	return ReportEditResult{}, errReportFileBacked
}

// FinalizeReport transitions a draft report to done in initialized SQLite state.
func FinalizeReport(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ReportStatusResult, error) {
	_ = ctx
	_ = root
	_ = resolver
	return ReportStatusResult{}, errReportFileBacked
}

// ArchiveReport transitions a done report to archived in initialized SQLite state.
func ArchiveReport(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ReportStatusResult, error) {
	_ = ctx
	_ = root
	_ = resolver
	return ReportStatusResult{}, errReportFileBacked
}

// CreateReport refuses SQLite authority; reports are file-backed after migration 0021.
func (s *Store) CreateReport(ctx context.Context, root project.Root, options ReportCreateOptions) (ReportCreateResult, error) {
	_ = s
	_ = ctx
	_ = root
	_ = options
	return ReportCreateResult{}, errReportFileBacked
}

// FinalizeReport refuses SQLite authority; reports are file-backed after migration 0021.
func (s *Store) FinalizeReport(ctx context.Context, root project.Root, ref string) (ReportStatusResult, error) {
	_ = s
	_ = ctx
	_ = root
	_ = ref
	return ReportStatusResult{}, errReportFileBacked
}

// ArchiveReport refuses SQLite authority; reports are file-backed after migration 0021.
func (s *Store) ArchiveReport(ctx context.Context, root project.Root, ref string) (ReportStatusResult, error) {
	_ = s
	_ = ctx
	_ = root
	_ = ref
	return ReportStatusResult{}, errReportFileBacked
}
