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
	if !strings.Contains(string(data), `"schema_version": 2`) {
		t.Fatalf("receipt missing schema_version 2: %s", data)
	}
	if strings.Contains(string(data), `"cwd"`) {
		t.Fatalf("receipt must not record cwd: %s", data)
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
			body:   "- [**V1.** What must be true. Command: `exact command`. Expect: exit 0.]",
			want:   "exact command",
			expect: "exit 0.",
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

func TestChangeVerifyRunsFromRepoRoot(t *testing.T) {
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
	if len(receipt.Results) != 1 || !receipt.Results[0].OK {
		t.Fatalf("results = %#v, want V1 ok at repo root", receipt.Results)
	}
	if receipt.SchemaVersion != 2 || receipt.ScopeDigest == "" || receipt.WorktreeClean != true {
		t.Fatalf("receipt = %#v, want schema v2 with scope digest and clean worktree", receipt)
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

func TestChangeVerifyV5ReceiptDigestExpiryViaChangeReceiptStatus(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-v5-digest", "v5-digest", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "v5-digest")
	folderRel := filepath.Join("docs", "changes", "20260727-v5-digest")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	node, err := assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	verdict, statusErr := changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || !verdict.OK {
		t.Fatalf("fresh receipt: ok=%v reason=%q err=%v", verdict.OK, verdict.Cause(), statusErr)
	}

	expired := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0 changed")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(expired), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: edit criteria")

	node, err = assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
	if err != nil {
		t.Fatalf("assemble after edit: %v", err)
	}
	verdict, statusErr = changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil {
		t.Fatalf("status err: %v", statusErr)
	}
	if verdict.OK || verdict.Cause() != "criteria changed (receipt expired)" {
		t.Fatalf("ok=%v reason=%q, want expired criteria", verdict.OK, verdict.Cause())
	}
}

// TASK-019 / C3-5: Expect is enforced by a minimal grammar — exit atom plus
// repeatable contains — and anything outside the grammar is advisory, never
// silently dropped and never part of ok.
func TestChangeExpectGrammarEnforcement(t *testing.T) {
	cases := []struct {
		name         string
		expect       string
		exitCode     int
		output       string
		wantOK       bool
		wantChecks   []changeVerifyExpectCheck
		wantAdvisory []string
	}{
		{
			name:       "absent expect enforces exit zero",
			expect:     "",
			exitCode:   0,
			wantOK:     true,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
		},
		{
			name:       "absent expect fails on nonzero exit",
			expect:     "",
			exitCode:   1,
			wantOK:     false,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: false}},
		},
		{
			name:       "exit atom mismatch fails",
			expect:     "exit 2",
			exitCode:   1,
			wantOK:     false,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "2", OK: false}},
		},
		{
			name:       "exit atom matches nonzero code",
			expect:     "exit 2.",
			exitCode:   2,
			wantOK:     true,
			wantChecks: []changeVerifyExpectCheck{{Kind: "exit", Value: "2", OK: true}},
		},
		{
			name:   "contains match passes",
			expect: "contains `all green`",
			output: "suite: all green\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "all green", OK: true},
			},
		},
		{
			name:   "contains mismatch fails even at exit zero",
			expect: "contains `nope`.",
			output: "ok\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "nope", OK: false},
			},
		},
		{
			name:   "multi atom and",
			expect: "exit 0 and contains `first` and contains `second`",
			output: "first then second\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "first", OK: true},
				{Kind: "contains", Value: "second", OK: true},
			},
		},
		{
			name:   "multi atom fails when one contains misses",
			expect: "exit 0 and contains `first` and contains `second`",
			output: "first only\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "first", OK: true},
				{Kind: "contains", Value: "second", OK: false},
			},
		},
		{
			name:   "backticked text keeps its own and",
			expect: "contains `a and b`",
			output: "x a and b y\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "a and b", OK: true},
			},
		},
		{
			name:         "unenforceable clause is advisory only",
			expect:       "exit 0 and the output reads well",
			exitCode:     0,
			wantOK:       true,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
			wantAdvisory: []string{"the output reads well"},
		},
		{
			name:         "advisory clause never rescues a failing atom",
			expect:       "exit 0 and looks right",
			exitCode:     1,
			wantOK:       false,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: false}},
			wantAdvisory: []string{"looks right"},
		},
		{
			name:         "retired and/or promise is advisory, not silently enforced",
			expect:       "exit 0 and/or specific output.",
			exitCode:     0,
			wantOK:       true,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
			wantAdvisory: []string{"exit 0 and/or specific output"},
		},
		{
			name:         "contains without backticks is advisory",
			expect:       "contains ok",
			output:       "ok\n",
			wantOK:       true,
			wantChecks:   []changeVerifyExpectCheck{{Kind: "exit", Value: "0", OK: true}},
			wantAdvisory: []string{"contains ok"},
		},
		{
			name:     "duplicate exit atoms fail loudly",
			expect:   "exit 1 and exit 0",
			exitCode: 0,
			wantOK:   false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit-conflict", Value: "1 and 0", OK: false},
			},
		},
		{
			name:   "conflict records contains true alongside",
			expect: "exit 1 and contains `sentinel` and exit 0",
			output: "has sentinel here\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit-conflict", Value: "1 and 0", OK: false},
				{Kind: "contains", Value: "sentinel", OK: true},
			},
		},
		{
			name:   "conflict records contains false alongside",
			expect: "exit 1 and contains `sentinel` and exit 0",
			output: "no match\n",
			wantOK: false,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit-conflict", Value: "1 and 0", OK: false},
				{Kind: "contains", Value: "sentinel", OK: false},
			},
		},
		{
			name:   "single exit with contains unchanged",
			expect: "exit 0 and contains `ok`",
			output: "ok\n",
			wantOK: true,
			wantChecks: []changeVerifyExpectCheck{
				{Kind: "exit", Value: "0", OK: true},
				{Kind: "contains", Value: "ok", OK: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectation := parseChangeExpectation(tc.expect)
			checks := evaluateChangeExpectation(expectation, tc.exitCode, tc.output)
			if got := changeExpectChecksPass(checks); got != tc.wantOK {
				t.Fatalf("ok = %v, want %v (checks=%#v)", got, tc.wantOK, checks)
			}
			if len(checks) != len(tc.wantChecks) {
				t.Fatalf("checks = %#v, want %#v", checks, tc.wantChecks)
			}
			for i, want := range tc.wantChecks {
				if checks[i] != want {
					t.Fatalf("check[%d] = %#v, want %#v", i, checks[i], want)
				}
			}
			if len(expectation.Advisory) != len(tc.wantAdvisory) {
				t.Fatalf("advisory = %#v, want %#v", expectation.Advisory, tc.wantAdvisory)
			}
			for i, want := range tc.wantAdvisory {
				if expectation.Advisory[i] != want {
					t.Fatalf("advisory[%d] = %q, want %q", i, expectation.Advisory[i], want)
				}
			}
		})
	}
}

func TestChangeVerifyDuplicateExitAtomsFailLoudly(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := shapeWithVerification("- **V1.** Contradictory. Command: `true`. Expect: exit 1 and exit 0.\n- **V2.** Still fine. Command: `echo ok`. Expect: exit 0 and contains `ok`.")
	dir := writeNewLayoutChange(t, repo, "20260728-exit-conflict", "exit-conflict", "", body)
	commitAllChangeTest(t, repo, "docs: shape exit-conflict")
	folderRel := filepath.Join("docs", "changes", "20260728-exit-conflict")

	criteria := parseChangeExecutableCriteria(string(mustRead(t, filepath.Join(dir, "shape.md"))))
	digestBefore := changeCriteriaDigest(criteria)

	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel})
	if err == nil {
		t.Fatalf("verify should fail on exit conflict\n%s", stdout.String())
	}
	printed := stripANSI(stdout.String())
	if !strings.Contains(printed, "contradictory exit atoms: 1 and 0") {
		t.Fatalf("stdout = %q, want plain failure naming both values", printed)
	}

	receipt := mustReadVerifyReceipt(t, dir)
	if len(receipt.Results) < 1 {
		t.Fatalf("results = %#v, want V1 recorded", receipt.Results)
	}
	v1 := receipt.Results[0]
	if v1.OK {
		t.Fatalf("V1 = %#v, want ok:false", v1)
	}
	if len(v1.ExpectChecks) != 1 || v1.ExpectChecks[0].Kind != "exit-conflict" || v1.ExpectChecks[0].Value != "1 and 0" || v1.ExpectChecks[0].OK {
		t.Fatalf("V1 checks = %#v, want exit-conflict naming 1 and 0", v1.ExpectChecks)
	}
	v2 := receipt.Results[1]
	if !v2.OK {
		t.Fatalf("V2 = %#v, want unchanged pass for exit 0 and contains", v2)
	}

	digestAfter := changeCriteriaDigest(parseChangeExecutableCriteria(string(mustRead(t, filepath.Join(dir, "shape.md")))))
	if digestBefore != digestAfter || digestBefore != receipt.CriteriaDigest {
		t.Fatalf("criteria digest must be unaffected by parse results: before=%s after=%s receipt=%s", digestBefore, digestAfter, receipt.CriteriaDigest)
	}
}

func TestChangeVerifyEnforcesExpectAndRecordsAtoms(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := shapeWithVerification("- **V1.** Green. Command: `echo ok`. Expect: exit 0 and contains `ok`.\n- **V2.** Advisory. Command: `echo hi`. Expect: exit 0 and the output reads well.")
	dir := writeNewLayoutChange(t, repo, "20260727-expect-grammar", "expect-grammar", "", body)
	commitAllChangeTest(t, repo, "docs: shape expect-grammar")
	folderRel := filepath.Join("docs", "changes", "20260727-expect-grammar")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	printed := stdout.String()
	if !strings.Contains(printed, "V2  unenforceable Expect clause \"the output reads well\"") {
		t.Fatalf("stdout = %q, want warning naming criterion and clause", printed)
	}
	if !strings.Contains(printed, "advisory") {
		t.Fatalf("stdout = %q, want the advisory rule stated", printed)
	}

	receipt := mustReadVerifyReceipt(t, dir)
	if len(receipt.Results) != 2 {
		t.Fatalf("results = %#v, want V1 and V2", receipt.Results)
	}
	v1 := receipt.Results[0]
	if !v1.OK || v1.Expect != "exit 0 and contains `ok`." {
		t.Fatalf("V1 = %#v, want ok with recorded expectation", v1)
	}
	wantChecks := []changeVerifyExpectCheck{
		{Kind: "exit", Value: "0", OK: true},
		{Kind: "contains", Value: "ok", OK: true},
	}
	if len(v1.ExpectChecks) != len(wantChecks) {
		t.Fatalf("V1 checks = %#v, want %#v", v1.ExpectChecks, wantChecks)
	}
	for i, want := range wantChecks {
		if v1.ExpectChecks[i] != want {
			t.Fatalf("V1 check[%d] = %#v, want %#v", i, v1.ExpectChecks[i], want)
		}
	}
	v2 := receipt.Results[1]
	if !v2.OK {
		t.Fatalf("V2 = %#v, want ok — an advisory clause never fails a criterion", v2)
	}
	if len(v2.Advisory) != 1 || v2.Advisory[0] != "the output reads well" {
		t.Fatalf("V2 advisory = %#v, want the unenforceable clause recorded", v2.Advisory)
	}
	for _, check := range v2.ExpectChecks {
		if check.Kind == "contains" {
			t.Fatalf("V2 checks = %#v, want no enforced contains atom", v2.ExpectChecks)
		}
	}

	// The same command with an unmet contains fails the criterion.
	missBody := shapeWithVerification("- **V1.** Miss. Command: `echo ok`. Expect: exit 0 and contains `nope`.")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(missBody), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: tighten contains expectation")
	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err == nil {
		t.Fatalf("exit-zero command with unmet contains must fail\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "output missing: contains `nope`") {
		t.Fatalf("stdout = %q, want the unmet atom named", stdout.String())
	}
	missed := mustReadVerifyReceipt(t, dir)
	if len(missed.Results) != 1 || missed.Results[0].OK || missed.Results[0].ExitCode != 0 {
		t.Fatalf("results = %#v, want exit 0 recorded with ok=false", missed.Results)
	}
}

// The packet's own verification line: a contains mismatch fails the criterion and
// the committed failing receipt blocks the cohort gate (with TASK-017's HEAD read).
func TestChangeVerifyExpectMismatchBlocksCohortGate(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Output-bound. Command: `echo ok`. Expect: exit 0 and contains `ok`.")
	dir := writeNewLayoutChange(t, repo, "20260727-expect-gate", "expect-gate", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "expect-gate")
	folderRel := filepath.Join("docs", "changes", "20260727-expect-gate")

	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit passing receipt")
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("met expectation should open the gate: %v", err)
	}

	missBody := shapeWithVerification("- **V1.** Output-bound. Command: `echo ok`. Expect: exit 0 and contains `nope`.")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(missBody), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: tighten V1 output expectation")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err == nil {
		t.Fatal("verify must fail on the unmet contains atom")
	}
	commitAllChangeTest(t, repo, "chore: commit failing receipt")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "receipt records failing criteria (V1)") {
		t.Fatalf("gate err = %v, want failing criteria V1", gateErr)
	}
}

func mustReadVerifyReceipt(t *testing.T, dir string) changeVerifyReceipt {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "receipts", "verify.json"))
	if err != nil {
		t.Fatalf("ReadFile receipt: %v", err)
	}
	var receipt changeVerifyReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatalf("Unmarshal receipt: %v", err)
	}
	return receipt
}

// TASK-017 / C3-3: the gate reads the receipt from committed HEAD, so an
// uncommitted receipt is evidence on one machine only and never opens the gate,
// while a dirty working tree cannot close it either.
func TestChangeReceiptStatusReadsCommittedHEADNotWorkingTree(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-head-receipt", "head-receipt", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "head-receipt")
	folderRel := filepath.Join("docs", "changes", "20260727-head-receipt")
	receiptPath := filepath.Join(dir, "receipts", "verify.json")

	node, err := assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	// Never verified: no receipt anywhere.
	verdict, statusErr := changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || verdict.OK || verdict.Cause() != "missing receipt" {
		t.Fatalf("never verified: ok=%v reason=%q err=%v, want missing receipt", verdict.OK, verdict.Cause(), statusErr)
	}

	// Verified at HEAD but nobody committed the receipt: blocks distinctly.
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	passing, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("ReadFile receipt: %v", err)
	}
	verdict, statusErr = changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || verdict.OK || verdict.Cause() != "receipt not committed at HEAD" {
		t.Fatalf("uncommitted receipt: ok=%v reason=%q err=%v, want not-committed block", verdict.OK, verdict.Cause(), statusErr)
	}
	if msg := formatChangeReceiptBlock("head-receipt", "1.0.0", verdict, filepath.ToSlash(folderRel)); !strings.Contains(msg, "commit the receipt") {
		t.Fatalf("block message = %q, want commit-the-receipt remedy", msg)
	}

	// The same receipt, committed, proceeds.
	commitAllChangeTest(t, repo, "chore: commit verify receipt")
	verdict, statusErr = changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || !verdict.OK {
		t.Fatalf("committed receipt: ok=%v reason=%q err=%v, want pass", verdict.OK, verdict.Cause(), statusErr)
	}

	// Dirty working tree is irrelevant: a locally failing receipt cannot close the gate.
	var mangled changeVerifyReceipt
	if err := json.Unmarshal(passing, &mangled); err != nil {
		t.Fatalf("unmarshal receipt: %v", err)
	}
	for i := range mangled.Results {
		mangled.Results[i].OK = false
	}
	mangledData, err := json.MarshalIndent(mangled, "", "  ")
	if err != nil {
		t.Fatalf("marshal mangled receipt: %v", err)
	}
	if err := os.WriteFile(receiptPath, append(mangledData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile mangled receipt: %v", err)
	}
	verdict, statusErr = changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || !verdict.OK {
		t.Fatalf("working-tree edit must not affect the gate: ok=%v reason=%q err=%v", verdict.OK, verdict.Cause(), statusErr)
	}

	// Nor can deleting it locally.
	if err := os.Remove(receiptPath); err != nil {
		t.Fatalf("Remove receipt: %v", err)
	}
	verdict, statusErr = changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || !verdict.OK {
		t.Fatalf("working-tree delete must not affect the gate: ok=%v reason=%q err=%v", verdict.OK, verdict.Cause(), statusErr)
	}
}

func TestChangeVerifyV5ReceiptOwnCommitExemption(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-v5-own-commit", "v5-own-commit", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "v5-own-commit")
	folderRel := filepath.Join("docs", "changes", "20260727-v5-own-commit")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	node, err := assembleChangeNodeFromFolder(repo, filepath.Join(repo, folderRel))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	verdict, statusErr := changeReceiptStatus(repo, folderRel, node, nil)
	if statusErr != nil || !verdict.OK {
		t.Fatalf("receipt-only commit must not stale: ok=%v reason=%q err=%v", verdict.OK, verdict.Cause(), statusErr)
	}
}

func TestChangeVerifySchemaV2(t *testing.T) {
	t.Run("writes-v2-fields-without-absolute-paths", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		body := shapeWithVerification("- **V1.** Smoke prose. Command: `true`. Expect: exit 0")
		dir := writeNewLayoutChange(t, repo, "20260728-schema-v2", "schema-v2", "2.0.0", body)
		commitAllChangeTest(t, repo, "docs: shape schema-v2")
		folderRel := filepath.Join("docs", "changes", "20260728-schema-v2")
		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("verify: %v\n%s", err, stdout.String())
		}
		receipt := mustReadVerifyReceipt(t, dir)
		if receipt.SchemaVersion != 2 {
			t.Fatalf("schema = %d, want 2", receipt.SchemaVersion)
		}
		if receipt.ScopeDigest == "" || receipt.DigestSpec != ChangeEvidenceDigestSpec || len(receipt.Exclusions) == 0 {
			t.Fatalf("digest fields incomplete: %#v", receipt)
		}
		if receipt.VerifiedRootTree == "" || receipt.VerifiedCommit == "" {
			t.Fatalf("provenance missing: %#v", receipt)
		}
		if !receipt.WorktreeClean {
			t.Fatal("worktree_clean must be true after clean verify")
		}
		if receipt.Toolchain.Go == "" || receipt.Toolchain.OS == "" || receipt.Toolchain.Arch == "" {
			t.Fatalf("toolchain incomplete: %#v", receipt.Toolchain)
		}
		if len(receipt.ScopeSections) == 0 {
			t.Fatal("scope_sections required")
		}
		raw := mustRead(t, filepath.Join(dir, "receipts", "verify.json"))
		if bytes.Contains(raw, []byte(repo)) || bytes.Contains(raw, []byte(filepath.ToSlash(repo))) {
			t.Fatalf("receipt must not embed absolute paths: %s", raw)
		}
		if bytes.Contains(raw, []byte(`"cwd"`)) {
			t.Fatalf("cwd must be dropped: %s", raw)
		}
		wantExclusions := ChangeEvidenceExclusions()
		if len(receipt.Exclusions) != len(wantExclusions) {
			t.Fatalf("exclusions = %#v, want %#v", receipt.Exclusions, wantExclusions)
		}
		for i := range wantExclusions {
			if receipt.Exclusions[i] != wantExclusions[i] {
				t.Fatalf("exclusions[%d] = %q, want %q", i, receipt.Exclusions[i], wantExclusions[i])
			}
		}
	})

	t.Run("refuses-dirty-tracked-and-staged-but-not-untracked", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
		dir := writeNewLayoutChange(t, repo, "20260728-dirty", "dirty", "", body)
		commitAllChangeTest(t, repo, "docs: shape dirty")
		folderRel := filepath.Join("docs", "changes", "20260728-dirty")
		shapePath := filepath.Join(dir, "shape.md")
		original := mustRead(t, shapePath)

		// Tracked unstaged edit.
		if err := os.WriteFile(shapePath, append(original, '\n'), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel})
		if err == nil || !strings.Contains(err.Error(), "working tree differs from HEAD; commit before verifying") {
			t.Fatalf("tracked dirty err = %v, want dirty refusal", err)
		}

		// Restore working tree, then stage an edit.
		if err := os.WriteFile(shapePath, original, 0o644); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if err := os.WriteFile(shapePath, append(original, []byte("\n")...), 0o644); err != nil {
			t.Fatalf("WriteFile staged: %v", err)
		}
		gitCLI(t, repo, "add", filepath.ToSlash(filepath.Join("docs", "changes", "20260728-dirty", "shape.md")))
		err = (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel})
		if err == nil || !strings.Contains(err.Error(), "working tree differs from HEAD; commit before verifying") {
			t.Fatalf("staged dirty err = %v, want dirty refusal", err)
		}

		// Clean tracked/staged state; untracked files must not refuse.
		gitCLI(t, repo, "reset", "--hard", "HEAD")
		if err := os.WriteFile(filepath.Join(repo, "untracked-only.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile untracked: %v", err)
		}
		if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
			t.Fatalf("untracked must not refuse verify: %v", err)
		}
	})

	t.Run("criteria-text-edit-changes-digest", func(t *testing.T) {
		before := changeCriteriaDigest([]changeCriterion{{
			ID: "V1", Text: "Smoke prose.", Command: "true", Expect: "exit 0",
		}})
		after := changeCriteriaDigest([]changeCriterion{{
			ID: "V1", Text: "Smoke prose changed.", Command: "true", Expect: "exit 0",
		}})
		if before == after {
			t.Fatal("criterion text must participate in criteria_digest")
		}
		sameCommand := changeCriteriaDigest([]changeCriterion{{
			ID: "V1", Text: "Smoke prose.", Command: "true", Expect: "exit 0",
		}})
		if before != sameCommand {
			t.Fatal("identical criteria must digest identically")
		}
	})
}
