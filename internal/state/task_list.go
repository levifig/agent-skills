package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

var taskStatusOrder = []string{"in_progress", "blocked", "todo", "review", "done"}
var taskListStatusOrder = []string{"in_progress", "blocked", "todo", "review", "done", "archived"}

// TaskList is the state-backed task-list read model.
type TaskList struct {
	ContractVersion    int                 `json:"contract_version,omitempty"`
	DatabaseScope      string              `json:"database_scope,omitempty"`
	DatabasePath       string              `json:"database_path,omitempty"`
	ProjectID          string              `json:"project_id,omitempty"`
	ProjectName        string              `json:"project_name,omitempty"`
	ProjectCurrentPath string              `json:"project_current_path,omitempty"`
	Diagnostics        []Diagnostic        `json:"diagnostics,omitempty"`
	Version            int                 `json:"version"`
	Tasks              map[string]TaskItem `json:"tasks"`
}

// TaskItem is a task entry returned by the state-backed task list.
type TaskItem struct {
	Title      string   `json:"title"`
	Spec       string   `json:"spec,omitempty"`
	Status     string   `json:"status"`
	Priority   string   `json:"priority,omitempty"`
	DependsOn  []string `json:"depends_on"`
	SourcePath string   `json:"source_path,omitempty"`
}

// TaskListOptions filter the state-backed task list.
type TaskListOptions struct {
	Active bool
	Status string
}

// ListTasks returns imported tasks from initialized SQLite state.
func ListTasks(ctx context.Context, root project.Root, resolver PathResolver, options TaskListOptions) (TaskList, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return TaskList{}, err
	}
	defer store.Close()
	return store.ListTasks(ctx, root, options)
}

// ListTasks returns imported tasks from an open store.
func (s *Store) ListTasks(ctx context.Context, root project.Root, options TaskListOptions) (TaskList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TaskList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TaskList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  task_alias.alias,
  tasks.title,
  tasks.status,
  COALESCE(tasks.priority, ''),
  COALESCE(spec_alias.alias, ''),
  COALESCE(sources.path, '')
FROM tasks
JOIN aliases task_alias
  ON task_alias.project_id = tasks.project_id
 AND task_alias.entity_kind = 'task'
 AND task_alias.entity_id = tasks.id
 AND task_alias.namespace = 'task'
LEFT JOIN aliases spec_alias
  ON spec_alias.project_id = tasks.project_id
 AND spec_alias.entity_kind = 'spec'
 AND spec_alias.entity_id = tasks.spec_id
 AND spec_alias.namespace = 'spec'
LEFT JOIN sources ON sources.id = tasks.body_source_id
WHERE tasks.project_id = ?
ORDER BY task_alias.alias
`, projectID)
	if err != nil {
		return TaskList{}, fmt.Errorf("query tasks: %w", err)
	}

	taskList := TaskList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Tasks:              map[string]TaskItem{},
	}
	for rows.Next() {
		var alias, title, status, priority, specAlias, sourcePath string
		if err := rows.Scan(&alias, &title, &status, &priority, &specAlias, &sourcePath); err != nil {
			rows.Close()
			return TaskList{}, fmt.Errorf("scan task: %w", err)
		}
		if !includeTaskStatus(status, options) {
			continue
		}
		status = LifecycleStatusForDisplay(LifecycleEntityTask, status)
		taskList.Tasks[alias] = TaskItem{
			Title:      title,
			Spec:       specAlias,
			Status:     status,
			Priority:   priority,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return TaskList{}, fmt.Errorf("close tasks: %w", err)
	}
	if err := rows.Err(); err != nil {
		return TaskList{}, fmt.Errorf("iterate tasks: %w", err)
	}

	for alias, task := range taskList.Tasks {
		dependencies, err := s.taskDependencyAliases(ctx, projectID, alias)
		if err != nil {
			return TaskList{}, err
		}
		task.DependsOn = dependencies
		taskList.Tasks[alias] = task
	}
	return taskList, nil
}

func (s *Store) taskDependencyAliases(ctx context.Context, projectID string, alias string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(dep_alias.alias, relationships.to_entity_id)
FROM relationships
JOIN aliases task_alias
  ON task_alias.project_id = relationships.project_id
 AND task_alias.entity_kind = 'task'
 AND task_alias.entity_id = relationships.from_entity_id
 AND task_alias.namespace = 'task'
LEFT JOIN aliases dep_alias
  ON dep_alias.project_id = relationships.project_id
 AND dep_alias.entity_kind = relationships.to_entity_kind
 AND dep_alias.entity_id = relationships.to_entity_id
 AND dep_alias.namespace = 'task'
WHERE relationships.project_id = ?
  AND relationships.from_entity_kind = 'task'
  AND relationships.to_entity_kind = 'task'
  AND relationships.relationship_type = 'blocked_by'
  AND task_alias.alias = ?
ORDER BY dep_alias.alias, relationships.to_entity_id
`, projectID, alias)
	if err != nil {
		return nil, fmt.Errorf("query task dependencies: %w", err)
	}
	defer rows.Close()

	dependencies := []string{}
	for rows.Next() {
		var dependency sql.NullString
		if err := rows.Scan(&dependency); err != nil {
			return nil, fmt.Errorf("scan task dependency: %w", err)
		}
		if dependency.Valid {
			dependencies = append(dependencies, dependency.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task dependencies: %w", err)
	}
	return dependencies, nil
}

func includeTaskStatus(status string, options TaskListOptions) bool {
	if options.Active && (LifecycleStatusMatches(LifecycleEntityTask, status, LifecycleStatusDone) || LifecycleStatusMatches(LifecycleEntityTask, status, LifecycleStatusArchived)) {
		return false
	}
	if !LifecycleStatusFilterMatches(LifecycleEntityTask, status, options.Status) {
		return false
	}
	return true
}

// ValidTaskStatus reports whether status is a known direct task status.
func ValidTaskStatus(status string) bool {
	for _, valid := range taskStatusOrder {
		if status == valid {
			return true
		}
	}
	return false
}

// TaskStatuses returns valid direct task statuses in display order.
func TaskStatuses() []string {
	return append([]string(nil), taskStatusOrder...)
}

// ValidTaskListStatus reports whether status is a known task-list filter status.
func ValidTaskListStatus(status string) bool {
	for _, valid := range taskListStatusOrder {
		if status == valid {
			return true
		}
	}
	return false
}

// TaskListStatuses returns valid task-list filter statuses in display order.
func TaskListStatuses() []string {
	return append([]string(nil), taskListStatusOrder...)
}
