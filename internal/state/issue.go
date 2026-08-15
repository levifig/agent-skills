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

const (
	IssueKindDelivery = "delivery"
	IssueKindDecision = "decision"

	IssueStatusTriage    = "triage"
	IssueStatusBacklog   = "backlog"
	IssueStatusTodo      = "todo"
	IssueStatusActive    = "active"
	IssueStatusDone      = "done"
	IssueStatusCancelled = "cancelled"
	IssueStatusDuplicate = "duplicate"

	IssueRelationshipRelatesTo = "relates_to"
	IssueRelationshipBlocks    = "blocks"
	IssueRelationshipBlockedBy = "blocked_by"

	IssueBucketNow   = "now"
	IssueBucketNext  = "next"
	IssueBucketLater = "later"
	IssueBucketNone  = "none"

	issueEntityKind      = "issue"
	issueNamespace       = "issue"
	issueBucketTagPrefix = "bucket:"
)

// IssueValidationError identifies malformed issue input.
type IssueValidationError struct {
	Field string
	Err   error
}

func (e *IssueValidationError) Error() string {
	if e == nil {
		return "issue validation failed"
	}
	return fmt.Sprintf("issue validation failed for %s: %v", e.Field, e.Err)
}

func (e *IssueValidationError) Unwrap() error { return e.Err }

// IssueTransactionError identifies the transactional stage that failed.
type IssueTransactionError struct {
	Stage string
	Err   error
}

func (e *IssueTransactionError) Error() string {
	if e == nil {
		return "issue transaction failed"
	}
	return fmt.Sprintf("issue transaction failed at %s: %v", e.Stage, e.Err)
}

func (e *IssueTransactionError) Unwrap() error { return e.Err }

// Issue is the derived read model for one issue.
type Issue struct {
	ID              string           `json:"id"`
	Alias           string           `json:"alias,omitempty"`
	ParentID        string           `json:"parent_id,omitempty"`
	Kind            string           `json:"kind"`
	Title           string           `json:"title"`
	Body            string           `json:"body"`
	Fog             string           `json:"fog,omitempty"`
	Status          string           `json:"status"`
	ArchivedAt      string           `json:"archived_at,omitempty"`
	StartedBranch   string           `json:"started_branch,omitempty"`
	StartedWorktree string           `json:"started_worktree,omitempty"`
	WorktreeMissing bool             `json:"worktree_missing,omitempty"`
	Criteria        []IssueCriterion `json:"criteria,omitempty"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
}

// IssueCreateOptions describes a new issue.
type IssueCreateOptions struct {
	Title    string
	Body     string
	Fog      string
	Kind     string
	Parent   string
	Criteria []IssueCriterionInput
}

// IssueUpdateOptions describes a partial issue mutation. Title, body, fog,
// and kind remain writable at every status — including cancelled and duplicate.
// StartedBranch and StartedWorktree are workspace facts written together
// through the same transaction as a status move when start records them.
type IssueUpdateOptions struct {
	Ref             string
	Title           string
	SetTitle        bool
	Body            string
	SetBody         bool
	Fog             string
	SetFog          bool
	Kind            string
	SetKind         bool
	Parent          string
	SetParent       bool
	Status          string
	SetStatus       bool
	StartedBranch   string
	StartedWorktree string
	SetStarted      bool
}

// IssueRemoveOptions cancels or marks an issue duplicate and archives it.
// The record and its relationship edges survive.
type IssueRemoveOptions struct {
	Ref         string
	Status      string
	DuplicateOf string
}

// IssueStatusParityMismatch is one issue whose status column disagrees with
// the latest events.to_status.
type IssueStatusParityMismatch struct {
	IssueID      string `json:"issue_id"`
	ColumnStatus string `json:"column_status"`
	EventStatus  string `json:"event_status"`
}

// IssueStatusParityResult is the projection check for a project's issues.
type IssueStatusParityResult struct {
	Consistent bool                        `json:"consistent"`
	Mismatches []IssueStatusParityMismatch `json:"mismatches,omitempty"`
}

var issueStatuses = []string{
	IssueStatusTriage,
	IssueStatusBacklog,
	IssueStatusTodo,
	IssueStatusActive,
	IssueStatusDone,
	IssueStatusCancelled,
	IssueStatusDuplicate,
}

var issueWriteStatuses = []string{
	IssueStatusTriage,
	IssueStatusBacklog,
	IssueStatusTodo,
	IssueStatusActive,
	IssueStatusDone,
}

func validIssueStatus(status string) bool {
	for _, candidate := range issueStatuses {
		if status == candidate {
			return true
		}
	}
	return false
}

func validIssueWriteStatus(status string) bool {
	for _, candidate := range issueWriteStatuses {
		if status == candidate {
			return true
		}
	}
	return false
}

// IssueWriteStatuses returns the statuses CreateIssue/UpdateIssue may write
// (triage, backlog, todo, active, done). Removal statuses are not included.
func IssueWriteStatuses() []string {
	out := make([]string, len(issueWriteStatuses))
	copy(out, issueWriteStatuses)
	return out
}

func validIssueKind(kind string) bool {
	return kind == IssueKindDelivery || kind == IssueKindDecision
}

func normalizeIssueTitle(value string) (string, error) {
	title := strings.TrimSpace(value)
	if title == "" {
		return "", &IssueValidationError{Field: "title", Err: fmt.Errorf("must be nonempty")}
	}
	return title, nil
}

func normalizeIssueKind(value string) (string, error) {
	kind := strings.TrimSpace(value)
	if kind == "" {
		return IssueKindDelivery, nil
	}
	if !validIssueKind(kind) {
		return "", &IssueValidationError{Field: "kind", Err: fmt.Errorf("must be delivery or decision")}
	}
	return kind, nil
}

// CreateIssue writes one issue, its initial status event, and a local alias
// when the project authority is local.
func CreateIssue(ctx context.Context, root project.Root, resolver PathResolver, options IssueCreateOptions) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.CreateIssue(ctx, root, options)
}

// CreateIssue writes one issue in a serializable transaction on an open store.
func (s *Store) CreateIssue(ctx context.Context, root project.Root, options IssueCreateOptions) (Issue, error) {
	title, err := normalizeIssueTitle(options.Title)
	if err != nil {
		return Issue{}, err
	}
	kind, err := normalizeIssueKind(options.Kind)
	if err != nil {
		return Issue{}, err
	}
	criteria, err := normalizeIssueCriteria(options.Criteria)
	if err != nil {
		return Issue{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	issueID, err := newOpaqueStateID("issue")
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "id", Err: err}
	}

	parentID := ""
	if strings.TrimSpace(options.Parent) != "" {
		parentID, _, err = resolveIssueRefTx(ctx, tx, projectID, options.Parent)
		if err != nil {
			return Issue{}, err
		}
		if err := rejectIssueParentCycle(ctx, tx, projectID, issueID, parentID); err != nil {
			return Issue{}, err
		}
	}

	alias, err := mintLocalIssueAliasTx(ctx, tx, projectID, now)
	if err != nil {
		return Issue{}, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO issues (id, project_id, parent_id, kind, title, body, fog, status, archived_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, issueID, projectID, emptyToNil(parentID), kind, title, options.Body, emptyToNil(strings.TrimSpace(options.Fog)), IssueStatusTriage, now, now); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "issue", Err: err}
	}
	if alias != "" {
		if err := insertAlias(ctx, tx, projectID, issueEntityKind, issueID, issueNamespace, alias, now); err != nil {
			return Issue{}, &IssueTransactionError{Stage: "alias", Err: err}
		}
	}
	if err := replaceIssueCriteriaTx(ctx, tx, projectID, issueID, criteria, now); err != nil {
		return Issue{}, err
	}
	if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issueID, "", IssueStatusTriage, "recorded by issue create", now); err != nil {
		return Issue{}, err
	}

	detail, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return detail, nil
}

// UpdateIssue mutates content, parent, kind, or a non-removal status.
func UpdateIssue(ctx context.Context, root project.Root, resolver PathResolver, options IssueUpdateOptions) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.UpdateIssue(ctx, root, options)
}

// UpdateIssue mutates an issue in a serializable transaction on an open store.
func (s *Store) UpdateIssue(ctx context.Context, root project.Root, options IssueUpdateOptions) (Issue, error) {
	if !options.SetTitle && !options.SetBody && !options.SetFog && !options.SetKind && !options.SetParent && !options.SetStatus && !options.SetStarted {
		return Issue{}, &IssueValidationError{Field: "update", Err: fmt.Errorf("requires at least one field")}
	}
	if options.SetTitle {
		if _, err := normalizeIssueTitle(options.Title); err != nil {
			return Issue{}, err
		}
	}
	if options.SetKind {
		if _, err := normalizeIssueKind(options.Kind); err != nil {
			return Issue{}, err
		}
		if strings.TrimSpace(options.Kind) == "" {
			return Issue{}, &IssueValidationError{Field: "kind", Err: fmt.Errorf("must be delivery or decision")}
		}
	}
	if options.SetStatus {
		status := strings.TrimSpace(options.Status)
		if !validIssueWriteStatus(status) {
			if validIssueStatus(status) {
				return Issue{}, &IssueValidationError{Field: "status", Err: fmt.Errorf("%s is a removal status; use RemoveIssue", status)}
			}
			return Issue{}, &IssueValidationError{Field: "status", Err: fmt.Errorf("must be one of triage, backlog, todo, active, done")}
		}
	}
	if options.SetStarted {
		startedBranch := strings.TrimSpace(options.StartedBranch)
		startedWorktree := strings.TrimSpace(options.StartedWorktree)
		if (startedBranch == "") != (startedWorktree == "") {
			return Issue{}, &IssueValidationError{Field: "started", Err: fmt.Errorf("started_branch and started_worktree must be set or cleared together")}
		}
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, options.Ref)
	if err != nil {
		return Issue{}, err
	}
	current, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read", Err: err}
	}

	title := current.Title
	if options.SetTitle {
		title = strings.TrimSpace(options.Title)
	}
	body := current.Body
	if options.SetBody {
		body = options.Body
	}
	fog := current.Fog
	if options.SetFog {
		fog = strings.TrimSpace(options.Fog)
	}
	kind := current.Kind
	if options.SetKind {
		kind = strings.TrimSpace(options.Kind)
	}
	parentID := current.ParentID
	if options.SetParent {
		parentID = ""
		if strings.TrimSpace(options.Parent) != "" {
			parentID, _, err = resolveIssueRefTx(ctx, tx, projectID, options.Parent)
			if err != nil {
				return Issue{}, err
			}
			if err := rejectIssueParentCycle(ctx, tx, projectID, issueID, parentID); err != nil {
				return Issue{}, err
			}
		}
	}
	status := current.Status
	if options.SetStatus {
		status = strings.TrimSpace(options.Status)
	}
	startedBranch := current.StartedBranch
	startedWorktree := current.StartedWorktree
	if options.SetStarted {
		startedBranch = strings.TrimSpace(options.StartedBranch)
		startedWorktree = strings.TrimSpace(options.StartedWorktree)
		if startedBranch != "" || startedWorktree != "" {
			if current.ArchivedAt != "" {
				return Issue{}, &IssueValidationError{Field: "started", Err: fmt.Errorf("archived issues cannot be started")}
			}
			if issueStartRefusedStatus(current.Status) {
				return Issue{}, &IssueValidationError{Field: "started", Err: fmt.Errorf("%s issues cannot be started", current.Status)}
			}
			if issueIsStarted(current) {
				return Issue{}, &IssueValidationError{Field: "started", Err: fmt.Errorf("issue is already started")}
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
UPDATE issues
SET parent_id = ?, kind = ?, title = ?, body = ?, fog = ?, status = ?, started_branch = ?, started_worktree = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, emptyToNil(parentID), kind, title, body, emptyToNil(fog), status, emptyToNil(startedBranch), emptyToNil(startedWorktree), now, projectID, issueID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "update", Err: err}
	}
	if options.SetStatus && status != current.Status {
		if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issueID, current.Status, status, "recorded by issue update", now); err != nil {
			return Issue{}, err
		}
	}

	detail, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return detail, nil
}

// GetIssue returns the derived read model for one issue.
func GetIssue(ctx context.Context, root project.Root, resolver PathResolver, ref string) (Issue, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.GetIssue(ctx, root, ref)
}

// GetIssue returns the derived read model from an open store.
func (s *Store) GetIssue(ctx context.Context, root project.Root, ref string) (Issue, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Issue{}, fmt.Errorf("begin issue show: %w", err)
	}
	defer tx.Rollback()
	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Issue{}, err
	}
	return loadIssueTx(ctx, tx, projectID, issueID)
}

// RemoveIssue sets cancelled or duplicate through the events path and archives
// the issue. The record and its relationship edges survive. Duplicate requires
// the surviving issue and records a relates_to edge to it.
func RemoveIssue(ctx context.Context, root project.Root, resolver PathResolver, options IssueRemoveOptions) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.RemoveIssue(ctx, root, options)
}

// RemoveIssue archives an issue on an open store.
func (s *Store) RemoveIssue(ctx context.Context, root project.Root, options IssueRemoveOptions) (Issue, error) {
	status := strings.TrimSpace(options.Status)
	if status != IssueStatusCancelled && status != IssueStatusDuplicate {
		return Issue{}, &IssueValidationError{Field: "status", Err: fmt.Errorf("removal must be cancelled or duplicate")}
	}
	if status == IssueStatusDuplicate && strings.TrimSpace(options.DuplicateOf) == "" {
		return Issue{}, &IssueValidationError{Field: "duplicate_of", Err: fmt.Errorf("duplicate removal requires a surviving issue")}
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, options.Ref)
	if err != nil {
		return Issue{}, err
	}
	current, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read", Err: err}
	}

	var survivorID string
	if status == IssueStatusDuplicate {
		survivorID, _, err = resolveIssueRefTx(ctx, tx, projectID, options.DuplicateOf)
		if err != nil {
			return Issue{}, err
		}
		if survivorID == issueID {
			return Issue{}, &IssueValidationError{Field: "duplicate_of", Err: fmt.Errorf("surviving issue must be a different issue")}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	archivedAt := current.ArchivedAt
	if archivedAt == "" {
		archivedAt = now
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE issues SET status = ?, archived_at = ?, updated_at = ? WHERE project_id = ? AND id = ?
`, status, archivedAt, now, projectID, issueID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "archive", Err: err}
	}
	if status != current.Status {
		if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issueID, current.Status, status, "recorded by issue remove", now); err != nil {
			return Issue{}, err
		}
	}
	if survivorID != "" {
		relationshipID := stableMigrationID("relationship", projectID, issueEntityKind, issueID, IssueRelationshipRelatesTo, issueEntityKind, survivorID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  origin = excluded.origin,
  updated_at = excluded.updated_at
`, relationshipID, projectID, issueEntityKind, issueID, issueEntityKind, survivorID, IssueRelationshipRelatesTo, "duplicate of", "command", now, now); err != nil {
			return Issue{}, &IssueTransactionError{Stage: "relates_to", Err: err}
		}
	}

	detail, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return detail, nil
}

// HardDeleteIssue permanently removes an issue row and its criteria.
// It is a last-resort operator tool and must never be proposed by an agent.
// The issue's minted number is not freed: issue_identity.next_number is never
// decremented, and the number is never reissued.
func HardDeleteIssue(ctx context.Context, root project.Root, resolver PathResolver, ref string) error {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.HardDeleteIssue(ctx, root, ref)
}

// HardDeleteIssue permanently removes an issue row on an open store.
// It is a last-resort operator tool and must never be proposed by an agent.
func (s *Store) HardDeleteIssue(ctx context.Context, root project.Root, ref string) error {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE issues SET parent_id = NULL, updated_at = ? WHERE project_id = ? AND parent_id = ?
`, time.Now().UTC().Format(time.RFC3339Nano), projectID, issueID); err != nil {
		return &IssueTransactionError{Stage: "detach children", Err: err}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM issues WHERE project_id = ? AND id = ?`, projectID, issueID); err != nil {
		return &IssueTransactionError{Stage: "delete", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return &IssueTransactionError{Stage: "commit", Err: err}
	}
	return nil
}

// CheckIssueStatusParity proves issues.status equals the latest event to_status
// for every issue in the project.
func CheckIssueStatusParity(ctx context.Context, root project.Root, resolver PathResolver) (IssueStatusParityResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueStatusParityResult{}, err
	}
	defer store.Close()
	return store.CheckIssueStatusParity(ctx, root)
}

// CheckIssueStatusParity proves column == latest event on an open store.
func (s *Store) CheckIssueStatusParity(ctx context.Context, root project.Root) (IssueStatusParityResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueStatusParityResult{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT i.id, i.status,
  COALESCE((
    SELECT e.to_status FROM events e
    WHERE e.project_id = i.project_id AND e.entity_kind = ? AND e.entity_id = i.id
      AND e.event_type = 'status_changed'
    ORDER BY e.created_at DESC, e.rowid DESC
    LIMIT 1
  ), '')
FROM issues AS i
WHERE i.project_id = ?
ORDER BY i.created_at, i.id
`, issueEntityKind, projectID)
	if err != nil {
		return IssueStatusParityResult{}, fmt.Errorf("check issue status parity: %w", err)
	}
	defer rows.Close()

	result := IssueStatusParityResult{Consistent: true, Mismatches: []IssueStatusParityMismatch{}}
	for rows.Next() {
		var mismatch IssueStatusParityMismatch
		if err := rows.Scan(&mismatch.IssueID, &mismatch.ColumnStatus, &mismatch.EventStatus); err != nil {
			return IssueStatusParityResult{}, fmt.Errorf("scan issue status parity: %w", err)
		}
		if mismatch.ColumnStatus != mismatch.EventStatus {
			result.Consistent = false
			result.Mismatches = append(result.Mismatches, mismatch)
		}
	}
	if err := rows.Err(); err != nil {
		return IssueStatusParityResult{}, fmt.Errorf("iterate issue status parity: %w", err)
	}
	return result, nil
}

// ListLatestIssueDoneAt returns the created_at of each issue's latest
// status_changed event to done. Issues never marked done are omitted.
func ListLatestIssueDoneAt(ctx context.Context, root project.Root, resolver PathResolver) (map[string]string, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListLatestIssueDoneAt(ctx, root)
}

// ListLatestIssueDoneAt returns latest done-event timestamps from an open store.
func (s *Store) ListLatestIssueDoneAt(ctx context.Context, root project.Root) (map[string]string, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT entity_id, created_at
FROM events
WHERE project_id = ? AND entity_kind = ? AND event_type = 'status_changed' AND to_status = ?
ORDER BY created_at DESC, rowid DESC
`, projectID, issueEntityKind, IssueStatusDone)
	if err != nil {
		return nil, fmt.Errorf("list latest issue done events: %w", err)
	}
	defer rows.Close()
	latest := map[string]string{}
	for rows.Next() {
		var issueID, createdAt string
		if err := rows.Scan(&issueID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan latest issue done event: %w", err)
		}
		if _, exists := latest[issueID]; exists {
			continue
		}
		latest[issueID] = createdAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest issue done events: %w", err)
	}
	return latest, nil
}

func insertIssueStatusEventTx(ctx context.Context, tx *sql.Tx, projectID, issueID, fromStatus, toStatus, note, now string) (string, error) {
	eventID, err := newOpaqueStateID("evt")
	if err != nil {
		return "", &IssueTransactionError{Stage: "event id", Err: err}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, 'status_changed', ?, ?, ?, ?, ?)
`, eventID, projectID, issueEntityKind, issueID, emptyToNil(fromStatus), toStatus, note, now, now); err != nil {
		return "", &IssueTransactionError{Stage: "event", Err: err}
	}
	return eventID, nil
}

// rejectIssueParentCycle refuses a parent that is the issue itself or any
// descendant. The walk happens inside the write transaction.
func rejectIssueParentCycle(ctx context.Context, tx *sql.Tx, projectID, issueID, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == issueID {
		return &IssueValidationError{Field: "parent", Err: fmt.Errorf("an issue cannot be its own parent")}
	}
	visited := map[string]bool{issueID: true}
	current := parentID
	for current != "" {
		if visited[current] {
			return &IssueValidationError{Field: "parent", Err: fmt.Errorf("parent %s would create a cycle", parentID)}
		}
		visited[current] = true
		var next sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT parent_id FROM issues WHERE project_id = ? AND id = ?`, projectID, current).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			return &IssueValidationError{Field: "parent", Err: fmt.Errorf("issue %q not found in SQLite state", current)}
		}
		if err != nil {
			return &IssueTransactionError{Stage: "walk parent", Err: err}
		}
		current = next.String
	}
	return nil
}

func resolveIssueRefTx(ctx context.Context, tx *sql.Tx, projectID, ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", &IssueValidationError{Field: "issue", Err: fmt.Errorf("must be nonempty")}
	}
	var kind, id, alias string
	err := tx.QueryRowContext(ctx, `
SELECT entity_kind, entity_id, alias FROM aliases
WHERE project_id = ? AND namespace = ? AND alias = ?
`, projectID, issueNamespace, trimmed).Scan(&kind, &id, &alias)
	switch {
	case err == nil:
		if kind != issueEntityKind {
			return "", "", fmt.Errorf("%q resolves to %s, not an issue", trimmed, kind)
		}
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT id FROM issues WHERE project_id = ? AND id = ?`, projectID, id).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", fmt.Errorf("issue %q not found in SQLite state", trimmed)
		}
		if err != nil {
			return "", "", fmt.Errorf("resolve issue %q: %w", trimmed, err)
		}
		return existing, alias, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", "", fmt.Errorf("resolve issue %q: %w", trimmed, err)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM issues WHERE project_id = ? AND id = ?`, projectID, trimmed).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("issue %q not found in SQLite state", trimmed)
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve issue %q: %w", trimmed, err)
	}
	return existing, "", nil
}

func loadIssueTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) (Issue, error) {
	var issue Issue
	var parentID, fog, archivedAt, alias, startedBranch, startedWorktree sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT i.id, i.parent_id, i.kind, i.title, i.body, i.fog, i.status, i.archived_at, i.started_branch, i.started_worktree, i.created_at, i.updated_at,
  (SELECT a.alias FROM aliases a WHERE a.project_id = i.project_id AND a.entity_kind = ? AND a.entity_id = i.id ORDER BY a.namespace, a.alias LIMIT 1)
FROM issues AS i
WHERE i.project_id = ? AND i.id = ?
`, issueEntityKind, projectID, issueID).Scan(
		&issue.ID, &parentID, &issue.Kind, &issue.Title, &issue.Body, &fog, &issue.Status, &archivedAt, &startedBranch, &startedWorktree, &issue.CreatedAt, &issue.UpdatedAt, &alias,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, fmt.Errorf("issue %s not found", issueID)
	}
	if err != nil {
		return Issue{}, err
	}
	issue.ParentID = parentID.String
	issue.Fog = fog.String
	issue.ArchivedAt = archivedAt.String
	issue.StartedBranch = startedBranch.String
	issue.StartedWorktree = startedWorktree.String
	issue.Alias = alias.String
	criteria, err := loadIssueCriteriaTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, err
	}
	issue.Criteria = criteria
	return issue, nil
}

func issueIsStarted(issue Issue) bool {
	return strings.TrimSpace(issue.StartedBranch) != "" || strings.TrimSpace(issue.StartedWorktree) != ""
}

func issueStartRefusedStatus(status string) bool {
	switch status {
	case IssueStatusDone, IssueStatusCancelled, IssueStatusDuplicate:
		return true
	default:
		return false
	}
}

// NearestStartedAncestor walks parent_id and returns the nearest ancestor
// that itself has a recorded started workspace.
func NearestStartedAncestor(ctx context.Context, root project.Root, resolver PathResolver, ref string) (Issue, bool, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, false, err
	}
	defer store.Close()
	return store.NearestStartedAncestor(ctx, root, ref)
}

// NearestStartedAncestor walks parent_id from an open store.
func (s *Store) NearestStartedAncestor(ctx context.Context, root project.Root, ref string) (Issue, bool, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Issue{}, false, fmt.Errorf("begin nearest started ancestor: %w", err)
	}
	defer tx.Rollback()
	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Issue{}, false, err
	}
	issue, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, false, err
	}
	visited := map[string]bool{}
	current := issue.ParentID
	for current != "" {
		if visited[current] {
			return Issue{}, false, fmt.Errorf("parent cycle detected in stored issue data at %s", current)
		}
		visited[current] = true
		parent, err := loadIssueTx(ctx, tx, projectID, current)
		if err != nil {
			return Issue{}, false, err
		}
		if issueIsStarted(parent) {
			return parent, true, nil
		}
		current = parent.ParentID
	}
	return Issue{}, false, nil
}
