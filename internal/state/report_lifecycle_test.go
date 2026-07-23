package state

import (
	"context"
	"os"
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
	if finalized.Report.Alias != "report-release-readiness" || finalized.Previous != "draft" || finalized.Status != "done" || finalized.EventID == "" {
		t.Fatalf("finalized = %#v, want done transition", finalized)
	}
	assertReportProjectContext(t, root, finalized.ContractVersion, finalized.DatabaseScope, finalized.DatabasePath, finalized.ProjectID, finalized.ProjectName, finalized.ProjectCurrentPath)
	assertReportEvent(t, root, stateHome, created.Report.ID, "draft", "done", "recorded by report finalize")

	archived, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-release-readiness")
	if err != nil {
		t.Fatalf("ArchiveReport() error = %v", err)
	}
	if archived.Report.Alias != "report-release-readiness" || archived.Previous != "done" || archived.Status != "archived" || archived.EventID == "" {
		t.Fatalf("archived = %#v, want archived transition", archived)
	}
	assertReportProjectContext(t, root, archived.ContractVersion, archived.DatabaseScope, archived.DatabasePath, archived.ProjectID, archived.ProjectName, archived.ProjectCurrentPath)
	assertReportEvent(t, root, stateHome, created.Report.ID, "done", "archived", "recorded by report archive")

	archivedReports, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{Status: "archived"})
	if err != nil {
		t.Fatalf("ListReports(archived) error = %v", err)
	}
	assertReportProjectContext(t, root, archivedReports.ContractVersion, archivedReports.DatabaseScope, archivedReports.DatabasePath, archivedReports.ProjectID, archivedReports.ProjectName, archivedReports.ProjectCurrentPath)
	if archivedReports.Reports["report-release-readiness"].Status != "archived" {
		t.Fatalf("archived reports = %#v, want archived report", archivedReports.Reports)
	}
}

func TestReportShowReadsSQLiteBody(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "body-roundtrip",
		Kind:    "audit",
		Source:  "manual",
		Body:    "# Body Roundtrip\n\nSQLite report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	show, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if show.Report.Body != "# Body Roundtrip\n\nSQLite report body." {
		t.Fatalf("Body = %q, want SQLite report body", show.Report.Body)
	}
	if show.Report.Kind != "audit" || show.Report.Status != "draft" || show.Report.Alias != "report-body-roundtrip" {
		t.Fatalf("Report = %#v, want created report metadata", show.Report)
	}
	assertReportProjectContext(t, root, show.ContractVersion, show.DatabaseScope, show.DatabasePath, show.ProjectID, show.ProjectName, show.ProjectCurrentPath)
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

	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err == nil || !strings.Contains(err.Error(), "not done") {
		t.Fatalf("ArchiveReport(draft) error = %v, want not done", err)
	}
	assertReportStatus(t, root, stateHome, "report-transition-check", "draft")

	if _, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err != nil {
		t.Fatalf("FinalizeReport() error = %v", err)
	}
	// Finalize of a done report is idempotent, not a rejected transition.
	refinalized, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check")
	if err != nil {
		t.Fatalf("FinalizeReport(done) error = %v, want idempotent success", err)
	}
	if refinalized.Previous != "done" || refinalized.Status != "done" || refinalized.EventID != "" {
		t.Fatalf("FinalizeReport(done) = %#v, want done result without a new event", refinalized)
	}
	assertReportStatus(t, root, stateHome, "report-transition-check", "done")

	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err != nil {
		t.Fatalf("ArchiveReport() error = %v", err)
	}
	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-transition-check"); err == nil || !strings.Contains(err.Error(), "not done") {
		t.Fatalf("ArchiveReport(archived) error = %v, want not done", err)
	}
	assertReportStatus(t, root, stateHome, "report-transition-check", "archived")
}

func TestEditReportBodyUpdatesBodyAndRoundTrips(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "edit-roundtrip",
		Kind:    "audit",
		Body:    "Original report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	edited, err := EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:  created.Report.Alias,
		Body: "Edited report body.",
	})
	if err != nil {
		t.Fatalf("EditReportBody() error = %v", err)
	}
	if edited.Report.Alias != "report-edit-roundtrip" || edited.Imported || edited.EventID == "" || edited.ContentHash == "" {
		t.Fatalf("edited = %#v, want non-imported edit with event id and content hash", edited)
	}
	assertBodyEventCount(t, root, stateHome, "report", created.Report.ID, "body_edited", 1)

	show, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if show.Report.Body != "Edited report body." || !show.Report.HasBody {
		t.Fatalf("Report = %#v, want edited body with HasBody", show.Report)
	}

	render, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{
		Kind: "report",
		Ref:  created.Report.Alias,
	})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact() error = %v", err)
	}
	content, err := os.ReadFile(render.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", render.Path, err)
	}
	rerendered, err := ReRenderDurableRender(string(content))
	if err != nil {
		t.Fatalf("ReRenderDurableRender() error = %v", err)
	}
	if rerendered != string(content) {
		t.Fatalf("re-render diverged from finalized file:\nfile:\n%q\nre-render:\n%q", content, rerendered)
	}
	doc, err := ParseDurableRender(string(content))
	if err != nil {
		t.Fatalf("ParseDurableRender() error = %v", err)
	}
	if doc.Body != "Edited report body." {
		t.Fatalf("parsed body = %q, want edited report body", doc.Body)
	}
}

func TestEditReportBodyStripsFullDurableRenderInput(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "render-strip",
		Kind:    "audit",
		Body:    "Original report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	// The render-drift remediation names the committed render itself as the
	// --body-file path, so the edit input is a complete durable render.
	render, err := RenderDurableDocument(DurableRenderDocument{
		Kind: "report",
		Fields: []DurableRenderField{
			{Key: "id", Value: created.Report.Alias},
			{Key: "report_kind", Value: "audit"},
			{Key: "status", Value: "draft"},
			{Key: "title", Value: "Render Strip"},
		},
		Body: "Inner prose from the committed render.",
	})
	if err != nil {
		t.Fatalf("RenderDurableDocument() error = %v", err)
	}

	if _, err := EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:  created.Report.Alias,
		Body: render,
	}); err != nil {
		t.Fatalf("EditReportBody() error = %v", err)
	}

	show, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if show.Report.Body != "Inner prose from the committed render." {
		t.Fatalf("Body = %q, want inner render body only", show.Report.Body)
	}
	if strings.HasPrefix(show.Report.Body, "---") || strings.Contains(show.Report.Body, "loaf:render") {
		t.Fatalf("Body = %q, want no frontmatter and no render stamp", show.Report.Body)
	}
}

func TestEditReportBodyGuardsHandEditedDurableRender(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "render-guard",
		Kind:    "audit",
		Body:    "Original report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if _, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias); err != nil {
		t.Fatalf("FinalizeReport() error = %v", err)
	}
	render, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "report", Ref: created.Report.Alias})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact() error = %v", err)
	}

	// An unmodified render matches the stored body, so the edit is clean.
	if _, err := EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:  created.Report.Alias,
		Body: "Second report body.",
	}); err != nil {
		t.Fatalf("EditReportBody(clean) error = %v", err)
	}
	if _, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "report", Ref: created.Report.Alias}); err != nil {
		t.Fatalf("FinalizeDurableArtifact(refresh) error = %v", err)
	}

	content, err := os.ReadFile(render.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", render.Path, err)
	}
	handEdited := strings.Replace(string(content), "Second report body.", "Hand-edited render prose.", 1)
	if handEdited == string(content) {
		t.Fatalf("render content missing stored body:\n%s", content)
	}
	if err := os.WriteFile(render.Path, []byte(handEdited), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", render.Path, err)
	}

	_, err = EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:  created.Report.Alias,
		Body: "Third report body.",
	})
	if err == nil {
		t.Fatal("EditReportBody(hand-edited render) error = nil, want divergence refusal")
	}
	for _, want := range []string{"no longer matches the SQLite body", "loaf report finalize", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("EditReportBody() error = %q, want containing %q", err, want)
		}
	}
	show, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if show.Report.Body != "Second report body." {
		t.Fatalf("Body = %q, want unchanged SQLite body after refusal", show.Report.Body)
	}

	edited, err := EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:   created.Report.Alias,
		Body:  "Forced report body.",
		Force: true,
	})
	if err != nil {
		t.Fatalf("EditReportBody(force) error = %v", err)
	}
	if edited.EventID == "" {
		t.Fatalf("edited = %#v, want recorded edit event", edited)
	}
	show, err = ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if show.Report.Body != "Forced report body." {
		t.Fatalf("Body = %q, want forced replacement body", show.Report.Body)
	}
}

func TestEditReportBodyTreatsUnparseableRenderAsDivergent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "mangled-render",
		Kind:    "audit",
		Body:    "Original report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	render, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "report", Ref: created.Report.Alias})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact() error = %v", err)
	}
	// A hand-mangled render (stamp stripped, frontmatter gone) must trip the
	// guard even though its prose can no longer be compared.
	if err := os.WriteFile(render.Path, []byte("Original report body.\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", render.Path, err)
	}

	_, err = EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:  created.Report.Alias,
		Body: "Replacement body.",
	})
	if err == nil || !strings.Contains(err.Error(), "no longer matches the SQLite body") {
		t.Fatalf("EditReportBody(mangled render) error = %v, want divergence refusal", err)
	}

	if _, err := EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:   created.Report.Alias,
		Body:  "Replacement body.",
		Force: true,
	}); err != nil {
		t.Fatalf("EditReportBody(force) error = %v", err)
	}
}

func TestEditReportBodyRejectsArchivedReportBeforeMutation(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "archived-edit",
		Kind:    "audit",
		Body:    "Archived report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if _, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias); err != nil {
		t.Fatalf("FinalizeReport() error = %v", err)
	}
	if _, err := ArchiveReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias); err != nil {
		t.Fatalf("ArchiveReport() error = %v", err)
	}

	_, err = EditReportBody(context.Background(), root, PathResolver{StateHome: stateHome}, ReportEditOptions{
		Ref:  created.Report.Alias,
		Body: "Stranded edit body.",
	})
	if err == nil {
		t.Fatal("EditReportBody(archived) error = nil, want archived refusal")
	}
	for _, want := range []string{"archived", "historical record"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("EditReportBody(archived) error = %q, want containing %q", err, want)
		}
	}

	show, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if show.Report.Body != "Archived report body." {
		t.Fatalf("Body = %q, want unchanged body after archived refusal", show.Report.Body)
	}
	assertBodyEventCount(t, root, stateHome, "report", created.Report.ID, "body_edited", 0)
}

func TestFinalizeReportIsIdempotentWhenAlreadyDone(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "idempotent-finalize"})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if _, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias); err != nil {
		t.Fatalf("FinalizeReport() error = %v", err)
	}

	again, err := FinalizeReport(context.Background(), root, PathResolver{StateHome: stateHome}, created.Report.Alias)
	if err != nil {
		t.Fatalf("FinalizeReport(done) error = %v, want idempotent success", err)
	}
	if again.Previous != "done" || again.Status != "done" || again.EventID != "" || again.Report.Status != "done" {
		t.Fatalf("again = %#v, want done result without a transition or new event", again)
	}
	assertReportStatus(t, root, stateHome, created.Report.Alias, "done")
	// Exactly one draft -> done event: the re-finalize recorded nothing.
	assertReportEvent(t, root, stateHome, created.Report.ID, "draft", "done", "recorded by report finalize")
}

func TestShowReportReportsHasBody(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	withoutBody, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "no-body"})
	if err != nil {
		t.Fatalf("CreateReport(no-body) error = %v", err)
	}
	show, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, withoutBody.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport(no-body) error = %v", err)
	}
	if show.Report.HasBody || show.Report.Body != "" {
		t.Fatalf("Report = %#v, want HasBody false without a body", show.Report)
	}

	withBody, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "with-body",
		Body:    "Report body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport(with-body) error = %v", err)
	}
	show, err = ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, withBody.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport(with-body) error = %v", err)
	}
	if !show.Report.HasBody || show.Report.Body != "Report body." {
		t.Fatalf("Report = %#v, want HasBody true with body", show.Report)
	}
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
