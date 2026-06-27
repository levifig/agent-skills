package state

import (
	"strings"
	"testing"
)

func TestRenderDurableDocumentIsDeterministicAndCanonical(t *testing.T) {
	doc := DurableRenderDocument{
		Kind: "spec",
		Fields: []DurableRenderField{
			{Key: "title", Value: "Render: Plan"},
			{Key: "id", Value: "SPEC-044"},
			{Key: "status", Value: "implementing"},
		},
		Body: "\r\n# Render Plan\r\n\r\nBody text.\r\n",
	}

	first, err := RenderDurableDocument(doc)
	if err != nil {
		t.Fatalf("RenderDurableDocument() error = %v", err)
	}
	second, err := RenderDurableDocument(doc)
	if err != nil {
		t.Fatalf("second RenderDurableDocument() error = %v", err)
	}
	if first != second {
		t.Fatal("RenderDurableDocument() was not byte-deterministic")
	}
	if strings.Contains(first, "\r") {
		t.Fatalf("render contains CR line endings: %q", first)
	}
	assertInOrder(t, first, "id: SPEC-044", "status: implementing", `title: "Render: Plan"`)
	if !strings.HasSuffix(first, "<!-- loaf:render kind=spec contract=durable-doc-v1 -->\n") {
		t.Fatalf("render stamp missing or wrong:\n%s", first)
	}
}

func TestParseDurableRenderRoundTripsThroughRenderer(t *testing.T) {
	rendered, err := RenderDurableDocument(DurableRenderDocument{
		Kind: "report",
		Fields: []DurableRenderField{
			{Key: "id", Value: "report-audit"},
			{Key: "report_kind", Value: "audit"},
			{Key: "status", Value: "final"},
			{Key: "title", Value: "Audit Report"},
		},
		Body: "# Audit Report\n\nFindings stay in the body.",
	})
	if err != nil {
		t.Fatalf("RenderDurableDocument() error = %v", err)
	}

	parsed, err := ParseDurableRender(rendered)
	if err != nil {
		t.Fatalf("ParseDurableRender() error = %v", err)
	}
	if parsed.Kind != "report" || parsed.Body != "# Audit Report\n\nFindings stay in the body." {
		t.Fatalf("parsed = %#v, want report with body", parsed)
	}
	rerendered, err := RenderDurableDocument(parsed)
	if err != nil {
		t.Fatalf("RenderDurableDocument(parsed) error = %v", err)
	}
	if rerendered != rendered {
		t.Fatalf("round-trip changed bytes:\n--- rendered ---\n%s\n--- rerendered ---\n%s", rendered, rerendered)
	}
}

func TestParseDurableRenderRejectsMissingOrUnsupportedStamp(t *testing.T) {
	if _, err := ParseDurableRender("---\nid: SPEC-044\n---\n\n# Body\n"); err == nil || !strings.Contains(err.Error(), "stamp") {
		t.Fatalf("ParseDurableRender(missing stamp) error = %v, want stamp error", err)
	}
	content := "---\nid: SPEC-044\n---\n\n# Body\n\n<!-- loaf:render kind=spec contract=durable-doc-v2 -->\n"
	if _, err := ParseDurableRender(content); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("ParseDurableRender(unsupported contract) error = %v, want unsupported contract", err)
	}
}

func TestDurableSpecAndReportDocumentsOmitVolatileFields(t *testing.T) {
	specRender, err := RenderDurableDocument(DurableSpecRenderDocument(SpecDetail{
		ID:        "spec:internal",
		Alias:     "SPEC-044",
		Title:     "Durable Render",
		Status:    "implementing",
		Tasks:     SpecTaskCounts{Todo: 1, InProgress: 2, Done: 3},
		Body:      "# Durable Render\n\nSpec body.",
		CreatedAt: "2026-06-24T00:00:00Z",
		UpdatedAt: "2026-06-24T01:00:00Z",
	}))
	if err != nil {
		t.Fatalf("RenderDurableDocument(spec) error = %v", err)
	}
	for _, forbidden := range []string{"2026-06-24T00:00:00Z", "2026-06-24T01:00:00Z", "Tasks:", "todo"} {
		if strings.Contains(specRender, forbidden) {
			t.Fatalf("spec render contains volatile %q:\n%s", forbidden, specRender)
		}
	}
	// The internal state ID is a random per-database surrogate; it must never
	// leak into a committed render or byte-reproducibility breaks (SPEC-044).
	if strings.Contains(specRender, "state_id") || strings.Contains(specRender, "spec:internal") {
		t.Fatalf("spec render leaks volatile internal state ID:\n%s", specRender)
	}
	if !strings.Contains(specRender, "id: SPEC-044") {
		t.Fatalf("spec render missing stable alias id:\n%s", specRender)
	}

	reportRender, err := RenderDurableDocument(DurableReportRenderDocument(ReportDetail{
		ID:        "report:internal",
		Alias:     "report-audit",
		Title:     "Audit",
		Kind:      "security",
		Status:    "draft",
		Body:      "# Audit\n\nReport body.",
		CreatedAt: "2026-06-24T00:00:00Z",
		UpdatedAt: "2026-06-24T01:00:00Z",
	}))
	if err != nil {
		t.Fatalf("RenderDurableDocument(report) error = %v", err)
	}
	if !strings.Contains(reportRender, "report_kind: security") || !strings.Contains(reportRender, "<!-- loaf:render kind=report contract=durable-doc-v1 -->") {
		t.Fatalf("report render missing kind/stamp:\n%s", reportRender)
	}
	if strings.Contains(reportRender, "2026-06-24T00:00:00Z") || strings.Contains(reportRender, "2026-06-24T01:00:00Z") {
		t.Fatalf("report render contains timestamps:\n%s", reportRender)
	}
	if strings.Contains(reportRender, "state_id") || strings.Contains(reportRender, "report:internal") {
		t.Fatalf("report render leaks volatile internal state ID:\n%s", reportRender)
	}
}

func TestReRenderDurableRenderCanonicalizesEditedBytes(t *testing.T) {
	edited := "---\ntitle: Demo\nid: SPEC-001\n---\n\n# Demo\n\n<!-- loaf:render kind=spec contract=durable-doc-v1 -->\n"
	rerendered, err := ReRenderDurableRender(edited)
	if err != nil {
		t.Fatalf("ReRenderDurableRender() error = %v", err)
	}
	if rerendered == edited {
		t.Fatal("ReRenderDurableRender() preserved non-canonical field order; want canonical bytes for drift comparison")
	}
	assertInOrder(t, rerendered, "id: SPEC-001", "title: Demo")
}

func assertInOrder(t *testing.T, text string, parts ...string) {
	t.Helper()
	offset := 0
	for _, part := range parts {
		index := strings.Index(text[offset:], part)
		if index < 0 {
			t.Fatalf("%q not found after offset %d in:\n%s", part, offset, text)
		}
		offset += index + len(part)
	}
}
