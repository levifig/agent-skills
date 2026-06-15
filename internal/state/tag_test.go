package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestTagsClassifyRequiredEntityKindsThroughManyToManyTable(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-tags.md", "# Tag Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-tags.md", "# Tag Task\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-tag-idea.md", "# Tag Idea\n")
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm.md", "# Tag Brainstorm\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): tag-me spark\n")
	writeAgentsFile(t, root.Path(), "reports/20260528-report.md", "# Tag Report\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Tag Task","spec":"SPEC-001","status":"todo"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	journal, err := LogJournal(context.Background(), root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "discover(tags): journal tag target"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}

	refs := map[string]string{
		"spec":          "SPEC-001",
		"task":          "TASK-001",
		"idea":          "20260528-tag-idea",
		"spark":         "SPARK-tag-me",
		"brainstorm":    "20260528-brainstorm",
		"session":       "20260528-session",
		"report":        "20260528-report",
		"journal_entry": journal.ID,
	}
	for kind, ref := range refs {
		added, err := AddTag(context.Background(), root, PathResolver{StateHome: stateHome}, ref, "SQLite")
		if err != nil {
			t.Fatalf("AddTag(%s %s) error = %v", kind, ref, err)
		}
		assertTagMutationContext(t, added, root)
	}
	if _, err := AddTag(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001", "sqlite"); err != nil {
		t.Fatalf("idempotent AddTag() error = %v", err)
	}

	tags, err := ListTags(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if tags.Tags["sqlite"].Count != len(refs) {
		t.Fatalf("sqlite count = %d, want %d", tags.Tags["sqlite"].Count, len(refs))
	}
	assertTagListContext(t, tags, root)

	show, err := ShowTag(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite")
	if err != nil {
		t.Fatalf("ShowTag() error = %v", err)
	}
	assertTagShowContext(t, show, root)
	gotKinds := map[string]bool{}
	for _, member := range show.Members {
		gotKinds[member.Kind] = true
	}
	for kind := range refs {
		if !gotKinds[kind] {
			t.Fatalf("members = %#v, missing kind %s", show.Members, kind)
		}
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var memberships int
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM entity_tags
JOIN tags ON tags.id = entity_tags.tag_id AND tags.project_id = entity_tags.project_id
WHERE entity_tags.project_id = ? AND tags.name = 'sqlite'
`, projectIDForTest(t, store, root)).Scan(&memberships)
	if err != nil {
		t.Fatalf("count memberships error = %v", err)
	}
	if memberships != len(refs) {
		t.Fatalf("memberships = %d, want %d after idempotent add", memberships, len(refs))
	}

	removed, err := RemoveTag(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001", "sqlite")
	if err != nil {
		t.Fatalf("RemoveTag() error = %v", err)
	}
	assertTagMutationContext(t, removed, root)
	show, err = ShowTag(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite")
	if err != nil {
		t.Fatalf("ShowTag() after remove error = %v", err)
	}
	if len(show.Members) != len(refs)-1 {
		t.Fatalf("member count after remove = %d, want %d", len(show.Members), len(refs)-1)
	}
	for _, member := range show.Members {
		if member.Kind == "task" && member.Alias == "TASK-001" {
			t.Fatalf("members = %#v, removed task still present", show.Members)
		}
	}
}

func assertTagMutationContext(t *testing.T, result TagMutationResult, root project.Root) {
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

func assertTagListContext(t *testing.T, result TagList, root project.Root) {
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

func assertTagShowContext(t *testing.T, result TagShowResult, root project.Root) {
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

func TestRemoveTagRejectsMissingMembership(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-tag-idea.md", "# Tag Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = RemoveTag(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-tag-idea", "sqlite")
	if err == nil {
		t.Fatal("RemoveTag() error = nil, want missing membership rejection")
	}
}
