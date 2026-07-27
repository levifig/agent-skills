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

func TestReleaseCohortGateRejectsFailingReceipt(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Always fails. Command: `false`. Expect: exit 0\n- **V3.** Also fails. Command: `exit 1`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-failing-receipt", "failing-receipt", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "failing-receipt")

	folderRel := filepath.Join("docs", "changes", "20260727-failing-receipt")
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel})
	if err == nil {
		t.Fatalf("verify should fail with failing criteria\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Wrote receipt:") {
		t.Fatalf("write-on-failure expected receipt; stdout=%q", stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit failing receipt")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "receipt records failing criteria (V1, V3)") {
		t.Fatalf("gate err = %v, want failing criteria V1, V3", gateErr)
	}

	// Same fixture with all criteria passing proceeds.
	bodyOK := shapeWithVerification("- **V1.** Root marker. Command: `true`. Expect: exit 0\n- **V3.** Also. Command: `true`. Expect: exit 0")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(bodyOK), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: fix criteria")
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify pass: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit passing receipt")
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("passing receipt should open gate: %v", err)
	}
}

func TestReleaseCohortGateReceiptFreshnessBootstrap(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-freshness", "freshness", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "freshness")
	folderRel := filepath.Join("docs", "changes", "20260727-freshness")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("receipt-only commit should not stale: %v", err)
	}

	// Touch then revert a non-receipt path — tree diff is empty, commit-by-commit is not.
	other := filepath.Join(repo, "other.txt")
	if err := os.WriteFile(other, []byte("touch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile other: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: touch other")
	if err := os.Remove(other); err != nil {
		t.Fatalf("Remove other: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: revert other")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "later non-receipt path requires criteria re-run") {
		t.Fatalf("touch-then-revert should stale: %v", gateErr)
	}

	// Re-verify after the stale commits.
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("re-verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: re-verify after stale")
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("fresh receipt should pass: %v", err)
	}

	// Any other later non-receipt path stales.
	if err := os.WriteFile(filepath.Join(repo, "stale.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile stale: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: later non-receipt path")
	gateErr = releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "later non-receipt path requires criteria re-run") {
		t.Fatalf("later path should stale: %v", gateErr)
	}
}

func TestChangeVerifyWritesReceiptOnFailure(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := shapeWithVerification("- **V1.** Fail. Command: `false`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-write-fail", "write-fail", "", body)
	commitAllChangeTest(t, repo, "docs: shape write-fail")
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260727-write-fail")})
	if err == nil {
		t.Fatal("expected verify failure")
	}
	receiptPath := filepath.Join(dir, "receipts", "verify.json")
	data, readErr := os.ReadFile(receiptPath)
	if readErr != nil {
		t.Fatalf("receipt should be written on failure: %v\n%s", readErr, stdout.String())
	}
	if !strings.Contains(string(data), `"ok": false`) {
		t.Fatalf("receipt = %s", data)
	}
}
