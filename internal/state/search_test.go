package state

import (
	"context"
	"strings"
	"testing"
)

func TestSearchReturnsCurrentProjectArtifactAndJournalHits(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(root) error = %v", err)
	}
	if _, err := Initialize(ctx, otherRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(otherRoot) error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	otherProjectID := projectIDForTest(t, store, otherRoot)

	if _, err := store.UpsertArtifactBody(ctx, projectID, "report", "report-current", ArtifactBodyKindMarkdown, "alpha current artifact", ""); err != nil {
		t.Fatalf("UpsertArtifactBody(current) error = %v", err)
	}
	if _, err := store.UpsertArtifactBody(ctx, otherProjectID, "report", "report-other", ArtifactBodyKindMarkdown, "alpha other artifact", ""); err != nil {
		t.Fatalf("UpsertArtifactBody(other) error = %v", err)
	}
	if _, err := LogJournal(ctx, root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "decision(search): alpha journal entry"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}

	current, err := Search(ctx, root, PathResolver{StateHome: stateHome}, SearchOptions{Query: "alpha"})
	if err != nil {
		t.Fatalf("Search(current) error = %v", err)
	}
	assertSearchProjectContext(t, root, current)
	if len(current.Results) != 2 {
		t.Fatalf("current results = %#v, want artifact + journal hits", current.Results)
	}
	if !searchHitsContain(current.Results, "artifact_body", projectID, "report-current") || !searchHitsContain(current.Results, "journal_entry", projectID, "") {
		t.Fatalf("current results = %#v, want current artifact and journal", current.Results)
	}
	if searchHitsContain(current.Results, "artifact_body", otherProjectID, "report-other") {
		t.Fatalf("current results = %#v, want no cross-project hit", current.Results)
	}

	all, err := Search(ctx, root, PathResolver{StateHome: stateHome}, SearchOptions{Query: "alpha", AllProjects: true})
	if err != nil {
		t.Fatalf("Search(all projects) error = %v", err)
	}
	if !all.AllProjects {
		t.Fatalf("AllProjects = false, want true")
	}
	if !searchHitsContain(all.Results, "artifact_body", otherProjectID, "report-other") {
		t.Fatalf("all-project results = %#v, want other project hit", all.Results)
	}
}

func TestSearchRedactsSnippetsAndUpdatesArtifactIndex(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)

	if _, err := store.UpsertArtifactBody(ctx, projectID, "report", "report-token", ArtifactBodyKindMarkdown, "oldtokenxyz sk-abcdefghijklmnopqrstuvwxyz123456", ""); err != nil {
		t.Fatalf("UpsertArtifactBody(token) error = %v", err)
	}
	tokenResult, err := store.Search(ctx, root, SearchOptions{Query: "oldtokenxyz"})
	if err != nil {
		t.Fatalf("Search(token) error = %v", err)
	}
	if len(tokenResult.Results) != 1 || tokenResult.Results[0].Snippet == "" {
		t.Fatalf("token results = %#v, want one snippet", tokenResult.Results)
	}
	if containsAny(tokenResult.Results[0].Snippet, "sk-abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("snippet = %q, want token redacted", tokenResult.Results[0].Snippet)
	}
	if !containsAny(tokenResult.Results[0].Snippet, "[REDACTED_TOKEN]") {
		t.Fatalf("snippet = %q, want redaction marker", tokenResult.Results[0].Snippet)
	}

	if _, err := store.UpsertArtifactBody(ctx, projectID, "report", "report-token", ArtifactBodyKindMarkdown, "newtokenabc plain body", ""); err != nil {
		t.Fatalf("UpsertArtifactBody(update) error = %v", err)
	}
	oldHits, err := store.Search(ctx, root, SearchOptions{Query: "oldtokenxyz"})
	if err != nil {
		t.Fatalf("Search(old) error = %v", err)
	}
	if len(oldHits.Results) != 0 {
		t.Fatalf("old hits = %#v, want none after update", oldHits.Results)
	}
	newHits, err := store.Search(ctx, root, SearchOptions{Query: "newtokenabc"})
	if err != nil {
		t.Fatalf("Search(new) error = %v", err)
	}
	if !searchHitsContain(newHits.Results, "artifact_body", projectID, "report-token") {
		t.Fatalf("new hits = %#v, want updated artifact", newHits.Results)
	}
}

func TestSearchReturnsTier2DocsHits(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	stateHome := t.TempDir()
	writeDocsFile(t, root.Path(), "docs/guide.md", "# Guide\n\nshared-doc-term current")
	writeDocsFile(t, otherRoot.Path(), "docs/guide.md", "# Guide\n\nshared-doc-term other")
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(root) error = %v", err)
	}
	if _, err := Initialize(ctx, otherRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(otherRoot) error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	otherProjectID := projectIDForTest(t, store, otherRoot)
	if _, err := store.IndexDocs(ctx, root, DocsIndexOptions{}); err != nil {
		t.Fatalf("IndexDocs(root) error = %v", err)
	}
	if _, err := store.IndexDocs(ctx, otherRoot, DocsIndexOptions{}); err != nil {
		t.Fatalf("IndexDocs(otherRoot) error = %v", err)
	}

	current, err := store.Search(ctx, root, SearchOptions{Query: "shared-doc-term"})
	if err != nil {
		t.Fatalf("Search(current) error = %v", err)
	}
	if !searchDocsHitContain(current.Results, projectID, "docs/guide.md") {
		t.Fatalf("current results = %#v, want current docs hit", current.Results)
	}
	currentHit, ok := searchDocsHit(current.Results, projectID, "docs/guide.md")
	if !ok || currentHit.LineStart != 3 || currentHit.Locator != "docs/guide.md:3" {
		t.Fatalf("current docs hit = %#v, %v; want line locator docs/guide.md:3", currentHit, ok)
	}
	if searchDocsHitContain(current.Results, otherProjectID, "docs/guide.md") {
		t.Fatalf("current results = %#v, want no cross-project docs hit", current.Results)
	}

	all, err := store.Search(ctx, root, SearchOptions{Query: "shared-doc-term", AllProjects: true})
	if err != nil {
		t.Fatalf("Search(all projects) error = %v", err)
	}
	if !searchDocsHitContain(all.Results, otherProjectID, "docs/guide.md") {
		t.Fatalf("all-project results = %#v, want other docs hit", all.Results)
	}
	otherHit, ok := searchDocsHit(all.Results, otherProjectID, "docs/guide.md")
	if !ok || otherHit.ProjectName == "" || otherHit.ProjectCurrentPath != otherRoot.Path() {
		t.Fatalf("other docs hit = %#v, %v; want project name and current path %q", otherHit, ok, otherRoot.Path())
	}
}

func TestSearchRefreshesStaleDocsIndexForCurrentWorktree(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeDocsFile(t, root.Path(), "docs/guide.md", "# Guide\n\nold-doc-term")
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	oldHits, err := store.Search(ctx, root, SearchOptions{Query: "old-doc-term"})
	if err != nil {
		t.Fatalf("Search(old) error = %v", err)
	}
	projectID := projectIDForTest(t, store, root)
	if !searchDocsHitContain(oldHits.Results, projectID, "docs/guide.md") {
		t.Fatalf("old hits = %#v, want lazily indexed docs hit", oldHits.Results)
	}

	writeDocsFile(t, root.Path(), "docs/guide.md", "# Guide\n\nnew-doc-term")
	staleHits, err := store.Search(ctx, root, SearchOptions{Query: "old-doc-term"})
	if err != nil {
		t.Fatalf("Search(stale old) error = %v", err)
	}
	if searchDocsHitContain(staleHits.Results, projectID, "docs/guide.md") {
		t.Fatalf("stale hits = %#v, want old docs term pruned after lazy refresh", staleHits.Results)
	}
	newHits, err := store.Search(ctx, root, SearchOptions{Query: "new-doc-term"})
	if err != nil {
		t.Fatalf("Search(new) error = %v", err)
	}
	if !searchDocsHitContain(newHits.Results, projectID, "docs/guide.md") {
		t.Fatalf("new hits = %#v, want refreshed docs hit", newHits.Results)
	}
}

func assertSearchProjectContext(t *testing.T, root interface{ Path() string }, result SearchResult) {
	t.Helper()
	if result.ContractVersion != StateJSONContractVersion || result.DatabaseScope != "global" || result.DatabasePath == "" || result.ProjectID == "" || result.ProjectCurrentPath != root.Path() {
		t.Fatalf("search context = %#v, want global project context for %s", result, root.Path())
	}
}

func searchHitsContain(hits []SearchHit, source string, projectID string, entityID string) bool {
	for _, hit := range hits {
		if hit.Source != source || hit.ProjectID != projectID {
			continue
		}
		if entityID == "" || hit.EntityID == entityID {
			return true
		}
	}
	return false
}

func searchDocsHitContain(hits []SearchHit, projectID string, path string) bool {
	_, ok := searchDocsHit(hits, projectID, path)
	return ok
}

func searchDocsHit(hits []SearchHit, projectID string, path string) (SearchHit, bool) {
	for _, hit := range hits {
		if hit.Tier == "tier2" && hit.Source == "docs_index" && hit.ProjectID == projectID && hit.Path == path {
			return hit, true
		}
	}
	return SearchHit{}, false
}

func containsAny(value string, needle string) bool {
	return strings.Contains(value, needle)
}
