package state

import (
	"context"
	"strings"
	"testing"
)

func TestListIssuesFiltersStatusKindAndArchived(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	delivery, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Delivery", Kind: IssueKindDelivery})
	if err != nil {
		t.Fatalf("CreateIssue(delivery) error = %v", err)
	}
	if _, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Decision", Kind: IssueKindDecision}); err != nil {
		t.Fatalf("CreateIssue(decision) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: delivery.ID, Status: IssueStatusTodo, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(todo) error = %v", err)
	}
	cancelled, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Cancelled"})
	if err != nil {
		t.Fatalf("CreateIssue(cancelled) error = %v", err)
	}
	if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: cancelled.ID, Status: IssueStatusCancelled}); err != nil {
		t.Fatalf("RemoveIssue() error = %v", err)
	}

	listed, err := store.ListIssues(ctx, root, IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 2 {
		t.Fatalf("default list = %#v, want 2 non-archived", listed.Issues)
	}

	todos, err := store.ListIssues(ctx, root, IssueListOptions{Status: IssueStatusTodo})
	if err != nil {
		t.Fatalf("ListIssues(todo) error = %v", err)
	}
	if len(todos.Issues) != 1 || todos.Issues[0].Title != "Delivery" {
		t.Fatalf("todo list = %#v, want Delivery", todos.Issues)
	}

	decisions, err := store.ListIssues(ctx, root, IssueListOptions{Kind: IssueKindDecision})
	if err != nil {
		t.Fatalf("ListIssues(decision) error = %v", err)
	}
	if len(decisions.Issues) != 1 || decisions.Issues[0].Title != "Decision" {
		t.Fatalf("decision list = %#v, want Decision", decisions.Issues)
	}

	archived, err := store.ListIssues(ctx, root, IssueListOptions{Archived: true, Status: IssueStatusCancelled})
	if err != nil {
		t.Fatalf("ListIssues(archived cancelled) error = %v", err)
	}
	if len(archived.Issues) != 1 || archived.Issues[0].Title != "Cancelled" {
		t.Fatalf("archived cancelled = %#v", archived.Issues)
	}
}

func TestIssueTreeIncludesGrandchildren(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Parent"})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Child", Parent: parent.Alias})
	if err != nil {
		t.Fatalf("CreateIssue(child) error = %v", err)
	}
	grand, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Grandchild", Parent: child.Alias})
	if err != nil {
		t.Fatalf("CreateIssue(grandchild) error = %v", err)
	}

	tree, err := store.IssueTree(ctx, root, parent.Alias, false)
	if err != nil {
		t.Fatalf("IssueTree() error = %v", err)
	}
	if len(tree.Roots) != 1 || tree.Roots[0].Title != "Parent" {
		t.Fatalf("roots = %#v, want Parent", tree.Roots)
	}
	if len(tree.Roots[0].Children) != 1 || tree.Roots[0].Children[0].Title != "Child" {
		t.Fatalf("children = %#v, want Child", tree.Roots[0].Children)
	}
	if len(tree.Roots[0].Children[0].Children) != 1 || tree.Roots[0].Children[0].Children[0].ID != grand.ID {
		t.Fatalf("grandchildren = %#v, want Grandchild", tree.Roots[0].Children[0].Children)
	}
}

func TestIssueTreeRejectsStoredParentCycle(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	a, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "A"})
	if err != nil {
		t.Fatalf("CreateIssue(A) error = %v", err)
	}
	b, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "B"})
	if err != nil {
		t.Fatalf("CreateIssue(B) error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE issues SET parent_id = ? WHERE id = ?`, b.ID, a.ID); err != nil {
		t.Fatalf("set A.parent=B: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE issues SET parent_id = ? WHERE id = ?`, a.ID, b.ID); err != nil {
		t.Fatalf("set B.parent=A: %v", err)
	}

	_, err = store.IssueTree(ctx, root, a.ID, false)
	if err == nil {
		t.Fatal("IssueTree() error = nil, want stored parent cycle")
	}
	if !strings.Contains(err.Error(), "parent cycle detected in stored issue data at ") {
		t.Fatalf("IssueTree() error = %v, want cycle message", err)
	}
	if !strings.Contains(err.Error(), a.ID) && !strings.Contains(err.Error(), b.ID) {
		t.Fatalf("IssueTree() error = %v, want a stored issue id", err)
	}
}

func TestIssueFrontierExcludesBlockedAndArchived(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	open, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Open"})
	if err != nil {
		t.Fatalf("CreateIssue(open) error = %v", err)
	}
	blocker, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Blocker"})
	if err != nil {
		t.Fatalf("CreateIssue(blocker) error = %v", err)
	}
	blocked, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Blocked"})
	if err != nil {
		t.Fatalf("CreateIssue(blocked) error = %v", err)
	}
	archived, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Archived"})
	if err != nil {
		t.Fatalf("CreateIssue(archived) error = %v", err)
	}
	if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: archived.ID, Status: IssueStatusCancelled}); err != nil {
		t.Fatalf("RemoveIssue(archived) error = %v", err)
	}
	if _, err := store.CreateLink(ctx, root, LinkMutationOptions{
		From: blocker.Alias,
		To:   blocked.Alias,
		Type: IssueRelationshipBlocks,
	}); err != nil {
		t.Fatalf("CreateLink(blocks) error = %v", err)
	}

	frontier, err := store.ListIssueFrontier(ctx, root)
	if err != nil {
		t.Fatalf("ListIssueFrontier() error = %v", err)
	}
	got := map[string]bool{}
	for _, issue := range frontier.Issues {
		got[issue.Title] = true
	}
	if !got["Open"] || !got["Blocker"] {
		t.Fatalf("frontier = %#v, want Open and Blocker", frontier.Issues)
	}
	if got["Blocked"] {
		t.Fatalf("frontier included blocked issue: %#v", frontier.Issues)
	}
	if got["Archived"] {
		t.Fatalf("frontier included archived issue: %#v", frontier.Issues)
	}

	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: blocker.ID, Status: IssueStatusDone, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(blocker done) error = %v", err)
	}
	unblocked, err := store.ListIssueFrontier(ctx, root)
	if err != nil {
		t.Fatalf("ListIssueFrontier(after done) error = %v", err)
	}
	foundBlocked := false
	for _, issue := range unblocked.Issues {
		if issue.ID == blocked.ID {
			foundBlocked = true
		}
		if issue.ID == open.ID && issue.Title != "Open" {
			t.Fatalf("open row = %#v", issue)
		}
	}
	if !foundBlocked {
		t.Fatalf("frontier after blocker done = %#v, want Blocked included", unblocked.Issues)
	}
}

func TestAddRemoveAndPromoteIssueCriterion(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Criteria"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	added, err := store.AddIssueCriterion(ctx, root, issue.Alias, IssueCriterionInput{Text: "Human check"})
	if err != nil {
		t.Fatalf("AddIssueCriterion() error = %v", err)
	}
	if len(added.Criteria) != 1 || added.Criteria[0].Tier != IssueCriterionTierH || added.Criteria[0].Position != 1 {
		t.Fatalf("added = %#v, want one H criterion", added.Criteria)
	}
	verified, err := store.AddIssueCriterion(ctx, root, issue.Alias, IssueCriterionInput{Text: "Smoke", Command: "true", Expect: "exit 0"})
	if err != nil {
		t.Fatalf("AddIssueCriterion(V) error = %v", err)
	}
	if len(verified.Criteria) != 2 || verified.Criteria[1].Tier != IssueCriterionTierV {
		t.Fatalf("verified = %#v, want V tier on second", verified.Criteria)
	}

	child, err := store.PromoteIssueCriterion(ctx, root, issue.Alias, 1, "")
	if err != nil {
		t.Fatalf("PromoteIssueCriterion() error = %v", err)
	}
	if child.Title != "Human check" || child.Kind != IssueKindDelivery || child.ParentID != issue.ID {
		t.Fatalf("child = %#v, want delivery child of parent", child)
	}
	if len(child.Criteria) != 1 || child.Criteria[0].Text != "Human check" || child.Criteria[0].Position != 1 {
		t.Fatalf("child criteria = %#v, want copied parent criterion as first criterion", child.Criteria)
	}
	still, err := store.GetIssue(ctx, root, issue.Alias)
	if err != nil {
		t.Fatalf("GetIssue(parent) error = %v", err)
	}
	if len(still.Criteria) != 2 {
		t.Fatalf("parent criteria after promote = %#v, want both retained", still.Criteria)
	}

	removed, err := store.RemoveIssueCriterion(ctx, root, issue.Alias, 1)
	if err != nil {
		t.Fatalf("RemoveIssueCriterion() error = %v", err)
	}
	if len(removed.Criteria) != 1 || removed.Criteria[0].Text != "Smoke" || removed.Criteria[0].Position != 1 {
		t.Fatalf("removed = %#v, want compacted Smoke at 1", removed.Criteria)
	}
}

func TestSetIssueBucketIsAdvisoryOnly(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Bucketed"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	set, err := store.SetIssueBucket(ctx, root, issue.Alias, IssueBucketNow)
	if err != nil {
		t.Fatalf("SetIssueBucket(now) error = %v", err)
	}
	if set.Bucket != IssueBucketNow {
		t.Fatalf("bucket = %q, want now", set.Bucket)
	}
	replaced, err := store.SetIssueBucket(ctx, root, issue.Alias, IssueBucketLater)
	if err != nil {
		t.Fatalf("SetIssueBucket(later) error = %v", err)
	}
	if replaced.Bucket != IssueBucketLater {
		t.Fatalf("replaced bucket = %q, want later", replaced.Bucket)
	}
	cleared, err := store.SetIssueBucket(ctx, root, issue.Alias, IssueBucketNone)
	if err != nil {
		t.Fatalf("SetIssueBucket(none) error = %v", err)
	}
	if cleared.Bucket != "" {
		t.Fatalf("cleared bucket = %q, want empty", cleared.Bucket)
	}

	frontier, err := store.ListIssueFrontier(ctx, root)
	if err != nil {
		t.Fatalf("ListIssueFrontier() error = %v", err)
	}
	if len(frontier.Issues) != 1 {
		t.Fatalf("frontier after bucket labels = %#v, want the issue still eligible", frontier.Issues)
	}
}

func TestExportIssuesIncludesRowsCriteriaAndRelationships(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	first, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Export me",
		Criteria: []IssueCriterionInput{{Text: "Done when exported"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	child, err := store.PromoteIssueCriterion(ctx, root, first.Alias, 1, "")
	if err != nil {
		t.Fatalf("PromoteIssueCriterion() error = %v", err)
	}
	second, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Related"})
	if err != nil {
		t.Fatalf("CreateIssue(second) error = %v", err)
	}
	if _, err := store.CreateLink(ctx, root, LinkMutationOptions{
		From: first.Alias,
		To:   second.Alias,
		Type: IssueRelationshipRelatesTo,
	}); err != nil {
		t.Fatalf("CreateLink(relates_to) error = %v", err)
	}

	snapshot, err := store.ExportIssues(ctx, root)
	if err != nil {
		t.Fatalf("ExportIssues() error = %v", err)
	}
	if snapshot.ExportKind != ExportKindIssue || snapshot.Format != ExportFormatJSON {
		t.Fatalf("snapshot header = %#v", snapshot)
	}
	if len(snapshot.Issues) != 3 {
		t.Fatalf("issues = %#v, want 3", snapshot.Issues)
	}
	if len(snapshot.Criteria) != 2 {
		t.Fatalf("criteria = %#v, want parent + promoted child", snapshot.Criteria)
	}
	if len(snapshot.Claims) != 1 || snapshot.Claims[0].ParentCriterionID != first.Criteria[0].ID || snapshot.Claims[0].ChildCriterionID != child.Criteria[0].ID {
		t.Fatalf("claims = %#v, want the promote claim", snapshot.Claims)
	}
	if len(snapshot.Relationships) != 1 || snapshot.Relationships[0].RelationshipType != IssueRelationshipRelatesTo {
		t.Fatalf("relationships = %#v, want relates_to", snapshot.Relationships)
	}
	if snapshot.Identity == nil {
		t.Fatal("identity = nil, want stored authority after minting local aliases")
	}
	if snapshot.Identity.Authority != IssueAuthorityLocal || snapshot.Identity.Prefix != DefaultIssuePrefix || snapshot.Identity.NextNumber != 4 {
		t.Fatalf("identity = %#v, want local %s next_number=4", snapshot.Identity, DefaultIssuePrefix)
	}
}

func TestExportIssuesOmitsIdentityWhenNoRow(t *testing.T) {
	root, store := issueTestFixture(t)

	snapshot, err := store.ExportIssues(context.Background(), root)
	if err != nil {
		t.Fatalf("ExportIssues() error = %v", err)
	}
	if snapshot.Identity != nil {
		t.Fatalf("identity = %#v, want omitted when no issue_identity row exists", snapshot.Identity)
	}
	if len(snapshot.Issues) != 0 || len(snapshot.Criteria) != 0 || len(snapshot.Claims) != 0 || len(snapshot.Relationships) != 0 {
		t.Fatalf("empty project export = %#v, want no rows", snapshot)
	}
}

func TestExportAllTablesIncludesIssueFoundation(t *testing.T) {
	found := map[string]bool{}
	for _, table := range exportAllTables {
		found[table.Name] = true
	}
	for _, name := range []string{"issues", "issue_criteria", "issue_criterion_claims", "issue_identity", "releases", "release_members"} {
		if !found[name] {
			t.Fatalf("exportAllTables missing %s", name)
		}
	}
}

func TestExportAllTablesIncludesWorkContracts(t *testing.T) {
	found := map[string]bool{}
	for _, table := range exportAllTables {
		found[table.Name] = true
	}
	for _, name := range []string{
		"work_contracts",
		"work_contract_criteria",
		"work_contract_criterion_claims",
		"work_contract_workspace",
		"work_contract_mappings",
		"work_contract_receipts",
	} {
		if !found[name] {
			t.Fatalf("exportAllTables missing %s", name)
		}
	}
}

func TestNormalizeIssueLinkType(t *testing.T) {
	got, err := NormalizeIssueLinkType("relates-to")
	if err != nil || got != IssueRelationshipRelatesTo {
		t.Fatalf("relates-to = %q %v, want relates_to", got, err)
	}
	got, err = NormalizeIssueLinkType("blocks")
	if err != nil || got != IssueRelationshipBlocks {
		t.Fatalf("blocks = %q %v", got, err)
	}
	if _, err := NormalizeIssueLinkType("implements"); err == nil {
		t.Fatal("implements must be rejected")
	}
}

func TestIssueListRejectsUnknownFilter(t *testing.T) {
	root, store := issueTestFixture(t)
	if _, err := store.ListIssues(context.Background(), root, IssueListOptions{Status: "blocked"}); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("ListIssues(blocked) error = %v, want status validation", err)
	}
}
