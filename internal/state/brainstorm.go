package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// BrainstormList is the state-backed brainstorm-list read model.
type BrainstormList struct {
	ContractVersion    int                       `json:"contract_version,omitempty"`
	DatabaseScope      string                    `json:"database_scope,omitempty"`
	DatabasePath       string                    `json:"database_path,omitempty"`
	ProjectID          string                    `json:"project_id,omitempty"`
	ProjectName        string                    `json:"project_name,omitempty"`
	ProjectCurrentPath string                    `json:"project_current_path,omitempty"`
	Version            int                       `json:"version"`
	Brainstorms        map[string]BrainstormItem `json:"brainstorms"`
}

// BrainstormItem is a brainstorm entry returned by the state-backed brainstorm list.
type BrainstormItem struct {
	Title      string `json:"title"`
	Status     string `json:"status"`
	SourcePath string `json:"source_path,omitempty"`
}

// BrainstormListOptions filter the state-backed brainstorm list.
type BrainstormListOptions struct {
	All    bool
	Status string
}

// BrainstormCaptureOptions describes a SQLite-backed brainstorm capture request.
type BrainstormCaptureOptions struct {
	Title string
	Body  string
}

// BrainstormCaptureResult describes a captured SQLite-backed brainstorm.
type BrainstormCaptureResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Brainstorm         TraceEntity `json:"brainstorm"`
	EventID            string      `json:"event_id"`
}

// ListBrainstorms returns imported brainstorms from initialized SQLite state.
func ListBrainstorms(ctx context.Context, root project.Root, resolver PathResolver, options BrainstormListOptions) (BrainstormList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BrainstormList{}, err
	}
	defer store.Close()
	return store.ListBrainstorms(ctx, root, options)
}

// ListBrainstorms returns imported brainstorms from an open store.
func (s *Store) ListBrainstorms(ctx context.Context, root project.Root, options BrainstormListOptions) (BrainstormList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BrainstormList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BrainstormList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  brainstorm_alias.alias,
  brainstorms.title,
  brainstorms.status,
  COALESCE(sources.path, '')
FROM brainstorms
JOIN aliases brainstorm_alias
  ON brainstorm_alias.project_id = brainstorms.project_id
 AND brainstorm_alias.entity_kind = 'brainstorm'
 AND brainstorm_alias.entity_id = brainstorms.id
 AND brainstorm_alias.namespace = 'brainstorm'
LEFT JOIN sources ON sources.id = brainstorms.body_source_id
WHERE brainstorms.project_id = ?
ORDER BY brainstorm_alias.alias
`, projectID)
	if err != nil {
		return BrainstormList{}, fmt.Errorf("query brainstorms: %w", err)
	}

	brainstorms := BrainstormList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Brainstorms:        map[string]BrainstormItem{},
	}
	for rows.Next() {
		var alias, title, status, sourcePath string
		if err := rows.Scan(&alias, &title, &status, &sourcePath); err != nil {
			rows.Close()
			return BrainstormList{}, fmt.Errorf("scan brainstorm: %w", err)
		}
		if !includeBrainstormStatus(status, options) {
			continue
		}
		status = LifecycleStatusForDisplay(LifecycleEntityBrainstorm, status)
		brainstorms.Brainstorms[alias] = BrainstormItem{
			Title:      title,
			Status:     status,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return BrainstormList{}, fmt.Errorf("close brainstorms: %w", err)
	}
	if err := rows.Err(); err != nil {
		return BrainstormList{}, fmt.Errorf("iterate brainstorms: %w", err)
	}
	return brainstorms, nil
}

func includeBrainstormStatus(status string, options BrainstormListOptions) bool {
	if !LifecycleStatusFilterMatches(LifecycleEntityBrainstorm, status, options.Status) {
		return false
	}
	if options.Status != "" {
		return true
	}
	if !options.All && (LifecycleStatusMatches(LifecycleEntityBrainstorm, status, LifecycleStatusDone) || LifecycleStatusMatches(LifecycleEntityBrainstorm, status, LifecycleStatusArchived)) {
		return false
	}
	return true
}

// CaptureBrainstorm captures a brainstorm in initialized SQLite state.
func CaptureBrainstorm(ctx context.Context, root project.Root, resolver PathResolver, options BrainstormCaptureOptions) (BrainstormCaptureResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BrainstormCaptureResult{}, err
	}
	defer store.Close()
	return store.CaptureBrainstorm(ctx, root, options)
}

// CaptureBrainstorm captures a brainstorm in an open store.
func (s *Store) CaptureBrainstorm(ctx context.Context, root project.Root, options BrainstormCaptureOptions) (BrainstormCaptureResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BrainstormCaptureResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BrainstormCaptureResult{}, err
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return BrainstormCaptureResult{}, fmt.Errorf("brainstorm capture requires --title")
	}
	if strings.TrimSpace(options.Body) == "" {
		return BrainstormCaptureResult{}, fmt.Errorf("brainstorm capture requires body content")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BrainstormCaptureResult{}, fmt.Errorf("begin brainstorm capture transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	alias, err := s.nextBrainstormAlias(ctx, tx, projectID, title, now)
	if err != nil {
		return BrainstormCaptureResult{}, err
	}
	brainstormID := stableMigrationID("brainstorm", projectID, alias)
	timestamp := now.Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO brainstorms (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, 'open', NULL, ?, ?)
`, brainstormID, projectID, title, timestamp, timestamp); err != nil {
		return BrainstormCaptureResult{}, fmt.Errorf("insert brainstorm %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "brainstorm", brainstormID, "brainstorm", alias, timestamp); err != nil {
		return BrainstormCaptureResult{}, err
	}
	eventID := stableMigrationID("event", projectID, "brainstorm", brainstormID, "created", "open")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'brainstorm', ?, 'status_changed', NULL, 'open', 'recorded by brainstorm capture', ?, ?)
`, eventID, projectID, brainstormID, timestamp, timestamp); err != nil {
		return BrainstormCaptureResult{}, fmt.Errorf("record brainstorm capture event: %w", err)
	}
	if _, err := upsertArtifactBodyTx(ctx, tx, projectID, "brainstorm", brainstormID, ArtifactBodyKindMarkdown, options.Body, nil, timestamp); err != nil {
		return BrainstormCaptureResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return BrainstormCaptureResult{}, fmt.Errorf("commit brainstorm capture transaction: %w", err)
	}

	return BrainstormCaptureResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Brainstorm:         TraceEntity{Kind: "brainstorm", ID: brainstormID, Alias: alias, Title: title, Status: "open"},
		EventID:            eventID,
	}, nil
}

func (s *Store) nextBrainstormAlias(ctx context.Context, tx *sql.Tx, projectID string, title string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(title)
	if slug == "" {
		slug = "brainstorm"
	}
	prefix := "BRAINSTORM-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'brainstorm' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check brainstorm alias %s: %w", alias, err)
		}
	}
}
