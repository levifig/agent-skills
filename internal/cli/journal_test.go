package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJournalSearchReturnsHitsWithoutScanningDocs proves `loaf journal search`
// (SPEC-056 M1) returns journal hits without touching the docs index: a broken
// (invalid UTF-8) docs fixture that would fail the docs scan must not affect the
// journal search command.
func TestJournalSearchReturnsHitsWithoutScanningDocs(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)

	logRun := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRun.Run([]string{"journal", "log", "decision(search): sphinx searchable entry"}); err != nil {
		t.Fatalf("journal log error = %v", err)
	}

	// A docs file whose bytes are not valid UTF-8 makes any docs-index scan fail.
	// If journal search routed through the global docs-indexing path, it would
	// error on this fixture.
	writeBrokenCLIDocsFile(t, workingDir, "docs/broken.md")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"journal", "search", "sphinx"})
	if err != nil {
		t.Fatalf("journal search error = %v (must not scan docs)", err)
	}
	out := stdout.String()
	if strings.Contains(out, "docs") && strings.Contains(out, "UTF-8") {
		t.Fatalf("journal search stdout = %q, must not report a docs scan error", out)
	}
	for _, want := range []string{"loaf journal search", "results: 1", "sphinx searchable entry"} {
		if !strings.Contains(out, want) {
			t.Fatalf("journal search stdout = %q, want %q", out, want)
		}
	}
}

// TestJournalSearchJSONReturnsOnlyJournalHits confirms the JSON output shape
// carries the journal hits the command emits today.
func TestJournalSearchJSONReturnsOnlyJournalHits(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	logRun := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRun.Run([]string{"journal", "log", "finding(search): obsidian json hit"}); err != nil {
		t.Fatalf("journal log error = %v", err)
	}
	writeBrokenCLIDocsFile(t, workingDir, "docs/broken.md")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "search", "--json", "obsidian"}); err != nil {
		t.Fatalf("journal search --json error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"source": "journal_entry"`) {
		t.Fatalf("journal search --json stdout = %q, want journal_entry hit", out)
	}
	if strings.Contains(out, `"source": "docs_index"`) || strings.Contains(out, `"source": "artifact_body"`) {
		t.Fatalf("journal search --json stdout = %q, want only journal hits", out)
	}
}

// TestJournalSearchErrorsOnMissingStateNonHook confirms the interactive search
// path still errors when state is missing (M2 leaves non-hook behavior intact).
func TestJournalSearchErrorsOnMissingStateNonHook(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir() // no state init: database absent

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"journal", "search", "anything"})
	if err == nil {
		t.Fatal("journal search error = nil, want uninitialized-state error on missing DB")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("journal search error = %v, want uninitialized-state sentinel", err)
	}
}

func writeBrokenCLIDocsFile(t *testing.T, rootPath string, relativePath string) {
	t.Helper()
	path := filepath.Join(rootPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
