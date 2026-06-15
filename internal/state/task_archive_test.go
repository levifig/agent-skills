package state

import (
	"context"
	"path/filepath"
	"testing"
)

func TestArchiveTasksArchivesDoneTasksAndRecordsEvents(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-archive.md", "# Archive Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-done.md", "# Done Task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-todo.md", "# Todo Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Done Task","spec":"SPEC-001","status":"done","priority":"P1"},
  "TASK-002":{"title":"Todo Task","spec":"SPEC-001","status":"todo","priority":"P2"}
}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ArchiveTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskArchiveOptions{Refs: []string{"TASK-001", "TASK-002", "SPEC-001", "TASK-999"}})
	if err != nil {
		t.Fatalf("ArchiveTasks() error = %v", err)
	}
	if len(result.Archived) != 1 || result.Archived[0].Task == nil || result.Archived[0].Task.Alias != "TASK-001" || result.Archived[0].Previous != "done" || result.Archived[0].Status != "archived" || result.Archived[0].EventID == "" {
		t.Fatalf("Archived = %#v, want TASK-001 archived with event", result.Archived)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if len(result.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want todo, wrong-kind, and missing refs", result.Skipped)
	}

	active, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{Active: true})
	if err != nil {
		t.Fatalf("ListTasks(active) error = %v", err)
	}
	if _, ok := active.Tasks["TASK-001"]; ok {
		t.Fatal("active task list includes archived task")
	}
	archived, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{Status: "archived"})
	if err != nil {
		t.Fatalf("ListTasks(archived) error = %v", err)
	}
	if len(archived.Tasks) != 1 || archived.Tasks["TASK-001"].Status != "archived" {
		t.Fatalf("archived list = %#v, want TASK-001", archived.Tasks)
	}
	show, err := ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if show.Task.Status != "archived" {
		t.Fatalf("show status = %q, want archived", show.Task.Status)
	}
	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}

	again, err := ArchiveTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskArchiveOptions{Refs: []string{"TASK-001"}})
	if err != nil {
		t.Fatalf("idempotent ArchiveTasks() error = %v", err)
	}
	if len(again.Archived) != 0 || len(again.Skipped) != 1 || again.Skipped[0].Reason != "already archived" {
		t.Fatalf("second ArchiveTasks() = %#v, want already archived skip", again)
	}
}

func TestArchiveTasksBySpecArchivesDoneTasksOnly(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-archive.md", "# Archive Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-done.md", "# Done Task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-todo.md", "# Todo Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Done Task","spec":"SPEC-001","status":"done","priority":"P1"},
  "TASK-002":{"title":"Todo Task","spec":"SPEC-001","status":"todo","priority":"P2"}
}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ArchiveTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskArchiveOptions{Spec: "SPEC-001"})
	if err != nil {
		t.Fatalf("ArchiveTasks(--spec) error = %v", err)
	}
	if result.Spec == nil || result.Spec.Alias != "SPEC-001" {
		t.Fatalf("Spec = %#v, want SPEC-001", result.Spec)
	}
	if len(result.Archived) != 1 || result.Archived[0].Task == nil || result.Archived[0].Task.Alias != "TASK-001" {
		t.Fatalf("Archived = %#v, want TASK-001 only", result.Archived)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("Skipped = %#v, want no skips for --spec selection", result.Skipped)
	}

	empty, err := ArchiveTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskArchiveOptions{Spec: "SPEC-001"})
	if err != nil {
		t.Fatalf("second ArchiveTasks(--spec) error = %v", err)
	}
	if len(empty.Archived) != 0 || len(empty.Skipped) != 0 || empty.Spec == nil {
		t.Fatalf("second --spec result = %#v, want empty result with spec", empty)
	}
}
