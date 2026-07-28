package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const provenanceFence = "```"

// provenanceTaskFile assembles a task packet body from its lines so fixtures can
// place fence markers at a controlled distance from the flipped checkbox.
func provenanceTaskFile(bodyLines ...string) string {
	return "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n" + strings.Join(bodyLines, "\n") + "\n"
}

// provenanceDistantFenceTask opens a fence ten lines above the example checkbox,
// well outside any --unified=3 window around it.
func provenanceDistantFenceTask(exampleBox, realBox string) string {
	return provenanceTaskFile(
		"## Example",
		"",
		provenanceFence+"bash",
		"loaf change check",
		"loaf change tasks --json",
		"loaf change verify docs/changes/20260727-prov",
		"loaf release --dry-run --bump release",
		"git log --oneline",
		"go test ./internal/cli/",
		"go vet ./internal/cli/",
		"npm run build",
		"npm run typecheck",
		"loaf check",
		exampleBox,
		provenanceFence,
		"",
		"## Steps",
		"",
		realBox,
	)
}

func TestProvenanceMarkdownFencedLines(t *testing.T) {
	content := provenanceDistantFenceTask("- [ ] Example flip", "- [ ] Real step")
	fenced := markdownFencedLines(content)

	lines := strings.Split(content, "\n")
	exampleLine, realLine, openerLine := 0, 0, 0
	for index, line := range lines {
		switch {
		case strings.Contains(line, "Example flip"):
			exampleLine = index + 1
		case strings.Contains(line, "Real step"):
			realLine = index + 1
		case line == provenanceFence+"bash":
			openerLine = index + 1
		}
	}
	if exampleLine == 0 || realLine == 0 || openerLine == 0 {
		t.Fatalf("fixture lines not located: opener=%d example=%d real=%d", openerLine, exampleLine, realLine)
	}
	if exampleLine-openerLine < 4 {
		t.Fatalf("fixture fence is inside a unified=3 window: opener=%d example=%d", openerLine, exampleLine)
	}
	if !fenced.fenced(exampleLine) {
		t.Fatalf("example checkbox at line %d should be fenced", exampleLine)
	}
	if fenced.fenced(realLine) {
		t.Fatalf("real step at line %d should not be fenced", realLine)
	}
	if !fenced.fenced(openerLine) {
		t.Fatalf("opening fence marker at line %d should be fenced", openerLine)
	}
	if fenced.fenced(1) {
		t.Fatalf("frontmatter line 1 should not be fenced")
	}
	var nilMap changeFencedLines
	if nilMap.fenced(exampleLine) {
		t.Fatalf("absent image should report nothing fenced")
	}
}

func TestProvenanceFlipGrammar(t *testing.T) {
	cases := []struct {
		name       string
		diff       string
		preFenced  changeFencedLines
		postFenced changeFencedLines
		want       bool
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
			name: "fenced block flip with the fence in the window",
			diff: "" +
				"@@ -10,3 +10,3 @@\n" +
				" " + provenanceFence + "\n" +
				"- - [ ] Example flip\n" +
				"+ - [x] Example flip\n" +
				" " + provenanceFence + "\n",
			preFenced:  changeFencedLines{10: true, 11: true, 12: true},
			postFenced: changeFencedLines{10: true, 11: true, 12: true},
			want:       false,
		},
		{
			name: "fenced block flip with no fence in the window",
			diff: "" +
				"@@ -30,1 +30,1 @@\n" +
				"- - [ ] Example flip\n" +
				"+ - [x] Example flip\n",
			preFenced:  changeFencedLines{30: true},
			postFenced: changeFencedLines{30: true},
			want:       false,
		},
		{
			name: "unfenced flip beside a fenced region elsewhere in the file",
			diff: "" +
				"@@ -40,1 +40,1 @@\n" +
				"- - [ ] Real step\n" +
				"+ - [x] Real step\n",
			preFenced:  changeFencedLines{20: true, 21: true, 22: true},
			postFenced: changeFencedLines{20: true, 21: true, 22: true},
			want:       true,
		},
		{
			name: "removed line fenced only in the pre-image",
			diff: "" +
				"@@ -12,1 +12,1 @@\n" +
				"- - [ ] Do it\n" +
				"+ - [x] Do it\n",
			preFenced: changeFencedLines{12: true},
			want:      false,
		},
		{
			name: "added line fenced only in the post-image",
			diff: "" +
				"@@ -12,1 +12,1 @@\n" +
				"- - [ ] Do it\n" +
				"+ - [x] Do it\n",
			postFenced: changeFencedLines{12: true},
			want:       false,
		},
		{
			name: "context lines advance both file positions",
			diff: "" +
				"@@ -8,5 +8,5 @@\n" +
				" ## Steps\n" +
				" \n" +
				"- - [ ] Do it\n" +
				"+ - [x] Do it\n" +
				" \n" +
				" done\n",
			preFenced:  changeFencedLines{8: true, 9: true, 11: true},
			postFenced: changeFencedLines{8: true, 9: true, 11: true},
			want:       true,
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
		{
			name: "unparseable hunk header credits nothing",
			diff: "" +
				"@@ malformed @@\n" +
				"- - [ ] Do it\n" +
				"+ - [x] Do it\n",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := diffContainsCheckboxFlip(tc.diff, tc.preFenced, tc.postFenced)
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
			beforeTask: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Example\n\n" + provenanceFence + "\n- [ ] Example flip\n" + provenanceFence + "\n\n## Steps\n\n- [ ] Real step\n",
			after: edit{
				taskBody: "---\nchange: prov\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Example\n\n" + provenanceFence + "\n- [x] Example flip\n" + provenanceFence + "\n\n## Steps\n\n- [ ] Real step\n",
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "distant fence block flip",
			beforeTask: provenanceDistantFenceTask("- [ ] Example flip", "- [ ] Real step"),
			after: edit{
				taskBody: provenanceDistantFenceTask("- [x] Example flip", "- [ ] Real step"),
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: false,
		},
		{
			name:       "genuine flip with a distant fenced example elsewhere",
			beforeTask: provenanceDistantFenceTask("- [ ] Example flip", "- [ ] Real step"),
			after: edit{
				taskBody: provenanceDistantFenceTask("- [ ] Example flip", "- [x] Real step"),
				outside:  "package main\n\nfunc main() {}\n",
			},
			wantFlip: true,
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

// TestProvenanceFlipHandlesImageEdges covers the commits with only one file
// image: a task file created checked, and a task file deleted outright.
func TestProvenanceFlipHandlesImageEdges(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-prov", "prov", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape")

	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.WriteFile(task, []byte(provenanceTaskFile("## Steps", "", "- [x] Do it")), 0o644); err != nil {
		t.Fatalf("WriteFile created task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: create task checked")

	folderRel := filepath.ToSlash(filepath.Join("docs", "changes", "20260727-prov"))
	status, err := changeFolderExecuted(repo, folderRel, changeLayoutNew, nil)
	if err != nil {
		t.Fatalf("changeFolderExecuted after creation: %v", err)
	}
	if !status.PathExecuted {
		t.Fatalf("path grade missing after creation commit")
	}
	if status.FlipExecuted {
		t.Fatalf("a task file created already checked is not a flip")
	}

	if err := os.Remove(task); err != nil {
		t.Fatalf("Remove task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() { _ = 1 }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go after delete: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: drop the task file")

	status, err = changeFolderExecuted(repo, folderRel, changeLayoutNew, nil)
	if err != nil {
		t.Fatalf("changeFolderExecuted after deletion: %v", err)
	}
	if status.FlipExecuted {
		t.Fatalf("deleting a task file is not a flip")
	}
}

// TestProvenanceFlipHandlesRootCommit proves the missing pre-image of a root
// commit is scored as a non-event rather than erroring.
func TestProvenanceFlipHandlesRootCommit(t *testing.T) {
	repo := realpath(t, t.TempDir())
	gitCLI(t, repo, "init", "-b", "main")
	writeReleaseVersionFiles(t, repo, "2.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-prov", "prov", "2.0.0", "")
	if err := os.WriteFile(filepath.Join(dir, "tasks", "TASK-001-work.md"), []byte(provenanceTaskFile("## Steps", "", "- [x] Do it")), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "feat: root commit carries the change")

	head, err := commandOutput(repo, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	taskRel := "docs/changes/20260727-prov/tasks/TASK-001-work.md"
	flipped, err := commitFlipsTaskCheckboxes(repo, strings.TrimSpace(head), []string{taskRel}, nil)
	if err != nil {
		t.Fatalf("commitFlipsTaskCheckboxes on root commit: %v", err)
	}
	if flipped {
		t.Fatalf("root commit creating a checked task file is not a flip")
	}
}
