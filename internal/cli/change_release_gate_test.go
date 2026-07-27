package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedCohortGateRepo(t *testing.T, version string) string {
	t.Helper()
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, version)
	return repo
}

func writeReleaseVersionFiles(t *testing.T, repo, version string) {
	t.Helper()
	pkg := "{\n  \"name\": \"demo\",\n  \"version\": \"" + version + "\"\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatalf("WriteFile package.json: %v", err)
	}
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

func TestReleaseCohortGateBlocksUnexecutedStableTarget(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	writeNewLayoutChange(t, repo, "20260727-cohort-member", "cohort-member", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape cohort member")

	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "targets 2.0.0 but is not executed") {
		t.Fatalf("err = %v, want not executed", err)
	}

	// Prerelease candidate bypasses.
	if err := releaseCohortPreflight(repo, "2.0.0-alpha.2", nil); err != nil {
		t.Fatalf("prerelease candidate should bypass: %v", err)
	}

	// Minor candidate 2.1.0 does not gate 2.0.0 cohort (warn only).
	var warnings []string
	if err := releaseCohortPreflight(repo, "2.1.0", &warnings); err != nil {
		t.Fatalf("2.1.0 candidate should not block on 2.0.0 cohort: %v", err)
	}
	if !findingsContain(warnings, "incomplete lower cohort") {
		t.Fatalf("warnings = %v, want lower cohort warn", warnings)
	}
}

func TestReleaseCohortGateLegacyConvertFirst(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	body := executableLineageDoc("legacy-member", "line", "", "")
	body = strings.Replace(body, "---\n", "---\ntarget_release: 2.0.0\n", 1)
	writeChangeFolder(t, repo, "20260727-legacy-member", body)
	commitAllChangeTest(t, repo, "docs: legacy cohort member")

	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "legacy layout — convert first") {
		t.Fatalf("err = %v, want convert first", err)
	}
}

func TestReleaseCohortGateAcceptsFlipExecutedMember(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-executed", "executed", "2.0.0", "")
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.WriteFile(task, []byte("---\nchange: executed\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: shape executed member")

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	if err := os.WriteFile(task, []byte("---\nchange: executed\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n\nnote\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task touch: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: path grade only")
	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "not executed") {
		t.Fatalf("path grade should not open gate: %v", err)
	}

	if err := os.WriteFile(task, []byte("---\nchange: executed\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile flip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go flip: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: execute task")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260727-executed")}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	if err := releaseCohortPreflight(repo, "2.0.0", nil); err != nil {
		t.Fatalf("flip-executed cohort with receipt should pass: %v", err)
	}
}

func TestComputeReleaseCandidateVersionFinalization(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.14")
	v, err := computeReleaseCandidateVersion(repo, releaseOptions{bump: "release"})
	if err != nil || v != "2.0.0" {
		t.Fatalf("release bump candidate = %q err=%v, want 2.0.0", v, err)
	}
	v, err = computeReleaseCandidateVersion(repo, releaseOptions{postMerge: true})
	if err != nil || v != "2.0.0" {
		t.Fatalf("post-merge candidate = %q err=%v, want 2.0.0", v, err)
	}
	v, err = computeReleaseCandidateVersion(repo, releaseOptions{bump: "minor"})
	if err != nil || v != "2.1.0" {
		t.Fatalf("minor bump candidate = %q err=%v, want 2.1.0", v, err)
	}
	v, err = computeReleaseCandidateVersion(repo, releaseOptions{bump: "prerelease"})
	if err != nil || v != "2.0.0-alpha.15" {
		t.Fatalf("prerelease bump candidate = %q err=%v, want 2.0.0-alpha.15", v, err)
	}
}
