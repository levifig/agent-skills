package state

import (
	"context"
	"testing"
)

func TestArchiveSessionTargetsHarnessSessionOnly(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	target, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "feature/archive-target",
		HarnessSessionID: "harness-archive-target",
	})
	if err != nil {
		t.Fatalf("target StartSession() error = %v", err)
	}
	other, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-archive-other",
	})
	if err != nil {
		t.Fatalf("other StartSession() error = %v", err)
	}

	result, err := ArchiveSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionArchiveOptions{
		Branch:           "main",
		HarnessSessionID: "harness-archive-target",
	})
	if err != nil {
		t.Fatalf("ArchiveSession() error = %v", err)
	}
	if result.Action != SessionArchiveActionArchived || result.Session.ID != target.Session.ID || result.Session.Status != "archived" {
		t.Fatalf("result = %#v, want archived target session", result)
	}

	activeOnly, err := ListSessions(context.Background(), root, PathResolver{StateHome: stateHome}, SessionListOptions{})
	if err != nil {
		t.Fatalf("ListSessions(activeOnly) error = %v", err)
	}
	if _, ok := activeOnly.Sessions[target.Session.Alias]; ok {
		t.Fatalf("active list includes archived target %#v", activeOnly.Sessions[target.Session.Alias])
	}
	if _, ok := activeOnly.Sessions[other.Session.Alias]; !ok {
		t.Fatalf("active list missing untouched active session %s", other.Session.Alias)
	}

	withArchived, err := ListSessions(context.Background(), root, PathResolver{StateHome: stateHome}, SessionListOptions{All: true})
	if err != nil {
		t.Fatalf("ListSessions(all) error = %v", err)
	}
	if withArchived.Sessions[target.Session.Alias].Status != "archived" {
		t.Fatalf("archived session = %#v, want archived status", withArchived.Sessions[target.Session.Alias])
	}
}

func TestArchiveSessionFallsBackToBranch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-branch-archive",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	result, err := ArchiveSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionArchiveOptions{
		Branch: "main",
	})
	if err != nil {
		t.Fatalf("ArchiveSession(branch) error = %v", err)
	}
	if result.Session.ID != start.Session.ID || result.Session.Status != "archived" {
		t.Fatalf("result = %#v, want branch session archived", result)
	}
}

func TestArchiveSessionAlreadyArchivedIsIdempotent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-idempotent-archive",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := ArchiveSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionArchiveOptions{HarnessSessionID: "harness-idempotent-archive"}); err != nil {
		t.Fatalf("first ArchiveSession() error = %v", err)
	}

	result, err := ArchiveSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionArchiveOptions{HarnessSessionID: "harness-idempotent-archive"})
	if err != nil {
		t.Fatalf("second ArchiveSession() error = %v", err)
	}
	if result.Action != SessionArchiveActionAlreadyArchived || result.Session.ID != start.Session.ID {
		t.Fatalf("result = %#v, want already-archived target", result)
	}
}
