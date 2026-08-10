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

// TestUpgradeRefusesAFifoAtTheLegacyAgentsFile covers the migration pass, which
// runs before the fenced writers and therefore had its own unhardened reads. A
// legacy .agents/AGENTS.md that is a pipe stops the pass where the fenced write
// would have stopped: nothing migrated, nothing backed up, and the run reports
// the project part as incomplete.
func TestUpgradeRefusesAFifoAtTheLegacyAgentsFile(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	canonical := filepath.Join(root, "AGENTS.md")
	writeManagedAgentsFileForTest(t, root)
	before := readFileBytes(t, canonical)
	legacy := filepath.Join(root, ".agents", "AGENTS.md")
	mkfifoForTest(t, legacy)

	result := runInstallWithDeadline(t, root, "upgrade", "--yes")

	var exitErr ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("upgrade error = %v, want a non-zero ExitError\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "Failed to migrate .agents/AGENTS.md") {
		t.Fatalf("upgrade output = %q, want the refused step named", result.output)
	}
	if !strings.Contains(result.output, "not a regular file") || !strings.Contains(result.output, "refusing to overwrite") {
		t.Fatalf("upgrade output = %q, want the migration refusal reported", result.output)
	}
	if !strings.Contains(result.output, "project surfaces incomplete") {
		t.Fatalf("upgrade output = %q, want the failure summary to name the project part", result.output)
	}
	assertStillNotARegularFile(t, legacy)
	if got := readFileBytes(t, canonical); !bytes.Equal(got, before) {
		t.Fatalf("AGENTS.md = %q, want it untouched; a refused migration must not half-write", got)
	}
	if _, err := os.Lstat(legacy + ".bak"); err == nil {
		t.Fatal("the refused migration left a backup behind; nothing should have moved")
	}
}

// TestUpgradeDryRunReportsAFifoAtTheLegacyAgentsFile is that migration's plan
// side. The metadata the plan used to decide on — is it a file, is it a
// symlink — says yes to a pipe, so a plan that asked only that promised a
// migration the apply path refuses to perform.
func TestUpgradeDryRunReportsAFifoAtTheLegacyAgentsFile(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeManagedAgentsFileForTest(t, root)
	legacy := filepath.Join(root, ".agents", "AGENTS.md")
	mkfifoForTest(t, legacy)

	result := runInstallWithDeadline(t, root, "upgrade", "--dry-run", "--json", "--yes")

	if result.err != nil {
		t.Fatalf("upgrade --dry-run --json error = %v\n%s", result.err, result.output)
	}
	plan := parseInstallPlanJSON(t, result.output)
	entry, found := findInstallPlanEntry(plan, "./AGENTS.md")
	if !found {
		t.Fatalf("project_files = %#v, want an entry for the root instruction file", plan.ProjectFiles)
	}
	if entry.Action != "error" {
		t.Fatalf("plan entry = %#v, want the migration reported as refused rather than promised", entry)
	}
	if !strings.Contains(entry.Detail, "not a regular file") || !strings.Contains(entry.Detail, "refusing to overwrite") {
		t.Fatalf("plan entry detail = %q, want the special-file refusal", entry.Detail)
	}
	assertStillNotARegularFile(t, legacy)
}

// TestUpgradeRefusesAFifoAtTheClaudeCompatibilityFile is the other half of the
// same pass: the real file that is about to be merged into AGENTS.md and
// replaced by a symlink.
func TestUpgradeRefusesAFifoAtTheClaudeCompatibilityFile(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	writeFixtureClaudeCLI(t, root)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	canonical := filepath.Join(root, "AGENTS.md")
	writeManagedAgentsFileForTest(t, root)
	before := readFileBytes(t, canonical)
	claudeLink := filepath.Join(root, ".claude", "CLAUDE.md")
	mkfifoForTest(t, claudeLink)

	result := runInstallWithDeadline(t, root, "upgrade", "--yes")

	var exitErr ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("upgrade error = %v, want a non-zero ExitError\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "Failed to replace .claude/CLAUDE.md") {
		t.Fatalf("upgrade output = %q, want the refused step named", result.output)
	}
	if !strings.Contains(result.output, "not a regular file") || !strings.Contains(result.output, "refusing to overwrite") {
		t.Fatalf("upgrade output = %q, want the replace refusal reported", result.output)
	}
	if !strings.Contains(result.output, "project surfaces incomplete") {
		t.Fatalf("upgrade output = %q, want the failure summary to name the project part", result.output)
	}
	assertStillNotARegularFile(t, claudeLink)
	if got := readFileBytes(t, canonical); !bytes.Equal(got, before) {
		t.Fatalf("AGENTS.md = %q, want it untouched; the merge never read anything to merge", got)
	}
}

// TestUpgradeDryRunReportsAFifoAtTheClaudeCompatibilityFile is its plan side.
func TestUpgradeDryRunReportsAFifoAtTheClaudeCompatibilityFile(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	writeFixtureClaudeCLI(t, root)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeManagedAgentsFileForTest(t, root)
	claudeLink := filepath.Join(root, ".claude", "CLAUDE.md")
	mkfifoForTest(t, claudeLink)

	result := runInstallWithDeadline(t, root, "upgrade", "--dry-run", "--json", "--yes")

	if result.err != nil {
		t.Fatalf("upgrade --dry-run --json error = %v\n%s", result.err, result.output)
	}
	plan := parseInstallPlanJSON(t, result.output)
	entry, found := findInstallPlanEntry(plan, ".claude/CLAUDE.md")
	if !found {
		t.Fatalf("project_files = %#v, want an entry for the compatibility file", plan.ProjectFiles)
	}
	if entry.Action != "error" {
		t.Fatalf("plan entry = %#v, want the replacement reported as refused", entry)
	}
	if !strings.Contains(entry.Detail, "not a regular file") || !strings.Contains(entry.Detail, "refusing to overwrite") {
		t.Fatalf("plan entry detail = %q, want the special-file refusal", entry.Detail)
	}
	assertStillNotARegularFile(t, claudeLink)
}

// TestUpgradeDryRunReportsAFifoAtTheMcpConfig covers the detection half of the
// harness configs. Detection answers yes or no, so it cannot refuse — but it
// can say which path it never managed to ask about, and it must answer at all
// rather than wait on the pipe.
func TestUpgradeDryRunReportsAFifoAtTheMcpConfig(t *testing.T) {
	root, home := setupUpgradeFixture(t)
	installUpgradeFixtureTarget(t, root, home, "cursor")
	writeManagedAgentsFileForTest(t, root)
	mcpPath := filepath.Join(root, ".cursor", "mcp.json")
	mkfifoForTest(t, mcpPath)

	result := runInstallWithDeadline(t, root, "upgrade", "--dry-run", "--json")

	if result.err != nil {
		t.Fatalf("upgrade --dry-run --json error = %v\n%s", result.err, result.output)
	}
	plan := parseInstallPlanJSON(t, result.output)
	notices := installMcpPlanNotices(plan.Mcp)
	if len(notices) == 0 {
		t.Fatalf("mcp = %#v, want the config it could not inspect reported", plan.Mcp)
	}
	if !strings.Contains(notices[0], "not a regular file") || !strings.Contains(notices[0], mcpPath) {
		t.Fatalf("mcp notice = %q, want the path and the reason", notices[0])
	}
	for _, entry := range plan.Mcp {
		if entry.Target == "cursor" && entry.Configured {
			t.Fatalf("mcp entry = %#v, want a config that could not be read reported as unconfigured", entry)
		}
	}
	assertStillNotARegularFile(t, mcpPath)
}

// TestMergeMcpConfigRefusesAFifo is the apply half. The merge rewrites the
// whole file, so a config it could not read would come back holding only
// Loaf's own entry.
func TestMergeMcpConfigRefusesAFifo(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".cursor", "mcp.json")
	mkfifoForTest(t, mcpPath)

	done := make(chan error, 1)
	go func() {
		done <- mergeJSONMcpConfig(mcpPath, "mcpServers", "linear", []string{"npx", "-y", "mcp-remote"})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, errNotRegularFile) {
			t.Fatalf("mergeJSONMcpConfig(FIFO) error = %v, want errNotRegularFile", err)
		}
		if !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("mergeJSONMcpConfig(FIFO) error = %q, want it phrased as a refusal to overwrite", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mergeJSONMcpConfig blocked on a FIFO; the merge read must be non-blocking")
	}
	assertStillNotARegularFile(t, mcpPath)
}

// TestCodexUserConfigReadersRefuseAFifo covers the Codex readers that reach
// into CODEX_HOME: the guidance file the journal block is merged into, and the
// hooks file the matcher groups are merged into.
func TestCodexUserConfigReadersRefuseAFifo(t *testing.T) {
	dir := t.TempDir()
	guidance := filepath.Join(dir, "AGENTS.md")
	mkfifoForTest(t, guidance)
	hooks := filepath.Join(dir, "hooks.json")
	mkfifoForTest(t, hooks)

	type refusal struct {
		name    string
		run     func() error
		refused func(error) bool
	}
	for _, subject := range []refusal{
		{
			name: "guidance",
			run: func() error {
				_, _, err := readOptionalInstallFile(guidance, "Codex journal guidance")
				return err
			},
			refused: func(err error) bool { return errors.Is(err, errNotRegularFile) },
		},
		{
			name: "hooks",
			run: func() error {
				_, err := readHookFile(hooks)
				return err
			},
			// The reconciler's reader refuses before it reads: a FIFO is not a
			// regular file, so the refusal names the destination rather than
			// wrapping a read error it never reached.
			refused: func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "not a regular file")
			},
		},
	} {
		t.Run(subject.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- subject.run() }()
			select {
			case err := <-done:
				if !subject.refused(err) {
					t.Fatalf("%s reader error = %v, want a non-regular-file refusal", subject.name, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s reader blocked on a FIFO", subject.name)
			}
		})
	}
	assertStillNotARegularFile(t, guidance)
	assertStillNotARegularFile(t, hooks)
}

// writeFixtureClaudeCLI puts a `claude` on the fixture PATH, which is the only
// signal upgrade uses to decide the project wants a .claude/CLAUDE.md.
func writeFixtureClaudeCLI(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "bin", "claude")
	writeInstallFile(t, path, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("Chmod(%s) error = %v", path, err)
	}
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
