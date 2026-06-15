package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

var taskAliasPattern = regexp.MustCompile(`^TASK-(\d+)$`)
var taskPriorityOrder = []string{"P0", "P1", "P2", "P3"}

// TaskCreateOptions describes a SQLite-backed task creation request.
type TaskCreateOptions struct {
	Title     string
	Spec      string
	Priority  string
	DependsOn []string
}

// TaskCreateResult describes a created SQLite-backed task.
type TaskCreateResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Task               TraceEntity   `json:"task"`
	Priority           string        `json:"priority"`
	Spec               *TraceEntity  `json:"spec,omitempty"`
	Depends            []TraceEntity `json:"depends_on"`
	EventID            string        `json:"event_id"`
}

// CreateTask creates a task in initialized SQLite state.
func CreateTask(ctx context.Context, root project.Root, resolver PathResolver, options TaskCreateOptions) (TaskCreateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TaskCreateResult{}, err
	}
	defer store.Close()
	return store.CreateTask(ctx, root, options)
}

// CreateTask creates a task in an open store.
func (s *Store) CreateTask(ctx context.Context, root project.Root, options TaskCreateOptions) (TaskCreateResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TaskCreateResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TaskCreateResult{}, err
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return TaskCreateResult{}, fmt.Errorf("task create requires --title")
	}
	priority := strings.TrimSpace(options.Priority)
	if priority == "" {
		priority = "P2"
	}
	if !ValidTaskPriority(priority) {
		return TaskCreateResult{}, fmt.Errorf("invalid priority %q (valid: %s)", priority, taskPriorityText())
	}

	var spec *TraceEntity
	var specID any
	if strings.TrimSpace(options.Spec) != "" {
		resolved, err := s.resolveTraceEntity(ctx, projectID, options.Spec)
		if err != nil {
			return TaskCreateResult{}, fmt.Errorf("resolve spec %q: %w", options.Spec, err)
		}
		if resolved.Kind != "spec" {
			return TaskCreateResult{}, fmt.Errorf("%q resolves to %s, not spec", options.Spec, resolved.Kind)
		}
		spec = &resolved
		specID = resolved.ID
	}

	dependencies := []TraceEntity{}
	for _, dependencyRef := range options.DependsOn {
		dependencyRef = strings.TrimSpace(dependencyRef)
		if dependencyRef == "" {
			continue
		}
		dependency, err := s.resolveTraceEntity(ctx, projectID, dependencyRef)
		if err != nil {
			return TaskCreateResult{}, fmt.Errorf("resolve dependency %q: %w", dependencyRef, err)
		}
		if dependency.Kind != "task" {
			return TaskCreateResult{}, fmt.Errorf("%q resolves to %s, not task", dependencyRef, dependency.Kind)
		}
		dependencies = append(dependencies, dependency)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskCreateResult{}, fmt.Errorf("begin task create transaction: %w", err)
	}
	defer tx.Rollback()

	alias, err := s.nextTaskAlias(ctx, tx, projectID)
	if err != nil {
		return TaskCreateResult{}, err
	}
	taskID := stableMigrationID("task", projectID, alias)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.ExecContext(ctx, `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, taskID, projectID, specID, title, "todo", priority, now, now)
	if err != nil {
		return TaskCreateResult{}, fmt.Errorf("insert task %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "task", taskID, "task", alias, now); err != nil {
		return TaskCreateResult{}, err
	}

	if spec != nil {
		relationshipID := stableMigrationID("relationship", projectID, "task", taskID, "implements", "spec", spec.ID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'task', ?, 'spec', ?, 'implements', 'recorded by task create', 'command', ?, ?)
`, relationshipID, projectID, taskID, spec.ID, now, now); err != nil {
			return TaskCreateResult{}, fmt.Errorf("record task spec relationship: %w", err)
		}
	}
	for _, dependency := range dependencies {
		relationshipID := stableMigrationID("relationship", projectID, "task", taskID, "blocked_by", "task", dependency.ID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'task', ?, 'task', ?, 'blocked_by', 'recorded by task create', 'command', ?, ?)
`, relationshipID, projectID, taskID, dependency.ID, now, now); err != nil {
			return TaskCreateResult{}, fmt.Errorf("record task dependency relationship: %w", err)
		}
	}

	eventID := stableMigrationID("event", projectID, "task", taskID, "created", "todo")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'task', ?, 'status_changed', NULL, 'todo', 'recorded by task create', ?, ?)
`, eventID, projectID, taskID, now, now); err != nil {
		return TaskCreateResult{}, fmt.Errorf("record task create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TaskCreateResult{}, fmt.Errorf("commit task create transaction: %w", err)
	}

	task := TraceEntity{Kind: "task", ID: taskID, Alias: alias, Title: title, Status: "todo"}
	return TaskCreateResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Task:               task,
		Priority:           priority,
		Spec:               spec,
		Depends:            dependencies,
		EventID:            eventID,
	}, nil
}

func (s *Store) nextTaskAlias(ctx context.Context, tx *sql.Tx, projectID string) (string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT alias
FROM aliases
WHERE project_id = ? AND namespace = 'task'
`, projectID)
	if err != nil {
		return "", fmt.Errorf("query task aliases: %w", err)
	}
	defer rows.Close()

	maxID := 0
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return "", fmt.Errorf("scan task alias: %w", err)
		}
		match := taskAliasPattern.FindStringSubmatch(alias)
		if len(match) != 2 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if id > maxID {
			maxID = id
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate task aliases: %w", err)
	}

	for next := maxID + 1; ; next++ {
		alias := fmt.Sprintf("TASK-%03d", next)
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'task' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check task alias %s: %w", alias, err)
		}
	}
}

func insertAlias(ctx context.Context, tx *sql.Tx, projectID string, entityKind string, entityID string, namespace string, alias string, now string) error {
	id := stableMigrationID("alias", projectID, namespace, alias)
	_, err := tx.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, entityKind, entityID, namespace, alias, now, now)
	if err != nil {
		return fmt.Errorf("insert alias %s:%s: %w", namespace, alias, err)
	}
	return nil
}

// ValidTaskPriority reports whether priority is a known task priority.
func ValidTaskPriority(priority string) bool {
	for _, valid := range taskPriorityOrder {
		if priority == valid {
			return true
		}
	}
	return false
}

// TaskPriorities returns valid task priorities in display order.
func TaskPriorities() []string {
	return append([]string(nil), taskPriorityOrder...)
}

func taskPriorityText() string {
	return strings.Join(TaskPriorities(), ", ")
}
