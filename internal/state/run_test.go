package state

import (
	"context"
	"strings"
	"testing"
)

func TestRunLifecycleAndReportRelationship(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	report, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "run-report", Kind: "audit"})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	created, err := CreateRun(context.Background(), root, PathResolver{StateHome: stateHome}, RunCreateOptions{
		GeneratorRef:     "pipeline/findings",
		GeneratorVersion: "v1",
		GeneratorHash:    "hash-run",
		Status:           "running",
		Metadata:         `{"kind":"fixture"}`,
		Report:           report.Report.Alias,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if !strings.HasPrefix(created.Run.Alias, "RUN-") || created.Run.Status != "running" || created.Run.GeneratorRef != "pipeline/findings" {
		t.Fatalf("created run = %#v, want running provenance run", created.Run)
	}
	if len(created.Run.Relationships) != 1 || created.Run.Relationships[0].Entity.Alias != report.Report.Alias {
		t.Fatalf("run relationships = %#v, want report relationship", created.Run.Relationships)
	}

	running, err := ListRuns(context.Background(), root, PathResolver{StateHome: stateHome}, RunListOptions{Status: "running", Generator: "pipeline/findings"})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(running.Runs) != 1 {
		t.Fatalf("running runs = %#v, want one", running.Runs)
	}

	completed, err := CompleteRun(context.Background(), root, PathResolver{StateHome: stateHome}, RunCompleteOptions{Run: created.Run.Alias})
	if err != nil {
		t.Fatalf("CompleteRun() error = %v", err)
	}
	if completed.Previous != "running" || completed.Status != "completed" || completed.Run.CompletedAt == "" {
		t.Fatalf("completed run = %#v, want completed transition", completed)
	}
	show, err := ShowRun(context.Background(), root, PathResolver{StateHome: stateHome}, created.Run.Alias)
	if err != nil {
		t.Fatalf("ShowRun() error = %v", err)
	}
	if show.Run.Status != "completed" || show.Run.Metadata != `{"kind":"fixture"}` {
		t.Fatalf("show run = %#v, want completed run preserving metadata", show.Run)
	}
}

func TestRunValidationRejectsInvalidStatusAndMetadata(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateRun(context.Background(), root, PathResolver{StateHome: stateHome}, RunCreateOptions{GeneratorRef: "pipeline/findings", Status: "started"}); err == nil || !strings.Contains(err.Error(), "invalid run status") {
		t.Fatalf("CreateRun(invalid status) error = %v, want invalid status", err)
	}
	if _, err := CreateRun(context.Background(), root, PathResolver{StateHome: stateHome}, RunCreateOptions{GeneratorRef: "pipeline/findings", Metadata: "{bad"}); err == nil || !strings.Contains(err.Error(), "run metadata must be valid JSON") {
		t.Fatalf("CreateRun(invalid metadata) error = %v, want metadata error", err)
	}
}
