package state

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestReportLifecycleCreatesFinalizesAndArchivesReport(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:   "release-readiness",
		Kind:   "audit",
		Source: "manual",
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if created.Report.Alias != "report-release-readiness" || created.Report.Title != "Release Readiness" || created.Report.Status != "draft" || created.Kind != "audit" || created.Source != "manual" || created.EventID == "" {
		t.Fatalf("created = %#v, want draft report metadata", created)
	}
	assertReportProjectContext(t, root, created.ContractVersion, created.DatabaseScope, created.DatabasePath, created.ProjectID, created.ProjectName, created.ProjectCurrentPath)
	assertReportEvent(t, root, stateHome, created.Report.ID, "", "draft", "source=manual")

	reports, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{})
	if err != nil {
		t.Fatalf("ListReports() error = %v", err)
	}
	assertReportProjectContext(t, root, reports.ContractVersion, reports.DatabaseScope, reports.DatabasePath, reports.ProjectID, reports.ProjectName, reports.ProjectCurrentPath)
	report := reports.Reports["report-release-readiness"]
	if report.Title != "Release Readiness" || report.Kind != "audit" || report.Status != "draft" {
		t.Fatalf("report = %#v, want created draft report", report)
	}

	finalized, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-release-readiness")
	if err != nil {
		t.Fatalf("FinalizeReport() error = %v", err)
	}
	if finalized.Report.Alias != "report-release-readiness" || finalized.Previous != "draft" || finalized.Status != "final" || finalized.EventID == "" {
		t.Fatalf("finalized = %#v, want final transition", finalized)
	}
	assertReportProjectContext(t, root, finalized.ContractVersion, finalized.DatabaseScope, finalized.DatabasePath, finalized.ProjectID, finalized.ProjectName, finalized.ProjectCurrentPath)
	assertReportEvent(t, root, stateHome, created.Report.ID, "draft", "final", "recorded by report finalize")

	archived, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-release-readiness")
	if err != nil {
		t.Fatalf("ArchiveReport() error = %v", err)
	}
	if archived.Report.Alias != "report-release-readiness" || archived.Previous != "final" || archived.Status != "archived" || archived.EventID == "" {
		t.Fatalf("archived = %#v, want archived transition", archived)
	}
	assertReportProjectContext(t, root, archived.ContractVersion, archived.DatabaseScope, archived.DatabasePath, archived.ProjectID, archived.ProjectName, archived.ProjectCurrentPath)
	assertReportEvent(t, root, stateHome, created.Report.ID, "final", "archived", "recorded by report archive")

	archivedReports, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{Status: "archived"})
	if err != nil {
		t.Fatalf("ListReports(archived) error = %v", err)
	}
	assertReportProjectContext(t, root, archivedReports.ContractVersion, archivedReports.DatabaseScope, archivedReports.DatabasePath, archivedReports.ProjectID, archivedReports.ProjectName, archivedReports.ProjectCurrentPath)
	if archivedReports.Reports["report-release-readiness"].Status != "archived" {
		t.Fatalf("archived reports = %#v, want archived report", archivedReports.Reports)
	}
}

func TestReportLifecycleRejectsInvalidTransitionsWithoutMutation(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "transition-check"}); err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err == nil || !strings.Contains(err.Error(), "not final") {
		t.Fatalf("ArchiveReport(draft) error = %v, want not final", err)
	}
	assertReportStatus(t, root, stateHome, "report-transition-check", "draft")

	if _, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err != nil {
		t.Fatalf("FinalizeReport() error = %v", err)
	}
	if _, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err == nil || !strings.Contains(err.Error(), "not draft") {
		t.Fatalf("FinalizeReport(final) error = %v, want not draft", err)
	}
	assertReportStatus(t, root, stateHome, "report-transition-check", "final")

	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err != nil {
		t.Fatalf("ArchiveReport() error = %v", err)
	}
	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err == nil || !strings.Contains(err.Error(), "not final") {
		t.Fatalf("ArchiveReport(archived) error = %v, want not final", err)
	}
	assertReportStatus(t, root, stateHome, "report-transition-check", "archived")
}

func TestReportLifecycleValidationAndAliasCollisions(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "../"}); err == nil || !strings.Contains(err.Error(), "requires a slug") {
		t.Fatalf("CreateReport(empty slug) error = %v, want slug validation", err)
	}

	first, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "../notes"})
	if err != nil {
		t.Fatalf("CreateReport(notes) error = %v", err)
	}
	second, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "notes"})
	if err != nil {
		t.Fatalf("CreateReport(notes duplicate) error = %v", err)
	}
	if first.Report.Alias != "report-notes" || second.Report.Alias != "report-notes-2" {
		t.Fatalf("aliases = %q, %q; want collision-safe report aliases", first.Report.Alias, second.Report.Alias)
	}
	assertReportProjectContext(t, root, first.ContractVersion, first.DatabaseScope, first.DatabasePath, first.ProjectID, first.ProjectName, first.ProjectCurrentPath)
	assertReportProjectContext(t, root, second.ContractVersion, second.DatabaseScope, second.DatabasePath, second.ProjectID, second.ProjectName, second.ProjectCurrentPath)
}

func assertReportProjectContext(t *testing.T, root project.Root, contractVersion int, databaseScope string, databasePath string, projectID string, projectName string, projectCurrentPath string) {
	t.Helper()
	if contractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", contractVersion, StateJSONContractVersion)
	}
	if databaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", databaseScope)
	}
	if databasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if projectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if projectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", projectName, filepath.Base(root.Path()))
	}
	if projectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", projectCurrentPath, root.Path())
	}
}

func assertReportStatus(t *testing.T, root project.Root, stateHome string, alias string, want string) {
	t.Helper()
	reports, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{})
	if err != nil {
		t.Fatalf("ListReports() error = %v", err)
	}
	got := reports.Reports[alias].Status
	if got != want {
		t.Fatalf("%s status = %q, want %q", alias, got, want)
	}
}

func assertReportEvent(t *testing.T, root project.Root, stateHome string, reportID string, fromStatus string, toStatus string, notePart string) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	var note string
	var count int
	err := store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*), COALESCE(MAX(note), '')
FROM events
WHERE project_id = ? AND entity_kind = 'report' AND entity_id = ? AND COALESCE(from_status, '') = ? AND to_status = ?
`, projectIDForTest(t, store, root), reportID, fromStatus, toStatus).Scan(&count, &note)
	if err != nil {
		t.Fatalf("read report event error = %v", err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1 for %s -> %s", count, fromStatus, toStatus)
	}
	if !strings.Contains(note, notePart) {
		t.Fatalf("event note = %q, want containing %q", note, notePart)
	}
}
