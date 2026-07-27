package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProvenanceFlipGrammar(t *testing.T) {
	cases := []struct {
		name string
		diff string
		want bool
	}{
		{
			name: "plain flip",
			diff: "" +
				"@@ -8,1 +8,1 @@\n" +
				"- - [ ] Do it\n" +
				"+ - [x] Do it\n",
			want: true,
		},
		{
			name: "flip with unrelated prose in same hunk",
			diff: "" +
				"@@ -6,4 +6,5 @@\n" +
				" ## Steps\n" +
				"- - [ ] Do it\n" +
				"+ - [x] Do it\n" +
				"+\n" +
				"+ note about the work\n",
			want: true,
		},
		{
			name: "squash batch several flips",
			diff: "" +
				"@@ -10,1 +10,1 @@\n" +
				"- - [ ] First step\n" +
				"+ - [x] First step\n" +
				"@@ -20,1 +20,1 @@\n" +
				"- - [ ] Second step\n" +
				"+ - [x] Second step\n" +
				"@@ -30,1 +30,1 @@\n" +
				"- - [ ] Third step\n" +
				"+ - [x] Third step\n",
			want: true,
		},
		{
			name: "reverse flip",
			diff: "" +
				"@@ -8,1 +8,1 @@\n" +
				"- - [x] Do it\n" +
				"+ - [ ] Do it\n",
			want: false,
		},
		{
			name: "added unchecked",
			diff: "" +
				"@@ -10,0 +11,1 @@\n" +
				"+ - [ ] Brand new step\n",
			want: false,
		},
		{
			name: "whitespace only",
			diff: "" +
				"@@ -8,1 +8,1 @@\n" +
				"- - [ ] Do it\n" +
				"+ - [ ] Do it  \n",
			want: false,
		},
		{
			name: "title only",
			diff: "" +
				"@@ -8,1 +8,1 @@\n" +
				"- - [ ] Do it\n" +
				"+ - [ ] Do it now\n",
			want: false,
		},
		{
			name: "fenced block flip",
			diff: "" +
				"@@ -10,3 +10,3 @@\n" +
				" ```\n" +
				"- - [ ] Example flip\n" +
				"+ - [x] Example flip\n" +
				" ```\n",
			want: false,
		},
		{
			name: "delete plus add without shared label",
			diff: "" +
				"@@ -8,1 +8,1 @@\n" +
				"- - [ ] Label A\n" +
				"+ - [x] Label B\n",
			want: false,
		},
		{
			name: "different hunks no shared label pairing across hunks",
			diff: "" +
				"@@ -8,1 +8,0 @@\n" +
				"- - [ ] Label A\n" +
				"@@ -20,0 +20,1 @@\n" +
				"+ - [x] Label A\n",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := diffContainsCheckboxFlip(tc.diff)
			if got != tc.want {
				t.Fatalf("diffContainsCheckboxFlip() = %v, want %v\ndiff:\n%s", got, tc.want, tc.diff)
			}
		})
	}
}

func TestProvenanceFlipGrammarCommitFixtures(t *testing.T) {
	type edit struct {
		taskBody string
		outside  string
	}
	cases := []struct {
		name       string
		beforeTask string
		after      edit
		wantFlip   bool
	}{
		{
			name:       "plain flip",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: true,
		},
		{
			name:       "flip with unrelated prose",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n\nImplementation notes landed with the flip.\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: true,
		},
		{
			name:       "squash batch several flips",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] First step\n\n## More\n\npad\n\n- [ ] Second step\n\n## End\n\npad\n\n- [ ] Third step\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] First step\n\n## More\n\npad\n\n- [x] Second step\n\n## End\n\npad\n\n- [x] Third step\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: true,
		},
		{
			name:       "reverse flip",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "added unchecked",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n- [ ] Brand new step\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "whitespace only",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it  \n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "title only",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it now\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "fenced block flip",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Example\n\n```\n- [ ] Example flip\n```\n\n## Steps\n\n- [ ] Real step\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Example\n\n```\n- [x] Example flip\n```\n\n## Steps\n\n- [ ] Real step\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "delete plus add without shared label",
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Label A\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Label B\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
			dir := writeNewLayoutChange(t, repo, "20260727-prov", "prov", "2.0.0", "")
			task := filepath.Join(dir, "tasks", "TASK-001-work.md")
			if err := os.WriteFile(task, []byte(tc.beforeTask), 0o644); err != nil {
				t.Fatalf("WriteFile before: %v", err)
			}
			if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
				t.Fatalf("WriteFile main.go: %v", err)
			}
			commitAllChangeTest(t, repo, "docs: shape")

			if err := os.WriteFile(task, []byte(tc.after.taskBody), 0o644); err != nil {
				t.Fatalf("WriteFile after task: %v", err)
			}
			if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(tc.after.outside), 0o644); err != nil {
				t.Fatalf("WriteFile after outside: %v", err)
			}
			commitAllChangeTest(t, repo, "feat: candidate edit")

			folderRel := filepath.ToSlash(filepath.Join("docs", "changes", "20260727-prov"))
			status, err := changeFolderExecuted(repo, folderRel, changeLayoutNew, nil)
			if err != nil {
				t.Fatalf("changeFolderExecuted: %v", err)
			}
			if !status.PathExecuted {
				t.Fatalf("path grade missing for companion outside edit")
			}
			if status.FlipExecuted != tc.wantFlip {
				t.Fatalf("FlipExecuted = %v, want %v", status.FlipExecuted, tc.wantFlip)
			}
			if !tc.wantFlip {
				err := releaseCohortPreflight(repo, "2.0.0", nil)
				if err == nil || !strings.Contains(err.Error(), "not executed") {
					t.Fatalf("negative fixture should block gate: %v", err)
				}
			}
		})
	}
}
