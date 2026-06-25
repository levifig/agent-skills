package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestFindingCRUDVerdictFiltersAndReportShow(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	report, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:   "security-audit",
		Kind:   "audit",
		Source: "test",
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	created, err := CreateFinding(context.Background(), root, PathResolver{StateHome: stateHome}, FindingCreateOptions{
		Report:     report.Report.Alias,
		Title:      "Missing authorization check",
		Severity:   "critical",
		Confidence: "high",
		Dimension:  "auth",
		Path:       "internal/auth.go",
		LineStart:  42,
		LineEnd:    44,
		Body:       "The handler accepts caller-controlled account IDs.",
		SetBody:    true,
	})
	if err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	if !strings.HasPrefix(created.Finding.Alias, "FINDING-") || created.Finding.Status != "open" || created.Finding.Report.Alias != report.Report.Alias {
		t.Fatalf("created finding = %#v, want open finding linked to report", created.Finding)
	}
	if created.Finding.Body != "The handler accepts caller-controlled account IDs." {
		t.Fatalf("Body = %q, want finding body", created.Finding.Body)
	}
	assertReportProjectContext(t, root, created.ContractVersion, created.DatabaseScope, created.DatabasePath, created.ProjectID, created.ProjectName, created.ProjectCurrentPath)

	openFindings, err := ListFindings(context.Background(), root, PathResolver{StateHome: stateHome}, FindingListOptions{Severity: "critical", Status: "open"})
	if err != nil {
		t.Fatalf("ListFindings(open critical) error = %v", err)
	}
	if len(openFindings.Findings) != 1 {
		t.Fatalf("open critical findings = %#v, want one", openFindings.Findings)
	}

	verdict, err := RecordFindingVerdict(context.Background(), root, PathResolver{StateHome: stateHome}, FindingVerdictOptions{
		Finding:   created.Finding.Alias,
		Outcome:   "confirmed",
		Rationale: "Reproduced with a cross-account request.",
	})
	if err != nil {
		t.Fatalf("RecordFindingVerdict() error = %v", err)
	}
	if verdict.Finding.Status != "confirmed" || verdict.Verdict.Outcome != "confirmed" || verdict.Verdict.Finding.Alias != created.Finding.Alias {
		t.Fatalf("verdict = %#v, want confirmed finding verdict", verdict)
	}

	show, err := ShowFinding(context.Background(), root, PathResolver{StateHome: stateHome}, created.Finding.Alias)
	if err != nil {
		t.Fatalf("ShowFinding() error = %v", err)
	}
	if show.Finding.Status != "confirmed" || len(show.Finding.Verdicts) != 1 || show.Finding.Verdicts[0].Rationale == "" {
		t.Fatalf("show finding = %#v, want confirmed finding with verdict", show.Finding)
	}

	confirmed, err := ListFindings(context.Background(), root, PathResolver{StateHome: stateHome}, FindingListOptions{Status: "confirmed", Severity: "critical"})
	if err != nil {
		t.Fatalf("ListFindings(confirmed critical) error = %v", err)
	}
	if len(confirmed.Findings) != 1 {
		t.Fatalf("confirmed critical findings = %#v, want one", confirmed.Findings)
	}

	reportShow, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, report.Report.Alias)
	if err != nil {
		t.Fatalf("ShowReport() error = %v", err)
	}
	if len(reportShow.Report.Findings) != 1 || reportShow.Report.Findings[0].Status != "confirmed" || reportShow.Report.Findings[0].Alias != created.Finding.Alias {
		t.Fatalf("report findings = %#v, want confirmed child finding", reportShow.Report.Findings)
	}
}

func TestFindingRunRelationships(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	report, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "run-audit"})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	runAlias := insertRunFixture(t, root, stateHome, "RUN-001")

	finding, err := CreateFinding(context.Background(), root, PathResolver{StateHome: stateHome}, FindingCreateOptions{
		Report:     report.Report.Alias,
		Run:        runAlias,
		Title:      "Pipeline finding",
		Severity:   "high",
		Confidence: "medium",
	})
	if err != nil {
		t.Fatalf("CreateFinding(run) error = %v", err)
	}
	if finding.Finding.Run == nil || finding.Finding.Run.Alias != runAlias {
		t.Fatalf("finding run = %#v, want %s", finding.Finding.Run, runAlias)
	}
	if _, err := RecordFindingVerdict(context.Background(), root, PathResolver{StateHome: stateHome}, FindingVerdictOptions{
		Finding:   finding.Finding.Alias,
		Run:       runAlias,
		Outcome:   "partial",
		Rationale: "Pipeline evidence was incomplete.",
	}); err != nil {
		t.Fatalf("RecordFindingVerdict(run) error = %v", err)
	}

	links, err := ListLinks(context.Background(), root, PathResolver{StateHome: stateHome}, runAlias)
	if err != nil {
		t.Fatalf("ListLinks(run) error = %v", err)
	}
	seen := map[string]bool{}
	for _, relationship := range links.Relationships {
		seen[relationship.Type+" "+relationship.Entity.Kind] = true
	}
	if !seen["produces finding"] || !seen["records verdict"] {
		t.Fatalf("run relationships = %#v, want produces finding and records verdict", links.Relationships)
	}
}

func TestFindingValidationRejectsInvalidVocabulary(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	report, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "validation"})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if _, err := CreateFinding(context.Background(), root, PathResolver{StateHome: stateHome}, FindingCreateOptions{
		Report:     report.Report.Alias,
		Title:      "Bad severity",
		Severity:   "blocker",
		Confidence: "high",
	}); err == nil || !strings.Contains(err.Error(), "invalid finding severity") {
		t.Fatalf("CreateFinding(invalid severity) error = %v, want invalid severity", err)
	}
	if _, err := ListFindings(context.Background(), root, PathResolver{StateHome: stateHome}, FindingListOptions{Status: "done"}); err == nil || !strings.Contains(err.Error(), "invalid finding status") {
		t.Fatalf("ListFindings(invalid status) error = %v, want invalid status", err)
	}
}

func TestImportFindingJSONLoadsFindingsVerdictsAndIsIdempotent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	fixtureDir := t.TempDir()
	findingsPath := filepath.Join(fixtureDir, "find.auth.json")
	verdictsPath := filepath.Join(fixtureDir, "VERDICTS.json")
	writeFindingJSONFixture(t, findingsPath, `[
  {
    "id": "auth-001",
    "title": "Missing authorization check",
    "severity": "critical",
    "confidence": "high",
    "path": "internal/auth.go",
    "line_start": 42,
    "message": "Handler accepts caller-controlled account IDs.",
    "verdicts": [
      {
        "id": "auth-001-confirmed",
        "outcome": "confirmed",
        "rationale": "Reproduced with a cross-account request."
      }
    ]
  },
  {
    "id": "auth-002",
    "title": "Overstated auth warning",
    "severity": "high",
    "confidence": "medium",
    "path": "internal/warning.go",
    "line": 7
  }
]`)
	writeFindingJSONFixture(t, verdictsPath, `{
  "verdicts": [
    {
      "id": "auth-002-refuted",
      "finding_id": "auth-002",
      "outcome": "refuted",
      "rationale": "The warning path is unreachable in production.",
      "notes": "Checked guard conditions."
    }
  ]
}`)

	imported, err := ImportFindingJSON(context.Background(), root, PathResolver{StateHome: stateHome}, FindingImportJSONOptions{
		Report:       "pipeline-audit",
		FindingFiles: []string{findingsPath},
		VerdictFiles: []string{verdictsPath},
	})
	if err != nil {
		t.Fatalf("ImportFindingJSON() error = %v", err)
	}
	if imported.Report.Alias != "report-pipeline-audit" {
		t.Fatalf("Report.Alias = %q, want created report alias", imported.Report.Alias)
	}
	if imported.FindingsImported != 2 || imported.VerdictsImported != 2 || imported.FindingsSkipped != 0 || imported.VerdictsSkipped != 0 {
		t.Fatalf("imported counts = %#v, want 2 findings and 2 verdicts imported", imported)
	}

	confirmed, err := ListFindings(context.Background(), root, PathResolver{StateHome: stateHome}, FindingListOptions{Severity: "critical", Status: "confirmed", Dimension: "auth"})
	if err != nil {
		t.Fatalf("ListFindings(confirmed critical auth) error = %v", err)
	}
	if len(confirmed.Findings) != 1 {
		t.Fatalf("confirmed findings = %#v, want one imported finding", confirmed.Findings)
	}
	for _, finding := range confirmed.Findings {
		if finding.Path != "internal/auth.go" || finding.LineStart != 42 {
			t.Fatalf("confirmed finding = %#v, want imported location", finding)
		}
	}

	refuted, err := ListFindings(context.Background(), root, PathResolver{StateHome: stateHome}, FindingListOptions{Status: "refuted", Severity: "high"})
	if err != nil {
		t.Fatalf("ListFindings(refuted high) error = %v", err)
	}
	if len(refuted.Findings) != 1 {
		t.Fatalf("refuted findings = %#v, want verdict-file update", refuted.Findings)
	}

	second, err := ImportFindingJSON(context.Background(), root, PathResolver{StateHome: stateHome}, FindingImportJSONOptions{
		Report:       "report-pipeline-audit",
		FindingFiles: []string{findingsPath},
		VerdictFiles: []string{verdictsPath},
	})
	if err != nil {
		t.Fatalf("ImportFindingJSON(second) error = %v", err)
	}
	if second.FindingsImported != 0 || second.VerdictsImported != 0 || second.FindingsSkipped != 2 || second.VerdictsSkipped != 2 {
		t.Fatalf("second counts = %#v, want idempotent skips", second)
	}
	reportShow, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-pipeline-audit")
	if err != nil {
		t.Fatalf("ShowReport(imported report) error = %v", err)
	}
	if len(reportShow.Report.Findings) != 2 {
		t.Fatalf("report findings = %#v, want two imported findings", reportShow.Report.Findings)
	}
}

func TestImportFindingJSONLoadsDeepEvaluationReportRows(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	fixtureDir := t.TempDir()
	findingsPath := filepath.Join(fixtureDir, "find.loaf-skill-suite.json")
	writeFindingJSONFixture(t, findingsPath, `[
  {
    "id": "loaf-build-validates-nothing",
    "title": "Build does not validate generated target output",
    "dimension": "build-integrity",
    "severity": "critical",
    "confidence": "high",
    "path": "cli/lib/build/targets/amp.ts",
    "message": "The generated plugin surface can ship syntactically invalid output.",
    "verdicts": [
      {
        "id": "loaf-build-validates-nothing-confirmed",
        "outcome": "confirmed",
        "rationale": "Deep evaluation reproduced invalid generated plugin syntax."
      }
    ]
  },
  {
    "id": "loaf-taxonomy-too-large",
    "title": "Skill taxonomy is too large",
    "dimension": "taxonomy",
    "severity": "high",
    "confidence": "medium",
    "message": "The deep evaluation treated this as a tempting but unsupported diagnosis.",
    "verdicts": [
      {
        "id": "loaf-taxonomy-too-large-refuted",
        "outcome": "refuted",
        "rationale": "The report concluded no skill-family merges were warranted."
      }
    ]
  }
]`)

	imported, err := ImportFindingJSON(context.Background(), root, PathResolver{StateHome: stateHome}, FindingImportJSONOptions{
		Report:       "report-loaf-skill-suite-deep-evaluation",
		ReportKind:   "audit",
		Source:       "deep-evaluation-report",
		FindingFiles: []string{findingsPath},
	})
	if err != nil {
		t.Fatalf("ImportFindingJSON(deep evaluation) error = %v", err)
	}
	if imported.Report.Alias != "report-loaf-skill-suite-deep-evaluation" || imported.FindingsImported != 2 || imported.VerdictsImported != 2 {
		t.Fatalf("imported = %#v, want deep-evaluation report with two findings and verdicts", imported)
	}

	confirmed, err := ListFindings(context.Background(), root, PathResolver{StateHome: stateHome}, FindingListOptions{Severity: "critical", Status: "confirmed"})
	if err != nil {
		t.Fatalf("ListFindings(critical confirmed) error = %v", err)
	}
	if len(confirmed.Findings) != 1 {
		t.Fatalf("confirmed findings = %#v, want one critical confirmed deep-evaluation finding", confirmed.Findings)
	}
	for _, finding := range confirmed.Findings {
		if finding.Report != "report-loaf-skill-suite-deep-evaluation" || finding.Dimension != "build-integrity" {
			t.Fatalf("confirmed finding = %#v, want deep-evaluation build-integrity finding", finding)
		}
	}

	reportShow, err := ShowReport(context.Background(), root, PathResolver{StateHome: stateHome}, "report-loaf-skill-suite-deep-evaluation")
	if err != nil {
		t.Fatalf("ShowReport(deep evaluation) error = %v", err)
	}
	if len(reportShow.Report.Findings) != 2 {
		t.Fatalf("report findings = %#v, want imported deep-evaluation decomposition", reportShow.Report.Findings)
	}
}

func insertRunFixture(t *testing.T, root project.Root, stateHome string, alias string) string {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	now := "2026-06-24T12:00:00Z"
	runID := stableMigrationID("run", projectID, alias)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO runs (id, project_id, generator_ref, generator_version, generator_hash, status, metadata, started_at, completed_at, created_at, updated_at)
VALUES (?, ?, 'pipeline/findings', 'v1', 'hash-run', 'completed', NULL, ?, ?, ?, ?)
`, runID, projectID, now, now, now, now); err != nil {
		t.Fatalf("insert run fixture error = %v", err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin run alias transaction error = %v", err)
	}
	defer tx.Rollback()
	if err := insertAlias(context.Background(), tx, projectID, "run", runID, "run", alias, now); err != nil {
		t.Fatalf("insert run alias error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit run alias transaction error = %v", err)
	}
	return alias
}

func writeFindingJSONFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s error = %v", path, err)
	}
}
