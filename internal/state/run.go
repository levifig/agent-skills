package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// RunCreateOptions describes a provenance run creation request.
type RunCreateOptions struct {
	GeneratorRef     string
	GeneratorVersion string
	GeneratorHash    string
	Status           string
	Metadata         string
	Report           string
}

// RunListOptions filters provenance runs.
type RunListOptions struct {
	Status    string
	Generator string
}

// RunCompleteOptions describes a run completion transition.
type RunCompleteOptions struct {
	Run      string
	Status   string
	Metadata string
}

// RunCreateResult describes a created provenance run.
type RunCreateResult struct {
	ContractVersion    int       `json:"contract_version,omitempty"`
	DatabaseScope      string    `json:"database_scope,omitempty"`
	DatabasePath       string    `json:"database_path,omitempty"`
	ProjectID          string    `json:"project_id,omitempty"`
	ProjectName        string    `json:"project_name,omitempty"`
	ProjectCurrentPath string    `json:"project_current_path,omitempty"`
	Run                RunDetail `json:"run"`
	EventID            string    `json:"event_id,omitempty"`
}

// RunShow describes a single provenance run.
type RunShow struct {
	ContractVersion    int       `json:"contract_version,omitempty"`
	DatabaseScope      string    `json:"database_scope,omitempty"`
	DatabasePath       string    `json:"database_path,omitempty"`
	ProjectID          string    `json:"project_id,omitempty"`
	ProjectName        string    `json:"project_name,omitempty"`
	ProjectCurrentPath string    `json:"project_current_path,omitempty"`
	Query              string    `json:"query"`
	Run                RunDetail `json:"run"`
}

// RunList describes filtered provenance runs.
type RunList struct {
	ContractVersion    int                `json:"contract_version,omitempty"`
	DatabaseScope      string             `json:"database_scope,omitempty"`
	DatabasePath       string             `json:"database_path,omitempty"`
	ProjectID          string             `json:"project_id,omitempty"`
	ProjectName        string             `json:"project_name,omitempty"`
	ProjectCurrentPath string             `json:"project_current_path,omitempty"`
	Filters            RunListOptions     `json:"filters,omitempty"`
	Runs               map[string]RunItem `json:"runs"`
}

// RunCompleteResult describes a run status transition.
type RunCompleteResult struct {
	ContractVersion    int       `json:"contract_version,omitempty"`
	DatabaseScope      string    `json:"database_scope,omitempty"`
	DatabasePath       string    `json:"database_path,omitempty"`
	ProjectID          string    `json:"project_id,omitempty"`
	ProjectName        string    `json:"project_name,omitempty"`
	ProjectCurrentPath string    `json:"project_current_path,omitempty"`
	Run                RunDetail `json:"run"`
	Previous           string    `json:"previous"`
	Status             string    `json:"status"`
	EventID            string    `json:"event_id,omitempty"`
}

// RunItem is a compact provenance run row.
type RunItem struct {
	ID               string `json:"id"`
	Alias            string `json:"alias,omitempty"`
	GeneratorRef     string `json:"generator_ref"`
	GeneratorVersion string `json:"generator_version,omitempty"`
	GeneratorHash    string `json:"generator_hash,omitempty"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at,omitempty"`
	CompletedAt      string `json:"completed_at,omitempty"`
}

// RunDetail contains run metadata and relationships.
type RunDetail struct {
	ID               string              `json:"id"`
	Alias            string              `json:"alias,omitempty"`
	GeneratorRef     string              `json:"generator_ref"`
	GeneratorVersion string              `json:"generator_version,omitempty"`
	GeneratorHash    string              `json:"generator_hash,omitempty"`
	Status           string              `json:"status"`
	Metadata         string              `json:"metadata,omitempty"`
	StartedAt        string              `json:"started_at,omitempty"`
	CompletedAt      string              `json:"completed_at,omitempty"`
	Relationships    []TraceRelationship `json:"relationships,omitempty"`
	CreatedAt        string              `json:"created_at"`
	UpdatedAt        string              `json:"updated_at"`
}

func CreateRun(ctx context.Context, root project.Root, resolver PathResolver, options RunCreateOptions) (RunCreateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return RunCreateResult{}, err
	}
	defer store.Close()
	return store.CreateRun(ctx, root, options)
}

func ShowRun(ctx context.Context, root project.Root, resolver PathResolver, ref string) (RunShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return RunShow{}, err
	}
	defer store.Close()
	return store.ShowRun(ctx, root, ref)
}

func ListRuns(ctx context.Context, root project.Root, resolver PathResolver, options RunListOptions) (RunList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return RunList{}, err
	}
	defer store.Close()
	return store.ListRuns(ctx, root, options)
}

func CompleteRun(ctx context.Context, root project.Root, resolver PathResolver, options RunCompleteOptions) (RunCompleteResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return RunCompleteResult{}, err
	}
	defer store.Close()
	return store.CompleteRun(ctx, root, options)
}

func (s *Store) CreateRun(ctx context.Context, root project.Root, options RunCreateOptions) (RunCreateResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return RunCreateResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return RunCreateResult{}, err
	}
	normalized, report, err := s.normalizeRunCreateOptions(ctx, projectID, options)
	if err != nil {
		return RunCreateResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RunCreateResult{}, fmt.Errorf("begin run create transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)
	alias, err := s.nextRunAlias(ctx, tx, projectID, normalized.GeneratorRef, now)
	if err != nil {
		return RunCreateResult{}, err
	}
	runID := stableMigrationID("run", projectID, alias)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO runs (id, project_id, generator_ref, generator_version, generator_hash, status, metadata, started_at, completed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, runID, projectID, normalized.GeneratorRef, emptyToNil(normalized.GeneratorVersion), emptyToNil(normalized.GeneratorHash), normalized.Status, emptyToNil(normalized.Metadata), timestamp, timestamp, timestamp); err != nil {
		return RunCreateResult{}, fmt.Errorf("insert run %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "run", runID, "run", alias, timestamp); err != nil {
		return RunCreateResult{}, err
	}
	if report != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'run', ?, 'report', ?, 'produces', 'recorded by run create', 'system', ?, ?)
ON CONFLICT(id) DO NOTHING
`, stableMigrationID("relationship", projectID, "run", runID, "produces", "report", report.ID), projectID, runID, report.ID, timestamp, timestamp); err != nil {
			return RunCreateResult{}, fmt.Errorf("record run report relationship: %w", err)
		}
	}
	eventID := stableMigrationID("event", projectID, "run", runID, "created", normalized.Status)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'run', ?, 'status_changed', NULL, ?, 'recorded by run create', ?, ?)
`, eventID, projectID, runID, normalized.Status, timestamp, timestamp); err != nil {
		return RunCreateResult{}, fmt.Errorf("record run create event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RunCreateResult{}, fmt.Errorf("commit run create transaction: %w", err)
	}
	detail, err := s.runDetail(ctx, projectID, TraceEntity{Kind: "run", ID: runID, Alias: alias})
	if err != nil {
		return RunCreateResult{}, err
	}
	return RunCreateResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Run:                detail,
		EventID:            eventID,
	}, nil
}

func (s *Store) ShowRun(ctx context.Context, root project.Root, ref string) (RunShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return RunShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return RunShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return RunShow{}, err
	}
	if entity.Kind != "run" {
		return RunShow{}, fmt.Errorf("run show target %q resolved to %s, not run", ref, entity.Kind)
	}
	detail, err := s.runDetail(ctx, projectID, entity)
	if err != nil {
		return RunShow{}, err
	}
	return RunShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Run:                detail,
	}, nil
}

func (s *Store) ListRuns(ctx context.Context, root project.Root, options RunListOptions) (RunList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return RunList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return RunList{}, err
	}
	normalized := options
	normalized.Status = strings.TrimSpace(options.Status)
	if normalized.Status != "" && !ValidRunStatus(normalized.Status) {
		return RunList{}, fmt.Errorf("invalid run status %q", normalized.Status)
	}
	normalized.Generator = strings.TrimSpace(options.Generator)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  runs.id,
  COALESCE(run_alias.alias, ''),
  runs.generator_ref,
  COALESCE(runs.generator_version, ''),
  COALESCE(runs.generator_hash, ''),
  runs.status,
  COALESCE(runs.started_at, ''),
  COALESCE(runs.completed_at, '')
FROM runs
LEFT JOIN aliases run_alias
  ON run_alias.project_id = runs.project_id
 AND run_alias.entity_kind = 'run'
 AND run_alias.entity_id = runs.id
 AND run_alias.namespace = 'run'
WHERE runs.project_id = ?
  AND (? = '' OR runs.status = ?)
  AND (? = '' OR runs.generator_ref = ?)
ORDER BY runs.created_at, run_alias.alias, runs.id
`, projectID, normalized.Status, normalized.Status, normalized.Generator, normalized.Generator)
	if err != nil {
		return RunList{}, fmt.Errorf("query runs: %w", err)
	}
	result := RunList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Filters:            normalized,
		Runs:               map[string]RunItem{},
	}
	for rows.Next() {
		var item RunItem
		if err := rows.Scan(&item.ID, &item.Alias, &item.GeneratorRef, &item.GeneratorVersion, &item.GeneratorHash, &item.Status, &item.StartedAt, &item.CompletedAt); err != nil {
			rows.Close()
			return RunList{}, fmt.Errorf("scan run: %w", err)
		}
		result.Runs[firstNonEmpty(item.Alias, item.ID)] = item
	}
	if err := rows.Close(); err != nil {
		return RunList{}, fmt.Errorf("close runs: %w", err)
	}
	if err := rows.Err(); err != nil {
		return RunList{}, fmt.Errorf("iterate runs: %w", err)
	}
	return result, nil
}

func (s *Store) CompleteRun(ctx context.Context, root project.Root, options RunCompleteOptions) (RunCompleteResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return RunCompleteResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return RunCompleteResult{}, err
	}
	entity, normalized, err := s.normalizeRunCompleteOptions(ctx, projectID, options)
	if err != nil {
		return RunCompleteResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RunCompleteResult{}, fmt.Errorf("begin run complete transaction: %w", err)
	}
	defer tx.Rollback()
	var previous string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM runs WHERE project_id = ? AND id = ?`, projectID, entity.ID).Scan(&previous); err != nil {
		return RunCompleteResult{}, fmt.Errorf("read run status: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(normalized.Metadata) != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, metadata = ?, completed_at = ?, updated_at = ? WHERE project_id = ? AND id = ?`, normalized.Status, normalized.Metadata, now, now, projectID, entity.ID); err != nil {
			return RunCompleteResult{}, fmt.Errorf("complete run: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, completed_at = ?, updated_at = ? WHERE project_id = ? AND id = ?`, normalized.Status, now, now, projectID, entity.ID); err != nil {
			return RunCompleteResult{}, fmt.Errorf("complete run: %w", err)
		}
	}
	eventID := stableMigrationID("event", projectID, "run", entity.ID, "status", previous, normalized.Status)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'run', ?, 'status_changed', ?, ?, 'recorded by run complete', ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, entity.ID, previous, normalized.Status, now, now); err != nil {
		return RunCompleteResult{}, fmt.Errorf("record run complete event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RunCompleteResult{}, fmt.Errorf("commit run complete transaction: %w", err)
	}
	detail, err := s.runDetail(ctx, projectID, entity)
	if err != nil {
		return RunCompleteResult{}, err
	}
	return RunCompleteResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Run:                detail,
		Previous:           previous,
		Status:             normalized.Status,
		EventID:            eventID,
	}, nil
}

func (s *Store) normalizeRunCreateOptions(ctx context.Context, projectID string, options RunCreateOptions) (RunCreateOptions, *TraceEntity, error) {
	normalized := options
	normalized.GeneratorRef = strings.TrimSpace(options.GeneratorRef)
	if normalized.GeneratorRef == "" {
		return RunCreateOptions{}, nil, fmt.Errorf("run create requires --generator")
	}
	normalized.GeneratorVersion = strings.TrimSpace(options.GeneratorVersion)
	normalized.GeneratorHash = strings.TrimSpace(options.GeneratorHash)
	normalized.Status = firstNonEmpty(strings.TrimSpace(options.Status), "pending")
	if !ValidRunStatus(normalized.Status) {
		return RunCreateOptions{}, nil, fmt.Errorf("invalid run status %q", normalized.Status)
	}
	normalized.Metadata = strings.TrimSpace(options.Metadata)
	if normalized.Metadata != "" && !jsonValid(normalized.Metadata) {
		return RunCreateOptions{}, nil, fmt.Errorf("run metadata must be valid JSON")
	}
	var report *TraceEntity
	if strings.TrimSpace(options.Report) != "" {
		resolved, err := s.resolveTraceEntity(ctx, projectID, options.Report)
		if err != nil {
			return RunCreateOptions{}, nil, err
		}
		if resolved.Kind != "report" {
			return RunCreateOptions{}, nil, fmt.Errorf("--report %q resolves to %s, not report", options.Report, resolved.Kind)
		}
		report = &resolved
	}
	return normalized, report, nil
}

func (s *Store) normalizeRunCompleteOptions(ctx context.Context, projectID string, options RunCompleteOptions) (TraceEntity, RunCompleteOptions, error) {
	if strings.TrimSpace(options.Run) == "" {
		return TraceEntity{}, RunCompleteOptions{}, fmt.Errorf("run complete requires a run")
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, options.Run)
	if err != nil {
		return TraceEntity{}, RunCompleteOptions{}, err
	}
	if entity.Kind != "run" {
		return TraceEntity{}, RunCompleteOptions{}, fmt.Errorf("run complete target %q resolved to %s, not run", options.Run, entity.Kind)
	}
	normalized := options
	normalized.Status = firstNonEmpty(strings.TrimSpace(options.Status), "completed")
	if normalized.Status != "completed" && normalized.Status != "failed" && normalized.Status != "archived" {
		return TraceEntity{}, RunCompleteOptions{}, fmt.Errorf("run complete status must be completed, failed, or archived")
	}
	normalized.Metadata = strings.TrimSpace(options.Metadata)
	if normalized.Metadata != "" && !jsonValid(normalized.Metadata) {
		return TraceEntity{}, RunCompleteOptions{}, fmt.Errorf("run metadata must be valid JSON")
	}
	return entity, normalized, nil
}

func (s *Store) runDetail(ctx context.Context, projectID string, entity TraceEntity) (RunDetail, error) {
	var generatorRef, status, createdAt, updatedAt string
	var generatorVersion, generatorHash, metadata, startedAt, completedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT generator_ref, generator_version, generator_hash, status, metadata, started_at, completed_at, created_at, updated_at
FROM runs
WHERE project_id = ? AND id = ?
`, projectID, entity.ID).Scan(&generatorRef, &generatorVersion, &generatorHash, &status, &metadata, &startedAt, &completedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RunDetail{}, fmt.Errorf("run %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return RunDetail{}, fmt.Errorf("read run %s: %w", entity.ID, err)
	}
	alias := entity.Alias
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "run", entity.ID); err == nil {
			alias = found
		}
	}
	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{Kind: "run", ID: entity.ID, Alias: alias, Title: generatorRef, Status: status})
	if err != nil {
		return RunDetail{}, err
	}
	return RunDetail{
		ID:               entity.ID,
		Alias:            alias,
		GeneratorRef:     generatorRef,
		GeneratorVersion: generatorVersion.String,
		GeneratorHash:    generatorHash.String,
		Status:           status,
		Metadata:         metadata.String,
		StartedAt:        startedAt.String,
		CompletedAt:      completedAt.String,
		Relationships:    relationships,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func (s *Store) nextRunAlias(ctx context.Context, tx *sql.Tx, projectID string, generatorRef string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(generatorRef)
	if slug == "" {
		slug = "run"
	}
	prefix := "RUN-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'run' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check run alias %s: %w", alias, err)
		}
	}
}

func jsonValid(value string) bool {
	return strings.TrimSpace(value) == "" || json.Valid([]byte(value))
}
