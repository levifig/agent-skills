package state

import (
	"context"
	"testing"
)

func TestListSpecsReadsImportedSQLiteSpecs(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-002-draft.md", `---
id: SPEC-002
title: Draft Spec
status: drafting
---
# Draft Spec
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-todo.md", "# Todo task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-progress.md", "# Progress task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-003-done.md", "# Done task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-004-review.md", "# Review task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"},
    "TASK-004": {"title": "Review Task", "spec": "SPEC-001", "status": "review", "priority": "P2"}
  }
}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	specs, err := ListSpecs(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListSpecs() error = %v", err)
	}

	spec := specs.Specs["SPEC-001"]
	if spec.Title != "Example Spec" || spec.Status != "implementing" || spec.SourcePath != ".agents/specs/SPEC-001-example.md" {
		t.Fatalf("SPEC-001 = %#v, want imported metadata", spec)
	}
	if spec.Tasks.Todo != 2 || spec.Tasks.InProgress != 1 || spec.Tasks.Done != 1 {
		t.Fatalf("SPEC-001 task counts = %#v, want todo=2 in_progress=1 done=1", spec.Tasks)
	}
	draft := specs.Specs["SPEC-002"]
	if draft.Title != "Draft Spec" || draft.Status != "drafting" {
		t.Fatalf("SPEC-002 = %#v, want imported draft spec", draft)
	}
	if draft.Tasks != (SpecTaskCounts{}) {
		t.Fatalf("SPEC-002 task counts = %#v, want zero counts", draft.Tasks)
	}
}
