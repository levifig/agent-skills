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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initCLIGitRepo(t)
			if tc.wantState == "verified" {
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
