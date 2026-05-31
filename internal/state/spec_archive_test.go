package state

import (
	"context"
	"testing"
)

func TestArchiveSpecsArchivesCompleteSpecsAndRecordsEvent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-complete.md", `---
id: SPEC-001
title: Complete Spec
status: complete
---
# Complete Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ArchiveSpecs(context.Background(), root, PathResolver{StateHome: stateHome}, []string{"SPEC-001"})
	if err != nil {
		t.Fatalf("ArchiveSpecs() error = %v", err)
	}
	if len(result.Archived) != 1 || result.Archived[0].Spec == nil || result.Archived[0].Spec.Alias != "SPEC-001" || result.Archived[0].Previous != "complete" || result.Archived[0].Status != "archived" || result.Archived[0].EventID == "" {
		t.Fatalf("Archived = %#v, want SPEC-001 archived with event", result.Archived)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("Skipped = %#v, want none", result.Skipped)
	}

	specs, err := ListSpecs(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListSpecs() error = %v", err)
	}
	if specs.Specs["SPEC-001"].Status != "archived" {
		t.Fatalf("SPEC-001 status = %q, want archived", specs.Specs["SPEC-001"].Status)
	}
	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}

	again, err := ArchiveSpecs(context.Background(), root, PathResolver{StateHome: stateHome}, []string{"SPEC-001"})
	if err != nil {
		t.Fatalf("idempotent ArchiveSpecs() error = %v", err)
	}
	if len(again.Archived) != 0 || len(again.Skipped) != 1 || again.Skipped[0].Reason != "already archived" {
		t.Fatalf("second ArchiveSpecs() = %#v, want already archived skip", again)
	}
}

func TestArchiveSpecsSkipsUnarchiveableRefs(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-draft.md", `---
id: SPEC-001
title: Draft Spec
status: drafting
---
# Draft Spec
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-002-complete.md", `---
id: SPEC-002
title: Complete Spec
status: complete
---
# Complete Spec
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-task.md", "# Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Task","status":"todo","priority":"P1"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ArchiveSpecs(context.Background(), root, PathResolver{StateHome: stateHome}, []string{"SPEC-001", "TASK-001", "SPEC-999", "SPEC-002"})
	if err != nil {
		t.Fatalf("ArchiveSpecs() error = %v", err)
	}
	if len(result.Archived) != 1 || result.Archived[0].Spec == nil || result.Archived[0].Spec.Alias != "SPEC-002" {
		t.Fatalf("Archived = %#v, want only SPEC-002", result.Archived)
	}
	if len(result.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want draft, wrong-kind, and missing refs", result.Skipped)
	}
	reasons := map[string]string{}
	for _, skipped := range result.Skipped {
		reasons[skipped.Ref] = skipped.Reason
	}
	if reasons["SPEC-001"] != "status is drafting, must be complete" {
		t.Fatalf("SPEC-001 reason = %q, want status skip", reasons["SPEC-001"])
	}
	if reasons["TASK-001"] != `"TASK-001" resolves to task, not spec` {
		t.Fatalf("TASK-001 reason = %q, want wrong-kind skip", reasons["TASK-001"])
	}
	if reasons["SPEC-999"] == "" {
		t.Fatalf("SPEC-999 reason missing in %#v", result.Skipped)
	}
}
