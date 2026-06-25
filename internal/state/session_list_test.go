package state

import (
	"context"
	"testing"
)

func TestListSessionsReadsImportedSQLiteSessions(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-active.md", `---
branch: feature/session-list
status: active
claude_session_id: session-active
---
[2026-05-28 10:00] decision(scope): active entry
`)
	writeAgentsFile(t, root.Path(), "sessions/archive/20260527-archived.md", `---
branch: old/session
status: active
claude_session_id: session-archived
---
[2026-05-27 10:00] discover(scope): archived entry
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	activeOnly, err := ListSessions(context.Background(), root, PathResolver{StateHome: stateHome}, SessionListOptions{})
	if err != nil {
		t.Fatalf("ListSessions(activeOnly) error = %v", err)
	}
	assertSessionProjectContext(t, root, activeOnly.ContractVersion, activeOnly.DatabaseScope, activeOnly.DatabasePath, activeOnly.ProjectID, activeOnly.ProjectName, activeOnly.ProjectCurrentPath)
	if _, ok := activeOnly.Sessions["20260527-archived"]; ok {
		t.Fatal("active-only session list includes archived session")
	}
	active := activeOnly.Sessions["20260528-active"]
	if active.Branch != "feature/session-list" || active.Status != "in_progress" || active.HarnessSessionID != "session-active" {
		t.Fatalf("active session = %#v, want imported metadata", active)
	}
	if active.SourcePath != ".agents/sessions/20260528-active.md" || active.JournalEntries != 1 {
		t.Fatalf("active session provenance = %#v, want source path and one journal entry", active)
	}

	withArchived, err := ListSessions(context.Background(), root, PathResolver{StateHome: stateHome}, SessionListOptions{All: true})
	if err != nil {
		t.Fatalf("ListSessions(all) error = %v", err)
	}
	assertSessionProjectContext(t, root, withArchived.ContractVersion, withArchived.DatabaseScope, withArchived.DatabasePath, withArchived.ProjectID, withArchived.ProjectName, withArchived.ProjectCurrentPath)
	archived := withArchived.Sessions["20260527-archived"]
	if archived.Status != "archived" || archived.SourcePath != ".agents/sessions/archive/20260527-archived.md" || archived.HarnessSessionID != "session-archived" {
		t.Fatalf("archived session = %#v, want archived imported metadata", archived)
	}
}
