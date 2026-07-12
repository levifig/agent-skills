package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJournalContextDiscoversActiveChangesFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	runChangeContextGit(t, root, "init", "-b", "main")
	runChangeContextGit(t, root, "config", "user.email", "test@example.invalid")
	runChangeContextGit(t, root, "config", "user.name", "Test")
	writeChangeContextFixture(t, root, "20260701-active", changeContextFixture("active", "work", "nested-discovery", "", "future-terminal"))
	runChangeContextGit(t, root, "add", "docs/changes")
	runChangeContextGit(t, root, "-c", "commit.gpgsign=false", "commit", "-m", "test: seed active Change")
	writeChangeContextFixture(t, root, "20260701-active", changeContextFixture("active", "work", "nested-discovery", "", "future-terminal")+"\ndirty evidence\n")

	nested := filepath.Join(root, "docs", "notes", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	var initOut bytes.Buffer
	if err := (Runner{Stdout: &initOut, WorkingDir: root}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	readContext := func(workingDir string) journalContextCLIResult {
		t.Helper()
		var stdout bytes.Buffer
		err := (Runner{Stdout: &stdout, WorkingDir: workingDir}).Run([]string{"journal", "context", "--layer", journalContextLayerActiveChanges, "--json"})
		if err != nil {
			t.Fatalf("journal context from %s error = %v\n%s", workingDir, err, stdout.String())
		}
		var result journalContextCLIResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("decode journal context from %s = %v\n%s", workingDir, err, stdout.String())
		}
		if result.Layers.ActiveChanges == nil || !result.Layers.ActiveChanges.Available || result.Layers.ActiveChanges.AvailableCount == 0 || len(result.Layers.ActiveChanges.Items) == 0 {
			t.Fatalf("active Changes from %s = %#v, want available nonempty layer", workingDir, result.Layers.ActiveChanges)
		}
		return result
	}

	fromRoot := readContext(root).Layers.ActiveChanges
	fromNested := readContext(nested).Layers.ActiveChanges
	if !reflect.DeepEqual(fromNested, fromRoot) {
		t.Fatalf("nested active Changes = %#v, want root result %#v", fromNested, fromRoot)
	}
	if got := fromNested.Items[0].ActiveReasons; !reflect.DeepEqual(got, []string{"working_tree_change", "lineage_unresolved"}) {
		t.Fatalf("nested active reasons = %#v, want dirty and unresolved evidence", got)
	}
}

func TestDiscoverActiveChangesUnionsHEADAndWorkingTree(t *testing.T) {
	root := t.TempDir()
	runChangeContextGit(t, root, "init", "-b", "main")
	runChangeContextGit(t, root, "config", "user.email", "test@example.invalid")
	runChangeContextGit(t, root, "config", "user.name", "Test")
	writeChangeContextFixture(t, root, "20260701-current", changeContextFixture("current", "main", "foo", "", "current"))
	writeChangeContextFixture(t, root, "20260702-historical", changeContextFixture("historical", "old", "foo-extra", "", "historical"))
	writeChangeContextFixture(t, root, "20260703-modified", changeContextFixture("modified", "old", "modified-lineage", "", "modified"))
	writeChangeContextFixture(t, root, "20260704-deleted", changeContextFixture("deleted", "old", "deleted-lineage", "", "deleted"))
	writeChangeContextFixture(t, root, "20260706-folder-dirty", changeContextFixture("folder-dirty", "old", "folder-dirty-lineage", "", "folder-dirty"))
	runChangeContextGit(t, root, "add", "docs/changes")
	runChangeContextGit(t, root, "-c", "commit.gpgsign=false", "commit", "-m", "test: seed Changes")

	writeChangeContextFixture(t, root, "20260703-modified", changeContextFixture("modified", "old", "modified-lineage", "", "modified")+"\nchanged\n")
	if err := os.Remove(filepath.Join(root, "docs", "changes", "20260704-deleted", "change.md")); err != nil {
		t.Fatal(err)
	}
	writeChangeContextFixture(t, root, "20260705-untracked", changeContextFixture("untracked", "old", "untracked-lineage", "", "untracked"))
	if err := os.WriteFile(filepath.Join(root, "docs", "changes", "20260706-folder-dirty", "research.md"), []byte("untracked evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	source, err := discoverActiveChanges(root)
	if err != nil {
		t.Fatalf("discoverActiveChanges() error = %v", err)
	}
	if got, want := source.LineageKeys, []string{"deleted-lineage", "folder-dirty-lineage", "foo", "foo-extra", "modified-lineage", "untracked-lineage"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LineageKeys = %#v, want %#v", got, want)
	}
	bySlug := map[string]activeChangeItem{}
	for _, item := range source.Items {
		bySlug[item.Slug] = item
	}
	if _, ok := bySlug["historical"]; ok {
		t.Fatalf("clean historical completed Change unexpectedly active: %#v", bySlug["historical"])
	}
	for slug, state := range map[string]string{"current": "clean", "modified": "modified", "deleted": "deleted", "untracked": "untracked", "folder-dirty": "modified"} {
		item, ok := bySlug[slug]
		if !ok || item.WorktreeState != state {
			t.Fatalf("%s item = %#v, want worktree state %q", slug, item, state)
		}
	}
	if !bySlug["deleted"].RetainedAtHEAD || bySlug["deleted"].SourceSHA256 == "" {
		t.Fatalf("deleted item = %#v, want retained HEAD source evidence", bySlug["deleted"])
	}
	if got := bySlug["current"].ActiveReasons; !reflect.DeepEqual(got, []string{"current_branch_match"}) {
		t.Fatalf("current active reasons = %#v", got)
	}
}

func TestDiscoverActiveChangesRetainsHEADAndDirtyWorktreeLineageKeys(t *testing.T) {
	root := t.TempDir()
	runChangeContextGit(t, root, "init", "-b", "main")
	runChangeContextGit(t, root, "config", "user.email", "test@example.invalid")
	runChangeContextGit(t, root, "config", "user.name", "Test")
	writeChangeContextFixture(t, root, "20260701-rewritten", changeContextFixture("rewritten", "work", "old-lineage", "", "old-terminal"))
	runChangeContextGit(t, root, "add", "docs/changes")
	runChangeContextGit(t, root, "-c", "commit.gpgsign=false", "commit", "-m", "test: retain original lineage")
	writeChangeContextFixture(t, root, "20260701-rewritten", changeContextFixture("rewritten", "work", "new-lineage", "", "new-terminal"))

	source, err := discoverActiveChanges(root)
	if err != nil {
		t.Fatalf("discoverActiveChanges() error = %v", err)
	}
	if got, want := source.LineageKeys, []string{"new-lineage", "old-lineage"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LineageKeys = %#v, want retained HEAD and dirty worktree keys %#v", got, want)
	}
	if len(source.Items) != 1 {
		t.Fatalf("active Changes = %#v, want one dirty Change", source.Items)
	}
	item := source.Items[0]
	if item.Lineage != "new-lineage" || !item.RetainedAtHEAD || item.WorktreeState != "modified" {
		t.Fatalf("active Change = %#v, want worktree lineage with retained HEAD evidence", item)
	}
}

func TestActiveChangesCursorPagesExactlyAndDetectsMismatchAndStaleness(t *testing.T) {
	source := activeChangeSource{Branch: "main", Fingerprint: "one", Items: []activeChangeItem{
		{Slug: "a", ChangePath: "docs/changes/a/change.md"},
		{Slug: "b", ChangePath: "docs/changes/b/change.md"},
		{Slug: "c", ChangePath: "docs/changes/c/change.md"},
	}}
	first, err := activeChangesPage(source, "project-a", "main", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := activeChangesPage(source, "project-a", "main", 2, first.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeActiveChangeCursor(first.Cursor)
	if err != nil {
		t.Fatalf("decode first cursor = %v", err)
	}
	if decoded.Version != activeChangeCursorVersion || decoded.Limit != 2 {
		t.Fatalf("first cursor version/limit = %d/%d, want %d/2", decoded.Version, decoded.Limit, activeChangeCursorVersion)
	}
	if got := []string{first.Items[0].Slug, first.Items[1].Slug, second.Items[0].Slug}; !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("page union = %#v", got)
	}
	if _, err := activeChangesPage(source, "project-a", "main", 1, first.Cursor); !isJournalContextCursorCode(err, activeChangeCursorInvalidCode) || !strings.Contains(err.Error(), "different layer limit") {
		t.Fatalf("changed-limit replay error = %v, want visible invalid cursor limit mismatch", err)
	}
	legacy := decoded
	legacy.Version = 1
	legacy.Checksum = activeChangeCursorChecksum(legacy)
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeActiveChangeCursor(base64.RawURLEncoding.EncodeToString(legacyJSON)); !isJournalContextCursorCode(err, activeChangeCursorInvalidCode) {
		t.Fatalf("version-1 cursor error = %v, want unreleased hard-cut rejection", err)
	}
	if _, err := activeChangesPage(source, "project-b", "main", 2, first.Cursor); !isJournalContextCursorCode(err, activeChangeCursorInvalidCode) {
		t.Fatalf("project mismatch error = %v, want invalid cursor", err)
	}
	source.Fingerprint = "two"
	if _, err := activeChangesPage(source, "project-a", "main", 2, first.Cursor); !isJournalContextCursorCode(err, activeChangeCursorStaleCode) {
		t.Fatalf("source change error = %v, want stale cursor", err)
	}
}

func isJournalContextCursorCode(err error, code string) bool {
	typed, ok := err.(*journalContextCursorError)
	return ok && typed.Code == code
}

func writeChangeContextFixture(t *testing.T, root, folder, content string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "changes", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	date := folder[:4] + "-" + folder[4:6] + "-" + folder[6:8]
	content = strings.Replace(content, "created: 2026-07-01", "created: "+date, 1)
	if err := os.WriteFile(filepath.Join(dir, "change.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func changeContextFixture(slug, branch, lineage, predecessor, terminal string) string {
	predecessorLine := ""
	if predecessor != "" {
		predecessorLine = "predecessor: " + predecessor + "\n"
	}
	return "---\nchange: " + slug + "\ncreated: 2026-07-01\nbranch: " + branch + "\nlineage: " + lineage + "\n" + predecessorLine + "release-after: " + terminal + "\n---\n\n# " + slug + "\n"
}

func runChangeContextGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
