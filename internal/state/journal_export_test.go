package state

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func seedExportFixture(t *testing.T, stateHome string, root project.Root) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "decision", "core", "chose sqlite", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "wrap", "", "wrapped the day", "main", "2026-07-01T10:00:00Z")
	seedJournalEntry(t, store, projectID, "task", "feat", "shipped journal recent", "feat/x", "2026-07-01T11:00:00Z")
}

func TestExportJournalMarkdownProducesReadableTimeline(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedExportFixture(t, stateHome, root)

	export, err := ExportJournalMarkdown(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ExportJournalMarkdown() error = %v", err)
	}
	if export.ExportKind != ExportKindJournal || export.Format != ExportFormatMarkdown {
		t.Fatalf("export kind/format = %q/%q, want journal/markdown", export.ExportKind, export.Format)
	}
	content := export.Content
	if !strings.Contains(content, "# Journal Export") {
		t.Fatalf("markdown missing title heading:\n%s", content)
	}
	if !strings.Contains(content, "## Journal Entries") {
		t.Fatalf("markdown missing entries section:\n%s", content)
	}
	for _, want := range []string{"chose sqlite", "wrapped the day", "shipped journal recent"} {
		if !strings.Contains(content, want) {
			t.Fatalf("markdown missing entry %q:\n%s", want, content)
		}
	}
	// Oldest-first: the earliest decision must appear before the later task.
	if strings.Index(content, "chose sqlite") > strings.Index(content, "shipped journal recent") {
		t.Fatalf("markdown timeline not oldest-first:\n%s", content)
	}
}

func TestExportJournalJSONLProducesValidPerLineJSON(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedExportFixture(t, stateHome, root)

	export, err := ExportJournalJSONL(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ExportJournalJSONL() error = %v", err)
	}
	if export.Format != ExportFormatJSONL {
		t.Fatalf("export format = %q, want jsonl", export.Format)
	}

	scanner := bufio.NewScanner(strings.NewReader(export.Content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var messages []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record JournalEntryRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid JSONL line %q: %v", line, err)
		}
		if record.ID == "" || record.EntryType == "" || record.Message == "" {
			t.Fatalf("JSONL record missing required fields: %#v", record)
		}
		messages = append(messages, record.Message)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan JSONL error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("JSONL line count = %d, want 3 (%v)", len(messages), messages)
	}
	if messages[0] != "chose sqlite" || messages[2] != "shipped journal recent" {
		t.Fatalf("JSONL ordering = %v, want oldest-first", messages)
	}
}

func TestExportJournalEmptyProjectStillValid(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	md, err := ExportJournalMarkdown(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ExportJournalMarkdown(empty) error = %v", err)
	}
	if !strings.Contains(md.Content, "No journal entries recorded.") {
		t.Fatalf("empty markdown missing empty-state line:\n%s", md.Content)
	}

	jsonl, err := ExportJournalJSONL(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ExportJournalJSONL(empty) error = %v", err)
	}
	if strings.TrimSpace(jsonl.Content) != "" {
		t.Fatalf("empty JSONL content = %q, want empty", jsonl.Content)
	}
}
