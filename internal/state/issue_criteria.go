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
	Text                 string
	Command              string
	Expect               string
	Tier                 string
	Position             int
	ServesParentPosition int
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
			Text:                 text,
			Command:              strings.TrimSpace(input.Command),
			Expect:               strings.TrimSpace(input.Expect),
			Tier:                 tier,
			Position:             position,
			ServesParentPosition: input.ServesParentPosition,
		})
	}
	return out, nil
}

func replaceIssueCriteriaTx(ctx context.Context, tx *sql.Tx, projectID, issueID string, criteria []IssueCriterionInput, now string) error {
	existing, err := loadIssueCriteriaTx(ctx, tx, projectID, issueID)
	if err != nil {
		return err
	}
	// Pair existing rows (already ascending by position) to incoming criteria
	// by slice order. Survivors and deletions are selected by row ID so a
	// legal gap (positions 1 and 3) cannot collide with UNIQUE(issue_id, position)
	// or delete a just-updated row. Written positions compact to 1..N.
	overlap := len(criteria)
	if overlap > len(existing) {
		overlap = len(existing)
	}
	for i := 0; i < overlap; i++ {
		criterion := criteria[i]
		if _, err := tx.ExecContext(ctx, `
UPDATE issue_criteria
SET text = ?, command = ?, expect = ?, tier = ?, position = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, criterion.Text, emptyToNil(criterion.Command), emptyToNil(criterion.Expect), criterion.Tier, i+1, now, projectID, existing[i].ID); err != nil {
			return &IssueTransactionError{Stage: "update criterion", Err: err}
		}
		if err := recordCriterionServesTx(ctx, tx, projectID, issueID, existing[i].ID, criterion.ServesParentPosition, now); err != nil {
			return err
		}
	}
	for i := overlap; i < len(existing); i++ {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM issue_criteria WHERE project_id = ? AND id = ?
`, projectID, existing[i].ID); err != nil {
			return &IssueTransactionError{Stage: "trim criteria", Err: err}
		}
	}
	for i := overlap; i < len(criteria); i++ {
		input := criteria[i]
		input.Position = i + 1
		criterionID, err := insertIssueCriterionTx(ctx, tx, projectID, issueID, input, now)
		if err != nil {
			return err
		}
		if err := recordCriterionServesTx(ctx, tx, projectID, issueID, criterionID, input.ServesParentPosition, now); err != nil {
			return err
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
	criterionID, err := insertIssueCriterionTx(ctx, tx, projectID, issueID, normalized[0], now)
	if err != nil {
		return Issue{}, err
	}
	if err := recordCriterionServesTx(ctx, tx, projectID, issueID, criterionID, normalized[0].ServesParentPosition, now); err != nil {
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
	remaining := make([]IssueCriterion, 0, len(current))
	removedID := ""
	for _, criterion := range current {
		if criterion.Position == position {
			removedID = criterion.ID
			continue
		}
		remaining = append(remaining, criterion)
	}
	if removedID == "" {
		return Issue{}, &IssueValidationError{Field: "position", Err: fmt.Errorf("criterion position %d not found", position)}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `DELETE FROM issue_criteria WHERE project_id = ? AND id = ?`, projectID, removedID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "delete criterion", Err: err}
	}
	// Compact in place so remaining criterion IDs — and their claims — survive.
	// Remaining rows are in ascending position; each move fills the hole just
	// freed, so UNIQUE (issue_id, position) never collides.
	for i, criterion := range remaining {
		want := i + 1
		if criterion.Position == want {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE issue_criteria SET position = ?, updated_at = ? WHERE project_id = ? AND id = ?
`, want, now, projectID, criterion.ID); err != nil {
			return Issue{}, &IssueTransactionError{Stage: "compact position", Err: err}
		}
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
func PromoteIssueCriterion(ctx context.Context, root project.Root, resolver PathResolver, ref string, position int, alias ...string) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	provided := ""
	if len(alias) > 0 {
		provided = alias[0]
	}
	return store.PromoteIssueCriterion(ctx, root, ref, position, provided)
}

// PromoteIssueCriterion creates a child issue on an open store. The promoted
// criterion is copied as the child's first criterion and a claim is recorded
// from that child criterion to the parent criterion, so coverage is satisfied
// by construction. A nonempty alias is stored as-is and does not advance the
// local identity counter.
func (s *Store) PromoteIssueCriterion(ctx context.Context, root project.Root, ref string, position int, providedAlias string) (Issue, error) {
	providedAlias = strings.TrimSpace(providedAlias)
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

	parentID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Issue{}, err
	}
	parent, err := loadIssueTx(ctx, tx, projectID, parentID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read parent", Err: err}
	}
	criterion, err := criterionAtPosition(parent.Criteria, position, "position")
	if err != nil {
		return Issue{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	issueID, err := newOpaqueStateID("issue")
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "id", Err: err}
	}
	if err := rejectIssueParentCycle(ctx, tx, projectID, issueID, parent.ID); err != nil {
		return Issue{}, err
	}
	alias := providedAlias
	if alias == "" {
		var mintErr error
		alias, mintErr = mintLocalIssueAliasTx(ctx, tx, projectID, now)
		if mintErr != nil {
			return Issue{}, mintErr
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO issues (id, project_id, parent_id, kind, title, body, fog, status, archived_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, NULL, ?, NULL, ?, ?)
`, issueID, projectID, parent.ID, IssueKindDelivery, criterion.Text, "", IssueStatusTriage, now, now); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "issue", Err: err}
	}
	if alias != "" {
		if err := insertAlias(ctx, tx, projectID, issueEntityKind, issueID, issueNamespace, alias, now); err != nil {
			return Issue{}, &IssueTransactionError{Stage: "alias", Err: err}
		}
	}
	copied := IssueCriterionInput{
		Text:     criterion.Text,
		Command:  criterion.Command,
		Expect:   criterion.Expect,
		Tier:     criterion.Tier,
		Position: 1,
	}
	normalized, err := normalizeIssueCriteria([]IssueCriterionInput{copied})
	if err != nil {
		return Issue{}, err
	}
	childCriterionID, err := insertIssueCriterionTx(ctx, tx, projectID, issueID, normalized[0], now)
	if err != nil {
		return Issue{}, err
	}
	if err := insertIssueCriterionClaimTx(ctx, tx, projectID, childCriterionID, criterion.ID, now); err != nil {
		return Issue{}, err
	}
	if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issueID, "", IssueStatusTriage, "recorded by issue promote", now); err != nil {
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

func recordCriterionServesTx(ctx context.Context, tx *sql.Tx, projectID, issueID, criterionID string, parentPosition int, now string) error {
	if parentPosition <= 0 {
		return nil
	}
	return claimNewCriterionAgainstParentTx(ctx, tx, projectID, issueID, criterionID, parentPosition, now)
}

func claimNewCriterionAgainstParentTx(ctx context.Context, tx *sql.Tx, projectID, childIssueID, childCriterionID string, parentPosition int, now string) error {
	child, err := loadIssueTx(ctx, tx, projectID, childIssueID)
	if err != nil {
		return &IssueTransactionError{Stage: "read child", Err: err}
	}
	if child.ParentID == "" {
		return &IssueValidationError{Field: "serves", Err: fmt.Errorf("issue %s has no parent", firstNonEmpty(child.Alias, child.ID))}
	}
	parent, err := loadIssueTx(ctx, tx, projectID, child.ParentID)
	if err != nil {
		return &IssueTransactionError{Stage: "read parent", Err: err}
	}
	parentCriterion, err := criterionAtPosition(parent.Criteria, parentPosition, "serves")
	if err != nil {
		return err
	}
	return insertIssueCriterionClaimTx(ctx, tx, projectID, childCriterionID, parentCriterion.ID, now)
}

func insertIssueCriterionTx(ctx context.Context, tx *sql.Tx, projectID, issueID string, criterion IssueCriterionInput, now string) (string, error) {
	id, err := newOpaqueStateID("icr")
	if err != nil {
		return "", &IssueTransactionError{Stage: "criterion id", Err: err}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO issue_criteria (id, project_id, issue_id, position, text, command, expect, tier, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, issueID, criterion.Position, criterion.Text, emptyToNil(criterion.Command), emptyToNil(criterion.Expect), criterion.Tier, now, now); err != nil {
		return "", &IssueTransactionError{Stage: "criterion", Err: err}
	}
	return id, nil
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
