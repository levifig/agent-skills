package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestArtifactBodyHelpersMaintainFTSInSameTransaction(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	body, err := store.UpsertArtifactBody(ctx, projectID, "report", "report-one", ArtifactBodyKindMarkdown, "alpha needle", "")
	if err != nil {
		t.Fatalf("UpsertArtifactBody() error = %v", err)
	}
	if body.ContentHash != artifactBodyHash("alpha needle") {
		t.Fatalf("ContentHash = %q, want hash of content", body.ContentHash)
	}

	read, ok, err := store.ReadArtifactBody(ctx, projectID, "report", "report-one", ArtifactBodyKindMarkdown)
	if err != nil {
		t.Fatalf("ReadArtifactBody() error = %v", err)
	}
	if !ok || read.Content != "alpha needle" {
		t.Fatalf("ReadArtifactBody() = %#v, %v; want alpha needle", read, ok)
	}
	assertArtifactSearchHitCount(t, store, "needle", 1)

	if _, err := store.UpsertArtifactBody(ctx, projectID, "report", "report-one", ArtifactBodyKindMarkdown, "beta haystack", ""); err != nil {
		t.Fatalf("second UpsertArtifactBody() error = %v", err)
	}
	assertArtifactSearchHitCount(t, store, "needle", 0)
	assertArtifactSearchHitCount(t, store, "haystack", 1)

	if err := store.DeleteArtifactBody(ctx, projectID, "report", "report-one", ArtifactBodyKindMarkdown); err != nil {
		t.Fatalf("DeleteArtifactBody() error = %v", err)
	}
	_, ok, err = store.ReadArtifactBody(ctx, projectID, "report", "report-one", ArtifactBodyKindMarkdown)
	if err != nil {
		t.Fatalf("ReadArtifactBody(after delete) error = %v", err)
	}
	if ok {
		t.Fatal("ReadArtifactBody(after delete) ok = true, want false")
	}
	assertArtifactSearchHitCount(t, store, "haystack", 0)
}

func TestArtifactBodyFallbackReadsMarkdownSourceWhenNoSQLiteBodyExists(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	writeAgentsFile(t, root.Path(), "tasks/TASK-001-fallback.md", `---
id: TASK-001
---
# Fallback Body

Markdown fallback prose.
`)
	body, err := store.artifactBodyOrSourceBody(ctx, root.Path(), status.ProjectID, "task", "task-one", sql.NullString{String: ".agents/tasks/TASK-001-fallback.md", Valid: true})
	if err != nil {
		t.Fatalf("artifactBodyOrSourceBody() error = %v", err)
	}
	if body != "# Fallback Body\n\nMarkdown fallback prose." {
		t.Fatalf("body = %q, want frontmatter-stripped fallback body", body)
	}
}

func TestReadImportedSourceBodyRejectsEscapingPaths(t *testing.T) {
	if _, err := readImportedSourceBody(filepath.Clean("/tmp/project"), "../secret.md"); err == nil {
		t.Fatal("readImportedSourceBody() error = nil, want escaping path rejection")
	}
}

func assertArtifactSearchHitCount(t *testing.T, store *Store, query string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM artifact_search WHERE artifact_search MATCH ?`, query).Scan(&got); err != nil {
		t.Fatalf("artifact search count for %q error = %v", query, err)
	}
	if got != want {
		t.Fatalf("artifact search count for %q = %d, want %d", query, got, want)
	}
}
