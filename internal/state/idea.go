package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// IdeaList is the state-backed idea-list read model.
type IdeaList struct {
	ContractVersion    int                 `json:"contract_version,omitempty"`
	DatabaseScope      string              `json:"database_scope,omitempty"`
	DatabasePath       string              `json:"database_path,omitempty"`
	ProjectID          string              `json:"project_id,omitempty"`
	ProjectName        string              `json:"project_name,omitempty"`
	ProjectCurrentPath string              `json:"project_current_path,omitempty"`
	Version            int                 `json:"version"`
	Ideas              map[string]IdeaItem `json:"ideas"`
}

// IdeaItem is an idea entry returned by the state-backed idea list.
type IdeaItem struct {
	Title      string `json:"title"`
	Status     string `json:"status"`
	SourcePath string `json:"source_path,omitempty"`
}

// IdeaListOptions filter the state-backed idea list.
type IdeaListOptions struct {
	All    bool
	Status string
}

// IdeaResolveResult describes a state-backed idea resolution mutation.
type IdeaResolveResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Idea               TraceEntity `json:"idea"`
	ResolvedBy         TraceEntity `json:"resolved_by"`
	Relationship       string      `json:"relationship"`
	EventID            string      `json:"event_id,omitempty"`
}

// IdeaPromoteOptions describes a SQLite-backed idea promotion request.
type IdeaPromoteOptions struct {
	Idea   string
	ToSpec string
}

// IdeaPromoteResult describes a state-backed idea promotion mutation.
type IdeaPromoteResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Idea               TraceEntity `json:"idea"`
	Spec               TraceEntity `json:"spec"`
	Relationship       string      `json:"relationship"`
}

// IdeaCaptureOptions describes a SQLite-backed idea capture request.
type IdeaCaptureOptions struct {
	Title string
}

// IdeaCaptureResult describes a captured SQLite-backed idea.
type IdeaCaptureResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Idea               TraceEntity `json:"idea"`
	EventID            string      `json:"event_id"`
}

// IdeaArchiveOptions describes a SQLite-backed idea archive request.
type IdeaArchiveOptions struct {
	Refs   []string
	Reason string
}

// IdeaArchiveResult describes a state-backed idea archive mutation.
type IdeaArchiveResult struct {
	ContractVersion    int               `json:"contract_version,omitempty"`
	DatabaseScope      string            `json:"database_scope,omitempty"`
	DatabasePath       string            `json:"database_path,omitempty"`
	ProjectID          string            `json:"project_id,omitempty"`
	ProjectName        string            `json:"project_name,omitempty"`
	ProjectCurrentPath string            `json:"project_current_path,omitempty"`
	Archived           []IdeaArchiveItem `json:"archived"`
	Skipped            []IdeaArchiveItem `json:"skipped"`
}

// IdeaArchiveItem describes one requested idea archive outcome.
type IdeaArchiveItem struct {
	Idea     *TraceEntity `json:"idea,omitempty"`
	Ref      string       `json:"ref,omitempty"`
	Previous string       `json:"previous_status,omitempty"`
	Status   string       `json:"status,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	EventID  string       `json:"event_id,omitempty"`
	Note     string       `json:"note,omitempty"`
}

// ListIdeas returns imported ideas from initialized SQLite state.
func ListIdeas(ctx context.Context, root project.Root, resolver PathResolver, options IdeaListOptions) (IdeaList, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return IdeaList{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return IdeaList{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return IdeaList{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return IdeaList{}, err
	}
	defer store.Close()
	return store.ListIdeas(ctx, root, options)
}

// ListIdeas returns imported ideas from an open store.
func (s *Store) ListIdeas(ctx context.Context, root project.Root, options IdeaListOptions) (IdeaList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IdeaList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IdeaList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  idea_alias.alias,
  ideas.title,
  ideas.status,
  COALESCE(sources.path, '')
FROM ideas
JOIN aliases idea_alias
  ON idea_alias.project_id = ideas.project_id
 AND idea_alias.entity_kind = 'idea'
 AND idea_alias.entity_id = ideas.id
 AND idea_alias.namespace = 'idea'
LEFT JOIN sources ON sources.id = ideas.body_source_id
WHERE ideas.project_id = ?
ORDER BY idea_alias.alias
`, projectID)
	if err != nil {
		return IdeaList{}, fmt.Errorf("query ideas: %w", err)
	}

	ideas := IdeaList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Ideas:              map[string]IdeaItem{},
	}
	for rows.Next() {
		var alias, title, status, sourcePath string
		if err := rows.Scan(&alias, &title, &status, &sourcePath); err != nil {
			rows.Close()
			return IdeaList{}, fmt.Errorf("scan idea: %w", err)
		}
		if !includeIdeaStatus(status, options) {
			continue
		}
		ideas.Ideas[alias] = IdeaItem{
			Title:      title,
			Status:     status,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return IdeaList{}, fmt.Errorf("close ideas: %w", err)
	}
	if err := rows.Err(); err != nil {
		return IdeaList{}, fmt.Errorf("iterate ideas: %w", err)
	}
	return ideas, nil
}

// CaptureIdea captures an idea in initialized SQLite state.
func CaptureIdea(ctx context.Context, root project.Root, resolver PathResolver, options IdeaCaptureOptions) (IdeaCaptureResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return IdeaCaptureResult{}, err
	}
	defer store.Close()
	return store.CaptureIdea(ctx, root, options)
}

// CaptureIdea captures an idea in an open store.
func (s *Store) CaptureIdea(ctx context.Context, root project.Root, options IdeaCaptureOptions) (IdeaCaptureResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IdeaCaptureResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IdeaCaptureResult{}, err
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return IdeaCaptureResult{}, fmt.Errorf("idea capture requires --title")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IdeaCaptureResult{}, fmt.Errorf("begin idea capture transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	alias, err := s.nextIdeaAlias(ctx, tx, projectID, title, now)
	if err != nil {
		return IdeaCaptureResult{}, err
	}
	ideaID := stableMigrationID("idea", projectID, alias)
	timestamp := now.Format(time.RFC3339)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ideas (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, NULL, ?, ?)
`, ideaID, projectID, title, "open", timestamp, timestamp)
	if err != nil {
		return IdeaCaptureResult{}, fmt.Errorf("insert idea %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "idea", ideaID, "idea", alias, timestamp); err != nil {
		return IdeaCaptureResult{}, err
	}

	eventID := stableMigrationID("event", projectID, "idea", ideaID, "created", "open")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'idea', ?, 'status_changed', NULL, 'open', 'recorded by idea capture', ?, ?)
`, eventID, projectID, ideaID, timestamp, timestamp); err != nil {
		return IdeaCaptureResult{}, fmt.Errorf("record idea capture event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return IdeaCaptureResult{}, fmt.Errorf("commit idea capture transaction: %w", err)
	}

	return IdeaCaptureResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Idea:               TraceEntity{Kind: "idea", ID: ideaID, Alias: alias, Title: title, Status: "open"},
		EventID:            eventID,
	}, nil
}

func (s *Store) nextIdeaAlias(ctx context.Context, tx *sql.Tx, projectID string, title string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(title)
	if slug == "" {
		slug = "idea"
	}
	prefix := "IDEA-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'idea' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check idea alias %s: %w", alias, err)
		}
	}
}

// ResolveIdea marks an idea resolved and records the resolving relationship.
func ResolveIdea(ctx context.Context, root project.Root, resolver PathResolver, ideaRef string, byRef string) (IdeaResolveResult, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return IdeaResolveResult{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return IdeaResolveResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return IdeaResolveResult{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return IdeaResolveResult{}, err
	}
	defer store.Close()
	return store.ResolveIdea(ctx, root, ideaRef, byRef)
}

// ResolveIdea marks an idea resolved in an open store.
func (s *Store) ResolveIdea(ctx context.Context, root project.Root, ideaRef string, byRef string) (IdeaResolveResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IdeaResolveResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IdeaResolveResult{}, err
	}
	idea, err := s.resolveTraceEntity(ctx, projectID, ideaRef)
	if err != nil {
		return IdeaResolveResult{}, err
	}
	if idea.Kind != "idea" {
		return IdeaResolveResult{}, fmt.Errorf("%q resolves to %s, not idea", ideaRef, idea.Kind)
	}
	target, err := s.resolveTraceEntity(ctx, projectID, byRef)
	if err != nil {
		return IdeaResolveResult{}, err
	}
	if err := validateResolutionTargetKind(target.Kind, byRef); err != nil {
		return IdeaResolveResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IdeaResolveResult{}, fmt.Errorf("begin idea resolve transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM ideas WHERE project_id = ? AND id = ?`, projectID, idea.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return IdeaResolveResult{}, fmt.Errorf("idea %q not found in SQLite state", ideaRef)
	}
	if err != nil {
		return IdeaResolveResult{}, fmt.Errorf("read idea status: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE ideas SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, "resolved", now, projectID, idea.ID); err != nil {
		return IdeaResolveResult{}, fmt.Errorf("update idea status: %w", err)
	}

	relationshipID := stableMigrationID("relationship", projectID, "idea", idea.ID, "resolved_by", target.Kind, target.ID)
	_, err = tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'command', ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  origin = excluded.origin,
  updated_at = excluded.updated_at
`, relationshipID, projectID, "idea", idea.ID, target.Kind, target.ID, "resolved_by", "recorded by idea resolve", now, now)
	if err != nil {
		return IdeaResolveResult{}, fmt.Errorf("record idea resolution relationship: %w", err)
	}

	eventID := ""
	if previousStatus != "resolved" {
		eventID = stableMigrationID("event", projectID, "idea", idea.ID, "status", previousStatus, "resolved")
		_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "idea", idea.ID, "status_changed", previousStatus, "resolved", "recorded by idea resolve", now, now)
		if err != nil {
			return IdeaResolveResult{}, fmt.Errorf("record idea resolution event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return IdeaResolveResult{}, fmt.Errorf("commit idea resolve transaction: %w", err)
	}

	idea.Status = "resolved"
	return IdeaResolveResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Idea:               idea,
		ResolvedBy:         target,
		Relationship:       relationshipID,
		EventID:            eventID,
	}, nil
}

// PromoteIdea records that an idea promoted to a spec in initialized SQLite state.
func PromoteIdea(ctx context.Context, root project.Root, resolver PathResolver, options IdeaPromoteOptions) (IdeaPromoteResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return IdeaPromoteResult{}, err
	}
	defer store.Close()
	return store.PromoteIdea(ctx, root, options)
}

// PromoteIdea records that an idea promoted to a spec in an open store.
func (s *Store) PromoteIdea(ctx context.Context, root project.Root, options IdeaPromoteOptions) (IdeaPromoteResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IdeaPromoteResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IdeaPromoteResult{}, err
	}
	idea, err := s.resolveTraceEntity(ctx, projectID, options.Idea)
	if err != nil {
		return IdeaPromoteResult{}, err
	}
	if idea.Kind != "idea" {
		return IdeaPromoteResult{}, fmt.Errorf("%q resolves to %s, not idea", options.Idea, idea.Kind)
	}
	spec, err := s.resolveTraceEntity(ctx, projectID, options.ToSpec)
	if err != nil {
		return IdeaPromoteResult{}, err
	}
	if spec.Kind != "spec" {
		return IdeaPromoteResult{}, fmt.Errorf("%q resolves to %s, not spec", options.ToSpec, spec.Kind)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	relationshipID := stableMigrationID("relationship", projectID, "idea", idea.ID, "promoted_to", "spec", spec.ID)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'command', ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  origin = excluded.origin,
  updated_at = excluded.updated_at
`, relationshipID, projectID, "idea", idea.ID, "spec", spec.ID, "promoted_to", "recorded by idea promote", now, now)
	if err != nil {
		return IdeaPromoteResult{}, fmt.Errorf("record idea promotion relationship: %w", err)
	}

	return IdeaPromoteResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Idea:               idea,
		Spec:               spec,
		Relationship:       relationshipID,
	}, nil
}

// ArchiveIdeas archives ideas in initialized SQLite state.
func ArchiveIdeas(ctx context.Context, root project.Root, resolver PathResolver, options IdeaArchiveOptions) (IdeaArchiveResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return IdeaArchiveResult{}, err
	}
	defer store.Close()
	return store.ArchiveIdeas(ctx, root, options)
}

// ArchiveIdeas archives ideas in an open store.
func (s *Store) ArchiveIdeas(ctx context.Context, root project.Root, options IdeaArchiveOptions) (IdeaArchiveResult, error) {
	if len(options.Refs) == 0 {
		return IdeaArchiveResult{}, fmt.Errorf("idea archive requires at least one idea")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IdeaArchiveResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IdeaArchiveResult{}, err
	}
	result := IdeaArchiveResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Archived:           []IdeaArchiveItem{},
		Skipped:            []IdeaArchiveItem{},
	}
	for _, ref := range options.Refs {
		item, archived, err := s.archiveIdea(ctx, projectID, ref, options.Reason)
		if err != nil {
			return IdeaArchiveResult{}, err
		}
		if archived {
			result.Archived = append(result.Archived, item)
		} else {
			result.Skipped = append(result.Skipped, item)
		}
	}
	return result, nil
}

func (s *Store) archiveIdea(ctx context.Context, projectID string, ref string, reason string) (IdeaArchiveItem, bool, error) {
	idea, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return IdeaArchiveItem{Ref: ref, Reason: err.Error()}, false, nil
	}
	if idea.Kind != "idea" {
		return IdeaArchiveItem{Idea: &idea, Ref: ref, Reason: fmt.Sprintf("%q resolves to %s, not idea", ref, idea.Kind)}, false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IdeaArchiveItem{}, false, fmt.Errorf("begin idea archive transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM ideas WHERE project_id = ? AND id = ?`, projectID, idea.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return IdeaArchiveItem{Idea: &idea, Ref: ref, Reason: fmt.Sprintf("idea %q not found in SQLite state", ref)}, false, nil
	}
	if err != nil {
		return IdeaArchiveItem{}, false, fmt.Errorf("read idea status: %w", err)
	}

	if previousStatus == "archived" {
		return IdeaArchiveItem{Idea: &idea, Ref: ref, Previous: previousStatus, Status: previousStatus, Reason: "already archived"}, false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE ideas SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, "archived", now, projectID, idea.ID); err != nil {
		return IdeaArchiveItem{}, false, fmt.Errorf("update idea status: %w", err)
	}

	note := firstNonEmpty(strings.TrimSpace(reason), "recorded by idea archive")
	eventID := stableMigrationID("event", projectID, "idea", idea.ID, "status", previousStatus, "archived")
	_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "idea", idea.ID, "status_changed", previousStatus, "archived", note, now, now)
	if err != nil {
		return IdeaArchiveItem{}, false, fmt.Errorf("record idea archive event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return IdeaArchiveItem{}, false, fmt.Errorf("commit idea archive transaction: %w", err)
	}

	idea.Status = "archived"
	return IdeaArchiveItem{Idea: &idea, Ref: ref, Previous: previousStatus, Status: "archived", EventID: eventID, Note: note}, true, nil
}

func includeIdeaStatus(status string, options IdeaListOptions) bool {
	if options.Status != "" && status != options.Status {
		return false
	}
	if options.Status != "" {
		return true
	}
	if !options.All && (status == "resolved" || status == "archived") {
		return false
	}
	return true
}
