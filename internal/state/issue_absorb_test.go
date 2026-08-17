package state

import (
	"context"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestIssueAbsorbMintsLocalIssueAndArchivesTask(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Leftover work", Priority: "P1"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	result, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias})
	if err != nil {
		t.Fatalf("Absorb() error = %v", err)
	}
	if result.Dismiss || result.Disposition != AbsorbDispositionAbsorbed {
		t.Fatalf("result = %#v, want absorbed", result)
	}
	if result.Issue == nil || result.Issue.Alias != "LOAF-1" || result.Issue.Title != "Leftover work" {
		t.Fatalf("issue = %#v, want LOAF-1 Leftover work", result.Issue)
	}
	if !strings.Contains(result.Issue.Body, "Absorbed from TASK-001") || !strings.Contains(result.Issue.Body, "task:"+created.Task.ID) {
		t.Fatalf("body = %q, want provenance naming TASK-001 and task id", result.Issue.Body)
	}
	if !strings.Contains(result.Issue.Body, "Priority: P1") || !strings.Contains(result.Issue.Body, "Status: todo") {
		t.Fatalf("body = %q, want status and priority", result.Issue.Body)
	}

	shown, err := ShowTask(context.Background(), root, resolver, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != LifecycleStatusArchived {
		t.Fatalf("task status = %q, want archived", shown.Task.Status)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var fromStatus, toStatus, note string
	if err := store.db.QueryRowContext(context.Background(), `
SELECT from_status, to_status, note
FROM events
WHERE project_id = ? AND entity_kind = 'task' AND entity_id = ? AND event_type = 'status_changed' AND to_status = ?
ORDER BY created_at DESC
LIMIT 1
`, result.ProjectID, created.Task.ID, LifecycleStatusArchived).Scan(&fromStatus, &toStatus, &note); err != nil {
		t.Fatalf("read absorb event: %v", err)
	}
	if fromStatus != LifecycleStatusTodo || toStatus != LifecycleStatusArchived {
		t.Fatalf("absorb event %s -> %s, want todo -> archived", fromStatus, toStatus)
	}
	if !strings.Contains(note, "absorbed into") || !strings.Contains(note, result.Issue.ID) {
		t.Fatalf("absorb event note = %q, want absorbed-into and issue id", note)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "TASK-001"); err == nil {
		t.Fatal("GetIssue(TASK-001) error = nil, want missing issue (source alias must not be a live issue identity)")
	}
	issue, err := GetIssue(context.Background(), root, resolver, "LOAF-1")
	if err != nil {
		t.Fatalf("GetIssue(LOAF-1) error = %v", err)
	}
	if issue.ID != result.Issue.ID {
		t.Fatalf("LOAF-1 id = %q, want %q", issue.ID, result.Issue.ID)
	}
}

func TestIssueAbsorbIntentMintsIssueAndResolvesDisposition(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateIntent(context.Background(), root, resolver, IntentCreateOptions{Title: "Parked direction", Body: "Keep this."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}

	result, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Intent.Alias})
	if err != nil {
		t.Fatalf("Absorb() error = %v", err)
	}
	if result.Issue == nil || result.Issue.Alias != "LOAF-1" || result.Issue.Title != "Parked direction" {
		t.Fatalf("issue = %#v, want LOAF-1 Parked direction", result.Issue)
	}
	if !strings.Contains(result.Issue.Body, "Absorbed from "+created.Intent.Alias) || !strings.Contains(result.Issue.Body, "intent:"+created.Intent.ID) {
		t.Fatalf("body = %q, want intent provenance", result.Issue.Body)
	}

	shown, err := ShowIntent(context.Background(), root, resolver, created.Intent.Alias)
	if err != nil {
		t.Fatalf("ShowIntent() error = %v", err)
	}
	if shown.Intent.Disposition != "resolved" || !strings.HasPrefix(shown.Intent.DispositionReason, intentAbsorbReasonPrefix) {
		t.Fatalf("intent = %#v, want resolved absorbed disposition", shown.Intent)
	}
	if _, err := GetIssue(context.Background(), root, resolver, created.Intent.Alias); err == nil {
		t.Fatal("GetIssue(intent alias) error = nil, want missing issue")
	}
}

func TestIssueAbsorbDismissArchivesTaskWithoutMinting(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Drop me"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	result, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: "TASK-001", Dismiss: true})
	if err != nil {
		t.Fatalf("Absorb(--dismiss) error = %v", err)
	}
	if !result.Dismiss || result.Disposition != AbsorbDispositionSuperseded || result.Issue != nil {
		t.Fatalf("result = %#v, want dismissed without issue", result)
	}
	shown, err := ShowTask(context.Background(), root, resolver, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != LifecycleStatusArchived {
		t.Fatalf("status = %q, want archived", shown.Task.Status)
	}
	listed, err := ListIssues(context.Background(), root, resolver, IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", listed.Issues)
	}
}

func TestIssueAbsorbRefuseChangeLocalAlreadyGoneAndUnknown(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if _, err := LookupAbsorbSource(context.Background(), root, resolver, "docs/changes/foo/tasks/TASK-001.md"); err == nil || !strings.Contains(err.Error(), "change-local") {
		t.Fatalf("path ref error = %v, want change-local refusal", err)
	}

	created, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Change local"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	attachTaskBodySource(t, store, created.ProjectID, created.Task.ID, "docs/changes/example/tasks/TASK-001-example.md")
	if _, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias}); err == nil || !strings.Contains(err.Error(), "change-local") {
		t.Fatalf("source-path error = %v, want change-local refusal", err)
	}

	open, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Once"})
	if err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	if _, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: open.Task.Alias}); err != nil {
		t.Fatalf("first Absorb() error = %v", err)
	}
	if _, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: open.Task.Alias}); err == nil || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("second Absorb() error = %v, want not leftover open work archived", err)
	}

	if _, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: "TASK-999"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown ref error = %v, want not found", err)
	}
}

func TestIssueAbsorbIsChangeLocalTaskPath(t *testing.T) {
	for _, path := range []string{
		"docs/changes/foo/tasks/TASK-001.md",
		"./docs/changes/foo/tasks/x.md",
		"docs/changes/tasks/nested.md",
		"prefix/docs/changes/x/tasks/y.md",
	} {
		if !IsChangeLocalTaskPath(path) {
			t.Fatalf("IsChangeLocalTaskPath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{
		"",
		"TASK-001",
		".agents/tasks/TASK-001.md",
		"docs/changes/foo/research/note.md",
	} {
		if IsChangeLocalTaskPath(path) {
			t.Fatalf("IsChangeLocalTaskPath(%q) = true, want false", path)
		}
	}
}

func TestIssueAbsorbRefusesDoneAndArchivedTasksAndAcceptsOpen(t *testing.T) {
	for _, status := range []string{
		LifecycleStatusTodo,
		LifecycleStatusInProgress,
		LifecycleStatusBlocked,
		LifecycleStatusReview,
	} {
		t.Run("absorb_"+status, func(t *testing.T) {
			root, resolver := absorbInitialized(t)
			created, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover " + status})
			if err != nil {
				t.Fatalf("CreateTask() error = %v", err)
			}
			if status != LifecycleStatusTodo {
				if _, err := UpdateTaskStatus(context.Background(), root, resolver, created.Task.Alias, status); err != nil {
					t.Fatalf("UpdateTaskStatus(%s) error = %v", status, err)
				}
			}
			result, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias})
			if err != nil {
				t.Fatalf("Absorb(%s) error = %v", status, err)
			}
			if result.Issue == nil {
				t.Fatalf("Absorb(%s) issue = nil, want minted issue", status)
			}
			shown, err := ShowTask(context.Background(), root, resolver, created.Task.Alias)
			if err != nil {
				t.Fatalf("ShowTask() error = %v", err)
			}
			if shown.Task.Status != LifecycleStatusArchived {
				t.Fatalf("status = %q, want archived", shown.Task.Status)
			}
		})
	}

	t.Run("refuse_done", func(t *testing.T) {
		root, resolver := absorbInitialized(t)
		created := createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
		_, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias})
		if err == nil || !strings.Contains(err.Error(), created.Task.Alias) || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "done") {
			t.Fatalf("Absorb(done) error = %v, want named leftover-open refusal", err)
		}
	})

	t.Run("refuse_done_dismiss", func(t *testing.T) {
		root, resolver := absorbInitialized(t)
		created := createTaskWithStatus(t, root, resolver, "Done dismiss", LifecycleStatusDone)
		_, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias, Dismiss: true})
		if err == nil || !strings.Contains(err.Error(), created.Task.Alias) || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "done") {
			t.Fatalf("Absorb(--dismiss done) error = %v, want named leftover-open refusal", err)
		}
	})

	t.Run("refuse_archived", func(t *testing.T) {
		root, resolver := absorbInitialized(t)
		created := createTaskWithStatus(t, root, resolver, "Archived leftover", LifecycleStatusDone)
		if _, err := ArchiveTasks(context.Background(), root, resolver, TaskArchiveOptions{Refs: []string{created.Task.Alias}}); err != nil {
			t.Fatalf("ArchiveTasks() error = %v", err)
		}
		_, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias})
		if err == nil || !strings.Contains(err.Error(), created.Task.Alias) || !strings.Contains(err.Error(), "not leftover open work") || !strings.Contains(err.Error(), "archived") {
			t.Fatalf("Absorb(archived) error = %v, want named leftover-open refusal", err)
		}
	})
}

func TestIssueAbsorbAcceptsOrdinarilyResolvedIntentOnce(t *testing.T) {
	root, resolver := absorbInitialized(t)
	created, err := CreateIntent(context.Background(), root, resolver, IntentCreateOptions{Title: "Already decided", Body: "Keep this."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	if _, err := ResolveIntent(context.Background(), root, resolver, IntentDispositionOptions{IntentRef: created.Intent.Alias, Reason: "shipped in a prior release"}); err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}

	result, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Intent.Alias})
	if err != nil {
		t.Fatalf("Absorb(ordinarily resolved) error = %v", err)
	}
	if result.Issue == nil {
		t.Fatal("Absorb(ordinarily resolved) issue = nil, want minted issue")
	}

	_, err = Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Intent.Alias})
	if err == nil || !strings.Contains(err.Error(), "already absorbed") {
		t.Fatalf("second Absorb() error = %v, want already absorbed", err)
	}
}

func absorbInitialized(t *testing.T) (project.Root, PathResolver) {
	t.Helper()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return root, resolver
}

func createTaskWithStatus(t *testing.T, root project.Root, resolver PathResolver, title, status string) TaskCreateResult {
	t.Helper()
	created, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: title})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if status != LifecycleStatusTodo {
		if _, err := UpdateTaskStatus(context.Background(), root, resolver, created.Task.Alias, status); err != nil {
			t.Fatalf("UpdateTaskStatus(%s) error = %v", status, err)
		}
	}
	return created
}

func attachTaskBodySource(t *testing.T, store *Store, projectID, taskID, sourcePath string) {
	t.Helper()
	now := "2026-08-17T00:00:00Z"
	sourceID := "src-" + taskID
	if _, err := store.db.Exec(`INSERT INTO sources (id, project_id, source_kind, path, imported_at, created_at, updated_at) VALUES (?, ?, 'markdown', ?, ?, ?, ?)`, sourceID, projectID, sourcePath, now, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE tasks SET body_source_id = ? WHERE id = ?`, sourceID, taskID); err != nil {
		t.Fatalf("set body_source_id: %v", err)
	}
}
