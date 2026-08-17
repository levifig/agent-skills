package state

import (
	"context"
	"testing"
)

func TestInspectOmitsLeftoverAbsorbOnHotPath(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertNoDiagnostic(t, status.Diagnostics, LeftoverOpenWorkCode)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverHistoryWorkCode)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverHistoryFrozenCode)
}

func TestInspectLeftoverAbsorbNamesOpenWork(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	status, err := InspectWithOptions(root, resolver, InspectOptions{LeftoverAbsorb: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, LeftoverOpenWorkCode, RepairCategoryLeftoverAbsorb, DiagnosticPolicyImportPending, false)
	assertDiagnosticDetail(t, status.Diagnostics, LeftoverOpenWorkCode, "preview_command", LeftoverAbsorbPreviewCommand)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverHistoryWorkCode)

	action := findRepairAction(t, RepairPlanForStatus(status), "preview-leftover-absorb")
	if action.Safe || action.Applied || action.Command != LeftoverAbsorbPreviewCommand {
		t.Fatalf("repair action = %#v, want manual leftover absorb preview", action)
	}
}

func TestInspectLeftoverAbsorbNamesProjectableHistory(t *testing.T) {
	root, resolver := absorbInitialized(t)
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)

	status, err := InspectWithOptions(root, resolver, InspectOptions{LeftoverAbsorb: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, LeftoverHistoryWorkCode, RepairCategoryLeftoverAbsorb, DiagnosticPolicyImportPending, false)
	assertDiagnosticDetail(t, status.Diagnostics, LeftoverHistoryWorkCode, "preview_command", LeftoverAbsorbHistoryPreviewCommand)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverOpenWorkCode)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverHistoryFrozenCode)

	action := findRepairAction(t, RepairPlanForStatus(status), "preview-leftover-absorb-history")
	if action.Safe || action.Command != LeftoverAbsorbHistoryPreviewCommand {
		t.Fatalf("repair action = %#v, want manual history preview", action)
	}
}

func TestInspectLeftoverAbsorbNamesFrozenHistoryWithoutRepair(t *testing.T) {
	root, resolver := absorbInitialized(t)
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Hand-made"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	status, err := InspectWithOptions(root, resolver, InspectOptions{LeftoverAbsorb: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	assertDiagnostic(t, status.Diagnostics, LeftoverHistoryFrozenCode)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverHistoryWorkCode)
	assertNoDiagnostic(t, status.Diagnostics, LeftoverOpenWorkCode)

	for _, action := range RepairPlanForStatus(status) {
		if action.Code == "preview-leftover-absorb-history" || action.Code == "preview-leftover-absorb" {
			t.Fatalf("frozen history grew a repair action: %#v", action)
		}
	}
}
