package state

import (
	"context"
	"strings"
	"testing"
)

const shapedDeliveryBody = "The problem is that readiness is derived from nine required headings.\n\nOut of scope: grading criterion quality.\n"

func TestPromoteRecordsClaimAndCoveragePasses(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Parent work",
		Body:  shapedDeliveryBody,
		Criteria: []IssueCriterionInput{
			{Text: "First slice"},
			{Text: "Second slice"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, 1); err != nil {
		t.Fatalf("Promote(1) error = %v", err)
	}
	if _, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, 2); err != nil {
		t.Fatalf("Promote(2) error = %v", err)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Shaped || !readiness.Covered || !readiness.Ready {
		t.Fatalf("readiness = %#v, want shaped+covered+ready after full promote", readiness)
	}
	if len(readiness.Failures) != 0 {
		t.Fatalf("failures = %#v, want none", readiness.Failures)
	}
	if len(readiness.Orphans) != 0 {
		t.Fatalf("orphans = %#v, want none", readiness.Orphans)
	}

	var claimCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM issue_criterion_claims`).Scan(&claimCount); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claimCount != 2 {
		t.Fatalf("claims = %d, want 2", claimCount)
	}
}

func TestUncoveredParentCriterionFailsReadiness(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Parent work",
		Body:  shapedDeliveryBody,
		Criteria: []IssueCriterionInput{
			{Text: "Covered slice"},
			{Text: "Left behind"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, 1); err != nil {
		t.Fatalf("Promote(1) error = %v", err)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if readiness.Ready || readiness.Covered {
		t.Fatalf("readiness = %#v, want uncovered failure", readiness)
	}
	found := false
	for _, failure := range readiness.Failures {
		if failure.Code == IssueReadinessUncovered && failure.Position == 2 && strings.Contains(failure.Message, "Left behind") {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %#v, want uncovered criterion 2 named", readiness.Failures)
	}
}

func TestOrphanChildCriterionIsReportedWithRemedy(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Parent work",
		Body:  shapedDeliveryBody,
		Criteria: []IssueCriterionInput{
			{Text: "Promoted slice"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	child, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, 1)
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if _, err := store.AddIssueCriterion(ctx, root, child.Alias, IssueCriterionInput{Text: "Stray extra work"}); err != nil {
		t.Fatalf("AddIssueCriterion(orphan) error = %v", err)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Ready {
		t.Fatalf("readiness = %#v, orphan must not fail the check", readiness)
	}
	if len(readiness.Orphans) != 1 {
		t.Fatalf("orphans = %#v, want one", readiness.Orphans)
	}
	orphan := readiness.Orphans[0]
	if orphan.ChildRef != child.Alias || orphan.Position != 2 || orphan.Text != "Stray extra work" {
		t.Fatalf("orphan = %#v", orphan)
	}
	wantRemedy := "loaf issue new --parent " + posixSingleQuote(parent.Alias) + " --status backlog -- " + posixSingleQuote("Stray extra work")
	if orphan.Remedy != wantRemedy {
		t.Fatalf("remedy = %q, want %q", orphan.Remedy, wantRemedy)
	}
}

func TestOrphanRemedyIsPOSIXSingleQuoted(t *testing.T) {
	text := "don't $(touch /tmp/pwned) `reboot`"
	parentRef := "LOAF-1"
	got := orphanCriterionRemedy(text, parentRef)
	want := "loaf issue new --parent 'LOAF-1' --status backlog -- 'don'\\''t $(touch /tmp/pwned) `reboot`'"
	if got != want {
		t.Fatalf("remedy = %q, want %q", got, want)
	}
	if strings.Contains(got, `"`) || strings.Contains(got, "&&") || strings.Contains(got, "<") {
		t.Fatalf("remedy still uses unsafe quoting or chaining: %q", got)
	}
	titleStart := strings.LastIndex(got, " -- ")
	if titleStart < 0 {
		t.Fatalf("remedy missing end-of-options terminator: %q", got)
	}
	if !posixSingleQuotedWord(got[titleStart+len(" -- "):]) {
		t.Fatalf("title is not a POSIX single-quoted word: %q", got)
	}
}

func TestOrphanRemedyPlacesHyphenLeadingTitleAfterEndOfOptions(t *testing.T) {
	got := orphanCriterionRemedy("--help", "LOAF-1")
	want := "loaf issue new --parent 'LOAF-1' --status backlog -- '--help'"
	if got != want {
		t.Fatalf("remedy = %q, want %q", got, want)
	}
}

func TestDecisionIssueReadyOnSharpQuestion(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	blank, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Pick a store",
		Kind:  IssueKindDecision,
		Body:  "Need a direction.",
	})
	if err != nil {
		t.Fatalf("CreateIssue(blank) error = %v", err)
	}
	notReady, err := store.CheckIssueReadiness(ctx, root, blank.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(blank) error = %v", err)
	}
	if notReady.Ready || notReady.Shaped {
		t.Fatalf("blank decision ready = %#v, want not ready", notReady)
	}
	if len(notReady.Failures) != 1 || notReady.Failures[0].Code != IssueReadinessNoQuestion {
		t.Fatalf("blank failures = %#v, want no_question only", notReady.Failures)
	}

	question, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Should we keep the local store?",
		Kind:  IssueKindDecision,
	})
	if err != nil {
		t.Fatalf("CreateIssue(question) error = %v", err)
	}
	ready, err := store.CheckIssueReadiness(ctx, root, question.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(question) error = %v", err)
	}
	if !ready.Ready || !ready.Shaped {
		t.Fatalf("question decision = %#v, want ready without criteria or a plan", ready)
	}
}

func TestDodAddServesRecordsClaim(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Parent",
		Body:     shapedDeliveryBody,
		Criteria: []IssueCriterionInput{{Text: "Parent criterion"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:  "Child",
		Parent: parent.Alias,
	})
	if err != nil {
		t.Fatalf("CreateIssue(child) error = %v", err)
	}
	added, err := store.AddIssueCriterion(ctx, root, child.Alias, IssueCriterionInput{
		Text:                 "Serves parent",
		ServesParentPosition: 1,
	})
	if err != nil {
		t.Fatalf("AddIssueCriterion(--serves) error = %v", err)
	}
	if len(added.Criteria) != 1 {
		t.Fatalf("added = %#v", added.Criteria)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Covered || !readiness.Ready {
		t.Fatalf("readiness = %#v, want covered after --serves", readiness)
	}
}

func TestClaimAndUnclaimToggleCoverage(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Parent",
		Body:     shapedDeliveryBody,
		Criteria: []IssueCriterionInput{{Text: "Parent criterion"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Child",
		Parent:   parent.Alias,
		Criteria: []IssueCriterionInput{{Text: "Already written"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(child) error = %v", err)
	}

	before, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(before) error = %v", err)
	}
	if before.Covered {
		t.Fatal("coverage passed before claim, want uncovered")
	}

	if _, err := store.ClaimIssueCriterion(ctx, root, child.Alias, 1, 1); err != nil {
		t.Fatalf("ClaimIssueCriterion() error = %v", err)
	}
	claimed, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(claimed) error = %v", err)
	}
	if !claimed.Covered {
		t.Fatalf("after claim = %#v, want covered", claimed)
	}

	if _, err := store.UnclaimIssueCriterion(ctx, root, child.Alias, 1, 1); err != nil {
		t.Fatalf("UnclaimIssueCriterion() error = %v", err)
	}
	after, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(after) error = %v", err)
	}
	if after.Covered {
		t.Fatal("coverage still passed after unclaim")
	}
}

func posixSingleQuotedWord(word string) bool {
	if len(word) < 2 || word[0] != '\'' || word[len(word)-1] != '\'' {
		return false
	}
	inner := word[1 : len(word)-1]
	inner = strings.ReplaceAll(inner, `'\''`, "")
	return !strings.Contains(inner, "'")
}

func TestReplaceIssueCriteriaPreservesClaimIDsAndDropsTail(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Parent",
		Body:  shapedDeliveryBody,
		Criteria: []IssueCriterionInput{
			{Text: "First slice"},
			{Text: "Second slice"},
			{Text: "Third slice"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	ids := []string{parent.Criteria[0].ID, parent.Criteria[1].ID, parent.Criteria[2].ID}
	for i := 1; i <= 3; i++ {
		if _, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, i); err != nil {
			t.Fatalf("Promote(%d) error = %v", i, err)
		}
	}

	replaced, err := store.ReplaceIssueCriteria(ctx, root, parent.Alias, []IssueCriterionInput{
		{Text: "First edited"},
		{Text: "Second edited"},
		{Text: "Third edited"},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria(edit) error = %v", err)
	}
	if len(replaced.Criteria) != 3 {
		t.Fatalf("replaced = %#v, want 3", replaced.Criteria)
	}
	for i, wantID := range ids {
		if replaced.Criteria[i].ID != wantID || replaced.Criteria[i].Text != []string{"First edited", "Second edited", "Third edited"}[i] {
			t.Fatalf("replaced[%d] = %#v, want id %s with edited text", i, replaced.Criteria[i], wantID)
		}
	}
	afterEdit, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(edit) error = %v", err)
	}
	if !afterEdit.Covered {
		t.Fatalf("coverage lost after in-place edit: %#v", afterEdit)
	}

	shrunk, err := store.ReplaceIssueCriteria(ctx, root, parent.Alias, []IssueCriterionInput{
		{Text: "First kept"},
		{Text: "Second kept"},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria(shrink) error = %v", err)
	}
	if len(shrunk.Criteria) != 2 || shrunk.Criteria[0].ID != ids[0] || shrunk.Criteria[1].ID != ids[1] {
		t.Fatalf("shrunk = %#v, want first two ids retained", shrunk.Criteria)
	}

	var remainingClaims int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM issue_criterion_claims WHERE parent_criterion_id IN (?, ?, ?)
`, ids[0], ids[1], ids[2]).Scan(&remainingClaims); err != nil {
		t.Fatalf("count remaining claims: %v", err)
	}
	if remainingClaims != 2 {
		t.Fatalf("claims after shrink = %d, want 2 (tail cascaded)", remainingClaims)
	}
	var tailRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM issue_criteria WHERE id = ?`, ids[2]).Scan(&tailRows); err != nil {
		t.Fatalf("count tail criterion: %v", err)
	}
	if tailRows != 0 {
		t.Fatalf("tail criterion %s still present", ids[2])
	}

	afterShrink, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness(shrink) error = %v", err)
	}
	if !afterShrink.Covered {
		t.Fatalf("coverage of remaining criteria lost after shrink: %#v", afterShrink)
	}
}

func TestReplaceIssueCriteriaOnChildPreservesClaims(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Parent",
		Body:     shapedDeliveryBody,
		Criteria: []IssueCriterionInput{{Text: "Parent slice"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, 1)
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	childID := child.Criteria[0].ID

	replaced, err := store.ReplaceIssueCriteria(ctx, root, child.Alias, []IssueCriterionInput{
		{Text: "Child text edited"},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria(child) error = %v", err)
	}
	if len(replaced.Criteria) != 1 || replaced.Criteria[0].ID != childID {
		t.Fatalf("child after replace = %#v, want id %s", replaced.Criteria, childID)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Covered || len(readiness.Orphans) != 0 {
		t.Fatalf("child replace invented an orphan or uncovered parent: %#v", readiness)
	}
}

func TestReplaceIssueCriteriaCompactsGappedPositionsOnExpand(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Gapped expand",
		Criteria: []IssueCriterionInput{
			{Text: "Keep first"},
			{Text: "Keep third-as-second"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	ids := []string{issue.Criteria[0].ID, issue.Criteria[1].ID}
	setIssueCriterionPositionsByOrder(t, store, issue.ID, []int{1, 3})

	replaced, err := store.ReplaceIssueCriteria(ctx, root, issue.Alias, []IssueCriterionInput{
		{Text: "One"},
		{Text: "Two"},
		{Text: "Three"},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria({1,3} -> 3) error = %v", err)
	}
	if len(replaced.Criteria) != 3 {
		t.Fatalf("replaced = %#v, want 3 compact rows", replaced.Criteria)
	}
	for i, wantText := range []string{"One", "Two", "Three"} {
		if replaced.Criteria[i].Position != i+1 || replaced.Criteria[i].Text != wantText {
			t.Fatalf("replaced[%d] = %#v, want position %d text %q", i, replaced.Criteria[i], i+1, wantText)
		}
	}
	if replaced.Criteria[0].ID != ids[0] || replaced.Criteria[1].ID != ids[1] {
		t.Fatalf("replaced ids = %s,%s, want %s,%s retained", replaced.Criteria[0].ID, replaced.Criteria[1].ID, ids[0], ids[1])
	}
	if replaced.Criteria[2].ID == ids[0] || replaced.Criteria[2].ID == ids[1] {
		t.Fatalf("inserted tail reused a survivor id: %#v", replaced.Criteria)
	}
}

func TestReplaceIssueCriteriaCompactsGappedPositionsOnShrink(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	issue, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Gapped shrink",
		Criteria: []IssueCriterionInput{
			{Text: "Survivor"},
			{Text: "Drop me"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	ids := []string{issue.Criteria[0].ID, issue.Criteria[1].ID}
	setIssueCriterionPositionsByOrder(t, store, issue.ID, []int{2, 3})

	replaced, err := store.ReplaceIssueCriteria(ctx, root, issue.Alias, []IssueCriterionInput{
		{Text: "Only"},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria({2,3} -> 1) error = %v", err)
	}
	if len(replaced.Criteria) != 1 || replaced.Criteria[0].ID != ids[0] || replaced.Criteria[0].Position != 1 || replaced.Criteria[0].Text != "Only" {
		t.Fatalf("replaced = %#v, want survivor %s at position 1", replaced.Criteria, ids[0])
	}
	var leftover int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM issue_criteria WHERE id = ?`, ids[1]).Scan(&leftover); err != nil {
		t.Fatalf("count leftover: %v", err)
	}
	if leftover != 0 {
		t.Fatalf("leftover criterion %s still present", ids[1])
	}
}

func TestCreateIssueServesParentPositionRecordsClaim(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Parent",
		Body:     shapedDeliveryBody,
		Criteria: []IssueCriterionInput{{Text: "Parent criterion"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:  "Child",
		Parent: parent.Alias,
		Criteria: []IssueCriterionInput{{
			Text:                 "Serves parent on create",
			ServesParentPosition: 1,
		}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(child --serves) error = %v", err)
	}
	if len(child.Criteria) != 1 {
		t.Fatalf("child criteria = %#v", child.Criteria)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Covered || !readiness.Ready || len(readiness.Orphans) != 0 {
		t.Fatalf("readiness = %#v, want covered after CreateIssue ServesParentPosition", readiness)
	}

	_, err = store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Orphan root",
		Criteria: []IssueCriterionInput{{
			Text:                 "No parent to serve",
			ServesParentPosition: 1,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "has no parent") {
		t.Fatalf("CreateIssue(no parent --serves) error = %v, want no parent", err)
	}

	_, err = store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:  "Missing parent position",
		Parent: parent.Alias,
		Criteria: []IssueCriterionInput{{
			Text:                 "Serves missing",
			ServesParentPosition: 2,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "criterion position 2 not found") {
		t.Fatalf("CreateIssue(missing parent pos) error = %v, want position 2 missing", err)
	}
}

func TestReplaceIssueCriteriaServesParentPositionRecordsClaim(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Parent",
		Body:     shapedDeliveryBody,
		Criteria: []IssueCriterionInput{{Text: "Parent criterion"}, {Text: "Second parent"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:    "Child",
		Parent:   parent.Alias,
		Criteria: []IssueCriterionInput{{Text: "Already written"}},
	})
	if err != nil {
		t.Fatalf("CreateIssue(child) error = %v", err)
	}
	keptID := child.Criteria[0].ID

	replaced, err := store.ReplaceIssueCriteria(ctx, root, child.Alias, []IssueCriterionInput{
		{Text: "Updated and serves", ServesParentPosition: 1},
		{Text: "Inserted and serves", ServesParentPosition: 2},
	})
	if err != nil {
		t.Fatalf("ReplaceIssueCriteria(--serves) error = %v", err)
	}
	if len(replaced.Criteria) != 2 || replaced.Criteria[0].ID != keptID {
		t.Fatalf("replaced = %#v, want updated first id %s", replaced.Criteria, keptID)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Covered || !readiness.Ready || len(readiness.Orphans) != 0 {
		t.Fatalf("readiness = %#v, want covered after ReplaceIssueCriteria ServesParentPosition", readiness)
	}

	_, err = store.ReplaceIssueCriteria(ctx, root, parent.Alias, []IssueCriterionInput{
		{Text: "Root cannot serve", ServesParentPosition: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "has no parent") {
		t.Fatalf("ReplaceIssueCriteria(no parent --serves) error = %v, want no parent", err)
	}
}

func setIssueCriterionPositionsByOrder(t *testing.T, store *Store, issueID string, positions []int) {
	t.Helper()
	ctx := context.Background()
	rows, err := store.db.QueryContext(ctx, `
SELECT id FROM issue_criteria WHERE issue_id = ? ORDER BY position, id
`, issueID)
	if err != nil {
		t.Fatalf("list criteria: %v", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan criterion id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate criteria: %v", err)
	}
	rows.Close()
	if len(ids) != len(positions) {
		t.Fatalf("criteria = %d, want %d positions", len(ids), len(positions))
	}
	// Park at high positions so UNIQUE(issue_id, position) cannot collide
	// while writing a legal gapped layout.
	for i, id := range ids {
		if _, err := store.db.ExecContext(ctx, `UPDATE issue_criteria SET position = ? WHERE id = ?`, 1000+i, id); err != nil {
			t.Fatalf("park position: %v", err)
		}
	}
	for i, id := range ids {
		if _, err := store.db.ExecContext(ctx, `UPDATE issue_criteria SET position = ? WHERE id = ?`, positions[i], id); err != nil {
			t.Fatalf("set position %d: %v", positions[i], err)
		}
	}
}

func TestRemoveCriterionPreservesRemainingClaimIDs(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Parent",
		Body:  shapedDeliveryBody,
		Criteria: []IssueCriterionInput{
			{Text: "Drop me"},
			{Text: "Keep me"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := store.PromoteIssueCriterion(ctx, root, parent.Alias, 2); err != nil {
		t.Fatalf("Promote(2) error = %v", err)
	}
	keptID := ""
	for _, criterion := range parent.Criteria {
		if criterion.Position == 2 {
			keptID = criterion.ID
		}
	}
	if keptID == "" {
		t.Fatal("missing parent criterion 2 id")
	}

	removed, err := store.RemoveIssueCriterion(ctx, root, parent.Alias, 1)
	if err != nil {
		t.Fatalf("RemoveIssueCriterion() error = %v", err)
	}
	if len(removed.Criteria) != 1 || removed.Criteria[0].ID != keptID || removed.Criteria[0].Position != 1 {
		t.Fatalf("after remove = %#v, want kept id at position 1", removed.Criteria)
	}

	readiness, err := store.CheckIssueReadiness(ctx, root, parent.Alias)
	if err != nil {
		t.Fatalf("CheckIssueReadiness() error = %v", err)
	}
	if !readiness.Covered {
		t.Fatalf("readiness = %#v, want claim to survive compact-renumber", readiness)
	}
}
