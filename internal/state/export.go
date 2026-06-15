package state

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	ExportKindAll              = "all"
	ExportKindReleaseReadiness = "release-readiness"
	ExportKindSpec             = "spec"
	ExportKindSession          = "session"
	ExportKindTriage           = "triage"
	ExportFormatJSON           = "json"
	ExportFormatMarkdown       = "markdown"
	ExportAudienceLocal        = "internal"
	ExportAudienceExternal     = "external"
	StateJSONContractVersion   = 1
)

// ExportSnapshot is a complete internal JSON view of current SQLite state.
type ExportSnapshot struct {
	ContractVersion    int                         `json:"contract_version"`
	ExportKind         string                      `json:"export_kind"`
	Format             string                      `json:"format"`
	Audience           string                      `json:"audience"`
	DatabaseScope      string                      `json:"database_scope"`
	ExportScope        string                      `json:"export_scope"`
	GeneratedAt        string                      `json:"generated_at"`
	ProjectID          string                      `json:"project_id"`
	ProjectName        string                      `json:"project_name"`
	ProjectCurrentPath string                      `json:"project_current_path"`
	DatabasePath       string                      `json:"database_path"`
	SchemaVersion      int                         `json:"schema_version"`
	Diagnostics        []Diagnostic                `json:"diagnostics"`
	RepairPlan         []RepairAction              `json:"repair_plan"`
	Manifest           ExportManifest              `json:"manifest"`
	Tables             map[string][]map[string]any `json:"tables"`
}

// ExportManifest is a compact, agent-friendly summary of an export snapshot.
type ExportManifest struct {
	ContractVersion    int            `json:"contract_version"`
	Verified           bool           `json:"verified"`
	DatabaseScope      string         `json:"database_scope"`
	ExportScope        string         `json:"export_scope"`
	SchemaVersion      int            `json:"schema_version"`
	ProjectID          string         `json:"project_id"`
	ProjectName        string         `json:"project_name"`
	ProjectCurrentPath string         `json:"project_current_path"`
	IntegrityCheck     string         `json:"integrity_check"`
	ForeignKeyCheck    string         `json:"foreign_key_check"`
	DiagnosticCount    int            `json:"diagnostic_count"`
	RepairActionCount  int            `json:"repair_action_count"`
	TableCount         int            `json:"table_count"`
	TableOrder         []string       `json:"table_order"`
	RowCounts          map[string]int `json:"row_counts"`
	TotalRows          int            `json:"total_rows"`
	GeneratedAt        string         `json:"generated_at"`
}

// MarkdownExport is a generated Markdown view of SQLite state.
type MarkdownExport struct {
	ContractVersion    int    `json:"contract_version"`
	Command            string `json:"command,omitempty"`
	ExportKind         string `json:"export_kind"`
	Format             string `json:"format"`
	Audience           string `json:"audience"`
	DatabaseScope      string `json:"database_scope"`
	ExportScope        string `json:"export_scope"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	DatabasePath       string `json:"database_path,omitempty"`
	Content            string `json:"content"`
}

type exportTable struct {
	Name         string
	OrderBy      string
	FilterColumn string
}

type releaseReadinessExportData struct {
	SchemaVersion      int
	Specs              SpecList
	Tasks              TaskList
	Sessions           SessionList
	Reports            ReportList
	SourceCoverage     []releaseReadinessSourceCoverage
	RelationshipCounts []releaseReadinessCount
	ExportCounts       []releaseReadinessCount
	RecentReports      []releaseReadinessReport
	RecentSessions     []releaseReadinessSession
}

type markdownExportContext struct {
	Audience           string
	DatabaseScope      string
	ExportScope        string
	ProjectID          string
	ProjectName        string
	ProjectCurrentPath string
	DatabasePath       string
}

type releaseReadinessSourceCoverage struct {
	Label string
	With  int
	Total int
}

type releaseReadinessCount struct {
	Label string
	Count int
}

type releaseReadinessReport struct {
	Title  string
	Kind   string
	Status string
}

type releaseReadinessSession struct {
	Branch         string
	Status         string
	JournalEntries int
}

var externalLeakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bSPEC-\d+\b`),
	regexp.MustCompile(`\bTASK-\d+\b`),
	regexp.MustCompile(`\.agents(?:/[^\s)]*)?`),
	regexp.MustCompile(`(?i)\bTrack[\s-]+[A-Za-z0-9]+\b`),
	regexp.MustCompile(`(?i)\bPhase[\s-]+[A-Za-z0-9]+\b`),
}

var exportAllTables = []exportTable{
	{Name: "schema_migrations", OrderBy: "version"},
	{Name: "projects", OrderBy: "id", FilterColumn: "id"},
	{Name: "project_paths", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "aliases", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "specs", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "tasks", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "ideas", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "sparks", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "brainstorms", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "shaping_drafts", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "sessions", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "session_state_snapshots", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "reports", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "journal_entries", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "events", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "relationships", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "tags", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "entity_tags", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "bundles", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "bundle_members", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "sources", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "backend_mappings", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "hook_events", OrderBy: "id", FilterColumn: "project_id"},
	{Name: "exports", OrderBy: "id", FilterColumn: "project_id"},
}

// ExportAllJSON returns a repository-non-mutating internal snapshot of SQLite state.
func ExportAllJSON(ctx context.Context, root project.Root, resolver PathResolver) (ExportSnapshot, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return ExportSnapshot{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return ExportSnapshot{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return ExportSnapshot{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return ExportSnapshot{}, fmt.Errorf("open state database for export: %w", err)
	}
	defer store.Close()

	tables := make(map[string][]map[string]any, len(exportAllTables))
	tableOrder := make([]string, 0, len(exportAllTables))
	rowCounts := make(map[string]int, len(exportAllTables))
	totalRows := 0
	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return ExportSnapshot{}, err
	}
	projectID := identity.ID
	integrityCheck, err := verifySQLiteIntegrity(ctx, store)
	if err != nil {
		return ExportSnapshot{}, fmt.Errorf("verify export integrity: %w", err)
	}
	foreignKeyCheck, err := verifyNoForeignKeyViolations(ctx, store)
	if err != nil {
		return ExportSnapshot{}, fmt.Errorf("verify export foreign keys: %w", err)
	}
	if err := store.validateExportTableFilters(ctx, exportAllTables); err != nil {
		return ExportSnapshot{}, err
	}
	for _, table := range exportAllTables {
		rows, err := store.exportRows(ctx, table, projectID)
		if err != nil {
			return ExportSnapshot{}, err
		}
		tables[table.Name] = rows
		tableOrder = append(tableOrder, table.Name)
		rowCounts[table.Name] = len(rows)
		totalRows += len(rows)
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	repairPlan := RepairPlanForStatus(status)

	return ExportSnapshot{
		ContractVersion:    StateJSONContractVersion,
		ExportKind:         ExportKindAll,
		Format:             ExportFormatJSON,
		Audience:           ExportAudienceLocal,
		DatabaseScope:      "global",
		ExportScope:        "project",
		GeneratedAt:        generatedAt,
		ProjectID:          projectID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		DatabasePath:       status.DatabasePath,
		SchemaVersion:      status.SchemaVersion,
		Diagnostics:        status.Diagnostics,
		RepairPlan:         repairPlan,
		Manifest: ExportManifest{
			ContractVersion:    StateJSONContractVersion,
			Verified:           true,
			DatabaseScope:      "global",
			ExportScope:        "project",
			SchemaVersion:      status.SchemaVersion,
			ProjectID:          projectID,
			ProjectName:        identity.FriendlyName,
			ProjectCurrentPath: identity.CurrentPath,
			IntegrityCheck:     integrityCheck,
			ForeignKeyCheck:    foreignKeyCheck,
			DiagnosticCount:    len(status.Diagnostics),
			RepairActionCount:  len(repairPlan),
			TableCount:         len(tableOrder),
			TableOrder:         tableOrder,
			RowCounts:          rowCounts,
			TotalRows:          totalRows,
			GeneratedAt:        generatedAt,
		},
		Tables: tables,
	}, nil
}

// ExportTriageMarkdown returns an external-safe Markdown triage summary.
func ExportTriageMarkdown(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownExport, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return MarkdownExport{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return MarkdownExport{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return MarkdownExport{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return MarkdownExport{}, fmt.Errorf("open state database for export: %w", err)
	}
	defer store.Close()

	ideas, err := store.ListIdeas(ctx, root, IdeaListOptions{All: true})
	if err != nil {
		return MarkdownExport{}, err
	}
	sparks, err := store.ListSparks(ctx, root, SparkListOptions{All: true})
	if err != nil {
		return MarkdownExport{}, err
	}
	brainstorms, err := store.ListBrainstorms(ctx, root, BrainstormListOptions{All: true})
	if err != nil {
		return MarkdownExport{}, err
	}

	exportContext := markdownExportContextFromStatus(status, ExportAudienceExternal)
	content := renderTriageMarkdown(exportContext, ideas, sparks, brainstorms)
	if err := ValidateExternalMarkdownExport(content); err != nil {
		return MarkdownExport{}, err
	}
	return markdownExportResult(ExportKindTriage, exportContext, content), nil
}

// ExportReleaseReadinessMarkdown returns an external-safe Markdown release readiness summary.
func ExportReleaseReadinessMarkdown(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownExport, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return MarkdownExport{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return MarkdownExport{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return MarkdownExport{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return MarkdownExport{}, fmt.Errorf("open state database for export: %w", err)
	}
	defer store.Close()

	data, err := store.releaseReadinessExportData(ctx, root, status.SchemaVersion)
	if err != nil {
		return MarkdownExport{}, err
	}
	exportContext := markdownExportContextFromStatus(status, ExportAudienceExternal)
	content := renderReleaseReadinessMarkdown(exportContext, data)
	if err := ValidateExternalMarkdownExport(content); err != nil {
		return MarkdownExport{}, err
	}
	return markdownExportResult(ExportKindReleaseReadiness, exportContext, content), nil
}

// ExportSpecMarkdown returns an internal Markdown summary for one spec.
func ExportSpecMarkdown(ctx context.Context, root project.Root, resolver PathResolver, ref string) (MarkdownExport, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return MarkdownExport{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return MarkdownExport{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return MarkdownExport{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return MarkdownExport{}, fmt.Errorf("open state database for export: %w", err)
	}
	defer store.Close()

	show, err := store.ShowSpec(ctx, root, ref)
	if err != nil {
		return MarkdownExport{}, err
	}
	exportContext := markdownExportContextFromStatus(status, ExportAudienceLocal)
	return markdownExportResult(ExportKindSpec, exportContext, renderSpecMarkdown(exportContext, show.Spec)), nil
}

// ExportSessionMarkdown returns an internal Markdown summary for one session.
func ExportSessionMarkdown(ctx context.Context, root project.Root, resolver PathResolver, ref string) (MarkdownExport, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return MarkdownExport{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return MarkdownExport{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return MarkdownExport{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return MarkdownExport{}, fmt.Errorf("open state database for export: %w", err)
	}
	defer store.Close()

	show, err := store.ShowSession(ctx, root, ref)
	if err != nil {
		return MarkdownExport{}, err
	}
	exportContext := markdownExportContextFromStatus(status, ExportAudienceLocal)
	return markdownExportResult(ExportKindSession, exportContext, renderSessionMarkdown(exportContext, show.Session)), nil
}

func (s *Store) releaseReadinessExportData(ctx context.Context, root project.Root, schemaVersion int) (releaseReadinessExportData, error) {
	specs, err := s.ListSpecs(ctx, root)
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	tasks, err := s.ListTasks(ctx, root, TaskListOptions{})
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	sessions, err := s.ListSessions(ctx, root, SessionListOptions{All: true})
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	reports, err := s.ListReports(ctx, root, ReportListOptions{})
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	identity, err := s.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	projectID := identity.ID
	sourceCoverage, err := s.releaseReadinessSourceCoverage(ctx, projectID)
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	relationshipCounts, err := s.releaseReadinessGroupedCounts(ctx, `
SELECT relationship_type, COUNT(*)
FROM relationships
WHERE project_id = ?
GROUP BY relationship_type
ORDER BY relationship_type
`, projectID)
	if err != nil {
		return releaseReadinessExportData{}, fmt.Errorf("query release relationship counts: %w", err)
	}
	exportCounts, err := s.releaseReadinessGroupedCounts(ctx, `
SELECT export_kind || '/' || format, COUNT(*)
FROM exports
WHERE project_id = ?
GROUP BY export_kind, format
ORDER BY export_kind, format
`, projectID)
	if err != nil {
		return releaseReadinessExportData{}, fmt.Errorf("query release export counts: %w", err)
	}
	recentReports, err := s.releaseReadinessRecentReports(ctx, projectID)
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	recentSessions, err := s.releaseReadinessRecentSessions(ctx, projectID)
	if err != nil {
		return releaseReadinessExportData{}, err
	}
	return releaseReadinessExportData{
		SchemaVersion:      schemaVersion,
		Specs:              specs,
		Tasks:              tasks,
		Sessions:           sessions,
		Reports:            reports,
		SourceCoverage:     sourceCoverage,
		RelationshipCounts: relationshipCounts,
		ExportCounts:       exportCounts,
		RecentReports:      recentReports,
		RecentSessions:     recentSessions,
	}, nil
}

func (s *Store) releaseReadinessSourceCoverage(ctx context.Context, projectID string) ([]releaseReadinessSourceCoverage, error) {
	tables := []struct {
		Label  string
		Table  string
		Column string
	}{
		{Label: "Specs", Table: "specs", Column: "body_source_id"},
		{Label: "Tasks", Table: "tasks", Column: "body_source_id"},
		{Label: "Sessions", Table: "sessions", Column: "body_source_id"},
		{Label: "Reports", Table: "reports", Column: "body_source_id"},
	}
	coverage := make([]releaseReadinessSourceCoverage, 0, len(tables))
	for _, table := range tables {
		query := fmt.Sprintf("SELECT COUNT(*), COUNT(%s) FROM %s WHERE project_id = ?", table.Column, table.Table)
		var total, withSource int
		if err := s.db.QueryRowContext(ctx, query, projectID).Scan(&total, &withSource); err != nil {
			return nil, fmt.Errorf("query %s source coverage: %w", table.Table, err)
		}
		coverage = append(coverage, releaseReadinessSourceCoverage{Label: table.Label, With: withSource, Total: total})
	}
	return coverage, nil
}

func (s *Store) releaseReadinessGroupedCounts(ctx context.Context, query string, projectID string) ([]releaseReadinessCount, error) {
	rows, err := s.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := []releaseReadinessCount{}
	for rows.Next() {
		var label string
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return nil, err
		}
		counts = append(counts, releaseReadinessCount{Label: label, Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func (s *Store) releaseReadinessRecentReports(ctx context.Context, projectID string) ([]releaseReadinessReport, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT title, report_kind, status
FROM reports
WHERE project_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 5
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query release recent reports: %w", err)
	}
	defer rows.Close()
	reports := []releaseReadinessReport{}
	for rows.Next() {
		var report releaseReadinessReport
		if err := rows.Scan(&report.Title, &report.Kind, &report.Status); err != nil {
			return nil, fmt.Errorf("scan release recent report: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release recent reports: %w", err)
	}
	return reports, nil
}

func (s *Store) releaseReadinessRecentSessions(ctx context.Context, projectID string) ([]releaseReadinessSession, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(sessions.branch, ''), sessions.status, COUNT(journal_entries.id)
FROM sessions
LEFT JOIN journal_entries
  ON journal_entries.project_id = sessions.project_id
 AND journal_entries.session_id = sessions.id
WHERE sessions.project_id = ?
GROUP BY sessions.id, sessions.branch, sessions.status, sessions.created_at
ORDER BY sessions.created_at DESC, sessions.id DESC
LIMIT 5
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query release recent sessions: %w", err)
	}
	defer rows.Close()
	sessions := []releaseReadinessSession{}
	for rows.Next() {
		var session releaseReadinessSession
		if err := rows.Scan(&session.Branch, &session.Status, &session.JournalEntries); err != nil {
			return nil, fmt.Errorf("scan release recent session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release recent sessions: %w", err)
	}
	return sessions, nil
}

func (s *Store) validateExportTableFilters(ctx context.Context, tables []exportTable) error {
	for _, table := range tables {
		hasProjectID, hasFilterColumn, err := s.exportTableColumnCoverage(ctx, table.Name, table.FilterColumn)
		if err != nil {
			return err
		}
		if hasProjectID && table.FilterColumn == "" {
			return fmt.Errorf("export table %s has project_id but no filter column", table.Name)
		}
		if table.FilterColumn != "" && !hasFilterColumn {
			return fmt.Errorf("export table %s filter column %s does not exist", table.Name, table.FilterColumn)
		}
	}
	return nil
}

func (s *Store) exportTableColumnCoverage(ctx context.Context, tableName string, filterColumn string) (bool, bool, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, false, fmt.Errorf("inspect export table %s: %w", tableName, err)
	}
	defer rows.Close()

	hasProjectID := false
	hasFilterColumn := filterColumn == ""
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, false, fmt.Errorf("scan export table %s columns: %w", tableName, err)
		}
		if name == "project_id" {
			hasProjectID = true
		}
		if name == filterColumn {
			hasFilterColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, false, fmt.Errorf("iterate export table %s columns: %w", tableName, err)
	}
	return hasProjectID, hasFilterColumn, nil
}

func (s *Store) exportRows(ctx context.Context, table exportTable, projectID string) ([]map[string]any, error) {
	query := fmt.Sprintf("SELECT * FROM %s", table.Name)
	args := []any{}
	if table.FilterColumn != "" {
		query += fmt.Sprintf(" WHERE %s = ?", table.FilterColumn)
		args = append(args, projectID)
	}
	query += fmt.Sprintf(" ORDER BY %s", table.OrderBy)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("export %s: %w", table.Name, err)
	}
	defer rows.Close()

	result, err := scanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("export %s: %w", table.Name, err)
	}
	return result, nil
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeExportValue(values[i])
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return []map[string]any{}, nil
	}
	return result, nil
}

func normalizeExportValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return string(v)
	default:
		return v
	}
}

// ValidateExternalMarkdownExport checks generated external Markdown against the SPEC-038 boundary.
func ValidateExternalMarkdownExport(content string) error {
	for _, pattern := range externalLeakPatterns {
		if match := pattern.FindString(content); match != "" {
			return fmt.Errorf("external export contains private Loaf reference %q", match)
		}
	}
	return nil
}

func markdownExportContextFromStatus(status Status, audience string) markdownExportContext {
	return markdownExportContext{
		Audience:           audience,
		DatabaseScope:      firstNonEmpty(status.DatabaseScope, "global"),
		ExportScope:        "project",
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		DatabasePath:       status.DatabasePath,
	}
}

func markdownExportResult(kind string, ctx markdownExportContext, content string) MarkdownExport {
	result := MarkdownExport{
		ContractVersion: StateJSONContractVersion,
		ExportKind:      kind,
		Format:          ExportFormatMarkdown,
		Audience:        ctx.Audience,
		DatabaseScope:   firstNonEmpty(ctx.DatabaseScope, "global"),
		ExportScope:     firstNonEmpty(ctx.ExportScope, "project"),
		ProjectID:       ctx.ProjectID,
		ProjectName:     ctx.ProjectName,
		Content:         content,
	}
	if ctx.Audience == ExportAudienceLocal {
		result.ProjectCurrentPath = ctx.ProjectCurrentPath
		result.DatabasePath = ctx.DatabasePath
	}
	return result
}

func renderMarkdownExportContext(b *strings.Builder, ctx markdownExportContext) {
	b.WriteString("## Project Context\n\n")
	fmt.Fprintf(b, "- Scope: %s database, %s export\n", firstNonEmpty(ctx.DatabaseScope, "global"), firstNonEmpty(ctx.ExportScope, "project"))
	if ctx.ProjectID != "" {
		fmt.Fprintf(b, "- Project: `%s`\n", ctx.ProjectID)
	}
	if ctx.ProjectName != "" {
		projectName := ctx.ProjectName
		if ctx.Audience == ExportAudienceExternal {
			projectName = sanitizeExternalText(projectName)
		}
		fmt.Fprintf(b, "- Project name: %s\n", projectName)
	}
	if ctx.Audience == ExportAudienceLocal {
		if ctx.ProjectCurrentPath != "" {
			fmt.Fprintf(b, "- Project path: `%s`\n", ctx.ProjectCurrentPath)
		}
		if ctx.DatabasePath != "" {
			fmt.Fprintf(b, "- Database: `%s`\n", ctx.DatabasePath)
		}
	}
	b.WriteString("\n")
}

func renderTriageMarkdown(ctx markdownExportContext, ideas IdeaList, sparks SparkList, brainstorms BrainstormList) string {
	var b strings.Builder
	b.WriteString("# Triage Export\n\n")
	b.WriteString("Audience: external\n")
	b.WriteString("Source: Loaf SQLite state\n\n")
	renderMarkdownExportContext(&b, ctx)
	renderIdeaExportSection(&b, ideas)
	renderSparkExportSection(&b, sparks)
	renderBrainstormExportSection(&b, brainstorms)
	return b.String()
}

func renderReleaseReadinessMarkdown(ctx markdownExportContext, data releaseReadinessExportData) string {
	specActive, specComplete, specArchived := releaseSpecStatusCounts(data.Specs)
	taskUnresolved, taskDone, taskArchived := releaseTaskStatusCounts(data.Tasks)
	activeSessions := releaseSessionStatusCount(data.Sessions, "active")
	draftReports := releaseReportStatusCount(data.Reports, "draft")
	warnings := releaseReadinessWarnings(data, specActive, specComplete, taskUnresolved, taskDone, activeSessions, draftReports)
	ready := len(warnings) == 0

	var b strings.Builder
	b.WriteString("# Release Readiness Export\n\n")
	b.WriteString("Audience: external\n")
	b.WriteString("Source: Loaf SQLite state\n\n")
	renderMarkdownExportContext(&b, ctx)

	b.WriteString("## State\n\n")
	b.WriteString("- SQLite state: ready\n")
	fmt.Fprintf(&b, "- Schema version: %d\n", data.SchemaVersion)
	if ready {
		b.WriteString("- Release readiness: ready\n\n")
	} else {
		b.WriteString("- Release readiness: not ready\n\n")
	}

	b.WriteString("## Work Status\n\n")
	fmt.Fprintf(&b, "- Specs: %d active, %d complete, %d archived\n", specActive, specComplete, specArchived)
	fmt.Fprintf(&b, "- Tasks: %d unresolved, %d done, %d archived\n", taskUnresolved, taskDone, taskArchived)
	fmt.Fprintf(&b, "- Sessions: %d active, %d total\n", activeSessions, len(data.Sessions.Sessions))
	fmt.Fprintf(&b, "- Reports: %d draft, %d total\n\n", draftReports, len(data.Reports.Reports))

	b.WriteString("## Warnings\n\n")
	if len(warnings) == 0 {
		b.WriteString("- No readiness warnings found.\n\n")
	} else {
		for _, warning := range warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Source Provenance\n\n")
	for _, coverage := range data.SourceCoverage {
		fmt.Fprintf(&b, "- %s: %d/%d with source\n", coverage.Label, coverage.With, coverage.Total)
	}
	b.WriteString("\n")

	b.WriteString("## Relationships\n\n")
	if len(data.RelationshipCounts) == 0 {
		b.WriteString("No relationships recorded.\n\n")
	} else {
		total := 0
		for _, count := range data.RelationshipCounts {
			total += count.Count
		}
		fmt.Fprintf(&b, "- Total relationships: %d\n", total)
		for _, count := range data.RelationshipCounts {
			fmt.Fprintf(&b, "- %s: %d\n", sanitizeExternalText(count.Label), count.Count)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Generated Exports\n\n")
	if len(data.ExportCounts) == 0 {
		b.WriteString("No generated exports recorded.\n\n")
	} else {
		for _, count := range data.ExportCounts {
			fmt.Fprintf(&b, "- %s: %d\n", sanitizeExternalText(count.Label), count.Count)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Recent Reports\n\n")
	if len(data.RecentReports) == 0 {
		b.WriteString("No reports recorded.\n\n")
	} else {
		for _, report := range data.RecentReports {
			fmt.Fprintf(&b, "- %s/%s: %s\n", sanitizeExternalText(report.Kind), sanitizeExternalText(report.Status), sanitizeExternalText(report.Title))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Recent Sessions\n\n")
	if len(data.RecentSessions) == 0 {
		b.WriteString("No sessions recorded.\n")
	} else {
		for _, session := range data.RecentSessions {
			branch := sanitizeExternalText(session.Branch)
			if branch == "" {
				fmt.Fprintf(&b, "- %s session with %d journal %s\n", sanitizeExternalText(session.Status), session.JournalEntries, pluralize(session.JournalEntries, "entry", "entries"))
			} else {
				fmt.Fprintf(&b, "- %s session on %s with %d journal %s\n", sanitizeExternalText(session.Status), branch, session.JournalEntries, pluralize(session.JournalEntries, "entry", "entries"))
			}
		}
	}
	return b.String()
}

func renderSpecMarkdown(ctx markdownExportContext, spec SpecDetail) string {
	var b strings.Builder
	b.WriteString("# Spec Export\n\n")
	b.WriteString("Audience: internal\n")
	b.WriteString("Source: Loaf SQLite state\n\n")
	renderMarkdownExportContext(&b, ctx)
	b.WriteString("## Spec\n\n")
	fmt.Fprintf(&b, "- Spec: `%s`\n", firstNonEmpty(spec.Alias, spec.ID))
	fmt.Fprintf(&b, "- Title: %s\n", spec.Title)
	fmt.Fprintf(&b, "- Status: %s\n", spec.Status)
	fmt.Fprintf(&b, "- Tasks: %d todo, %d in progress, %d done\n", spec.Tasks.Todo, spec.Tasks.InProgress, spec.Tasks.Done)
	if spec.CreatedAt != "" {
		fmt.Fprintf(&b, "- Created: %s\n", spec.CreatedAt)
	}
	if spec.UpdatedAt != "" {
		fmt.Fprintf(&b, "- Updated: %s\n", spec.UpdatedAt)
	}
	b.WriteString("\n")

	b.WriteString("## Sources\n\n")
	if len(spec.Sources) == 0 {
		b.WriteString("No sources recorded.\n\n")
	} else {
		for _, source := range spec.Sources {
			fmt.Fprintf(&b, "- `%s`", source.Path)
			if source.Hash != "" {
				fmt.Fprintf(&b, " `%s`", source.Hash)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Relationships\n\n")
	if len(spec.Relationships) == 0 {
		b.WriteString("No relationships recorded.\n\n")
	} else {
		for _, relationship := range spec.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(&b, "- %s `%s` %s `%s`", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(&b, " - %s", relationship.Reason)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Body\n\n")
	if strings.TrimSpace(spec.Body) == "" {
		b.WriteString("No imported body recorded.\n")
	} else {
		b.WriteString(strings.TrimSpace(spec.Body))
		b.WriteString("\n")
	}
	return b.String()
}

func renderSessionMarkdown(ctx markdownExportContext, session SessionDetail) string {
	var b strings.Builder
	b.WriteString("# Session Export\n\n")
	b.WriteString("Audience: internal\n")
	b.WriteString("Source: Loaf SQLite state\n\n")
	renderMarkdownExportContext(&b, ctx)
	b.WriteString("## Session\n\n")
	fmt.Fprintf(&b, "- Session: `%s`\n", firstNonEmpty(session.Alias, session.ID))
	fmt.Fprintf(&b, "- Status: %s\n", session.Status)
	if session.Branch != "" {
		fmt.Fprintf(&b, "- Branch: `%s`\n", session.Branch)
	}
	if session.HarnessSessionID != "" {
		fmt.Fprintf(&b, "- Harness session: `%s`\n", session.HarnessSessionID)
	}
	if session.CreatedAt != "" {
		fmt.Fprintf(&b, "- Created: %s\n", session.CreatedAt)
	}
	if session.UpdatedAt != "" {
		fmt.Fprintf(&b, "- Updated: %s\n", session.UpdatedAt)
	}
	b.WriteString("\n")

	b.WriteString("## Sources\n\n")
	if len(session.Sources) == 0 {
		b.WriteString("No sources recorded.\n\n")
	} else {
		for _, source := range session.Sources {
			fmt.Fprintf(&b, "- `%s`", source.Path)
			if source.Hash != "" {
				fmt.Fprintf(&b, " `%s`", source.Hash)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Journal Entries\n\n")
	if len(session.JournalEntries) == 0 {
		b.WriteString("No journal entries recorded.\n\n")
	} else {
		for _, entry := range session.JournalEntries {
			label := entry.EntryType
			if entry.Scope != "" {
				label = fmt.Sprintf("%s(%s)", entry.EntryType, entry.Scope)
			}
			fmt.Fprintf(&b, "- `%s`: %s\n", label, entry.Message)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Relationships\n\n")
	if len(session.Relationships) == 0 {
		b.WriteString("No relationships recorded.\n")
	} else {
		for _, relationship := range session.Relationships {
			target := firstNonEmpty(relationship.Entity.Alias, relationship.Entity.ID)
			fmt.Fprintf(&b, "- %s `%s` %s `%s`", relationship.Direction, relationship.Type, relationship.Entity.Kind, target)
			if relationship.Reason != "" {
				fmt.Fprintf(&b, " - %s", relationship.Reason)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderIdeaExportSection(b *strings.Builder, ideas IdeaList) {
	b.WriteString("## Ideas\n\n")
	if len(ideas.Ideas) == 0 {
		b.WriteString("No ideas found.\n\n")
		return
	}
	for _, status := range sortedIdeaStatuses(ideas) {
		fmt.Fprintf(b, "### %s\n\n", exportStatusHeading(status))
		for _, title := range sortedIdeaTitlesForStatus(ideas, status) {
			fmt.Fprintf(b, "- %s\n", sanitizeExternalText(title))
		}
		b.WriteString("\n")
	}
}

func renderSparkExportSection(b *strings.Builder, sparks SparkList) {
	b.WriteString("## Sparks\n\n")
	if len(sparks.Sparks) == 0 {
		b.WriteString("No sparks found.\n\n")
		return
	}
	for _, status := range sortedSparkStatuses(sparks) {
		fmt.Fprintf(b, "### %s\n\n", exportStatusHeading(status))
		for _, text := range sortedSparkTextsForStatus(sparks, status) {
			fmt.Fprintf(b, "- %s\n", sanitizeExternalText(text))
		}
		b.WriteString("\n")
	}
}

func renderBrainstormExportSection(b *strings.Builder, brainstorms BrainstormList) {
	b.WriteString("## Brainstorms\n\n")
	if len(brainstorms.Brainstorms) == 0 {
		b.WriteString("No brainstorms found.\n\n")
		return
	}
	for _, status := range sortedBrainstormStatuses(brainstorms) {
		fmt.Fprintf(b, "### %s\n\n", exportStatusHeading(status))
		for _, title := range sortedBrainstormTitlesForStatus(brainstorms, status) {
			fmt.Fprintf(b, "- %s\n", sanitizeExternalText(title))
		}
		b.WriteString("\n")
	}
}

func releaseSpecStatusCounts(specs SpecList) (active int, complete int, archived int) {
	for _, spec := range specs.Specs {
		switch spec.Status {
		case "complete":
			complete++
		case "archived":
			archived++
		default:
			active++
		}
	}
	return active, complete, archived
}

func releaseTaskStatusCounts(tasks TaskList) (unresolved int, done int, archived int) {
	for _, task := range tasks.Tasks {
		switch task.Status {
		case "done":
			done++
		case "archived":
			archived++
		default:
			unresolved++
		}
	}
	return unresolved, done, archived
}

func releaseSessionStatusCount(sessions SessionList, status string) int {
	count := 0
	for _, session := range sessions.Sessions {
		if session.Status == status {
			count++
		}
	}
	return count
}

func releaseReportStatusCount(reports ReportList, status string) int {
	count := 0
	for _, report := range reports.Reports {
		if report.Status == status {
			count++
		}
	}
	return count
}

func releaseReadinessWarnings(data releaseReadinessExportData, specActive int, specComplete int, taskUnresolved int, taskDone int, activeSessions int, draftReports int) []string {
	warnings := []string{}
	if taskUnresolved > 0 {
		warnings = append(warnings, fmt.Sprintf("Unresolved work remains: %d task %s not done or archived.", taskUnresolved, pluralize(taskUnresolved, "is", "are")))
	}
	if specActive > 0 {
		warnings = append(warnings, fmt.Sprintf("Active specs remain: %d spec %s not complete or archived.", specActive, pluralize(specActive, "is", "are")))
	}
	if taskDone > 0 {
		warnings = append(warnings, fmt.Sprintf("Archive candidate: %d completed task %s still active.", taskDone, pluralize(taskDone, "is", "are")))
	}
	if specComplete > 0 {
		warnings = append(warnings, fmt.Sprintf("Archive candidate: %d complete spec %s still active.", specComplete, pluralize(specComplete, "is", "are")))
	}
	if activeSessions > 0 {
		warnings = append(warnings, fmt.Sprintf("Active sessions remain: %d session %s active.", activeSessions, pluralize(activeSessions, "is", "are")))
	}
	if draftReports > 0 {
		warnings = append(warnings, fmt.Sprintf("Draft reports remain: %d report %s not final.", draftReports, pluralize(draftReports, "is", "are")))
	}
	for _, coverage := range data.SourceCoverage {
		if coverage.With < coverage.Total {
			warnings = append(warnings, fmt.Sprintf("Source provenance incomplete for %s: %d/%d rows have source links.", strings.ToLower(coverage.Label), coverage.With, coverage.Total))
		}
	}
	if len(data.ExportCounts) == 0 {
		warnings = append(warnings, "No generated export records are present in SQLite state.")
	}
	return warnings
}

func sortedIdeaStatuses(ideas IdeaList) []string {
	statuses := map[string]bool{}
	for _, idea := range ideas.Ideas {
		statuses[idea.Status] = true
	}
	return sortedStatusKeys(statuses)
}

func sortedSparkStatuses(sparks SparkList) []string {
	statuses := map[string]bool{}
	for _, spark := range sparks.Sparks {
		statuses[spark.Status] = true
	}
	return sortedStatusKeys(statuses)
}

func sortedBrainstormStatuses(brainstorms BrainstormList) []string {
	statuses := map[string]bool{}
	for _, brainstorm := range brainstorms.Brainstorms {
		statuses[brainstorm.Status] = true
	}
	return sortedStatusKeys(statuses)
}

func sortedStatusKeys(statuses map[string]bool) []string {
	preferred := []string{"open", "todo", "in_progress", "review", "resolved", "archived", "done"}
	result := []string{}
	for _, status := range preferred {
		if statuses[status] {
			result = append(result, status)
			delete(statuses, status)
		}
	}
	remaining := make([]string, 0, len(statuses))
	for status := range statuses {
		remaining = append(remaining, status)
	}
	sort.Strings(remaining)
	return append(result, remaining...)
}

func sortedIdeaTitlesForStatus(ideas IdeaList, status string) []string {
	titles := []string{}
	for _, idea := range ideas.Ideas {
		if idea.Status == status {
			titles = append(titles, idea.Title)
		}
	}
	sort.Strings(titles)
	return titles
}

func sortedSparkTextsForStatus(sparks SparkList, status string) []string {
	texts := []string{}
	for _, spark := range sparks.Sparks {
		if spark.Status == status {
			if spark.Scope != "" {
				texts = append(texts, fmt.Sprintf("%s: %s", spark.Scope, spark.Text))
			} else {
				texts = append(texts, spark.Text)
			}
		}
	}
	sort.Strings(texts)
	return texts
}

func sortedBrainstormTitlesForStatus(brainstorms BrainstormList, status string) []string {
	titles := []string{}
	for _, brainstorm := range brainstorms.Brainstorms {
		if brainstorm.Status == status {
			titles = append(titles, brainstorm.Title)
		}
	}
	sort.Strings(titles)
	return titles
}

func exportStatusHeading(status string) string {
	if status == "" {
		return "Unspecified"
	}
	words := strings.Fields(strings.ReplaceAll(status, "_", " "))
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func sanitizeExternalText(value string) string {
	cleaned := strings.TrimSpace(value)
	for _, pattern := range externalLeakPatterns {
		cleaned = pattern.ReplaceAllString(cleaned, "internal reference")
	}
	return cleaned
}

func pluralize(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
