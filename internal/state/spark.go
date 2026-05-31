package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// SparkList is the state-backed spark-list read model.
type SparkList struct {
	Version int                  `json:"version"`
	Sparks  map[string]SparkItem `json:"sparks"`
}

// SparkItem is a spark entry returned by the state-backed spark list.
type SparkItem struct {
	Text       string `json:"text"`
	Scope      string `json:"scope,omitempty"`
	Status     string `json:"status"`
	SourcePath string `json:"source_path,omitempty"`
}

// SparkShow is the state-backed single-spark read model.
type SparkShow struct {
	Query string      `json:"query"`
	Spark SparkDetail `json:"spark"`
}

// SparkDetail contains operational spark metadata plus imported source context.
type SparkDetail struct {
	ID            string              `json:"id"`
	Alias         string              `json:"alias,omitempty"`
	Text          string              `json:"text"`
	Scope         string              `json:"scope,omitempty"`
	Status        string              `json:"status"`
	Sources       []TraceSource       `json:"sources"`
	Relationships []TraceRelationship `json:"relationships"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}

// SparkListOptions filter the state-backed spark list.
type SparkListOptions struct {
	All    bool
	Status string
}

// SparkResolveResult describes a state-backed spark resolution mutation.
type SparkResolveResult struct {
	Spark        TraceEntity `json:"spark"`
	ResolvedBy   TraceEntity `json:"resolved_by"`
	Relationship string      `json:"relationship"`
	EventID      string      `json:"event_id,omitempty"`
	Reason       string      `json:"reason,omitempty"`
}

// SparkResolveOptions describes a SQLite-backed spark resolution request.
type SparkResolveOptions struct {
	Spark  string
	By     string
	Reason string
}

// SparkPromoteOptions describes a SQLite-backed spark promotion request.
type SparkPromoteOptions struct {
	Spark  string
	ToIdea string
}

// SparkPromoteResult describes a state-backed spark promotion mutation.
type SparkPromoteResult struct {
	Spark        TraceEntity `json:"spark"`
	Idea         TraceEntity `json:"idea"`
	Relationship string      `json:"relationship"`
}

// SparkCaptureOptions describes a SQLite-backed spark capture request.
type SparkCaptureOptions struct {
	Text  string
	Scope string
}

// SparkCaptureResult describes a captured SQLite-backed spark.
type SparkCaptureResult struct {
	Spark   TraceEntity `json:"spark"`
	Scope   string      `json:"scope,omitempty"`
	EventID string      `json:"event_id"`
}

// ListSparks returns imported sparks from initialized SQLite state.
func ListSparks(ctx context.Context, root project.Root, resolver PathResolver, options SparkListOptions) (SparkList, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return SparkList{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return SparkList{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return SparkList{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return SparkList{}, err
	}
	defer store.Close()
	return store.ListSparks(ctx, root, options)
}

// ListSparks returns imported sparks from an open store.
func (s *Store) ListSparks(ctx context.Context, root project.Root, options SparkListOptions) (SparkList, error) {
	projectID := ProjectID(root)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  spark_alias.alias,
  sparks.text,
  COALESCE(sparks.scope, ''),
  sparks.status,
  COALESCE(sources.path, '')
FROM sparks
JOIN aliases spark_alias
  ON spark_alias.project_id = sparks.project_id
 AND spark_alias.entity_kind = 'spark'
 AND spark_alias.entity_id = sparks.id
 AND spark_alias.namespace = 'spark'
LEFT JOIN sources ON sources.id = sparks.source_id
WHERE sparks.project_id = ?
ORDER BY spark_alias.alias
`, projectID)
	if err != nil {
		return SparkList{}, fmt.Errorf("query sparks: %w", err)
	}

	sparks := SparkList{Version: 1, Sparks: map[string]SparkItem{}}
	for rows.Next() {
		var alias, text, scope, status, sourcePath string
		if err := rows.Scan(&alias, &text, &scope, &status, &sourcePath); err != nil {
			rows.Close()
			return SparkList{}, fmt.Errorf("scan spark: %w", err)
		}
		if !includeSparkStatus(status, options) {
			continue
		}
		sparks.Sparks[alias] = SparkItem{
			Text:       text,
			Scope:      scope,
			Status:     status,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return SparkList{}, fmt.Errorf("close sparks: %w", err)
	}
	if err := rows.Err(); err != nil {
		return SparkList{}, fmt.Errorf("iterate sparks: %w", err)
	}
	return sparks, nil
}

// ShowSpark returns one spark from initialized SQLite state.
func ShowSpark(ctx context.Context, root project.Root, resolver PathResolver, ref string) (SparkShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SparkShow{}, err
	}
	defer store.Close()
	return store.ShowSpark(ctx, root, ref)
}

// ShowSpark returns one spark from an open store.
func (s *Store) ShowSpark(ctx context.Context, root project.Root, ref string) (SparkShow, error) {
	projectID := ProjectID(root)
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return SparkShow{}, err
	}
	if entity.Kind != "spark" {
		return SparkShow{}, fmt.Errorf("spark show target %q resolved to %s, not spark", ref, entity.Kind)
	}

	spark, err := s.sparkDetail(ctx, projectID, entity)
	if err != nil {
		return SparkShow{}, err
	}
	return SparkShow{Query: ref, Spark: spark}, nil
}

func (s *Store) sparkDetail(ctx context.Context, projectID string, entity TraceEntity) (SparkDetail, error) {
	var text, status, createdAt, updatedAt string
	var scope, sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  sparks.text,
  sparks.scope,
  sparks.status,
  sparks.created_at,
  sparks.updated_at,
  sources.path,
  sources.hash
FROM sparks
LEFT JOIN sources ON sources.id = sparks.source_id
WHERE sparks.project_id = ? AND sparks.id = ?
`, projectID, entity.ID).Scan(&text, &scope, &status, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return SparkDetail{}, fmt.Errorf("spark %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return SparkDetail{}, fmt.Errorf("read spark %s: %w", entity.ID, err)
	}

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "spark", entity.ID); err == nil {
			alias = found
		}
	}

	sources := []TraceSource{}
	if sourcePath.Valid && sourcePath.String != "" {
		sources = append(sources, TraceSource{Path: filepath.ToSlash(sourcePath.String), Hash: sourceHash.String})
	}

	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{
		Kind:   "spark",
		ID:     entity.ID,
		Alias:  alias,
		Title:  text,
		Status: status,
	})
	if err != nil {
		return SparkDetail{}, err
	}

	return SparkDetail{
		ID:            entity.ID,
		Alias:         alias,
		Text:          text,
		Scope:         scope.String,
		Status:        status,
		Sources:       sources,
		Relationships: relationships,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

// CaptureSpark captures a spark in initialized SQLite state.
func CaptureSpark(ctx context.Context, root project.Root, resolver PathResolver, options SparkCaptureOptions) (SparkCaptureResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SparkCaptureResult{}, err
	}
	defer store.Close()
	return store.CaptureSpark(ctx, root, options)
}

// CaptureSpark captures a spark in an open store.
func (s *Store) CaptureSpark(ctx context.Context, root project.Root, options SparkCaptureOptions) (SparkCaptureResult, error) {
	projectID := ProjectID(root)
	text := strings.TrimSpace(options.Text)
	if text == "" {
		return SparkCaptureResult{}, fmt.Errorf("spark capture requires --text")
	}
	scope := strings.TrimSpace(options.Scope)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SparkCaptureResult{}, fmt.Errorf("begin spark capture transaction: %w", err)
	}
	defer tx.Rollback()

	alias, err := s.nextSparkAlias(ctx, tx, projectID, text)
	if err != nil {
		return SparkCaptureResult{}, err
	}
	sparkID := stableMigrationID("spark", projectID, alias)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.ExecContext(ctx, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, NULL, ?, ?)
`, sparkID, projectID, emptyToNil(scope), "open", text, now, now)
	if err != nil {
		return SparkCaptureResult{}, fmt.Errorf("insert spark %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "spark", sparkID, "spark", alias, now); err != nil {
		return SparkCaptureResult{}, err
	}

	eventID := stableMigrationID("event", projectID, "spark", sparkID, "created", "open")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'spark', ?, 'status_changed', NULL, 'open', 'recorded by spark capture', ?, ?)
`, eventID, projectID, sparkID, now, now); err != nil {
		return SparkCaptureResult{}, fmt.Errorf("record spark capture event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return SparkCaptureResult{}, fmt.Errorf("commit spark capture transaction: %w", err)
	}

	return SparkCaptureResult{
		Spark:   TraceEntity{Kind: "spark", ID: sparkID, Alias: alias, Title: text, Status: "open"},
		Scope:   scope,
		EventID: eventID,
	}, nil
}

func (s *Store) nextSparkAlias(ctx context.Context, tx *sql.Tx, projectID string, text string) (string, error) {
	slug := normalizeSparkSlug(text)
	if slug == "" {
		slug = "spark"
	}
	for next := 1; ; next++ {
		alias := "SPARK-" + slug
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", alias, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'spark' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check spark alias %s: %w", alias, err)
		}
	}
}

// ResolveSpark marks a spark resolved and records the resolving relationship.
func ResolveSpark(ctx context.Context, root project.Root, resolver PathResolver, sparkRef string, byRef string) (SparkResolveResult, error) {
	return ResolveSparkWithOptions(ctx, root, resolver, SparkResolveOptions{Spark: sparkRef, By: byRef})
}

// ResolveSparkWithOptions marks a spark resolved and records optional rationale.
func ResolveSparkWithOptions(ctx context.Context, root project.Root, resolver PathResolver, options SparkResolveOptions) (SparkResolveResult, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return SparkResolveResult{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return SparkResolveResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return SparkResolveResult{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return SparkResolveResult{}, err
	}
	defer store.Close()
	return store.ResolveSparkWithOptions(ctx, root, options)
}

// ResolveSpark marks a spark resolved in an open store.
func (s *Store) ResolveSpark(ctx context.Context, root project.Root, sparkRef string, byRef string) (SparkResolveResult, error) {
	return s.ResolveSparkWithOptions(ctx, root, SparkResolveOptions{Spark: sparkRef, By: byRef})
}

// ResolveSparkWithOptions marks a spark resolved in an open store.
func (s *Store) ResolveSparkWithOptions(ctx context.Context, root project.Root, options SparkResolveOptions) (SparkResolveResult, error) {
	projectID := ProjectID(root)
	spark, err := s.resolveTraceEntity(ctx, projectID, options.Spark)
	if err != nil {
		return SparkResolveResult{}, err
	}
	if spark.Kind != "spark" {
		return SparkResolveResult{}, fmt.Errorf("%q resolves to %s, not spark", options.Spark, spark.Kind)
	}
	target, err := s.resolveTraceEntity(ctx, projectID, options.By)
	if err != nil {
		return SparkResolveResult{}, err
	}
	if err := validateResolutionTargetKind(target.Kind, options.By); err != nil {
		return SparkResolveResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SparkResolveResult{}, fmt.Errorf("begin spark resolve transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM sparks WHERE project_id = ? AND id = ?`, projectID, spark.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return SparkResolveResult{}, fmt.Errorf("spark %q not found in SQLite state", options.Spark)
	}
	if err != nil {
		return SparkResolveResult{}, fmt.Errorf("read spark status: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE sparks SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, "resolved", now, projectID, spark.ID); err != nil {
		return SparkResolveResult{}, fmt.Errorf("update spark status: %w", err)
	}

	reason := firstNonEmpty(strings.TrimSpace(options.Reason), "recorded by spark resolve")
	relationshipID := stableMigrationID("relationship", projectID, "spark", spark.ID, "resolved_by", target.Kind, target.ID)
	_, err = tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  updated_at = excluded.updated_at
`, relationshipID, projectID, "spark", spark.ID, target.Kind, target.ID, "resolved_by", reason, now, now)
	if err != nil {
		return SparkResolveResult{}, fmt.Errorf("record spark resolution relationship: %w", err)
	}

	eventID := ""
	if previousStatus != "resolved" {
		eventID = stableMigrationID("event", projectID, "spark", spark.ID, "status", previousStatus, "resolved")
		_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "spark", spark.ID, "status_changed", previousStatus, "resolved", reason, now, now)
		if err != nil {
			return SparkResolveResult{}, fmt.Errorf("record spark resolution event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return SparkResolveResult{}, fmt.Errorf("commit spark resolve transaction: %w", err)
	}

	spark.Status = "resolved"
	return SparkResolveResult{
		Spark:        spark,
		ResolvedBy:   target,
		Relationship: relationshipID,
		EventID:      eventID,
		Reason:       reason,
	}, nil
}

// PromoteSpark records that a spark promoted to an idea in initialized SQLite state.
func PromoteSpark(ctx context.Context, root project.Root, resolver PathResolver, options SparkPromoteOptions) (SparkPromoteResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SparkPromoteResult{}, err
	}
	defer store.Close()
	return store.PromoteSpark(ctx, root, options)
}

// PromoteSpark records that a spark promoted to an idea in an open store.
func (s *Store) PromoteSpark(ctx context.Context, root project.Root, options SparkPromoteOptions) (SparkPromoteResult, error) {
	projectID := ProjectID(root)
	spark, err := s.resolveTraceEntity(ctx, projectID, options.Spark)
	if err != nil {
		return SparkPromoteResult{}, err
	}
	if spark.Kind != "spark" {
		return SparkPromoteResult{}, fmt.Errorf("%q resolves to %s, not spark", options.Spark, spark.Kind)
	}
	idea, err := s.resolveTraceEntity(ctx, projectID, options.ToIdea)
	if err != nil {
		return SparkPromoteResult{}, err
	}
	if idea.Kind != "idea" {
		return SparkPromoteResult{}, fmt.Errorf("%q resolves to %s, not idea", options.ToIdea, idea.Kind)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	relationshipID := stableMigrationID("relationship", projectID, "spark", spark.ID, "promoted_to", "idea", idea.ID)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  updated_at = excluded.updated_at
`, relationshipID, projectID, "spark", spark.ID, "idea", idea.ID, "promoted_to", "recorded by spark promote", now, now)
	if err != nil {
		return SparkPromoteResult{}, fmt.Errorf("record spark promotion relationship: %w", err)
	}

	return SparkPromoteResult{
		Spark:        spark,
		Idea:         idea,
		Relationship: relationshipID,
	}, nil
}

func includeSparkStatus(status string, options SparkListOptions) bool {
	if options.Status != "" && status != options.Status {
		return false
	}
	if options.Status != "" {
		return true
	}
	if !options.All && status == "resolved" {
		return false
	}
	return true
}
