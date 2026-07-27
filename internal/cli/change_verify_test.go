package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeVerifyWritesReceipt(t *testing.T) {
	repo := initCLIGitRepo(t)
	dir := writeNewLayoutChange(t, repo, "20260727-verify-me", "verify-me", "", "")
	commitAllChangeTest(t, repo, "docs: shape verify-me")
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260727-verify-me")}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	receiptPath := filepath.Join(dir, "receipts", "verify.json")
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("receipt missing: %v", err)
	}
	if !strings.Contains(string(data), `"criteria_digest"`) || !strings.Contains(string(data), `"verified_commit"`) {
		t.Fatalf("receipt = %s", data)
	}
	if !strings.Contains(stdout.String(), "Wrote receipt:") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestChangeVerifyLegacyRefused(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := writeChangeFolder(t, repo, "20260727-legacy-verify", executableLineageDoc("legacy-verify", "line", "", ""))
	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}.Run([]string{"change", "verify", folder})
	if err == nil || !strings.Contains(err.Error(), "new-layout-only") {
		t.Fatalf("err = %v, want new-layout-only", err)
	}
}
