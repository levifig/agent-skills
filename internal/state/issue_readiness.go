package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

const (
	IssueReadinessMissingBody       = "missing_body"
	IssueReadinessMissingCriterion  = "missing_criterion"
	IssueReadinessMissingOutOfScope = "missing_out_of_scope"
	IssueReadinessNoQuestion        = "no_question"
	IssueReadinessUncovered         = "uncovered"
)

// IssueReadinessFailure is one shaping or coverage failure.
type IssueReadinessFailure struct {
	Code     string `json:"code"`
	Position int    `json:"position,omitempty"`
	Text     string `json:"text,omitempty"`
	Message  string `json:"message"`
}

// IssueReadinessOrphan is a child criterion that claims no parent criterion.
// It is reported, not a failure by itself.
type IssueReadinessOrphan struct {
	ChildRef string `json:"child_ref"`
	Position int    `json:"position"`
	Text     string `json:"text"`
	Remedy   string `json:"remedy"`
}

// IssueReadiness is derived readiness for one issue.
type IssueReadiness struct {
	Issue    Issue                   `json:"issue"`
	Kind     string                  `json:"kind"`
	Shaped   bool                    `json:"shaped"`
	Covered  bool                    `json:"covered"`
	Ready    bool                    `json:"ready"`
	Failures []IssueReadinessFailure `json:"failures"`
	Orphans  []IssueReadinessOrphan  `json:"orphans"`
	Children []IssueSummary          `json:"children,omitempty"`
}

// CheckIssueReadiness derives readiness from the issue row, not from markdown
// section presence.
func CheckIssueReadiness(ctx context.Context, root project.Root, resolver PathResolver, ref string) (IssueReadiness, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueReadiness{}, err
	}
	defer store.Close()
	return store.CheckIssueReadiness(ctx, root, ref)
}

// CheckIssueReadiness derives readiness on an open store.
func (s *Store) CheckIssueReadiness(ctx context.Context, root project.Root, ref string) (IssueReadiness, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueReadiness{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueReadiness{}, fmt.Errorf("begin issue check: %w", err)
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return IssueReadiness{}, err
	}
	issue, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return IssueReadiness{}, err
	}
	children, err := listIssueChildrenTx(ctx, tx, projectID, issueID)
	if err != nil {
		return IssueReadiness{}, err
	}
	return evaluateIssueReadiness(ctx, tx, projectID, issue, children)
}

func evaluateIssueReadiness(ctx context.Context, tx *sql.Tx, projectID string, issue Issue, children []IssueSummary) (IssueReadiness, error) {
	result := IssueReadiness{
		Issue:    issue,
		Kind:     issue.Kind,
		Covered:  true,
		Failures: []IssueReadinessFailure{},
		Orphans:  []IssueReadinessOrphan{},
		Children: children,
	}

	switch issue.Kind {
	case IssueKindDecision:
		if !hasSharpQuestion(issue.Title, issue.Body) {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessNoQuestion,
				Message: "decision issue needs a sharp question (a '?' in the title or body)",
			})
		}
	default:
		if strings.TrimSpace(issue.Body) == "" {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessMissingBody,
				Message: "delivery issue needs a nonempty body (the problem)",
			})
		}
		if len(issue.Criteria) == 0 {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessMissingCriterion,
				Message: "delivery issue needs at least one definition-of-done criterion",
			})
		}
		if !hasOutOfScopeStatement(issue.Body) {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessMissingOutOfScope,
				Message: "delivery issue body needs an explicit out-of-scope statement",
			})
		}
	}
	result.Shaped = len(result.Failures) == 0

	if len(children) > 0 {
		childIDs := make([]string, 0, len(children))
		for _, child := range children {
			childIDs = append(childIDs, child.ID)
		}
		claims, err := listIssueCriterionClaimsForChildrenTx(ctx, tx, projectID, childIDs)
		if err != nil {
			return IssueReadiness{}, err
		}
		claimedParent := map[string]bool{}
		claimedChild := map[string]bool{}
		for _, claim := range claims {
			claimedParent[claim.ParentCriterionID] = true
			claimedChild[claim.ChildCriterionID] = true
		}
		for _, criterion := range issue.Criteria {
			if claimedParent[criterion.ID] {
				continue
			}
			result.Covered = false
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:     IssueReadinessUncovered,
				Position: criterion.Position,
				Text:     criterion.Text,
				Message:  fmt.Sprintf("uncovered criterion %d: %s", criterion.Position, criterion.Text),
			})
		}
		parentRef := firstNonEmpty(issue.Alias, issue.ID)
		for _, child := range children {
			childIssue, err := loadIssueTx(ctx, tx, projectID, child.ID)
			if err != nil {
				return IssueReadiness{}, err
			}
			childRef := firstNonEmpty(childIssue.Alias, childIssue.ID)
			for _, criterion := range childIssue.Criteria {
				if claimedChild[criterion.ID] {
					continue
				}
				result.Orphans = append(result.Orphans, IssueReadinessOrphan{
					ChildRef: childRef,
					Position: criterion.Position,
					Text:     criterion.Text,
					Remedy:   orphanCriterionRemedy(criterion.Text, parentRef),
				})
			}
		}
	}

	result.Ready = result.Shaped && result.Covered
	return result, nil
}

func hasSharpQuestion(title, body string) bool {
	return strings.Contains(title, "?") || strings.Contains(body, "?")
}

func hasOutOfScopeStatement(body string) bool {
	return strings.Contains(strings.ToLower(body), "out of scope")
}

func orphanCriterionRemedy(text, parentRef string) string {
	// Options first, then `--`, then the title so a hyphen-leading criterion
	// (e.g. "--help") is positional, not an unknown flag.
	return "loaf issue new --parent " + posixSingleQuote(parentRef) + " --status backlog -- " + posixSingleQuote(text)
}

// PosixSingleQuote wraps s in POSIX single quotes. Embedded single quotes
// become '\” so the result is a single shell word with no expansion.
func PosixSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func posixSingleQuote(s string) string {
	return PosixSingleQuote(s)
}
