package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestBundlesCollectRowsByTagQueryAndExplicitMembership(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-bundle.md", "# Bundle Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-bundle.md", "# Bundle Task\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-bundle-idea.md", "# Bundle Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Bundle Task","spec":"SPEC-001","status":"todo"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if _, err := AddTag(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001", "sqlite"); err != nil {
		t.Fatalf("AddTag(spec) error = %v", err)
	}
	if _, err := AddTag(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-bundle-idea", "sqlite"); err != nil {
		t.Fatalf("AddTag(idea) error = %v", err)
	}

	created, err := CreateBundle(context.Background(), root, PathResolver{StateHome: stateHome}, BundleCreateOptions{
		Slug:  "sqlite-backend",
		Title: "SQLite Backend",
		Tags:  []string{"sqlite"},
	})
	if err != nil {
		t.Fatalf("CreateBundle() error = %v", err)
	}
	if created.Slug != "sqlite-backend" || created.Title != "SQLite Backend" || len(created.Tags) != 1 || created.Tags[0] != "sqlite" {
		t.Fatalf("created = %#v, want sqlite-backend bundle with sqlite tag", created)
	}
	assertBundleMutationContext(t, created, root)

	added, err := AddBundleMember(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend", "TASK-001")
	if err != nil {
		t.Fatalf("AddBundleMember(task) error = %v", err)
	}
	assertBundleMutationContext(t, added, root)
	if _, err := AddBundleMember(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend", "SPEC-001"); err != nil {
		t.Fatalf("AddBundleMember(spec duplicate) error = %v", err)
	}
	if _, err := AddBundleMember(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend", "TASK-001"); err != nil {
		t.Fatalf("idempotent AddBundleMember(task) error = %v", err)
	}

	show, err := ShowBundle(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend")
	if err != nil {
		t.Fatalf("ShowBundle() error = %v", err)
	}
	if len(show.TagMatched) != 2 || len(show.Explicit) != 2 || len(show.Members) != 3 {
		t.Fatalf("show = %#v, want 2 tag-matched, 2 explicit, 3 union members", show)
	}
	assertBundleShowContext(t, show, root)
	if !hasBundleMember(show.Members, "spec", "SPEC-001") || !hasBundleMember(show.Members, "task", "TASK-001") || !hasBundleMember(show.Members, "idea", "20260528-bundle-idea") {
		t.Fatalf("members = %#v, want spec, task, and idea", show.Members)
	}

	list, err := ListBundles(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListBundles() error = %v", err)
	}
	item := list.Bundles["sqlite-backend"]
	if item.Title != "SQLite Backend" || item.ExplicitCount != 2 || item.TagMatchedCount != 2 || item.MemberCount != 3 {
		t.Fatalf("bundle list item = %#v, want explicit/tag/union counts", item)
	}
	assertBundleListContext(t, list, root)

	updated, err := UpdateBundle(context.Background(), root, PathResolver{StateHome: stateHome}, BundleUpdateOptions{
		Slug:     "sqlite-backend",
		Title:    "SQLite Runtime",
		SetTitle: true,
		Tags:     []string{"sqlite", "state"},
		SetTags:  true,
	})
	if err != nil {
		t.Fatalf("UpdateBundle() error = %v", err)
	}
	if updated.Title != "SQLite Runtime" || len(updated.Tags) != 2 || updated.Tags[0] != "sqlite" || updated.Tags[1] != "state" {
		t.Fatalf("updated = %#v, want new title and sorted tags", updated)
	}
	assertBundleMutationContext(t, updated, root)
	show, err = ShowBundle(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend")
	if err != nil {
		t.Fatalf("ShowBundle() after update error = %v", err)
	}
	if show.Title != "SQLite Runtime" || len(show.TagQuery) != 2 {
		t.Fatalf("show after update = %#v, want updated title and tags", show)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var explicit int
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM bundle_members
JOIN bundles ON bundles.id = bundle_members.bundle_id AND bundles.project_id = bundle_members.project_id
WHERE bundle_members.project_id = ? AND bundles.slug = 'sqlite-backend'
`, projectIDForTest(t, store, root)).Scan(&explicit)
	if err != nil {
		t.Fatalf("count explicit bundle members error = %v", err)
	}
	if explicit != 2 {
		t.Fatalf("explicit members = %d, want 2 after idempotent add", explicit)
	}

	removed, err := RemoveBundleMember(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend", "TASK-001")
	if err != nil {
		t.Fatalf("RemoveBundleMember(task) error = %v", err)
	}
	assertBundleMutationContext(t, removed, root)
	show, err = ShowBundle(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend")
	if err != nil {
		t.Fatalf("ShowBundle() after remove error = %v", err)
	}
	if len(show.Members) != 2 || hasBundleMember(show.Members, "task", "TASK-001") {
		t.Fatalf("members after remove = %#v, want task removed and tag members retained", show.Members)
	}
}

func assertBundleMutationContext(t *testing.T, result BundleMutationResult, root project.Root) {
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

func assertBundleListContext(t *testing.T, result BundleList, root project.Root) {
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

func assertBundleShowContext(t *testing.T, result BundleShowResult, root project.Root) {
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

func TestRemoveBundleMemberRejectsMissingExplicitMembership(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-bundle-idea.md", "# Bundle Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if _, err := CreateBundle(context.Background(), root, PathResolver{StateHome: stateHome}, BundleCreateOptions{Slug: "sqlite-backend"}); err != nil {
		t.Fatalf("CreateBundle() error = %v", err)
	}

	_, err = RemoveBundleMember(context.Background(), root, PathResolver{StateHome: stateHome}, "sqlite-backend", "20260528-bundle-idea")
	if err == nil {
		t.Fatal("RemoveBundleMember() error = nil, want missing explicit membership rejection")
	}
}

func hasBundleMember(members []TraceEntity, kind string, alias string) bool {
	for _, member := range members {
		if member.Kind == kind && member.Alias == alias {
			return true
		}
	}
	return false
}
