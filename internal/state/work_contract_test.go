package state

import (
	"context"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func workContractFixture(t *testing.T) (context.Context, project.Root, *Store, AuthorityRef) {
	t.Helper()
	root, store := issueTestFixture(t)
	ctx := context.Background()
	contract, err := store.CreateWorkContract(ctx, root, WorkContractCreateOptions{
		AuthorityRef: AuthorityRef{Provider: AuthorityProviderLinear, Key: "ENG-82"},
		Title:        "Contract machinery keys to refs",
		Body:         "Problem body.\n\nOut of scope: migration.",
		Criteria: []IssueCriterionInput{{
			Text: "DoD keys to provider-qualified authority refs",
			Tier: IssueCriterionTierH,
		}},
	})
	if err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
	}
	if contract.AuthorityRef.String() != "linear:ENG-82" {
		t.Fatalf("contract ref = %q, want linear:ENG-82", contract.AuthorityRef.String())
	}
	return ctx, root, store, contract.AuthorityRef
}

func TestWorkContractCreateShowVerifyReadiness(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)

	shown, err := store.ShowWorkContract(ctx, root, ref.String())
	if err != nil {
		t.Fatalf("ShowWorkContract() error = %v", err)
	}
	if len(shown.Contract.Criteria) != 1 {
		t.Fatalf("criteria len = %d, want 1", len(shown.Contract.Criteria))
	}

	readiness, err := store.CheckWorkContractReadiness(ctx, root, ref.String())
	if err != nil {
		t.Fatalf("CheckWorkContractReadiness() error = %v", err)
	}
	if !readiness.Shaped || !readiness.Ready {
		t.Fatalf("readiness = shaped:%v ready:%v failures:%#v", readiness.Shaped, readiness.Ready, readiness.Failures)
	}

	markdown := RenderWorkContractMarkdown(shown)
	if markdown == "" || !containsAll(markdown, "Contract machinery", "Definition of Done") {
		t.Fatalf("RenderWorkContractMarkdown() = %q, want title and DoD", markdown)
	}
}

func TestWorkContractWorkspaceBinding(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)

	updated, err := store.UpdateWorkContract(ctx, root, WorkContractUpdateOptions{
		AuthorityRef:    ref,
		StartedBranch:   "issue/loaf-68",
		StartedWorktree: "/tmp/issue-loaf-68",
		SetStarted:      true,
	})
	if err != nil {
		t.Fatalf("UpdateWorkContract() error = %v", err)
	}
	if updated.StartedBranch != "issue/loaf-68" || updated.StartedWorktree != "/tmp/issue-loaf-68" {
		t.Fatalf("workspace = %#v", updated)
	}
}

func TestWorkContractReceiptAndMapping(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)
	resolver := PathResolver{StateHome: t.TempDir()}

	if err := store.UpsertWorkContractMapping(ctx, root, ref.String(), "linear_url", "https://linear.app/issue/ENG-82"); err != nil {
		t.Fatalf("UpsertWorkContractMapping() error = %v", err)
	}
	if err := store.UpsertWorkContractReceipt(ctx, root, ref.String(), "export", "rendered-to-linear"); err != nil {
		t.Fatalf("UpsertWorkContractReceipt() error = %v", err)
	}

	shown, err := store.ShowWorkContract(ctx, root, ref.String())
	if err != nil {
		t.Fatalf("ShowWorkContract() error = %v", err)
	}
	if len(shown.Contract.Mappings) != 1 || len(shown.Contract.Receipts) != 1 {
		t.Fatalf("mappings=%#v receipts=%#v", shown.Contract.Mappings, shown.Contract.Receipts)
	}
	_ = resolver
}

func TestRepeatedWorkContractUpsertsRebuildSingleProjection(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}

	if err := store.UpsertWorkContractMapping(ctx, root, ref.String(), "linear_url", "https://linear.app/issue/ENG-82"); err != nil {
		t.Fatalf("UpsertWorkContractMapping(first) error = %v", err)
	}
	if err := store.UpsertWorkContractMapping(ctx, root, ref.String(), "linear_url", "https://linear.app/issue/ENG-82-updated"); err != nil {
		t.Fatalf("UpsertWorkContractMapping(second) error = %v", err)
	}
	if err := store.UpsertWorkContractReceipt(ctx, root, ref.String(), "export", "first"); err != nil {
		t.Fatalf("UpsertWorkContractReceipt(first) error = %v", err)
	}
	if err := store.UpsertWorkContractReceipt(ctx, root, ref.String(), "export", "second"); err != nil {
		t.Fatalf("UpsertWorkContractReceipt(second) error = %v", err)
	}

	if _, err := RebuildMutableCoreProjectionsForProject(ctx, store, projectID); err != nil {
		t.Fatalf("RebuildMutableCoreProjectionsForProject() error = %v", err)
	}

	var mappings, receipts int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM work_contract_mappings
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND mapping_kind = ?
`, projectID, ref.Provider, ref.Key, "linear_url").Scan(&mappings); err != nil {
		t.Fatalf("count mappings: %v", err)
	}
	if mappings != 1 {
		t.Fatalf("mapping rows = %d, want 1", mappings)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM work_contract_receipts
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND receipt_kind = ?
`, projectID, ref.Provider, ref.Key, "export").Scan(&receipts); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if receipts != 1 {
		t.Fatalf("receipt rows = %d, want 1", receipts)
	}

	var mappingValue, receiptValue string
	if err := store.db.QueryRowContext(ctx, `
SELECT mapping_value FROM work_contract_mappings
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND mapping_kind = ?
`, projectID, ref.Provider, ref.Key, "linear_url").Scan(&mappingValue); err != nil {
		t.Fatalf("read mapping: %v", err)
	}
	if mappingValue != "https://linear.app/issue/ENG-82-updated" {
		t.Fatalf("mapping_value = %q, want updated URL", mappingValue)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT receipt_value FROM work_contract_receipts
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND receipt_kind = ?
`, projectID, ref.Provider, ref.Key, "export").Scan(&receiptValue); err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	if receiptValue != "second" {
		t.Fatalf("receipt_value = %q, want second", receiptValue)
	}

	assertDistinctFactSubjects(t, store, projectID, FactKindRefRegistered, "mapping_kind", "linear_url", 2, 1)
	assertDistinctFactSubjects(t, store, projectID, FactKindVerificationRecorded, "receipt_kind", "export", 2, 1)
}

func TestRepeatedRenderOutRebuildsSingleProjection(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Render twice",
		Body:  "Problem body.\n\nOut of scope: none.",
		Kind:  IssueKindDelivery,
		Criteria: []IssueCriterionInput{{
			Text: "Holds",
			Tier: IssueCriterionTierH,
		}},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	first, err := store.RenderOutIssue(ctx, root, IssueRenderOutOptions{
		Ref:    issue.ID,
		Branch: "issue/loaf-62-rebuild",
	})
	if err != nil {
		t.Fatalf("RenderOutIssue(first) error = %v", err)
	}
	if _, err := store.RenderOutIssue(ctx, root, IssueRenderOutOptions{
		Ref:    issue.ID,
		Branch: "issue/loaf-62-rebuild",
	}); err != nil {
		t.Fatalf("RenderOutIssue(second) error = %v", err)
	}

	projectID := first.ProjectID
	if _, err := RebuildMutableCoreProjectionsForProject(ctx, store, projectID); err != nil {
		t.Fatalf("RebuildMutableCoreProjectionsForProject() error = %v", err)
	}

	var mappings, receipts int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM work_contract_mappings
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND mapping_kind = ?
`, projectID, first.AuthorityRef.Provider, first.AuthorityRef.Key, RenderOutMappingIssueID).Scan(&mappings); err != nil {
		t.Fatalf("count render-out mappings: %v", err)
	}
	if mappings != 1 {
		t.Fatalf("render-out mapping rows = %d, want 1", mappings)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM work_contract_receipts
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND receipt_kind = ?
`, projectID, first.AuthorityRef.Provider, first.AuthorityRef.Key, RenderOutReceiptKind).Scan(&receipts); err != nil {
		t.Fatalf("count render-out receipts: %v", err)
	}
	if receipts != 1 {
		t.Fatalf("render-out receipt rows = %d, want 1", receipts)
	}

	assertDistinctFactSubjects(t, store, projectID, FactKindRefRegistered, "mapping_kind", RenderOutMappingIssueID, 2, 1)
	assertDistinctFactSubjects(t, store, projectID, FactKindVerificationRecorded, "receipt_kind", RenderOutReceiptKind, 2, 1)
}

func assertDistinctFactSubjects(t *testing.T, store *Store, projectID, kind, payloadKey, payloadValue string, wantFacts, wantSubjects int) {
	t.Helper()
	var facts, subjects int
	query := `
SELECT COUNT(*), COUNT(DISTINCT json_extract(payload, '$.subject_id'))
FROM facts
WHERE project_id = ? AND kind = ? AND json_extract(payload, '$.` + payloadKey + `') = ?
`
	if err := store.db.QueryRowContext(context.Background(), query, projectID, kind, payloadValue).Scan(&facts, &subjects); err != nil {
		t.Fatalf("count %s facts: %v", kind, err)
	}
	if facts != wantFacts || subjects != wantSubjects {
		t.Fatalf("%s facts=%d subjects=%d, want facts=%d subjects=%d", kind, facts, subjects, wantFacts, wantSubjects)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

func TestWorkContractFrontierAndCriterionAppend(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)

	frontier, err := store.ListWorkContractFrontier(ctx, root)
	if err != nil {
		t.Fatalf("ListWorkContractFrontier() error = %v", err)
	}
	if len(frontier) != 1 || frontier[0].AuthorityRef.String() != ref.String() {
		t.Fatalf("frontier = %#v, want [%s]", frontier, ref)
	}

	updated, err := store.AddWorkContractCriterion(ctx, root, ref.String(), IssueCriterionInput{
		Text:    "Package tests pass",
		Command: "go test ./internal/state/ -run TestWorkContractFrontierAndCriterionAppend",
		Expect:  "exit 0",
	})
	if err != nil {
		t.Fatalf("AddWorkContractCriterion() error = %v", err)
	}
	if len(updated.Criteria) != 2 || updated.Criteria[1].Position != 2 {
		t.Fatalf("criteria after add = %#v", updated.Criteria)
	}

	removed, err := store.RemoveWorkContractCriterion(ctx, root, ref.String(), 1)
	if err != nil {
		t.Fatalf("RemoveWorkContractCriterion() error = %v", err)
	}
	if len(removed.Criteria) != 1 || removed.Criteria[0].Position != 1 || removed.Criteria[0].Text != "Package tests pass" {
		t.Fatalf("criteria after remove = %#v", removed.Criteria)
	}

	if _, err := store.UpdateWorkContract(ctx, root, WorkContractUpdateOptions{
		AuthorityRef: ref,
		Status:       IssueStatusDone,
		SetStatus:    true,
	}); err != nil {
		t.Fatalf("UpdateWorkContract() error = %v", err)
	}
	frontier, err = store.ListWorkContractFrontier(ctx, root)
	if err != nil {
		t.Fatalf("ListWorkContractFrontier() after done error = %v", err)
	}
	if len(frontier) != 0 {
		t.Fatalf("frontier after done = %#v, want empty", frontier)
	}
}
