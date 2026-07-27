package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChangeJSONAcceptsCanonicalTarget(t *testing.T) {
	meta := parseChangeJSON(`{
  "change": "example",
  "created": "2026-07-27",
  "branch": "example",
  "target_release": "2.0.0"
}`)
	if len(meta.Findings) != 0 {
		t.Fatalf("findings = %v", meta.Findings)
	}
	if meta.TargetRelease != "2.0.0" {
		t.Fatalf("target = %q", meta.TargetRelease)
	}
}

func TestParseChangeJSONRejectsGrammarAndClosedSchema(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"v-prefix", `{"change":"a","created":"2026-07-27","branch":"a","target_release":"v2.0.0"}`, "canonical MAJOR.MINOR.PATCH"},
		{"prerelease", `{"change":"a","created":"2026-07-27","branch":"a","target_release":"2.0.0-alpha.1"}`, "canonical MAJOR.MINOR.PATCH"},
		{"leading-zero", `{"change":"a","created":"2026-07-27","branch":"a","target_release":"02.0.0"}`, "canonical MAJOR.MINOR.PATCH"},
		{"status-key", `{"change":"a","created":"2026-07-27","branch":"a","status":"todo"}`, "status-like change.json key"},
		{"unknown-key", `{"change":"a","created":"2026-07-27","branch":"a","lineage":"x"}`, "unknown change.json key"},
		{"malformed", `{not-json`, "malformed change.json"},
		{"missing-change", `{"created":"2026-07-27","branch":"a"}`, `field "change" is required`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := parseChangeJSON(tc.content)
			if !findingsContain(meta.Findings, tc.want) {
				t.Fatalf("findings = %v, want substring %q", meta.Findings, tc.want)
			}
		})
	}
}

func TestLoadChangeNodesPrefersJSONAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "docs", "changes", "20260727-example")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	md := changeDoc(changeFrontmatter("example", "2026-07-27", "example"), productSections()...)
	if err := os.WriteFile(filepath.Join(folder, "change.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{"change":"example","created":"2026-07-27","branch":"example","target_release":"2.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	shape := "# Title\n\n" + strings.Join([]string{
		"## Problem\n\nAuthored problem.\n",
		"## Hypothesis\n\nAuthored hypothesis.\n",
		"## Scope\n\nAuthored scope.\n",
		"## Observable Workflow\n\nAuthored workflow.\n",
		"## Rabbit Holes and No-Gos\n\nAuthored holes.\n",
		"## Planning Contract\n\nAuthored plan.\n",
		"## Implementation Units\n\nAuthored units.\n",
		"## Verification Contract\n\nAuthored verify.\n",
		"## Definition of Done\n\nAuthored done.\n",
	}, "\n")
	if err := os.WriteFile(filepath.Join(folder, "shape.md"), []byte(shape), 0o644); err != nil {
		t.Fatal(err)
	}

	nodes, err := loadChangeNodes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d", len(nodes))
	}
	if nodes[0].Layout != changeLayoutNew || nodes[0].TargetRelease != "2.0.0" {
		t.Fatalf("node = %+v", nodes[0])
	}

	if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{bad`), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes, err = loadChangeNodes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes[0].ParseFindings) == 0 {
		t.Fatal("expected fail-closed parse findings for malformed change.json")
	}
	if nodes[0].Layout != changeLayoutNew {
		t.Fatal("malformed change.json must not fall back to legacy layout")
	}
}

func TestAssembleLegacyLoadsTargetRelease(t *testing.T) {
	fm := "---\nchange: root\ncreated: 2026-07-10\nbranch: root\nlineage: line\ntarget_release: 2.0.0\n---\n"
	doc := changeDoc(fm, append(productSections(), executableSections()...)...)
	node := assembleLegacyLayoutNode("docs/changes/20260710-root", doc)
	if node.TargetRelease != "2.0.0" || node.Layout != changeLayoutLegacy {
		t.Fatalf("node = %+v", node)
	}
}

func TestDeriveChangeCohorts(t *testing.T) {
	nodes := []changeNode{
		{Slug: "a", TargetRelease: "2.0.0", Folder: "docs/changes/20260710-a"},
		{Slug: "b", TargetRelease: "2.0.0", Folder: "docs/changes/20260711-b"},
		{Slug: "c", TargetRelease: "2.1.0", Folder: "docs/changes/20260712-c"},
		{Slug: "d", Folder: "docs/changes/20260713-d"},
	}
	cohorts := deriveChangeCohorts(nodes)
	if len(cohorts["2.0.0"]) != 2 || len(cohorts["2.1.0"]) != 1 || len(cohorts[""]) != 0 {
		t.Fatalf("cohorts = %#v", cohorts)
	}
}

func TestChangeCheckFailsClosedOnMalformedJSONBesideMarkdown(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := writeChangeFolder(t, repo, "20260727-example", changeDoc(changeFrontmatter("example", "2026-07-27", "example"), append(productSections(), executableSections()...)...))
	if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{bad`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runChangeCheckJSON(t, repo, "docs/changes/20260727-example")
	if err == nil || !findingsContain(out.Findings, "malformed change.json") {
		t.Fatalf("err=%v out=%+v", err, out)
	}
}

func TestDeriveChangeRetargetEventsUnionsSurfaces(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := writeChangeFolder(t, repo, "20260727-example", changeDoc(
		"---\nchange: example\ncreated: 2026-07-27\nbranch: example\ntarget_release: 2.0.0\n---\n",
		append(productSections(), executableSections()...)...,
	))
	commitAllChangeTest(t, repo, "docs: add targeted change")

	jsonPath := filepath.Join(folder, "change.json")
	if err := os.WriteFile(jsonPath, []byte(`{"change":"example","created":"2026-07-27","branch":"example","target_release":"2.1.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(folder, "change.md")); err != nil {
		t.Fatal(err)
	}
	commitAllChangeTest(t, repo, "docs: convert and retarget")

	events, err := deriveChangeRetargetEvents(repo, commandOutput)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Folder == "docs/changes/20260727-example" && event.From == "2.0.0" && event.To == "2.1.0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %#v, want 2.0.0 -> 2.1.0", events)
	}
}

func TestRetentionAllowsAtomicConversionAndBlocksJSONTargetDeletion(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := writeChangeFolder(t, repo, "20260727-example", changeDoc(
		"---\nchange: example\ncreated: 2026-07-27\nbranch: example\ntarget_release: 2.0.0\n---\n",
		append(productSections(), executableSections()...)...,
	))
	commitAllChangeTest(t, repo, "docs: add targeted change")

	if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{"change":"example","created":"2026-07-27","branch":"example","target_release":"2.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(folder, "change.md")); err != nil {
		t.Fatal(err)
	}
	commitAllChangeTest(t, repo, "docs: atomic convert")

	deleted, err := deletedLineageChangesWithOutput(repo, commandOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("atomic conversion treated as deletion: %v", deleted)
	}

	if err := os.Remove(filepath.Join(folder, "change.json")); err != nil {
		t.Fatal(err)
	}
	commitAllChangeTest(t, repo, "docs: delete target-declaring json")
	deleted, err = deletedLineageChangesWithOutput(repo, commandOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) == 0 {
		t.Fatal("expected retention finding for deleted target-declaring change.json")
	}
}

func TestResolveChangeFolderFindsJSONLayout(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := filepath.Join(repo, "docs", "changes", "20260727-example")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{"change":"example","created":"2026-07-27","branch":"example"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gotFolder, gotFile, err := resolveChangeFolder(repo, "docs/changes/20260727-example")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(gotFolder) != "20260727-example" || filepath.Base(gotFile) != "change.json" {
		t.Fatalf("folder=%q file=%q", gotFolder, gotFile)
	}
}
