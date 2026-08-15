package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// changeDoc assembles a change.md body from frontmatter and section blocks so
// each test can isolate exactly one Verification-Contract clause.
func changeDoc(frontmatter string, sections ...string) string {
	var b strings.Builder
	b.WriteString(frontmatter)
	b.WriteString("\n# Title\n\n")
	for _, section := range sections {
		b.WriteString(section)
		b.WriteString("\n\n")
	}
	return b.String()
}

func changeFrontmatter(change, created, branch string) string {
	return strings.Join([]string{
		"---",
		"change: " + change,
		"created: " + created,
		"branch: " + branch,
		"---",
	}, "\n")
}

func lineageFrontmatter(change, created, branch, lineage, predecessor, releaseAfter string) string {
	lines := []string{"---", "change: " + change, "created: " + created, "branch: " + branch, "lineage: " + lineage}
	if predecessor != "" {
		lines = append(lines, "predecessor: "+predecessor)
	}
	if releaseAfter != "" {
		lines = append(lines, "release-after: "+releaseAfter)
	}
	return strings.Join(append(lines, "---"), "\n")
}

// productSections returns the five required Product Contract H2s, each with a
// line of body so they read as present and non-empty.
func productSections() []string {
	return []string{
		"## Problem\n\nThe friction.",
		"## Hypothesis\n\nThe bet.",
		"## Scope\n\nIn and out.",
		"## Observable Workflow\n\nWhat ships.",
		"## Rabbit Holes and No-Gos\n\nBoundaries.",
	}
}

// executableSections returns the four sections that drive derived executability.
func executableSections() []string {
	return []string{
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	}
}

// writeChangeFolder materializes docs/changes/<folder>/change.md under root and
// returns the absolute folder path.
func writeChangeFolder(t *testing.T, root, folder, content string) string {
	t.Helper()
	dir := filepath.Join(root, "docs", "changes", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(change.md) error = %v", err)
	}
	return dir
}

func runChangeCheckJSON(t *testing.T, repo string, args ...string) (changeCheckJSON, error) {
	t.Helper()
	var stdout bytes.Buffer
	runArgs := append([]string{"change", "check"}, args...)
	runArgs = append(runArgs, "--json")
	err := Runner{Stdout: &stdout, WorkingDir: repo}.Run(runArgs)
	var out changeCheckJSON
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), decodeErr)
	}
	return out, err
}

func executableLineageDoc(slug, lineage, predecessor, releaseAfter string) string {
	sections := append(productSections(), executableSections()...)
	return changeDoc(lineageFrontmatter(slug, "2026-07-10", slug, lineage, predecessor, releaseAfter), sections...)
}

func commitAllChangeTest(t *testing.T, repo, message string) {
	t.Helper()
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "commit", "-m", message)
}

func writeReleaseVersionFiles(t *testing.T, repo, version string) {
	t.Helper()
	pkg := "{\n  \"name\": \"demo\",\n  \"version\": \"" + version + "\"\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatalf("WriteFile package.json: %v", err)
	}
}

func seedCohortGateRepo(t *testing.T, version string) string {
	t.Helper()
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, version)
	return repo
}

func writeNewLayoutChange(t *testing.T, repo, folder, slug, target string, shapeBody string) string {
	t.Helper()
	dir := filepath.Join(repo, "docs", "changes", folder)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	meta := "{\n  \"change\": \"" + slug + "\",\n  \"created\": \"2026-07-27\",\n  \"branch\": \"" + slug + "\""
	if target != "" {
		meta += ",\n  \"target_release\": \"" + target + "\""
	}
	meta += "\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "change.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile change.json: %v", err)
	}
	if shapeBody == "" {
		shapeBody = authoredShapeBody()
	}
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(shapeBody), 0o644); err != nil {
		t.Fatalf("WriteFile shape.md: %v", err)
	}
	return dir
}

func authoredShapeBody() string {
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- **V1.** Smoke.\n  - Command: `true`\n  - Expect: exit 0",
		"## Definition of Done\n\n- Gates pass.",
	)
	var b strings.Builder
	b.WriteString("# Demo\n\n")
	for _, s := range sections {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	return b.String()
}

func flipExecuteChange(t *testing.T, repo, dir, slug string) {
	t.Helper()
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.MkdirAll(filepath.Dir(task), 0o755); err != nil {
		t.Fatalf("MkdirAll tasks: %v", err)
	}
	unchecked := "---\nchange: " + slug + "\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"
	if err := os.WriteFile(task, []byte(unchecked), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: shape "+slug)

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	checked := "---\nchange: " + slug + "\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"
	if err := os.WriteFile(task, []byte(checked), 0o644); err != nil {
		t.Fatalf("WriteFile flip: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: execute "+slug)
}

func TestChangeLineageValidation(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, repo string) string
		want  string
	}{
		{"duplicate-slug", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", ""))
			return writeChangeFolder(t, repo, "20260711-root", strings.Replace(executableLineageDoc("root", "line", "", ""), "created: 2026-07-10", "created: 2026-07-11", 1))
		}, "duplicate Change slug"},
		{"multiple-roots", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", ""))
			return writeChangeFolder(t, repo, "20260710-other", executableLineageDoc("other", "line", "", ""))
		}, "multiple roots"},
		{"self-reference", func(t *testing.T, repo string) string {
			return writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "root", ""))
		}, "cannot name itself"},
		{"missing-predecessor", func(t *testing.T, repo string) string {
			return writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "missing", ""))
		}, "predecessor \"missing\" is not materialized"},
		{"lineage-mismatch", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "other", "", ""))
			return writeChangeFolder(t, repo, "20260710-next", executableLineageDoc("next", "line", "root", ""))
		}, "has lineage \"other\", want \"line\""},
		{"cycle", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "next", ""))
			return writeChangeFolder(t, repo, "20260710-next", executableLineageDoc("next", "line", "root", ""))
		}, "predecessor cycle"},
		{"longer-cycle", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-one", executableLineageDoc("one", "line", "three", ""))
			writeChangeFolder(t, repo, "20260710-two", executableLineageDoc("two", "line", "one", ""))
			return writeChangeFolder(t, repo, "20260710-three", executableLineageDoc("three", "line", "two", ""))
		}, "predecessor cycle"},
		{"duplicate-lineage-key", func(t *testing.T, repo string) string {
			doc := strings.Replace(executableLineageDoc("root", "line", "", ""), "lineage: line", "lineage: line\nlineage: other", 1)
			return writeChangeFolder(t, repo, "20260710-root", doc)
		}, "duplicate frontmatter field \"lineage\""},
		{"dependency-without-lineage", func(t *testing.T, repo string) string {
			doc := strings.Replace(executableLineageDoc("child", "line", "root", ""), "lineage: line\n", "", 1)
			return writeChangeFolder(t, repo, "20260710-child", doc)
		}, "predecessor and release-after require lineage"},
		{"multiple-children", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", ""))
			writeChangeFolder(t, repo, "20260710-left", executableLineageDoc("left", "line", "root", ""))
			return writeChangeFolder(t, repo, "20260710-right", executableLineageDoc("right", "line", "root", ""))
		}, "multiple materialized children"},
		{"conflicting-release-after", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "terminal"))
			return writeChangeFolder(t, repo, "20260710-terminal", executableLineageDoc("terminal", "line", "root", "root"))
		}, "conflicting release-after terminals"},
		{"release-after-not-terminal", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "root"))
			return writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "root", ""))
		}, "release-after \"root\" is not the lineage terminal"},
		{"release-after-on-non-root", func(t *testing.T, repo string) string {
			writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "child"))
			return writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "root", "child"))
		}, "root \"root\" must own the declaration"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			folder := tc.setup(t, repo)
			out, err := runChangeCheckJSON(t, repo, folder)
			all := append(append(append([]string{}, out.Findings...), out.Warnings...), out.Gaps...)
			if (tc.name != "missing-predecessor" && err == nil) || !findingsContain(all, tc.want) {
				t.Fatalf("err = %v findings = %v warnings = %v, want %q", err, out.Findings, out.Warnings, tc.want)
			}
		})
	}
}

func TestChangeCheckRejectsMixedCaseDuplicateIdentityAndLineageFields(t *testing.T) {
	cases := []struct {
		key         string
		replacement string
	}{
		{key: "change", replacement: "change: root\nChAnGe: root"},
		{key: "created", replacement: "created: 2026-07-10\nCrEaTeD: 2026-07-10"},
		{key: "lineage", replacement: "lineage: line\nLiNeAgE: line"},
		{key: "predecessor", replacement: "predecessor: prior\nPrEdEcEsSoR: prior"},
		{key: "release-after", replacement: "release-after: terminal\nReLeAsE-AfTeR: terminal"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			doc := executableLineageDoc("root", "line", "prior", "terminal")
			original := map[string]string{
				"change":        "change: root",
				"created":       "created: 2026-07-10",
				"lineage":       "lineage: line",
				"predecessor":   "predecessor: prior",
				"release-after": "release-after: terminal",
			}[tc.key]
			doc = strings.Replace(doc, original, tc.replacement, 1)
			folder := writeChangeFolder(t, repo, "20260710-root", doc)
			out, err := runChangeCheckJSON(t, repo, folder)
			want := "duplicate frontmatter field \"" + tc.key + "\""
			if err == nil || !findingsContain(out.Findings, want) || !findingsContain(out.Findings, "docs/changes/20260710-root/change.md:") {
				t.Fatalf("err = %v findings = %v, want repo-relative %q", err, out.Findings, want)
			}
		})
	}
}

func TestChangeCheckRejectsMalformedAndUnclosedFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		doc  func() string
		want string
	}{
		{name: "lineage-without-colon", doc: func() string {
			return strings.Replace(executableLineageDoc("root", "line", "", ""), "lineage: line", "lineage line", 1)
		}, want: "malformed frontmatter line"},
		{name: "predecessor-without-colon", doc: func() string {
			return strings.Replace(executableLineageDoc("child", "line", "root", ""), "predecessor: root", "predecessor root", 1)
		}, want: "malformed frontmatter line"},
		{name: "empty-key", doc: func() string {
			return strings.Replace(executableLineageDoc("root", "line", "", ""), "lineage: line", "lineage: line\n: value", 1)
		}, want: "key cannot be empty"},
		{name: "unclosed", doc: func() string {
			return strings.Replace(executableLineageDoc("root", "line", "", ""), "\n---\n# Title", "\n# Title", 1)
		}, want: "frontmatter is not closed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			folder := writeChangeFolder(t, repo, "20260710-root", tc.doc())
			out, err := runChangeCheckJSON(t, repo, folder)
			if err == nil || !findingsContain(out.Findings, tc.want) || !findingsContain(out.Findings, "docs/changes/20260710-root/change.md:") {
				t.Fatalf("err = %v findings = %v, want repo-relative %q", err, out.Findings, tc.want)
			}
		})
	}
}

func TestChangeCheckDoesNotLeakUnrelatedLineageFindings(t *testing.T) {
	repo := initCLIGitRepo(t)
	good := writeChangeFolder(t, repo, "20260710-good", executableLineageDoc("good", "good-line", "", ""))
	badOne := writeChangeFolder(t, repo, "20260710-bad-one", executableLineageDoc("bad-one", "bad-line", "bad-two", ""))
	writeChangeFolder(t, repo, "20260710-bad-two", executableLineageDoc("bad-two", "bad-line", "bad-one", ""))
	goodOut, err := runChangeCheckJSON(t, repo, good)
	if err != nil || !goodOut.Passed || findingsContain(goodOut.Findings, "predecessor cycle") {
		t.Fatalf("good lineage err = %v out = %+v", err, goodOut)
	}
	badOut, err := runChangeCheckJSON(t, repo, badOne)
	if err == nil || !findingsContain(badOut.Findings, "predecessor cycle") {
		t.Fatalf("bad lineage err = %v out = %+v", err, badOut)
	}
}

func TestChangeCheckIncludesLocalFindingsFromItsWholeLineage(t *testing.T) {
	repo := initCLIGitRepo(t)
	root := writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "child"))
	malformedChild := strings.Replace(executableLineageDoc("child", "line", "root", ""), "\n---\n# Title", "\nmalformed child frontmatter\n---\n# Title", 1)
	writeChangeFolder(t, repo, "20260710-child", malformedChild)

	out, err := runChangeCheckJSON(t, repo, root)
	if err == nil || !findingsContain(out.Findings, "docs/changes/20260710-child/change.md: malformed frontmatter") {
		t.Fatalf("same-lineage local finding was omitted: err = %v out = %+v", err, out)
	}
}

func TestChangeCheckDoesNotTreatUnlineagedMalformedFileAsGlobal(t *testing.T) {
	repo := initCLIGitRepo(t)
	good := writeChangeFolder(t, repo, "20260710-good", executableLineageDoc("good", "good-line", "", "good"))
	malformed := strings.Replace(changeDoc(changeFrontmatter("legacy", "2026-07-10", "legacy"), productSections()...), "\n---\n# Title", "\nmalformed legacy frontmatter\n---\n# Title", 1)
	legacy := writeChangeFolder(t, repo, "20260710-legacy", malformed)
	goodOut, err := runChangeCheckJSON(t, repo, good)
	if err != nil || !goodOut.Passed || !goodOut.Executable || len(goodOut.Findings) != 0 {
		t.Fatalf("unrelated unlineaged file blocked good lineage: err = %v out = %+v", err, goodOut)
	}
	legacyOut, err := runChangeCheckJSON(t, repo, legacy)
	if err == nil || !findingsContain(legacyOut.Findings, "malformed frontmatter") {
		t.Fatalf("malformed file should fail its own check: err = %v out = %+v", err, legacyOut)
	}
}

func TestChangeCheckScopesUnlineagedFindingsToTheirChange(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(), executableSections()...)
	good := writeChangeFolder(t, repo, "20260710-good", changeDoc(changeFrontmatter("good", "2026-07-10", "good"), sections...))
	malformed := strings.Replace(changeDoc(changeFrontmatter("legacy", "2026-07-10", "legacy"), productSections()...), "\n---\n# Title", "\nmalformed legacy frontmatter\n---\n# Title", 1)
	legacy := writeChangeFolder(t, repo, "20260710-legacy", malformed)

	goodOut, err := runChangeCheckJSON(t, repo, good, "--require-executable")
	if err != nil || !goodOut.Passed || !goodOut.Executable || len(goodOut.Findings) != 0 {
		t.Fatalf("unrelated unlineaged file blocked valid unlineaged Change: err = %v out = %+v", err, goodOut)
	}
	legacyOut, err := runChangeCheckJSON(t, repo, legacy)
	if err == nil || !findingsContain(legacyOut.Findings, "malformed frontmatter") {
		t.Fatalf("malformed unlineaged Change should fail its own check: err = %v out = %+v", err, legacyOut)
	}
}

func TestChangeCheckIncludesGlobalFindingsForEveryLineage(t *testing.T) {
	repo := initCLIGitRepo(t)
	good := writeChangeFolder(t, repo, "20260710-good", executableLineageDoc("good", "good-line", "", ""))
	writeChangeFolder(t, repo, "20260710-duplicate", executableLineageDoc("duplicate", "first-line", "", ""))
	duplicate := strings.Replace(executableLineageDoc("duplicate", "second-line", "", ""), "created: 2026-07-10", "created: 2026-07-11", 1)
	writeChangeFolder(t, repo, "20260711-duplicate", duplicate)
	out, err := runChangeCheckJSON(t, repo, good)
	if err == nil || !findingsContain(out.Findings, "duplicate Change slug") {
		t.Fatalf("global finding was omitted: err = %v out = %+v", err, out)
	}
}

func TestChangeInitRejectsSlugExistingOnAnotherDate(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeChangeFolder(t, repo, "20260709-reused", strings.Replace(executableLineageDoc("reused", "line", "", ""), "created: 2026-07-10", "created: 2026-07-09", 1))
	err := Runner{WorkingDir: repo}.Run([]string{"change", "init", "reused"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("init error = %v", err)
	}
}

func TestChangeCheckRequiresCommittedPredecessorButNotCompletion(t *testing.T) {
	repo := initCLIGitRepo(t)
	root := writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "child"))
	commitAllChangeTest(t, repo, "docs: add root")
	child := writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "root", ""))
	out, err := runChangeCheckJSON(t, repo, child, "--require-executable")
	if err != nil || !out.Executable {
		t.Fatalf("committed predecessor err = %v out = %+v", err, out)
	}
	if root == "" {
		t.Fatal("root should be materialized")
	}
	// A root that is only in the working tree cannot satisfy another Change's ancestry.
	repo = initCLIGitRepo(t)
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "child"))
	child = writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "root", ""))
	out, err = runChangeCheckJSON(t, repo, child, "--require-executable")
	if err == nil || !findingsContain(out.Findings, "not structurally executable") || !findingsContain(out.Findings, "implementation completion is not implied") {
		t.Fatalf("err = %v findings = %v", err, out.Findings)
	}
}

func TestChangeCheckRequiresCommittedPredecessorGraphToBeExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	root := writeChangeFolder(t, repo, "20260710-root", changeDoc(lineageFrontmatter("root", "2026-07-10", "root", "line", "", ""), productSections()...))
	commitAllChangeTest(t, repo, "docs: add structurally incomplete root")

	if err := os.WriteFile(filepath.Join(root, "change.md"), []byte(executableLineageDoc("root", "line", "", "")), 0o644); err != nil {
		t.Fatal(err)
	}
	child := writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "root", ""))
	out, err := runChangeCheckJSON(t, repo, child, "--require-executable")
	if err == nil || out.Executable || !findingsContain(out.Gaps, "committed predecessor \"root\" is not structurally executable") {
		t.Fatalf("dirty graph repair bypassed committed predecessor validation: err = %v out = %+v", err, out)
	}
}

func TestChangeCheckRequireExecutableRejectsMissingPredecessor(t *testing.T) {
	repo := initCLIGitRepo(t)
	child := writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "missing", ""))
	out, err := runChangeCheckJSON(t, repo, child, "--require-executable")
	if err == nil || !findingsContain(out.Gaps, "predecessor \"missing\" is not materialized") {
		t.Fatalf("err = %v findings = %v", err, out.Findings)
	}
}

func TestChangeCheckBareReportsMissingPredecessorAsExecutionGap(t *testing.T) {
	repo := initCLIGitRepo(t)
	child := writeChangeFolder(t, repo, "20260710-child", executableLineageDoc("child", "line", "missing", ""))
	bare, err := runChangeCheckJSON(t, repo, child)
	if err != nil || !bare.Passed || bare.Executable || !findingsContain(bare.Gaps, "predecessor \"missing\" is not materialized") {
		t.Fatalf("bare check err = %v out = %+v", err, bare)
	}
	required, err := runChangeCheckJSON(t, repo, child, "--require-executable")
	if err == nil || required.Passed || required.Executable || !findingsContain(required.Gaps, "predecessor \"missing\" is not materialized") {
		t.Fatalf("required check err = %v out = %+v", err, required)
	}
}

func TestChangeCheckRequireExecutableTraversesCommittedThreeNodeChain(t *testing.T) {
	repo := initCLIGitRepo(t)
	root := writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "terminal"))
	terminal := writeChangeFolder(t, repo, "20260710-terminal", executableLineageDoc("terminal", "line", "", ""))
	out, err := runChangeCheckJSON(t, repo, terminal, "--require-executable")
	if err == nil || !findingsContain(out.Findings, "multiple roots") {
		t.Fatalf("stale terminal err = %v out = %+v", err, out)
	}
	writeChangeFolder(t, repo, "20260710-terminal", executableLineageDoc("terminal", "line", "middle", ""))
	out, err = runChangeCheckJSON(t, repo, terminal, "--require-executable")
	if err == nil || !findingsContain(out.Gaps, "predecessor \"middle\" is not materialized") {
		t.Fatalf("absent middle err = %v out = %+v", err, out)
	}
	middle := writeChangeFolder(t, repo, "20260710-middle", executableLineageDoc("middle", "line", "root", ""))
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	gitCLI(t, repo, "add", filepath.Join(middle, "change.md"))
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "commit", "-m", "docs: add middle only")
	out, err = runChangeCheckJSON(t, repo, terminal, "--require-executable")
	if err == nil || !findingsContain(out.Gaps, "predecessor \"root\" is not committed and retained in HEAD") {
		t.Fatalf("absent committed root err = %v out = %+v", err, out)
	}
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "terminal"))
	commitAllChangeTest(t, repo, "docs: complete lineage")
	out, err = runChangeCheckJSON(t, repo, terminal, "--require-executable")
	if err != nil || !out.Executable {
		t.Fatalf("complete chain err = %v out = %+v", err, out)
	}
}

func TestChangeListFindsRetainedLineageWithoutBranch(t *testing.T) {
	t.Skip("retired: change list --lineage replaced by units/cohort projection (TASK-004)")
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	repo := initCLIGitRepo(t)
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "terminal"))
	commitAllChangeTest(t, repo, "docs: retain root")
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: repo, StateHome: t.TempDir()}.Run([]string{"change", "list", "--lineage", "line", "--json"})
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if !strings.Contains(stdout.String(), "\"root\"") || !strings.Contains(stdout.String(), "\"journalAvailable\": false") {
		t.Fatalf("list output = %s", stdout.String())
	}
}

func TestChangeListReadsExactScopeDecisionWithoutMutatingState(t *testing.T) {
	t.Skip("retired: change list --lineage replaced by units/cohort projection (TASK-004)")
	ctx := context.Background()
	repo := initCLIGitRepo(t)
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	t.Setenv("LOAF_DB", databasePath)
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "terminal"))
	commitAllChangeTest(t, repo, "docs: retain root")
	projectRoot, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Initialize(ctx, projectRoot, state.PathResolver{}); err != nil {
		t.Fatal(err)
	}
	const decision = "root then terminal; no release between nodes"
	if _, err := state.LogJournal(ctx, projectRoot, state.PathResolver{}, state.JournalLogOptions{Entry: "decision(lineage/line): " + decision}); err != nil {
		t.Fatal(err)
	}
	beforeBytes, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeProjects, beforePaths := changeTestIdentityCounts(t, databasePath)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "list", "--lineage", "line", "--json"}); err != nil {
		t.Fatalf("change list error = %v", err)
	}
	var result changeListJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", stdout.String(), err)
	}
	if !result.JournalAvailable || result.LineageDecision != decision {
		t.Fatalf("result = %+v, want exact-scope journal decision", result)
	}
	afterBytes, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	afterProjects, afterPaths := changeTestIdentityCounts(t, databasePath)
	if !bytes.Equal(beforeBytes, afterBytes) || beforeProjects != afterProjects || beforePaths != afterPaths {
		t.Fatalf("change list mutated state: bytes_equal=%t projects=%d->%d paths=%d->%d", bytes.Equal(beforeBytes, afterBytes), beforeProjects, afterProjects, beforePaths, afterPaths)
	}
}

func TestChangeListWarnsWhenJournalEnrichmentReadFails(t *testing.T) {
	t.Skip("retired: change list --lineage replaced by units/cohort projection (TASK-004)")
	repo := initCLIGitRepo(t)
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	t.Setenv("LOAF_DB", databasePath)
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "root"))
	if err := os.WriteFile(databasePath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	var jsonOutput bytes.Buffer
	if err := (Runner{Stdout: &jsonOutput, WorkingDir: repo}).Run([]string{"change", "list", "--lineage", "line", "--json"}); err != nil {
		t.Fatalf("change list --json error = %v", err)
	}
	var result changeListJSON
	if err := json.Unmarshal(jsonOutput.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.JournalAvailable || len(result.Warnings) != 1 || result.Warnings[0] != changeListJournalReadWarning {
		t.Fatalf("result = %+v, want deterministic journal read warning", result)
	}
	var human bytes.Buffer
	if err := (Runner{Stdout: &human, WorkingDir: repo}).Run([]string{"change", "list", "--lineage", "line"}); err != nil {
		t.Fatalf("change list error = %v", err)
	}
	if !strings.Contains(human.String(), "warning: "+changeListJournalReadWarning) {
		t.Fatalf("human output = %q, want visible warning", human.String())
	}
}

func changeTestIdentityCounts(t *testing.T, databasePath string) (int, int) {
	t.Helper()
	database, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(databasePath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var projects, paths int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projects); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM project_paths`).Scan(&paths); err != nil {
		t.Fatal(err)
	}
	return projects, paths
}

func TestChangeListJSONIsRelativeAndByteDeterministicAfterBranchRenameAndDelete(t *testing.T) {
	t.Skip("retired: change list --lineage replaced by units/cohort projection (TASK-004)")
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	repo := initCLIGitRepo(t)
	gitCLI(t, repo, "switch", "-c", "lineage-work")
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "root"))
	commitAllChangeTest(t, repo, "docs: retain lineage")
	gitCLI(t, repo, "branch", "-m", "renamed-lineage-work")
	gitCLI(t, repo, "switch", "main")
	gitCLI(t, repo, "merge", "--ff-only", "renamed-lineage-work")
	gitCLI(t, repo, "branch", "-D", "renamed-lineage-work")
	run := func() string {
		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: repo, StateHome: t.TempDir()}).Run([]string{"change", "list", "--lineage", "line", "--json"}); err != nil {
			t.Fatal(err)
		}
		return stdout.String()
	}
	first, second := run(), run()
	if first != second {
		t.Fatalf("repeated JSON differs:\n%s\n%s", first, second)
	}
	var decoded changeListJSON
	if err := json.Unmarshal([]byte(first), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Nodes) != 1 || decoded.Nodes[0].Folder != "docs/changes/20260710-root" || strings.Contains(first, repo) {
		t.Fatalf("decoded = %+v output = %s", decoded, first)
	}
}

// --- V1: violations, exit non-zero -----------------------------------------

// V1(a): status-like frontmatter keys.
func TestChangeCheckV1RejectsStatusLikeKeys(t *testing.T) {
	for _, key := range []string{"readiness", "status", "state"} {
		t.Run(key, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				key + ": whatever",
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError", err)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for status-like key %q", key)
			}
			if !findingsContain(out.Findings, key) {
				t.Fatalf("findings = %v, want mention of banned key %q", out.Findings, key)
			}
		})
	}
}

// V1(a): progress vocabulary as a frontmatter value in any field.
func TestChangeCheckV1RejectsProgressVocabularyValues(t *testing.T) {
	for _, value := range []string{"active", "in-progress", "done", "archived"} {
		t.Run(value, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				"phase: " + value,
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for progress value %q", value, err)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for progress value %q", value)
			}
			if !findingsContain(out.Findings, value) {
				t.Fatalf("findings = %v, want mention of progress value %q", out.Findings, value)
			}
		})
	}
}

// V1(a): the full canonical change-state vocabulary (Decision 22) plus released
// is banned as a frontmatter value under ANY key. This is external review round
// 4's exact probe set — each canonical state (and released) stored under an
// arbitrary key that is not itself status-like.
func TestChangeCheckV1RejectsCanonicalStateVocabularyUnderArbitraryKeys(t *testing.T) {
	cases := []struct{ key, value string }{
		{"phase", "shaping"},
		{"queue", "backlog"},
		{"lane", "todo"},
		{"review_phase", "review"},
		{"merge_state", "merged"},
		{"release_state", "released"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				tc.key + ": " + tc.value,
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for %s: %s", err, tc.key, tc.value)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for %q stored under key %q", tc.value, tc.key)
			}
			if !findingsContain(out.Findings, tc.value) || !findingsContain(out.Findings, tc.key) {
				t.Fatalf("findings = %v, want mention of banned value %q under key %q", out.Findings, tc.value, tc.key)
			}
		})
	}
}

// V1(a): matching is case-insensitive on the normalized value — underscores and
// spaces normalize to hyphens, so "In Progress" and "in_progress" both match the
// canonical "in-progress".
func TestChangeCheckV1RejectsStateVocabularyRegardlessOfCasingOrSeparator(t *testing.T) {
	for _, value := range []string{"In Progress", "in_progress", "IN-PROGRESS", "Merged", "BACKLOG"} {
		t.Run(value, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			fm := strings.Join([]string{
				"---",
				"change: demo",
				"created: 2026-07-04",
				"branch: demo",
				"phase: " + value,
				"---",
			}, "\n")
			folder := writeChangeFolder(t, repo, "20260704-demo", changeDoc(fm, productSections()...))

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for normalized state value %q", err, value)
			}
			if out.Passed {
				t.Fatalf("passed = true, want false for normalized state value %q", value)
			}
		})
	}
}

// V1(a) exemption: identity fields (change, created, branch) are exempt from the
// state-vocabulary ban — their semantics are checked elsewhere. A branch is a git
// ref that may legitimately be named after a state word, so branch: review passes.
func TestChangeCheckV1AllowsBranchNamedLikeState(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "review"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil; branch: review is an identity field, exempt from the state ban", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true; a branch named after a state word must not be a violation. findings = %v", out.Findings)
	}
}

// V1(b): frontmatter must open the file at byte one.
func TestChangeCheckV1RejectsFrontmatterNotAtByteOne(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", "\n"+body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	if !findingsContain(out.Findings, "byte one") {
		t.Fatalf("findings = %v, want byte-one violation", out.Findings)
	}
}

// V1(c): malformed folder name.
func TestChangeCheckV1RejectsMalformedFolderName(t *testing.T) {
	cases := map[string]string{
		"no-date-prefix": "shape-first",
		"bad-date":       "2026-07-first",
		"uppercase-slug": "20260704-Demo",
		"underscore":     "20260704-de_mo",
	}
	for name, folderName := range cases {
		t.Run(name, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
			folder := writeChangeFolder(t, repo, folderName, body)

			out, err := runChangeCheckJSON(t, repo, folder)
			var exitErr ExitError
			if !errors.As(err, &exitErr) || exitErr.Code == 0 {
				t.Fatalf("err = %v, want non-zero ExitError for folder %q", folderName, err)
			}
			if !findingsContain(out.Findings, "folder name") {
				t.Fatalf("findings = %v, want folder-name violation", out.Findings)
			}
		})
	}
}

// V1(d): identity mismatch — change: vs folder slug.
func TestChangeCheckV1RejectsSlugMismatch(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("other", "2026-07-04", "other"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	if !findingsContain(out.Findings, "change:") {
		t.Fatalf("findings = %v, want change/slug mismatch", out.Findings)
	}
}

// V1(d): identity mismatch — created: date vs folder date prefix.
func TestChangeCheckV1RejectsCreatedDateMismatch(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-05", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	if !findingsContain(out.Findings, "created:") {
		t.Fatalf("findings = %v, want created/date mismatch", out.Findings)
	}
}

// V1(e): missing Product Contract sections.
func TestChangeCheckV1RejectsMissingProductSections(t *testing.T) {
	repo := initCLIGitRepo(t)
	// Only Problem present; the other four product sections missing.
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), "## Problem\n\nThe friction.")
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError", err)
	}
	for _, want := range []string{"Hypothesis", "Scope", "Observable Workflow", "Rabbit Holes and No-Gos"} {
		if !findingsContain(out.Findings, want) {
			t.Fatalf("findings = %v, want missing-section mention of %q", out.Findings, want)
		}
	}
}

// --- V2: report, exit zero for a valid shaping-stage document ----------------

// A document with only the product sections is valid (exit zero) but reported
// non-executable, with the missing tail sections listed as gaps.
func TestChangeCheckV2ShapingStagePassesButNotExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("shaping-stage check err = %v, want nil (exit zero)", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true for a valid shaping-stage document")
	}
	if out.Executable {
		t.Fatalf("executable = true, want false for product-only document")
	}
	for _, want := range []string{"Planning Contract", "Implementation Units", "Verification Contract", "Definition of Done"} {
		if !findingsContain(out.Gaps, want) {
			t.Fatalf("gaps = %v, want gap for %q", out.Gaps, want)
		}
	}
}

// V2: executability derivation with a single gap — an empty tail section counts
// as a gap even though the heading is present.
func TestChangeCheckV2ExecutabilityGapForEmptySection(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- V1. exits non-zero.",
		"## Definition of Done\n", // present but empty
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil (exit zero, empty section is a gap not a violation)", err)
	}
	if out.Executable {
		t.Fatalf("executable = true, want false when Definition of Done is empty")
	}
	if !findingsContain(out.Gaps, "Definition of Done") {
		t.Fatalf("gaps = %v, want Definition of Done gap", out.Gaps)
	}
	if findingsContain(out.Gaps, "Planning Contract") {
		t.Fatalf("gaps = %v, should not flag the non-empty Planning Contract", out.Gaps)
	}
}

// V2: a fully-populated document reports executable with no gaps.
func TestChangeCheckV2FullDocumentIsExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(), executableSections()...)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !out.Passed || !out.Executable {
		t.Fatalf("passed=%v executable=%v, want both true", out.Passed, out.Executable)
	}
	if len(out.Gaps) != 0 {
		t.Fatalf("gaps = %v, want none", out.Gaps)
	}
}

// --- V2 placeholder discounting: authored content, not scaffolding -----------

// TestChangeCheckFreshInitNotExecutable is the traceable resolution of the U3
// finding: a Change scaffolded by `loaf change init` used to read
// executable:yes because the template's bracket placeholders counted as content
// (the literal "present and non-empty" reading let a placeholder-only document
// satisfy the V3 gate). With V2 discounting placeholders and comments, a fresh
// Change reads not-executable — its tail is scaffolding, not authored content.
//
// Every tail section ships as scaffolding — bracket placeholders and HTML
// comments only, no bare labels — so all four executable sections read as gaps
// on a fresh init and none is executable until the author writes real content.
func TestChangeCheckFreshInitNotExecutable(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "fresh-demo"}); err != nil {
		t.Fatalf("change init error = %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-fresh-demo")

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("check on freshly-init'd change err = %v, want nil (shaping-stage is valid, exit zero)", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true; a freshly-init'd Change has no violations. findings = %v", out.Findings)
	}
	if out.Executable {
		t.Fatalf("executable = true, want false; a freshly-templated Change has no authored tail. gaps = %v", out.Gaps)
	}
	for _, want := range []string{"Planning Contract", "Implementation Units", "Verification Contract", "Definition of Done"} {
		if !findingsContain(out.Gaps, want) {
			t.Fatalf("gaps = %v, want scaffolding-only tail section %q listed as a gap", out.Gaps, want)
		}
	}
}

// A tail section mixing a placeholder line with one authored line is non-empty:
// discounting removes only the placeholder, and any real content keeps the
// section authored.
func TestChangeCheckSectionMixedPlaceholderIsAuthored(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nReal shaping prose.",
		"## Implementation Units\n\n- [What this Change delivers.]\n- Wire the token rotation job.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if findingsContain(out.Gaps, "Implementation Units") {
		t.Fatalf("gaps = %v, must not flag Implementation Units; it carries an authored line beside the placeholder", out.Gaps)
	}
	if !out.Executable {
		t.Fatalf("executable = false, want true; every tail section is authored. gaps = %v", out.Gaps)
	}
}

// A tail section whose only content is an HTML comment reads as empty — comments
// are guidance, not authored content.
func TestChangeCheckSectionOnlyHTMLCommentIsEmpty(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nReal shaping prose.",
		"## Implementation Units\n\n- Ship the thing.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n<!-- fill this in before the draft to ready flip -->",
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if out.Executable {
		t.Fatalf("executable = true, want false; Definition of Done holds only a comment. gaps = %v", out.Gaps)
	}
	if !findingsContain(out.Gaps, "Definition of Done") {
		t.Fatalf("gaps = %v, want a Definition of Done gap for a comment-only section", out.Gaps)
	}
}

// Decision pinned: a surviving alphanumeric label (e.g. **U1**) after
// bracket-discounting is authored content. The V2 clause left "structure with
// only placeholder content" to implementation; the deterministic rule strips
// placeholder spans and comments, never bare labels, so a bullet like
// `- **U1 — [Unit name].** [What it delivers.]` reads as authored via its label.
func TestChangeCheckStructuralLabelCountsAsAuthored(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nReal shaping prose.",
		"## Implementation Units\n\n- **U1 — [Unit name].** [What it delivers.]",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if findingsContain(out.Gaps, "Implementation Units") {
		t.Fatalf("gaps = %v, Implementation Units should read authored: the **U1** label survives discounting", out.Gaps)
	}
	if !out.Executable {
		t.Fatalf("executable = false, want true; every tail section is authored. gaps = %v", out.Gaps)
	}
}

// --- V3: --require-executable turns the report into a gate -------------------

func TestChangeCheckV3RequireExecutableFailsOnShapingDoc(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder, "--require-executable")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("err = %v, want non-zero ExitError with --require-executable on a shaping doc", err)
	}
	if out.Passed {
		t.Fatalf("passed = true, want false under --require-executable when not executable")
	}
}

func TestChangeCheckV3RequireExecutablePassesOnFullDoc(t *testing.T) {
	repo := initCLIGitRepo(t)
	sections := append(productSections(), executableSections()...)
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "demo"), sections...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder, "--require-executable")
	if err != nil {
		t.Fatalf("err = %v, want nil for an executable document under --require-executable", err)
	}
	if !out.Passed || !out.Executable {
		t.Fatalf("passed=%v executable=%v, want both true", out.Passed, out.Executable)
	}
}

// --- Branch mismatch is a warning, never a violation ------------------------

func TestChangeCheckBranchMismatchIsWarningNotViolation(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "some-other-branch"), productSections()...)
	folder := writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v, want nil (branch mismatch is a warning)", err)
	}
	if !out.Passed {
		t.Fatalf("passed = false, want true; branch mismatch must not be a violation")
	}
	if len(out.Warnings) == 0 {
		t.Fatalf("warnings = %v, want a branch-mismatch warning", out.Warnings)
	}
	if !findingsContain(out.Warnings, "branch") {
		t.Fatalf("warnings = %v, want branch-mismatch mention", out.Warnings)
	}
}

// --- Folder resolution by branch --------------------------------------------

func TestChangeCheckResolvesByBranch(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-demo", body)

	out, err := runChangeCheckJSON(t, repo) // no positional path
	if err != nil {
		t.Fatalf("err = %v, want nil resolving by branch", err)
	}
	if !strings.Contains(out.Folder, "20260704-demo") {
		t.Fatalf("folder = %q, want the branch-matched folder", out.Folder)
	}
}

func TestChangeCheckByBranchErrorsWhenNoMatch(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "not-main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-demo", body)

	var stdout, stderr bytes.Buffer
	err := Runner{Stdout: &stdout, Stderr: &stderr, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want an error telling the user to pass a path")
	}
}

func TestChangeCheckByBranchErrorsWhenManyMatch(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	body := changeDoc(changeFrontmatter("demo", "2026-07-04", "main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-demo", body)
	body2 := changeDoc(changeFrontmatter("other", "2026-07-04", "main"), productSections()...)
	writeChangeFolder(t, repo, "20260704-other", body2)

	err := Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want an error when multiple folders match the branch")
	}
}

// The no-match error lists every discovered Change folder with its branch: value
// so the user can pick a path — a bare check on a branch with no matching Change
// (e.g. main right after init) is a dead end otherwise.
func TestChangeCheckByBranchNoMatchListsAvailableFolders(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	writeChangeFolder(t, repo, "20260704-demo",
		changeDoc(changeFrontmatter("demo", "2026-07-04", "feat-one"), productSections()...))
	writeChangeFolder(t, repo, "20260705-other",
		changeDoc(changeFrontmatter("other", "2026-07-05", "feat-two"), productSections()...))

	err := Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want a no-match error listing available folders")
	}
	msg := err.Error()
	for _, want := range []string{
		"no change folder matches branch",
		"available change folders:",
		"20260704-demo", "branch: feat-one",
		"20260705-other", "branch: feat-two",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want it to contain %q", msg, want)
		}
	}
}

// The ambiguous-match error lists the candidate folders too.
func TestChangeCheckByBranchManyMatchListsAvailableFolders(t *testing.T) {
	repo := initCLIGitRepo(t) // current branch is main
	writeChangeFolder(t, repo, "20260704-demo",
		changeDoc(changeFrontmatter("demo", "2026-07-04", "main"), productSections()...))
	writeChangeFolder(t, repo, "20260704-other",
		changeDoc(changeFrontmatter("other", "2026-07-04", "main"), productSections()...))

	err := Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "check"})
	if err == nil {
		t.Fatalf("err = nil, want an ambiguous-match error listing available folders")
	}
	msg := err.Error()
	for _, want := range []string{
		"multiple change folders match branch",
		"available change folders:",
		"20260704-demo", "20260704-other",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want it to contain %q", msg, want)
		}
	}
}

// --- init: happy path, refuse existing, bad slug ---------------------------

func TestChangeInitHappyPath(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "init", "auth-token-rotation"}); err != nil {
		t.Fatalf("change init error = %v", err)
	}

	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-auth-token-rotation")
	jsonData, err := os.ReadFile(filepath.Join(folder, "change.json"))
	if err != nil {
		t.Fatalf("ReadFile(change.json) error = %v", err)
	}
	jsonContent := string(jsonData)
	for _, want := range []string{
		`"change": "auth-token-rotation"`,
		`"created": "` + time.Now().Format("2006-01-02") + `"`,
		`"branch": "auth-token-rotation"`,
	} {
		if !strings.Contains(jsonContent, want) {
			t.Fatalf("change.json = %q, want stamped %q", jsonContent, want)
		}
	}
	shape, err := os.ReadFile(filepath.Join(folder, "shape.md"))
	if err != nil {
		t.Fatalf("ReadFile(shape.md) error = %v", err)
	}
	if !strings.Contains(string(shape), "[Change Title]") {
		t.Fatalf("shape.md dropped body placeholders:\n%s", shape)
	}
	if _, err := os.Stat(filepath.Join(folder, "tasks")); err != nil {
		t.Fatalf("tasks/ missing after init: %v", err)
	}
	seedPath := filepath.Join(folder, "tasks", changeSeedTaskFile)
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("seeded task packet missing: %v", err)
	}
	seedBody := string(seed)
	for _, want := range []string{
		"change: auth-token-rotation",
		"id: TASK-001",
		"- [ ]",
		"[short title]",
		"expected to rename",
	} {
		if !strings.Contains(seedBody, want) {
			t.Fatalf("seeded packet missing %q:\n%s", want, seedBody)
		}
	}
	if strings.Contains(seedBody, "- [x]") || strings.Contains(seedBody, "- [X]") {
		t.Fatalf("seeded packet must keep boxes unchecked:\n%s", seedBody)
	}
	if _, err := os.Stat(filepath.Join(folder, "tasks", ".gitkeep")); !os.IsNotExist(err) {
		t.Fatalf(".gitkeep must not be written; err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "brief.md")); !os.IsNotExist(err) {
		t.Fatalf("brief.md should not exist on full scaffold; err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "change.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy change.md should not exist on new scaffold; err=%v", err)
	}
	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("check on freshly-init'd change err = %v out=%+v, want nil", err, out)
	}
	if !out.Passed {
		t.Fatalf("fresh scaffold should pass check, findings=%v", out.Findings)
	}
	if out.Executable {
		t.Fatalf("fresh scaffold must stay non-executable until authored; gaps=%v", out.Gaps)
	}
	if len(out.Notices) != 0 {
		t.Fatalf("fresh scaffold must carry no deprecation notice; notices=%v", out.Notices)
	}

	var tasksOut bytes.Buffer
	if err := (Runner{Stdout: &tasksOut, WorkingDir: repo}).Run([]string{"change", "tasks", folder, "--json"}); err != nil {
		t.Fatalf("change tasks error = %v", err)
	}
	var tasksResult changeTasksJSON
	if err := json.Unmarshal(tasksOut.Bytes(), &tasksResult); err != nil {
		t.Fatalf("Unmarshal tasks: %v\n%s", err, tasksOut.String())
	}
	if len(tasksResult.Tasks) != 1 || tasksResult.Tasks[0].ID != "TASK-001" {
		t.Fatalf("tasks = %+v, want single TASK-001", tasksResult.Tasks)
	}
	if tasksResult.Tasks[0].Complete {
		t.Fatalf("seeded TASK-001 must have derived completion false")
	}
	if tasksResult.Tasks[0].CheckboxTotal < 1 || tasksResult.Tasks[0].CheckboxDone != 0 {
		t.Fatalf("seeded checkboxes = done %d / total %d, want unchecked", tasksResult.Tasks[0].CheckboxDone, tasksResult.Tasks[0].CheckboxTotal)
	}
}

func TestChangeInitBriefMode(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "captured-ask", "--brief"}); err != nil {
		t.Fatalf("change init --brief error = %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-captured-ask")
	if _, err := os.Stat(filepath.Join(folder, "change.json")); err != nil {
		t.Fatalf("change.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "brief.md")); err != nil {
		t.Fatalf("brief.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "shape.md")); !os.IsNotExist(err) {
		t.Fatalf("shape.md must be absent in brief mode; err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "tasks")); !os.IsNotExist(err) {
		t.Fatalf("tasks/ must be absent in brief mode; err=%v", err)
	}
	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("brief-only check should not violate: err=%v out=%+v", err, out)
	}
	if out.Executable {
		t.Fatalf("brief-only folder must be non-executable")
	}
	if !findingsContain(out.Gaps, "shape.md") {
		t.Fatalf("brief-only should report shape.md gap, gaps=%v", out.Gaps)
	}
}

// Scaffolded --brief output carries the shared problem-space skeleton headings
// (pitch-entrypoint brief contract). Headings are the contract; placeholder
// prose may evolve without this test churning.
func TestChangeInitBriefCarriesSkeletonHeadings(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "skeleton-brief", "--brief"}); err != nil {
		t.Fatalf("change init --brief error = %v", err)
	}
	today := time.Now().Format("20060102")
	briefPath := filepath.Join(repo, "docs", "changes", today+"-skeleton-brief", "brief.md")
	body, err := os.ReadFile(briefPath)
	if err != nil {
		t.Fatalf("ReadFile(brief.md) error = %v", err)
	}
	content := string(body)
	for _, want := range []string{
		"## Problem Statement",
		"## Who Has It",
		"## Current Alternatives",
		"## Value Proposition",
		"## Constraints",
		"## Sequencing and Relationships",
		"## Sources and Research Links",
		"## Open Questions",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("scaffolded brief.md missing skeleton heading %q\n%s", want, content)
		}
	}
	// Supersession/accretion comment still present (Decision 7).
	for _, want := range []string{
		"accrete",
		"freezes when shape.md exists",
		"never mechanically load-bearing",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("scaffolded brief.md missing accretion/supersession phrase %q\n%s", want, content)
		}
	}
}

// init's success output carries a next-steps hint: work happens on branch
// <slug>, so create/switch to it or pass the folder path to check explicitly.
// Without it, `loaf change init` on main followed by a bare `loaf change check`
// dead-ends on "no change folder matches branch main".
func TestChangeInitEmitsNextStepsHint(t *testing.T) {
	repo := initCLIGitRepo(t)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "init", "auth-token-rotation"}); err != nil {
		t.Fatalf("change init error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`branch "auth-token-rotation"`,
		"git switch -c auth-token-rotation",
		"loaf change check",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("init output = %q, want next-steps hint containing %q", output, want)
		}
	}
}

func TestChangeInitRefusesExistingFolder(t *testing.T) {
	repo := initCLIGitRepo(t)
	today := time.Now().Format("20060102")
	existing := filepath.Join(repo, "docs", "changes", today+"-demo")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "demo"})
	if err == nil {
		t.Fatalf("err = nil, want refusal for an existing change folder")
	}
	if !strings.Contains(err.Error(), "exists") {
		t.Fatalf("err = %v, want an 'exists' message", err)
	}
}

// --- Captured-folder promotion (TASK-006 / Decision 12) ---------------------

func changeInitCaptureFolder(t *testing.T, repo, slug string) string {
	t.Helper()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", slug, "--brief"}); err != nil {
		t.Fatalf("change init --brief error = %v", err)
	}
	today := time.Now().Format("20060102")
	return filepath.Join(repo, "docs", "changes", today+"-"+slug)
}

func readChangeFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return body
}

func stampTargetReleaseOnChangeJSON(t *testing.T, folder, version string) {
	t.Helper()
	path := filepath.Join(folder, "change.json")
	var meta map[string]string
	if err := json.Unmarshal(readChangeFileBytes(t, path), &meta); err != nil {
		t.Fatalf("Unmarshal change.json: %v", err)
	}
	meta["target_release"] = version
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("Marshal change.json: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile change.json: %v", err)
	}
}

func TestChangeInitPromotesCaptureOnlyFolder(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := changeInitCaptureFolder(t, repo, "promo-happy")
	stampTargetReleaseOnChangeJSON(t, folder, "2.1.0")
	briefBefore := readChangeFileBytes(t, filepath.Join(folder, "brief.md"))
	jsonBefore := readChangeFileBytes(t, filepath.Join(folder, "change.json"))

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "init", "promo-happy"}); err != nil {
		t.Fatalf("promotion init error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Promoted capture") {
		t.Fatalf("promotion output = %q, want Promoted capture message distinct from Created change", out)
	}
	if strings.Contains(out, "Created change:") {
		t.Fatalf("promotion must not print fresh-scaffold Created change; got %q", out)
	}

	if !bytes.Equal(briefBefore, readChangeFileBytes(t, filepath.Join(folder, "brief.md"))) {
		t.Fatalf("brief.md bytes mutated by promotion")
	}
	if !bytes.Equal(jsonBefore, readChangeFileBytes(t, filepath.Join(folder, "change.json"))) {
		t.Fatalf("change.json bytes mutated by promotion")
	}
	if _, err := os.Stat(filepath.Join(folder, "shape.md")); err != nil {
		t.Fatalf("shape.md missing after promotion: %v", err)
	}
	seedPath := filepath.Join(folder, "tasks", changeSeedTaskFile)
	gotSeed := readChangeFileBytes(t, seedPath)
	wantSeed := []byte(stampChangeTaskSeed(changeTaskTemplate, "promo-happy"))
	if !bytes.Equal(gotSeed, wantSeed) {
		t.Fatalf("seed task mismatch after promotion")
	}

	check, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("check after promotion err = %v out=%+v", err, check)
	}
	if check.State == "captured" || check.Captured {
		t.Fatalf("after promotion want shaped-or-better, got state=%q captured=%v", check.State, check.Captured)
	}
	if check.State != "shaped" && check.State != "executable" && check.State != "executing" && check.State != "complete" && check.State != "verified" {
		t.Fatalf("after promotion unexpected state %q", check.State)
	}

	// Fully-materialized folder still rejects.
	err = Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-happy"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("materialized re-init err = %v, want already exists", err)
	}
	if !bytes.Equal(briefBefore, readChangeFileBytes(t, filepath.Join(folder, "brief.md"))) {
		t.Fatalf("brief.md mutated by rejected re-init")
	}
}

func TestChangeInitResumesPartialPromotion(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := changeInitCaptureFolder(t, repo, "promo-resume")
	briefBefore := readChangeFileBytes(t, filepath.Join(folder, "brief.md"))
	jsonBefore := readChangeFileBytes(t, filepath.Join(folder, "change.json"))

	// Simulate interruption after seed publish, before shape.md marker rename.
	tasksDir := filepath.Join(folder, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("MkdirAll tasks: %v", err)
	}
	seedBody := []byte(stampChangeTaskSeed(changeTaskTemplate, "promo-resume"))
	if err := os.WriteFile(filepath.Join(tasksDir, changeSeedTaskFile), seedBody, 0o644); err != nil {
		t.Fatalf("WriteFile seed: %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "init", "promo-resume"}); err != nil {
		t.Fatalf("resume init error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Resumed capture promotion") {
		t.Fatalf("resume output = %q, want Resumed capture promotion", stdout.String())
	}
	if !bytes.Equal(briefBefore, readChangeFileBytes(t, filepath.Join(folder, "brief.md"))) {
		t.Fatalf("brief.md mutated by resume")
	}
	if !bytes.Equal(jsonBefore, readChangeFileBytes(t, filepath.Join(folder, "change.json"))) {
		t.Fatalf("change.json mutated by resume")
	}
	if !bytes.Equal(seedBody, readChangeFileBytes(t, filepath.Join(tasksDir, changeSeedTaskFile))) {
		t.Fatalf("seed task overwritten by resume")
	}
	if _, err := os.Stat(filepath.Join(folder, "shape.md")); err != nil {
		t.Fatalf("shape.md missing after resume: %v", err)
	}
	check, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("check after resume err = %v", err)
	}
	if check.State == "captured" {
		t.Fatalf("after resume still captured")
	}
}

func TestChangeInitPromotionSurvivesStrayTempsAndPartialWrites(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := changeInitCaptureFolder(t, repo, "promo-temps")
	tasksDir := filepath.Join(folder, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("MkdirAll tasks: %v", err)
	}
	// Partial task write (temp only) and partial marker write (temp only).
	if err := os.WriteFile(filepath.Join(tasksDir, changePublishTempPrefix+"seed partial"), []byte("half-written seed"), 0o644); err != nil {
		t.Fatalf("WriteFile partial seed temp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, changePublishTempPrefix+"shape partial"), []byte("half-written shape"), 0o644); err != nil {
		t.Fatalf("WriteFile partial shape temp: %v", err)
	}
	// Also a half-written seed destination must not exist; only temps.

	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "promo-temps"}); err != nil {
		t.Fatalf("promotion with stray temps error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "shape.md")); err != nil {
		t.Fatalf("shape.md missing: %v", err)
	}
	gotSeed := readChangeFileBytes(t, filepath.Join(tasksDir, changeSeedTaskFile))
	wantSeed := []byte(stampChangeTaskSeed(changeTaskTemplate, "promo-temps"))
	if !bytes.Equal(gotSeed, wantSeed) {
		t.Fatalf("seed incomplete after promotion past temps")
	}
	check, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("check err = %v", err)
	}
	if check.State == "captured" {
		t.Fatalf("stray temps stranded folder as captured or unresumable")
	}
}

func TestChangeInitPromotionFailClosedMatrix(t *testing.T) {
	t.Run("repeated-brief", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		folder := changeInitCaptureFolder(t, repo, "promo-rebrief")
		before := listChangeFolderSnapshot(t, folder)
		err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-rebrief", "--brief"})
		if err == nil {
			t.Fatalf("err = nil, want repeated --brief refusal")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("err = %v, want already exists", err)
		}
		assertChangeFolderUntouched(t, folder, before)
	})

	t.Run("missing-brief", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		today := time.Now().Format("20060102")
		folder := filepath.Join(repo, "docs", "changes", today+"-promo-jsononly")
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := writeChangeJSON(filepath.Join(folder, "change.json"), "promo-jsononly", time.Now()); err != nil {
			t.Fatalf("writeChangeJSON: %v", err)
		}
		before := listChangeFolderSnapshot(t, folder)
		err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-jsononly"})
		if err == nil || !strings.Contains(err.Error(), "brief.md is missing") {
			t.Fatalf("err = %v, want missing brief", err)
		}
		assertChangeFolderUntouched(t, folder, before)
	})

	t.Run("hybrid-layout", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		folder := changeInitCaptureFolder(t, repo, "promo-hybrid")
		if err := os.WriteFile(filepath.Join(folder, "change.md"), []byte("---\nchange: promo-hybrid\n---\n"), 0o644); err != nil {
			t.Fatalf("WriteFile change.md: %v", err)
		}
		before := listChangeFolderSnapshot(t, folder)
		err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-hybrid"})
		if err == nil || !strings.Contains(err.Error(), "hybrid") {
			t.Fatalf("err = %v, want hybrid refusal", err)
		}
		assertChangeFolderUntouched(t, folder, before)
	})

	t.Run("invalid-schema", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		folder := changeInitCaptureFolder(t, repo, "promo-badmeta")
		if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{"change":"promo-badmeta","created":"2026-07-30","branch":"promo-badmeta","status":"todo"}`+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile change.json: %v", err)
		}
		before := listChangeFolderSnapshot(t, folder)
		err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-badmeta"})
		if err == nil || !strings.Contains(err.Error(), "invalid change.json") {
			t.Fatalf("err = %v, want invalid change.json", err)
		}
		assertChangeFolderUntouched(t, folder, before)
	})

	t.Run("diverged-tasks", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		folder := changeInitCaptureFolder(t, repo, "promo-diverged")
		tasksDir := filepath.Join(folder, "tasks")
		if err := os.MkdirAll(tasksDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tasksDir, changeSeedTaskFile), []byte("not the seed\n"), 0o644); err != nil {
			t.Fatalf("WriteFile diverged seed: %v", err)
		}
		before := listChangeFolderSnapshot(t, folder)
		err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-diverged"})
		if err == nil || !strings.Contains(err.Error(), "diverged") {
			t.Fatalf("err = %v, want diverged tasks refusal", err)
		}
		assertChangeFolderUntouched(t, folder, before)
	})

	t.Run("materialized", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "promo-full"}); err != nil {
			t.Fatalf("full init: %v", err)
		}
		today := time.Now().Format("20060102")
		folder := filepath.Join(repo, "docs", "changes", today+"-promo-full")
		before := listChangeFolderSnapshot(t, folder)
		err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "init", "promo-full"})
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("err = %v, want already exists for materialized", err)
		}
		assertChangeFolderUntouched(t, folder, before)
	})
}

func TestPublishChangeFileExclusiveRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shape.md")
	if err := os.WriteFile(dest, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err := publishChangeFileExclusive(dest, []byte("replacement\n"))
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("err = %v, want refusing to overwrite", err)
	}
	if got := string(readChangeFileBytes(t, dest)); got != "original\n" {
		t.Fatalf("dest mutated: %q", got)
	}
}

func listChangeFolderSnapshot(t *testing.T, folder string) map[string][]byte {
	t.Helper()
	snap := map[string][]byte{}
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(folder, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snap[filepath.ToSlash(rel)] = body
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot walk: %v", err)
	}
	return snap
}

func assertChangeFolderUntouched(t *testing.T, folder string, before map[string][]byte) {
	t.Helper()
	after := listChangeFolderSnapshot(t, folder)
	if len(before) != len(after) {
		t.Fatalf("folder file count changed: before=%d after=%d", len(before), len(after))
	}
	for rel, body := range before {
		got, ok := after[rel]
		if !ok {
			t.Fatalf("file vanished: %s", rel)
		}
		if !bytes.Equal(body, got) {
			t.Fatalf("file mutated: %s", rel)
		}
	}
}

func TestChangeInitRejectsBadSlug(t *testing.T) {
	for _, slug := range []string{"Bad", "under_score", "with space", "trailing-", "-leading", "double--hyphen", ""} {
		t.Run(slug, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			args := []string{"change", "init"}
			if slug != "" {
				args = append(args, slug)
			}
			err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run(args)
			if err == nil {
				t.Fatalf("err = nil, want rejection for slug %q", slug)
			}
		})
	}
}

func TestChangeReportNewHappyPath(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "report-demo"}); err != nil {
		t.Fatalf("init error = %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join("docs", "changes", today+"-report-demo")
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "report", "new", "shaping", "--kind", "approval", folder}); err != nil {
		t.Fatalf("report new error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Created report:") {
		t.Fatalf("output = %q, want Created report", output)
	}
	if !strings.Contains(output, "Design language") {
		t.Fatalf("output = %q, want design-language guidance", output)
	}
	matches, err := filepath.Glob(filepath.Join(repo, folder, "reports", "*-approval-shaping.html"))
	if err != nil {
		t.Fatalf("Glob error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("want 1 approval report, got %v", matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	content := string(body)
	for _, want := range []string{
		`<!-- source:`,
		`<meta charset="utf-8">`,
		`--accent:`,
		`--machine:`,
		`kind <b>approval</b>`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("report missing invariant %q:\n%s", want, content)
		}
	}
}

func TestChangeReportNewRefusesUnknownKind(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "report-demo"}); err != nil {
		t.Fatalf("init error = %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join("docs", "changes", today+"-report-demo")
	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "report", "new", "x", "--kind", "memo", folder})
	if err == nil || !strings.Contains(err.Error(), "unknown report kind") {
		t.Fatalf("err = %v, want unknown report kind", err)
	}
	if err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("err = %v, want registry printed", err)
	}
}

func TestChangeReportNewRefusesCollision(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "report-demo"}); err != nil {
		t.Fatalf("init error = %v", err)
	}
	today := time.Now().Format("20060102")
	folderRel := filepath.Join("docs", "changes", today+"-report-demo")
	frozen := time.Date(2026, 7, 27, 15, 4, 5, 0, time.Local)
	prev := changeReportClock
	changeReportClock = func() time.Time { return frozen }
	t.Cleanup(func() { changeReportClock = prev })

	reports := filepath.Join(repo, folderRel, "reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	existing := filepath.Join(reports, "20260727-150405-note-dup.html")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "report", "new", "dup", "--kind", "note", folderRel})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already exists", err)
	}
}

// --- Pilot smoke: the change model's own pilot must pass --------------------

func TestChangeCheckPilotPasses(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	pilot := filepath.Join("docs", "changes", "20260704-shape-first-change-workflow")
	if _, err := os.Stat(filepath.Join(repoRoot, pilot, "change.md")); err != nil {
		t.Skipf("pilot change not present: %v", err)
	}

	out, err := runChangeCheckJSON(t, repoRoot, pilot)
	if err != nil {
		t.Fatalf("pilot check err = %v, want nil (no violations)", err)
	}
	if !out.Passed {
		t.Fatalf("pilot passed = false, findings = %v", out.Findings)
	}
	if !out.Executable {
		t.Fatalf("pilot executable = false, gaps = %v", out.Gaps)
	}
}

func findingsContain(findings []string, substr string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, substr) {
			return true
		}
	}
	return false
}
