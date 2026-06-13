package state

import (
	"context"
	"strings"
	"testing"
)

func TestRecordSessionStateSnapshotUpsertsLatestSummary(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "feature/snapshot",
		HarnessSessionID: "snapshot-session",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	first, err := RecordSessionStateSnapshot(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStateSnapshotOptions{
		SessionRef:       start.Session.Alias,
		Content:          "## Current State\n\nfirst summary",
		ObservedBranch:   "feature/snapshot",
		ObservedWorktree: root.Path(),
	})
	if err != nil {
		t.Fatalf("RecordSessionStateSnapshot(first) error = %v", err)
	}
	second, err := RecordSessionStateSnapshot(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStateSnapshotOptions{
		SessionRef:       start.Session.Alias,
		Content:          "## Current State\n\nsecond summary",
		ObservedBranch:   "feature/snapshot",
		ObservedWorktree: root.Path(),
	})
	if err != nil {
		t.Fatalf("RecordSessionStateSnapshot(second) error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("snapshot ID changed from %q to %q, want per-session upsert", first.ID, second.ID)
	}

	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if show.Session.StateSnapshot == nil {
		t.Fatal("StateSnapshot = nil, want latest snapshot")
	}
	if show.Session.StateSnapshot.Content != "## Current State\n\nsecond summary" {
		t.Fatalf("snapshot content = %q, want latest summary", show.Session.StateSnapshot.Content)
	}
	if show.Session.StateSnapshot.ObservedBranch != "feature/snapshot" || show.Session.StateSnapshot.ObservedWorktree != root.Path() {
		t.Fatalf("snapshot provenance = %#v, want branch and worktree", show.Session.StateSnapshot)
	}
}

func TestRecordSessionStateSnapshotRejectsInvalidInputs(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := RecordSessionStateSnapshot(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStateSnapshotOptions{
		SessionRef: "missing-session",
		Content:    "## Current State\n\nsummary",
	})
	if err == nil {
		t.Fatal("RecordSessionStateSnapshot(missing) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found in SQLite state") {
		t.Fatalf("error = %v, want not-found error", err)
	}

	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	_, err = RecordSessionStateSnapshot(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStateSnapshotOptions{
		SessionRef: start.Session.Alias,
		Content:    "   ",
	})
	if err == nil {
		t.Fatal("RecordSessionStateSnapshot(empty) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "content cannot be empty") {
		t.Fatalf("error = %v, want empty-content error", err)
	}
}
