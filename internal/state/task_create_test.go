package state

import (
	"context"
	"strings"
	"testing"
)

func TestCreateTaskDefaultsAndIntegratesWithReads(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	result, err := CreateTask(context.Background(), root, PathResolver{StateHome: stateHome}, TaskCreateOptions{Title: "New Task"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if result.Task.Alias != "TASK-001" || result.Task.Title != "New Task" || result.Task.Status != "todo" || result.Priority != "P2" || result.EventID == "" {
		t.Fatalf("result = %#v, want default TASK-001 todo task", result)
	}
	if result.Spec.ID != "" {
		t.Fatalf("Spec = %#v, want empty spec", result.Spec)
	}
	if len(result.Depends) != 0 {
		t.Fatalf("Depends = %#v, want none", result.Depends)
	}

	tasks, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	task := tasks.Tasks["TASK-001"]
	if task.Title != "New Task" || task.Status != "todo" || task.Priority != "P2" {
		t.Fatalf("TASK-001 = %#v, want created task defaults", task)
	}

	show, err := ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if show.Task.Title != "New Task" || show.Task.Priority != "P2" {
		t.Fatalf("show = %#v, want created task details", show)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Alias != "TASK-001" || trace.Entity.Status != "todo" {
		t.Fatalf("trace entity = %#v, want created task", trace.Entity)
	}
}

func TestCreateTaskWithSpecAndDependencies(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Existing Task\n")
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := CreateTask(context.Background(), root, PathResolver{StateHome: stateHome}, TaskCreateOptions{
		Title:     "Follow-up Task",
		Spec:      "SPEC-001",
		Priority:  "P1",
		DependsOn: []string{"TASK-001"},
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if result.Task.Alias != "TASK-002" || result.Task.Title != "Follow-up Task" || result.Priority != "P1" {
		t.Fatalf("result.Task = %#v, want TASK-002 follow-up", result.Task)
	}
	if result.Spec.Alias != "SPEC-001" {
		t.Fatalf("Spec = %#v, want SPEC-001", result.Spec)
	}
	if len(result.Depends) != 1 || result.Depends[0].Alias != "TASK-001" {
		t.Fatalf("Depends = %#v, want TASK-001", result.Depends)
	}

	show, err := ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-002")
	if err != nil {
		t.Fatalf("ShowTask(TASK-002) error = %v", err)
	}
	if show.Task.Spec != "SPEC-001" || show.Task.Priority != "P1" || len(show.Task.DependsOn) != 1 || show.Task.DependsOn[0] != "TASK-001" {
		t.Fatalf("show = %#v, want spec, priority, and dependency aliases", show)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-002")
	if err != nil {
		t.Fatalf("Trace(TASK-002) error = %v", err)
	}
	if !hasRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-001") {
		t.Fatalf("trace relationships = %#v, want implements SPEC-001", trace.Relationships)
	}
	if !hasRelationship(trace.Relationships, "outbound", "blocked_by", "task", "TASK-001") {
		t.Fatalf("trace relationships = %#v, want blocked_by TASK-001", trace.Relationships)
	}
}

func TestCreateTaskValidation(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Existing Task\n")
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	cases := []struct {
		name    string
		options TaskCreateOptions
		want    string
	}{
		{name: "missing title", options: TaskCreateOptions{}, want: "requires --title"},
		{name: "invalid priority", options: TaskCreateOptions{Title: "Bad", Priority: "P9"}, want: "invalid priority"},
		{name: "missing spec", options: TaskCreateOptions{Title: "Bad", Spec: "SPEC-999"}, want: "not found"},
		{name: "missing dependency", options: TaskCreateOptions{Title: "Bad", DependsOn: []string{"TASK-999"}}, want: "not found"},
		{name: "non-task dependency", options: TaskCreateOptions{Title: "Bad", DependsOn: []string{"SPEC-001"}}, want: "not task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTask(context.Background(), root, PathResolver{StateHome: stateHome}, tc.options)
			if err == nil {
				t.Fatal("CreateTask() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func hasRelationship(relationships []TraceRelationship, direction string, relationshipType string, kind string, alias string) bool {
	for _, relationship := range relationships {
		if relationship.Direction == direction && relationship.Type == relationshipType && relationship.Entity.Kind == kind && relationship.Entity.Alias == alias {
			return true
		}
	}
	return false
}
