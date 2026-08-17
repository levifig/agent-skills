package state

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIssueAbsorbAllProjectsOpenLeftoversAndSkipsHistory(t *testing.T) {
	root, resolver := absorbInitialized(t)
	open, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"})
	if err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	intent, err := CreateIntent(context.Background(), root, resolver, IntentCreateOptions{Title: "Tracked leftover", Body: "Keep this."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	resolved, err := CreateIntent(context.Background(), root, resolver, IntentCreateOptions{Title: "Already decided", Body: "Prior."})
	if err != nil {
		t.Fatalf("CreateIntent(resolved) error = %v", err)
	}
	if _, err := ResolveIntent(context.Background(), root, resolver, IntentDispositionOptions{IntentRef: resolved.Intent.Alias, Reason: "shipped in a prior release"}); err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}

	plan, err := PlanAbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{})
	if err != nil {
		t.Fatalf("PlanAbsorbAll() error = %v", err)
	}
	if plan.Absorbed != 2 || plan.Skipped != 0 || plan.Refused != 0 || len(plan.Items) != 2 {
		t.Fatalf("plan = %#v, want 2 absorb items (open task + tracked intent)", plan)
	}

	result, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{})
	if err != nil {
		t.Fatalf("AbsorbAll() error = %v", err)
	}
	if result.Absorbed != 2 || result.DryRun {
		t.Fatalf("result = %#v, want 2 absorbed", result)
	}
	byTitle := map[string]AbsorbProjectionItem{}
	for _, item := range result.Items {
		if item.Issue == nil {
			t.Fatalf("item = %#v, want minted issue", item)
		}
		byTitle[item.Issue.Title] = item
	}
	openItem := byTitle["Open leftover"]
	if openItem.Issue.Alias != "LOAF-1" || openItem.Issue.Status != IssueStatusTriage {
		t.Fatalf("open issue = %#v, want LOAF-1 triage", openItem.Issue)
	}
	if !strings.Contains(openItem.Issue.Body, "Absorbed from "+open.Task.Alias) || !strings.Contains(openItem.Issue.Body, "task:"+open.Task.ID) {
		t.Fatalf("open body = %q, want task provenance", openItem.Issue.Body)
	}
	intentItem := byTitle["Tracked leftover"]
	if intentItem.Issue.Status != IssueStatusBacklog || !strings.Contains(intentItem.Issue.Body, "intent:"+intent.Intent.ID) {
		t.Fatalf("intent issue = %#v, want backlog with intent provenance", intentItem.Issue)
	}

	shown, err := ShowTask(context.Background(), root, resolver, open.Task.Alias)
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != LifecycleStatusArchived {
		t.Fatalf("open task status = %q, want archived", shown.Task.Status)
	}
	if _, err := GetIssue(context.Background(), root, resolver, open.Task.Alias); err == nil {
		t.Fatal("GetIssue(TASK alias) error = nil, want missing issue")
	}

	again, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{})
	if err != nil {
		t.Fatalf("second AbsorbAll() error = %v", err)
	}
	if again.Absorbed != 0 || len(again.Items) != 0 {
		t.Fatalf("second AbsorbAll() = %#v, want empty (history rows stay omitted)", again)
	}
}

func TestIssueAbsorbAllDryRunWritesNothing(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Stay open"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	result, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{DryRun: true})
	if err != nil {
		t.Fatalf("AbsorbAll(--dry-run) error = %v", err)
	}
	if !result.DryRun || result.Absorbed != 1 || result.Items[0].Issue != nil {
		t.Fatalf("dry-run result = %#v, want one planned absorb and no issue", result)
	}
	listed, err := ListIssues(context.Background(), root, resolver, IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none after dry-run", listed.Issues)
	}
	shown, err := ShowTask(context.Background(), root, resolver, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != LifecycleStatusTodo {
		t.Fatalf("status = %q, want todo after dry-run", shown.Task.Status)
	}
}

func TestIssueAbsorbAllHistoryMapsStatusesAndStaysOffFrontier(t *testing.T) {
	root, resolver := absorbInitialized(t)
	done := createTaskWithStatus(t, root, resolver, "Shipped leftover", LifecycleStatusDone)
	archived := createTaskWithStatus(t, root, resolver, "Archived leftover", LifecycleStatusDone)
	if _, err := ArchiveTasks(context.Background(), root, resolver, TaskArchiveOptions{Refs: []string{archived.Task.Alias}}); err != nil {
		t.Fatalf("ArchiveTasks() error = %v", err)
	}
	resolved, err := CreateIntent(context.Background(), root, resolver, IntentCreateOptions{Title: "Resolved leftover", Body: "Prior."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	if _, err := ResolveIntent(context.Background(), root, resolver, IntentDispositionOptions{IntentRef: resolved.Intent.Alias, Reason: "shipped in a prior release"}); err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}

	result, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{History: true})
	if err != nil {
		t.Fatalf("AbsorbAll(--history) error = %v", err)
	}
	if result.Absorbed != 3 {
		t.Fatalf("result = %#v, want 3 absorbed history rows", result)
	}
	byTitle := map[string]Issue{}
	for _, item := range result.Items {
		if item.Issue == nil {
			t.Fatalf("item = %#v, want minted issue", item)
		}
		byTitle[item.Issue.Title] = *item.Issue
	}
	if byTitle["Shipped leftover"].Status != IssueStatusDone {
		t.Fatalf("done issue status = %q, want done", byTitle["Shipped leftover"].Status)
	}
	if byTitle["Archived leftover"].Status != IssueStatusCancelled || byTitle["Archived leftover"].ArchivedAt == "" {
		t.Fatalf("archived issue = %#v, want cancelled and archived", byTitle["Archived leftover"])
	}
	if byTitle["Resolved leftover"].Status != IssueStatusDone {
		t.Fatalf("resolved intent issue status = %q, want done", byTitle["Resolved leftover"].Status)
	}
	if !strings.Contains(byTitle["Shipped leftover"].Body, "task:"+done.Task.ID) {
		t.Fatalf("done body = %q, want task id provenance", byTitle["Shipped leftover"].Body)
	}

	frontier, err := ListIssueFrontier(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ListIssueFrontier() error = %v", err)
	}
	if len(frontier.Issues) != 0 {
		t.Fatalf("frontier = %#v, want empty for history projection", frontier.Issues)
	}

	again, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{History: true})
	if err != nil {
		t.Fatalf("second AbsorbAll(--history) error = %v", err)
	}
	if again.Absorbed != 0 || again.Skipped != 3 {
		t.Fatalf("second history run = %#v, want 3 skipped", again)
	}
}

func TestIssueAbsorbAllHistoryRefusesIndependentIssues(t *testing.T) {
	root, resolver := absorbInitialized(t)
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Hand-made"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	_, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{History: true})
	var independent *AbsorbHistoryIndependentError
	if err == nil || !errors.As(err, &independent) || independent.Count != 1 || !strings.Contains(err.Error(), "LOAF-47") {
		t.Fatalf("AbsorbAll(--history) error = %v, want typed independent-issue refusal", err)
	}
	listed, err := ListIssues(context.Background(), root, resolver, IssueListOptions{Archived: true})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 1 || listed.Issues[0].Title != "Hand-made" {
		t.Fatalf("issues = %#v, want only the hand-made issue", listed.Issues)
	}
}

func TestIssueAbsorbAllHistoryAllowedAfterOpenProjection(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	if _, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{}); err != nil {
		t.Fatalf("AbsorbAll() error = %v", err)
	}
	result, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{History: true})
	if err != nil {
		t.Fatalf("AbsorbAll(--history) after open projection error = %v", err)
	}
	if result.Absorbed != 1 || result.Skipped != 1 {
		t.Fatalf("history after open = %#v, want 1 absorbed done and 1 skipped open", result)
	}
	var minted *Issue
	for _, item := range result.Items {
		if item.Action == AbsorbActionAbsorb {
			minted = item.Issue
		}
	}
	if minted == nil || minted.Title != "Done leftover" || minted.Status != IssueStatusDone {
		t.Fatalf("history issue = %#v, want Done leftover as done", minted)
	}
}

func TestIssueAbsorbAllDismissesOpenWithoutMinting(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Drop me"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	result, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{Dismiss: true})
	if err != nil {
		t.Fatalf("AbsorbAll(--dismiss) error = %v", err)
	}
	if result.Dismissed != 1 || result.Absorbed != 0 {
		t.Fatalf("result = %#v, want one dismiss", result)
	}
	if result.Items[0].Issue != nil {
		t.Fatalf("dismiss item = %#v, want no issue", result.Items[0])
	}
	listed, err := ListIssues(context.Background(), root, resolver, IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", listed.Issues)
	}
}

func TestIssueAbsorbAllRefusesChangeLocalAndKeepsSlugProvenance(t *testing.T) {
	root, resolver := absorbInitialized(t)
	created, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Docs revision"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	local, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Change local"})
	if err != nil {
		t.Fatalf("CreateTask(local) error = %v", err)
	}
	store, err := OpenStore(created.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE aliases SET alias = ? WHERE project_id = ? AND entity_kind = 'task' AND entity_id = ?`, "20260712-docs-revision-progress", created.ProjectID, created.Task.ID); err != nil {
		store.Close()
		t.Fatalf("retarget task alias: %v", err)
	}
	attachTaskBodySource(t, store, local.ProjectID, local.Task.ID, "docs/changes/example/tasks/TASK-001-example.md")
	store.Close()

	result, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{})
	if err != nil {
		t.Fatalf("AbsorbAll() error = %v", err)
	}
	if result.Absorbed != 1 || result.Refused != 1 {
		t.Fatalf("result = %#v, want one absorb and one refuse", result)
	}
	var absorbed AbsorbProjectionItem
	for _, item := range result.Items {
		if item.Action == AbsorbActionAbsorb {
			absorbed = item
		}
		if item.Action == AbsorbActionRefuse && !strings.Contains(item.Reason, "change-local") {
			t.Fatalf("refuse = %#v, want change-local reason", item)
		}
	}
	if absorbed.Issue == nil || !strings.Contains(absorbed.Issue.Body, "Absorbed from 20260712-docs-revision-progress") {
		t.Fatalf("slug issue = %#v, want slug provenance", absorbed.Issue)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "20260712-docs-revision-progress"); err == nil {
		t.Fatal("GetIssue(slug) error = nil, want missing issue")
	}
}

func TestIssueAbsorbHistorySingleRefStillRefusesDone(t *testing.T) {
	root, resolver := absorbInitialized(t)
	created := createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	_, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: created.Task.Alias})
	if err == nil || !strings.Contains(err.Error(), "not leftover open work") {
		t.Fatalf("single-ref Absorb(done) error = %v, want leftover-open refusal", err)
	}
}

func TestReportLeftoverAbsorbNamesOpenAndProjectableHistory(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	if _, err := CreateIntent(context.Background(), root, resolver, IntentCreateOptions{Title: "Tracked leftover", Body: "Keep this."}); err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}

	report, err := ReportLeftoverAbsorb(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ReportLeftoverAbsorb() error = %v", err)
	}
	if report.OpenAbsorb != 2 || report.OpenRefuse != 0 || report.HistoryAbsorb != 1 || report.HistoryFrozen {
		t.Fatalf("report = %#v, want 2 open absorbs and 1 projectable history row", report)
	}
}

func TestReportLeftoverAbsorbFreezesHistoryWithoutLoadingMintPath(t *testing.T) {
	root, resolver := absorbInitialized(t)
	createTaskWithStatus(t, root, resolver, "Done leftover", LifecycleStatusDone)
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Hand-made"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	report, err := ReportLeftoverAbsorb(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ReportLeftoverAbsorb() error = %v", err)
	}
	if !report.HistoryFrozen || report.IndependentIssues != 1 || report.FrozenHistory != 1 || report.OpenActionable() != 0 || report.HistoryActionable() != 0 {
		t.Fatalf("report = %#v, want frozen history with one leftover row", report)
	}
}

func TestReportLeftoverAbsorbSkipsAlreadyRepresentedOpenWork(t *testing.T) {
	root, resolver := absorbInitialized(t)
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := AbsorbAll(context.Background(), root, resolver, AbsorbAllOptions{}); err != nil {
		t.Fatalf("AbsorbAll() error = %v", err)
	}

	report, err := ReportLeftoverAbsorb(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("ReportLeftoverAbsorb() error = %v", err)
	}
	if report.OpenActionable() != 0 || report.HistoryFrozen || report.HistoryActionable() != 0 {
		t.Fatalf("report = %#v, want empty after open absorb", report)
	}
}

func TestLeftoverAbsorbUnavailableForUninitializedAndUnregistered(t *testing.T) {
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	_, err := ReportLeftoverAbsorb(context.Background(), root, resolver)
	if !LeftoverAbsorbUnavailable(err) {
		t.Fatalf("uninitialized error = %v, want leftover-unavailable", err)
	}

	registered, registeredResolver := absorbInitialized(t)
	other := projectRoot(t)
	_, err = ReportLeftoverAbsorb(context.Background(), other, registeredResolver)
	if !LeftoverAbsorbUnavailable(err) {
		t.Fatalf("unregistered error = %v, want leftover-unavailable", err)
	}
	_ = registered
}
