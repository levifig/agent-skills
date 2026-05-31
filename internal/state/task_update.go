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

// TaskUpdateOptions describes SQLite-backed task metadata updates.
type TaskUpdateOptions struct {
	Ref          string
	Status       string
	SetStatus    bool
	Priority     string
	SetPriority  bool
	Spec         string
	SetSpec      bool
	DependsOn    []string
	SetDependsOn bool
	Session      string
	SetSession   bool
}

// TaskStatusUpdateResult describes a task status mutation.
type TaskStatusUpdateResult struct {
	Task     TraceEntity   `json:"task"`
	Previous string        `json:"previous_status"`
	Status   string        `json:"status"`
	Priority string        `json:"priority,omitempty"`
	Spec     *TraceEntity  `json:"spec,omitempty"`
	Depends  []TraceEntity `json:"depends_on,omitempty"`
	Session  *TraceEntity  `json:"session,omitempty"`
	EventID  string        `json:"event_id,omitempty"`
}

// UpdateTaskStatus updates a task's status in initialized SQLite state.
func UpdateTaskStatus(ctx context.Context, root project.Root, resolver PathResolver, ref string, status string) (TaskStatusUpdateResult, error) {
	return UpdateTask(ctx, root, resolver, TaskUpdateOptions{Ref: ref, Status: status, SetStatus: true})
}

// UpdateTask updates a task in initialized SQLite state.
func UpdateTask(ctx context.Context, root project.Root, resolver PathResolver, options TaskUpdateOptions) (TaskStatusUpdateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TaskStatusUpdateResult{}, err
	}
	defer store.Close()
	return store.UpdateTask(ctx, root, options)
}

// UpdateTaskStatus updates a task's status in an open store.
func (s *Store) UpdateTaskStatus(ctx context.Context, root project.Root, ref string, status string) (TaskStatusUpdateResult, error) {
	return s.UpdateTask(ctx, root, TaskUpdateOptions{Ref: ref, Status: status, SetStatus: true})
}

// UpdateTask updates a task in an open store.
func (s *Store) UpdateTask(ctx context.Context, root project.Root, options TaskUpdateOptions) (TaskStatusUpdateResult, error) {
	projectID := ProjectID(root)
	if !options.SetStatus && !options.SetPriority && !options.SetSpec && !options.SetDependsOn && !options.SetSession {
		return TaskStatusUpdateResult{}, fmt.Errorf("task update requires at least one update")
	}
	if options.SetStatus && !ValidTaskStatus(options.Status) {
		return TaskStatusUpdateResult{}, fmt.Errorf("invalid task status %q", options.Status)
	}
	if options.SetPriority && !ValidTaskPriority(options.Priority) {
		return TaskStatusUpdateResult{}, fmt.Errorf("invalid priority %q", options.Priority)
	}
	task, err := s.resolveTraceEntity(ctx, projectID, options.Ref)
	if err != nil {
		return TaskStatusUpdateResult{}, err
	}
	if task.Kind != "task" {
		return TaskStatusUpdateResult{}, fmt.Errorf("%q resolves to %s, not task", options.Ref, task.Kind)
	}

	var spec *TraceEntity
	if options.SetSpec && !isNoneValue(options.Spec) {
		resolved, err := s.resolveTraceEntity(ctx, projectID, options.Spec)
		if err != nil {
			return TaskStatusUpdateResult{}, fmt.Errorf("resolve spec %q: %w", options.Spec, err)
		}
		if resolved.Kind != "spec" {
			return TaskStatusUpdateResult{}, fmt.Errorf("%q resolves to %s, not spec", options.Spec, resolved.Kind)
		}
		spec = &resolved
	}

	dependencies := []TraceEntity{}
	if options.SetDependsOn {
		for _, dependencyRef := range options.DependsOn {
			if isNoneValue(dependencyRef) {
				continue
			}
			dependency, err := s.resolveTraceEntity(ctx, projectID, dependencyRef)
			if err != nil {
				return TaskStatusUpdateResult{}, fmt.Errorf("resolve dependency %q: %w", dependencyRef, err)
			}
			if dependency.Kind != "task" {
				return TaskStatusUpdateResult{}, fmt.Errorf("%q resolves to %s, not task", dependencyRef, dependency.Kind)
			}
			dependencies = append(dependencies, dependency)
		}
	}

	var session *TraceEntity
	if options.SetSession && !isNoneValue(options.Session) {
		resolved, err := s.resolveTraceEntity(ctx, projectID, options.Session)
		if err != nil {
			return TaskStatusUpdateResult{}, fmt.Errorf("resolve session %q: %w", options.Session, err)
		}
		if resolved.Kind != "session" {
			return TaskStatusUpdateResult{}, fmt.Errorf("%q resolves to %s, not session", options.Session, resolved.Kind)
		}
		session = &resolved
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskStatusUpdateResult{}, fmt.Errorf("begin task update transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	var previousPriority, previousSpecID sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT status, priority, spec_id FROM tasks WHERE project_id = ? AND id = ?`, projectID, task.ID).Scan(&previousStatus, &previousPriority, &previousSpecID)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskStatusUpdateResult{}, fmt.Errorf("task %q not found in SQLite state", options.Ref)
	}
	if err != nil {
		return TaskStatusUpdateResult{}, fmt.Errorf("read task metadata: %w", err)
	}

	finalStatus := previousStatus
	if options.SetStatus {
		finalStatus = options.Status
	}
	finalPriority := previousPriority.String
	if options.SetPriority {
		finalPriority = options.Priority
	}
	var finalSpecID any
	if previousSpecID.Valid {
		finalSpecID = previousSpecID.String
	}
	if options.SetSpec {
		finalSpecID = nil
		if spec != nil {
			finalSpecID = spec.ID
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status = ?, priority = ?, spec_id = ?, updated_at = ? WHERE project_id = ? AND id = ?`, finalStatus, emptyToNil(finalPriority), finalSpecID, now, projectID, task.ID); err != nil {
		return TaskStatusUpdateResult{}, fmt.Errorf("update task metadata: %w", err)
	}

	if options.SetSpec {
		if err := replaceTaskRelationships(ctx, tx, projectID, task.ID, "implements", "spec", nil, now); err != nil {
			return TaskStatusUpdateResult{}, err
		}
		if spec != nil {
			if err := insertTaskRelationship(ctx, tx, projectID, task.ID, "implements", "spec", spec.ID, "recorded by task update", now); err != nil {
				return TaskStatusUpdateResult{}, err
			}
		}
	}
	if options.SetDependsOn {
		if err := replaceTaskRelationships(ctx, tx, projectID, task.ID, "blocked_by", "task", dependencies, now); err != nil {
			return TaskStatusUpdateResult{}, err
		}
	}
	if options.SetSession {
		if err := replaceTaskRelationships(ctx, tx, projectID, task.ID, "associated_with", "session", nil, now); err != nil {
			return TaskStatusUpdateResult{}, err
		}
		if session != nil {
			if err := insertTaskRelationship(ctx, tx, projectID, task.ID, "associated_with", "session", session.ID, "recorded by task update", now); err != nil {
				return TaskStatusUpdateResult{}, err
			}
		}
	}

	eventID := ""
	if options.SetStatus && previousStatus != finalStatus {
		eventID = stableMigrationID("event", projectID, "task", task.ID, "status", previousStatus, finalStatus)
		_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "task", task.ID, "status_changed", previousStatus, finalStatus, "recorded by task update", now, now)
		if err != nil {
			return TaskStatusUpdateResult{}, fmt.Errorf("record task status event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return TaskStatusUpdateResult{}, fmt.Errorf("commit task update transaction: %w", err)
	}

	task.Status = finalStatus
	resultSpec := spec
	if !options.SetSpec && previousSpecID.Valid {
		if resolved, err := s.entityDetails(ctx, projectID, "spec", previousSpecID.String); err == nil {
			if alias, err := s.entityAlias(ctx, projectID, "spec", previousSpecID.String); err == nil {
				resolved.Alias = alias
			}
			resultSpec = &resolved
		}
	}
	resultDepends := dependencies
	if !options.SetDependsOn {
		resultDepends = nil
	}
	resultSession := session
	return TaskStatusUpdateResult{
		Task:     task,
		Previous: previousStatus,
		Status:   finalStatus,
		Priority: finalPriority,
		Spec:     resultSpec,
		Depends:  resultDepends,
		Session:  resultSession,
		EventID:  eventID,
	}, nil
}

func replaceTaskRelationships(ctx context.Context, tx *sql.Tx, projectID string, taskID string, relationshipType string, targetKind string, targets []TraceEntity, now string) error {
	if _, err := tx.ExecContext(ctx, `
DELETE FROM relationships
WHERE project_id = ?
  AND from_entity_kind = 'task'
  AND from_entity_id = ?
  AND relationship_type = ?
  AND to_entity_kind = ?
`, projectID, taskID, relationshipType, targetKind); err != nil {
		return fmt.Errorf("clear task %s relationships: %w", relationshipType, err)
	}
	for _, target := range targets {
		if err := insertTaskRelationship(ctx, tx, projectID, taskID, relationshipType, targetKind, target.ID, "recorded by task update", now); err != nil {
			return err
		}
	}
	return nil
}

func insertTaskRelationship(ctx context.Context, tx *sql.Tx, projectID string, taskID string, relationshipType string, targetKind string, targetID string, reason string, now string) error {
	relationshipID := stableMigrationID("relationship", projectID, "task", taskID, relationshipType, targetKind, targetID)
	_, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES (?, ?, 'task', ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  updated_at = excluded.updated_at
`, relationshipID, projectID, taskID, targetKind, targetID, relationshipType, reason, now, now)
	if err != nil {
		return fmt.Errorf("record task %s relationship: %w", relationshipType, err)
	}
	return nil
}

func isNoneValue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "none")
}
