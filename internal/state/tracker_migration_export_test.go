package state

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTrackerMigrationPacketKeepsOnlyOpenTrackerWork(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()

	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLocal, Prefix: "DOJO"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	parent, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title: "Open parent",
		Body:  "Problem and constraints.",
		Criteria: []IssueCriterionInput{
			{Position: 2, Text: "Operator can complete the flow", Command: "test -f /private/project/secret", Expect: "exit 0"},
			{Position: 1, Text: "Destination is verified"},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssue(parent) error = %v", err)
	}
	child, err := store.CreateIssue(ctx, root, IssueCreateOptions{
		Title:  "Open child",
		Parent: parent.ID,
		Kind:   IssueKindDelivery,
	})
	if err != nil {
		t.Fatalf("CreateIssue(child) error = %v", err)
	}
	done, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Completed work"})
	if err != nil {
		t.Fatalf("CreateIssue(done) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: done.ID, Status: IssueStatusDone, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(done) error = %v", err)
	}
	doneParent, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Completed parent"})
	if err != nil {
		t.Fatalf("CreateIssue(done parent) error = %v", err)
	}
	openUnderDone, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Open under completed parent", Parent: doneParent.ID})
	if err != nil {
		t.Fatalf("CreateIssue(open under done) error = %v", err)
	}
	if _, err := store.UpdateIssue(ctx, root, IssueUpdateOptions{Ref: doneParent.ID, Status: IssueStatusDone, SetStatus: true}); err != nil {
		t.Fatalf("UpdateIssue(done parent) error = %v", err)
	}
	archived, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Cancelled work"})
	if err != nil {
		t.Fatalf("CreateIssue(archived) error = %v", err)
	}
	if _, err := store.RemoveIssue(ctx, root, IssueRemoveOptions{Ref: archived.ID, Status: IssueStatusCancelled}); err != nil {
		t.Fatalf("RemoveIssue(archived) error = %v", err)
	}
	now := "2026-08-31T00:00:00Z"
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES
  ('z-open-edge', (SELECT id FROM projects LIMIT 1), 'issue', ?, 'issue', ?, 'blocks', 'must land first', 'test', ?, ?),
  ('a-open-edge', (SELECT id FROM projects LIMIT 1), 'issue', ?, 'issue', ?, 'relates_to', 'paired work', 'test', ?, ?),
  ('closed-edge', (SELECT id FROM projects LIMIT 1), 'issue', ?, 'issue', ?, 'relates_to', 'historical', 'test', ?, ?)
`, parent.ID, child.ID, now, now, child.ID, parent.ID, now, now, parent.ID, done.ID, now, now); err != nil {
		t.Fatalf("insert relationships: %v", err)
	}

	packet, err := store.ExportTrackerMigration(ctx, root)
	if err != nil {
		t.Fatalf("ExportTrackerMigration() error = %v", err)
	}
	if packet.ContractVersion != TrackerMigrationContractVersion || packet.ExportKind != ExportKindTrackerMigration {
		t.Fatalf("packet contract = %#v", packet)
	}
	if packet.Project.ID == "" || packet.Project.LabelHint == "" {
		t.Fatalf("project = %#v, want stable identity and label", packet.Project)
	}
	if len(packet.Issues) != 3 {
		t.Fatalf("issues = %#v, want three open issues", packet.Issues)
	}
	if packet.Issues[0].ID != parent.ID || packet.Issues[0].Alias != "DOJO-1" || packet.Issues[0].Body != "Problem and constraints." {
		t.Fatalf("parent export = %#v", packet.Issues[0])
	}
	if got := packet.Issues[0].DefinitionOfDone; !reflect.DeepEqual(got, []TrackerMigrationCriterion{
		{Position: 1, Text: "Operator can complete the flow"},
		{Position: 2, Text: "Destination is verified"},
	}) {
		t.Fatalf("definition_of_done = %#v", got)
	}
	if packet.Issues[1].ParentID != parent.ID {
		t.Fatalf("child export = %#v, want parent_id %q", packet.Issues[1], parent.ID)
	}
	if packet.Issues[2].ID != openUnderDone.ID || packet.Issues[2].ParentID != "" {
		t.Fatalf("open issue under omitted completed parent = %#v, want no dangling parent_id", packet.Issues[2])
	}
	if len(packet.Relationships) != 2 ||
		packet.Relationships[0].FromIssueID != child.ID || packet.Relationships[0].ToIssueID != parent.ID || packet.Relationships[0].Type != IssueRelationshipRelatesTo ||
		packet.Relationships[1].FromIssueID != parent.ID || packet.Relationships[1].ToIssueID != child.ID || packet.Relationships[1].Type != IssueRelationshipBlocks {
		t.Fatalf("relationships = %#v, want only open issue edges in stable source-ID order", packet.Relationships)
	}

	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("json.Marshal(packet) error = %v", err)
	}
	if !strings.Contains(string(encoded), `"label_hint"`) || strings.Contains(string(encoded), `"label":`) {
		t.Fatalf("project label contract = %s, want label_hint only", encoded)
	}
	for _, forbidden := range []string{
		"database_path", "database_scope", "project_current_path", "started_branch", "started_worktree",
		"journal", "wrap", "handoff", "idea", "spark", "report", "harness_session", "/private/project/secret",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("packet leaked forbidden field/value %q: %s", forbidden, encoded)
		}
	}

	again, err := store.ExportTrackerMigration(ctx, root)
	if err != nil {
		t.Fatalf("second ExportTrackerMigration() error = %v", err)
	}
	if !reflect.DeepEqual(packet, again) {
		t.Fatalf("exports differ:\nfirst:  %#v\nsecond: %#v", packet, again)
	}
	all, err := store.ExportIssues(ctx, root)
	if err != nil {
		t.Fatalf("ExportIssues() after migration export error = %v", err)
	}
	if len(all.Issues) != 6 {
		t.Fatalf("source issues after export = %d, want 6 untouched", len(all.Issues))
	}
}

func TestTrackerMigrationPacketUsesOneReadSnapshot(t *testing.T) {
	root, store := issueTestFixture(t)
	ctx := context.Background()
	if _, err := store.SetIssueIdentity(ctx, root, IssueIdentityOptions{Authority: IssueAuthorityLocal, Prefix: "SNAP"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	first, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "First"})
	if err != nil {
		t.Fatalf("CreateIssue(first) error = %v", err)
	}
	second, err := store.CreateIssue(ctx, root, IssueCreateOptions{Title: "Second"})
	if err != nil {
		t.Fatalf("CreateIssue(second) error = %v", err)
	}

	writer, err := OpenStore(store.path)
	if err != nil {
		t.Fatalf("OpenStore(writer) error = %v", err)
	}
	defer writer.Close()
	now := "2026-08-31T00:00:00Z"
	packet, err := store.exportTrackerMigration(ctx, root, trackerMigrationExportHooks{
		afterIssuesRead: func() error {
			_, err := writer.db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES ('mid-export-edge', (SELECT id FROM projects LIMIT 1), 'issue', ?, 'issue', ?, 'blocks', 'arrived during export', 'test', ?, ?)
`, first.ID, second.ID, now, now)
			return err
		},
	})
	if err != nil {
		t.Fatalf("exportTrackerMigration() error = %v", err)
	}
	if len(packet.Relationships) != 0 {
		t.Fatalf("snapshot relationships = %#v, want mid-export write excluded", packet.Relationships)
	}

	after, err := store.ExportTrackerMigration(ctx, root)
	if err != nil {
		t.Fatalf("ExportTrackerMigration(after write) error = %v", err)
	}
	if len(after.Relationships) != 1 || after.Relationships[0].Reason != "arrived during export" {
		t.Fatalf("later snapshot relationships = %#v, want committed edge", after.Relationships)
	}
}
