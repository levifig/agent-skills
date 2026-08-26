package state

import (
	"context"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func issueTestFixture(t *testing.T) (project.Root, *Store) {
	t.Helper()
	root := projectRoot(t)
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return root, store
}

func TestLookupIssueIdentityDoesNotMaterializeDefault(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	identity, ok, err := store.LookupIssueIdentity(ctx, root)
	if err != nil {
		t.Fatalf("LookupIssueIdentity() error = %v", err)
	}
	if ok {
		t.Fatalf("LookupIssueIdentity() = %#v, want missing", identity)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM issue_identity`).Scan(&count); err != nil {
		t.Fatalf("count identity: %v", err)
	}
	if count != 0 {
		t.Fatalf("identity rows = %d, want 0", count)
	}
}

func TestIssueCreateMintsLocalAliasFromPrefix(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	identity, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLocal, Prefix: "LOAF"})
	if err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if identity.Authority != IssueAuthorityLocal || identity.Prefix != "LOAF" || identity.NextNumber != 1 {
		t.Fatalf("identity = %#v, want local LOAF next_number=1", identity)
	}

	first, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "First issue"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if first.Alias != "LOAF-1" || first.Status != IssueStatusTriage || first.Kind != IssueKindDelivery {
		t.Fatalf("first = %#v, want LOAF-1 triage delivery", first)
	}

	second, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Should this mint an alias?", Kind: IssueKindDecision})
	if err != nil {
		t.Fatalf("CreateIssue(second) error = %v", err)
	}
	if second.Alias != "" {
		t.Fatalf("second alias = %q, want empty ledger decision", second.Alias)
	}

	after, err := store.GetIssueIdentity(ctx, root)
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if after.NextNumber != 2 {
		t.Fatalf("next_number = %d, want 2 (decision questions skip alias mint)", after.NextNumber)
	}
}

func TestIssueCreateTrackerAuthorityMintsNoAlias(t *testing.T) {
	for _, authority := range []string{IssueAuthorityLinear, IssueAuthorityGitHub} {
		t.Run(authority, func(t *testing.T) {
			root, store := issueTestFixture(t)
			ctx := context.Background()
			if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: authority}); err != nil {
				t.Fatalf("SetIssueIdentity(%s) error = %v", authority, err)
			}
			issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Tracker-backed"})
			if err != nil {
				t.Fatalf("CreateIssue() error = %v", err)
			}
			if issue.Alias != "" {
				t.Fatalf("alias = %q, want empty for authority %s", issue.Alias, authority)
			}
			if issue.ID == "" {
				t.Fatal("tracker-backed issue must still receive an opaque id")
			}
			after, err := store.GetIssueIdentity(ctx, root)
			if err != nil {
				t.Fatalf("GetIssueIdentity() error = %v", err)
			}
			if after.NextNumber != 1 {
				t.Fatalf("next_number = %d, want 1 (tracker mints nothing)", after.NextNumber)
			}
			var aliasCount int
			if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM aliases WHERE entity_kind = 'issue' AND entity_id = ?`, issue.ID).Scan(&aliasCount); err != nil {
				t.Fatalf("count aliases: %v", err)
			}
			if aliasCount != 0 {
				t.Fatalf("issue aliases = %d, want 0", aliasCount)
			}
		})
	}
}

func TestIssueCreateProvidedAliasDoesNotAdvanceCounter(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLinear}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Delegated", Alias: "ENG-88"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Alias != "ENG-88" {
		t.Fatalf("alias = %q, want ENG-88", issue.Alias)
	}
	after, err := store.GetIssueIdentity(ctx, root)
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if after.NextNumber != 1 {
		t.Fatalf("next_number = %d, want 1", after.NextNumber)
	}
}

func TestIssueHardDeleteDoesNotReissueNumber(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}

	first, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Keep"})
	if err != nil {
		t.Fatalf("CreateIssue(first) error = %v", err)
	}
	highest, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Highest"})
	if err != nil {
		t.Fatalf("CreateIssue(highest) error = %v", err)
	}
	if highest.Alias != "LOAF-2" {
		t.Fatalf("highest alias = %q, want LOAF-2", highest.Alias)
	}

	if err := store.HardDeleteIssue(ctx, root, highest.Alias); err != nil {
		t.Fatalf("HardDeleteIssue() error = %v", err)
	}
	if _, err := store.GetIssue(ctx, root, highest.Alias); err == nil {
		t.Fatal("GetIssue(hard-deleted) error = nil, want not found")
	}
	if _, err := store.GetIssue(ctx, root, first.Alias); err != nil {
		t.Fatalf("GetIssue(survivor) error = %v", err)
	}

	next, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "After hard delete"})
	if err != nil {
		t.Fatalf("CreateIssue(after delete) error = %v", err)
	}
	if next.Alias != "LOAF-3" {
		t.Fatalf("alias after hard-delete = %q, want LOAF-3 (number must not be reused)", next.Alias)
	}
}

func TestIssueRemoveCancelledArchivesAndPreservesRecordAndEdges(t *testing.T) {
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
	if child.ParentID != parent.ID {
		t.Fatalf("child parent = %q, want %q", child.ParentID, parent.ID)
	}

	removed, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: child.Alias, Status: IssueStatusCancelled})
	if err != nil {
		t.Fatalf("RemoveIssue() error = %v", err)
	}
	if removed.Status != IssueStatusCancelled || removed.ArchivedAt == "" {
		t.Fatalf("removed = %#v, want cancelled and archived", removed)
	}

	still, err := store.GetIssue(ctx, root, child.Alias)
	if err != nil {
		t.Fatalf("GetIssue(cancelled) error = %v", err)
	}
	if still.ID != child.ID || still.ParentID != parent.ID || still.Status != IssueStatusCancelled {
		t.Fatalf("surviving record = %#v, want same id/parent cancelled", still)
	}

	var issueCount, eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM issues WHERE id = ?`, child.ID).Scan(&issueCount); err != nil {
		t.Fatalf("count issue: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE entity_kind = 'issue' AND entity_id = ?`, child.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if issueCount != 1 {
		t.Fatalf("issue rows = %d, want 1 (record survives)", issueCount)
	}
	if eventCount < 2 {
		t.Fatalf("events = %d, want create + cancel", eventCount)
	}
}

func TestIssueRemoveDuplicateRecordsRelatesTo(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	survivor, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Survivor"})
	if err != nil {
		t.Fatalf("CreateIssue(survivor) error = %v", err)
	}
	dup, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Duplicate"})
	if err != nil {
		t.Fatalf("CreateIssue(duplicate) error = %v", err)
	}

	removed, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{
		Ref:         dup.Alias,
		Status:      IssueStatusDuplicate,
		DuplicateOf: survivor.Alias,
	})
	if err != nil {
		t.Fatalf("RemoveIssue(duplicate) error = %v", err)
	}
	if removed.Status != IssueStatusDuplicate || removed.ArchivedAt == "" {
		t.Fatalf("removed = %#v, want duplicate and archived", removed)
	}

	var relType, toKind, toID string
	err = store.db.QueryRowContext(ctx, `
SELECT relationship_type, to_entity_kind, to_entity_id
FROM relationships
WHERE from_entity_kind = 'issue' AND from_entity_id = ?
`, dup.ID).Scan(&relType, &toKind, &toID)
	if err != nil {
		t.Fatalf("read relates_to: %v", err)
	}
	if relType != IssueRelationshipRelatesTo || toKind != "issue" || toID != survivor.ID {
		t.Fatalf("relationship = %s %s %s, want relates_to issue %s", relType, toKind, toID, survivor.ID)
	}

	if _, err := store.GetIssue(ctx, root, dup.Alias); err != nil {
		t.Fatalf("duplicate record must survive: %v", err)
	}

	if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: survivor.Alias, Status: IssueStatusDuplicate}); err == nil {
		t.Fatal("duplicate without survivor must fail")
	}
}

func TestIssueContentMutableAtEveryStatus(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	statuses := []string{
		IssueStatusTriage,
		IssueStatusBacklog,
		IssueStatusTodo,
		IssueStatusActive,
		IssueStatusDone,
		IssueStatusCancelled,
		IssueStatusDuplicate,
	}
	survivor, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Survivor for duplicate"})
	if err != nil {
		t.Fatalf("CreateIssue(survivor) error = %v", err)
	}

	for _, status := range statuses {
		issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{
			Title: "Original " + status,
			Body:  "original body",
			Fog:   "original fog",
		})
		if err != nil {
			t.Fatalf("CreateIssue(%s) error = %v", status, err)
		}
		switch status {
		case IssueStatusTriage:
			// default
		case IssueStatusCancelled:
			if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: issue.ID, Status: IssueStatusCancelled}); err != nil {
				t.Fatalf("RemoveIssue(%s) error = %v", status, err)
			}
		case IssueStatusDuplicate:
			if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: issue.ID, Status: IssueStatusDuplicate, DuplicateOf: survivor.ID}); err != nil {
				t.Fatalf("RemoveIssue(%s) error = %v", status, err)
			}
		default:
			if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: issue.ID, Status: status, SetStatus: true}); err != nil {
				t.Fatalf("UpdateIssue(status=%s) error = %v", status, err)
			}
		}

		updated, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{
			Ref:      issue.ID,
			Title:    "Retitled " + status,
			SetTitle: true,
			Body:     "rewritten body for " + status,
			SetBody:  true,
			Fog:      "rewritten fog for " + status,
			SetFog:   true,
		})
		if err != nil {
			t.Fatalf("UpdateIssue(content at %s) error = %v", status, err)
		}
		if updated.Title != "Retitled "+status || updated.Body != "rewritten body for "+status || updated.Fog != "rewritten fog for "+status {
			t.Fatalf("content at %s = %#v, want retitled fields", status, updated)
		}
		if updated.Status != status {
			t.Fatalf("status after content edit = %q, want %s", updated.Status, status)
		}
	}
}

func TestIssueStatusWritesThroughEvents(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Evented"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: issue.ID, Status: IssueStatusActive, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(active) error = %v", err)
	}

	var from, to string
	if err := store.db.QueryRowContext(ctx, `
SELECT COALESCE(from_status, ''), to_status FROM events
WHERE entity_kind = 'issue' AND entity_id = ?
ORDER BY created_at, id
`, issue.ID).Scan(&from, &to); err != nil {
		t.Fatalf("read first event: %v", err)
	}
	if from != "" || to != IssueStatusTriage {
		t.Fatalf("create event = %q -> %q, want empty -> triage", from, to)
	}

	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE entity_kind = 'issue' AND entity_id = ?`, issue.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("events = %d, want 2", eventCount)
	}

	parity, err := store.CheckIssueStatusParity(ctx, root)
	if err != nil {
		t.Fatalf("CheckIssueStatusParity() error = %v", err)
	}
	if !parity.Consistent {
		t.Fatalf("parity = %#v, want consistent", parity)
	}
}

func TestIssueStatusParityDetectsCorruptedColumn(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Parity"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE issues SET status = ? WHERE id = ?`, IssueStatusDone, issue.ID); err != nil {
		t.Fatalf("corrupt status column: %v", err)
	}

	parity, err := store.CheckIssueStatusParity(ctx, root)
	if err != nil {
		t.Fatalf("CheckIssueStatusParity() error = %v", err)
	}
	if parity.Consistent || len(parity.Mismatches) != 1 {
		t.Fatalf("parity = %#v, want one mismatch", parity)
	}
	mismatch := parity.Mismatches[0]
	if mismatch.IssueID != issue.ID || mismatch.ColumnStatus != IssueStatusDone || mismatch.EventStatus != IssueStatusTriage {
		t.Fatalf("mismatch = %#v, want column done vs event triage", mismatch)
	}
}

func TestIssueParentCycleRefusedAtWrite(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	a, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "A"})
	if err != nil {
		t.Fatalf("CreateIssue(A) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: a.ID, Parent: a.ID, SetParent: true}); err == nil {
		t.Fatal("self-parent must be refused")
	}

	b, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "B", Parent: a.ID})
	if err != nil {
		t.Fatalf("CreateIssue(B) error = %v", err)
	}
	c, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "C", Parent: b.ID})
	if err != nil {
		t.Fatalf("CreateIssue(C) error = %v", err)
	}

	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: a.ID, Parent: b.ID, SetParent: true}); err == nil {
		t.Fatal("parenting A to descendant B must be refused")
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: a.ID, Parent: c.ID, SetParent: true}); err == nil {
		t.Fatal("parenting A to descendant C must be refused")
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: a.ID, Parent: a.Alias, SetParent: true}); err == nil {
		t.Fatal("self-parent via alias must be refused")
	}

	// A valid move (C under A, skipping B) is allowed.
	moved, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: c.ID, Parent: a.ID, SetParent: true})
	if err != nil {
		t.Fatalf("valid parent move error = %v", err)
	}
	if moved.ParentID != a.ID {
		t.Fatalf("moved parent = %q, want %q", moved.ParentID, a.ID)
	}
}

func TestIssueIdentityCounterNotDerivedFromMax(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "One"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(ctx, `UPDATE issue_identity SET next_number = 10 WHERE project_id = ?`, projectID); err != nil {
		t.Fatalf("advance counter: %v", err)
	}
	next, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Ten"})
	if err != nil {
		t.Fatalf("CreateIssue(after bump) error = %v", err)
	}
	if next.Alias != "LOAF-10" {
		t.Fatalf("alias = %q, want LOAF-10 from stored counter, not MAX(existing)+1", next.Alias)
	}
}

func TestIssueCriteriaStoredWithVerifyGrammar(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "With criteria",
		Criteria: []IssueCriterionInput{
			{Text: "Smoke", Command: "true", Expect: "exit 0", Tier: IssueCriterionTierV},
			{Text: "A human check", Tier: IssueCriterionTierH},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if len(issue.Criteria) != 2 {
		t.Fatalf("criteria = %#v, want 2", issue.Criteria)
	}
	if issue.Criteria[0].Command != "true" || issue.Criteria[0].Expect != "exit 0" || issue.Criteria[0].Tier != "V" {
		t.Fatalf("V criterion = %#v", issue.Criteria[0])
	}
	if issue.Criteria[1].Tier != "H" || issue.Criteria[1].Command != "" {
		t.Fatalf("H criterion = %#v", issue.Criteria[1])
	}

	replaced, err := store.ReplaceIssueCriteria(ctx, root, issue.Alias, []IssueCriterionInput{
		{Text: "Output bound", Command: "echo ok", Expect: "exit 0 and contains `ok`", Tier: "V"},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria() error = %v", err)
	}
	if len(replaced.Criteria) != 1 || replaced.Criteria[0].Expect != "exit 0 and contains `ok`" {
		t.Fatalf("replaced = %#v", replaced.Criteria)
	}
}

func TestIssueSchemaConstraints(t *testing.T) {
	root, store := issueTestFixture(t)
	projectID := projectIDForTest(t, store, root)
	now := "2026-08-15T00:00:00Z"

	if err := execSchemaSQL(t, store, `
INSERT INTO issues (id, project_id, kind, title, body, status, created_at, updated_at)
VALUES ('issue-bad-status', ?, 'delivery', 'Bad', '', 'blocked', ?, ?)
`, projectID, now, now); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Fatalf("blocked status error = %v, want CHECK", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO issues (id, project_id, kind, title, body, status, created_at, updated_at)
VALUES ('issue-bad-kind', ?, 'epic', 'Bad', '', 'triage', ?, ?)
`, projectID, now, now); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Fatalf("epic kind error = %v, want CHECK", err)
	}

	mustExecSchemaSQL(t, store, `
INSERT INTO issues (id, project_id, kind, title, body, status, created_at, updated_at)
VALUES ('issue-ok', ?, 'delivery', 'Ok', '', 'triage', ?, ?)
`, projectID, now, now)
	if err := execSchemaSQL(t, store, `
INSERT INTO issue_criteria (id, project_id, issue_id, position, text, tier, created_at, updated_at)
VALUES ('crit-bad', ?, 'issue-ok', 1, 'text', 'X', ?, ?)
`, projectID, now, now); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Fatalf("tier X error = %v, want CHECK", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO issue_identity (id, project_id, authority, prefix, next_number, created_at, updated_at)
VALUES ('ident-bad', ?, 'jira', 'LOAF', 1, ?, ?)
`, projectID, now, now); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Fatalf("jira authority error = %v, want CHECK", err)
	}
}

func TestIssueCriterionClaimFKRejectsCrossProject(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	if _, err := Initialize(ctx, rootA, resolver); err != nil {
		t.Fatalf("Initialize(A) error = %v", err)
	}
	if _, err := Initialize(ctx, rootB, resolver); err != nil {
		t.Fatalf("Initialize(B) error = %v", err)
	}
	path, err := resolver.DatabasePath(rootA)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	parentA, err := store.CreateIssue(ctx, rootA, IssueCreateOptions{
		Title:    "A parent",
		Criteria: []IssueCriterionInput{{Text: "A criterion"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(A parent) error = %v", err)
	}
	childA, err := store.CreateIssue(ctx, rootA, IssueCreateOptions{
		Title:    "A child",
		Parent:   parentA.ID,
		Criteria: []IssueCriterionInput{{Text: "A child criterion"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(A child) error = %v", err)
	}
	parentB, err := store.CreateIssue(ctx, rootB, IssueCreateOptions{
		Title:    "B parent",
		Criteria: []IssueCriterionInput{{Text: "B criterion"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(B parent) error = %v", err)
	}

	projectA := projectIDForTest(t, store, rootA)
	now := "2026-08-15T00:00:00Z"
	err = execSchemaSQL(t, store, `
INSERT INTO issue_criterion_claims (id, project_id, child_criterion_id, parent_criterion_id, created_at, updated_at)
VALUES ('claim-xproj', ?, ?, ?, ?, ?)
`, projectA, childA.Criteria[0].ID, parentB.Criteria[0].ID, now, now)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("cross-project claim error = %v, want FOREIGN KEY", err)
	}
}

func TestIssueAliasResolutionIsNamespaceScoped(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	projectID := projectIDForTest(t, store, root)
	now := "2026-08-15T00:00:00Z"

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Namespaced"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Alias == "" {
		t.Fatal("CreateIssue() alias is empty")
	}

	mustExecSchemaSQL(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-shadow-same', ?, 'task', 'task-shadow', 'aaa', ?, ?, ?)
`, projectID, issue.Alias, now, now)

	resolved, err := store.GetIssue(ctx, root, issue.Alias)
	if err != nil {
		t.Fatalf("GetIssue(%q) error = %v, want the issue despite an earlier-namespace alias", issue.Alias, err)
	}
	if resolved.ID != issue.ID {
		t.Fatalf("GetIssue(%q) = %q, want %q", issue.Alias, resolved.ID, issue.ID)
	}

	mustExecSchemaSQL(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-shadow-only', ?, 'task', 'task-only', 'aaa', 'SHADOW-1', ?, ?)
`, projectID, now, now)
	if _, err := store.GetIssue(ctx, root, "SHADOW-1"); err == nil {
		t.Fatal("GetIssue(SHADOW-1) error = nil, want not found for a non-issue-namespace alias")
	}
}

func TestIssueStatusParityIgnoresLaterNonStatusEvent(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Parity later note"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: issue.ID, Status: IssueStatusActive, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(active) error = %v", err)
	}

	projectID := projectIDForTest(t, store, root)
	later := "2099-01-01T00:00:00Z"
	mustExecSchemaSQL(t, store, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, note, created_at, updated_at)
VALUES ('evt-noted-later', ?, 'issue', ?, 'noted', 'later note must not win latest-event', ?, ?)
`, projectID, issue.ID, later, later)

	parity, err := store.CheckIssueStatusParity(ctx, root)
	if err != nil {
		t.Fatalf("CheckIssueStatusParity() error = %v", err)
	}
	if !parity.Consistent {
		t.Fatalf("parity = %#v, want consistent after a later non-status event", parity)
	}
}

func TestIssuePrefixConstraintMatchesGoValidation(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	projectID := projectIDForTest(t, store, root)
	now := "2026-08-15T00:00:00Z"

	if err := execSchemaSQL(t, store, `
INSERT INTO issue_identity (id, project_id, authority, prefix, next_number, created_at, updated_at)
VALUES ('ident-bad-prefix', ?, 'local', 'LOAF-', 1, ?, ?)
`, projectID, now, now); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Fatalf("prefix LOAF- error = %v, want CHECK", err)
	}

	if err := execSchemaSQL(t, store, `
INSERT INTO issue_identity (id, project_id, authority, prefix, next_number, created_at, updated_at)
VALUES ('ident-nul-prefix', ?, 'local', ?, 1, ?, ?)
`, projectID, "A\x00B", now, now); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
		t.Fatalf("prefix A\\x00B error = %v, want CHECK", err)
	}

	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Prefix: "LOAF-"}); err == nil {
		t.Fatal("SetIssueIdentity(LOAF-) error = nil, want rejection")
	}
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Prefix: "LOAFÉ"}); err == nil {
		t.Fatal("SetIssueIdentity(LOAFÉ) error = nil, want rejection of non-ASCII")
	}
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Prefix: "ÅBO"}); err == nil {
		t.Fatal("SetIssueIdentity(ÅBO) error = nil, want rejection of non-ASCII letter-first")
	}
}

func TestIssueDefaultIdentityIsLocalLoaf(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Default identity"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Alias != "LOAF-1" {
		t.Fatalf("alias = %q, want LOAF-1 from default local/LOAF identity", issue.Alias)
	}
}

func TestIssueRemoveHighestDoesNotReissueNumber(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	if _, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "One"}); err != nil {
		t.Fatalf("CreateIssue(1) error = %v", err)
	}
	highest, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Two"})
	if err != nil {
		t.Fatalf("CreateIssue(2) error = %v", err)
	}
	if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: highest.Alias, Status: IssueStatusCancelled}); err != nil {
		t.Fatalf("RemoveIssue() error = %v", err)
	}
	next, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Three"})
	if err != nil {
		t.Fatalf("CreateIssue(3) error = %v", err)
	}
	if next.Alias != "LOAF-3" {
		t.Fatalf("alias after cancel = %q, want LOAF-3", next.Alias)
	}
}
