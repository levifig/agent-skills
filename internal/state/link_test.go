package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestLinksCreateListRemoveAndTraceRelationships(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-link.md", "# Link Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-link.md", "# Link Task\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-link-idea.md", "# Link Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Link Task","status":"todo"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	created, err := CreateLink(context.Background(), root, PathResolver{StateHome: stateHome}, LinkMutationOptions{
		From:   "20260528-link-idea",
		To:     "SPEC-001",
		Type:   "resolved_by",
		Reason: "captured in task test",
	})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}
	if created.Type != "resolved_by" || created.From.Alias != "20260528-link-idea" || created.To.Alias != "SPEC-001" || created.Reason != "captured in task test" {
		t.Fatalf("created = %#v, want idea resolved_by SPEC-001", created)
	}
	assertLinkMutationContext(t, created, root)

	updated, err := CreateLink(context.Background(), root, PathResolver{StateHome: stateHome}, LinkMutationOptions{
		From:   "20260528-link-idea",
		To:     "SPEC-001",
		Type:   "resolved_by",
		Reason: "updated reason",
	})
	if err != nil {
		t.Fatalf("idempotent CreateLink() error = %v", err)
	}
	if updated.RelationshipID != created.RelationshipID || updated.Reason != "updated reason" {
		t.Fatalf("updated = %#v, want same relationship with updated reason", updated)
	}

	if _, err := CreateLink(context.Background(), root, PathResolver{StateHome: stateHome}, LinkMutationOptions{
		From: "TASK-001",
		To:   "SPEC-001",
		Type: "implements",
	}); err != nil {
		t.Fatalf("CreateLink(task implements spec) error = %v", err)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var relationshipRows int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM relationships WHERE project_id = ?`, projectIDForTest(t, store, root)).Scan(&relationshipRows); err != nil {
		t.Fatalf("count relationships error = %v", err)
	}
	if relationshipRows != 2 {
		t.Fatalf("relationship rows = %d, want 2 after idempotent upsert and second link", relationshipRows)
	}

	list, err := ListLinks(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ListLinks() error = %v", err)
	}
	if len(list.Relationships) != 2 {
		t.Fatalf("relationships = %#v, want two inbound links", list.Relationships)
	}
	assertLinkListContext(t, list, root)
	if !hasStateTraceRelationship(list.Relationships, "inbound", "resolved_by", "idea", "20260528-link-idea") {
		t.Fatalf("relationships = %#v, want inbound idea resolution", list.Relationships)
	}
	if !hasStateTraceRelationship(list.Relationships, "inbound", "implements", "task", "TASK-001") {
		t.Fatalf("relationships = %#v, want inbound task implementation", list.Relationships)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-link-idea")
	if err != nil {
		t.Fatalf("Trace(idea) error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "resolved_by", "spec", "SPEC-001") {
		t.Fatalf("trace relationships = %#v, want outbound link", trace.Relationships)
	}

	removed, err := RemoveLink(context.Background(), root, PathResolver{StateHome: stateHome}, LinkMutationOptions{
		From: "20260528-link-idea",
		To:   "SPEC-001",
		Type: "resolved_by",
	})
	if err != nil {
		t.Fatalf("RemoveLink() error = %v", err)
	}
	if removed.Reason != "updated reason" {
		t.Fatalf("removed.Reason = %q, want stored reason", removed.Reason)
	}
	assertLinkMutationContext(t, removed, root)
	trace, err = Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-link-idea")
	if err != nil {
		t.Fatalf("Trace(idea) after remove error = %v", err)
	}
	if hasStateTraceRelationship(trace.Relationships, "outbound", "resolved_by", "spec", "SPEC-001") {
		t.Fatalf("trace relationships = %#v, want resolved_by link removed", trace.Relationships)
	}
}

func assertLinkMutationContext(t *testing.T, result LinkMutationResult, root project.Root) {
	t.Helper()
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
}

func assertLinkListContext(t *testing.T, result LinkListResult, root project.Root) {
	t.Helper()
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
}

func TestRemoveLinkRejectsMissingRelationship(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-link.md", "# Link Spec\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-link-idea.md", "# Link Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = RemoveLink(context.Background(), root, PathResolver{StateHome: stateHome}, LinkMutationOptions{
		From: "20260528-link-idea",
		To:   "SPEC-001",
		Type: "resolved_by",
	})
	if err == nil {
		t.Fatal("RemoveLink() error = nil, want missing relationship rejection")
	}
}
