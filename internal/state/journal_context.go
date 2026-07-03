package state

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// JournalContextOptions describes a continuity digest request.
type JournalContextOptions struct {
	// Branch scopes the "recent branch entries" layer. When empty the branch
	// layer is omitted and the digest degrades to the project wrap plus open
	// tasks.
	Branch string
	// BranchLimit caps the recent branch entries layer. Zero applies the
	// default limit.
	BranchLimit int
}

// JournalContext is the state-backed continuity digest read model. It is
// computed at read time and never persisted: the latest project-level wrap, the
// recent entries scoped to the current branch, and the open tasks from the
// tasks table.
type JournalContext struct {
	ContractVersion    int                  `json:"contract_version,omitempty"`
	DatabaseScope      string               `json:"database_scope,omitempty"`
	DatabasePath       string               `json:"database_path,omitempty"`
	ProjectID          string               `json:"project_id,omitempty"`
	ProjectName        string               `json:"project_name,omitempty"`
	ProjectCurrentPath string               `json:"project_current_path,omitempty"`
	Branch             string               `json:"branch,omitempty"`
	LatestWrap         *JournalEntryRecord  `json:"latest_wrap,omitempty"`
	BranchEntries      []JournalEntryRecord `json:"branch_entries"`
	OpenTasks          []JournalContextTask `json:"open_tasks"`
}

// JournalContextTask is an open task surfaced in the continuity digest.
type JournalContextTask struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
	Spec     string `json:"spec,omitempty"`
}

const defaultJournalContextBranchLimit = 10

// JournalContextForRoot computes the continuity digest from initialized SQLite state.
func JournalContextForRoot(ctx context.Context, root project.Root, resolver PathResolver, options JournalContextOptions) (JournalContext, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return JournalContext{}, err
	}
	defer store.Close()
	return store.JournalContext(ctx, root, options)
}

// JournalContext computes the continuity digest from an open store.
func (s *Store) JournalContext(ctx context.Context, root project.Root, options JournalContextOptions) (JournalContext, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return JournalContext{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return JournalContext{}, err
	}
	branch := strings.TrimSpace(options.Branch)
	branchLimit := options.BranchLimit
	if branchLimit <= 0 {
		branchLimit = defaultJournalContextBranchLimit
	}

	latestWrap, err := s.latestProjectWrap(ctx, projectID)
	if err != nil {
		return JournalContext{}, err
	}

	branchEntries := []JournalEntryRecord{}
	if branch != "" {
		branchEntries, err = s.journalTimeline(ctx, projectID, branch, "", 0, branchLimit)
		if err != nil {
			return JournalContext{}, err
		}
	}

	openTasks, err := s.openTasksForContext(ctx, root)
	if err != nil {
		return JournalContext{}, err
	}

	return JournalContext{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Branch:             branch,
		LatestWrap:         latestWrap,
		BranchEntries:      branchEntries,
		OpenTasks:          openTasks,
	}, nil
}

// latestProjectWrap returns the most recent project-scoped wrap entry, or nil
// when none exists (the digest then degrades to project-wrap-only absence:
// branch entries plus open tasks, or nothing on a fresh project).
func (s *Store) latestProjectWrap(ctx context.Context, projectID string) (*JournalEntryRecord, error) {
	var entry JournalEntryRecord
	err := s.db.QueryRowContext(ctx, `
SELECT
  id,
  entry_type,
  COALESCE(scope, ''),
  message,
  COALESCE(observed_branch, ''),
  COALESCE(observed_worktree, ''),
  COALESCE(harness_session_id, ''),
  created_at
FROM journal_entries
WHERE project_id = ? AND entry_type = 'wrap'
ORDER BY created_at DESC, rowid DESC
LIMIT 1
`, projectID).Scan(
		&entry.ID,
		&entry.EntryType,
		&entry.Scope,
		&entry.Message,
		&entry.ObservedBranch,
		&entry.ObservedWorktree,
		&entry.HarnessSessionID,
		&entry.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query latest project wrap: %w", err)
	}
	return &entry, nil
}

// openTasksForContext returns the project's open (in_progress/pending) tasks in
// a deterministic order for the continuity digest. It reuses the task-list read
// model so status vocabulary and display rules stay consistent with `loaf task`.
func (s *Store) openTasksForContext(ctx context.Context, root project.Root) ([]JournalContextTask, error) {
	list, err := s.ListTasks(ctx, root, TaskListOptions{Active: true})
	if err != nil {
		return nil, err
	}
	tasks := make([]JournalContextTask, 0, len(list.Tasks))
	for ref, item := range list.Tasks {
		tasks = append(tasks, JournalContextTask{
			Ref:      ref,
			Title:    item.Title,
			Status:   item.Status,
			Priority: item.Priority,
			Spec:     item.Spec,
		})
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Ref < tasks[j].Ref
	})
	return tasks, nil
}
