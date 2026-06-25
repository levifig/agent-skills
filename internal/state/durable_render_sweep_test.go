package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSweepDurableRenderContractsUpgradesStaleCleanRender(t *testing.T) {
	root := projectRoot(t)
	specRel := filepath.ToSlash(filepath.Join(".agents", "specs", "SPEC-001.md"))
	writeAgentsFile(t, root.Path(), "specs/SPEC-001.md", durableRenderFixture(t, DurableRenderDocument{
		Kind: "spec",
		Fields: []DurableRenderField{
			{Key: "id", Value: "SPEC-001"},
			{Key: "status", Value: "implementing"},
			{Key: "title", Value: "Sweep Spec"},
		},
		Body: "# Sweep Spec\n\nBody.",
	}, "durable-doc-v1"))

	result, err := SweepDurableRenderContracts(root, DurableRenderSweepOptions{TargetContract: "durable-doc-v2"})
	if err != nil {
		t.Fatalf("SweepDurableRenderContracts() error = %v", err)
	}
	if result.UpgradeNeeded != 1 || result.Upgraded != 1 || result.Drift != 0 || result.Invalid != 0 || result.HasBlockingFindings() {
		t.Fatalf("result = %#v, want one upgraded file and no blockers", result)
	}
	if len(result.Files) != 1 || result.Files[0].RelativePath != specRel || result.Files[0].Status != "upgraded" || result.Files[0].FromContract != "durable-doc-v1" || result.Files[0].ToContract != "durable-doc-v2" {
		t.Fatalf("files = %#v, want upgraded stale spec render", result.Files)
	}
	content, err := os.ReadFile(filepath.Join(root.Path(), filepath.FromSlash(specRel)))
	if err != nil {
		t.Fatalf("ReadFile(upgraded spec) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "<!-- loaf:render kind=spec contract=durable-doc-v2 -->") {
		t.Fatalf("upgraded render = %q, want v2 stamp", text)
	}

	withDurableRenderContractForTest(t, "durable-doc-v2")
	rerendered, err := ReRenderDurableRender(text)
	if err != nil {
		t.Fatalf("ReRenderDurableRender(upgraded) error = %v", err)
	}
	if rerendered != text {
		t.Fatalf("upgraded render does not self-render")
	}
}

func TestSweepDurableRenderContractsSeparatesUpgradeFromDrift(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001.md", durableRenderFixture(t, DurableRenderDocument{
		Kind: "spec",
		Fields: []DurableRenderField{
			{Key: "id", Value: "SPEC-001"},
			{Key: "status", Value: "implementing"},
			{Key: "title", Value: "Clean Spec"},
		},
		Body: "# Clean Spec",
	}, "durable-doc-v1"))
	drifted := strings.Replace(durableRenderFixture(t, DurableRenderDocument{
		Kind: "report",
		Fields: []DurableRenderField{
			{Key: "id", Value: "report-drift"},
			{Key: "status", Value: "draft"},
			{Key: "title", Value: "Drift Report"},
		},
		Body: "# Drift Report",
	}, "durable-doc-v1"), "id: report-drift\nstatus: draft\ntitle: Drift Report", "title: Drift Report\nid: report-drift\nstatus: draft", 1)
	writeAgentsFile(t, root.Path(), "reports/report-drift.md", drifted)

	result, err := SweepDurableRenderContracts(root, DurableRenderSweepOptions{TargetContract: "durable-doc-v2"})
	if err != nil {
		t.Fatalf("SweepDurableRenderContracts() error = %v", err)
	}
	if result.UpgradeNeeded != 1 || result.Upgraded != 1 || result.Drift != 1 || result.Invalid != 0 || !result.HasBlockingFindings() {
		t.Fatalf("result = %#v, want one upgrade and one drift blocker", result)
	}
	statuses := map[string]string{}
	for _, file := range result.Files {
		statuses[file.RelativePath] = file.Status
	}
	if statuses[".agents/specs/SPEC-001.md"] != "upgraded" || statuses[".agents/reports/report-drift.md"] != "drift" {
		t.Fatalf("statuses = %#v, want clean upgrade separate from drift", statuses)
	}
	driftedContent, err := os.ReadFile(filepath.Join(root.Path(), ".agents", "reports", "report-drift.md"))
	if err != nil {
		t.Fatalf("ReadFile(drifted report) error = %v", err)
	}
	if strings.Contains(string(driftedContent), "durable-doc-v2") {
		t.Fatalf("drifted report was upgraded despite self-render drift:\n%s", driftedContent)
	}
}

func durableRenderFixture(t *testing.T, doc DurableRenderDocument, contract string) string {
	t.Helper()
	content, err := renderDurableDocumentWithContract(doc, contract)
	if err != nil {
		t.Fatalf("renderDurableDocumentWithContract() error = %v", err)
	}
	return content
}

func withDurableRenderContractForTest(t *testing.T, contract string) {
	t.Helper()
	previous := durableRenderCurrentContract
	durableRenderCurrentContract = contract
	t.Cleanup(func() {
		durableRenderCurrentContract = previous
	})
}
