package state

import (
	"context"

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
	HasBody       bool                `json:"has_body"`
	Relationships []TraceRelationship `json:"relationships"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// ShowReport refuses SQLite authority; reports are file-backed after migration 0021.
func ShowReport(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ReportShow, error) {
	_ = ctx
	_ = root
	_ = resolver
	_ = ref
	return ReportShow{}, errReportFileBacked
}

// ShowReport refuses SQLite authority; reports are file-backed after migration 0021.
func (s *Store) ShowReport(ctx context.Context, root project.Root, ref string) (ReportShow, error) {
	_ = s
	_ = ctx
	_ = root
	_ = ref
	return ReportShow{}, errReportFileBacked
}
