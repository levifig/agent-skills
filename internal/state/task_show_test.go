package state

import (
	"context"
	"strings"
	"testing"
)

func TestShowTaskReadsImportedSQLiteTask(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), `---
id: TASK-001
title: Frontmatter Task
status: todo
---
# Task body

Imported task prose.
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}

	task := result.Task
	if result.Query != "TASK-001" {
		t.Fatalf("Query = %q, want TASK-001", result.Query)
	}
	if task.Alias != "TASK-001" || task.Title != "Example Task" || task.Status != "todo" || task.Priority != "P1" || task.Spec != "SPEC-001" {
		t.Fatalf("Task = %#v, want imported task metadata", task)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "TASK-000" {
		t.Fatalf("DependsOn = %#v, want TASK-000", task.DependsOn)
	}
	if len(task.Sources) != 1 || task.Sources[0].Path != ".agents/tasks/TASK-001-example.md" || task.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want task source with hash", task.Sources)
	}
	if !strings.Contains(task.Body, "# Task body") || !strings.Contains(task.Body, "Imported task prose.") {
		t.Fatalf("Body = %q, want imported source body without frontmatter", task.Body)
	}
	if strings.Contains(task.Body, "Frontmatter Task") || strings.Contains(task.Body, "---") {
		t.Fatalf("Body = %q, want frontmatter stripped", task.Body)
	}
	if task.CreatedAt == "" || task.UpdatedAt == "" {
		t.Fatalf("CreatedAt/UpdatedAt = %q/%q, want timestamps", task.CreatedAt, task.UpdatedAt)
	}
}

func TestShowTaskRejectsMissingAndNonTaskTargets(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Task body\n")
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err := ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err == nil {
		t.Fatal("ShowTask(SPEC-001) error = nil, want non-task rejection")
	}
	if !strings.Contains(err.Error(), "not task") {
		t.Fatalf("error = %v, want non-task rejection", err)
	}

	_, err = ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-999")
	if err == nil {
		t.Fatal("ShowTask(TASK-999) error = nil, want missing-target rejection")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
}
