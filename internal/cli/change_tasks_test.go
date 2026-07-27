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

func TestChangeCheckLegacyEmitsDeprecationNotice(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := writeChangeFolder(t, repo, "20260710-legacy-demo", executableLineageDoc("legacy-demo", "line", "", ""))
	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v out=%+v", err, out)
	}
	if out.Layout != "legacy" {
		t.Fatalf("layout = %q, want legacy", out.Layout)
	}
	if !findingsContain(out.Notices, "Removal boundary") {
		t.Fatalf("notices = %v, want removal-boundary deprecation", out.Notices)
	}
}

func TestChangeCheckBriefOnlyReportedAsCaptured(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "brief-only", "--brief"}); err != nil {
		t.Fatalf("init --brief: %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-brief-only")
	out, err := runChangeCheckJSON(t, repo, folder)
	if err != nil {
		t.Fatalf("err = %v out=%+v", err, out)
	}
	if !out.Captured {
		t.Fatalf("captured = false, want true")
	}
	if out.Executable {
		t.Fatalf("executable = true, want false")
	}
	if !findingsContain(out.Warnings, "captured, not shaped") {
		t.Fatalf("warnings = %v, want captured warning", out.Warnings)
	}
}

func TestChangeTasksJSONStableIndex(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "task-index"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-task-index")
	tasksDir := filepath.Join(folder, "tasks")
	writeTask := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tasksDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	writeTask("TASK-001-parent.md", `---
change: task-index
id: TASK-001
title: Parent
---

# TASK-001 — Parent

## Steps

- [ ] Close when children done
`)
	writeTask("TASK-002-child.md", `---
change: task-index
id: TASK-002
title: Child
parent: TASK-001
blocks:
  - TASK-003
---

# TASK-002 — Child

## Steps

- [x] Done
`)
	writeTask("TASK-003-blocked.md", `---
change: task-index
id: TASK-003
title: Blocked
blocked-by:
  - TASK-002
---

# TASK-003 — Blocked

## Steps

- [ ] Waiting
`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "tasks", folder, "--json"}); err != nil {
		t.Fatalf("tasks: %v", err)
	}
	var result changeTasksJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, stdout.String())
	}
	if result.Change != "task-index" || len(result.Tasks) != 3 {
		t.Fatalf("result = %+v", result)
	}
	byID := map[string]changeTask{}
	for _, task := range result.Tasks {
		byID[task.ID] = task
	}
	if byID["TASK-001"].Children[0] != "TASK-002" {
		t.Fatalf("parent children = %v", byID["TASK-001"].Children)
	}
	if !byID["TASK-002"].Complete {
		t.Fatalf("TASK-002 should be complete")
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings = %v", result.Findings)
	}
}

func TestChangeTaskHygieneViolations(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "hygiene"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	today := time.Now().Format("20060102")
	folder := filepath.Join(repo, "docs", "changes", today+"-hygiene")
	tasksDir := filepath.Join(folder, "tasks")
	body := `---
change: hygiene
id: TASK-001
title: Bad
parent: TASK-001
status: done
unknown: x
relates-to: other-change/TASK-002
---

# Bad
`
	if err := os.WriteFile(filepath.Join(tasksDir, "TASK-001-bad.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := runChangeCheckJSON(t, repo, folder)
	if err == nil {
		t.Fatalf("want violations, got pass: %+v", out)
	}
	for _, want := range []string{"parent cannot be self", "banned", "unknown task frontmatter", "cross-change", "zero checkboxes"} {
		if !findingsContain(out.Findings, want) && !findingsContain(out.Warnings, want) {
			t.Fatalf("missing %q in findings=%v warnings=%v", want, out.Findings, out.Warnings)
		}
	}
}

func TestChangeShowDerivesPRSet(t *testing.T) {
	repo := initCLIGitRepo(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "init", "show-demo"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	today := time.Now().Format("20060102")
	folderRel := filepath.Join("docs", "changes", today+"-show-demo")
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test",
		"commit", "-m", "feat: land show demo (#141)")
	// Touch again under another PR subject.
	if err := os.WriteFile(filepath.Join(repo, folderRel, "shape.md"),
		append([]byte("\n"), mustRead(t, filepath.Join(repo, folderRel, "shape.md"))...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test",
		"commit", "-m", "fix: tweak shape (#142)")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "show", folderRel, "--json"}); err != nil {
		t.Fatalf("show: %v\n%s", err, stdout.String())
	}
	var result changeShowJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(result.PRs) != 2 || result.PRs[0] != 141 || result.PRs[1] != 142 {
		t.Fatalf("prs = %v, want [141 142]", result.PRs)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return data
}

func TestChangeCheckMalformedJSONNoFallback(t *testing.T) {
	repo := initCLIGitRepo(t)
	folder := writeChangeFolder(t, repo, "20260710-keep-both", executableLineageDoc("keep-both", "line", "", ""))
	if err := os.WriteFile(filepath.Join(folder, "change.json"), []byte(`{broken`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out, err := runChangeCheckJSON(t, repo, folder)
	if err == nil {
		t.Fatalf("want violation for malformed JSON, got %+v", out)
	}
	joined := strings.Join(out.Findings, "\n")
	if !strings.Contains(joined, "malformed change.json") {
		t.Fatalf("findings = %v, want malformed change.json", out.Findings)
	}
}
