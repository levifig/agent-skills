package state

import (
	"context"
	"testing"
)

func TestSetSpecStatusTransitionsDraftToImplementingToComplete(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-draft.md", `---
id: SPEC-001
title: Draft Spec
status: draft
---
# Draft Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	resolver := PathResolver{StateHome: stateHome}

	// draft -> implementing (legacy spelling canonicalizes to in_progress)
	first, err := SetSpecStatus(context.Background(), root, resolver, "SPEC-001", "implementing")
	if err != nil {
		t.Fatalf("SetSpecStatus(implementing) error = %v", err)
	}
	if first.Previous != LifecycleStatusDraft || first.Status != LifecycleStatusInProgress {
		t.Fatalf("first transition = %s -> %s, want draft -> in_progress", first.Previous, first.Status)
	}
	if first.EventID == "" {
		t.Fatal("first transition missing event id")
	}
	if first.Spec == nil || first.Spec.Alias != "SPEC-001" || first.Spec.Status != LifecycleStatusInProgress {
		t.Fatalf("first transition spec = %#v, want SPEC-001 in_progress", first.Spec)
	}
	if got := rawLifecycleEventFromStatus(t, first.DatabasePath, first.EventID); got != LifecycleStatusDraft {
		t.Fatalf("first event from_status = %q, want draft", got)
	}
	if got := rawLifecycleEventToStatus(t, first.DatabasePath, first.EventID); got != LifecycleStatusInProgress {
		t.Fatalf("first event to_status = %q, want in_progress", got)
	}

	// implementing -> complete (legacy spelling canonicalizes to done)
	second, err := SetSpecStatus(context.Background(), root, resolver, "SPEC-001", "complete")
	if err != nil {
		t.Fatalf("SetSpecStatus(complete) error = %v", err)
	}
	if second.Previous != LifecycleStatusInProgress || second.Status != LifecycleStatusDone {
		t.Fatalf("second transition = %s -> %s, want in_progress -> done", second.Previous, second.Status)
	}
	if second.EventID == "" {
		t.Fatal("second transition missing event id")
	}

	// show reflects the new status
	show, err := ShowSpec(context.Background(), root, resolver, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Status != LifecycleStatusDone {
		t.Fatalf("ShowSpec status = %q, want done", show.Spec.Status)
	}

	list, err := ListSpecs(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ListSpecs() error = %v", err)
	}
	if list.Specs["SPEC-001"].Status != LifecycleStatusDone {
		t.Fatalf("ListSpecs status = %q, want done", list.Specs["SPEC-001"].Status)
	}
}

func TestSetSpecStatusAcceptsCanonicalStatus(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-draft.md", `---
id: SPEC-001
title: Draft Spec
status: draft
---
# Draft Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := SetSpecStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001", LifecycleStatusInProgress)
	if err != nil {
		t.Fatalf("SetSpecStatus(in_progress) error = %v", err)
	}
	if result.Status != LifecycleStatusInProgress {
		t.Fatalf("status = %q, want in_progress", result.Status)
	}
}

func TestSetSpecStatusRejectsInvalidStatus(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-draft.md", `---
id: SPEC-001
title: Draft Spec
status: draft
---
# Draft Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err := SetSpecStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001", "bogus")
	if err == nil {
		t.Fatal("SetSpecStatus(bogus) expected error, got nil")
	}

	// status unchanged after rejection
	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Status != LifecycleStatusDraft {
		t.Fatalf("ShowSpec status = %q, want draft (unchanged)", show.Spec.Status)
	}
}

func TestSetSpecStatusRejectsNonSpecRef(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-task.md", "# Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Task","status":"todo","priority":"P1"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err := SetSpecStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001", LifecycleStatusDone)
	if err == nil {
		t.Fatal("SetSpecStatus(TASK-001) expected error, got nil")
	}
}
