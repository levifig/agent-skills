package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestIndexDocsScansMarkdownMaintainsFTSAndPrunes(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeDocsFile(t, root.Path(), "docs/guide.md", "# Guide\n\nalpha needle")
	writeDocsFile(t, root.Path(), "docs/note.txt", "alpha ignored")
	writeDocsFile(t, root.Path(), "docs/decisions/README.md", "alpha generated index")

	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	first, err := store.IndexDocs(ctx, root, DocsIndexOptions{})
	if err != nil {
		t.Fatalf("IndexDocs(first) error = %v", err)
	}
	assertDocsIndexContext(t, root, first)
	if first.Scanned != 1 || first.Indexed != 1 || first.Removed != 0 {
		t.Fatalf("first counts = scanned %d indexed %d removed %d, want 1/1/0", first.Scanned, first.Indexed, first.Removed)
	}
	if len(first.Docs) != 1 || first.Docs[0].Path != "docs/guide.md" || first.Docs[0].ContentHash == "" {
		t.Fatalf("first docs = %#v, want indexed docs/guide.md with content hash", first.Docs)
	}
	assertDocsSearchHitCount(t, store, "needle", 1)
	assertDocsSearchHitCount(t, store, "ignored", 0)
	assertDocsSearchHitCount(t, store, "generated", 0)

	writeDocsFile(t, root.Path(), "docs/guide.md", "# Guide\n\nbeta haystack")
	second, err := store.IndexDocs(ctx, root, DocsIndexOptions{})
	if err != nil {
		t.Fatalf("IndexDocs(second) error = %v", err)
	}
	if second.Scanned != 1 || second.Indexed != 1 || second.Removed != 0 {
		t.Fatalf("second counts = scanned %d indexed %d removed %d, want 1/1/0", second.Scanned, second.Indexed, second.Removed)
	}
	assertDocsSearchHitCount(t, store, "needle", 0)
	assertDocsSearchHitCount(t, store, "haystack", 1)

	if err := os.Remove(filepath.Join(root.Path(), "docs", "guide.md")); err != nil {
		t.Fatalf("Remove(docs/guide.md) error = %v", err)
	}
	third, err := store.IndexDocs(ctx, root, DocsIndexOptions{})
	if err != nil {
		t.Fatalf("IndexDocs(third) error = %v", err)
	}
	if third.Scanned != 0 || third.Indexed != 0 || third.Removed != 1 {
		t.Fatalf("third counts = scanned %d indexed %d removed %d, want 0/0/1", third.Scanned, third.Indexed, third.Removed)
	}
	assertDocsSearchHitCount(t, store, "haystack", 0)
}

func TestScanDocsIndexCandidatesRejectsNonUTF8Markdown(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootPath, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll(docs) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "docs", "bad.md"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatalf("WriteFile(bad.md) error = %v", err)
	}

	_, err := scanDocsIndexCandidates(rootPath)
	if err == nil || !strings.Contains(err.Error(), "must be UTF-8 text") {
		t.Fatalf("scanDocsIndexCandidates() error = %v, want UTF-8 rejection", err)
	}
}

func TestIndexDocsUsesInvokingLinkedWorktreeWhenProvided(t *testing.T) {
	requireGit(t)
	main := initGitRepo(t)
	linked := addLinkedWorktree(t, main, "docs-index-linked")
	writeDocsFile(t, linked, "docs/linked.md", "# Linked\n\nbranch-only docs")
	root, err := project.ResolveRoot(linked)
	if err != nil {
		t.Fatalf("ResolveRoot(linked) error = %v", err)
	}
	if root.Path() != main {
		t.Fatalf("ResolveRoot(linked) = %q, want identity root %q", root.Path(), main)
	}

	ctx := context.Background()
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	result, err := store.IndexDocs(ctx, root, DocsIndexOptions{WorktreePath: linked})
	if err != nil {
		t.Fatalf("IndexDocs(linked) error = %v", err)
	}
	if result.ProjectCurrentPath != main {
		t.Fatalf("ProjectCurrentPath = %q, want identity root %q", result.ProjectCurrentPath, main)
	}
	if result.IndexedWorktree != filepath.ToSlash(linked) || result.IndexedRef != "docs-index-linked" {
		t.Fatalf("index provenance = worktree %q ref %q, want linked worktree branch", result.IndexedWorktree, result.IndexedRef)
	}
	if len(result.Docs) != 1 || result.Docs[0].Path != "docs/linked.md" {
		t.Fatalf("Docs = %#v, want linked worktree docs file", result.Docs)
	}
}

func assertDocsIndexContext(t *testing.T, root interface{ Path() string }, result DocsIndexResult) {
	t.Helper()
	if result.ContractVersion != StateJSONContractVersion || result.DatabaseScope != "global" || result.DatabasePath == "" || result.ProjectID == "" || result.ProjectCurrentPath != root.Path() {
		t.Fatalf("docs index context = %#v, want global project context for %s", result, root.Path())
	}
	if result.IndexedWorktree != filepath.ToSlash(root.Path()) {
		t.Fatalf("IndexedWorktree = %q, want %q", result.IndexedWorktree, filepath.ToSlash(root.Path()))
	}
}

func assertDocsSearchHitCount(t *testing.T, store *Store, query string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM docs_search WHERE docs_search MATCH ?`, query).Scan(&got); err != nil {
		t.Fatalf("docs search count for %q error = %v", query, err)
	}
	if got != want {
		t.Fatalf("docs search count for %q = %d, want %d", query, got, want)
	}
}

func writeDocsFile(t *testing.T, rootPath string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(rootPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
