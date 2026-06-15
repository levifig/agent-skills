package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// TaskShow is the state-backed single-task read model.
type TaskShow struct {
	ContractVersion    int        `json:"contract_version,omitempty"`
	DatabaseScope      string     `json:"database_scope,omitempty"`
	DatabasePath       string     `json:"database_path,omitempty"`
	ProjectID          string     `json:"project_id,omitempty"`
	ProjectName        string     `json:"project_name,omitempty"`
	ProjectCurrentPath string     `json:"project_current_path,omitempty"`
	Query              string     `json:"query"`
	Task               TaskDetail `json:"task"`
}

// TaskDetail contains operational task metadata plus imported source context.
type TaskDetail struct {
	ID        string        `json:"id"`
	Alias     string        `json:"alias,omitempty"`
	Title     string        `json:"title"`
	Status    string        `json:"status"`
	Priority  string        `json:"priority,omitempty"`
	Spec      string        `json:"spec,omitempty"`
	DependsOn []string      `json:"depends_on"`
	Sessions  []string      `json:"sessions,omitempty"`
	Sources   []TraceSource `json:"sources"`
	Body      string        `json:"body,omitempty"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
}

// ShowTask returns one imported task from initialized SQLite state.
func ShowTask(ctx context.Context, root project.Root, resolver PathResolver, ref string) (TaskShow, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return TaskShow{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return TaskShow{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return TaskShow{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return TaskShow{}, err
	}
	defer store.Close()
	return store.ShowTask(ctx, root, ref)
}

// ShowTask returns one imported task from an open store.
func (s *Store) ShowTask(ctx context.Context, root project.Root, ref string) (TaskShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TaskShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TaskShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return TaskShow{}, err
	}
	if entity.Kind != "task" {
		return TaskShow{}, fmt.Errorf("task show target %q resolved to %s, not task", ref, entity.Kind)
	}

	task, err := s.taskDetail(ctx, root, projectID, entity)
	if err != nil {
		return TaskShow{}, err
	}
	return TaskShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Task:               task,
	}, nil
}

func (s *Store) taskDetail(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (TaskDetail, error) {
	var title, status, createdAt, updatedAt string
	var priority, specAlias, sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  tasks.title,
  tasks.status,
  tasks.priority,
  tasks.created_at,
  tasks.updated_at,
  spec_alias.alias,
  sources.path,
  sources.hash
FROM tasks
LEFT JOIN aliases spec_alias
  ON spec_alias.project_id = tasks.project_id
 AND spec_alias.entity_kind = 'spec'
 AND spec_alias.entity_id = tasks.spec_id
 AND spec_alias.namespace = 'spec'
LEFT JOIN sources ON sources.id = tasks.body_source_id
WHERE tasks.project_id = ? AND tasks.id = ?
`, projectID, entity.ID).Scan(&title, &status, &priority, &createdAt, &updatedAt, &specAlias, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskDetail{}, fmt.Errorf("task %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return TaskDetail{}, fmt.Errorf("read task %s: %w", entity.ID, err)
	}

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "task", entity.ID); err == nil {
			alias = found
		}
	}
	dependencies, err := s.taskDependencyAliases(ctx, projectID, alias)
	if err != nil {
		return TaskDetail{}, err
	}
	sessions, err := s.taskSessionAliases(ctx, projectID, alias)
	if err != nil {
		return TaskDetail{}, err
	}

	sources := []TraceSource{}
	body := ""
	if sourcePath.Valid && sourcePath.String != "" {
		path := filepath.ToSlash(sourcePath.String)
		sources = append(sources, TraceSource{Path: path, Hash: sourceHash.String})
		if content, err := readImportedSourceBody(root.Path(), path); err == nil {
			body = content
		}
	}

	return TaskDetail{
		ID:        entity.ID,
		Alias:     alias,
		Title:     title,
		Status:    status,
		Priority:  priority.String,
		Spec:      specAlias.String,
		DependsOn: dependencies,
		Sessions:  sessions,
		Sources:   sources,
		Body:      body,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func readImportedSourceBody(rootPath string, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("source path %q is absolute", relPath)
	}
	clean := filepath.Clean(filepath.FromSlash(relPath))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("source path %q escapes project root", relPath)
	}
	content, err := os.ReadFile(filepath.Join(rootPath, clean))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(markdownBody(content)), nil
}

func (s *Store) taskSessionAliases(ctx context.Context, projectID string, alias string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(session_alias.alias, relationships.to_entity_id)
FROM relationships
JOIN aliases task_alias
  ON task_alias.project_id = relationships.project_id
 AND task_alias.entity_kind = 'task'
 AND task_alias.entity_id = relationships.from_entity_id
 AND task_alias.namespace = 'task'
LEFT JOIN aliases session_alias
  ON session_alias.project_id = relationships.project_id
 AND session_alias.entity_kind = relationships.to_entity_kind
 AND session_alias.entity_id = relationships.to_entity_id
 AND session_alias.namespace = 'session'
WHERE relationships.project_id = ?
  AND relationships.from_entity_kind = 'task'
  AND relationships.to_entity_kind = 'session'
  AND relationships.relationship_type = 'associated_with'
  AND task_alias.alias = ?
ORDER BY session_alias.alias, relationships.to_entity_id
`, projectID, alias)
	if err != nil {
		return nil, fmt.Errorf("query task sessions: %w", err)
	}
	defer rows.Close()

	sessions := []string{}
	for rows.Next() {
		var session sql.NullString
		if err := rows.Scan(&session); err != nil {
			return nil, fmt.Errorf("scan task session: %w", err)
		}
		if session.Valid {
			sessions = append(sessions, session.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task sessions: %w", err)
	}
	return sessions, nil
}

func markdownBody(content []byte) string {
	text := string(content)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return text
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return text
}
