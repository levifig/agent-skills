package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// ArtifactEntityCreateOptions describes a native plan, handoff, or council creation request.
type ArtifactEntityCreateOptions struct {
	Kind             string
	Title            string
	Body             string
	Spec             string
	HarnessSessionID string
	Task             string
}

// ArtifactEntityListOptions filters native plan, handoff, or council lists.
type ArtifactEntityListOptions struct {
	Kind   string
	Status string
	All    bool
}

// ArtifactEntityCreateResult describes a created native artifact.
type ArtifactEntityCreateResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Entity             TraceEntity `json:"entity"`
	EventID            string      `json:"event_id"`
}

// ArtifactEntityList is the state-backed native artifact list model.
type ArtifactEntityList struct {
	ContractVersion    int                           `json:"contract_version,omitempty"`
	DatabaseScope      string                        `json:"database_scope,omitempty"`
	DatabasePath       string                        `json:"database_path,omitempty"`
	ProjectID          string                        `json:"project_id,omitempty"`
	ProjectName        string                        `json:"project_name,omitempty"`
	ProjectCurrentPath string                        `json:"project_current_path,omitempty"`
	Kind               string                        `json:"kind"`
	Entities           map[string]ArtifactEntityItem `json:"entities"`
}

// ArtifactEntityItem is a compact native artifact list row.
type ArtifactEntityItem struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ArtifactEntityShow is the state-backed single native artifact read model.
type ArtifactEntityShow struct {
	ContractVersion    int                  `json:"contract_version,omitempty"`
	DatabaseScope      string               `json:"database_scope,omitempty"`
	DatabasePath       string               `json:"database_path,omitempty"`
	ProjectID          string               `json:"project_id,omitempty"`
	ProjectName        string               `json:"project_name,omitempty"`
	ProjectCurrentPath string               `json:"project_current_path,omitempty"`
	Query              string               `json:"query"`
	Entity             ArtifactEntityDetail `json:"entity"`
}

// ArtifactEntityDetail contains native artifact metadata plus body content.
type ArtifactEntityDetail struct {
	ID            string              `json:"id"`
	Kind          string              `json:"kind"`
	Alias         string              `json:"alias,omitempty"`
	Title         string              `json:"title"`
	Status        string              `json:"status"`
	Sources       []TraceSource       `json:"sources"`
	Body          string              `json:"body,omitempty"`
	Relationships []TraceRelationship `json:"relationships"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// CreateArtifactEntity creates a plan, handoff, or council in initialized SQLite state.
func CreateArtifactEntity(ctx context.Context, root project.Root, resolver PathResolver, options ArtifactEntityCreateOptions) (ArtifactEntityCreateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	defer store.Close()
	return store.CreateArtifactEntity(ctx, root, options)
}

// ListArtifactEntities lists plans, handoffs, or councils in initialized SQLite state.
func ListArtifactEntities(ctx context.Context, root project.Root, resolver PathResolver, options ArtifactEntityListOptions) (ArtifactEntityList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ArtifactEntityList{}, err
	}
	defer store.Close()
	return store.ListArtifactEntities(ctx, root, options)
}

// ShowArtifactEntity returns one plan, handoff, or council from initialized SQLite state.
func ShowArtifactEntity(ctx context.Context, root project.Root, resolver PathResolver, kind string, ref string) (ArtifactEntityShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return ArtifactEntityShow{}, err
	}
	defer store.Close()
	return store.ShowArtifactEntity(ctx, root, kind, ref)
}

// CreateArtifactEntity creates a plan, handoff, or council in an open store.
func (s *Store) CreateArtifactEntity(ctx context.Context, root project.Root, options ArtifactEntityCreateOptions) (ArtifactEntityCreateResult, error) {
	kind, table, err := normalizeArtifactEntityKind(options.Kind)
	if err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return ArtifactEntityCreateResult{}, fmt.Errorf("%s new requires --title", kind)
	}
	if strings.TrimSpace(options.Body) == "" {
		return ArtifactEntityCreateResult{}, fmt.Errorf("%s new requires body content", kind)
	}
	specID, harnessSessionID, taskID, err := s.resolveArtifactEntityContext(ctx, projectID, kind, options)
	if err != nil {
		return ArtifactEntityCreateResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ArtifactEntityCreateResult{}, fmt.Errorf("begin %s create transaction: %w", kind, err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)
	alias, err := s.nextArtifactEntityAlias(ctx, tx, projectID, kind, title, now)
	if err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	id := stableMigrationID(kind, projectID, alias)
	switch kind {
	case "plan", "council":
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (id, project_id, spec_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, NULL, ?, ?)
`, table), id, projectID, emptyToNil(specID), title, LifecycleStatusDraft, timestamp, timestamp); err != nil {
			return ArtifactEntityCreateResult{}, fmt.Errorf("insert %s %s: %w", kind, alias, err)
		}
	case "handoff":
		// The handoff correlation column is journal-first (harness_session_id)
		// after the SPEC-056 migration but session_id on the pre-migration
		// schema. Match the live table so the write works on both shapes.
		correlationColumn, err := handoffCorrelationColumn(ctx, tx)
		if err != nil {
			return ArtifactEntityCreateResult{}, err
		}
		insertHandoff := fmt.Sprintf(`
INSERT INTO handoffs (id, project_id, %s, task_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, correlationColumn)
		if _, err := tx.ExecContext(ctx, insertHandoff, id, projectID, emptyToNil(harnessSessionID), emptyToNil(taskID), title, LifecycleStatusDraft, timestamp, timestamp); err != nil {
			return ArtifactEntityCreateResult{}, fmt.Errorf("insert handoff %s: %w", alias, err)
		}
	default:
		return ArtifactEntityCreateResult{}, fmt.Errorf("unsupported artifact entity kind %q", kind)
	}
	if err := insertAlias(ctx, tx, projectID, kind, id, kind, alias, timestamp); err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	eventID := stableMigrationID("event", projectID, kind, id, "created", LifecycleStatusDraft)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, 'status_changed', NULL, ?, ?, ?, ?)
`, eventID, projectID, kind, id, LifecycleStatusDraft, "recorded by "+kind+" new", timestamp, timestamp); err != nil {
		return ArtifactEntityCreateResult{}, fmt.Errorf("record %s create event: %w", kind, err)
	}
	if _, err := upsertArtifactBodyTx(ctx, tx, projectID, kind, id, ArtifactBodyKindMarkdown, options.Body, nil, timestamp); err != nil {
		return ArtifactEntityCreateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ArtifactEntityCreateResult{}, fmt.Errorf("commit %s create transaction: %w", kind, err)
	}

	return ArtifactEntityCreateResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Entity:             TraceEntity{Kind: kind, ID: id, Alias: alias, Title: title, Status: LifecycleStatusDraft},
		EventID:            eventID,
	}, nil
}

// ListArtifactEntities lists plans, handoffs, or councils from an open store.
func (s *Store) ListArtifactEntities(ctx context.Context, root project.Root, options ArtifactEntityListOptions) (ArtifactEntityList, error) {
	kind, table, err := normalizeArtifactEntityKind(options.Kind)
	if err != nil {
		return ArtifactEntityList{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ArtifactEntityList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ArtifactEntityList{}, err
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT artifact_alias.alias, %s.title, %s.status
FROM %s
JOIN aliases artifact_alias
  ON artifact_alias.project_id = %s.project_id
 AND artifact_alias.entity_kind = ?
 AND artifact_alias.entity_id = %s.id
 AND artifact_alias.namespace = ?
WHERE %s.project_id = ?
ORDER BY artifact_alias.alias
`, table, table, table, table, table, table), kind, kind, projectID)
	if err != nil {
		return ArtifactEntityList{}, fmt.Errorf("query %ss: %w", kind, err)
	}
	result := ArtifactEntityList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Kind:               kind,
		Entities:           map[string]ArtifactEntityItem{},
	}
	for rows.Next() {
		var alias, title, status string
		if err := rows.Scan(&alias, &title, &status); err != nil {
			rows.Close()
			return ArtifactEntityList{}, fmt.Errorf("scan %s: %w", kind, err)
		}
		if !LifecycleStatusFilterMatches(kind, status, options.Status) {
			continue
		}
		if !options.All && LifecycleStatusMatches(kind, status, LifecycleStatusArchived) {
			continue
		}
		status = LifecycleStatusForDisplay(kind, status)
		result.Entities[alias] = ArtifactEntityItem{Title: title, Status: status}
	}
	if err := rows.Close(); err != nil {
		return ArtifactEntityList{}, fmt.Errorf("close %ss: %w", kind, err)
	}
	if err := rows.Err(); err != nil {
		return ArtifactEntityList{}, fmt.Errorf("iterate %ss: %w", kind, err)
	}
	return result, nil
}

// ShowArtifactEntity returns one plan, handoff, or council from an open store.
func (s *Store) ShowArtifactEntity(ctx context.Context, root project.Root, kind string, ref string) (ArtifactEntityShow, error) {
	kind, table, err := normalizeArtifactEntityKind(kind)
	if err != nil {
		return ArtifactEntityShow{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ArtifactEntityShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ArtifactEntityShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return ArtifactEntityShow{}, err
	}
	if entity.Kind != kind {
		return ArtifactEntityShow{}, fmt.Errorf("%s show target %q resolved to %s, not %s", kind, ref, entity.Kind, kind)
	}
	detail, err := s.artifactEntityDetail(ctx, root, projectID, kind, table, entity)
	if err != nil {
		return ArtifactEntityShow{}, err
	}
	return ArtifactEntityShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Entity:             detail,
	}, nil
}

func (s *Store) artifactEntityDetail(ctx context.Context, root project.Root, projectID string, kind string, table string, entity TraceEntity) (ArtifactEntityDetail, error) {
	var title, status, createdAt, updatedAt string
	var sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s.title, %s.status, %s.created_at, %s.updated_at, sources.path, sources.hash
FROM %s
LEFT JOIN sources ON sources.id = %s.body_source_id
WHERE %s.project_id = ? AND %s.id = ?
`, table, table, table, table, table, table, table, table), projectID, entity.ID).Scan(&title, &status, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtifactEntityDetail{}, fmt.Errorf("%s %q not found in SQLite state", kind, firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return ArtifactEntityDetail{}, fmt.Errorf("read %s %s: %w", kind, entity.ID, err)
	}
	status = LifecycleStatusForDisplay(kind, status)
	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, kind, entity.ID); err == nil {
			alias = found
		}
	}
	sources := []TraceSource{}
	if sourcePath.Valid && sourcePath.String != "" {
		sources = append(sources, TraceSource{Path: filepath.ToSlash(sourcePath.String), Hash: sourceHash.String})
	}
	body, err := s.artifactBodyOrSourceBody(ctx, root.Path(), projectID, kind, entity.ID, sourcePath)
	if err != nil {
		return ArtifactEntityDetail{}, err
	}
	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{Kind: kind, ID: entity.ID, Alias: alias, Title: title, Status: status})
	if err != nil {
		return ArtifactEntityDetail{}, err
	}
	return ArtifactEntityDetail{
		ID:            entity.ID,
		Kind:          kind,
		Alias:         alias,
		Title:         title,
		Status:        status,
		Sources:       sources,
		Body:          body,
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Store) resolveArtifactEntityContext(ctx context.Context, projectID string, kind string, options ArtifactEntityCreateOptions) (string, string, string, error) {
	var specID, taskID string
	if options.Spec != "" {
		entity, err := s.resolveTraceEntity(ctx, projectID, options.Spec)
		if err != nil {
			return "", "", "", err
		}
		if entity.Kind != "spec" {
			return "", "", "", fmt.Errorf("--spec %q resolves to %s, not spec", options.Spec, entity.Kind)
		}
		specID = entity.ID
	}
	if options.Task != "" {
		entity, err := s.resolveTraceEntity(ctx, projectID, options.Task)
		if err != nil {
			return "", "", "", err
		}
		if entity.Kind != "task" {
			return "", "", "", fmt.Errorf("--task %q resolves to %s, not task", options.Task, entity.Kind)
		}
		taskID = entity.ID
	}
	if kind == "handoff" {
		// The harness session id is an opaque provenance tag, not an entity
		// reference: pass it through without resolution.
		return "", strings.TrimSpace(options.HarnessSessionID), taskID, nil
	}
	return specID, "", "", nil
}

// handoffCorrelationColumn returns the name of the handoffs correlation column,
// tolerating both the journal-first schema (harness_session_id) and the
// pre-migration schema (session_id).
func handoffCorrelationColumn(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(handoffs)`)
	if err != nil {
		return "", fmt.Errorf("inspect handoffs columns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return "", fmt.Errorf("scan handoffs columns: %w", err)
		}
		if name == "harness_session_id" {
			return "harness_session_id", nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate handoffs columns: %w", err)
	}
	return "session_id", nil
}

func (s *Store) nextArtifactEntityAlias(ctx context.Context, tx *sql.Tx, projectID string, kind string, title string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(title)
	if slug == "" {
		slug = kind
	}
	prefix := strings.ToUpper(kind) + "-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = ? AND alias = ?`, projectID, kind, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check %s alias %s: %w", kind, alias, err)
		}
	}
}

func normalizeArtifactEntityKind(kind string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "plan":
		return "plan", "plans", nil
	case "handoff":
		return "handoff", "handoffs", nil
	case "council":
		return "council", "councils", nil
	default:
		return "", "", fmt.Errorf("unsupported artifact entity kind %q", kind)
	}
}
