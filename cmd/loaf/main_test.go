package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/state"
)

func TestNewRunnerWiresBuildInfo(t *testing.T) {
	originalCommit, originalDate := buildCommit, buildDate
	t.Cleanup(func() {
		buildCommit, buildDate = originalCommit, originalDate
	})
	buildCommit = "abc1234"
	buildDate = "2026-06-27T12:00:00Z"

	runner := newRunner(io.Discard, io.Discard)
	if runner.BuildCommit != "abc1234" {
		t.Fatalf("runner.BuildCommit = %q, want %q", runner.BuildCommit, "abc1234")
	}
	if runner.BuildDate != "2026-06-27T12:00:00Z" {
		t.Fatalf("runner.BuildDate = %q, want %q", runner.BuildDate, "2026-06-27T12:00:00Z")
	}
}

func TestPublicBinaryVersionShowsInjectedBuildInfoNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "loaf")
	ldflags := "-X main.buildCommit=abc1234 -X main.buildDate=2026-06-27T12:00:00Z"
	if output, err := runCommand(repoRoot, "go", "build", "-ldflags", ldflags, "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build with ldflags error = %v\n%s", err, output)
	}

	env := envWith("LOAF_DB=" + filepath.Join(t.TempDir(), "loaf.sqlite"))
	output, err := runBinary(binary, repoRoot, env, "--version")
	if err != nil {
		t.Fatalf("loaf --version error = %v\n%s", err, output)
	}
	for _, want := range []string{"built 2026-06-27T12:00:00Z", "git abc1234"} {
		if !strings.Contains(output, want) {
			t.Fatalf("--version output = %q, want to contain %q", output, want)
		}
	}

	// A plain build (no ldflags) must keep the clean version line.
	cleanBinary := filepath.Join(t.TempDir(), "loaf-clean")
	if output, err := runCommand(repoRoot, "go", "build", "-o", cleanBinary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build (clean) error = %v\n%s", err, output)
	}
	cleanOutput, err := runBinary(cleanBinary, repoRoot, env, "--version")
	if err != nil {
		t.Fatalf("clean loaf --version error = %v\n%s", err, cleanOutput)
	}
	if strings.Contains(cleanOutput, "(built") || strings.Contains(cleanOutput, "git abc1234") {
		t.Fatalf("clean --version output = %q, want no injected build info", cleanOutput)
	}
}

func TestPublicBinaryDispatchesStateVersionAndReleasePreflightNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	// version reads Targets/Content from the binary's own installed
	// distribution, so the compiled binary runs from a distribution-shaped
	// fixture instead of a bare temp directory.
	binary := filepath.Join(writeInstalledDistributionFixture(t, repoRoot, "9.9.9-native-dispatch"), "bin", "loaf")

	workingDir := realpath(t, t.TempDir())
	dataHome := t.TempDir()
	legacyStateHome := t.TempDir()
	output, err := runBinary(binary, workingDir, envWith(
		"XDG_DATA_HOME="+dataHome,
		"XDG_STATE_HOME="+legacyStateHome,
	), "state", "path")
	if err != nil {
		t.Fatalf("loaf state path error = %v\n%s", err, output)
	}
	statePath := strings.TrimSpace(output)
	if statePath != filepath.Join(dataHome, "loaf", "loaf.sqlite") {
		t.Fatalf("state path = %q, want global database under data home %q", statePath, dataHome)
	}
	if strings.HasPrefix(statePath, workingDir+string(filepath.Separator)) {
		t.Fatalf("state path = %q, want outside working dir %q", statePath, workingDir)
	}

	output, err = runBinary(binary, repoRoot, envWith(), "version")
	if err != nil {
		t.Fatalf("loaf version error = %v\n%s", err, output)
	}
	for _, want := range []string{"loaf", "Content:", "Skills:", "Agents:", "Hooks:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output = %q, want %q", output, want)
		}
	}

	output, err = runBinary(binary, workingDir, envWith(), "release", "--post-merge")
	if err == nil {
		t.Fatalf("loaf release --post-merge error = nil, want native lineage-preflight failure\n%s", output)
	}
	for _, want := range []string{"release blocked: cannot inspect committed Change graph at HEAD", "inspect committed Change paths at HEAD"} {
		if !strings.Contains(output, want) {
			t.Fatalf("release output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "Verifying post-merge state") {
		t.Fatalf("release output = %q, want lineage preflight before post-merge actions", output)
	}
	if strings.Contains(output, "TypeScript fallback") {
		t.Fatalf("release output = %q, want native post-merge path without fallback lookup", output)
	}
}

func TestPublicBinaryDispatchesVersionFlagNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	// Same distribution-shaped fixture as above: --version's Content sections
	// come from the binary's own distribution, never the working directory.
	binary := filepath.Join(writeInstalledDistributionFixture(t, repoRoot, "9.9.9-native-dispatch"), "bin", "loaf")

	output, err := runBinary(binary, repoRoot, envWith(), "--version")
	if err != nil {
		t.Fatalf("loaf --version error = %v\n%s", err, output)
	}
	for _, want := range []string{"loaf", "Content:", "Skills:", "Agents:", "Hooks:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("--version output = %q, want %q", output, want)
		}
	}
}

func TestPublicBinaryMigrateWorktreeStorageHelpAndDebugNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	main := createMainRepo(t, "help-main")

	output, err := runBinary(binary, main, envWith(), "migrate", "worktree-storage", "--help")
	if err != nil {
		t.Fatalf("loaf migrate worktree-storage --help error = %v\n%s", err, output)
	}
	for _, want := range []string{"--apply", "dry-run", "LOAF_DEBUG_RESOLVE"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output = %q, want %q", output, want)
		}
	}

	nonGit := realpath(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(nonGit, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonGit, ".agents", "AGENTS.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents/AGENTS.md) error = %v", err)
	}
	output, err = runBinary(binary, nonGit, envWith("LOAF_DEBUG_RESOLVE=1"), "migrate", "worktree-storage")
	if err == nil {
		t.Fatalf("loaf migrate worktree-storage in non-git dir error = nil, want git-context rejection\n%s", output)
	}
	for _, want := range []string{"LOAF_DEBUG_RESOLVE", "findMainWorktreeRoot fell back to parent-walk"} {
		if !strings.Contains(output, want) {
			t.Fatalf("debug output = %q, want %q", output, want)
		}
	}
}

func TestPublicBinaryPreA3WorktreeRefusalNudgeNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)

	main := createMainRepo(t, "nudge-identical")
	linked := addLinkedWorktree(t, main, "nudge-identical")
	seedIdenticalAgentsCheckout(t, main, linked)
	output, err := runBinary(binary, linked, envWith(), "doctor")
	if exitCode(err) == 2 {
		t.Fatalf("loaf doctor in identical worktree hit pre-A3 refusal\n%s", output)
	}
	if strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("identical worktree output = %q, want no pre-A3 refusal", output)
	}
	raw, err := os.ReadFile(filepath.Join(linked, ".agents", ".moved-to"))
	if err != nil {
		t.Fatalf("ReadFile(.moved-to) error = %v", err)
	}
	if string(raw) != main+"\n" {
		t.Fatalf(".moved-to = %q, want %q", raw, main+"\n")
	}

	main = createMainRepo(t, "nudge-divergent")
	linked = addLinkedWorktree(t, main, "nudge-divergent")
	seedIdenticalAgentsCheckout(t, main, linked)
	if err := os.WriteFile(filepath.Join(linked, ".agents", "AGENTS.md"), []byte("# Divergent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(divergent AGENTS.md) error = %v", err)
	}
	output, err = runBinary(binary, linked, envWith(), "journal", "recent")
	if exitCode(err) != 2 {
		t.Fatalf("loaf journal recent divergent exit = %d, want 2\n%s", exitCode(err), output)
	}
	for _, want := range []string{"SPEC-036", "loaf migrate worktree-storage"} {
		if !strings.Contains(output, want) {
			t.Fatalf("divergent refusal output = %q, want %q", output, want)
		}
	}

	main = createMainRepo(t, "nudge-refuse")
	linked = addLinkedWorktree(t, main, "nudge-refuse")
	seedPreA3WorktreeLayout(t, linked)
	output, err = runBinary(binary, linked, envWith(), "journal", "recent")
	if exitCode(err) != 2 {
		t.Fatalf("loaf journal recent exit = %d, want 2\n%s", exitCode(err), output)
	}
	for _, want := range []string{"SPEC-036", "loaf migrate worktree-storage", "LOAF_DEBUG_RESOLVE"} {
		if !strings.Contains(output, want) {
			t.Fatalf("refusal output = %q, want %q", output, want)
		}
	}

	main = createMainRepo(t, "nudge-unknown")
	linked = addLinkedWorktree(t, main, "nudge-unknown")
	seedPreA3WorktreeLayout(t, linked)
	output, err = runBinary(binary, linked, envWith(), "not-a-command")
	if exitCode(err) != 2 {
		t.Fatalf("loaf not-a-command exit = %d, want 2\n%s", exitCode(err), output)
	}
	for _, want := range []string{"unknown command 'not-a-command'", "SPEC-036", "loaf migrate worktree-storage"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unknown-command refusal output = %q, want %q", output, want)
		}
	}

	main = createMainRepo(t, "nudge-allow")
	linked = addLinkedWorktree(t, main, "nudge-allow")
	seedPreA3WorktreeLayout(t, linked)
	output, err = runBinary(binary, linked, envWith(), "migrate", "worktree-storage")
	if err != nil {
		t.Fatalf("loaf migrate worktree-storage error = %v\n%s", err, output)
	}
	if !strings.Contains(output, "Dry run") || strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("migrate allowlist output = %q, want dry-run without refusal", output)
	}

	for _, allowed := range [][]string{{"--help"}, {"--version"}} {
		main = createMainRepo(t, "nudge-allow-"+strings.TrimPrefix(allowed[0], "--"))
		linked = addLinkedWorktree(t, main, "nudge-allow-"+strings.TrimPrefix(allowed[0], "--"))
		seedPreA3WorktreeLayout(t, linked)
		output, err = runBinary(binary, linked, envWith(), allowed...)
		if err != nil {
			t.Fatalf("loaf %v error = %v\n%s", allowed, err, output)
		}
		if strings.Contains(output, "SPEC-036 centralizes") {
			t.Fatalf("allowlisted %v output = %q, want no refusal", allowed, output)
		}
	}

	main = createMainRepo(t, "nudge-main")
	output, err = runBinary(binary, main, envWith(), "version")
	if exitCode(err) == 2 || strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("main checkout output = %q, error = %v, want no pre-A3 refusal", output, err)
	}

	main = createMainRepo(t, "nudge-migrated")
	linked = addLinkedWorktree(t, main, "nudge-migrated")
	if err := os.MkdirAll(filepath.Join(linked, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(linked .agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(linked, ".agents", ".moved-to"), []byte(main+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.moved-to) error = %v", err)
	}
	output, err = runBinary(binary, linked, envWith(), "version")
	if exitCode(err) == 2 || strings.Contains(output, "SPEC-036 centralizes") {
		t.Fatalf("migrated linked worktree output = %q, error = %v, want no pre-A3 refusal", output, err)
	}

	main = createMainRepo(t, "nudge-stale-pointer")
	linked = addLinkedWorktree(t, main, "nudge-stale-pointer")
	seedPreA3WorktreeLayout(t, linked)
	if err := os.WriteFile(filepath.Join(linked, ".agents", ".moved-to"), []byte("/this/does/not/exist\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale .moved-to) error = %v", err)
	}
	output, err = runBinary(binary, linked, envWith(), "journal", "recent")
	if exitCode(err) != 2 || !strings.Contains(output, "SPEC-036") {
		t.Fatalf("stale pointer output = %q, error = %v, want pre-A3 refusal", output, err)
	}
}

func TestPublicBinaryMigrateWorktreeStorageFlagValidationNatively(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	main := createMainRepo(t, "flag-mutex")
	linked := addLinkedWorktree(t, main, "flag-mutex")
	seedPreA3WorktreeLayout(t, linked)

	output, err := runBinary(binary, linked, envWith(), "migrate", "worktree-storage", "--force-from-worktree", "--force-from-main")
	if err == nil {
		t.Fatalf("loaf migrate worktree-storage flag conflict error = nil, want rejection\n%s", output)
	}
	for _, want := range []string{"--force-from-worktree", "--force-from-main"} {
		if !strings.Contains(output, want) {
			t.Fatalf("flag conflict output = %q, want %q", output, want)
		}
	}
}

func TestPublicBinaryRootHelpAndUnknownCommandAreNative(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(repoRoot, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}

	output, err := runBinary(binary, repoRoot, envWith())
	if err != nil {
		t.Fatalf("loaf root help error = %v\n%s", err, output)
	}
	for _, want := range []string{"Usage: loaf <command> [options]", "Commands:", "journal", "task", "release"} {
		if !strings.Contains(output, want) {
			t.Fatalf("root help output = %q, want %q", output, want)
		}
	}

	output, err = runBinary(binary, repoRoot, envWith(), "not-a-command")
	if err == nil {
		t.Fatalf("loaf not-a-command error = nil, want exit error\n%s", output)
	}
	for _, want := range []string{"error: unknown command 'not-a-command'", "Usage: loaf <command> [options]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unknown-command output = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "TypeScript fallback") {
		t.Fatalf("unknown-command output = %q, want native error without fallback lookup", output)
	}

	output, err = runBinary(binary, repoRoot, envWith(), "--agent-help")
	if err != nil {
		t.Fatalf("loaf --agent-help error = %v\n%s", err, output)
	}
	var doc struct {
		Name     string `json:"name"`
		Commands []struct {
			Name string `json:"name"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(output), &doc); err != nil {
		t.Fatalf("agent help JSON parse error = %v\n%s", err, output)
	}
	if doc.Name != "loaf" || len(doc.Commands) < 15 {
		t.Fatalf("agent help = %#v, want full native command catalog", doc)
	}
}

func TestPublicBinaryConcurrentStateInitConverges(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	for iteration := 0; iteration < 10; iteration++ {
		workingDir := realpath(t, t.TempDir())
		databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
		env := envWith("LOAF_DB=" + databasePath)

		type processResult struct {
			output string
			err    error
		}
		start := make(chan struct{})
		started := make(chan struct{}, 2)
		results := make(chan processResult, 2)
		var wg sync.WaitGroup
		for process := 0; process < 2; process++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cmd := exec.Command(binary, "state", "init", "--json")
				cmd.Dir = workingDir
				cmd.Env = env
				var output bytes.Buffer
				cmd.Stdout = &output
				cmd.Stderr = &output
				<-start
				if err := cmd.Start(); err != nil {
					started <- struct{}{}
					results <- processResult{output: output.String(), err: err}
					return
				}
				started <- struct{}{}
				err := cmd.Wait()
				results <- processResult{output: output.String(), err: err}
			}()
		}
		close(start)
		for process := 0; process < 2; process++ {
			<-started
		}
		wg.Wait()
		close(results)
		projectIDs := make([]string, 0, 2)
		for result := range results {
			if result.err != nil {
				t.Fatalf("iteration %d concurrent state init error = %v\n%s", iteration, result.err, result.output)
			}
			var status state.Status
			if err := json.Unmarshal([]byte(result.output), &status); err != nil {
				t.Fatalf("iteration %d decode state init output = %v\n%s", iteration, err, result.output)
			}
			if status.ProjectID == "" {
				t.Fatalf("iteration %d state init output = %#v, want nonempty project ID", iteration, status)
			}
			projectIDs = append(projectIDs, status.ProjectID)
		}
		if len(projectIDs) != 2 || projectIDs[0] != projectIDs[1] {
			t.Fatalf("iteration %d concurrent project IDs = %#v, want one shared ID", iteration, projectIDs)
		}

		// Read the resulting database through a mode=ro URI so this assertion
		// cannot repair or otherwise mutate the fixture.
		values := url.Values{}
		values.Set("mode", "ro")
		readOnlyDSN := (&url.URL{Scheme: "file", Path: filepath.ToSlash(databasePath), RawQuery: values.Encode()}).String()
		db, err := sql.Open("sqlite3", readOnlyDSN)
		if err != nil {
			t.Fatalf("iteration %d sql.Open(read-only) error = %v", iteration, err)
		}
		func() {
			defer db.Close()
			for query, want := range map[string]int{
				`SELECT COUNT(*) FROM projects`:                           1,
				`SELECT COUNT(*) FROM project_paths`:                      1,
				`SELECT COUNT(*) FROM project_paths WHERE is_current = 1`: 1,
				`SELECT COUNT(*) FROM project_paths AS paths LEFT JOIN projects ON projects.id = paths.project_id WHERE projects.id IS NULL`: 0,
			} {
				var got int
				if err := db.QueryRow(query).Scan(&got); err != nil {
					t.Fatalf("iteration %d %s error = %v", iteration, query, err)
				}
				if got != want {
					t.Fatalf("iteration %d %s = %d, want %d", iteration, query, got, want)
				}
			}
			var currentPath string
			if err := db.QueryRow(`SELECT path FROM project_paths WHERE is_current = 1`).Scan(&currentPath); err != nil {
				t.Fatalf("iteration %d read current path error = %v", iteration, err)
			}
			if currentPath != workingDir {
				t.Fatalf("iteration %d current path = %q, want %q", iteration, currentPath, workingDir)
			}
		}()
	}
}

func TestPublicBinaryConcurrentJournalLogsRetainCanonicalAndDerivedRows(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	for iteration := 0; iteration < 3; iteration++ {
		workingDir := createMainRepo(t, fmt.Sprintf("journal-contention-%d", iteration))
		databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
		env := envWith("LOAF_DB=" + databasePath)
		if output, err := runBinary(binary, workingDir, env, "state", "init", "--json"); err != nil {
			t.Fatalf("iteration %d state init error = %v\n%s", iteration, err, output)
		}

		runPublicJournalBusyProbe(t, iteration, binary, workingDir, databasePath, env)

		const writers = 48
		barrierDir := t.TempDir()
		releasePath := filepath.Join(barrierDir, "release")
		readyPaths := make([]string, writers)
		publicPIDPaths := make([]string, writers)
		publicReapedPaths := make([]string, writers)
		cancelPaths := make([]string, writers)
		processes := make([]*journalBarrierProcess, 0, writers)
		for writer := 0; writer < writers; writer++ {
			readyPaths[writer] = filepath.Join(barrierDir, fmt.Sprintf("ready-%02d", writer))
			publicPIDPaths[writer] = filepath.Join(barrierDir, fmt.Sprintf("public-pid-%02d", writer))
			publicReapedPaths[writer] = filepath.Join(barrierDir, fmt.Sprintf("public-reaped-%02d", writer))
			cancelPaths[writer] = filepath.Join(barrierDir, fmt.Sprintf("cancel-%02d", writer))
			processes = append(processes, startJournalBarrierProcess(t, journalBarrierProcessOptions{
				writer:           writer,
				binary:           binary,
				workingDir:       workingDir,
				env:              env,
				message:          fmt.Sprintf("decision(contention): process-%02d", writer),
				readyPath:        readyPaths[writer],
				releasePath:      releasePath,
				publicPIDPath:    publicPIDPaths[writer],
				publicReapedPath: publicReapedPaths[writer],
				cancelPath:       cancelPaths[writer],
			}))
		}
		t.Cleanup(func() {
			if err := stopJournalBarrierProcesses(processes); err != nil {
				t.Errorf("iteration %d stop journal barrier processes: %v", iteration, err)
			}
		})
		waitForJournalBarrierFiles(t, iteration, "ready", readyPaths, processes)
		if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
			t.Fatalf("iteration %d release 48-writer barrier: %v", iteration, err)
		}
		waitForJournalBarrierFiles(t, iteration, "public-pid", publicPIDPaths, processes)
		for _, process := range processes {
			result := waitForJournalBarrierProcess(t, iteration, process, 15*time.Second)
			if result.err != nil {
				t.Fatalf("iteration %d writer %02d error = %v\n%s", iteration, result.writer, result.err, result.output)
			}
		}

		db := openReadOnlySQLite(t, databasePath)
		defer db.Close()
		assertSQLiteJournalDurability(t, db, writers)
		store, err := state.OpenStoreReadOnly(databasePath)
		if err != nil {
			t.Fatalf("iteration %d open read-only state: %v", iteration, err)
		}
		parity, err := state.InspectJournalSearchParity(t.Context(), store)
		if closeErr := store.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			t.Fatalf("iteration %d inspect journal parity: %v", iteration, err)
		}
		if want := (state.JournalSearchParity{CanonicalRows: writers, IndexRows: writers, Ready: true}); parity != want {
			t.Fatalf("iteration %d journal parity = %#v, want %#v", iteration, parity, want)
		}
	}
}

func TestPublicBinaryJournalBarrierCleanupReapsBlockedChild(t *testing.T) {
	repoRoot := repoRoot(t)
	binary := buildLoafBinary(t, repoRoot)
	workingDir := createMainRepo(t, "journal-cleanup")
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	env := envWith("LOAF_DB=" + databasePath)
	if output, err := runBinary(binary, workingDir, env, "state", "init", "--json"); err != nil {
		t.Fatalf("state init error = %v\n%s", err, output)
	}

	barrierDir := t.TempDir()
	readyPath := filepath.Join(barrierDir, "ready")
	releasePath := filepath.Join(barrierDir, "release")
	publicPIDPath := filepath.Join(barrierDir, "public-pid")
	publicReapedPath := filepath.Join(barrierDir, "public-reaped")
	cancelPath := filepath.Join(barrierDir, "cancel")
	process := startJournalBarrierProcess(t, journalBarrierProcessOptions{
		writer:           -2,
		binary:           binary,
		workingDir:       workingDir,
		env:              env,
		message:          "decision(contention-cleanup): cleanup-held-lock",
		readyPath:        readyPath,
		releasePath:      releasePath,
		publicPIDPath:    publicPIDPath,
		publicReapedPath: publicReapedPath,
		cancelPath:       cancelPath,
	})
	t.Cleanup(func() {
		if err := stopJournalBarrierProcesses([]*journalBarrierProcess{process}); err != nil {
			t.Errorf("stop cleanup regression process: %v", err)
		}
	})
	waitForJournalBarrierFiles(t, 0, "cleanup-ready", []string{readyPath}, []*journalBarrierProcess{process})

	lockDB := openWritableSQLite(t, databasePath)
	lockConnection, err := lockDB.Conn(t.Context())
	if err != nil {
		lockDB.Close()
		t.Fatalf("acquire cleanup lock connection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lockConnection.ExecContext(context.Background(), `ROLLBACK`)
		_ = lockConnection.Close()
		_ = lockDB.Close()
	})
	if _, err := lockConnection.ExecContext(t.Context(), `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("begin immediate cleanup lock: %v", err)
	}
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release cleanup barrier: %v", err)
	}
	waitForJournalBarrierFiles(t, 0, "cleanup-public-pid", []string{publicPIDPath}, []*journalBarrierProcess{process})
	time.Sleep(500 * time.Millisecond)
	select {
	case <-process.done:
		t.Fatalf("cleanup public writer exited before cancellation: %v\n%s", process.result.err, process.result.output)
	default:
	}
	if err := stopJournalBarrierProcesses([]*journalBarrierProcess{process}); err != nil {
		t.Fatalf("cancel blocked public writer: %v", err)
	}
	if process.result.err != nil {
		t.Fatalf("cleanup helper error = %v\n%s", process.result.err, process.result.output)
	}
	if _, err := os.Stat(publicReapedPath); err != nil {
		t.Fatalf("reaped marker missing while write lock remains held: %v", err)
	}
	db := openReadOnlySQLite(t, databasePath)
	var probeRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE message = 'cleanup-held-lock'`).Scan(&probeRows); err != nil {
		db.Close()
		t.Fatalf("count cleanup probe rows: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close cleanup read-only database: %v", err)
	}
	if probeRows != 0 {
		t.Fatalf("cleanup probe rows = %d, want zero", probeRows)
	}
	if _, err := lockConnection.ExecContext(t.Context(), `ROLLBACK`); err != nil {
		t.Fatalf("rollback cleanup lock: %v", err)
	}
}

type journalBarrierProcessResult struct {
	writer int
	output string
	err    error
}

type journalBarrierProcess struct {
	writer           int
	command          *exec.Cmd
	output           *bytes.Buffer
	publicPIDPath    string
	publicReapedPath string
	cancelPath       string
	done             chan struct{}
	result           journalBarrierProcessResult
}

type journalBarrierProcessOptions struct {
	writer           int
	binary           string
	workingDir       string
	env              []string
	message          string
	readyPath        string
	releasePath      string
	publicPIDPath    string
	publicReapedPath string
	cancelPath       string
}

func runPublicJournalBusyProbe(t *testing.T, iteration int, binary, workingDir, databasePath string, env []string) {
	t.Helper()
	barrierDir := t.TempDir()
	readyPath := filepath.Join(barrierDir, "ready")
	releasePath := filepath.Join(barrierDir, "release")
	publicPIDPath := filepath.Join(barrierDir, "public-pid")
	publicReapedPath := filepath.Join(barrierDir, "public-reaped")
	cancelPath := filepath.Join(barrierDir, "cancel")
	process := startJournalBarrierProcess(t, journalBarrierProcessOptions{
		writer:           -1,
		binary:           binary,
		workingDir:       workingDir,
		env:              env,
		message:          "decision(contention-probe): probe-held-lock",
		readyPath:        readyPath,
		releasePath:      releasePath,
		publicPIDPath:    publicPIDPath,
		publicReapedPath: publicReapedPath,
		cancelPath:       cancelPath,
	})
	t.Cleanup(func() {
		if err := stopJournalBarrierProcesses([]*journalBarrierProcess{process}); err != nil {
			t.Errorf("iteration %d stop busy-probe process: %v", iteration, err)
		}
	})
	waitForJournalBarrierFiles(t, iteration, "probe-ready", []string{readyPath}, []*journalBarrierProcess{process})

	lockDB := openWritableSQLite(t, databasePath)
	lockConnection, err := lockDB.Conn(t.Context())
	if err != nil {
		lockDB.Close()
		t.Fatalf("iteration %d acquire probe lock connection: %v", iteration, err)
	}
	if _, err := lockConnection.ExecContext(t.Context(), `BEGIN IMMEDIATE`); err != nil {
		lockConnection.Close()
		lockDB.Close()
		t.Fatalf("iteration %d begin immediate probe lock: %v", iteration, err)
	}
	startedAt := time.Now()
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		_, _ = lockConnection.ExecContext(t.Context(), `ROLLBACK`)
		lockConnection.Close()
		lockDB.Close()
		t.Fatalf("iteration %d release busy probe barrier: %v", iteration, err)
	}
	waitForJournalBarrierFiles(t, iteration, "probe-public-pid", []string{publicPIDPath}, []*journalBarrierProcess{process})
	result := waitForJournalBarrierProcess(t, iteration, process, 15*time.Second)
	elapsed := time.Since(startedAt)
	if _, err := lockConnection.ExecContext(t.Context(), `ROLLBACK`); err != nil {
		lockConnection.Close()
		lockDB.Close()
		t.Fatalf("iteration %d rollback probe lock: %v", iteration, err)
	}
	if err := lockConnection.Close(); err != nil {
		lockDB.Close()
		t.Fatalf("iteration %d close probe lock connection: %v", iteration, err)
	}
	if err := lockDB.Close(); err != nil {
		t.Fatalf("iteration %d close probe lock database: %v", iteration, err)
	}
	if result.err == nil {
		t.Fatalf("iteration %d held-lock probe succeeded after %s, want SQLITE_BUSY failure\n%s", iteration, elapsed, result.output)
	}
	if elapsed < 4*time.Second || elapsed > 15*time.Second {
		t.Fatalf("iteration %d held-lock probe elapsed %s, want busy-timeout-scale failure", iteration, elapsed)
	}
	if !strings.Contains(strings.ToLower(result.output), "database is locked") && !strings.Contains(strings.ToLower(result.output), "sqlite_busy") {
		t.Fatalf("iteration %d held-lock probe output = %q, want SQLite busy/locked error", iteration, result.output)
	}
	db := openReadOnlySQLite(t, databasePath)
	defer db.Close()
	var probeRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE message = 'probe-held-lock'`).Scan(&probeRows); err != nil {
		t.Fatalf("iteration %d count held-lock probe rows: %v", iteration, err)
	}
	if probeRows != 0 {
		t.Fatalf("iteration %d held-lock probe rows = %d, want zero", iteration, probeRows)
	}
}

func startJournalBarrierProcess(t *testing.T, options journalBarrierProcessOptions) *journalBarrierProcess {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestPublicBinaryConcurrentJournalLogBarrierChild$", "-test.v")
	command.Dir = options.workingDir
	command.Env = append(options.env,
		"LOAF_JOURNAL_BARRIER_CHILD=1",
		"LOAF_JOURNAL_BARRIER_BINARY="+options.binary,
		"LOAF_JOURNAL_BARRIER_WORKING_DIR="+options.workingDir,
		fmt.Sprintf("LOAF_JOURNAL_BARRIER_WRITER=%d", options.writer),
		"LOAF_JOURNAL_BARRIER_MESSAGE="+options.message,
		"LOAF_JOURNAL_BARRIER_READY="+options.readyPath,
		"LOAF_JOURNAL_BARRIER_RELEASE="+options.releasePath,
		"LOAF_JOURNAL_BARRIER_PUBLIC_PID="+options.publicPIDPath,
		"LOAF_JOURNAL_BARRIER_PUBLIC_REAPED="+options.publicReapedPath,
		"LOAF_JOURNAL_BARRIER_CANCEL="+options.cancelPath,
	)
	process := &journalBarrierProcess{
		writer:           options.writer,
		command:          command,
		output:           &bytes.Buffer{},
		publicPIDPath:    options.publicPIDPath,
		publicReapedPath: options.publicReapedPath,
		cancelPath:       options.cancelPath,
		done:             make(chan struct{}),
	}
	command.Stdout = process.output
	command.Stderr = process.output
	if err := command.Start(); err != nil {
		t.Fatalf("start barrier child %02d: %v", options.writer, err)
	}
	go func() {
		err := command.Wait()
		process.result = journalBarrierProcessResult{writer: options.writer, output: process.output.String(), err: err}
		close(process.done)
	}()
	return process
}

func TestPublicBinaryConcurrentJournalLogBarrierChild(t *testing.T) {
	if os.Getenv("LOAF_JOURNAL_BARRIER_CHILD") != "1" {
		return
	}
	readyPath := os.Getenv("LOAF_JOURNAL_BARRIER_READY")
	releasePath := os.Getenv("LOAF_JOURNAL_BARRIER_RELEASE")
	cancelPath := os.Getenv("LOAF_JOURNAL_BARRIER_CANCEL")
	if err := publishJournalBarrierFile(readyPath, []byte("ready\n")); err != nil {
		t.Fatalf("signal barrier readiness: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(cancelPath); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect barrier cancellation: %v", err)
		}
		if _, err := os.Stat(releasePath); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect barrier release: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("wait for barrier release timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
	workingDir := os.Getenv("LOAF_JOURNAL_BARRIER_WORKING_DIR")
	cmd := exec.Command(os.Getenv("LOAF_JOURNAL_BARRIER_BINARY"), "journal", "log", "--json", "--branch", "main", "--worktree", workingDir,
		os.Getenv("LOAF_JOURNAL_BARRIER_MESSAGE"))
	cmd.Dir = workingDir
	cmd.Env = os.Environ()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start public loaf writer: %v", err)
	}
	publicPIDPath := os.Getenv("LOAF_JOURNAL_BARRIER_PUBLIC_PID")
	temporaryPIDPath := publicPIDPath + ".tmp"
	if err := os.WriteFile(temporaryPIDPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o600); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("record public loaf PID: %v", err)
	}
	if err := os.Rename(temporaryPIDPath, publicPIDPath); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("publish public loaf PID: %v", err)
	}
	var canceled atomic.Bool
	stopCancelMonitor := make(chan struct{})
	cancelMonitorDone := make(chan struct{})
	go func() {
		defer close(cancelMonitorDone)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCancelMonitor:
				return
			case <-ticker.C:
				if _, err := os.Stat(cancelPath); err == nil {
					canceled.Store(true)
					_ = cmd.Process.Kill()
					return
				} else if !os.IsNotExist(err) {
					canceled.Store(true)
					_ = cmd.Process.Kill()
					return
				}
			}
		}
	}()
	waitErr := cmd.Wait()
	close(stopCancelMonitor)
	<-cancelMonitorDone
	publicReapedPath := os.Getenv("LOAF_JOURNAL_BARRIER_PUBLIC_REAPED")
	if err := publishJournalBarrierFile(publicReapedPath, []byte("reaped\n")); err != nil {
		t.Fatalf("record reaped public loaf process: %v", err)
	}
	if canceled.Load() {
		return
	}
	if waitErr != nil {
		t.Fatalf("public loaf writer error = %v\n%s", waitErr, output.String())
	}
}

func waitForJournalBarrierFiles(t *testing.T, iteration int, stage string, paths []string, processes []*journalBarrierProcess) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		ready := 0
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				ready++
			} else if !os.IsNotExist(err) {
				t.Fatalf("iteration %d inspect %s barrier file %s: %v", iteration, stage, path, err)
			}
		}
		if ready == len(paths) {
			return
		}
		for _, process := range processes {
			select {
			case <-process.done:
				t.Fatalf("iteration %d writer %02d exited before %s rendezvous: %v\n%s", iteration, process.result.writer, stage, process.result.err, process.result.output)
			default:
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("iteration %d %s rendezvous timed out at %d/%d", iteration, stage, ready, len(paths))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForJournalBarrierProcess(t *testing.T, iteration int, process *journalBarrierProcess, timeout time.Duration) journalBarrierProcessResult {
	t.Helper()
	select {
	case <-process.done:
		return process.result
	case <-time.After(timeout):
		t.Fatalf("iteration %d writer %02d did not exit within %s", iteration, process.writer, timeout)
		return journalBarrierProcessResult{}
	}
}

func stopJournalBarrierProcesses(processes []*journalBarrierProcess) error {
	for _, process := range processes {
		if err := publishJournalBarrierFile(process.cancelPath, []byte("cancel\n")); err != nil {
			return fmt.Errorf("publish cancel for writer %02d: %w", process.writer, err)
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for _, process := range processes {
		select {
		case <-process.done:
		case <-time.After(time.Until(deadline)):
			return fmt.Errorf("writer %02d did not exit after cancellation", process.writer)
		}
		if _, err := os.Stat(process.publicPIDPath); err == nil {
			if _, err := os.Stat(process.publicReapedPath); err != nil {
				return fmt.Errorf("writer %02d published a public PID without a reaped handshake: %w", process.writer, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect writer %02d public PID marker: %w", process.writer, err)
		}
	}
	return nil
}

func publishJournalBarrierFile(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, content, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			_ = os.Remove(temporaryPath)
			return nil
		}
		return err
	}
	return nil
}

func openWritableSQLite(t *testing.T, databasePath string) *sql.DB {
	t.Helper()
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(databasePath), RawQuery: values.Encode()}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open writable SQLite database: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping writable SQLite database: %v", err)
	}
	return db
}

func openReadOnlySQLite(t *testing.T, databasePath string) *sql.DB {
	t.Helper()
	values := url.Values{}
	values.Set("mode", "ro")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(databasePath), RawQuery: values.Encode()}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open read-only SQLite database: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping read-only SQLite database: %v", err)
	}
	return db
}

func assertSQLiteJournalDurability(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for query, expected := range map[string]int{
		`SELECT COUNT(*) FROM journal_entries`:                want,
		`SELECT COUNT(*) FROM journal_search`:                 want,
		`SELECT COUNT(DISTINCT id) FROM journal_entries`:      want,
		`SELECT COUNT(DISTINCT message) FROM journal_entries`: want,
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != expected {
			t.Fatalf("%s = %d, want %d", query, got, expected)
		}
	}
	var quickCheck string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&quickCheck); err != nil {
		t.Fatalf("PRAGMA quick_check: %v", err)
	}
	if quickCheck != "ok" {
		t.Fatalf("PRAGMA quick_check = %q, want ok", quickCheck)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("PRAGMA foreign_key_check returned a violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PRAGMA foreign_key_check: %v", err)
	}
}

func buildLoafBinary(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(root, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}
	return binary
}

func createMainRepo(t *testing.T, name string) string {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", repoPath, err)
	}
	repoPath = realpath(t, repoPath)
	git(t, repoPath, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	git(t, repoPath, "add", ".")
	git(t, repoPath, "-c", "user.name=Test User", "-c", "user.email=test@test.com", "-c", "commit.gpgsign=false", "commit", "-m", "Initial commit")
	if err := os.MkdirAll(filepath.Join(repoPath, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".agents", "AGENTS.md"), []byte("# Project Instructions\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents/AGENTS.md) error = %v", err)
	}
	return repoPath
}

func addLinkedWorktree(t *testing.T, repoPath string, branch string) string {
	t.Helper()
	worktreePath := filepath.Join(t.TempDir(), branch)
	git(t, repoPath, "worktree", "add", "-b", branch, worktreePath)
	return realpath(t, worktreePath)
}

func seedPreA3WorktreeLayout(t *testing.T, worktreePath string) {
	t.Helper()
	agents := filepath.Join(worktreePath, ".agents")
	if err := os.MkdirAll(filepath.Join(agents, "sessions"), 0o755); err != nil {
		t.Fatalf("MkdirAll(sessions) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(agents, "kb"), 0o755); err != nil {
		t.Fatalf("MkdirAll(kb) error = %v", err)
	}
	for rel, body := range map[string]string{
		"AGENTS.md":                           "# Worktree AGENTS\n",
		"sessions/20260519-120000-session.md": "# Session\n",
		"kb/note.md":                          "# Note\n",
	} {
		if err := os.WriteFile(filepath.Join(agents, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}
}

func seedIdenticalAgentsCheckout(t *testing.T, mainPath string, worktreePath string) {
	t.Helper()
	for rel, body := range map[string]string{
		"AGENTS.md": "# Project Instructions\n",
		"loaf.json": "{\"project\":\"loaf\"}\n",
		"specs/SPEC-036-worktree-aware-agents-storage.md":     "# SPEC-036\n",
		"reports/report-codex-handoff-journal-first-audit.md": "# Report\n",
	} {
		for _, root := range []string{mainPath, worktreePath} {
			target := filepath.Join(root, ".agents", filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(target), err)
			}
			if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
				t.Fatalf("WriteFile(%s) error = %v", target, err)
			}
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s error = %v\n%s", strings.Join(args, " "), err, output)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %q missing go.mod: %v", root, err)
	}
	return root
}

func runCommand(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runBinary(binary string, dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func envWith(overrides ...string) []string {
	blocked := make(map[string]bool, len(overrides))
	for _, override := range overrides {
		if key, _, ok := strings.Cut(override, "="); ok {
			blocked[key] = true
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if ok && blocked[key] {
			continue
		}
		env = append(env, value)
	}
	return append(env, overrides...)
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return realpath
}
