package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// WorkContractReadiness is derived readiness for one ref-keyed contract.
type WorkContractReadiness struct {
	Contract WorkContract            `json:"contract"`
	Kind     string                  `json:"kind"`
	Shaped   bool                    `json:"shaped"`
	Covered  bool                    `json:"covered"`
	Ready    bool                    `json:"ready"`
	Failures []IssueReadinessFailure `json:"failures"`
	Orphans  []IssueReadinessOrphan  `json:"orphans"`
	Children []WorkContractSummary   `json:"children,omitempty"`
}

// CheckWorkContractReadiness derives readiness from the contract store.
func CheckWorkContractReadiness(ctx context.Context, root project.Root, resolver PathResolver, rawRef string) (WorkContractReadiness, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return WorkContractReadiness{}, err
	}
	defer store.Close()
	return store.CheckWorkContractReadiness(ctx, root, rawRef)
}

func (s *Store) CheckWorkContractReadiness(ctx context.Context, root project.Root, rawRef string) (WorkContractReadiness, error) {
	authorityRef, err := ParseAuthorityRef(rawRef)
	if err != nil {
		return WorkContractReadiness{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContractReadiness{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return WorkContractReadiness{}, err
	}
	defer tx.Rollback()

	contract, err := loadWorkContractByRefTx(ctx, tx, projectID, authorityRef)
	if err != nil {
		return WorkContractReadiness{}, err
	}
	children, err := listWorkContractChildrenTx(ctx, tx, projectID, contract.ID)
	if err != nil {
		return WorkContractReadiness{}, err
	}
	return evaluateWorkContractReadiness(ctx, tx, projectID, contract, children)
}

func evaluateWorkContractReadiness(ctx context.Context, tx *sql.Tx, projectID string, contract WorkContract, children []WorkContractSummary) (WorkContractReadiness, error) {
	result := WorkContractReadiness{
		Contract: contract,
		Kind:     contract.Kind,
		Covered:  true,
		Failures: []IssueReadinessFailure{},
		Orphans:  []IssueReadinessOrphan{},
		Children: children,
	}

	switch contract.Kind {
	case IssueKindDecision:
		if !hasSharpQuestion(contract.Title, contract.Body) {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessNoQuestion,
				Message: "decision contract needs a sharp question (a '?' in the title or body)",
			})
		}
	default:
		if strings.TrimSpace(contract.Body) == "" {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessMissingBody,
				Message: "delivery contract needs a nonempty body (the problem)",
			})
		}
		if len(contract.Criteria) == 0 {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessMissingCriterion,
				Message: "delivery contract needs at least one definition-of-done criterion",
			})
		}
		if !hasOutOfScopeStatement(contract.Body) {
			result.Failures = append(result.Failures, IssueReadinessFailure{
				Code:    IssueReadinessMissingOutOfScope,
				Message: "delivery contract body needs an explicit out-of-scope statement",
			})
		}
	}
	result.Shaped = len(result.Failures) == 0

	if len(children) > 0 {
		childIDs := make([]string, 0, len(children))
		childByID := map[string]WorkContractSummary{}
		for _, child := range children {
			childIDs = append(childIDs, child.ID)
			childByID[child.ID] = child
		}
		claims, err := listWorkContractCriterionClaimsForChildrenTx(ctx, tx, projectID, childIDs)
		if err != nil {
			return WorkContractReadiness{}, err
		}
		claimedParent := map[string]bool{}
		claimedChild := map[string]bool{}
		for _, claim := range claims {
			claimedParent[claim.ParentCriterionID] = true
			claimedChild[claim.ChildCriterionID] = true
		}
		for _, criterion := range contract.Criteria {
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
		parentRef := contract.AuthorityRef.String()
		for _, child := range children {
			childContract, err := loadWorkContractTx(ctx, tx, projectID, child.ID)
			if err != nil {
				return WorkContractReadiness{}, err
			}
			childRef := childContract.AuthorityRef.String()
			for _, criterion := range childContract.Criteria {
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
