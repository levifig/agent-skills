package state

import (
	"context"
	"testing"
)

func TestListReportsReadsImportedSQLiteReports(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "reports/draft.md", `---
title: Draft Report
type: research
status: draft
source: ad-hoc
---
# Draft Report
`)
	writeAgentsFile(t, root.Path(), "reports/final.md", `---
title: Final Report
kind: audit
status: final
source: SPEC-001
---
# Final Report
`)
	writeAgentsFile(t, root.Path(), "reports/archive/old.md", `---
title: Old Report
type: research
status: final
source: old
---
# Old Report
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	reports, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{})
	if err != nil {
		t.Fatalf("ListReports() error = %v", err)
	}

	draft := reports.Reports["draft"]
	if draft.Title != "Draft Report" || draft.Kind != "research" || draft.Status != "draft" || draft.SourcePath != ".agents/reports/draft.md" {
		t.Fatalf("draft report = %#v, want imported metadata", draft)
	}
	final := reports.Reports["final"]
	if final.Title != "Final Report" || final.Kind != "audit" || final.Status != "final" {
		t.Fatalf("final report = %#v, want kind from legacy kind frontmatter", final)
	}
	archived := reports.Reports["old"]
	if archived.Title != "Old Report" || archived.Status != "archived" || archived.SourcePath != ".agents/reports/archive/old.md" {
		t.Fatalf("archived report = %#v, want archive-location status", archived)
	}

	research, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{Type: "research"})
	if err != nil {
		t.Fatalf("ListReports(type) error = %v", err)
	}
	if len(research.Reports) != 2 || research.Reports["draft"].Kind != "research" || research.Reports["old"].Kind != "research" {
		t.Fatalf("research reports = %#v, want draft and archived research reports", research.Reports)
	}

	finalOnly, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{Status: "final"})
	if err != nil {
		t.Fatalf("ListReports(status) error = %v", err)
	}
	if len(finalOnly.Reports) != 1 || finalOnly.Reports["final"].Status != "final" {
		t.Fatalf("final reports = %#v, want only final report", finalOnly.Reports)
	}
}
