package state

import (
	"context"
	"strings"
	"testing"
)

func TestShowSpecReadsImportedSQLiteSpec(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec

Imported spec prose.
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-todo.md", "# Todo task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-progress.md", "# Progress task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-003-done.md", "# Done task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}

	spec := result.Spec
	if result.Query != "SPEC-001" {
		t.Fatalf("Query = %q, want SPEC-001", result.Query)
	}
	if spec.Alias != "SPEC-001" || spec.Title != "Example Spec" || spec.Status != "implementing" {
		t.Fatalf("Spec = %#v, want imported spec metadata", spec)
	}
	if spec.Tasks.Todo != 1 || spec.Tasks.InProgress != 1 || spec.Tasks.Done != 1 {
		t.Fatalf("Tasks = %#v, want one per status bucket", spec.Tasks)
	}
	if len(spec.Sources) != 1 || spec.Sources[0].Path != ".agents/specs/SPEC-001-example.md" || spec.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want spec source with hash", spec.Sources)
	}
	if !strings.Contains(spec.Body, "# Example Spec") || !strings.Contains(spec.Body, "Imported spec prose.") {
		t.Fatalf("Body = %q, want imported source body without frontmatter", spec.Body)
	}
	if strings.Contains(spec.Body, "status: implementing") || strings.Contains(spec.Body, "---") {
		t.Fatalf("Body = %q, want frontmatter stripped", spec.Body)
	}
	if !hasStateTraceRelationship(spec.Relationships, "inbound", "implements", "task", "TASK-001") {
		t.Fatalf("Relationships = %#v, want inbound task implements relationship", spec.Relationships)
	}
	if spec.CreatedAt == "" || spec.UpdatedAt == "" {
		t.Fatalf("CreatedAt/UpdatedAt = %q/%q, want timestamps", spec.CreatedAt, spec.UpdatedAt)
	}
}

func TestShowSpecRejectsNonSpecReference(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-example.md", "# Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Example Task","status":"todo"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err == nil {
		t.Fatal("ShowSpec() error = nil, want non-spec rejection")
	}
	if !strings.Contains(err.Error(), "not spec") {
		t.Fatalf("error = %v, want non-spec rejection", err)
	}
}
