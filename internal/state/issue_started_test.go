package state

import (
	"context"
	"strings"
	"testing"
)

func TestIssueStartedColumnsExistAfterMigration(t *testing.T) {
	_, store := issueTestFixture(t)
	rows, err := store.db.Query(`PRAGMA table_info(issues)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(issues) error = %v", err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info: %v", err)
	}
	if !found["started_branch"] || !found["started_worktree"] {
		t.Fatalf("issues columns = %v, want started_branch and started_worktree", found)
	}
}

func TestUpdateIssueRecordsStartedWorkspaceThroughEvents(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Start me"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.StartedBranch != "" || issue.StartedWorktree != "" {
		t.Fatalf("create started fields = %#v, want empty", issue)
	}

	updated, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             issue.ID,
		Status:          IssueStatusActive,
		SetStatus:       true,
		StartedBranch:   "issue/loaf-1",
		StartedWorktree: "/tmp/repo-wt/issue-loaf-1",
		SetStarted:      true,
	})
	if err != nil {
		t.Fatalf("UpdateIssue(start) error = %v", err)
	}
	if updated.Status != IssueStatusActive {
		t.Fatalf("status = %q, want active", updated.Status)
	}
	if updated.StartedBranch != "issue/loaf-1" || updated.StartedWorktree != "/tmp/repo-wt/issue-loaf-1" {
		t.Fatalf("started = %q / %q", updated.StartedBranch, updated.StartedWorktree)
	}

	parity, err := store.CheckIssueStatusParity(ctx, root)
	if err != nil {
		t.Fatalf("CheckIssueStatusParity() error = %v", err)
	}
	if !parity.Consistent {
		t.Fatalf("parity = %#v, want consistent after start", parity)
	}

	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             issue.ID,
		StartedBranch:   "issue/other",
		StartedWorktree: "/tmp/other",
		SetStarted:      true,
	}); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("second start error = %v, want already started", err)
	}

	cleared, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:        issue.ID,
		SetStarted: true,
	})
	if err != nil {
		t.Fatalf("UpdateIssue(clear started) error = %v", err)
	}
	if cleared.StartedBranch != "" || cleared.StartedWorktree != "" {
		t.Fatalf("cleared started = %#v, want empty", cleared)
	}
	if cleared.Status != IssueStatusActive {
		t.Fatalf("status after clear = %q, want active (stop does not change status)", cleared.Status)
	}
}

func TestUpdateIssueRefusesStartedOnTerminalAndArchived(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	done, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Done"})
	if err != nil {
		t.Fatalf("CreateIssue(done) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: done.ID, Status: IssueStatusDone, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(done) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             done.ID,
		StartedBranch:   "issue/done",
		StartedWorktree: "/tmp/done",
		SetStarted:      true,
	}); err == nil || !strings.Contains(err.Error(), "done") {
		t.Fatalf("start done error = %v, want refusal", err)
	}

	cancelled, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Cancelled"})
	if err != nil {
		t.Fatalf("CreateIssue(cancelled) error = %v", err)
	}
	if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: cancelled.ID, Status: IssueStatusCancelled}); err != nil {
		t.Fatalf("RemoveIssue() error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             cancelled.ID,
		StartedBranch:   "issue/cancelled",
		StartedWorktree: "/tmp/cancelled",
		SetStarted:      true,
	}); err == nil || !strings.Contains(err.Error(), "archived") && !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("start archived error = %v, want refusal", err)
	}
}

func TestUpdateIssueRejectsPartialStartedPair(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Partial"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:           issue.ID,
		StartedBranch: "issue/only-branch",
		SetStarted:    true,
	}); err == nil || !strings.Contains(err.Error(), "together") {
		t.Fatalf("partial started error = %v, want together", err)
	}
}

func TestListIssuesStartedFilterAndExportIncludesFields(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	started, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Live workspace"})
	if err != nil {
		t.Fatalf("CreateIssue(started) error = %v", err)
	}
	idle, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Idle"})
	if err != nil {
		t.Fatalf("CreateIssue(idle) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             started.ID,
		Status:          IssueStatusActive,
		SetStatus:       true,
		StartedBranch:   "issue/loaf-1",
		StartedWorktree: "/tmp/repo-wt/issue-loaf-1",
		SetStarted:      true,
	}); err != nil {
		t.Fatalf("UpdateIssue(start) error = %v", err)
	}

	listed, err := store.ListIssues(ctx, root, IssueListOptions{Started: true})
	if err != nil {
		t.Fatalf("ListIssues(started) error = %v", err)
	}
	if len(listed.Issues) != 1 || listed.Issues[0].ID != started.ID {
		t.Fatalf("started list = %#v, want only the started issue", listed.Issues)
	}
	if listed.Issues[0].StartedBranch != "issue/loaf-1" || listed.Issues[0].StartedWorktree != "/tmp/repo-wt/issue-loaf-1" {
		t.Fatalf("listed started fields = %#v", listed.Issues[0])
	}

	all, err := store.ListIssues(ctx, root, IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(all.Issues) != 2 {
		t.Fatalf("all issues = %#v, want both", all.Issues)
	}
	_ = idle

	snapshot, err := store.ExportIssues(ctx, root)
	if err != nil {
		t.Fatalf("ExportIssues() error = %v", err)
	}
	var exported *Issue
	for i := range snapshot.Issues {
		if snapshot.Issues[i].ID == started.ID {
			exported = &snapshot.Issues[i]
			break
		}
	}
	if exported == nil {
		t.Fatal("export missing started issue")
	}
	if exported.StartedBranch != "issue/loaf-1" || exported.StartedWorktree != "/tmp/repo-wt/issue-loaf-1" {
		t.Fatalf("exported started fields = %#v", exported)
	}
}

func TestNearestStartedAncestorWalksParentChain(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	grand, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Grandparent"})
	if err != nil {
		t.Fatalf("CreateIssue(grand) error = %v", err)
	}
	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Parent", Parent: grand.ID})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Child", Parent: parent.ID})
	if err != nil {
		t.Fatalf("CreateIssue(child) error = %v", err)
	}

	if _, found, err := store.NearestStartedAncestor(ctx, root, child.ID); err != nil || found {
		t.Fatalf("nearest before start = found %v err %v, want none", found, err)
	}

	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             grand.ID,
		Status:          IssueStatusActive,
		SetStatus:       true,
		StartedBranch:   "issue/grand",
		StartedWorktree: "/tmp/grand",
		SetStarted:      true,
	}); err != nil {
		t.Fatalf("UpdateIssue(grand start) error = %v", err)
	}

	got, found, err := store.NearestStartedAncestor(ctx, root, child.ID)
	if err != nil || !found {
		t.Fatalf("nearest after grand start = found %v err %v", found, err)
	}
	if got.ID != grand.ID || got.StartedBranch != "issue/grand" {
		t.Fatalf("nearest = %#v, want grandparent", got)
	}

	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
		Ref:             parent.ID,
		Status:          IssueStatusActive,
		SetStatus:       true,
		StartedBranch:   "issue/parent",
		StartedWorktree: "/tmp/parent",
		SetStarted:      true,
	}); err != nil {
		t.Fatalf("UpdateIssue(parent start) error = %v", err)
	}
	got, found, err = store.NearestStartedAncestor(ctx, root, child.ID)
	if err != nil || !found || got.ID != parent.ID || got.StartedBranch != "issue/parent" {
		t.Fatalf("nearest after parent start = %#v found %v err %v, want parent", got, found, err)
	}
}

func TestNearestStartedAncestorRejectsStoredParentCycle(t *testing.T) {
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

	_, _, err = store.NearestStartedAncestor(ctx, root, a.ID)
	if err == nil {
		t.Fatal("NearestStartedAncestor() error = nil, want stored parent cycle")
	}
	if !strings.Contains(err.Error(), "parent cycle detected in stored issue data at ") {
		t.Fatalf("NearestStartedAncestor() error = %v, want cycle message", err)
	}
	if !strings.Contains(err.Error(), a.ID) && !strings.Contains(err.Error(), b.ID) {
		t.Fatalf("NearestStartedAncestor() error = %v, want a stored issue id", err)
	}
}
