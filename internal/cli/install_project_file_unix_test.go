//go:build unix

package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A FIFO at a project path is the case that has to be tested with a deadline
// rather than an assertion about a return value. The failure being pinned is
// not a wrong answer, it is no answer: opening a named pipe for reading waits
// for a writer that is never coming, and `loaf upgrade` would sit there until
// somebody killed it. Every case here therefore runs the command on another
// goroutine and fails on the clock.
//
// Reaching these readers at all takes a second signal. The detector refuses to
// read the FIFO too, so the directory has to look like a Loaf project by some
// other evidence — the project config when the pipe is at AGENTS.md, the
// managed section when the pipe is at the config.

// TestReadRegularFileRefusesAFifoWithoutBlocking is the unit-level guard under
// all of it: the shared reader settles the type on the descriptor it opened,
// and reports the refusal rather than waiting.
func TestReadRegularFileRefusesAFifoWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "AGENTS.md")
	mkfifoForTest(t, fifo)

	type readResult struct {
		body []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		body, err := readRegularFile(fifo, projectFileReadLimit)
		done <- readResult{body: body, err: err}
	}()

	select {
	case result := <-done:
		if !errors.Is(result.err, errNotRegularFile) {
			t.Fatalf("readRegularFile(FIFO) = %q, %v, want errNotRegularFile", result.body, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readRegularFile(FIFO) blocked; the open must be non-blocking and the type settled on the descriptor")
	}

	regular := filepath.Join(dir, "regular.md")
	writeInstallFile(t, regular, "# Project\n")
	body, err := readRegularFile(regular, projectFileReadLimit)
	if err != nil || string(body) != "# Project\n" {
		t.Fatalf("readRegularFile(regular file) = %q, %v, want the file contents unchanged", body, err)
	}
}

// TestUpgradeRefusesAFifoAtTheManagedProjectFile is the apply side. The fenced
// write is the one that would have hung, and its refusal is the same refusal a
// malformed fingerprint gets: nothing written, the project part abandoned, a
// non-zero exit.
func TestUpgradeRefusesAFifoAtTheManagedProjectFile(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	configPath := filepath.Join(root, ".agents", "loaf.json")
	writeInstallFile(t, configPath, preservedLoafConfigBody)
	agentsPath := filepath.Join(root, "AGENTS.md")
	mkfifoForTest(t, agentsPath)

	result := runInstallWithDeadline(t, root, "upgrade", "--yes")

	var exitErr ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("upgrade error = %v, want a non-zero ExitError\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "not a regular file") || !strings.Contains(result.output, "refusing to overwrite") {
		t.Fatalf("upgrade output = %q, want the special-file refusal reported", result.output)
	}
	if !strings.Contains(result.output, "skipping the remaining project surfaces") {
		t.Fatalf("upgrade output = %q, want the abort reported", result.output)
	}
	if !strings.Contains(result.output, "project surfaces incomplete") {
		t.Fatalf("upgrade output = %q, want the failure summary to name the project part", result.output)
	}
	assertStillNotARegularFile(t, agentsPath)
	if got := readFileBytes(t, configPath); !bytes.Equal(got, []byte(preservedLoafConfigBody)) {
		t.Fatalf("loaf.json = %q, want the surfaces after the refusal left alone", got)
	}
}

// TestUpgradeDryRunReportsAFifoAtTheManagedProjectFile is the plan side, and
// the reason the plan reader had to be hardened separately: a preview that
// blocks is worse than one that reports, because the whole point of --dry-run
// is finding out what would happen without committing to it.
func TestUpgradeDryRunReportsAFifoAtTheManagedProjectFile(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), preservedLoafConfigBody)
	agentsPath := filepath.Join(root, "AGENTS.md")
	mkfifoForTest(t, agentsPath)

	result := runInstallWithDeadline(t, root, "upgrade", "--dry-run", "--json")

	if result.err != nil {
		t.Fatalf("upgrade --dry-run --json error = %v\n%s", result.err, result.output)
	}
	plan := parseInstallPlanJSON(t, result.output)
	entry, found := findInstallPlanEntry(plan, "AGENTS.md")
	if !found {
		t.Fatalf("project_files = %#v, want an entry for the path it cannot read", plan.ProjectFiles)
	}
	if entry.Action != "error" {
		t.Fatalf("plan entry = %#v, want the write reported as refused rather than promised", entry)
	}
	if !strings.Contains(entry.Detail, "not a regular file") || !strings.Contains(entry.Detail, "refusing to overwrite") {
		t.Fatalf("plan entry detail = %q, want the special-file refusal", entry.Detail)
	}
	assertStillNotARegularFile(t, agentsPath)
}

// TestUpgradeRefusesAFifoAtTheProjectConfig closes the round-4 residual: an
// ordinary unreadable config was already preserved and reported, but a FIFO
// blocked the read before that refusal could be reached.
func TestUpgradeRefusesAFifoAtTheProjectConfig(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeManagedAgentsFileForTest(t, root)
	configPath := filepath.Join(root, ".agents", "loaf.json")
	mkfifoForTest(t, configPath)

	result := runInstallWithDeadline(t, root, "upgrade", "--yes")

	if result.err != nil {
		t.Fatalf("upgrade error = %v\n%s", result.err, result.output)
	}
	assertPreservedConfigReported(t, result.output)
	assertStillNotARegularFile(t, configPath)
}

// TestUpgradeDryRunReportsAFifoAtTheProjectConfig is that residual's plan side.
func TestUpgradeDryRunReportsAFifoAtTheProjectConfig(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeManagedAgentsFileForTest(t, root)
	configPath := filepath.Join(root, ".agents", "loaf.json")
	mkfifoForTest(t, configPath)

	result := runInstallWithDeadline(t, root, "upgrade", "--dry-run", "--json")

	if result.err != nil {
		t.Fatalf("upgrade --dry-run --json error = %v\n%s", result.err, result.output)
	}
	plan := parseInstallPlanJSON(t, result.output)
	entry, found := findInstallPlanEntry(plan, filepath.Join(".agents", "loaf.json"))
	if !found {
		t.Fatalf("project_files = %#v, want an entry for the config it cannot read", plan.ProjectFiles)
	}
	if entry.Action != "skipped" {
		t.Fatalf("plan entry = %#v, want it skipped rather than promised", entry)
	}
	if !strings.Contains(entry.Detail, "could not be read") {
		t.Fatalf("plan entry detail = %q, want the read refusal", entry.Detail)
	}
	assertStillNotARegularFile(t, configPath)
}

// installRunResult is what a command produced, carried back from the goroutine
// it ran on so the assertions stay on the test's own goroutine.
type installRunResult struct {
	output string
	err    error
}

// runInstallWithDeadline runs a command with a deadline, so a read that waits
// on a FIFO fails this test instead of hanging the package until the go test
// timeout kills every other test with it.
func runInstallWithDeadline(t *testing.T, root string, args ...string) installRunResult {
	t.Helper()
	done := make(chan installRunResult, 1)
	go func() {
		var stdout bytes.Buffer
		err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run(args)
		done <- installRunResult{output: stdout.String(), err: err}
	}()
	select {
	case result := <-done:
		return result
	case <-time.After(30 * time.Second):
		t.Fatalf("%v did not finish within 30s; a project-file read must never wait on a FIFO", args)
		return installRunResult{}
	}
}

func mkfifoForTest(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("Mkfifo(%s) unavailable here: %v", path, err)
	}
}
