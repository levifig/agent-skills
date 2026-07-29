package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeReceiptFreshness(t *testing.T) {
	t.Run("post-squash-protocol-clone-stays-verified", func(t *testing.T) {
		repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
		// Unchecked task lands on main first so the squash commit's diff carries a real flip.
		dir := writeNewLayoutChange(t, repo, "20260727-squash", "squash", "1.0.0", "")
		task := filepath.Join(dir, "tasks", "TASK-001-work.md")
		if err := os.WriteFile(task, []byte("---\nchange: squash\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"), 0o644); err != nil {
			t.Fatalf("WriteFile unchecked: %v", err)
		}
		commitAllChangeTest(t, repo, "docs: shape squash on main")

		gitCLI(t, repo, "checkout", "-b", "feature-squash")
		if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatalf("WriteFile main.go: %v", err)
		}
		if err := os.WriteFile(task, []byte("---\nchange: squash\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"), 0o644); err != nil {
			t.Fatalf("WriteFile flip: %v", err)
		}
		commitAllChangeTest(t, repo, "feat: execute squash")
		folderRel := filepath.Join("docs", "changes", "20260727-squash")
		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify on branch: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: commit receipt on feature branch")

		gitCLI(t, repo, "checkout", "main")
		gitCLI(t, repo, "merge", "--squash", "feature-squash")
		gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "squash: land feature")
		gitCLI(t, repo, "branch", "-D", "feature-squash")

		if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
			t.Fatalf("author machine after squash should verify: %v", err)
		}

		clone := t.TempDir()
		cmd := exec.Command("git", "clone", "--", "file://"+repo, clone)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("protocol clone: %v\n%s", err, out)
		}
		if err := releaseCohortPreflight(clone, "1.0.0", nil); err != nil {
			t.Fatalf("protocol clone must yield same verified verdict: %v", err)
		}
	})

	t.Run("cohort-of-two-receipts-coexist", func(t *testing.T) {
		repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
		dirA := writeNewLayoutChange(t, repo, "20260727-cohort-a", "cohort-a", "1.0.0", "")
		dirB := writeNewLayoutChange(t, repo, "20260727-cohort-b", "cohort-b", "1.0.0", "")
		flipExecuteChange(t, repo, dirA, "cohort-a")

		taskB := filepath.Join(dirB, "tasks", "TASK-001-work.md")
		if err := os.MkdirAll(filepath.Dir(taskB), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(taskB, []byte("---\nchange: cohort-b\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"), 0o644); err != nil {
			t.Fatalf("WriteFile unchecked: %v", err)
		}
		commitAllChangeTest(t, repo, "docs: shape cohort-b")
		if err := os.WriteFile(filepath.Join(repo, "main_b.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatalf("WriteFile main_b.go: %v", err)
		}
		if err := os.WriteFile(taskB, []byte("---\nchange: cohort-b\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"), 0o644); err != nil {
			t.Fatalf("WriteFile flip: %v", err)
		}
		commitAllChangeTest(t, repo, "feat: execute cohort-b")

		folderA := filepath.Join("docs", "changes", "20260727-cohort-a")
		folderB := filepath.Join("docs", "changes", "20260727-cohort-b")
		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderA}); err != nil {
			t.Fatalf("verify A: %v", err)
		}
		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderB}); err != nil {
			t.Fatalf("verify B: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: commit both cohort receipts")
		if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
			t.Fatalf("N=2 cohort receipts must coexist: %v", err)
		}
	})

	t.Run("touch-then-revert-inverse-stays-fresh", func(t *testing.T) {
		repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
		dir := writeNewLayoutChange(t, repo, "20260727-revert", "revert", "1.0.0", "")
		flipExecuteChange(t, repo, dir, "revert")
		folderRel := filepath.Join("docs", "changes", "20260727-revert")
		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: commit receipt")

		path := filepath.Join(repo, "touch.txt")
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: touch")
		if err := os.Remove(path); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: restore bytes")
		// ADR-024 Decision 4: byte-identical restore un-stales deliberately.
		if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
			t.Fatalf("byte-identical restore must stay fresh: %v", err)
		}
	})

	t.Run("every-reason-is-a-typed-block-never-inspection-error", func(t *testing.T) {
		repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
		body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0\n- **V2.** Also. Command: `true`. Expect: exit 0")
		dir := writeNewLayoutChange(t, repo, "20260727-reasons", "reasons", "1.0.0", body)
		flipExecuteChange(t, repo, dir, "reasons")
		folderRel := filepath.Join("docs", "changes", "20260727-reasons")
		node, err := assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}

		assertBlock := func(t *testing.T, verdict changeReceiptVerdict, want changeReceiptReason, substr string) {
			t.Helper()
			if verdict.OK || verdict.Reason != want {
				t.Fatalf("verdict=%#v, want reason %v", verdict, want)
			}
			msg := formatChangeReceiptBlock("reasons", "1.0.0", verdict, folderRel)
			if strings.Contains(msg, "cannot inspect") || strings.Contains(msg, "exit status") {
				t.Fatalf("inspection error leaked: %s", msg)
			}
			if !strings.Contains(msg, substr) {
				t.Fatalf("msg=%q, want substr %q", msg, substr)
			}
			if want != changeReceiptFailingResults && !strings.Contains(msg, "loaf change verify") {
				t.Fatalf("msg=%q, want remedy", msg)
			}
		}

		verdict, err := changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("missing: %v", err)
		}
		assertBlock(t, verdict, changeReceiptMissing, "missing receipt")

		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify: %v", err)
		}
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("uncommitted: %v", err)
		}
		assertBlock(t, verdict, changeReceiptUncommitted, "not committed")

		commitAllChangeTest(t, repo, "chore: commit receipt")
		node, _ = assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil || !verdict.OK {
			t.Fatalf("fresh: %#v err=%v", verdict, err)
		}

		v1 := changeVerifyReceipt{SchemaVersion: 1, Change: "reasons", CriteriaDigest: "x", Results: []changeVerifyCriterionResult{{ID: "V1", OK: true}, {ID: "V2", OK: true}}}
		writeCommittedReceipt(t, repo, dir, v1)
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("schema: %v", err)
		}
		assertBlock(t, verdict, changeReceiptUnsupportedSchema, "unsupported receipt schema_version 1")

		if err := os.WriteFile(filepath.Join(dir, "receipts", "verify.json"), []byte("{not-json"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: corrupt receipt")
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("unreadable: %v", err)
		}
		assertBlock(t, verdict, changeReceiptUnreadable, "unreadable")

		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("re-verify: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: restore receipt")
		expired := shapeWithVerification("- **V1.** Smoke changed. Command: `true`. Expect: exit 0\n- **V2.** Also. Command: `true`. Expect: exit 0")
		if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(expired), 0o644); err != nil {
			t.Fatalf("WriteFile shape: %v", err)
		}
		commitAllChangeTest(t, repo, "docs: change criteria text")
		node, _ = assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("criteria: %v", err)
		}
		assertBlock(t, verdict, changeReceiptCriteriaMismatch, "criteria changed")

		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify after criteria: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: re-verify")
		driftPath := filepath.Join(repo, "internal", "cli", "drift.go")
		if err := os.MkdirAll(filepath.Dir(driftPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(driftPath, []byte("package cli\n"), 0o644); err != nil {
			t.Fatalf("WriteFile drift: %v", err)
		}
		commitAllChangeTest(t, repo, "feat: drift content")
		node, _ = assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("drift: %v", err)
		}
		assertBlock(t, verdict, changeReceiptContentDrift, "content changed under")
		if !strings.Contains(verdict.Cause(), "`internal`") {
			t.Fatalf("cause should name internal section: %s", verdict.Cause())
		}

		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify clean: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: fresh again")
		good := mustReadVerifyReceipt(t, dir)
		good.Results = good.Results[:1]
		writeCommittedReceipt(t, repo, dir, good)
		node, _ = assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("gap: %v", err)
		}
		assertBlock(t, verdict, changeReceiptResultsGap, "missing criteria (V2)")

		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: full results")
		failing := mustReadVerifyReceipt(t, dir)
		failing.Results[0].OK = false
		writeCommittedReceipt(t, repo, dir, failing)
		verdict, err = changeReceiptStatus(repo, folderRel, node, nil)
		if err != nil {
			t.Fatalf("failing: %v", err)
		}
		assertBlock(t, verdict, changeReceiptFailingResults, "failing criteria (V1)")
	})
}

func writeCommittedReceipt(t *testing.T, repo, dir string, receipt changeVerifyReceipt) {
	t.Helper()
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "receipts", "verify.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: write receipt fixture")
}
