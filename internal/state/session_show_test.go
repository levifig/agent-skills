package state

import (
	"context"
	"strings"
	"testing"
)

func TestShowSessionReadsImportedSQLiteSession(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-active.md", `---
branch: feature/session-show
status: active
claude_session_id: session-active
---
[2026-05-28 10:00] decision(sqlite): keep session state queryable
[2026-05-28 10:05] discover(sqlite): imported journal entries
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-session.md", "# Session Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Session Task","status":"todo","priority":"P2"}
}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if _, err := UpdateTask(context.Background(), root, PathResolver{StateHome: stateHome}, TaskUpdateOptions{
		Ref:        "TASK-001",
		Session:    "20260528-active",
		SetSession: true,
	}); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-active")
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}

	session := show.Session
	if show.Query != "20260528-active" || session.Alias != "20260528-active" {
		t.Fatalf("show = %#v, want query and alias", show)
	}
	if session.Branch != "feature/session-show" || session.Status != "active" || session.HarnessSessionID != "session-active" {
		t.Fatalf("session metadata = %#v, want imported frontmatter", session)
	}
	if len(session.Sources) != 1 || session.Sources[0].Path != ".agents/sessions/20260528-active.md" || session.Sources[0].Hash == "" {
		t.Fatalf("sources = %#v, want imported source provenance", session.Sources)
	}
	if len(session.JournalEntries) != 2 {
		t.Fatalf("journal entries = %#v, want two imported entries", session.JournalEntries)
	}
	if session.JournalEntries[0].EntryType != "decision" || session.JournalEntries[1].EntryType != "discover" {
		t.Fatalf("journal entries = %#v, want source-order entries", session.JournalEntries)
	}
	if !hasJournalEntry(session.JournalEntries, "decision", "sqlite", "keep session state queryable") {
		t.Fatalf("journal entries = %#v, want decision entry", session.JournalEntries)
	}
	if !hasJournalEntry(session.JournalEntries, "discover", "sqlite", "imported journal entries") {
		t.Fatalf("journal entries = %#v, want discover entry", session.JournalEntries)
	}
	if !hasRelationship(session.Relationships, "inbound", "associated_with", "task", "TASK-001") {
		t.Fatalf("relationships = %#v, want associated task", session.Relationships)
	}
	if session.CreatedAt == "" || session.UpdatedAt == "" {
		t.Fatalf("timestamps = created %q updated %q, want populated", session.CreatedAt, session.UpdatedAt)
	}
}

func TestShowSessionRejectsNonSessionReference(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-demo.md", "# Demo Spec\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-active.md", "# Session\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err == nil {
		t.Fatal("ShowSession(SPEC-001) error = nil, want wrong-kind error")
	}
	if !strings.Contains(err.Error(), "not session") {
		t.Fatalf("error = %v, want wrong-kind error", err)
	}
}

func hasJournalEntry(entries []SessionJournalEntry, entryType string, scope string, message string) bool {
	for _, entry := range entries {
		if entry.EntryType == entryType && entry.Scope == scope && entry.Message == message {
			return true
		}
	}
	return false
}
