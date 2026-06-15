package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// TaskArchiveOptions describes a SQLite-backed task archive request.
type TaskArchiveOptions struct {
	Refs []string
	Spec string
}

// TaskArchiveResult describes a state-backed task archive mutation.
type TaskArchiveResult struct {
	ContractVersion    int               `json:"contract_version,omitempty"`
	DatabaseScope      string            `json:"database_scope,omitempty"`
	DatabasePath       string            `json:"database_path,omitempty"`
	ProjectID          string            `json:"project_id,omitempty"`
	ProjectName        string            `json:"project_name,omitempty"`
	ProjectCurrentPath string            `json:"project_current_path,omitempty"`
	Spec               *TraceEntity      `json:"spec,omitempty"`
	Archived           []TaskArchiveItem `json:"archived"`
	Skipped            []TaskArchiveItem `json:"skipped"`
}

// TaskArchiveItem describes one requested task archive outcome.
type TaskArchiveItem struct {
	Task     *TraceEntity `json:"task,omitempty"`
	Ref      string       `json:"ref,omitempty"`
	Previous string       `json:"previous_status,omitempty"`
	Status   string       `json:"status,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	EventID  string       `json:"event_id,omitempty"`
}

// ArchiveTasks archives done tasks in initialized SQLite state.
func ArchiveTasks(ctx context.Context, root project.Root, resolver PathResolver, options TaskArchiveOptions) (TaskArchiveResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TaskArchiveResult{}, err
	}
	defer store.Close()
	return store.ArchiveTasks(ctx, root, options)
}

// ArchiveTasks archives done tasks in an open store.
func (s *Store) ArchiveTasks(ctx context.Context, root project.Root, options TaskArchiveOptions) (TaskArchiveResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TaskArchiveResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TaskArchiveResult{}, err
	}
	if options.Spec != "" && len(options.Refs) > 0 {
		return TaskArchiveResult{}, fmt.Errorf("task archive accepts task ids or --spec, not both")
	}
	if options.Spec == "" && len(options.Refs) == 0 {
		return TaskArchiveResult{}, fmt.Errorf("task archive requires task ids or --spec")
	}

	result := TaskArchiveResult{Archived: []TaskArchiveItem{}, Skipped: []TaskArchiveItem{}}
	result.ContractVersion = StateJSONContractVersion
	result.DatabaseScope = identity.DatabaseScope
	result.DatabasePath = identity.DatabasePath
	result.ProjectID = identity.ID
	result.ProjectName = identity.FriendlyName
	result.ProjectCurrentPath = identity.CurrentPath
	refs := options.Refs
	if options.Spec != "" {
		spec, err := s.resolveTraceEntity(ctx, projectID, options.Spec)
		if err != nil {
			return TaskArchiveResult{}, fmt.Errorf("resolve spec %q: %w", options.Spec, err)
		}
		if spec.Kind != "spec" {
			return TaskArchiveResult{}, fmt.Errorf("%q resolves to %s, not spec", options.Spec, spec.Kind)
		}
		result.Spec = &spec
		doneRefs, err := s.doneTaskAliasesForSpec(ctx, projectID, spec.ID)
		if err != nil {
			return TaskArchiveResult{}, err
		}
		refs = doneRefs
	}

	for _, ref := range refs {
		item, archived, err := s.archiveTask(ctx, projectID, ref)
		if err != nil {
			return TaskArchiveResult{}, err
		}
		if archived {
			result.Archived = append(result.Archived, item)
		} else {
			result.Skipped = append(result.Skipped, item)
		}
	}
	return result, nil
}

func (s *Store) doneTaskAliasesForSpec(ctx context.Context, projectID string, specID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT task_alias.alias
FROM tasks
JOIN aliases task_alias
  ON task_alias.project_id = tasks.project_id
 AND task_alias.entity_kind = 'task'
 AND task_alias.entity_id = tasks.id
 AND task_alias.namespace = 'task'
WHERE tasks.project_id = ?
  AND tasks.spec_id = ?
  AND tasks.status = 'done'
ORDER BY task_alias.alias
`, projectID, specID)
	if err != nil {
		return nil, fmt.Errorf("query done tasks for spec: %w", err)
	}
	defer rows.Close()

	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scan done task alias: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate done task aliases: %w", err)
	}
	return refs, nil
}

func (s *Store) archiveTask(ctx context.Context, projectID string, ref string) (TaskArchiveItem, bool, error) {
	task, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return TaskArchiveItem{Ref: ref, Reason: err.Error()}, false, nil
	}
	if task.Kind != "task" {
		return TaskArchiveItem{Task: &task, Ref: ref, Reason: fmt.Sprintf("%q resolves to %s, not task", ref, task.Kind)}, false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskArchiveItem{}, false, fmt.Errorf("begin task archive transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM tasks WHERE project_id = ? AND id = ?`, projectID, task.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskArchiveItem{Task: &task, Ref: ref, Reason: fmt.Sprintf("task %q not found in SQLite state", ref)}, false, nil
	}
	if err != nil {
		return TaskArchiveItem{}, false, fmt.Errorf("read task status: %w", err)
	}

	if previousStatus == "archived" {
		return TaskArchiveItem{Task: &task, Ref: ref, Previous: previousStatus, Status: previousStatus, Reason: "already archived"}, false, nil
	}
	if previousStatus != "done" {
		return TaskArchiveItem{Task: &task, Ref: ref, Previous: previousStatus, Status: previousStatus, Reason: fmt.Sprintf("status is %s, must be done", previousStatus)}, false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, "archived", now, projectID, task.ID); err != nil {
		return TaskArchiveItem{}, false, fmt.Errorf("update task status: %w", err)
	}

	eventID := stableMigrationID("event", projectID, "task", task.ID, "status", previousStatus, "archived")
	_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "task", task.ID, "status_changed", previousStatus, "archived", "recorded by task archive", now, now)
	if err != nil {
		return TaskArchiveItem{}, false, fmt.Errorf("record task archive event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TaskArchiveItem{}, false, fmt.Errorf("commit task archive transaction: %w", err)
	}

	task.Status = "archived"
	return TaskArchiveItem{Task: &task, Ref: ref, Previous: previousStatus, Status: "archived", EventID: eventID}, true, nil
}
