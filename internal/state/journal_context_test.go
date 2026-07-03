package state

import (
	"context"
	"testing"
)

func TestJournalContextLayersWrapBranchEntriesAndOpenTasks(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Seed two open tasks and one done task; only the open ones should surface.
	openTask, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open task"})
	if err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	doneTask, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Done task"})
	if err != nil {
		t.Fatalf("CreateTask(done) error = %v", err)
	}
	if _, err := UpdateTaskStatus(context.Background(), root, resolver, doneTask.Task.Alias, "done"); err != nil {
		t.Fatalf("UpdateTaskStatus(done) error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	// Project wrap plus branch entries on the current branch.
	seedJournalEntry(t, store, projectID, "wrap", "", "project checkpoint", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "feat", "chose approach A", "feat/context", "2026-07-01T10:00:00Z")
	seedJournalEntry(t, store, projectID, "discover", "other", "unrelated branch note", "feat/other", "2026-07-01T11:00:00Z")

	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{Branch: "feat/context"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	assertSessionProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	if result.LatestWrap == nil || result.LatestWrap.Message != "project checkpoint" {
		t.Fatalf("latest wrap = %#v, want project checkpoint", result.LatestWrap)
	}
	if len(result.BranchEntries) != 1 || result.BranchEntries[0].Message != "chose approach A" {
		t.Fatalf("branch entries = %#v, want single feat/context entry", result.BranchEntries)
	}
	if len(result.OpenTasks) != 1 || result.OpenTasks[0].Ref != openTask.Task.Alias {
		t.Fatalf("open tasks = %#v, want single open task %s", result.OpenTasks, openTask.Task.Alias)
	}
	if result.OpenTasks[0].Title != "Open task" {
		t.Fatalf("open task title = %q, want Open task", result.OpenTasks[0].Title)
	}
}

func TestJournalContextFreshBranchDegradesToProjectWrapAndTasks(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Still open"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "wrap", "", "project checkpoint", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "main", "main branch work", "main", "2026-07-01T10:00:00Z")

	// A brand-new branch with no entries of its own: wrap + open tasks survive,
	// branch layer is empty.
	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{Branch: "feat/fresh"})
	if err != nil {
		t.Fatalf("JournalContext(fresh branch) error = %v", err)
	}
	if result.LatestWrap == nil || result.LatestWrap.Message != "project checkpoint" {
		t.Fatalf("latest wrap = %#v, want project checkpoint", result.LatestWrap)
	}
	if len(result.BranchEntries) != 0 {
		t.Fatalf("fresh-branch entries = %#v, want none", result.BranchEntries)
	}
	if len(result.OpenTasks) != 1 {
		t.Fatalf("open tasks = %#v, want the single open task", result.OpenTasks)
	}
}

func TestJournalContextNoWrapDegradesGracefully(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "decision", "feat", "branch-only work", "feat/context", "2026-07-01T10:00:00Z")

	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{Branch: "feat/context"})
	if err != nil {
		t.Fatalf("JournalContext(no wrap) error = %v", err)
	}
	if result.LatestWrap != nil {
		t.Fatalf("latest wrap = %#v, want nil (no wrap exists)", result.LatestWrap)
	}
	if len(result.BranchEntries) != 1 {
		t.Fatalf("branch entries = %#v, want single entry", result.BranchEntries)
	}
	if len(result.OpenTasks) != 0 {
		t.Fatalf("open tasks = %#v, want none", result.OpenTasks)
	}
}

func TestJournalContextNoBranchOmitsBranchLayer(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "wrap", "", "project checkpoint", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "main", "main branch work", "main", "2026-07-01T10:00:00Z")

	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{})
	if err != nil {
		t.Fatalf("JournalContext(no branch) error = %v", err)
	}
	if result.LatestWrap == nil {
		t.Fatal("latest wrap = nil, want project checkpoint")
	}
	if len(result.BranchEntries) != 0 {
		t.Fatalf("branch entries = %#v, want none when no branch given", result.BranchEntries)
	}
}
