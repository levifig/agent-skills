package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if !strings.Contains(string(data), `"cwd": "`+filepath.ToSlash(repo)) && !strings.Contains(string(data), `"cwd": "`+repo) {
		t.Fatalf("receipt missing repo-root cwd: %s", data)
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

func shapeWithVerification(vbody string) string {
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n"+vbody,
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

func TestChangeVerifyParsesInlineAndSubBulletForms(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		want   string
		expect string
	}{
		{
			name:   "inline",
			body:   "- **V1.** Prose. Command: `true`. Expect: exit 0.",
			want:   "true",
			expect: "exit 0.",
		},
		{
			name:   "inline-checkbox-scaffold",
			body:   "- [**V1.** What must be true. Command: `exact command`. Expect: exit 0 and/or specific output.]",
			want:   "exact command",
			expect: "exit 0 and/or specific output.",
		},
		{
			name:   "sub-bullet",
			body:   "- **V1.** Smoke.\n  - Command: `true`\n  - Expect: exit 0",
			want:   "true",
			expect: "exit 0",
		},
		{
			name:   "inline-expect-optional",
			body:   "- **V1.** Prose. Command: `true`.",
			want:   "true",
			expect: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseChangeExecutableCriteria(shapeWithVerification(tc.body))
			if len(got) != 1 {
				t.Fatalf("criteria = %#v, want 1", got)
			}
			if got[0].ID != "V1" || got[0].Command != tc.want {
				t.Fatalf("got = %#v, want command %q", got[0], tc.want)
			}
			if got[0].Expect != tc.expect {
				t.Fatalf("expect = %q, want %q", got[0].Expect, tc.expect)
			}
		})
	}
}

func TestChangeVerifyIgnoresHTier(t *testing.T) {
	body := "- **V1.** Gate. Command: `true`.\n\n- **H1.** Human review only.\n\n- [**H2.** Also human.]"
	got := parseChangeExecutableCriteria(shapeWithVerification(body))
	if len(got) != 1 || got[0].ID != "V1" {
		t.Fatalf("criteria = %#v, want only V1", got)
	}
}

func TestChangeVerifyRunsFromRepoRootAndRecordsCwd(t *testing.T) {
	repo := initCLIGitRepo(t)
	marker := filepath.Join(repo, "root-marker.txt")
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("WriteFile marker: %v", err)
	}
	body := shapeWithVerification("- **V1.** Root-scoped. Command: `test -f root-marker.txt`. Expect: exit 0.")
	dir := writeNewLayoutChange(t, repo, "20260728-cwd-root", "cwd-root", "", body)
	commitAllChangeTest(t, repo, "docs: shape cwd-root")
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260728-cwd-root")}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "receipts", "verify.json"))
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	var receipt changeVerifyReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if receipt.Cwd != repo {
		t.Fatalf("cwd = %q, want repo root %q", receipt.Cwd, repo)
	}
	if len(receipt.Results) != 1 || !receipt.Results[0].OK {
		t.Fatalf("results = %#v, want V1 ok at repo root", receipt.Results)
	}
}

func TestChangeVerifyParsesFreshScaffoldCriterion(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "parse-probe"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join("docs", "changes", today+"-parse-probe")
	shapePath := filepath.Join(repo, folder, "shape.md")
	body, err := os.ReadFile(shapePath)
	if err != nil {
		t.Fatalf("ReadFile shape: %v", err)
	}
	criteria := parseChangeExecutableCriteria(string(body))
	if len(criteria) != 1 {
		t.Fatalf("scaffold criteria = %#v, want 1 parsed criterion", criteria)
	}
	if criteria[0].Command != "exact command" {
		t.Fatalf("command = %q, want scaffold placeholder", criteria[0].Command)
	}
	var stdout bytes.Buffer
	err = (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folder})
	if err != nil && strings.Contains(err.Error(), "no executable criteria found") {
		t.Fatalf("verify refused parse: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "V1") {
		t.Fatalf("stdout = %q, want V1 criterion run", stdout.String())
	}
}
