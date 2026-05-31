package state

import (
	"context"
	"testing"
)

func TestListTasksReadsImportedSQLiteTasks(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Task body\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-done.md", "# Done task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "Example Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1",
      "depends_on": ["TASK-000"]
    },
    "TASK-002": {
      "title": "Done Task",
      "spec": "SPEC-001",
      "status": "done",
      "priority": "P2",
      "depends_on": []
    }
  }
}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	tasks, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}

	task := tasks.Tasks["TASK-001"]
	if task.Title != "Example Task" || task.Status != "todo" || task.Priority != "P1" || task.Spec != "SPEC-001" {
		t.Fatalf("TASK-001 = %#v, want imported metadata", task)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "TASK-000" {
		t.Fatalf("TASK-001 DependsOn = %#v, want TASK-000", task.DependsOn)
	}
	if task.SourcePath != ".agents/tasks/TASK-001-example.md" {
		t.Fatalf("TASK-001 SourcePath = %q, want task source", task.SourcePath)
	}
	doneTask, ok := tasks.Tasks["TASK-002"]
	if !ok {
		t.Fatal("TASK-002 missing from unfiltered list")
	}
	if doneTask.DependsOn == nil || len(doneTask.DependsOn) != 0 {
		t.Fatalf("TASK-002 DependsOn = %#v, want empty dependency list", doneTask.DependsOn)
	}
}

func TestListTasksFiltersActiveAndStatus(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Task body\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-done.md", "# Done task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	active, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{Active: true})
	if err != nil {
		t.Fatalf("ListTasks(active) error = %v", err)
	}
	if _, ok := active.Tasks["TASK-002"]; ok {
		t.Fatal("active list includes done task")
	}

	done, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{Status: "done"})
	if err != nil {
		t.Fatalf("ListTasks(done) error = %v", err)
	}
	if len(done.Tasks) != 1 || done.Tasks["TASK-002"].Status != "done" {
		t.Fatalf("done list = %#v, want only TASK-002", done.Tasks)
	}

	activeDone, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{Active: true, Status: "done"})
	if err != nil {
		t.Fatalf("ListTasks(active done) error = %v", err)
	}
	if len(activeDone.Tasks) != 0 {
		t.Fatalf("active done list = %#v, want empty", activeDone.Tasks)
	}
}
