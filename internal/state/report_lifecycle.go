package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

var reportSlugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

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

// CreateReport creates a draft report in initialized SQLite state.
func CreateReport(ctx context.Context, root project.Root, resolver PathResolver, options ReportCreateOptions) (ReportCreateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ReportCreateResult{}, err
	}
	defer store.Close()
	return store.CreateReport(ctx, root, options)
}

// FinalizeReport transitions a draft report to final in initialized SQLite state.
func FinalizeReport(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ReportStatusResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ReportStatusResult{}, err
	}
	defer store.Close()
	return store.FinalizeReport(ctx, root, ref)
}

// ArchiveReport transitions a final report to archived in initialized SQLite state.
func ArchiveReport(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ReportStatusResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ReportStatusResult{}, err
	}
	defer store.Close()
	return store.ArchiveReport(ctx, root, ref)
}

// CreateReport creates a draft report in an open store.
func (s *Store) CreateReport(ctx context.Context, root project.Root, options ReportCreateOptions) (ReportCreateResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ReportCreateResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ReportCreateResult{}, err
	}
	slug := normalizeReportSlug(options.Slug)
	if slug == "" {
		return ReportCreateResult{}, fmt.Errorf("report create requires a slug")
	}
	kind := strings.TrimSpace(options.Kind)
	if kind == "" {
		kind = "research"
	}
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = "ad-hoc"
	}
	title := reportTitleFromSlug(slug)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReportCreateResult{}, fmt.Errorf("begin report create transaction: %w", err)
	}
	defer tx.Rollback()

	alias, err := s.nextReportAlias(ctx, tx, projectID, slug)
	if err != nil {
		return ReportCreateResult{}, err
	}
	reportID := stableMigrationID("report", projectID, alias)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.ExecContext(ctx, `
INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, 'draft', NULL, ?, ?)
`, reportID, projectID, kind, title, now, now)
	if err != nil {
		return ReportCreateResult{}, fmt.Errorf("insert report %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "report", reportID, "report", alias, now); err != nil {
		return ReportCreateResult{}, err
	}

	eventID := stableMigrationID("event", projectID, "report", reportID, "created", "draft")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'report', ?, 'status_changed', NULL, 'draft', ?, ?, ?)
`, eventID, projectID, reportID, reportCreateEventNote(source), now, now); err != nil {
		return ReportCreateResult{}, fmt.Errorf("record report create event: %w", err)
	}
	if options.SetBody {
		if _, err := upsertArtifactBodyTx(ctx, tx, projectID, "report", reportID, ArtifactBodyKindMarkdown, options.Body, nil, now); err != nil {
			return ReportCreateResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return ReportCreateResult{}, fmt.Errorf("commit report create transaction: %w", err)
	}

	return ReportCreateResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Report:             TraceEntity{Kind: "report", ID: reportID, Alias: alias, Title: title, Status: "draft"},
		Kind:               kind,
		Source:             source,
		EventID:            eventID,
	}, nil
}

// FinalizeReport transitions a draft report to final in an open store.
func (s *Store) FinalizeReport(ctx context.Context, root project.Root, ref string) (ReportStatusResult, error) {
	return s.updateReportStatus(ctx, root, ref, "draft", "final", "finalize")
}

// ArchiveReport transitions a final report to archived in an open store.
func (s *Store) ArchiveReport(ctx context.Context, root project.Root, ref string) (ReportStatusResult, error) {
	return s.updateReportStatus(ctx, root, ref, "final", "archived", "archive")
}

func (s *Store) updateReportStatus(ctx context.Context, root project.Root, ref string, requiredStatus string, nextStatus string, command string) (ReportStatusResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ReportStatusResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ReportStatusResult{}, err
	}
	report, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return ReportStatusResult{}, err
	}
	if report.Kind != "report" {
		return ReportStatusResult{}, fmt.Errorf("%q resolves to %s, not report", ref, report.Kind)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReportStatusResult{}, fmt.Errorf("begin report %s transaction: %w", command, err)
	}
	defer tx.Rollback()

	var title, previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT title, status FROM reports WHERE project_id = ? AND id = ?`, projectID, report.ID).Scan(&title, &previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ReportStatusResult{}, fmt.Errorf("report %q not found in SQLite state", ref)
	}
	if err != nil {
		return ReportStatusResult{}, fmt.Errorf("read report metadata: %w", err)
	}
	if previousStatus != requiredStatus {
		return ReportStatusResult{}, fmt.Errorf("report %q is not %s (status: %s)", firstNonEmpty(report.Alias, ref), requiredStatus, previousStatus)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE reports SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, nextStatus, now, projectID, report.ID); err != nil {
		return ReportStatusResult{}, fmt.Errorf("update report status: %w", err)
	}

	eventID := stableMigrationID("event", projectID, "report", report.ID, "status", previousStatus, nextStatus)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'report', ?, 'status_changed', ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, report.ID, previousStatus, nextStatus, "recorded by report "+command, now, now); err != nil {
		return ReportStatusResult{}, fmt.Errorf("record report status event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ReportStatusResult{}, fmt.Errorf("commit report %s transaction: %w", command, err)
	}

	report.Title = title
	report.Status = nextStatus
	return ReportStatusResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Report:             report,
		Previous:           previousStatus,
		Status:             nextStatus,
		EventID:            eventID,
	}, nil
}

func (s *Store) nextReportAlias(ctx context.Context, tx *sql.Tx, projectID string, slug string) (string, error) {
	base := "report-" + slug
	for suffix := 0; ; suffix++ {
		alias := base
		if suffix > 0 {
			alias = fmt.Sprintf("%s-%d", base, suffix+1)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'report' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check report alias %s: %w", alias, err)
		}
	}
}

func normalizeReportSlug(slug string) string {
	normalized := strings.ToLower(strings.TrimSpace(slug))
	normalized = reportSlugCleaner.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func reportTitleFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func reportCreateEventNote(source string) string {
	return fmt.Sprintf("recorded by report create; source=%s", source)
}
