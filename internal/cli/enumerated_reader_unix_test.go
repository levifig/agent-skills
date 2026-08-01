//go:build unix

package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Enumerated discovered paths must open through the descriptor-hardened reader
// so a FIFO named like a real entry cannot hang listing commands.

func TestRenderDriftSkipsFifoSpecWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".agents", "specs")
	mkdirAll(t, specsDir)
	// No final render stamp: the real file is skipped for drift, so the only
	// interesting entry is the FIFO. The command must not hang or block on it.
	writeInstallFile(t, filepath.Join(specsDir, "SPEC-001-real.md"), "# Real\n")
	mkfifoForTest(t, filepath.Join(specsDir, "SPEC-fifo.md"))

	done := make(chan checkResult, 1)
	go func() {
		// Empty context is push-scoped (tool and command both blank).
		done <- runNativeRenderDrift(checkHookContext{}, root)
	}()

	select {
	case result := <-done:
		if result.Blocked {
			t.Fatalf("render-drift blocked on a FIFO: %#v", result)
		}
		found := false
		for _, warning := range result.Warnings {
			if strings.Contains(warning, "SPEC-fifo.md") && strings.Contains(warning, "not a regular file") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("warnings = %#v, want a skip notice for the FIFO", result.Warnings)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runNativeRenderDrift blocked on a FIFO; enumerated reads must be non-blocking")
	}
}

func TestEphemeralProvenanceSkipsFifoSpecWithoutBlocking(t *testing.T) {
	root := initEnumeratedGitRepo(t)
	specsDir := filepath.Join(root, ".agents", "specs")
	mkdirAll(t, specsDir)
	writeInstallFile(t, filepath.Join(specsDir, "SPEC-001-real.md"), "# Real\n")
	mkfifoForTest(t, filepath.Join(specsDir, "SPEC-fifo.md"))

	done := make(chan checkResult, 1)
	go func() {
		done <- runNativeEphemeralProvenance(checkHookContext{}, root)
	}()

	select {
	case result := <-done:
		found := false
		for _, warning := range result.Warnings {
			if strings.Contains(warning, "SPEC-fifo.md") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("warnings = %#v, want a skip notice for the FIFO", result.Warnings)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runNativeEphemeralProvenance blocked on a FIFO")
	}
}

func TestChangeTaskListingSkipsFifoWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "docs", "changes", "20260731-test-change")
	tasksDir := filepath.Join(folder, "tasks")
	mkdirAll(t, tasksDir)
	writeInstallFile(t, filepath.Join(tasksDir, "TASK-001-real.md"), "---\nid: TASK-001\n---\n# Real\n")
	mkfifoForTest(t, filepath.Join(tasksDir, "TASK-fifo.md"))

	type listResult struct {
		names    []string
		findings []string
	}
	done := make(chan listResult, 1)
	go func() {
		names, _, findings := listChangeTaskFileContents(root, folder, "docs/changes/20260731-test-change", changeTaskContentWorkingTree, nil)
		done <- listResult{names: names, findings: findings}
	}()

	select {
	case result := <-done:
		if len(result.names) != 1 || result.names[0] != "TASK-001-real.md" {
			t.Fatalf("names = %#v, want only the real task", result.names)
		}
		found := false
		for _, finding := range result.findings {
			if strings.Contains(finding, "TASK-fifo.md") && strings.Contains(finding, "not a regular file") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("findings = %#v, want a skip notice for the FIFO", result.findings)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listChangeTaskFileContents blocked on a FIFO")
	}
}

func TestReleaseIncompleteTasksSkipsFifoWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, ".agents", "tasks")
	mkdirAll(t, tasksDir)
	writeInstallFile(t, filepath.Join(tasksDir, "TASK-001.md"), "status: todo\n# Real\n")
	mkfifoForTest(t, filepath.Join(tasksDir, "TASK-fifo.md"))

	done := make(chan []releaseIncompleteTask, 1)
	go func() {
		done <- scanReleaseIncompleteTasks(root)
	}()

	select {
	case incomplete := <-done:
		if len(incomplete) != 1 {
			t.Fatalf("incomplete = %#v, want the real task only (FIFO skipped)", incomplete)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scanReleaseIncompleteTasks blocked on a FIFO")
	}
}

func TestCopyFilePreservingModeRefusesFifo(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "from")
	to := filepath.Join(dir, "to")
	mkfifoForTest(t, from)

	done := make(chan error, 1)
	go func() {
		done <- copyFilePreservingMode(from, to)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, errNotRegularFile) {
			t.Fatalf("copyFilePreservingMode(FIFO) error = %v, want errNotRegularFile", err)
		}
		if _, err := os.Stat(to); !os.IsNotExist(err) {
			t.Fatalf("copy created destination despite refusal: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copyFilePreservingMode blocked on a FIFO")
	}
}

func TestFilesHaveSameContentRefusesFifo(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	writeInstallFile(t, a, "same\n")
	mkfifoForTest(t, b)

	done := make(chan bool, 1)
	go func() {
		done <- filesHaveSameContent(a, b)
	}()

	select {
	case same := <-done:
		if same {
			t.Fatal("filesHaveSameContent reported true against a FIFO")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("filesHaveSameContent blocked on a FIFO")
	}
}

func TestReadValidatedChangeRefusesFifo(t *testing.T) {
	root := initEnumeratedGitRepo(t)
	folder := filepath.Join(root, "docs", "changes", "20260731-fifo-change")
	mkdirAll(t, folder)
	changePath := filepath.Join(folder, "change.md")
	mkfifoForTest(t, changePath)

	done := make(chan error, 1)
	go func() {
		_, err := readValidatedChange(root, changePath, changePath, "docs/changes/20260731-fifo-change/change.md", changeOriginOps{})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("readValidatedChange accepted a FIFO")
		}
		if !errors.Is(err, errNotRegularFile) && !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v, want errNotRegularFile", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readValidatedChange blocked on a FIFO")
	}
}

func TestReadValidatedChangeBoundsRead(t *testing.T) {
	root := initEnumeratedGitRepo(t)
	folder := filepath.Join(root, "docs", "changes", "20260731-bound-change")
	mkdirAll(t, folder)
	changePath := filepath.Join(folder, "change.md")
	body := "---\nslug: bound-change\n---\n# Change\n\nSmall enough.\n"
	writeInstallFile(t, changePath, body)
	runEnumeratedGit(t, root, "add", ".")
	runEnumeratedGit(t, root, "-c", "commit.gpgsign=false", "commit", "-m", "add change")

	// Use the slug selector so revalidation walks the same path production does.
	content, err := readValidatedChange(root, "bound-change", changePath, "docs/changes/20260731-bound-change/change.md", changeOriginOps{})
	if err != nil {
		t.Fatalf("readValidatedChange(valid) error = %v", err)
	}
	if !bytes.Equal(content, []byte(body)) {
		t.Fatalf("content = %q, want %q", content, body)
	}
}

func initEnumeratedGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runEnumeratedGit(t, root, "init")
	runEnumeratedGit(t, root, "config", "user.email", "test@example.com")
	runEnumeratedGit(t, root, "config", "user.name", "test")
	// Empty initial commit so HEAD exists for any git-backed helpers.
	runEnumeratedGit(t, root, "commit", "--allow-empty", "-m", "init")
	// Match production path shape: git and EvalSymlinks both resolve through
	// /private on macOS temp dirs, so callers must use the real path.
	evaluated, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s) error = %v", root, err)
	}
	return evaluated
}

func runEnumeratedGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
