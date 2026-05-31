package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

// TraceResult describes a state-backed entity and its immediate provenance graph.
type TraceResult struct {
	Query         string              `json:"query"`
	Entity        TraceEntity         `json:"entity"`
	Sources       []TraceSource       `json:"sources"`
	Relationships []TraceRelationship `json:"relationships"`
}

// TraceEntity is a compact representation of a traced row.
type TraceEntity struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Alias  string `json:"alias,omitempty"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

// TraceSource links an entity back to imported source material.
type TraceSource struct {
	Path string `json:"path"`
	Hash string `json:"hash,omitempty"`
}

// TraceRelationship describes one immediate inbound or outbound relationship.
type TraceRelationship struct {
	Direction string      `json:"direction"`
	Type      string      `json:"type"`
	Entity    TraceEntity `json:"entity"`
	Reason    string      `json:"reason,omitempty"`
}

func validateResolutionTargetKind(kind string, ref string) error {
	switch kind {
	case "spec", "task", "idea", "brainstorm", "shaping_draft", "session", "report":
		return nil
	default:
		return fmt.Errorf("%q resolves to %s, which cannot resolve another entity", ref, kind)
	}
}

// Trace resolves a human-facing alias or internal row ID from initialized SQLite state.
func Trace(ctx context.Context, root project.Root, resolver PathResolver, ref string) (TraceResult, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return TraceResult{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return TraceResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` or `loaf state init` first")
	} else if err != nil {
		return TraceResult{}, fmt.Errorf("inspect state database: %w", err)
	}

	store, err := OpenStore(databasePath)
	if err != nil {
		return TraceResult{}, err
	}
	defer store.Close()
	return store.Trace(ctx, root, ref)
}

// Trace resolves a human-facing alias or internal row ID from an open store.
func (s *Store) Trace(ctx context.Context, root project.Root, ref string) (TraceResult, error) {
	projectID := ProjectID(root)
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return TraceResult{}, err
	}
	sources, err := s.traceSources(ctx, entity)
	if err != nil {
		return TraceResult{}, err
	}
	relationships, err := s.traceRelationships(ctx, projectID, entity)
	if err != nil {
		return TraceResult{}, err
	}
	return TraceResult{
		Query:         ref,
		Entity:        entity,
		Sources:       sources,
		Relationships: relationships,
	}, nil
}

func (s *Store) resolveTraceEntity(ctx context.Context, projectID string, ref string) (TraceEntity, error) {
	var kind, id, alias string
	err := s.db.QueryRowContext(ctx, `
SELECT entity_kind, entity_id, alias
FROM aliases
WHERE project_id = ? AND alias = ?
ORDER BY namespace
LIMIT 1
	`, projectID, ref).Scan(&kind, &id, &alias)
	if errors.Is(err, sql.ErrNoRows) {
		kind, id, err = s.resolveEntityByInternalID(ctx, projectID, ref)
		alias = ""
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TraceEntity{}, fmt.Errorf("trace target %q not found in SQLite state", ref)
		}
		return TraceEntity{}, fmt.Errorf("resolve trace target %q: %w", ref, err)
	}
	entity, err := s.entityDetails(ctx, projectID, kind, id)
	if err != nil {
		return TraceEntity{}, err
	}
	if alias != "" {
		entity.Alias = alias
	} else if foundAlias, err := s.entityAlias(ctx, projectID, kind, id); err == nil {
		entity.Alias = foundAlias
	}
	return entity, nil
}

func (s *Store) resolveEntityByInternalID(ctx context.Context, projectID string, ref string) (string, string, error) {
	for _, kind := range []string{"spec", "task", "idea", "spark", "brainstorm", "shaping_draft", "session", "report", "journal_entry"} {
		table := traceTable(kind)
		var id string
		err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE project_id = ? AND id = ?`, table), projectID, ref).Scan(&id)
		if err == nil {
			return kind, id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", "", err
		}
	}
	return "", "", sql.ErrNoRows
}

func (s *Store) entityDetails(ctx context.Context, projectID string, kind string, id string) (TraceEntity, error) {
	entity := TraceEntity{Kind: kind, ID: id}
	switch kind {
	case "spec", "task", "idea", "brainstorm", "shaping_draft", "report":
		table := traceTable(kind)
		var title, status sql.NullString
		err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT title, status FROM %s WHERE project_id = ? AND id = ?`, table), projectID, id).Scan(&title, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return entityWithAliasFallback(ctx, s, projectID, entity)
		}
		if err != nil {
			return TraceEntity{}, fmt.Errorf("read %s %s: %w", kind, id, err)
		}
		entity.Title = title.String
		entity.Status = status.String
	case "spark":
		var text, status sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT text, status FROM sparks WHERE project_id = ? AND id = ?`, projectID, id).Scan(&text, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return entityWithAliasFallback(ctx, s, projectID, entity)
		}
		if err != nil {
			return TraceEntity{}, fmt.Errorf("read spark %s: %w", id, err)
		}
		entity.Title = text.String
		entity.Status = status.String
	case "session":
		var status sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT status FROM sessions WHERE project_id = ? AND id = ?`, projectID, id).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return entityWithAliasFallback(ctx, s, projectID, entity)
		}
		if err != nil {
			return TraceEntity{}, fmt.Errorf("read session %s: %w", id, err)
		}
		entity.Status = status.String
	case "journal_entry":
		var entryType, scope, message sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT entry_type, scope, message FROM journal_entries WHERE project_id = ? AND id = ?`, projectID, id).Scan(&entryType, &scope, &message)
		if errors.Is(err, sql.ErrNoRows) {
			return entityWithAliasFallback(ctx, s, projectID, entity)
		}
		if err != nil {
			return TraceEntity{}, fmt.Errorf("read journal entry %s: %w", id, err)
		}
		if scope.String != "" {
			entity.Title = fmt.Sprintf("%s(%s): %s", entryType.String, scope.String, message.String)
		} else {
			entity.Title = fmt.Sprintf("%s: %s", entryType.String, message.String)
		}
	default:
		return TraceEntity{}, fmt.Errorf("unsupported trace entity kind %q", kind)
	}
	return entity, nil
}

func entityWithAliasFallback(ctx context.Context, s *Store, projectID string, entity TraceEntity) (TraceEntity, error) {
	alias, err := s.entityAlias(ctx, projectID, entity.Kind, entity.ID)
	if err != nil {
		return entity, nil
	}
	entity.Alias = alias
	return entity, nil
}

func (s *Store) traceSources(ctx context.Context, entity TraceEntity) ([]TraceSource, error) {
	query, args, ok := traceSourceQuery(entity)
	if !ok {
		return []TraceSource{}, nil
	}
	var path, hash sql.NullString
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&path, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return []TraceSource{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read trace source: %w", err)
	}
	return []TraceSource{{Path: filepath.ToSlash(path.String), Hash: hash.String}}, nil
}

func traceSourceQuery(entity TraceEntity) (string, []any, bool) {
	switch entity.Kind {
	case "spec", "task", "idea", "brainstorm", "shaping_draft", "report":
		return fmt.Sprintf(`SELECT sources.path, sources.hash FROM %s JOIN sources ON sources.id = %s.body_source_id WHERE %s.id = ?`, traceTable(entity.Kind), traceTable(entity.Kind), traceTable(entity.Kind)), []any{entity.ID}, true
	case "spark":
		return `SELECT sources.path, sources.hash FROM sparks JOIN sources ON sources.id = sparks.source_id WHERE sparks.id = ?`, []any{entity.ID}, true
	case "session":
		return `SELECT sources.path, sources.hash FROM sessions JOIN sources ON sources.id = sessions.body_source_id WHERE sessions.id = ?`, []any{entity.ID}, true
	case "journal_entry":
		return `SELECT sources.path, sources.hash FROM journal_entries JOIN sessions ON sessions.id = journal_entries.session_id JOIN sources ON sources.id = sessions.body_source_id WHERE journal_entries.id = ?`, []any{entity.ID}, true
	default:
		return "", nil, false
	}
}

func (s *Store) traceRelationships(ctx context.Context, projectID string, entity TraceEntity) ([]TraceRelationship, error) {
	var relationships []TraceRelationship
	outbound, err := s.queryTraceRelationships(ctx, projectID, "outbound", `
SELECT relationship_type, to_entity_kind, to_entity_id, reason
FROM relationships
WHERE project_id = ? AND from_entity_kind = ? AND from_entity_id = ?
ORDER BY relationship_type, to_entity_kind, to_entity_id
`, entity.Kind, entity.ID)
	if err != nil {
		return nil, err
	}
	inbound, err := s.queryTraceRelationships(ctx, projectID, "inbound", `
SELECT relationship_type, from_entity_kind, from_entity_id, reason
FROM relationships
WHERE project_id = ? AND to_entity_kind = ? AND to_entity_id = ?
ORDER BY relationship_type, from_entity_kind, from_entity_id
`, entity.Kind, entity.ID)
	if err != nil {
		return nil, err
	}
	relationships = append(relationships, outbound...)
	relationships = append(relationships, inbound...)
	return relationships, nil
}

func (s *Store) queryTraceRelationships(ctx context.Context, projectID string, direction string, query string, kind string, id string) ([]TraceRelationship, error) {
	rows, err := s.db.QueryContext(ctx, query, projectID, kind, id)
	if err != nil {
		return nil, fmt.Errorf("query %s relationships: %w", direction, err)
	}

	type rawRelationship struct {
		relationshipType string
		otherKind        string
		otherID          string
		reason           string
	}
	var raw []rawRelationship
	for rows.Next() {
		var relationshipType, otherKind, otherID string
		var reason sql.NullString
		if err := rows.Scan(&relationshipType, &otherKind, &otherID, &reason); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan %s relationship: %w", direction, err)
		}
		raw = append(raw, rawRelationship{
			relationshipType: relationshipType,
			otherKind:        otherKind,
			otherID:          otherID,
			reason:           reason.String,
		})
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close %s relationships: %w", direction, err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s relationships: %w", direction, err)
	}

	var relationships []TraceRelationship
	for _, relationship := range raw {
		other, err := s.entityDetails(ctx, projectID, relationship.otherKind, relationship.otherID)
		if err != nil {
			return nil, err
		}
		if alias, err := s.entityAlias(ctx, projectID, relationship.otherKind, relationship.otherID); err == nil {
			other.Alias = alias
		}
		relationships = append(relationships, TraceRelationship{
			Direction: direction,
			Type:      relationship.relationshipType,
			Entity:    other,
			Reason:    relationship.reason,
		})
	}
	return relationships, nil
}

func (s *Store) entityAlias(ctx context.Context, projectID string, kind string, id string) (string, error) {
	var alias string
	err := s.db.QueryRowContext(ctx, `
SELECT alias
FROM aliases
WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
ORDER BY namespace, alias
LIMIT 1
`, projectID, kind, id).Scan(&alias)
	return alias, err
}

func traceTable(kind string) string {
	switch kind {
	case "spec":
		return "specs"
	case "task":
		return "tasks"
	case "idea":
		return "ideas"
	case "spark":
		return "sparks"
	case "brainstorm":
		return "brainstorms"
	case "shaping_draft":
		return "shaping_drafts"
	case "session":
		return "sessions"
	case "report":
		return "reports"
	case "journal_entry":
		return "journal_entries"
	default:
		return ""
	}
}
