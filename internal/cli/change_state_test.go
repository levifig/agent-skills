package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeListShowStateAgreementAcrossLadder(t *testing.T) {
	cases := []struct {
		name      string
		wantState string
		setup     func(t *testing.T, repo string) (folderRel, slug string)
	}{
		{
			name:      "captured",
			wantState: "captured",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "ladder-captured", "--brief"}); err != nil {
					t.Fatalf("init: %v", err)
				}
				return mustFindChangeFolder(t, repo, "ladder-captured"), "ladder-captured"
			},
		},
		{
			name:      "shaped",
			wantState: "shaped",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				dir := writeNewLayoutChange(t, repo, "20260727-ladder-shaped", "ladder-shaped", "", "# Title only\n")
				return relFromRoot(repo, dir), "ladder-shaped"
			},
		},
		{
			name:      "executable",
			wantState: "executable",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				dir := writeNewLayoutChange(t, repo, "20260727-ladder-executable", "ladder-executable", "", "")
				return relFromRoot(repo, dir), "ladder-executable"
			},
		},
		{
			name:      "executing",
			wantState: "executing",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				dir := writeNewLayoutChange(t, repo, "20260727-ladder-executing", "ladder-executing", "", "")
				task := filepath.Join(dir, "tasks", "TASK-001-work.md")
				if err := os.WriteFile(task, []byte("---\nchange: ladder-executing\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"), 0o644); err != nil {
					t.Fatalf("WriteFile task: %v", err)
				}
				commitAllChangeTest(t, repo, "docs: shape ladder-executing")
				if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
					t.Fatalf("WriteFile main.go: %v", err)
				}
				if err := os.WriteFile(task, []byte("---\nchange: ladder-executing\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n\nnote\n"), 0o644); err != nil {
					t.Fatalf("WriteFile task touch: %v", err)
				}
				commitAllChangeTest(t, repo, "chore: path grade ladder-executing")
				return relFromRoot(repo, dir), "ladder-executing"
			},
		},
		{
			name:      "complete",
			wantState: "complete",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				dir := writeNewLayoutChange(t, repo, "20260727-ladder-complete", "ladder-complete", "", "")
				flipExecuteChange(t, repo, dir, "ladder-complete")
				return relFromRoot(repo, dir), "ladder-complete"
			},
		},
		{
			name:      "verified",
			wantState: "verified",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				dir := writeNewLayoutChange(t, repo, "20260727-ladder-verified", "ladder-verified", "1.0.0", "")
				flipExecuteChange(t, repo, dir, "ladder-verified")
				folderRel := relFromRoot(repo, dir)
				var stdout bytes.Buffer
				if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
					t.Fatalf("verify: %v\n%s", err, stdout.String())
				}
				commitAllChangeTest(t, repo, "chore: commit ladder-verified receipt")
				return folderRel, "ladder-verified"
			},
		},
		{
			name:      "structurally-rejected-receipt",
			wantState: "executing",
			setup: func(t *testing.T, repo string) (string, string) {
				t.Helper()
				dir := writeNewLayoutChange(t, repo, "20260727-ladder-rejected", "ladder-rejected", "1.0.0", "")
				flipExecuteChange(t, repo, dir, "ladder-rejected")
				later := filepath.Join(dir, "tasks", "TASK-002-later.md")
				if err := os.WriteFile(later, []byte("---\nchange: ladder-rejected\nid: TASK-002\ntitle: Later\nstatus: in-progress\n---\n\n# Later\n\n## Steps\n\n- [ ] Descoped\n"), 0o644); err != nil {
					t.Fatalf("WriteFile TASK-002: %v", err)
				}
				commitAllChangeTest(t, repo, "docs: add banned task frontmatter")
				folderRel := relFromRoot(repo, dir)
				var stdout bytes.Buffer
				if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
					t.Fatalf("verify: %v\n%s", err, stdout.String())
				}
				commitAllChangeTest(t, repo, "chore: commit ladder-rejected receipt")
				return folderRel, "ladder-rejected"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			if tc.wantState == "verified" || tc.name == "structurally-rejected-receipt" {
				writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
			}
			folderRel, slug := tc.setup(t, repo)

			listState := changeListStateForSlug(t, repo, slug)
			showState := changeShowState(t, repo, folderRel)
			checkState := changeCheckState(t, repo, folderRel)

			if listState != tc.wantState {
				t.Fatalf("list state = %q, want %q", listState, tc.wantState)
			}
			if showState != listState {
				t.Fatalf("show state = %q, list state = %q", showState, listState)
			}
			if checkState != listState {
				t.Fatalf("check state = %q, list state = %q", checkState, listState)
			}
		})
	}
}

func TestChangeStateStructurallyRejectedReceiptNeverDisplaysVerified(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-banned-verified", "banned-verified", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "banned-verified")
	later := filepath.Join(dir, "tasks", "TASK-002-later.md")
	if err := os.WriteFile(later, []byte("---\nchange: banned-verified\nid: TASK-002\ntitle: Later\nstatus: in-progress\n---\n\n# Later\n\n## Steps\n\n- [ ] Descoped\n"), 0o644); err != nil {
		t.Fatalf("WriteFile TASK-002: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: add banned task frontmatter")
	folderRel := relFromRoot(repo, dir)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit receipt under banned frontmatter")

	listState := changeListStateForSlug(t, repo, "banned-verified")
	showState := changeShowState(t, repo, folderRel)
	showPlain := changeShowPlainState(t, repo, folderRel)
	checkState := changeCheckState(t, repo, folderRel)
	for _, got := range []string{listState, showState, showPlain, checkState} {
		if got == "verified" {
			t.Fatalf("surfaces report verified despite structural rejection: list=%q show=%q plain=%q check=%q", listState, showState, showPlain, checkState)
		}
	}
	if listState != showState || listState != showPlain || listState != checkState {
		t.Fatalf("surfaces disagree: list=%q show=%q plain=%q check=%q", listState, showState, showPlain, checkState)
	}
	checkOut := changeCheckFindings(t, repo, folderRel)
	if !strings.Contains(checkOut, "status") || !strings.Contains(checkOut, "banned") {
		t.Fatalf("check must show the violation; got %q", checkOut)
	}

	if err := os.WriteFile(later, []byte("---\nchange: banned-verified\nid: TASK-002\ntitle: Later\n---\n\n# Later\n\n## Steps\n\n- [ ] Descoped\n"), 0o644); err != nil {
		t.Fatalf("WriteFile repair: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: drop banned key")
	// Receipt may stale from the non-receipt path edit; re-verify and commit.
	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("re-verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: re-verify after repair")

	listState = changeListStateForSlug(t, repo, "banned-verified")
	showState = changeShowState(t, repo, folderRel)
	showPlain = changeShowPlainState(t, repo, folderRel)
	checkState = changeCheckState(t, repo, folderRel)
	if listState != "verified" || showState != "verified" || showPlain != "verified" || checkState != "verified" {
		t.Fatalf("after repair want verified everywhere; list=%q show=%q plain=%q check=%q", listState, showState, showPlain, checkState)
	}
}

func changeShowPlainState(t *testing.T, repo, folderRel string) string {
	t.Helper()
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "show", folderRel}); err != nil {
		t.Fatalf("show: %v\n%s", err, stdout.String())
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(stripANSI(line))
		if strings.HasPrefix(line, "State:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "State:"))
		}
		if strings.HasPrefix(line, "state:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "state:"))
		}
	}
	t.Fatalf("show output missing state line:\n%s", stdout.String())
	return ""
}

func changeCheckFindings(t *testing.T, repo, folderRel string) string {
	t.Helper()
	var stdout bytes.Buffer
	_ = (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "check", folderRel, "--json"})
	return stdout.String()
}

func mustFindChangeFolder(t *testing.T, repo, slug string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repo, "docs", "changes"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	suffix := "-" + slug
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return filepath.ToSlash(filepath.Join("docs", "changes", entry.Name()))
		}
	}
	t.Fatalf("change folder for %q not found", slug)
	return ""
}

func changeListStateForSlug(t *testing.T, repo, slug string) string {
	t.Helper()
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "list", "--json"}); err != nil {
		t.Fatalf("list: %v\n%s", err, stdout.String())
	}
	var result changeListUnitJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal list: %v", err)
	}
	for _, unit := range result.Units {
		if unit.Slug == slug {
			return unit.State
		}
	}
	t.Fatalf("slug %q missing from list", slug)
	return ""
}

func changeShowState(t *testing.T, repo, folderRel string) string {
	t.Helper()
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "show", folderRel, "--json"}); err != nil {
		t.Fatalf("show: %v\n%s", err, stdout.String())
	}
	var result changeShowJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal show: %v", err)
	}
	return result.State
}

func changeCheckState(t *testing.T, repo, folderRel string) string {
	t.Helper()
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "check", folderRel, "--json"})
	// check may exit non-zero for shaped (violations); still decode JSON.
	var result changeCheckJSON
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("check err=%v unmarshal=%v stdout=%s", err, unmarshalErr, stdout.String())
	}
	return result.State
}

// --- TASK-025: verified/state evidence reads committed HEAD, not the working tree ---

func TestChangeStateIgnoresUncommittedBannedFrontmatter(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-clean-verified", "clean-verified", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "clean-verified")
	folderRel := relFromRoot(repo, dir)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit clean-verified receipt")

	if got := changeShowState(t, repo, folderRel); got != "verified" {
		t.Fatalf("committed-clean state = %q, want verified", got)
	}

	// Dirty working-tree banned frontmatter must not demote the committed-clean member.
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.WriteFile(task, []byte("---\nchange: clean-verified\nid: TASK-001\ntitle: Work\nstatus: in-progress\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile dirty banned: %v", err)
	}
	if got := changeShowState(t, repo, folderRel); got != "verified" {
		t.Fatalf("dirty banned frontmatter demoted state to %q, want verified", got)
	}
	checkOut := changeCheckFindings(t, repo, folderRel)
	if !strings.Contains(checkOut, "banned") && !strings.Contains(checkOut, "status") {
		t.Fatalf("check must still see the working-tree ban; got %q", checkOut)
	}
}

func TestChangeStateEvidenceSeesCommittedTasksWhenWorkingTreeDeletesTasks(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-head-tasks", "head-tasks", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "head-tasks")
	banned := filepath.Join(dir, "tasks", "TASK-002-later.md")
	if err := os.WriteFile(banned, []byte("---\nchange: head-tasks\nid: TASK-002\ntitle: Later\nstatus: in-progress\n---\n\n# Later\n\n## Steps\n\n- [ ] Descoped\n"), 0o644); err != nil {
		t.Fatalf("WriteFile banned: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: commit banned task at HEAD")
	folderRel := relFromRoot(repo, dir)

	// Evidence path must still see the committed banned task after a WT deletion.
	if err := os.RemoveAll(filepath.Join(dir, "tasks")); err != nil {
		t.Fatalf("RemoveAll tasks: %v", err)
	}
	if clean, _ := changeStructurallyCleanForState(repo, mustAssembleNode(t, repo, folderRel), commandOutput); clean {
		t.Fatal("evidence path must still see committed banned frontmatter after WT task delete")
	}
	report, err := changeCohortStructuralReport(repo, mustAssembleHEADNode(t, repo, folderRel), mustLoadHEADNodes(t, repo), commandOutput)
	if err != nil {
		t.Fatalf("cohort structural: %v", err)
	}
	joined := strings.Join(report.Violations, "\n")
	if !strings.Contains(joined, "banned") && !strings.Contains(joined, "status") {
		t.Fatalf("gate composite must keep committed task findings; got %q", joined)
	}
}

func mustAssembleNode(t *testing.T, repo, folderRel string) changeNode {
	t.Helper()
	node, err := assembleChangeNodeFromFolder(repo, filepath.Join(repo, filepath.FromSlash(folderRel)))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return node
}

func mustLoadHEADNodes(t *testing.T, repo string) []changeNode {
	t.Helper()
	nodes, err := loadChangeNodesAtHEAD(repo)
	if err != nil {
		t.Fatalf("loadChangeNodesAtHEAD: %v", err)
	}
	return nodes
}

func mustAssembleHEADNode(t *testing.T, repo, folderRel string) changeNode {
	t.Helper()
	nodes := mustLoadHEADNodes(t, repo)
	node, ok := changeNodeForFolder(nodes, folderRel)
	if !ok {
		t.Fatalf("HEAD node for %s missing", folderRel)
	}
	return node
}

func TestChangeShowListSurfaceStructuralLoadError(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")

	victim := writeNewLayoutChange(t, repo, "20260727-load-victim", "load-victim", "1.0.0", "")
	flipExecuteChange(t, repo, victim, "load-victim")
	victimRel := relFromRoot(repo, victim)

	other := writeNewLayoutChange(t, repo, "20260727-load-other", "load-other", "", "")
	flipExecuteChange(t, repo, other, "load-other")
	otherRel := relFromRoot(repo, other)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", victimRel}); err != nil {
		t.Fatalf("verify victim: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit victim receipt")

	if got := changeShowState(t, repo, victimRel); got != "verified" {
		t.Fatalf("victim state before fault = %q, want verified", got)
	}
	otherBefore := changeShowState(t, repo, otherRel)
	if otherBefore == "verified" {
		t.Fatalf("untargeted other must not be verified; got %q", otherBefore)
	}

	old := changeEvidenceGitOutput
	changeEvidenceGitOutput = func(cwd, name string, args ...string) (string, error) {
		// Fault only the recursive docs/changes listing used to load HEAD nodes —
		// not per-path ls-tree reads provenance uses for task pre-images.
		if name == "git" && len(args) > 0 && args[0] == "ls-tree" {
			recursive, docsChanges := false, false
			for _, a := range args {
				if a == "-r" {
					recursive = true
				}
				if a == "docs/changes" {
					docsChanges = true
				}
			}
			if recursive && docsChanges {
				return "", fmt.Errorf("read change.json: permission denied")
			}
		}
		return commandOutput(cwd, name, args...)
	}
	defer func() { changeEvidenceGitOutput = old }()

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "show", victimRel}); err != nil {
		t.Fatalf("show victim: %v\n%s", err, stdout.String())
	}
	showOut := stripANSI(stdout.String())
	if !strings.Contains(showOut, "state:") {
		t.Fatalf("show missing readable state line; got:\n%s", showOut)
	}
	if strings.Contains(showOut, "state:    verified") || strings.Contains(showOut, "state: verified") {
		t.Fatalf("victim must demote under structural load error; got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "warn:") || !strings.Contains(showOut, "structural evaluation failed:") || !strings.Contains(showOut, "permission denied") {
		t.Fatalf("show must surface structural load warning; got:\n%s", showOut)
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "show", victimRel, "--json"}); err != nil {
		t.Fatalf("show json: %v\n%s", err, stdout.String())
	}
	var show changeShowJSON
	if err := json.Unmarshal(stdout.Bytes(), &show); err != nil {
		t.Fatalf("Unmarshal show: %v", err)
	}
	if show.State == "verified" {
		t.Fatalf("show JSON state = verified, want demoted")
	}
	if !findingsContain(show.Warnings, "structural evaluation failed:") {
		t.Fatalf("show JSON warnings = %#v, want structural evaluation failed", show.Warnings)
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "list", "--json"}); err != nil {
		t.Fatalf("list: %v\n%s", err, stdout.String())
	}
	var list changeListUnitJSON
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
		t.Fatalf("Unmarshal list: %v", err)
	}
	var victimUnit, otherUnit *changeListUnit
	for i := range list.Units {
		switch list.Units[i].Slug {
		case "load-victim":
			victimUnit = &list.Units[i]
		case "load-other":
			otherUnit = &list.Units[i]
		}
	}
	if victimUnit == nil || otherUnit == nil {
		t.Fatalf("list units missing members: %#v", list.Units)
	}
	if victimUnit.State == "verified" {
		t.Fatalf("list victim state = verified, want demoted")
	}
	if !findingsContain(victimUnit.Warnings, "structural evaluation failed:") {
		t.Fatalf("list victim warnings = %#v", victimUnit.Warnings)
	}
	if !findingsContain(list.Warnings, "structural evaluation failed:") {
		t.Fatalf("list top-level warnings = %#v", list.Warnings)
	}
	if otherUnit.State != otherBefore {
		t.Fatalf("other member affected: state=%q, want %q", otherUnit.State, otherBefore)
	}
	if len(otherUnit.Warnings) != 0 {
		t.Fatalf("other member warnings = %#v, want none", otherUnit.Warnings)
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "list"}); err != nil {
		t.Fatalf("list plain: %v\n%s", err, stdout.String())
	}
	plain := stripANSI(stdout.String())
	if !strings.Contains(plain, "warn:") || !strings.Contains(plain, "structural evaluation failed:") {
		t.Fatalf("list plain must surface warn; got:\n%s", plain)
	}
}

// TASK-028: receipt check receives the HEAD node (content + folder). An
// uncommitted criteria edit or folder rename must not move the verified rung.
func TestChangeStateVerifiedRungIgnoresDirtyCriteriaAndRename(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-head-receipt-node", "head-receipt-node", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "head-receipt-node")
	folderRel := relFromRoot(repo, dir)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit verified receipt")
	if got := changeShowState(t, repo, folderRel); got != "verified" {
		t.Fatalf("baseline state = %q, want verified", got)
	}

	// Dirty criteria on disk: working-tree shape would mismatch the receipt digest.
	shapePath := filepath.Join(dir, "shape.md")
	shape, err := os.ReadFile(shapePath)
	if err != nil {
		t.Fatalf("ReadFile shape: %v", err)
	}
	dirty := strings.Replace(string(shape), "Command: `true`", "Command: `false`", 1)
	if dirty == string(shape) {
		t.Fatal("fixture shape missing expected V1 command to dirty")
	}
	if err := os.WriteFile(shapePath, []byte(dirty), 0o644); err != nil {
		t.Fatalf("WriteFile dirty shape: %v", err)
	}
	if got := changeShowState(t, repo, folderRel); got != "verified" {
		t.Fatalf("dirty criteria demoted state to %q, want verified", got)
	}

	// Restore shape; a working-tree folder that does not match HEAD must still
	// resolve the receipt via the HEAD node's folder (slug fallback).
	if err := os.WriteFile(shapePath, shape, 0o644); err != nil {
		t.Fatalf("restore shape: %v", err)
	}
	wtNode := mustAssembleNode(t, repo, folderRel)
	wtNode.Folder = "docs/changes/20260727-head-receipt-renamed"
	wtNode.Content = strings.Replace(wtNode.Content, "Command: `true`", "Command: `false`", 1)
	_, evidenceGit, pinErr := pinEvidenceAtHEAD(repo, commandOutput)
	if pinErr != nil {
		t.Fatalf("pin: %v", pinErr)
	}
	ok, receiptErr, clean, evalErr, _ := evaluateVerifiedRungAtCommit(repo, wtNode, evidenceGit)
	if evalErr != "" || receiptErr != nil || !ok || !clean {
		t.Fatalf("renamed+dirty WT node: ok=%v clean=%v receiptErr=%v evalErr=%q, want verified rung", ok, clean, receiptErr, evalErr)
	}
}

// TASK-028: evidence derivation resolves HEAD once; subsequent evidence git args
// carry the pinned SHA, so a mid-derivation commit cannot split the inputs.
func TestChangeStateEvidencePinsHEADOnce(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-pin-head", "pin-head", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "pin-head")
	folderRel := relFromRoot(repo, dir)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit pin-head receipt")

	node := mustAssembleNode(t, repo, folderRel)
	var revParseHEAD int
	var pinnedSHA string
	var postPinSymbolic int
	seam := func(cwd, name string, args ...string) (string, error) {
		if name == "git" && len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			revParseHEAD++
			out, err := commandOutput(cwd, name, args...)
			pinnedSHA = strings.TrimSpace(out)
			return out, err
		}
		if pinnedSHA != "" {
			for _, a := range args {
				if a == "HEAD" || strings.HasPrefix(a, "HEAD:") || strings.HasSuffix(a, "..HEAD") || strings.HasPrefix(a, "HEAD..") {
					postPinSymbolic++
				}
			}
		}
		return commandOutput(cwd, name, args...)
	}
	state, warnings := deriveChangeStateDetailed(repo, node, seam)
	if state != "verified" {
		t.Fatalf("state = %q warnings=%v, want verified", state, warnings)
	}
	if revParseHEAD != 1 {
		t.Fatalf("rev-parse HEAD count = %d, want exactly 1 pin", revParseHEAD)
	}
	if pinnedSHA == "" {
		t.Fatal("pin did not capture a SHA")
	}
	if postPinSymbolic != 0 {
		t.Fatalf("post-pin symbolic HEAD tokens = %d, want 0", postPinSymbolic)
	}
}

// TASK-028: gate preflight also pins HEAD once for the whole evidence derivation.
func TestReleaseCohortGatePinsHEADOnce(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-gate-pin", "gate-pin", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "gate-pin")
	folderRel := relFromRoot(repo, dir)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit gate-pin receipt")

	var revParseHEAD int
	seam := func(cwd, name string, args ...string) (string, error) {
		if name == "git" && len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			revParseHEAD++
		}
		return commandOutput(cwd, name, args...)
	}
	if err := releaseCohortPreflightWithOutput(repo, "1.0.0", seam, nil); err != nil {
		t.Fatalf("gate: %v", err)
	}
	if revParseHEAD != 1 {
		t.Fatalf("gate rev-parse HEAD count = %d, want exactly 1 pin", revParseHEAD)
	}
}

// TASK-029: a truncated committed receipt demotes conservatively and surfaces
// the load error on show, list, and check --json (same warning plumbing).
func TestChangeStateTruncatedReceiptSurfacesWarning(t *testing.T) {
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-trunc-receipt", "trunc-receipt", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "trunc-receipt")
	folderRel := relFromRoot(repo, dir)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit valid receipt")
	if got := changeShowState(t, repo, folderRel); got != "verified" {
		t.Fatalf("baseline = %q, want verified", got)
	}

	receiptPath := filepath.Join(dir, "receipts", "verify.json")
	if err := os.WriteFile(receiptPath, []byte(`{"schema_version":1,"cri`), 0o644); err != nil {
		t.Fatalf("WriteFile truncated receipt: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit truncated receipt")

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "show", folderRel}); err != nil {
		t.Fatalf("show: %v\n%s", err, stdout.String())
	}
	showOut := stripANSI(stdout.String())
	if strings.Contains(showOut, "state:    verified") || strings.Contains(showOut, "state: verified") {
		t.Fatalf("truncated receipt must demote; got:\n%s", showOut)
	}
	if !strings.Contains(showOut, "warn:") || !strings.Contains(showOut, "receipt evaluation failed:") {
		t.Fatalf("show must surface receipt warning; got:\n%s", showOut)
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "list", "--json"}); err != nil {
		t.Fatalf("list: %v\n%s", err, stdout.String())
	}
	var list changeListUnitJSON
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
		t.Fatalf("Unmarshal list: %v", err)
	}
	var unit *changeListUnit
	for i := range list.Units {
		if list.Units[i].Slug == "trunc-receipt" {
			unit = &list.Units[i]
			break
		}
	}
	if unit == nil {
		t.Fatalf("list missing trunc-receipt: %#v", list.Units)
	}
	if unit.State == "verified" {
		t.Fatalf("list state = verified, want demoted")
	}
	if !findingsContain(unit.Warnings, "receipt evaluation failed:") {
		t.Fatalf("list unit warnings = %#v", unit.Warnings)
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "check", folderRel, "--json"}); err != nil {
		t.Fatalf("check: %v\n%s", err, stdout.String())
	}
	var check changeCheckJSON
	if err := json.Unmarshal(stdout.Bytes(), &check); err != nil {
		t.Fatalf("Unmarshal check: %v", err)
	}
	if check.State == "verified" {
		t.Fatalf("check JSON state = verified, want demoted")
	}
	if !findingsContain(check.Warnings, "receipt evaluation failed:") {
		t.Fatalf("check JSON warnings = %#v, want receipt evaluation failed", check.Warnings)
	}
}
