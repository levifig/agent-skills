package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var regexpANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRunnerDoctorHealthyProjectRunsNatively(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor error = %v", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "loaf doctor") || strings.Contains(output, "failed") {
		t.Fatalf("doctor output = %q, want native healthy report", output)
	}
	if strings.Contains(output, "args=doctor") {
		t.Fatalf("doctor output = %q, want no legacy bridge output", output)
	}
}

func TestRunnerDoctorReturnsExitOneForFailures(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorFile(t, filepath.Join(root, ".cursor", "rules", "loaf.mdc"), "legacy\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor error = %v, want exit code 1", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "stale-cursor-mdc") || !strings.Contains(output, "failed") {
		t.Fatalf("doctor failure output = %q, want failing stale-file check", output)
	}
}

func TestRunnerDoctorFixPromptsBeforeEachRepairAndAcceptsYes(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	writeDoctorFile(t, filepath.Join(root, ".cursor", "rules", "loaf.mdc"), "legacy\n")
	var stdout bytes.Buffer

	err := (Runner{Stdout: &stdout, Stdin: strings.NewReader("y\ny\n"), WorkingDir: root}).Run([]string{"doctor", "--fix"})
	if err != nil {
		t.Fatalf("doctor --fix error = %v\n%s", err, stdout.String())
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../AGENTS.md")
	if _, err := os.Lstat(filepath.Join(root, ".cursor", "rules", "loaf.mdc")); !os.IsNotExist(err) {
		t.Fatalf("stale cursor file still exists: %v", err)
	}
	output := stripANSI(stdout.String())
	for _, want := range []string{"Create .claude/CLAUDE.md", "Remove stale .cursor/rules/loaf.mdc", "[y/N]", "2 fixed", "5 passed", "1 skipped"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor --fix output = %q, want %q", output, want)
		}
	}
}

func TestRunnerDoctorFixDeclineLeavesFailureWithoutMutation(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	stale := filepath.Join(root, ".cursor", "rules", "loaf.mdc")
	writeDoctorFile(t, stale, "legacy\n")
	var stdout bytes.Buffer

	err := (Runner{Stdout: &stdout, Stdin: strings.NewReader("n\nn\n"), WorkingDir: root}).Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor --fix error = %v, want exit code 1", err)
	}
	if pathExistsForDoctor(filepath.Join(root, ".claude", "CLAUDE.md")) == true {
		t.Fatal("declined Claude repair mutated the checkout")
	}
	assertInstallFile(t, stale, "legacy\n")
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "2 declined") || !strings.Contains(output, "2 failed") {
		t.Fatalf("doctor --fix output = %q, want declined failures in tally", output)
	}
}

func TestRunnerDoctorFixSupportsMixedAnswersAcrossChecks(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	stale := filepath.Join(root, ".cursor", "rules", "loaf.mdc")
	writeDoctorFile(t, stale, "legacy\n")
	var stdout bytes.Buffer

	err := (Runner{Stdout: &stdout, Stdin: strings.NewReader("y\nn\n"), WorkingDir: root}).Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor --fix error = %v, want exit code 1", err)
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../AGENTS.md")
	assertInstallFile(t, stale, "legacy\n")
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "1 fixed") || !strings.Contains(output, "1 declined") || !strings.Contains(output, "1 failed") {
		t.Fatalf("doctor --fix output = %q, want rechecked fixed check and declined failure tally", output)
	}
}

func TestRunnerDoctorFixDeclinedClaudeMigrationIsNotOfferedAgain(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	claude := filepath.Join(root, ".claude", "CLAUDE.md")
	body := "# Claude Notes\n\n" + doctorFence("9.8.7-test.1")
	writeDoctorFile(t, claude, body)
	var stdout bytes.Buffer

	err := (Runner{Stdout: &stdout, Stdin: strings.NewReader("n\ny\n"), WorkingDir: root}).Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor --fix error = %v, want exit code 1", err)
	}
	assertInstallFile(t, claude, body)
	if isSymlinkForDoctor(claude) {
		t.Fatal("declined Claude migration was applied by a later duplicate-fence check")
	}
	output := stripANSI(stdout.String())
	if prompts := strings.Count(output, "[y/N]"); prompts != 1 {
		t.Fatalf("doctor --fix prompts = %d, want one logical Claude repair prompt\n%s", prompts, output)
	}
	if !strings.Contains(output, "already handled by claude-symlink") || !strings.Contains(output, "1 declined") {
		t.Fatalf("doctor --fix output = %q, want stable repair identity suppression", output)
	}
}

func TestRunnerDoctorFixForceRepairsWithoutPrompt(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	writeDoctorFile(t, filepath.Join(root, ".cursor", "rules", "loaf.mdc"), "legacy\n")
	var stdout bytes.Buffer

	err := (Runner{Stdout: &stdout, Stdin: strings.NewReader("n\nn\n"), WorkingDir: root}).Run([]string{"doctor", "--fix", "--force"})
	if err != nil {
		t.Fatalf("doctor --fix --force error = %v\n%s", err, stdout.String())
	}
	if output := stripANSI(stdout.String()); strings.Contains(output, "[y/N]") || !strings.Contains(output, "2 fixed") {
		t.Fatalf("doctor --fix --force output = %q, want no prompts and both repairs", output)
	}
}

func TestRunnerDoctorFixNonInteractiveSkipsRepairsSafely(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	stale := filepath.Join(root, ".cursor", "rules", "loaf.mdc")
	writeDoctorFile(t, stale, "legacy\n")
	var stdout bytes.Buffer
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()
	defer input.Close()

	err = (Runner{Stdout: &stdout, Stdin: input, WorkingDir: root}).Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor --fix error = %v, want exit code 1", err)
	}
	if pathExistsForDoctor(filepath.Join(root, ".claude", "CLAUDE.md")) {
		t.Fatal("non-interactive doctor created Claude link")
	}
	assertInstallFile(t, stale, "legacy\n")
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "non-interactive") || !strings.Contains(output, "--fix --force") || !strings.Contains(output, "2 repairs skipped") || strings.Contains(output, "declined") || strings.Contains(output, "[y/N]") {
		t.Fatalf("doctor --fix output = %q, want safe non-interactive guidance without prompt", output)
	}
}

func TestRunnerDoctorFixDevNullIsNonInteractive(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	stale := filepath.Join(root, ".cursor", "rules", "loaf.mdc")
	writeDoctorFile(t, stale, "legacy\n")
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var stdout bytes.Buffer

	err = (Runner{Stdout: &stdout, Stdin: input, WorkingDir: root}).Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor --fix </dev/null error = %v, want exit code 1", err)
	}
	if pathExistsForDoctor(filepath.Join(root, ".claude", "CLAUDE.md")) {
		t.Fatal("doctor --fix </dev/null created Claude link")
	}
	assertInstallFile(t, stale, "legacy\n")
	output := stripANSI(stdout.String())
	if strings.Contains(output, "[y/N]") || strings.Contains(output, "declined") || !strings.Contains(output, "2 repairs skipped") || !strings.Contains(output, "non-interactive") {
		t.Fatalf("doctor --fix </dev/null output = %q, want skipped non-interactive repairs", output)
	}
}

func TestRunnerDoctorRejectsForceWithoutFix(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	for _, args := range [][]string{{"doctor", "--force"}, {"doctor", "--help", "--force"}} {
		err := (Runner{WorkingDir: root}).Run(args)
		if err == nil || !strings.Contains(err.Error(), "--force requires --fix") {
			t.Fatalf("%v error = %v, want clear usage error", args, err)
		}
	}
}

func TestRunnerDoctorWarningsDoNotFail(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("0.0.1-test"))
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor warning-only error = %v, want nil", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "Fenced section version drift") || strings.Contains(output, "failed") {
		t.Fatalf("doctor warning output = %q, want warning-only success", output)
	}
}

func TestCheckFencedVersionAcceptsFingerprintedAndLegacyHeaders(t *testing.T) {
	const currentVersion = "9.8.7-test.1"
	for _, tc := range []struct {
		name        string
		body        string
		wantStatus  doctorStatus
		wantMessage string
		reject      string
	}{
		{name: "fingerprinted current", body: generateFencedContent(currentVersion), wantStatus: doctorPass, wantMessage: "matches installed"},
		{name: "fingerprinted drift", body: generateFencedContent("0.0.1-test"), wantStatus: doctorWarn, wantMessage: "Fenced section version drift", reject: "No loaf:managed fenced section"},
		{name: "legacy current", body: doctorFence(currentVersion), wantStatus: doctorPass, wantMessage: "matches installed"},
		{name: "malformed fingerprint", body: "<!-- loaf:managed:start v9.8.7-test.1 sha256=bad -->\nbody\n<!-- loaf:managed:end -->\n", wantStatus: doctorWarn, wantMessage: "No loaf:managed fenced section"},
		{name: "malformed header", body: "<!-- loaf:managed:start v9.8.7-test.1 extra -->\nbody\n<!-- loaf:managed:end -->\n", wantStatus: doctorWarn, wantMessage: "No loaf:managed fenced section"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeDoctorFixture(t, currentVersion)
			writeDoctorAgents(t, root, tc.body)
			result := checkFencedVersion(currentVersion).Run(doctorContext{projectRoot: root})
			if result.Status != tc.wantStatus || !strings.Contains(result.Message, tc.wantMessage) || tc.reject != "" && strings.Contains(result.Message, tc.reject) {
				t.Fatalf("fenced-version result = %#v, want status %q containing %q and not %q", result, tc.wantStatus, tc.wantMessage, tc.reject)
			}
		})
	}
}

func TestRunnerDoctorFixMigratesLegacyLayout(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorFile(t, filepath.Join(root, ".agents", "AGENTS.md"), "# Legacy Canonical\n\nlegacy context\n")
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	writeDoctorFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# Claude Notes\n\nclaude context\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor", "--fix", "--force"})
	if err != nil {
		t.Fatalf("doctor --fix legacy migration error = %v", err)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	for _, want := range []string{"legacy context", "claude context", "## Migrated from .claude/CLAUDE.md"} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("canonical = %q, want %q", canonical, want)
		}
	}
	if info, err := os.Lstat(filepath.Join(root, "AGENTS.md")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("root AGENTS.md must be a real file after migration: info=%v err=%v", info, err)
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../AGENTS.md")
	for _, path := range []string{".agents/AGENTS.md.bak", ".claude/CLAUDE.md.bak"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("doctor backup %s missing: %v", path, err)
		}
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "fixed") || strings.Contains(output, "failed") {
		t.Fatalf("doctor --fix output = %q, want fixes and clean report", output)
	}
}

func TestRunnerDoctorFixStripsDuplicateFencedSection(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	writeDoctorFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# claude\n\n<!-- loaf:managed:start v0.1.0 -->\nold fence\n<!-- loaf:managed:end -->\n\ntrailing text\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor", "--fix", "--force"})
	if err != nil {
		t.Fatalf("doctor --fix duplicate fence error = %v", err)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	if strings.Contains(canonical, "old fence") {
		t.Fatalf("canonical = %q, want old managed fence stripped", canonical)
	}
	if !strings.Contains(canonical, "trailing text") {
		t.Fatalf("canonical = %q, want user text preserved", canonical)
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../AGENTS.md")
}

func TestRunnerDoctorFixReplacesDanglingLegacyRootSymlink(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"doctor", "--fix", "--force"})
	if err != nil {
		t.Fatalf("doctor --fix dangling legacy link error = %v\n%s", err, stdout.String())
	}
	info, err := os.Lstat(filepath.Join(root, "AGENTS.md"))
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("root AGENTS.md must be a real file after repair: info=%v err=%v", info, err)
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../AGENTS.md")
}

func TestRunnerDoctorRejectsDirectoryCanonical(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	mkdirAll(t, filepath.Join(root, "AGENTS.md"))
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor error = %v, want exit code 1", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "AGENTS.md is a directory") || strings.Contains(output, "Created .claude/CLAUDE.md") {
		t.Fatalf("doctor output = %q, want non-fixable directory diagnostic without Claude link creation", output)
	}
}

func TestRunnerDoctorPreservesExistingLegacyBackupAndDoesNotDuplicateOnRetry(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1")+"# Root Notes\n")
	legacy := filepath.Join(root, ".agents", "AGENTS.md")
	writeDoctorFile(t, legacy, "# Legacy Notes\n")
	writeDoctorFile(t, legacy+".bak", "# Earlier Backup\n")
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"doctor", "--fix", "--force"}); err != nil {
		t.Fatalf("doctor --fix error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, legacy+".bak", "# Earlier Backup\n")
	assertInstallFile(t, legacy+".bak.1", "# Legacy Notes\n")

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"doctor", "--fix", "--force"}); err != nil {
		t.Fatalf("second doctor --fix error = %v\n%s", err, stdout.String())
	}
	body := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	if count := strings.Count(body, "## Migrated from .agents/AGENTS.md"); count != 1 {
		t.Fatalf("canonical migration headings = %d, want 1\n%s", count, body)
	}
}

func TestRunnerDoctorVerboseAndHelpAreNative(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "verbose", args: []string{"doctor", "--verbose"}, want: "canonical-agents-file"},
		{name: "help", args: []string{"doctor", "--help"}, want: "Usage: loaf doctor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: root,
			}.Run(tc.args)
			if err != nil {
				t.Fatalf("%v error = %v", tc.args, err)
			}
			if output := stripANSI(stdout.String()); !strings.Contains(output, tc.want) {
				t.Fatalf("%v output = %q, want %q", tc.args, output, tc.want)
			}
		})
	}
}

func TestRunnerDoctorRejectsUnknownOptionsNatively(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	err := Runner{
		WorkingDir: root,
	}.Run([]string{"doctor", "--wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown doctor option") {
		t.Fatalf("doctor unknown option error = %v, want native option error", err)
	}
}

func writeDoctorFixture(t *testing.T, version string) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"`+version+`"}`+"\n")
	return root
}

func writeDoctorAgents(t *testing.T, root string, body string) {
	t.Helper()
	writeDoctorFile(t, filepath.Join(root, "AGENTS.md"), body)
}

func doctorFence(version string) string {
	return strings.Join([]string{
		"<!-- loaf:managed:start v" + version + " -->",
		"<!-- Maintained by loaf install/upgrade - do not edit manually -->",
		"## Loaf Framework",
		"",
		"Sample fenced content.",
		"<!-- loaf:managed:end -->",
		"",
	}, "\n")
}

func symlinkFile(t *testing.T, target string, linkPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("Symlink(%s, %s) error = %v", target, linkPath, err)
	}
}

func writeDoctorFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, body)
}

func assertSymlinkTarget(t *testing.T, linkPath string, want string) {
	t.Helper()
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat(%s) error = %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", linkPath)
	}
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s) error = %v", linkPath, err)
	}
	if got != want {
		t.Fatalf("Readlink(%s) = %q, want %q", linkPath, got, want)
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return body
}

func stripANSI(value string) string {
	return regexpANSI.ReplaceAllString(value, "")
}
