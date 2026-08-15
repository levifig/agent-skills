package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	IssueCriterionTierV = "V"
	IssueCriterionTierH = "H"
)

// IssueCriterionInput is one definition-of-done line to store.
// Command and Expect use the loaf change verify grammar (exit N, contains <text>).
type IssueCriterionInput struct {
	Text     string
	Command  string
	Expect   string
	Tier     string
	Position int
}

// IssueCriterion is one stored definition-of-done line.
type IssueCriterion struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Text     string `json:"text"`
	Command  string `json:"command,omitempty"`
	Expect   string `json:"expect,omitempty"`
	Tier     string `json:"tier"`
}

// ReplaceIssueCriteria replaces every criterion on an issue.
func ReplaceIssueCriteria(ctx context.Context, root project.Root, resolver PathResolver, ref string, inputs []IssueCriterionInput) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.ReplaceIssueCriteria(ctx, root, ref, inputs)
}

// ReplaceIssueCriteria replaces every criterion on an issue in one transaction.
func (s *Store) ReplaceIssueCriteria(ctx context.Context, root project.Root, ref string, inputs []IssueCriterionInput) (Issue, error) {
	criteria, err := normalizeIssueCriteria(inputs)
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

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Issue{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := replaceIssueCriteriaTx(ctx, tx, projectID, issueID, criteria, now); err != nil {
		return Issue{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, issueID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "touch issue", Err: err}
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

func normalizeIssueCriteria(inputs []IssueCriterionInput) ([]IssueCriterionInput, error) {
	out := make([]IssueCriterionInput, 0, len(inputs))
	for i, input := range inputs {
		text := strings.TrimSpace(input.Text)
		if text == "" {
			return nil, &IssueValidationError{Field: "criteria.text", Err: fmt.Errorf("item %d must be nonempty", i+1)}
		}
		tier := strings.TrimSpace(input.Tier)
		if tier == "" {
			if strings.TrimSpace(input.Command) != "" {
				tier = IssueCriterionTierV
			} else {
				tier = IssueCriterionTierH
			}
		}
		if tier != IssueCriterionTierV && tier != IssueCriterionTierH {
			return nil, &IssueValidationError{Field: "criteria.tier", Err: fmt.Errorf("item %d must be V or H", i+1)}
		}
		position := input.Position
		if position == 0 {
			position = i + 1
		}
		if position < 1 {
			return nil, &IssueValidationError{Field: "criteria.position", Err: fmt.Errorf("item %d must be >= 1", i+1)}
		}
		out = append(out, IssueCriterionInput{
			Text:     text,
			Command:  strings.TrimSpace(input.Command),
			Expect:   strings.TrimSpace(input.Expect),
			Tier:     tier,
			Position: position,
		})
	}
	return out, nil
}

func replaceIssueCriteriaTx(ctx context.Context, tx *sql.Tx, projectID, issueID string, criteria []IssueCriterionInput, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM issue_criteria WHERE project_id = ? AND issue_id = ?`, projectID, issueID); err != nil {
		return &IssueTransactionError{Stage: "clear criteria", Err: err}
	}
	for _, criterion := range criteria {
		id, err := newOpaqueStateID("icr")
		if err != nil {
			return &IssueTransactionError{Stage: "criterion id", Err: err}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO issue_criteria (id, project_id, issue_id, position, text, command, expect, tier, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, issueID, criterion.Position, criterion.Text, emptyToNil(criterion.Command), emptyToNil(criterion.Expect), criterion.Tier, now, now); err != nil {
			return &IssueTransactionError{Stage: "criterion", Err: err}
		}
	}
	return nil
}

// AddIssueCriterion appends one criterion to an issue.
func AddIssueCriterion(ctx context.Context, root project.Root, resolver PathResolver, ref string, input IssueCriterionInput) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.AddIssueCriterion(ctx, root, ref, input)
}

// AddIssueCriterion appends one criterion on an open store.
func (s *Store) AddIssueCriterion(ctx context.Context, root project.Root, ref string, input IssueCriterionInput) (Issue, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Issue{}, err
	}
	var max sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(position) FROM issue_criteria WHERE project_id = ? AND issue_id = ?`, projectID, issueID).Scan(&max); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "max position", Err: err}
	}
	input.Position = 1
	if max.Valid {
		input.Position = int(max.Int64) + 1
	}
	normalized, err := normalizeIssueCriteria([]IssueCriterionInput{input})
	if err != nil {
		return Issue{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertIssueCriterionTx(ctx, tx, projectID, issueID, normalized[0], now); err != nil {
		return Issue{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, issueID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "touch issue", Err: err}
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

// RemoveIssueCriterion deletes the criterion at position and compact-renumbers.
func RemoveIssueCriterion(ctx context.Context, root project.Root, resolver PathResolver, ref string, position int) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.RemoveIssueCriterion(ctx, root, ref, position)
}

// RemoveIssueCriterion deletes one criterion on an open store.
func (s *Store) RemoveIssueCriterion(ctx context.Context, root project.Root, ref string, position int) (Issue, error) {
	if position < 1 {
		return Issue{}, &IssueValidationError{Field: "position", Err: fmt.Errorf("must be >= 1")}
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

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Issue{}, err
	}
	current, err := loadIssueCriteriaTx(ctx, tx, projectID, issueID)
	if err != nil {
		return Issue{}, err
	}
	remaining := make([]IssueCriterionInput, 0, len(current))
	found := false
	for _, criterion := range current {
		if criterion.Position == position {
			found = true
			continue
		}
		remaining = append(remaining, IssueCriterionInput{
			Text:    criterion.Text,
			Command: criterion.Command,
			Expect:  criterion.Expect,
			Tier:    criterion.Tier,
		})
	}
	if !found {
		return Issue{}, &IssueValidationError{Field: "position", Err: fmt.Errorf("criterion position %d not found", position)}
	}
	normalized, err := normalizeIssueCriteria(remaining)
	if err != nil {
		return Issue{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := replaceIssueCriteriaTx(ctx, tx, projectID, issueID, normalized, now); err != nil {
		return Issue{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, issueID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "touch issue", Err: err}
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

// PromoteIssueCriterion creates a child delivery issue from the criterion at
// position. The parent criterion stays in place.
func PromoteIssueCriterion(ctx context.Context, root project.Root, resolver PathResolver, ref string, position int) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.PromoteIssueCriterion(ctx, root, ref, position)
}

// PromoteIssueCriterion creates a child issue on an open store.
func (s *Store) PromoteIssueCriterion(ctx context.Context, root project.Root, ref string, position int) (Issue, error) {
	if position < 1 {
		return Issue{}, &IssueValidationError{Field: "position", Err: fmt.Errorf("must be >= 1")}
	}
	parent, err := s.GetIssue(ctx, root, ref)
	if err != nil {
		return Issue{}, err
	}
	var criterion *IssueCriterion
	for i := range parent.Criteria {
		if parent.Criteria[i].Position == position {
			criterion = &parent.Criteria[i]
			break
		}
	}
	if criterion == nil {
		return Issue{}, &IssueValidationError{Field: "position", Err: fmt.Errorf("criterion position %d not found", position)}
	}
	return s.CreateIssue(ctx, root, IssueCreateOptions{
		Title:  criterion.Text,
		Kind:   IssueKindDelivery,
		Parent: parent.ID,
	})
}

func insertIssueCriterionTx(ctx context.Context, tx *sql.Tx, projectID, issueID string, criterion IssueCriterionInput, now string) error {
	id, err := newOpaqueStateID("icr")
	if err != nil {
		return &IssueTransactionError{Stage: "criterion id", Err: err}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO issue_criteria (id, project_id, issue_id, position, text, command, expect, tier, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, issueID, criterion.Position, criterion.Text, emptyToNil(criterion.Command), emptyToNil(criterion.Expect), criterion.Tier, now, now); err != nil {
		return &IssueTransactionError{Stage: "criterion", Err: err}
	}
	return nil
}

func loadIssueCriteriaTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) ([]IssueCriterion, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, position, text, COALESCE(command, ''), COALESCE(expect, ''), tier
FROM issue_criteria
WHERE project_id = ? AND issue_id = ?
ORDER BY position, id
`, projectID, issueID)
	if err != nil {
		return nil, fmt.Errorf("read issue criteria: %w", err)
	}
	defer rows.Close()
	criteria := []IssueCriterion{}
	for rows.Next() {
		var criterion IssueCriterion
		if err := rows.Scan(&criterion.ID, &criterion.Position, &criterion.Text, &criterion.Command, &criterion.Expect, &criterion.Tier); err != nil {
			return nil, fmt.Errorf("scan issue criterion: %w", err)
		}
		criteria = append(criteria, criterion)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue criteria: %w", err)
	}
	return criteria, nil
}
