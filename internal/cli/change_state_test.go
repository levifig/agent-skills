package cli

import (
	"bytes"
	"encoding/json"
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
	if changeStructurallyCleanForState(repo, mustAssembleNode(t, repo, folderRel), commandOutput) {
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
